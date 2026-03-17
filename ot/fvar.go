package ot

import (
	"encoding/binary"
	"math"
)

// Variable font axis tags (registered axes)
var (
	TagAxisWeight      = MakeTag('w', 'g', 'h', 't') // Weight axis
	TagAxisWidth       = MakeTag('w', 'd', 't', 'h') // Width axis
	TagAxisSlant       = MakeTag('s', 'l', 'n', 't') // Slant axis
	TagAxisItalic      = MakeTag('i', 't', 'a', 'l') // Italic axis
	TagAxisOpticalSize = MakeTag('o', 'p', 's', 'z') // Optical size axis
)

// TagFvar is the table tag for the font variations table.
var TagFvar = MakeTag('f', 'v', 'a', 'r')

// AxisFlags for variation axes.
type AxisFlags uint16

const (
	// AxisFlagHidden indicates the axis should not be exposed in user interfaces.
	AxisFlagHidden AxisFlags = 0x0001
)

// Variation represents a single axis value setting.
type Variation struct {
	Tag   Tag
	Value float32
}

// AxisInfo describes a variation axis.
type AxisInfo struct {
	Index        int
	Tag          Tag
	NameID       uint16
	Flags        AxisFlags
	MinValue     float32
	DefaultValue float32
	MaxValue     float32
}

// NamedInstance represents a predefined style like "Bold" or "Light".
type NamedInstance struct {
	Coords           []float32
	Index            int
	SubfamilyNameID  uint16
	PostScriptNameID uint16 // 0 if not present
}

// Fvar represents a parsed fvar (Font Variations) table.
type Fvar struct {
	data          []byte
	axisCount     int
	instanceCount int
	axisOffset    int
	instanceSize  int
}

// ParseFvar parses an fvar table.
func ParseFvar(data []byte) (*Fvar, error) {
	if len(data) < 16 {
		return nil, ErrInvalidTable
	}

	// Check version (must be 1.0)
	major := binary.BigEndian.Uint16(data[0:])
	minor := binary.BigEndian.Uint16(data[2:])
	if major != 1 || minor != 0 {
		return nil, ErrInvalidFormat
	}

	axisOffset := int(binary.BigEndian.Uint16(data[4:]))
	// reserved := binary.BigEndian.Uint16(data[6:]) // should be 2
	axisCount := int(binary.BigEndian.Uint16(data[8:]))
	axisSize := int(binary.BigEndian.Uint16(data[10:]))
	instanceCount := int(binary.BigEndian.Uint16(data[12:]))
	instanceSize := int(binary.BigEndian.Uint16(data[14:]))

	// Validate axisSize (must be 20)
	if axisSize != 20 {
		return nil, ErrInvalidFormat
	}

	// Validate instanceSize
	minInstanceSize := axisCount*4 + 4
	if instanceSize < minInstanceSize {
		return nil, ErrInvalidFormat
	}

	// Validate data length
	axesEnd := axisOffset + axisCount*20
	instancesEnd := axesEnd + instanceCount*instanceSize
	if instancesEnd > len(data) {
		return nil, ErrInvalidOffset
	}

	return &Fvar{
		data:          data,
		axisCount:     axisCount,
		instanceCount: instanceCount,
		axisOffset:    axisOffset,
		instanceSize:  instanceSize,
	}, nil
}

// HasData returns true if the fvar table has variation data.
func (f *Fvar) HasData() bool {
	return f != nil && f.axisCount > 0
}

// AxisCount returns the number of variation axes.
func (f *Fvar) AxisCount() int {
	if f == nil {
		return 0
	}
	return f.axisCount
}

// AxisInfos returns information about all variation axes.
func (f *Fvar) AxisInfos() []AxisInfo {
	if f == nil || f.axisCount == 0 {
		return nil
	}

	axes := make([]AxisInfo, f.axisCount)
	for i := 0; i < f.axisCount; i++ {
		axes[i] = f.axisInfoAt(i)
	}
	return axes
}

// FindAxis finds an axis by its tag.
func (f *Fvar) FindAxis(tag Tag) (AxisInfo, bool) {
	if f == nil {
		return AxisInfo{}, false
	}

	for i := 0; i < f.axisCount; i++ {
		info := f.axisInfoAt(i)
		if info.Tag == tag {
			return info, true
		}
	}
	return AxisInfo{}, false
}

