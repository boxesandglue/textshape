package ot

import (
	"sort"
)

// OT Map - HarfBuzz-style lookup organization
//
// HarfBuzz equivalent: hb_ot_map_t in hb-ot-map.hh
//
// This system organizes lookups with their associated masks, allowing
// efficient application of features to specific glyphs.

// LookupMap represents a single lookup with its associated properties.
// HarfBuzz equivalent: hb_ot_map_t::lookup_map_t in hb-ot-map.hh:72-87
type LookupMap struct {
	Index        uint16 // Lookup index in GSUB/GPOS table
	Mask         uint32 // Feature mask - only apply to glyphs where (glyph.mask & mask) != 0
	FeatureTag   Tag    // The feature tag this lookup belongs to
	AutoZWNJ     bool   // Automatically handle ZWNJ (Zero Width Non-Joiner)
	AutoZWJ      bool   // Automatically handle ZWJ (Zero Width Joiner)
	Random       bool   // Random feature (for 'rand' feature)
	PerSyllable  bool   // Apply per syllable (for Indic/USE shapers)
}

// OTMap organizes lookups for GSUB and GPOS tables.
// HarfBuzz equivalent: hb_ot_map_t in hb-ot-map.hh:42-166
type OTMap struct {
	// Lookups organized by table (0=GSUB, 1=GPOS)
	// HarfBuzz: hb_sorted_vector_t<lookup_map_t> lookups[2]
	GSUBLookups []LookupMap
	GPOSLookups []LookupMap

	// Global mask applied to all glyphs
	// HarfBuzz: hb_mask_t global_mask
	GlobalMask uint32

	// Feature requests from shapers (used during plan compilation)
	// HarfBuzz equivalent: hb_ot_map_builder_t::feature_infos
	featureRequests []featureRequest
}

// NewOTMap creates a new empty OT map.
func NewOTMap() *OTMap {
	return &OTMap{
		GlobalMask: MaskGlobal,
	}
}

// AddGSUBLookup adds a GSUB lookup to the map.
// HarfBuzz equivalent: adding to lookups[0] in hb_ot_map_builder_t::compile()
func (m *OTMap) AddGSUBLookup(index uint16, mask uint32, featureTag Tag) {
	// Determine if this is the 'rand' (random) feature
	// HarfBuzz: sets lookup.random = true for 'rand' feature
	isRandom := featureTag == MakeTag('r', 'a', 'n', 'd')

	m.GSUBLookups = append(m.GSUBLookups, LookupMap{
		Index:        index,
		Mask:         mask,
		FeatureTag:   featureTag,
		AutoZWNJ:     true,
		AutoZWJ:      true,
		Random:       isRandom,       // True for 'rand' feature
		PerSyllable:  false,          // Set by shapers if needed
	})
}

// AddGPOSLookup adds a GPOS lookup to the map.
// HarfBuzz equivalent: adding to lookups[1] in hb_ot_map_builder_t::compile()
func (m *OTMap) AddGPOSLookup(index uint16, mask uint32, featureTag Tag) {
	// Determine if this is the 'rand' (random) feature
	// HarfBuzz: sets lookup.random = true for 'rand' feature
	isRandom := featureTag == MakeTag('r', 'a', 'n', 'd')

	m.GPOSLookups = append(m.GPOSLookups, LookupMap{
		Index:        index,
		Mask:         mask,
		FeatureTag:   featureTag,
		AutoZWNJ:     true,
		AutoZWJ:      true,
		Random:       isRandom,       // True for 'rand' feature
		PerSyllable:  false,          // Set by shapers if needed
	})
}

// ApplyGSUB applies all GSUB lookups in the map to the buffer.
// HarfBuzz equivalent: hb_ot_map_t::apply() with GSUB proxy in hb-ot-layout.cc:2010-2060
func (m *OTMap) ApplyGSUB(gsub *GSUB, buf *Buffer, font *Font, gdef *GDEF) {
	if gsub == nil {
		return
	}

	for _, lookup := range m.GSUBLookups {
		gsub.applyLookupWithMap(int(lookup.Index), buf, font, gdef, &lookup)
	}
}

// ApplyGPOS applies all GPOS lookups in the map to the buffer.
// HarfBuzz equivalent: hb_ot_map_t::apply() with GPOS proxy in hb-ot-layout.cc:2010-2060
func (m *OTMap) ApplyGPOS(gpos *GPOS, buf *Buffer, font *Font, gdef *GDEF) {
	if gpos == nil {
		return
	}

	for _, lookup := range m.GPOSLookups {
		gpos.applyLookupWithMap(int(lookup.Index), buf, font, gdef, &lookup)
	}
}

