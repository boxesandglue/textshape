package ot

// USE Shaper - Universal Shaping Engine implementation
// HarfBuzz equivalent: hb-ot-shaper-use.cc
//
// The Universal Shaping Engine (USE) is Microsoft's specification for
// shaping complex scripts that don't have dedicated shapers.
// It handles scripts like Khmer, Tibetan, Javanese, Balinese, etc.
//
// Reference: https://docs.microsoft.com/en-us/typography/script-development/use
//
// Like HarfBuzz, USE category is stored directly on GlyphInfo.USECategory
// so it survives GSUB operations automatically.

// useSyllableAccessor implements SyllableAccessor for USE shaper.
type useSyllableAccessor struct {
	buf *Buffer
}

func (a *useSyllableAccessor) GetSyllable(i int) uint8 {
	return a.buf.Info[i].Syllable
}

func (a *useSyllableAccessor) GetCategory(i int) uint8 {
	return a.buf.Info[i].USECategory
}

func (a *useSyllableAccessor) SetCategory(i int, cat uint8) {
	a.buf.Info[i].USECategory = cat
}

func (a *useSyllableAccessor) Len() int {
	return len(a.buf.Info)
}

// USE Features - applied in order
// HarfBuzz equivalent: use_basic_features[], use_topographical_features[], use_other_features[]
var (
	// Basic features - applied before reordering
	// HarfBuzz: F_MANUAL_ZWJ | F_PER_SYLLABLE
	useBasicFeatures = []Tag{
		MakeTag('r', 'k', 'r', 'f'), // Rakar Forms
		MakeTag('a', 'b', 'v', 'f'), // Above-base Forms
		MakeTag('b', 'l', 'w', 'f'), // Below-base Forms
		MakeTag('h', 'a', 'l', 'f'), // Half Forms
		MakeTag('p', 's', 't', 'f'), // Post-base Forms
		MakeTag('v', 'a', 't', 'u'), // Vattu Variants
		MakeTag('c', 'j', 'c', 't'), // Conjunct Forms
	}

	// Topographical features - applied for positional forms (like Arabic)
	useTopographicalFeatures = []Tag{
		MakeTag('i', 's', 'o', 'l'), // Isolated Forms
		MakeTag('i', 'n', 'i', 't'), // Initial Forms
		MakeTag('m', 'e', 'd', 'i'), // Medial Forms
		MakeTag('f', 'i', 'n', 'a'), // Final Forms
	}

	// Other features - applied after reordering
	// HarfBuzz: F_MANUAL_ZWJ (NOT per-syllable)
	useOtherFeatures = []Tag{
		MakeTag('a', 'b', 'v', 's'), // Above-base Substitutions
		MakeTag('b', 'l', 'w', 's'), // Below-base Substitutions
		MakeTag('h', 'a', 'l', 'n'), // Halant Forms
		MakeTag('p', 'r', 'e', 's'), // Pre-base Substitutions
		MakeTag('p', 's', 't', 's'), // Post-base Substitutions
		MakeTag('r', 'l', 'i', 'g'), // Required Ligatures (common feature, HarfBuzz common_features[])
	}

	// Horizontal features - applied after other features (like all scripts)
	// HarfBuzz equivalent: horizontal_features[] in hb-ot-shape.cc:309-319
	useHorizontalFeatures = []Tag{
		MakeTag('c', 'a', 'l', 't'), // Contextual Alternates
		MakeTag('c', 'l', 'i', 'g'), // Contextual Ligatures
		MakeTag('l', 'i', 'g', 'a'), // Standard Ligatures
		MakeTag('r', 'c', 'l', 't'), // Required Contextual Alternates
	}

	// Repha feature
	useRphfFeature = MakeTag('r', 'p', 'h', 'f')
	// Pre-base feature
	usePrefFeature = MakeTag('p', 'r', 'e', 'f')
)

// JoiningForm represents the joining form for topographical features.
type JoiningForm uint8

const (
	JoiningFormIsol JoiningForm = iota
	JoiningFormInit
	JoiningFormMedi
	JoiningFormFina
	JoiningFormNone
)

