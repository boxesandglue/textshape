package ot

// OT Apply Context - HarfBuzz-style lookup application context
//
// HarfBuzz equivalent: hb_ot_apply_context_t in hb-ot-layout-gsubgpos.hh:678-795
//
// This unified context handles both GSUB and GPOS lookups with proper
// ZWNJ/ZWJ handling, syllable tracking, and mask-based filtering.

// TableType indicates which OpenType table we're processing.
// HarfBuzz equivalent: table_index in hb_ot_apply_context_t
type TableType int

const (
	TableGSUB TableType = 0 // GSUB table
	TableGPOS TableType = 1 // GPOS table
)

// OTApplyContext provides context for applying OpenType lookups.
// HarfBuzz equivalent: hb_ot_apply_context_t in hb-ot-layout-gsubgpos.hh:678-746
//
// This struct uses HarfBuzz field names but also provides compatibility
// aliases for the legacy GSUBContext/GPOSContext field names.
type OTApplyContext struct {
	// Core components
	// HarfBuzz: hb_buffer_t *buffer, hb_font_t *font
	Buffer    *Buffer
	Font      *Font
	GDEF      *GDEF
	TableType TableType // 0=GSUB, 1=GPOS

	// Direction for RTL handling
	// HarfBuzz: hb_direction_t direction
	Direction Direction

	// Lookup properties
	// HarfBuzz: hb_mask_t lookup_mask, unsigned lookup_props
	//
	// FeatureMask is the mask for the current feature.
	// HarfBuzz name: lookup_mask
	// Legacy name: FeatureMask (from GSUBContext/GPOSContext)
	// When 0, mask filtering is disabled (applies to all glyphs).
	FeatureMask uint32

	// LookupFlag contains the lookup flags from the lookup table.
	// HarfBuzz name: lookup_props
	// Legacy name: LookupFlag (from GSUBContext/GPOSContext)
	LookupFlag uint16

	// LookupIndex is the current lookup index.
	// HarfBuzz: lookup_index
	LookupIndex int

	// Mark filtering
	// HarfBuzz: Uses lookup_props with MarkFilteringSet flag
	MarkFilteringSet int // Mark filtering set index (-1 if not used)

	// ZWNJ/ZWJ handling
	// HarfBuzz: bool auto_zwnj, bool auto_zwj in hb_ot_apply_context_t:735-736
	AutoZWNJ bool // Automatically handle ZWNJ (default: true)
	AutoZWJ  bool // Automatically handle ZWJ (default: true)

	// Syllable tracking
	// HarfBuzz: bool per_syllable, unsigned new_syllables in hb_ot_apply_context_t:737-739
	PerSyllable  bool  // Apply lookup per-syllable (GSUB only)
	NewSyllables int   // New syllable value for substituted glyphs (-1 = don't change)
	MatchSyllable uint8 // Reference syllable for per-syllable matching (0 = no constraint)
	// HarfBuzz equivalent: matcher_t::syllable set via reset(start_index_)

	// Per-syllable range constraints
	// When PerSyllable is true, these define the valid range for context matching
	RangeStart int // Start of current syllable range (inclusive)
	RangeEnd   int // End of current syllable range (exclusive)

	// Random feature support
	// HarfBuzz: bool random in hb_ot_apply_context_t:738
	Random bool

	// Recursion control
	// HarfBuzz: unsigned nesting_level_left in hb_ot_apply_context_t:732
	NestingLevel int // Current nesting level for nested lookups

	// RecurseFunc is the callback for recursive lookup application.
	// HarfBuzz equivalent: recurse_func_t recurse_func in hb_ot_apply_context_t:733
	// This is set by the caller (GPOS/GSUB) to enable nested lookup calls.
	RecurseFunc func(ctx *OTApplyContext, lookupIndex int) bool

	// Match positions for context/chaining lookups
	// HarfBuzz: hb_vector_t<uint32_t> match_positions
	MatchPositions []int

	// GPOS-specific: last base glyph tracking
	// HarfBuzz: signed last_base, unsigned last_base_until in hb_ot_apply_context_t:741-742
	LastBase      int // Index of last base glyph (-1 if none)
	LastBaseUntil int // Valid until this index

	// Has glyph classes from GDEF
	// HarfBuzz: bool has_glyph_classes
	HasGlyphClasses bool
}

// NewOTApplyContext creates a new apply context.
// HarfBuzz equivalent: hb_ot_apply_context_t constructor in hb-ot-layout-gsubgpos.hh:747-776
func NewOTApplyContext(tableType TableType, buf *Buffer, font *Font, gdef *GDEF) *OTApplyContext {
	ctx := &OTApplyContext{
		Buffer:           buf,
		Font:             font,
		GDEF:             gdef,
		TableType:        tableType,
		Direction:        buf.Direction,
		FeatureMask:      1, // Default mask (HarfBuzz: lookup_mask)
		LookupFlag:       0, // HarfBuzz: lookup_props
		LookupIndex:      -1,
		MarkFilteringSet: -1,
		AutoZWNJ:         true,
		AutoZWJ:          true,
		PerSyllable:      false,
		NewSyllables:     -1,
		Random:           false,
		NestingLevel:     HBMaxNestingLevel,
		LastBase:         -1,
		LastBaseUntil:    0,
	}

	if gdef != nil {
		ctx.HasGlyphClasses = gdef.HasGlyphClasses()
	}

	return ctx
}

// HBMaxNestingLevel is the maximum nesting level for recursive lookups.
// HarfBuzz: HB_MAX_NESTING_LEVEL = 64
const HBMaxNestingLevel = 64

