package ot

// Khmer shaper implementation based on HarfBuzz's hb-ot-shaper-khmer.cc
//
// Khmer uses Indic categories but has its own syllable structure and reordering rules.
// Key differences from Indic:
// - Coeng (U+17D2) is the subscript-forming character (like halant)
// - No repha concept
// - Different vowel reordering rules

// KhmerCategory represents character categories for Khmer shaping.
// These values match HarfBuzz's hb-ot-shaper-khmer-machine.rl
type KhmerCategory uint8

const (
	K_Other        KhmerCategory = 0  // Non-Khmer characters (default)
	K_C            KhmerCategory = 1  // Consonant
	K_V            KhmerCategory = 2  // Independent vowel
	K_N            KhmerCategory = 3  // Number (not in Ragel, but we need it)
	K_H            KhmerCategory = 4  // Coeng (halant equivalent)
	K_ZWNJ         KhmerCategory = 5  // Zero-width non-joiner
	K_ZWJ          KhmerCategory = 6  // Zero-width joiner
	K_Placeholder  KhmerCategory = 10 // Placeholder
	K_DottedCircle KhmerCategory = 11 // Dotted circle
	K_Ra           KhmerCategory = 15 // Ra (special treatment for Robatic)
	K_VAbv         KhmerCategory = 20 // Vowel above
	K_VBlw         KhmerCategory = 21 // Vowel below
	K_VPre         KhmerCategory = 22 // Vowel pre (left)
	K_VPst         KhmerCategory = 23 // Vowel post (right)
	K_Robatic      KhmerCategory = 25 // Robatic consonants
	K_Xgroup       KhmerCategory = 26 // Consonant modifiers above
	K_Ygroup       KhmerCategory = 27 // Consonant modifiers post
)

// KhmerSyllableType defines syllable types for Khmer.
type KhmerSyllableType uint8

const (
	KhmerConsonantSyllable KhmerSyllableType = iota
	KhmerBrokenCluster
	KhmerNonKhmerCluster
)

// hasKhmerScript checks if the buffer contains Khmer script characters.
func (s *Shaper) hasKhmerScript(buf *Buffer) bool {
	for _, info := range buf.Info {
		if isKhmerScript(info.Codepoint) {
			return true
		}
	}
	return false
}

// isKhmerScript returns true if the codepoint is in the Khmer script range.
func isKhmerScript(cp Codepoint) bool {
	return cp >= 0x1780 && cp <= 0x17FF
}

