package ot

import "encoding/binary"

// SegmentOp represents the type of a path segment operation.
type SegmentOp uint8

const (
	// SegmentMoveTo moves the current point to a new position (starts a new contour).
	SegmentMoveTo SegmentOp = iota
	// SegmentLineTo draws a straight line to the endpoint.
	SegmentLineTo
	// SegmentQuadTo draws a quadratic Bézier curve. Args: [0]=control, [1]=endpoint.
	SegmentQuadTo
	// SegmentCubeTo draws a cubic Bézier curve. Args: [0]=ctrl1, [1]=ctrl2, [2]=endpoint.
	SegmentCubeTo
)

// OutlinePoint represents a 2D point in glyph outline coordinates.
type OutlinePoint struct{ X, Y float32 }

// Segment represents a single path segment in a glyph outline.
type Segment struct {
	Op   SegmentOp
	Args [3]OutlinePoint
}

// GlyphOutline contains the path segments for a glyph's outline.
type GlyphOutline struct {
	Segments []Segment
}

// pathBuilder converts raw TrueType contour points into path segments.
// This is a port of HarfBuzz's path-builder.hh logic.
type pathBuilder struct {
	segments []Segment

	// State tracking for the current contour.
	// TrueType contours can start with off-curve points, and consecutive
	// off-curve points imply an on-curve midpoint between them.
	firstOnCurve  *OutlinePoint // first on-curve point seen
	firstOffCurve *OutlinePoint // first off-curve point (if contour starts off-curve)
	lastOffCurve  *OutlinePoint // pending off-curve point from previous consumePoint
}

func (pb *pathBuilder) moveTo(p OutlinePoint) {
	pb.segments = append(pb.segments, Segment{
		Op:   SegmentMoveTo,
		Args: [3]OutlinePoint{p},
	})
}

func (pb *pathBuilder) lineTo(p OutlinePoint) {
	pb.segments = append(pb.segments, Segment{
		Op:   SegmentLineTo,
		Args: [3]OutlinePoint{p},
	})
}

func (pb *pathBuilder) quadTo(ctrl, end OutlinePoint) {
	pb.segments = append(pb.segments, Segment{
		Op:   SegmentQuadTo,
		Args: [3]OutlinePoint{ctrl, end},
	})
}

// consumePoint processes a single contour point. The onCurve flag indicates
// whether the point is on-curve (true) or an off-curve control point (false).
func (pb *pathBuilder) consumePoint(x, y float32, onCurve bool) {
	p := OutlinePoint{x, y}

	if pb.firstOnCurve == nil && pb.firstOffCurve == nil && pb.lastOffCurve == nil {
		// Very first point of the contour.
		if onCurve {
			pb.firstOnCurve = &OutlinePoint{p.X, p.Y}
			pb.moveTo(p)
		} else {
			pb.firstOffCurve = &OutlinePoint{p.X, p.Y}
		}
		return
	}

	if pb.firstOnCurve == nil {
		// We haven't seen an on-curve point yet; contour started off-curve.
		if onCurve {
			pb.firstOnCurve = &OutlinePoint{p.X, p.Y}
			pb.moveTo(p)
		} else {
			// Two consecutive off-curve at start: implicit on-curve midpoint.
			mid := midpoint(*pb.firstOffCurve, p)
			pb.firstOnCurve = &OutlinePoint{mid.X, mid.Y}
			pb.moveTo(mid)
			pb.lastOffCurve = &OutlinePoint{p.X, p.Y}
		}
		return
	}

	if pb.lastOffCurve != nil {
		if onCurve {
			pb.quadTo(*pb.lastOffCurve, p)
			pb.lastOffCurve = nil
		} else {
			// Two consecutive off-curve: emit quad to implicit midpoint.
			mid := midpoint(*pb.lastOffCurve, p)
			pb.quadTo(*pb.lastOffCurve, mid)
			pb.lastOffCurve = &OutlinePoint{p.X, p.Y}
		}
		return
	}

	// No pending off-curve.
	if onCurve {
		pb.lineTo(p)
	} else {
		pb.lastOffCurve = &OutlinePoint{p.X, p.Y}
	}
}

