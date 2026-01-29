package ot

// Arabic shaper implementation based on HarfBuzz's hb-ot-shaper-arabic.cc
// This implements the joining logic for Arabic and related scripts.

// Arabic Unicode Normalization
// These are the key compositions that need to happen before shaping:
//
// Base + Combining Mark → Precomposed
// U+0627 ALEF + U+0653 MADDAH ABOVE → U+0622 ALEF WITH MADDA ABOVE
// U+0627 ALEF + U+0654 HAMZA ABOVE → U+0623 ALEF WITH HAMZA ABOVE
// U+0627 ALEF + U+0655 HAMZA BELOW → U+0625 ALEF WITH HAMZA BELOW
// U+0648 WAW  + U+0654 HAMZA ABOVE → U+0624 WAW WITH HAMZA ABOVE
// U+064A YEH  + U+0654 HAMZA ABOVE → U+0626 YEH WITH HAMZA ABOVE
// U+06C1 HEH GOAL + U+0654 HAMZA ABOVE → U+06C2 HEH GOAL WITH HAMZA ABOVE
// U+06D2 YEH BARREE + U+0654 HAMZA ABOVE → U+06D3 YEH BARREE WITH HAMZA ABOVE
// U+06D5 AE + U+0654 HAMZA ABOVE → U+06C0 HEH WITH YEH ABOVE

// unicodeComposition maps (base, mark) pairs to their precomposed form.
// This covers canonical Arabic compositions from Unicode.
var unicodeComposition = map[[2]Codepoint]Codepoint{
	// Core Arabic compositions
	{0x0627, 0x0653}: 0x0622, // ALEF + MADDAH → ALEF WITH MADDA
	{0x0627, 0x0654}: 0x0623, // ALEF + HAMZA ABOVE → ALEF WITH HAMZA ABOVE
	{0x0627, 0x0655}: 0x0625, // ALEF + HAMZA BELOW → ALEF WITH HAMZA BELOW
	{0x0648, 0x0654}: 0x0624, // WAW + HAMZA ABOVE → WAW WITH HAMZA ABOVE
	{0x064A, 0x0654}: 0x0626, // YEH + HAMZA ABOVE → YEH WITH HAMZA ABOVE
	// Extended Arabic compositions
	{0x06C1, 0x0654}: 0x06C2, // HEH GOAL + HAMZA ABOVE → HEH GOAL WITH HAMZA ABOVE
	{0x06D2, 0x0654}: 0x06D3, // YEH BARREE + HAMZA ABOVE → YEH BARREE WITH HAMZA ABOVE
	{0x06D5, 0x0654}: 0x06C0, // AE + HAMZA ABOVE → HEH WITH YEH ABOVE
}

// normalizeArabic performs font-aware Unicode normalization on the buffer.
// Following HarfBuzz's approach:
//   - It composes base + mark sequences into precomposed forms IF the font has
//     a glyph for the composed form.
//   - This allows fonts that lack individual glyphs for base/mark to still
//     render correctly using precomposed glyphs.
//   - When merging, use the minimum cluster value (for proper RTL cluster behavior).
func (s *Shaper) normalizeArabic(buf *Buffer) {
	if len(buf.Info) < 2 || s.cmap == nil {
		return
	}

	newInfo := make([]GlyphInfo, 0, len(buf.Info))
	i := 0
	for i < len(buf.Info) {
		// Check if current char can compose with next char
		if i+1 < len(buf.Info) {
			base := buf.Info[i].Codepoint
			mark := buf.Info[i+1].Codepoint

			if composed, ok := unicodeComposition[[2]Codepoint{base, mark}]; ok {
				// Check if font has the composed form
				composedGid, composedFound := s.cmap.Lookup(composed)

				if composedFound && composedGid != 0 {
					// Font has the composed form - use it
					// Merge clusters: use minimum cluster (for RTL compatibility)
					cluster := min(buf.Info[i].Cluster, buf.Info[i+1].Cluster)
					newInfo = append(newInfo, GlyphInfo{
						Codepoint: composed,
						Cluster:   cluster,
						Mask:      MaskGlobal, // HarfBuzz: glyphs have global_mask
					})
					i += 2
					continue
				}
				// Font doesn't have composed form - leave decomposed
			}
		}

		newInfo = append(newInfo, buf.Info[i])
		i++
	}

	buf.Info = newInfo
	buf.Pos = make([]GlyphPos, len(newInfo))
}

// JoiningType represents the Arabic joining behavior of a character.
type JoiningType uint8