// getKhmerCategory returns the Khmer category for a codepoint.
// Based on HarfBuzz's set_khmer_properties() which uses hb_indic_get_categories().
func getKhmerCategory(cp Codepoint) KhmerCategory {
	switch {
	// Consonants
	case cp >= 0x1780 && cp <= 0x17A2:
		// Check for Ra (special Robatic treatment)
		if cp == 0x179A { // Khmer Letter Ro
			return K_Ra
		}
		return K_C

	// Independent vowels
	case cp >= 0x17A3 && cp <= 0x17B3:
		return K_V

	// Dependent vowels - categories from HarfBuzz hb-ot-shaper-indic-table.cc
	// U+17B6: VR (vowel right)
	case cp == 0x17B6:
		return K_VPst

	// U+17B7-17BA: VA (vowel above)
	case cp == 0x17B7, cp == 0x17B8, cp == 0x17B9, cp == 0x17BA:
		return K_VAbv

	// U+17BB-17BD: VB (vowel below)
	case cp == 0x17BB, cp == 0x17BC, cp == 0x17BD:
		return K_VBlw

	// U+17BE: VA (vowel above) - NOT VPre! It gets decomposed to U+17C1 + U+17BE
	case cp == 0x17BE:
		return K_VAbv

	// U+17BF, U+17C0: VR (vowel right)
	case cp == 0x17BF, cp == 0x17C0:
		return K_VPst

	// U+17C1-17C3: VL (vowel left/pre) - the only true VPre vowels
	case cp == 0x17C1, cp == 0x17C2, cp == 0x17C3:
		return K_VPre

	// U+17C4-17C5: VR (vowel right/post)
	case cp == 0x17C4, cp == 0x17C5:
		return K_VPst

	// Various signs - based on HarfBuzz gen-indic-table.py categories
	// Xgroup: U+17C6, U+17CB, U+17CD-17D1
	case cp == 0x17C6: // Nikahit - Xgroup in HarfBuzz!
		return K_Xgroup

	// Ygroup: U+17C7, U+17C8, U+17D3, U+17DD
	case cp == 0x17C7, cp == 0x17C8: // Reahmuk (visarga), Yuukaleapintu
		return K_Ygroup

	// Robatic: U+17C9, U+17CA, U+17CC (register shifters)
	case cp == 0x17C9, cp == 0x17CA, cp == 0x17CC:
		return K_Robatic

	// Xgroup: U+17CB, U+17CD-U+17D1
	case cp == 0x17CB:
		return K_Xgroup
	case cp >= 0x17CD && cp <= 0x17D1:
		return K_Xgroup

	// Coeng (subscript-forming character)
	case cp == 0x17D2:
		return K_H

	// Ygroup: U+17D3, U+17DD
	case cp == 0x17D3, cp == 0x17DD:
		return K_Ygroup

	// U+17D9: Khmer Sign Phnaek Muan — PLACEHOLDER in HarfBuzz indic table _(GB,C)
	// HarfBuzz equivalent: hb-ot-shaper-indic-table.cc:372, set_khmer_properties()
	case cp == 0x17D9:
		return K_Placeholder

	// ZWNJ and ZWJ
	case cp == 0x200C:
		return K_ZWNJ
	case cp == 0x200D:
		return K_ZWJ

	// Numbers
	case cp >= 0x17E0 && cp <= 0x17E9:
		return K_N
	case cp >= 0x17F0 && cp <= 0x17F9:
		return K_N

	default:
		return K_Other // Non-Khmer characters (spaces, punctuation, etc.)
	}
}

// shapeKhmer applies Khmer-specific shaping to the buffer.
// HarfBuzz equivalent: _hb_ot_shaper_khmer
//
func (s *Shaper) shapeKhmer(buf *Buffer, features []Feature) {
	// Set direction to LTR for Khmer
	if buf.Direction == 0 {
		buf.Direction = DirectionLTR
	}

	// Normalize buffer: decompose split matras (U+17BE, U+17BF, U+17C0, U+17C4, U+17C5),
	// reorder marks by canonical combining class, and recompose.
	// HarfBuzz: hb-ot-shaper-khmer.cc uses HB_OT_SHAPE_NORMALIZATION_MODE_COMPOSED_DIACRITICS
	s.normalizeBuffer(buf, NormalizationModeComposedDiacritics)

	// Map codepoints to glyphs
	s.mapCodepointsToGlyphs(buf)

	// Step 3: Set glyph classes from GDEF
	s.setGlyphClasses(buf)

	// Step 4: Setup Khmer categories
	categories := s.setupKhmerCategories(buf)

	// Step 5: Find syllables and merge clusters
	hasBrokenSyllable := s.findKhmerSyllables(buf, categories)

	// Step 6: Insert dotted circles for broken clusters
	// HarfBuzz equivalent: hb_syllabic_insert_dotted_circles()
	if hasBrokenSyllable {
		s.insertKhmerDottedCircles(buf, &categories)
	}

	// Step 7: Reorder syllables (move VPre to front, handle Coeng+Ra)
	s.reorderKhmer(buf, categories)

	// Step 7: Apply GSUB features (Buffer-based preserves clusters automatically)
	s.applyKhmerGSUBFeatures(buf)

	// Step 8: Set base advances
	s.setBaseAdvances(buf)

	// Step 9: Apply GPOS features
	_, gposFeatures := s.categorizeFeatures(features)
	gposFeatures = append(gposFeatures, s.getKhmerGPOSFeatures()...)
	s.applyGPOS(buf, gposFeatures)

	// Note: Khmer uses ZeroWidthMarksNone, so NO zeroMarkWidthsByGDEF call here
	// HarfBuzz: _hb_ot_shaper_khmer has zero_width_marks = HB_OT_SHAPE_ZERO_WIDTH_MARKS_NONE
}

