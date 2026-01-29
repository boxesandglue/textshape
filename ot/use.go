package ot

// USE Shaper - Universal Shaping Engine implementation
// HarfBuzz equivalent: hb-ot-shaper-use.cc
//
// The Universal Shaping Engine (USE) is Microsoft's specification for
// shaping complex scripts that don't have dedicated shapers.
// It handles scripts like Khmer, Tibetan, Javanese, Balinese, etc.
//
// Reference: https://docs.microsoft.com/en-us/typography/script-development/use

// useSyllableAccessor implements SyllableAccessor for USE shaper.
type useSyllableAccessor struct {
	syllables []USESyllableInfo
}

func (a *useSyllableAccessor) GetSyllable(i int) uint8 {
	return a.syllables[i].Syllable
}

func (a *useSyllableAccessor) GetCategory(i int) uint8 {
	return uint8(a.syllables[i].Category)
}

func (a *useSyllableAccessor) SetCategory(i int, cat uint8) {
	a.syllables[i].Category = USECategory(cat)
}

func (a *useSyllableAccessor) Len() int {
	return len(a.syllables)
}

// USE Features - applied in order
// HarfBuzz equivalent: use_basic_features[], use_topographical_features[], use_other_features[]
var (
	// Basic features - applied before reordering, per-syllable
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
	useOtherFeatures = []Tag{
		MakeTag('a', 'b', 'v', 's'), // Above-base Substitutions
		MakeTag('b', 'l', 'w', 's'), // Below-base Substitutions
		MakeTag('h', 'a', 'l', 'n'), // Halant Forms
		MakeTag('p', 'r', 'e', 's'), // Pre-base Substitutions
		MakeTag('p', 's', 't', 's'), // Post-base Substitutions
	}

	// Horizontal features - applied after other features (like all scripts)
	// HarfBuzz equivalent: horizontal_features[] in hb-ot-shape.cc:309-319
	// These are applied for ALL scripts with horizontal direction.
	useHorizontalFeatures = []Tag{
		MakeTag('c', 'a', 'l', 't'), // Contextual Alternates
		MakeTag('c', 'l', 'i', 'g'), // Contextual Ligatures
		MakeTag('l', 'i', 'g', 'a'), // Standard Ligatures
		MakeTag('r', 'c', 'l', 't'), // Required Contextual Alternates
	}

	// Pre-processing features (before syllable detection)
	usePreProcessingFeatures = []Tag{
		MakeTag('l', 'o', 'c', 'l'), // Localized Forms
		MakeTag('c', 'c', 'm', 'p'), // Glyph Composition/Decomposition
		MakeTag('n', 'u', 'k', 't'), // Nukta Forms
		MakeTag('a', 'k', 'h', 'n'), // Akhand
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

// hasUSEScript checks if the buffer contains USE-handled script characters.
func (s *Shaper) hasUSEScript(buf *Buffer) bool {
	for _, info := range buf.Info {
		if isUSEScript(info.Codepoint) {
			return true
		}
	}
	return false
}

// shapeUSE applies USE shaping to the buffer.
// HarfBuzz equivalent: _hb_ot_shaper_use in hb-ot-shaper-use.cc
func (s *Shaper) shapeUSE(buf *Buffer, features []Feature) {
	// Set direction based on script if not already set
	if buf.Direction == 0 {
		// Most USE scripts are LTR
		buf.Direction = DirectionLTR
	}

	// Step 0: Preprocess vowel constraints (insert dotted circles)
	// HarfBuzz equivalent: _hb_preprocess_text_vowel_constraints() in hb-ot-shaper-vowel-constraints.cc
	// This is called BEFORE normalization in HarfBuzz's preprocess_text hook
	PreprocessVowelConstraints(buf)

	// Step 1: Normalize Unicode
	// HarfBuzz uses COMPOSED_DIACRITICS_NO_SHORT_CIRCUIT for USE
	s.normalizeBuffer(buf, NormalizationModeComposedDiacritics)

	// Step 1.5: Initialize masks after normalization
	// HarfBuzz equivalent: hb_ot_shape_initialize_masks()
	buf.ResetMasks(MaskGlobal)

	// Step 1.6: Setup Arabic-like joining masks for scripts that use it
	// HarfBuzz equivalent: setup_masks_use() calling setup_masks_arabic_plan()
	// This is done BEFORE allocating use_category (setupUSECategories)
	useArabicJoining := hasArabicJoining(buf.Script)
	if useArabicJoining {
		s.setupMasksArabicPlan(buf)
	}

	// Step 2: Map codepoints to glyphs
	s.mapCodepointsToGlyphs(buf)

	// Step 3: Setup USE categories for each glyph
	syllables := s.setupUSECategories(buf)

	// Step 4: Find syllables
	hasBroken := FindSyllablesUSE(syllables)

	// Step 5: Insert dotted circles for broken clusters
	// HarfBuzz equivalent: hb_syllabic_insert_dotted_circles() in reorder_use()
	if hasBroken {
		accessor := &useSyllableAccessor{syllables: syllables}
		// USE(B) = Base category, USE(R) = Repha category
		// HarfBuzz: hb-ot-shaper-use.cc:455-458
		s.insertSyllabicDottedCircles(buf, accessor,
			uint8(USE_BrokenCluster), // broken syllable type
			uint8(USE_B),             // dotted circle category
			int(USE_R))               // repha category
		// Update syllables slice after insertion (buffer length may have changed)
		syllables = s.setupUSECategories(buf)
		FindSyllablesUSE(syllables)
	}

	// Step 5.5: Mark syllables as unsafe to break
	// HarfBuzz equivalent: buffer->unsafe_to_break(start, end) in setup_syllables_use()
	// Note: unsafe_to_break is a flag for line-breaking, not cluster merging
	// We don't implement line-breaking, so this is a no-op for now

	// Step 5: Apply pre-processing features
	s.applyUSEPreProcessingFeatures(buf, syllables)

	// Step 6: Setup masks for rphf (repha feature)
	s.setupRphfMask(buf, syllables)

	// Step 7: Apply rphf and record results
	s.applyRphfUSE(buf, syllables)

	// Step 8: Apply pref and record results
	s.applyPrefUSE(buf, syllables)

	// Step 9: Apply basic features
	s.applyUSEBasicFeatures(buf, syllables)

	// Step 10: Reorder syllables
	s.reorderUSE(buf, syllables)

	// Step 11: Setup topographical masks
	// For scripts with Arabic-like joining, this is skipped (masks already set)
	// HarfBuzz equivalent: setup_topographical_masks() checking use_plan->arabic_plan
	if !useArabicJoining {
		s.setupTopographicalMasks(buf, syllables)
	}

	// Step 12: Apply other features + horizontal features (calt, clig, liga, rclt)
	// HarfBuzz: All features are compiled together in map.compile() and applied together
	// See hb-ot-shape.cc:375-376 - horizontal_features[] added via map->add_feature()
	s.applyUSEOtherFeatures(buf, syllables)

	// Step 13: Set base advances
	s.setBaseAdvances(buf)

	// Step 14: Zero mark widths (EARLY mode - before GPOS!)
	// HarfBuzz: _hb_ot_shaper_use has zero_width_marks = HB_OT_SHAPE_ZERO_WIDTH_MARKS_BY_GDEF_EARLY
	s.zeroMarkWidthsByGDEF(buf)

	// Step 15: Apply GPOS features
	_, gposFeatures := s.categorizeFeatures(features)
	gposFeatures = append(gposFeatures, s.getUSEGPOSFeatures()...)
	s.applyGPOS(buf, gposFeatures)

	// Step 16: Reverse buffer if RTL
	if buf.Direction == DirectionRTL {
		s.reverseBuffer(buf)
	}
}

// setupUSECategories assigns USE categories to each glyph.
// HarfBuzz equivalent: setup_masks_use() in hb-ot-shaper-use.cc:188-210
func (s *Shaper) setupUSECategories(buf *Buffer) []USESyllableInfo {
	syllables := make([]USESyllableInfo, len(buf.Info))

	for i := range buf.Info {
		syllables[i].Category = getUSECategory(buf.Info[i].Codepoint)
	}

	return syllables
}

// applyUSEPreProcessingFeatures applies pre-processing features.
// HarfBuzz equivalent: collect_features_use() lines 117-121
// Uses OT Map to collect all lookups and apply them sorted by index (like HarfBuzz).
func (s *Shaper) applyUSEPreProcessingFeatures(buf *Buffer, syllables []USESyllableInfo) {
	if s.gsub == nil {
		return
	}

	// Convert tags to Features
	features := make([]Feature, len(usePreProcessingFeatures))
	for i, tag := range usePreProcessingFeatures {
		features[i] = Feature{Tag: tag, Value: 1}
	}

	// Use CompileMap to collect all lookups and sort them by index
	// HarfBuzz: All features are enabled via map->enable_feature() and applied together
	otMap := CompileMap(s.gsub, nil, features, buf.Script, buf.Language)
	otMap.ApplyGSUB(s.gsub, buf, s.font, s.gdef)
}

// setupRphfMask sets up masks for rphf (repha) feature.
// HarfBuzz equivalent: setup_rphf_mask() in hb-ot-shaper-use.cc:213-229
func (s *Shaper) setupRphfMask(buf *Buffer, syllables []USESyllableInfo) {
	// Mark first 1-3 characters of each syllable for rphf
	// If first char is R (repha), mark just it
	// Otherwise mark first 3 chars (potential Ra + H sequence)
	n := len(buf.Info)
	i := 0

	for i < n {
		syllable := syllables[i].Syllable
		start := i

		// Find syllable end
		end := i + 1
		for end < n && syllables[end].Syllable == syllable {
			end++
		}

		// Mark for rphf
		var limit int
		if syllables[start].Category == USE_R {
			limit = 1
		} else {
			limit = min(3, end-start)
		}

		for j := start; j < start+limit; j++ {
			buf.Info[j].Mask |= 1 << 0 // rphf mask bit
		}

		i = end
	}
}

// applyRphfUSE applies rphf and records substituted repha.
// HarfBuzz equivalent: record_rphf_use() in hb-ot-shaper-use.cc:310-332
// Uses buffer-based feature application (not codepoint-based) to support multi-step ligatures.
func (s *Shaper) applyRphfUSE(buf *Buffer, syllables []USESyllableInfo) {
	if s.gsub == nil {
		return
	}

	// Store original glyphs to detect substitution
	originalGlyphs := make([]GlyphID, len(buf.Info))
	for i := range buf.Info {
		originalGlyphs[i] = buf.Info[i].GlyphID
	}

	// Apply rphf using buffer-based approach
	// HarfBuzz: rphf is applied via OT map pipeline with mask
	variationsIndex := s.gsub.FindVariationsIndex(s.normalizedCoordsI)
	s.gsub.ApplyFeatureToBufferWithMaskAndVariations(useRphfFeature, buf, s.gdef, MaskGlobal, s.font, variationsIndex)

	// Mark substituted rephas as USE_R
	n := len(buf.Info)
	i := 0
	for i < n {
		syllable := syllables[i].Syllable
		start := i

		// Find syllable end
		end := i + 1
		for end < n && syllables[end].Syllable == syllable {
			end++
		}

		// Check for substituted rphf
		for j := start; j < end && (buf.Info[j].Mask&(1<<0)) != 0; j++ {
			if buf.Info[j].GlyphID != originalGlyphs[j] {
				// Substituted - mark as repha
				syllables[j].Category = USE_R
				break
			}
		}

		i = end
	}
}

// applyPrefUSE applies pref and records substituted pre-base forms.
// HarfBuzz equivalent: record_pref_use() in hb-ot-shaper-use.cc:334-352
// Uses buffer-based feature application (not codepoint-based) to support multi-step ligatures.
func (s *Shaper) applyPrefUSE(buf *Buffer, syllables []USESyllableInfo) {
	if s.gsub == nil {
		return
	}

	// Store original glyphs
	originalGlyphs := make([]GlyphID, len(buf.Info))
	for i := range buf.Info {
		originalGlyphs[i] = buf.Info[i].GlyphID
	}

	// Apply pref using buffer-based approach
	// HarfBuzz: pref is applied via OT map pipeline with mask
	variationsIndex := s.gsub.FindVariationsIndex(s.normalizedCoordsI)
	s.gsub.ApplyFeatureToBufferWithMaskAndVariations(usePrefFeature, buf, s.gdef, MaskGlobal, s.font, variationsIndex)

	// Mark substituted pref as VPre
	n := len(buf.Info)
	i := 0
	for i < n {
		syllable := syllables[i].Syllable
		start := i

		// Find syllable end
		end := i + 1
		for end < n && syllables[end].Syllable == syllable {
			end++
		}

		// Check for substituted pref
		for j := start; j < end; j++ {
			if buf.Info[j].GlyphID != originalGlyphs[j] {
				syllables[j].Category = USE_VPre
				break
			}
		}

		i = end
	}
}

// applyUSEBasicFeatures applies basic shaping features.
// HarfBuzz equivalent: collect_features_use() lines 131-133
// Uses OT Map to collect all lookups and apply them sorted by index (like HarfBuzz).
func (s *Shaper) applyUSEBasicFeatures(buf *Buffer, syllables []USESyllableInfo) {
	if s.gsub == nil {
		return
	}

	// Convert tags to Features
	features := make([]Feature, len(useBasicFeatures))
	for i, tag := range useBasicFeatures {
		features[i] = Feature{Tag: tag, Value: 1}
	}

	// Use CompileMap to collect all lookups and sort them by index
	// HarfBuzz: All features are enabled via map->enable_feature() and applied together
	otMap := CompileMap(s.gsub, nil, features, buf.Script, buf.Language)
	otMap.ApplyGSUB(s.gsub, buf, s.font, s.gdef)
}

// reorderUSE reorders glyphs within syllables.
// HarfBuzz equivalent: reorder_use() in hb-ot-shaper-use.cc:447-470
func (s *Shaper) reorderUSE(buf *Buffer, syllables []USESyllableInfo) {
	n := len(buf.Info)
	i := 0

	for i < n {
		syllable := syllables[i].Syllable
		start := i

		// Find syllable end
		end := i + 1
		for end < n && syllables[end].Syllable == syllable {
			end++
		}

		// Reorder this syllable
		s.reorderSyllableUSE(buf, syllables, start, end)

		i = end
	}
}

// reorderSyllableUSE reorders a single syllable.
// HarfBuzz equivalent: reorder_syllable_use() in hb-ot-shaper-use.cc:361-445
func (s *Shaper) reorderSyllableUSE(buf *Buffer, syllables []USESyllableInfo, start, end int) {
	syllableType := syllables[start].SyllableType

	// Only reorder certain syllable types
	if syllableType != USE_ViramaTerminatedCluster &&
		syllableType != USE_SakotTerminatedCluster &&
		syllableType != USE_StandardCluster &&
		syllableType != USE_SymbolCluster &&
		syllableType != USE_BrokenCluster {
		return
	}

	// Move repha (R) forward
	if syllables[start].Category == USE_R && end-start > 1 {
		s.moveRephaForward(buf, syllables, start, end)
	}

	// Move VPre and VMPre backward
	s.movePreBasesBackward(buf, syllables, start, end)
}

// moveRephaForward moves repha toward the end, before post-base glyphs.
// HarfBuzz equivalent: reorder_syllable_use() lines 397-421
func (s *Shaper) moveRephaForward(buf *Buffer, syllables []USESyllableInfo, start, end int) {
	// Find position to insert repha (before first post-base glyph)
	insertPos := end - 1
	for i := start + 1; i < end; i++ {
		if isUSEPostBase(syllables[i].Category) || isUSEHalant(syllables[i].Category) {
			insertPos = i - 1
			break
		}
	}

	if insertPos <= start {
		return
	}

	// Merge clusters
	buf.MergeClusters(start, insertPos+1)

	// Move repha
	saved := buf.Info[start]
	savedSyl := syllables[start]
	copy(buf.Info[start:insertPos], buf.Info[start+1:insertPos+1])
	copy(syllables[start:insertPos], syllables[start+1:insertPos+1])
	buf.Info[insertPos] = saved
	syllables[insertPos] = savedSyl

	// Move positions too
	if len(buf.Pos) > insertPos {
		savedPos := buf.Pos[start]
		copy(buf.Pos[start:insertPos], buf.Pos[start+1:insertPos+1])
		buf.Pos[insertPos] = savedPos
	}
}

// movePreBasesBackward moves VPre and VMPre backward to after halant.
// HarfBuzz equivalent: reorder_syllable_use() lines 423-444
func (s *Shaper) movePreBasesBackward(buf *Buffer, syllables []USESyllableInfo, start, end int) {
	j := start
	for i := start; i < end; i++ {
		cat := syllables[i].Category
		if isUSEHalant(cat) {
			j = i + 1
		} else if (cat == USE_VPre || cat == USE_VMPre) && j < i {
			// Merge clusters and move
			buf.MergeClusters(j, i+1)

			saved := buf.Info[i]
			savedSyl := syllables[i]
			copy(buf.Info[j+1:i+1], buf.Info[j:i])
			copy(syllables[j+1:i+1], syllables[j:i])
			buf.Info[j] = saved
			syllables[j] = savedSyl

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
func (s *Shaper) setupTopographicalMasks(buf *Buffer, syllables []USESyllableInfo) {
	// For scripts that don't use Arabic joining (most USE scripts),
	// topographical features are determined by syllable boundaries.
	// We set masks based on joining behavior between syllables.

	n := len(buf.Info)
	if n == 0 {
		return
	}

	// Track syllable positions for joining
	var lastStart int
	lastForm := JoiningFormNone

	i := 0
	for i < n {
		syllable := syllables[i].Syllable
		start := i

		// Find syllable end
		end := i + 1
		for end < n && syllables[end].Syllable == syllable {
			end++
		}

		syllableType := syllables[start].SyllableType

		// Hieroglyph and non-clusters don't join
		if syllableType == USE_HieroglyphCluster || syllableType == USE_NonCluster {
			lastForm = JoiningFormNone
			i = end
			continue
		}

		// Determine joining
		join := lastForm == JoiningFormFina || lastForm == JoiningFormIsol

		if join {
			// Update previous syllable's form
			var newForm JoiningForm
			if lastForm == JoiningFormFina {
				newForm = JoiningFormMedi
			} else {
				newForm = JoiningFormInit
			}
			for j := lastStart; j < start; j++ {
				buf.Info[j].Mask |= uint32(newForm) << 1 // Shift past rphf bit
			}
		}

		// Set this syllable's form
		var thisForm JoiningForm
		if join {
			thisForm = JoiningFormFina
		} else {
			thisForm = JoiningFormIsol
		}
		for j := start; j < end; j++ {
			buf.Info[j].Mask |= uint32(thisForm) << 1
		}

		lastStart = start
		lastForm = thisForm
		i = end
	}
}

// applyUSEOtherFeatures applies post-reordering features including horizontal features.
// HarfBuzz equivalent: collect_features_use() lines 143-145 + hb_ot_shape_collect_features()
//
// In HarfBuzz, ALL features are collected into the map builder and compiled TOGETHER:
// 1. use_other_features (abvs, blws, haln, pres, psts) via enable_feature(..., F_MANUAL_ZWJ)
// 2. horizontal_features (calt, clig, liga, rclt) via add_feature() in hb_ot_shape_collect_features
//
// Then map.compile() creates a sorted lookup list, and all lookups are applied together.
// We replicate this by combining both feature lists into one CompileMap call.
func (s *Shaper) applyUSEOtherFeatures(buf *Buffer, syllables []USESyllableInfo) {
	if s.gsub == nil {
		return
	}

	// Combine all USE features into one list
	// HarfBuzz: All features are compiled together in map.compile()
	// This ensures correct lookup ordering when features share lookups
	allFeatures := make([]Feature, 0, len(useTopographicalFeatures)+len(useOtherFeatures)+len(useHorizontalFeatures))

	// Add topographical features (isol, init, medi, fina)
	// HarfBuzz: collect_features_use() lines 139-140
	// These use masks set by either setupMasksArabicPlan or setupTopographicalMasks
	for _, tag := range useTopographicalFeatures {
		allFeatures = append(allFeatures, Feature{Tag: tag, Value: 1})
	}

	// Add USE other features (abvs, blws, haln, pres, psts)
	for _, tag := range useOtherFeatures {
		allFeatures = append(allFeatures, Feature{Tag: tag, Value: 1})
	}

	// Add horizontal features (calt, clig, liga, rclt)
	// HarfBuzz: horizontal_features[] in hb-ot-shape.cc:309-319
	for _, tag := range useHorizontalFeatures {
		allFeatures = append(allFeatures, Feature{Tag: tag, Value: 1})
	}

	// Use CompileMap to collect all lookups and sort them by index
	// This replicates HarfBuzz's map.compile() which sorts all lookups together
	otMap := CompileMap(s.gsub, nil, allFeatures, buf.Script, buf.Language)
	otMap.ApplyGSUB(s.gsub, buf, s.font, s.gdef)
}

// getUSEGPOSFeatures returns GPOS features for USE shaping.
func (s *Shaper) getUSEGPOSFeatures() []Feature {
	return []Feature{
		{Tag: MakeTag('d', 'i', 's', 't'), Value: 1}, // Distances
		{Tag: MakeTag('a', 'b', 'v', 'm'), Value: 1}, // Above-base Mark Positioning
		{Tag: MakeTag('b', 'l', 'w', 'm'), Value: 1}, // Below-base Mark Positioning
		{Tag: MakeTag('m', 'a', 'r', 'k'), Value: 1}, // Mark Positioning
		{Tag: MakeTag('m', 'k', 'm', 'k'), Value: 1}, // Mark-to-Mark Positioning
	}
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

// mergeSyllableClustersUSE merges clusters within each syllable.
// HarfBuzz equivalent: buffer->unsafe_to_break(start, end) in setup_syllables_use()
// This ensures that all glyphs within a syllable share the same (minimum) cluster value.
// IMPORTANT: ZWNJ (U+200C) forms cluster boundaries. The part before ZWNJ merges to
// its minimum, and the part starting with ZWNJ merges to ZWNJ's cluster.
func (s *Shaper) mergeSyllableClustersUSE(buf *Buffer, syllables []USESyllableInfo) {
	n := len(buf.Info)
	i := 0

	for i < n {
		syllable := syllables[i].Syllable
		start := i

		// Find syllable end
		end := i + 1
		for end < n && syllables[end].Syllable == syllable {
			end++
		}

		// Find ZWNJ position within syllable (if any)
		zwnjPos := -1
		for j := start; j < end; j++ {
			if buf.Info[j].Codepoint == 0x200C {
				zwnjPos = j
				break
			}
		}

		if zwnjPos >= 0 {
			// Merge part before ZWNJ
			if zwnjPos > start {
				buf.MergeClusters(start, zwnjPos)
			}
			// Merge part starting with ZWNJ (including ZWNJ) to ZWNJ's cluster
			if end > zwnjPos {
				// The cluster after ZWNJ should all be set to ZWNJ's cluster
				zwnjCluster := buf.Info[zwnjPos].Cluster
				for j := zwnjPos; j < end; j++ {
					buf.Info[j].Cluster = zwnjCluster
				}
			}
		} else {
			// No ZWNJ - merge entire syllable
			if end > start {
				buf.MergeClusters(start, end)
			}
		}

		i = end
	}
}