// shapeUSE applies USE shaping to the buffer.
// HarfBuzz equivalent: _hb_ot_shaper_use in hb-ot-shaper-use.cc
func (s *Shaper) shapeUSE(buf *Buffer, features []Feature) {
	// Set direction based on script if not already set
	if buf.Direction == 0 {
		buf.Direction = DirectionLTR
	}

	// Step 0: Preprocess vowel constraints (insert dotted circles)
	// HarfBuzz: _hb_preprocess_text_vowel_constraints()
	PreprocessVowelConstraints(buf)

	// Step 1: Normalize Unicode
	// HarfBuzz: compose_use() prevents recomposition when 'a' is a mark
	s.composeFilter = composeUSE
	s.normalizeBuffer(buf, NormalizationModeComposedDiacritics)
	s.composeFilter = nil

	// Step 1.5: Initialize masks
	buf.ResetMasks(MaskGlobal)

	// Step 1.6: Setup Arabic-like joining masks
	// HarfBuzz: setup_masks_use() calling setup_masks_arabic_plan()
	useArabicJoining := hasArabicJoining(buf.Script)
	if useArabicJoining {
		s.setupMasksArabicPlan(buf)
	}

	// Step 2: Map codepoints to glyphs
	s.mapCodepointsToGlyphs(buf)

	// Step 2b: Set glyph classes from GDEF (or synthesize from Unicode)
	// HarfBuzz: this is done in the main shaping pipeline (hb-ot-shape.cc)
	// and is needed for zeroMarkWidthsByGDEF to correctly identify marks.
	s.setGlyphClasses(buf)

	// Step 3: Setup USE categories on GlyphInfo
	// HarfBuzz: setup_masks_use() -> info.use_category() = data[cp]
	s.setupUSECategories(buf)

	// Step 4: Find syllables (uses Ragel state machine)
	// HarfBuzz equivalent: setup_syllables_use() GSUB pause
	// In HarfBuzz, setup_syllables_use() does: find_syllables + setup_rphf_mask + setup_topographical_masks
	hasBroken := s.findUSESyllables(buf)

	// Step 4b: Setup rphf mask (before pre-processing, like HarfBuzz)
	// HarfBuzz: setup_rphf_mask() called inside setup_syllables_use()
	s.setupRphfMask(buf)

	// Step 4c: Setup topographical masks (before pre-processing, like HarfBuzz)
	// HarfBuzz: setup_topographical_masks() called inside setup_syllables_use()
	if !useArabicJoining {
		s.setupTopographicalMasks(buf)
	}

	// Step 5: Apply pre-processing features (locl, ccmp, nukt, akhn)
	// HarfBuzz: F_PER_SYLLABLE
	s.applyUSEPreProcessingFeatures(buf)

	// Step 8: Apply rphf and record results
	// HarfBuzz: _hb_clear_substitution_flags -> rphf -> record_rphf_use
	s.applyRphfUSE(buf)

	// Step 9: Apply pref and record results
	// HarfBuzz: _hb_clear_substitution_flags -> pref -> record_pref_use
	s.applyPrefUSE(buf)

	// Step 10: Apply basic features (rkrf, abvf, blwf, half, pstf, vatu, cjct)
	// HarfBuzz: F_MANUAL_ZWJ | F_PER_SYLLABLE
	s.applyUSEBasicFeatures(buf)

	// Step 11: Reorder syllables (includes dotted circle insertion)
	// HarfBuzz equivalent: reorder_use() GSUB pause callback
	// In HarfBuzz, reorder_use() first inserts dotted circles, then reorders.
	s.reorderUSE(buf, hasBroken)

	// Step 12: Apply other features + horizontal features
	// HarfBuzz: topographical features + use_other_features
	s.applyUSEOtherFeatures(buf)

	// Step 14: Set base advances
	s.setBaseAdvances(buf)

	// Step 15: Zero mark widths (EARLY mode - before GPOS!)
	// HarfBuzz: zero_width_marks = HB_OT_SHAPE_ZERO_WIDTH_MARKS_BY_GDEF_EARLY
	s.zeroMarkWidthsByGDEFEarly(buf)

	// Step 16: Apply GPOS features
	_, gposFeatures := s.categorizeFeatures(features)
	gposFeatures = append(gposFeatures, s.getUSEGPOSFeatures()...)
	s.applyGPOS(buf, gposFeatures)

	// Step 16b: Zero width of default ignorables
	// HarfBuzz: hb_ot_zero_width_default_ignorables() in hb-ot-shape.cc:1085
	zeroWidthDefaultIgnorables(buf)

	// Step 16c: Propagate attachment offsets (cursive → marks)
	// HarfBuzz: GPOS::position_finish_offsets() in hb-ot-shape.cc:1086
	PropagateAttachmentOffsets(buf.Pos, buf.Direction)

	// Step 17: Reverse buffer if RTL
	if buf.Direction == DirectionRTL {
		s.reverseBuffer(buf)
	}
}

