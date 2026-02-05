package ot

// Hangul Jamo Shaper
//
// HarfBuzz equivalent: hb-ot-shaper-hangul.cc
//
// This implements Hangul text shaping:
// - Jamo composition (L+V+T -> precomposed syllable if font has glyph)
// - Jamo decomposition (precomposed syllable -> L+V+T if font lacks glyph)
// - Feature masking for ljmo/vjmo/tjmo

// Hangul Jamo constants.
// HarfBuzz equivalent: hb-ot-shaper-hangul.cc:35-43
const (
	lBase  Codepoint = 0x1100
	vBase  Codepoint = 0x1161
	tBase  Codepoint = 0x11A7
	sBase  Codepoint = 0xAC00
	lCount           = 19
	vCount           = 21
	tCount           = 28
	nCount           = vCount * tCount // 588
	sCount           = lCount * nCount // 11172
)

// Hangul feature values for GlyphInfo.HangulFeature
const (
	hangulJMO  uint8 = 0 // No feature
	hangulLJMO uint8 = 1 // Leading Jamo
	hangulVJMO uint8 = 2 // Vowel Jamo
	hangulTJMO uint8 = 3 // Trailing Jamo
)

// Range checks for Hangul characters.
// HarfBuzz: isCombining* are the narrow composable ranges used for LVT composition.
// isL/isV/isT are the wider ranges including Old Hangul Jamo, used for feature tagging.
// HarfBuzz equivalent: hb-ot-shaper-hangul.cc:45-80

// isCombiningL returns true for the 19 composable Leading Jamo (U+1100..U+1112).
func isCombiningL(u Codepoint) bool {
	return u >= 0x1100 && u <= 0x1112
}

// isCombiningV returns true for the 21 composable Vowel Jamo (U+1161..U+1175).
func isCombiningV(u Codepoint) bool {
	return u >= 0x1161 && u <= 0x1175
}

// isCombiningT returns true for the 27 composable Trailing Jamo (U+11A8..U+11C2).
func isCombiningT(u Codepoint) bool {
	return u >= 0x11A8 && u <= 0x11C2
}

// isL returns true for all Leading Jamo (U+1100..U+115F, U+A960..U+A97C).
func isL(u Codepoint) bool {
	return (u >= 0x1100 && u <= 0x115F) || (u >= 0xA960 && u <= 0xA97C)
}

// isV returns true for all Vowel Jamo (U+1160..U+11A7, U+D7B0..U+D7C6).
func isV(u Codepoint) bool {
	return (u >= 0x1160 && u <= 0x11A7) || (u >= 0xD7B0 && u <= 0xD7C6)
}

// isT returns true for all Trailing Jamo (U+11A8..U+11FF, U+D7CB..U+D7FB).
func isT(u Codepoint) bool {
	return (u >= 0x11A8 && u <= 0x11FF) || (u >= 0xD7CB && u <= 0xD7FB)
}

func isCombinedS(u Codepoint) bool {
	return u >= sBase && u < sBase+Codepoint(sCount)
}

func isHangulTone(u Codepoint) bool {
	return u >= 0x302E && u <= 0x302F
}

// isZeroWidthChar checks if the font has a glyph for this codepoint
// and its advance width is zero.
// HarfBuzz equivalent: is_zero_width_char() in hb-ot-shaper-hangul.cc:113-117
func (s *Shaper) isZeroWidthChar(u Codepoint) bool {
	if s.cmap == nil || s.hmtx == nil {
		return false
	}
	glyph, ok := s.cmap.Lookup(u)
	if !ok {
		return false
	}
	return s.hmtx.GetAdvanceWidth(glyph) == 0
}

