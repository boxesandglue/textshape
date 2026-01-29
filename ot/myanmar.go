package ot

// Myanmar shaper implementation based on HarfBuzz's hb-ot-shaper-myanmar.cc
//
// HarfBuzz equivalent: hb-ot-shaper-myanmar.cc
//
// Myanmar uses Indic categories from the indic table but has its own
// syllable structure and reordering rules.

// MyanmarCategory represents character categories for Myanmar shaping.
// These values match HarfBuzz's hb-ot-shaper-myanmar-machine.rl exports.
// HarfBuzz equivalent: myanmar_category_t
type MyanmarCategory uint8

const (
	M_Other        MyanmarCategory = 0  // Non-Myanmar characters
	M_C            MyanmarCategory = 1  // Consonant
	M_IV           MyanmarCategory = 2  // Independent vowel
	M_DB           MyanmarCategory = 3  // Dot below
	M_H            MyanmarCategory = 4  // Halant (Myanmar virama U+1039)
	M_ZWNJ         MyanmarCategory = 5  // Zero-width non-joiner
	M_ZWJ          MyanmarCategory = 6  // Zero-width joiner
	M_SM           MyanmarCategory = 8  // Visarga and Shan tones
	M_A            MyanmarCategory = 9  // Anusvara
	M_GB           MyanmarCategory = 10 // Placeholder (generic base)
	M_DottedCircle MyanmarCategory = 11 // Dotted circle
	M_Ra           MyanmarCategory = 15 // Myanmar Ra (U+101B)
	M_CS           MyanmarCategory = 18 // Consonant preceding Kinzi
	M_VAbv         MyanmarCategory = 20 // Vowel above
	M_VBlw         MyanmarCategory = 21 // Vowel below
	M_VPre         MyanmarCategory = 22 // Vowel pre (left)
	M_VPst         MyanmarCategory = 23 // Vowel post (right)
	M_As           MyanmarCategory = 32 // Asat (U+103A)
	M_MH           MyanmarCategory = 35 // Medial Ha (U+103E)
	M_MR           MyanmarCategory = 36 // Medial Ra (U+103C)
	M_MW           MyanmarCategory = 37 // Medial Wa, Shan Wa (U+103D, U+1082)
	M_MY           MyanmarCategory = 38 // Medial Ya, Mon Na, Mon Ma (U+103B, U+105E, U+105F)
	M_PT           MyanmarCategory = 39 // Pwo and other tones
	M_VS           MyanmarCategory = 40 // Variation selectors
	M_ML           MyanmarCategory = 41 // Medial Mon La (U+1060)
	M_SMPst        MyanmarCategory = 57 // Post-syllable SM
)

// MyanmarSyllableType defines syllable types for Myanmar.
// HarfBuzz equivalent: myanmar_syllable_type_t
type MyanmarSyllableType uint8

const (
	MyanmarConsonantSyllable  MyanmarSyllableType = 0
	MyanmarBrokenCluster      MyanmarSyllableType = 1
	MyanmarNonMyanmarCluster  MyanmarSyllableType = 2
)

// MyanmarPosition represents visual positions in a syllable.
// HarfBuzz equivalent: ot_position_t in hb-ot-shaper-indic.hh
type MyanmarPosition uint8

const (
	M_POS_START          MyanmarPosition = 0
	M_POS_RA_TO_BECOME_REPH MyanmarPosition = 1
	M_POS_PRE_M          MyanmarPosition = 2
	M_POS_PRE_C          MyanmarPosition = 3
	M_POS_BASE_C         MyanmarPosition = 4
	M_POS_AFTER_MAIN     MyanmarPosition = 5
	M_POS_ABOVE_C        MyanmarPosition = 6
	M_POS_BEFORE_SUB     MyanmarPosition = 7
	M_POS_BELOW_C        MyanmarPosition = 8
	M_POS_AFTER_SUB      MyanmarPosition = 9
	M_POS_BEFORE_POST    MyanmarPosition = 10
	M_POS_POST_C         MyanmarPosition = 11
	M_POS_AFTER_POST     MyanmarPosition = 12
	M_POS_SMVD           MyanmarPosition = 13
	M_POS_END            MyanmarPosition = 14
)

