package ot

import (
	"os"
	"testing"
)

func TestGlyphOutlineSimple(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	face, err := LoadFaceFromData(data, 0)
	if err != nil {
		t.Fatalf("Failed to load face: %v", err)
	}

	// Look up glyph ID for 'A' (U+0041) via cmap.
	cmap := face.Cmap()
	if cmap == nil {
		t.Fatal("Font has no cmap")
	}
	gid, ok := cmap.Lookup('A')
	if !ok {
		t.Fatal("cmap has no mapping for 'A'")
	}

	outline, hasOutline := face.GlyphOutline(gid)
	if !hasOutline {
		t.Fatal("Expected outline for 'A', got none")
	}
	if len(outline.Segments) == 0 {
		t.Fatal("Expected non-empty segments for 'A'")
	}

	// First segment must be a MoveTo.
	if outline.Segments[0].Op != SegmentMoveTo {
		t.Errorf("First segment should be MoveTo, got %d", outline.Segments[0].Op)
	}

	// Verify we have at least some line/quad segments.
	hasLine := false
	for _, seg := range outline.Segments {
		if seg.Op == SegmentLineTo || seg.Op == SegmentQuadTo {
			hasLine = true
			break
		}
	}
	if !hasLine {
		t.Error("Expected at least one LineTo or QuadTo segment for 'A'")
	}

	t.Logf("Glyph 'A' (gid=%d): %d segments", gid, len(outline.Segments))
}

func TestGlyphOutlineSpace(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	face, err := LoadFaceFromData(data, 0)
	if err != nil {
		t.Fatalf("Failed to load face: %v", err)
	}

	cmap := face.Cmap()
	if cmap == nil {
		t.Fatal("Font has no cmap")
	}
	gid, ok := cmap.Lookup(' ')
	if !ok {
		t.Fatal("cmap has no mapping for space")
	}

	_, hasOutline := face.GlyphOutline(gid)
	if hasOutline {
		t.Error("Space glyph should have no outline")
	}
}

func TestGlyphOutlineCFF(t *testing.T) {
	fontPath := findTestFont("SourceSansPro-Regular.otf")
	if fontPath == "" {
		t.Skip("SourceSansPro-Regular.otf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	face, err := LoadFaceFromData(data, 0)
	if err != nil {
		t.Fatalf("Failed to load face: %v", err)
	}

	if !face.IsCFF() {
		t.Skip("Expected CFF font")
	}

	cmap := face.Cmap()
	if cmap == nil {
		t.Fatal("Font has no cmap")
	}
	gid, ok := cmap.Lookup('A')
	if !ok {
		t.Fatal("cmap has no mapping for 'A'")
	}

	outline, hasOutline := face.GlyphOutline(gid)
	if !hasOutline {
		t.Fatal("Expected outline for CFF glyph 'A', got none")
	}
	if len(outline.Segments) == 0 {
		t.Fatal("Expected non-empty segments for CFF 'A'")
	}

	// First segment must be a MoveTo.
	if outline.Segments[0].Op != SegmentMoveTo {
		t.Errorf("First segment should be MoveTo, got %d", outline.Segments[0].Op)
	}

	// CFF uses cubic Bézier curves (SegmentCubeTo), not quadratic.
	hasCubic := false
	for _, seg := range outline.Segments {
		if seg.Op == SegmentCubeTo {
			hasCubic = true
			break
		}
	}
	if !hasCubic {
		t.Error("Expected at least one CubeTo segment for CFF 'A'")
	}

	t.Logf("CFF Glyph 'A' (gid=%d): %d segments", gid, len(outline.Segments))
}

func TestGlyphOutlineCFFMultipleGlyphs(t *testing.T) {
	fontPath := findTestFont("SourceSansPro-Regular.otf")
	if fontPath == "" {
		t.Skip("SourceSansPro-Regular.otf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	face, err := LoadFaceFromData(data, 0)
	if err != nil {
		t.Fatalf("Failed to load face: %v", err)
	}

	if !face.IsCFF() {
		t.Skip("Expected CFF font")
	}

	cmap := face.Cmap()
	if cmap == nil {
		t.Fatal("Font has no cmap")
	}

	for _, r := range "ABCabc0123" {
		gid, ok := cmap.Lookup(Codepoint(r))
		if !ok {
			t.Errorf("cmap has no mapping for %q", r)
			continue
		}
		outline, hasOutline := face.GlyphOutline(gid)
		if !hasOutline {
			t.Errorf("Expected outline for CFF %q (gid=%d)", r, gid)
			continue
		}
		if outline.Segments[0].Op != SegmentMoveTo {
			t.Errorf("First segment for CFF %q should be MoveTo, got %d", r, outline.Segments[0].Op)
		}
	}
}

func TestGlyphOutlineCFFSpace(t *testing.T) {
	fontPath := findTestFont("SourceSansPro-Regular.otf")
	if fontPath == "" {
		t.Skip("SourceSansPro-Regular.otf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	face, err := LoadFaceFromData(data, 0)
	if err != nil {
		t.Fatalf("Failed to load face: %v", err)
	}

	if !face.IsCFF() {
		t.Skip("Expected CFF font")
	}

	cmap := face.Cmap()
	if cmap == nil {
		t.Fatal("Font has no cmap")
	}
	gid, ok := cmap.Lookup(' ')
	if !ok {
		t.Fatal("cmap has no mapping for space")
	}

	_, hasOutline := face.GlyphOutline(gid)
	if hasOutline {
		t.Error("Space glyph in CFF font should have no outline")
	}
}

func TestGlyphOutlineMultipleGlyphs(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	face, err := LoadFaceFromData(data, 0)
	if err != nil {
		t.Fatalf("Failed to load face: %v", err)
	}

	cmap := face.Cmap()
	if cmap == nil {
		t.Fatal("Font has no cmap")
	}

	// Test a variety of characters.
	for _, r := range "ABCabc0123" {
		gid, ok := cmap.Lookup(Codepoint(r))
		if !ok {
			t.Errorf("cmap has no mapping for %q", r)
			continue
		}
		outline, hasOutline := face.GlyphOutline(gid)
		if !hasOutline {
			t.Errorf("Expected outline for %q (gid=%d)", r, gid)
			continue
		}
		if outline.Segments[0].Op != SegmentMoveTo {
			t.Errorf("First segment for %q should be MoveTo, got %d", r, outline.Segments[0].Op)
		}
	}
}

func TestPathBuilderImplicitOnCurve(t *testing.T) {
	// Simulate a contour with two consecutive off-curve points.
	// Points: off(0,100), off(100,100), on(100,0), on(0,0)
	// Between the two off-curve points, an implicit on-curve midpoint (50,100)
	// should be generated.
	var pb pathBuilder

	pb.consumePoint(0, 100, false)
	pb.consumePoint(100, 100, false)
	pb.consumePoint(100, 0, true)
	pb.consumePoint(0, 0, true)
	pb.contourEnd()

	if len(pb.segments) == 0 {
		t.Fatal("Expected segments from pathBuilder")
	}
	if pb.segments[0].Op != SegmentMoveTo {
		t.Errorf("First segment should be MoveTo, got %d", pb.segments[0].Op)
	}

	// Should have at least one QuadTo from the implicit midpoint logic.
	hasQuad := false
	for _, seg := range pb.segments {
		if seg.Op == SegmentQuadTo {
			hasQuad = true
			break
		}
	}
	if !hasQuad {
		t.Error("Expected QuadTo segment from implicit on-curve midpoint handling")
	}
}
