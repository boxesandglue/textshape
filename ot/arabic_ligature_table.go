package ot

// Arabic Ligature Tables for Fallback Shaping
//
// These tables define ligatures using Unicode Arabic Presentation Forms.
// Used when font has no GSUB features but has presentation form glyphs.
//
// Source: HarfBuzz hb-ot-shaper-arabic-table.hh:423-558

// ligaturePair represents a single ligature: components -> ligature glyph
type ligaturePair struct {
	component uint16 // Second component (first is in ligatureSet)
	ligature  uint16 // Resulting ligature
}

// ligatureSet represents all ligatures starting with a specific first glyph
type ligatureSet struct {
	first     uint16         // First component (in presentation form)
	ligatures []ligaturePair // List of ligatures
}

// ligature3Pair represents a 3-component ligature
type ligature3Pair struct {
	component1 uint16 // Second component
	component2 uint16 // Third component
	ligature   uint16 // Resulting ligature
}

// ligature3Set represents all 3-component ligatures starting with a specific first glyph
type ligature3Set struct {
	first     uint16          // First component (in presentation form)
	ligatures []ligature3Pair // List of ligatures
}

// ligatureTable contains 2-component ligatures
// Source: HarfBuzz hb-ot-shaper-arabic-table.hh:423-524
var ligatureTable = []ligatureSet{
	// BEH initial form (0xFE91)
	{0xFE91, []ligaturePair{
		{0xFEE2, 0xFC08}, // BEH + MEEM -> ARABIC LIGATURE BEH WITH MEEM ISOLATED FORM
		{0xFEE4, 0xFC9F}, // BEH + MEEM.medi -> ARABIC LIGATURE BEH WITH MEEM INITIAL FORM
		{0xFEA0, 0xFC9C}, // BEH + JEEM.medi -> ARABIC LIGATURE BEH WITH JEEM INITIAL FORM
		{0xFEA4, 0xFC9D}, // BEH + HAH.medi -> ARABIC LIGATURE BEH WITH HAH INITIAL FORM
		{0xFEA8, 0xFC9E}, // BEH + KHAH.medi -> ARABIC LIGATURE BEH WITH KHAH INITIAL FORM
	}},
	// BEH medial form (0xFE92)
	{0xFE92, []ligaturePair{
		{0xFEAE, 0xFC6A}, // BEH + REH.fina -> ARABIC LIGATURE BEH WITH REH FINAL FORM
		{0xFEE6, 0xFC6D}, // BEH + NOON.fina -> ARABIC LIGATURE BEH WITH NOON FINAL FORM
		{0xFEF2, 0xFC6F}, // BEH + YEH.fina -> ARABIC LIGATURE BEH WITH YEH FINAL FORM
	}},
	// TEH initial form (0xFE97)
	{0xFE97, []ligaturePair{
		{0xFEE2, 0xFC0E}, // TEH + MEEM -> ARABIC LIGATURE TEH WITH MEEM ISOLATED FORM
		{0xFEE4, 0xFCA4}, // TEH + MEEM.medi -> ARABIC LIGATURE TEH WITH MEEM INITIAL FORM
		{0xFEA0, 0xFCA1}, // TEH + JEEM.medi -> ARABIC LIGATURE TEH WITH JEEM INITIAL FORM
		{0xFEA4, 0xFCA2}, // TEH + HAH.medi -> ARABIC LIGATURE TEH WITH HAH INITIAL FORM
		{0xFEA8, 0xFCA3}, // TEH + KHAH.medi -> ARABIC LIGATURE TEH WITH KHAH INITIAL FORM
	}},
	// TEH medial form (0xFE98)
	{0xFE98, []ligaturePair{
		{0xFEAE, 0xFC70}, // TEH + REH.fina -> ARABIC LIGATURE TEH WITH REH FINAL FORM
		{0xFEE6, 0xFC73}, // TEH + NOON.fina -> ARABIC LIGATURE TEH WITH NOON FINAL FORM
		{0xFEF2, 0xFC75}, // TEH + YEH.fina -> ARABIC LIGATURE TEH WITH YEH FINAL FORM
	}},
	// THEH initial form (0xFE9B)
	{0xFE9B, []ligaturePair{
		{0xFEE2, 0xFC12}, // THEH + MEEM -> ARABIC LIGATURE THEH WITH MEEM ISOLATED FORM
	}},
	// JEEM initial form (0xFE9F)
	{0xFE9F, []ligaturePair{
		{0xFEE4, 0xFCA8}, // JEEM + MEEM.medi -> ARABIC LIGATURE JEEM WITH MEEM INITIAL FORM
	}},
	// HAH initial form (0xFEA3)
	{0xFEA3, []ligaturePair{
		{0xFEE4, 0xFCAA}, // HAH + MEEM.medi -> ARABIC LIGATURE HAH WITH MEEM INITIAL FORM
	}},
	// KHAH initial form (0xFEA7)
	{0xFEA7, []ligaturePair{
		{0xFEE4, 0xFCAC}, // KHAH + MEEM.medi -> ARABIC LIGATURE KHAH WITH MEEM INITIAL FORM
	}},
	// SEEN initial form (0xFEB3)
	{0xFEB3, []ligaturePair{
		{0xFEE4, 0xFCB0}, // SEEN + MEEM.medi -> ARABIC LIGATURE SEEN WITH MEEM INITIAL FORM
	}},
	// SHEEN initial form (0xFEB7)
	{0xFEB7, []ligaturePair{
		{0xFEE4, 0xFD30}, // SHEEN + MEEM.medi -> ARABIC LIGATURE SHEEN WITH MEEM INITIAL FORM
	}},
	// FEH initial form (0xFED3)
	{0xFED3, []ligaturePair{
		{0xFEF2, 0xFC32}, // FEH + YEH.fina -> ARABIC LIGATURE FEH WITH YEH ISOLATED FORM
	}},
	// LAM initial form (0xFEDF) - main ligature source
	{0xFEDF, []ligaturePair{
		{0xFE9E, 0xFC3F}, // LAM + JEEM.fina -> ARABIC LIGATURE LAM WITH JEEM ISOLATED FORM
		{0xFEA0, 0xFCC9}, // LAM + JEEM.medi -> ARABIC LIGATURE LAM WITH JEEM INITIAL FORM
		{0xFEA2, 0xFC40}, // LAM + HAH.fina -> ARABIC LIGATURE LAM WITH HAH ISOLATED FORM
		{0xFEA4, 0xFCCA}, // LAM + HAH.medi -> ARABIC LIGATURE LAM WITH HAH INITIAL FORM
		{0xFEA6, 0xFC41}, // LAM + KHAH.fina -> ARABIC LIGATURE LAM WITH KHAH ISOLATED FORM
		{0xFEA8, 0xFCCB}, // LAM + KHAH.medi -> ARABIC LIGATURE LAM WITH KHAH INITIAL FORM
		{0xFEE2, 0xFC42}, // LAM + MEEM.fina -> ARABIC LIGATURE LAM WITH MEEM ISOLATED FORM
		{0xFEE4, 0xFCCC}, // LAM + MEEM.medi -> ARABIC LIGATURE LAM WITH MEEM INITIAL FORM
		{0xFEF2, 0xFC44}, // LAM + YEH.fina -> ARABIC LIGATURE LAM WITH YEH ISOLATED FORM
		{0xFEEC, 0xFCCD}, // LAM + HEH.medi -> ARABIC LIGATURE LAM WITH HEH INITIAL FORM
		// Lam-Alef ligatures (mandatory)
		{0xFE82, 0xFEF5}, // LAM + ALEF_MADDA.fina -> ARABIC LIGATURE LAM WITH ALEF WITH MADDA ABOVE ISOLATED FORM
		{0xFE84, 0xFEF7}, // LAM + ALEF_HAMZA_ABOVE.fina -> ARABIC LIGATURE LAM WITH ALEF WITH HAMZA ABOVE ISOLATED FORM
		{0xFE88, 0xFEF9}, // LAM + ALEF_HAMZA_BELOW.fina -> ARABIC LIGATURE LAM WITH ALEF WITH HAMZA BELOW ISOLATED FORM
		{0xFE8E, 0xFEFB}, // LAM + ALEF.fina -> ARABIC LIGATURE LAM WITH ALEF ISOLATED FORM
	}},
	// LAM medial form (0xFEE0) - for final form ligatures
	{0xFEE0, []ligaturePair{
		{0xFEF0, 0xFC86}, // LAM + ALEF_MAKSURA.fina -> ARABIC LIGATURE LAM WITH ALEF MAKSURA FINAL FORM
		// Lam-Alef ligatures (final position)
		{0xFE82, 0xFEF6}, // LAM + ALEF_MADDA.fina -> ARABIC LIGATURE LAM WITH ALEF WITH MADDA ABOVE FINAL FORM
		{0xFE84, 0xFEF8}, // LAM + ALEF_HAMZA_ABOVE.fina -> ARABIC LIGATURE LAM WITH ALEF WITH HAMZA ABOVE FINAL FORM
		{0xFE88, 0xFEFA}, // LAM + ALEF_HAMZA_BELOW.fina -> ARABIC LIGATURE LAM WITH ALEF WITH HAMZA BELOW FINAL FORM
		{0xFE8E, 0xFEFC}, // LAM + ALEF.fina -> ARABIC LIGATURE LAM WITH ALEF FINAL FORM
	}},
	// MEEM initial form (0xFEE3)
	{0xFEE3, []ligaturePair{
		{0xFEA0, 0xFCCE}, // MEEM + JEEM.medi -> ARABIC LIGATURE MEEM WITH JEEM INITIAL FORM
		{0xFEA4, 0xFCCF}, // MEEM + HAH.medi -> ARABIC LIGATURE MEEM WITH HAH INITIAL FORM
		{0xFEA8, 0xFCD0}, // MEEM + KHAH.medi -> ARABIC LIGATURE MEEM WITH KHAH INITIAL FORM
		{0xFEE4, 0xFCD1}, // MEEM + MEEM.medi -> ARABIC LIGATURE MEEM WITH MEEM INITIAL FORM
	}},
	// NOON initial form (0xFEE7)
	{0xFEE7, []ligaturePair{
		{0xFEE2, 0xFC4E}, // NOON + MEEM.fina -> ARABIC LIGATURE NOON WITH MEEM ISOLATED FORM
		{0xFEE4, 0xFCD5}, // NOON + MEEM.medi -> ARABIC LIGATURE NOON WITH MEEM INITIAL FORM
		{0xFEA0, 0xFCD2}, // NOON + JEEM.medi -> ARABIC LIGATURE NOON WITH JEEM INITIAL FORM
		{0xFEA4, 0xFCD3}, // NOON + HAH.medi -> ARABIC LIGATURE NOON WITH HAH INITIAL FORM
	}},
	// NOON medial form (0xFEE8)
	{0xFEE8, []ligaturePair{
		{0xFEF2, 0xFC8F}, // NOON + YEH.fina -> ARABIC LIGATURE NOON WITH YEH FINAL FORM
	}},
	// YEH initial form (0xFEF3)
	{0xFEF3, []ligaturePair{
		{0xFEA0, 0xFCDA}, // YEH + JEEM.medi -> ARABIC LIGATURE YEH WITH JEEM INITIAL FORM
		{0xFEA4, 0xFCDB}, // YEH + HAH.medi -> ARABIC LIGATURE YEH WITH HAH INITIAL FORM
		{0xFEA8, 0xFCDC}, // YEH + KHAH.medi -> ARABIC LIGATURE YEH WITH KHAH INITIAL FORM
		{0xFEE4, 0xFCDD}, // YEH + MEEM.medi -> ARABIC LIGATURE YEH WITH MEEM INITIAL FORM
	}},
	// YEH medial form (0xFEF4)
	{0xFEF4, []ligaturePair{
		{0xFEAE, 0xFC91}, // YEH + REH.fina -> ARABIC LIGATURE YEH WITH REH FINAL FORM
		{0xFEE6, 0xFC94}, // YEH + NOON.fina -> ARABIC LIGATURE YEH WITH NOON FINAL FORM
	}},
}