// axisInfoAt returns the axis info at the given index.
func (f *Fvar) axisInfoAt(index int) AxisInfo {
	off := f.axisOffset + index*20
	return AxisInfo{
		Index:        index,
		Tag:          Tag(binary.BigEndian.Uint32(f.data[off:])),
		MinValue:     fixed1616ToFloat(binary.BigEndian.Uint32(f.data[off+4:])),
		DefaultValue: fixed1616ToFloat(binary.BigEndian.Uint32(f.data[off+8:])),
		MaxValue:     fixed1616ToFloat(binary.BigEndian.Uint32(f.data[off+12:])),
		Flags:        AxisFlags(binary.BigEndian.Uint16(f.data[off+16:])),
		NameID:       binary.BigEndian.Uint16(f.data[off+18:]),
	}
}

// InstanceCount returns the number of named instances.
func (f *Fvar) InstanceCount() int {
	if f == nil {
		return 0
	}
	return f.instanceCount
}

// NamedInstances returns all named instances.
func (f *Fvar) NamedInstances() []NamedInstance {
	if f == nil || f.instanceCount == 0 {
		return nil
	}

	instances := make([]NamedInstance, f.instanceCount)
	for i := 0; i < f.instanceCount; i++ {
		instances[i] = f.namedInstanceAt(i)
	}
	return instances
}

// NamedInstanceAt returns the named instance at the given index.
func (f *Fvar) NamedInstanceAt(index int) (NamedInstance, bool) {
	if f == nil || index < 0 || index >= f.instanceCount {
		return NamedInstance{}, false
	}
	return f.namedInstanceAt(index), true
}

// namedInstanceAt returns the named instance at the given index.
func (f *Fvar) namedInstanceAt(index int) NamedInstance {
	instancesStart := f.axisOffset + f.axisCount*20
	off := instancesStart + index*f.instanceSize

	inst := NamedInstance{
		Index:           index,
		SubfamilyNameID: binary.BigEndian.Uint16(f.data[off:]),
		// flags at off+2 (reserved, ignored)
		Coords: make([]float32, f.axisCount),
	}

	// Read coordinates
	coordOff := off + 4
	for i := 0; i < f.axisCount; i++ {
		inst.Coords[i] = fixed1616ToFloat(binary.BigEndian.Uint32(f.data[coordOff+i*4:]))
	}

	// Check for optional postscript name ID
	if f.instanceSize >= f.axisCount*4+6 {
		inst.PostScriptNameID = binary.BigEndian.Uint16(f.data[off+4+f.axisCount*4:])
	}

	return inst
}

// NormalizeAxisValue normalizes a user-space axis value to the range [-1, 1].
// Values below default normalize to [-1, 0], values above to [0, 1].
func (f *Fvar) NormalizeAxisValue(axisIndex int, value float32) float32 {
	if f == nil || axisIndex < 0 || axisIndex >= f.axisCount {
		return 0
	}

	info := f.axisInfoAt(axisIndex)

	// Clamp to axis range
	value = clampFloat32(value, info.MinValue, info.MaxValue)

	if value == info.DefaultValue {
		return 0
	} else if value < info.DefaultValue {
		return (value - info.DefaultValue) / (info.DefaultValue - info.MinValue)
	} else {
		return (value - info.DefaultValue) / (info.MaxValue - info.DefaultValue)
	}
}

// NormalizeVariations converts user-space variations to normalized coordinates.
func (f *Fvar) NormalizeVariations(variations []Variation) []float32 {
	if f == nil || f.axisCount == 0 {
		return nil
	}

	coords := make([]float32, f.axisCount)

	// Set defaults (0 = default position)
	// Then apply any specified variations

	for _, v := range variations {
		for i := 0; i < f.axisCount; i++ {
			info := f.axisInfoAt(i)
			if info.Tag == v.Tag {
				coords[i] = f.NormalizeAxisValue(i, v.Value)
				break
			}
		}
	}

	return coords
}

// fixed1616ToFloat converts a 16.16 fixed-point number to float32.
func fixed1616ToFloat(v uint32) float32 {
	return float32(int32(v)) / 65536.0
}

// floatToFixed1616 converts a float32 to a 16.16 fixed-point number.
func floatToFixed1616(v float32) uint32 {
	return uint32(int32(math.Round(float64(v) * 65536.0)))
}

// clampFloat32 clamps a value to the range [min, max].
func clampFloat32(v, min, max float32) float32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// floatToF2DOT14 converts a float32 in range [-1, 1] to F2DOT14 format.
// F2DOT14 is a fixed-point format where 1.0 = 16384.
func floatToF2DOT14(v float32) int {
	return int(math.Round(float64(v) * 16384.0))
}
