package subset

import (
	"encoding/binary"
	"sort"

	"github.com/boxesandglue/textshape/ot"
)

// subsetGDEF creates a subsetted GDEF table with remapped glyph IDs.
func (p *Plan) subsetGDEF() ([]byte, error) {
	if p.gdef == nil {
		return nil, nil
	}

	builder := newGDEFBuilder(p.glyphMap, p.glyphSet, p.gdef)
	return builder.build()
}

// gdefBuilder builds a subsetted GDEF table.
type gdefBuilder struct {
	glyphMap map[ot.GlyphID]ot.GlyphID
	glyphSet map[ot.GlyphID]bool
	gdef     *ot.GDEF
}

func newGDEFBuilder(glyphMap map[ot.GlyphID]ot.GlyphID, glyphSet map[ot.GlyphID]bool, gdef *ot.GDEF) *gdefBuilder {
	return &gdefBuilder{
		glyphMap: glyphMap,
		glyphSet: glyphSet,
		gdef:     gdef,
	}
}

func (b *gdefBuilder) build() ([]byte, error) {
	major, minor := b.gdef.Version()

	// Build individual components
	var glyphClassDef, attachList, ligCaretList, markAttachClassDef, markGlyphSetsDef []byte

	if b.gdef.HasGlyphClasses() {
		glyphClassDef = b.subsetGlyphClassDef()
	}

	if b.gdef.HasAttachList() {
		attachList = b.subsetAttachList()
	}

	if b.gdef.HasLigCaretList() {
		ligCaretList = b.subsetLigCaretList()
	}

	if b.gdef.HasMarkAttachClasses() {
		markAttachClassDef = b.subsetMarkAttachClassDef()
	}

	if minor >= 2 && b.gdef.HasMarkGlyphSets() {
		markGlyphSetsDef = b.subsetMarkGlyphSetsDef()
	}

	// Calculate header size
	headerSize := 12 // version(4) + 4 offsets(2 each)
	if minor >= 2 {
		headerSize = 14 // + markGlyphSetsDefOffset(2)
	}

	// Calculate offsets
	offset := headerSize

	glyphClassDefOffset := uint16(0)
	if len(glyphClassDef) > 0 {
		glyphClassDefOffset = uint16(offset)
		offset += len(glyphClassDef)
	}

	attachListOffset := uint16(0)
	if len(attachList) > 0 {
		attachListOffset = uint16(offset)
		offset += len(attachList)
	}

	ligCaretListOffset := uint16(0)
	if len(ligCaretList) > 0 {
		ligCaretListOffset = uint16(offset)
		offset += len(ligCaretList)
	}

	markAttachClassDefOffset := uint16(0)
	if len(markAttachClassDef) > 0 {
		markAttachClassDefOffset = uint16(offset)
		offset += len(markAttachClassDef)
	}

	markGlyphSetsDefOffset := uint16(0)
	if minor >= 2 && len(markGlyphSetsDef) > 0 {
		markGlyphSetsDefOffset = uint16(offset)
		offset += len(markGlyphSetsDef)
	}

	// Build final table
	totalSize := offset
	data := make([]byte, totalSize)

	// Header
	binary.BigEndian.PutUint16(data[0:], major)
	binary.BigEndian.PutUint16(data[2:], minor)
	binary.BigEndian.PutUint16(data[4:], glyphClassDefOffset)
	binary.BigEndian.PutUint16(data[6:], attachListOffset)
	binary.BigEndian.PutUint16(data[8:], ligCaretListOffset)
	binary.BigEndian.PutUint16(data[10:], markAttachClassDefOffset)

	if minor >= 2 {
		binary.BigEndian.PutUint16(data[12:], markGlyphSetsDefOffset)
	}

	// Copy component data
	off := headerSize
	if len(glyphClassDef) > 0 {
		copy(data[off:], glyphClassDef)
		off += len(glyphClassDef)
	}
	if len(attachList) > 0 {
		copy(data[off:], attachList)
		off += len(attachList)
	}
	if len(ligCaretList) > 0 {
		copy(data[off:], ligCaretList)
		off += len(ligCaretList)
	}
	if len(markAttachClassDef) > 0 {
		copy(data[off:], markAttachClassDef)
		off += len(markAttachClassDef)
	}
	if minor >= 2 && len(markGlyphSetsDef) > 0 {
		copy(data[off:], markGlyphSetsDef)
	}

	return data, nil
}

