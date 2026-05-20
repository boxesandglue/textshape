package ot

import (
	"bytes"
	"os"
	"testing"

	"github.com/boxesandglue/textshape/internal/testutil"
)

// These tests cover the sbix and CBDT/CBLC parsers plus the unified
// Face-level color PNG accessor. Fonts come from HarfBuzz's own corpus
// at test/fuzzing/fonts/ — same source we used for COLR/CPAL Phase 1.
// SIL OFL 1.1 (see HB test/COPYING) makes them redistributable.

// pngSignature is the standard 8-byte PNG file header. Used as the
// minimal correctness check for "this blob is at least a PNG".
var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

func loadFontForColor(t *testing.T, name string) *Font {
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

func TestSbixParseRejectsCorrupt(t *testing.T) {
	// sbix-extents.ttf is a HarfBuzz fuzz fixture with a deliberately
	// corrupt sbix header (declares ~761 MB of sbix data inside a
	// 582-byte file). The parser must reject it gracefully — no panic,
	// no out-of-bounds read. HasColorPNG must report false.
	//
	// HarfBuzz equivalent: sbix::sanitize (sbix.hh:368-375) returns
	// false on this fixture; HB exposes that via has_data() == false.
	f := loadFontForColor(t, "sbix-extents.ttf")
	if !f.HasTable(TagSbix) {
		t.Fatal("sbix table tag missing from sbix-extents.ttf")
	}
	// TableData itself catches the bogus length and refuses; ParseSbix
	// never runs on these bytes. HasColorPNG must therefore return false.
	if got := f.HasColorPNG(); got {
		t.Error("HasColorPNG() = true on corrupt-sbix font, want false")
	}
}

// TestSbixParseSynthetic builds a minimal-but-valid sbix table by hand
// and verifies the happy path: one strike at ppem=64 containing a single
// PNG-signed blob with the expected xOffset/yOffset.
//
// HarfBuzz equivalent: this is what hb-test-color-fonts.c would test
// against a real Apple Color Emoji, but we cannot commit such a font.
// The synthetic blob is small and exercises the same SBIXStrike +
// SBIXGlyph reader code path as a real font.
func TestSbixParseSynthetic(t *testing.T) {
	const numGlyphs = 2
	// Build a single SBIXGlyph: xOff=3, yOff=-5, type='png ', data=8x"PG".
	// SBIXGlyph header is 8 bytes; payload is arbitrary here.
	glyph := []byte{
		0x00, 0x03, // xOffset = 3
		0xFF, 0xFB, // yOffset = -5 (two's complement int16)
		'p', 'n', 'g', ' ', // graphicType = 'png '
		'P', 'G', 'P', 'G', 'P', 'G', 'P', 'G', // 8 bytes of pretend PNG
	}
	// Strike layout: 4-byte header (ppem, resolution) + (numGlyphs+1)*4 offsets.
	// Glyph 0 lives at offset (4 + 12) = 16 from strike start; glyph 1 absent.
	strike := []byte{}
	strike = appendBE16(strike, 64) // ppem
	strike = appendBE16(strike, 72) // resolution
	const glyphOff = uint32(4 + (numGlyphs+1)*4)
	strike = appendBE32(strike, glyphOff)                          // imageOffsets[0]
	strike = appendBE32(strike, glyphOff+uint32(len(glyph)))       // imageOffsets[1]
	strike = appendBE32(strike, glyphOff+uint32(len(glyph)))       // imageOffsets[2] (sentinel; gid 1 zero-length)
	strike = append(strike, glyph...)

	// sbix layout: 8-byte header + 4-byte strike offset, then strike body.
	const strikeAbsOff = uint32(8 + 4)
	tbl := []byte{}
	tbl = appendBE16(tbl, 1)              // version = 1
	tbl = appendBE16(tbl, 1)              // flags = 0x0001
	tbl = appendBE32(tbl, 1)              // numStrikes = 1
	tbl = appendBE32(tbl, strikeAbsOff)   // strikeOffsets[0]
	tbl = append(tbl, strike...)

	sbix, err := ParseSbix(tbl, numGlyphs)
	if err != nil {
		t.Fatalf("ParseSbix: %v", err)
	}
	if !sbix.HasData() {
		t.Fatal("HasData() = false")
	}
	if len(sbix.Strikes) != 1 || sbix.Strikes[0].PPEM != 64 {
		t.Fatalf("strikes = %+v", sbix.Strikes)
	}
	g := sbix.GlyphBlob(0, 0)
	if g == nil {
		t.Fatal("GlyphBlob(0, 0) = nil")
	}
	if g.GraphicType != MakeTag('p', 'n', 'g', ' ') {
		t.Errorf("GraphicType = %v, want 'png '", g.GraphicType)
	}
	if g.XOffset != 3 || g.YOffset != -5 {
		t.Errorf("offsets = (%d, %d), want (3, -5)", g.XOffset, g.YOffset)
	}
	if string(g.Data) != "PGPGPGPG" {
		t.Errorf("Data = %q, want PGPGPGPG", g.Data)
	}
	// gid 1 has zero-length offset range: must return nil.
	if got := sbix.GlyphBlob(1, 0); got != nil {
		t.Errorf("GlyphBlob(1, 0) = %+v, want nil for zero-length entry", got)
	}
}

func appendBE16(b []byte, v uint16) []byte { return append(b, byte(v>>8), byte(v)) }
func appendBE32(b []byte, v uint32) []byte {
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func TestSbixChooseStrike(t *testing.T) {
	// chooseStrike is pure-arithmetic; we test it with a synthetic
	// strike list directly. The expected behavior matches HB at
	// OT/Color/sbix/sbix.hh:267-292: prefer smallest strike at least
	// as large as the request; if all are smaller, prefer the largest.
	s := &Sbix{Version: 1, Strikes: []SbixStrike{
		{PPEM: 32}, {PPEM: 64}, {PPEM: 96}, {PPEM: 256},
	}}
	cases := []struct {
		requested int
		wantPPEM  uint16
	}{
		{requested: 20, wantPPEM: 32},   // smallest at-least 20
		{requested: 32, wantPPEM: 32},   // exact match
		{requested: 50, wantPPEM: 64},   // smallest at-least 50
		{requested: 96, wantPPEM: 96},
		{requested: 200, wantPPEM: 256}, // smallest at-least 200
		{requested: 500, wantPPEM: 256}, // none big enough — pick largest
		{requested: 0, wantPPEM: 256},   // 0 means "largest"
	}
	for _, c := range cases {
		got := s.chooseStrike(c.requested)
		if got == nil {
			t.Errorf("chooseStrike(%d) = nil", c.requested)
			continue
		}
		if got.PPEM != c.wantPPEM {
			t.Errorf("chooseStrike(%d) = ppem %d, want %d", c.requested, got.PPEM, c.wantPPEM)
		}
	}
}

func TestCBLCParse(t *testing.T) {
	f := loadFontForColor(t, "NotoColorEmoji.subset.ttf")
	if !f.HasTable(TagCBLC) || !f.HasTable(TagCBDT) {
		t.Fatal("CBLC/CBDT tables missing from NotoColorEmoji.subset.ttf")
	}
	cblcData, _ := f.TableData(TagCBLC)
	cblc, err := ParseCBLC(cblcData)
	if err != nil {
		t.Fatalf("ParseCBLC: %v", err)
	}
	if !cblc.HasData() {
		t.Fatal("HasData() = false")
	}
	if len(cblc.strikes) != 1 {
		t.Errorf("strikes len = %d, want 1 (subset font has 1 strike)", len(cblc.strikes))
	}
	if got := cblc.strikes[0].PPEMY; got != 109 {
		t.Errorf("strike[0].PPEMY = %d, want 109", got)
	}
}

func TestCBDTGlyphPNG(t *testing.T) {
	// NotoColorEmoji.subset.ttf carries 6 glyphs, GIDs 1..5 covered by
	// CBDT (gid 0 = .notdef). Each one resolves to a PNG bitmap.
	f := loadFontForColor(t, "NotoColorEmoji.subset.ttf")
	cblcData, _ := f.TableData(TagCBLC)
	cbdtData, _ := f.TableData(TagCBDT)
	cblc, err := ParseCBLC(cblcData)
	if err != nil {
		t.Fatal(err)
	}

	pngCount := 0
	for gid := GlyphID(1); gid <= 5; gid++ {
		g := cblc.GlyphPNG(gid, 0, cbdtData)
		if g == nil {
			t.Logf("gid %d: no bitmap (acceptable if subset gap)", gid)
			continue
		}
		if !bytes.HasPrefix(g.PNG, pngSignature) {
			t.Errorf("gid %d: blob does not start with PNG signature; first 8 bytes: % x",
				gid, g.PNG[:min(8, len(g.PNG))])
			continue
		}
		if g.PPEM != 109 {
			t.Errorf("gid %d: PPEM = %d, want 109", gid, g.PPEM)
		}
		if g.Width == 0 || g.Height == 0 {
			t.Errorf("gid %d: zero dimensions (W=%d H=%d)", gid, g.Width, g.Height)
		}
		pngCount++
	}
	if pngCount == 0 {
		t.Error("no PNG bitmaps extracted from NotoColorEmoji subset — pipeline broken")
	}
}

func TestCBLCFormat3(t *testing.T) {
	// The index_format3 variant carries the same data as the default,
	// but the per-glyph offsets are uint16 instead of uint32. The
	// PNG bytes for any given glyph should be byte-identical.
	f1 := loadFontForColor(t, "NotoColorEmoji.subset.ttf")
	f3 := loadFontForColor(t, "NotoColorEmoji.subset.index_format3.ttf")

	cblc1Data, _ := f1.TableData(TagCBLC)
	cbdt1Data, _ := f1.TableData(TagCBDT)
	cblc3Data, _ := f3.TableData(TagCBLC)
	cbdt3Data, _ := f3.TableData(TagCBDT)

	c1, err := ParseCBLC(cblc1Data)
	if err != nil {
		t.Fatal(err)
	}
	c3, err := ParseCBLC(cblc3Data)
	if err != nil {
		t.Fatal(err)
	}

	for gid := GlyphID(1); gid <= 5; gid++ {
		g1 := c1.GlyphPNG(gid, 0, cbdt1Data)
		g3 := c3.GlyphPNG(gid, 0, cbdt3Data)
		if (g1 == nil) != (g3 == nil) {
			t.Errorf("gid %d: format1 nil=%v vs format3 nil=%v", gid, g1 == nil, g3 == nil)
			continue
		}
		if g1 == nil {
			continue
		}
		if !bytes.Equal(g1.PNG, g3.PNG) {
			t.Errorf("gid %d: PNG bytes differ between format1 and format3", gid)
		}
	}
}

func TestFontHasColorPNG(t *testing.T) {
	cases := []struct {
		font     string
		expected bool
	}{
		{"NotoColorEmoji.subset.ttf", true},
		{"sbix-extents.ttf", false}, // corrupt sbix table → graceful false
		{"Roboto-Regular.ttf", false},
		{"TwemojiMozilla.subset.ttf", false}, // COLR font, no PNGs
	}
	for _, c := range cases {
		f := loadFontForColor(t, c.font)
		if got := f.HasColorPNG(); got != c.expected {
			t.Errorf("%s: HasColorPNG() = %v, want %v", c.font, got, c.expected)
		}
	}
}

func TestFontGlyphColorPNG(t *testing.T) {
	f := loadFontForColor(t, "NotoColorEmoji.subset.ttf")
	g := f.GlyphColorPNG(1, 0)
	if g == nil {
		t.Fatal("GlyphColorPNG(1, 0) = nil")
	}
	if g.Source != TagCBDT {
		t.Errorf("Source = %v, want TagCBDT", g.Source)
	}
	if !bytes.HasPrefix(g.PNG, pngSignature) {
		t.Errorf("PNG does not start with PNG signature; first 8 bytes: % x", g.PNG[:min(8, len(g.PNG))])
	}
	if g.PPEM != 109 {
		t.Errorf("PPEM = %d, want 109", g.PPEM)
	}

	// Plain text font: must return nil.
	roboto := loadFontForColor(t, "Roboto-Regular.ttf")
	if got := roboto.GlyphColorPNG(1, 12); got != nil {
		t.Errorf("Roboto.GlyphColorPNG(1, 12) = %v, want nil", got)
	}
}
