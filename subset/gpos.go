package subset

import (
	"encoding/binary"
	"sort"

	"github.com/boxesandglue/textshape/ot"
)

// subsetGPOS creates a subsetted GPOS table with remapped glyph IDs.
func (p *Plan) subsetGPOS() ([]byte, error) {
	if p.gpos == nil {
		return nil, nil
	}

	// Get the 'kern' feature's lookups
	featList, err := p.gpos.ParseFeatureList()
	if err != nil {
		return nil, nil
	}

	kernLookups := featList.FindFeature(ot.TagKern)
	if len(kernLookups) == 0 {
		return nil, nil
	}

	builder := newGPOSBuilder(p.glyphMap, p.glyphSet)

	// Only process lookups referenced by 'kern' feature
	for _, lookupIdx := range kernLookups {
		lookup := p.gpos.GetLookup(int(lookupIdx))
		if lookup == nil {
			continue
		}

		subsetLookup := builder.subsetLookup(lookup)
		if subsetLookup != nil {
			builder.addLookup(subsetLookup)
		}
	}

	// If no lookups remain, return nil (don't include empty GPOS)
	if len(builder.lookups) == 0 {
		return nil, nil
	}

	return builder.build()
}

// gposBuilder builds a subsetted GPOS table.
type gposBuilder struct {
	glyphMap map[ot.GlyphID]ot.GlyphID
	glyphSet map[ot.GlyphID]bool
	lookups  []*gposLookupBuilder
}

type gposLookupBuilder struct {
	subtables  [][]byte
	lookupType uint16
	flag       uint16
}

func newGPOSBuilder(glyphMap map[ot.GlyphID]ot.GlyphID, glyphSet map[ot.GlyphID]bool) *gposBuilder {
	return &gposBuilder{
		glyphMap: glyphMap,
		glyphSet: glyphSet,
	}
}

func (b *gposBuilder) addLookup(lookup *gposLookupBuilder) {
	b.lookups = append(b.lookups, lookup)
}