// Recurse applies a nested lookup.
// HarfBuzz equivalent: recurse() in hb_ot_apply_context_t (hb-ot-layout-gsubgpos.hh:704-724)
//
// This method:
// - Checks if RecurseFunc is set
// - Checks nesting level to prevent infinite recursion
// - Decrements nesting level before calling, restores after
// - Returns false if recursion is not possible or lookup fails
func (ctx *OTApplyContext) Recurse(lookupIndex int) bool {
	if ctx.RecurseFunc == nil {
		return false
	}

	// Check nesting level to prevent infinite recursion
	// HarfBuzz: if (unlikely (nesting_level_left == 0)) { buffer->successful = false; return default_return_value (); }
	if ctx.NestingLevel == 0 {
		return false
	}

	// Decrement nesting level, call recurse_func, restore level
	// HarfBuzz: nesting_level_left--; bool ret = recurse_func(this, sub_lookup_index); nesting_level_left++;
	ctx.NestingLevel--
	ret := ctx.RecurseFunc(ctx, lookupIndex)
	ctx.NestingLevel++

	return ret
}

// SetLookupMask sets the lookup mask (feature mask).
// HarfBuzz equivalent: set_lookup_mask() in hb-ot-layout-gsubgpos.hh:784
func (ctx *OTApplyContext) SetLookupMask(mask uint32) {
	ctx.FeatureMask = mask
	ctx.LastBase = -1
	ctx.LastBaseUntil = 0
}

// SetAutoZWJ sets the auto-ZWJ flag.
// HarfBuzz equivalent: set_auto_zwj() in hb-ot-layout-gsubgpos.hh:785
func (ctx *OTApplyContext) SetAutoZWJ(autoZWJ bool) {
	ctx.AutoZWJ = autoZWJ
}

// SetAutoZWNJ sets the auto-ZWNJ flag.
// HarfBuzz equivalent: set_auto_zwnj() in hb-ot-layout-gsubgpos.hh:786
func (ctx *OTApplyContext) SetAutoZWNJ(autoZWNJ bool) {
	ctx.AutoZWNJ = autoZWNJ
}

// SetPerSyllable sets the per-syllable flag.
// HarfBuzz equivalent: set_per_syllable() in hb-ot-layout-gsubgpos.hh:787
func (ctx *OTApplyContext) SetPerSyllable(perSyllable bool) {
	ctx.PerSyllable = perSyllable
}

// SetRandom sets the random flag for random features.
// HarfBuzz equivalent: set_random() in hb-ot-layout-gsubgpos.hh:788
func (ctx *OTApplyContext) SetRandom(random bool) {
	ctx.Random = random
}

// SetLookupProps sets the lookup properties (flags).
// HarfBuzz equivalent: set_lookup_props() in hb-ot-layout-gsubgpos.hh:791-795
func (ctx *OTApplyContext) SetLookupProps(props uint16) {
	ctx.LookupFlag = props
	// Extract mark filtering set if used
	if props&LookupFlagUseMarkFilteringSet != 0 {
		// Mark filtering set index is stored separately in the lookup
		// and should be set via SetMarkFilteringSet
	}
}

// SetMarkFilteringSet sets the mark filtering set index.
// Validates the index and disables the flag if invalid (like HarfBuzz sanitize_lookup_props).
// HarfBuzz equivalent: sanitize_lookup_props() in GDEF.hh:1017-1028
func (ctx *OTApplyContext) SetMarkFilteringSet(set int) {
	// Validate mark filtering set index
	// HarfBuzz: if index >= mark_glyph_sets.length, unset the UseMarkFilteringSet flag
	if ctx.LookupFlag&LookupFlagUseMarkFilteringSet != 0 {
		if ctx.GDEF != nil {
			maxSets := ctx.GDEF.MarkGlyphSetCount()
			if set < 0 || set >= maxSets {
				// Invalid mark filtering set index; unset the flag
				// Use &^ (AND NOT) operator to clear the bit
				ctx.LookupFlag &^= LookupFlagUseMarkFilteringSet
				ctx.MarkFilteringSet = -1
				return
			}
		}
	}
	ctx.MarkFilteringSet = set
}

// --- Matching Logic ---

// MayMatchResult represents the result of may_match().
// HarfBuzz equivalent: may_match_t in matcher_t
type MayMatchResult int

const (
	MatchNo    MayMatchResult = iota // Definitely no match
	MatchYes                         // Definitely a match
	MatchMaybe                       // Maybe a match (needs further check)
)

// MaySkipResult represents the result of may_skip().
// HarfBuzz equivalent: may_skip_t in matcher_t
type MaySkipResult int

const (
	SkipNo    MaySkipResult = iota // Don't skip
	SkipYes                        // Skip this glyph
	SkipMaybe                      // Maybe skip (default ignorable)
)

// MayMatch checks if a glyph may match based on mask and syllable.
// HarfBuzz equivalent: matcher_t::may_match() in hb-ot-layout-gsubgpos.hh:434-445
func (ctx *OTApplyContext) MayMatch(index int, contextMatch bool) MayMatchResult {
	if index < 0 || index >= len(ctx.Buffer.Info) {
		return MatchNo
	}
	info := &ctx.Buffer.Info[index]

	// Determine effective mask
	// HarfBuzz: mask = context_match ? -1 : c->lookup_mask
	var mask uint32 = 0xFFFFFFFF
	if !contextMatch {
		mask = ctx.FeatureMask
	}

	// Check mask
	// HarfBuzz: return mask ? (info.mask & mask) : true
	// If mask is 0 (global feature), always matches.
	// Otherwise, check if glyph has the required mask bits set.
	if mask != 0 && (info.Mask&mask) == 0 {
		return MatchNo
	}

	return MatchMaybe
}

