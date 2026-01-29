package ot

import (
	"encoding/binary"
)

// Glyf represents the parsed glyf table (glyph data).
type Glyf struct {
	data []byte
	loca *Loca
}

// Loca represents the parsed loca table (index to location).
type Loca struct {
	offsets   []uint32 // Glyph offsets into glyf table
	numGlyphs int
	isShort   bool // true for short format (16-bit offsets)
}

// GlyphData represents the raw data for a single glyph.
type GlyphData struct {
	Data             []byte
	NumberOfContours int16 // -1 for composite, >= 0 for simple
}

// ParseLoca parses the loca table.
// indexToLocFormat: 0 = short (16-bit), 1 = long (32-bit)
func ParseLoca(data []byte, numGlyphs int, indexToLocFormat int16) (*Loca, error) {
	l := &Loca{
		numGlyphs: numGlyphs,
		isShort:   indexToLocFormat == 0,
	}

	// loca has numGlyphs+1 entries
	numEntries := numGlyphs + 1

	if l.isShort {
		// Short format: 16-bit offsets (actual offset = value * 2)
		if len(data) < numEntries*2 {
			return nil, ErrInvalidOffset
		}
		l.offsets = make([]uint32, numEntries)
		for i := 0; i < numEntries; i++ {
			l.offsets[i] = uint32(binary.BigEndian.Uint16(data[i*2:])) * 2
		}
	} else {
		// Long format: 32-bit offsets
		if len(data) < numEntries*4 {
			return nil, ErrInvalidOffset
		}
		l.offsets = make([]uint32, numEntries)
		for i := 0; i < numEntries; i++ {
			l.offsets[i] = binary.BigEndian.Uint32(data[i*4:])
		}
	}

	return l, nil
}

// GetOffset returns the offset and length for a glyph.
// Returns (offset, length, ok)
func (l *Loca) GetOffset(gid GlyphID) (uint32, uint32, bool) {
	idx := int(gid)
	if idx < 0 || idx >= l.numGlyphs {
		return 0, 0, false
	}
	start := l.offsets[idx]
	end := l.offsets[idx+1]
	return start, end - start, true
}

// NumGlyphs returns the number of glyphs.
func (l *Loca) NumGlyphs() int {
	return l.numGlyphs
}

// IsShort returns true if using short (16-bit) format.
func (l *Loca) IsShort() bool {
	return l.isShort
}

// ParseGlyf parses the glyf table using a loca table.
func ParseGlyf(data []byte, loca *Loca) (*Glyf, error) {
	return &Glyf{
		data: data,
		loca: loca,
	}, nil
}

// GetGlyph returns the glyph data for a glyph ID.
func (g *Glyf) GetGlyph(gid GlyphID) *GlyphData {
	offset, length, ok := g.loca.GetOffset(gid)
	if !ok {
		return nil
	}

	// Empty glyph (like space)
	if length == 0 {
		return &GlyphData{
			Data:             nil,
			NumberOfContours: 0,
		}
	}

	if int(offset)+int(length) > len(g.data) {
		return nil
	}

	data := g.data[offset : offset+length]
	if len(data) < 2 {
		return nil
	}

	numberOfContours := int16(binary.BigEndian.Uint16(data))

	return &GlyphData{
		Data:             data,
		NumberOfContours: numberOfContours,
	}
}

// GetGlyphBytes returns the raw bytes for a glyph.
func (g *Glyf) GetGlyphBytes(gid GlyphID) []byte {
	offset, length, ok := g.loca.GetOffset(gid)
	if !ok || length == 0 {
		return nil
	}
	if int(offset)+int(length) > len(g.data) {
		return nil
	}
	return g.data[offset : offset+length]
}

