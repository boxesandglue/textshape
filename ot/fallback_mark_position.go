package ot

// Fallback Mark Positioning
//
// This provides fallback positioning for combining marks when GPOS is not available.
// It uses glyph extents and Unicode Combining Class to position marks relative to base glyphs.
//
// Source: HarfBuzz hb-ot-shape-fallback.cc

// Unicode Combining Class values used for positioning
// Source: Unicode Standard, Chapter 4.3
const (
	cccNotReordered       uint8 = 0
	cccOverlay            uint8 = 1
	cccNukta              uint8 = 7
	cccKanaVoicing        uint8 = 8
	cccVirama             uint8 = 9
	cccAttachedBelowLeft  uint8 = 200
	cccAttachedBelow      uint8 = 202
	cccAttachedAbove      uint8 = 214
	cccAttachedAboveRight uint8 = 216
	cccBelowLeft          uint8 = 218
	cccBelow              uint8 = 220
	cccBelowRight         uint8 = 222
	cccLeft               uint8 = 224
	cccRight              uint8 = 226
	cccAboveLeft          uint8 = 228
	cccAbove              uint8 = 230
	cccAboveRight         uint8 = 232
	cccDoubleBelow        uint8 = 233
	cccDoubleAbove        uint8 = 234
	cccIotaSubscript      uint8 = 240
)

// fallbackMarkPosition performs fallback mark positioning on a buffer.
// This is called when GPOS mark positioning is not available.
// Source: HarfBuzz _hb_ot_shape_fallback_mark_position() in hb-ot-shape-fallback.cc:456-483
func (s *Shaper) fallbackMarkPosition(buf *Buffer) {
	if s.glyf == nil || s.hmtx == nil {
		return
	}

	// Process each cluster
	start := 0
	for i := 1; i < len(buf.Info); i++ {
		// Start new cluster at non-mark glyphs
		if getModifiedCombiningClass(buf.Info[i].Codepoint) == 0 {
			s.positionCluster(buf, start, i)
			start = i
		}
	}
	// Process final cluster
	s.positionCluster(buf, start, len(buf.Info))
}

// positionCluster positions marks within a single cluster.
// Source: HarfBuzz position_cluster() in hb-ot-shape-fallback.cc:441-453
func (s *Shaper) positionCluster(buf *Buffer, start, end int) {
	if end-start < 2 {
		return
	}
	s.positionClusterImpl(buf, start, end)
}

// positionClusterImpl implements the cluster positioning logic.
// Source: HarfBuzz position_cluster_impl() in hb-ot-shape-fallback.cc:411-439
func (s *Shaper) positionClusterImpl(buf *Buffer, start, end int) {
	// Find base glyphs and their following marks
	for i := start; i < end; i++ {
		if getCombiningClass(buf.Info[i].Codepoint) == 0 {
			// Found a base glyph - find its marks
			j := i + 1
			for j < end {
				// Skip hidden and default-ignorable glyphs
				// TODO: implement isHidden and isDefaultIgnorable checks
				if getCombiningClass(buf.Info[j].Codepoint) == 0 {
					break
				}
				j++
			}

			// Position marks around this base
			s.positionAroundBaseImpl(buf, i, j)

			// Continue after the marks
			i = j - 1
		}
	}
}

// positionAroundBaseImpl positions marks around a base glyph.
// Source: HarfBuzz position_around_base() in hb-ot-shape-fallback.cc:315-409
func (s *Shaper) positionAroundBaseImpl(buf *Buffer, base, end int) {
	// Get base extents
	baseExtents, ok := s.glyf.GetGlyphExtents(buf.Info[base].GlyphID)
	if !ok {
		// If no extents, zero mark advances and return
		s.zeroMarkAdvances(buf, base+1, end)
		return
	}

	// HarfBuzz line 335: base_extents.y_bearing += buffer->pos[base].y_offset;
	baseExtents.YBearing += buf.Pos[base].YOffset

	// Use horizontal advance for width (generally better, works for zero-ink glyphs)
	// HarfBuzz lines 339-340
	baseExtents.XBearing = 0
	baseExtents.Width = int16(s.hmtx.GetAdvanceWidth(buf.Info[base].GlyphID))

	// Position marks around the base
	s.positionAroundBase(buf, base, end, baseExtents)
}