// MayMatchInfo is like MayMatch but takes a GlyphInfo pointer directly.
// This is needed for backtrack matching which uses the output buffer, not input.
func (ctx *OTApplyContext) MayMatchInfo(info *GlyphInfo, contextMatch bool) MayMatchResult {
	if info == nil {
		return MatchNo
	}

	var mask uint32 = 0xFFFFFFFF
	if !contextMatch {
		mask = ctx.FeatureMask
	}

	if mask != 0 && (info.Mask&mask) == 0 {
		return MatchNo
	}

	if ctx.PerSyllable && ctx.MatchSyllable != 0 && info.Syllable != ctx.MatchSyllable {
		return MatchNo
	}

	return MatchMaybe
}

// MaySkip checks if a glyph should be skipped.
// HarfBuzz equivalent: matcher_t::may_skip() in hb-ot-layout-gsubgpos.hh:453-470
func (ctx *OTApplyContext) MaySkip(index int, contextMatch bool) MaySkipResult {
	if index < 0 || index >= len(ctx.Buffer.Info) {
		return SkipYes
	}
	info := &ctx.Buffer.Info[index]

	// Check glyph property (LookupFlag, GDEF)
	// HarfBuzz: if (!c->check_glyph_property(&info, lookup_props)) return SKIP_YES
	if !ctx.CheckGlyphProperty(info) {
		return SkipYes
	}

	// Check default ignorables
	// HarfBuzz: if (unlikely(_hb_glyph_info_is_default_ignorable(&info) &&
	//                       (ignore_zwnj || !_hb_glyph_info_is_zwnj(&info)) &&
	//                       (ignore_zwj || !_hb_glyph_info_is_zwj(&info)) &&
	//                       (ignore_hidden || !_hb_glyph_info_is_hidden(&info))))
	//             return SKIP_MAYBE
	// Use GlyphProps flags instead of checking Codepoint
	// (Codepoint may be 0 after normalization)
	if (info.GlyphProps & GlyphPropsDefaultIgnorable) != 0 {
		// Determine ignore flags based on table type and context
		// HarfBuzz: ignore_zwnj = c->table_index == 1 || (context_match && c->auto_zwnj)
		ignoreZWNJ := ctx.TableType == TableGPOS || (contextMatch && ctx.AutoZWNJ)
		// HarfBuzz: ignore_zwj = context_match || c->auto_zwj
		ignoreZWJ := contextMatch || ctx.AutoZWJ
		// HarfBuzz: ignore_hidden = c->table_index == 1
		ignoreHidden := ctx.TableType == TableGPOS

		// Check specific character types using preserved flags (not Codepoint which may be 0)
		// HarfBuzz: uses UPROPS_MASK_Cf_ZWNJ and UPROPS_MASK_Cf_ZWJ flags in unicode_props
		isZWNJ := (info.GlyphProps & GlyphPropsZWNJ) != 0
		isZWJ := (info.GlyphProps & GlyphPropsZWJ) != 0
		// HarfBuzz: UPROPS_MASK_HIDDEN for CGJ, Mongolian FVS, TAG chars
		// These should NOT be skipped during GSUB context matching (ignore_hidden=false)
		isHidden := (info.GlyphProps & GlyphPropsHidden) != 0

		if (ignoreZWNJ || !isZWNJ) &&
			(ignoreZWJ || !isZWJ) &&
			(ignoreHidden || !isHidden) {
			return SkipMaybe
		}
	}

	return SkipNo
}

// MaySkipInfo is like MaySkip but takes a GlyphInfo pointer directly.
// This is needed for backtrack matching which uses the output buffer, not input.
// HarfBuzz: prev() in skipping_iterator_t uses out_info, not info.
func (ctx *OTApplyContext) MaySkipInfo(info *GlyphInfo, contextMatch bool) MaySkipResult {
	if info == nil {
		return SkipYes
	}

	// Check glyph property (LookupFlag, GDEF)
	if !ctx.CheckGlyphProperty(info) {
		return SkipYes
	}

	// Check default ignorables
	if (info.GlyphProps & GlyphPropsDefaultIgnorable) != 0 {
		ignoreZWNJ := ctx.TableType == TableGPOS || (contextMatch && ctx.AutoZWNJ)
		ignoreZWJ := contextMatch || ctx.AutoZWJ
		ignoreHidden := ctx.TableType == TableGPOS

		isZWNJ := (info.GlyphProps & GlyphPropsZWNJ) != 0
		isZWJ := (info.GlyphProps & GlyphPropsZWJ) != 0
		isHidden := (info.GlyphProps & GlyphPropsHidden) != 0

		if (ignoreZWNJ || !isZWNJ) &&
			(ignoreZWJ || !isZWJ) &&
			(ignoreHidden || !isHidden) {
			return SkipMaybe
		}
	}

	return SkipNo
}

