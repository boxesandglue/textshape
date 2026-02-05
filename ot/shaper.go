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
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"unicode"
)

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

	// USECategory holds the USE character category for USE shaping.
	// HarfBuzz equivalent: use_category() stored in var2.u8[3] via HB_BUFFER_ALLOCATE_VAR
	// Stored directly on GlyphInfo so it survives GSUB operations.
	USECategory uint8

	// HangulFeature holds the Hangul Jamo feature type for Hangul shaping.
	// HarfBuzz equivalent: hangul_shaping_feature() stored via ot_shaper_var_u8_auxiliary()
	// Values: 0=none, 1=LJMO, 2=VJMO, 3=TJMO
	HangulFeature uint8
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
//
//	Bits 7-5: lig_id (3 bits, values 0-7)
//	Bit 4:    IS_LIG_BASE (set for ligature glyphs, unset for marks/components)
//	Bits 3-0: lig_comp or lig_num_comps (4 bits, values 0-15)
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
	Script             Tag
	Language           Tag
	LanguageCandidates []Tag // Multiple language candidates in priority order (BCP47→OT may produce multiple)

	// PreContext and PostContext hold Unicode codepoints that surround the text being shaped.
	// Used for Arabic joining: context characters affect the joining form of the first/last glyphs.
	// HarfBuzz equivalent: context[2][CONTEXT_LENGTH] and context_len[2] in hb-buffer.hh:110-111
	PreContext  []Codepoint
	PostContext []Codepoint

	// ScratchFlags holds temporary flags used during shaping.
	// HarfBuzz equivalent: scratch_flags in hb-buffer.hh
	ScratchFlags ScratchFlags

	// RandomState for the 'rand' feature's pseudo-random number generator.
	// HarfBuzz equivalent: random_state in hb-buffer.hh
	// Uses minstd_rand: state = state * 48271 % 2147483647
	// Initial value 1 (set in NewBuffer/reset).
	RandomState uint32

	// ClusterLevel controls cluster merging behavior.
	// HarfBuzz equivalent: cluster_level in hb-buffer.hh
	// 0 = MONOTONE_GRAPHEMES (default): merge marks into base, monotone order
	// 1 = MONOTONE_CHARACTERS: keep marks separate, monotone order
	// 2 = CHARACTERS: keep marks separate, no monotone enforcement
	// 3 = GRAPHEMES: merge marks into base, no monotone enforcement
	ClusterLevel int

	// NotFoundVSGlyph is the glyph ID to use for variation selectors that are
	// not found in the font. -1 means not set (VS will be removed from buffer).
	// HarfBuzz equivalent: not_found_variation_selector in hb-buffer.hh
	NotFoundVSGlyph int
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
		RandomState:     1,  // HarfBuzz: random_state initialized to 1
		NotFoundVSGlyph: -1, // HarfBuzz: HB_CODEPOINT_INVALID
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

	// HarfBuzz: merge_clusters_impl() in hb-buffer.cc:550-554
	// If cluster level is NOT monotone (level 2 or 3), skip actual merging.
	// HB_BUFFER_CLUSTER_LEVEL_IS_MONOTONE = level 0 or 1
	if b.ClusterLevel != 0 && b.ClusterLevel != 1 {
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

// mergeClustersSlice merges cluster values in a range of GlyphInfo.
// HarfBuzz equivalent: hb_buffer_t::merge_clusters_impl() in hb-buffer.cc:547-582
// Finds the minimum cluster in [start, end), extends the range to include
// adjacent glyphs with the same cluster values, then sets all to the minimum.
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

	// Extend end: include following glyphs with the same cluster as the last glyph
	// HarfBuzz: hb-buffer.cc:567-568
	if minCluster != info[end-1].Cluster {
		for end < len(info) && info[end-1].Cluster == info[end].Cluster {
			end++
		}
	}

	// Extend start: include preceding glyphs with the same cluster as the first glyph
	// HarfBuzz: hb-buffer.cc:571-573
	if minCluster != info[start].Cluster {
		for start > 0 && info[start-1].Cluster == info[start].Cluster {
			start--
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

// isRegionalIndicator returns true if the codepoint is a Regional Indicator Symbol (U+1F1E6-U+1F1FF).
// Pairs of Regional Indicators form flag emoji and should be merged into one cluster.
func isRegionalIndicator(cp Codepoint) bool {
	return cp >= 0x1F1E6 && cp <= 0x1F1FF
}

// formClusters merges clusters for grapheme groups (base + continuations).
// HarfBuzz equivalent: hb_form_clusters() in hb-ot-shape.cc:577-589
// This ensures that a base character and its continuations share the same cluster.
func formClusters(buf *Buffer) {
	if len(buf.Info) < 2 {
		return
	}

	// First pass: determine continuation flags for each glyph.
	// HarfBuzz equivalent: hb_set_unicode_props() in hb-ot-shape.cc:470-546
	// This handles context-dependent continuations:
	// - ZWJ (U+200D) is a continuation
	// - Extended_Pictographic character after ZWJ is also a continuation
	// - Second Regional Indicator in a pair is a continuation (flag emoji)
	n := len(buf.Info)
	isCont := make([]bool, n)
	for i := 0; i < n; i++ {
		cp := buf.Info[i].Codepoint
		if isContinuation(cp) {
			isCont[i] = true
			// HarfBuzz: if ZWJ and next char is Extended_Pictographic, mark it as continuation too
			if cp == 0x200D && i+1 < n && isExtendedPictographic(buf.Info[i+1].Codepoint) {
				i++
				isCont[i] = true
			}
		} else if i > 0 && isRegionalIndicator(cp) && isRegionalIndicator(buf.Info[i-1].Codepoint) {
			// HarfBuzz: RI + RI → second RI is continuation (flag emoji pair)
			isCont[i] = true
		}
	}

	// Second pass: merge or mark clusters for grapheme groups (base + continuations)
	// HarfBuzz: hb-ot-shape.cc:583-588
	// IS_GRAPHEMES (level 0,3): merge marks into base cluster
	// !IS_GRAPHEMES (level 1,2): just mark unsafe_to_break, keep marks separate
	isGraphemes := buf.ClusterLevel == 0 || buf.ClusterLevel == 3
	start := 0
	for i := 1; i < n; i++ {
		if isCont[i] {
			continue
		}
		// This is a new base - process the previous grapheme
		if i > start+1 {
			if isGraphemes {
				buf.MergeClusters(start, i)
			}
		}
		start = i
	}
	// Process the last grapheme
	if n > start+1 {
		if isGraphemes {
			buf.MergeClusters(start, n)
		}
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

// replaceGlyphs replaces numIn input glyphs with numOut output glyphs.
// HarfBuzz equivalent: hb_buffer_t::replace_glyphs() in hb-buffer.hh:319-327
func (b *Buffer) replaceGlyphs(numIn, numOut int, codepoints []Codepoint) {
	for i := 0; i < numOut; i++ {
		info := b.Info[b.Idx]
		info.Codepoint = codepoints[i]
		info.GlyphID = 0
		b.outInfo = append(b.outInfo, info)
		b.outLen++
	}
	b.Idx += numIn
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
	gvar *Gvar // TrueType glyph variations (for advance deltas when no HVAR)
	vmtx *Vmtx // Vertical metrics
	vhea *Vhea // Vertical header
	vorg *VORG // Vertical origin (CFF/CFF2 fonts)

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

	// Script-specific compose filter callback.
	// HarfBuzz equivalent: plan->shaper->compose in hb-ot-shape-normalize.cc:448
	// If set, this is called before recomposition. Return false to prevent composition.
	composeFilter func(a, b Codepoint) bool

	// Arabic fallback shaping plan.
	// Used when font has no GSUB but has Unicode Arabic Presentation Forms.
	// HarfBuzz equivalent: arabic_fallback_plan_t in hb-ot-shaper-arabic-fallback.hh
	arabicFallbackPlan *arabicFallbackPlan

	// Indic shaping plans - one per script.
	// HarfBuzz equivalent: indic_shape_plan_t in hb-ot-shaper-indic.cc:289-308
	// Lazily initialized when first shaping Indic text.
	indicPlans map[Tag]*IndicPlan

	// Synthetic bold/slant (HarfBuzz: hb_font_set_synthetic_bold / hb_font_set_synthetic_slant)
	xEmbolden       float32
	yEmbolden       float32
	emboldenInPlace bool
	slant           float32

	// Computed bold strengths in font units (xStrength = round(upem * xEmbolden))
	xStrength int16
	yStrength int16
}

// SetSyntheticBold sets synthetic bold parameters.
// x and y are embolden strengths in em-units (e.g. 0.02).
// If inPlace is true, advances and origins are not modified (only extents/drawing).
// HarfBuzz equivalent: hb_font_set_synthetic_bold()
func (s *Shaper) SetSyntheticBold(x, y float32, inPlace bool) {
	s.xEmbolden = x
	s.yEmbolden = y
	s.emboldenInPlace = inPlace
	s.xStrength = int16(math.Round(float64(s.face.Upem()) * float64(x)))
	s.yStrength = int16(math.Round(float64(s.face.Upem()) * float64(y)))
}

// SyntheticBold returns the current synthetic bold parameters.
func (s *Shaper) SyntheticBold() (x, y float32, inPlace bool) {
	return s.xEmbolden, s.yEmbolden, s.emboldenInPlace
}

// SetSyntheticSlant sets the synthetic slant value.
// HarfBuzz equivalent: hb_font_set_synthetic_slant()
// Slant has no effect on shaping positions; it only affects extents and drawing.
func (s *Shaper) SetSyntheticSlant(slant float32) {
	s.slant = slant
}

// SyntheticSlant returns the current synthetic slant value.
func (s *Shaper) SyntheticSlant() float32 {
	return s.slant
}

// getGlyphHAdvanceWithBold returns the horizontal advance for a glyph including
// synthetic bold. Used internally for v-origin calculations.
func (s *Shaper) getGlyphHAdvanceWithBold(glyph GlyphID) int16 {
	adv := int16(s.GetGlyphHAdvanceVar(glyph))
	if s.xStrength != 0 && !s.emboldenInPlace && adv != 0 {
		adv += s.xStrength
	}
	return adv
}

// isGDEFBlocklisted checks if a font's GDEF table is known to be broken.
// HarfBuzz equivalent: hb-ot-layout.cc:152-266 (GDEF::is_blocklisted)
// Fonts like certain versions of Times New Roman, Tahoma, Courier New, etc.
// have GDEF tables that incorrectly classify base glyphs as marks, causing
// their advances to be zeroed by zeroMarkWidthsByGDEF.
func isGDEFBlocklisted(gdefLen, gsubLen, gposLen int) bool {
	// HarfBuzz uses HB_CODEPOINT_ENCODE3(x,y,z) = (x<<42)|(y<<21)|z
	key := (uint64(gdefLen) << 42) | (uint64(gsubLen) << 21) | uint64(gposLen)
	switch key {
	case (442 << 42) | (2874 << 21) | 42038, // timesi.ttf Windows 7
		(430 << 42) | (2874 << 21) | 40662,    // timesbi.ttf Windows 7
		(442 << 42) | (2874 << 21) | 39116,    // timesi.ttf Windows 7
		(430 << 42) | (2874 << 21) | 39374,    // timesbi.ttf Windows 7
		(490 << 42) | (3046 << 21) | 41638,    // Times New Roman Italic OS X 10.11.3
		(478 << 42) | (3046 << 21) | 41902,    // Times New Roman Bold Italic OS X 10.11.3
		(898 << 42) | (12554 << 21) | 46470,   // tahoma.ttf Windows 8
		(910 << 42) | (12566 << 21) | 47732,   // tahomabd.ttf Windows 8
		(928 << 42) | (23298 << 21) | 59332,   // tahoma.ttf Windows 8.1
		(940 << 42) | (23310 << 21) | 60732,   // tahomabd.ttf Windows 8.1
		(964 << 42) | (23836 << 21) | 60072,   // tahoma.ttf v6.04 Windows 8.1 x64
		(976 << 42) | (23832 << 21) | 61456,   // tahomabd.ttf v6.04 Windows 8.1 x64
		(994 << 42) | (24474 << 21) | 60336,   // tahoma.ttf Windows 10
		(1006 << 42) | (24470 << 21) | 61740,  // tahomabd.ttf Windows 10
		(1006 << 42) | (24576 << 21) | 61346,  // tahoma.ttf v6.91 Windows 10 x64
		(1018 << 42) | (24572 << 21) | 62828,  // tahomabd.ttf v6.91 Windows 10 x64
		(1006 << 42) | (24576 << 21) | 61352,  // tahoma.ttf Windows 10 AU
		(1018 << 42) | (24572 << 21) | 62834,  // tahomabd.ttf Windows 10 AU
		(832 << 42) | (7324 << 21) | 47162,    // Tahoma.ttf Mac OS X 10.9
		(844 << 42) | (7302 << 21) | 45474,    // Tahoma Bold.ttf Mac OS X 10.9
		(180 << 42) | (13054 << 21) | 7254,    // himalaya.ttf Windows 7
		(192 << 42) | (12638 << 21) | 7254,    // himalaya.ttf Windows 8
		(192 << 42) | (12690 << 21) | 7254,    // himalaya.ttf Windows 8.1
		(188 << 42) | (248 << 21) | 3852,      // Cantarell-Regular/Oblique 0.0.21
		(188 << 42) | (264 << 21) | 3426,      // Cantarell-Bold/Bold-Oblique 0.0.21
		(1058 << 42) | (47032 << 21) | 11818,  // Padauk.ttf RHEL 7.2
		(1046 << 42) | (47030 << 21) | 12600,  // Padauk-Bold.ttf RHEL 7.2
		(1058 << 42) | (71796 << 21) | 16770,  // Padauk.ttf Ubuntu 16.04
		(1046 << 42) | (71790 << 21) | 17862,  // Padauk-Bold.ttf Ubuntu 16.04
		(1046 << 42) | (71788 << 21) | 17112,  // Padauk-book.ttf 2.80
		(1058 << 42) | (71794 << 21) | 17514,  // Padauk-bookbold.ttf 2.80
		(1330 << 42) | (109904 << 21) | 57938, // Padauk-book.ttf 3.0
		(1330 << 42) | (109904 << 21) | 58972, // Padauk-bookbold.ttf 3.0
		(1004 << 42) | (59092 << 21) | 14836,  // Padauk.ttf v2.5
		(588 << 42) | (5078 << 21) | 14418,    // Courier New.ttf macOS 15
		(588 << 42) | (5078 << 21) | 14238,    // Courier New Bold.ttf macOS 15
		(894 << 42) | (17162 << 21) | 33960,   // cour.ttf Windows 10
		(894 << 42) | (17154 << 21) | 34472,   // courbd.ttf Windows 10
		(816 << 42) | (7868 << 21) | 17052,    // cour.ttf Windows 8.1
		(816 << 42) | (7868 << 21) | 17138:    // courbd.ttf Windows 8.1
		return true
	}
	return false
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
	var gsubLen int
	if font.HasTable(TagGSUB) {
		data, err := font.TableData(TagGSUB)
		if err == nil {
			gsubLen = len(data)
			s.gsub, _ = ParseGSUB(data)
		}
	}

	// Parse GPOS (optional)
	var gposLen int
	if font.HasTable(TagGPOS) {
		data, err := font.TableData(TagGPOS)
		if err == nil {
			gposLen = len(data)
			s.gpos, _ = ParseGPOS(data)
		}
	}

	// HarfBuzz equivalent: hb-ot-layout.cc:152-266 (GDEF::is_blocklisted)
	// Blocklist fonts with broken GDEF tables that misclassify base glyphs as marks.
	if s.gdef != nil {
		var gdefLen int
		if data, err := font.TableData(TagGDEF); err == nil {
			gdefLen = len(data)
		}
		if isGDEFBlocklisted(gdefLen, gsubLen, gposLen) {
			s.gdef = nil
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

	// Parse vhea/vmtx (vertical metrics)
	if font.HasTable(TagVhea) {
		if data, err := font.TableData(TagVhea); err == nil {
			s.vhea, _ = ParseVhea(data)
		}
	}
	if s.vhea != nil && font.HasTable(TagVmtx) {
		if data, err := font.TableData(TagVmtx); err == nil {
			s.vmtx, _ = ParseVmtx(data, int(s.vhea.NumberOfVMetrics), font.NumGlyphs())
		}
	}

	// Parse VORG (vertical origin, CFF/CFF2 fonts)
	if font.HasTable(TagVORG) {
		if data, err := font.TableData(TagVORG); err == nil {
			s.vorg, _ = ParseVORG(data)
		}
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

	// Parse gvar (glyph variations, for advance deltas when no HVAR)
	// HarfBuzz: gvar used in _glyf_get_advance_with_var_unscaled() when no HVAR
	if font.HasTable(TagGvar) {
		data, err := font.TableData(TagGvar)
		if err == nil {
			s.gvar, _ = ParseGvar(data)
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
	// HarfBuzz: hb_font_set_variations() sets ALL axes matching the tag, not just the first.
	// This is important for fonts with multiple axes sharing the same tag.
	for _, v := range variations {
		for i := 0; i < axisCount; i++ {
			if axes[i].Tag == v.Tag {
				s.designCoords[i] = clampFloat32(v.Value, axes[i].MinValue, axes[i].MaxValue)
				s.normalizedCoords[i] = s.fvar.NormalizeAxisValue(i, v.Value)
				s.normalizedCoordsI[i] = floatToF2DOT14(s.normalizedCoords[i])
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

	// HarfBuzz: Default features (common_features[], horizontal_features[]) are ALWAYS
	// added first, then user features are appended AFTER. compile() merges duplicates
	// so user features can override defaults (e.g., -calt disables calt but keeps kern).
	// See hb-ot-shape.cc:320-399 (hb_ot_shape_collect_features)
	if len(features) > 0 {
		allFeatures := make([]Feature, 0, len(s.defaultFeatures)+len(features))
		allFeatures = append(allFeatures, s.defaultFeatures...)
		allFeatures = append(allFeatures, features...)
		features = allFeatures
	} else {
		features = s.defaultFeatures
	}

	// Step 1: Guess segment properties (script, direction, language)
	// HarfBuzz equivalent: hb_buffer_guess_segment_properties() in hb-buffer.cc
	buf.GuessSegmentProperties()

	// Step 1.1: Resolve language candidates against font's GSUB table
	// HarfBuzz equivalent: hb_ot_layout_table_select_script() tries multiple language tags
	if len(buf.LanguageCandidates) > 1 && s.gsub != nil {
		best := s.gsub.FindBestLanguage(buf.Script, buf.LanguageCandidates)
		if best != 0 {
			buf.Language = best
		}
	}

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

	// Step 3.5: Ensure native direction
	// HarfBuzz equivalent: hb_ensure_native_direction() in hb-ot-shape.cc:592-648
	// If the buffer direction doesn't match the script's native direction,
	// reverse the buffer and flip the direction. This ensures features designed
	// for RTL scripts work correctly even when direction is overridden to LTR.
	ensureNativeDirection(buf)

	// Step 3.6: Rotate chars (mirror for RTL)
	// HarfBuzz equivalent: hb_ot_rotate_chars() in hb-ot-shape.cc:655-684
	// For RTL text, replace codepoints with their bidi mirrors if the font has
	// a glyph for the mirrored codepoint. Otherwise, set the rtlm feature mask.
	s.rotateChars(buf)

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
		// Hangul shaper with Jamo composition/decomposition
		s.shapeHangul(buf, features)
	case "qaag":
		// Zawgyi (Myanmar visual encoding) shaper
		// HarfBuzz equivalent: _hb_ot_shaper_myanmar_zawgyi in hb-ot-shaper-myanmar.cc
		s.shapeQaag(buf, features)
	default:
		// Default shaping path
		s.shapeDefault(buf, features)
	}

	// Step 3b: Apply space fallback widths for special Unicode spaces
	// HarfBuzz equivalent: _hb_ot_shape_fallback_spaces() in hb-ot-shape-fallback.cc
	s.applySpaceFallback(buf)

	// Step 3c: Deal with variation selectors that have NotFoundVSGlyph
	// HarfBuzz equivalent: hb_ot_deal_with_variation_selectors() in hb-ot-shape.cc:806-825
	// This runs AFTER all positioning to zero the advance of VS fallback glyphs.
	if buf.NotFoundVSGlyph >= 0 {
		for i := range buf.Info {
			if IsVariationSelector(buf.Info[i].Codepoint) &&
				buf.Info[i].GlyphID == GlyphID(buf.NotFoundVSGlyph) {
				buf.Pos[i].XAdvance = 0
				buf.Pos[i].YAdvance = 0
				buf.Pos[i].XOffset = 0
				buf.Pos[i].YOffset = 0
			}
		}
	}

	// Step 4: Handle default ignorables (after all shaping)
	// HarfBuzz: hb-ot-shape.cc:828-851 (hb_ot_hide_default_ignorables)
	s.hideDefaultIgnorables(buf)
}

// ensureNativeDirection ensures the buffer direction matches the script's native
// horizontal direction. If it doesn't (e.g., LTR for Arabic which is natively RTL),
// the buffer is reversed and the direction is changed.
// HarfBuzz equivalent: hb_ensure_native_direction() in hb-ot-shape.cc:592-648
//
// Special case: If the script is natively RTL but the direction is LTR, and the
// buffer contains only numbers (no letters), the direction is kept as LTR.
// This allows digit sequences in Arabic to be shaped in LTR direction.
func ensureNativeDirection(buf *Buffer) {
	direction := buf.Direction
	// Vertical directions are not reordered based on script native direction.
	// HarfBuzz: hb_ensure_native_direction() only handles horizontal directions.
	if direction.IsVertical() {
		return
	}
	// Script tag 0 means Common/Unknown — no native direction to enforce.
	if buf.Script == 0 {
		return
	}
	// Normalize script tag to ISO 15924 uppercase-first format for lookup.
	// Test runner may pass lowercase tags (e.g., 'arab' from --script=arab).
	horizDir := normalizeAndGetHorizontalDirection(buf.Script)

	// Special case: RTL script with LTR direction
	// Check if buffer has only numbers/RI (no letters) - keep LTR for digit sequences
	// HarfBuzz: hb-ot-shape.cc:614-634
	if horizDir == DirectionRTL && direction == DirectionLTR {
		foundNumber := false
		foundLetter := false
		foundRI := false
		for _, info := range buf.Info {
			gc := getGeneralCategory(info.Codepoint)
			if isLetterCategory(gc) {
				foundLetter = true
				break
			} else if gc == GCDecimalNumber {
				foundNumber = true
			} else if isRegionalIndicator(info.Codepoint) {
				foundRI = true
			}
		}
		if (foundNumber || foundRI) && !foundLetter {
			horizDir = DirectionLTR
		}
	}

	// If direction doesn't match native, reverse buffer and flip direction
	if direction.IsHorizontal() && direction != horizDir && horizDir.IsValid() {
		buf.Reverse()
		buf.Direction = directionReverse(direction)
	}
}

// directionReverse returns the opposite direction (LTR↔RTL, TTB↔BTT).
// HarfBuzz: HB_DIRECTION_REVERSE(dir) = (dir ^ 1)
func directionReverse(d Direction) Direction {
	return Direction(int(d) ^ 1)
}

// isLetterCategory returns true if the general category is a letter category.
// HarfBuzz: HB_UNICODE_GENERAL_CATEGORY_IS_LETTER(gc)
func isLetterCategory(gc GeneralCategory) bool {
	switch gc {
	case GCUppercaseLetter, GCLowercaseLetter, GCTitlecaseLetter,
		GCModifierLetter, GCOtherLetter:
		return true
	}
	return false
}

// normalizeAndGetHorizontalDirection normalizes a script tag to ISO 15924
// uppercase-first format and returns its native horizontal direction.
// Handles both uppercase-first ('Arab') and lowercase ('arab') script tags.
// HarfBuzz equivalent: hb_script_get_horizontal_direction() in hb-common.cc
func normalizeAndGetHorizontalDirection(script Tag) Direction {
	// Normalize to ISO 15924 uppercase-first: first char uppercase, rest lowercase
	b0 := byte((script >> 24) & 0xFF)
	b1 := byte((script >> 16) & 0xFF)
	b2 := byte((script >> 8) & 0xFF)
	b3 := byte(script & 0xFF)
	if b0 >= 'a' && b0 <= 'z' {
		b0 -= 'a' - 'A'
	}
	if b1 >= 'A' && b1 <= 'Z' {
		b1 += 'a' - 'A'
	}
	if b2 >= 'A' && b2 <= 'Z' {
		b2 += 'a' - 'A'
	}
	if b3 >= 'A' && b3 <= 'Z' {
		b3 += 'a' - 'A'
	}
	normalized := MakeTag(b0, b1, b2, b3)
	return GetHorizontalDirection(normalized)
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

	// 2. Check if buffer starts with a mark (BOT flag + no pre-context + first char is mark)
	// BOT = Beginning Of Text
	// HarfBuzz: !(buffer->flags & HB_BUFFER_FLAG_BOT) || buffer->context_len[0] || !is_mark
	if buf.Flags&BufferFlagBOT == 0 ||
		len(buf.PreContext) > 0 ||
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
//   - dottedCirclePosition: The position to assign to the inserted dotted circle (-1 if not applicable)
//
// Returns true if any dotted circles were inserted.
func (s *Shaper) insertSyllabicDottedCircles(buf *Buffer, accessor SyllableAccessor,
	brokenSyllableType uint8, dottedCircleCategory uint8, rephaCategory int, dottedCirclePosition int) bool {

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
	// HarfBuzz: hb-ot-shaper-syllabic.cc:55-61
	dottedCircle := GlyphInfo{
		GlyphID:   dottedCircleGlyph,
		Codepoint: 0x25CC,
	}
	dottedCircle.IndicCategory = dottedCircleCategory
	if dottedCirclePosition != -1 {
		dottedCircle.IndicPosition = uint8(dottedCirclePosition)
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
			// HarfBuzz: hb-ot-shaper-syllabic.cc:73-76
			ginfo := dottedCircle
			ginfo.Cluster = buf.Info[buf.Idx].Cluster
			ginfo.Mask = buf.Info[buf.Idx].Mask
			ginfo.Syllable = buf.Info[buf.Idx].Syllable

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
	case DirectionTTB, DirectionBTT:
		// Vertical: apply vert (Vertical Alternates)
		// HarfBuzz: hb-ot-shape.cc:345-347
		gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('v', 'e', 'r', 't'), Value: 1})
	}

	s.applyGSUB(buf, gsubFeatures)
	s.setBaseAdvances(buf)

	// Fallback: add default GPOS features if categorization yielded none
	// (e.g., font has no GPOS table at all). Normally defaults are already
	// in the features list from Shape().
	if len(gposFeatures) == 0 {
		gposFeatures = s.getDefaultGPOSFeatures(buf.Direction)
	}

	// For vertical text, replace 'kern' with 'vkrn'
	// HarfBuzz: hb-ot-shape.cc:127-129
	if buf.Direction.IsVertical() {
		gposFeatures = replaceKernForVertical(gposFeatures)
	}

	// Default shaper uses LATE mode for zero width marks
	// HarfBuzz: HB_OT_SHAPE_ZERO_WIDTH_MARKS_BY_GDEF_LATE in _hb_ot_shaper_default
	s.applyGPOSWithZeroWidthMarks(buf, gposFeatures, ZeroWidthMarksByGDEFLate)
	if !buf.Direction.IsVertical() {
		s.applyKernTableFallback(buf, features) // Fallback if no GPOS kern (horizontal only)
	}

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
	// Pass raw user features to applyArabicFeatures. It builds CompileMap internally
	// to merge defaults with user overrides (e.g., -calt disables calt).
	// HarfBuzz: hb_ot_shape_collect_features() + compile() handles merging
	s.applyArabicFeatures(buf, features)

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
// rotateChars mirrors codepoints for backward directions and substitutes vertical forms.
// HarfBuzz equivalent: hb_ot_rotate_chars() in hb-ot-shape.cc:655-684
//
// HarfBuzz uses HB_DIRECTION_IS_BACKWARD which includes both RTL and BTT.
// For backward directions: apply bidi mirroring.
// For vertical directions: apply vertical forms substitution.
// BTT gets both treatments.
func (s *Shaper) rotateChars(buf *Buffer) {
	// Apply bidi mirroring for backward directions (RTL, BTT)
	// HarfBuzz: HB_DIRECTION_IS_BACKWARD = (dir & 1) != 0
	isBackward := buf.Direction == DirectionRTL || buf.Direction == DirectionBTT
	if isBackward {
		for i := range buf.Info {
			mirrored, ok := bidiMirrorTable[buf.Info[i].Codepoint]
			if !ok {
				continue
			}
			if s.cmap != nil {
				if gid, found := s.cmap.Lookup(mirrored); found && gid != 0 {
					buf.Info[i].Codepoint = mirrored
				}
			}
		}
	}

	// Apply vertical forms for vertical directions (TTB, BTT)
	// HarfBuzz: hb-ot-shape.cc:676-685
	if buf.Direction.IsVertical() {
		for i := range buf.Info {
			if vf, ok := verticalFormsTable[buf.Info[i].Codepoint]; ok {
				if s.cmap != nil {
					if gid, found := s.cmap.Lookup(vf); found && gid != 0 {
						buf.Info[i].Codepoint = vf
					}
				}
			}
		}
	}
}

// verticalFormsTable maps codepoints to their vertical presentation forms.
// CJK Compatibility Forms (FE10-FE19) and CJK Compatibility Forms (FE30-FE4F).
// HarfBuzz: hb-ot-shape.cc:676-685 uses Unicode decomposition mapping.
var verticalFormsTable = map[Codepoint]Codepoint{
	0x2013: 0xFE32, // EN DASH → PRESENTATION FORM FOR VERTICAL EN DASH
	0x2014: 0xFE31, // EM DASH → PRESENTATION FORM FOR VERTICAL EM DASH
	0x2025: 0xFE30, // TWO DOT LEADER → PRESENTATION FORM FOR VERTICAL TWO DOT LEADER (via U+2026)
	0x2026: 0xFE19, // HORIZONTAL ELLIPSIS → PRESENTATION FORM FOR VERTICAL HORIZONTAL ELLIPSIS
	0x3001: 0xFE11, // IDEOGRAPHIC COMMA → PRESENTATION FORM FOR VERTICAL IDEOGRAPHIC COMMA
	0x3002: 0xFE12, // IDEOGRAPHIC FULL STOP → PRESENTATION FORM FOR VERTICAL IDEOGRAPHIC FULL STOP
	0x3008: 0xFE3F, // LEFT ANGLE BRACKET
	0x3009: 0xFE40, // RIGHT ANGLE BRACKET
	0x300A: 0xFE3D, // LEFT DOUBLE ANGLE BRACKET
	0x300B: 0xFE3E, // RIGHT DOUBLE ANGLE BRACKET
	0x300C: 0xFE41, // LEFT CORNER BRACKET
	0x300D: 0xFE42, // RIGHT CORNER BRACKET
	0x300E: 0xFE43, // LEFT WHITE CORNER BRACKET
	0x300F: 0xFE44, // RIGHT WHITE CORNER BRACKET
	0x3010: 0xFE3B, // LEFT BLACK LENTICULAR BRACKET
	0x3011: 0xFE3C, // RIGHT BLACK LENTICULAR BRACKET
	0x3014: 0xFE39, // LEFT TORTOISE SHELL BRACKET
	0x3015: 0xFE3A, // RIGHT TORTOISE SHELL BRACKET
	0x3016: 0xFE17, // LEFT WHITE LENTICULAR BRACKET
	0x3017: 0xFE18, // RIGHT WHITE LENTICULAR BRACKET
	0xFF01: 0xFE15, // FULLWIDTH EXCLAMATION MARK
	0xFF08: 0xFE35, // FULLWIDTH LEFT PARENTHESIS
	0xFF09: 0xFE36, // FULLWIDTH RIGHT PARENTHESIS
	0xFF0C: 0xFE10, // FULLWIDTH COMMA
	0xFF1A: 0xFE13, // FULLWIDTH COLON
	0xFF1B: 0xFE14, // FULLWIDTH SEMICOLON
	0xFF1F: 0xFE16, // FULLWIDTH QUESTION MARK
	0xFF3B: 0xFE47, // FULLWIDTH LEFT SQUARE BRACKET
	0xFF3D: 0xFE48, // FULLWIDTH RIGHT SQUARE BRACKET
	0xFF5B: 0xFE37, // FULLWIDTH LEFT CURLY BRACKET
	0xFF5D: 0xFE38, // FULLWIDTH RIGHT CURLY BRACKET
}

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
		// Value==0 features (e.g., -kern) must be passed through so CompileMap
		// can merge them with defaults. HarfBuzz: compile() handles Value==0
		// by skipping the feature after merging (hb-ot-map.cc:268).

		// Check if feature exists in GSUB
		if s.gsub != nil {
			if featureList, err := s.gsub.ParseFeatureList(); err == nil {
				if f.Value == 0 || featureList.FindFeature(f.Tag) != nil {
					gsub = append(gsub, f)
				}
			}
		}

		// Check if feature exists in GPOS
		if s.gpos != nil {
			if featureList, err := s.gpos.ParseFeatureList(); err == nil {
				if f.Value == 0 || featureList.FindFeature(f.Tag) != nil {
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

// replaceKernForVertical replaces 'kern' features with 'vkrn' for vertical text.
// HarfBuzz: hb-ot-shape.cc:127-129 selects kern_tag based on direction.
func replaceKernForVertical(features []Feature) []Feature {
	tagKern := MakeTag('k', 'e', 'r', 'n')
	tagVkrn := MakeTag('v', 'k', 'r', 'n')
	result := make([]Feature, 0, len(features))
	for _, f := range features {
		if f.Tag == tagKern {
			f.Tag = tagVkrn
		}
		result = append(result, f)
	}
	return result
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
		// HarfBuzz: enable_feature('rand', F_RANDOM, HB_OT_MAP_MAX_VALUE)
		// enable_feature adds F_GLOBAL, so rand is global with value=MAX_VALUE.
		// When alt_index==MAX_VALUE and Random==true, randomize in AlternateSubst.
		{Tag: MakeTag('r', 'a', 'n', 'd'), Value: otMapMaxValue, Random: true},
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
	// HarfBuzz equivalent: handle_variation_selector_cluster() in hb-ot-shape-normalize.cc:203-252
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
			} else if buf.NotFoundVSGlyph >= 0 {
				// VS not found in font but NotFoundVSGlyph is set:
				// Mark VS for later replacement with the specified glyph ID.
				// HarfBuzz: hb-ot-shape-normalize.cc:226-227, hb-ot-shape.cc:806-825
				buf.Info[i].GlyphID = GlyphID(buf.NotFoundVSGlyph)
				// Clear default ignorable flag so GSUB won't skip this glyph.
				// HarfBuzz: _hb_glyph_info_clear_default_ignorable() in normalize.cc:227
				buf.Info[i].GlyphProps &^= GlyphPropsDefaultIgnorable
				buf.Info[i].GlyphProps |= GlyphPropsSubstituted // prevent hiding
			}
			// If no variant found and NotFoundVSGlyph not set, VS stays as separate glyph
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
	case 0x3000: // IDEOGRAPHIC SPACE -> SPACE
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

// setBaseAdvances sets the base advance widths from hmtx (horizontal) or vmtx (vertical).
// For variable fonts, it also applies HVAR deltas.
// HarfBuzz equivalent: hb_ot_get_glyph_h_advances() / hb_ot_get_glyph_v_advances() in hb-ot-font.cc
//
// If hmtx is not available, uses upem/2 as default advance (HarfBuzz behavior).
func (s *Shaper) setBaseAdvances(buf *Buffer) {
	if buf.Direction.IsVertical() {
		s.setBaseAdvancesVertical(buf)
		return
	}

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

	// Check if we need to apply variation deltas
	applyHvar := s.hvar != nil && s.hvar.HasData() && s.normalizedCoordsI != nil
	// gvar fallback: when no HVAR, use gvar phantom points for advance deltas
	// HarfBuzz equivalent: _glyf_get_advance_with_var_unscaled() in OT/glyf/glyf.hh:376-402
	applyGvar := !applyHvar && s.gvar != nil && s.gvar.HasData() && s.glyf != nil && s.normalizedCoordsI != nil && s.hasNonZeroCoords()

	for i := range buf.Info {
		adv := s.hmtx.GetAdvanceWidth(buf.Info[i].GlyphID)

		if applyHvar {
			// Apply HVAR delta if available
			delta := s.hvar.GetAdvanceDelta(buf.Info[i].GlyphID, s.normalizedCoordsI)
			adv = uint16(int32(adv) + int32(math.Floor(delta+0.5)))
		} else if applyGvar {
			// Apply gvar phantom point deltas
			// HarfBuzz: advance = phantom_right.x - phantom_left.x
			adv = s.getAdvanceWithGvar(buf.Info[i].GlyphID, adv)
		}

		buf.Pos[i].XAdvance = int16(adv)

		// Synthetic bold: widen horizontal advance
		if s.xStrength != 0 && !s.emboldenInPlace && buf.Pos[i].XAdvance != 0 {
			buf.Pos[i].XAdvance += s.xStrength
		}
	}
}

// setBaseAdvancesVertical sets the base advance heights for vertical text.
// HarfBuzz equivalent: hb_ot_get_glyph_v_advances() in hb-ot-font.cc
func (s *Shaper) setBaseAdvancesVertical(buf *Buffer) {
	if s.vmtx != nil {
		// Use vmtx advance heights
		// gvar fallback for vertical: use phantom points TOP/BOTTOM
		applyGvar := s.gvar != nil && s.gvar.HasData() && s.glyf != nil &&
			s.normalizedCoordsI != nil && s.hasNonZeroCoords()

		for i := range buf.Info {
			adv := s.vmtx.GetAdvanceHeight(buf.Info[i].GlyphID)

			if applyGvar {
				adv = s.getVAdvanceWithGvar(buf.Info[i].GlyphID, adv)
			}

			buf.Pos[i].YAdvance = -int16(adv)
			// Synthetic bold: make vertical advance more negative (larger)
			if s.yStrength != 0 && !s.emboldenInPlace {
				buf.Pos[i].YAdvance -= s.yStrength
			}
			buf.Pos[i].XAdvance = 0
		}
	} else {
		// Fallback: use ascender - descender as advance height
		// HarfBuzz: hb-ot-hmtx-table.hh default for vertical = ascender - descender
		// With synthetic bold, HarfBuzz adds yStrength to ascender in font_h_extents,
		// then the embolden wrapper adds yStrength again — the effects cancel out,
		// so the fallback advance is unchanged.
		defaultAdvance := int16(s.face.Ascender() - s.face.Descender())
		for i := range buf.Info {
			buf.Pos[i].YAdvance = -defaultAdvance
			buf.Pos[i].XAdvance = 0
		}
	}
}

// getVAdvanceWithGvar computes the advance height using gvar phantom point deltas.
// HarfBuzz equivalent: _glyf_get_advance_with_var_unscaled() for vertical
// Phantom points TOP (idx+2) and BOTTOM (idx+3) give vertical advance.
func (s *Shaper) getVAdvanceWithGvar(gid GlyphID, baseAdvance uint16) uint16 {
	numContourPoints := s.glyf.GetContourPointCount(gid)
	numTotalPoints := numContourPoints + 4

	deltas := s.gvar.GetGlyphDeltas(gid, s.normalizedCoordsI, numTotalPoints)
	if deltas == nil {
		return baseAdvance
	}

	phantomTop := numContourPoints + 2
	phantomBottom := numContourPoints + 3

	if phantomBottom >= len(deltas.YDeltas) {
		return baseAdvance
	}

	// advance_height = phantom_top.y - phantom_bottom.y
	advanceDelta := deltas.YDeltas[phantomTop] - deltas.YDeltas[phantomBottom]
	result := int32(baseAdvance) + int32(math.Floor(advanceDelta+0.5))
	if result < 0 {
		result = 0
	}
	return uint16(result)
}

// hasNonZeroCoords returns true if any normalized coordinate is non-zero.
// HarfBuzz equivalent: font->has_nonzero_coords check
func (s *Shaper) hasNonZeroCoords() bool {
	for _, c := range s.normalizedCoordsI {
		if c != 0 {
			return true
		}
	}
	return false
}

// getAdvanceWithGvar computes the advance width using gvar phantom point deltas.
// HarfBuzz equivalent: _glyf_get_advance_with_var_unscaled() in OT/glyf/glyf.hh:376-402
// The phantom points are appended after the contour points:
//   - PHANTOM_LEFT (index numPoints+0): x = xMin - LSB
//   - PHANTOM_RIGHT (index numPoints+1): x = advance + (xMin - LSB)
//   - PHANTOM_TOP (index numPoints+2): y = yMax + TSB
//   - PHANTOM_BOTTOM (index numPoints+3): y = yMax + TSB - vAdv
//
// Advance = PHANTOM_RIGHT.x - PHANTOM_LEFT.x after gvar deltas.
func (s *Shaper) getAdvanceWithGvar(gid GlyphID, baseAdvance uint16) uint16 {
	// Count contour points in the glyph
	numContourPoints := s.glyf.GetContourPointCount(gid)
	numTotalPoints := numContourPoints + 4 // +4 phantom points

	// Get gvar deltas for all points including phantoms
	deltas := s.gvar.GetGlyphDeltas(gid, s.normalizedCoordsI, numTotalPoints)
	if deltas == nil {
		return baseAdvance
	}

	// Phantom point indices
	phantomLeft := numContourPoints
	phantomRight := numContourPoints + 1

	if phantomRight >= len(deltas.XDeltas) {
		return baseAdvance
	}

	// HarfBuzz: advance = phantom_right.x - phantom_left.x
	// phantom_left.x starts at h_delta = xMin - LSB
	// phantom_right.x starts at advance + h_delta
	// After gvar: advance = (advance + h_delta + delta_right) - (h_delta + delta_left)
	//           = advance + delta_right - delta_left
	advanceDelta := deltas.XDeltas[phantomRight] - deltas.XDeltas[phantomLeft]
	result := int32(baseAdvance) + int32(math.Floor(advanceDelta+0.5))
	if result < 0 {
		result = 0
	}
	return uint16(result)
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

	// Apply 'locl' (Localized Forms) feature - only via LangSys, no global fallback
	// HarfBuzz: locl is in common_features[] and applied via script/language LangSys
	s.gsub.ApplyFeatureToBufferLangSysOnly(TagLocl, buf, s.gdef, MaskGlobal, s.font, variationsIndex)

	// Apply automatic fractions before other features
	s.applyAutomaticFractions(buf)

	// First, apply required features from the script/language system
	s.applyRequiredGSUBFeaturesToBufferWithVariations(buf, variationsIndex)

	// Apply each feature with HarfBuzz-style merging.
	// Features with the same tag are merged: later global overrides earlier,
	// and per-cluster ranges get dedicated mask bits.
	// HarfBuzz: compile() merges duplicates, then lookups are collected once per tag.
	nextBit := uint(8) // Start after positional mask bits (1-7)
	for _, tag := range uniqueFeatureTags(features) {
		nextBit = applyFeatureWithMergedMask(tag, features, nextBit, buf, s.gsub, s.gdef, s.font)
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
//  1. GPOS lookups
//  2. zero_mark_widths_by_gdef (LATE mode)
//  3. hb_ot_zero_width_default_ignorables
//  4. position_finish_offsets (PropagateAttachmentOffsets)
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

	// Propagate attachment offsets (cursive → marks)
	// This must be done after all GPOS lookups have set up the attachment chains
	// AND after mark advances have been zeroed!
	// HarfBuzz: GPOS::position_finish_offsets() in hb-ot-shape.cc:1086
	PropagateAttachmentOffsets(buf.Pos, buf.Direction)

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

	// For vertical text: subtract vertical origins
	// HarfBuzz: hb-ot-shape.cc:1092-1094
	if buf.Direction.IsVertical() {
		s.subtractGlyphVOrigins(buf)
	}
}

// zeroWidthDefaultIgnorables zeros advance widths and offsets of default ignorables.
// HarfBuzz equivalent: hb_ot_zero_width_default_ignorables() in hb-ot-shape.cc:783-803
//
// This is called AFTER GPOS lookups, but BEFORE PropagateAttachmentOffsets.
// The order is critical: hb-ot-shape.cc:1083-1086:
//  1. position_finish_advances
//  2. hb_ot_zero_width_default_ignorables  <-- HERE
//  3. position_finish_offsets
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
	s.zeroMarkWidthsByGDEFAdjust(buf, false)
}

// zeroMarkWidthsByGDEFEarly zeros mark advances and optionally adjusts offsets for EARLY mode.
// HarfBuzz equivalent: zero_mark_widths_by_gdef(buffer, adjust_offsets_when_zeroing)
//
// HarfBuzz logic (hb-ot-shape.cc:198-204, 1044-1045):
//
//	adjust_mark_positioning_when_zeroing = !apply_gpos && !apply_kerx && (!apply_kern || !cross_kerning)
//	adjust_offsets_when_zeroing = adjust_mark_positioning_when_zeroing && HB_DIRECTION_IS_FORWARD(direction)
//
// So adjust_offsets is only true for FALLBACK mark positioning (no GPOS).
// When GPOS is present, we just zero the mark advance without transferring to offset.
func (s *Shaper) zeroMarkWidthsByGDEFEarly(buf *Buffer) {
	// Only adjust offsets when there's no GPOS to handle mark positioning
	// HarfBuzz: adjust_mark_positioning_when_zeroing = !apply_gpos && ...
	adjustMarkPositioning := s.gpos == nil
	adjustOffsets := adjustMarkPositioning && (buf.Direction == DirectionLTR || buf.Direction == DirectionTTB)
	s.zeroMarkWidthsByGDEFAdjust(buf, adjustOffsets)
}

// zeroMarkWidthsByGDEFAdjust is the core implementation shared by EARLY and LATE modes.
func (s *Shaper) zeroMarkWidthsByGDEFAdjust(buf *Buffer, adjustOffsets bool) {
	for i := range buf.Pos {
		if buf.Info[i].GlyphClass == GlyphClassMark {
			if adjustOffsets {
				// HarfBuzz: adjust_mark_offsets()
				buf.Pos[i].XOffset -= buf.Pos[i].XAdvance
				buf.Pos[i].YOffset -= buf.Pos[i].YAdvance
			}
			buf.Pos[i].XAdvance = 0
			buf.Pos[i].YAdvance = 0
		}
	}
}

// hasGlyphHOrigins returns true if the font has horizontal glyph origins.
// HarfBuzz equivalent: font->has_glyph_h_origin_func() in hb-font.hh
//
// For horizontal text, h_origins are always (0, 0) so this returns false.
// For vertical text, we need to transform from v_origin to h_origin space for GPOS.
func (s *Shaper) hasGlyphHOrigins() bool {
	// HarfBuzz: h_origin is always (0,0) for horizontal fonts.
	// The h_origin func is never overridden - HarfBuzz always returns false here.
	// The actual vertical origin handling happens via v_origins.
	return false
}

// hasGlyphVOrigins returns true if the font has vertical glyph origins.
// HarfBuzz equivalent: font->has_glyph_v_origin_func()
func (s *Shaper) hasGlyphVOrigins() bool {
	return true // HarfBuzz always returns true (has fallback)
}

// getGlyphExtentsWithVar returns glyph extents with gvar variations applied.
// For variable TrueType fonts with gvar, this recomputes the bounding box from
// varied contour points. For non-variable fonts or CFF fonts, falls back to
// the static extents from the glyf table header.
// HarfBuzz equivalent: glyf_impl.get_extents_with_var_unscaled()
func (s *Shaper) getGlyphExtentsWithVar(glyph GlyphID) (GlyphExtents, bool) {
	if s.glyf == nil {
		return GlyphExtents{}, false
	}

	// If no gvar variations active, use static extents
	if s.gvar == nil || !s.gvar.HasData() || s.normalizedCoordsI == nil || !s.hasNonZeroCoords() {
		return s.glyf.GetGlyphExtents(glyph)
	}

	// Get static extents first (as fallback)
	staticExt, ok := s.glyf.GetGlyphExtents(glyph)
	if !ok {
		return GlyphExtents{}, false
	}

	// Parse the glyph's contour points
	glyphBytes := s.glyf.GetGlyphBytes(glyph)
	if glyphBytes == nil || len(glyphBytes) < 10 {
		return staticExt, true
	}

	numberOfContours := int16(binary.BigEndian.Uint16(glyphBytes[0:]))
	if numberOfContours <= 0 {
		// Composite or empty - use static extents for now
		return staticExt, true
	}

	points, _, err := ParseSimpleGlyph(glyphBytes)
	if err != nil || len(points) == 0 {
		return staticExt, true
	}

	// Build GlyphPoint array for IUP interpolation
	origCoords := make([]GlyphPoint, len(points))
	for i, p := range points {
		origCoords[i] = GlyphPoint{X: p.X, Y: p.Y}
	}

	// Get gvar deltas (numPoints = contour points + 4 phantom points)
	numTotalPoints := len(points) + 4
	deltas := s.gvar.GetGlyphDeltasWithCoords(glyph, s.normalizedCoordsI, numTotalPoints, origCoords)
	if deltas == nil {
		return staticExt, true
	}

	// Apply deltas to contour points and recompute bounding box.
	// HarfBuzz accumulates bounds in float, then applies roundf() at the end.
	// See points_aggregator_t::contour_bounds_t in OT/glyf/glyf.hh
	var fMinX, fMinY, fMaxX, fMaxY float64
	first := true
	for i, p := range points {
		fx := float64(p.X) + deltas.XDeltas[i]
		fy := float64(p.Y) + deltas.YDeltas[i]
		if first {
			fMinX, fMaxX = fx, fx
			fMinY, fMaxY = fy, fy
			first = false
		} else {
			if fx < fMinX {
				fMinX = fx
			}
			if fx > fMaxX {
				fMaxX = fx
			}
			if fy < fMinY {
				fMinY = fy
			}
			if fy > fMaxY {
				fMaxY = fy
			}
		}
	}

	// HarfBuzz: extents->x_bearing = roundf(min_x); width = roundf(max_x - x_bearing);
	//           extents->y_bearing = roundf(max_y); height = roundf(min_y - y_bearing);
	xBearing := int16(math.Round(fMinX))
	yBearing := int16(math.Round(fMaxY))
	width := int16(math.Round(fMaxX - float64(xBearing)))
	height := int16(math.Round(fMinY - float64(yBearing)))

	return GlyphExtents{
		XBearing: xBearing,
		YBearing: yBearing,
		Width:    width,
		Height:   height,
	}, true
}

// GetGlyphVOrigin returns the vertical origin (x, y) for a glyph in font units.
// Exported for use by test runners that need to replicate HarfBuzz's scaling order.
func (s *Shaper) GetGlyphVOrigin(glyph GlyphID) (x, y int16) {
	return s.getGlyphVOrigin(glyph)
}

// GetGlyphHAdvanceVar returns the horizontal advance for a glyph,
// including HVAR/gvar variation deltas if active. Exported for font-size
// scaling in test runners that need to match HarfBuzz's scaling order.
func (s *Shaper) GetGlyphHAdvanceVar(glyph GlyphID) uint16 {
	if s.hmtx == nil {
		return 0
	}
	adv := s.hmtx.GetAdvanceWidth(glyph)
	applyHvar := s.hvar != nil && s.hvar.HasData() && s.normalizedCoordsI != nil
	applyGvar := !applyHvar && s.gvar != nil && s.gvar.HasData() && s.glyf != nil &&
		s.normalizedCoordsI != nil && s.hasNonZeroCoords()
	if applyHvar {
		delta := s.hvar.GetAdvanceDelta(glyph, s.normalizedCoordsI)
		adv = uint16(int32(adv) + int32(math.Floor(delta+0.5)))
	} else if applyGvar {
		adv = s.getAdvanceWithGvar(glyph, adv)
	}
	return adv
}

// getGlyphVOrigin returns the vertical origin (x, y) for a glyph.
// HarfBuzz equivalent: hb_ot_get_glyph_v_origins() in hb-ot-font.cc
//
// The vertical origin is the point from which a glyph is positioned in vertical text.
// X origin = half the horizontal advance (centers the glyph)
// Y origin priority:
//  1. VORG table (CFF/CFF2 fonts)
//  2. vmtx + glyf: top phantom point from vmtx,glyf[,gvar]
//  3. glyf extents: y_bearing + (font_advance + height) / 2
//  4. Fallback: face.Ascender()
func (s *Shaper) getGlyphVOrigin(glyph GlyphID) (x, y int16) {
	// X origin: horizontal advance / 2 (center the glyph horizontally)
	// For variable fonts, use the varied advance width.
	// With synthetic bold (!inPlace): use bold-adjusted advance → (adv+xStrength)/2
	var xOrigin int16
	if s.hmtx != nil {
		hAdv := s.getGlyphHAdvanceWithBold(glyph)
		xOrigin = hAdv / 2
	}

	// 1. VORG table (CFF/CFF2 fonts)
	if s.vorg != nil {
		yOrigin := s.vorg.GetVertOriginY(glyph)
		if !s.emboldenInPlace {
			xOrigin += s.xStrength
			yOrigin += s.yStrength
		}
		return xOrigin, yOrigin
	}

	// 2. vmtx + glyf: use top phantom point
	// HarfBuzz: glyf.get_v_origin_with_var_unscaled() uses phantom_top.y
	// phantom_top.y = yMax + TSB (base), then gvar delta is applied to both together
	if s.vmtx != nil && s.glyf != nil {
		if ext, ok := s.glyf.GetGlyphExtents(glyph); ok {
			tsb := s.vmtx.GetTsb(glyph)
			yOrigin := int(ext.YBearing) + int(tsb)

			// Apply gvar delta to phantom_top.y for variable fonts
			if s.gvar != nil && s.gvar.HasData() && s.normalizedCoordsI != nil && s.hasNonZeroCoords() {
				numContourPoints := s.glyf.GetContourPointCount(glyph)
				numTotalPoints := numContourPoints + 4
				deltas := s.gvar.GetGlyphDeltas(glyph, s.normalizedCoordsI, numTotalPoints)
				if deltas != nil {
					phantomTop := numContourPoints + 2
					if phantomTop < len(deltas.YDeltas) {
						yOrigin += int(math.Round(deltas.YDeltas[phantomTop]))
					}
				}
			}

			yResult := int16(yOrigin)
			if !s.emboldenInPlace {
				xOrigin += s.xStrength
				yResult += s.yStrength
			}
			return xOrigin, yResult
		}
		// Empty glyph with vmtx: fallback to ascender
		yResult := s.face.Ascender()
		if !s.emboldenInPlace {
			xOrigin += s.xStrength
			yResult += s.yStrength
		}
		return xOrigin, yResult
	}

	// 3. Glyph extents fallback (no vmtx)
	// HarfBuzz: origin = extents.y_bearing + ((font_advance - (-extents.height)) >> 1)
	// where font_advance = ascender - descender
	// With bold: HarfBuzz adds yStrength to ascender via font_h_extents
	fontAdvance := int(s.face.Ascender()) + int(s.yStrength) - int(s.face.Descender())
	if s.glyf != nil {
		ext, ok := s.getGlyphExtentsWithVar(glyph)
		if ok && (ext.YBearing != 0 || ext.Height != 0) {
			// Non-empty glyph: center vertically
			// With bold: extents get yStrength added to YBearing, plus direct yStrength → 2*yStrength
			yBearing := int(ext.YBearing)
			height := int(-ext.Height)
			if s.yStrength != 0 {
				yBearing += int(s.yStrength)
				height += int(s.yStrength)
			}
			yOrigin := yBearing + ((fontAdvance - height) >> 1)
			if !s.emboldenInPlace {
				xOrigin += s.xStrength
				yOrigin += int(s.yStrength)
			}
			return xOrigin, int16(yOrigin)
		}
		// Empty glyph (e.g., space): HarfBuzz returns {0,0,0,0} and true
		// origin = 0 + ((font_advance - 0) >> 1) = font_advance / 2
		yResult := int16(fontAdvance >> 1)
		if !s.emboldenInPlace {
			xOrigin += s.xStrength
			yResult += s.yStrength
		}
		return xOrigin, yResult
	}

	// 4. Fallback: ascender
	yResult := s.face.Ascender()
	if !s.emboldenInPlace {
		xOrigin += s.xStrength
		yResult += s.yStrength
	}
	return xOrigin, yResult
}

// addGlyphHOrigins adds horizontal glyph origins to buffer positions.
// HarfBuzz equivalent: font->add_glyph_h_origin_with_fallback() in hb-font.hh
// For horizontal text this is a no-op (h_origin is always 0,0).
func (s *Shaper) addGlyphHOrigins(buf *Buffer) {
	// h_origins are always (0, 0) for horizontal fonts. No-op.
}

// subtractGlyphHOrigins subtracts horizontal glyph origins from buffer positions.
// HarfBuzz equivalent: font->subtract_glyph_h_origin_with_fallback()
// For horizontal text this is a no-op.
func (s *Shaper) subtractGlyphHOrigins(buf *Buffer) {
	// h_origins are always (0, 0) for horizontal fonts. No-op.
}

// addGlyphVOrigins adds vertical glyph origins to buffer positions.
// HarfBuzz equivalent: font->add_glyph_v_origin() in hb-font.hh
// Transforms from horizontal origin space to vertical origin space.
func (s *Shaper) addGlyphVOrigins(buf *Buffer) {
	for i := range buf.Info {
		x, y := s.getGlyphVOrigin(buf.Info[i].GlyphID)
		buf.Pos[i].XOffset += x
		buf.Pos[i].YOffset += y
	}
}

// subtractGlyphVOrigins subtracts vertical glyph origins from buffer positions.
// HarfBuzz equivalent: font->subtract_glyph_v_origin() in hb-font.hh
// Called after GPOS to transform back from vertical origin space.
func (s *Shaper) subtractGlyphVOrigins(buf *Buffer) {
	for i := range buf.Info {
		x, y := s.getGlyphVOrigin(buf.Info[i].GlyphID)
		buf.Pos[i].XOffset -= x
		buf.Pos[i].YOffset -= y
	}
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

// spaceType represents the type of Unicode space character for fallback width calculation.
// HarfBuzz equivalent: hb_unicode_funcs_t::space_t in hb-unicode.hh
type spaceType int

const (
	spaceNotSpace    spaceType = 0
	spaceEM          spaceType = 1  // full em
	spaceEM2         spaceType = 2  // 1/2 em
	spaceEM3         spaceType = 3  // 1/3 em
	spaceEM4         spaceType = 4  // 1/4 em
	spaceEM5         spaceType = 5  // 1/5 em
	spaceEM6         spaceType = 6  // 1/6 em
	spaceEM16        spaceType = 16 // 1/16 em
	space4EM18       spaceType = 17 // 4/18 em
	spaceRegular     spaceType = 18
	spaceFigure      spaceType = 19
	spacePunctuation spaceType = 20
	spaceNarrow      spaceType = 21
)

// getSpaceType returns the space fallback type for a Unicode codepoint.
// HarfBuzz equivalent: hb_unicode_funcs_t::space_fallback_type() in hb-unicode.hh
func getSpaceType(cp Codepoint) spaceType {
	switch cp {
	case 0x0020, 0x00A0:
		return spaceRegular
	case 0x2000: // EN QUAD
		return spaceEM2
	case 0x2001: // EM QUAD
		return spaceEM
	case 0x2002: // EN SPACE
		return spaceEM2
	case 0x2003: // EM SPACE
		return spaceEM
	case 0x2004: // THREE-PER-EM SPACE
		return spaceEM3
	case 0x2005: // FOUR-PER-EM SPACE
		return spaceEM4
	case 0x2006: // SIX-PER-EM SPACE
		return spaceEM6
	case 0x2007: // FIGURE SPACE
		return spaceFigure
	case 0x2008: // PUNCTUATION SPACE
		return spacePunctuation
	case 0x2009: // THIN SPACE
		return spaceEM5
	case 0x200A: // HAIR SPACE
		return spaceEM16
	case 0x202F: // NARROW NO-BREAK SPACE
		return spaceNarrow
	case 0x205F: // MEDIUM MATHEMATICAL SPACE
		return space4EM18
	case 0x3000: // IDEOGRAPHIC SPACE
		return spaceEM
	default:
		return spaceNotSpace
	}
}

// applySpaceFallback adjusts advance widths for special Unicode space characters.
// HarfBuzz equivalent: _hb_ot_shape_fallback_spaces() in hb-ot-shape-fallback.cc
func (s *Shaper) applySpaceFallback(buf *Buffer) {
	upem := int(s.face.Upem())
	horizontal := buf.Direction.IsHorizontal()

	for i := range buf.Info {
		st := getSpaceType(buf.Info[i].Codepoint)
		if st == spaceNotSpace || st == spaceRegular {
			continue
		}

		// HarfBuzz only applies space fallback when the font does NOT have a
		// dedicated glyph for the space character (the fallback type is set during
		// normalization only when the glyph is replaced by U+0020 SPACE).
		// Skip fallback if the font provides a glyph for this codepoint.
		if glyph, ok := s.cmap.Lookup(buf.Info[i].Codepoint); ok && glyph != 0 {
			continue
		}

		switch st {
		case spaceEM, spaceEM2, spaceEM3, spaceEM4, spaceEM5, spaceEM6, spaceEM16:
			// Width = upem / space_type (with rounding)
			// HarfBuzz: (font->x_scale + ((int) space_type)/2) / (int) space_type
			divisor := int(st)
			if horizontal {
				buf.Pos[i].XAdvance = int16((upem + divisor/2) / divisor)
			} else {
				buf.Pos[i].YAdvance = -int16((upem + divisor/2) / divisor)
			}

		case space4EM18:
			// 4/18 of em
			// HarfBuzz: (int64_t) +font->x_scale * 4 / 18
			if horizontal {
				buf.Pos[i].XAdvance = int16(int64(upem) * 4 / 18)
			} else {
				buf.Pos[i].YAdvance = -int16(int64(upem) * 4 / 18)
			}

		case spaceFigure:
			// Width of digit '0'-'9'
			for u := rune('0'); u <= '9'; u++ {
				if glyph, ok := s.cmap.Lookup(Codepoint(u)); ok {
					adv := s.getGlyphHAdvance(glyph)
					if horizontal {
						buf.Pos[i].XAdvance = int16(adv)
					} else {
						buf.Pos[i].YAdvance = -int16(adv)
					}
					break
				}
			}

		case spacePunctuation:
			// Width of '.' or ','
			if glyph, ok := s.cmap.Lookup(Codepoint('.')); ok {
				adv := s.getGlyphHAdvance(glyph)
				if horizontal {
					buf.Pos[i].XAdvance = int16(adv)
				} else {
					buf.Pos[i].YAdvance = -int16(adv)
				}
			} else if glyph, ok := s.cmap.Lookup(Codepoint(',')); ok {
				adv := s.getGlyphHAdvance(glyph)
				if horizontal {
					buf.Pos[i].XAdvance = int16(adv)
				} else {
					buf.Pos[i].YAdvance = -int16(adv)
				}
			}

		case spaceNarrow:
			// Half the current advance
			// HarfBuzz: pos[i].x_advance /= 2
			if horizontal {
				buf.Pos[i].XAdvance /= 2
			} else {
				buf.Pos[i].YAdvance /= 2
			}
		}
	}
}