// subsetLookup subsets a single lookup, returning nil if empty.
func (b *gposBuilder) subsetLookup(lookup *ot.GPOSLookup) *gposLookupBuilder {
	lb := &gposLookupBuilder{
		lookupType: lookup.Type,
		flag:       lookup.Flag,
	}

	for _, subtable := range lookup.Subtables() {
		var data []byte

		switch st := subtable.(type) {
		case *ot.SinglePos:
			data = b.subsetSinglePos(st)
		case *ot.PairPos:
			data = b.subsetPairPos(st)
		case *ot.CursivePos:
			data = b.subsetCursivePos(st)
		case *ot.MarkBasePos:
			data = b.subsetMarkBasePos(st)
		case *ot.MarkLigPos:
			data = b.subsetMarkLigPos(st)
		case *ot.MarkMarkPos:
			data = b.subsetMarkMarkPos(st)
			// TODO: Context, ChainContext
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

// subsetSinglePos subsets a SinglePos subtable.
func (b *gposBuilder) subsetSinglePos(sp *ot.SinglePos) []byte {
	covGlyphs := sp.Coverage().Glyphs()
	if len(covGlyphs) == 0 {
		return nil
	}

	format := sp.Format()
	valueFormat := sp.ValueFormat()

	if format == 1 {
		// Format 1: single value for all covered glyphs
		var newGlyphs []ot.GlyphID
		for _, g := range covGlyphs {
			if newG, ok := b.glyphMap[g]; ok {
				newGlyphs = append(newGlyphs, newG)
			}
		}
		if len(newGlyphs) == 0 {
			return nil
		}
		sort.Slice(newGlyphs, func(i, j int) bool { return newGlyphs[i] < newGlyphs[j] })
		return b.buildSinglePosFormat1(newGlyphs, sp.ValueRecord(), valueFormat)
	}

	// Format 2: per-glyph values
	type entry struct {
		glyph ot.GlyphID
		vr    ot.ValueRecord
	}
	var entries []entry

	valueRecords := sp.ValueRecords()
	for i, g := range covGlyphs {
		if newG, ok := b.glyphMap[g]; ok {
			if i < len(valueRecords) {
				entries = append(entries, entry{newG, valueRecords[i]})
			}
		}
	}
	if len(entries) == 0 {
		return nil
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].glyph < entries[j].glyph })

	glyphs := make([]ot.GlyphID, len(entries))
	vrs := make([]ot.ValueRecord, len(entries))
	for i, e := range entries {
		glyphs[i] = e.glyph
		vrs[i] = e.vr
	}

	return b.buildSinglePosFormat2(glyphs, vrs, valueFormat)
}

func (b *gposBuilder) buildSinglePosFormat1(glyphs []ot.GlyphID, vr ot.ValueRecord, valueFormat uint16) []byte {
	coverage := buildCoverageFormat1(glyphs)
	vrSize := valueRecordSize(valueFormat)

	// Format 1: format(2) + coverageOffset(2) + valueFormat(2) + valueRecord(vrSize)
	headerSize := 6 + vrSize
	data := make([]byte, headerSize+len(coverage))

	binary.BigEndian.PutUint16(data[0:], 1)
	binary.BigEndian.PutUint16(data[2:], uint16(headerSize))
	binary.BigEndian.PutUint16(data[4:], valueFormat)
	writeValueRecord(data[6:], vr, valueFormat)
	copy(data[headerSize:], coverage)

	return data
}

func (b *gposBuilder) buildSinglePosFormat2(glyphs []ot.GlyphID, vrs []ot.ValueRecord, valueFormat uint16) []byte {
	coverage := buildCoverageFormat1(glyphs)
	vrSize := valueRecordSize(valueFormat)

	// Format 2: format(2) + coverageOffset(2) + valueFormat(2) + valueCount(2) + valueRecords[]
	headerSize := 8 + len(vrs)*vrSize
	data := make([]byte, headerSize+len(coverage))

	binary.BigEndian.PutUint16(data[0:], 2)
	binary.BigEndian.PutUint16(data[2:], uint16(headerSize))
	binary.BigEndian.PutUint16(data[4:], valueFormat)
	binary.BigEndian.PutUint16(data[6:], uint16(len(vrs)))

	off := 8
	for _, vr := range vrs {
		writeValueRecord(data[off:], vr, valueFormat)
		off += vrSize
	}
	copy(data[headerSize:], coverage)

	return data
}

// subsetPairPos subsets a PairPos subtable.
func (b *gposBuilder) subsetPairPos(pp *ot.PairPos) []byte {
	format := pp.Format()

	if format == 1 {
		return b.subsetPairPosFormat1(pp)
	}
	if format == 2 {
		return b.subsetPairPosFormat2(pp)
	}
	return nil
}

// pairSetEntry holds a remapped pair set.
type pairSetEntry struct {
	pairs      []pairValueEntry
	firstGlyph ot.GlyphID
}

// pairValueEntry holds a remapped pair value.
type pairValueEntry struct {
	secondGlyph ot.GlyphID
	value1      ot.ValueRecord
	value2      ot.ValueRecord
}

func (b *gposBuilder) subsetPairPosFormat1(pp *ot.PairPos) []byte {
	covGlyphs := pp.Coverage().Glyphs()
	pairSets := pp.PairSets()

	if len(covGlyphs) == 0 || len(pairSets) == 0 {
		return nil
	}

	var sets []pairSetEntry

	for i, firstGlyph := range covGlyphs {
		newFirst, okFirst := b.glyphMap[firstGlyph]
		if !okFirst || i >= len(pairSets) {
			continue
		}

		var pairs []pairValueEntry
		for _, pvr := range pairSets[i] {
			newSecond, okSecond := b.glyphMap[pvr.SecondGlyph]
			if okSecond {
				pairs = append(pairs, pairValueEntry{
					secondGlyph: newSecond,
					value1:      pvr.Value1,
					value2:      pvr.Value2,
				})
			}
		}

		if len(pairs) > 0 {
			// Sort pairs by second glyph
			sort.Slice(pairs, func(i, j int) bool {
				return pairs[i].secondGlyph < pairs[j].secondGlyph
			})
			sets = append(sets, pairSetEntry{firstGlyph: newFirst, pairs: pairs})
		}
	}

	if len(sets) == 0 {
		return nil
	}

	// Sort by first glyph
	sort.Slice(sets, func(i, j int) bool {
		return sets[i].firstGlyph < sets[j].firstGlyph
	})

	return b.buildPairPosFormat1(sets, pp.ValueFormat1(), pp.ValueFormat2())
}

func (b *gposBuilder) buildPairPosFormat1(sets []pairSetEntry, vf1, vf2 uint16) []byte {
	// Build coverage from first glyphs
	glyphs := make([]ot.GlyphID, len(sets))
	for i, s := range sets {
		glyphs[i] = s.firstGlyph
	}
	coverage := buildCoverageFormat1(glyphs)

	vr1Size := valueRecordSize(vf1)
	vr2Size := valueRecordSize(vf2)
	pairRecordSize := 2 + vr1Size + vr2Size

	// Header: format(2) + coverageOffset(2) + vf1(2) + vf2(2) + pairSetCount(2) + pairSetOffsets[]
	headerSize := 10 + len(sets)*2

	// Build pair set data
	pairSetData := make([]byte, 0)
	pairSetOffsets := make([]uint16, len(sets))

	for i, set := range sets {
		pairSetOffsets[i] = uint16(headerSize + len(pairSetData))

		// PairSet: pairValueCount(2) + PairValueRecords[]
		pairSetSize := 2 + len(set.pairs)*pairRecordSize
		pairSet := make([]byte, pairSetSize)

		binary.BigEndian.PutUint16(pairSet[0:], uint16(len(set.pairs)))
		off := 2
		for _, p := range set.pairs {
			binary.BigEndian.PutUint16(pairSet[off:], uint16(p.secondGlyph))
			off += 2
			writeValueRecord(pairSet[off:], p.value1, vf1)
			off += vr1Size
			writeValueRecord(pairSet[off:], p.value2, vf2)
			off += vr2Size
		}

		pairSetData = append(pairSetData, pairSet...)
	}

	totalSize := headerSize + len(pairSetData) + len(coverage)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1)                                   // format
	binary.BigEndian.PutUint16(data[2:], uint16(headerSize+len(pairSetData))) // coverage offset
	binary.BigEndian.PutUint16(data[4:], vf1)                                 // valueFormat1
	binary.BigEndian.PutUint16(data[6:], vf2)                                 // valueFormat2
	binary.BigEndian.PutUint16(data[8:], uint16(len(sets)))                   // pairSetCount

	for i, off := range pairSetOffsets {
		binary.BigEndian.PutUint16(data[10+i*2:], off)
	}

	copy(data[headerSize:], pairSetData)
	copy(data[headerSize+len(pairSetData):], coverage)

	return data
}