const (
	joiningTypeU JoiningType = iota // Non-joining
	joiningTypeL                    // Left-joining only
	joiningTypeR                    // Right-joining only
	joiningTypeD                    // Dual-joining (both sides)
	joiningTypeC                    // Join-causing (like ZWJ)
	joiningTypeT                    // Transparent (combining marks)
	joiningTypeX                    // Determined by Unicode category

	// Special joining groups
	joiningGroupAlaph      // Syriac ALAPH
	joiningGroupDalathRish // Syriac DALATH/RISH
)

// ArabicAction represents the shaping action for a character.
// HarfBuzz equivalent: hb-ot-shaper-arabic.cc:115-131 (arabic_action_t)
// The order MUST match arabic_features array and HarfBuzz's enum.
type ArabicAction uint8

const (
	arabicActionISOL ArabicAction = iota // Isolated form (index 0)
	arabicActionFINA                     // Final form (index 1)
	arabicActionFIN2                     // Final form variant 2 - Syriac ALAPH (index 2)
	arabicActionFIN3                     // Final form variant 3 - Syriac DALATH_RISH (index 3)
	arabicActionMEDI                     // Medial form (index 4)
	arabicActionMED2                     // Medial form variant 2 - Syriac ALAPH (index 5)
	arabicActionINIT                     // Initial form (index 6)
	arabicActionNone                     // No action (index 7) - ARABIC_NUM_FEATURES

	// STCH (Stretching) actions - used for Syriac/Arabic kashida stretching
	// HarfBuzz equivalent: STCH_FIXED, STCH_REPEATING in hb-ot-shaper-arabic.cc:129-130
	arabicActionSTCH_FIXED     // Fixed tile in stretching sequence (index 8)
	arabicActionSTCH_REPEATING // Repeating tile in stretching sequence (index 9)
)

// arabicStateEntry represents a state table entry with previous action, current action, and next state.
type arabicStateEntry struct {
	prevAction ArabicAction
	currAction ArabicAction
	nextState  uint8
}

// Arabic state machine: 7 states, 6 joining type columns
// HarfBuzz equivalent: hb-ot-shaper-arabic.cc:133-161 (arabic_state_table)
// Columns: jt_U, jt_L, jt_R, jt_D, jg_ALAPH, jg_DALATH_RISH
var arabicStateTable = [7][6]arabicStateEntry{
	// State 0: prev was U, not willing to join.
	{
		{arabicActionNone, arabicActionNone, 0}, // U
		{arabicActionNone, arabicActionISOL, 2}, // L
		{arabicActionNone, arabicActionISOL, 1}, // R
		{arabicActionNone, arabicActionISOL, 2}, // D
		{arabicActionNone, arabicActionISOL, 1}, // ALAPH
		{arabicActionNone, arabicActionISOL, 6}, // DALATH_RISH -> state 6
	},
	// State 1: prev was R or ISOL/ALAPH, not willing to join.
	{
		{arabicActionNone, arabicActionNone, 0}, // U
		{arabicActionNone, arabicActionISOL, 2}, // L
		{arabicActionNone, arabicActionISOL, 1}, // R
		{arabicActionNone, arabicActionISOL, 2}, // D
		{arabicActionNone, arabicActionFIN2, 5}, // ALAPH -> FIN2, state 5
		{arabicActionNone, arabicActionISOL, 6}, // DALATH_RISH -> state 6
	},
	// State 2: prev was D/L in ISOL form, willing to join.
	{
		{arabicActionNone, arabicActionNone, 0}, // U
		{arabicActionNone, arabicActionISOL, 2}, // L
		{arabicActionINIT, arabicActionFINA, 1}, // R: prev=INIT, curr=FINA
		{arabicActionINIT, arabicActionFINA, 3}, // D: prev=INIT, curr=FINA
		{arabicActionINIT, arabicActionFINA, 4}, // ALAPH: prev=INIT, curr=FINA
		{arabicActionINIT, arabicActionFINA, 6}, // DALATH_RISH -> state 6
	},
	// State 3: prev was D in FINA form, willing to join.
	{
		{arabicActionNone, arabicActionNone, 0}, // U
		{arabicActionNone, arabicActionISOL, 2}, // L
		{arabicActionMEDI, arabicActionFINA, 1}, // R: prev=MEDI
		{arabicActionMEDI, arabicActionFINA, 3}, // D: prev=MEDI
		{arabicActionMEDI, arabicActionFINA, 4}, // ALAPH: prev=MEDI
		{arabicActionMEDI, arabicActionFINA, 6}, // DALATH_RISH -> state 6
	},
	// State 4: prev was FINA ALAPH, not willing to join.
	{
		{arabicActionNone, arabicActionNone, 0}, // U
		{arabicActionNone, arabicActionISOL, 2}, // L
		{arabicActionMED2, arabicActionISOL, 1}, // R: prev=MED2
		{arabicActionMED2, arabicActionISOL, 2}, // D: prev=MED2
		{arabicActionMED2, arabicActionFIN2, 5}, // ALAPH -> FIN2, state 5
		{arabicActionMED2, arabicActionISOL, 6}, // DALATH_RISH -> state 6
	},
	// State 5: prev was FIN2/FIN3 ALAPH, not willing to join.
	{
		{arabicActionNone, arabicActionNone, 0}, // U
		{arabicActionNone, arabicActionISOL, 2}, // L
		{arabicActionISOL, arabicActionISOL, 1}, // R: prev=ISOL
		{arabicActionISOL, arabicActionISOL, 2}, // D: prev=ISOL
		{arabicActionISOL, arabicActionFIN2, 5}, // ALAPH -> FIN2, state 5
		{arabicActionISOL, arabicActionISOL, 6}, // DALATH_RISH -> state 6
	},
	// State 6: prev was DALATH/RISH, not willing to join.
	{
		{arabicActionNone, arabicActionNone, 0}, // U
		{arabicActionNone, arabicActionISOL, 2}, // L
		{arabicActionNone, arabicActionISOL, 1}, // R
		{arabicActionNone, arabicActionISOL, 2}, // D
		{arabicActionNone, arabicActionFIN3, 5}, // ALAPH -> FIN3, state 5
		{arabicActionNone, arabicActionISOL, 6}, // DALATH_RISH -> state 6
	},
}