// CheckGlyphProperty checks if a glyph matches the lookup properties.
// HarfBuzz equivalent: check_glyph_property() in hb-ot-layout-gsubgpos.hh:829-844
func (ctx *OTApplyContext) CheckGlyphProperty(info *GlyphInfo) bool {
	// Get glyph properties from GDEF
	var glyphProps uint16
	if ctx.GDEF != nil && ctx.HasGlyphClasses {
		glyphProps = uint16(ctx.GDEF.GetGlyphClass(info.GlyphID))
	}

	// Convert glyph class to properties
	// HarfBuzz: _hb_glyph_info_get_glyph_props()
	props := glyphClassToProps(glyphProps)

	// Check if glyph class should be ignored
	// HarfBuzz: if (glyph_props & match_props & LookupFlag::IgnoreFlags) return false
	ignoreFlags := ctx.LookupFlag & (LookupFlagIgnoreBaseGlyphs | LookupFlagIgnoreLigatures | LookupFlagIgnoreMarks)
	if props&ignoreFlags != 0 {
		return false
	}

	// Check mark properties
	// HarfBuzz: if (glyph_props & HB_OT_LAYOUT_GLYPH_PROPS_MARK) return match_properties_mark(...)
	if props&glyphPropsMark != 0 {
		return ctx.MatchPropertiesMark(info, props)
	}

	return true
}

// MatchPropertiesMark checks mark attachment properties.
// HarfBuzz equivalent: match_properties_mark() in hb-ot-layout-gsubgpos.hh:806-824
func (ctx *OTApplyContext) MatchPropertiesMark(info *GlyphInfo, glyphProps uint16) bool {
	matchProps := ctx.LookupFlag

	// Check mark filtering set
	// HarfBuzz: if (match_props & LookupFlag::UseMarkFilteringSet)
	//             return gdef_accel.mark_set_covers(match_props >> 16, info->codepoint)
	if matchProps&LookupFlagUseMarkFilteringSet != 0 {
		if ctx.GDEF != nil && ctx.MarkFilteringSet >= 0 {
			return ctx.GDEF.IsInMarkGlyphSet(info.GlyphID, ctx.MarkFilteringSet)
		}
		return true // No GDEF, allow all marks
	}

	// Check mark attachment type (high byte of lookup flags)
	// HarfBuzz: if (match_props & LookupFlag::MarkAttachmentType)
	//             return (match_props & LookupFlag::MarkAttachmentType) == (glyph_props & LookupFlag::MarkAttachmentType)
	const lookupFlagMarkAttachmentType uint16 = 0xFF00
	if matchProps&lookupFlagMarkAttachmentType != 0 {
		// Get mark attachment class from GDEF
		if ctx.GDEF != nil {
			markAttachClass := uint16(ctx.GDEF.GetMarkAttachClass(info.GlyphID)) << 8
			return (matchProps & lookupFlagMarkAttachmentType) == markAttachClass
		}
	}

	return true
}

// ShouldSkipGlyph returns true if the glyph at index should be skipped.
// This is the simplified interface for backward compatibility.
// HarfBuzz equivalent: combination of may_skip() and may_match()
func (ctx *OTApplyContext) ShouldSkipGlyph(index int) bool {
	skip := ctx.MaySkip(index, false)
	if skip == SkipYes {
		return true
	}

	match := ctx.MayMatch(index, false)
	if match == MatchNo {
		return true
	}

	// If skip is SKIP_MAYBE and match is not NO, we should process the glyph
	return false
}

// ShouldSkipContextGlyph returns true if the glyph should be skipped during context matching.
// HarfBuzz equivalent: uses iter_context which is initialized with context_match=true
// This means the mask is NOT checked (mask = -1 in HarfBuzz).
// HarfBuzz: hb-ot-layout-gsubgpos.hh:781 - iter_context.init(this, true)
//
// HarfBuzz match() logic (hb-ot-layout-gsubgpos.hh:562-578):
//
//	if (skip == SKIP_YES) return SKIP;
//	if (match == MATCH_YES || (match == MATCH_MAYBE && skip == SKIP_NO)) return MATCH;
//	if (skip == SKIP_NO) return NOT_MATCH;
//	return SKIP;  // SKIP_MAYBE with non-YES match -> SKIP
func (ctx *OTApplyContext) ShouldSkipContextGlyph(index int) bool {
	skip := ctx.MaySkip(index, true) // contextMatch=true
	if skip == SkipYes {
		return true
	}

	match := ctx.MayMatch(index, true) // contextMatch=true - ignores mask!

	// HarfBuzz: A glyph matches (should NOT be skipped) only if:
	// - match == MATCH_YES, OR
	// - match == MATCH_MAYBE AND skip == SKIP_NO
	// Otherwise, skip it (default ignorables with SKIP_MAYBE get skipped)
	if match == MatchYes || (match == MatchMaybe && skip == SkipNo) {
		return false // MATCH - don't skip
	}

	// skip == SKIP_NO -> NOT_MATCH (don't skip, but will cause mismatch)
	// skip == SKIP_MAYBE -> SKIP (skip this glyph)
	if skip == SkipNo {
		return false // NOT_MATCH - don't skip, let caller handle mismatch
	}

	return true // SKIP - skip this glyph (default ignorable)
}

// --- Glyph Properties ---

// Glyph property flags matching HarfBuzz
// HarfBuzz: HB_OT_LAYOUT_GLYPH_PROPS_* in hb-ot-layout.hh:79-90
const (
	glyphPropsBase        uint16 = 0x02 // HB_OT_LAYOUT_GLYPH_PROPS_BASE_GLYPH
	glyphPropsLigature    uint16 = 0x04 // HB_OT_LAYOUT_GLYPH_PROPS_LIGATURE
	glyphPropsMark        uint16 = 0x08 // HB_OT_LAYOUT_GLYPH_PROPS_MARK
	glyphPropsSubstituted uint16 = 0x10 // HB_OT_LAYOUT_GLYPH_PROPS_SUBSTITUTED
	glyphPropsLigated     uint16 = 0x20 // HB_OT_LAYOUT_GLYPH_PROPS_LIGATED
	glyphPropsMultiplied  uint16 = 0x40 // HB_OT_LAYOUT_GLYPH_PROPS_MULTIPLIED
)