func (b *gposBuilder) subsetPairPosFormat2(pp *ot.PairPos) []byte {
	// For format 2 (class-based), we need to:
	// 1. Remap glyphs in coverage
	// 2. Remap glyphs in ClassDef1 and ClassDef2
	// 3. Keep the class matrix but potentially compact classes

	covGlyphs := pp.Coverage().Glyphs()
	classDef1 := pp.ClassDef1()
	classDef2 := pp.ClassDef2()

	if len(covGlyphs) == 0 || classDef1 == nil || classDef2 == nil {
		return nil
	}

	// Remap coverage
	var newCovGlyphs []ot.GlyphID
	for _, g := range covGlyphs {
		if newG, ok := b.glyphMap[g]; ok {
			newCovGlyphs = append(newCovGlyphs, newG)
		}
	}
	if len(newCovGlyphs) == 0 {
		return nil
	}
	sort.Slice(newCovGlyphs, func(i, j int) bool { return newCovGlyphs[i] < newCovGlyphs[j] })

	// Remap ClassDef1 (first glyphs)
	class1Mapping := b.remapClassDef(classDef1)
	if len(class1Mapping) == 0 {
		return nil
	}

	// Remap ClassDef2 (second glyphs)
	class2Mapping := b.remapClassDef(classDef2)
	if len(class2Mapping) == 0 {
		return nil
	}

	return b.buildPairPosFormat2(newCovGlyphs, class1Mapping, class2Mapping,
		pp.ClassMatrix(), pp.Class1Count(), pp.Class2Count(),
		pp.ValueFormat1(), pp.ValueFormat2())
}

// classEntry holds a remapped glyph and its class.
type classEntry struct {
	glyph ot.GlyphID
	class uint16
}

func (b *gposBuilder) remapClassDef(cd *ot.ClassDef) []classEntry {
	var entries []classEntry
	for glyph, class := range cd.Mapping() {
		if newG, ok := b.glyphMap[glyph]; ok {
			entries = append(entries, classEntry{newG, class})
		}
	}
	return entries
}

func (b *gposBuilder) buildPairPosFormat2(covGlyphs []ot.GlyphID, class1, class2 []classEntry,
	classMatrix [][]ot.PairClassRecord, class1Count, class2Count uint16,
	vf1, vf2 uint16) []byte {

	coverage := buildCoverageFormat1(covGlyphs)
	classDef1 := buildClassDefFormat2(class1)
	classDef2 := buildClassDefFormat2(class2)

	vr1Size := valueRecordSize(vf1)
	vr2Size := valueRecordSize(vf2)
	classRecordSize := vr1Size + vr2Size

	// Header: format(2) + coverageOff(2) + vf1(2) + vf2(2) + classDef1Off(2) + classDef2Off(2) +
	//         class1Count(2) + class2Count(2) + Class1Records[]
	headerSize := 16
	matrixSize := int(class1Count) * int(class2Count) * classRecordSize

	classDef1Off := headerSize + matrixSize
	classDef2Off := classDef1Off + len(classDef1)
	coverageOff := classDef2Off + len(classDef2)

	totalSize := coverageOff + len(coverage)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 2)
	binary.BigEndian.PutUint16(data[2:], uint16(coverageOff))
	binary.BigEndian.PutUint16(data[4:], vf1)
	binary.BigEndian.PutUint16(data[6:], vf2)
	binary.BigEndian.PutUint16(data[8:], uint16(classDef1Off))
	binary.BigEndian.PutUint16(data[10:], uint16(classDef2Off))
	binary.BigEndian.PutUint16(data[12:], class1Count)
	binary.BigEndian.PutUint16(data[14:], class2Count)

	// Write class matrix
	off := headerSize
	for c1 := 0; c1 < int(class1Count); c1++ {
		if c1 < len(classMatrix) {
			for c2 := 0; c2 < int(class2Count); c2++ {
				if c2 < len(classMatrix[c1]) {
					writeValueRecord(data[off:], classMatrix[c1][c2].Value1, vf1)
					off += vr1Size
					writeValueRecord(data[off:], classMatrix[c1][c2].Value2, vf2)
					off += vr2Size
				} else {
					off += classRecordSize
				}
			}
		} else {
			off += int(class2Count) * classRecordSize
		}
	}

	copy(data[classDef1Off:], classDef1)
	copy(data[classDef2Off:], classDef2)
	copy(data[coverageOff:], coverage)

	return data
}