// contourEnd closes the current contour, connecting back to the start.
func (pb *pathBuilder) contourEnd() {
	if pb.firstOnCurve == nil {
		// Entire contour is off-curve points (degenerate but possible).
		// If we have a single off-curve point, there's nothing to draw.
		if pb.firstOffCurve != nil && pb.lastOffCurve != nil {
			mid := midpoint(*pb.firstOffCurve, *pb.lastOffCurve)
			pb.moveTo(mid)
			pb.quadTo(*pb.lastOffCurve, *pb.firstOffCurve)
			pb.quadTo(*pb.firstOffCurve, mid)
		}
		pb.reset()
		return
	}

	// Close path: handle pending off-curve points.
	if pb.lastOffCurve != nil {
		if pb.firstOffCurve != nil {
			// Both a pending last and a first off-curve: close through implicit midpoints.
			mid := midpoint(*pb.lastOffCurve, *pb.firstOffCurve)
			pb.quadTo(*pb.lastOffCurve, mid)
			pb.quadTo(*pb.firstOffCurve, *pb.firstOnCurve)
		} else {
			pb.quadTo(*pb.lastOffCurve, *pb.firstOnCurve)
		}
	} else if pb.firstOffCurve != nil {
		pb.quadTo(*pb.firstOffCurve, *pb.firstOnCurve)
	} else {
		pb.lineTo(*pb.firstOnCurve)
	}

	pb.reset()
}

func (pb *pathBuilder) reset() {
	pb.firstOnCurve = nil
	pb.firstOffCurve = nil
	pb.lastOffCurve = nil
}

func midpoint(a, b OutlinePoint) OutlinePoint {
	return OutlinePoint{
		X: (a.X + b.X) / 2,
		Y: (a.Y + b.Y) / 2,
	}
}

// GlyphOutline extracts the outline (path segments) for a glyph.
// Returns the outline and true if the glyph has outline data.
// For empty glyphs (e.g. space), returns an empty outline and false.
// Supports both TrueType (glyf table, quadratic) and CFF (cubic) outlines.
func (f *Face) GlyphOutline(gid GlyphID) (GlyphOutline, bool) {
	if f.isCFF {
		// CFF outline extraction.
		// HarfBuzz equivalent: OT::cff1::accelerator_t::get_path in hb-ot-cff1-table.cc:444-564
		cff := f.getCFF()
		if cff == nil {
			return GlyphOutline{}, false
		}
		return cffGlyphOutline(cff, gid)
	}

	g := f.getGlyf()
	if g == nil {
		return GlyphOutline{}, false
	}

	return g.glyphOutline(gid)
}

// glyphOutline extracts the outline for a glyph from the glyf table.
func (g *Glyf) glyphOutline(gid GlyphID) (GlyphOutline, bool) {
	glyph := g.GetGlyph(gid)
	if glyph == nil || glyph.Data == nil {
		return GlyphOutline{}, false
	}

	if glyph.NumberOfContours >= 0 {
		return g.simpleGlyphOutline(glyph)
	}
	return g.compositeGlyphOutline(glyph)
}

// simpleGlyphOutline extracts the outline for a simple glyph.
func (g *Glyf) simpleGlyphOutline(glyph *GlyphData) (GlyphOutline, bool) {
	if glyph.NumberOfContours == 0 {
		return GlyphOutline{}, false
	}

	data := glyph.Data
	numContours := int(glyph.NumberOfContours)

	// Read endPtsOfContours (at offset 10 after glyph header).
	if len(data) < 10+numContours*2 {
		return GlyphOutline{}, false
	}
	endPts := make([]int, numContours)
	for i := 0; i < numContours; i++ {
		endPts[i] = int(binary.BigEndian.Uint16(data[10+i*2:]))
	}

	points, _, err := ParseSimpleGlyph(data)
	if err != nil || len(points) == 0 {
		return GlyphOutline{}, false
	}

	var pb pathBuilder
	contourIdx := 0
	for i, pt := range points {
		pb.consumePoint(float32(pt.X), float32(pt.Y), pt.OnCurve)
		if contourIdx < numContours && i == endPts[contourIdx] {
			pb.contourEnd()
			contourIdx++
		}
	}

	if len(pb.segments) == 0 {
		return GlyphOutline{}, false
	}
	return GlyphOutline{Segments: pb.segments}, true
}

