package ot

import (
	"os"
	"testing"

	"github.com/boxesandglue/textshape/internal/testutil"
)

// HarfBuzz reference: the CPAL accessors validated here are the same ones
// HB exposes via hb_ot_color_palette_get_count / _get_colors / _get_flags
// (hb-ot-color.cc:85, :179, :147).
//
// Test fonts come from HarfBuzz's own test corpus
// (/Users/patrick/tmp/harfbuzz/test/api/fonts/), so the expected values
// below can be reproduced by linking against libharfbuzz and calling the
// public C API directly.

// chromacheck-colr.ttf is a minimal COLRv0/CPALv0 fixture: 1 base glyph,
// 1 layer, 1 palette × 1 color.
//
// CPAL header bytes (offset 8 into the table) decode as:
//   version=0  numColors=1  numPalettes=1  numColorRecords=1
//   colorRecordsZ -> offset 14 from CPAL table start
// The single color record bytes are 00 00 c8 ff i.e. BGRA
// (B=0, G=0, R=200, A=255) — dark red (#C80000). The font author chose a
// distinctive color so "color rendered correctly" is visually obvious.
func loadCPAL(t *testing.T, fontName string) *CPAL {
	t.Helper()
	path := testutil.FindTestFont(fontName)
	if path == "" {
		t.Skipf("%s not found", fontName)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", fontName, err)
	}
	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", fontName, err)
	}
	if !font.HasTable(TagCPAL) {
		t.Skipf("%s has no CPAL table", fontName)
	}
	cpalData, err := font.TableData(TagCPAL)
	if err != nil {
		t.Fatalf("CPAL data: %v", err)
	}
	cpal, err := ParseCPAL(cpalData)
	if err != nil {
		t.Fatalf("ParseCPAL: %v", err)
	}
	return cpal
}

func TestCPALChromaCheck(t *testing.T) {
	cpal := loadCPAL(t, "chromacheck-colr.ttf")

	if !cpal.HasData() {
		t.Fatal("expected HasData() == true")
	}
	if got := cpal.Version; got != 0 {
		t.Errorf("Version = %d, want 0", got)
	}
	if got := cpal.NumPalettes(); got != 1 {
		t.Errorf("NumPalettes = %d, want 1", got)
	}
	if got := cpal.NumColors(); got != 1 {
		t.Errorf("NumColors = %d, want 1", got)
	}

	colors := cpal.PaletteColors(0)
	if len(colors) != 1 {
		t.Fatalf("PaletteColors(0) len = %d, want 1", len(colors))
	}
	// chromacheck-colr ships a single dark-red color record (#C80000).
	// On-disk bytes 00 00 c8 ff are read in B,G,R,A order — see the
	// commentary above and OT/Color/CPAL/CPAL.hh:167.
	want := BGRAColor{Blue: 0, Green: 0, Red: 200, Alpha: 255}
	if colors[0] != want {
		t.Errorf("color[0] = %+v, want %+v", colors[0], want)
	}
}

func TestCPALTwemojiMozillaSubset(t *testing.T) {
	cpal := loadCPAL(t, "TwemojiMozilla.subset.ttf")

	if got := cpal.NumPalettes(); got != 1 {
		t.Errorf("NumPalettes = %d, want 1", got)
	}
	if got := cpal.NumColors(); got != 2 {
		t.Errorf("NumColors = %d, want 2", got)
	}

	colors := cpal.PaletteColors(0)
	if len(colors) != 2 {
		t.Fatalf("PaletteColors(0) len = %d, want 2", len(colors))
	}
	for i, c := range colors {
		if c.Alpha == 0 {
			t.Errorf("color[%d] has Alpha=0 (fully transparent — unexpected for emoji)", i)
		}
	}

	// Out-of-range palette index returns nil — matches HB returning an
	// empty hb_array_t<const BGRAColor> at OT/Color/CPAL/CPAL.hh:192-193.
	if got := cpal.PaletteColors(99); got != nil {
		t.Errorf("PaletteColors(99) = %v, want nil", got)
	}
}

func TestCPALInvalid(t *testing.T) {
	// Too short for header.
	if _, err := ParseCPAL([]byte{0, 0, 0, 1}); err == nil {
		t.Error("expected error for short data")
	}
}

func TestCPALPaletteFlagsDefaultV0(t *testing.T) {
	cpal := loadCPAL(t, "TwemojiMozilla.subset.ttf")
	// CPAL version 0 must always report PaletteFlagDefault, mirroring
	// HB's CPALV1Tail::get_palette_flags fallback at
	// OT/Color/CPAL/CPAL.hh:54 (returns DEFAULT when paletteFlagsZ is null).
	if got := cpal.PaletteFlags(0); got != PaletteFlagDefault {
		t.Errorf("PaletteFlags(0) = %d, want PaletteFlagDefault for v0 table", got)
	}
}