// getMyanmarCategory returns the Myanmar category for a codepoint.
// HarfBuzz equivalent: set_myanmar_properties() which uses hb_indic_get_categories()
//
// This function maps IndicCategory values to MyanmarCategory values as HarfBuzz does.
func getMyanmarCategory(cp Codepoint) MyanmarCategory {
	// Check for Variation Selectors first
	// HarfBuzz: myanmar_machine.rl uses VS category
	if IsVariationSelector(cp) {
		return M_VS
	}

	// Use the indic table like HarfBuzz does
	cat, _ := GetIndicCategories(cp)

	// Map Indic categories to Myanmar categories
	// HarfBuzz: info.myanmar_category() = (myanmar_category_t) (type & 0xFFu)
	switch cat {
	case ICatC:
		// Check for Ra (U+101B) - special handling
		if cp == 0x101B {
			return M_Ra
		}
		return M_C
	case ICatV:
		return M_IV
	case ICatN:
		return M_DB // Nukta/Dot below
	case ICatH:
		return M_H
	case ICatZWNJ:
		return M_ZWNJ
	case ICatZWJ:
		return M_ZWJ
	case ICatM:
		// Matras - need to determine VAbv, VBlw, VPre, VPst from position
		return getMyanmarMatraCategory(cp)
	case ICatSM:
		return M_SM
	case ICatSMPst:
		return M_SMPst
	case ICatA:
		return M_A
	case ICatPLACEHOLDER:
		return M_GB
	case ICatDOTTEDCIRCLE:
		return M_DottedCircle
	case ICatRa:
		return M_Ra
	case ICatCS:
		return M_CS
	case ICatSymbol:
		return M_GB
	case ICatCM:
		// Consonant modifiers - check Myanmar-specific ones
		return getMyanmarModifierCategory(cp)
	default:
		return M_Other
	}
}

// getMyanmarMatraCategory determines the specific vowel category for Myanmar matras.
// HarfBuzz uses position from indic table to determine this.
func getMyanmarMatraCategory(cp Codepoint) MyanmarCategory {
	_, pos := GetIndicCategories(cp)
	switch pos {
	case IPosPreM:
		return M_VPre
	case IPosAboveC:
		return M_VAbv
	case IPosBelowC:
		return M_VBlw
	case IPosAfterPost, IPosAfterMain, IPosAfterSub:
		return M_VPst
	default:
		return M_VPst
	}
}

// getMyanmarModifierCategory determines the category for Myanmar consonant modifiers.
// HarfBuzz: These are Myanmar-specific categories (MY, MR, MW, MH, ML, As, PT, VS)
func getMyanmarModifierCategory(cp Codepoint) MyanmarCategory {
	switch cp {
	// Medial Ya (U+103B), Mon Na (U+105E), Mon Ma (U+105F)
	case 0x103B, 0x105E, 0x105F:
		return M_MY
	// Medial Ra (U+103C)
	case 0x103C:
		return M_MR
	// Medial Wa (U+103D), Shan Wa (U+1082)
	case 0x103D, 0x1082:
		return M_MW
	// Medial Ha (U+103E)
	case 0x103E:
		return M_MH
	// Medial Mon La (U+1060)
	case 0x1060:
		return M_ML
	// Asat (U+103A)
	case 0x103A:
		return M_As
	// Pwo and other tones
	case 0x1063, 0x1064, 0x1069, 0x106A, 0x106B, 0x106C, 0x106D,
		0x1087, 0x1088, 0x1089, 0x108A, 0x108B, 0x108C, 0x108D, 0x108F,
		0x109A, 0x109B, 0x109C:
		return M_PT
	default:
		return M_C // Default to consonant
	}
}

// setMyanmarProperties sets Myanmar category on each glyph.
// HarfBuzz equivalent: set_myanmar_properties() in hb-ot-shaper-myanmar.cc:67-74
func setMyanmarProperties(info *GlyphInfo) {
	info.MyanmarCategory = uint8(getMyanmarCategory(info.Codepoint))
}

// Consonant flags for Myanmar
// HarfBuzz: CONSONANT_FLAGS_MYANMAR in hb-ot-shaper-myanmar.cc:92
const consonantFlagsMyanmar = (1 << M_C) | (1 << M_CS) | (1 << M_Ra) | (1 << M_IV) | (1 << M_GB) | (1 << M_DottedCircle)

