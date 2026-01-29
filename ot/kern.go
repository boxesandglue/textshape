package ot

import "encoding/binary"

// Kern represents the TrueType 'kern' table.
// This provides kerning as a fallback when GPOS is not available.
type Kern struct {
	subtables []kernSubtable
}

type kernSubtable interface {
	// KernPair returns the kerning value for a glyph pair.
	// Returns 0 if no kerning is defined.
	KernPair(left, right GlyphID) int16
}

// TagKernTable is the tag for the kern table.
var TagKernTable = MakeTag('k', 'e', 'r', 'n')

// ParseKern parses a TrueType kern table.
func ParseKern(data []byte, numGlyphs int) (*Kern, error) {
	if len(data) < 4 {
		return nil, ErrInvalidTable
	}

	version := binary.BigEndian.Uint16(data)

	var subtables []kernSubtable
	var err error

	switch version {
	case 0:
		// Microsoft format
		subtables, err = parseKernMicrosoft(data, numGlyphs)
	case 1:
		// Apple format
		subtables, err = parseKernApple(data, numGlyphs)
	default:
		return nil, ErrInvalidTable
	}

	if err != nil {
		return nil, err
	}

	return &Kern{subtables: subtables}, nil
}

// parseKernMicrosoft parses Microsoft kern table format.
// Header: version (2), nTables (2)
func parseKernMicrosoft(data []byte, numGlyphs int) ([]kernSubtable, error) {
	if len(data) < 4 {
		return nil, ErrInvalidTable
	}

	nTables := binary.BigEndian.Uint16(data[2:])
	offset := 4

	var subtables []kernSubtable
	for i := 0; i < int(nTables); i++ {
		if offset+6 > len(data) {
			break
		}

		// Subtable header: version (2), length (2), coverage (2)
		length := int(binary.BigEndian.Uint16(data[offset+2:]))
		coverage := binary.BigEndian.Uint16(data[offset+4:])
		format := coverage >> 8

		// Only process horizontal kerning (bit 0 = 1 means horizontal)
		// and non-cross-stream (bit 2 = 0)
		isHorizontal := coverage&0x01 != 0
		isCrossStream := coverage&0x04 != 0

		if isHorizontal && !isCrossStream {
			subtableData := data[offset:]
			if length > len(subtableData) {
				length = len(subtableData)
			}

			var st kernSubtable
			var err error

			switch format {
			case 0:
				st, err = parseKernFormat0(subtableData, 6)
			case 2:
				st, err = parseKernFormat2(subtableData, 6, numGlyphs)
			}

			if err == nil && st != nil {
				subtables = append(subtables, st)
			}
		}

		offset += length
		if length == 0 {
			break // Prevent infinite loop
		}
	}

	return subtables, nil
}

// parseKernApple parses Apple kern table format.
func parseKernApple(data []byte, numGlyphs int) ([]kernSubtable, error) {
	if len(data) < 4 {
		return nil, ErrInvalidTable
	}

	// Check if new or old Apple format
	// New format: version (2), padding (2), nTables (4)
	// Old format: version (2), nTables (2)
	var nTables uint32
	var headerSize int

	nextWord := binary.BigEndian.Uint16(data[2:])
	if nextWord == 0 && len(data) >= 8 {
		// New Apple format
		nTables = binary.BigEndian.Uint32(data[4:])
		headerSize = 8
	} else {
		// Old Apple format
		nTables = uint32(nextWord)
		headerSize = 4
	}

	offset := headerSize
	var subtables []kernSubtable

	for i := uint32(0); i < nTables; i++ {
		if offset+8 > len(data) {
			break
		}

		// Apple subtable header: length (4), coverage (2), tupleIndex (2)
		length := int(binary.BigEndian.Uint32(data[offset:]))
		coverage := binary.BigEndian.Uint16(data[offset+4:])
		format := coverage & 0xFF

		// Check for vertical and cross-stream flags
		isVertical := coverage&0x8000 != 0
		isCrossStream := coverage&0x4000 != 0

		if !isVertical && !isCrossStream {
			subtableData := data[offset:]
			if length > len(subtableData) {
				length = len(subtableData)
			}

			var st kernSubtable
			var err error

			switch format {
			case 0:
				st, err = parseKernFormat0(subtableData, 8)
			case 2:
				st, err = parseKernFormat2(subtableData, 8, numGlyphs)
			case 3:
				st, err = parseKernFormat3(subtableData, 8, numGlyphs)
			}

			if err == nil && st != nil {
				subtables = append(subtables, st)
			}
		}

		offset += length
		if length == 0 {
			break
		}
	}

	return subtables, nil
}