// setupUSECategories assigns USE categories to each glyph's USECategory field.
// HarfBuzz equivalent: setup_masks_use() -> info.use_category() = data[cp]
func (s *Shaper) setupUSECategories(buf *Buffer) {
	for i := range buf.Info {
		buf.Info[i].USECategory = uint8(getUSECategory(buf.Info[i].Codepoint))
	}
}

// findUSESyllables runs the Ragel state machine and stores syllable data on GlyphInfo.
// Returns true if any broken clusters were found.
func (s *Shaper) findUSESyllables(buf *Buffer) bool {
	// Build temporary USESyllableInfo slice for the state machine
	syllables := make([]USESyllableInfo, len(buf.Info))
	for i := range buf.Info {
		syllables[i].Category = USECategory(buf.Info[i].USECategory)
		syllables[i].Codepoint = buf.Info[i].Codepoint
	}

	hasBroken := FindSyllablesUSE(syllables)

	// Copy results back to buffer
	for i := range syllables {
		buf.Info[i].Syllable = syllables[i].Syllable
	}

	return hasBroken
}

// applyUSEPreProcessingFeatures applies pre-processing features.
// HarfBuzz equivalent: collect_features_use() lines 117-121
func (s *Shaper) applyUSEPreProcessingFeatures(buf *Buffer) {
	if s.gsub == nil {
		return
	}

	// HarfBuzz: locl, ccmp: F_PER_SYLLABLE; akhn: F_MANUAL_ZWJ | F_PER_SYLLABLE
	features := []Feature{
		{Tag: MakeTag('l', 'o', 'c', 'l'), Value: 1, PerSyllable: true},
		{Tag: MakeTag('c', 'c', 'm', 'p'), Value: 1, PerSyllable: true},
		{Tag: MakeTag('n', 'u', 'k', 't'), Value: 1, PerSyllable: true},
		{Tag: MakeTag('a', 'k', 'h', 'n'), Value: 1, PerSyllable: true, ManualZWJ: true},
	}

	otMap := CompileMap(s.gsub, nil, features, buf.Script, buf.Language)
	otMap.ApplyGSUB(s.gsub, buf, s.font, s.gdef)
}

// setupRphfMask sets up masks for rphf (repha) feature.
// HarfBuzz equivalent: setup_rphf_mask() in hb-ot-shaper-use.cc:213-229
func (s *Shaper) setupRphfMask(buf *Buffer) {
	n := len(buf.Info)
	i := 0

	for i < n {
		syllable := buf.Info[i].Syllable
		start := i

		end := i + 1
		for end < n && buf.Info[end].Syllable == syllable {
			end++
		}

		// Mark first 1 or 3 chars for rphf
		var limit int
		if USECategory(buf.Info[start].USECategory) == USE_R {
			limit = 1
		} else {
			limit = min(3, end-start)
		}

		for j := start; j < start+limit; j++ {
			buf.Info[j].Mask |= MaskRphf
		}

		i = end
	}
}

// applyRphfUSE applies rphf and records substituted repha.
// HarfBuzz equivalent: record_rphf_use() in hb-ot-shaper-use.cc:310-332
func (s *Shaper) applyRphfUSE(buf *Buffer) {
	if s.gsub == nil {
		return
	}

	// HarfBuzz: _hb_clear_substitution_flags() before rphf
	clearSubstitutionFlags(buf)

	// HarfBuzz: rphf has F_MANUAL_ZWJ | F_PER_SYLLABLE, applied with rphf_mask
	features := []Feature{
		{Tag: useRphfFeature, Value: 1, PerSyllable: true, ManualZWJ: true},
	}
	otMap := CompileMap(s.gsub, nil, features, buf.Script, buf.Language)
	for i := range otMap.GSUBLookups {
		otMap.GSUBLookups[i].Mask = MaskRphf
	}
	otMap.ApplyGSUB(s.gsub, buf, s.font, s.gdef)

	// record_rphf_use: Mark substituted rephas as USE_R
	n := len(buf.Info)
	i := 0
	for i < n {
		syllable := buf.Info[i].Syllable
		start := i

		end := i + 1
		for end < n && buf.Info[end].Syllable == syllable {
			end++
		}

		for j := start; j < end && (buf.Info[j].Mask&MaskRphf) != 0; j++ {
			if (buf.Info[j].GlyphProps & GlyphPropsSubstituted) != 0 {
				buf.Info[j].USECategory = uint8(USE_R)
				break
			}
		}

		i = end
	}
}

