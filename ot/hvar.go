package ot

import (
	"encoding/binary"
)

// TagHvar is the table tag for the horizontal metrics variations table.
var TagHvar = MakeTag('H', 'V', 'A', 'R')

// TagVvar is the table tag for the vertical metrics variations table.
var TagVvar = MakeTag('V', 'V', 'A', 'R')

// TagGvar is the table tag for the glyph variations table.
var TagGvar = MakeTag('g', 'v', 'a', 'r')

// TagSTAT is the table tag for the style attributes table.
var TagSTAT = MakeTag('S', 'T', 'A', 'T')

// TagMvar is the table tag for the metrics variations table.
var TagMvar = MakeTag('M', 'V', 'A', 'R')

// TagCvar is the table tag for the CVT variations table.
var TagCvar = MakeTag('c', 'v', 'a', 'r')

// Hvar represents a parsed HVAR (Horizontal Metrics Variations) table.
// It allows adjusting glyph advances based on variation axis settings.
type Hvar struct {
	data     []byte
	varStore *ItemVariationStore
	advMap   *DeltaSetIndexMap
}

// ParseHvar parses an HVAR table.
func ParseHvar(data []byte) (*Hvar, error) {
	if len(data) < 20 {
		return nil, ErrInvalidTable
	}

	// Check version (must be 1.0)
	major := binary.BigEndian.Uint16(data[0:])
	minor := binary.BigEndian.Uint16(data[2:])
	if major != 1 || minor != 0 {
		return nil, ErrInvalidFormat
	}

	varStoreOffset := binary.BigEndian.Uint32(data[4:])
	advMapOffset := binary.BigEndian.Uint32(data[8:])
	// lsbMapOffset := binary.BigEndian.Uint32(data[12:]) // optional, not used
	// rsbMapOffset := binary.BigEndian.Uint32(data[16:]) // optional, not used

	h := &Hvar{data: data}

	// Parse ItemVariationStore
	if varStoreOffset != 0 && int(varStoreOffset) < len(data) {
		vs, err := parseItemVariationStore(data[varStoreOffset:])
		if err != nil {
			return nil, err
		}
		h.varStore = vs
	}

	// Parse advance DeltaSetIndexMap
	if advMapOffset != 0 && int(advMapOffset) < len(data) {
		dm, err := parseDeltaSetIndexMap(data[advMapOffset:])
		if err != nil {
			return nil, err
		}
		h.advMap = dm
	}

	return h, nil
}

// HasData returns true if the HVAR table has valid data.
func (h *Hvar) HasData() bool {
	return h != nil && h.varStore != nil
}

// GetAdvanceDelta returns the advance delta for a glyph at the given
// normalized coordinates. The coordinates should be in F2DOT14 format
// (scaled by 16384, where 1.0 = 16384).
func (h *Hvar) GetAdvanceDelta(glyph GlyphID, normalizedCoords []int) float64 {
	if h == nil || h.varStore == nil {
		return 0
	}

	// Map glyph to variation index
	var varIdx uint32
	if h.advMap != nil {
		varIdx = h.advMap.Map(uint32(glyph))
	} else {
		// No advMap means identity mapping: use glyph ID as inner index
		varIdx = uint32(glyph)
	}

	return h.varStore.GetDelta(varIdx, normalizedCoords)
}

// ItemVariationStore holds variation data for different regions.
type ItemVariationStore struct {
	data       []byte
	regions    *VarRegionList
	dataSets   []varDataOffset
	regionData []byte
}

type varDataOffset struct {
	offset uint32
	data   []byte
}

