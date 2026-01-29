package ot

// Arabic Shaper implementation
//
// HarfBuzz equivalent: hb-ot-shaper-arabic.cc
//
// This file implements the OTShaper interface for Arabic and related scripts.

func init() {
	// Initialize Arabic shaper functions
	// HarfBuzz equivalent: _hb_ot_shaper_arabic in hb-ot-shaper-arabic.cc:752-768
	ArabicShaper.CollectFeatures = collectFeaturesArabic
	ArabicShaper.SetupMasks = setupMasksArabicShaper
	ArabicShaper.ReorderMarks = reorderMarksArabic
}

// collectFeaturesArabic adds Arabic-specific features to the plan.
// HarfBuzz equivalent: collect_features_arabic() in hb-ot-shaper-arabic.cc:366-420
func collectFeaturesArabic(plan *ShapePlan) {
	// Arabic features in order of application
	// HarfBuzz: arabic_features[] in hb-ot-shaper-arabic.cc:44-59

	// Localized forms - applied to all glyphs
	plan.Map.AddGSUBFeature(MakeTag('l', 'o', 'c', 'l'), MaskGlobal, true)

	// Positional features - each has its own mask
	plan.Map.AddGSUBFeature(MakeTag('i', 's', 'o', 'l'), MaskIsol, false)
	plan.Map.AddGSUBFeature(MakeTag('f', 'i', 'n', 'a'), MaskFina, false)
	plan.Map.AddGSUBFeature(MakeTag('f', 'i', 'n', '2'), MaskFin2, false)
	plan.Map.AddGSUBFeature(MakeTag('f', 'i', 'n', '3'), MaskFin3, false)
	plan.Map.AddGSUBFeature(MakeTag('m', 'e', 'd', 'i'), MaskMedi, false)
	plan.Map.AddGSUBFeature(MakeTag('m', 'e', 'd', '2'), MaskMed2, false)
	plan.Map.AddGSUBFeature(MakeTag('i', 'n', 'i', 't'), MaskInit, false)

	// Required ligatures - applied to all glyphs
	plan.Map.AddGSUBFeature(MakeTag('r', 'l', 'i', 'g'), MaskGlobal, true)

	// Required contextual alternates
	plan.Map.AddGSUBFeature(MakeTag('r', 'c', 'l', 't'), MaskGlobal, true)

	// Contextual alternates
	plan.Map.AddGSUBFeature(MakeTag('c', 'a', 'l', 't'), MaskGlobal, true)

	// Standard ligatures (optional)
	plan.Map.AddGSUBFeature(MakeTag('l', 'i', 'g', 'a'), MaskGlobal, false)

	// Discretionary ligatures (optional)
	plan.Map.AddGSUBFeature(MakeTag('d', 'l', 'i', 'g'), MaskGlobal, false)

	// Contextual swash (optional)
	plan.Map.AddGSUBFeature(MakeTag('c', 's', 'w', 'h'), MaskGlobal, false)

	// Mark positioning features
	plan.Map.AddGPOSFeature(MakeTag('c', 'u', 'r', 's'), MaskGlobal, true)
	plan.Map.AddGPOSFeature(MakeTag('k', 'e', 'r', 'n'), MaskGlobal, false)
	plan.Map.AddGPOSFeature(MakeTag('m', 'a', 'r', 'k'), MaskGlobal, true)
	plan.Map.AddGPOSFeature(MakeTag('m', 'k', 'm', 'k'), MaskGlobal, true)
}

// setupMasksArabicShaper sets up masks for Arabic shaping.
// HarfBuzz equivalent: setup_masks_arabic() in hb-ot-shaper-arabic.cc:462-490
func setupMasksArabicShaper(plan *ShapePlan, buf *Buffer, font *Font) {
	// Step 1: Initialize global mask on all glyphs
	buf.ResetMasks(MaskGlobal)

	// Step 2: Run Arabic joining analysis and set positional masks
	// HarfBuzz: arabic_joining() then setup_masks_arabic_plan()
	actions := arabicJoiningAnalysis(buf)

	// Step 3: Set masks based on joining actions
	// HarfBuzz: setup_masks_arabic_plan() in hb-ot-shaper-arabic.cc:396-420
	for i, action := range actions {
		mask := arabicActionToMask(action)
		if mask != 0 {
			buf.Info[i].Mask |= mask
		}
	}
}

