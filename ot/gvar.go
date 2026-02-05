package ot

import (
	"encoding/binary"
)

// Gvar represents a parsed gvar (Glyph Variations) table.
// It contains variation data for TrueType glyph outlines.
type Gvar struct {
	data                []byte
	axisCount           int
	sharedTupleCount    int
	glyphCount          int
	flags               uint16
	sharedTuplesOffset  uint32
	glyphVarDataOffset  uint32
	glyphVarDataOffsets []uint32 // Offset for each glyph's variation data
}

// ParseGvar parses a gvar table.
func ParseGvar(data []byte) (*Gvar, error) {
	if len(data) < 20 {
		return nil, ErrInvalidTable
	}

	version := binary.BigEndian.Uint16(data[0:])
	if version != 1 {
		return nil, ErrInvalidFormat
	}

	g := &Gvar{
		data:               data,
		axisCount:          int(binary.BigEndian.Uint16(data[4:])),
		sharedTupleCount:   int(binary.BigEndian.Uint16(data[6:])),
		sharedTuplesOffset: binary.BigEndian.Uint32(data[8:]),
		glyphCount:         int(binary.BigEndian.Uint16(data[12:])),
		flags:              binary.BigEndian.Uint16(data[14:]),
		glyphVarDataOffset: binary.BigEndian.Uint32(data[16:]),
	}

	// Parse glyph variation data offsets
	longOffsets := (g.flags & 1) != 0
	offsetsStart := 20

	g.glyphVarDataOffsets = make([]uint32, g.glyphCount+1)

	if longOffsets {
		// 32-bit offsets
		if len(data) < offsetsStart+(g.glyphCount+1)*4 {
			return nil, ErrInvalidOffset
		}
		for i := 0; i <= g.glyphCount; i++ {
			g.glyphVarDataOffsets[i] = binary.BigEndian.Uint32(data[offsetsStart+i*4:])
		}
	} else {
		// 16-bit offsets (multiplied by 2)
		if len(data) < offsetsStart+(g.glyphCount+1)*2 {
			return nil, ErrInvalidOffset
		}
		for i := 0; i <= g.glyphCount; i++ {
			g.glyphVarDataOffsets[i] = uint32(binary.BigEndian.Uint16(data[offsetsStart+i*2:])) * 2
		}
	}

	return g, nil
}

// HasData returns true if the gvar table has valid data.
func (g *Gvar) HasData() bool {
	return g != nil && g.glyphCount > 0
}

// AxisCount returns the number of variation axes.
func (g *Gvar) AxisCount() int {
	return g.axisCount
}

// GlyphCount returns the number of glyphs with variation data.
func (g *Gvar) GlyphCount() int {
	return g.glyphCount
}

// getSharedTuple returns the coordinates for a shared tuple.
// Coordinates are in F2DOT14 format.
func (g *Gvar) getSharedTuple(index int) []int16 {
	if index >= g.sharedTupleCount {
		return nil
	}

	tupleSize := g.axisCount * 2
	offset := int(g.sharedTuplesOffset) + index*tupleSize

	if offset+tupleSize > len(g.data) {
		return nil
	}

	coords := make([]int16, g.axisCount)
	for i := 0; i < g.axisCount; i++ {
		coords[i] = int16(binary.BigEndian.Uint16(g.data[offset+i*2:]))
	}
	return coords
}

// GlyphDeltas represents the variation deltas for a single glyph.
type GlyphDeltas struct {
	// XDeltas and YDeltas contain the accumulated delta values for each point.
	// The length matches the number of points in the glyph (including phantom points).
	// float64 is used for precision during accumulation across multiple tuple variations.
	XDeltas []float64
	YDeltas []float64
}

// GlyphPoint represents a point for IUP interpolation.
type GlyphPoint struct {
	X, Y int16
}

// GetGlyphDeltas computes the delta values for a glyph at the given
// normalized coordinates. The coordinates should be in F2DOT14 format.
// numPoints is the number of points in the glyph (including 4 phantom points).
// Note: This version doesn't perform proper IUP interpolation for sparse deltas.
// Use GetGlyphDeltasWithCoords for proper IUP support.
func (g *Gvar) GetGlyphDeltas(glyphID GlyphID, normalizedCoords []int, numPoints int) *GlyphDeltas {
	return g.GetGlyphDeltasWithCoords(glyphID, normalizedCoords, numPoints, nil)
}

