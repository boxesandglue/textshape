package ot

// Shaper and Buffer Implementation
//
// This file implements the text shaping pipeline and buffer management.
// HarfBuzz equivalent files:
//   - hb-buffer.cc / hb-buffer.hh (Buffer operations, Lines 60-400)
//   - hb-ot-shape.cc (Shaping pipeline, Lines 600-1200)
//   - hb-ot-shaper.hh (Shaper selection and hooks)
//
// Key structures:
//   - Buffer: Holds glyphs being shaped, implements two-buffer pattern (Line ~60)
//   - Shaper: Main shaping engine with font tables (Line ~339)
//
// Buffer operations (HarfBuzz hb-buffer.cc):
//   - clearOutput: Initialize output buffer (Line ~240)
//   - moveTo: Move to position, copying glyphs (Line ~305)
//   - nextGlyph: Copy current glyph to output (Line ~271)
//   - outputGlyph: Copy with new GlyphID (Line ~258)
//   - sync: Replace input with output (Line ~282)
//
// Shaping pipeline (HarfBuzz hb-ot-shape.cc):
//   - Shape: Main entry point (Line ~580)
//   - shapeDefault: Default shaper for Latin/Cyrillic (Line ~636)
//   - shapeArabic: Arabic shaper (Line ~752)
//   - shapeIndic: Indic shaper (see indic.go)

import (
	"fmt"
	"sync"
	"unicode"
)

// Debug flag for GPOS debugging - set to true to enable debug output
var debugGPOS = false

// SetDebugGPOS enables or disables debug output for GPOS/Arabic processing.
func SetDebugGPOS(enabled bool) {
	debugGPOS = enabled
}

func debugPrintf(format string, args ...interface{}) {
	if debugGPOS {
		fmt.Printf(format, args...)
	}
}

// Note: Direction, DirectionLTR, DirectionRTL are defined in gpos.go

// GlyphInfo holds information about a shaped glyph.
// HarfBuzz equivalent: hb_glyph_info_t in hb-buffer.h
type GlyphInfo struct {
	Codepoint  Codepoint // Original Unicode codepoint (0 if synthetic)
	GlyphID    GlyphID   // Glyph index in the font
	Cluster    int       // Cluster index (maps back to original text position)
	GlyphClass int       // GDEF glyph class (if available)
	Mask       uint32    // Feature mask - determines which features apply to this glyph
	// HarfBuzz equivalent: hb_glyph_info_t.mask in hb-buffer.h
	// Each feature has a unique mask bit. Lookups only apply to glyphs
	// where (glyph.mask & lookup.mask) != 0.

	// GlyphProps holds glyph properties for GSUB/GPOS processing.
	// HarfBuzz equivalent: glyph_props() in hb-ot-layout.hh
	// Flags:
	//   0x02 = BASE_GLYPH (from GDEF class 1)
	//   0x04 = LIGATURE (from GDEF class 2)
	//   0x08 = MARK (from GDEF class 3)
	//   0x10 = SUBSTITUTED (glyph was substituted by GSUB)
	//   0x20 = LIGATED (glyph is result of ligature substitution)
	//   0x40 = MULTIPLIED (glyph is component of multiple substitution)
	GlyphProps uint16

	// LigProps holds ligature properties (lig_id and lig_comp).
	// HarfBuzz equivalent: lig_props() in hb-ot-layout.hh
	// Upper 3 bits: lig_id (identifies which ligature this belongs to)
	// Lower 5 bits: lig_comp (component index within ligature, 0 = the ligature itself)
	LigProps uint8

	// Syllable holds the syllable index for complex script shaping.
	// HarfBuzz equivalent: syllable() in hb-buffer.hh
	// Used by Indic, Khmer, Myanmar, USE shapers to constrain GSUB lookups
	// to operate only within syllable boundaries (F_PER_SYLLABLE flag).
	Syllable uint8

	// ModifiedCCC holds an overridden combining class, used by Arabic reorder_marks.
	// HarfBuzz equivalent: _hb_glyph_info_set_modified_combining_class()
	// When non-zero, this value is used instead of the standard Unicode CCC.
	// Arabic MCMs (modifier combining marks) get their CCC changed to 22/26
	// after being reordered to the beginning of the mark sequence.
	ModifiedCCC uint8

	// IndicCategory holds the Indic character category for Indic shaping.
	// HarfBuzz equivalent: indic_category() stored in var1 via HB_BUFFER_ALLOCATE_VAR
	// This is preserved through GSUB substitutions because GlyphInfo is copied as a whole.
	IndicCategory uint8

	// IndicPosition holds the Indic character position for Indic shaping.
	// HarfBuzz equivalent: indic_position() stored in var1 via HB_BUFFER_ALLOCATE_VAR
	// This is preserved through GSUB substitutions because GlyphInfo is copied as a whole.
	IndicPosition uint8

	// ArabicShapingAction holds the Arabic shaping action for STCH (stretching) feature.
	// HarfBuzz equivalent: arabic_shaping_action() stored via ot_shaper_var_u8_auxiliary()
	// Values: arabicActionSTCH_FIXED, arabicActionSTCH_REPEATING (set by recordStch)
	ArabicShapingAction uint8

	// MyanmarCategory holds the Myanmar character category for Myanmar shaping.
	// HarfBuzz equivalent: myanmar_category() stored via ot_shaper_var_u8_category()
	MyanmarCategory uint8

	// MyanmarPosition holds the Myanmar character position for Myanmar shaping.
	// HarfBuzz equivalent: myanmar_position() stored via ot_shaper_var_u8_auxiliary()
	MyanmarPosition uint8
}

// Glyph property constants.
// HarfBuzz equivalent: HB_OT_LAYOUT_GLYPH_PROPS_* in hb-ot-layout.hh
const (
	GlyphPropsBaseGlyph        uint16 = 0x02 // GDEF class 1: Base glyph
	GlyphPropsLigature         uint16 = 0x04 // GDEF class 2: Ligature glyph
	GlyphPropsMark             uint16 = 0x08 // GDEF class 3: Mark glyph
	GlyphPropsSubstituted      uint16 = 0x10 // Glyph was substituted by GSUB
	GlyphPropsLigated          uint16 = 0x20 // Glyph is result of ligature substitution
	GlyphPropsMultiplied       uint16 = 0x40 // Glyph is component of multiple substitution
	GlyphPropsDefaultIgnorable uint16 = 0x80 // Unicode default ignorable character
	// HarfBuzz UPROPS_MASK_Cf_ZWNJ and UPROPS_MASK_Cf_ZWJ in hb-ot-layout.hh
	GlyphPropsZWNJ uint16 = 0x100 // Zero-Width Non-Joiner (U+200C)
	GlyphPropsZWJ  uint16 = 0x200 // Zero-Width Joiner (U+200D)
	// HarfBuzz UPROPS_MASK_HIDDEN in hb-ot-layout.hh:199
	// Set for: CGJ (U+034F), Mongolian FVS (U+180B-U+180D, U+180F), TAG chars (U+E0020-U+E007F)
	// These should NOT be skipped during GSUB context matching (ignore_hidden=false for GSUB)
	GlyphPropsHidden uint16 = 0x400

	// GlyphPropsPreserve are the flags preserved across substitutions
	GlyphPropsPreserve uint16 = GlyphPropsSubstituted | GlyphPropsLigated | GlyphPropsMultiplied | GlyphPropsDefaultIgnorable | GlyphPropsZWNJ | GlyphPropsZWJ | GlyphPropsHidden
)

// IsMultiplied returns true if this glyph is a component of a multiple substitution.
// HarfBuzz equivalent: _hb_glyph_info_multiplied() in hb-ot-layout.hh:565
func (g *GlyphInfo) IsMultiplied() bool {
	return g.GlyphProps&GlyphPropsMultiplied != 0
}

// IsLigated returns true if this glyph is the result of a ligature substitution.
// HarfBuzz equivalent: _hb_glyph_info_ligated() in hb-ot-layout.hh:561
func (g *GlyphInfo) IsLigated() bool {
	return g.GlyphProps&GlyphPropsLigated != 0
}

// IsMark returns true if this glyph is a mark (GDEF class 3).
// HarfBuzz equivalent: _hb_glyph_info_is_mark() in hb-ot-layout.hh:549
func (g *GlyphInfo) IsMark() bool {
	return g.GlyphProps&GlyphPropsMark != 0
}

// IsBaseGlyph returns true if this glyph is a base glyph (GDEF class 1).
// HarfBuzz equivalent: _hb_glyph_info_is_base_glyph() in hb-ot-layout.hh:537
func (g *GlyphInfo) IsBaseGlyph() bool {
	return g.GlyphProps&GlyphPropsBaseGlyph != 0
}

// IsLigature returns true if this glyph is a ligature (GDEF class 2).
// HarfBuzz equivalent: _hb_glyph_info_is_ligature() in hb-ot-layout.hh:543
func (g *GlyphInfo) IsLigature() bool {
	return g.GlyphProps&GlyphPropsLigature != 0
}

// GetLigID returns the ligature ID for this glyph.
// HarfBuzz equivalent: _hb_glyph_info_get_lig_id() in hb-ot-layout.hh:481
func (g *GlyphInfo) GetLigID() uint8 {
	return g.LigProps >> 5
}

// LigProps bit layout (HarfBuzz equivalent in hb-ot-layout.hh:425-448):
//   Bits 7-5: lig_id (3 bits, values 0-7)
//   Bit 4:    IS_LIG_BASE (set for ligature glyphs, unset for marks/components)
//   Bits 3-0: lig_comp or lig_num_comps (4 bits, values 0-15)
const isLigBase uint8 = 0x10 // HarfBuzz: IS_LIG_BASE

// GetLigComp returns the ligature component index for this glyph.
// Returns 0 if this is the ligature itself (IS_LIG_BASE set).
// HarfBuzz equivalent: _hb_glyph_info_get_lig_comp() in hb-ot-layout.hh:493
func (g *GlyphInfo) GetLigComp() uint8 {
	// If IS_LIG_BASE is set, this is a ligature glyph, not a component
	// HarfBuzz: if (_hb_glyph_info_ligated_internal(info)) return 0;
	if g.LigProps&isLigBase != 0 {
		return 0
	}
	return g.LigProps & 0x0F
}

// SetLigPropsForComponent sets lig_props for a component of a multiple substitution.
// HarfBuzz equivalent: _hb_glyph_info_set_lig_props_for_component() in hb-ot-layout.hh:475
// which calls _hb_glyph_info_set_lig_props_for_mark(info, 0, comp)
func (g *GlyphInfo) SetLigPropsForComponent(compIdx int) {
	// lig_id = 0, lig_comp = compIdx (0-based)
	// HarfBuzz: info->lig_props() = (lig_id << 5) | (lig_comp & 0x0F)
	g.LigProps = uint8(compIdx & 0x0F)
}

// SetLigPropsForLigature sets lig_props for a ligature glyph.
// HarfBuzz equivalent: _hb_glyph_info_set_lig_props_for_ligature() in hb-ot-layout.hh:459
func (g *GlyphInfo) SetLigPropsForLigature(ligID uint8, numComps int) {
	// HarfBuzz: info->lig_props() = (lig_id << 5) | IS_LIG_BASE | (lig_num_comps & 0x0F)
	g.LigProps = (ligID << 5) | isLigBase | uint8(numComps&0x0F)
}

// SetLigPropsForMark sets lig_props for a mark glyph attached to a ligature.
// HarfBuzz equivalent: _hb_glyph_info_set_lig_props_for_mark() in hb-ot-layout.hh:467
func (g *GlyphInfo) SetLigPropsForMark(ligID uint8, ligComp int) {
	// HarfBuzz: info->lig_props() = (lig_id << 5) | (lig_comp & 0x0F)
	g.LigProps = (ligID << 5) | uint8(ligComp&0x0F)
}

// GetLigNumComps returns the number of components in a ligature.
// HarfBuzz equivalent: _hb_glyph_info_get_lig_num_comps() in hb-ot-layout.hh:501
func (g *GlyphInfo) GetLigNumComps() int {
	// For ligatures (GDEF class 2 + IS_LIG_BASE), return lig_num_comps from lower 4 bits
	// HarfBuzz: if ((glyph_props & LIGATURE) && ligated_internal) return lig_props & 0x0F
	if (g.GlyphProps&GlyphPropsLigature) != 0 && (g.LigProps&isLigBase) != 0 {
		return int(g.LigProps & 0x0F)
	}
	return 1
}

// GlyphPos holds positioning information for a shaped glyph.
// HarfBuzz equivalent: hb_glyph_position_t in hb-buffer.h
type GlyphPos struct {
	XAdvance int16 // Horizontal advance
	YAdvance int16 // Vertical advance
	XOffset  int16 // Horizontal offset
	YOffset  int16 // Vertical offset

	// Attachment chain for mark/cursive positioning.
	// HarfBuzz: var.i16[0] via attach_chain() macro in OT/Layout/GPOS/Common.hh
	// Relative offset to attached glyph: negative = backwards, positive = forward.
	// Zero means no attachment.
	AttachChain int16

	// Attachment type for mark/cursive positioning.
	// HarfBuzz: var.u8[2] via attach_type() macro in OT/Layout/GPOS/Common.hh
	AttachType uint8
}

// BufferFlags controls buffer behavior during shaping.
// These match HarfBuzz's hb_buffer_flags_t.
type BufferFlags uint32

const (
	// BufferFlagDefault is the default buffer flag.
	BufferFlagDefault BufferFlags = 0
	// BufferFlagBOT indicates beginning of text paragraph.
	BufferFlagBOT BufferFlags = 1 << iota
	// BufferFlagEOT indicates end of text paragraph.
	BufferFlagEOT
	// BufferFlagPreserveDefaultIgnorables keeps default ignorable characters visible.
	BufferFlagPreserveDefaultIgnorables
	// BufferFlagRemoveDefaultIgnorables removes default ignorable characters from output.
	BufferFlagRemoveDefaultIgnorables
	// BufferFlagDoNotInsertDottedCircle prevents dotted circle insertion for invalid sequences.
	BufferFlagDoNotInsertDottedCircle
)