// glyphClassToProps converts GDEF glyph class to property flags.
// HarfBuzz equivalent: _hb_glyph_info_get_glyph_props()
// Note: Component (class 4) returns 0 - HarfBuzz has no GLYPH_PROPS flag for it
func glyphClassToProps(glyphClass uint16) uint16 {
	switch glyphClass {
	case 1: // Base
		return glyphPropsBase
	case 2: // Ligature
		return glyphPropsLigature
	case 3: // Mark
		return glyphPropsMark
	default: // Unclassified (0) or Component (4)
		return 0
	}
}

// LookupFlag constants are defined in gpos.go:
// LookupFlagRightToLeft, LookupFlagIgnoreBaseGlyphs, LookupFlagIgnoreLigatures,
// LookupFlagIgnoreMarks, LookupFlagUseMarkFilteringSet
// LookupFlagMarkAttachmentType is the high byte (0xFF00)

// --- Buffer Operations ---
// HarfBuzz equivalent: methods in hb_ot_apply_context_t (hb-ot-layout-gsubgpos.hh:885-906)

// ReplaceGlyph replaces the current glyph with a new one and advances.
// HarfBuzz equivalent: replace_glyph() in hb-ot-layout-gsubgpos.hh:885-889
//
// HarfBuzz: Calls buffer->replace_glyph() which uses replace_glyphs(1, 1, ...)
// This copies all properties (including cluster) from the current glyph.
func (ctx *OTApplyContext) ReplaceGlyph(newGlyph GlyphID) {
	ctx.setGlyphClass(newGlyph, 0, false, false)
	// Use output buffer pattern to preserve cluster information
	// HarfBuzz: buffer->replace_glyph(glyph_index) in hb-ot-layout-gsubgpos.hh:888
	ctx.Buffer.outputGlyph(newGlyph)
	ctx.Buffer.Idx++
}

// ReplaceGlyphInplace replaces the current glyph without advancing.
// HarfBuzz equivalent: replace_glyph_inplace() in hb-ot-layout-gsubgpos.hh:890-894
func (ctx *OTApplyContext) ReplaceGlyphInplace(newGlyph GlyphID) {
	ctx.setGlyphClass(newGlyph, 0, false, false)
	ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID = newGlyph
}

// ReplaceGlyphWithLigature replaces the current glyph with a ligature glyph.
// HarfBuzz equivalent: replace_glyph_with_ligature() in hb-ot-layout-gsubgpos.hh:895-900
//
// HarfBuzz: Calls buffer->replace_glyph() which uses replace_glyphs(1, 1, ...)
// This copies all properties (including cluster) from the current glyph.
func (ctx *OTApplyContext) ReplaceGlyphWithLigature(newGlyph GlyphID, classGuess int) {
	ctx.setGlyphClass(newGlyph, classGuess, true, false)
	// Use output buffer pattern to preserve cluster information
	// HarfBuzz: buffer->replace_glyph(glyph_index) in hb-ot-layout-gsubgpos.hh:899
	ctx.Buffer.outputGlyph(newGlyph)
	ctx.Buffer.Idx++
}

// OutputGlyphForComponent outputs a glyph as part of a multiple substitution.
// HarfBuzz equivalent: output_glyph_for_component() in hb-ot-layout-gsubgpos.hh:901-906
//
// HarfBuzz: Calls buffer->output_glyph() which uses replace_glyphs(0, 1, ...)
// This inserts a new glyph inheriting properties from the current glyph.
func (ctx *OTApplyContext) OutputGlyphForComponent(newGlyph GlyphID, classGuess int) {
	ctx.setGlyphClass(newGlyph, classGuess, false, true)
	// For multiple substitution, we insert additional glyphs
	// Use output buffer pattern to preserve cluster information
	// HarfBuzz: buffer->output_glyph(glyph_index) in hb-ot-layout-gsubgpos.hh:905
	ctx.Buffer.outputGlyph(newGlyph)
}

// setGlyphClass sets glyph properties on the current glyph.
// HarfBuzz equivalent: _set_glyph_class() in hb-ot-layout-gsubgpos.hh:846-883
func (ctx *OTApplyContext) setGlyphClass(glyph GlyphID, classGuess int, ligature, component bool) {
	// Update syllable if tracking
	if ctx.NewSyllables >= 0 {
		// TODO: Set syllable on current glyph
	}

	info := &ctx.Buffer.Info[ctx.Buffer.Idx]

	// Get current props and add SUBSTITUTED flag
	// HarfBuzz: props |= HB_OT_LAYOUT_GLYPH_PROPS_SUBSTITUTED
	props := info.GlyphProps
	props |= GlyphPropsSubstituted

	if ligature {
		// HarfBuzz: props |= HB_OT_LAYOUT_GLYPH_PROPS_LIGATED
		props |= GlyphPropsLigated
		// HarfBuzz: Clear MULTIPLIED bit when ligating
		// "In the only place that the MULTIPLIED bit is used, Uniscribe
		// seems to only care about the 'last' transformation between
		// Ligature and Multiple substitutions."
		props &^= GlyphPropsMultiplied
	}
	if component {
		// HarfBuzz: props |= HB_OT_LAYOUT_GLYPH_PROPS_MULTIPLIED
		props |= GlyphPropsMultiplied
	}

	// Set glyph class from GDEF or guess
	// HarfBuzz: lines 871-882
	if ctx.GDEF != nil && ctx.HasGlyphClasses {
		// Preserve SUBSTITUTED/LIGATED/MULTIPLIED flags, get base class from GDEF
		props &= GlyphPropsPreserve
		glyphClass := ctx.GDEF.GetGlyphClass(glyph)
		// Convert GDEF class to glyph props
		switch glyphClass {
		case 1: // BaseGlyph
			props |= GlyphPropsBaseGlyph
		case 2: // Ligature
			props |= GlyphPropsLigature
		case 3: // Mark
			props |= GlyphPropsMark
		}
		info.GlyphClass = glyphClass
	} else if classGuess != 0 {
		props &= GlyphPropsPreserve
		props |= uint16(classGuess)
		info.GlyphClass = classGuess
	}

	info.GlyphProps = props
}

