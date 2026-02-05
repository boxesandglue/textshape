package ot

// OT Shaper - HarfBuzz-style script-specific shaping
//
// HarfBuzz equivalent: hb_ot_shaper_t in hb-ot-shaper.hh:67-169
//
// This system provides a unified interface for script-specific shapers.
// Each shaper can customize various phases of the shaping process.

// ZeroWidthMarksType controls how zero-width marks are handled.
// HarfBuzz equivalent: hb_ot_shape_zero_width_marks_type_t in hb-ot-shaper.hh:44-48
type ZeroWidthMarksType int

const (
	// ZeroWidthMarksNone - Don't zero mark advances
	ZeroWidthMarksNone ZeroWidthMarksType = iota
	// ZeroWidthMarksByGDEFEarly - Zero mark advances early (before GPOS)
	ZeroWidthMarksByGDEFEarly
	// ZeroWidthMarksByGDEFLate - Zero mark advances late (after GPOS)
	ZeroWidthMarksByGDEFLate
)

// OTShaper defines the interface for script-specific shapers.
// HarfBuzz equivalent: hb_ot_shaper_t in hb-ot-shaper.hh:67-169
//
// All function fields are optional (nil means use default behavior).
// This design matches HarfBuzz where NULL function pointers are allowed.
type OTShaper struct {
	// Name identifies this shaper (for debugging)
	Name string

	// CollectFeatures is called during plan compilation.
	// Shapers should add their features to the plan's map.
	// HarfBuzz: collect_features (hb-ot-shaper.hh:74)
	CollectFeatures func(plan *ShapePlan)

	// OverrideFeatures is called after common features are added.
	// Shapers can override or modify features here.
	// HarfBuzz: override_features (hb-ot-shaper.hh:82)
	OverrideFeatures func(plan *ShapePlan)

	// DataCreate is called at the end of plan compilation.
	// Returns shaper-specific data that will be stored in the plan.
	// HarfBuzz: data_create (hb-ot-shaper.hh:90)
	DataCreate func(plan *ShapePlan) interface{}

	// DataDestroy is called when the plan is destroyed.
	// HarfBuzz: data_destroy (hb-ot-shaper.hh:98)
	DataDestroy func(data interface{})

	// PreprocessText is called before shaping starts.
	// Shapers can modify the buffer text here.
	// HarfBuzz: preprocess_text (hb-ot-shaper.hh:106)
	PreprocessText func(plan *ShapePlan, buf *Buffer, font *Font)

	// PostprocessGlyphs is called after shaping ends.
	// Shapers can modify glyphs here.
	// HarfBuzz: postprocess_glyphs (hb-ot-shaper.hh:115)
	PostprocessGlyphs func(plan *ShapePlan, buf *Buffer, font *Font)

	// Decompose is called during normalization.
	// Returns the decomposition of a codepoint (a, b) or ok=false if not decomposable.
	// HarfBuzz: decompose (hb-ot-shaper.hh:124)
	Decompose func(c *NormalizeContext, ab Codepoint) (a, b Codepoint, ok bool)

	// Compose is called during normalization.
	// Returns the composition of (a, b) or ok=false if not composable.
	// HarfBuzz: compose (hb-ot-shaper.hh:133)
	Compose func(c *NormalizeContext, a, b Codepoint) (ab Codepoint, ok bool)

	// SetupMasks is called to set feature masks on glyphs.
	// Shapers should use the plan's map to get masks and set them on the buffer.
	// HarfBuzz: setup_masks (hb-ot-shaper.hh:144)
	SetupMasks func(plan *ShapePlan, buf *Buffer, font *Font)

	// ReorderMarks is called to reorder combining marks.
	// HarfBuzz: reorder_marks (hb-ot-shaper.hh:153)
	ReorderMarks func(plan *ShapePlan, buf *Buffer, start, end int)

	// GPOSTag - If not zero, must match GPOS script tag for GPOS to be applied.
	// HarfBuzz: gpos_tag (hb-ot-shaper.hh:162)
	GPOSTag Tag

	// NormalizationPreference controls how normalization is performed.
	// HarfBuzz: normalization_preference (hb-ot-shaper.hh:164)
	NormalizationPreference NormalizationMode

	// ZeroWidthMarks controls how zero-width marks are handled.
	// HarfBuzz: zero_width_marks (hb-ot-shaper.hh:166)
	ZeroWidthMarks ZeroWidthMarksType

	// FallbackPosition enables fallback positioning when GPOS is not available.
	// HarfBuzz: fallback_position (hb-ot-shaper.hh:168)
	FallbackPosition bool
}