// Buffer holds a sequence of glyphs being shaped.
type Buffer struct {
	Info      []GlyphInfo
	Pos       []GlyphPos
	Direction Direction
	Flags     BufferFlags

	// Idx is the cursor into Info and Pos arrays.
	// HarfBuzz: hb_buffer_t::idx (hb-buffer.hh line 97)
	Idx int

	// Output buffer for in-place modifications
	// HarfBuzz: hb_buffer_t::out_info, out_len, have_output (hb-buffer.hh lines 93-102)
	outInfo    []GlyphInfo
	outLen     int
	haveOutput bool

	// Serial counter for ligature IDs
	// HarfBuzz: hb_buffer_t::serial (hb-buffer.hh line 109)
	serial uint8

	// Script and Language for shaping (optional, can be auto-detected)
	Script   Tag
	Language Tag

	// ScratchFlags holds temporary flags used during shaping.
	// HarfBuzz equivalent: scratch_flags in hb-buffer.hh
	ScratchFlags ScratchFlags
}

// ScratchFlags are temporary flags used during shaping.
type ScratchFlags uint32

const (
	// ScratchFlagArabicHasStch indicates buffer has STCH glyphs that need post-processing.
	ScratchFlagArabicHasStch ScratchFlags = 1 << 0
)

// NewBuffer creates a new empty buffer.
// Direction is initially unset (0) and should be set explicitly or via GuessSegmentProperties.
func NewBuffer() *Buffer {
	return &Buffer{
		// Direction is 0 (unset) - will be determined by GuessSegmentProperties or shaper
	}
}

// AddCodepoints adds Unicode codepoints to the buffer.
// Marks (Unicode category M) are assigned to the same cluster as the preceding base character.
func (b *Buffer) AddCodepoints(codepoints []Codepoint) {
	// HarfBuzz: cluster = index into input text (hb-buffer.cc:1858)
	// No mark grouping here - clusters are merged during shaping (ligatures, etc.)
	for i, cp := range codepoints {
		info := GlyphInfo{
			Codepoint: cp,
			Cluster:   i,
			Mask:      MaskGlobal, // HarfBuzz: glyphs start with global_mask
		}
		// HarfBuzz: _hb_glyph_info_set_unicode_props() sets UPROPS_MASK_IGNORABLE and Cf flags
		if IsDefaultIgnorable(cp) {
			info.GlyphProps |= GlyphPropsDefaultIgnorable
		}
		if cp == 0x200C { // ZWNJ
			info.GlyphProps |= GlyphPropsZWNJ
		}
		if cp == 0x200D { // ZWJ
			info.GlyphProps |= GlyphPropsZWJ
		}
		// HarfBuzz: UPROPS_MASK_HIDDEN for CGJ, Mongolian FVS, TAG chars
		// These should NOT be skipped during GSUB context matching
		if isHiddenDefaultIgnorable(cp) {
			info.GlyphProps |= GlyphPropsHidden
		}
		b.Info = append(b.Info, info)
	}
	b.Pos = make([]GlyphPos, len(b.Info))
}

// AddString adds a string to the buffer.
// Marks (Unicode category M) are assigned to the same cluster as the preceding base character.
func (b *Buffer) AddString(s string) {
	// HarfBuzz: cluster = index into input text (hb-buffer.cc:1858)
	// No mark grouping here - clusters are merged during shaping (ligatures, etc.)
	runes := []rune(s)
	for i, r := range runes {
		cp := Codepoint(r)
		info := GlyphInfo{
			Codepoint: cp,
			Cluster:   i,
			Mask:      MaskGlobal, // HarfBuzz: glyphs start with global_mask
		}
		// HarfBuzz: _hb_glyph_info_set_unicode_props() sets UPROPS_MASK_IGNORABLE and Cf flags
		if IsDefaultIgnorable(cp) {
			info.GlyphProps |= GlyphPropsDefaultIgnorable
		}
		if cp == 0x200C { // ZWNJ
			info.GlyphProps |= GlyphPropsZWNJ
		}
		if cp == 0x200D { // ZWJ
			info.GlyphProps |= GlyphPropsZWJ
		}
		// HarfBuzz: UPROPS_MASK_HIDDEN for CGJ, Mongolian FVS, TAG chars
		// These should NOT be skipped during GSUB context matching
		if isHiddenDefaultIgnorable(cp) {
			info.GlyphProps |= GlyphPropsHidden
		}
		b.Info = append(b.Info, info)
	}
	b.Pos = make([]GlyphPos, len(b.Info))
}

// SetDirection sets the text direction.
func (b *Buffer) SetDirection(dir Direction) {
	b.Direction = dir
}

// Len returns the number of glyphs in the buffer.
func (b *Buffer) Len() int {
	return len(b.Info)
}

// Clear removes all glyphs from the buffer.
func (b *Buffer) Clear() {
	b.Info = b.Info[:0]
	b.Pos = b.Pos[:0]
}

// Reset clears the buffer and resets all properties to defaults.
func (b *Buffer) Reset() {
	b.Info = b.Info[:0]
	b.Pos = b.Pos[:0]
	b.Direction = 0 // Unset - will be determined by GuessSegmentProperties or shaper
	b.Flags = BufferFlagDefault
	b.Script = 0
	b.Language = 0
	b.serial = 0
	b.ScratchFlags = 0
}

// Reverse reverses the order of glyphs in the buffer.
// HarfBuzz equivalent: hb_buffer_reverse() in hb-buffer.cc:387
func (b *Buffer) Reverse() {
	for i, j := 0, len(b.Info)-1; i < j; i, j = i+1, j-1 {
		b.Info[i], b.Info[j] = b.Info[j], b.Info[i]
		b.Pos[i], b.Pos[j] = b.Pos[j], b.Pos[i]
	}
}

// ReverseRange reverses the order of glyphs in the range [start, end).
// HarfBuzz equivalent: buffer->reverse_range() in hb-buffer.hh:242-247
func (b *Buffer) ReverseRange(start, end int) {
	if start >= end || start < 0 || end > len(b.Info) {
		return
	}
	for i, j := start, end-1; i < j; i, j = i+1, j-1 {
		b.Info[i], b.Info[j] = b.Info[j], b.Info[i]
	}
	if len(b.Pos) >= end {
		for i, j := start, end-1; i < j; i, j = i+1, j-1 {
			b.Pos[i], b.Pos[j] = b.Pos[j], b.Pos[i]
		}
	}
}

// AllocateLigID allocates a new ligature ID.
// HarfBuzz equivalent: _hb_allocate_lig_id() in hb-ot-layout.hh:512
func (b *Buffer) AllocateLigID() uint8 {
	b.serial++
	ligID := b.serial & 0x07 // Only 3 bits for lig_id
	if ligID == 0 {
		// Zero is reserved for "no ligature", try again
		return b.AllocateLigID()
	}
	return ligID
}

// MergeClusters merges clusters in the range [start, end).
// All glyphs in the range are assigned the minimum cluster value found in the range.
// HarfBuzz equivalent: hb_buffer_t::merge_clusters_impl() in hb-buffer.cc:547-582
func (b *Buffer) MergeClusters(start, end int) {
	if end-start < 2 {
		return
	}
	if start < 0 || end > len(b.Info) {
		return
	}

	// Find minimum cluster in range
	minCluster := b.Info[start].Cluster
	for i := start + 1; i < end; i++ {
		if b.Info[i].Cluster < minCluster {
			minCluster = b.Info[i].Cluster
		}
	}

	// Extend end: If the last glyph in range has a different cluster than the minimum,
	// extend the range to include following glyphs with the same cluster.
	// HarfBuzz: hb-buffer.cc:565-568
	if minCluster != b.Info[end-1].Cluster {
		for end < len(b.Info) && b.Info[end-1].Cluster == b.Info[end].Cluster {
			end++
		}
	}

	// Set all glyphs in extended range to the minimum cluster
	for i := start; i < end; i++ {
		b.Info[i].Cluster = minCluster
	}
}

// mergeClustersSlice merges clusters in a GlyphInfo slice (used during normalization).
// HarfBuzz equivalent: hb_buffer_t::merge_clusters_impl() in hb-buffer.cc:547-582
// This is a standalone version for use with temporary slices during normalization.
func mergeClustersSlice(info []GlyphInfo, start, end int) {
	if end-start < 2 {
		return
	}
	if start < 0 || end > len(info) {
		return
	}

	// Find minimum cluster in range
	minCluster := info[start].Cluster
	for i := start + 1; i < end; i++ {
		if info[i].Cluster < minCluster {
			minCluster = info[i].Cluster
		}
	}

	// Extend end: If the last glyph in range has a different cluster than the minimum,
	// extend the range to include following glyphs with the same cluster.
	if minCluster != info[end-1].Cluster {
		for end < len(info) && info[end-1].Cluster == info[end].Cluster {
			end++
		}
	}

	// Set all glyphs in extended range to the minimum cluster
	for i := start; i < end; i++ {
		info[i].Cluster = minCluster
	}
}

// isContinuation checks if a codepoint is a grapheme continuation character.
// HarfBuzz equivalent: hb_set_unicode_props() in hb-ot-shape.cc:470-546
// HarfBuzz marks these as CONTINUATION (merged into previous grapheme cluster):
// - Marks (Mn, Mc, Me) - always continuations
// - ZWJ (U+200D) - Zeile 515-517
// - Emoji_Modifiers (U+1F3FB-U+1F3FF) - Zeile 501-505
// - Tags (U+E0020-U+E007F) and Katakana voiced (U+FF9E-U+FF9F) - Zeile 541-542
// Note: Regional Indicators and Extended_Pictographic after ZWJ are handled
// separately but we skip that for now as it's mainly for emoji sequences.
func isContinuation(cp Codepoint) bool {
	// Marks (Mn, Mc, Me)
	if unicode.Is(unicode.M, rune(cp)) {
		return true
	}
	// ZWJ (Zero Width Joiner)
	if cp == 0x200D {
		return true
	}
	// Emoji_Modifiers (skin tone modifiers)
	if cp >= 0x1F3FB && cp <= 0x1F3FF {
		return true
	}
	// Tags (for emoji sub-region flags)
	if cp >= 0xE0020 && cp <= 0xE007F {
		return true
	}
	// Katakana voiced/semi-voiced marks
	if cp == 0xFF9E || cp == 0xFF9F {
		return true
	}
	return false
}

// formClusters merges clusters for grapheme groups (base + continuations).
// HarfBuzz equivalent: hb_form_clusters() in hb-ot-shape.cc:577-589
// This ensures that a base character and its continuations share the same cluster.
func formClusters(buf *Buffer) {
	if len(buf.Info) < 2 {
		return
	}

	// Find grapheme boundaries and merge clusters
	// A grapheme is: base character + any following continuations
	// HarfBuzz uses _hb_glyph_info_is_continuation() which checks for continuation flag
	start := 0
	for i := 1; i < len(buf.Info); i++ {
		// Check if this is a continuation
		if isContinuation(buf.Info[i].Codepoint) {
			// This is a continuation - continue the current grapheme
			continue
		}
		// This is a new base - merge the previous grapheme's clusters
		if i > start+1 {
			buf.MergeClusters(start, i)
		}
		start = i
	}
	// Merge the last grapheme
	if len(buf.Info) > start+1 {
		buf.MergeClusters(start, len(buf.Info))
	}
}

// GuessSegmentProperties guesses direction, script, and language from buffer content.
// This is similar to HarfBuzz's hb_buffer_guess_segment_properties().
func (b *Buffer) GuessSegmentProperties() {
	if len(b.Info) == 0 {
		return
	}

	// Guess script from buffer contents
	// HarfBuzz equivalent: hb_buffer_t::guess_segment_properties() in hb-buffer.cc:703-732
	if b.Script == 0 {
		for _, info := range b.Info {
			script := GetScriptTag(info.Codepoint)
			// Skip Common (0) and Inherited scripts - they don't determine the script
			if script != 0 {
				b.Script = script
				break
			}
		}
	}

	// Guess direction from script
	// HarfBuzz: props.direction = hb_script_get_horizontal_direction(props.script)
	if b.Direction == 0 {
		b.Direction = GetHorizontalDirection(b.Script)
		// If direction is still 0 (invalid), default to LTR
		if b.Direction == 0 {
			b.Direction = DirectionLTR
		}
	}
}

// GlyphIDs returns just the glyph IDs.
func (b *Buffer) GlyphIDs() []GlyphID {
	ids := make([]GlyphID, len(b.Info))
	for i, info := range b.Info {
		ids[i] = info.GlyphID
	}
	return ids
}

// Codepoints returns a slice of codepoints from the buffer.
func (b *Buffer) Codepoints() []Codepoint {
	cps := make([]Codepoint, len(b.Info))
	for i, info := range b.Info {
		cps[i] = info.Codepoint
	}
	return cps
}

// clearOutput initializes the output buffer for in-place modifications.
// HarfBuzz equivalent: hb_buffer_t::clear_output() in hb-buffer.cc:393
func (b *Buffer) clearOutput() {
	b.haveOutput = true
	b.Idx = 0
	b.outLen = 0
	// In HarfBuzz, out_info initially points to info (in-place)
	// We use a separate slice but will sync it back later
	if cap(b.outInfo) < len(b.Info) {
		b.outInfo = make([]GlyphInfo, 0, len(b.Info)+10)
	}
	b.outInfo = b.outInfo[:0]
}

// BacktrackLen returns the number of glyphs available for backtrack matching.
// HarfBuzz equivalent: hb_buffer_t::backtrack_len() in hb-buffer.hh:232
// When have_output is true, this returns out_len (output buffer size).
// When have_output is false, this returns idx (current position in input).
func (b *Buffer) BacktrackLen() int {
	if b.haveOutput {
		return b.outLen
	}
	return b.Idx
}

// BacktrackInfo returns the GlyphInfo at the given backtrack position.
// HarfBuzz: prev() uses out_info when have_output is true.
// This is needed because backtrack matching uses the output buffer, not input!
func (b *Buffer) BacktrackInfo(pos int) *GlyphInfo {
	if b.haveOutput {
		if pos < 0 || pos >= b.outLen {
			return nil
		}
		return &b.outInfo[pos]
	}
	// No output buffer, use input buffer
	if pos < 0 || pos >= len(b.Info) {
		return nil
	}
	return &b.Info[pos]
}

// HaveOutput returns whether the buffer is in output mode.
func (b *Buffer) HaveOutput() bool {
	return b.haveOutput
}

// nextGlyph copies the current glyph (at Idx) to output and advances Idx.
// HarfBuzz equivalent: hb_buffer_t::next_glyph() in hb-buffer.hh:350-364
func (b *Buffer) nextGlyph() {
	if b.haveOutput {
		// Copy current glyph info to output (preserves cluster!)
		b.outInfo = append(b.outInfo, b.Info[b.Idx])
		b.outLen++
	}
	b.Idx++
}