// parseItemVariationStore parses an ItemVariationStore.
func parseItemVariationStore(data []byte) (*ItemVariationStore, error) {
	if len(data) < 8 {
		return nil, ErrInvalidTable
	}

	format := binary.BigEndian.Uint16(data[0:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	regionListOffset := binary.BigEndian.Uint32(data[2:])
	dataSetCount := binary.BigEndian.Uint16(data[6:])

	if len(data) < 8+int(dataSetCount)*4 {
		return nil, ErrInvalidOffset
	}

	store := &ItemVariationStore{data: data}

	// Parse region list
	if regionListOffset != 0 && int(regionListOffset) < len(data) {
		rl, err := parseVarRegionList(data[regionListOffset:])
		if err != nil {
			return nil, err
		}
		store.regions = rl
		store.regionData = data[regionListOffset:]
	}

	// Parse data set offsets
	store.dataSets = make([]varDataOffset, dataSetCount)
	for i := 0; i < int(dataSetCount); i++ {
		off := binary.BigEndian.Uint32(data[8+i*4:])
		store.dataSets[i].offset = off
		if off != 0 && int(off) < len(data) {
			store.dataSets[i].data = data[off:]
		}
	}

	return store, nil
}

// GetDelta returns the delta value for a variation index at the given
// normalized coordinates.
func (vs *ItemVariationStore) GetDelta(varIdx uint32, coords []int) float64 {
	if vs == nil || vs.regions == nil {
		return 0
	}

	outer := varIdx >> 16
	inner := varIdx & 0xFFFF

	if int(outer) >= len(vs.dataSets) {
		return 0
	}

	dataSet := vs.dataSets[outer]
	if dataSet.data == nil {
		return 0
	}

	return vs.getVarDataDelta(dataSet.data, int(inner), coords)
}

// getVarDataDelta extracts delta from a VarData subtable.
func (vs *ItemVariationStore) getVarDataDelta(varData []byte, inner int, coords []int) float64 {
	if len(varData) < 6 {
		return 0
	}

	itemCount := binary.BigEndian.Uint16(varData[0:])
	wordSizeCount := binary.BigEndian.Uint16(varData[2:])
	regionIndexCount := binary.BigEndian.Uint16(varData[4:])

	if inner >= int(itemCount) {
		return 0
	}

	longWords := (wordSizeCount & 0x8000) != 0
	wordCount := int(wordSizeCount & 0x7FFF)

	if len(varData) < 6+int(regionIndexCount)*2 {
		return 0
	}

	// Parse region indices
	regionIndices := make([]uint16, regionIndexCount)
	for i := 0; i < int(regionIndexCount); i++ {
		regionIndices[i] = binary.BigEndian.Uint16(varData[6+i*2:])
	}

	// Calculate row size and offset
	var rowSize int
	if longWords {
		rowSize = wordCount*4 + (int(regionIndexCount)-wordCount)*2
	} else {
		rowSize = wordCount*2 + (int(regionIndexCount) - wordCount)
	}

	deltaStart := 6 + int(regionIndexCount)*2
	rowOffset := deltaStart + inner*rowSize

	if rowOffset+rowSize > len(varData) {
		return 0
	}

	row := varData[rowOffset:]

	// Compute delta using float64 precision (matches HarfBuzz's double)
	var delta float64
	for i := 0; i < int(regionIndexCount); i++ {
		regionIdx := int(regionIndices[i])
		scalar := vs.regions.Evaluate(regionIdx, coords)
		if scalar == 0 {
			continue
		}

		var itemDelta int32
		if longWords {
			if i < wordCount {
				// 32-bit delta
				itemDelta = int32(binary.BigEndian.Uint32(row[i*4:]))
			} else {
				// 16-bit delta after 32-bit section
				offset := wordCount*4 + (i-wordCount)*2
				itemDelta = int32(int16(binary.BigEndian.Uint16(row[offset:])))
			}
		} else {
			if i < wordCount {
				// 16-bit delta
				itemDelta = int32(int16(binary.BigEndian.Uint16(row[i*2:])))
			} else {
				// 8-bit delta after 16-bit section
				offset := wordCount*2 + (i - wordCount)
				itemDelta = int32(int8(row[offset]))
			}
		}

		delta += scalar * float64(itemDelta)
	}

	return delta
}

// VarRegionList holds the list of variation regions.
type VarRegionList struct {
	data        []byte
	axisCount   int
	regionCount int
}

// parseVarRegionList parses a VarRegionList.
func parseVarRegionList(data []byte) (*VarRegionList, error) {
	if len(data) < 4 {
		return nil, ErrInvalidTable
	}

	axisCount := int(binary.BigEndian.Uint16(data[0:]))
	regionCount := int(binary.BigEndian.Uint16(data[2:]))

	// Each region has axisCount * 6 bytes (3 F2DOT14 values per axis)
	expectedSize := 4 + regionCount*axisCount*6
	if len(data) < expectedSize {
		return nil, ErrInvalidOffset
	}

	return &VarRegionList{
		data:        data,
		axisCount:   axisCount,
		regionCount: regionCount,
	}, nil
}

// Evaluate computes the scalar for a region at the given normalized coordinates.
// Coordinates are in F2DOT14 format (1.0 = 16384).
func (rl *VarRegionList) Evaluate(regionIndex int, coords []int) float64 {
	if rl == nil || regionIndex >= rl.regionCount {
		return 0
	}

	if len(coords) == 0 {
		return 0
	}

	// Each region is axisCount * 6 bytes
	regionOffset := 4 + regionIndex*rl.axisCount*6

	// HarfBuzz uses double precision for scalar calculations.
	// hb-ot-var-common.hh:157: "double scalar = 1.0;"
	var scalar float64 = 1.0
	for i := 0; i < rl.axisCount; i++ {
		axisOffset := regionOffset + i*6
		startCoord := int16(binary.BigEndian.Uint16(rl.data[axisOffset:]))
		peakCoord := int16(binary.BigEndian.Uint16(rl.data[axisOffset+2:]))
		endCoord := int16(binary.BigEndian.Uint16(rl.data[axisOffset+4:]))

		// Get coordinate value (or 0 if not specified)
		var coord int
		if i < len(coords) {
			coord = coords[i]
		}

		// Evaluate axis contribution
		factor := evaluateRegionAxis(int(startCoord), int(peakCoord), int(endCoord), coord)
		if factor == 0 {
			return 0
		}
		scalar *= factor
	}

	return scalar
}

// evaluateRegionAxis evaluates the contribution of a single axis to the region scalar.
// All values are in F2DOT14 format (1.0 = 16384).
// evaluateRegionAxis evaluates the contribution of a single axis to the region scalar.
// All values are in F2DOT14 format (1.0 = 16384).
// HarfBuzz uses double precision for these calculations.
// HarfBuzz equivalent: calculate_scalar() in hb-ot-var-common.hh:157-205
func evaluateRegionAxis(start, peak, end, coord int) float64 {
	if peak == 0 || coord == peak {
		return 1.0
	}
	if coord == 0 {
		return 0
	}

	// Sanity checks
	if start > peak || peak > end {
		return 1.0
	}
	if start < 0 && end > 0 && peak != 0 {
		return 1.0
	}

	if coord <= start || coord >= end {
		return 0
	}

	// Interpolate using float64 (matches HarfBuzz's double precision)
	if coord < peak {
		return float64(coord-start) / float64(peak-start)
	}
	return float64(end-coord) / float64(end-peak)
}

// DeltaSetIndexMap maps glyph IDs to variation indices.
type DeltaSetIndexMap struct {
	data          []byte
	format        uint8
	entryFormat   uint8
	mapCount      uint32
	innerBitCount int
	width         int
}

// parseDeltaSetIndexMap parses a DeltaSetIndexMap.
func parseDeltaSetIndexMap(data []byte) (*DeltaSetIndexMap, error) {
	if len(data) < 1 {
		return nil, ErrInvalidTable
	}

	format := data[0]

	var entryFormat uint8
	var mapCount uint32
	var headerSize int

	switch format {
	case 0:
		// Format 0: 16-bit map count
		if len(data) < 4 {
			return nil, ErrInvalidTable
		}
		entryFormat = data[1]
		mapCount = uint32(binary.BigEndian.Uint16(data[2:]))
		headerSize = 4
	case 1:
		// Format 1: 32-bit map count
		if len(data) < 6 {
			return nil, ErrInvalidTable
		}
		entryFormat = data[1]
		mapCount = binary.BigEndian.Uint32(data[2:])
		headerSize = 6
	default:
		return nil, ErrInvalidFormat
	}

	innerBitCount := int((entryFormat & 0x0F) + 1)
	width := int(((entryFormat >> 4) & 0x03) + 1)

	expectedSize := headerSize + int(mapCount)*width
	if len(data) < expectedSize {
		return nil, ErrInvalidOffset
	}

	return &DeltaSetIndexMap{
		data:          data,
		format:        format,
		entryFormat:   entryFormat,
		mapCount:      mapCount,
		innerBitCount: innerBitCount,
		width:         width,
	}, nil
}

// Map returns the variation index for a glyph.
// Returns 16.16 format: (outer << 16) | inner
func (dm *DeltaSetIndexMap) Map(glyph uint32) uint32 {
	if dm == nil {
		return glyph // Identity mapping
	}

	// If mapCount is 0, pass through unchanged
	if dm.mapCount == 0 {
		return glyph
	}

	// Clamp to last entry
	idx := glyph
	if idx >= dm.mapCount {
		idx = dm.mapCount - 1
	}

	// Calculate header size
	var headerSize int
	if dm.format == 0 {
		headerSize = 4
	} else {
		headerSize = 6
	}

	// Read the entry
	entryOffset := headerSize + int(idx)*dm.width
	var u uint32
	switch dm.width {
	case 1:
		u = uint32(dm.data[entryOffset])
	case 2:
		u = uint32(binary.BigEndian.Uint16(dm.data[entryOffset:]))
	case 3:
		u = uint32(dm.data[entryOffset])<<16 |
			uint32(dm.data[entryOffset+1])<<8 |
			uint32(dm.data[entryOffset+2])
	case 4:
		u = binary.BigEndian.Uint32(dm.data[entryOffset:])
	default:
		return glyph
	}

	// Extract outer and inner from packed format
	outer := u >> dm.innerBitCount
	inner := u & ((1 << dm.innerBitCount) - 1)

	return (outer << 16) | inner
}
