package ot

import (
	"sort"
)

// Arabic Fallback Shaping Implementation
//
// This provides fallback Arabic shaping for fonts that:
// 1. Don't have GSUB features for positional forms (init/medi/fina/isol)
// 2. But DO have glyphs for Unicode Arabic Presentation Forms (U+FE70-U+FEFF)
//
// Source: HarfBuzz hb-ot-shaper-arabic-fallback.hh

// Arabic fallback feature tags (in order of application)
// Source: HarfBuzz hb-ot-shaper-arabic-fallback.hh:42-51
var arabicFallbackFeatures = []Tag{
	MakeTag('i', 'n', 'i', 't'), // 0: Initial form
	MakeTag('m', 'e', 'd', 'i'), // 1: Medial form
	MakeTag('f', 'i', 'n', 'a'), // 2: Final form
	MakeTag('i', 's', 'o', 'l'), // 3: Isolated form
	MakeTag('r', 'l', 'i', 'g'), // 4: Required ligatures (3-component)
	MakeTag('r', 'l', 'i', 'g'), // 5: Required ligatures (2-component)
	MakeTag('r', 'l', 'i', 'g'), // 6: Required ligatures (mark)
}

// Maximum number of fallback lookups
const arabicFallbackMaxLookups = 7

// fallbackSubstEntry represents a single substitution in a fallback lookup
type fallbackSubstEntry struct {
	glyph      GlyphID // Input glyph
	substitute GlyphID // Output glyph
}

// fallbackLigatureEntry represents a ligature in a fallback lookup
type fallbackLigatureEntry struct {
	firstGlyph GlyphID   // First component
	components []GlyphID // Remaining components
	ligature   GlyphID   // Result
}

// fallbackLookup represents a synthesized lookup for fallback shaping
type fallbackLookup struct {
	lookupType  int                     // 1=Single, 4=Ligature
	ignoreMarks bool                    // LookupFlag::IgnoreMarks
	singles     []fallbackSubstEntry    // For Single Substitution
	ligatures   []fallbackLigatureEntry // For Ligature Substitution
}

// arabicFallbackPlan contains all fallback lookups for Arabic shaping
type arabicFallbackPlan struct {
	numLookups int
	masks      [arabicFallbackMaxLookups]uint32
	lookups    [arabicFallbackMaxLookups]*fallbackLookup
}

// createArabicFallbackPlan creates a fallback plan for Arabic shaping.
// Returns nil if no fallback is needed (font doesn't have presentation forms).
// Source: HarfBuzz arabic_fallback_plan_create() in hb-ot-shaper-arabic-fallback.hh:323-347
func createArabicFallbackPlan(font *Font, cmap *Cmap) *arabicFallbackPlan {
	// First try Unicode-based fallback (presentation forms)
	plan := createUnicodeFallbackPlan(font, cmap)
	if plan != nil {
		return plan
	}

	// Then try Windows-1256 fallback
	return createWin1256FallbackPlan(cmap)
}

// createUnicodeFallbackPlan creates a fallback plan using Unicode presentation forms.
// Source: HarfBuzz arabic_fallback_plan_init_unicode() in hb-ot-shaper-arabic-fallback.hh:296-319
func createUnicodeFallbackPlan(font *Font, cmap *Cmap) *arabicFallbackPlan {
	plan := &arabicFallbackPlan{}
	j := 0

	// Try to create lookups for each feature
	for i := 0; i < len(arabicFallbackFeatures); i++ {
		mask := arabicFeatureToMask(arabicFallbackFeatures[i], i)
		if mask == 0 {
			continue
		}

		lookup := synthesizeFallbackLookup(font, cmap, i)
		if lookup != nil {
			plan.masks[j] = mask
			plan.lookups[j] = lookup
			j++
		}
	}

	plan.numLookups = j

	if j == 0 {
		return nil
	}

	return plan
}

