package subset

import (
	"encoding/binary"
	"sort"

	"github.com/boxesandglue/textshape/ot"
)

// subsetGSUB creates a subsetted GSUB table with remapped glyph IDs.
func (p *Plan) subsetGSUB() ([]byte, error) {
	if p.gsub == nil {
		return nil, nil
	}

	// Get the 'liga' feature's lookups
	featList, err := p.gsub.ParseFeatureList()
	if err != nil {
		return nil, nil
	}

	ligaLookups := featList.FindFeature(ot.TagLiga)
	if len(ligaLookups) == 0 {
		return nil, nil
	}

	builder := newGSUBBuilder(p.glyphMap, p.glyphSet)

	// Only process lookups referenced by 'liga' feature
	for _, lookupIdx := range ligaLookups {
		lookup := p.gsub.GetLookup(int(lookupIdx))
		if lookup == nil {
			continue
		}

		subsetLookup := builder.subsetLookup(lookup)
		if subsetLookup != nil {
			builder.addLookup(subsetLookup)
		}
	}

	// If no lookups remain, return nil (don't include empty GSUB)
	if len(builder.lookups) == 0 {
		return nil, nil
	}

	return builder.build()
}

// gsubBuilder builds a subsetted GSUB table.
type gsubBuilder struct {
	glyphMap map[ot.GlyphID]ot.GlyphID
	glyphSet map[ot.GlyphID]bool
	lookups  []*lookupBuilder
	features []featureRecord
	scripts  []scriptRecord
}

type lookupBuilder struct {
	subtables  [][]byte
	lookupType uint16
	flag       uint16
}

type featureRecord struct {
	lookups []uint16
	tag     ot.Tag
}

type scriptRecord struct {
	dfltLang *langSysRecord
	langSys  []langSysRecord
	tag      ot.Tag
}

type langSysRecord struct {
	features []uint16
	tag      ot.Tag
	reqFeat  uint16
}

func newGSUBBuilder(glyphMap map[ot.GlyphID]ot.GlyphID, glyphSet map[ot.GlyphID]bool) *gsubBuilder {
	return &gsubBuilder{
		glyphMap: glyphMap,
		glyphSet: glyphSet,
	}
}

func (b *gsubBuilder) addLookup(lookup *lookupBuilder) {
	b.lookups = append(b.lookups, lookup)
}

// subsetLookup subsets a single lookup, returning nil if empty.
func (b *gsubBuilder) subsetLookup(lookup *ot.GSUBLookup) *lookupBuilder {
	lb := &lookupBuilder{
		lookupType: lookup.Type,
		flag:       lookup.Flag,
	}

	for _, subtable := range lookup.Subtables() {
		var data []byte

		switch st := subtable.(type) {
		case *ot.SingleSubst:
			data = b.subsetSingleSubst(st)
		case *ot.MultipleSubst:
			data = b.subsetMultipleSubst(st)
		case *ot.AlternateSubst:
			data = b.subsetAlternateSubst(st)
		case *ot.LigatureSubst:
			data = b.subsetLigatureSubst(st)
			// TODO: Context, ChainContext, Extension
		}

		if data != nil && len(data) > 0 {
			lb.subtables = append(lb.subtables, data)
		}
	}

	if len(lb.subtables) == 0 {
		return nil
	}
	return lb
}