// GetGlyphDeltasWithCoords computes the delta values for a glyph at the given
// normalized coordinates with proper IUP interpolation.
// origCoords contains the original point coordinates for IUP interpolation.
// If nil, a simplified interpolation is used.
func (g *Gvar) GetGlyphDeltasWithCoords(glyphID GlyphID, normalizedCoords []int, numPoints int, origCoords []GlyphPoint) *GlyphDeltas {
	if g == nil || int(glyphID) >= g.glyphCount {
		return nil
	}

	// Get the glyph's variation data
	startOffset := g.glyphVarDataOffset + g.glyphVarDataOffsets[glyphID]
	endOffset := g.glyphVarDataOffset + g.glyphVarDataOffsets[glyphID+1]

	if startOffset == endOffset {
		// No variation data for this glyph
		return nil
	}

	if int(endOffset) > len(g.data) {
		return nil
	}

	glyphData := g.data[startOffset:endOffset]
	if len(glyphData) < 4 {
		return nil
	}

	// Parse TupleVariationCount
	tupleVarCount := binary.BigEndian.Uint16(glyphData[0:])
	tupleCount := int(tupleVarCount & 0x0FFF)
	sharedPointNumbers := (tupleVarCount & 0x8000) != 0
	dataOffset := binary.BigEndian.Uint16(glyphData[2:])

	if tupleCount == 0 {
		return nil
	}

	// Initialize result
	deltas := &GlyphDeltas{
		XDeltas: make([]float64, numPoints),
		YDeltas: make([]float64, numPoints),
	}

	// Parse shared point numbers if present
	var sharedPoints []int
	serializedDataStart := int(dataOffset)
	if sharedPointNumbers {
		var consumed int
		sharedPoints, consumed = g.parsePointNumbers(glyphData[serializedDataStart:])
		serializedDataStart += consumed
	}

	// Parse each tuple variation header and accumulate deltas
	headerOffset := 4
	serializedOffset := serializedDataStart

	for t := 0; t < tupleCount; t++ {
		if headerOffset+4 > len(glyphData) {
			break
		}

		variationDataSize := int(binary.BigEndian.Uint16(glyphData[headerOffset:]))
		tupleIndex := binary.BigEndian.Uint16(glyphData[headerOffset+2:])
		headerOffset += 4

		// Flags from tupleIndex
		embeddedPeakTuple := (tupleIndex & 0x8000) != 0
		intermediateRegion := (tupleIndex & 0x4000) != 0
		privatePointNumbers := (tupleIndex & 0x2000) != 0
		tupleIdx := int(tupleIndex & 0x0FFF)

		// Get peak tuple coordinates
		var peakCoords []int16
		if embeddedPeakTuple {
			peakCoords = make([]int16, g.axisCount)
			for i := 0; i < g.axisCount; i++ {
				if headerOffset+2 > len(glyphData) {
					break
				}
				peakCoords[i] = int16(binary.BigEndian.Uint16(glyphData[headerOffset:]))
				headerOffset += 2
			}
		} else {
			peakCoords = g.getSharedTuple(tupleIdx)
		}

		// Get intermediate region if present
		var startCoords, endCoords []int16
		if intermediateRegion {
			startCoords = make([]int16, g.axisCount)
			endCoords = make([]int16, g.axisCount)
			for i := 0; i < g.axisCount; i++ {
				if headerOffset+2 > len(glyphData) {
					break
				}
				startCoords[i] = int16(binary.BigEndian.Uint16(glyphData[headerOffset:]))
				headerOffset += 2
			}
			for i := 0; i < g.axisCount; i++ {
				if headerOffset+2 > len(glyphData) {
					break
				}
				endCoords[i] = int16(binary.BigEndian.Uint16(glyphData[headerOffset:]))
				headerOffset += 2
			}
		}

		// Calculate scalar for this tuple
		scalar := g.calculateScalar(peakCoords, startCoords, endCoords, normalizedCoords)
		if scalar == 0 {
			serializedOffset += variationDataSize
			continue
		}

		// Parse point numbers for this tuple
		var pointIndices []int
		deltaDataStart := serializedOffset
		if privatePointNumbers {
			var consumed int
			pointIndices, consumed = g.parsePointNumbers(glyphData[serializedOffset:])
			deltaDataStart += consumed
		} else {
			pointIndices = sharedPoints
		}

		// Parse deltas
		xDeltas, yDeltas, _ := g.parseDeltas(glyphData[deltaDataStart:], len(pointIndices), numPoints)

		// Apply deltas with scalar
		if len(pointIndices) == 0 {
			// All points
			for i := 0; i < numPoints && i < len(xDeltas); i++ {
				deltas.XDeltas[i] += float64(xDeltas[i]) * scalar
				deltas.YDeltas[i] += float64(yDeltas[i]) * scalar
			}
		} else {
			// Specific points - need interpolation for missing points
			g.applyDeltasWithInterpolation(deltas, pointIndices, xDeltas, yDeltas, scalar, numPoints, origCoords)
		}

		serializedOffset += variationDataSize
	}

	return deltas
}