// Arabic feature tags
// HarfBuzz equivalent: hb-ot-shaper-arabic.cc:101-111 (arabic_features)
var (
	tagIsol = MakeTag('i', 's', 'o', 'l') // Isolated
	tagFina = MakeTag('f', 'i', 'n', 'a') // Final
	tagFin2 = MakeTag('f', 'i', 'n', '2') // Final 2 (Syriac ALAPH)
	tagFin3 = MakeTag('f', 'i', 'n', '3') // Final 3 (Syriac DALATH_RISH)
	tagMedi = MakeTag('m', 'e', 'd', 'i') // Medial
	tagMed2 = MakeTag('m', 'e', 'd', '2') // Medial 2 (Syriac ALAPH)
	tagInit = MakeTag('i', 'n', 'i', 't') // Initial
	tagRlig = MakeTag('r', 'l', 'i', 'g') // Required Ligatures
	tagCalt = MakeTag('c', 'a', 'l', 't') // Contextual Alternates
	tagLiga = MakeTag('l', 'i', 'g', 'a') // Standard Ligatures
	tagCcmp = MakeTag('c', 'c', 'm', 'p') // Glyph Composition/Decomposition
)

// GetJoiningTypeDebug returns the joining type for a Unicode codepoint (exported for debugging).
func GetJoiningTypeDebug(u Codepoint) JoiningType {
	return getJoiningType(u, getGeneralCategory(u))
}

// getJoiningType returns the joining type for a Unicode codepoint.
// HarfBuzz equivalent: get_joining_type() in hb-ot-shaper-arabic.cc:86-97
//
// This is a wrapper around joiningType() (from the generated table) that
// handles codepoints not in the table by checking their Unicode General Category.
func getJoiningType(u Codepoint, genCat GeneralCategory) JoiningType {
	// First, check the table
	jt := joiningType(u)
	if jt != joiningTypeX {
		return jt
	}

	// For codepoints not in the table (X), check Unicode General Category.
	// Non-Spacing Marks (Mn), Enclosing Marks (Me), and Format (Cf) are transparent.
	// HarfBuzz: FLAG(NON_SPACING_MARK) | FLAG(ENCLOSING_MARK) | FLAG(FORMAT)
	switch genCat {
	case GCNonSpacingMark, GCEnclosingMark, GCFormat:
		return joiningTypeT
	default:
		return joiningTypeU
	}
}

// joiningTypeColumn maps JoiningType to state table column index.
func joiningTypeColumn(jt JoiningType) int {
	switch jt {
	case joiningTypeU, joiningTypeX:
		return 0
	case joiningTypeL:
		return 1
	case joiningTypeR:
		return 2
	case joiningTypeD, joiningTypeC:
		return 3
	case joiningGroupAlaph:
		return 4
	case joiningGroupDalathRish:
		return 5
	default:
		return 0
	}
}

