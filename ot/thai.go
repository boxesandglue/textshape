package ot

// Thai/Lao Shaper
//
// HarfBuzz equivalent: hb-ot-shaper-thai.cc
//
// This implements Thai and Lao text shaping, specifically:
// - Sara Am decomposition and reordering
// - PUA shaping fallback (not implemented yet)

// isSaraAm returns true if the codepoint is Thai or Lao Sara Am.
// HarfBuzz equivalent: IS_SARA_AM macro in hb-ot-shaper-thai.cc:319
// Thai Sara Am: U+0E33, Lao Sara Am: U+0EB3
func isSaraAm(cp Codepoint) bool {
	return (cp & ^Codepoint(0x0080)) == 0x0E33
}

// nikhahitFromSaraAm converts Sara Am to Nikhahit.
// HarfBuzz equivalent: NIKHAHIT_FROM_SARA_AM macro in hb-ot-shaper-thai.cc:320
// Thai: U+0E33 -> U+0E4D, Lao: U+0EB3 -> U+0ECD
func nikhahitFromSaraAm(cp Codepoint) Codepoint {
	return cp - 0x0E33 + 0x0E4D
}

// saraAaFromSaraAm converts Sara Am to Sara Aa.
// HarfBuzz equivalent: SARA_AA_FROM_SARA_AM macro in hb-ot-shaper-thai.cc:321
// Thai: U+0E33 -> U+0E32, Lao: U+0EB3 -> U+0EB2
func saraAaFromSaraAm(cp Codepoint) Codepoint {
	return cp - 1
}

// isThaiAboveBaseMark returns true if the codepoint is a Thai/Lao above-base mark.
// HarfBuzz equivalent: IS_ABOVE_BASE_MARK macro in hb-ot-shaper-thai.cc:322
// Thai: <0E31, 0E34..0E37, 0E47..0E4E>
// Lao:  <0EB1, 0EB4..0EB7, 0EBB, 0EC8..0ECD>
func isThaiAboveBaseMark(cp Codepoint) bool {
	// Normalize to Thai range for comparison
	u := cp & ^Codepoint(0x0080)
	return (u >= 0x0E34 && u <= 0x0E37) ||
		(u >= 0x0E47 && u <= 0x0E4E) ||
		u == 0x0E31 ||
		u == 0x0E3B // Note: 0x0E3B is included in HarfBuzz
}

// mergeOutClusters merges clusters in the output buffer range [start, end).
// HarfBuzz equivalent: merge_out_clusters() in hb-buffer.cc:584-620
func mergeOutClusters(buf *Buffer, start, end int) {
	if end-start < 2 {
		return
	}

	// Find minimum cluster in range
	cluster := buf.outInfo[start].Cluster
	for i := start + 1; i < end; i++ {
		if buf.outInfo[i].Cluster < cluster {
			cluster = buf.outInfo[i].Cluster
		}
	}

	// Extend start backwards if adjacent glyphs have same cluster
	for start > 0 && buf.outInfo[start-1].Cluster == buf.outInfo[start].Cluster {
		start--
	}

	// Extend end forwards if adjacent glyphs have same cluster
	for end < buf.outLen && buf.outInfo[end-1].Cluster == buf.outInfo[end].Cluster {
		end++
	}

	// Set all glyphs in range to minimum cluster
	for i := start; i < end; i++ {
		buf.outInfo[i].Cluster = cluster
	}
}

// preprocessTextThai implements Sara Am decomposition and reordering.
// HarfBuzz equivalent: preprocess_text_thai() in hb-ot-shaper-thai.cc:265-372
//
// When you have a SARA AM, decompose it in NIKHAHIT + SARA AA, *and* move the
// NIKHAHIT backwards over any above-base marks.
//
// Example: <0E14, 0E4B, 0E33> -> <0E14, 0E4D, 0E4B, 0E32>
func (s *Shaper) preprocessTextThai(buf *Buffer) {
	if buf.Len() == 0 {
		return
	}

	buf.clearOutput()
	count := buf.Len()

	for buf.Idx = 0; buf.Idx < count; {
		u := buf.Info[buf.Idx].Codepoint

		// Not Sara Am - just copy to output
		if !isSaraAm(u) {
			buf.nextGlyph()
			continue
		}

		// Is Sara Am. Decompose and reorder.
		// Output Nikhahit first
		nikhahit := nikhahitFromSaraAm(u)
		nikhahitInfo := buf.Info[buf.Idx]
		nikhahitInfo.Codepoint = nikhahit
		nikhahitInfo.GlyphID = GlyphID(nikhahit)
		buf.outputInfo(nikhahitInfo)

		// Replace Sara Am with Sara Aa
		saraAa := saraAaFromSaraAm(u)
		buf.Info[buf.Idx].Codepoint = saraAa
		buf.Info[buf.Idx].GlyphID = GlyphID(saraAa)
		buf.nextGlyph()

		// Now reorder: Move Nikhahit (at end-2) before any above-base marks
		end := buf.outLen
		if end < 2 {
			continue
		}

		// Find the start position (skip backwards over above-base marks)
		start := end - 2
		for start > 0 && isThaiAboveBaseMark(buf.outInfo[start-1].Codepoint) {
			start--
		}

		if start+2 < end {
			// Move Nikhahit (end-2) to the beginning (start)
			// Merge clusters first
			mergeOutClusters(buf, start, end)

			// Save Nikhahit
			t := buf.outInfo[end-2]

			// Shift elements right
			copy(buf.outInfo[start+1:end-1], buf.outInfo[start:end-2])

			// Place Nikhahit at start
			buf.outInfo[start] = t
		} else {
			// Since we decomposed, and NIKHAHIT is combining, merge clusters with the
			// previous cluster.
			if start > 0 {
				mergeOutClusters(buf, start-1, end)
			}
		}
	}

	buf.sync()
}

// shapeThai applies Thai/Lao shaping.
// HarfBuzz equivalent: _hb_ot_shaper_thai in hb-ot-shaper-thai.cc:374-391
func (s *Shaper) shapeThai(buf *Buffer, features []Feature) {
	// Step 0: Preprocess text (Sara Am decomposition)
	// This happens BEFORE normalization in HarfBuzz
	s.preprocessTextThai(buf)

	// Step 1: Normalize Unicode (decompose, reorder marks, recompose)
	s.normalizeBuffer(buf, NormalizationModeAuto)

	// Step 2: Initialize masks
	buf.ResetMasks(MaskGlobal)

	// Step 3: Map codepoints to glyphs
	s.mapCodepointsToGlyphs(buf)

	// Step 4: Set glyph classes from GDEF
	s.setGlyphClasses(buf)

	// Step 5: Categorize and apply features
	gsubFeatures, gposFeatures := s.categorizeFeatures(features)

	// Add direction-dependent features (Thai is always LTR)
	gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('l', 't', 'r', 'a'), Value: 1})
	gsubFeatures = append(gsubFeatures, Feature{Tag: MakeTag('l', 't', 'r', 'm'), Value: 1})

	s.applyGSUB(buf, gsubFeatures)
	s.setBaseAdvances(buf)
	// Thai shaper uses LATE zero width marks (HarfBuzz: HB_OT_SHAPE_ZERO_WIDTH_MARKS_BY_GDEF_LATE)
	s.applyGPOSWithZeroWidthMarks(buf, gposFeatures, ZeroWidthMarksByGDEFLate)

	// Note: Thai does NOT use fallback positioning (fallback_position = false in HarfBuzz)
	// So we don't call applyKernTableFallback here
}