// arabicFeatureToMask returns the mask for a given feature index
func arabicFeatureToMask(tag Tag, index int) uint32 {
	switch index {
	case 0: // init
		return MaskInit
	case 1: // medi
		return MaskMedi
	case 2: // fina
		return MaskFina
	case 3: // isol
		return MaskIsol
	case 4, 5, 6: // rlig
		return MaskGlobal
	}
	return 0
}

// synthesizeFallbackLookup creates a fallback lookup for a given feature index.
// Source: HarfBuzz arabic_fallback_synthesize_lookup() in hb-ot-shaper-arabic-fallback.hh:203-220
func synthesizeFallbackLookup(font *Font, cmap *Cmap, featureIndex int) *fallbackLookup {
	if featureIndex < 4 {
		return synthesizeSingleSubstLookup(font, cmap, featureIndex)
	}
	switch featureIndex {
	case 4:
		return synthesizeLigatureLookup(font, cmap, ligature3Table, true)
	case 5:
		return synthesizeLigatureLookup2(font, cmap, ligatureTable, true)
	case 6:
		return synthesizeLigatureLookupMark(font, cmap, ligatureMarkTable, false)
	}
	return nil
}

// synthesizeSingleSubstLookup creates a Single Substitution lookup from the shaping table.
// Source: HarfBuzz arabic_fallback_synthesize_lookup_single() in hb-ot-shaper-arabic-fallback.hh:53-102
func synthesizeSingleSubstLookup(font *Font, cmap *Cmap, featureIndex int) *fallbackLookup {
	var entries []fallbackSubstEntry

	// Iterate through shaping table
	for u := Codepoint(shapingTableFirst); u <= shapingTableLast; u++ {
		s := getShapingEntry(u, featureIndex)
		if s == 0 {
			continue
		}

		// Get glyph IDs
		uGlyph, uFound := cmap.Lookup(u)
		sGlyph, sFound := cmap.Lookup(s)

		if !uFound || !sFound || uGlyph == 0 || sGlyph == 0 || uGlyph == sGlyph {
			continue
		}

		entries = append(entries, fallbackSubstEntry{
			glyph:      uGlyph,
			substitute: sGlyph,
		})
	}

	if len(entries) == 0 {
		return nil
	}

	// Sort by glyph ID for binary search
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].glyph < entries[j].glyph
	})

	return &fallbackLookup{
		lookupType:  1, // Single Substitution
		ignoreMarks: true,
		singles:     entries,
	}
}

// synthesizeLigatureLookup creates a Ligature lookup from the 3-component table.
func synthesizeLigatureLookup(font *Font, cmap *Cmap, table []ligature3Set, ignoreMarks bool) *fallbackLookup {
	var entries []fallbackLigatureEntry

	for _, set := range table {
		firstGlyph, firstFound := cmap.Lookup(Codepoint(set.first))
		if !firstFound || firstGlyph == 0 {
			continue
		}

		for _, lig := range set.ligatures {
			ligGlyph, ligFound := cmap.Lookup(Codepoint(lig.ligature))
			if !ligFound || ligGlyph == 0 {
				continue
			}

			comp1Glyph, comp1Found := cmap.Lookup(Codepoint(lig.component1))
			comp2Glyph, comp2Found := cmap.Lookup(Codepoint(lig.component2))
			if !comp1Found || !comp2Found || comp1Glyph == 0 || comp2Glyph == 0 {
				continue
			}

			entries = append(entries, fallbackLigatureEntry{
				firstGlyph: firstGlyph,
				components: []GlyphID{comp1Glyph, comp2Glyph},
				ligature:   ligGlyph,
			})
		}
	}

	if len(entries) == 0 {
		return nil
	}

	// Sort by first glyph for efficient lookup
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].firstGlyph < entries[j].firstGlyph
	})

	return &fallbackLookup{
		lookupType:  4, // Ligature Substitution
		ignoreMarks: ignoreMarks,
		ligatures:   entries,
	}
}