// NormalizeContext provides context for decompose/compose callbacks.
// HarfBuzz equivalent: hb_ot_shape_normalize_context_t in hb-ot-shape-normalize.hh:54-65
type NormalizeContext struct {
	Plan   *ShapePlan
	Buffer *Buffer
	Font   *Font
	Shaper *Shaper
}

// ShapePlan holds a compiled shaping plan.
// HarfBuzz equivalent: hb_ot_shape_plan_t in hb-ot-shape.hh
//
// The plan is compiled once and can be reused for multiple shaping calls.
// This improves performance by avoiding repeated feature lookups.
type ShapePlan struct {
	// Shaper is the script-specific shaper for this plan
	Shaper *OTShaper

	// Map contains the compiled lookup map
	Map *OTMap

	// Props contains segment properties (direction, script, language)
	Props SegmentProperties

	// ShaperData holds shaper-specific data created by DataCreate
	ShaperData interface{}

	// Cached masks for common features
	FracMask uint32
	NumrMask uint32
	DnomMask uint32
	HasFrac  bool

	RTLMMask uint32
	HasVert  bool

	KernMask         uint32
	RequestedKerning bool

	// Internal references
	gsub *GSUB
	gpos *GPOS
	gdef *GDEF
}

// SegmentProperties holds text segment properties.
// HarfBuzz equivalent: hb_segment_properties_t
type SegmentProperties struct {
	Direction Direction
	Script    Tag
	Language  Tag
}

// --- Predefined Shapers ---

// DefaultShaper is the default shaper for scripts without special handling.
// HarfBuzz equivalent: _hb_ot_shaper_default in hb-ot-shaper-default.cc:34-50
var DefaultShaper = &OTShaper{
	Name:                    "default",
	NormalizationPreference: NormalizationModeAuto,
	ZeroWidthMarks:          ZeroWidthMarksByGDEFLate,
	FallbackPosition:        true,
}

// QaagShaper is the shaper for Zawgyi (Myanmar visual encoding).
// HarfBuzz equivalent: _hb_ot_shaper_myanmar_zawgyi in hb-ot-shaper-myanmar.cc:363-378
//
// Zawgyi is a legacy encoding for Myanmar that uses visual ordering.
// Characters are already in display order, so no reordering is needed.
// All callbacks are nil (use default behavior), but with:
// - NormalizationModeNone: No normalization
// - ZeroWidthMarksNone: Don't zero mark advances
// - FallbackPosition: false: No fallback positioning
var QaagShaper = &OTShaper{
	Name:                    "qaag",
	NormalizationPreference: NormalizationModeNone,
	ZeroWidthMarks:          ZeroWidthMarksNone,
	FallbackPosition:        false,
}

// --- Shaper Selection ---