// GetGlyphExtents returns the extents (bounding box) for a glyph.
// Returns false if the glyph has no outline (e.g., space) or doesn't exist.
func (g *Glyf) GetGlyphExtents(gid GlyphID) (GlyphExtents, bool) {
	offset, length, ok := g.loca.GetOffset(gid)
	if !ok {
		return GlyphExtents{}, false
	}

	// Empty glyph (like space) - no extents
	if length == 0 {
		return GlyphExtents{}, false
	}

	// Glyph header: numberOfContours (2) + xMin (2) + yMin (2) + xMax (2) + yMax (2) = 10 bytes
	if int(offset)+10 > len(g.data) {
		return GlyphExtents{}, false
	}

	data := g.data[offset:]
	// Skip numberOfContours (bytes 0-1)
	xMin := int16(binary.BigEndian.Uint16(data[2:]))
	yMin := int16(binary.BigEndian.Uint16(data[4:]))
	xMax := int16(binary.BigEndian.Uint16(data[6:]))
	yMax := int16(binary.BigEndian.Uint16(data[8:]))

	return GlyphExtents{
		XBearing: xMin,
		YBearing: yMax,                  // Top of glyph
		Width:    xMax - xMin,
		Height:   -(yMax - yMin),        // Negative height (top to bottom)
	}, true
}

// IsComposite returns true if the glyph is a composite glyph.
func (gd *GlyphData) IsComposite() bool {
	return gd.NumberOfContours < 0
}

// Composite glyph flags
const (
	argAreWords     uint16 = 0x0001 // Args are words (otherwise bytes)
	argsAreXYValues uint16 = 0x0002 // Args are xy values (otherwise points)
	roundXYToGrid   uint16 = 0x0004
	weHaveAScale    uint16 = 0x0008 // Scale value present
	moreComponents  uint16 = 0x0020 // More components follow
	weHaveXYScale   uint16 = 0x0040 // Separate X and Y scale
	weHave2x2       uint16 = 0x0080 // 2x2 transform matrix
	weHaveInstr     uint16 = 0x0100 // Instructions follow
	useMyMetrics    uint16 = 0x0200
	overlapCompound uint16 = 0x0400
)

// CompositeComponent represents a component in a composite glyph.
type CompositeComponent struct {
	GlyphID GlyphID
	Flags   uint16
	Arg1    int16
	Arg2    int16
	// Transform matrix components (optional)
	Scale   float32
	ScaleX  float32
	ScaleY  float32
	Scale01 float32
	Scale10 float32
}

// GetComponents returns the component glyph IDs for a composite glyph.
// For simple glyphs, returns nil.
func (g *Glyf) GetComponents(gid GlyphID) []GlyphID {
	glyph := g.GetGlyph(gid)
	if glyph == nil || !glyph.IsComposite() {
		return nil
	}

	components := g.parseComposite(glyph.Data)
	result := make([]GlyphID, len(components))
	for i, comp := range components {
		result[i] = comp.GlyphID
	}
	return result
}

// parseComposite parses composite glyph components.
func (g *Glyf) parseComposite(data []byte) []CompositeComponent {
	if len(data) < 10 {
		return nil
	}

	// Skip glyph header (10 bytes: numberOfContours, xMin, yMin, xMax, yMax)
	offset := 10
	var components []CompositeComponent

	for {
		if offset+4 > len(data) {
			break
		}

		flags := binary.BigEndian.Uint16(data[offset:])
		glyphIndex := GlyphID(binary.BigEndian.Uint16(data[offset+2:]))
		offset += 4

		comp := CompositeComponent{
			GlyphID: glyphIndex,
			Flags:   flags,
		}

		// Parse arguments
		if flags&argAreWords != 0 {
			if offset+4 > len(data) {
				break
			}
			comp.Arg1 = int16(binary.BigEndian.Uint16(data[offset:]))
			comp.Arg2 = int16(binary.BigEndian.Uint16(data[offset+2:]))
			offset += 4
		} else {
			if offset+2 > len(data) {
				break
			}
			comp.Arg1 = int16(int8(data[offset]))
			comp.Arg2 = int16(int8(data[offset+1]))
			offset += 2
		}

		// Skip transform components (we just need glyph IDs for closure)
		if flags&weHaveAScale != 0 {
			offset += 2 // F2Dot14
		} else if flags&weHaveXYScale != 0 {
			offset += 4 // 2 x F2Dot14
		} else if flags&weHave2x2 != 0 {
			offset += 8 // 4 x F2Dot14
		}

		components = append(components, comp)

		if flags&moreComponents == 0 {
			break
		}
	}

	return components
}

