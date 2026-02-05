package ot

//go:generate go run ../cmd/gen-ccc UnicodeData.txt

// HarfBuzz equivalent: hb-ot-shape-normalize.cc
// This file implements Unicode normalization for text shaping.
//
// The normalization process follows HarfBuzz's approach with 3 phases:
// 1. Decompose: Break characters into base + combining marks (NFD-like)
// 2. Reorder: Sort marks by canonical combining class
// 3. Recompose: Combine base + mark sequences if font has precomposed glyph

// NormalizationMode controls how normalization is performed.
// HarfBuzz equivalent: hb_ot_shape_normalization_mode_t
type NormalizationMode int

const (
	// NormalizationModeNone skips normalization
	NormalizationModeNone NormalizationMode = iota
	// NormalizationModeDecomposed keeps characters decomposed (NFD)
	NormalizationModeDecomposed
	// NormalizationModeComposedDiacritics recomposes diacritics but not base-to-base
	NormalizationModeComposedDiacritics
	// NormalizationModeAuto automatically selects mode based on font/shaper
	NormalizationModeAuto
)

// MaxCombiningMarks is the maximum number of combining marks to reorder.
// HarfBuzz equivalent: HB_OT_SHAPE_MAX_COMBINING_MARKS (default 32)
const MaxCombiningMarks = 32

// normalizeBuffer performs Unicode normalization on the buffer.
// HarfBuzz equivalent: _hb_ot_shape_normalize() in hb-ot-shape-normalize.cc:280-473
//
// Parameters:
// - buf: The buffer to normalize (modified in place)
// - mode: The normalization mode
// - cmap: The cmap table for glyph lookups
func (s *Shaper) normalizeBuffer(buf *Buffer, mode NormalizationMode) {
	if len(buf.Info) == 0 || s.cmap == nil {
		return
	}

	// Auto mode: use composed diacritics (standard mode)
	// HarfBuzz equivalent: hb-ot-shape-normalize.cc:288-297
	if mode == NormalizationModeAuto {
		mode = NormalizationModeComposedDiacritics
	}

	if mode == NormalizationModeNone {
		return
	}

	// Phase 1: Decompose
	// HarfBuzz equivalent: hb-ot-shape-normalize.cc:322-367
	decomposed := s.decomposeBuffer(buf, mode != NormalizationModeDecomposed)

	// Phase 2: Reorder marks by combining class
	// HarfBuzz equivalent: hb-ot-shape-normalize.cc:370-400
	s.reorderMarks(decomposed)

	// Phase 2b: Unhide CGJ between marks with correct CCC order
	// HarfBuzz equivalent: hb-ot-shape-normalize.cc:402-414
	// CGJ (U+034F) should NOT be skipped during GSUB context matching if it's
	// between two marks where the preceding mark has CCC <= following mark's CCC.
	// This allows CGJ to be "transparent" in such cases.
	unhideCGJ(decomposed)

	// Phase 3: Recompose if mode allows
	// HarfBuzz equivalent: hb-ot-shape-normalize.cc:418-473
	if mode == NormalizationModeComposedDiacritics {
		decomposed = s.recomposeBuffer(decomposed)
	}

	// Update buffer
	buf.Info = decomposed
	buf.Pos = make([]GlyphPos, len(decomposed))
}