// compositeGlyphOutline extracts the outline for a composite glyph by
// recursively resolving components and applying their transforms.
func (g *Glyf) compositeGlyphOutline(glyph *GlyphData) (GlyphOutline, bool) {
	components := g.parseCompositeWithTransform(glyph.Data)
	if len(components) == 0 {
		return GlyphOutline{}, false
	}

	var allSegments []Segment
	for _, comp := range components {
		sub, ok := g.glyphOutline(comp.GlyphID)
		if !ok {
			continue
		}
		// Apply the component's affine transform to each segment point.
		for _, seg := range sub.Segments {
			var transformed Segment
			transformed.Op = seg.Op
			n := argsCount(seg.Op)
			for j := 0; j < n; j++ {
				transformed.Args[j] = comp.transformPoint(seg.Args[j])
			}
			allSegments = append(allSegments, transformed)
		}
	}

	if len(allSegments) == 0 {
		return GlyphOutline{}, false
	}
	return GlyphOutline{Segments: allSegments}, true
}

// argsCount returns the number of meaningful Args entries for a segment op.
func argsCount(op SegmentOp) int {
	switch op {
	case SegmentMoveTo, SegmentLineTo:
		return 1
	case SegmentQuadTo:
		return 2
	case SegmentCubeTo:
		return 3
	}
	return 0
}

// compositeTransform holds a component's affine transform matrix.
// The transform is: [xx xy] [x]   [dx]
//
//	[yx yy] [y] + [dy]
type compositeTransform struct {
	GlyphID GlyphID
	dx, dy  float32
	xx, xy  float32
	yx, yy  float32
}

func (ct *compositeTransform) transformPoint(p OutlinePoint) OutlinePoint {
	return OutlinePoint{
		X: ct.xx*p.X + ct.xy*p.Y + ct.dx,
		Y: ct.yx*p.X + ct.yy*p.Y + ct.dy,
	}
}

// parseCompositeWithTransform parses composite glyph components including
// their full affine transforms (translation, scale, xy-scale, 2x2 matrix).
func (g *Glyf) parseCompositeWithTransform(data []byte) []compositeTransform {
	if len(data) < 10 {
		return nil
	}

	offset := 10 // skip glyph header
	var components []compositeTransform

	for {
		if offset+4 > len(data) {
			break
		}

		flags := binary.BigEndian.Uint16(data[offset:])
		glyphIndex := GlyphID(binary.BigEndian.Uint16(data[offset+2:]))
		offset += 4

		ct := compositeTransform{
			GlyphID: glyphIndex,
			xx:      1, yy: 1, // identity
		}

		// Parse translation arguments.
		if flags&argAreWords != 0 {
			if offset+4 > len(data) {
				break
			}
			ct.dx = float32(int16(binary.BigEndian.Uint16(data[offset:])))
			ct.dy = float32(int16(binary.BigEndian.Uint16(data[offset+2:])))
			offset += 4
		} else {
			if offset+2 > len(data) {
				break
			}
			ct.dx = float32(int8(data[offset]))
			ct.dy = float32(int8(data[offset+1]))
			offset += 2
		}

		// Parse transform matrix components (F2Dot14 format).
		if flags&weHaveAScale != 0 {
			if offset+2 > len(data) {
				break
			}
			scale := f2dot14(binary.BigEndian.Uint16(data[offset:]))
			ct.xx = scale
			ct.yy = scale
			offset += 2
		} else if flags&weHaveXYScale != 0 {
			if offset+4 > len(data) {
				break
			}
			ct.xx = f2dot14(binary.BigEndian.Uint16(data[offset:]))
			ct.yy = f2dot14(binary.BigEndian.Uint16(data[offset+2:]))
			offset += 4
		} else if flags&weHave2x2 != 0 {
			if offset+8 > len(data) {
				break
			}
			ct.xx = f2dot14(binary.BigEndian.Uint16(data[offset:]))
			ct.yx = f2dot14(binary.BigEndian.Uint16(data[offset+2:]))
			ct.xy = f2dot14(binary.BigEndian.Uint16(data[offset+4:]))
			ct.yy = f2dot14(binary.BigEndian.Uint16(data[offset+6:]))
			offset += 8
		}

		components = append(components, ct)

		if flags&moreComponents == 0 {
			break
		}
	}

	return components
}

// f2dot14 converts a F2Dot14 fixed-point value to float32.
func f2dot14(v uint16) float32 {
	return float32(int16(v)) / 16384.0
}