// ligatureMarkTable contains ligatures for combining marks
// Source: HarfBuzz hb-ot-shaper-arabic-table.hh:527-542
var ligatureMarkTable = []ligatureSet{
	// SHADDA (0x0651) - uses Unicode codepoints, not presentation forms
	{0x0651, []ligaturePair{
		{0x064C, 0xFC5E}, // SHADDA + DAMMATAN -> ARABIC LIGATURE SHADDA WITH DAMMATAN ISOLATED FORM
		{0x064E, 0xFC60}, // SHADDA + FATHA -> ARABIC LIGATURE SHADDA WITH FATHA ISOLATED FORM
		{0x064F, 0xFC61}, // SHADDA + DAMMA -> ARABIC LIGATURE SHADDA WITH DAMMA ISOLATED FORM
		{0x0650, 0xFC62}, // SHADDA + KASRA -> ARABIC LIGATURE SHADDA WITH KASRA ISOLATED FORM
		{0x064B, 0xF2EE}, // SHADDA + FATHATAN -> PUA ARABIC LIGATURE SHADDA WITH FATHATAN ISOLATED FORM
	}},
}

// ligature3Table contains 3-component ligatures
// Source: HarfBuzz hb-ot-shaper-arabic-table.hh:545-558
var ligature3Table = []ligature3Set{
	// LAM initial form (0xFEDF)
	{0xFEDF, []ligature3Pair{
		{0xFEE4, 0xFEA4, 0xFD88}, // LAM + MEEM.medi + HAH.medi -> ARABIC LIGATURE LAM WITH MEEM WITH HAH INITIAL FORM
		{0xFEE0, 0xFEEA, 0xF201}, // LAM + LAM.medi + HEH.fina -> PUA ARABIC LIGATURE LELLAH ISOLATED FORM
		{0xFEE4, 0xFEA0, 0xF211}, // LAM + MEEM.medi + JEEM.medi -> PUA ARABIC LIGATURE LAM WITH MEEM WITH JEEM INITIAL FORM
	}},
}
