package ot

// Arabic Windows-1256 Fallback Shaping
//
// This provides fallback Arabic shaping for fonts that use Windows-1256 encoding.
// These fonts map Arabic Unicode codepoints to specific glyph IDs that match
// the Windows-1256 codepage positions.
//
// Source: HarfBuzz hb-ot-shaper-arabic-win1256.hh

// win1256Signature contains the expected glyph IDs for signature codepoints
// to detect Windows-1256 encoded fonts.
// Source: HarfBuzz hb-ot-shaper-arabic-fallback.hh:260-264
var win1256Signature = []struct {
	codepoint Codepoint
	glyphID   GlyphID
}{
	{0x0627, 199}, // ALEF
	{0x0644, 225}, // LAM
	{0x0649, 236}, // ALEF MAKSURA
	{0x064A, 237}, // YEH
	{0x0652, 250}, // SUKUN
}

// isWin1256Font checks if the font appears to be Windows-1256 encoded
// by checking if specific Arabic codepoints map to expected glyph IDs.
func isWin1256Font(cmap *Cmap) bool {
	for _, sig := range win1256Signature {
		glyph, found := cmap.Lookup(sig.codepoint)
		if !found || glyph != sig.glyphID {
			return false
		}
	}
	return true
}

// win1256SingleSubst contains single substitution data for Windows-1256 fonts.
// Each entry maps input glyph IDs to output glyph IDs.
// Source: HarfBuzz hb-ot-shaper-arabic-win1256.hh:235-258
type win1256SubstTable struct {
	inputs  []GlyphID
	outputs []GlyphID
}

// initmediSubst: Common substitutions for init and medi forms
// Source: HarfBuzz initmediSubLookup
var win1256InitmediSubst = win1256SubstTable{
	inputs:  []GlyphID{198, 200, 201, 202, 203, 204, 205, 206, 211, 212, 213, 214, 223, 225, 227, 228, 236, 237},
	outputs: []GlyphID{162, 4, 5, 5, 6, 7, 9, 11, 13, 14, 15, 26, 140, 141, 142, 143, 154, 154},
}

// initSubst: Additional init-only substitutions
// Source: HarfBuzz initSubLookup
var win1256InitSubst = win1256SubstTable{
	inputs:  []GlyphID{218, 219, 221, 222, 229},
	outputs: []GlyphID{27, 30, 128, 131, 144},
}

// mediSubst: Additional medi-only substitutions
// Source: HarfBuzz mediSubLookup
var win1256MediSubst = win1256SubstTable{
	inputs:  []GlyphID{218, 219, 221, 222, 229},
	outputs: []GlyphID{28, 31, 129, 138, 149},
}

// finaSubst: Final form substitutions
// Source: HarfBuzz finaSubLookup
var win1256FinaSubst = win1256SubstTable{
	inputs:  []GlyphID{194, 195, 197, 198, 199, 201, 204, 205, 206, 218, 219, 229, 236, 237},
	outputs: []GlyphID{2, 1, 3, 181, 0, 159, 8, 10, 12, 29, 127, 152, 160, 156},
}

// medifinaLamAlefSubst: Lam-Alef medi/fina substitutions
// Source: HarfBuzz medifinaLamAlefSubLookup
var win1256MedifinaLamAlefSubst = win1256SubstTable{
	inputs:  []GlyphID{165, 178, 180, 252},
	outputs: []GlyphID{170, 179, 185, 255},
}

// win1256LigatureEntry represents a ligature for Windows-1256 fonts
type win1256LigatureEntry struct {
	first     GlyphID
	second    GlyphID
	ligature  GlyphID
}

// win1256LamAlefLigatures: Lam + Alef ligatures
// Source: HarfBuzz lamAlefLigaturesSubLookup
// LAM (225) + various Alef forms → ligature
var win1256LamAlefLigatures = []win1256LigatureEntry{
	{225, 199, 165}, // LAM + ALEF → ligature
	{225, 195, 178}, // LAM + ALEF_HAMZA_ABOVE → ligature
	{225, 194, 180}, // LAM + ALEF_MADDA → ligature
	{225, 197, 252}, // LAM + ALEF_HAMZA_BELOW → ligature
}