// applyLookupWithMap applies a single GSUB lookup with properties from LookupMap.
// HarfBuzz equivalent: apply_string() called from hb_ot_map_t::apply()
// The lookup properties (mask, auto_zwj, etc.) are set before application.
//
// HarfBuzz reference: hb-ot-layout.cc:2042-2052
func (g *GSUB) applyLookupWithMap(lookupIndex int, buf *Buffer, font *Font, gdef *GDEF, lookupMap *LookupMap) {
	lookup := g.GetLookup(lookupIndex)
	if lookup == nil {
		return
	}

	// Determine mark filtering set index
	markFilteringSet := -1
	if lookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
		markFilteringSet = int(lookup.MarkFilter)
	}

	// HarfBuzz: lookup_mask is never 0. Glyphs with (info.mask & lookup_mask) == 0 are skipped.
	// All glyphs start with MaskGlobal, so global features (with MaskGlobal) match all glyphs.
	effectiveMask := lookupMap.Mask

	ctx := &OTApplyContext{
		Buffer:           buf,
		Font:             font,
		LookupFlag:       lookup.Flag,
		GDEF:             gdef,
		HasGlyphClasses:  gdef != nil && gdef.HasGlyphClasses(),
		MarkFilteringSet: markFilteringSet,
		FeatureMask:      effectiveMask,
		TableType:        TableGSUB,
		AutoZWNJ:         lookupMap.AutoZWNJ,     // From LookupMap (HarfBuzz: lookup.auto_zwnj)
		AutoZWJ:          lookupMap.AutoZWJ,      // From LookupMap (HarfBuzz: lookup.auto_zwj)
		Random:           lookupMap.Random,       // From LookupMap (HarfBuzz: lookup.random)
		PerSyllable:      lookupMap.PerSyllable,  // From LookupMap (HarfBuzz: lookup.per_syllable)
	}

	// Type 8 (Reverse Chain Single Substitution) must be applied in reverse order
	// HarfBuzz: Reverse lookups are always in-place (don't use output buffer)
	if lookup.Type == GSUBTypeReverseChainSingle {
		buf.Idx = len(buf.Info) - 1
		for buf.Idx >= 0 {
			if ctx.ShouldSkipGlyph(buf.Idx) {
				buf.Idx--
				continue
			}
			for _, subtable := range lookup.subtables {
				if subtable.Apply(ctx) > 0 {
					break
				}
			}
			buf.Idx--
		}
		return
	}

	// Normal forward iteration for all other lookup types
	// HarfBuzz equivalent: apply_string() with clear_output/sync pattern
	// (hb-ot-layout.cc:1986-1996)
	//
	// GSUB uses output buffer to properly propagate cluster information:
	// - clear_output() before substitution
	// - outputGlyph() / nextGlyph() during substitution (copies all properties)
	// - sync() after substitution
	buf.clearOutput()

	buf.Idx = 0
	for buf.Idx < len(buf.Info) {
		// Skip glyphs that should be ignored based on LookupFlag, GDEF, and FeatureMask
		if ctx.ShouldSkipGlyph(buf.Idx) {
			buf.nextGlyph()
			continue
		}

		applied := false
		for _, subtable := range lookup.subtables {
			if subtable.Apply(ctx) > 0 {
				applied = true
				break
			}
		}
		if !applied {
			buf.nextGlyph()
		}
	}

	buf.sync()
}