// applyPrefUSE applies pref and records substituted pre-base forms.
// HarfBuzz equivalent: record_pref_use() in hb-ot-shaper-use.cc:334-352
func (s *Shaper) applyPrefUSE(buf *Buffer) {
	if s.gsub == nil {
		return
	}

	// HarfBuzz: _hb_clear_substitution_flags() before pref
	clearSubstitutionFlags(buf)

	// HarfBuzz: pref has F_MANUAL_ZWJ | F_PER_SYLLABLE
	features := []Feature{
		{Tag: usePrefFeature, Value: 1, PerSyllable: true, ManualZWJ: true},
	}
	otMap := CompileMap(s.gsub, nil, features, buf.Script, buf.Language)
	otMap.ApplyGSUB(s.gsub, buf, s.font, s.gdef)

	// record_pref_use: Mark substituted pref as VPre
	n := len(buf.Info)
	i := 0
	for i < n {
		syllable := buf.Info[i].Syllable
		start := i

		end := i + 1
		for end < n && buf.Info[end].Syllable == syllable {
			end++
		}

		for j := start; j < end; j++ {
			if (buf.Info[j].GlyphProps & GlyphPropsSubstituted) != 0 {
				buf.Info[j].USECategory = uint8(USE_VPre)
				break
			}
		}

		i = end
	}
}

// applyUSEBasicFeatures applies basic shaping features.
// HarfBuzz equivalent: collect_features_use() lines 131-133
func (s *Shaper) applyUSEBasicFeatures(buf *Buffer) {
	if s.gsub == nil {
		return
	}

	// HarfBuzz: all basic features have F_MANUAL_ZWJ | F_PER_SYLLABLE
	features := make([]Feature, len(useBasicFeatures))
	for i, tag := range useBasicFeatures {
		features[i] = Feature{Tag: tag, Value: 1, PerSyllable: true, ManualZWJ: true}
	}

	otMap := CompileMap(s.gsub, nil, features, buf.Script, buf.Language)
	otMap.ApplyGSUB(s.gsub, buf, s.font, s.gdef)
}

// reorderUSE reorders glyphs within syllables.
// HarfBuzz equivalent: reorder_use() in hb-ot-shaper-use.cc:447-470
// In HarfBuzz, this function first inserts dotted circles for broken clusters,
// then reorders each syllable.
func (s *Shaper) reorderUSE(buf *Buffer, hasBroken bool) {
	// HarfBuzz: hb_syllabic_insert_dotted_circles() called inside reorder_use()
	// This happens AFTER basic features, not before pre-processing.
	if hasBroken {
		accessor := &useSyllableAccessor{buf: buf}
		if s.insertSyllabicDottedCircles(buf, accessor,
			uint8(USE_BrokenCluster),
			uint8(USE_B),
			int(USE_R)) {
			// Set USECategory on inserted dotted circles.
			// The generic function copies Syllable but doesn't know about USECategory.
			// We set it here: dotted circle (U+25CC) gets USE_B category.
			for i := range buf.Info {
				if buf.Info[i].Codepoint == 0x25CC && buf.Info[i].USECategory == 0 {
					buf.Info[i].USECategory = uint8(USE_B)
				}
			}
		}
	}

	n := len(buf.Info)
	i := 0

	for i < n {
		syllable := buf.Info[i].Syllable
		start := i

		end := i + 1
		for end < n && buf.Info[end].Syllable == syllable {
			end++
		}

		s.reorderSyllableUSE(buf, start, end)

		i = end
	}
}

// reorderSyllableUSE reorders a single syllable.
// HarfBuzz equivalent: reorder_syllable_use() in hb-ot-shaper-use.cc:361-445
func (s *Shaper) reorderSyllableUSE(buf *Buffer, start, end int) {
	syllableType := USESyllableType(buf.Info[start].Syllable & 0x0F)

	if syllableType != USE_ViramaTerminatedCluster &&
		syllableType != USE_SakotTerminatedCluster &&
		syllableType != USE_StandardCluster &&
		syllableType != USE_SymbolCluster &&
		syllableType != USE_BrokenCluster {
		return
	}

	// Move repha (R) forward
	if USECategory(buf.Info[start].USECategory) == USE_R && end-start > 1 {
		s.moveRephaForward(buf, start, end)
	}

	// Move VPre and VMPre backward
	s.movePreBasesBackward(buf, start, end)
}