// positionAroundBase positions all marks around a base glyph.
// Source: HarfBuzz position_around_base() in hb-ot-shape-fallback.cc:315-409
func (s *Shaper) positionAroundBase(buf *Buffer, base, end int, baseExtents GlyphExtents) {
	// Get ligature info from base
	// HarfBuzz lines 342-345
	ligID := buf.Info[base].GetLigID()
	numLigComponents := buf.Info[base].GetLigNumComps()

	// Calculate x_offset and y_offset based on direction
	// HarfBuzz lines 347-351
	xOffset := int16(0)
	yOffset := int16(0)
	if buf.Direction == DirectionLTR || buf.Direction == DirectionTTB {
		xOffset = -buf.Pos[base].XAdvance
		yOffset = -buf.Pos[base].YAdvance
	}

	// Determine horizontal direction for ligature component positioning
	// HarfBuzz lines 372-377
	horizDir := buf.Direction
	if !buf.Direction.IsHorizontal() {
		// For vertical text, use script's horizontal direction
		// Simplified: default to LTR for most scripts, RTL for Arabic/Hebrew
		horizDir = DirectionLTR // TODO: Use script-specific direction
	}

	// Track component and CCC for stacking
	// HarfBuzz lines 353-356
	componentExtents := baseExtents
	lastLigComponent := -1
	lastCCC := uint8(255)
	clusterExtents := baseExtents

	// Position each mark after the base
	// HarfBuzz lines 358-408
	for i := base + 1; i < end; i++ {
		ccc := getModifiedCombiningClass(buf.Info[i].Codepoint)
		if ccc != 0 {
			// Handle ligature components
			// HarfBuzz lines 361-383
			if numLigComponents > 1 {
				thisLigID := buf.Info[i].GetLigID()
				thisLigComponent := int(buf.Info[i].GetLigComp()) - 1

				// Conditions for attaching to the last component
				if ligID == 0 || ligID != thisLigID || thisLigComponent >= numLigComponents {
					thisLigComponent = numLigComponents - 1
				}

				if lastLigComponent != thisLigComponent {
					lastLigComponent = thisLigComponent
					lastCCC = 255
					componentExtents = baseExtents

					// Adjust extents for this component
					if horizDir == DirectionLTR {
						componentExtents.XBearing += int16(thisLigComponent * int(componentExtents.Width) / numLigComponents)
					} else {
						componentExtents.XBearing += int16((numLigComponents - 1 - thisLigComponent) * int(componentExtents.Width) / numLigComponents)
					}
					componentExtents.Width /= int16(numLigComponents)
				}
			}

			// Handle same-CCC stacking
			// HarfBuzz lines 386-391
			thisCCC := recategorizeCCC(ccc)
			if lastCCC != thisCCC {
				lastCCC = thisCCC
				clusterExtents = componentExtents
			}

			// Position this mark (note: clusterExtents is modified by positionMark for stacking)
			s.positionMark(buf, &clusterExtents, i, ccc)

			// Zero the mark's advance (HarfBuzz lines 395-396)
			buf.Pos[i].XAdvance = 0
			buf.Pos[i].YAdvance = 0

			// Add global offsets (HarfBuzz lines 397-398)
			buf.Pos[i].XOffset += xOffset
			buf.Pos[i].YOffset += yOffset
		} else {
			// Not a mark - update offsets for subsequent marks
			// HarfBuzz lines 400-407
			if buf.Direction == DirectionLTR || buf.Direction == DirectionTTB {
				xOffset -= buf.Pos[i].XAdvance
				yOffset -= buf.Pos[i].YAdvance
			} else {
				xOffset += buf.Pos[i].XAdvance
				yOffset += buf.Pos[i].YAdvance
			}
		}
	}
}