// win1256ShaddaLigatures: Shadda + vowel ligatures
// Source: HarfBuzz shaddaLigaturesSubLookup
// SHADDA (248) + vowel → ligature
var win1256ShaddaLigatures = []win1256LigatureEntry{
	{248, 243, 172}, // SHADDA + FATHA → ligature
	{248, 245, 173}, // SHADDA + DAMMA → ligature
	{248, 246, 175}, // SHADDA + KASRA → ligature
}

// createWin1256FallbackPlan creates a fallback plan for Windows-1256 encoded fonts.
// Returns nil if the font is not Windows-1256 encoded.
func createWin1256FallbackPlan(cmap *Cmap) *arabicFallbackPlan {
	if !isWin1256Font(cmap) {
		return nil
	}

	plan := &arabicFallbackPlan{}
	j := 0

	// init lookup (index 0): initmedi + init
	initLookup := createWin1256SingleLookup([]win1256SubstTable{win1256InitmediSubst, win1256InitSubst})
	if initLookup != nil {
		plan.masks[j] = MaskInit
		plan.lookups[j] = initLookup
		j++
	}

	// medi lookup (index 1): initmedi + medi + medifinaLamAlef
	mediLookup := createWin1256SingleLookup([]win1256SubstTable{win1256InitmediSubst, win1256MediSubst, win1256MedifinaLamAlefSubst})
	if mediLookup != nil {
		plan.masks[j] = MaskMedi
		plan.lookups[j] = mediLookup
		j++
	}

	// fina lookup (index 2): fina + medifinaLamAlef
	finaLookup := createWin1256SingleLookup([]win1256SubstTable{win1256FinaSubst, win1256MedifinaLamAlefSubst})
	if finaLookup != nil {
		plan.masks[j] = MaskFina
		plan.lookups[j] = finaLookup
		j++
	}

	// rlig lookup (index 3): Lam-Alef ligatures (IgnoreMarks)
	rligLookup := createWin1256LigatureLookup(win1256LamAlefLigatures, true)
	if rligLookup != nil {
		plan.masks[j] = MaskGlobal
		plan.lookups[j] = rligLookup
		j++
	}

	// rlig marks lookup (index 4): Shadda ligatures (no IgnoreMarks)
	rligMarksLookup := createWin1256LigatureLookup(win1256ShaddaLigatures, false)
	if rligMarksLookup != nil {
		plan.masks[j] = MaskGlobal
		plan.lookups[j] = rligMarksLookup
		j++
	}

	plan.numLookups = j

	if j == 0 {
		return nil
	}

	return plan
}

// createWin1256SingleLookup creates a single substitution lookup from multiple tables
func createWin1256SingleLookup(tables []win1256SubstTable) *fallbackLookup {
	// Merge all tables into one
	substMap := make(map[GlyphID]GlyphID)
	for _, table := range tables {
		for i, input := range table.inputs {
			if i < len(table.outputs) {
				substMap[input] = table.outputs[i]
			}
		}
	}

	if len(substMap) == 0 {
		return nil
	}

	// Convert to sorted entries
	entries := make([]fallbackSubstEntry, 0, len(substMap))
	for input, output := range substMap {
		entries = append(entries, fallbackSubstEntry{
			glyph:      input,
			substitute: output,
		})
	}

	// Sort by glyph ID
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].glyph > entries[j].glyph {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	return &fallbackLookup{
		lookupType:  1,
		ignoreMarks: true,
		singles:     entries,
	}
}

// createWin1256LigatureLookup creates a ligature lookup from ligature entries
func createWin1256LigatureLookup(ligatures []win1256LigatureEntry, ignoreMarks bool) *fallbackLookup {
	if len(ligatures) == 0 {
		return nil
	}

	entries := make([]fallbackLigatureEntry, 0, len(ligatures))
	for _, lig := range ligatures {
		entries = append(entries, fallbackLigatureEntry{
			firstGlyph: lig.first,
			components: []GlyphID{lig.second},
			ligature:   lig.ligature,
		})
	}

	return &fallbackLookup{
		lookupType:  4,
		ignoreMarks: ignoreMarks,
		ligatures:   entries,
	}
}
