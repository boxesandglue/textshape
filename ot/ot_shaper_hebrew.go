package ot

// Hebrew Shaper
// HarfBuzz equivalent: hb-ot-shaper-hebrew.cc
//
// Hebrew requires:
// 1. Special mark reordering (reorder_marks_hebrew)
// 2. Special composition for presentation forms (compose_hebrew)
// 3. GPOS tag restriction (only apply GPOS if script is 'hebr')

// Hebrew Modified Combining Class constants
// HarfBuzz equivalent: hb-unicode.hh:333-349
const (
	hebrewCCC10 = 22 // sheva
	hebrewCCC11 = 15 // hataf segol
	hebrewCCC12 = 16 // hataf patah
	hebrewCCC13 = 17 // hataf qamats
	hebrewCCC14 = 23 // hiriq
	hebrewCCC15 = 18 // tsere
	hebrewCCC16 = 19 // segol
	hebrewCCC17 = 20 // patah
	hebrewCCC18 = 21 // qamats & qamats qatan
	hebrewCCC19 = 14 // holam & holam haser for vav
	hebrewCCC20 = 24 // qubuts
	hebrewCCC21 = 12 // dagesh
	hebrewCCC22 = 25 // meteg
	hebrewCCC23 = 13 // rafe
	hebrewCCC24 = 10 // shin dot
	hebrewCCC25 = 11 // sin dot
	hebrewCCC26 = 26 // point varika
)

// reorderMarksHebrewSlice performs Hebrew-specific mark reordering.
// HarfBuzz equivalent: reorder_marks_hebrew() in hb-ot-shaper-hebrew.cc:165-190
//
// This function looks for a specific pattern and swaps marks:
// Pattern: (patach/qamats) (sheva/hiriq) (meteg/below)
// If found, swap the last two marks.
//
// This is needed because Hebrew vowels need to be positioned correctly
// when multiple marks are stacked under a base character.
func reorderMarksHebrewSlice(info []GlyphInfo, start, end int) {
	// Need at least 3 marks for this pattern
	// HarfBuzz: for (unsigned i = start + 2; i < end; i++)
	for i := start + 2; i < end; i++ {
		c0 := getGlyphInfoCombiningClass(&info[i-2])
		c1 := getGlyphInfoCombiningClass(&info[i-1])
		c2 := getGlyphInfoCombiningClass(&info[i])

		// Check for pattern:
		// c0 = patach (CCC17=20) or qamats (CCC18=21)
		// c1 = sheva (CCC10=22) or hiriq (CCC14=23)
		// c2 = meteg (CCC22=25) or below (CCC 220)
		// HarfBuzz: hb-ot-shaper-hebrew.cc:179-181
		if (c0 == hebrewCCC17 || c0 == hebrewCCC18) && // patach or qamats
			(c1 == hebrewCCC10 || c1 == hebrewCCC14) && // sheva or hiriq
			(c2 == hebrewCCC22 || c2 == 220) { // meteg or below

			// Merge clusters before swapping
			// HarfBuzz: buffer->merge_clusters(i - 1, i + 1)
			// We merge clusters inline since we're working with a slice
			minCluster := info[i-1].Cluster
			if info[i].Cluster < minCluster {
				minCluster = info[i].Cluster
			}
			info[i-1].Cluster = minCluster
			info[i].Cluster = minCluster

			// Swap the last two marks
			// HarfBuzz: hb_swap(info[i - 1], info[i])
			info[i-1], info[i] = info[i], info[i-1]

			// Only do this once per mark sequence
			// HarfBuzz: break
			break
		}
	}
}

// reorderMarksHebrew is the OTShaper callback wrapper.
// HarfBuzz equivalent: reorder_marks field in _hb_ot_shaper_hebrew
func reorderMarksHebrew(plan *ShapePlan, buf *Buffer, start, end int) {
	reorderMarksHebrewSlice(buf.Info, start, end)
}