// applyLookupWithMap applies a single GPOS lookup with properties from LookupMap.
// HarfBuzz equivalent: apply_string() called from hb_ot_map_t::apply()
// The lookup properties (mask, auto_zwj, etc.) are set before application.
//
// HarfBuzz reference: hb-ot-layout.cc:2042-2052
func (g *GPOS) applyLookupWithMap(lookupIndex int, buf *Buffer, font *Font, gdef *GDEF, lookupMap *LookupMap) {
	lookup := g.GetLookup(lookupIndex)
	if lookup == nil {
		return
	}

	// Determine mark filtering set index
	markFilteringSet := -1
	if lookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
		markFilteringSet = int(lookup.MarkFilter)
	}

	// HarfBuzz: lookup_mask is never 0. Glyphs with (info.mask & lookup_mask) == 0 are skipped.
	// All glyphs start with MaskGlobal, so global features (with MaskGlobal) match all glyphs.
	effectiveMask := lookupMap.Mask

	ctx := &OTApplyContext{
		Buffer:           buf,
		Font:             font,
		LookupFlag:       lookup.Flag,
		LookupIndex:      lookupIndex,
		GDEF:             gdef,
		HasGlyphClasses:  gdef != nil && gdef.HasGlyphClasses(),
		MarkFilteringSet: markFilteringSet,
		FeatureMask:      effectiveMask,
		TableType:        TableGPOS,
		AutoZWNJ:         lookupMap.AutoZWNJ,    // From LookupMap (HarfBuzz: lookup.auto_zwnj)
		AutoZWJ:          lookupMap.AutoZWJ,     // From LookupMap (HarfBuzz: lookup.auto_zwj)
		Random:           lookupMap.Random,      // From LookupMap (HarfBuzz: lookup.random)
		PerSyllable:      lookupMap.PerSyllable, // From LookupMap (HarfBuzz: lookup.per_syllable)
		NestingLevel:     HBMaxNestingLevel,     // Initialize nesting level
	}

	// Set RecurseFunc for nested lookup application
	// HarfBuzz equivalent: recurse_func in hb_ot_apply_context_t
	// This closure captures the GPOS reference for recursive lookups
	ctx.RecurseFunc = func(subCtx *OTApplyContext, nestedLookupIndex int) bool {
		nestedLookup := g.GetLookup(nestedLookupIndex)
		if nestedLookup == nil {
			return false
		}

		// Apply nested lookup with the current context's buffer position
		// HarfBuzz: The nested lookup uses the same buffer but may have different flags
		savedLookupFlag := subCtx.LookupFlag
		savedMarkFilteringSet := subCtx.MarkFilteringSet
		savedLookupIndex := subCtx.LookupIndex

		// Set nested lookup properties
		subCtx.LookupFlag = nestedLookup.Flag
		subCtx.LookupIndex = nestedLookupIndex
		if nestedLookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
			subCtx.MarkFilteringSet = int(nestedLookup.MarkFilter)
		} else {
			subCtx.MarkFilteringSet = -1
		}

		// Apply the nested lookup's subtables
		applied := false
		for _, subtable := range nestedLookup.subtables {
			if subtable.Apply(subCtx) {
				applied = true
				break
			}
		}

		// Restore original context properties
		subCtx.LookupFlag = savedLookupFlag
		subCtx.MarkFilteringSet = savedMarkFilteringSet
		subCtx.LookupIndex = savedLookupIndex

		return applied
	}

	buf.Idx = 0
	for buf.Idx < len(buf.Info) {
		// Skip glyphs that should be ignored based on LookupFlag, GDEF, and FeatureMask
		if ctx.ShouldSkipGlyph(buf.Idx) {
			buf.Idx++
			continue
		}

		applied := false
		for _, subtable := range lookup.subtables {
			if subtable.Apply(ctx) {
				applied = true
				break
			}
		}
		if !applied {
			buf.Idx++
		}
	}
}