// RemapComposite creates a new composite glyph with remapped component IDs.
func RemapComposite(data []byte, glyphMap map[GlyphID]GlyphID) []byte {
	if len(data) < 10 {
		return data
	}

	// Check if this is a composite
	numberOfContours := int16(binary.BigEndian.Uint16(data))
	if numberOfContours >= 0 {
		// Simple glyph, no remapping needed
		return data
	}

	// Make a copy to modify
	result := make([]byte, len(data))
	copy(result, data)

	// Parse and remap component glyph IDs
	offset := 10
	for {
		if offset+4 > len(result) {
			break
		}

		flags := binary.BigEndian.Uint16(result[offset:])
		oldGID := GlyphID(binary.BigEndian.Uint16(result[offset+2:]))

		// Remap the glyph ID
		if newGID, ok := glyphMap[oldGID]; ok {
			binary.BigEndian.PutUint16(result[offset+2:], uint16(newGID))
		}

		offset += 4

		// Skip arguments
		if flags&argAreWords != 0 {
			offset += 4
		} else {
			offset += 2
		}

		// Skip transform components
		if flags&weHaveAScale != 0 {
			offset += 2
		} else if flags&weHaveXYScale != 0 {
			offset += 4
		} else if flags&weHave2x2 != 0 {
			offset += 8
		}

		if flags&moreComponents == 0 {
			break
		}
	}

	return result
}

// BuildLoca builds a loca table from glyph offsets.
// If useShort is true, uses 16-bit format (offsets must be even and < 131072).
func BuildLoca(offsets []uint32, useShort bool) []byte {
	if useShort {
		data := make([]byte, len(offsets)*2)
		for i, off := range offsets {
			binary.BigEndian.PutUint16(data[i*2:], uint16(off/2))
		}
		return data
	}

	data := make([]byte, len(offsets)*4)
	for i, off := range offsets {
		binary.BigEndian.PutUint32(data[i*4:], off)
	}
	return data
}

// SimpleGlyphPoint represents a point in a simple glyph outline.
type SimpleGlyphPoint struct {
	X       int16
	Y       int16
	OnCurve bool
}