// ReplaceGlyphs replaces the current glyph with multiple glyphs.
// HarfBuzz equivalent: Sequence::apply() in OT/Layout/GSUB/Sequence.hh:34-130
//
// This is used for Multiple Substitution (1 -> N).
func (ctx *OTApplyContext) ReplaceGlyphs(newGlyphs []GlyphID) {
	count := len(newGlyphs)

	if count == 0 {
		// Deletion: consume input without output (HarfBuzz: buffer->delete_glyph())
		// Spec disallows this, but Uniscribe allows it.
		// https://github.com/harfbuzz/harfbuzz/issues/253
		ctx.Buffer.Idx++
		return
	}

	if count == 1 {
		// Special-case to make it in-place and not consider this
		// as a "multiplied" substitution.
		// HarfBuzz: c->replace_glyph (substitute.arrayZ[0])
		ctx.ReplaceGlyph(newGlyphs[0])
		return
	}

	// Multiple substitution: 1 -> N (MULTIPLIED)
	// HarfBuzz: Sequence.hh lines 95-107

	// Determine glyph class: if current glyph is a ligature, use BASE_GLYPH
	// HarfBuzz: klass = _hb_glyph_info_is_ligature(&c->buffer->cur()) ? HB_OT_LAYOUT_GLYPH_PROPS_BASE_GLYPH : 0
	var klass int
	if ctx.Buffer.Info[ctx.Buffer.Idx].IsLigature() {
		klass = int(GlyphPropsBaseGlyph)
	}

	// Get lig_id from current glyph
	// HarfBuzz: lig_id = _hb_glyph_info_get_lig_id(&c->buffer->cur())
	ligID := ctx.Buffer.Info[ctx.Buffer.Idx].GetLigID()

	// Output all glyphs with output_glyph_for_component
	// HarfBuzz: for (unsigned int i = 0; i < count; i++) { ... c->output_glyph_for_component(...) }
	for i := 0; i < count; i++ {
		// If not attached to a ligature, set lig_props for component
		// HarfBuzz: if (!lig_id) _hb_glyph_info_set_lig_props_for_component(&c->buffer->cur(), i)
		if ligID == 0 {
			ctx.Buffer.Info[ctx.Buffer.Idx].SetLigPropsForComponent(i)
		}
		ctx.OutputGlyphForComponent(newGlyphs[i], klass)
	}

	// Skip the consumed input glyph
	// HarfBuzz: c->buffer->skip_glyph()
	ctx.Buffer.Idx++
}

// DeleteGlyph deletes the current glyph by skipping it without output.
// HarfBuzz equivalent: replace_glyphs(1, 0, NULL)
//
// In the output buffer pattern, deletion means:
// - Consume 1 input glyph (idx++)
// - Produce 0 output glyphs (no outputGlyph call)
func (ctx *OTApplyContext) DeleteGlyph() {
	// Skip input glyph without outputting
	// HarfBuzz: idx += 1, out_len += 0
	ctx.Buffer.Idx++
}

