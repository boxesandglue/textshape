package ot

// Unicode Script detection
//
// HarfBuzz equivalent: hb-unicode.hh unicode->script() and hb-common.cc hb_script_get_horizontal_direction()
//
// This file provides functions to determine the Unicode script for a codepoint
// and the horizontal direction for a script.

// GetScriptTag returns the OpenType script tag for a Unicode codepoint.
// Uses the UCD (Unicode Character Database) table for accurate script detection.
// HarfBuzz equivalent: unicode->script() in hb-unicode.hh
func GetScriptTag(cp Codepoint) Tag {
	// Use UCD table from ucd_table.go (generated from Unicode data)
	scriptStr := getScriptTag(cp)

	// Convert 4-char script tag string to Tag
	// Common (Zyyy) and Inherited (Zinh) return 0 to indicate they don't determine script
	switch scriptStr {
	case "Zyyy", "Zinh", "Zzzz":
		return 0
	case "":
		return 0
	default:
		// Convert UCD script codes to OpenType script tags.
		// HarfBuzz equivalent: hb_ot_old_tag_from_script() in hb-ot-tag.cc:36-60
		// Some scripts have different OpenType tags (with trailing spaces) vs UCD codes.
		switch scriptStr {
		case "Laoo":
			return MakeTag('L', 'a', 'o', ' ')
		case "Yiii":
			return MakeTag('Y', 'i', ' ', ' ')
		case "Nkoo":
			return MakeTag('N', 'k', 'o', ' ')
		case "Vaii":
			return MakeTag('V', 'a', 'i', ' ')
		}

		// Pad to 4 chars if needed
		for len(scriptStr) < 4 {
			scriptStr += " "
		}
		// Return ISO 15924 format (uppercase-first): 'Arab', 'Hebr', 'Phag'
		// HarfBuzz stores scripts internally in this format.
		// Conversion to OpenType format (lowercase) happens in GSUB/GPOS lookup.
		return MakeTag(scriptStr[0], scriptStr[1], scriptStr[2], scriptStr[3])
	}
}

// GetHorizontalDirection returns the horizontal direction for a script tag.
// HarfBuzz equivalent: hb_script_get_horizontal_direction() in hb-common.cc:523-613
// Script tags are ISO 15924 format (uppercase-first): 'Arab', 'Hebr', etc.
func GetHorizontalDirection(script Tag) Direction {
	switch script {
	// RTL scripts (from HarfBuzz hb-common.cc:528-600)
	case MakeTag('A', 'r', 'a', 'b'), // Arabic
		MakeTag('H', 'e', 'b', 'r'), // Hebrew
		MakeTag('S', 'y', 'r', 'c'), // Syriac
		MakeTag('T', 'h', 'a', 'a'), // Thaana
		MakeTag('C', 'p', 'r', 't'), // Cypriot
		MakeTag('K', 'h', 'a', 'r'), // Kharoshthi
		MakeTag('P', 'h', 'n', 'x'), // Phoenician
		MakeTag('N', 'k', 'o', ' '), // NKo
		MakeTag('L', 'y', 'd', 'i'), // Lydian
		MakeTag('A', 'v', 's', 't'), // Avestan
		MakeTag('A', 'r', 'm', 'i'), // Imperial Aramaic
		MakeTag('P', 'h', 'l', 'i'), // Inscriptional Pahlavi
		MakeTag('P', 'r', 't', 'i'), // Inscriptional Parthian
		MakeTag('S', 'a', 'r', 'b'), // Old South Arabian
		MakeTag('O', 'r', 'k', 'h'), // Old Turkic
		MakeTag('S', 'a', 'm', 'r'), // Samaritan
		MakeTag('M', 'a', 'n', 'd'), // Mandaic
		MakeTag('M', 'e', 'r', 'c'), // Meroitic Cursive
		MakeTag('M', 'e', 'r', 'o'), // Meroitic Hieroglyphs
		MakeTag('M', 'a', 'n', 'i'), // Manichaean
		MakeTag('M', 'e', 'n', 'd'), // Mende Kikakui
		MakeTag('N', 'b', 'a', 't'), // Nabataean
		MakeTag('N', 'a', 'r', 'b'), // Old North Arabian
		MakeTag('P', 'a', 'l', 'm'), // Palmyrene
		MakeTag('P', 'h', 'l', 'p'), // Psalter Pahlavi
		MakeTag('H', 'a', 't', 'r'), // Hatran
		MakeTag('A', 'd', 'l', 'm'), // Adlam
		MakeTag('R', 'o', 'h', 'g'), // Hanifi Rohingya
		MakeTag('S', 'o', 'g', 'o'), // Old Sogdian
		MakeTag('S', 'o', 'g', 'd'), // Sogdian
		MakeTag('E', 'l', 'y', 'm'), // Elymaic
		MakeTag('C', 'h', 'r', 's'), // Chorasmian
		MakeTag('Y', 'e', 'z', 'i'), // Yezidi
		MakeTag('O', 'u', 'g', 'r'): // Old Uyghur
		return DirectionRTL

	// Bidirectional scripts (can be RTL or LTR) - return 0 (invalid)
	case MakeTag('H', 'u', 'n', 'g'), // Old Hungarian
		MakeTag('I', 't', 'a', 'l'), // Old Italic
		MakeTag('R', 'u', 'n', 'r'), // Runic
		MakeTag('T', 'f', 'n', 'g'): // Tifinagh
		return 0 // Invalid - caller should default to LTR
	}

	// Default to LTR for all other scripts
	return DirectionLTR
}

// IsScriptCommon returns true if the codepoint is in Common or Inherited script.
// These codepoints don't determine the script of a text run.
// HarfBuzz: checks for HB_SCRIPT_COMMON and HB_SCRIPT_INHERITED
func IsScriptCommon(cp Codepoint) bool {
	script := GetScriptTag(cp)
	return script == 0
}