// subsetGlyphClassDef subsets the GlyphClassDef.
func (b *gdefBuilder) subsetGlyphClassDef() []byte {
	var entries []classEntry

	for oldGlyph := range b.glyphSet {
		class := b.gdef.GetGlyphClass(oldGlyph)
		if class != ot.GlyphClassUnclassified {
			if newGlyph, ok := b.glyphMap[oldGlyph]; ok {
				entries = append(entries, classEntry{newGlyph, uint16(class)})
			}
		}
	}

	if len(entries) == 0 {
		return nil
	}

	return buildClassDefFormat2(entries)
}

// subsetAttachList subsets the AttachList.
func (b *gdefBuilder) subsetAttachList() []byte {
	type attachEntry struct {
		points []uint16
		glyph  ot.GlyphID
	}
	var entries []attachEntry

	for oldGlyph := range b.glyphSet {
		points := b.gdef.GetAttachPoints(oldGlyph)
		if len(points) > 0 {
			if newGlyph, ok := b.glyphMap[oldGlyph]; ok {
				entries = append(entries, attachEntry{glyph: newGlyph, points: points})
			}
		}
	}

	if len(entries) == 0 {
		return nil
	}

	// Sort by glyph
	sort.Slice(entries, func(i, j int) bool { return entries[i].glyph < entries[j].glyph })

	// Build coverage
	glyphs := make([]ot.GlyphID, len(entries))
	for i, e := range entries {
		glyphs[i] = e.glyph
	}
	coverage := buildCoverageFormat1(glyphs)

	// Header: coverageOffset(2) + glyphCount(2) + attachPointOffsets[]
	headerSize := 4 + len(entries)*2

	// Build AttachPoint tables
	attachPointData := make([]byte, 0)
	attachPointOffsets := make([]uint16, len(entries))

	for i, e := range entries {
		attachPointOffsets[i] = uint16(headerSize + len(attachPointData))

		// AttachPoint: pointCount(2) + pointIndices[]
		apSize := 2 + len(e.points)*2
		ap := make([]byte, apSize)
		binary.BigEndian.PutUint16(ap[0:], uint16(len(e.points)))
		for j, pt := range e.points {
			binary.BigEndian.PutUint16(ap[2+j*2:], pt)
		}
		attachPointData = append(attachPointData, ap...)
	}

	totalSize := headerSize + len(attachPointData) + len(coverage)
	data := make([]byte, totalSize)

	// Coverage offset (relative to start of AttachList)
	binary.BigEndian.PutUint16(data[0:], uint16(headerSize+len(attachPointData)))
	binary.BigEndian.PutUint16(data[2:], uint16(len(entries)))

	for i, off := range attachPointOffsets {
		binary.BigEndian.PutUint16(data[4+i*2:], off)
	}

	copy(data[headerSize:], attachPointData)
	copy(data[headerSize+len(attachPointData):], coverage)

	return data
}