// CompileMap creates an OTMap from feature list and requested features.
// HarfBuzz equivalent: hb_ot_map_builder_t::compile() in hb-ot-map.cc:250-400
//
// This function:
// 1. For each requested feature, finds the lookups in the font
// 2. Uses script/language-specific feature selection (CRITICAL FIX!)
// 3. Assigns masks based on feature type (global vs positional)
// 4. Collects all lookups into the map for efficient application
//
// HarfBuzz reference:
// - Line 242-248: hb_ot_layout_collect_features_map() gets script/language-specific features
// - Line 279: Checks if feature exists in the script/language-specific map
func CompileMap(gsub *GSUB, gpos *GPOS, features []Feature, scriptTag Tag, languageTag Tag) *OTMap {
	m := NewOTMap()
	featureMap := NewFeatureMap()

	// Process GSUB features
	if gsub != nil {
		featureList, err := gsub.ParseFeatureList()
		if err == nil {
			// Get script/language-specific feature indices
			// HarfBuzz: hb_ot_layout_collect_features_map() in hb-ot-map.cc:244-248
			//   const OT::LangSys &l = g.get_script (script_index).get_lang_sys (language_index);
			var featureIndices []uint16
			scriptList, err := gsub.ParseScriptList()
			if err == nil && scriptList != nil {
				// Try to get LangSys for the requested script/language
				langSys := scriptList.GetLangSys(scriptTag, languageTag)
				// If script not found, fall back to DFLT script
				// HarfBuzz: hb_ot_layout_table_select_script() falls back to DFLT
				if langSys == nil {
					langSys = scriptList.GetDefaultScript()
				}
				if langSys != nil {
					featureIndices = langSys.FeatureIndices
				}
			}

			for _, f := range features {
				if f.Value == 0 {
					continue
				}

				var lookups []uint16
				if featureIndices != nil {
					// Use script/language-specific search
					// HarfBuzz: feature_indices[table_index].has() in hb-ot-map.cc:279
					lookups = featureList.FindFeatureByIndices(f.Tag, featureIndices)
				} else {
					// Fallback to global search only if no script/language found at all
					// This is a last resort - should rarely happen
					lookups = featureList.FindFeature(f.Tag)
				}

				if lookups == nil {
					continue
				}

				// Get mask for this feature
				mask := featureMap.GetMask(f.Tag)

				for _, lookupIdx := range lookups {
					m.AddGSUBLookup(lookupIdx, mask, f.Tag)
				}
			}
		}
	}

	// Process GPOS features
	if gpos != nil {
		featureList, err := gpos.ParseFeatureList()
		if err == nil {
			// Get script/language-specific feature indices
			// HarfBuzz: hb_ot_layout_collect_features_map() in hb-ot-map.cc:244-248
			//   const OT::LangSys &l = g.get_script (script_index).get_lang_sys (language_index);
			var featureIndices []uint16
			scriptList, err := gpos.ParseScriptList()
			if err == nil && scriptList != nil {
				// Try to get LangSys for the requested script/language
				langSys := scriptList.GetLangSys(scriptTag, languageTag)
				// If script not found, fall back to DFLT script
				// HarfBuzz: hb_ot_layout_table_select_script() falls back to DFLT
				if langSys == nil {
					langSys = scriptList.GetDefaultScript()
				}
				if langSys != nil {
					featureIndices = langSys.FeatureIndices
				}
			}

			for _, f := range features {
				if f.Value == 0 {
					continue
				}

				var lookups []uint16
				if featureIndices != nil {
					// Use script/language-specific search
					// HarfBuzz: feature_indices[table_index].has() in hb-ot-map.cc:279
					lookups = featureList.FindFeatureByIndices(f.Tag, featureIndices)
				} else {
					// Fallback to global search only if no script/language found at all
					// This is a last resort - should rarely happen
					lookups = featureList.FindFeature(f.Tag)
				}

				if lookups == nil {
					continue
				}

				// Get mask for this feature
				mask := featureMap.GetMask(f.Tag)

				for _, lookupIdx := range lookups {
					m.AddGPOSLookup(lookupIdx, mask, f.Tag)
				}
			}
		}
	}

	// Sort lookups and merge duplicates
	// HarfBuzz equivalent: hb-ot-map.cc:362-377
	// When multiple features reference the same lookup, we must deduplicate
	// to avoid applying the lookup multiple times.
	m.GSUBLookups = deduplicateLookups(m.GSUBLookups)
	m.GPOSLookups = deduplicateLookups(m.GPOSLookups)

	return m
}

// deduplicateLookups sorts lookups by index and merges duplicates.
// HarfBuzz equivalent: hb-ot-map.cc:362-377
// When the same lookup is referenced by multiple features (e.g., 'dist' and 'kern'),
// we merge them by OR-ing their masks and AND-ing their auto_zwnj/auto_zwj flags.
func deduplicateLookups(lookups []LookupMap) []LookupMap {
	if len(lookups) <= 1 {
		return lookups
	}

	// Sort by lookup index
	sort.Slice(lookups, func(i, j int) bool {
		return lookups[i].Index < lookups[j].Index
	})

	// Merge duplicates
	j := 0
	for i := 1; i < len(lookups); i++ {
		if lookups[i].Index != lookups[j].Index {
			j++
			lookups[j] = lookups[i]
		} else {
			// Same lookup index - merge properties
			// HarfBuzz: lookups[j].mask |= lookups[i].mask
			lookups[j].Mask |= lookups[i].Mask
			// HarfBuzz: lookups[j].auto_zwnj &= lookups[i].auto_zwnj
			lookups[j].AutoZWNJ = lookups[j].AutoZWNJ && lookups[i].AutoZWNJ
			// HarfBuzz: lookups[j].auto_zwj &= lookups[i].auto_zwj
			lookups[j].AutoZWJ = lookups[j].AutoZWJ && lookups[i].AutoZWJ
		}
	}

	return lookups[:j+1]
}