// subsetSingleSubst subsets a SingleSubst subtable.
func (b *gsubBuilder) subsetSingleSubst(st *ot.SingleSubst) []byte {
	mapping := st.Mapping()
	if len(mapping) == 0 {
		return nil
	}

	// Filter and remap
	var entries []struct {
		in, out ot.GlyphID
	}

	for inGlyph, outGlyph := range mapping {
		newIn, okIn := b.glyphMap[inGlyph]
		newOut, okOut := b.glyphMap[outGlyph]
		if okIn && okOut {
			entries = append(entries, struct{ in, out ot.GlyphID }{newIn, newOut})
		}
	}

	if len(entries) == 0 {
		return nil
	}

	// Sort by input glyph
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].in < entries[j].in
	})

	// Check if we can use format 1 (constant delta)
	canUseFormat1 := true
	delta := int(entries[0].out) - int(entries[0].in)
	for _, e := range entries {
		if int(e.out)-int(e.in) != delta {
			canUseFormat1 = false
			break
		}
	}

	// Also check if coverage is consecutive (for format 1)
	if canUseFormat1 {
		for i := 1; i < len(entries); i++ {
			if entries[i].in != entries[i-1].in+1 {
				canUseFormat1 = false
				break
			}
		}
	}

	if canUseFormat1 && len(entries) > 1 {
		return b.buildSingleSubstFormat1(entries, int16(delta))
	}
	return b.buildSingleSubstFormat2(entries)
}

func (b *gsubBuilder) buildSingleSubstFormat1(entries []struct{ in, out ot.GlyphID }, delta int16) []byte {
	// Build coverage
	glyphs := make([]ot.GlyphID, len(entries))
	for i, e := range entries {
		glyphs[i] = e.in
	}
	coverage := buildCoverageFormat1(glyphs)

	// Format 1: format(2) + coverageOffset(2) + deltaGlyphID(2)
	data := make([]byte, 6+len(coverage))
	binary.BigEndian.PutUint16(data[0:], 1)             // format
	binary.BigEndian.PutUint16(data[2:], 6)             // coverage offset
	binary.BigEndian.PutUint16(data[4:], uint16(delta)) // delta
	copy(data[6:], coverage)

	return data
}

func (b *gsubBuilder) buildSingleSubstFormat2(entries []struct{ in, out ot.GlyphID }) []byte {
	// Build coverage
	glyphs := make([]ot.GlyphID, len(entries))
	for i, e := range entries {
		glyphs[i] = e.in
	}
	coverage := buildCoverageFormat1(glyphs)

	// Format 2: format(2) + coverageOffset(2) + glyphCount(2) + substitutes[](2*n)
	headerSize := 6 + len(entries)*2
	data := make([]byte, headerSize+len(coverage))

	binary.BigEndian.PutUint16(data[0:], 2)                    // format
	binary.BigEndian.PutUint16(data[2:], uint16(headerSize))   // coverage offset
	binary.BigEndian.PutUint16(data[4:], uint16(len(entries))) // glyph count

	for i, e := range entries {
		binary.BigEndian.PutUint16(data[6+i*2:], uint16(e.out))
	}
	copy(data[headerSize:], coverage)

	return data
}

// multipleSubstEntry holds a remapped multiple substitution entry.
type multipleSubstEntry struct {
	out []ot.GlyphID
	in  ot.GlyphID
}

// subsetMultipleSubst subsets a MultipleSubst subtable.
func (b *gsubBuilder) subsetMultipleSubst(st *ot.MultipleSubst) []byte {
	mapping := st.Mapping()
	if len(mapping) == 0 {
		return nil
	}

	// Filter and remap
	var entries []multipleSubstEntry

	for inGlyph, outGlyphs := range mapping {
		newIn, okIn := b.glyphMap[inGlyph]
		if !okIn {
			continue
		}

		newOut := make([]ot.GlyphID, 0, len(outGlyphs))
		allOk := true
		for _, g := range outGlyphs {
			if newG, ok := b.glyphMap[g]; ok {
				newOut = append(newOut, newG)
			} else {
				allOk = false
				break
			}
		}
		if allOk {
			entries = append(entries, multipleSubstEntry{in: newIn, out: newOut})
		}
	}

	if len(entries) == 0 {
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].in < entries[j].in
	})

	return b.buildMultipleSubst(entries)
}