// synthesizeLigatureLookup2 creates a Ligature lookup from the 2-component table.
func synthesizeLigatureLookup2(font *Font, cmap *Cmap, table []ligatureSet, ignoreMarks bool) *fallbackLookup {
	var entries []fallbackLigatureEntry

	for _, set := range table {
		firstGlyph, firstFound := cmap.Lookup(Codepoint(set.first))
		if !firstFound || firstGlyph == 0 {
			continue
		}

		for _, lig := range set.ligatures {
			ligGlyph, ligFound := cmap.Lookup(Codepoint(lig.ligature))
			if !ligFound || ligGlyph == 0 {
				continue
			}

			compGlyph, compFound := cmap.Lookup(Codepoint(lig.component))
			if !compFound || compGlyph == 0 {
				continue
			}

			entries = append(entries, fallbackLigatureEntry{
				firstGlyph: firstGlyph,
				components: []GlyphID{compGlyph},
				ligature:   ligGlyph,
			})
		}
	}

	if len(entries) == 0 {
		return nil
	}

	// Sort by first glyph for efficient lookup
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].firstGlyph < entries[j].firstGlyph
	})

	return &fallbackLookup{
		lookupType:  4, // Ligature Substitution
		ignoreMarks: ignoreMarks,
		ligatures:   entries,
	}
}

// synthesizeLigatureLookupMark creates a Ligature lookup for mark ligatures.
func synthesizeLigatureLookupMark(font *Font, cmap *Cmap, table []ligatureSet, ignoreMarks bool) *fallbackLookup {
	var entries []fallbackLigatureEntry

	for _, set := range table {
		// For mark table, first is a Unicode codepoint (not presentation form)
		firstGlyph, firstFound := cmap.Lookup(Codepoint(set.first))
		if !firstFound || firstGlyph == 0 {
			continue
		}

		for _, lig := range set.ligatures {
			ligGlyph, ligFound := cmap.Lookup(Codepoint(lig.ligature))
			if !ligFound || ligGlyph == 0 {
				continue
			}

			compGlyph, compFound := cmap.Lookup(Codepoint(lig.component))
			if !compFound || compGlyph == 0 {
				continue
			}

			entries = append(entries, fallbackLigatureEntry{
				firstGlyph: firstGlyph,
				components: []GlyphID{compGlyph},
				ligature:   ligGlyph,
			})
		}
	}

	if len(entries) == 0 {
		return nil
	}

	// Sort by first glyph for efficient lookup
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].firstGlyph < entries[j].firstGlyph
	})

	return &fallbackLookup{
		lookupType:  4, // Ligature Substitution
		ignoreMarks: ignoreMarks,
		ligatures:   entries,
	}
}

// applyFallbackLookup applies a single fallback lookup to the buffer.
// Source: HarfBuzz arabic_fallback_plan_shape() in hb-ot-shaper-arabic-fallback.hh:368-381
func (lookup *fallbackLookup) apply(buf *Buffer, mask uint32, gdef *GDEF) {
	if lookup == nil {
		return
	}

	switch lookup.lookupType {
	case 1: // Single Substitution
		lookup.applySingleSubst(buf, mask, gdef)
	case 4: // Ligature Substitution
		lookup.applyLigatureSubst(buf, mask, gdef)
	}
}

// applySingleSubst applies single substitution to the buffer.
func (lookup *fallbackLookup) applySingleSubst(buf *Buffer, mask uint32, gdef *GDEF) {
	for i := range buf.Info {
		info := &buf.Info[i]

		// Check mask
		if info.Mask&mask == 0 {
			continue
		}

		// Skip marks if IgnoreMarks flag is set
		if lookup.ignoreMarks && gdef != nil {
			glyphClass := gdef.GetGlyphClass(info.GlyphID)
			if glyphClass == GlyphClassMark {
				continue
			}
		}

		// Binary search for substitution
		idx := sort.Search(len(lookup.singles), func(j int) bool {
			return lookup.singles[j].glyph >= info.GlyphID
		})

		if idx < len(lookup.singles) && lookup.singles[idx].glyph == info.GlyphID {
			info.GlyphID = lookup.singles[idx].substitute
		}
	}
}