// moveRephaForward moves repha toward the end, before post-base glyphs.
// HarfBuzz equivalent: reorder_syllable_use() lines 397-421
func (s *Shaper) moveRephaForward(buf *Buffer, start, end int) {
	insertPos := end - 1
	for i := start + 1; i < end; i++ {
		cat := USECategory(buf.Info[i].USECategory)
		if isUSEPostBase(cat) || isUSEHalant(cat, &buf.Info[i]) {
			insertPos = i - 1
			break
		}
	}

	if insertPos <= start {
		return
	}

	buf.MergeClusters(start, insertPos+1)

	saved := buf.Info[start]
	copy(buf.Info[start:insertPos], buf.Info[start+1:insertPos+1])
	buf.Info[insertPos] = saved

	if len(buf.Pos) > insertPos {
		savedPos := buf.Pos[start]
		copy(buf.Pos[start:insertPos], buf.Pos[start+1:insertPos+1])
		buf.Pos[insertPos] = savedPos
	}
}

// movePreBasesBackward moves VPre and VMPre backward to after halant.
// HarfBuzz equivalent: reorder_syllable_use() lines 423-444
func (s *Shaper) movePreBasesBackward(buf *Buffer, start, end int) {
	j := start
	for i := start; i < end; i++ {
		cat := USECategory(buf.Info[i].USECategory)
		if isUSEHalant(cat, &buf.Info[i]) {
			j = i + 1
		} else if (cat == USE_VPre || cat == USE_VMPre) && buf.Info[i].GetLigComp() == 0 && j < i {
			buf.MergeClusters(j, i+1)

			saved := buf.Info[i]
			copy(buf.Info[j+1:i+1], buf.Info[j:i])
			buf.Info[j] = saved

			if len(buf.Pos) > i {
				savedPos := buf.Pos[i]
				copy(buf.Pos[j+1:i+1], buf.Pos[j:i])
				buf.Pos[j] = savedPos
			}
		}
	}
}

// setupTopographicalMasks sets up masks for isol/init/medi/fina features.
// HarfBuzz equivalent: setup_topographical_masks() in hb-ot-shaper-use.cc:231-294
var joiningFormToMask = [4]uint32{
	MaskIsol, // JoiningFormIsol
	MaskInit, // JoiningFormInit
	MaskMedi, // JoiningFormMedi
	MaskFina, // JoiningFormFina
}

func (s *Shaper) setupTopographicalMasks(buf *Buffer) {
	n := len(buf.Info)
	if n == 0 {
		return
	}

	// HarfBuzz equivalent: setup_topographical_masks() in hb-ot-shaper-use.cc:231-294
	// HarfBuzz uses plan->map.get_1_mask() to get actual masks from the compiled map.
	// We use hardcoded masks. If the mask equals the global mask (meaning the feature
	// is not in the font), HarfBuzz sets it to 0.
	masks := joiningFormToMask
	var allMasks uint32
	for i := 0; i < 4; i++ {
		allMasks |= masks[i]
	}
	if allMasks == 0 {
		return
	}
	// HarfBuzz: hb_mask_t other_masks = ~all_masks;
	otherMasks := ^allMasks

	var lastStart int
	lastForm := JoiningFormNone

	i := 0
	for i < n {
		syllable := buf.Info[i].Syllable
		start := i

		end := i + 1
		for end < n && buf.Info[end].Syllable == syllable {
			end++
		}

		syllableType := USESyllableType(buf.Info[start].Syllable & 0x0F)

		if syllableType == USE_HieroglyphCluster || syllableType == USE_NonCluster {
			lastForm = JoiningFormNone
			i = end
			continue
		}

		join := lastForm == JoiningFormFina || lastForm == JoiningFormIsol

		if join {
			// HarfBuzz: Fixup previous syllable's form
			var newForm JoiningForm
			if lastForm == JoiningFormFina {
				newForm = JoiningFormMedi
			} else {
				newForm = JoiningFormInit
			}
			// HarfBuzz: info[i].mask = (info[i].mask & other_masks) | masks[last_form]
			// REPLACE topographical bits, don't just OR
			for j := lastStart; j < start; j++ {
				buf.Info[j].Mask = (buf.Info[j].Mask & otherMasks) | masks[newForm]
			}
		}

		var thisForm JoiningForm
		if join {
			thisForm = JoiningFormFina
		} else {
			thisForm = JoiningFormIsol
		}
		// HarfBuzz: info[i].mask = (info[i].mask & other_masks) | masks[last_form]
		for j := start; j < end; j++ {
			buf.Info[j].Mask = (buf.Info[j].Mask & otherMasks) | masks[thisForm]
		}

		lastStart = start
		lastForm = thisForm
		i = end
	}
}