// calculateScalar computes the scalar value for a tuple variation.
func (g *Gvar) calculateScalar(peak, start, end []int16, coords []int) float64 {
	if len(peak) == 0 {
		return 0
	}

	var scalar float64 = 1.0

	for i := 0; i < len(peak) && i < len(coords); i++ {
		peakVal := int(peak[i])
		coordVal := coords[i]

		if peakVal == 0 {
			continue
		}

		if coordVal == peakVal {
			continue
		}

		// Check intermediate region
		var startVal, endVal int
		if start != nil && end != nil {
			startVal = int(start[i])
			endVal = int(end[i])
		} else {
			// No intermediate region - use default
			if peakVal > 0 {
				startVal = 0
				endVal = peakVal
			} else {
				startVal = peakVal
				endVal = 0
			}
		}

		// Outside the region?
		if coordVal <= startVal || coordVal >= endVal {
			if coordVal < startVal && peakVal > startVal {
				return 0
			}
			if coordVal > endVal && peakVal < endVal {
				return 0
			}
			if coordVal == 0 {
				return 0
			}
		}

		// Interpolate
		if coordVal < peakVal {
			if peakVal != startVal {
				scalar *= float64(coordVal-startVal) / float64(peakVal-startVal)
			}
		} else {
			if peakVal != endVal {
				scalar *= float64(endVal-coordVal) / float64(endVal-peakVal)
			}
		}
	}

	return scalar
}

// parsePointNumbers parses packed point numbers.
// Returns the point indices and number of bytes consumed.
func (g *Gvar) parsePointNumbers(data []byte) ([]int, int) {
	if len(data) == 0 {
		return nil, 0
	}

	count := int(data[0])
	offset := 1

	if count == 0 {
		// All points
		return nil, 1
	}

	if count&0x80 != 0 {
		// High byte present
		if len(data) < 2 {
			return nil, 1
		}
		count = ((count & 0x7F) << 8) | int(data[1])
		offset = 2
	}

	points := make([]int, 0, count)
	pointsRead := 0
	lastPoint := 0

	for pointsRead < count && offset < len(data) {
		runHeader := data[offset]
		offset++

		pointsAreWords := (runHeader & 0x80) != 0
		runCount := int(runHeader&0x7F) + 1

		for i := 0; i < runCount && pointsRead < count; i++ {
			var delta int
			if pointsAreWords {
				if offset+2 > len(data) {
					break
				}
				delta = int(binary.BigEndian.Uint16(data[offset:]))
				offset += 2
			} else {
				if offset >= len(data) {
					break
				}
				delta = int(data[offset])
				offset++
			}
			lastPoint += delta
			points = append(points, lastPoint)
			pointsRead++
		}
	}

	return points, offset
}

// parseDeltas parses packed delta values.
func (g *Gvar) parseDeltas(data []byte, numDeltas, numPoints int) (xDeltas, yDeltas []int16, consumed int) {
	if numDeltas == 0 {
		numDeltas = numPoints
	}

	xDeltas = make([]int16, numDeltas)
	yDeltas = make([]int16, numDeltas)
	offset := 0

	// Parse X deltas
	deltasRead := 0
	for deltasRead < numDeltas && offset < len(data) {
		runHeader := data[offset]
		offset++

		deltasAreZero := (runHeader & 0x80) != 0
		deltasAreWords := (runHeader & 0x40) != 0
		runCount := int(runHeader&0x3F) + 1

		for i := 0; i < runCount && deltasRead < numDeltas; i++ {
			var delta int16
			if deltasAreZero {
				delta = 0
			} else if deltasAreWords {
				if offset+2 > len(data) {
					break
				}
				delta = int16(binary.BigEndian.Uint16(data[offset:]))
				offset += 2
			} else {
				if offset >= len(data) {
					break
				}
				delta = int16(int8(data[offset]))
				offset++
			}
			xDeltas[deltasRead] = delta
			deltasRead++
		}
	}

	// Parse Y deltas
	deltasRead = 0
	for deltasRead < numDeltas && offset < len(data) {
		runHeader := data[offset]
		offset++

		deltasAreZero := (runHeader & 0x80) != 0
		deltasAreWords := (runHeader & 0x40) != 0
		runCount := int(runHeader&0x3F) + 1

		for i := 0; i < runCount && deltasRead < numDeltas; i++ {
			var delta int16
			if deltasAreZero {
				delta = 0
			} else if deltasAreWords {
				if offset+2 > len(data) {
					break
				}
				delta = int16(binary.BigEndian.Uint16(data[offset:]))
				offset += 2
			} else {
				if offset >= len(data) {
					break
				}
				delta = int16(int8(data[offset]))
				offset++
			}
			yDeltas[deltasRead] = delta
			deltasRead++
		}
	}

	return xDeltas, yDeltas, offset
}