// arabicJoining performs Arabic joining analysis on the buffer.
// It sets the ArabicAction for each glyph in the buffer.
// hb-ot-shaper-arabic.cc:299-374 (arabic_joining)
func (s *Shaper) arabicJoining(buf *Buffer) []ArabicAction {
	if len(buf.Info) == 0 {
		return nil
	}

	actions := make([]ArabicAction, len(buf.Info))
	state := uint8(0)
	prevI := -1 // Index of previous non-transparent character

	for i := 0; i < len(buf.Info); i++ {
		cp := buf.Info[i].Codepoint
		jt := getJoiningType(cp, getGeneralCategory(cp))

		// Skip transparent characters (combining marks)
		if jt == joiningTypeT {
			actions[i] = arabicActionNone
			continue
		}

		col := joiningTypeColumn(jt)
		entry := arabicStateTable[state][col]

		// Apply previous action
		if prevI >= 0 && entry.prevAction != arabicActionNone {
			actions[prevI] = entry.prevAction
		}

		// Set current action
		actions[i] = entry.currAction

		// Update state
		state = entry.nextState
		prevI = i
	}

	return actions
}

// setupMasksArabic sets up masks for Arabic shaping.
// HarfBuzz equivalent: setup_masks_arabic() in hb-ot-shaper-arabic.cc:405-411
//
// This function:
// 1. Sets MaskGlobal on all glyphs (for features like ccmp, rlig, calt, liga)
// 2. Runs arabicJoining() to determine the positional action for each glyph
// 3. Sets the appropriate positional mask (MaskIsol, MaskInit, MaskMedi, MaskFina, etc.)
//
// After this, lookups can check (glyph.mask & lookup.mask) to determine if they apply.
func (s *Shaper) setupMasksArabic(buf *Buffer) []ArabicAction {
	debugPrintf("setupMasksArabic: called with %d glyphs\n", len(buf.Info))

	// Step 1: Set global mask on all glyphs
	buf.ResetMasks(MaskGlobal)

	// Step 2: Run Arabic joining analysis
	actions := s.arabicJoining(buf)
	debugPrintf("setupMasksArabic: actions len=%d\n", len(actions))

	// Step 2.5: For Mongolian script, copy action from base to variation selectors
	// HarfBuzz equivalent: mongolian_variation_selectors() in hb-ot-shaper-arabic.cc:377-385
	// This is critical for ligature matching: FVS must have the same mask as the base
	// character so that ligature lookups can match Base+FVS sequences.
	if s.hasMongolianScript(buf) {
		mongolianVariationSelectors(buf, actions)
	}

	// Step 3: Set positional masks based on joining actions
	// HarfBuzz equivalent: info[i].mask |= arabic_plan->mask_array[action]
	for i, action := range actions {
		if i < len(buf.Info) {
			mask := arabicActionToMask(action)
			buf.Info[i].Mask |= mask
			debugPrintf("setupMasksArabic: [%d] U+%04X action=%d mask=0x%X -> total=0x%X\n",
				i, buf.Info[i].Codepoint, action, mask, buf.Info[i].Mask)
		}
	}

	return actions
}

// setupMasksArabicPlan performs Arabic joining analysis and sets positional masks.
// This is the version for USE shaper - it does NOT reset masks.
// HarfBuzz equivalent: setup_masks_arabic_plan() in hb-ot-shaper-arabic.cc:388-402
//
// This is called from USE shaper for scripts that use Arabic-like joining
// (Adlam, Mongolian, N'Ko, etc.) but are handled by the USE shaper, not Arabic shaper.
func (s *Shaper) setupMasksArabicPlan(buf *Buffer) {
	// Step 1: Run Arabic joining analysis
	// HarfBuzz equivalent: arabic_joining(buffer)
	actions := s.arabicJoining(buf)

	// Step 2: For Mongolian script, copy action from base to variation selectors
	// HarfBuzz equivalent: mongolian_variation_selectors() in hb-ot-shaper-arabic.cc:377-385
	if buf.Script == MakeTag('M', 'o', 'n', 'g') {
		mongolianVariationSelectors(buf, actions)
	}

	// Step 3: Set positional masks based on joining actions
	// HarfBuzz equivalent: info[i].mask |= arabic_plan->mask_array[action]
	for i, action := range actions {
		if i < len(buf.Info) {
			buf.Info[i].Mask |= arabicActionToMask(action)
		}
	}
}