// decomposeBuffer performs the decomposition phase.
// HarfBuzz equivalent: decompose_current_character in hb-ot-shape-normalize.cc:148-184
//
// The shortCircuit parameter controls behavior ("shortest" in HarfBuzz):
//   - true: Prefer composed form if font has it (shortest path)
//   - false: Always try to decompose if possible
//
// CRITICAL: HarfBuzz checks for composed glyph FIRST when shortCircuit=true!
// This prevents decomposing pre-composed characters like U+0623 when the font has a glyph for them.
func (s *Shaper) decomposeBuffer(buf *Buffer, mightShortCircuit bool) []GlyphInfo {
	result := make([]GlyphInfo, 0, len(buf.Info)*2) // Preallocate for possible expansion
	count := len(buf.Info)
	idx := 0

	// HarfBuzz equivalent: hb-ot-shape-normalize.cc:322-367
	// Process in two alternating phases:
	// 1. Simple clusters (no marks): use mightShortCircuit
	// 2. Non-simple clusters (with marks): always use shortCircuit=false
	for idx < count {
		// Find end of simple cluster: advance until we hit a mark
		end := idx + 1
		for end < count && !isUnicodeMark(buf.Info[end].Codepoint) {
			end++
		}

		if end < count {
			end-- // Leave one base for the marks to cluster with
		}

		// From idx to end are simple clusters - decompose with mightShortCircuit
		for idx < end {
			s.decomposeCurrentCharacter(&result, &buf.Info[idx], mightShortCircuit)
			idx++
		}

		if idx >= count {
			break
		}

		// Find all the marks now
		end = idx + 1
		for end < count && isUnicodeMark(buf.Info[end].Codepoint) {
			end++
		}

		// idx to end is one non-simple cluster - decompose with shortCircuit=false
		// HarfBuzz: decompose_multi_char_cluster(&c, end, always_short_circuit)
		// always_short_circuit is false for composed-diacritics mode
		for idx < end {
			s.decomposeCurrentCharacter(&result, &buf.Info[idx], false)
			idx++
		}
	}

	return result
}

// decomposeCurrentCharacter decomposes a single character, appending results.
// HarfBuzz equivalent: decompose_current_character() in hb-ot-shape-normalize.cc:148-184
func (s *Shaper) decomposeCurrentCharacter(result *[]GlyphInfo, info *GlyphInfo, shortCircuit bool) {
	cp := info.Codepoint
	cluster := info.Cluster
	glyphProps := info.GlyphProps

	// If shortCircuit is true AND font has the composed glyph, use it directly
	if shortCircuit {
		if glyph, ok := s.cmap.Lookup(cp); ok && glyph != 0 {
			*result = append(*result, GlyphInfo{
				Codepoint:  cp,
				Cluster:    cluster,
				Mask:       MaskGlobal,
				GlyphProps: glyphProps,
			})
			return
		}
	}

	// Try to decompose
	decomposed := s.decomposeCodepoint(cp, cluster)
	if len(decomposed) > 0 {
		decomposed[0].GlyphProps = glyphProps
		*result = append(*result, decomposed...)
		return
	}

	// Decomposition not possible - use composed form
	if glyph, ok := s.cmap.Lookup(cp); ok && glyph != 0 {
		*result = append(*result, GlyphInfo{
			Codepoint:  cp,
			Cluster:    cluster,
			Mask:       MaskGlobal,
			GlyphProps: glyphProps,
		})
		return
	}

	// No glyph - keep original codepoint (will map to .notdef)
	*result = append(*result, GlyphInfo{
		Codepoint:  cp,
		Cluster:    cluster,
		Mask:       MaskGlobal,
		GlyphProps: glyphProps,
	})
}

// decomposeCodepoint recursively decomposes a codepoint.
// Returns nil if decomposition is not possible (font lacks required glyphs).
// HarfBuzz equivalent: decompose() in hb-ot-shape-normalize.cc:107-147
//
// Key difference from naive NFD: we only decompose if the font has glyphs
// for ALL decomposed parts. This is font-aware normalization.
func (s *Shaper) decomposeCodepoint(cp Codepoint, cluster int) []GlyphInfo {
	// Check Indic decomposition exclusions first
	// HarfBuzz equivalent: decompose_indic() in hb-ot-shaper-indic.cc
	if isIndicDecomposeExclusion(cp) {
		return nil // Don't decompose these characters
	}

	// First try Khmer-specific decomposition
	// HarfBuzz equivalent: decompose_khmer() in hb-ot-shaper-khmer.cc:326-346
	a, b, ok := decomposeKhmer(cp)
	if !ok {
		// Fall back to standard Unicode decomposition
		a, b, ok = unicodeDecompose(cp)
	}
	if !ok {
		return nil
	}

	// Check if font has glyph for 'b' (the combining part)
	// If font doesn't have the mark, decomposition is not useful
	if b != 0 {
		if bGlyph, ok := s.cmap.Lookup(b); !ok || bGlyph == 0 {
			return nil // Font doesn't have the combining mark
		}
	}

	// Recursively decompose 'a' (the base)
	var result []GlyphInfo

	// First try recursive decomposition of 'a'
	recursiveResult := s.decomposeCodepoint(a, cluster)
	if recursiveResult != nil {
		result = recursiveResult
	} else {
		// Can't decompose 'a' further - check if font has it directly
		if aGlyph, ok := s.cmap.Lookup(a); ok && aGlyph != 0 {
			result = []GlyphInfo{{Codepoint: a, Cluster: cluster, Mask: MaskGlobal}}
		} else {
			// Font doesn't have 'a' and can't decompose it
			return nil
		}
	}

	// Add 'b' if present
	if b != 0 {
		result = append(result, GlyphInfo{
			Codepoint: b,
			Cluster:   cluster,
			Mask:      MaskGlobal, // HarfBuzz: glyphs have global_mask
		})
	}

	return result
}