// arabicJoiningAnalysis performs Arabic joining analysis on the buffer.
// Returns the action for each glyph.
// HarfBuzz equivalent: arabic_joining() in hb-ot-shaper-arabic.cc:279-354
func arabicJoiningAnalysis(buf *Buffer) []ArabicAction {
	if len(buf.Info) == 0 {
		return nil
	}

	actions := make([]ArabicAction, len(buf.Info))

	// Initialize state machine
	// HarfBuzz: state starts at 0, prev_action at NONE
	state := uint8(0)
	prevAction := arabicActionNone

	// Process each character
	for i := 0; i < len(buf.Info); i++ {
		thisType := getJoiningType(buf.Info[i].Codepoint, getGeneralCategory(buf.Info[i].Codepoint))

		// Handle transparent characters (marks)
		if thisType == joiningTypeT {
			actions[i] = arabicActionNone
			continue
		}

		// Get action from state machine
		entry := arabicStateTable[state][joiningTypeColumn(thisType)]
		actions[i] = entry.currAction

		// Update previous character's action if needed
		if prevAction != arabicActionNone && entry.prevAction != arabicActionNone {
			for j := i - 1; j >= 0; j-- {
				if actions[j] != arabicActionNone {
					actions[j] = entry.prevAction
					break
				}
			}
		}

		prevAction = entry.currAction
		state = entry.nextState
	}

	return actions
}

// Arabic Modifier Combining Marks (MCM) list.
// HarfBuzz equivalent: modifier_combining_marks[] in hb-ot-shaper-arabic.cc:658-674
// Reference: https://www.unicode.org/reports/tr53/
var arabicModifierCombiningMarks = []Codepoint{
	0x0654, // ARABIC HAMZA ABOVE
	0x0655, // ARABIC HAMZA BELOW
	0x0658, // ARABIC MARK NOON GHUNNA
	0x06DC, // ARABIC SMALL HIGH SEEN
	0x06E3, // ARABIC SMALL LOW SEEN
	0x06E7, // ARABIC SMALL HIGH YEH
	0x06E8, // ARABIC SMALL HIGH NOON
	0x08CA, // ARABIC SMALL HIGH FARSI YEH
	0x08CB, // ARABIC SMALL HIGH YEH BARREE WITH TWO DOTS BELOW
	0x08CD, // ARABIC SMALL HIGH ZAH
	0x08CE, // ARABIC LARGE ROUND DOT ABOVE
	0x08CF, // ARABIC LARGE ROUND DOT BELOW
	0x08D3, // ARABIC SMALL LOW WAW
	0x08F3, // ARABIC SMALL HIGH WAW
}

// isArabicMCM checks if a codepoint is an Arabic Modifier Combining Mark.
// HarfBuzz equivalent: info_is_mcm() in hb-ot-shaper-arabic.cc:676-684
func isArabicMCM(cp Codepoint) bool {
	for _, mcm := range arabicModifierCombiningMarks {
		if cp == mcm {
			return true
		}
	}
	return false
}

// reorderMarksArabic reorders Arabic combining marks.
// HarfBuzz equivalent: reorder_marks_arabic() in hb-ot-shaper-arabic.cc:686-750
//
// This function moves Modifier Combining Marks (MCM) with combining class 220 or 230
// to the beginning of the mark sequence. This is required for correct Arabic
// mark rendering, as MCMs like HAMZA ABOVE/BELOW should be positioned before
// other marks with the same combining class.
//
// After reordering, the MCMs get their combining class changed to 22 (for CC 220)
// or 26 (for CC 230) to maintain sorted order for subsequent processing.
//
// Note: This version is kept for OTShaper interface compatibility.
func reorderMarksArabic(plan *ShapePlan, buf *Buffer, start, end int) {
	reorderMarksArabicSlice(buf.Info, start, end)
}