// outputGlyph inserts a glyph into output without advancing Idx.
// The new glyph inherits properties from the current glyph (Idx).
// HarfBuzz equivalent: hb_buffer_t::output_glyph() in hb-buffer.hh:328-329
//
// This is equivalent to replace_glyphs(0, 1, &glyph) in HarfBuzz:
// - Copies all properties (cluster, mask, etc.) from current glyph
// - Sets the new GlyphID
// - Adds to output buffer
// - Does NOT advance Idx (caller must do that)
func (b *Buffer) outputGlyph(glyphID GlyphID) {
	// Copy current glyph's properties (preserves cluster!)
	// HarfBuzz: *pinfo = orig_info (hb-buffer.hh:314)
	info := b.Info[b.Idx]
	info.GlyphID = glyphID
	b.outInfo = append(b.outInfo, info)
	b.outLen++
}

// outputInfo appends a GlyphInfo directly to the output buffer.
// Unlike outputGlyph(), this does NOT copy from current glyph.
// HarfBuzz equivalent: hb_buffer_t::output_info() in hb-buffer.hh:331
func (b *Buffer) outputInfo(info GlyphInfo) {
	b.outInfo = append(b.outInfo, info)
	b.outLen++
}

// sync finalizes the output buffer and replaces Info with the output.
// HarfBuzz equivalent: hb_buffer_t::sync() in hb-buffer.cc:416
func (b *Buffer) sync() {
	if !b.haveOutput {
		return
	}

	// Copy any remaining glyphs from input to output
	for b.Idx < len(b.Info) {
		b.nextGlyph()
	}

	// Replace Info with output
	b.Info = make([]GlyphInfo, len(b.outInfo))
	copy(b.Info, b.outInfo)

	// Reset output state
	b.haveOutput = false
	b.outLen = 0
	b.Idx = 0

	// Recreate Pos array with correct length
	b.Pos = make([]GlyphPos, len(b.Info))
}

// deleteGlyphsInplace removes glyphs from the buffer that match the filter.
// This is used for removing default ignorables after shaping.
// HarfBuzz equivalent: hb_buffer_t::delete_glyphs_inplace() in hb-buffer.cc:656-700
func (b *Buffer) deleteGlyphsInplace(filter func(*GlyphInfo) bool) {
	// Merge clusters and delete filtered glyphs.
	// NOTE: We can't use out-buffer as we have positioning data.
	j := 0
	count := len(b.Info)
	for i := 0; i < count; i++ {
		if filter(&b.Info[i]) {
			// Merge clusters.
			// Same logic as delete_glyph(), but for in-place removal.
			cluster := b.Info[i].Cluster
			if i+1 < count && cluster == b.Info[i+1].Cluster {
				continue // Cluster survives; do nothing.
			}

			if j > 0 {
				// Merge cluster backward.
				if cluster < b.Info[j-1].Cluster {
					mask := b.Info[i].Mask
					oldCluster := b.Info[j-1].Cluster
					for k := j; k > 0 && b.Info[k-1].Cluster == oldCluster; k-- {
						b.Info[k-1].Cluster = cluster
						b.Info[k-1].Mask = mask
					}
				}
				continue
			}

			if i+1 < count {
				// Merge cluster forward.
				b.MergeClusters(i, i+2)
			}
			continue
		}

		if j != i {
			b.Info[j] = b.Info[i]
			b.Pos[j] = b.Pos[i]
		}
		j++
	}
	b.Info = b.Info[:j]
	b.Pos = b.Pos[:j]
}

// shiftForward shifts glyphs in the Info array forward by count positions.
// This is used during buffer rewinding when idx < count.
// HarfBuzz equivalent: hb_buffer_t::shift_forward() in hb-buffer.cc:225-252
func (b *Buffer) shiftForward(count int) bool {
	if !b.haveOutput {
		return false
	}

	// Calculate new length
	oldLen := len(b.Info)
	newLen := oldLen + count

	// Extend Info slice to accommodate shifted elements
	if cap(b.Info) < newLen {
		// Need to allocate more space
		newInfo := make([]GlyphInfo, newLen)
		copy(newInfo, b.Info[:b.Idx])
		copy(newInfo[b.Idx+count:], b.Info[b.Idx:])
		b.Info = newInfo
	} else {
		// Have enough capacity, just extend and shift
		b.Info = b.Info[:newLen]
		// Shift Info[Idx:oldLen] to Info[Idx+count:]
		copy(b.Info[b.Idx+count:], b.Info[b.Idx:oldLen])
	}

	// Clear the gap if idx + count > oldLen
	// HarfBuzz: hb_memset (info + len, 0, (idx + count - len) * sizeof (info[0]));
	if b.Idx+count > oldLen {
		for j := oldLen; j < b.Idx+count; j++ {
			b.Info[j] = GlyphInfo{}
		}
	}

	// Advance idx
	b.Idx += count

	return true
}

// moveTo moves the buffer position to output index i.
// If i > outLen, this copies glyphs from input to output to reach position i.
// If i < outLen, this rewinds by moving glyphs from output back to input.
// HarfBuzz equivalent: hb_buffer_t::move_to() in hb-buffer.cc:469-513
//
// The parameter i is an output-buffer index (distance from beginning of output).
// This function ensures that outLen reaches i by copying glyphs from input.
func (b *Buffer) moveTo(i int) bool {
	if !b.haveOutput {
		// No output buffer active, just set index
		if i > len(b.Info) {
			return false
		}
		b.Idx = i
		return true
	}

	if b.outLen < i {
		// Need to move forward: copy glyphs from input to output
		count := i - b.outLen
		if b.Idx+count > len(b.Info) {
			return false
		}

		// Copy 'count' glyphs from input[Idx:] to output
		for j := 0; j < count; j++ {
			b.outInfo = append(b.outInfo, b.Info[b.Idx])
			b.Idx++
			b.outLen++
		}
	} else if b.outLen > i {
		// Tricky part: rewinding...
		// HarfBuzz: hb-buffer.cc:491-509
		count := b.outLen - i

		// If we don't have enough space before Idx, shift forward to make room
		// HarfBuzz: if (unlikely (idx < count && !shift_forward (count - idx))) return false;
		if b.Idx < count {
			if !b.shiftForward(count - b.Idx) {
				return false
			}
		}

		// Now we have enough space: idx >= count
		b.Idx -= count
		b.outLen -= count

		// Copy glyphs from output back to input
		// HarfBuzz: memmove (info + idx, out_info + out_len, count * sizeof (out_info[0]));
		copy(b.Info[b.Idx:b.Idx+count], b.outInfo[b.outLen:b.outLen+count])

		// Truncate outInfo to outLen so subsequent append() works correctly
		// In Go, append() adds to the slice end, not to a specific index.
		// Without truncation, appended glyphs would appear after the "ghost" entries.
		b.outInfo = b.outInfo[:b.outLen]
	}
	// If outLen == i, we're already at the right position

	return true
}

// ReorderMarksCallback is a function that performs script-specific mark reordering.
// HarfBuzz equivalent: hb_ot_shaper_t::reorder_marks callback
// Parameters:
//   - info: The slice of glyph info to reorder
//   - start: Start index of the mark sequence
//   - end: End index of the mark sequence (exclusive)
type ReorderMarksCallback func(info []GlyphInfo, start, end int)

// HasArabicFallbackPlan returns true if the shaper has an Arabic fallback plan.
// Used for debugging and testing.
func (s *Shaper) HasArabicFallbackPlan() bool {
	return s.arabicFallbackPlan != nil
}

// DebugArabicFallbackPlan prints debug information about the Arabic fallback plan.
func (s *Shaper) DebugArabicFallbackPlan() {
	if s.arabicFallbackPlan == nil {
		fmt.Println("  No fallback plan")
		return
	}
	fmt.Printf("  Num lookups: %d\n", s.arabicFallbackPlan.numLookups)
	for i := 0; i < s.arabicFallbackPlan.numLookups; i++ {
		lookup := s.arabicFallbackPlan.lookups[i]
		if lookup == nil {
			continue
		}
		fmt.Printf("  Lookup %d: type=%d ignoreMarks=%v mask=0x%08X\n",
			i, lookup.lookupType, lookup.ignoreMarks, s.arabicFallbackPlan.masks[i])
		if lookup.lookupType == 1 {
			fmt.Printf("    Singles: %d entries\n", len(lookup.singles))
			for j, entry := range lookup.singles {
				if j < 5 {
					fmt.Printf("      glyph %d -> %d\n", entry.glyph, entry.substitute)
				}
			}
			if len(lookup.singles) > 5 {
				fmt.Printf("      ... and %d more\n", len(lookup.singles)-5)
			}
		} else if lookup.lookupType == 4 {
			fmt.Printf("    Ligatures: %d entries\n", len(lookup.ligatures))
			for j, entry := range lookup.ligatures {
				if j < 5 {
					fmt.Printf("      glyph %d + %v -> %d\n", entry.firstGlyph, entry.components, entry.ligature)
				}
			}
		}
	}
}

// Shaper holds font data and performs text shaping.
type Shaper struct {
	font *Font
	face *Face // Font metrics (ascender, descender, upem, etc.) - like HarfBuzz hb_font_t
	cmap *Cmap
	gdef *GDEF
	gsub *GSUB
	gpos *GPOS
	kern *Kern // TrueType kern table (fallback for GPOS)
	hmtx *Hmtx
	glyf *Glyf // TrueType glyph data (for fallback mark positioning)
	fvar *Fvar
	avar *Avar
	hvar *Hvar

	// Default features to apply when nil is passed to Shape
	defaultFeatures []Feature

	// Variation state (for variable fonts)
	designCoords      []float32 // User-space coordinates
	normalizedCoords  []float32 // Normalized coordinates [-1, 1]
	normalizedCoordsI []int     // Normalized coords in F2DOT14 format, after avar mapping

	// Script-specific mark reordering callback.
	// HarfBuzz equivalent: plan->shaper->reorder_marks in hb-ot-shape-normalize.cc:394-395
	// Set this before calling normalizeBuffer for scripts that need mark reordering
	// (e.g., Arabic, Hebrew). Reset to nil after normalization.
	reorderMarksCallback ReorderMarksCallback

	// Arabic fallback shaping plan.
	// Used when font has no GSUB but has Unicode Arabic Presentation Forms.
	// HarfBuzz equivalent: arabic_fallback_plan_t in hb-ot-shaper-arabic-fallback.hh
	arabicFallbackPlan *arabicFallbackPlan

	// Indic shaping plans - one per script.
	// HarfBuzz equivalent: indic_shape_plan_t in hb-ot-shaper-indic.cc:289-308
	// Lazily initialized when first shaping Indic text.
	indicPlans map[Tag]*IndicPlan
}

// NewShaper creates a shaper from a parsed font.
// HarfBuzz equivalent: hb_font_create() + hb_shape_plan_create() in hb-font.cc, hb-shape-plan.cc
func NewShaper(font *Font) (*Shaper, error) {
	// Create Face first (for metrics access)
	face, err := NewFace(font)
	if err != nil {
		return nil, err
	}

	return NewShaperFromFace(face)
}

// NewShaperFromFace creates a shaper from an existing Face.
// This is useful when you already have a Face with metrics.
// HarfBuzz equivalent: hb_font_t holds both face and shaping data
func NewShaperFromFace(face *Face) (*Shaper, error) {
	font := face.Font

	s := &Shaper{
		font: font,
		face: face,
	}

	// Parse cmap (required)
	if font.HasTable(TagCmap) {
		data, err := font.TableData(TagCmap)
		if err != nil {
			return nil, err
		}
		s.cmap, err = ParseCmap(data)
		if err != nil {
			return nil, err
		}

		// For Symbol fonts, set font page from OS/2 table for Arabic PUA mapping
		if s.cmap.IsSymbol() && font.HasTable(TagOS2) {
			if os2Data, err := font.TableData(TagOS2); err == nil {
				if os2, err := ParseOS2(os2Data); err == nil {
					// For OS/2 version 0, font page is in high byte of fsSelection
					// Source: HarfBuzz hb-ot-os2-table.hh:333-342
					if os2.Version == 0 {
						s.cmap.SetFontPage(os2.FsSelection & 0xFF00)
					}
				}
			}
		}
	}

	// Parse GDEF (optional)
	if font.HasTable(TagGDEF) {
		data, err := font.TableData(TagGDEF)
		if err == nil {
			s.gdef, _ = ParseGDEF(data)
		}
	}

	// Parse GSUB (optional)
	if font.HasTable(TagGSUB) {
		data, err := font.TableData(TagGSUB)
		if err == nil {
			s.gsub, _ = ParseGSUB(data)
		}
	}

	// Parse GPOS (optional)
	if font.HasTable(TagGPOS) {
		data, err := font.TableData(TagGPOS)
		if err == nil {
			s.gpos, _ = ParseGPOS(data)
		}
	}

	// Parse kern table (fallback for GPOS kerning)
	if font.HasTable(TagKernTable) {
		data, err := font.TableData(TagKernTable)
		if err == nil {
			s.kern, _ = ParseKern(data, font.NumGlyphs())
		}
	}

	// Parse hmtx (optional but important for positioning)
	if font.HasTable(TagHmtx) && font.HasTable(TagHhea) {
		s.hmtx, _ = ParseHmtxFromFont(font)
	}

	// Parse glyf (optional, for fallback mark positioning)
	// Requires loca table and head table (for indexToLocFormat)
	if font.HasTable(TagGlyf) && font.HasTable(TagLoca) && font.HasTable(TagHead) {
		headData, err := font.TableData(TagHead)
		if err == nil {
			head, err := ParseHead(headData)
			if err == nil {
				locaData, err := font.TableData(TagLoca)
				if err == nil {
					loca, err := ParseLoca(locaData, font.NumGlyphs(), head.IndexToLocFormat)
					if err == nil {
						glyfData, err := font.TableData(TagGlyf)
						if err == nil {
							s.glyf, _ = ParseGlyf(glyfData, loca)
						}
					}
				}
			}
		}
	}

	// Parse fvar (variable fonts)
	if font.HasTable(TagFvar) {
		data, err := font.TableData(TagFvar)
		if err == nil {
			s.fvar, _ = ParseFvar(data)
			// Initialize variation coords to defaults (all zeros = default position)
			if s.fvar != nil && s.fvar.AxisCount() > 0 {
				axisCount := s.fvar.AxisCount()
				s.designCoords = make([]float32, axisCount)
				s.normalizedCoords = make([]float32, axisCount)
				s.normalizedCoordsI = make([]int, axisCount)
				// Set design coords to default values
				for i, axis := range s.fvar.AxisInfos() {
					s.designCoords[i] = axis.DefaultValue
				}
			}
		}
	}

	// Parse avar (axis variations mapping)
	if font.HasTable(TagAvar) {
		data, err := font.TableData(TagAvar)
		if err == nil {
			s.avar, _ = ParseAvar(data)
		}
	}

	// Parse HVAR (horizontal metrics variations)
	if font.HasTable(TagHvar) {
		data, err := font.TableData(TagHvar)
		if err == nil {
			s.hvar, _ = ParseHvar(data)
		}
	}

	// Initialize Arabic fallback plan if needed
	// HarfBuzz: arabic_fallback_plan_create() in hb-ot-shaper-arabic-fallback.hh:323-347
	// Only creates plan for Arabic script fonts without GSUB positional features
	// but with Unicode Arabic Presentation Forms
	arabicTag := MakeTag('a', 'r', 'a', 'b')
	if needsArabicFallback(s.gsub, arabicTag, s.cmap) {
		s.arabicFallbackPlan = createArabicFallbackPlan(font, s.cmap)
	}

	// Set default features
	s.defaultFeatures = DefaultFeatures()

	return s, nil
}