// applyLigatureSubst applies ligature substitution to the buffer.
func (lookup *fallbackLookup) applyLigatureSubst(buf *Buffer, mask uint32, gdef *GDEF) {
	if len(buf.Info) < 2 {
		return
	}

	i := 0
	for i < len(buf.Info) {
		info := &buf.Info[i]

		// Check mask
		if info.Mask&mask == 0 {
			i++
			continue
		}

		// Skip marks if IgnoreMarks flag is set
		if lookup.ignoreMarks && isMarkGlyph(buf.Info[i], gdef) {
			i++
			continue
		}

		// Find ligatures starting with this glyph
		matched := false
		for _, lig := range lookup.ligatures {
			if lig.firstGlyph != info.GlyphID {
				continue
			}

			// Try to match components
			componentCount := len(lig.components)
			if i+1+componentCount > len(buf.Info) {
				continue
			}

			// Match components (skipping marks if needed)
			matchIdx := i + 1
			componentsMatched := 0
			skippedMarks := make([]int, 0, 8) // Track skipped mark positions

			for componentsMatched < componentCount && matchIdx < len(buf.Info) {
				nextInfo := &buf.Info[matchIdx]

				// Skip marks if IgnoreMarks
				if lookup.ignoreMarks && isMarkGlyph(*nextInfo, gdef) {
					skippedMarks = append(skippedMarks, matchIdx)
					matchIdx++
					continue
				}

				if nextInfo.GlyphID != lig.components[componentsMatched] {
					break
				}
				componentsMatched++
				matchIdx++
			}

			if componentsMatched == componentCount {
				// Match successful - apply ligature
				// Allocate ligature ID and set properties
				// HarfBuzz: hb_ot_layout_substitute_lookup() sets lig_id and num_comps
				ligID := buf.AllocateLigID()
				numComponents := componentCount + 1 // first glyph + components

				// Replace first glyph with ligature
				info.GlyphID = lig.ligature
				info.SetLigPropsForLigature(ligID, numComponents)
				info.GlyphProps |= GlyphPropsLigature // Mark as ligature for GetLigNumComps()

				// Set ligature component for skipped marks
				// HarfBuzz: marks between components belong to the preceding component
				// All skippedMarks are between the first glyph and the last component,
				// so they all belong to component 1 (the first component, which is the first glyph)
				for _, markIdx := range skippedMarks {
					buf.Info[markIdx].SetLigPropsForMark(ligID, 1)
				}

				// Collect all cluster values that should merge into the ligature
				mergedClusters := make(map[int]bool)
				for j := i; j < matchIdx; j++ {
					mergedClusters[buf.Info[j].Cluster] = true
				}

				// Find minimum cluster (for RTL text, this is the ligature's cluster)
				ligCluster := buf.Info[i].Cluster
				for j := i + 1; j < matchIdx; j++ {
					if buf.Info[j].Cluster < ligCluster {
						ligCluster = buf.Info[j].Cluster
					}
				}
				buf.Info[i].Cluster = ligCluster

				// Remove consumed glyphs (but keep skipped marks)
				// We need to remove only the matched component glyphs, not the marks
				// Build new slice: keep [0..i], keep skipped marks, then [matchIdx..]
				newInfo := make([]GlyphInfo, 0, len(buf.Info)-(matchIdx-i-1)+len(skippedMarks))
				newInfo = append(newInfo, buf.Info[:i+1]...)
				for _, markIdx := range skippedMarks {
					if markIdx < matchIdx {
						mark := buf.Info[markIdx]
						mark.Cluster = ligCluster // Update mark's cluster to match ligature
						newInfo = append(newInfo, mark)
					}
				}
				// Append remaining glyphs, updating clusters and lig props for marks
				// that belonged to merged components
				lastComponent := numComponents // Marks after the ligature attach to last component
				for j := matchIdx; j < len(buf.Info); j++ {
					glyph := buf.Info[j]
					if mergedClusters[glyph.Cluster] && isMarkGlyph(glyph, gdef) {
						glyph.Cluster = ligCluster
						glyph.SetLigPropsForMark(ligID, lastComponent)
					}
					newInfo = append(newInfo, glyph)
				}
				buf.Info = newInfo

				if len(buf.Pos) > len(buf.Info) {
					buf.Pos = buf.Pos[:len(buf.Info)]
				}

				matched = true
				break
			}
		}

		if !matched {
			i++
		}
	}
}