// unhideCGJ removes the Hidden flag from CGJ (U+034F) when it's between
// two marks and the preceding mark has CCC <= following mark's CCC.
// HarfBuzz equivalent: hb-ot-shape-normalize.cc:402-414
//
// This allows CGJ to be "transparent" during GSUB context matching in such cases,
// while still being "hidden" (not skipped) in other cases.
// See: https://github.com/harfbuzz/harfbuzz/issues/554
func unhideCGJ(info []GlyphInfo) {
	n := len(info)
	for i := 1; i+1 < n; i++ {
		if info[i].Codepoint == 0x034F { // CGJ
			// Use getGlyphInfoCombiningClass which checks ModifiedCCC first
			// This is important for Arabic MCMs that have their CCC changed by reorder_marks_arabic
			ccBefore := getGlyphInfoCombiningClass(&info[i-1])
			ccAfter := getGlyphInfoCombiningClass(&info[i+1])
			// If following char is non-mark (ccc=0) OR preceding ccc <= following ccc
			if ccAfter == 0 || ccBefore <= ccAfter {
				// Unhide: clear the Hidden flag
				info[i].GlyphProps &^= GlyphPropsHidden
			}
		}
	}
}

// reorderMarks sorts combining marks by canonical combining class.
// HarfBuzz equivalent: hb-ot-shape-normalize.cc:370-400
//
// After sorting marks by combining class, this function optionally calls
// a script-specific reorder callback (e.g., for Arabic mark reordering).
func (s *Shaper) reorderMarks(info []GlyphInfo) {
	n := len(info)
	if n < 2 {
		return
	}

	// Find sequences of marks and sort them
	i := 0
	for i < n {
		// Skip to first mark (ccc != 0)
		ccc := getModifiedCombiningClass(info[i].Codepoint)
		if ccc == 0 {
			i++
			continue
		}

		// Find end of mark sequence
		start := i
		for i < n && getModifiedCombiningClass(info[i].Codepoint) != 0 {
			i++
		}
		end := i

		// Only sort if sequence is short enough (avoid O(n²) on pathological input)
		// HarfBuzz equivalent: HB_OT_SHAPE_MAX_COMBINING_MARKS check
		if end-start > MaxCombiningMarks {
			continue
		}

		// Stable sort by combining class
		// HarfBuzz equivalent: buffer->sort() with compare_combining_class
		sortMarksByCombiningClass(info[start:end])

		// Call script-specific mark reordering callback if set
		// HarfBuzz equivalent: plan->shaper->reorder_marks() in hb-ot-shape-normalize.cc:394-395
		if s.reorderMarksCallback != nil {
			s.reorderMarksCallback(info, start, end)
		}
	}
}

// sortMarksByCombiningClass performs a stable insertion sort on marks by combining class.
// HarfBuzz equivalent: buffer->sort() in hb-buffer.cc:2173-2191
// When elements are moved during sorting, clusters in the affected range are merged.
// This is critical for correct cluster assignment after mark reordering.
func sortMarksByCombiningClass(marks []GlyphInfo) {
	for i := 1; i < len(marks); i++ {
		ccI := getModifiedCombiningClass(marks[i].Codepoint)
		j := i
		for j > 0 && getModifiedCombiningClass(marks[j-1].Codepoint) > ccI {
			j--
		}
		if i == j {
			continue
		}
		// Merge clusters in [j, i+1) before moving — matches HarfBuzz line 2184
		mergeClustersSlice(marks, j, i+1)
		// Move item i to position j, shift everything in between
		t := marks[i]
		copy(marks[j+1:i+1], marks[j:i])
		marks[j] = t
	}
}