// Note: TagKern, TagMark, TagMkmk are defined in gpos.go

// --- Variable Font Methods ---

// HasVariations returns true if the font is a variable font.
func (s *Shaper) HasVariations() bool {
	return s.fvar != nil && s.fvar.HasData()
}

// SetVariations sets the variation axis values.
// This overrides all existing variations. Axes not included will be set to their default values.
func (s *Shaper) SetVariations(variations []Variation) {
	if s.fvar == nil || s.fvar.AxisCount() == 0 {
		return
	}

	axisCount := s.fvar.AxisCount()
	axes := s.fvar.AxisInfos()

	// Reset to defaults
	for i := 0; i < axisCount; i++ {
		s.designCoords[i] = axes[i].DefaultValue
		s.normalizedCoords[i] = 0
		s.normalizedCoordsI[i] = 0
	}

	// Apply specified variations
	for _, v := range variations {
		for i := 0; i < axisCount; i++ {
			if axes[i].Tag == v.Tag {
				s.designCoords[i] = clampFloat32(v.Value, axes[i].MinValue, axes[i].MaxValue)
				s.normalizedCoords[i] = s.fvar.NormalizeAxisValue(i, v.Value)
				s.normalizedCoordsI[i] = floatToF2DOT14(s.normalizedCoords[i])
				break
			}
		}
	}

	// Apply avar mapping
	s.applyAvarMapping()
}

// SetVariation sets a single variation axis value.
// Note: This is less efficient than SetVariations for setting multiple axes.
func (s *Shaper) SetVariation(tag Tag, value float32) {
	if s.fvar == nil || s.fvar.AxisCount() == 0 {
		return
	}

	axes := s.fvar.AxisInfos()
	for i, axis := range axes {
		if axis.Tag == tag {
			s.designCoords[i] = clampFloat32(value, axis.MinValue, axis.MaxValue)
			s.normalizedCoords[i] = s.fvar.NormalizeAxisValue(i, value)
			s.normalizedCoordsI[i] = floatToF2DOT14(s.normalizedCoords[i])
			// Apply avar mapping
			s.applyAvarMapping()
			return
		}
	}
}

// SetNamedInstance sets the variation to a named instance (e.g., "Bold", "Light").
func (s *Shaper) SetNamedInstance(index int) {
	if s.fvar == nil {
		return
	}

	instance, ok := s.fvar.NamedInstanceAt(index)
	if !ok {
		return
	}

	// Copy instance coordinates
	axisCount := s.fvar.AxisCount()
	for i := 0; i < axisCount && i < len(instance.Coords); i++ {
		s.designCoords[i] = instance.Coords[i]
		s.normalizedCoords[i] = s.fvar.NormalizeAxisValue(i, instance.Coords[i])
		s.normalizedCoordsI[i] = floatToF2DOT14(s.normalizedCoords[i])
	}

	// Apply avar mapping
	s.applyAvarMapping()
}

// DesignCoords returns the current design-space coordinates.
// Returns nil for non-variable fonts.
func (s *Shaper) DesignCoords() []float32 {
	if s.designCoords == nil {
		return nil
	}
	result := make([]float32, len(s.designCoords))
	copy(result, s.designCoords)
	return result
}

// NormalizedCoords returns the current normalized coordinates (range [-1, 1]).
// Returns nil for non-variable fonts.
func (s *Shaper) NormalizedCoords() []float32 {
	if s.normalizedCoords == nil {
		return nil
	}
	result := make([]float32, len(s.normalizedCoords))
	copy(result, s.normalizedCoords)
	return result
}

// Fvar returns the parsed fvar table, or nil if not present.
func (s *Shaper) Fvar() *Fvar {
	return s.fvar
}

// Hvar returns the parsed HVAR table, or nil if not present.
func (s *Shaper) Hvar() *Hvar {
	return s.hvar
}

// HasHvar returns true if the font has HVAR data for variable advances.
func (s *Shaper) HasHvar() bool {
	return s.hvar != nil && s.hvar.HasData()
}

// applyAvarMapping applies avar non-linear mapping to normalizedCoordsI.
func (s *Shaper) applyAvarMapping() {
	if s.avar == nil || !s.avar.HasData() {
		return
	}
	s.normalizedCoordsI = s.avar.MapCoords(s.normalizedCoordsI)
}

// Shape shapes the text in the buffer using the specified features.
// If features is nil, default features are used.
// This is the main shaping entry point, similar to HarfBuzz's hb_shape().
//
// HarfBuzz equivalent: hb_shape() -> hb_shape_full() -> hb_ot_shape_internal()
// in hb-shape.cc and hb-ot-shape.cc
func (s *Shaper) Shape(buf *Buffer, features []Feature) {
	if buf.Len() == 0 {
		return
	}

	// Use default features if none specified
	if len(features) == 0 {
		features = s.defaultFeatures
	}

	// Step 1: Guess segment properties (script, direction, language)
	// HarfBuzz equivalent: hb_buffer_guess_segment_properties() in hb-buffer.cc
	buf.GuessSegmentProperties()

	// Step 1.5: Form clusters - merge grapheme clusters (base + marks)
	// HarfBuzz equivalent: hb_form_clusters() in hb-ot-shape.cc:577-589
	// This is called BEFORE shaping to group base characters with their marks
	formClusters(buf)

	// Step 2: Insert dotted circle before orphaned marks (if at BOT)
	// HarfBuzz equivalent: hb_insert_dotted_circle() in hb-ot-shape.cc:1184
	// This happens BEFORE shaper dispatch so it works for all shapers!
	s.insertDottedCircle(buf)

	// Step 3: Select the appropriate shaper based on script, direction, and font script tag
	// HarfBuzz equivalent: hb_ot_shaper_categorize() in hb-ot-shaper.hh
	// The font's actual script tag (e.g., 'knd3' vs 'knd2') determines which shaper to use.
	// For Indic scripts with version 3 tags, USE shaper is used instead of Indic shaper.
	var shaper *OTShaper
	if s.gsub != nil {
		fontScriptTag := s.gsub.FindChosenScriptTag(buf.Script)
		shaper = SelectShaperWithFont(buf.Script, buf.Direction, fontScriptTag)
	} else {
		shaper = SelectShaper(buf.Script, buf.Direction)
	}

	// Step 4: Dispatch to the appropriate shaping function based on shaper
	// HarfBuzz: Uses function pointers in hb_ot_shaper_t
	switch shaper.Name {
	case "arabic":
		// Arabic shaping path (also handles Phags-Pa and Mongolian joining)
		s.shapeArabic(buf, features)
	case "khmer":
		// Khmer has its own shaper (separate from Indic and USE)
		s.shapeKhmer(buf, features)
	case "indic":
		// Indic shaping path (Devanagari, Bengali, Tamil, etc.)
		s.shapeIndic(buf, features)
	case "use":
		// USE shaping path (Tibetan, Javanese, etc. - NOT Khmer/Myanmar!)
		s.shapeUSE(buf, features)
	case "myanmar":
		// Myanmar shaper
		// HarfBuzz equivalent: _hb_ot_shaper_myanmar in hb-ot-shaper-myanmar.cc
		s.shapeMyanmar(buf, features)
	case "thai":
		// Thai/Lao shaper with Sara Am decomposition
		s.shapeThai(buf, features)
	case "hebrew":
		// Hebrew shaper with mark reordering
		s.shapeHebrew(buf, features)
	case "hangul":
		// Hangul shaper (currently falls back to default)
		s.shapeDefault(buf, features)
	case "qaag":
		// Zawgyi (Myanmar visual encoding) shaper
		// HarfBuzz equivalent: _hb_ot_shaper_myanmar_zawgyi in hb-ot-shaper-myanmar.cc
		s.shapeQaag(buf, features)
	default:
		// Default shaping path
		s.shapeDefault(buf, features)
	}

	// Step 4: Handle default ignorables (after all shaping)
	// HarfBuzz: hb-ot-shape.cc:828-851 (hb_ot_hide_default_ignorables)
	s.hideDefaultIgnorables(buf)
}

// insertDottedCircle inserts U+25CC dotted circle before orphaned marks.
// This happens when text starts with a mark (e.g., combining diacritic) without
// a base character. The dotted circle provides a visible base for the mark.
// HarfBuzz equivalent: hb_insert_dotted_circle() in hb-ot-shape.cc:549-574
func (s *Shaper) insertDottedCircle(buf *Buffer) {
	// 1. Check if dotted circle insertion is disabled
	if buf.Flags&BufferFlagDoNotInsertDottedCircle != 0 {
		return
	}

	// 2. Check if buffer starts with a mark (BOT flag + first char is mark)
	// BOT = Beginning Of Text
	if buf.Flags&BufferFlagBOT == 0 ||
		buf.Len() == 0 ||
		!IsUnicodeMark(buf.Info[0].Codepoint) {
		return
	}

	// 3. Check if font has dotted circle glyph (U+25CC)
	if !s.font.HasGlyph(0x25CC) {
		return
	}

	// 4. Create dotted circle glyph info
	dottedCircle := GlyphInfo{
		Codepoint: 0x25CC,
		Cluster:   buf.Info[0].Cluster, // Same cluster as the mark
		Mask:      buf.Info[0].Mask,    // Same mask as the mark
	}

	// 5. Insert at beginning using output buffer mechanism
	buf.clearOutput()
	buf.Idx = 0
	buf.outputInfo(dottedCircle)
	buf.sync()
}

// SyllableAccessor provides methods to access syllable information for dotted circle insertion.
// Different shapers (Indic, USE, Khmer) use different syllable storage mechanisms.
type SyllableAccessor interface {
	// GetSyllable returns the syllable byte (upper 4 bits = serial, lower 4 bits = type)
	GetSyllable(i int) uint8
	// GetCategory returns the shaper-specific category for a glyph
	GetCategory(i int) uint8
	// SetCategory sets the shaper-specific category for a glyph
	SetCategory(i int, cat uint8)
	// Len returns the number of glyphs
	Len() int
}

// insertSyllabicDottedCircles inserts dotted circle glyphs at the start of broken syllables.
// HarfBuzz equivalent: hb_syllabic_insert_dotted_circles() in hb-ot-shaper-syllabic.cc:33-100
//
// Parameters:
//   - buf: The buffer to modify
//   - accessor: Provides access to syllable information
//   - brokenSyllableType: The syllable type that indicates a broken cluster (lower 4 bits)
//   - dottedCircleCategory: The category to assign to the inserted dotted circle
//   - rephaCategory: The category of repha glyphs (-1 if not applicable)
//
// Returns true if any dotted circles were inserted.
func (s *Shaper) insertSyllabicDottedCircles(buf *Buffer, accessor SyllableAccessor,
	brokenSyllableType uint8, dottedCircleCategory uint8, rephaCategory int) bool {

	// 1. Check if dotted circle insertion is disabled
	if buf.Flags&BufferFlagDoNotInsertDottedCircle != 0 {
		return false
	}

	// 2. Check if there are any broken syllables
	hasBroken := false
	for i := 0; i < accessor.Len(); i++ {
		syllable := accessor.GetSyllable(i)
		if (syllable & 0x0F) == brokenSyllableType {
			hasBroken = true
			break
		}
	}
	if !hasBroken {
		return false
	}

	// 3. Check if font has dotted circle glyph (U+25CC)
	dottedCircleGlyph, ok := s.cmap.Lookup(0x25CC)
	if !ok || dottedCircleGlyph == 0 {
		return false
	}

	// 4. Create dotted circle template
	dottedCircle := GlyphInfo{
		GlyphID:   dottedCircleGlyph,
		Codepoint: 0x25CC,
	}

	// 5. Insert dotted circles using output buffer mechanism
	// HarfBuzz: Uses clear_output/output_info/next_glyph/sync pattern
	buf.clearOutput()
	buf.Idx = 0

	lastSyllable := uint8(0)
	for buf.Idx < len(buf.Info) {
		syllable := accessor.GetSyllable(buf.Idx)

		// Check if this is a new broken syllable
		if lastSyllable != syllable && (syllable&0x0F) == brokenSyllableType {
			lastSyllable = syllable

			// Create the dotted circle with same cluster/mask/syllable as current glyph
			ginfo := dottedCircle
			ginfo.Cluster = buf.Info[buf.Idx].Cluster
			ginfo.Mask = buf.Info[buf.Idx].Mask

			// Insert dotted circle after possible Repha
			// HarfBuzz: hb-ot-shaper-syllabic.cc:81-87
			if rephaCategory != -1 {
				for buf.Idx < len(buf.Info) &&
					lastSyllable == accessor.GetSyllable(buf.Idx) &&
					accessor.GetCategory(buf.Idx) == uint8(rephaCategory) {
					buf.nextGlyph()
				}
			}

			// Output the dotted circle
			buf.outputInfo(ginfo)
		} else {
			buf.nextGlyph()
		}
	}
	buf.sync()

	return true
}