// ParseSimpleGlyph parses a simple glyph and returns its points.
// This includes phantom points (4 points at the end for metrics).
func ParseSimpleGlyph(data []byte) ([]SimpleGlyphPoint, int, error) {
	if len(data) < 10 {
		return nil, 0, ErrInvalidTable
	}

	numberOfContours := int16(binary.BigEndian.Uint16(data[0:]))
	if numberOfContours < 0 {
		// Composite glyph - not handled here
		return nil, 0, ErrInvalidFormat
	}

	if numberOfContours == 0 {
		// Empty glyph (like space)
		return nil, 0, nil
	}

	offset := 10 // Skip header

	// Read endPtsOfContours
	if offset+int(numberOfContours)*2 > len(data) {
		return nil, 0, ErrInvalidOffset
	}

	var numPoints int
	for i := 0; i < int(numberOfContours); i++ {
		endPt := int(binary.BigEndian.Uint16(data[offset+i*2:]))
		if endPt+1 > numPoints {
			numPoints = endPt + 1
		}
	}
	offset += int(numberOfContours) * 2

	// Read instructionLength
	if offset+2 > len(data) {
		return nil, 0, ErrInvalidOffset
	}
	instructionLength := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2

	// Skip instructions
	offset += instructionLength
	if offset > len(data) {
		return nil, 0, ErrInvalidOffset
	}

	// Parse flags
	flags := make([]byte, numPoints)
	for i := 0; i < numPoints; {
		if offset >= len(data) {
			return nil, 0, ErrInvalidOffset
		}
		flag := data[offset]
		offset++
		flags[i] = flag
		i++

		// Check for repeat flag
		if flag&0x08 != 0 {
			if offset >= len(data) {
				return nil, 0, ErrInvalidOffset
			}
			repeatCount := int(data[offset])
			offset++
			for j := 0; j < repeatCount && i < numPoints; j++ {
				flags[i] = flag
				i++
			}
		}
	}

	// Parse X coordinates
	points := make([]SimpleGlyphPoint, numPoints)
	var x int16 = 0
	for i := 0; i < numPoints; i++ {
		flag := flags[i]
		xShort := (flag & 0x02) != 0
		xSame := (flag & 0x10) != 0

		if xShort {
			if offset >= len(data) {
				return nil, 0, ErrInvalidOffset
			}
			if xSame {
				x += int16(data[offset])
			} else {
				x -= int16(data[offset])
			}
			offset++
		} else {
			if !xSame {
				if offset+2 > len(data) {
					return nil, 0, ErrInvalidOffset
				}
				x += int16(binary.BigEndian.Uint16(data[offset:]))
				offset += 2
			}
			// else: x is unchanged (same as previous)
		}
		points[i].X = x
		points[i].OnCurve = (flag & 0x01) != 0
	}

	// Parse Y coordinates
	var y int16 = 0
	for i := 0; i < numPoints; i++ {
		flag := flags[i]
		yShort := (flag & 0x04) != 0
		ySame := (flag & 0x20) != 0

		if yShort {
			if offset >= len(data) {
				return nil, 0, ErrInvalidOffset
			}
			if ySame {
				y += int16(data[offset])
			} else {
				y -= int16(data[offset])
			}
			offset++
		} else {
			if !ySame {
				if offset+2 > len(data) {
					return nil, 0, ErrInvalidOffset
				}
				y += int16(binary.BigEndian.Uint16(data[offset:]))
				offset += 2
			}
		}
		points[i].Y = y
	}

	return points, int(numberOfContours), nil
}

// InstanceSimpleGlyph creates a new glyph with deltas applied to points.
func InstanceSimpleGlyph(data []byte, xDeltas, yDeltas []int16) []byte {
	if len(data) < 10 {
		return data
	}

	numberOfContours := int16(binary.BigEndian.Uint16(data[0:]))
	if numberOfContours <= 0 {
		// Composite or empty glyph - return unchanged
		return data
	}

	// Parse the original glyph
	points, numContours, err := ParseSimpleGlyph(data)
	if err != nil || len(points) == 0 {
		return data
	}

	// Apply deltas to points
	for i := 0; i < len(points) && i < len(xDeltas); i++ {
		points[i].X += xDeltas[i]
		points[i].Y += yDeltas[i]
	}

	// Re-encode the glyph
	return encodeSimpleGlyph(data, points, numContours)
}

