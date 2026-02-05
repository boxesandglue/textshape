package ot

import "encoding/binary"

// Vhea represents the vertical header table.
type Vhea struct {
	Version              uint32
	Ascender             int16  // Vertical typographic ascender
	Descender            int16  // Vertical typographic descender
	LineGap              int16  // Vertical typographic line gap
	AdvanceHeightMax     uint16 // Maximum advance height
	MinTopSideBearing    int16
	MinBottomSideBearing int16
	YMaxExtent           int16
	CaretSlopeRise       int16
	CaretSlopeRun        int16
	CaretOffset          int16
	MetricDataFormat     int16
	NumberOfVMetrics     uint16
}

// ParseVhea parses the vhea (vertical header) table.
func ParseVhea(data []byte) (*Vhea, error) {
	if len(data) < 36 {
		return nil, ErrInvalidTable
	}

	v := &Vhea{
		Version:              binary.BigEndian.Uint32(data[0:]),
		Ascender:             int16(binary.BigEndian.Uint16(data[4:])),
		Descender:            int16(binary.BigEndian.Uint16(data[6:])),
		LineGap:              int16(binary.BigEndian.Uint16(data[8:])),
		AdvanceHeightMax:     binary.BigEndian.Uint16(data[10:]),
		MinTopSideBearing:    int16(binary.BigEndian.Uint16(data[12:])),
		MinBottomSideBearing: int16(binary.BigEndian.Uint16(data[14:])),
		YMaxExtent:           int16(binary.BigEndian.Uint16(data[16:])),
		CaretSlopeRise:       int16(binary.BigEndian.Uint16(data[18:])),
		CaretSlopeRun:        int16(binary.BigEndian.Uint16(data[20:])),
		CaretOffset:          int16(binary.BigEndian.Uint16(data[22:])),
		// 24-30: reserved (4 int16)
		MetricDataFormat: int16(binary.BigEndian.Uint16(data[32:])),
		NumberOfVMetrics: binary.BigEndian.Uint16(data[34:]),
	}

	return v, nil
}

// Vmtx represents the vertical metrics table.
type Vmtx struct {
	vMetrics          []LongVerMetric
	topSideBearings   []int16
	lastAdvanceHeight uint16
}

// LongVerMetric contains the advance height and top side bearing for a glyph.
type LongVerMetric struct {
	AdvanceHeight uint16
	Tsb           int16 // Top side bearing
}

// ParseVmtx parses the vmtx table.
// It requires numberOfVMetrics from vhea and numGlyphs from maxp.
func ParseVmtx(data []byte, numberOfVMetrics, numGlyphs int) (*Vmtx, error) {
	if numberOfVMetrics <= 0 {
		return nil, ErrInvalidTable
	}

	expectedSize := numberOfVMetrics*4 + (numGlyphs-numberOfVMetrics)*2
	if len(data) < expectedSize {
		return nil, ErrInvalidTable
	}

	v := &Vmtx{
		vMetrics:        make([]LongVerMetric, numberOfVMetrics),
		topSideBearings: make([]int16, numGlyphs-numberOfVMetrics),
	}

	off := 0
	for i := 0; i < numberOfVMetrics; i++ {
		v.vMetrics[i].AdvanceHeight = binary.BigEndian.Uint16(data[off:])
		v.vMetrics[i].Tsb = int16(binary.BigEndian.Uint16(data[off+2:]))
		off += 4
	}

	if numberOfVMetrics > 0 {
		v.lastAdvanceHeight = v.vMetrics[numberOfVMetrics-1].AdvanceHeight
	}

	for i := 0; i < numGlyphs-numberOfVMetrics; i++ {
		v.topSideBearings[i] = int16(binary.BigEndian.Uint16(data[off:]))
		off += 2
	}

	return v, nil
}

// GetAdvanceHeight returns the advance height for a glyph.
func (v *Vmtx) GetAdvanceHeight(glyph GlyphID) uint16 {
	if int(glyph) < len(v.vMetrics) {
		return v.vMetrics[glyph].AdvanceHeight
	}
	return v.lastAdvanceHeight
}

// GetTsb returns the top side bearing for a glyph.
func (v *Vmtx) GetTsb(glyph GlyphID) int16 {
	if int(glyph) < len(v.vMetrics) {
		return v.vMetrics[glyph].Tsb
	}
	idx := int(glyph) - len(v.vMetrics)
	if idx >= 0 && idx < len(v.topSideBearings) {
		return v.topSideBearings[idx]
	}
	return 0
}