// LigatePositions replaces glyphs at specific positions with a ligature.
// HarfBuzz equivalent: ligate_input() in hb-ot-layout-gsubgpos.hh:1450-1563
func (ctx *OTApplyContext) LigatePositions(ligGlyph GlyphID, positions []int) {
	count := len(positions)
	if count == 0 {
		return
	}

	if count == 1 {
		// Special-case to make it in-place and not consider this
		// as a "ligated" substitution.
		// HarfBuzz: c->replace_glyph (ligGlyph)
		ctx.ReplaceGlyph(ligGlyph)
		return
	}

	// 1. Merge clusters for all positions (HarfBuzz line 1460)
	firstPos := positions[0]
	lastPos := positions[count-1]
	ctx.Buffer.MergeClusters(firstPos, lastPos+1)

	// 2. Determine ligature type (HarfBuzz lines 1494-1503)
	// - If a base and one or more marks ligate, consider that as a base, NOT ligature
	// - If all components are marks, this is a mark ligature
	isBaseLigature := ctx.Buffer.Info[positions[0]].IsBaseGlyph()
	isMarkLigature := ctx.Buffer.Info[positions[0]].IsMark()
	for i := 1; i < count; i++ {
		if !ctx.Buffer.Info[positions[i]].IsMark() {
			isBaseLigature = false
			isMarkLigature = false
			break
		}
	}
	isLigature := !isBaseLigature && !isMarkLigature

	// 3. Determine class and allocate lig_id (HarfBuzz lines 1505-1506)
	var klass int
	var ligID uint8
	if isLigature {
		klass = int(GlyphPropsLigature)
		ligID = ctx.Buffer.AllocateLigID()
	}

	// 4. Get tracking info for component counting (HarfBuzz lines 1507-1509)
	lastLigID := ctx.Buffer.Info[ctx.Buffer.Idx].GetLigID()
	lastNumComponents := ctx.Buffer.Info[ctx.Buffer.Idx].GetLigNumComps()
	componentsSoFar := lastNumComponents

	// 5. Set lig_props for the ligature glyph (HarfBuzz lines 1511-1518)
	if isLigature {
		// Total component count is the sum of all components
		totalComponentCount := 0
		for i := 0; i < count; i++ {
			totalComponentCount += ctx.Buffer.Info[positions[i]].GetLigNumComps()
		}
		ctx.Buffer.Info[ctx.Buffer.Idx].SetLigPropsForLigature(ligID, totalComponentCount)
	}

	// 6. Replace first glyph with ligature (HarfBuzz line 1519)
	ctx.ReplaceGlyphWithLigature(ligGlyph, klass)

	// 7. Loop over remaining component positions (HarfBuzz lines 1521-1544)
	for i := 1; i < count; i++ {
		// Copy marks between components
		for ctx.Buffer.Idx < positions[i] {
			if isLigature {
				// Update lig_props for marks (HarfBuzz lines 1525-1533)
				thisComp := ctx.Buffer.Info[ctx.Buffer.Idx].GetLigComp()
				if thisComp == 0 {
					thisComp = uint8(lastNumComponents)
				}
				newLigComp := componentsSoFar - lastNumComponents + min(int(thisComp), lastNumComponents)
				ctx.Buffer.Info[ctx.Buffer.Idx].SetLigPropsForMark(ligID, newLigComp)
			}
			ctx.Buffer.nextGlyph()
		}

		// Update tracking (HarfBuzz lines 1538-1540)
		lastLigID = ctx.Buffer.Info[ctx.Buffer.Idx].GetLigID()
		lastNumComponents = ctx.Buffer.Info[ctx.Buffer.Idx].GetLigNumComps()
		componentsSoFar += lastNumComponents

		// Skip the component position (delete it) (HarfBuzz line 1543)
		ctx.Buffer.Idx++
	}

	// 8. Re-adjust components for marks following (HarfBuzz lines 1546-1561)
	if !isMarkLigature && lastLigID != 0 {
		for i := ctx.Buffer.Idx; i < len(ctx.Buffer.Info); i++ {
			if lastLigID != ctx.Buffer.Info[i].GetLigID() {
				break
			}
			thisComp := ctx.Buffer.Info[i].GetLigComp()
			if thisComp == 0 {
				break
			}
			newLigComp := componentsSoFar - lastNumComponents + min(int(thisComp), lastNumComponents)
			ctx.Buffer.Info[i].SetLigPropsForMark(ligID, newLigComp)
		}
	}

	_ = lastLigID // Avoid unused variable warning
}

// --- Navigation ---

// NextGlyph finds the next glyph that should not be skipped.
// HarfBuzz equivalent: skipping_iterator_t::next() with iter_input (context_match=false)
// The mask IS checked here.
func (ctx *OTApplyContext) NextGlyph(startIndex int) int {
	for i := startIndex + 1; i < len(ctx.Buffer.Info); i++ {
		if !ctx.ShouldSkipGlyph(i) {
			return i
		}
	}
	return -1
}

// PrevGlyph finds the previous glyph that should not be skipped.
// HarfBuzz equivalent: skipping_iterator_t::prev() with iter_input (context_match=false)
// The mask IS checked here.
func (ctx *OTApplyContext) PrevGlyph(startIndex int) int {
	for i := startIndex - 1; i >= 0; i-- {
		if !ctx.ShouldSkipGlyph(i) {
			return i
		}
	}
	return -1
}

// ContextMatchFunc is a callback that checks whether a glyph matches at a given position.
// It is used by NextContextMatch/PrevContextMatch to implement the 3-way match logic
// from HarfBuzz's skipping_iterator_t::match().
// Returns true if the glyph matches the expected value.
type ContextMatchFunc func(info *GlyphInfo) bool

// NextContextGlyph finds the next non-skippable glyph for context matching.
// This simplified version does NOT implement the 3-way match logic.
// For proper HarfBuzz-compatible context matching, use NextContextMatch instead.
func (ctx *OTApplyContext) NextContextGlyph(startIndex int) int {
	for i := startIndex + 1; i < len(ctx.Buffer.Info); i++ {
		skip := ctx.MaySkip(i, true) // contextMatch=true
		if skip == SkipYes {
			continue
		}
		if skip == SkipNo || skip == SkipMaybe {
			return i
		}
	}
	return -1
}

// PrevContextGlyph finds the previous non-skippable glyph for context matching.
// This simplified version does NOT implement the 3-way match logic.
// For proper HarfBuzz-compatible context matching, use PrevContextMatch instead.
func (ctx *OTApplyContext) PrevContextGlyph(startIndex int) int {
	for i := startIndex - 1; i >= 0; i-- {
		skip := ctx.MaySkip(i, true) // contextMatch=true
		if skip == SkipYes {
			continue
		}
		if skip == SkipNo || skip == SkipMaybe {
			return i
		}
	}
	return -1
}