// build serializes the GPOS table.
func (b *gposBuilder) build() ([]byte, error) {
	if len(b.lookups) == 0 {
		return nil, nil
	}

	// Build lookup list
	lookupList := b.buildLookupList()

	// Build minimal script list (DFLT/dflt)
	scriptList := b.buildScriptList(len(b.lookups))

	// Build feature list (all lookups under 'kern' feature)
	featureList := b.buildFeatureList(len(b.lookups))

	// GPOS header: version(4) + scriptListOff(2) + featureListOff(2) + lookupListOff(2)
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

func (b *gposBuilder) buildLookupList() []byte {
	// LookupList: lookupCount(2) + lookupOffsets[](2*n) + Lookup tables
	headerSize := 2 + len(b.lookups)*2

	lookupData := make([]byte, 0)
	lookupOffsets := make([]uint16, len(b.lookups))

	for i, lookup := range b.lookups {
		lookupOffsets[i] = uint16(headerSize + len(lookupData))

		// Lookup: lookupType(2) + lookupFlag(2) + subTableCount(2) + subTableOffsets[]
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

func (b *gposBuilder) buildScriptList(numLookups int) []byte {
	// Minimal script list: DFLT script with dflt language system
	scriptCount := 1
	headerSize := 2 + scriptCount*6

	langSysSize := 6 + 2 // One feature index
	scriptSize := 4 + langSysSize

	totalSize := headerSize + scriptSize
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], uint16(scriptCount))
	copy(data[2:], []byte("DFLT"))
	binary.BigEndian.PutUint16(data[6:], uint16(headerSize))

	scriptOff := headerSize
	binary.BigEndian.PutUint16(data[scriptOff:], 4)
	binary.BigEndian.PutUint16(data[scriptOff+2:], 0)

	langSysOff := scriptOff + 4
	binary.BigEndian.PutUint16(data[langSysOff:], 0)
	binary.BigEndian.PutUint16(data[langSysOff+2:], 0xFFFF)
	binary.BigEndian.PutUint16(data[langSysOff+4:], 1)
	binary.BigEndian.PutUint16(data[langSysOff+6:], 0)

	return data
}

func (b *gposBuilder) buildFeatureList(numLookups int) []byte {
	// Single 'kern' feature that includes all lookups
	featureCount := 1
	headerSize := 2 + featureCount*6

	featureSize := 4 + numLookups*2

	totalSize := headerSize + featureSize
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], uint16(featureCount))
	copy(data[2:], []byte("kern"))
	binary.BigEndian.PutUint16(data[6:], uint16(headerSize))

	featureOff := headerSize
	binary.BigEndian.PutUint16(data[featureOff:], 0)
	binary.BigEndian.PutUint16(data[featureOff+2:], uint16(numLookups))
	for i := 0; i < numLookups; i++ {
		binary.BigEndian.PutUint16(data[featureOff+4+i*2:], uint16(i))
	}

	return data
}

// valueRecordSize returns the byte size of a ValueRecord with the given format.
func valueRecordSize(format uint16) int {
	count := 0
	for f := format & 0xFF; f != 0; f >>= 1 {
		if f&1 != 0 {
			count++
		}
	}
	return count * 2
}

// writeValueRecord writes a ValueRecord to data.
func writeValueRecord(data []byte, vr ot.ValueRecord, format uint16) {
	off := 0
	if format&ot.ValueFormatXPlacement != 0 {
		binary.BigEndian.PutUint16(data[off:], uint16(vr.XPlacement))
		off += 2
	}
	if format&ot.ValueFormatYPlacement != 0 {
		binary.BigEndian.PutUint16(data[off:], uint16(vr.YPlacement))
		off += 2
	}
	if format&ot.ValueFormatXAdvance != 0 {
		binary.BigEndian.PutUint16(data[off:], uint16(vr.XAdvance))
		off += 2
	}
	if format&ot.ValueFormatYAdvance != 0 {
		binary.BigEndian.PutUint16(data[off:], uint16(vr.YAdvance))
		off += 2
	}
	// Device tables (not supported, write zeros if present)
	if format&ot.ValueFormatXPlaDevice != 0 {
		off += 2
	}
	if format&ot.ValueFormatYPlaDevice != 0 {
		off += 2
	}
	if format&ot.ValueFormatXAdvDevice != 0 {
		off += 2
	}
	if format&ot.ValueFormatYAdvDevice != 0 {
		off += 2
	}
}