// shapeDefault applies default shaping (no script-specific processing).
// HarfBuzz equivalent: _hb_ot_shaper_default in hb-ot-shaper-default.cc
func (s *Shaper) shapeDefault(buf *Buffer, features []Feature) {
	// Step 1: Normalize Unicode (decompose, reorder marks, recompose)
	// HarfBuzz equivalent: _hb_ot_shape_normalize() in hb-ot-shape.cc
	s.normalizeBuffer(buf, NormalizationModeAuto)

	// Step 2: Initialize masks: all glyphs get MaskGlobal so global features apply
	// HarfBuzz equivalent: hb_ot_shape_initialize_masks() in hb-ot-shape.cc:1175
	buf.ResetMasks(MaskGlobal)

	// Step 3: Map codepoints to glyphs
	s.mapCodepointsToGlyphs(buf)

	// Step 4: Set glyph classes from GDEF
	s.setGlyphClasses(buf)

	// Step 5: Categorize and apply features
	gsubFeatures, gposFeatures := s.categorizeFeatures(features)

	// Add direction-dependent features (HarfBuzz: hb-ot-shape.cc:332-347)
	switch buf.Direction {
	case DirectionRTL:
		// RTL: apply rtla (RTL Alternates) and rtlm (RTL Mirrored Forms)
		gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('r', 't', 'l', 'a'), Value: 1})
		gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('r', 't', 'l', 'm'), Value: 1})
	case DirectionLTR:
		// LTR: apply ltra and ltrm
		gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('l', 't', 'r', 'a'), Value: 1})
		gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('l', 't', 'r', 'm'), Value: 1})
	}

	s.applyGSUB(buf, gsubFeatures)
	s.setBaseAdvances(buf)

	// Add default GPOS features if none provided
	// HarfBuzz equivalent: common_features[] and horizontal_features[] in hb-ot-shape.cc:295-318
	// These features are always enabled by default for mark positioning, cursive, kerning, etc.
	if len(gposFeatures) == 0 {
		gposFeatures = s.getDefaultGPOSFeatures(buf.Direction)
	}

	// Default shaper uses LATE mode for zero width marks
	// HarfBuzz: HB_OT_SHAPE_ZERO_WIDTH_MARKS_BY_GDEF_LATE in _hb_ot_shaper_default
	s.applyGPOSWithZeroWidthMarks(buf, gposFeatures, ZeroWidthMarksByGDEFLate)
	s.applyKernTableFallback(buf, features) // Fallback if no GPOS kern

	// Reverse buffer for RTL display (HarfBuzz: hb-ot-shape.cc:1106-1107)
	if buf.Direction == DirectionRTL {
		s.reverseClusters(buf)
	}
}

// shapeHebrew performs Hebrew-specific shaping.
// HarfBuzz equivalent: Hebrew shaper in hb-ot-shaper-hebrew.cc
//
// Hebrew requires:
// 1. Special mark reordering during normalization
// 2. Special composition for presentation forms (fallback for old fonts)
// 3. GPOS is only applied if script tag is 'hebr'
func (s *Shaper) shapeHebrew(buf *Buffer, features []Feature) {
	// Step 1: Normalize Unicode with Hebrew mark reordering
	// HarfBuzz: reorder_marks_hebrew() callback during normalization
	s.reorderMarksCallback = reorderMarksHebrewSlice
	s.normalizeBuffer(buf, NormalizationModeAuto)
	s.reorderMarksCallback = nil

	// Step 2: Initialize masks
	buf.ResetMasks(MaskGlobal)

	// Step 3: Map codepoints to glyphs
	s.mapCodepointsToGlyphs(buf)

	// Step 4: Set glyph classes from GDEF
	s.setGlyphClasses(buf)

	// Step 5: Categorize and apply features
	gsubFeatures, gposFeatures := s.categorizeFeatures(features)

	// Add RTL features (Hebrew is RTL)
	gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('r', 't', 'l', 'a'), Value: 1})
	gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('r', 't', 'l', 'm'), Value: 1})

	s.applyGSUB(buf, gsubFeatures)
	s.setBaseAdvances(buf)
	// Hebrew uses LATE mode for zero width marks
	// HarfBuzz: HB_OT_SHAPE_ZERO_WIDTH_MARKS_BY_GDEF_LATE in _hb_ot_shaper_hebrew
	s.applyGPOSWithZeroWidthMarks(buf, gposFeatures, ZeroWidthMarksByGDEFLate)
	s.applyKernTableFallback(buf, features)

	// Reverse buffer for RTL display
	if buf.Direction == DirectionRTL {
		s.reverseClusters(buf)
	}
}

// shapeQaag performs shaping for Zawgyi (Myanmar visual encoding).
// HarfBuzz equivalent: _hb_ot_shaper_myanmar_zawgyi in hb-ot-shaper-myanmar.cc:363-378
//
// Zawgyi is a legacy encoding for Myanmar that uses visual ordering.
// Characters are already in display order, so:
// - No normalization (NormalizationModeNone)
// - No zero-width marks (ZeroWidthMarksNone)
// - No fallback positioning (FallbackPosition = false)
func (s *Shaper) shapeQaag(buf *Buffer, features []Feature) {
	// Step 1: NO normalization - Zawgyi uses visual encoding
	// HarfBuzz: HB_OT_SHAPE_NORMALIZATION_MODE_NONE

	// Step 2: Initialize masks: all glyphs get MaskGlobal so global features apply
	buf.ResetMasks(MaskGlobal)

	// Step 3: Map codepoints to glyphs
	s.mapCodepointsToGlyphs(buf)

	// Step 4: Set glyph classes from GDEF
	s.setGlyphClasses(buf)

	// Step 5: Categorize and apply features
	gsubFeatures, gposFeatures := s.categorizeFeatures(features)

	// Add direction-dependent features
	switch buf.Direction {
	case DirectionRTL:
		gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('r', 't', 'l', 'a'), Value: 1})
		gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('r', 't', 'l', 'm'), Value: 1})
	case DirectionLTR:
		gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('l', 't', 'r', 'a'), Value: 1})
		gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('l', 't', 'r', 'm'), Value: 1})
	}

	s.applyGSUB(buf, gsubFeatures)
	s.setBaseAdvances(buf)

	// Add default GPOS features if none provided
	if len(gposFeatures) == 0 {
		gposFeatures = s.getDefaultGPOSFeatures(buf.Direction)
	}

	// Qaag uses ZeroWidthMarksNone - don't zero mark advances
	// HarfBuzz: HB_OT_SHAPE_ZERO_WIDTH_MARKS_NONE
	s.applyGPOSWithZeroWidthMarks(buf, gposFeatures, ZeroWidthMarksNone)

	// NO fallback kern - Qaag has FallbackPosition = false
	// HarfBuzz: fallback_position = false

	// Reverse buffer for RTL display
	if buf.Direction == DirectionRTL {
		s.reverseClusters(buf)
	}
}

// hideDefaultIgnorables handles default ignorable characters after shaping.
// Based on HarfBuzz hb-ot-shape.cc:828-851.
// Default ignorables (ZWJ, ZWNJ, variation selectors, BOM, etc.) are either:
// - Replaced with an invisible glyph (space), or
// - Deleted from the buffer entirely
func (s *Shaper) hideDefaultIgnorables(buf *Buffer) {
	if buf.Flags&BufferFlagPreserveDefaultIgnorables != 0 {
		return
	}

	// Check if we have any default ignorables
	// HarfBuzz: Uses GlyphPropsDefaultIgnorable flag set during AddCodepoints
	hasDefaultIgnorables := false
	for i := range buf.Info {
		if buf.Info[i].GlyphProps&GlyphPropsDefaultIgnorable != 0 {
			hasDefaultIgnorables = true
			break
		}
	}
	if !hasDefaultIgnorables {
		return
	}

	// HarfBuzz: Try to get an invisible glyph (space) to replace default ignorables
	// If not available or REMOVE flag is set, delete them entirely
	var invisibleGlyph GlyphID
	hasInvisible := false
	if buf.Flags&BufferFlagRemoveDefaultIgnorables == 0 && s.cmap != nil {
		if gid, ok := s.cmap.Lookup(' '); ok && gid != 0 {
			invisibleGlyph = gid
			hasInvisible = true
		}
	}

	if hasInvisible {
		// Replace default ignorables with invisible glyph (zero-width space)
		// HarfBuzz: Only hide if NOT substituted (see _hb_glyph_info_is_default_ignorable)
		// This allows GSUB to substitute default ignorables like U+180E (Mongolian Vowel Separator)
		for i := range buf.Info {
			if buf.Info[i].GlyphProps&GlyphPropsDefaultIgnorable != 0 &&
				buf.Info[i].GlyphProps&GlyphPropsSubstituted == 0 {
				buf.Info[i].GlyphID = invisibleGlyph
				buf.Pos[i].XAdvance = 0
				buf.Pos[i].YAdvance = 0
			}
		}
	} else {
		// Delete default ignorables from buffer
		// HarfBuzz: buffer->delete_glyphs_inplace(_hb_glyph_info_is_default_ignorable)
		// Only delete if NOT substituted
		buf.deleteGlyphsInplace(func(info *GlyphInfo) bool {
			return info.GlyphProps&GlyphPropsDefaultIgnorable != 0 &&
				info.GlyphProps&GlyphPropsSubstituted == 0
		})
	}
}

// hasArabicScript checks if the buffer contains Arabic-script characters.
func (s *Shaper) hasArabicScript(buf *Buffer) bool {
	for _, info := range buf.Info {
		if isArabicScript(info.Codepoint) {
			return true
		}
	}
	return false
}

// hasPhagsPaScript checks if the buffer contains Phags-pa script characters.
func (s *Shaper) hasPhagsPaScript(buf *Buffer) bool {
	for _, info := range buf.Info {
		if info.Codepoint >= 0xA840 && info.Codepoint <= 0xA877 {
			return true
		}
	}
	return false
}

// hasMongolianScript checks if the buffer contains Mongolian script characters.
func (s *Shaper) hasMongolianScript(buf *Buffer) bool {
	for _, info := range buf.Info {
		if info.Codepoint >= 0x1800 && info.Codepoint <= 0x18AF {
			return true
		}
	}
	return false
}

// shapeArabic applies Arabic-specific shaping to the buffer.
// HarfBuzz equivalent: hb_ot_shape_internal() with arabic shaper
//
// Buffer order in HarfBuzz (from hb-ot-shape.cc):
//  1. hb_ensure_native_direction(): For Arabic RTL, direction matches native
//     direction, so buffer is NOT reversed. Buffer stays in LOGICAL order.
//  2. preprocess_text -> setup_masks_arabic -> arabic_joining: Runs in LOGICAL order
//  3. hb_ot_substitute_pre/plan: GSUB applied in LOGICAL order
//  4. hb_ot_position: GPOS applied, then buffer reversed at the END for RTL
func (s *Shaper) shapeArabic(buf *Buffer, features []Feature) {
	// Set direction based on script if not already set
	// Arabic/Syriac are RTL, Phags-pa is LTR (but with Arabic-like joining)
	if buf.Direction == 0 {
		if s.hasPhagsPaScript(buf) {
			buf.Direction = DirectionLTR
		} else {
			buf.Direction = DirectionRTL
		}
	}

	// Step 0: Normalize Unicode (decompose, reorder marks, recompose)
	// HarfBuzz equivalent: _hb_ot_shape_normalize() in hb-ot-shape-normalize.cc
	// This replaces the old normalizeArabic() with full 3-phase normalization.
	//
	// Arabic requires special mark reordering: MCMs (Modifier Combining Marks) like
	// HAMZA ABOVE/BELOW need to be moved to the beginning of the mark sequence.
	// HarfBuzz equivalent: plan->shaper->reorder_marks in hb-ot-shape-normalize.cc:394-395
	s.reorderMarksCallback = reorderMarksArabicSlice
	s.normalizeBuffer(buf, NormalizationModeComposedDiacritics)
	s.reorderMarksCallback = nil // Reset callback after normalization

	// Step 0.5: Initialize masks after normalization
	// HarfBuzz equivalent: hb_ot_shape_initialize_masks()
	// All glyphs get MaskGlobal, then Arabic-specific masks are added via applyArabicFeatures
	buf.ResetMasks(MaskGlobal)

	// Step 0.6: Re-map codepoints to glyphs after normalization
	s.mapCodepointsToGlyphs(buf)

	// Step 0.7: Apply automatic fractions (works with Arabic-Indic digits too)
	s.applyAutomaticFractions(buf)

	// NOTE: Buffer stays in LOGICAL order here!
	// HarfBuzz's hb_ensure_native_direction() does NOT reverse for Arabic RTL
	// because direction=RTL matches native script direction=RTL.

	// Step 1: Apply Arabic-specific GSUB features (positional forms)
	// This internally calls arabicJoining() and applies features per-glyph
	// Buffer is in LOGICAL order (left-to-right in memory = logical order)
	//
	// User GSUB features are passed to applyArabicFeatures, which filters out
	// standard Arabic features (ccmp, rlig, calt, liga, etc.) and applies the rest.
	gsubFeatures, _ := s.categorizeFeatures(features)
	s.applyArabicFeatures(buf, gsubFeatures)

	// Step 1.5: Set glyph classes from GDEF AFTER GSUB (CRITICAL!)
	// GSUB may have decomposed glyphs (e.g., U+0623 → Alef + HamzaAbove)
	// We need glyph classes for the FINAL glyphs, not the input glyphs!
	// HarfBuzz equivalent: called as part of hb_ot_shape_setup_masks()
	s.setGlyphClasses(buf)

	// Step 2: Set base advances
	s.setBaseAdvances(buf)

	// Step 3: Apply GPOS features
	// Arabic shaper uses LATE zero width marks (HarfBuzz: HB_OT_SHAPE_ZERO_WIDTH_MARKS_BY_GDEF_LATE)
	_, gposFeatures := s.categorizeFeatures(features)
	s.applyGPOSWithZeroWidthMarks(buf, gposFeatures, ZeroWidthMarksByGDEFLate)
	s.applyKernTableFallback(buf, features) // Fallback if no GPOS kern

	// Step 4: Reverse for RTL display output
	// HarfBuzz equivalent: hb_buffer_reverse() at end of hb_ot_position()
	// This happens BEFORE postprocess_glyphs, converting logical to visual order.
	if buf.Direction == DirectionRTL {
		s.reverseBuffer(buf)
	}

	// Step 5: Arabic post-processing (STCH stretching)
	// HarfBuzz equivalent: postprocess_glyphs_arabic() in hb-ot-shaper-arabic.cc:647-653
	// Called after reversal, so buffer is in visual order.
	postprocessGlyphsArabic(buf, s)
}

// reverseBuffer reverses the entire buffer (Info and Pos arrays).
// HarfBuzz equivalent: hb_buffer_t::reverse()
func (s *Shaper) reverseBuffer(buf *Buffer) {
	n := len(buf.Info)
	for i := 0; i < n/2; i++ {
		j := n - 1 - i
		buf.Info[i], buf.Info[j] = buf.Info[j], buf.Info[i]
		if len(buf.Pos) > j {
			buf.Pos[i], buf.Pos[j] = buf.Pos[j], buf.Pos[i]
		}
	}
}