// SelectShaperWithFont returns the appropriate shaper based on script, direction, and font script tag.
// HarfBuzz equivalent: hb_ot_shaper_categorize() in hb-ot-shaper.hh:176-350
//
// This function implements the critical logic from HarfBuzz where Indic scripts with
// script tag version 3 (e.g., 'knd3', 'dev3') use the USE shaper instead of the Indic shaper.
// See hb-ot-shaper.hh:242-245:
//
//	else if ((gsub_script & 0x000000FF) == '3')
//	    return &_hb_ot_shaper_use;
//
// Parameters:
//   - script: The Unicode script tag (e.g., 'Knda' for Kannada)
//   - direction: Text direction
//   - fontScriptTag: The actual script tag found in the font's GSUB table (e.g., 'knd3')
func SelectShaperWithFont(script Tag, direction Direction, fontScriptTag Tag) *OTShaper {
	// For Indic scripts, check if the font uses version 3 script tag
	// HarfBuzz: hb-ot-shaper.hh:220-245
	switch script {
	case MakeTag('D', 'e', 'v', 'a'), // Devanagari
		MakeTag('B', 'e', 'n', 'g'), // Bengali
		MakeTag('G', 'u', 'r', 'u'), // Gurmukhi
		MakeTag('G', 'u', 'j', 'r'), // Gujarati
		MakeTag('O', 'r', 'y', 'a'), // Oriya
		MakeTag('T', 'a', 'm', 'l'), // Tamil
		MakeTag('T', 'e', 'l', 'u'), // Telugu
		MakeTag('K', 'n', 'd', 'a'), // Kannada
		MakeTag('M', 'l', 'y', 'm'): // Malayalam
		// Note: Sinhala is NOT in this group - it uses USE shaper (hb-ot-shaper.hh:280)

		// Check for DFLT or latn fallback
		if fontScriptTag == MakeTag('D', 'F', 'L', 'T') ||
			fontScriptTag == MakeTag('l', 'a', 't', 'n') {
			return DefaultShaper
		}
		// If script tag ends with '3', use USE shaper
		// HarfBuzz: else if ((gsub_script & 0x000000FF) == '3')
		if byte(fontScriptTag&0xFF) == '3' {
			return USEShaper
		}
		return IndicShaper

	case MakeTag('M', 'y', 'm', 'r'): // Myanmar
		// Myanmar: DFLT/latn/mymr use default, otherwise Myanmar shaper
		if fontScriptTag == MakeTag('D', 'F', 'L', 'T') ||
			fontScriptTag == MakeTag('l', 'a', 't', 'n') ||
			fontScriptTag == MakeTag('m', 'y', 'm', 'r') {
			return DefaultShaper
		}
		return MyanmarShaper

	case MakeTag('Q', 'a', 'a', 'g'): // Zawgyi (Myanmar visual encoding)
		// Zawgyi uses visual encoding - no reordering needed
		// HarfBuzz equivalent: _hb_ot_shaper_myanmar_zawgyi in hb-ot-shaper-myanmar.cc
		return QaagShaper
	}

	// For other scripts, use the standard selection
	return SelectShaper(script, direction)
}