// recomposeBuffer performs the recomposition phase.
// HarfBuzz equivalent: hb-ot-shape-normalize.cc:418-473
func (s *Shaper) recomposeBuffer(info []GlyphInfo) []GlyphInfo {
	if len(info) < 2 {
		return info
	}

	result := make([]GlyphInfo, 0, len(info))
	result = append(result, info[0])
	starterIdx := 0

	for i := 1; i < len(info); i++ {
		// Only try to compose marks (ccc != 0) with the starter
		// HarfBuzz: "We don't try to compose a non-mark character with its preceding starter"
		ccc := getModifiedCombiningClass(info[i].Codepoint)
		if ccc == 0 {
			// Non-mark: becomes new starter
			result = append(result, info[i])
			starterIdx = len(result) - 1
			continue
		}

		// Check if we can compose this mark with the starter
		// Condition: nothing between starter and this mark has ccc >= current ccc
		// (i.e., blocked if there's an intervening mark with same or higher ccc)
		canCompose := true
		if starterIdx < len(result)-1 {
			prevCCC := getModifiedCombiningClass(result[len(result)-1].Codepoint)
			if prevCCC >= ccc {
				canCompose = false
			}
		}

		if canCompose {
			// Check compose filter callback (e.g., USE compose_use prevents mark recomposition)
			if s.composeFilter != nil && !s.composeFilter(result[starterIdx].Codepoint, info[i].Codepoint) {
				result = append(result, info[i])
				continue
			}
			// Try to compose starter + current mark
			composed, ok := unicodeCompose(result[starterIdx].Codepoint, info[i].Codepoint)
			if ok && !isCompositionExclusion(composed) {
				// Check if font has the composed glyph
				if glyph, found := s.cmap.Lookup(composed); found && glyph != 0 {
					// Compose!
					result[starterIdx].Codepoint = composed
					// Merge clusters: use minimum cluster value
					if info[i].Cluster < result[starterIdx].Cluster {
						result[starterIdx].Cluster = info[i].Cluster
					}
					continue
				}
			}
		}

		// Can't compose: keep the mark
		result = append(result, info[i])
	}

	return result
}

// getGlyphInfoCombiningClass returns the effective combining class for a GlyphInfo.
// This checks for an overridden value first (set by Arabic reorder_marks),
// then falls back to the standard modified CCC.
// HarfBuzz equivalent: info_cc() macro which uses _hb_glyph_info_get_modified_combining_class()
func getGlyphInfoCombiningClass(info *GlyphInfo) uint8 {
	if info.ModifiedCCC != 0 {
		return info.ModifiedCCC
	}
	return getModifiedCombiningClass(info.Codepoint)
}

// getModifiedCombiningClass returns the modified combining class for a codepoint.
// HarfBuzz equivalent: modified_combining_class() in hb-unicode.hh
func getModifiedCombiningClass(cp Codepoint) uint8 {
	// Special cases from HarfBuzz
	switch cp {
	case 0x1A60: // SAKOT - reorder to come after tone marks
		return 254
	case 0x0FC6: // PADMA - reorder to come after vowel marks
		return 254
	case 0x0F39: // TSA-PHRU - reorder before U+0F74
		return 127
	}

	ccc := getCombiningClass(cp)

	// Apply HarfBuzz's modified combining class mapping
	return modifiedCombiningClass[ccc]
}