func (b *gsubBuilder) buildMultipleSubst(entries []multipleSubstEntry) []byte {
	// Build coverage
	glyphs := make([]ot.GlyphID, len(entries))
	for i, e := range entries {
		glyphs[i] = e.in
	}
	coverage := buildCoverageFormat1(glyphs)

	// Calculate sequence table sizes
	seqOffsets := make([]uint16, len(entries))
	headerSize := 6 + len(entries)*2 // format + coverageOff + seqCount + seqOffsets[]

	seqDataStart := headerSize
	seqData := make([]byte, 0)

	for i, e := range entries {
		seqOffsets[i] = uint16(seqDataStart + len(seqData))
		// Sequence: glyphCount(2) + glyphs[](2*n)
		seqLen := 2 + len(e.out)*2
		seq := make([]byte, seqLen)
		binary.BigEndian.PutUint16(seq[0:], uint16(len(e.out)))
		for j, g := range e.out {
			binary.BigEndian.PutUint16(seq[2+j*2:], uint16(g))
		}
		seqData = append(seqData, seq...)
	}

	totalSize := headerSize + len(seqData) + len(coverage)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1)                               // format
	binary.BigEndian.PutUint16(data[2:], uint16(headerSize+len(seqData))) // coverage offset
	binary.BigEndian.PutUint16(data[4:], uint16(len(entries)))            // sequence count

	for i, off := range seqOffsets {
		binary.BigEndian.PutUint16(data[6+i*2:], off)
	}

	copy(data[headerSize:], seqData)
	copy(data[headerSize+len(seqData):], coverage)

	return data
}

// alternateSubstEntry holds a remapped alternate substitution entry.
type alternateSubstEntry struct {
	alts []ot.GlyphID
	in   ot.GlyphID
}

// subsetAlternateSubst subsets an AlternateSubst subtable.
func (b *gsubBuilder) subsetAlternateSubst(st *ot.AlternateSubst) []byte {
	mapping := st.Mapping()
	if len(mapping) == 0 {
		return nil
	}

	var entries []alternateSubstEntry

	for inGlyph, alternates := range mapping {
		newIn, okIn := b.glyphMap[inGlyph]
		if !okIn {
			continue
		}

		newAlts := make([]ot.GlyphID, 0, len(alternates))
		for _, g := range alternates {
			if newG, ok := b.glyphMap[g]; ok {
				newAlts = append(newAlts, newG)
			}
		}
		if len(newAlts) > 0 {
			entries = append(entries, alternateSubstEntry{in: newIn, alts: newAlts})
		}
	}

	if len(entries) == 0 {
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].in < entries[j].in
	})

	return b.buildAlternateSubst(entries)
}

func (b *gsubBuilder) buildAlternateSubst(entries []alternateSubstEntry) []byte {
	glyphs := make([]ot.GlyphID, len(entries))
	for i, e := range entries {
		glyphs[i] = e.in
	}
	coverage := buildCoverageFormat1(glyphs)

	// Calculate alternate set offsets
	altSetOffsets := make([]uint16, len(entries))
	headerSize := 6 + len(entries)*2

	altData := make([]byte, 0)
	for i, e := range entries {
		altSetOffsets[i] = uint16(headerSize + len(altData))
		// AlternateSet: glyphCount(2) + alternates[](2*n)
		setLen := 2 + len(e.alts)*2
		set := make([]byte, setLen)
		binary.BigEndian.PutUint16(set[0:], uint16(len(e.alts)))
		for j, g := range e.alts {
			binary.BigEndian.PutUint16(set[2+j*2:], uint16(g))
		}
		altData = append(altData, set...)
	}

	totalSize := headerSize + len(altData) + len(coverage)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1)
	binary.BigEndian.PutUint16(data[2:], uint16(headerSize+len(altData)))
	binary.BigEndian.PutUint16(data[4:], uint16(len(entries)))

	for i, off := range altSetOffsets {
		binary.BigEndian.PutUint16(data[6+i*2:], off)
	}

	copy(data[headerSize:], altData)
	copy(data[headerSize+len(altData):], coverage)

	return data
}