// reverseRange reverses the glyph order for a range [start, end).
func (s *Shaper) reverseRange(buf *Buffer, start, end int) {
	for i, j := start, end-1; i < j; i, j = i+1, j-1 {
		buf.Info[i], buf.Info[j] = buf.Info[j], buf.Info[i]
		buf.Pos[i], buf.Pos[j] = buf.Pos[j], buf.Pos[i]
	}
}

// reverseClusters reverses buffer clusters for RTL text.
// First, it reverses the entire buffer, then reverses each cluster group
// to restore the original intra-cluster order. This keeps glyphs within
// a cluster in their original order while reversing the overall buffer order.
func (s *Shaper) reverseClusters(buf *Buffer) {
	n := len(buf.Info)
	if n == 0 {
		return
	}

	// For RTL text, we simply reverse the entire buffer.
	// Glyphs within a cluster are already in the correct relative order
	// (marks after their base) and should remain so after reversal.
	s.reverseRange(buf, 0, n)
}

// categorizeFeatures separates features into GSUB and GPOS categories.
// Features are categorized based on whether they exist in the font's GSUB or GPOS table.
func (s *Shaper) categorizeFeatures(features []Feature) (gsub, gpos []Feature) {
	for _, f := range features {
		if f.Value == 0 {
			continue // Disabled feature
		}

		// Check if feature exists in GSUB
		if s.gsub != nil {
			if featureList, err := s.gsub.ParseFeatureList(); err == nil {
				if featureList.FindFeature(f.Tag) != nil {
					gsub = append(gsub, f)
				}
			}
		}

		// Check if feature exists in GPOS
		if s.gpos != nil {
			if featureList, err := s.gpos.ParseFeatureList(); err == nil {
				if featureList.FindFeature(f.Tag) != nil {
					gpos = append(gpos, f)
				}
			}
		}
	}
	return
}

// getDefaultGPOSFeatures returns the default GPOS features that HarfBuzz enables automatically.
// HarfBuzz equivalent: common_features[] and horizontal_features[] in hb-ot-shape.cc:295-318
func (s *Shaper) getDefaultGPOSFeatures(direction Direction) []Feature {
	// Common GPOS features (always enabled)
	// HarfBuzz: common_features[] in hb-ot-shape.cc:295-305
	features := []Feature{
		{Tag: MakeTag('a', 'b', 'v', 'm'), Value: 1}, // Above Base Mark
		{Tag: MakeTag('b', 'l', 'w', 'm'), Value: 1}, // Below Base Mark
		{Tag: MakeTag('m', 'a', 'r', 'k'), Value: 1}, // Mark Positioning
		{Tag: MakeTag('m', 'k', 'm', 'k'), Value: 1}, // Mark to Mark Positioning
	}

	// Horizontal-specific GPOS features
	// HarfBuzz: horizontal_features[] in hb-ot-shape.cc:308-318
	if direction.IsHorizontal() {
		features = append(features,
			Feature{Tag: MakeTag('c', 'u', 'r', 's'), Value: 1}, // Cursive Positioning
			Feature{Tag: MakeTag('d', 'i', 's', 't'), Value: 1}, // Distances
			Feature{Tag: MakeTag('k', 'e', 'r', 'n'), Value: 1}, // Kerning
		)
	}

	return features
}

// getDefaultGSUBFeatures returns the default GSUB features that HarfBuzz enables automatically.
// HarfBuzz equivalent: common_features[] and horizontal_features[] in hb-ot-shape.cc:295-318
//
// These features are always enabled globally by HarfBuzz:
// - ccmp (Glyph Composition/Decomposition)
// - locl (Localized Forms)
// - rlig (Required Ligatures)
// - liga (Standard Ligatures) - horizontal only
// - calt (Contextual Alternates) - horizontal only
// - clig (Contextual Ligatures) - horizontal only
// - rclt (Required Contextual Alternates) - horizontal only
func (s *Shaper) getDefaultGSUBFeatures(direction Direction) []Feature {
	// Common GSUB features (always enabled)
	// HarfBuzz: common_features[] in hb-ot-shape.cc:295-305
	features := []Feature{
		{Tag: MakeTag('c', 'c', 'm', 'p'), Value: 1}, // Glyph Composition/Decomposition
		{Tag: MakeTag('l', 'o', 'c', 'l'), Value: 1}, // Localized Forms
		{Tag: MakeTag('r', 'l', 'i', 'g'), Value: 1}, // Required Ligatures
	}

	// Horizontal-specific GSUB features
	// HarfBuzz: horizontal_features[] in hb-ot-shape.cc:308-318
	if direction.IsHorizontal() {
		features = append(features,
			Feature{Tag: MakeTag('c', 'a', 'l', 't'), Value: 1}, // Contextual Alternates
			Feature{Tag: MakeTag('c', 'l', 'i', 'g'), Value: 1}, // Contextual Ligatures
			Feature{Tag: MakeTag('l', 'i', 'g', 'a'), Value: 1}, // Standard Ligatures
			Feature{Tag: MakeTag('r', 'c', 'l', 't'), Value: 1}, // Required Contextual Alternates
		)
	}

	return features
}

// mapCodepointsToGlyphs converts Unicode codepoints to glyph IDs.
// This function also handles Variation Selectors by combining base + VS
// into a single variant glyph when the font supports it (cmap format 14).
// Reference: HarfBuzz hb-ot-shape-normalize.cc:203-252 (handle_variation_selector_cluster)
func (s *Shaper) mapCodepointsToGlyphs(buf *Buffer) {
	if s.cmap == nil {
		return
	}

	// Pass 1: Handle Variation Selectors (combine base + VS)
	// Iterate backwards to safely remove VS entries
	// This follows HarfBuzz's approach: if font has variant glyph, combine;
	// otherwise leave both characters separate for GSUB to handle.
	for i := len(buf.Info) - 1; i > 0; i-- {
		cp := buf.Info[i].Codepoint
		if IsVariationSelector(cp) {
			baseCp := buf.Info[i-1].Codepoint
			if glyph, ok := s.cmap.LookupVariation(baseCp, cp); ok {
				// Found combined variant - use it for base
				buf.Info[i-1].GlyphID = glyph
				// Remove the VS entry from buffer
				// Preserve cluster: the VS was part of the base's cluster
				buf.Info = append(buf.Info[:i], buf.Info[i+1:]...)
				if len(buf.Pos) > i {
					buf.Pos = append(buf.Pos[:i], buf.Pos[i+1:]...)
				}
			}
			// If no variant found, VS stays as separate glyph (GSUB may handle it)
		}
	}

	// Pass 2: Normal codepoint -> glyph lookup
	for i := range buf.Info {
		if buf.Info[i].GlyphID != 0 {
			continue // Already set (from VS combination)
		}
		cp := buf.Info[i].Codepoint
		glyph, ok := s.cmap.Lookup(cp)

		if ok {
			buf.Info[i].GlyphID = glyph
		} else {
			// Try fallback mappings for equivalent characters
			fallback := getCodepointFallback(cp)
			if fallback != cp {
				glyph, ok = s.cmap.Lookup(fallback)
				if ok {
					buf.Info[i].GlyphID = glyph
					continue
				}
			}
			// Use .notdef glyph (0)
			buf.Info[i].GlyphID = 0
		}
	}
}

// getCodepointFallback returns a fallback codepoint for characters that have
// equivalent representations. Returns the same codepoint if no fallback exists.
func getCodepointFallback(cp Codepoint) Codepoint {
	switch cp {
	// Hyphen variants -> HYPHEN (U+2010)
	case 0x2011: // NON-BREAKING HYPHEN -> HYPHEN
		return 0x2010
	// Space variants -> SPACE (U+0020)
	case 0x00A0: // NO-BREAK SPACE -> SPACE
		return 0x0020
	case 0x2000: // EN QUAD -> SPACE
		return 0x0020
	case 0x2001: // EM QUAD -> SPACE
		return 0x0020
	case 0x2002: // EN SPACE -> SPACE
		return 0x0020
	case 0x2003: // EM SPACE -> SPACE
		return 0x0020
	case 0x2004: // THREE-PER-EM SPACE -> SPACE
		return 0x0020
	case 0x2005: // FOUR-PER-EM SPACE -> SPACE
		return 0x0020
	case 0x2006: // SIX-PER-EM SPACE -> SPACE
		return 0x0020
	case 0x2007: // FIGURE SPACE -> SPACE
		return 0x0020
	case 0x2008: // PUNCTUATION SPACE -> SPACE
		return 0x0020
	case 0x2009: // THIN SPACE -> SPACE
		return 0x0020
	case 0x200A: // HAIR SPACE -> SPACE
		return 0x0020
	case 0x202F: // NARROW NO-BREAK SPACE -> SPACE
		return 0x0020
	case 0x205F: // MEDIUM MATHEMATICAL SPACE -> SPACE
		return 0x0020
	}
	return cp
}

// setGlyphClasses sets GDEF glyph classes and updates GlyphProps accordingly.
// If no GDEF or no GlyphClassDef, falls back to synthesizing classes from Unicode.
// HarfBuzz equivalent: _hb_ot_layout_set_glyph_props() + hb_synthesize_glyph_classes()
func (s *Shaper) setGlyphClasses(buf *Buffer) {
	if s.gdef != nil && s.gdef.HasGlyphClasses() {
		// Use GDEF glyph classes and set corresponding GlyphProps
		for i := range buf.Info {
			glyphClass := s.gdef.GetGlyphClass(buf.Info[i].GlyphID)
			buf.Info[i].GlyphClass = glyphClass
			// Set GlyphProps based on GDEF class (preserve existing flags)
			// HarfBuzz: _hb_glyph_info_set_glyph_props() sets props based on GDEF class
			switch glyphClass {
			case 1: // BaseGlyph
				buf.Info[i].GlyphProps |= GlyphPropsBaseGlyph
			case 2: // Ligature
				buf.Info[i].GlyphProps |= GlyphPropsLigature
			case 3: // Mark
				buf.Info[i].GlyphProps |= GlyphPropsMark
			}
		}
	} else {
		// Fallback: synthesize glyph classes from Unicode General_Category
		// HarfBuzz: hb_synthesize_glyph_classes() in hb-ot-shape.cc:867-890
		s.synthesizeGlyphClasses(buf)
	}
}

// synthesizeGlyphClasses sets glyph classes based on Unicode General_Category.
// This is used as a fallback when the font has no GDEF GlyphClassDef table.
// HarfBuzz equivalent: hb_synthesize_glyph_classes() in hb-ot-shape.cc:867-890
func (s *Shaper) synthesizeGlyphClasses(buf *Buffer) {
	for i := range buf.Info {
		cp := buf.Info[i].Codepoint

		// HarfBuzz logic:
		// - If general_category is NON_SPACING_MARK and NOT default_ignorable → MARK
		// - Otherwise → BASE_GLYPH
		//
		// Comment from HarfBuzz:
		// "Never mark default-ignorables as marks. They won't get in the way of
		// lookups anyway, but having them as mark will cause them to be skipped
		// over if the lookup-flag says so, but at least for the Mongolian
		// variation selectors, looks like Uniscribe marks them as non-mark.
		// Some Mongolian fonts without GDEF rely on this."
		gc := getGeneralCategory(cp)
		if gc == GCNonSpacingMark && !IsDefaultIgnorable(cp) {
			buf.Info[i].GlyphClass = GlyphClassMark
			buf.Info[i].GlyphProps |= GlyphPropsMark
		} else {
			buf.Info[i].GlyphClass = GlyphClassBase
			buf.Info[i].GlyphProps |= GlyphPropsBaseGlyph
		}
	}
}

// setBaseAdvances sets the base advance widths from hmtx.
// For variable fonts, it also applies HVAR deltas.
// HarfBuzz equivalent: hb_ot_get_glyph_h_advances() in hb-ot-font.cc
//
// If hmtx is not available, uses upem/2 as default advance (HarfBuzz behavior).
func (s *Shaper) setBaseAdvances(buf *Buffer) {
	// HarfBuzz: default_advance = hb_face_get_upem (face) / 2 for horizontal
	// See hb-ot-hmtx-table.hh:272
	if s.hmtx == nil {
		// No hmtx table - use default advance of upem/2
		defaultAdvance := int16(s.face.Upem() / 2)
		for i := range buf.Info {
			buf.Pos[i].XAdvance = defaultAdvance
		}
		return
	}

	// Check if we need to apply HVAR deltas
	applyHvar := s.hvar != nil && s.hvar.HasData() && s.normalizedCoordsI != nil

	for i := range buf.Info {
		adv := s.hmtx.GetAdvanceWidth(buf.Info[i].GlyphID)

		// Apply HVAR delta if available
		if applyHvar {
			delta := s.hvar.GetAdvanceDelta(buf.Info[i].GlyphID, s.normalizedCoordsI)
			adv = uint16(int32(adv) + roundToInt(delta))
		}

		buf.Pos[i].XAdvance = int16(adv)
	}
}

// roundToInt rounds a float32 to the nearest int32.
func roundToInt(v float32) int32 {
	if v >= 0 {
		return int32(v + 0.5)
	}
	return int32(v - 0.5)
}

// applyGSUB applies GSUB features to the buffer.
// HarfBuzz equivalent: hb_ot_substitute_pre() in hb-ot-shape.cc
// This version works directly on the Buffer to preserve cluster information.
func (s *Shaper) applyGSUB(buf *Buffer, features []Feature) {
	if s.gsub == nil {
		return
	}

	// Compute variations_index once for the entire GSUB application
	// HarfBuzz: hb_ot_shape_plan_key_t::variations_index[] in hb-ot-shape.hh
	variationsIndex := s.gsub.FindVariationsIndex(s.normalizedCoordsI)

	// Apply 'rvrn' feature first (Required Variation Alternates)
	// HarfBuzz: hb-ot-shape.cc - setup_masks_features() adds rvrn with F_GLOBAL|F_HAS_FALLBACK
	// This feature allows fonts to specify alternate glyphs based on variation axis values.
	// It must be applied before all other features.
	rvrnTag := MakeTag('r', 'v', 'r', 'n')
	s.gsub.ApplyFeatureToBufferWithMaskAndVariations(rvrnTag, buf, s.gdef, MaskGlobal, s.font, variationsIndex)

	// Apply automatic fractions before other features
	s.applyAutomaticFractions(buf)

	// First, apply required features from the script/language system
	s.applyRequiredGSUBFeaturesToBufferWithVariations(buf, variationsIndex)

	// Apply each explicitly requested feature
	for _, f := range features {
		if f.Value == 0 {
			continue // Feature disabled
		}
		// TODO: Respect f.Start/f.End for partial feature application
		s.gsub.ApplyFeatureToBufferWithMaskAndVariations(f.Tag, buf, s.gdef, MaskGlobal, s.font, variationsIndex)
	}
}

