package ot

// Indic character categories and positions.
//
// HarfBuzz equivalent: hb-ot-shaper-indic.hh and hb-ot-shaper-indic-table.cc
//
// Categories classify characters for syllable parsing.
// Positions determine visual ordering within a syllable.

// IndicCategory represents the category of an Indic character.
// HarfBuzz equivalent: indic_category_t in hb-ot-shaper-indic-machine.hh
type IndicCategory uint8

const (
	ICatX            IndicCategory = 0  // Other
	ICatC            IndicCategory = 1  // Consonant
	ICatV            IndicCategory = 2  // Vowel
	ICatN            IndicCategory = 3  // Nukta
	ICatH            IndicCategory = 4  // Halant/Virama
	ICatZWNJ         IndicCategory = 5  // Zero Width Non-Joiner
	ICatZWJ          IndicCategory = 6  // Zero Width Joiner
	ICatM            IndicCategory = 7  // Matra (vowel sign)
	ICatSM           IndicCategory = 8  // Syllable Modifier (anusvara, visarga)
	ICatA            IndicCategory = 9  // Vedic Accent / VD (Vedic Sign)
	ICatPLACEHOLDER  IndicCategory = 10 // Placeholder (number, etc.)
	ICatDOTTEDCIRCLE IndicCategory = 11 // Dotted Circle (U+25CC)
	ICatRS           IndicCategory = 12 // Reordering Spacing Mark (rare)
	ICatMPst         IndicCategory = 13 // Post-base Matra
	ICatRepha        IndicCategory = 14 // Repha (Malayalam)
	ICatRa           IndicCategory = 15 // Ra consonant (special for Reph formation)
	ICatCM           IndicCategory = 16 // Consonant Medial
	ICatSymbol       IndicCategory = 17 // Symbol
	ICatCS           IndicCategory = 18 // Consonant with Stacker
	ICatSMPst        IndicCategory = 57 // Post-base Syllable Modifier
)

// IndicPosition represents the visual position of a character in a syllable.
// HarfBuzz equivalent: ot_position_t in hb-ot-shaper-indic.hh
type IndicPosition uint8

const (
	IPosStart      IndicPosition = 0
	IPosRaToBeReph IndicPosition = 1  // Ra that will become Reph
	IPosPreM       IndicPosition = 2  // Pre-base Matra
	IPosPreC       IndicPosition = 3  // Pre-base Consonant
	IPosBaseC      IndicPosition = 4  // Base Consonant
	IPosAfterMain  IndicPosition = 5  // After main consonant
	IPosAboveC     IndicPosition = 6  // Above base consonant
	IPosBeforeSub  IndicPosition = 7  // Before sub-joined consonant
	IPosBelowC     IndicPosition = 8  // Below base consonant
	IPosAfterSub   IndicPosition = 9  // After sub-joined consonant
	IPosBeforePost IndicPosition = 10 // Before post-base consonant
	IPosPostC      IndicPosition = 11 // Post-base Consonant
	IPosAfterPost  IndicPosition = 12 // After post-base consonant
	IPosSMVD       IndicPosition = 13 // Syllable Modifier / Vedic
	IPosEnd        IndicPosition = 14
)

// IndicSyllableType represents the type of Indic syllable.
// HarfBuzz equivalent: indic_syllable_type_t
type IndicSyllableType uint8

const (
	IndicConsonantSyllable IndicSyllableType = 0
	IndicVowelSyllable     IndicSyllableType = 1
	IndicStandaloneCluster IndicSyllableType = 2
	IndicSymbolCluster     IndicSyllableType = 3
	IndicBrokenCluster     IndicSyllableType = 4
	IndicNonIndicCluster   IndicSyllableType = 5
)

// IndicInfo holds the Indic category and position for a glyph.
// This is stored in GlyphInfo as additional shaping data.
type IndicInfo struct {
	Category IndicCategory
	Position IndicPosition
	Syllable uint8 // Syllable index (serial << 4 | type)
}

// combineIndicCategories combines category and position into a single uint16.
// This matches HarfBuzz's INDIC_COMBINE_CATEGORIES macro.
func combineIndicCategories(cat IndicCategory, pos IndicPosition) uint16 {
	return uint16(cat) | (uint16(pos) << 8)
}

// getIndicCategory extracts category from combined value.
func getIndicCategory(combined uint16) IndicCategory {
	return IndicCategory(combined & 0xFF)
}

// getIndicPosition extracts position from combined value.
func getIndicPosition(combined uint16) IndicPosition {
	return IndicPosition(combined >> 8)
}

// IsIndicConsonant returns true if the category is a consonant type.
// HarfBuzz: is_consonant() using CONSONANT_FLAGS_INDIC
func IsIndicConsonant(cat IndicCategory) bool {
	switch cat {
	case ICatC, ICatCS, ICatRa, ICatCM, ICatV, ICatPLACEHOLDER, ICatDOTTEDCIRCLE:
		return true
	}
	return false
}

// IsIndicJoiner returns true if the category is ZWJ or ZWNJ.
func IsIndicJoiner(cat IndicCategory) bool {
	return cat == ICatZWJ || cat == ICatZWNJ
}

// IsIndicHalant returns true if the category is Halant.
func IsIndicHalant(cat IndicCategory) bool {
	return cat == ICatH
}