// ligatureEntry holds a remapped ligature.
type ligatureEntry struct {
	components []ot.GlyphID
	ligGlyph   ot.GlyphID
}

// ligatureSetEntry holds a remapped ligature set.
type ligatureSetEntry struct {
	ligatures  []ligatureEntry
	firstGlyph ot.GlyphID
}

// subsetLigatureSubst subsets a LigatureSubst subtable.
func (b *gsubBuilder) subsetLigatureSubst(st *ot.LigatureSubst) []byte {
	ligSets := st.LigatureSets()
	covGlyphs := st.Coverage().Glyphs()

	if len(ligSets) == 0 || len(covGlyphs) == 0 {
		return nil
	}

	var sets []ligatureSetEntry

	for i, ligSet := range ligSets {
		if i >= len(covGlyphs) {
			break
		}
		firstGlyph := covGlyphs[i]

		newFirst, okFirst := b.glyphMap[firstGlyph]
		if !okFirst {
			continue
		}

		var ligs []ligatureEntry
		for _, lig := range ligSet {
			newLigGlyph, okLig := b.glyphMap[lig.LigGlyph]
			if !okLig {
				continue
			}

			// Remap components
			newComps := make([]ot.GlyphID, 0, len(lig.Components))
			allOk := true
			for _, comp := range lig.Components {
				if newComp, ok := b.glyphMap[comp]; ok {
					newComps = append(newComps, newComp)
				} else {
					allOk = false
					break
				}
			}

			if allOk {
				ligs = append(ligs, ligatureEntry{ligGlyph: newLigGlyph, components: newComps})
			}
		}

		if len(ligs) > 0 {
			sets = append(sets, ligatureSetEntry{firstGlyph: newFirst, ligatures: ligs})
		}
	}

	if len(sets) == 0 {
		return nil
	}

	// Sort by first glyph
	sort.Slice(sets, func(i, j int) bool {
		return sets[i].firstGlyph < sets[j].firstGlyph
	})

	return b.buildLigatureSubst(sets)
}

func (b *gsubBuilder) buildLigatureSubst(sets []ligatureSetEntry) []byte {
	// Build coverage from first glyphs
	glyphs := make([]ot.GlyphID, len(sets))
	for i, s := range sets {
		glyphs[i] = s.firstGlyph
	}
	coverage := buildCoverageFormat1(glyphs)

	// Header: format(2) + coverageOffset(2) + ligSetCount(2) + ligSetOffsets[](2*n)
	headerSize := 6 + len(sets)*2

	// Build ligature set data
	ligSetData := make([]byte, 0)
	ligSetOffsets := make([]uint16, len(sets))

	for i, set := range sets {
		ligSetOffsets[i] = uint16(headerSize + len(ligSetData))

		// LigatureSet: ligCount(2) + ligOffsets[](2*n) + Ligature tables
		ligSetHeaderSize := 2 + len(set.ligatures)*2
		ligTables := make([]byte, 0)
		ligOffsets := make([]uint16, len(set.ligatures))

		for j, lig := range set.ligatures {
			ligOffsets[j] = uint16(ligSetHeaderSize + len(ligTables))
			// Ligature: ligGlyph(2) + compCount(2) + components[](2*(n-1))
			ligLen := 4 + len(lig.components)*2
			ligTable := make([]byte, ligLen)
			binary.BigEndian.PutUint16(ligTable[0:], uint16(lig.ligGlyph))
			binary.BigEndian.PutUint16(ligTable[2:], uint16(len(lig.components)+1)) // +1 for first glyph
			for k, comp := range lig.components {
				binary.BigEndian.PutUint16(ligTable[4+k*2:], uint16(comp))
			}
			ligTables = append(ligTables, ligTable...)
		}

		// Build ligature set
		ligSetTable := make([]byte, ligSetHeaderSize+len(ligTables))
		binary.BigEndian.PutUint16(ligSetTable[0:], uint16(len(set.ligatures)))
		for j, off := range ligOffsets {
			binary.BigEndian.PutUint16(ligSetTable[2+j*2:], off)
		}
		copy(ligSetTable[ligSetHeaderSize:], ligTables)

		ligSetData = append(ligSetData, ligSetTable...)
	}

	totalSize := headerSize + len(ligSetData) + len(coverage)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1)                                  // format
	binary.BigEndian.PutUint16(data[2:], uint16(headerSize+len(ligSetData))) // coverage offset
	binary.BigEndian.PutUint16(data[4:], uint16(len(sets)))                  // ligature set count

	for i, off := range ligSetOffsets {
		binary.BigEndian.PutUint16(data[6+i*2:], off)
	}

	copy(data[headerSize:], ligSetData)
	copy(data[headerSize+len(ligSetData):], coverage)

	return data
}