// reorderMarksArabicSlice is the slice-based version of reorderMarksArabic.
// Used during normalization when working with temporary slices.
// HarfBuzz equivalent: reorder_marks_arabic() in hb-ot-shaper-arabic.cc:686-750
func reorderMarksArabicSlice(info []GlyphInfo, start, end int) {
	// Note: We process even single-element sequences because we need to
	// set ModifiedCCC for Arabic MCMs regardless of reordering.
	// HarfBuzz does NOT have an early return here.
	if end <= start {
		return
	}

	i := start
	// Process combining classes 220 (below) and 230 (above)
	for cc := uint8(220); cc <= 230; cc += 10 {
		// Skip marks with lower combining class
		for i < end && getModifiedCombiningClass(info[i].Codepoint) < cc {
			i++
		}

		if i == end {
			break
		}

		// Skip if current mark has higher combining class
		if getModifiedCombiningClass(info[i].Codepoint) > cc {
			continue
		}

		// Find range of MCMs with this combining class
		j := i
		for j < end && getModifiedCombiningClass(info[j].Codepoint) == cc && isArabicMCM(info[j].Codepoint) {
			j++
		}

		if i == j {
			continue
		}

		// Shift the MCMs to the beginning of the mark sequence
		// HarfBuzz: memmove pattern in reorder_marks_arabic lines 723-726
		mergeClustersSlice(info, start, j)

		// Save MCMs
		mcmCount := j - i
		temp := make([]GlyphInfo, mcmCount)
		copy(temp, info[i:j])

		// Shift other marks forward
		copy(info[start+mcmCount:], info[start:i])

		// Place MCMs at the beginning
		copy(info[start:], temp)

		// Renumber combining class for the moved MCMs
		// HarfBuzz: uses 25 for CC 220, 26 for CC 230
		// These values are smaller than all Arabic categories and will be
		// folded back to 220/230 during fallback mark positioning.
		// This renumbering is critical for CGJ handling in unhideCGJ().
		newStart := start + mcmCount
		var newCC uint8
		if cc == 220 {
			newCC = 25 // HB_MODIFIED_COMBINING_CLASS_CCC22 = 25
		} else {
			newCC = 26 // HB_MODIFIED_COMBINING_CLASS_CCC26 = 26
		}

		// Update the modified combining class for the moved MCMs
		// HarfBuzz: _hb_glyph_info_set_modified_combining_class() lines 742-746
		for k := start; k < newStart; k++ {
			info[k].ModifiedCCC = newCC
		}

		i = j
	}
}

// --- Helper functions ---

// AddGSUBFeature adds a GSUB feature to the map with the given mask.
// The 'required' parameter indicates if this feature is mandatory.
func (m *OTMap) AddGSUBFeature(tag Tag, mask uint32, required bool) {
	// For now, we just add to the existing GSUBLookups
	// A full implementation would use the FeatureList to find lookups
	_ = required // Will be used when we have full feature compilation
	m.featureRequests = append(m.featureRequests, featureRequest{
		tag:       tag,
		mask:      mask,
		tableType: 0, // GSUB
	})
}

// AddGPOSFeature adds a GPOS feature to the map with the given mask.
func (m *OTMap) AddGPOSFeature(tag Tag, mask uint32, required bool) {
	_ = required
	m.featureRequests = append(m.featureRequests, featureRequest{
		tag:       tag,
		mask:      mask,
		tableType: 1, // GPOS
	})
}

// featureRequest represents a request to add a feature to the map.
type featureRequest struct {
	tag       Tag
	mask      uint32
	tableType int // 0=GSUB, 1=GPOS
}

// --- STCH (Stretching) Feature Implementation ---
// HarfBuzz equivalent: hb-ot-shaper-arabic.cc:445-644

// recordStch records glyphs that need STCH treatment.
// HarfBuzz equivalent: record_stch() in hb-ot-shaper-arabic.cc:452-476
func recordStch(buf *Buffer) {
	for i := range buf.Info {
		info := &buf.Info[i]
		if (info.GlyphProps & GlyphPropsMultiplied) == 0 {
			continue
		}
		comp := info.GetLigComp()
		if comp%2 != 0 {
			info.ArabicShapingAction = uint8(arabicActionSTCH_REPEATING)
		} else {
			info.ArabicShapingAction = uint8(arabicActionSTCH_FIXED)
		}
		buf.ScratchFlags |= ScratchFlagArabicHasStch
	}
}