// isConsonantMyanmar checks if a glyph is a consonant-like character.
// HarfBuzz equivalent: is_consonant_myanmar() in hb-ot-shaper-myanmar.cc:94-98
func isConsonantMyanmar(info *GlyphInfo) bool {
	// If it ligated, all bets are off
	if info.GlyphProps&GlyphPropsLigated != 0 {
		return false
	}
	cat := MyanmarCategory(info.MyanmarCategory)
	return (1<<cat)&consonantFlagsMyanmar != 0
}

// setupMyanmarCategories assigns Myanmar categories to each glyph.
// HarfBuzz equivalent: setup_masks_myanmar() in hb-ot-shaper-myanmar.cc:137-151
func (s *Shaper) setupMyanmarCategories(buf *Buffer) []MyanmarCategory {
	categories := make([]MyanmarCategory, len(buf.Info))
	for i := range buf.Info {
		setMyanmarProperties(&buf.Info[i])
		categories[i] = MyanmarCategory(buf.Info[i].MyanmarCategory)
	}
	return categories
}

// setupSyllablesMyanmar finds syllables and marks unsafe-to-break regions.
// HarfBuzz equivalent: setup_syllables_myanmar() in hb-ot-shaper-myanmar.cc:153-163
func (s *Shaper) setupSyllablesMyanmar(buf *Buffer, categories []MyanmarCategory) bool {
	hasBroken := FindSyllablesMyanmar(buf, categories)

	// Mark syllables as unsafe to break
	// HarfBuzz: foreach_syllable (buffer, start, end) buffer->unsafe_to_break (start, end);
	n := len(buf.Info)
	if n == 0 {
		return hasBroken
	}

	i := 0
	for i < n {
		syllable := buf.Info[i].Syllable
		start := i
		end := i + 1
		for end < n && buf.Info[end].Syllable == syllable {
			end++
		}
		// Merge clusters within syllable
		if end > start+1 {
			buf.MergeClusters(start, end)
		}
		i = end
	}

	return hasBroken
}

// compareMyanmarOrder compares two glyphs by their Myanmar position for sorting.
// HarfBuzz equivalent: compare_myanmar_order() in hb-ot-shaper-myanmar.cc:165-172
func compareMyanmarOrder(a, b *GlyphInfo) int {
	return int(a.MyanmarPosition) - int(b.MyanmarPosition)
}

// initialReorderingConsonantSyllableMyanmar performs initial reordering of a Myanmar consonant syllable.
// HarfBuzz equivalent: initial_reordering_consonant_syllable() in hb-ot-shaper-myanmar.cc:178-301
func (s *Shaper) initialReorderingConsonantSyllableMyanmar(buf *Buffer, start, end int) {
	info := buf.Info

	base := end
	hasReph := false

	// Find Kinzi (Ra + As + H)
	// HarfBuzz: lines 187-197
	limit := start
	if start+3 <= end &&
		MyanmarCategory(info[start].MyanmarCategory) == M_Ra &&
		MyanmarCategory(info[start+1].MyanmarCategory) == M_As &&
		MyanmarCategory(info[start+2].MyanmarCategory) == M_H {
		limit += 3
		base = start
		hasReph = true
	}

	// Find base consonant
	// HarfBuzz: lines 199-209
	if !hasReph {
		base = limit
	}
	for i := limit; i < end; i++ {
		if isConsonantMyanmar(&info[i]) {
			base = i
			break
		}
	}

	// Assign positions
	// HarfBuzz: lines 213-269
	i := start
	// Kinzi gets AFTER_MAIN
	for ; i < start+ternary(hasReph, 3, 0); i++ {
		info[i].MyanmarPosition = uint8(M_POS_AFTER_MAIN)
	}
	// Pre-base consonants
	for ; i < base; i++ {
		info[i].MyanmarPosition = uint8(M_POS_PRE_C)
	}
	// Base consonant
	if i < end {
		info[i].MyanmarPosition = uint8(M_POS_BASE_C)
		i++
	}

	// Post-base reordering
	// HarfBuzz: lines 224-269
	pos := M_POS_AFTER_MAIN
	for ; i < end; i++ {
		cat := MyanmarCategory(info[i].MyanmarCategory)

		if cat == M_MR { // Pre-base reordering
			info[i].MyanmarPosition = uint8(M_POS_PRE_C)
			continue
		}
		if cat == M_VPre { // Left matra
			info[i].MyanmarPosition = uint8(M_POS_PRE_M)
			continue
		}
		if cat == M_VS {
			if i > start {
				info[i].MyanmarPosition = info[i-1].MyanmarPosition
			}
			continue
		}

		if pos == M_POS_AFTER_MAIN && cat == M_VBlw {
			pos = M_POS_BELOW_C
			info[i].MyanmarPosition = uint8(pos)
			continue
		}

		if pos == M_POS_BELOW_C && cat == M_A {
			info[i].MyanmarPosition = uint8(M_POS_BEFORE_SUB)
			continue
		}
		if pos == M_POS_BELOW_C && cat == M_VBlw {
			info[i].MyanmarPosition = uint8(pos)
			continue
		}
		if pos == M_POS_BELOW_C && cat != M_A {
			pos = M_POS_AFTER_SUB
			info[i].MyanmarPosition = uint8(pos)
			continue
		}
		info[i].MyanmarPosition = uint8(pos)
	}

	// Sort by position
	// HarfBuzz: buffer->sort (start, end, compare_myanmar_order);
	s.sortMyanmarSyllable(buf, start, end)

	// Flip left-matra sequence
	// HarfBuzz: lines 276-300
	firstLeftMatra := end
	lastLeftMatra := end
	for i := start; i < end; i++ {
		if MyanmarPosition(info[i].MyanmarPosition) == M_POS_PRE_M {
			if firstLeftMatra == end {
				firstLeftMatra = i
			}
			lastLeftMatra = i
		}
	}

	// https://github.com/harfbuzz/harfbuzz/issues/3863
	if firstLeftMatra < lastLeftMatra {
		// Reverse the left-matra sequence
		buf.ReverseRange(firstLeftMatra, lastLeftMatra+1)
		// Reverse back VS, etc.
		i := firstLeftMatra
		for j := i; j <= lastLeftMatra; j++ {
			if MyanmarCategory(info[j].MyanmarCategory) == M_VPre {
				buf.ReverseRange(i, j+1)
				i = j + 1
			}
		}
	}
}