// build serializes the GSUB table.
func (b *gsubBuilder) build() ([]byte, error) {
	if len(b.lookups) == 0 {
		return nil, nil
	}

	// Build lookup list
	lookupList := b.buildLookupList()

	// Build minimal script list (DFLT/dflt)
	scriptList := b.buildScriptList(len(b.lookups))

	// Build feature list (all lookups under 'liga' and 'kern' features)
	featureList := b.buildFeatureList(len(b.lookups))

	// GSUB header: version(4) + scriptListOff(2) + featureListOff(2) + lookupListOff(2)
	headerSize := 10

	scriptListOff := headerSize
	featureListOff := scriptListOff + len(scriptList)
	lookupListOff := featureListOff + len(featureList)

	totalSize := lookupListOff + len(lookupList)
	data := make([]byte, totalSize)

	// Version 1.0
	binary.BigEndian.PutUint16(data[0:], 1)
	binary.BigEndian.PutUint16(data[2:], 0)
	binary.BigEndian.PutUint16(data[4:], uint16(scriptListOff))
	binary.BigEndian.PutUint16(data[6:], uint16(featureListOff))
	binary.BigEndian.PutUint16(data[8:], uint16(lookupListOff))

	copy(data[scriptListOff:], scriptList)
	copy(data[featureListOff:], featureList)
	copy(data[lookupListOff:], lookupList)

	return data, nil
}

func (b *gsubBuilder) buildLookupList() []byte {
	// LookupList: lookupCount(2) + lookupOffsets[](2*n) + Lookup tables
	headerSize := 2 + len(b.lookups)*2

	lookupData := make([]byte, 0)
	lookupOffsets := make([]uint16, len(b.lookups))

	for i, lookup := range b.lookups {
		lookupOffsets[i] = uint16(headerSize + len(lookupData))

		// Lookup: lookupType(2) + lookupFlag(2) + subTableCount(2) + subTableOffsets[](2*n)
		lookupHeaderSize := 6 + len(lookup.subtables)*2

		subtableData := make([]byte, 0)
		subtableOffsets := make([]uint16, len(lookup.subtables))

		for j, st := range lookup.subtables {
			subtableOffsets[j] = uint16(lookupHeaderSize + len(subtableData))
			subtableData = append(subtableData, st...)
		}

		lookupTable := make([]byte, lookupHeaderSize+len(subtableData))
		binary.BigEndian.PutUint16(lookupTable[0:], lookup.lookupType)
		binary.BigEndian.PutUint16(lookupTable[2:], lookup.flag)
		binary.BigEndian.PutUint16(lookupTable[4:], uint16(len(lookup.subtables)))
		for j, off := range subtableOffsets {
			binary.BigEndian.PutUint16(lookupTable[6+j*2:], off)
		}
		copy(lookupTable[lookupHeaderSize:], subtableData)

		lookupData = append(lookupData, lookupTable...)
	}

	data := make([]byte, headerSize+len(lookupData))
	binary.BigEndian.PutUint16(data[0:], uint16(len(b.lookups)))
	for i, off := range lookupOffsets {
		binary.BigEndian.PutUint16(data[2+i*2:], off)
	}
	copy(data[headerSize:], lookupData)

	return data
}