// encodeSimpleGlyph re-encodes a simple glyph with modified points.
func encodeSimpleGlyph(originalData []byte, points []SimpleGlyphPoint, numContours int) []byte {
	if len(originalData) < 10 {
		return originalData
	}

	// Calculate new bounding box
	var xMin, yMin, xMax, yMax int16
	if len(points) > 0 {
		xMin, yMin = points[0].X, points[0].Y
		xMax, yMax = points[0].X, points[0].Y
		for _, p := range points[1:] {
			if p.X < xMin {
				xMin = p.X
			}
			if p.X > xMax {
				xMax = p.X
			}
			if p.Y < yMin {
				yMin = p.Y
			}
			if p.Y > yMax {
				yMax = p.Y
			}
		}
	}

	// Read endPtsOfContours from original
	endPtsOfContours := make([]uint16, numContours)
	for i := 0; i < numContours; i++ {
		endPtsOfContours[i] = binary.BigEndian.Uint16(originalData[10+i*2:])
	}

	// Read instructions from original
	instrOffset := 10 + numContours*2
	if instrOffset+2 > len(originalData) {
		return originalData
	}
	instructionLength := int(binary.BigEndian.Uint16(originalData[instrOffset:]))
	var instructions []byte
	if instructionLength > 0 && instrOffset+2+instructionLength <= len(originalData) {
		instructions = originalData[instrOffset+2 : instrOffset+2+instructionLength]
	}

	// Build new glyph data
	var result []byte

	// Header
	header := make([]byte, 10)
	binary.BigEndian.PutUint16(header[0:], uint16(numContours))
	binary.BigEndian.PutUint16(header[2:], uint16(xMin))
	binary.BigEndian.PutUint16(header[4:], uint16(yMin))
	binary.BigEndian.PutUint16(header[6:], uint16(xMax))
	binary.BigEndian.PutUint16(header[8:], uint16(yMax))
	result = append(result, header...)

	// endPtsOfContours
	for _, endPt := range endPtsOfContours {
		result = append(result, byte(endPt>>8), byte(endPt))
	}

	// Instructions
	result = append(result, byte(instructionLength>>8), byte(instructionLength))
	result = append(result, instructions...)

	// Encode flags and coordinates
	// Use simple encoding (no repeat flags, may be larger but correct)
	flags := make([]byte, len(points))
	var xCoords, yCoords []byte

	var lastX, lastY int16 = 0, 0
	for i, p := range points {
		var flag byte
		if p.OnCurve {
			flag |= 0x01
		}

		dx := p.X - lastX
		dy := p.Y - lastY

		// Encode X
		if dx == 0 {
			flag |= 0x10 // xIsSame
		} else if dx >= -255 && dx <= 255 {
			flag |= 0x02 // xShort
			if dx > 0 {
				flag |= 0x10 // positive
				xCoords = append(xCoords, byte(dx))
			} else {
				xCoords = append(xCoords, byte(-dx))
			}
		} else {
			// xLong
			xCoords = append(xCoords, byte(dx>>8), byte(dx))
		}

		// Encode Y
		if dy == 0 {
			flag |= 0x20 // yIsSame
		} else if dy >= -255 && dy <= 255 {
			flag |= 0x04 // yShort
			if dy > 0 {
				flag |= 0x20 // positive
				yCoords = append(yCoords, byte(dy))
			} else {
				yCoords = append(yCoords, byte(-dy))
			}
		} else {
			// yLong
			yCoords = append(yCoords, byte(dy>>8), byte(dy))
		}

		flags[i] = flag
		lastX = p.X
		lastY = p.Y
	}

	result = append(result, flags...)
	result = append(result, xCoords...)
	result = append(result, yCoords...)

	return result
}

// ParseGlyfFromFont parses both glyf and loca tables from a font.
func ParseGlyfFromFont(font *Font) (*Glyf, error) {
	// Get numGlyphs from maxp
	maxpData, err := font.TableData(TagMaxp)
	if err != nil {
		return nil, err
	}
	if len(maxpData) < 6 {
		return nil, ErrInvalidTable
	}
	numGlyphs := int(binary.BigEndian.Uint16(maxpData[4:]))

	// Get indexToLocFormat from head
	headData, err := font.TableData(TagHead)
	if err != nil {
		return nil, err
	}
	if len(headData) < 54 {
		return nil, ErrInvalidTable
	}
	indexToLocFormat := int16(binary.BigEndian.Uint16(headData[50:]))

	// Parse loca
	locaData, err := font.TableData(TagLoca)
	if err != nil {
		return nil, err
	}
	loca, err := ParseLoca(locaData, numGlyphs, indexToLocFormat)
	if err != nil {
		return nil, err
	}

	// Parse glyf
	glyfData, err := font.TableData(TagGlyf)
	if err != nil {
		return nil, err
	}

	return ParseGlyf(glyfData, loca)
}