// applyAutomaticFractions applies automatic fraction formatting.
// When a FRACTION SLASH (U+2044) is found, the 'frac' feature is applied.
// The 'frac' feature in fonts typically uses chained context substitution
// to apply 'numr' to digits before and 'dnom' to digits after the slash.
func (s *Shaper) applyAutomaticFractions(buf *Buffer) {
	if s.gsub == nil {
		return
	}

	// Check if there's a FRACTION SLASH in the buffer
	const fractionSlash = 0x2044
	slashIndex := -1
	for i, info := range buf.Info {
		if info.Codepoint == fractionSlash {
			slashIndex = i
			break
		}
	}

	if slashIndex == -1 {
		return
	}

	// Check if font has frac or (numr and dnom) features
	numrTag := MakeTag('n', 'u', 'm', 'r')
	dnomTag := MakeTag('d', 'n', 'o', 'm')
	fracTag := MakeTag('f', 'r', 'a', 'c')

	featureList, err := s.gsub.ParseFeatureList()
	if err != nil {
		return
	}

	hasNumr := featureList.FindFeature(numrTag) != nil
	hasDnom := featureList.FindFeature(dnomTag) != nil
	hasFrac := featureList.FindFeature(fracTag) != nil

	// HarfBuzz requires frac OR (numr AND dnom) to enable automatic fractions
	if !hasFrac && !(hasNumr && hasDnom) {
		return
	}

	// Find fraction boundaries (digits before and after slash)
	// Following HarfBuzz: hb-ot-shape.cc lines 715-747
	start := slashIndex
	end := slashIndex + 1

	// Find digits before the slash
	for start > 0 && isDigit(buf.Info[start-1].Codepoint) {
		start--
	}

	// Find digits after the slash
	for end < len(buf.Info) && isDigit(buf.Info[end].Codepoint) {
		end++
	}

	// Must have digits on both sides
	if start == slashIndex || end == slashIndex+1 {
		return
	}

	glyphs := buf.GlyphIDs()

	// Apply 'numr' to digits before the slash (and 'frac' if available)
	if hasNumr {
		preGlyphs := make([]GlyphID, slashIndex-start)
		copy(preGlyphs, glyphs[start:slashIndex])
		preGlyphs = s.gsub.ApplyFeatureWithGDEF(numrTag, preGlyphs, s.gdef, s.font)
		if hasFrac {
			preGlyphs = s.gsub.ApplyFeatureWithGDEF(fracTag, preGlyphs, s.gdef, s.font)
		}
		copy(glyphs[start:slashIndex], preGlyphs)
	}

	// Apply 'frac' to the fraction slash
	if hasFrac {
		slashGlyph := []GlyphID{glyphs[slashIndex]}
		slashGlyph = s.gsub.ApplyFeatureWithGDEF(fracTag, slashGlyph, s.gdef, s.font)
		glyphs[slashIndex] = slashGlyph[0]
	}

	// Apply 'dnom' to digits after the slash (and 'frac' if available)
	if hasDnom {
		postGlyphs := make([]GlyphID, end-slashIndex-1)
		copy(postGlyphs, glyphs[slashIndex+1:end])
		postGlyphs = s.gsub.ApplyFeatureWithGDEF(dnomTag, postGlyphs, s.gdef, s.font)
		if hasFrac {
			postGlyphs = s.gsub.ApplyFeatureWithGDEF(fracTag, postGlyphs, s.gdef, s.font)
		}
		copy(glyphs[slashIndex+1:end], postGlyphs)
	}

	// Update buffer with transformed glyphs
	for i, glyph := range glyphs {
		if i < len(buf.Info) {
			buf.Info[i].GlyphID = glyph
		}
	}
}

// isDigit returns true if the codepoint is a digit (0-9 or Arabic-Indic digits).
func isDigit(cp Codepoint) bool {
	// ASCII digits 0-9
	if cp >= 0x0030 && cp <= 0x0039 {
		return true
	}
	// Arabic-Indic digits U+0660-U+0669
	if cp >= 0x0660 && cp <= 0x0669 {
		return true
	}
	// Extended Arabic-Indic digits U+06F0-U+06F9
	if cp >= 0x06F0 && cp <= 0x06F9 {
		return true
	}
	return false
}

// applyFeatureToDigitsBefore applies the 'numr' feature to consecutive digits before position.
func (s *Shaper) applyFeatureToDigitsBefore(buf *Buffer, pos int) {
	numrTag := MakeTag('n', 'u', 'm', 'r')

	// Find the start of the digit sequence
	start := pos - 1
	for start >= 0 && isDigit(buf.Info[start].Codepoint) {
		start--
	}
	start++ // Move back to first digit

	if start >= pos {
		return // No digits before
	}

	// Extract glyphs for this range
	glyphs := make([]GlyphID, pos-start)
	for i := start; i < pos; i++ {
		glyphs[i-start] = buf.Info[i].GlyphID
	}

	// Apply numr feature
	newGlyphs := s.gsub.ApplyFeatureWithGDEF(numrTag, glyphs, s.gdef, s.font)

	// Update buffer with transformed glyphs
	for i, glyph := range newGlyphs {
		buf.Info[start+i].GlyphID = glyph
	}
}

// applyFeatureToDigitsAfter applies the 'dnom' feature to consecutive digits after position.
func (s *Shaper) applyFeatureToDigitsAfter(buf *Buffer, pos int) {
	dnomTag := MakeTag('d', 'n', 'o', 'm')

	// Find the end of the digit sequence
	start := pos + 1
	end := start
	for end < len(buf.Info) && isDigit(buf.Info[end].Codepoint) {
		end++
	}

	if start >= end {
		return // No digits after
	}

	// Extract glyphs for this range
	glyphs := make([]GlyphID, end-start)
	for i := start; i < end; i++ {
		glyphs[i-start] = buf.Info[i].GlyphID
	}

	// Apply dnom feature
	newGlyphs := s.gsub.ApplyFeatureWithGDEF(dnomTag, glyphs, s.gdef, s.font)

	// Update buffer with transformed glyphs
	for i, glyph := range newGlyphs {
		buf.Info[start+i].GlyphID = glyph
	}
}

// applyRequiredGSUBFeaturesToBuffer applies required features directly to the buffer.
// HarfBuzz equivalent: applying required feature lookups in hb_ot_substitute_pre()
// This preserves cluster information during substitution.
func (s *Shaper) applyRequiredGSUBFeaturesToBuffer(buf *Buffer) {
	s.applyRequiredGSUBFeaturesToBufferWithVariations(buf, VariationsNotFoundIndex)
}

// applyRequiredGSUBFeaturesToBufferWithVariations applies required GSUB features with FeatureVariations support.
func (s *Shaper) applyRequiredGSUBFeaturesToBufferWithVariations(buf *Buffer, variationsIndex uint32) {
	if s.gsub == nil {
		return
	}

	// Get the script list
	scriptList, err := s.gsub.ParseScriptList()
	if err != nil {
		return
	}

	// Get the default script/language system
	langSys := scriptList.GetDefaultScript()
	if langSys == nil {
		return
	}

	// Apply required feature if present
	if langSys.RequiredFeature >= 0 {
		featureList, err := s.gsub.ParseFeatureList()
		if err != nil {
			return
		}

		featureIdx := uint16(langSys.RequiredFeature)
		feature, err := featureList.GetFeature(langSys.RequiredFeature)
		if err == nil && feature != nil {
			// Check if this feature has a FeatureVariations substitution
			var lookups []uint16
			fv := s.gsub.GetFeatureVariations()
			if variationsIndex != VariationsNotFoundIndex && fv != nil {
				lookups = fv.GetSubstituteLookups(variationsIndex, featureIdx)
			}
			// Use original lookups if no substitution
			if lookups == nil {
				lookups = feature.Lookups
			}

			// Apply all lookups from the required feature
			for _, lookupIdx := range lookups {
				s.gsub.ApplyLookupToBuffer(int(lookupIdx), buf, s.gdef, s.font)
			}
		}
	}
}

// updateBufferFromGlyphsWithCodepoints updates the buffer after GSUB processing,
// including codepoint information for default ignorable tracking.
func (s *Shaper) updateBufferFromGlyphsWithCodepoints(buf *Buffer, glyphs []GlyphID, codepoints []Codepoint) {
	// If length changed, we need to rebuild the buffer
	if len(glyphs) != len(buf.Info) {
		// Create new info array
		newInfo := make([]GlyphInfo, len(glyphs))

		// Simple cluster preservation: try to maintain clusters
		// This is a simplified approach
		oldLen := len(buf.Info)
		for i, glyph := range glyphs {
			newInfo[i].GlyphID = glyph
			// Map cluster from original position (simplified)
			if i < oldLen {
				newInfo[i].Cluster = buf.Info[i].Cluster
				// Use codepoint from new codepoints slice if available
				if codepoints != nil && i < len(codepoints) {
					newInfo[i].Codepoint = codepoints[i]
				} else {
					newInfo[i].Codepoint = buf.Info[i].Codepoint
				}
			} else if oldLen > 0 {
				// For added glyphs, use last cluster
				newInfo[i].Cluster = buf.Info[oldLen-1].Cluster
				if codepoints != nil && i < len(codepoints) {
					newInfo[i].Codepoint = codepoints[i]
				}
			}
			// Update glyph class
			if s.gdef != nil && s.gdef.HasGlyphClasses() {
				newInfo[i].GlyphClass = s.gdef.GetGlyphClass(glyph)
			}
		}

		buf.Info = newInfo
		buf.Pos = make([]GlyphPos, len(glyphs))
	} else {
		// Same length, just update glyph IDs, classes and codepoints
		for i, glyph := range glyphs {
			buf.Info[i].GlyphID = glyph
			if codepoints != nil && i < len(codepoints) {
				buf.Info[i].Codepoint = codepoints[i]
			}
			if s.gdef != nil && s.gdef.HasGlyphClasses() {
				buf.Info[i].GlyphClass = s.gdef.GetGlyphClass(glyph)
			}
		}
	}
}

// applyGPOS applies GPOS features to the buffer.
// HarfBuzz equivalent: hb_ot_position() in hb-ot-shape.cc:1038-1095
func (s *Shaper) applyGPOS(buf *Buffer, features []Feature) {
	s.applyGPOSWithZeroWidthMarks(buf, features, ZeroWidthMarksNone)
}

// applyGPOSWithZeroWidthMarks applies GPOS features with zero-width-marks mode.
// HarfBuzz equivalent: hb_ot_position_complex() in hb-ot-shape.cc:1008-1095
//
// The zeroWidthMarksMode parameter controls when mark advances are zeroed:
// - ZeroWidthMarksNone: Don't zero (caller handles it)
// - ZeroWidthMarksByGDEFEarly: Zero before GPOS lookups (not used here)
// - ZeroWidthMarksByGDEFLate: Zero after GPOS lookups, before PropagateAttachmentOffsets
//
// CRITICAL: For LATE mode, mark advances MUST be zeroed BEFORE PropagateAttachmentOffsets!
// HarfBuzz sequence (hb-ot-shape.cc:1070-1086):
//   1. GPOS lookups
//   2. zero_mark_widths_by_gdef (LATE mode)
//   3. hb_ot_zero_width_default_ignorables
//   4. position_finish_offsets (PropagateAttachmentOffsets)
func (s *Shaper) applyGPOSWithZeroWidthMarks(buf *Buffer, features []Feature, zeroWidthMarksMode ZeroWidthMarksType) {
	// HarfBuzz: hb_ot_position_complex() in hb-ot-shape.cc:1032-1095
	// IMPORTANT: zero_mark_widths_by_gdef is called REGARDLESS of whether GPOS is present!
	// HarfBuzz checks c->plan->zero_marks which is independent of GPOS features.

	// Clear attachment chains before applying GPOS
	// HarfBuzz: GPOS::position_start()
	for i := range buf.Pos {
		buf.Pos[i].AttachChain = 0
		buf.Pos[i].AttachType = 0
	}

	// Track if we added h_origins (need to subtract them back later)
	addedHOrigins := false

	// Only apply GPOS if we have the table and features
	if s.gpos != nil && len(features) > 0 {
		// We change glyph origin to what GPOS expects (horizontal), apply GPOS, change it back.
		// HarfBuzz: hb-ot-shape.cc:1047-1051
		//
		// h_origin defaults to zero; only apply it if the font has it.
		// For most horizontal fonts, h_origins are (0, 0), so this is a no-op.
		// For fonts with v_origins (vertical fonts), we convert v_origins to h_origins.
		if s.hasGlyphHOrigins() {
			s.addGlyphHOrigins(buf)
			addedHOrigins = true
		}

		// Compile OTMap and apply all GPOS lookups
		// HarfBuzz equivalent: hb_ot_map_t::apply() in hb-ot-layout.cc:2010-2060
		// CRITICAL: Pass script/language for script-specific feature selection
		otMap := CompileMap(nil, s.gpos, features, buf.Script, buf.Language)
		otMap.ApplyGPOS(s.gpos, buf, s.font, s.gdef)
	}

	// Zero mark widths by GDEF (LATE mode)
	// HarfBuzz: zero_mark_widths_by_gdef() in hb-ot-shape.cc:1070-1075
	// CRITICAL: Called REGARDLESS of whether GPOS is present or has features!
	// Must be done BEFORE PropagateAttachmentOffsets for correct offset calculation!
	if zeroWidthMarksMode == ZeroWidthMarksByGDEFLate {
		s.zeroMarkWidthsByGDEF(buf)
	}

	// Zero width of default ignorables
	// HarfBuzz: hb_ot_zero_width_default_ignorables() in hb-ot-shape.cc:1085
	zeroWidthDefaultIgnorables(buf)

	// DEBUG: Print state before PropagateAttachmentOffsets
	if debugGPOS {
		for i := range buf.Info {
			if buf.Pos[i].AttachChain != 0 || buf.Pos[i].XOffset != 0 {
				debugPrintf("Before PropagateAttachmentOffsets: [%d] gid=%d cluster=%d xoff=%d chain=%d type=%d xadv=%d\n",
					i, buf.Info[i].GlyphID, buf.Info[i].Cluster, buf.Pos[i].XOffset,
					buf.Pos[i].AttachChain, buf.Pos[i].AttachType, buf.Pos[i].XAdvance)
			}
		}
	}

	// Propagate attachment offsets (cursive → marks)
	// This must be done after all GPOS lookups have set up the attachment chains
	// AND after mark advances have been zeroed!
	// HarfBuzz: GPOS::position_finish_offsets() in hb-ot-shape.cc:1086
	PropagateAttachmentOffsets(buf.Pos, buf.Direction)

	// DEBUG: Print state after PropagateAttachmentOffsets
	if debugGPOS {
		for i := range buf.Info {
			if buf.Pos[i].XOffset != 0 {
				debugPrintf("After PropagateAttachmentOffsets: [%d] gid=%d cluster=%d xoff=%d\n",
					i, buf.Info[i].GlyphID, buf.Info[i].Cluster, buf.Pos[i].XOffset)
			}
		}
	}

	// Fallback mark positioning when GPOS is not available
	// HarfBuzz equivalent: _hb_ot_shape_fallback_mark_position() in hb-ot-shape-fallback.cc
	// Called after GPOS lookups when the font has no mark positioning tables.
	//
	// Only apply fallback positioning for shapers that support it.
	// Shapers with ZeroWidthMarksNone (like Qaag) typically have fallback_position = false.
	if s.gpos == nil && zeroWidthMarksMode != ZeroWidthMarksNone {
		s.fallbackMarkPosition(buf)
	}

	// Subtract h_origins back (change from GPOS horizontal coordinate system to original)
	// HarfBuzz: hb-ot-shape.cc:1088-1090
	if addedHOrigins {
		s.subtractGlyphHOrigins(buf)
	}
}