// kernFormat0 is simple pair kerning (glyph pairs).
type kernFormat0 struct {
	pairs map[uint32]int16 // key: left<<16 | right
}

func parseKernFormat0(data []byte, headerSize int) (*kernFormat0, error) {
	if len(data) < headerSize+8 {
		return nil, ErrInvalidTable
	}

	offset := headerSize
	nPairs := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 8 // skip nPairs, searchRange, entrySelector, rangeShift

	if len(data) < offset+nPairs*6 {
		return nil, ErrInvalidTable
	}

	pairs := make(map[uint32]int16, nPairs)
	for i := 0; i < nPairs; i++ {
		left := binary.BigEndian.Uint16(data[offset:])
		right := binary.BigEndian.Uint16(data[offset+2:])
		value := int16(binary.BigEndian.Uint16(data[offset+4:]))
		pairs[uint32(left)<<16|uint32(right)] = value
		offset += 6
	}

	return &kernFormat0{pairs: pairs}, nil
}

func (k *kernFormat0) KernPair(left, right GlyphID) int16 {
	return k.pairs[uint32(left)<<16|uint32(right)]
}

// kernFormat2 is class-based kerning.
type kernFormat2 struct {
	leftClasses  []uint16 // class value for each glyph (indexed by glyph - firstGlyph)
	rightClasses []uint16
	leftFirst    uint16
	rightFirst   uint16
	leftCount    uint16
	rightCount   uint16
	rowWidth     uint16 // bytes per row in kern array
	kernArray    []byte // raw kern values
	arrayOffset  int    // offset of array in subtable
}

func parseKernFormat2(data []byte, headerSize int, numGlyphs int) (*kernFormat2, error) {
	if len(data) < headerSize+8 {
		return nil, ErrInvalidTable
	}

	offset := headerSize
	rowWidth := binary.BigEndian.Uint16(data[offset:])
	leftClassOffset := binary.BigEndian.Uint16(data[offset+2:])
	rightClassOffset := binary.BigEndian.Uint16(data[offset+4:])
	kernArrayOffset := binary.BigEndian.Uint16(data[offset+6:])

	k := &kernFormat2{
		rowWidth:    rowWidth,
		arrayOffset: int(kernArrayOffset),
	}

	// Parse left class table
	if int(leftClassOffset)+4 > len(data) {
		return nil, ErrInvalidTable
	}
	leftStart := int(leftClassOffset)
	k.leftFirst = binary.BigEndian.Uint16(data[leftStart:])
	k.leftCount = binary.BigEndian.Uint16(data[leftStart+2:])

	if leftStart+4+int(k.leftCount)*2 > len(data) {
		return nil, ErrInvalidTable
	}
	k.leftClasses = make([]uint16, k.leftCount)
	for i := uint16(0); i < k.leftCount; i++ {
		k.leftClasses[i] = binary.BigEndian.Uint16(data[leftStart+4+int(i)*2:])
	}

	// Parse right class table
	if int(rightClassOffset)+4 > len(data) {
		return nil, ErrInvalidTable
	}
	rightStart := int(rightClassOffset)
	k.rightFirst = binary.BigEndian.Uint16(data[rightStart:])
	k.rightCount = binary.BigEndian.Uint16(data[rightStart+2:])

	if rightStart+4+int(k.rightCount)*2 > len(data) {
		return nil, ErrInvalidTable
	}
	k.rightClasses = make([]uint16, k.rightCount)
	for i := uint16(0); i < k.rightCount; i++ {
		k.rightClasses[i] = binary.BigEndian.Uint16(data[rightStart+4+int(i)*2:])
	}

	// Store kern array reference
	if int(kernArrayOffset) < len(data) {
		k.kernArray = data[kernArrayOffset:]
	}

	return k, nil
}

