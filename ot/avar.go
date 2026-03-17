package ot

import (
	"encoding/binary"
)

// TagAvar is the table tag for the axis variations table.
var TagAvar = MakeTag('a', 'v', 'a', 'r')

// Avar represents a parsed avar (Axis Variations) table.
// It provides non-linear mapping for normalized axis values.
type Avar struct {
	data      []byte
	axisMaps  []axisValueMap
	axisCount int
}

// axisValueMap holds the segment map for one axis.
type axisValueMap struct {
	segments []avarSegment
}

// avarSegment is a single segment in the axis value map.
type avarSegment struct {
	fromCoord int16 // F2DOT14
	toCoord   int16 // F2DOT14
}

// ParseAvar parses an avar table.
func ParseAvar(data []byte) (*Avar, error) {
	if len(data) < 8 {
		return nil, ErrInvalidTable
	}

	// Check version
	major := binary.BigEndian.Uint16(data[0:])
	minor := binary.BigEndian.Uint16(data[2:])
	if major != 1 || minor != 0 {
		return nil, ErrInvalidFormat
	}

	// reserved := binary.BigEndian.Uint16(data[4:])
	axisCount := int(binary.BigEndian.Uint16(data[6:]))

	a := &Avar{
		data:      data,
		axisCount: axisCount,
		axisMaps:  make([]axisValueMap, axisCount),
	}

	// Parse axis segment maps
	offset := 8
	for i := 0; i < axisCount; i++ {
		if offset+2 > len(data) {
			return nil, ErrInvalidOffset
		}

		positionMapCount := int(binary.BigEndian.Uint16(data[offset:]))
		offset += 2

		if offset+positionMapCount*4 > len(data) {
			return nil, ErrInvalidOffset
		}

		a.axisMaps[i].segments = make([]avarSegment, positionMapCount)
		for j := 0; j < positionMapCount; j++ {
			a.axisMaps[i].segments[j] = avarSegment{
				fromCoord: int16(binary.BigEndian.Uint16(data[offset:])),
				toCoord:   int16(binary.BigEndian.Uint16(data[offset+2:])),
			}
			offset += 4
		}
	}

	return a, nil
}

// HasData returns true if the avar table has valid data.
func (a *Avar) HasData() bool {
	return a != nil && a.axisCount > 0
}

// MapValue maps a normalized coordinate through the avar segment map.
// Both input and output are in F2DOT14 format.
func (a *Avar) MapValue(axisIndex int, value int) int {
	if a == nil || axisIndex < 0 || axisIndex >= a.axisCount {
		return value
	}

	segments := a.axisMaps[axisIndex].segments
	if len(segments) == 0 {
		return value
	}

	// Find the segment containing this value
	v := int16(value)

	// Before first segment
	if v <= segments[0].fromCoord {
		return int(segments[0].toCoord)
	}

	// After last segment
	if v >= segments[len(segments)-1].fromCoord {
		return int(segments[len(segments)-1].toCoord)
	}

	// Find interpolation segment
	for i := 1; i < len(segments); i++ {
		if v < segments[i].fromCoord {
			// Interpolate between segments[i-1] and segments[i]
			from1 := int(segments[i-1].fromCoord)
			from2 := int(segments[i].fromCoord)
			to1 := int(segments[i-1].toCoord)
			to2 := int(segments[i].toCoord)

			// Linear interpolation
			// result = to1 + (v - from1) * (to2 - to1) / (from2 - from1)
			if from2 == from1 {
				return to1
			}

			result := to1 + (int(v)-from1)*(to2-to1)/(from2-from1)
			return result
		}
	}

	return value
}

// MapCoords maps an array of normalized coordinates through avar.
// Input and output are in F2DOT14 format.
func (a *Avar) MapCoords(coords []int) []int {
	if a == nil || len(coords) == 0 {
		return coords
	}

	result := make([]int, len(coords))
	for i, v := range coords {
		if i < a.axisCount {
			result[i] = a.MapValue(i, v)
		} else {
			result[i] = v
		}
	}
	return result
}