func (b *gsubBuilder) buildScriptList(numLookups int) []byte {
	// Minimal script list: DFLT script with dflt language system
	// ScriptList: scriptCount(2) + scriptRecords[](6*n) + Script tables

	// ScriptRecord: scriptTag(4) + scriptOffset(2)
	scriptCount := 1
	headerSize := 2 + scriptCount*6

	// Script table: defaultLangSys(2) + langSysCount(2)
	// DefaultLangSys: lookupOrder(2) + reqFeatureIndex(2) + featureIndexCount(2) + featureIndices[](2*n)
	langSysSize := 6 + 2 // One feature index
	scriptSize := 4 + langSysSize

	totalSize := headerSize + scriptSize
	data := make([]byte, totalSize)

	// Header
	binary.BigEndian.PutUint16(data[0:], uint16(scriptCount))
	// DFLT script tag
	copy(data[2:], []byte("DFLT"))
	binary.BigEndian.PutUint16(data[6:], uint16(headerSize))

	// Script table
	scriptOff := headerSize
	binary.BigEndian.PutUint16(data[scriptOff:], 4)   // offset to default LangSys (relative to script)
	binary.BigEndian.PutUint16(data[scriptOff+2:], 0) // no other language systems

	// DefaultLangSys
	langSysOff := scriptOff + 4
	binary.BigEndian.PutUint16(data[langSysOff:], 0)        // lookupOrder (reserved)
	binary.BigEndian.PutUint16(data[langSysOff+2:], 0xFFFF) // no required feature
	binary.BigEndian.PutUint16(data[langSysOff+4:], 1)      // 1 feature
	binary.BigEndian.PutUint16(data[langSysOff+6:], 0)      // feature index 0

	return data
}

func (b *gsubBuilder) buildFeatureList(numLookups int) []byte {
	// Single 'liga' feature that includes all lookups
	// FeatureList: featureCount(2) + featureRecords[](6*n) + Feature tables

	featureCount := 1
	headerSize := 2 + featureCount*6

	// Feature table: featureParams(2) + lookupIndexCount(2) + lookupListIndices[](2*n)
	featureSize := 4 + numLookups*2

	totalSize := headerSize + featureSize
	data := make([]byte, totalSize)

	// Header
	binary.BigEndian.PutUint16(data[0:], uint16(featureCount))
	// 'liga' feature tag
	copy(data[2:], []byte("liga"))
	binary.BigEndian.PutUint16(data[6:], uint16(headerSize))

	// Feature table
	featureOff := headerSize
	binary.BigEndian.PutUint16(data[featureOff:], 0) // no feature params
	binary.BigEndian.PutUint16(data[featureOff+2:], uint16(numLookups))
	for i := 0; i < numLookups; i++ {
		binary.BigEndian.PutUint16(data[featureOff+4+i*2:], uint16(i))
	}

	return data
}

// buildCoverageFormat1 builds a format 1 coverage table from sorted glyphs.
func buildCoverageFormat1(glyphs []ot.GlyphID) []byte {
	// Ensure sorted
	sort.Slice(glyphs, func(i, j int) bool { return glyphs[i] < glyphs[j] })

	// Format 1: format(2) + glyphCount(2) + glyphArray[](2*n)
	data := make([]byte, 4+len(glyphs)*2)
	binary.BigEndian.PutUint16(data[0:], 1)
	binary.BigEndian.PutUint16(data[2:], uint16(len(glyphs)))
	for i, g := range glyphs {
		binary.BigEndian.PutUint16(data[4+i*2:], uint16(g))
	}
	return data
}