// Hebrew presentation form dagesh table
// Maps base character (U+05D0..U+05EA) to dagesh form (0xFB30..0xFB4A)
// HarfBuzz equivalent: sDageshForms[] in hb-ot-shaper-hebrew.cc:45-73
var hebrewDageshForms = [27]Codepoint{
	0xFB30, // ALEF      (05D0)
	0xFB31, // BET       (05D1)
	0xFB32, // GIMEL     (05D2)
	0xFB33, // DALET     (05D3)
	0xFB34, // HE        (05D4)
	0xFB35, // VAV       (05D5)
	0xFB36, // ZAYIN     (05D6)
	0x0000, // HET       (05D7) - no dagesh form
	0xFB38, // TET       (05D8)
	0xFB39, // YOD       (05D9)
	0xFB3A, // FINAL KAF (05DA)
	0xFB3B, // KAF       (05DB)
	0xFB3C, // LAMED     (05DC)
	0x0000, // FINAL MEM (05DD) - no dagesh form
	0xFB3E, // MEM       (05DE)
	0x0000, // FINAL NUN (05DF) - no dagesh form
	0xFB40, // NUN       (05E0)
	0xFB41, // SAMEKH    (05E1)
	0x0000, // AYIN      (05E2) - no dagesh form
	0xFB43, // FINAL PE  (05E3)
	0xFB44, // PE        (05E4)
	0x0000, // FINAL TSADI (05E5) - no dagesh form
	0xFB46, // TSADI     (05E6)
	0xFB47, // QOF       (05E7)
	0xFB48, // RESH      (05E8)
	0xFB49, // SHIN      (05E9)
	0xFB4A, // TAV       (05EA)
}

// composeHebrew performs Hebrew-specific composition.
// HarfBuzz equivalent: compose_hebrew() in hb-ot-shaper-hebrew.cc:34-163
//
// This is used for old fonts that don't have GPOS mark positioning.
// It composes Hebrew base + mark sequences into presentation forms.
//
// Note: The composition is only applied when the font does NOT have GPOS mark support.
// This is checked via c.Plan and its has_gpos_mark field.
func composeHebrew(c *NormalizeContext, a, b Codepoint) (ab Codepoint, ok bool) {
	// First try standard Unicode composition
	ab, ok = unicodeCompose(a, b)
	if ok {
		return ab, true
	}

	// Hebrew fallback composition is only for old fonts without GPOS mark support
	// HarfBuzz: if (!found && (c->plan && !c->plan->has_gpos_mark))
	// For now, we check if GPOS exists (simplified check)
	// TODO: Check for has_gpos_mark specifically when ShapePlan is extended
	if c.Plan != nil && c.Shaper != nil && c.Shaper.gpos != nil {
		// Font has GPOS, skip fallback composition
		return 0, false
	}

	// Special-case Hebrew presentation forms
	// HarfBuzz: hb-ot-shaper-hebrew.cc:85-159
	switch b {
	case 0x05B4: // HIRIQ
		if a == 0x05D9 { // YOD
			return 0xFB1D, true
		}

	case 0x05B7: // PATAH
		if a == 0x05F2 { // YIDDISH YOD YOD
			return 0xFB1F, true
		} else if a == 0x05D0 { // ALEF
			return 0xFB2E, true
		}

	case 0x05B8: // QAMATS
		if a == 0x05D0 { // ALEF
			return 0xFB2F, true
		}

	case 0x05B9: // HOLAM
		if a == 0x05D5 { // VAV
			return 0xFB4B, true
		}

	case 0x05BC: // DAGESH
		if a >= 0x05D0 && a <= 0x05EA {
			form := hebrewDageshForms[a-0x05D0]
			if form != 0 {
				return form, true
			}
		} else if a == 0xFB2A { // SHIN WITH SHIN DOT
			return 0xFB2C, true
		} else if a == 0xFB2B { // SHIN WITH SIN DOT
			return 0xFB2D, true
		}

	case 0x05BF: // RAFE
		switch a {
		case 0x05D1: // BET
			return 0xFB4C, true
		case 0x05DB: // KAF
			return 0xFB4D, true
		case 0x05E4: // PE
			return 0xFB4E, true
		}

	case 0x05C1: // SHIN DOT
		if a == 0x05E9 { // SHIN
			return 0xFB2A, true
		} else if a == 0xFB49 { // SHIN WITH DAGESH
			return 0xFB2C, true
		}

	case 0x05C2: // SIN DOT
		if a == 0x05E9 { // SHIN
			return 0xFB2B, true
		} else if a == 0xFB49 { // SHIN WITH DAGESH
			return 0xFB2D, true
		}
	}

	return 0, false
}

func init() {
	// Initialize HebrewShaper with Hebrew-specific functions
	// HarfBuzz equivalent: _hb_ot_shaper_hebrew in hb-ot-shaper-hebrew.cc:192-209
	HebrewShaper.GPOSTag = MakeTag('h', 'e', 'b', 'r')
	HebrewShaper.ReorderMarks = reorderMarksHebrew
	HebrewShaper.Compose = composeHebrew
}