// applyDeltasWithInterpolation applies deltas to specific points and interpolates others.
// This implements the IUP (Interpolate Untouched Points) algorithm.
func (g *Gvar) applyDeltasWithInterpolation(deltas *GlyphDeltas, pointIndices []int, xDelta, yDelta []int16, scalar float64, numPoints int, origCoords []GlyphPoint) {
	// Create a set of touched points
	touched := make(map[int]bool)
	for _, idx := range pointIndices {
		touched[idx] = true
	}

	// Apply deltas to specified points
	for i, ptIdx := range pointIndices {
		if ptIdx >= numPoints || i >= len(xDelta) {
			continue
		}
		deltas.XDeltas[ptIdx] += float64(xDelta[i]) * scalar
		deltas.YDeltas[ptIdx] += float64(yDelta[i]) * scalar
	}

	// IUP: For untouched points, interpolate from neighboring touched points
	// Since we don't have contour information here, we treat all points as one contour
	// This is a simplification - proper IUP would need contour boundaries

	// Find all untouched points and interpolate
	for i := 0; i < numPoints; i++ {
		if touched[i] {
			continue
		}

		// Find previous touched point
		prevTouched := -1
		for j := 1; j <= numPoints; j++ {
			idx := (i - j + numPoints) % numPoints
			if touched[idx] {
				prevTouched = idx
				break
			}
		}

		// Find next touched point
		nextTouched := -1
		for j := 1; j <= numPoints; j++ {
			idx := (i + j) % numPoints
			if touched[idx] {
				nextTouched = idx
				break
			}
		}

		if prevTouched == -1 || nextTouched == -1 {
			// No touched points found, delta stays 0
			continue
		}

		if prevTouched == nextTouched {
			// Only one touched point, use its delta
			deltas.XDeltas[i] = deltas.XDeltas[prevTouched]
			deltas.YDeltas[i] = deltas.YDeltas[prevTouched]
			continue
		}

		// IUP interpolation using original coordinates
		if origCoords != nil && i < len(origCoords) && prevTouched < len(origCoords) && nextTouched < len(origCoords) {
			deltas.XDeltas[i] += iupInterpolate(
				origCoords[i].X,
				origCoords[prevTouched].X, origCoords[nextTouched].X,
				deltas.XDeltas[prevTouched], deltas.XDeltas[nextTouched],
			)
			deltas.YDeltas[i] += iupInterpolate(
				origCoords[i].Y,
				origCoords[prevTouched].Y, origCoords[nextTouched].Y,
				deltas.YDeltas[prevTouched], deltas.YDeltas[nextTouched],
			)
		} else {
			// Fallback: simple average
			deltas.XDeltas[i] = (deltas.XDeltas[prevTouched] + deltas.XDeltas[nextTouched]) / 2
			deltas.YDeltas[i] = (deltas.YDeltas[prevTouched] + deltas.YDeltas[nextTouched]) / 2
		}
	}
}

// iupInterpolate performs IUP interpolation for a single coordinate.
// coord is the coordinate of the untouched point.
// coord1, coord2 are the coordinates of the two surrounding touched points.
// delta1, delta2 are the deltas of the two surrounding touched points.
func iupInterpolate(coord, coord1, coord2 int16, delta1, delta2 float64) float64 {
	// Ensure coord1 <= coord2 for the algorithm
	if coord1 > coord2 {
		coord1, coord2 = coord2, coord1
		delta1, delta2 = delta2, delta1
	}

	if coord <= coord1 {
		// coord is at or before coord1 - use delta1
		return delta1
	}
	if coord >= coord2 {
		// coord is at or after coord2 - use delta2
		return delta2
	}

	// coord is strictly between coord1 and coord2 - interpolate
	if coord1 == coord2 {
		// Same position, use average
		return (delta1 + delta2) / 2
	}

	// Linear interpolation: delta = delta1 + (delta2 - delta1) * (coord - coord1) / (coord2 - coord1)
	t := float64(coord-coord1) / float64(coord2-coord1)
	return delta1 + t*(delta2-delta1)
}