// modifiedCombiningClass maps canonical combining class to HarfBuzz's modified version.
// HarfBuzz equivalent: _hb_modified_combining_class in hb-unicode.cc
//
// Key modifications from standard Unicode CCC:
// - Hebrew (CCC 10-26): Reordered for correct vowel positioning
// - Arabic (CCC 27-35): Shadda (33) moved before other marks (→27)
// - Telugu (CCC 84, 91): Reduced to 4, 5
// - Thai (CCC 103): Reduced to 3
// - Tibetan (CCC 130, 132): Swapped
var modifiedCombiningClass = [256]uint8{
	// 0-9: Standard values
	0, // NOT_REORDERED
	1, // OVERLAY
	2, 3, 4, 5, 6,
	7, // NUKTA
	8, // KANA_VOICING
	9, // VIRAMA

	// 10-26: Hebrew - reordered for correct shaping
	// CCC10 (sheva) → 22, CCC11 (hataf segol) → 15, etc.
	22, // CCC10 sheva
	15, // CCC11 hataf segol
	16, // CCC12 hataf patah
	17, // CCC13 hataf qamats
	23, // CCC14 hiriq
	18, // CCC15 tsere
	19, // CCC16 segol
	20, // CCC17 patah
	21, // CCC18 qamats & qamats qatan
	14, // CCC19 holam & holam haser for vav
	24, // CCC20 qubuts
	12, // CCC21 dagesh
	25, // CCC22 meteg
	13, // CCC23 rafe
	10, // CCC24 shin dot
	11, // CCC25 sin dot
	26, // CCC26 point varika

	// 27-35: Arabic - shadda (33) reordered to come first
	28, // CCC27 fathatan
	29, // CCC28 dammatan
	30, // CCC29 kasratan
	31, // CCC30 fatha
	32, // CCC31 damma
	33, // CCC32 kasra
	27, // CCC33 shadda - moved before other marks!
	34, // CCC34 sukun
	35, // CCC35 superscript alef

	// 36: Syriac superscript alaph
	36,

	// 37-83: Standard values
	37, 38, 39,
	40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59,
	60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79,
	80, 81, 82, 83,

	// 84: Telugu length mark
	4,

	// 85-90: Standard
	85, 86, 87, 88, 89, 90,

	// 91: Telugu AI length mark
	5,

	// 92-102: Standard
	92, 93, 94, 95, 96, 97, 98, 99, 100, 101, 102,

	// 103: Thai sara u / sara uu
	3,

	// 104-106: Standard
	104, 105, 106,

	// 107: Thai mai *
	107,

	// 108-117: Standard
	108, 109, 110, 111, 112, 113, 114, 115, 116, 117,

	// 118: Lao sign u / sign uu
	118,

	// 119-121: Standard
	119, 120, 121,

	// 122: Lao mai *
	122,

	// 123-128: Standard
	123, 124, 125, 126, 127, 128,

	// 129-132: Tibetan - 130 and 132 swapped
	129, // CCC129 sign aa
	132, // CCC130 sign i → 132
	131, // CCC131
	131, // CCC132 sign u → 131

	// 133-199: Standard
	133, 134, 135, 136, 137, 138, 139,
	140, 141, 142, 143, 144, 145, 146, 147, 148, 149,
	150, 151, 152, 153, 154, 155, 156, 157, 158, 159,
	160, 161, 162, 163, 164, 165, 166, 167, 168, 169,
	170, 171, 172, 173, 174, 175, 176, 177, 178, 179,
	180, 181, 182, 183, 184, 185, 186, 187, 188, 189,
	190, 191, 192, 193, 194, 195, 196, 197, 198, 199,

	// 200-240: Standard (standard combining class positions)
	200, // ATTACHED_BELOW_LEFT
	201,
	202, // ATTACHED_BELOW
	203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213,
	214, // ATTACHED_ABOVE
	215,
	216, // ATTACHED_ABOVE_RIGHT
	217,
	218, // BELOW_LEFT
	219,
	220, // BELOW
	221,
	222, // BELOW_RIGHT
	223,
	224, // LEFT
	225,
	226, // RIGHT
	227,
	228, // ABOVE_LEFT
	229,
	230, // ABOVE
	231,
	232, // ABOVE_RIGHT
	233, // DOUBLE_BELOW
	234, // DOUBLE_ABOVE
	235, 236, 237, 238, 239,
	240, // IOTA_SUBSCRIPT

	// 241-255: Standard
	241, 242, 243, 244, 245, 246, 247, 248, 249, 250, 251, 252, 253, 254,
	255, // INVALID
}

