package ot

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/boxesandglue/textshape/internal/testutil"
)

// HarfBuzz reference: the SVG accessors validated here mirror
// hb_ot_color_has_svg (hb-ot-color.cc:287) and
// hb_ot_color_glyph_reference_svg (hb-ot-color.cc:306). Test fonts come
// from HarfBuzz's own corpus under SIL OFL 1.1.

func loadFontForSVG(t *testing.T, name string) *Font {
	t.Helper()
	path := testutil.FindTestFont(name)
	if path == "" {
		t.Skipf("%s not found", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return font
}

// chromacheck-svg.ttf is the minimal SVG-OT smoke test: one document
// covering exactly glyph 1. Useful for proving the parser does not
// stumble on the tightest possible table.
func TestSVGChromaCheck(t *testing.T) {
	f := loadFontForSVG(t, "chromacheck-svg.ttf")
	if !f.HasColorSVG() {
		t.Fatal("HasColorSVG() = false, want true")
	}
	blob := f.GlyphColorSVG(1)
	if blob == nil {
		t.Fatal("GlyphColorSVG(1) = nil")
	}
	if !bytes.Contains(blob, []byte("<svg")) {
		t.Errorf("blob does not contain '<svg'; first 80 bytes: %q", blob[:min(80, len(blob))])
	}
	// Glyph 0 has no SVG entry — must return nil.
	if got := f.GlyphColorSVG(0); got != nil {
		t.Errorf("GlyphColorSVG(0) = %q, want nil", got)
	}
	// Glyphs outside any range — must return nil.
	if got := f.GlyphColorSVG(99); got != nil {
		t.Errorf("GlyphColorSVG(99) = %q, want nil", got)
	}
}

// TestSVGMultiGlyphs exercises the range-spanning case: one SVG
// document describing several glyphs at once. Every GID in the range
// must resolve to the same document bytes — HB's bsearch+range_lookup
// guarantee (SVG.hh:46) that we mirror here.
func TestSVGMultiGlyphs(t *testing.T) {
	f := loadFontForSVG(t, "TestSVGmultiGlyphs.otf")
	if !f.HasColorSVG() {
		t.Fatal("HasColorSVG() = false")
	}
	// Header dump: entries cover gid ranges [3-7] and [8-13]. Every
	// gid inside a range must return its range's document; gids
	// outside (0-2 and 14+) must return nil.
	doc1 := f.GlyphColorSVG(3)
	if doc1 == nil {
		t.Fatal("GlyphColorSVG(3) = nil")
	}
	for gid := GlyphID(4); gid <= 7; gid++ {
		got := f.GlyphColorSVG(gid)
		if !bytes.Equal(got, doc1) {
			t.Errorf("GlyphColorSVG(%d) differs from GlyphColorSVG(3); same range should share doc", gid)
		}
	}
	doc2 := f.GlyphColorSVG(8)
	if doc2 == nil {
		t.Fatal("GlyphColorSVG(8) = nil")
	}
	if bytes.Equal(doc1, doc2) {
		t.Error("range [3-7] and [8-13] returned the same doc — header says they should differ")
	}
	for gid := GlyphID(9); gid <= 13; gid++ {
		got := f.GlyphColorSVG(gid)
		if !bytes.Equal(got, doc2) {
			t.Errorf("GlyphColorSVG(%d) differs from GlyphColorSVG(8); same range should share doc", gid)
		}
	}
	// Outside any range.
	for _, gid := range []GlyphID{0, 1, 2, 14, 99} {
		if got := f.GlyphColorSVG(gid); got != nil {
			t.Errorf("GlyphColorSVG(%d) = %q, want nil (outside ranges)", gid, got)
		}
	}
}

// TestSVGGzip verifies gzip auto-decompression. Spec allows SVG
// documents to be stored gzip-compressed (magic 0x1F 0x8B); our parser
// transparently inflates so callers receive XML bytes regardless of
// on-disk encoding.
func TestSVGGzip(t *testing.T) {
	f := loadFontForSVG(t, "TestSVGgzip.otf")
	if !f.HasColorSVG() {
		t.Fatal("HasColorSVG() = false")
	}
	blob := f.GlyphColorSVG(3)
	if blob == nil {
		t.Fatal("GlyphColorSVG(3) = nil")
	}
	// After inflation the blob must start with the XML/SVG header,
	// not the gzip magic.
	if blob[0] == 0x1F && blob[1] == 0x8B {
		t.Error("blob still gzip-compressed after GlyphColorSVG; decompression failed")
	}
	if !strings.Contains(string(blob), "<svg") {
		t.Errorf("decompressed blob has no '<svg' tag: %q", blob[:min(120, len(blob))])
	}
}

func TestSVGNonColorFontReturnsNil(t *testing.T) {
	// Roboto is a plain TTF: no SVG table at all.
	f := loadFontForSVG(t, "Roboto-Regular.ttf")
	if f.HasColorSVG() {
		t.Error("HasColorSVG() = true on Roboto, want false")
	}
	if got := f.GlyphColorSVG(1); got != nil {
		t.Errorf("GlyphColorSVG(1) on Roboto = %q, want nil", got)
	}
}