// Modified CCC constants matching HarfBuzz HB_MODIFIED_COMBINING_CLASS_CCC*
// These are the OUTPUT values from modifiedCombiningClass table in normalize.go
// Source: hb-unicode.hh lines 333-401
const (
	// Hebrew (CCC 10-26)
	modCCC10 = 22 // sheva
	modCCC11 = 15 // hataf segol
	modCCC12 = 16 // hataf patah
	modCCC13 = 17 // hataf qamats
	modCCC14 = 23 // hiriq
	modCCC15 = 18 // tsere
	modCCC16 = 19 // segol
	modCCC17 = 20 // patah
	modCCC18 = 21 // qamats & qamats qatan
	modCCC19 = 14 // holam & holam haser for vav
	modCCC20 = 24 // qubuts
	modCCC21 = 12 // dagesh
	modCCC22 = 25 // meteg
	modCCC23 = 13 // rafe
	modCCC24 = 10 // shin dot
	modCCC25 = 11 // sin dot
	modCCC26 = 26 // point varika

	// Arabic (CCC 27-35)
	modCCC27 = 28 // fathatan
	modCCC28 = 29 // dammatan
	modCCC29 = 30 // kasratan
	modCCC30 = 31 // fatha
	modCCC31 = 32 // damma
	modCCC32 = 33 // kasra
	modCCC33 = 27 // shadda (reordered to come first!)
	modCCC34 = 34 // sukun
	modCCC35 = 35 // superscript alef

	// Syriac (CCC 36)
	modCCC36 = 36 // superscript alaph

	// Thai (CCC 103, 107)
	modCCC103 = 3   // sara u / sara uu
	modCCC107 = 107 // mai *

	// Lao (CCC 118, 122)
	modCCC118 = 118 // sign u / sign uu
	modCCC122 = 122 // mai *

	// Tibetan (CCC 129, 130, 132)
	modCCC129 = 129 // sign aa
	modCCC130 = 132 // sign i (swapped with 132)
	modCCC132 = 131 // sign u (swapped with 130)
)

// recategorizeCCC maps modified CCCs to standard positional categories.
// Source: HarfBuzz recategorize_marks() in hb-ot-shape-fallback.cc:82-166
// Input is the MODIFIED combining class from modifiedCombiningClass table.
func recategorizeCCC(modCCC uint8) uint8 {
	switch modCCC {
	// Hebrew - Below marks
	case modCCC10, modCCC11, modCCC12, modCCC13, modCCC14, modCCC15, modCCC16, modCCC17, modCCC18, modCCC20, modCCC22:
		return cccBelow
	// Hebrew - Attached above
	case modCCC23: // rafe
		return cccAttachedAbove
	// Hebrew - Above right
	case modCCC24: // shin dot
		return cccAboveRight
	// Hebrew - Above left
	case modCCC25, modCCC19: // sin dot, holam
		return cccAboveLeft
	// Hebrew - Above
	case modCCC26: // point varika
		return cccAbove
	// Hebrew - dagesh (no recategorization, break in HarfBuzz)
	case modCCC21:
		return modCCC

	// Arabic and Syriac - Above marks
	case modCCC27, modCCC28, modCCC30, modCCC31, modCCC33, modCCC34, modCCC35, modCCC36:
		return cccAbove

	// Arabic and Syriac - Below marks
	case modCCC29, modCCC32:
		return cccBelow

	// Thai
	case modCCC103: // sara u / sara uu
		return cccBelowRight
	case modCCC107: // mai
		return cccAboveRight

	// Lao
	case modCCC118: // sign u / sign uu
		return cccBelow
	case modCCC122: // mai
		return cccAbove

	// Tibetan
	case modCCC129: // sign aa
		return cccBelow
	case modCCC130: // sign i
		return cccAbove
	case modCCC132: // sign u
		return cccBelow

	// Standard CCCs (220=Below, 230=Above)
	case cccBelow:
		return cccBelow
	case cccAbove:
		return cccAbove
	}
	return modCCC
}