// subsetLigCaretList subsets the LigCaretList.
func (b *gdefBuilder) subsetLigCaretList() []byte {
	type caretEntry struct {
		carets []ot.CaretValue
		glyph  ot.GlyphID
	}
	var entries []caretEntry

	for oldGlyph := range b.glyphSet {
		carets := b.gdef.GetLigCarets(oldGlyph)
		if len(carets) > 0 {
			if newGlyph, ok := b.glyphMap[oldGlyph]; ok {
				entries = append(entries, caretEntry{glyph: newGlyph, carets: carets})
			}
		}
	}

	if len(entries) == 0 {
		return nil
	}

	// Sort by glyph
	sort.Slice(entries, func(i, j int) bool { return entries[i].glyph < entries[j].glyph })

	// Build coverage
	glyphs := make([]ot.GlyphID, len(entries))
	for i, e := range entries {
		glyphs[i] = e.glyph
	}
	coverage := buildCoverageFormat1(glyphs)

	// Header: coverageOffset(2) + ligGlyphCount(2) + ligGlyphOffsets[]
	headerSize := 4 + len(entries)*2

	// Build LigGlyph tables
	ligGlyphData := make([]byte, 0)
	ligGlyphOffsets := make([]uint16, len(entries))

	for i, e := range entries {
		ligGlyphOffsets[i] = uint16(headerSize + len(ligGlyphData))

		// Calculate LigGlyph table size
		// LigGlyph: caretCount(2) + caretValueOffsets[]
		ligGlyphHeaderSize := 2 + len(e.carets)*2

		// Build CaretValue tables
		caretValueData := make([]byte, 0)
		caretValueOffsets := make([]uint16, len(e.carets))

		for j, cv := range e.carets {
			caretValueOffsets[j] = uint16(ligGlyphHeaderSize + len(caretValueData))

			// CaretValue: format(2) + coordinate/pointIndex(2)
			cvData := make([]byte, 4)
			binary.BigEndian.PutUint16(cvData[0:], cv.Format())
			switch cv.Format() {
			case 1, 3:
				binary.BigEndian.PutUint16(cvData[2:], uint16(cv.Coordinate()))
			case 2:
				binary.BigEndian.PutUint16(cvData[2:], cv.PointIndex())
			}
			caretValueData = append(caretValueData, cvData...)
		}

		// Build LigGlyph table
		lgSize := ligGlyphHeaderSize + len(caretValueData)
		lg := make([]byte, lgSize)
		binary.BigEndian.PutUint16(lg[0:], uint16(len(e.carets)))
		for j, off := range caretValueOffsets {
			binary.BigEndian.PutUint16(lg[2+j*2:], off)
		}
		copy(lg[ligGlyphHeaderSize:], caretValueData)

		ligGlyphData = append(ligGlyphData, lg...)
	}

	totalSize := headerSize + len(ligGlyphData) + len(coverage)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], uint16(headerSize+len(ligGlyphData)))
	binary.BigEndian.PutUint16(data[2:], uint16(len(entries)))

	for i, off := range ligGlyphOffsets {
		binary.BigEndian.PutUint16(data[4+i*2:], off)
	}

	copy(data[headerSize:], ligGlyphData)
	copy(data[headerSize+len(ligGlyphData):], coverage)

	return data
}

// subsetMarkAttachClassDef subsets the MarkAttachClassDef.
func (b *gdefBuilder) subsetMarkAttachClassDef() []byte {
	var entries []classEntry

	for oldGlyph := range b.glyphSet {
		class := b.gdef.GetMarkAttachClass(oldGlyph)
		if class != 0 {
			if newGlyph, ok := b.glyphMap[oldGlyph]; ok {
				entries = append(entries, classEntry{newGlyph, uint16(class)})
			}
		}
	}

	if len(entries) == 0 {
		return nil
	}

	return buildClassDefFormat2(entries)
}

// subsetMarkGlyphSetsDef subsets the MarkGlyphSetsDef.
func (b *gdefBuilder) subsetMarkGlyphSetsDef() []byte {
	markSetCount := b.gdef.MarkGlyphSetCount()
	if markSetCount == 0 {
		return nil
	}

	// Build subset coverages for each set
	var coverages [][]byte
	for i := 0; i < markSetCount; i++ {
		cov := b.subsetMarkGlyphSet(i)
		coverages = append(coverages, cov)
	}

	// Check if all coverages are empty
	allEmpty := true
	for _, cov := range coverages {
		if len(cov) > 0 {
			allEmpty = false
			break
		}
	}
	if allEmpty {
		return nil
	}

	// Header: format(2) + markSetCount(2) + coverageOffsets[](4*n)
	headerSize := 4 + markSetCount*4

	// Calculate total size and offsets
	offset := headerSize
	offsets := make([]uint32, markSetCount)

	for i, cov := range coverages {
		if len(cov) > 0 {
			offsets[i] = uint32(offset)
			offset += len(cov)
		}
	}

	totalSize := offset
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1) // format
	binary.BigEndian.PutUint16(data[2:], uint16(markSetCount))

	for i, off := range offsets {
		binary.BigEndian.PutUint32(data[4+i*4:], off)
	}

	// Copy coverages
	offset = headerSize
	for _, cov := range coverages {
		if len(cov) > 0 {
			copy(data[offset:], cov)
			offset += len(cov)
		}
	}

	return data
}

// subsetMarkGlyphSet subsets a single mark glyph set.
func (b *gdefBuilder) subsetMarkGlyphSet(setIndex int) []byte {
	var newGlyphs []ot.GlyphID

	for oldGlyph := range b.glyphSet {
		if b.gdef.IsInMarkGlyphSet(oldGlyph, setIndex) {
			if newGlyph, ok := b.glyphMap[oldGlyph]; ok {
				newGlyphs = append(newGlyphs, newGlyph)
			}
		}
	}

	if len(newGlyphs) == 0 {
		return nil
	}

	return buildCoverageFormat1(newGlyphs)
}
