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

// featureMaskEntry stores the allocated mask and shift for a compiled feature.
// HarfBuzz equivalent: hb_ot_map_t::feature_map_t in hb-ot-map.hh:88-107
type featureMaskEntry struct {
	Shift uint   // Bit position where this feature's bits start
	Mask  uint32 // Bit mask for this feature
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

	// Feature mask allocations for per-cluster range features.
	// HarfBuzz equivalent: hb_ot_map_t::features (sorted vector of feature_map_t)
	FeatureMasks map[Tag]featureMaskEntry
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
	m.addGSUBLookupWithFlags(index, mask, featureTag, false, true)
}

// addGSUBLookupWithFlags adds a GSUB lookup with explicit per-syllable and auto-ZWJ flags.
// HarfBuzz equivalent: adding to lookups[0] with feature flags from hb_ot_map_feature_flags_t
func (m *OTMap) addGSUBLookupWithFlags(index uint16, mask uint32, featureTag Tag, perSyllable bool, autoZWJ bool) {
	isRandom := featureTag == MakeTag('r', 'a', 'n', 'd')

	m.GSUBLookups = append(m.GSUBLookups, LookupMap{
		Index:       index,
		Mask:        mask,
		FeatureTag:  featureTag,
		AutoZWNJ:    true,
		AutoZWJ:     autoZWJ,
		Random:      isRandom,
		PerSyllable: perSyllable,
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
			if ctx.PerSyllable {
				ctx.MatchSyllable = buf.Info[buf.Idx].Syllable
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

		// Set per-syllable reference syllable from current glyph position.
		// HarfBuzz: skipping_iterator_t::reset() sets matcher.syllable from info[start_index].syllable()
		if ctx.PerSyllable {
			ctx.MatchSyllable = buf.Info[buf.Idx].Syllable
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

// featureInfo is used internally during CompileMap for HarfBuzz-style feature merging.
// HarfBuzz equivalent: hb_ot_map_builder_t::feature_info_t in hb-ot-map.hh:258-273
type featureInfo struct {
	tag          Tag
	seq          int    // sequence number for stable sorting
	maxValue     uint32 // maximum feature value
	defaultValue uint32 // default value for glyphs not in range (0 for non-global)
	isGlobal     bool   // F_GLOBAL: applies to all glyphs
	perSyllable  bool
	manualZWJ    bool
}

// CompileMap creates an OTMap from feature list and requested features.
// HarfBuzz equivalent: hb_ot_map_builder_t::compile() in hb-ot-map.cc:183-391
//
// This implements HarfBuzz's full feature merging algorithm:
// 1. Collect all features into featureInfo list with sequence numbers
// 2. Sort by tag (stable by sequence), merge duplicates per HarfBuzz rules
// 3. Allocate mask bits for non-global features
// 4. Find lookups in GSUB/GPOS using script/language-specific feature indices
// 5. Assign masks to lookups
func CompileMap(gsub *GSUB, gpos *GPOS, features []Feature, scriptTag Tag, languageTag Tag) *OTMap {
	m := NewOTMap()
	m.FeatureMasks = make(map[Tag]featureMaskEntry)

	// Step 1: Build feature info list with sequence numbers
	// HarfBuzz: feature_infos collected via add_feature() calls
	var infos []featureInfo
	for i, f := range features {
		// A feature is global if either:
		// - IsGlobal() returns true (Start==0, End==FeatureGlobalEnd), or
		// - Start==0 and End==0 (struct literal without explicit End, treated as global)
		// HarfBuzz: F_GLOBAL is passed explicitly; we infer it from Start/End.
		isGlobal := f.IsGlobal() || (f.Start == 0 && f.End == 0)
		infos = append(infos, featureInfo{
			tag:          f.Tag,
			seq:          i,
			maxValue:     f.Value,
			defaultValue: 0,
			isGlobal:     isGlobal,
			perSyllable:  f.PerSyllable,
			manualZWJ:    f.ManualZWJ,
		})
		if isGlobal {
			infos[len(infos)-1].defaultValue = f.Value
		}
	}

	// Step 2: Sort by tag (stable by seq) and merge duplicates
	// HarfBuzz: hb-ot-map.cc:213-240
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].tag != infos[j].tag {
			return infos[i].tag < infos[j].tag
		}
		return infos[i].seq < infos[j].seq
	})

	// Merge duplicates
	if len(infos) > 1 {
		j := 0
		for i := 1; i < len(infos); i++ {
			if infos[i].tag != infos[j].tag {
				j++
				infos[j] = infos[i]
			} else {
				// Same tag: merge per HarfBuzz rules (hb-ot-map.cc:224-238)
				if infos[i].isGlobal {
					// Later global overrides earlier: take its max_value and default_value
					infos[j].isGlobal = true
					infos[j].maxValue = infos[i].maxValue
					infos[j].defaultValue = infos[i].defaultValue
				} else {
					// Later non-global: remove F_GLOBAL from earlier, take max of max_value
					if infos[j].isGlobal {
						infos[j].isGlobal = false
					}
					if infos[i].maxValue > infos[j].maxValue {
						infos[j].maxValue = infos[i].maxValue
					}
					// Inherit default_value from j (earlier entry)
				}
			}
		}
		infos = infos[:j+1]
	}

	// Step 3: Allocate mask bits
	// HarfBuzz: hb-ot-map.cc:250-324
	globalBitShift := uint(31)
	globalBitMask := uint32(1) << globalBitShift
	m.GlobalMask = globalBitMask
	nextBit := uint(1) // Bit 0 is reserved (HB_GLYPH_FLAG_DEFINED), start from 1

	// Compute mask for each feature
	type compiledFeature struct {
		info featureInfo
		mask uint32
	}
	var compiled []compiledFeature

	for _, info := range infos {
		if info.maxValue == 0 {
			// Feature disabled (e.g., -calt), skip entirely
			// HarfBuzz: hb-ot-map.cc:268 "if (!info->max_value...) continue"
			continue
		}

		var mask uint32
		if info.isGlobal && info.maxValue == 1 {
			// Global feature with value 1: use the global bit
			// HarfBuzz: hb-ot-map.cc:312-315
			mask = globalBitMask
		} else {
			// Non-global or multi-valued: allocate dedicated bits
			// HarfBuzz: hb-ot-map.cc:316-321
			bitsNeeded := bitStorage(info.maxValue)
			if bitsNeeded > 8 {
				bitsNeeded = 8 // HB_OT_MAP_MAX_BITS
			}
			if nextBit+bitsNeeded >= globalBitShift {
				continue // Not enough bits
			}
			mask = (1<<(nextBit+bitsNeeded) - 1) &^ (1<<nextBit - 1)
			m.FeatureMasks[info.tag] = featureMaskEntry{Shift: nextBit, Mask: mask}
			// Set default_value in global_mask
			m.GlobalMask |= (info.defaultValue << nextBit) & mask
			nextBit += bitsNeeded
		}

		compiled = append(compiled, compiledFeature{info: info, mask: mask})
	}

	// Step 4: Collect lookups for compiled features
	// HarfBuzz: hb-ot-map.cc:332-389
	featureMap := NewFeatureMap()

	// Helper to get feature indices for a table
	getFeatureIndices := func(scriptList *ScriptList, scriptTag, languageTag Tag) []uint16 {
		if scriptList == nil {
			return nil
		}
		langSys := scriptList.GetLangSys(scriptTag, languageTag)
		if langSys == nil {
			langSys = scriptList.GetDefaultScript()
		}
		if langSys != nil {
			return langSys.FeatureIndices
		}
		return nil
	}

	// Process GSUB features
	if gsub != nil {
		featureList, err := gsub.ParseFeatureList()
		if err == nil {
			scriptList, _ := gsub.ParseScriptList()
			featureIndices := getFeatureIndices(scriptList, scriptTag, languageTag)

			for _, cf := range compiled {
				// Use the compiled mask, but check if FeatureMap has a positional mask override
				mask := cf.mask
				if positionalMask := featureMap.GetMask(cf.info.tag); positionalMask != MaskGlobal {
					mask = positionalMask
				}

				var lookups []uint16
				if featureIndices != nil {
					lookups = featureList.FindFeatureByIndices(cf.info.tag, featureIndices)
				} else {
					lookups = featureList.FindFeature(cf.info.tag)
				}

				for _, lookupIdx := range lookups {
					m.addGSUBLookupWithFlags(lookupIdx, mask, cf.info.tag, cf.info.perSyllable, !cf.info.manualZWJ)
				}
			}
		}
	}

	// Process GPOS features
	if gpos != nil {
		featureList, err := gpos.ParseFeatureList()
		if err == nil {
			scriptList, _ := gpos.ParseScriptList()
			featureIndices := getFeatureIndices(scriptList, scriptTag, languageTag)

			for _, cf := range compiled {
				mask := cf.mask

				var lookups []uint16
				if featureIndices != nil {
					lookups = featureList.FindFeatureByIndices(cf.info.tag, featureIndices)
				} else {
					lookups = featureList.FindFeature(cf.info.tag)
				}

				for _, lookupIdx := range lookups {
					m.AddGPOSLookup(lookupIdx, mask, cf.info.tag)
				}
			}
		}
	}

	// Sort lookups and merge duplicates
	// HarfBuzz equivalent: hb-ot-map.cc:362-377
	m.GSUBLookups = deduplicateLookups(m.GSUBLookups)
	m.GPOSLookups = deduplicateLookups(m.GPOSLookups)

	return m
}

// bitStorage returns the number of bits needed to store a value.
// HarfBuzz equivalent: hb_bit_storage() in hb-algs.hh
func bitStorage(v uint32) uint {
	if v == 0 {
		return 0
	}
	n := uint(0)
	for v > 0 {
		n++
		v >>= 1
	}
	return n
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