// mongolianVariationSelectors copies the arabic_shaping_action from base characters
// to following Mongolian Free Variation Selectors (U+180B-U+180D, U+180F).
// HarfBuzz equivalent: mongolian_variation_selectors() in hb-ot-shaper-arabic.cc:377-385
//
// This is essential for Mongolian script because:
// - FVS characters get JoiningType T (transparent) and thus arabicActionNone
// - Without this fix, FVS won't have positional masks (e.g., MaskIsol)
// - Ligature lookups with mask filtering will skip FVS because mask check fails
// - By copying the action from the base, FVS gets the same mask and ligatures work
func mongolianVariationSelectors(buf *Buffer, actions []ArabicAction) {
	if len(buf.Info) < 2 || len(actions) < 2 {
		return
	}

	for i := 1; i < len(buf.Info); i++ {
		cp := buf.Info[i].Codepoint
		// Mongolian Free Variation Selectors: U+180B-U+180D, U+180F
		if (cp >= 0x180B && cp <= 0x180D) || cp == 0x180F {
			// Copy action from previous character
			actions[i] = actions[i-1]
		}
	}
}

// applyArabicFeatures applies Arabic-specific GSUB features.
// HarfBuzz equivalent: setup_masks_arabic + GSUB application
//
// For proper HarfBuzz compatibility:
// - First runs setupMasksArabic() to set masks based on joining analysis
// - Positional features are applied with mask filtering (only to glyphs with matching mask)
// - rlig, calt, liga are applied after with proper pauses
// - User GSUB features are applied at the end
//
// This is the HarfBuzz-style approach where lookups see the full glyph stream
// but only affect glyphs where (glyph.mask & feature.mask) != 0.
func (s *Shaper) applyArabicFeatures(buf *Buffer, userFeatures []Feature) {
	// Step 1: Set up masks for Arabic shaping
	// HarfBuzz equivalent: setup_masks_arabic() + arabic_joining()
	// This sets MaskGlobal on all glyphs, runs joining analysis,
	// and sets positional masks (MaskIsol, MaskInit, MaskMedi, MaskFina, etc.)
	s.setupMasksArabic(buf)

	// Check if we need fallback shaping (for fonts without GSUB but with presentation forms)
	useFallback := s.arabicFallbackPlan != nil

	// Apply ccmp first (may decompose glyphs via Multiple Substitution)
	// HarfBuzz: ccmp is applied before positional features, and new glyphs
	// inherit the mask of the original glyph.
	// Use MaskGlobal since ccmp applies to all glyphs.
	// Using Buffer-based method to preserve cluster information.
	if s.gsub != nil {
		s.gsub.ApplyFeatureToBufferWithMask(tagCcmp, buf, s.gdef, MaskGlobal, s.font)
	}

	// CRITICAL: Set glyph classes from GDEF immediately after ccmp!
	// ccmp may create new glyphs (e.g., decompose U+0623 → Alef + HamzaAbove).
	// We need to know which glyphs are marks BEFORE applying positional features.
	// HarfBuzz equivalent: In HarfBuzz, this happens implicitly because the glyph
	// classes are set during substitution via _set_glyph_props().
	s.setGlyphClasses(buf)

	// CRITICAL: After ccmp, clear positional masks from marks!
	// ccmp may decompose precomposed characters (e.g., U+0623 → Alef + HamzaAbove).
	// The new glyphs inherit the mask of the original glyph, but marks should
	// only have MaskGlobal, not positional masks (isol/init/medi/fina).
	// HarfBuzz equivalent: This is implicit in HarfBuzz because setupMasksArabic
	// is called AFTER GSUB, but we call it BEFORE.
	s.clearPositionalMasksFromMarks(buf)

	// Apply direction-dependent features FIRST (before positional features!)
	// HarfBuzz: hb-ot-shape.cc:332-347 - ltra/ltrm for LTR, rtla/rtlm for RTL
	// These are applied BEFORE positional features (init, fina, medi, etc.)
	// Critical for Phags-Pa: ltrm lookup 6 ligates base+VS before positional features
	if s.gsub != nil {
		switch buf.Direction {
		case DirectionLTR:
			s.gsub.ApplyFeatureToBufferWithMask(MakeTag('l', 't', 'r', 'a'), buf, s.gdef, MaskGlobal, s.font)
			s.gsub.ApplyFeatureToBufferWithMask(MakeTag('l', 't', 'r', 'm'), buf, s.gdef, MaskGlobal, s.font)
		case DirectionRTL:
			s.gsub.ApplyFeatureToBufferWithMask(MakeTag('r', 't', 'l', 'a'), buf, s.gdef, MaskGlobal, s.font)
			s.gsub.ApplyFeatureToBufferWithMask(MakeTag('r', 't', 'l', 'm'), buf, s.gdef, MaskGlobal, s.font)
		}
	}

	// Apply positional features (or fallback if needed)
	if useFallback {
		// Use fallback shaping for fonts without GSUB but with presentation forms
		// HarfBuzz: arabic_fallback_plan_shape() in hb-ot-shaper-arabic-fallback.hh:368-381
		s.arabicFallbackPlan.shape(buf, s.gdef)
	} else if s.gsub != nil {
		// Apply positional features with mask-based filtering using OT Map
		// HarfBuzz equivalent: All lookups are collected, sorted by lookup index,
		// and applied in that order. This is CRITICAL because lookups may have
		// dependencies - e.g., init lookups may need to see unmodified glyphs
		// before fina lookups change them.
		//
		// HarfBuzz: hb-ot-map.cc:362-377 - lookups are sorted by index, not feature order
		// Example: If font has init=lookup 9, fina=lookup 12, then init is applied FIRST
		// even though fina appears before init in the feature list!
		//
		// This is the key to Phags-Pa VS handling: init lookup 9 must see the unmirrored
		// lookahead glyph BEFORE fina lookup 12 changes it.
		positionalFeatures := []struct {
			tag  Tag
			mask uint32
		}{
			{tagIsol, MaskIsol},
			{tagFina, MaskFina},
			{tagFin2, MaskFin2},
			{tagFin3, MaskFin3},
			{tagMedi, MaskMedi},
			{tagMed2, MaskMed2},
			{tagInit, MaskInit},
		}

		// Build OT Map with all positional features
		otMap := NewOTMap()
		featureList, err := s.gsub.ParseFeatureList()
		if err == nil {
			for _, pf := range positionalFeatures {
				lookups := featureList.FindFeature(pf.tag)
				for _, lookupIdx := range lookups {
					otMap.AddGSUBLookup(lookupIdx, pf.mask, pf.tag)
				}
			}
		}

		// Sort lookups by index and deduplicate
		// HarfBuzz: hb-ot-map.cc:362-377
		otMap.GSUBLookups = deduplicateLookups(otMap.GSUBLookups)

		// Apply all lookups in sorted order
		otMap.ApplyGSUB(s.gsub, buf, s.font, s.gdef)
	}

	// Apply stch (stretching) feature and record STCH glyphs
	// HarfBuzz: map->enable_feature(HB_TAG('s','t','c','h')); map->add_gsub_pause(record_stch);
	// This must be done BEFORE rlig/calt/liga
	//
	// IMPORTANT: Clear MULTIPLIED flag before stch, so recordStch() only sees
	// glyphs multiplied by the stch feature, not by earlier MultipleSubst lookups.
	if s.gsub != nil {
		// Clear MULTIPLIED flag on all glyphs before stch
		for i := range buf.Info {
			buf.Info[i].GlyphProps &^= GlyphPropsMultiplied
		}
		tagStch := MakeTag('s', 't', 'c', 'h')
		s.gsub.ApplyFeatureToBufferWithMask(tagStch, buf, s.gdef, MaskGlobal, s.font)
		recordStch(buf)
	}

	// Apply rlig, calt, liga features.
	// HarfBuzz: hb-ot-shaper-arabic.cc:228-238
	//
	// CRITICAL: The pause behavior differs by script!
	// - Arabic: Pause between rlig and calt (for IranNastaliq ALLAH ligature, issue #1573)
	// - Mongolian/other: NO pause - rlig+calt+liga applied TOGETHER, sorted by lookup index
	//
	// HarfBuzz comment (line 200-204):
	//   "At least for Arabic, looks like Uniscribe has a pause between rlig and calt.
	//    Otherwise the IranNastaliq's ALLAH ligature won't work.
	//    However, testing shows that rlig and calt are applied together for Mongolian
	//    in Uniscribe. As such, we only add a pause for Arabic, not other scripts."
	//
	if s.gsub != nil {
		if s.isArabicProper(buf) {
			// Arabic: Apply rlig FIRST (with pause), then calt+liga separately
			s.gsub.ApplyFeatureToBufferWithMask(tagRlig, buf, s.gdef, MaskGlobal, s.font)
			s.gsub.ApplyFeatureToBufferWithMask(tagCalt, buf, s.gdef, MaskGlobal, s.font)
			s.gsub.ApplyFeatureToBufferWithMask(tagLiga, buf, s.gdef, MaskGlobal, s.font)
		} else {
			// Mongolian and other scripts: Apply rlig+calt+liga TOGETHER
			// All lookups sorted by index (no pause between features)
			s.applyRligCaltLigaTogether(buf)
		}
	}

	// Apply user GSUB features (e.g., salt=2, ss01)
	// Standard Arabic features (ccmp, rlig, calt, liga, positional) are already applied above,
	// so we filter them out to avoid double application.
	// HarfBuzz: In HarfBuzz, all features go through OT Map which deduplicates lookups.
	// Since we apply standard features separately, we filter them here.
	s.applyUserArabicGSUBFeatures(buf, userFeatures)
}