// sortMyanmarSyllable sorts glyphs within a syllable by Myanmar position.
// HarfBuzz equivalent: buffer->sort() with compare_myanmar_order
func (s *Shaper) sortMyanmarSyllable(buf *Buffer, start, end int) {
	if end-start < 2 {
		return
	}

	// Stable sort by position
	// Use insertion sort for stability (small arrays)
	for i := start + 1; i < end; i++ {
		j := i
		for j > start && compareMyanmarOrder(&buf.Info[j-1], &buf.Info[j]) > 0 {
			// Swap
			buf.Info[j-1], buf.Info[j] = buf.Info[j], buf.Info[j-1]
			buf.Pos[j-1], buf.Pos[j] = buf.Pos[j], buf.Pos[j-1]
			j--
		}
	}
}

// reorderSyllableMyanmar reorders a single Myanmar syllable.
// HarfBuzz equivalent: reorder_syllable_myanmar() in hb-ot-shaper-myanmar.cc:303-320
func (s *Shaper) reorderSyllableMyanmar(buf *Buffer, start, end int) {
	syllableType := MyanmarSyllableType(buf.Info[start].Syllable & 0x0F)
	switch syllableType {
	case MyanmarBrokenCluster, MyanmarConsonantSyllable:
		s.initialReorderingConsonantSyllableMyanmar(buf, start, end)
	case MyanmarNonMyanmarCluster:
		// Nothing to do
	}
}