// positionMark positions a single mark relative to its base.
// Source: HarfBuzz position_mark() in hb-ot-shape-fallback.cc:208-313
func (s *Shaper) positionMark(buf *Buffer, baseExtents *GlyphExtents, i int, ccc uint8) {
	markExtents, ok := s.glyf.GetGlyphExtents(buf.Info[i].GlyphID)
	if !ok {
		return
	}

	// Recategorize CCC for proper Y positioning
	// HarfBuzz: _hb_ot_shape_fallback_mark_position_recategorize_marks()
	posCCC := recategorizeCCC(ccc)

	// Y gap (typically 1/16 of upem)
	yGap := int16(s.face.Upem() / 16)

	pos := &buf.Pos[i]
	pos.XOffset = 0
	pos.YOffset = 0

	// X positioning based on combining class
	switch ccc {
	case cccDoubleBelow, cccDoubleAbove:
		// Position based on direction
		if buf.Direction == DirectionLTR {
			pos.XOffset = baseExtents.XBearing + baseExtents.Width - markExtents.Width/2 - markExtents.XBearing
		} else if buf.Direction == DirectionRTL {
			pos.XOffset = baseExtents.XBearing - markExtents.Width/2 - markExtents.XBearing
		} else {
			// Center align for vertical text
			pos.XOffset = baseExtents.XBearing + (baseExtents.Width-markExtents.Width)/2 - markExtents.XBearing
		}

	case cccAttachedBelowLeft, cccBelowLeft, cccAboveLeft:
		// Left align
		pos.XOffset = baseExtents.XBearing - markExtents.XBearing

	case cccAttachedAboveRight, cccBelowRight, cccAboveRight:
		// Right align
		pos.XOffset = baseExtents.XBearing + baseExtents.Width - markExtents.Width - markExtents.XBearing

	default:
		// Center align (most common for Arabic marks)
		pos.XOffset = baseExtents.XBearing + (baseExtents.Width-markExtents.Width)/2 - markExtents.XBearing
	}

	// Y positioning based on recategorized combining class
	switch posCCC {
	case cccDoubleBelow, cccBelowLeft, cccBelow, cccBelowRight:
		// Add gap
		baseExtents.Height -= yGap
		fallthrough

	case cccAttachedBelowLeft, cccAttachedBelow:
		// Position below base
		pos.YOffset = baseExtents.YBearing + baseExtents.Height - markExtents.YBearing
		// Never shift up "below" marks
		if (yGap > 0) == (pos.YOffset > 0) {
			baseExtents.Height -= pos.YOffset
			pos.YOffset = 0
		}
		baseExtents.Height += markExtents.Height

	case cccDoubleAbove, cccAboveLeft, cccAbove, cccAboveRight:
		// Add gap
		baseExtents.YBearing += yGap
		baseExtents.Height -= yGap
		fallthrough

	case cccAttachedAbove, cccAttachedAboveRight:
		// Position above base
		pos.YOffset = baseExtents.YBearing - (markExtents.YBearing + markExtents.Height)
		// Don't shift down "above" marks too much
		if (yGap > 0) != (pos.YOffset > 0) {
			correction := -pos.YOffset / 2
			baseExtents.YBearing += correction
			baseExtents.Height -= correction
			pos.YOffset += correction
		}
		baseExtents.YBearing -= markExtents.Height
		baseExtents.Height += markExtents.Height
	}
}

// zeroMarkAdvances sets advance to zero for marks in the given range.
// Source: HarfBuzz zero_mark_advances() in hb-ot-shape-fallback.cc:189-206
func (s *Shaper) zeroMarkAdvances(buf *Buffer, start, end int) {
	for i := start; i < end; i++ {
		if getCombiningClass(buf.Info[i].Codepoint) != 0 {
			buf.Pos[i].XAdvance = 0
			buf.Pos[i].YAdvance = 0
		}
	}
}