// applyUserArabicGSUBFeatures applies user-requested GSUB features that are not
// standard Arabic features. Standard Arabic features (ccmp, rlig, calt, liga, clig,
// positional) are already applied by applyArabicFeatures, so they are filtered out.
// HarfBuzz: In HarfBuzz, user features go through the same OT Map as standard features,
// and lookup deduplication prevents double application. Since we apply standard features
// separately, we filter them here to achieve the same result.
func (s *Shaper) applyUserArabicGSUBFeatures(buf *Buffer, userFeatures []Feature) {
	if s.gsub == nil || len(userFeatures) == 0 {
		return
	}

	for _, f := range userFeatures {
		if f.Value == 0 {
			continue
		}
		// Skip standard Arabic features that are already applied
		if isStandardArabicGSUBFeature(f.Tag) {
			continue
		}
		// Apply the user feature
		// TODO: Features with value > 1 (like salt=2) need special handling
		// for AlternateSubst. For now, apply with standard mask.
		s.gsub.ApplyFeatureToBufferWithMask(f.Tag, buf, s.gdef, MaskGlobal, s.font)
	}
}

// isStandardArabicGSUBFeature returns true if the tag is a standard Arabic GSUB feature
// that is already applied by applyArabicFeatures.
// HarfBuzz: These are added via map->enable_feature() in collect_features_arabic()
func isStandardArabicGSUBFeature(tag Tag) bool {
	switch tag {
	case MakeTag('c', 'c', 'm', 'p'), // ccmp
		MakeTag('l', 'o', 'c', 'l'), // locl (applied via rtla/rtlm path)
		MakeTag('r', 'l', 'i', 'g'), // rlig
		MakeTag('c', 'a', 'l', 't'), // calt
		MakeTag('l', 'i', 'g', 'a'), // liga
		MakeTag('c', 'l', 'i', 'g'), // clig
		MakeTag('i', 's', 'o', 'l'), // isol
		MakeTag('f', 'i', 'n', 'a'), // fina
		MakeTag('f', 'i', 'n', '2'), // fin2
		MakeTag('f', 'i', 'n', '3'), // fin3
		MakeTag('m', 'e', 'd', 'i'), // medi
		MakeTag('m', 'e', 'd', '2'), // med2
		MakeTag('i', 'n', 'i', 't'), // init
		MakeTag('l', 't', 'r', 'a'), // ltra (direction feature)
		MakeTag('l', 't', 'r', 'm'), // ltrm (direction feature)
		MakeTag('r', 't', 'l', 'a'), // rtla (direction feature)
		MakeTag('r', 't', 'l', 'm'): // rtlm (direction feature)
		return true
	}
	return false
}