// SelectShaper returns the appropriate shaper for the given script and direction.
// HarfBuzz equivalent: hb_ot_shaper_categorize() in hb-ot-shaper.hh:176-350
// Script tags are ISO 15924 format (uppercase-first): 'Arab', 'Hebr', etc.
// Note: For Indic scripts, prefer SelectShaperWithFont which considers the font's script tag.
func SelectShaper(script Tag, direction Direction) *OTShaper {
	switch script {
	// Arabic and related scripts
	case MakeTag('A', 'r', 'a', 'b'), // Arabic
		MakeTag('S', 'y', 'r', 'c'): // Syriac
		if direction.IsHorizontal() {
			return ArabicShaper
		}
		return DefaultShaper

	// Mongolian and related (uses Arabic joining but different direction)
	case MakeTag('M', 'o', 'n', 'g'), // Mongolian
		MakeTag('P', 'h', 'a', 'g'): // Phags-pa
		return ArabicShaper

	// Indic scripts
	case MakeTag('D', 'e', 'v', 'a'), // Devanagari
		MakeTag('B', 'e', 'n', 'g'), // Bengali
		MakeTag('G', 'u', 'r', 'u'), // Gurmukhi
		MakeTag('G', 'u', 'j', 'r'), // Gujarati
		MakeTag('O', 'r', 'y', 'a'), // Oriya
		MakeTag('T', 'a', 'm', 'l'), // Tamil
		MakeTag('T', 'e', 'l', 'u'), // Telugu
		MakeTag('K', 'n', 'd', 'a'), // Kannada
		MakeTag('M', 'l', 'y', 'm'): // Malayalam
		// Note: Sinhala uses USE shaper, not Indic (hb-ot-shaper.hh:280)
		return IndicShaper

	// Khmer
	case MakeTag('K', 'h', 'm', 'r'):
		return KhmerShaper

	// Myanmar
	case MakeTag('M', 'y', 'm', 'r'):
		return MyanmarShaper

	// Zawgyi (Myanmar visual encoding) - uses default shaper
	case MakeTag('Q', 'a', 'a', 'g'):
		return DefaultShaper

	// USE scripts (Universal Shaping Engine)
	// HarfBuzz: hb-ot-shaper.hh:275-414 - comprehensive list of USE scripts
	case MakeTag('S', 'i', 'n', 'h'), // Sinhala (USE, not Indic - hb-ot-shaper.hh:280)
		MakeTag('J', 'a', 'v', 'a'), // Javanese
		MakeTag('B', 'a', 'l', 'i'), // Balinese
		MakeTag('S', 'u', 'n', 'd'), // Sundanese
		MakeTag('T', 'i', 'b', 't'), // Tibetan
		MakeTag('A', 'h', 'o', 'm'), // Ahom
		MakeTag('B', 'a', 't', 'k'), // Batak
		MakeTag('B', 'h', 'k', 's'), // Bhaiksuki
		MakeTag('B', 'r', 'a', 'h'), // Brahmi
		MakeTag('B', 'u', 'g', 'i'), // Buginese
		MakeTag('B', 'u', 'h', 'd'), // Buhid
		MakeTag('C', 'a', 'k', 'm'), // Chakma
		MakeTag('C', 'h', 'a', 'm'), // Cham
		MakeTag('D', 'i', 'a', 'k'), // Dives Akuru
		MakeTag('D', 'o', 'g', 'r'), // Dogra
		MakeTag('G', 'r', 'a', 'n'), // Grantha
		MakeTag('G', 'o', 'n', 'g'), // Gunjala Gondi
		MakeTag('G', 'u', 'k', 'h'), // Gurung Khema
		MakeTag('H', 'a', 'n', 'o'), // Hanunoo
		MakeTag('K', 'a', 'i', 't'), // Kaithi
		MakeTag('K', 'a', 'w', 'i'), // Kawi
		MakeTag('K', 'a', 'l', 'i'), // Kayah Li
		MakeTag('K', 'h', 'a', 'r'), // Kharoshthi
		MakeTag('K', 'h', 'o', 'j'), // Khojki
		MakeTag('S', 'i', 'n', 'd'), // Khudawadi
		MakeTag('K', 'r', 'a', 'i'), // Kirat Rai
		MakeTag('L', 'e', 'p', 'c'), // Lepcha
		MakeTag('L', 'i', 'm', 'b'), // Limbu
		MakeTag('M', 'a', 'h', 'j'), // Mahajani
		MakeTag('M', 'a', 'k', 'a'), // Makasar
		MakeTag('M', 'a', 'r', 'c'), // Marchen
		MakeTag('G', 'o', 'n', 'm'), // Masaram Gondi
		MakeTag('M', 't', 'e', 'i'), // Meetei Mayek
		MakeTag('M', 'o', 'd', 'i'), // Modi
		MakeTag('M', 'u', 'l', 't'), // Multani
		MakeTag('N', 'a', 'n', 'd'), // Nandinagari
		MakeTag('T', 'a', 'l', 'u'), // New Tai Lue
		MakeTag('N', 'e', 'w', 'a'), // Newa
		MakeTag('R', 'j', 'n', 'g'), // Rejang
		MakeTag('S', 'a', 'u', 'r'), // Saurashtra
		MakeTag('S', 'h', 'r', 'd'), // Sharada
		MakeTag('S', 'i', 'd', 'd'), // Siddham
		MakeTag('S', 'o', 'y', 'o'), // Soyombo
		MakeTag('S', 'y', 'l', 'o'), // Syloti Nagri
		MakeTag('T', 'g', 'l', 'g'), // Tagalog
		MakeTag('T', 'a', 'g', 'b'), // Tagbanwa
		MakeTag('T', 'a', 'l', 'e'), // Tai Le
		MakeTag('L', 'a', 'n', 'a'), // Tai Tham
		MakeTag('T', 'a', 'v', 't'), // Tai Viet
		MakeTag('T', 'a', 'k', 'r'), // Takri
		MakeTag('T', 'i', 'r', 'h'), // Tirhuta
		MakeTag('T', 'u', 'l', 'u'), // Tulu Tigalari
		MakeTag('Z', 'a', 'n', 'b'), // Zanabazar Square
		// Scripts with Arabic-like joining that use USE shaper
		MakeTag('A', 'd', 'l', 'm'), // Adlam
		MakeTag('C', 'h', 'r', 's'), // Chorasmian
		MakeTag('R', 'o', 'h', 'g'), // Hanifi Rohingya
		MakeTag('M', 'a', 'n', 'd'), // Mandaic
		MakeTag('M', 'a', 'n', 'i'), // Manichaean
		MakeTag('N', 'k', 'o', ' '), // N'Ko
		MakeTag('O', 'u', 'g', 'r'), // Old Uyghur
		MakeTag('P', 'h', 'l', 'p'), // Psalter Pahlavi
		MakeTag('S', 'o', 'g', 'd'): // Sogdian
		return USEShaper

	// Thai and Lao
	case MakeTag('T', 'h', 'a', 'i'),
		MakeTag('L', 'a', 'o', ' '):
		return ThaiShaper

	// Hebrew
	case MakeTag('H', 'e', 'b', 'r'):
		return HebrewShaper

	// Hangul
	case MakeTag('H', 'a', 'n', 'g'):
		return HangulShaper

	default:
		return DefaultShaper
	}
}