// insertMyanmarDottedCircles inserts dotted circle glyphs at the start of broken clusters.
// HarfBuzz equivalent: hb_syllabic_insert_dotted_circles() called from reorder_myanmar()
func (s *Shaper) insertMyanmarDottedCircles(buf *Buffer, categories *[]MyanmarCategory) {
	if s.cmap == nil {
		return
	}
	dottedCircleGlyph, ok := s.cmap.Lookup(0x25CC)
	if !ok || dottedCircleGlyph == 0 {
		return
	}

	// Build new buffer with dotted circles inserted
	newInfo := make([]GlyphInfo, 0, len(buf.Info)+10)
	newPos := make([]GlyphPos, 0, len(buf.Pos)+10)
	newCategories := make([]MyanmarCategory, 0, len(*categories)+10)

	lastSyllable := uint8(0)
	for i := 0; i < len(buf.Info); i++ {
		syllable := buf.Info[i].Syllable
		syllableType := MyanmarSyllableType(syllable & 0x0F)

		// Check if this is a new broken cluster
		if lastSyllable != syllable && syllableType == MyanmarBrokenCluster {
			// Insert dotted circle before this glyph
			dottedCircleInfo := GlyphInfo{
				GlyphID:          dottedCircleGlyph,
				Codepoint:        0x25CC,
				Cluster:          buf.Info[i].Cluster,
				Syllable:         syllable,
				MyanmarCategory:  uint8(M_DottedCircle),
				MyanmarPosition:  uint8(M_POS_BASE_C),
				GlyphClass:       1,
			}
			newInfo = append(newInfo, dottedCircleInfo)
			newPos = append(newPos, GlyphPos{})
			newCategories = append(newCategories, M_DottedCircle)
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

// reorderMyanmar performs Myanmar reordering.
// HarfBuzz equivalent: reorder_myanmar() in hb-ot-shaper-myanmar.cc:322-344
func (s *Shaper) reorderMyanmar(buf *Buffer, categories *[]MyanmarCategory) bool {
	ret := false

	// Insert dotted circles for broken clusters
	// HarfBuzz: hb_syllabic_insert_dotted_circles()
	hasBroken := false
	for _, info := range buf.Info {
		if MyanmarSyllableType(info.Syllable&0x0F) == MyanmarBrokenCluster {
			hasBroken = true
			break
		}
	}
	if hasBroken {
		s.insertMyanmarDottedCircles(buf, categories)
		ret = true
	}

	// Reorder each syllable
	// HarfBuzz: foreach_syllable (buffer, start, end)
	n := len(buf.Info)
	i := 0
	for i < n {
		syllable := buf.Info[i].Syllable
		start := i
		end := i + 1
		for end < n && buf.Info[end].Syllable == syllable {
			end++
		}
		s.reorderSyllableMyanmar(buf, start, end)
		i = end
	}

	return ret
}

// applyMyanmarGSUBFeatures applies Myanmar GSUB features.
// HarfBuzz equivalent: collect_features_myanmar() + feature application
func (s *Shaper) applyMyanmarGSUBFeatures(buf *Buffer) {
	if s.gsub == nil {
		return
	}

	// Pre-processing features (before reordering pause)
	preFeatures := []Tag{
		MakeTag('l', 'o', 'c', 'l'),
		MakeTag('c', 'c', 'm', 'p'),
	}

	// Basic features (applied per-syllable)
	// HarfBuzz: myanmar_basic_features in hb-ot-shaper-myanmar.cc:41-53
	basicFeatures := []Tag{
		MakeTag('r', 'p', 'h', 'f'), // Reph forms
		MakeTag('p', 'r', 'e', 'f'), // Pre-base forms
		MakeTag('b', 'l', 'w', 'f'), // Below-base forms
		MakeTag('p', 's', 't', 'f'), // Post-base forms
	}

	// Other features (applied globally after clearing syllables)
	// HarfBuzz: myanmar_other_features in hb-ot-shaper-myanmar.cc:54-65
	otherFeatures := []Tag{
		MakeTag('p', 'r', 'e', 's'), // Pre-base substitutions
		MakeTag('a', 'b', 'v', 's'), // Above-base substitutions
		MakeTag('b', 'l', 'w', 's'), // Below-base substitutions
		MakeTag('p', 's', 't', 's'), // Post-base substitutions
	}

	// Apply features
	for _, feature := range preFeatures {
		s.gsub.ApplyFeatureToBuffer(feature, buf, s.gdef, s.font)
	}
	for _, feature := range basicFeatures {
		s.gsub.ApplyFeatureToBuffer(feature, buf, s.gdef, s.font)
	}
	for _, feature := range otherFeatures {
		s.gsub.ApplyFeatureToBuffer(feature, buf, s.gdef, s.font)
	}
}

// getMyanmarGPOSFeatures returns GPOS features for Myanmar.
func (s *Shaper) getMyanmarGPOSFeatures() []Feature {
	return []Feature{
		{Tag: MakeTag('d', 'i', 's', 't'), Value: 1}, // Distances
		{Tag: MakeTag('a', 'b', 'v', 'm'), Value: 1}, // Above-base Mark Positioning
		{Tag: MakeTag('b', 'l', 'w', 'm'), Value: 1}, // Below-base Mark Positioning
		{Tag: MakeTag('m', 'a', 'r', 'k'), Value: 1}, // Mark Positioning
		{Tag: MakeTag('m', 'k', 'm', 'k'), Value: 1}, // Mark-to-Mark Positioning
	}
}

// shapeMyanmar applies Myanmar-specific shaping to the buffer.
// HarfBuzz equivalent: _hb_ot_shaper_myanmar
func (s *Shaper) shapeMyanmar(buf *Buffer, features []Feature) {
	// Set direction to LTR for Myanmar
	if buf.Direction == 0 {
		buf.Direction = DirectionLTR
	}

	// Step 1: Normalize Unicode (decompose, reorder marks, recompose)
	// HarfBuzz: _hb_ot_shape_normalize() in hb-ot-shape.cc
	// Myanmar uses NormalizationModeComposedDiacritics (no short circuit)
	s.normalizeBuffer(buf, NormalizationModeComposedDiacritics)

	// Step 2: Initialize masks: all glyphs get MaskGlobal
	buf.ResetMasks(MaskGlobal)

	// Step 3: Map codepoints to glyphs (after normalization, handles VS)
	s.mapCodepointsToGlyphs(buf)

	// Step 4: Set glyph classes from GDEF
	s.setGlyphClasses(buf)

	// Step 5: Setup Myanmar categories
	categories := s.setupMyanmarCategories(buf)

	// Step 6: Find syllables
	s.setupSyllablesMyanmar(buf, categories)

	// Step 7: Apply locl and ccmp (before reordering)
	if s.gsub != nil {
		s.gsub.ApplyFeatureToBuffer(MakeTag('l', 'o', 'c', 'l'), buf, s.gdef, s.font)
		s.gsub.ApplyFeatureToBuffer(MakeTag('c', 'c', 'm', 'p'), buf, s.gdef, s.font)
	}

	// Step 8: Reorder syllables
	s.reorderMyanmar(buf, &categories)

	// Step 9: Apply basic and other GSUB features
	if s.gsub != nil {
		// Basic features (per syllable with pauses in HarfBuzz)
		basicFeatures := []Tag{
			MakeTag('r', 'p', 'h', 'f'),
			MakeTag('p', 'r', 'e', 'f'),
			MakeTag('b', 'l', 'w', 'f'),
			MakeTag('p', 's', 't', 'f'),
		}
		for _, feature := range basicFeatures {
			s.gsub.ApplyFeatureToBuffer(feature, buf, s.gdef, s.font)
		}

		// Clear syllable info (HarfBuzz: hb_syllabic_clear_var)
		for i := range buf.Info {
			buf.Info[i].Syllable = 0
		}

		// Other features
		otherFeatures := []Tag{
			MakeTag('p', 'r', 'e', 's'),
			MakeTag('a', 'b', 'v', 's'),
			MakeTag('b', 'l', 'w', 's'),
			MakeTag('p', 's', 't', 's'),
		}
		for _, feature := range otherFeatures {
			s.gsub.ApplyFeatureToBuffer(feature, buf, s.gdef, s.font)
		}

		// Apply default GSUB features (liga, calt, clig, rclt, rlig)
		// HarfBuzz: common_features[] and horizontal_features[] in hb-ot-shape.cc:295-318
		// These are global features applied after script-specific features
		defaultFeatures := s.getDefaultGSUBFeatures(buf.Direction)
		for _, f := range defaultFeatures {
			s.gsub.ApplyFeatureToBuffer(f.Tag, buf, s.gdef, s.font)
		}
	}

	// Step 10: Set base advances
	s.setBaseAdvances(buf)

	// Step 11: Apply GPOS features
	_, gposFeatures := s.categorizeFeatures(features)
	gposFeatures = append(gposFeatures, s.getMyanmarGPOSFeatures()...)
	s.applyGPOS(buf, gposFeatures)

	// Step 12: Zero mark widths by GDEF (EARLY for Myanmar)
	// HarfBuzz: zero_width_marks = HB_OT_SHAPE_ZERO_WIDTH_MARKS_BY_GDEF_EARLY
	s.zeroMarkWidthsByGDEF(buf)
}

// Helper function
func ternary(cond bool, a, b int) int {
	if cond {
		return a
	}
	return b
}