func (k *kernFormat2) KernPair(left, right GlyphID) int16 {
	// Apple kern format 2: class values are pre-multiplied byte offsets
	// - leftClass = (rowIndex * rowWidth) + arrayOffset (offset from subtable start)
	// - rightClass = (columnIndex * 2) (byte offset within row)
	// For glyphs outside range:
	// - leftClass defaults to arrayOffset (row 0)
	// - rightClass defaults to 0 (column 0)

	var leftClass uint16 = uint16(k.arrayOffset) // Default: row 0
	if left >= GlyphID(k.leftFirst) && left < GlyphID(k.leftFirst)+GlyphID(k.leftCount) {
		leftClass = k.leftClasses[left-GlyphID(k.leftFirst)]
	}

	var rightClass uint16 = 0 // Default: column 0
	if right >= GlyphID(k.rightFirst) && right < GlyphID(k.rightFirst)+GlyphID(k.rightCount) {
		rightClass = k.rightClasses[right-GlyphID(k.rightFirst)]
	}

	// The sum gives address relative to subtable start
	// But kernArray starts at arrayOffset, so subtract it
	address := int(leftClass) + int(rightClass)
	kernIdx := address - k.arrayOffset

	if kernIdx < 0 || kernIdx+2 > len(k.kernArray) {
		return 0
	}

	return int16(binary.BigEndian.Uint16(k.kernArray[kernIdx:]))
}

// kernFormat3 is Apple's compact class-based format.
type kernFormat3 struct {
	leftClasses  []uint8
	rightClasses []uint8
	kernIndex    []uint8 // 2D index: [leftClass][rightClass]
	kernValues   []int16
	leftCount    uint8
	rightCount   uint8
}

func parseKernFormat3(data []byte, headerSize int, numGlyphs int) (*kernFormat3, error) {
	if len(data) < headerSize+6 {
		return nil, ErrInvalidTable
	}

	offset := headerSize
	glyphCount := int(binary.BigEndian.Uint16(data[offset:]))
	kernValueCount := data[offset+2]
	leftClassCount := data[offset+3]
	rightClassCount := data[offset+4]
	// flags at offset+5 is ignored

	offset += 6

	// Check bounds
	needed := 2*int(kernValueCount) + 2*glyphCount + int(leftClassCount)*int(rightClassCount)
	if len(data) < offset+needed {
		return nil, ErrInvalidTable
	}

	k := &kernFormat3{
		leftCount:  leftClassCount,
		rightCount: rightClassCount,
	}

	// Parse kern values
	k.kernValues = make([]int16, kernValueCount)
	for i := range k.kernValues {
		k.kernValues[i] = int16(binary.BigEndian.Uint16(data[offset+i*2:]))
	}
	offset += 2 * int(kernValueCount)

	// Parse class arrays
	k.leftClasses = make([]uint8, glyphCount)
	copy(k.leftClasses, data[offset:offset+glyphCount])
	offset += glyphCount

	k.rightClasses = make([]uint8, glyphCount)
	copy(k.rightClasses, data[offset:offset+glyphCount])
	offset += glyphCount

	// Parse index array
	indexSize := int(leftClassCount) * int(rightClassCount)
	k.kernIndex = make([]uint8, indexSize)
	copy(k.kernIndex, data[offset:offset+indexSize])

	return k, nil
}

func (k *kernFormat3) KernPair(left, right GlyphID) int16 {
	if int(left) >= len(k.leftClasses) || int(right) >= len(k.rightClasses) {
		return 0
	}

	leftClass := k.leftClasses[left]
	rightClass := k.rightClasses[right]

	if leftClass >= k.leftCount || rightClass >= k.rightCount {
		return 0
	}

	idx := int(leftClass)*int(k.rightCount) + int(rightClass)
	if idx >= len(k.kernIndex) {
		return 0
	}

	valueIdx := k.kernIndex[idx]
	if int(valueIdx) >= len(k.kernValues) {
		return 0
	}

	return k.kernValues[valueIdx]
}

// KernPair returns the kerning value for a glyph pair.
// It checks all subtables and returns the first non-zero value.
func (k *Kern) KernPair(left, right GlyphID) int16 {
	for _, st := range k.subtables {
		if v := st.KernPair(left, right); v != 0 {
			return v
		}
	}
	return 0
}

// HasKerning returns true if any kerning data is available.
func (k *Kern) HasKerning() bool {
	return len(k.subtables) > 0
}
