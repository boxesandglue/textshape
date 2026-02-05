package ot

// USE category constants - Universal Shaping Engine
// HarfBuzz equivalent: hb-ot-shaper-use-machine.hh:57-100
//
// These define the character categories used by the USE shaper.
// Each character is assigned a category based on its role in syllable formation.

type USECategory uint8

const (
	USE_O     USECategory = 0  // OTHER
	USE_B     USECategory = 1  // BASE
	USE_V     USECategory = 2  // VOWEL (no position)
	USE_VM    USECategory = 3  // VOWEL_MOD (no position)
	USE_N     USECategory = 4  // BASE_NUM
	USE_GB    USECategory = 5  // BASE_OTHER (Generic Base)
	USE_CGJ   USECategory = 6  // CGJ (Combining Grapheme Joiner)
	USE_F     USECategory = 7  // FINAL (no position)
	USE_FM    USECategory = 8  // FINAL_MOD (no position)
	USE_M     USECategory = 9  // MEDIAL (no position)
	USE_CM    USECategory = 10 // CONS_MOD (no position)
	USE_SUB   USECategory = 11 // CONS_SUB
	USE_H     USECategory = 12 // HALANT
	USE_HN    USECategory = 13 // HALANT_NUM
	USE_ZWNJ  USECategory = 14 // ZWNJ
	USE_SM    USECategory = 15 // SYLLABLE_MOD (no position)
	USE_WJ    USECategory = 16 // Word_Joiner
	USE_R     USECategory = 18 // REPHA
	USE_VPre  USECategory = 22 // VOWEL PRE
	USE_VMPre USECategory = 23 // VOWEL_MOD PRE
	USE_FAbv  USECategory = 24 // FINAL ABOVE
	USE_FBlw  USECategory = 25 // FINAL BELOW
	USE_FPst  USECategory = 26 // FINAL POST
	USE_MAbv  USECategory = 27 // MEDIAL ABOVE
	USE_MBlw  USECategory = 28 // MEDIAL BELOW
	USE_MPst  USECategory = 29 // MEDIAL POST
	USE_MPre  USECategory = 30 // MEDIAL PRE
	USE_CMAbv USECategory = 31 // CONS_MOD ABOVE
	USE_CMBlw USECategory = 32 // CONS_MOD BELOW
	USE_VAbv  USECategory = 33 // VOWEL ABOVE
	USE_VBlw  USECategory = 34 // VOWEL BELOW
	USE_VPst  USECategory = 35 // VOWEL POST
	USE_VMAbv USECategory = 37 // VOWEL_MOD ABOVE
	USE_VMBlw USECategory = 38 // VOWEL_MOD BELOW
	USE_VMPst USECategory = 39 // VOWEL_MOD POST
	USE_SMAbv USECategory = 41 // SYLLABLE_MOD ABOVE
	USE_SMBlw USECategory = 42 // SYLLABLE_MOD BELOW
	USE_CS    USECategory = 43 // CONS_WITH_STACKER
	USE_IS    USECategory = 44 // INVISIBLE_STACKER
	USE_FMAbv USECategory = 45 // FINAL_MOD ABOVE
	USE_FMBlw USECategory = 46 // FINAL_MOD BELOW
	USE_FMPst USECategory = 47 // FINAL_MOD POST
	USE_Sk    USECategory = 48 // SAKOT
	USE_G     USECategory = 49 // HIEROGLYPH
	USE_J     USECategory = 50 // HIEROGLYPH_JOINER
	USE_SB    USECategory = 51 // HIEROGLYPH_SEGMENT_BEGIN
	USE_SE    USECategory = 52 // HIEROGLYPH_SEGMENT_END
	USE_HVM   USECategory = 53 // HALANT_OR_VOWEL_MODIFIER
	USE_HM    USECategory = 54 // HIEROGLYPH_MOD
	USE_HR    USECategory = 55 // HIEROGLYPH_MIRROR
	USE_RK    USECategory = 56 // REORDERING_KILLER
)

// USESyllableInfo holds syllable information for a glyph.
type USESyllableInfo struct {
	Category     USECategory
	SyllableType USESyllableType
	Syllable     uint8     // Syllable index (upper 4 bits = serial, lower 4 bits = type)
	Codepoint    Codepoint // Original codepoint, needed for ZWNJ filtering
}

// USESyllableType defines the types of syllables in USE.
// HarfBuzz equivalent: use_syllable_type_t in hb-ot-shaper-use-machine.hh:43-53
type USESyllableType uint8

const (
	USE_ViramaTerminatedCluster USESyllableType = iota
	USE_SakotTerminatedCluster
	USE_StandardCluster
	USE_NumberJoinerTerminatedCluster
	USE_NumeralCluster
	USE_SymbolCluster
	USE_HieroglyphCluster
	USE_BrokenCluster
	USE_NonCluster
)

// String returns the name of the syllable type.
func (t USESyllableType) String() string {
	switch t {
	case USE_ViramaTerminatedCluster:
		return "virama_terminated"
	case USE_SakotTerminatedCluster:
		return "sakot_terminated"
	case USE_StandardCluster:
		return "standard"
	case USE_NumberJoinerTerminatedCluster:
		return "number_joiner_terminated"
	case USE_NumeralCluster:
		return "numeral"
	case USE_SymbolCluster:
		return "symbol"
	case USE_HieroglyphCluster:
		return "hieroglyph"
	case USE_BrokenCluster:
		return "broken"
	case USE_NonCluster:
		return "non_cluster"
	default:
		return "unknown"
	}
}

// isUSEVowel returns true if the category is a vowel.
func isUSEVowel(cat USECategory) bool {
	switch cat {
	case USE_VPre, USE_VAbv, USE_VBlw, USE_VPst:
		return true
	}
	return false
}

// isUSEMedial returns true if the category is a medial.
func isUSEMedial(cat USECategory) bool {
	switch cat {
	case USE_MPre, USE_MAbv, USE_MBlw, USE_MPst:
		return true
	}
	return false
}

// isUSEModifier returns true if the category is a modifier.
func isUSEModifier(cat USECategory) bool {
	switch cat {
	case USE_CMAbv, USE_CMBlw, USE_VMPre, USE_VMAbv, USE_VMBlw, USE_VMPst,
		USE_SMAbv, USE_SMBlw, USE_FMAbv, USE_FMBlw, USE_FMPst:
		return true
	}
	return false
}

// isUSEFinal returns true if the category is a final consonant.
func isUSEFinal(cat USECategory) bool {
	switch cat {
	case USE_FAbv, USE_FBlw, USE_FPst:
		return true
	}
	return false
}

// isUSEHalant returns true if the category is a halant/virama and the glyph is not ligated.
// HarfBuzz equivalent: is_halant_use() in hb-ot-shaper-use.cc:354-359
// HarfBuzz checks: (info.use_category() == USE(H) || ...) && !_hb_glyph_info_ligated(&info)
func isUSEHalant(cat USECategory, info *GlyphInfo) bool {
	if info != nil && (info.GlyphProps&GlyphPropsLigated) != 0 {
		return false
	}
	return cat == USE_H || cat == USE_HVM || cat == USE_IS
}