// preprocessTextHangul implements Hangul Jamo composition/decomposition.
// HarfBuzz equivalent: preprocess_text_hangul() in hb-ot-shaper-hangul.cc:129-300
func (s *Shaper) preprocessTextHangul(buf *Buffer) {
	count := buf.Len()
	if count == 0 {
		return
	}

	buf.clearOutput()
	start := 0
	end := 0

	for buf.Idx = 0; buf.Idx < count; {
		u := buf.Info[buf.Idx].Codepoint

		if isHangulTone(u) {
			// Tone mark handling
			if start < end && end == buf.outLen {
				buf.nextGlyph()
				if !s.isZeroWidthChar(u) {
					mergeOutClusters(buf, start, end+1)
					// Move tone mark before the syllable
					tone := buf.outInfo[end]
					copy(buf.outInfo[start+1:end+1], buf.outInfo[start:end])
					buf.outInfo[start] = tone
				}
			} else {
				// Lone tone mark - optionally insert dotted circle
				if s.font.HasGlyph(0x25CC) {
					var chars [2]Codepoint
					if !s.isZeroWidthChar(u) {
						chars[0] = u
						chars[1] = 0x25CC
					} else {
						chars[0] = 0x25CC
						chars[1] = u
					}
					buf.replaceGlyphs(1, 2, chars[:])
				} else {
					buf.nextGlyph()
				}
			}
			start = buf.outLen
			end = buf.outLen
			continue
		}

		start = buf.outLen

		if isL(u) && buf.Idx+1 < count {
			l := u
			v := buf.Info[buf.Idx+1].Codepoint
			if isV(v) {
				var t Codepoint
				var tindex Codepoint
				if buf.Idx+2 < count {
					t = buf.Info[buf.Idx+2].Codepoint
					if isT(t) {
						tindex = t - tBase
					} else {
						t = 0
					}
				}

				// Try composition if all are in the combining ranges
				if isCombiningL(l) && isCombiningV(v) && (t == 0 || isCombiningT(t)) {
					composed := sBase + (l-lBase)*Codepoint(nCount) + (v-vBase)*Codepoint(tCount) + tindex
					if s.font.HasGlyph(composed) {
						numIn := 2
						if t != 0 {
							numIn = 3
						}
						buf.replaceGlyphs(numIn, 1, []Codepoint{composed})
						end = start + 1
						continue
					}
				}

				// Composition failed or not possible. Tag individual Jamo.
				buf.Info[buf.Idx].HangulFeature = hangulLJMO
				buf.nextGlyph()
				buf.Info[buf.Idx].HangulFeature = hangulVJMO
				buf.nextGlyph()
				if t != 0 {
					buf.Info[buf.Idx].HangulFeature = hangulTJMO
					buf.nextGlyph()
					end = start + 3
				} else {
					end = start + 2
				}
				mergeOutClusters(buf, start, end)
				continue
			}
		} else if isCombinedS(u) {
			hasGlyph := s.font.HasGlyph(u)
			lindex := (u - sBase) / Codepoint(nCount)
			nindex := (u - sBase) % Codepoint(nCount)
			vindex := nindex / Codepoint(tCount)
			tindex := nindex % Codepoint(tCount)

			// Check if next char is a combining T that can be appended
			if tindex == 0 && buf.Idx+1 < count && isCombiningT(buf.Info[buf.Idx+1].Codepoint) {
				newTindex := buf.Info[buf.Idx+1].Codepoint - tBase
				newS := u + newTindex
				if s.font.HasGlyph(newS) {
					buf.replaceGlyphs(2, 1, []Codepoint{newS})
					end = start + 1
					continue
				}
			}

			// Check if decomposition is needed
			if !hasGlyph || (tindex == 0 && buf.Idx+1 < count && isT(buf.Info[buf.Idx+1].Codepoint)) {
				decomposed := [3]Codepoint{
					lBase + lindex,
					vBase + vindex,
					tBase + tindex,
				}
				if s.font.HasGlyph(decomposed[0]) &&
					s.font.HasGlyph(decomposed[1]) &&
					(tindex == 0 || s.font.HasGlyph(decomposed[2])) {
					sLen := 2
					if tindex != 0 {
						sLen = 3
					}
					buf.replaceGlyphs(1, sLen, decomposed[:sLen])

					// If original had glyph but we're decomposing because of following T
					if hasGlyph && tindex == 0 {
						buf.nextGlyph()
						sLen++
					}

					end = start + sLen
					for i := start; i < end; i++ {
						switch i - start {
						case 0:
							buf.outInfo[i].HangulFeature = hangulLJMO
						case 1:
							buf.outInfo[i].HangulFeature = hangulVJMO
						default:
							buf.outInfo[i].HangulFeature = hangulTJMO
						}
					}
					mergeOutClusters(buf, start, end)
					continue
				}
			}

			if hasGlyph {
				end = start + 1
			}
		}

		buf.nextGlyph()
	}

	buf.sync()
}