// buildClassDefFormat2 builds a ClassDef format 2 table from class entries.
func buildClassDefFormat2(entries []classEntry) []byte {
	if len(entries) == 0 {
		// Empty ClassDef format 1
		return []byte{0, 1, 0, 0, 0, 0}
	}

	// Sort by glyph
	sort.Slice(entries, func(i, j int) bool { return entries[i].glyph < entries[j].glyph })

	// Build ranges
	type classRange struct {
		start, end ot.GlyphID
		class      uint16
	}
	var ranges []classRange

	start := entries[0].glyph
	end := entries[0].glyph
	class := entries[0].class

	for i := 1; i < len(entries); i++ {
		e := entries[i]
		if e.glyph == end+1 && e.class == class {
			end = e.glyph
		} else {
			ranges = append(ranges, classRange{start, end, class})
			start = e.glyph
			end = e.glyph
			class = e.class
		}
	}
	ranges = append(ranges, classRange{start, end, class})

	// Format 2: format(2) + classRangeCount(2) + ClassRangeRecords[](6*n)
	data := make([]byte, 4+len(ranges)*6)
	binary.BigEndian.PutUint16(data[0:], 2)
	binary.BigEndian.PutUint16(data[2:], uint16(len(ranges)))

	for i, r := range ranges {
		off := 4 + i*6
		binary.BigEndian.PutUint16(data[off:], uint16(r.start))
		binary.BigEndian.PutUint16(data[off+2:], uint16(r.end))
		binary.BigEndian.PutUint16(data[off+4:], r.class)
	}

	return data
}

// --- CursivePos Subsetting ---

// cursiveEntry holds a remapped cursive attachment entry.
type cursiveEntry struct {
	entry *ot.Anchor
	exit  *ot.Anchor
	glyph ot.GlyphID
}

// subsetCursivePos subsets a CursivePos subtable.
func (b *gposBuilder) subsetCursivePos(cp *ot.CursivePos) []byte {
	covGlyphs := cp.Coverage().Glyphs()
	records := cp.EntryExitRecords()

	if len(covGlyphs) == 0 || len(records) == 0 {
		return nil
	}

	var entries []cursiveEntry
	for i, g := range covGlyphs {
		if i >= len(records) {
			break
		}
		if newG, ok := b.glyphMap[g]; ok {
			entries = append(entries, cursiveEntry{
				glyph: newG,
				entry: records[i].EntryAnchor,
				exit:  records[i].ExitAnchor,
			})
		}
	}

	if len(entries) == 0 {
		return nil
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].glyph < entries[j].glyph })

	return b.buildCursivePos(entries)
}

func (b *gposBuilder) buildCursivePos(entries []cursiveEntry) []byte {
	glyphs := make([]ot.GlyphID, len(entries))
	for i, e := range entries {
		glyphs[i] = e.glyph
	}
	coverage := buildCoverageFormat1(glyphs)

	// Header: format(2) + coverageOffset(2) + entryExitCount(2) + entryExitRecords[](4*n)
	headerSize := 6 + len(entries)*4

	// Build anchor data
	anchorData := make([]byte, 0)
	entryOffsets := make([]uint16, len(entries))
	exitOffsets := make([]uint16, len(entries))

	for i, e := range entries {
		if e.entry != nil {
			entryOffsets[i] = uint16(headerSize + len(anchorData))
			anchorData = append(anchorData, buildAnchor(e.entry)...)
		}
		if e.exit != nil {
			exitOffsets[i] = uint16(headerSize + len(anchorData))
			anchorData = append(anchorData, buildAnchor(e.exit)...)
		}
	}

	totalSize := headerSize + len(anchorData) + len(coverage)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1)                                  // format
	binary.BigEndian.PutUint16(data[2:], uint16(headerSize+len(anchorData))) // coverage offset
	binary.BigEndian.PutUint16(data[4:], uint16(len(entries)))               // entry/exit count

	for i := range entries {
		off := 6 + i*4
		binary.BigEndian.PutUint16(data[off:], entryOffsets[i])
		binary.BigEndian.PutUint16(data[off+2:], exitOffsets[i])
	}

	copy(data[headerSize:], anchorData)
	copy(data[headerSize+len(anchorData):], coverage)

	return data
}

// --- MarkBasePos Subsetting ---

// markEntry holds a remapped mark entry.
type markEntry struct {
	anchor *ot.Anchor
	glyph  ot.GlyphID
	class  uint16
}

// baseEntry holds a remapped base entry.
type baseEntry struct {
	anchors []*ot.Anchor // Per-class anchors
	glyph   ot.GlyphID
}

// subsetMarkBasePos subsets a MarkBasePos subtable.
func (b *gposBuilder) subsetMarkBasePos(mb *ot.MarkBasePos) []byte {
	markCovGlyphs := mb.MarkCoverage().Glyphs()
	baseCovGlyphs := mb.BaseCoverage().Glyphs()
	markArray := mb.MarkArray()
	baseArray := mb.BaseArray()
	classCount := mb.ClassCount()

	if len(markCovGlyphs) == 0 || len(baseCovGlyphs) == 0 || markArray == nil || baseArray == nil {
		return nil
	}

	// Remap marks
	var marks []markEntry
	for i, g := range markCovGlyphs {
		if i >= len(markArray.Records) {
			break
		}
		if newG, ok := b.glyphMap[g]; ok {
			rec := markArray.Records[i]
			marks = append(marks, markEntry{
				glyph:  newG,
				class:  rec.Class,
				anchor: rec.Anchor,
			})
		}
	}

	// Remap bases
	var bases []baseEntry
	for i, g := range baseCovGlyphs {
		if i >= baseArray.Rows {
			break
		}
		if newG, ok := b.glyphMap[g]; ok {
			bases = append(bases, baseEntry{
				glyph:   newG,
				anchors: baseArray.Anchors[i],
			})
		}
	}

	if len(marks) == 0 || len(bases) == 0 {
		return nil
	}

	sort.Slice(marks, func(i, j int) bool { return marks[i].glyph < marks[j].glyph })
	sort.Slice(bases, func(i, j int) bool { return bases[i].glyph < bases[j].glyph })

	return b.buildMarkBasePos(marks, bases, classCount)
}