// applyStch applies STCH stretching to the buffer.
// HarfBuzz equivalent: apply_stch() in hb-ot-shaper-arabic.cc:478-644
func applyStch(buf *Buffer, shaper *Shaper) {
	if (buf.ScratchFlags & ScratchFlagArabicHasStch) == 0 {
		return
	}

	rtl := buf.Direction == DirectionRTL

	// For LTR, reverse the buffer to visual order (RTL is already reversed by shapeArabic)
	// HarfBuzz: if (!plan->rtl) buffer->reverse();
	if !rtl {
		buf.Reverse()
	}

	sign := int32(1)
	var extraGlyphsNeeded int
	originalCount := len(buf.Info)

	const (
		stepMEASURE = 0
		stepCUT     = 1
	)

	for step := stepMEASURE; step <= stepCUT; step++ {
		count := originalCount
		newLen := count + extraGlyphsNeeded
		j := newLen

		for i := count; i > 0; i-- {
			info := &buf.Info[i-1]
			action := ArabicAction(info.ArabicShapingAction)

			if action != arabicActionSTCH_FIXED && action != arabicActionSTCH_REPEATING {
				if step == stepCUT {
					j--
					buf.Info[j] = buf.Info[i-1]
					buf.Pos[j] = buf.Pos[i-1]
				}
				continue
			}

			var wTotal, wFixed, wRepeating int32
			var nFixed, nRepeating int

			end := i
			for i > 0 {
				info := &buf.Info[i-1]
				action := ArabicAction(info.ArabicShapingAction)
				if action != arabicActionSTCH_FIXED && action != arabicActionSTCH_REPEATING {
					break
				}
				i--
				width := int32(shaper.getGlyphHAdvance(buf.Info[i].GlyphID))
				if action == arabicActionSTCH_FIXED {
					wFixed += width
					nFixed++
				} else {
					wRepeating += width
					nRepeating++
				}
			}
			start := i
			context := i

			for context > 0 {
				info := &buf.Info[context-1]
				action := ArabicAction(info.ArabicShapingAction)
				if action == arabicActionSTCH_FIXED || action == arabicActionSTCH_REPEATING {
					break
				}
				if !IsDefaultIgnorable(info.Codepoint) && !isArabicGeneralCategoryWord(info.Codepoint) {
					break
				}
				context--
				wTotal += int32(buf.Pos[context].XAdvance)
			}
			i++

			var nCopies int
			wRemaining := wTotal - wFixed
			if sign*wRemaining > sign*wRepeating && sign*wRepeating > 0 {
				nCopies = int(sign*wRemaining)/int(sign*wRepeating) - 1
			}

			var extraRepeatOverlap int32
			shortfall := sign*wRemaining - sign*wRepeating*int32(nCopies+1)
			if shortfall > 0 && nRepeating > 0 {
				nCopies++
				excess := int32(nCopies+1)*sign*wRepeating - sign*wRemaining
				if excess > 0 {
					extraRepeatOverlap = excess / int32(nCopies*nRepeating)
					wRemaining = 0
				}
			}

			if step == stepMEASURE {
				extraGlyphsNeeded += nCopies * nRepeating
			} else {
				xOffset := wRemaining / 2

				for k := end; k > start; k-- {
					info := &buf.Info[k-1]
					width := int32(shaper.getGlyphHAdvance(info.GlyphID))

					repeat := 1
					if ArabicAction(info.ArabicShapingAction) == arabicActionSTCH_REPEATING {
						repeat += nCopies
					}

					buf.Pos[k-1].XAdvance = 0
					for n := 0; n < repeat; n++ {
						if rtl {
							xOffset -= width
							if n > 0 {
								xOffset += extraRepeatOverlap
							}
						}
						j--
						buf.Info[j] = buf.Info[k-1]
						buf.Pos[j] = buf.Pos[k-1]
						buf.Pos[j].XOffset = int16(xOffset)
						if !rtl {
							xOffset += width
							if n > 0 {
								xOffset -= extraRepeatOverlap
							}
						}
					}
				}
			}
		}

		if step == stepMEASURE && extraGlyphsNeeded > 0 {
			newCapacity := len(buf.Info) + extraGlyphsNeeded
			if cap(buf.Info) < newCapacity {
				newInfo := make([]GlyphInfo, len(buf.Info), newCapacity)
				newPos := make([]GlyphPos, len(buf.Pos), newCapacity)
				copy(newInfo, buf.Info)
				copy(newPos, buf.Pos)
				buf.Info = newInfo
				buf.Pos = newPos
			}
			buf.Info = buf.Info[:newCapacity]
			buf.Pos = buf.Pos[:newCapacity]
		} else if step == stepCUT {
			buf.Info = buf.Info[:newLen]
			buf.Pos = buf.Pos[:newLen]
		}
	}

	// Reverse back for LTR
	// HarfBuzz: if (!plan->rtl) buffer->reverse();
	if !rtl {
		buf.Reverse()
	}
}

// getGlyphHAdvance returns the horizontal advance for a glyph.
func (s *Shaper) getGlyphHAdvance(glyph GlyphID) int16 {
	if s.hmtx != nil {
		return int16(s.hmtx.GetAdvanceWidth(glyph))
	}
	return 0
}

// isArabicGeneralCategoryWord checks if codepoint is a "word" character for STCH context.
func isArabicGeneralCategoryWord(cp Codepoint) bool {
	gc := getGeneralCategory(cp)
	switch gc {
	case GCUnassigned, GCPrivateUse, GCModifierLetter, GCOtherLetter,
		GCSpacingMark, GCEnclosingMark, GCNonSpacingMark,
		GCDecimalNumber, GCLetterNumber, GCOtherNumber,
		GCCurrencySymbol, GCModifierSymbol, GCMathSymbol, GCOtherSymbol:
		return true
	}
	return false
}

// postprocessGlyphsArabic performs Arabic-specific post-processing after GPOS.
func postprocessGlyphsArabic(buf *Buffer, shaper *Shaper) {
	applyStch(buf, shaper)
}