// setupKhmerCategories assigns Khmer categories to each glyph.
func (s *Shaper) setupKhmerCategories(buf *Buffer) []KhmerCategory {
	categories := make([]KhmerCategory, len(buf.Info))
	for i := range buf.Info {
		categories[i] = getKhmerCategory(buf.Info[i].Codepoint)
	}
	return categories
}

// findKhmerSyllables identifies syllables and merges clusters.
// HarfBuzz equivalent: setup_syllables_khmer() + find_syllables_khmer()
// Returns true if any broken clusters were found.
func (s *Shaper) findKhmerSyllables(buf *Buffer, categories []KhmerCategory) bool {
	// Use Ragel-generated syllable finder (in khmer_machine.go)
	return FindSyllablesKhmer(buf, categories)
}

// insertKhmerDottedCircles inserts dotted circle glyphs at the start of broken clusters.
// HarfBuzz equivalent: hb_syllabic_insert_dotted_circles() in hb-ot-shaper-syllabic.cc
func (s *Shaper) insertKhmerDottedCircles(buf *Buffer, categories *[]KhmerCategory) {
	// Get the glyph ID for U+25CC (DOTTED CIRCLE)
	if s.cmap == nil {
		return
	}
	dottedCircleGlyph, ok := s.cmap.Lookup(0x25CC)
	if !ok || dottedCircleGlyph == 0 {
		// Font doesn't have dotted circle glyph, skip insertion
		return
	}

	// Build new buffer with dotted circles inserted
	newInfo := make([]GlyphInfo, 0, len(buf.Info)+10)
	newPos := make([]GlyphPos, 0, len(buf.Pos)+10)
	newCategories := make([]KhmerCategory, 0, len(*categories)+10)

	lastSyllable := uint32(0)
	for i := 0; i < len(buf.Info); i++ {
		syllable := buf.Info[i].Mask & 0xFFFF
		syllableType := KhmerSyllableType(syllable & 0x0F)

		// Check if this is a new broken cluster
		if lastSyllable != syllable && syllableType == KhmerBrokenCluster {
			// Insert dotted circle before this glyph
			dottedCircleInfo := GlyphInfo{
				GlyphID:    dottedCircleGlyph,
				Codepoint:  0x25CC,
				Cluster:    buf.Info[i].Cluster,
				Mask:       buf.Info[i].Mask, // Same syllable
				GlyphClass: 1,                // Base glyph
			}
			newInfo = append(newInfo, dottedCircleInfo)
			newPos = append(newPos, GlyphPos{})
			newCategories = append(newCategories, K_DottedCircle)
		}
		lastSyllable = syllable

		newInfo = append(newInfo, buf.Info[i])
		newPos = append(newPos, buf.Pos[i])
		newCategories = append(newCategories, (*categories)[i])
	}

	buf.Info = newInfo
	buf.Pos = newPos
	*categories = newCategories
}

// reorderKhmer reorders glyphs within syllables.
// HarfBuzz equivalent: reorder_khmer() + reorder_syllable_khmer()
func (s *Shaper) reorderKhmer(buf *Buffer, categories []KhmerCategory) {
	n := len(buf.Info)
	if n == 0 {
		return
	}

	i := 0
	for i < n {
		// Find syllable boundaries by checking syllable serial
		syllable := buf.Info[i].Mask & 0xFFFF
		start := i
		end := i + 1
		for end < n && (buf.Info[end].Mask&0xFFFF) == syllable {
			end++
		}

		// Reorder within syllable
		s.reorderKhmerSyllable(buf, categories, start, end)

		i = end
	}
}