func (b *gposBuilder) buildMarkBasePos(marks []markEntry, bases []baseEntry, classCount uint16) []byte {
	// Build coverages
	markGlyphs := make([]ot.GlyphID, len(marks))
	for i, m := range marks {
		markGlyphs[i] = m.glyph
	}
	markCoverage := buildCoverageFormat1(markGlyphs)

	baseGlyphs := make([]ot.GlyphID, len(bases))
	for i, base := range bases {
		baseGlyphs[i] = base.glyph
	}
	baseCoverage := buildCoverageFormat1(baseGlyphs)

	// Header: format(2) + markCoverageOff(2) + baseCoverageOff(2) + classCount(2) + markArrayOff(2) + baseArrayOff(2)
	headerSize := 12

	// Build MarkArray
	markArrayData := buildMarkArray(marks)
	markArrayOff := headerSize

	// Build BaseArray
	baseArrayData := buildBaseArray(bases, int(classCount))
	baseArrayOff := markArrayOff + len(markArrayData)

	// Coverage offsets
	markCoverageOff := baseArrayOff + len(baseArrayData)
	baseCoverageOff := markCoverageOff + len(markCoverage)

	totalSize := baseCoverageOff + len(baseCoverage)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1)
	binary.BigEndian.PutUint16(data[2:], uint16(markCoverageOff))
	binary.BigEndian.PutUint16(data[4:], uint16(baseCoverageOff))
	binary.BigEndian.PutUint16(data[6:], classCount)
	binary.BigEndian.PutUint16(data[8:], uint16(markArrayOff))
	binary.BigEndian.PutUint16(data[10:], uint16(baseArrayOff))

	copy(data[markArrayOff:], markArrayData)
	copy(data[baseArrayOff:], baseArrayData)
	copy(data[markCoverageOff:], markCoverage)
	copy(data[baseCoverageOff:], baseCoverage)

	return data
}

// --- MarkLigPos Subsetting ---

// ligEntry holds a remapped ligature entry.
type ligEntry struct {
	anchors [][]*ot.Anchor // [component][class]
	glyph   ot.GlyphID
}

// subsetMarkLigPos subsets a MarkLigPos subtable.
func (b *gposBuilder) subsetMarkLigPos(ml *ot.MarkLigPos) []byte {
	markCovGlyphs := ml.MarkCoverage().Glyphs()
	ligCovGlyphs := ml.LigatureCoverage().Glyphs()
	markArray := ml.MarkArray()
	ligArray := ml.LigatureArray()
	classCount := ml.ClassCount()

	if len(markCovGlyphs) == 0 || len(ligCovGlyphs) == 0 || markArray == nil || ligArray == nil {
		return nil
	}

	// Remap marks
	var marks []markEntry
	for i, g := range markCovGlyphs {
		if i >= len(markArray.Records) {
			break
		}
		if newG, ok := b.glyphMap[g]; ok {
			rec := markArray.Records[i]
			marks = append(marks, markEntry{
				glyph:  newG,
				class:  rec.Class,
				anchor: rec.Anchor,
			})
		}
	}

	// Remap ligatures
	var ligs []ligEntry
	for i, g := range ligCovGlyphs {
		if i >= len(ligArray.Attachments) {
			break
		}
		if newG, ok := b.glyphMap[g]; ok {
			la := ligArray.Attachments[i]
			ligs = append(ligs, ligEntry{
				glyph:   newG,
				anchors: la.Anchors,
			})
		}
	}

	if len(marks) == 0 || len(ligs) == 0 {
		return nil
	}

	sort.Slice(marks, func(i, j int) bool { return marks[i].glyph < marks[j].glyph })
	sort.Slice(ligs, func(i, j int) bool { return ligs[i].glyph < ligs[j].glyph })

	return b.buildMarkLigPos(marks, ligs, classCount)
}