// Hangul feature tags
var (
	tagLJMO = MakeTag('l', 'j', 'm', 'o')
	tagVJMO = MakeTag('v', 'j', 'm', 'o')
	tagTJMO = MakeTag('t', 'j', 'm', 'o')
)

// shapeHangul applies Hangul shaping.
// HarfBuzz equivalent: _hb_ot_shaper_hangul in hb-ot-shaper-hangul.cc
func (s *Shaper) shapeHangul(buf *Buffer, features []Feature) {
	// Step 0: Preprocess text (Jamo composition/decomposition)
	// This happens BEFORE normalization in HarfBuzz
	s.preprocessTextHangul(buf)

	// Step 1: NO normalization (HarfBuzz: NormalizationModeNone for hangul)

	// Step 2: Initialize masks
	buf.ResetMasks(MaskGlobal)

	// Step 3: Map codepoints to glyphs
	s.mapCodepointsToGlyphs(buf)

	// Step 4: Set glyph classes from GDEF
	s.setGlyphClasses(buf)

	// Step 5: Categorize features
	gsubFeatures, gposFeatures := s.categorizeFeatures(features)

	// Add direction-dependent features (Hangul is LTR)
	gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('l', 't', 'r', 'a'), Value: 1})
	gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('l', 't', 'r', 'm'), Value: 1})

	// Set up Hangul Jamo mask bits for ljmo/vjmo/tjmo features.
	// These features are NOT global - they only apply to glyphs tagged during preprocessing.
	// HarfBuzz: collect_features_hangul adds ljmo/vjmo/tjmo with F_MANUAL_ZWJ | F_PER_SYLLABLE
	const (
		ljmoMaskBit uint32 = 1 << 8
		vjmoMaskBit uint32 = 1 << 9
		tjmoMaskBit uint32 = 1 << 10
	)
	for i := range buf.Info {
		switch buf.Info[i].HangulFeature {
		case hangulLJMO:
			buf.Info[i].Mask |= ljmoMaskBit
		case hangulVJMO:
			buf.Info[i].Mask |= vjmoMaskBit
		case hangulTJMO:
			buf.Info[i].Mask |= tjmoMaskBit
		}
	}

	// HarfBuzz: override_features disables calt for hangul
	// Must be appended AFTER defaults so mergeFeatureInfo sees it last (later global overrides earlier)
	gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('c', 'a', 'l', 't'), Value: 0})

	s.applyGSUB(buf, gsubFeatures)

	// Apply ljmo/vjmo/tjmo features with their dedicated masks
	// These are applied AFTER the main GSUB pass, with mask-based filtering
	if s.gsub != nil {
		s.gsub.ApplyFeatureToBufferWithMask(tagLJMO, buf, s.gdef, ljmoMaskBit, s.font)
		s.gsub.ApplyFeatureToBufferWithMask(tagVJMO, buf, s.gdef, vjmoMaskBit, s.font)
		s.gsub.ApplyFeatureToBufferWithMask(tagTJMO, buf, s.gdef, tjmoMaskBit, s.font)
	}
	s.setBaseAdvances(buf)

	// Hangul uses ZeroWidthMarksNone
	s.applyGPOSWithZeroWidthMarks(buf, gposFeatures, ZeroWidthMarksNone)

	// Reverse buffer for RTL display
	if buf.Direction == DirectionRTL {
		s.reverseClusters(buf)
	}
}