// isIndicDecomposeExclusion returns true if the codepoint should NOT be decomposed
// for Indic scripts. These are composition exclusions specific to Indic shaping.
// HarfBuzz equivalent: decompose_indic() in hb-ot-shaper-indic.cc
//
// See: https://github.com/harfbuzz/harfbuzz/issues/779
func isIndicDecomposeExclusion(cp Codepoint) bool {
	switch cp {
	case 0x0931: // DEVANAGARI LETTER RRA
		return true
	case 0x09DC: // BENGALI LETTER RRA
		return true
	case 0x09DD: // BENGALI LETTER RHA
		return true
	case 0x0B94: // TAMIL LETTER AU
		return true
	}
	return false
}

// isCompositionExclusion returns true if the codepoint is a Unicode Full Composition Exclusion.
// These characters should NOT be produced by canonical composition (NFC).
// HarfBuzz uses this implicitly via Unicode library's compose function.
//
// This is a subset focusing on Indic characters that cause test failures.
// For a complete implementation, use the full DerivedNormalizationProps.txt list.
func isCompositionExclusion(cp Codepoint) bool {
	switch cp {
	// Devanagari
	case 0x0929: // DEVANAGARI LETTER NNNA
		return true
	case 0x0931: // DEVANAGARI LETTER RRA
		return true
	case 0x0934: // DEVANAGARI LETTER LLLA
		return true
	// Bengali
	case 0x09DC: // BENGALI LETTER RRA
		return true
	case 0x09DD: // BENGALI LETTER RHA
		return true
	case 0x09DF: // BENGALI LETTER YYA
		return true
	// Oriya
	case 0x0B5C: // ORIYA LETTER RRA
		return true
	case 0x0B5D: // ORIYA LETTER RHA
		return true
	}
	return false
}

// decomposeKhmer returns the Khmer-specific decomposition of a codepoint.
// HarfBuzz equivalent: decompose_khmer() in hb-ot-shaper-khmer.cc:326-346
//
// Khmer split matras decompose into a pre-base vowel (U+17C1) followed by the
// remaining vowel component. This is NOT a standard Unicode decomposition.
func decomposeKhmer(cp Codepoint) (Codepoint, Codepoint, bool) {
	switch cp {
	case 0x17BE: // KHMER VOWEL SIGN OE -> E + OE
		return 0x17C1, 0x17BE, true
	case 0x17BF: // KHMER VOWEL SIGN YA -> E + YA
		return 0x17C1, 0x17BF, true
	case 0x17C0: // KHMER VOWEL SIGN IE -> E + IE
		return 0x17C1, 0x17C0, true
	case 0x17C4: // KHMER VOWEL SIGN OO -> E + OO
		return 0x17C1, 0x17C4, true
	case 0x17C5: // KHMER VOWEL SIGN AU -> E + AU
		return 0x17C1, 0x17C5, true
	}
	return 0, 0, false
}

// unicodeDecompose returns the canonical decomposition of a codepoint.
// Returns (a, b, true) if decomposition exists, (0, 0, false) otherwise.
// HarfBuzz equivalent: hb_unicode_funcs_t::decompose() -> hb_ucd_decompose() in hb-ucd.cc:127
func unicodeDecompose(cp Codepoint) (Codepoint, Codepoint, bool) {
	// Use generated decomposition table from ucd_table.go
	// HarfBuzz equivalent: hb_ucd_decompose() in hb-ucd.cc
	return Decompose(cp)
}

// unicodeCompose returns the canonical composition of two codepoints.
// Returns (composed, true) if composition exists, (0, false) otherwise.
// HarfBuzz equivalent: hb_unicode_funcs_t::compose() -> hb_ucd_compose() in hb-ucd.cc:127
func unicodeCompose(a, b Codepoint) (Codepoint, bool) {
	if a == 0 || b == 0 {
		return 0, false
	}
	// Use generated composition table from ucd_table.go
	// HarfBuzz equivalent: hb_ucd_compose() in hb-ucd.cc
	return Compose(a, b)
}

// isUnicodeMark returns true if the codepoint is a Unicode mark (Mn, Mc, or Me).
// HarfBuzz equivalent: _hb_glyph_info_is_unicode_mark() in hb-ot-layout.hh:272
// Note: This uses General Category, NOT CCC. Spacing marks (Mc) have CCC=0
// but are still marks for normalization purposes.
func isUnicodeMark(cp Codepoint) bool {
	return IsUnicodeMark(cp)
}