func (b *gposBuilder) buildMarkLigPos(marks []markEntry, ligs []ligEntry, classCount uint16) []byte {
	// Build coverages
	markGlyphs := make([]ot.GlyphID, len(marks))
	for i, m := range marks {
		markGlyphs[i] = m.glyph
	}
	markCoverage := buildCoverageFormat1(markGlyphs)

	ligGlyphs := make([]ot.GlyphID, len(ligs))
	for i, l := range ligs {
		ligGlyphs[i] = l.glyph
	}
	ligCoverage := buildCoverageFormat1(ligGlyphs)

	// Header: format(2) + markCoverageOff(2) + ligCoverageOff(2) + classCount(2) + markArrayOff(2) + ligArrayOff(2)
	headerSize := 12

	// Build MarkArray
	markArrayData := buildMarkArray(marks)
	markArrayOff := headerSize

	// Build LigatureArray
	ligArrayData := buildLigatureArray(ligs, int(classCount))
	ligArrayOff := markArrayOff + len(markArrayData)

	// Coverage offsets
	markCoverageOff := ligArrayOff + len(ligArrayData)
	ligCoverageOff := markCoverageOff + len(markCoverage)

	totalSize := ligCoverageOff + len(ligCoverage)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1)
	binary.BigEndian.PutUint16(data[2:], uint16(markCoverageOff))
	binary.BigEndian.PutUint16(data[4:], uint16(ligCoverageOff))
	binary.BigEndian.PutUint16(data[6:], classCount)
	binary.BigEndian.PutUint16(data[8:], uint16(markArrayOff))
	binary.BigEndian.PutUint16(data[10:], uint16(ligArrayOff))

	copy(data[markArrayOff:], markArrayData)
	copy(data[ligArrayOff:], ligArrayData)
	copy(data[markCoverageOff:], markCoverage)
	copy(data[ligCoverageOff:], ligCoverage)

	return data
}

// --- MarkMarkPos Subsetting ---

// subsetMarkMarkPos subsets a MarkMarkPos subtable.
func (b *gposBuilder) subsetMarkMarkPos(mm *ot.MarkMarkPos) []byte {
	mark1CovGlyphs := mm.Mark1Coverage().Glyphs()
	mark2CovGlyphs := mm.Mark2Coverage().Glyphs()
	mark1Array := mm.Mark1Array()
	mark2Array := mm.Mark2Array()
	classCount := mm.ClassCount()

	if len(mark1CovGlyphs) == 0 || len(mark2CovGlyphs) == 0 || mark1Array == nil || mark2Array == nil {
		return nil
	}

	// Remap mark1
	var mark1s []markEntry
	for i, g := range mark1CovGlyphs {
		if i >= len(mark1Array.Records) {
			break
		}
		if newG, ok := b.glyphMap[g]; ok {
			rec := mark1Array.Records[i]
			mark1s = append(mark1s, markEntry{
				glyph:  newG,
				class:  rec.Class,
				anchor: rec.Anchor,
			})
		}
	}

	// Remap mark2 (uses BaseArray structure)
	var mark2s []baseEntry
	for i, g := range mark2CovGlyphs {
		if i >= mark2Array.Rows {
			break
		}
		if newG, ok := b.glyphMap[g]; ok {
			mark2s = append(mark2s, baseEntry{
				glyph:   newG,
				anchors: mark2Array.Anchors[i],
			})
		}
	}

	if len(mark1s) == 0 || len(mark2s) == 0 {
		return nil
	}

	sort.Slice(mark1s, func(i, j int) bool { return mark1s[i].glyph < mark1s[j].glyph })
	sort.Slice(mark2s, func(i, j int) bool { return mark2s[i].glyph < mark2s[j].glyph })

	return b.buildMarkMarkPos(mark1s, mark2s, classCount)
}

func (b *gposBuilder) buildMarkMarkPos(mark1s []markEntry, mark2s []baseEntry, classCount uint16) []byte {
	// Build coverages
	mark1Glyphs := make([]ot.GlyphID, len(mark1s))
	for i, m := range mark1s {
		mark1Glyphs[i] = m.glyph
	}
	mark1Coverage := buildCoverageFormat1(mark1Glyphs)

	mark2Glyphs := make([]ot.GlyphID, len(mark2s))
	for i, m := range mark2s {
		mark2Glyphs[i] = m.glyph
	}
	mark2Coverage := buildCoverageFormat1(mark2Glyphs)

	// Header: format(2) + mark1CoverageOff(2) + mark2CoverageOff(2) + classCount(2) + mark1ArrayOff(2) + mark2ArrayOff(2)
	headerSize := 12

	// Build Mark1Array
	mark1ArrayData := buildMarkArray(mark1s)
	mark1ArrayOff := headerSize

	// Build Mark2Array (same structure as BaseArray)
	mark2ArrayData := buildBaseArray(mark2s, int(classCount))
	mark2ArrayOff := mark1ArrayOff + len(mark1ArrayData)

	// Coverage offsets
	mark1CoverageOff := mark2ArrayOff + len(mark2ArrayData)
	mark2CoverageOff := mark1CoverageOff + len(mark1Coverage)

	totalSize := mark2CoverageOff + len(mark2Coverage)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1)
	binary.BigEndian.PutUint16(data[2:], uint16(mark1CoverageOff))
	binary.BigEndian.PutUint16(data[4:], uint16(mark2CoverageOff))
	binary.BigEndian.PutUint16(data[6:], classCount)
	binary.BigEndian.PutUint16(data[8:], uint16(mark1ArrayOff))
	binary.BigEndian.PutUint16(data[10:], uint16(mark2ArrayOff))

	copy(data[mark1ArrayOff:], mark1ArrayData)
	copy(data[mark2ArrayOff:], mark2ArrayData)
	copy(data[mark1CoverageOff:], mark1Coverage)
	copy(data[mark2CoverageOff:], mark2Coverage)

	return data
}