// isMarkGlyph determines if a glyph is a mark, using GDEF if available,
// or falling back to Unicode General Category.
func isMarkGlyph(info GlyphInfo, gdef *GDEF) bool {
	// Use GDEF if available
	if gdef != nil {
		glyphClass := gdef.GetGlyphClass(info.GlyphID)
		return glyphClass == GlyphClassMark
	}

	// Fallback: check Unicode General Category of original codepoint
	// Combining marks have General_Category Mn, Mc, or Me
	gc := getGeneralCategory(info.Codepoint)
	return gc == GCNonSpacingMark || gc == GCSpacingMark || gc == GCEnclosingMark
}

// arabicFallbackPlanShape applies all fallback lookups to the buffer.
// Source: HarfBuzz arabic_fallback_plan_shape() in hb-ot-shaper-arabic-fallback.hh:368-381
func (plan *arabicFallbackPlan) shape(buf *Buffer, gdef *GDEF) {
	if plan == nil {
		return
	}

	for i := 0; i < plan.numLookups; i++ {
		if plan.lookups[i] != nil {
			plan.lookups[i].apply(buf, plan.masks[i], gdef)
		}
	}
}

// needsArabicFallback determines if Arabic fallback shaping is needed.
// Returns true if:
// 1. Script is Arabic proper (not Syriac, Mongolian, etc.)
// 2. Font has no GSUB features for positional forms
// 3. Font has glyphs for presentation forms OR is a Windows-1256 encoded font
//
// Source: HarfBuzz arabic_fallback_plan_create() decision logic
func needsArabicFallback(gsub *GSUB, scriptTag Tag, cmap *Cmap) bool {
	// Only for Arabic proper
	if scriptTag != MakeTag('a', 'r', 'a', 'b') {
		return false
	}

	if cmap == nil {
		return false
	}

	// Check if font has positional features
	if gsub != nil {
		featureList, err := gsub.ParseFeatureList()
		if err == nil {
			// If font has init feature for Arabic, no fallback needed
			lookups := featureList.FindFeature(MakeTag('i', 'n', 'i', 't'))
			if len(lookups) > 0 {
				return false
			}
		}
	}

	// Check if font has ANY presentation form glyphs
	// Test various presentation forms (isolated, final, initial, medial)
	// HarfBuzz: arabic_fallback_plan_init_unicode checks all entries in shaping_table
	testCodepoints := []Codepoint{
		// Isolated forms
		0xFE8D, // ALEF isolated
		0xFE8F, // BEH isolated
		// Final forms
		0xFE8E, // ALEF final
		0xFE90, // BEH final
		// Initial forms
		0xFE91, // BEH initial
		0xFEDF, // LAM initial
		// Medial forms
		0xFE92, // BEH medial
		0xFEE0, // LAM medial
		// Lam-Alef ligatures
		0xFEFB, // LAM-ALEF isolated
		0xFEFC, // LAM-ALEF final
	}

	for _, cp := range testCodepoints {
		glyph, found := cmap.Lookup(cp)
		if found && glyph != 0 {
			return true
		}
	}

	// Also check for Windows-1256 encoded fonts
	// HarfBuzz: arabic_fallback_plan_init_win1256() signature check
	if isWin1256Font(cmap) {
		return true
	}

	return false
}