// isArabicProper returns true if the buffer contains Arabic (not Mongolian/Syriac/etc).
// HarfBuzz: plan->props.script == HB_SCRIPT_ARABIC
// This determines whether there's a pause between rlig and calt.
func (s *Shaper) isArabicProper(buf *Buffer) bool {
	// Check buffer's detected script tag (OpenType 'arab')
	arabicTag := MakeTag('a', 'r', 'a', 'b')
	if buf.Script == arabicTag {
		return true
	}
	// Fallback: check first non-transparent codepoint
	for _, info := range buf.Info {
		cp := info.Codepoint
		// Arabic proper ranges (not Mongolian, Syriac, Phags-Pa)
		if (cp >= 0x0600 && cp <= 0x06FF) || // Arabic
			(cp >= 0x0750 && cp <= 0x077F) || // Arabic Supplement
			(cp >= 0x08A0 && cp <= 0x08FF) || // Arabic Extended-A
			(cp >= 0xFB50 && cp <= 0xFDFF) || // Arabic Presentation Forms-A
			(cp >= 0xFE70 && cp <= 0xFEFF) { // Arabic Presentation Forms-B
			return true
		}
		// If we see Mongolian, Syriac, or Phags-Pa first, it's not Arabic proper
		if (cp >= 0x1800 && cp <= 0x18AF) || // Mongolian
			(cp >= 0x0700 && cp <= 0x074F) || // Syriac
			(cp >= 0xA840 && cp <= 0xA877) { // Phags-Pa
			return false
		}
	}
	return false
}