// zeroWidthDefaultIgnorables zeros advance widths and offsets of default ignorables.
// HarfBuzz equivalent: hb_ot_zero_width_default_ignorables() in hb-ot-shape.cc:783-803
//
// This is called AFTER GPOS lookups, but BEFORE PropagateAttachmentOffsets.
// The order is critical: hb-ot-shape.cc:1083-1086:
//   1. position_finish_advances
//   2. hb_ot_zero_width_default_ignorables  <-- HERE
//   3. position_finish_offsets
//
// Without this, default ignorables (like CGJ U+034F) would contribute their
// XAdvance to the offset calculation in PropagateAttachmentOffsets.
func zeroWidthDefaultIgnorables(buf *Buffer) {
	for i := range buf.Info {
		// Check if this is a default ignorable that hasn't been substituted
		// HarfBuzz: Uses GlyphPropsDefaultIgnorable flag set during AddCodepoints
		if (buf.Info[i].GlyphProps&GlyphPropsDefaultIgnorable) != 0 &&
			(buf.Info[i].GlyphProps&GlyphPropsSubstituted) == 0 {
			// Zero both advances
			buf.Pos[i].XAdvance = 0
			buf.Pos[i].YAdvance = 0
			// Zero the main-direction offset
			// HarfBuzz: zeros x_offset for horizontal, y_offset for vertical
			if buf.Direction.IsHorizontal() {
				buf.Pos[i].XOffset = 0
			} else {
				buf.Pos[i].YOffset = 0
			}
		}
	}
}

// zeroMarkWidthsByGDEF zeros the advance widths of mark glyphs.
// HarfBuzz equivalent: zero_mark_widths_by_gdef() in hb-ot-shape.cc:992-1002
//
// This is called AFTER GPOS positioning (LATE mode for most shapers).
// When GPOS has been applied, we don't adjust offsets - just zero advances.
func (s *Shaper) zeroMarkWidthsByGDEF(buf *Buffer) {
	for i := range buf.Pos {
		if buf.Info[i].GlyphClass == GlyphClassMark {
			// Zero BOTH advances (not just in-direction!)
			// HarfBuzz zeros both x_advance and y_advance regardless of direction
			buf.Pos[i].XAdvance = 0
			buf.Pos[i].YAdvance = 0
		}
	}
}

// hasGlyphHOrigins returns true if the font has horizontal glyph origins.
// HarfBuzz equivalent: font->has_glyph_h_origin_func() in hb-font.hh
//
// For most horizontal fonts, h_origins are (0, 0), so this returns false.
// For fonts with v_origins (vertical/CJK fonts), h_origins can be derived from v_origins.
func (s *Shaper) hasGlyphHOrigins() bool {
	// TODO: Implement proper h_origin detection.
	// For now, return false since most fonts don't have h_origins.
	// When we add vmtx support, check for vmtx table here.
	//
	// HarfBuzz checks: font->has_glyph_h_origin_func() || font->has_glyph_v_origins_func()
	// If vmtx exists, we can derive h_origins from v_origins.
	return false
}

// addGlyphHOrigins adds horizontal glyph origins to buffer positions.
// HarfBuzz equivalent: font->add_glyph_h_origins() in hb-font.hh:866-932
//
// This transforms positions from font coordinate space to GPOS coordinate space.
// GPOS expects positions in horizontal origin space.
func (s *Shaper) addGlyphHOrigins(buf *Buffer) {
	// For most fonts, h_origins are (0, 0), so this is a no-op.
	// This is called when hasGlyphHOrigins() returns true.
	//
	// HarfBuzz implementation (apply_glyph_h_origins_with_fallback):
	// 1. Try to get h_origins for each glyph
	// 2. If no h_origins, try v_origins and convert:
	//    origin.x -= advance / 2
	//    origin.y -= ascender
	// 3. If neither, use (0, 0)
	// 4. Add origins to x_offset and y_offset
	//
	// Since hasGlyphHOrigins() returns false, this function is never called.
	// When we implement vmtx support, we'll implement this properly.
}

// subtractGlyphHOrigins subtracts horizontal glyph origins from buffer positions.
// HarfBuzz equivalent: font->subtract_glyph_h_origins() in hb-font.hh:866-932
//
// This transforms positions from GPOS coordinate space back to font coordinate space.
func (s *Shaper) subtractGlyphHOrigins(buf *Buffer) {
	// For most fonts, h_origins are (0, 0), so this is a no-op.
	// This is called when hasGlyphHOrigins() returns true.
	//
	// HarfBuzz implementation:
	// 1. Get the same h_origins as addGlyphHOrigins()
	// 2. Subtract origins from x_offset and y_offset
	//
	// Since hasGlyphHOrigins() returns false, this function is never called.
	// When we implement vmtx support, we'll implement this properly.
}

// scriptAllowsKernFallback returns true if the script allows legacy kern table fallback.
// Some scripts (Indic, Khmer, Myanmar, Thai, USE-based) use the 'dist' feature instead
// of 'kern' and should not apply legacy kern tables.
func scriptAllowsKernFallback(script Tag) bool {
	// Scripts that use 'dist' feature and should NOT use legacy kern fallback
	// Based on HarfBuzz shaper fallback_position settings
	switch script {
	// Indic scripts
	case MakeTag('D', 'e', 'v', 'a'), // Devanagari
		MakeTag('B', 'e', 'n', 'g'), // Bengali
		MakeTag('G', 'u', 'r', 'u'), // Gurmukhi
		MakeTag('G', 'u', 'j', 'r'), // Gujarati
		MakeTag('O', 'r', 'y', 'a'), // Oriya
		MakeTag('T', 'a', 'm', 'l'), // Tamil
		MakeTag('T', 'e', 'l', 'u'), // Telugu
		MakeTag('K', 'n', 'd', 'a'), // Kannada
		MakeTag('M', 'l', 'y', 'm'), // Malayalam
		MakeTag('S', 'i', 'n', 'h'), // Sinhala
		// Southeast Asian scripts
		MakeTag('K', 'h', 'm', 'r'), // Khmer
		MakeTag('M', 'y', 'm', 'r'), // Myanmar
		MakeTag('T', 'h', 'a', 'i'), // Thai
		MakeTag('L', 'a', 'o', 'o'), // Lao
		// Tibetan and related
		MakeTag('T', 'i', 'b', 't'), // Tibetan
		// USE-based scripts
		MakeTag('J', 'a', 'v', 'a'), // Javanese
		MakeTag('B', 'a', 'l', 'i'), // Balinese
		MakeTag('S', 'u', 'n', 'd'), // Sundanese
		MakeTag('R', 'j', 'n', 'g'), // Rejang
		MakeTag('L', 'e', 'p', 'c'), // Lepcha
		MakeTag('B', 'u', 'g', 'i'), // Buginese
		MakeTag('M', 'a', 'k', 'a'), // Makasar
		MakeTag('B', 'a', 't', 'k'), // Batak
		MakeTag('T', 'a', 'l', 'u'), // New Tai Lue
		MakeTag('T', 'a', 'v', 't'), // Tai Viet
		MakeTag('C', 'h', 'a', 'm'), // Cham
		MakeTag('K', 'a', 'l', 'i'), // Kayah Li
		MakeTag('T', 'g', 'l', 'g'), // Tagalog
		MakeTag('H', 'a', 'n', 'o'), // Hanunoo
		MakeTag('B', 'u', 'h', 'd'), // Buhid
		MakeTag('T', 'a', 'g', 'b'): // Tagbanwa
		return false
	default:
		return true
	}
}

// applyKernTableFallback applies TrueType kern table kerning.
// This is used as a fallback when GPOS is not available or has no kern feature.
// The kerning is applied like HarfBuzz: split evenly between the two glyphs,
// with the second glyph also getting an x_offset adjustment.
func (s *Shaper) applyKernTableFallback(buf *Buffer, features []Feature) {
	if s.kern == nil || !s.kern.HasKerning() {
		return
	}

	// Check if script allows kern fallback (Indic/USE scripts use 'dist' instead)
	if !scriptAllowsKernFallback(buf.Script) {
		return
	}

	// Check if kern feature is requested and enabled
	kernEnabled := false
	for _, f := range features {
		if f.Tag == TagKern && f.Value > 0 {
			kernEnabled = true
			break
		}
	}
	if !kernEnabled {
		return
	}

	// Check if GPOS already has kern feature (don't apply twice)
	if s.gpos != nil {
		if featureList, err := s.gpos.ParseFeatureList(); err == nil {
			if featureList.FindFeature(TagKern) != nil {
				return // GPOS has kern, don't use fallback
			}
		}
	}

	// Apply kern table kerning like HarfBuzz
	horizontal := buf.Direction.IsHorizontal()
	glyphs := buf.GlyphIDs()

	for i := 0; i < len(glyphs)-1; i++ {
		// Skip marks (simplified check - proper implementation would use GDEF)
		if buf.Info[i].GlyphClass == GlyphClassMark {
			continue
		}

		// Find next non-mark glyph
		j := i + 1
		for j < len(glyphs) && buf.Info[j].GlyphClass == GlyphClassMark {
			j++
		}
		if j >= len(glyphs) {
			break
		}

		kern := s.kern.KernPair(glyphs[i], glyphs[j])
		if kern == 0 {
			continue
		}

		// Split kern value like HarfBuzz
		kern1 := kern >> 1
		kern2 := kern - kern1

		if horizontal {
			buf.Pos[i].XAdvance += kern1
			buf.Pos[j].XAdvance += kern2
			buf.Pos[j].XOffset += kern2
		} else {
			buf.Pos[i].YAdvance += kern1
			buf.Pos[j].YAdvance += kern2
			buf.Pos[j].YOffset += kern2
		}
	}
}

// GuessDirection guesses text direction from the content.
func GuessDirection(s string) Direction {
	for _, r := range s {
		if unicode.Is(unicode.Arabic, r) ||
			unicode.Is(unicode.Hebrew, r) ||
			unicode.Is(unicode.Syriac, r) ||
			unicode.Is(unicode.Thaana, r) {
			return DirectionRTL
		}
		if unicode.IsLetter(r) {
			return DirectionLTR
		}
	}
	return DirectionLTR
}

// ShapeString is a convenience function that shapes a string and returns
// the glyph IDs and positions.
func (s *Shaper) ShapeString(text string) ([]GlyphID, []GlyphPos) {
	buf := NewBuffer()
	buf.AddString(text)
	buf.SetDirection(GuessDirection(text))
	s.Shape(buf, nil) // Use default features
	return buf.GlyphIDs(), buf.Pos
}

// HasGSUB returns true if the shaper has GSUB data.
func (s *Shaper) HasGSUB() bool {
	return s.gsub != nil
}

// HasGPOS returns true if the shaper has GPOS data.
func (s *Shaper) HasGPOS() bool {
	return s.gpos != nil
}

// HasGDEF returns true if the shaper has GDEF data.
func (s *Shaper) HasGDEF() bool {
	return s.gdef != nil
}

// HasHmtx returns true if the shaper has hmtx data.
func (s *Shaper) HasHmtx() bool {
	return s.hmtx != nil
}

// GDEF returns the GDEF table (may be nil).
func (s *Shaper) GDEF() *GDEF {
	return s.gdef
}

// SetDefaultFeatures sets the default features to apply when Shape is called with nil.
func (s *Shaper) SetDefaultFeatures(features []Feature) {
	s.defaultFeatures = features
}

// DefaultFeatures returns the current default features.
func (s *Shaper) GetDefaultFeatures() []Feature {
	return s.defaultFeatures
}

// Font returns the font associated with this shaper.
func (s *Shaper) Font() *Font {
	return s.font
}

// GetGlyphName returns a debug name for a glyph (just the ID as string).
func GetGlyphName(glyph GlyphID) string {
	return string(rune('A' + int(glyph)%26)) // Simple debug representation
}

// Shaper cache for convenience function
var shaperCache = make(map[*Font]*Shaper)
var shaperCacheMu sync.RWMutex

// Shape is a convenience function that shapes text in a buffer using a font.
// It caches shapers internally for efficiency.
// This is similar to HarfBuzz's hb_shape() function.
func Shape(font *Font, buf *Buffer, features []Feature) error {
	shaperCacheMu.RLock()
	shaper, ok := shaperCache[font]
	shaperCacheMu.RUnlock()

	if !ok {
		var err error
		shaper, err = NewShaper(font)
		if err != nil {
			return err
		}

		shaperCacheMu.Lock()
		shaperCache[font] = shaper
		shaperCacheMu.Unlock()
	}

	shaper.Shape(buf, features)
	return nil
}

// ClearShaperCache clears the internal shaper cache.
// Call this if fonts are being released to allow garbage collection.
func ClearShaperCache() {
	shaperCacheMu.Lock()
	shaperCache = make(map[*Font]*Shaper)
	shaperCacheMu.Unlock()
}