// --- Helper functions for building anchor-based tables ---

// buildAnchor builds an Anchor table (format 1).
func buildAnchor(a *ot.Anchor) []byte {
	// Format 1: format(2) + x(2) + y(2)
	data := make([]byte, 6)
	binary.BigEndian.PutUint16(data[0:], 1) // Use format 1 for simplicity
	binary.BigEndian.PutUint16(data[2:], uint16(a.X))
	binary.BigEndian.PutUint16(data[4:], uint16(a.Y))
	return data
}

// buildMarkArray builds a MarkArray table.
func buildMarkArray(marks []markEntry) []byte {
	// MarkArray: markCount(2) + markRecords[](4*n) + anchors[]
	headerSize := 2 + len(marks)*4

	anchorData := make([]byte, 0)
	anchorOffsets := make([]uint16, len(marks))

	for i, m := range marks {
		if m.anchor != nil {
			anchorOffsets[i] = uint16(headerSize + len(anchorData))
			anchorData = append(anchorData, buildAnchor(m.anchor)...)
		}
	}

	totalSize := headerSize + len(anchorData)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], uint16(len(marks)))
	for i, m := range marks {
		off := 2 + i*4
		binary.BigEndian.PutUint16(data[off:], m.class)
		binary.BigEndian.PutUint16(data[off+2:], anchorOffsets[i])
	}

	copy(data[headerSize:], anchorData)
	return data
}

// buildBaseArray builds a BaseArray (AnchorMatrix) table.
func buildBaseArray(bases []baseEntry, classCount int) []byte {
	// BaseArray: baseCount(2) + baseRecords[](2*classCount*n) + anchors[]
	headerSize := 2 + len(bases)*classCount*2

	anchorData := make([]byte, 0)
	anchorOffsets := make([][]uint16, len(bases))

	for i, base := range bases {
		anchorOffsets[i] = make([]uint16, classCount)
		for c := 0; c < classCount; c++ {
			if c < len(base.anchors) && base.anchors[c] != nil {
				anchorOffsets[i][c] = uint16(headerSize + len(anchorData))
				anchorData = append(anchorData, buildAnchor(base.anchors[c])...)
			}
		}
	}

	totalSize := headerSize + len(anchorData)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], uint16(len(bases)))
	for i := range bases {
		baseRecOff := 2 + i*classCount*2
		for c := 0; c < classCount; c++ {
			binary.BigEndian.PutUint16(data[baseRecOff+c*2:], anchorOffsets[i][c])
		}
	}

	copy(data[headerSize:], anchorData)
	return data
}

// buildLigatureArray builds a LigatureArray table.
func buildLigatureArray(ligs []ligEntry, classCount int) []byte {
	// LigatureArray: ligCount(2) + ligAttachOffsets[](2*n) + LigatureAttach tables
	headerSize := 2 + len(ligs)*2

	ligAttachData := make([]byte, 0)
	ligAttachOffsets := make([]uint16, len(ligs))

	for i, lig := range ligs {
		ligAttachOffsets[i] = uint16(headerSize + len(ligAttachData))
		ligAttachData = append(ligAttachData, buildLigatureAttach(lig.anchors, classCount)...)
	}

	totalSize := headerSize + len(ligAttachData)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], uint16(len(ligs)))
	for i, off := range ligAttachOffsets {
		binary.BigEndian.PutUint16(data[2+i*2:], off)
	}

	copy(data[headerSize:], ligAttachData)
	return data
}

// buildLigatureAttach builds a LigatureAttach table.
func buildLigatureAttach(anchors [][]*ot.Anchor, classCount int) []byte {
	componentCount := len(anchors)
	// LigatureAttach: componentCount(2) + componentRecords[](2*classCount*n) + anchors[]
	headerSize := 2 + componentCount*classCount*2

	anchorData := make([]byte, 0)
	anchorOffsets := make([][]uint16, componentCount)

	for comp := 0; comp < componentCount; comp++ {
		anchorOffsets[comp] = make([]uint16, classCount)
		for c := 0; c < classCount; c++ {
			if comp < len(anchors) && c < len(anchors[comp]) && anchors[comp][c] != nil {
				anchorOffsets[comp][c] = uint16(headerSize + len(anchorData))
				anchorData = append(anchorData, buildAnchor(anchors[comp][c])...)
			}
		}
	}

	totalSize := headerSize + len(anchorData)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], uint16(componentCount))
	for comp := 0; comp < componentCount; comp++ {
		compRecOff := 2 + comp*classCount*2
		for c := 0; c < classCount; c++ {
			binary.BigEndian.PutUint16(data[compRecOff+c*2:], anchorOffsets[comp][c])
		}
	}

	copy(data[headerSize:], anchorData)
	return data
}