// NextContextMatch finds the next context glyph that matches, implementing HarfBuzz's
// 3-way match logic (hb-ot-layout-gsubgpos.hh:562-578):
//
//	SKIP_YES                        → skip (continue to next)
//	SKIP_NO  + match                → MATCH (return position)
//	SKIP_NO  + no match             → NOT_MATCH (return -1, rule fails)
//	SKIP_MAYBE + match              → MATCH (return position)
//	SKIP_MAYBE + no match           → SKIP (continue to next)
//
// This is critical for default ignorables like ZWJ: they have SKIP_MAYBE and if
// they don't match the expected class/coverage, they should be skipped over, not
// cause the entire rule to fail.
func (ctx *OTApplyContext) NextContextMatch(startIndex int, matchFn ContextMatchFunc) int {
	for i := startIndex + 1; i < len(ctx.Buffer.Info); i++ {
		skip := ctx.MaySkip(i, true) // contextMatch=true
		if skip == SkipYes {
			continue
		}

		// Check may_match: mask (always passes for context: mask=0xFFFFFFFF) and per_syllable
		// HarfBuzz: may_match() checks (info.mask & mask) and (per_syllable && syllable && syllable != info.syllable())
		info := &ctx.Buffer.Info[i]
		mayMatch := true
		if ctx.PerSyllable && ctx.MatchSyllable != 0 && info.Syllable != ctx.MatchSyllable {
			mayMatch = false
		}

		if !mayMatch {
			// may_match returned MATCH_NO
			if skip == SkipNo {
				return -1 // NOT_MATCH
			}
			// skip == SkipMaybe → SKIP
			continue
		}

		matched := matchFn(info)

		if matched {
			return i // MATCH
		}

		if skip == SkipNo {
			return -1 // NOT_MATCH: non-skippable glyph doesn't match → rule fails
		}

		// skip == SkipMaybe && !matched → SKIP: continue searching
	}
	return -1
}

// PrevContextMatch finds the previous context glyph that matches, with 3-way match logic.
// See NextContextMatch for detailed documentation.
func (ctx *OTApplyContext) PrevContextMatch(startIndex int, matchFn ContextMatchFunc) int {
	// HarfBuzz: backtrack matching uses out_info (output buffer) when have_output is true.
	// This is critical because earlier substitutions change the backtrack context.
	// HarfBuzz: prev() in skipping_iterator_t uses out_info for backtrack.
	// HarfBuzz reference: hb-ot-layout-gsubgpos.hh:1569-1601 (match_backtrack)
	if ctx.Buffer.HaveOutput() {
		// Use output buffer for backtrack matching
		backtrackLen := ctx.Buffer.BacktrackLen()
		// startIndex is relative to input buffer; convert to backtrack position
		// When have_output, positions before Idx map to outInfo positions
		pos := backtrackLen
		if startIndex < ctx.Buffer.Idx {
			pos = startIndex
		}
		for i := pos - 1; i >= 0; i-- {
			info := ctx.Buffer.BacktrackInfo(i)
			if info == nil {
				return -1
			}
			skip := ctx.MaySkipInfo(info, true) // contextMatch=true
			if skip == SkipYes {
				continue
			}

			mayMatch := true
			if ctx.PerSyllable && ctx.MatchSyllable != 0 && info.Syllable != ctx.MatchSyllable {
				mayMatch = false
			}

			if !mayMatch {
				if skip == SkipNo {
					return -1
				}
				continue
			}

			matched := matchFn(info)
			if matched {
				return i
			}
			if skip == SkipNo {
				return -1
			}
		}
		return -1
	}

	// No output buffer — use input buffer directly
	for i := startIndex - 1; i >= 0; i-- {
		skip := ctx.MaySkip(i, true) // contextMatch=true
		if skip == SkipYes {
			continue
		}

		// Check may_match: per_syllable check
		// HarfBuzz: may_match() checks (per_syllable && syllable && syllable != info.syllable())
		info := &ctx.Buffer.Info[i]
		mayMatch := true
		if ctx.PerSyllable && ctx.MatchSyllable != 0 && info.Syllable != ctx.MatchSyllable {
			mayMatch = false
		}

		if !mayMatch {
			if skip == SkipNo {
				return -1 // NOT_MATCH
			}
			continue // SKIP
		}

		matched := matchFn(info)

		if matched {
			return i // MATCH
		}

		if skip == SkipNo {
			return -1 // NOT_MATCH
		}

		// skip == SkipMaybe && !matched → SKIP
	}
	return -1
}

// --- Utility Methods ---

// IsDefaultIgnorableAt returns true if the glyph at index is a default ignorable.
func (ctx *OTApplyContext) IsDefaultIgnorableAt(index int) bool {
	if index < 0 || index >= len(ctx.Buffer.Info) {
		return false
	}
	return IsDefaultIgnorable(ctx.Buffer.Info[index].Codepoint)
}

// IsVariationSelectorAt returns true if the glyph at index is a variation selector.
func (ctx *OTApplyContext) IsVariationSelectorAt(index int) bool {
	if index < 0 || index >= len(ctx.Buffer.Info) {
		return false
	}
	cp := ctx.Buffer.Info[index].Codepoint
	// FE00-FE0F: Variation Selectors
	// E0100-E01EF: Variation Selectors Supplement
	return (cp >= 0xFE00 && cp <= 0xFE0F) || (cp >= 0xE0100 && cp <= 0xE01EF)
}

// MergeClusters merges clusters in the range [start, end).
func (ctx *OTApplyContext) MergeClusters(start, end int) {
	ctx.Buffer.MergeClusters(start, end)
}

// --- GPOS-specific ---

// AdjustPosition adjusts the position at the given index with a ValueRecord.
// HarfBuzz equivalent: applies ValueRecord values directly to position.
func (ctx *OTApplyContext) AdjustPosition(index int, vr *ValueRecord) {
	if ctx.Buffer == nil || index < 0 || index >= len(ctx.Buffer.Pos) {
		return
	}
	ctx.Buffer.Pos[index].XOffset += vr.XPlacement
	ctx.Buffer.Pos[index].YOffset += vr.YPlacement
	ctx.Buffer.Pos[index].XAdvance += vr.XAdvance
	ctx.Buffer.Pos[index].YAdvance += vr.YAdvance
}