// applyUSEOtherFeatures applies post-reordering features including horizontal features.
// HarfBuzz equivalent: collect_features_use() lines 143-145 + hb_ot_shape_collect_features()
func (s *Shaper) applyUSEOtherFeatures(buf *Buffer) {
	if s.gsub == nil {
		return
	}

	allFeatures := make([]Feature, 0, len(useTopographicalFeatures)+len(useOtherFeatures)+len(useHorizontalFeatures))

	// Topographical features (isol, init, medi, fina) - no special flags
	for _, tag := range useTopographicalFeatures {
		allFeatures = append(allFeatures, Feature{Tag: tag, Value: 1})
	}

	// Other features (abvs, blws, haln, pres, psts) - F_MANUAL_ZWJ
	for _, tag := range useOtherFeatures {
		allFeatures = append(allFeatures, Feature{Tag: tag, Value: 1, ManualZWJ: true})
	}

	// Horizontal features (calt, clig, liga, rclt) - no special flags
	for _, tag := range useHorizontalFeatures {
		allFeatures = append(allFeatures, Feature{Tag: tag, Value: 1})
	}

	otMap := CompileMap(s.gsub, nil, allFeatures, buf.Script, buf.Language)
	otMap.ApplyGSUB(s.gsub, buf, s.font, s.gdef)
}

// getUSEGPOSFeatures returns GPOS features for USE shaping.
// HarfBuzz: common_features[] + horizontal_features[] in hb-ot-shape.cc:295-318
func (s *Shaper) getUSEGPOSFeatures() []Feature {
	features := []Feature{
		{Tag: MakeTag('a', 'b', 'v', 'm'), Value: 1}, // Above-base Mark Positioning
		{Tag: MakeTag('b', 'l', 'w', 'm'), Value: 1}, // Below-base Mark Positioning
		{Tag: MakeTag('m', 'a', 'r', 'k'), Value: 1}, // Mark Positioning
		{Tag: MakeTag('m', 'k', 'm', 'k'), Value: 1}, // Mark-to-Mark Positioning
		{Tag: MakeTag('c', 'u', 'r', 's'), Value: 1}, // Cursive Positioning
		{Tag: MakeTag('d', 'i', 's', 't'), Value: 1}, // Distances
		{Tag: MakeTag('k', 'e', 'r', 'n'), Value: 1}, // Kerning
	}
	return features
}

// isUSEPostBase returns true if the category is a post-base element.
func isUSEPostBase(cat USECategory) bool {
	switch cat {
	case USE_FAbv, USE_FBlw, USE_FPst, USE_FMAbv, USE_FMBlw, USE_FMPst,
		USE_MAbv, USE_MBlw, USE_MPst, USE_MPre,
		USE_VAbv, USE_VBlw, USE_VPst, USE_VPre,
		USE_VMAbv, USE_VMBlw, USE_VMPst, USE_VMPre:
		return true
	}
	return false
}

// clearSubstitutionFlags clears the SUBSTITUTED flag on all glyphs.
// HarfBuzz equivalent: _hb_clear_substitution_flags() in hb-ot-layout.hh
// IMPORTANT: Only clears SUBSTITUTED, NOT LIGATED or MULTIPLIED.
// HarfBuzz: _hb_glyph_info_clear_substituted() only clears HB_OT_LAYOUT_GLYPH_PROPS_SUBSTITUTED.
// The LIGATED flag must be preserved for is_halant_use() checks during reordering.
func clearSubstitutionFlags(buf *Buffer) {
	for i := range buf.Info {
		buf.Info[i].GlyphProps &^= GlyphPropsSubstituted
	}
}

// composeUSE is the USE shaper's compose filter.
// HarfBuzz equivalent: compose_use() in hb-ot-shaper-use.cc:481-492
func composeUSE(a, _ Codepoint) bool {
	return !IsUnicodeMark(a)
}