// reorderKhmerSyllable reorders a single Khmer syllable.
// HarfBuzz equivalent: reorder_consonant_syllable() in hb-ot-shaper-khmer.cc:206-279
//
// IMPORTANT: Like HarfBuzz, we only move info (not pos). Positions are set
// later in hb_ot_position_default / setBaseAdvances.
// IMPORTANT: VPre reordering is an else-if inside the same loop as Coeng+Ra,
// not a separate loop. This ensures correct ordering when both are present.
func (s *Shaper) reorderKhmerSyllable(buf *Buffer, categories []KhmerCategory, start, end int) {
	// HarfBuzz: reorder_consonant_syllable() lines 224-279
	// Single loop handles both Coeng+Ra and VPre reordering.
	numCoengs := 0
	for i := start + 1; i < end; i++ {
		if categories[i] == K_H && numCoengs <= 2 && i+1 < end {
			// Coeng found - check for Ra (Robat)
			numCoengs++

			if categories[i+1] == K_Ra {
				// Move Coeng+Ra to the start
				// HarfBuzz: buffer->merge_clusters(start, i + 2);
				buf.MergeClusters(start, i+2)

				savedInfo := [2]GlyphInfo{buf.Info[i], buf.Info[i+1]}
				savedCat := [2]KhmerCategory{categories[i], categories[i+1]}

				copy(buf.Info[start+2:i+2], buf.Info[start:i])
				copy(categories[start+2:i+2], categories[start:i])

				buf.Info[start] = savedInfo[0]
				buf.Info[start+1] = savedInfo[1]
				categories[start] = savedCat[0]
				categories[start+1] = savedCat[1]

				numCoengs = 2 // Done with Coeng+Ra
			}
		} else if categories[i] == K_VPre {
			// Reorder left matra piece - move to start
			// HarfBuzz: lines 270-278
			buf.MergeClusters(start, i+1)

			savedInfo := buf.Info[i]
			savedCat := categories[i]

			copy(buf.Info[start+1:i+1], buf.Info[start:i])
			copy(categories[start+1:i+1], categories[start:i])

			buf.Info[start] = savedInfo
			categories[start] = savedCat
		}
	}
}

// applyKhmerGSUBFeatures applies Khmer GSUB features.
// HarfBuzz equivalent: collect_features_khmer() registers features, then
// hb_ot_map_t::apply() applies them via hb-ot-layout.cc
//
// Features applied (in order):
//   - locl, ccmp: Pre-processing features
//   - pref, blwf, abvf, pstf, cfar: Basic shaping features
//   - pres, abvs, blws, psts, clig: Presentation features
//
// This Buffer-based version preserves cluster information during substitution.
func (s *Shaper) applyKhmerGSUBFeatures(buf *Buffer) {
	if s.gsub == nil {
		return
	}

	// Pre-processing features
	preFeatures := []Tag{
		MakeTag('l', 'o', 'c', 'l'),
		MakeTag('c', 'c', 'm', 'p'),
	}

	// Basic features (applied per-syllable in HarfBuzz, but we apply globally for simplicity)
	basicFeatures := []Tag{
		MakeTag('p', 'r', 'e', 'f'), // Pre-base forms (Coeng+Ra)
		MakeTag('b', 'l', 'w', 'f'), // Below-base forms
		MakeTag('a', 'b', 'v', 'f'), // Above-base forms
		MakeTag('p', 's', 't', 'f'), // Post-base forms
		MakeTag('c', 'f', 'a', 'r'), // Conjunct form after Robatic
	}

	// Other features
	otherFeatures := []Tag{
		MakeTag('p', 'r', 'e', 's'), // Pre-base substitutions
		MakeTag('a', 'b', 'v', 's'), // Above-base substitutions
		MakeTag('b', 'l', 'w', 's'), // Below-base substitutions
		MakeTag('p', 's', 't', 's'), // Post-base substitutions
		MakeTag('c', 'l', 'i', 'g'), // Contextual ligatures
	}

	// Apply all features using Buffer-based method (preserves clusters)
	allFeatures := append(preFeatures, basicFeatures...)
	allFeatures = append(allFeatures, otherFeatures...)

	for _, feature := range allFeatures {
		s.gsub.ApplyFeatureToBuffer(feature, buf, s.gdef, s.font)
	}
}

// getKhmerGPOSFeatures returns GPOS features for Khmer.
func (s *Shaper) getKhmerGPOSFeatures() []Feature {
	return []Feature{
		{Tag: MakeTag('d', 'i', 's', 't'), Value: 1}, // Distances
		{Tag: MakeTag('a', 'b', 'v', 'm'), Value: 1}, // Above-base Mark Positioning
		{Tag: MakeTag('b', 'l', 'w', 'm'), Value: 1}, // Below-base Mark Positioning
		{Tag: MakeTag('m', 'a', 'r', 'k'), Value: 1}, // Mark Positioning
		{Tag: MakeTag('m', 'k', 'm', 'k'), Value: 1}, // Mark-to-Mark Positioning
	}
}