// --- Placeholder Shapers ---
// These will be implemented fully later. For now they use default behavior.

// ArabicShaper handles Arabic and related scripts.
// HarfBuzz equivalent: _hb_ot_shaper_arabic in hb-ot-shaper-arabic.cc:752-768
var ArabicShaper = &OTShaper{
	Name:                    "arabic",
	NormalizationPreference: NormalizationModeAuto,
	ZeroWidthMarks:          ZeroWidthMarksByGDEFLate,
	FallbackPosition:        true,
	// Functions will be set in init()
}

// IndicShaper handles Indic scripts.
// HarfBuzz equivalent: _hb_ot_shaper_indic
var IndicShaper = &OTShaper{
	Name:                    "indic",
	NormalizationPreference: NormalizationModeComposedDiacritics,
	ZeroWidthMarks:          ZeroWidthMarksNone, // HarfBuzz: NONE (not LATE!)
	FallbackPosition:        false,              // Indic uses 'dist' not 'kern'
}

// KhmerShaper handles Khmer script.
// HarfBuzz equivalent: _hb_ot_shaper_khmer
var KhmerShaper = &OTShaper{
	Name:                    "khmer",
	NormalizationPreference: NormalizationModeComposedDiacritics,
	ZeroWidthMarks:          ZeroWidthMarksNone, // HarfBuzz: NONE (not LATE!)
	FallbackPosition:        false,
}

// MyanmarShaper handles Myanmar script.
// HarfBuzz equivalent: _hb_ot_shaper_myanmar (hb-ot-shaper-myanmar.cc:361)
var MyanmarShaper = &OTShaper{
	Name:                    "myanmar",
	NormalizationPreference: NormalizationModeComposedDiacritics,
	ZeroWidthMarks:          ZeroWidthMarksByGDEFEarly, // HarfBuzz: BY_GDEF_EARLY, not LATE!
	FallbackPosition:        false,
}

// USEShaper handles USE (Universal Shaping Engine) scripts.
// HarfBuzz equivalent: _hb_ot_shaper_use
var USEShaper = &OTShaper{
	Name:                    "use",
	NormalizationPreference: NormalizationModeComposedDiacritics,
	ZeroWidthMarks:          ZeroWidthMarksByGDEFEarly, // HarfBuzz: BY_GDEF_EARLY
	FallbackPosition:        false,
}

// ThaiShaper handles Thai and Lao scripts.
// HarfBuzz equivalent: _hb_ot_shaper_thai
var ThaiShaper = &OTShaper{
	Name:                    "thai",
	NormalizationPreference: NormalizationModeAuto,
	ZeroWidthMarks:          ZeroWidthMarksByGDEFLate,
	FallbackPosition:        true,
}

// HebrewShaper handles Hebrew script.
// HarfBuzz equivalent: _hb_ot_shaper_hebrew
var HebrewShaper = &OTShaper{
	Name:                    "hebrew",
	NormalizationPreference: NormalizationModeAuto,
	ZeroWidthMarks:          ZeroWidthMarksByGDEFLate,
	FallbackPosition:        true,
}

// HangulShaper handles Hangul script.
// HarfBuzz equivalent: _hb_ot_shaper_hangul
var HangulShaper = &OTShaper{
	Name:                    "hangul",
	NormalizationPreference: NormalizationModeNone,
	ZeroWidthMarks:          ZeroWidthMarksNone,
	FallbackPosition:        true,
}