// applyRligCaltLigaTogether applies rlig, calt, and liga features together
// using OT Map with lookups sorted by index.
// HarfBuzz: For non-Arabic scripts (like Mongolian), there's no pause between
// these features, so all lookups are sorted by index and applied in that order.
func (s *Shaper) applyRligCaltLigaTogether(buf *Buffer) {
	featureList, err := s.gsub.ParseFeatureList()
	if err != nil {
		// Fallback to separate application
		s.gsub.ApplyFeatureToBufferWithMask(tagRlig, buf, s.gdef, MaskGlobal, s.font)
		s.gsub.ApplyFeatureToBufferWithMask(tagCalt, buf, s.gdef, MaskGlobal, s.font)
		s.gsub.ApplyFeatureToBufferWithMask(tagLiga, buf, s.gdef, MaskGlobal, s.font)
		return
	}

	// Build OT Map with rlig, calt, liga features
	otMap := NewOTMap()

	ligatureFeatures := []Tag{tagRlig, tagCalt, tagLiga}
	for _, tag := range ligatureFeatures {
		lookups := featureList.FindFeature(tag)
		for _, lookupIdx := range lookups {
			otMap.AddGSUBLookup(lookupIdx, MaskGlobal, tag)
		}
	}

	// Sort lookups by index and deduplicate
	// This ensures lookup 9 is applied before lookup 10, regardless of which
	// feature (rlig or calt) references them.
	otMap.GSUBLookups = deduplicateLookups(otMap.GSUBLookups)

	// Apply all lookups in sorted order
	otMap.ApplyGSUB(s.gsub, buf, s.font, s.gdef)
}

// getGlyphClass returns the GDEF glyph class for a glyph, or 0 if not available.
func (s *Shaper) getGlyphClass(glyph GlyphID) int {
	if s.gdef != nil && s.gdef.HasGlyphClasses() {
		return s.gdef.GetGlyphClass(glyph)
	}
	return 0
}

// isArabicScript returns true if the codepoint is in an Arabic-like script.
func isArabicScript(cp Codepoint) bool {
	// Arabic: U+0600-U+06FF
	if cp >= 0x0600 && cp <= 0x06FF {
		return true
	}
	// Arabic Supplement: U+0750-U+077F
	if cp >= 0x0750 && cp <= 0x077F {
		return true
	}
	// Arabic Extended-A: U+08A0-U+08FF
	if cp >= 0x08A0 && cp <= 0x08FF {
		return true
	}
	// Arabic Presentation Forms-A: U+FB50-U+FDFF
	if cp >= 0xFB50 && cp <= 0xFDFF {
		return true
	}
	// Arabic Presentation Forms-B: U+FE70-U+FEFF
	if cp >= 0xFE70 && cp <= 0xFEFF {
		return true
	}
	// Syriac: U+0700-U+074F
	if cp >= 0x0700 && cp <= 0x074F {
		return true
	}
	// Phags-pa: U+A840-U+A877 (has Arabic-like joining)
	if cp >= 0xA840 && cp <= 0xA877 {
		return true
	}
	// Mongolian: U+1800-U+18AF (has Arabic-like joining)
	// Note: Mongolian uses the same joining model as Arabic with isol/init/medi/fina
	if cp >= 0x1800 && cp <= 0x18AF {
		return true
	}
	return false
}

// clearPositionalMasksFromMarks removes positional masks from mark glyphs.
// After ccmp, marks may inherit positional masks from their base glyphs,
// but they should only have MaskGlobal.
// HarfBuzz equivalent: This is implicit in HarfBuzz because setupMasks happens after GSUB.
func (s *Shaper) clearPositionalMasksFromMarks(buf *Buffer) {
	// Positional masks to clear
	positionalMasks := MaskIsol | MaskFina | MaskFin2 | MaskFin3 | MaskMedi | MaskMed2 | MaskInit

	for i := range buf.Info {
		// Check if this glyph is a mark (GDEF class 3)
		gclass := 0
		if s.gdef != nil {
			gclass = s.gdef.GetGlyphClass(buf.Info[i].GlyphID)
		}

		if gclass == GlyphClassMark {
			// Clear positional masks, keep only MaskGlobal
			buf.Info[i].Mask &^= positionalMasks
		}
	}
}
