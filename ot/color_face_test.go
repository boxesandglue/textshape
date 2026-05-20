package ot

import (
	"os"
	"testing"

	"github.com/boxesandglue/textshape/internal/testutil"
)

// These tests cover the Face-level color accessors in font.go and pin them
// against the same fonts the lower-level COLR/CPAL parser tests use. The
// goal is to verify that the Face wrappers match the HarfBuzz C-API
// contracts (hb_ot_color_*) — see each function's doc comment for the
// corresponding HB source location.

func loadFont(t *testing.T, fontName string) *Font {
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
	return font
}

func TestFontHasColorLayers(t *testing.T) {
	t.Run("twemoji", func(t *testing.T) {
		f := loadFont(t, "TwemojiMozilla.subset.ttf")
		if !f.HasColorLayers() {
			t.Error("HasColorLayers() = false, want true for COLRv0 font")
		}
		if !f.HasColorPalettes() {
			t.Error("HasColorPalettes() = false, want true for font with CPAL")
		}
	})
	t.Run("non-color font (Roboto)", func(t *testing.T) {
		// Plain text font: must not pretend to have color data. Mirrors HB
		// returning false on hb_face_t without COLR/CPAL tables.
		f := loadFont(t, "Roboto-Regular.ttf")
		if f.HasColorLayers() {
			t.Error("HasColorLayers() = true on Roboto, want false")
		}
		if f.HasColorPalettes() {
			t.Error("HasColorPalettes() = true on Roboto, want false")
		}
	})
}

func TestFontGlyphColorLayers(t *testing.T) {
	f := loadFont(t, "TwemojiMozilla.subset.ttf")

	// Walk the small subset's glyph range and confirm Face-level lookup
	// returns identical results to the low-level COLR parser path used in
	// TestCOLRTwemojiMozillaSubset.
	mappedGIDs := 0
	totalLayers := 0
	for gid := range 64 {
		layers := f.GlyphColorLayers(GlyphID(gid))
		if layers != nil {
			mappedGIDs++
			totalLayers += len(layers)
		}
	}
	if mappedGIDs != 2 {
		t.Errorf("mapped base glyphs = %d, want 2", mappedGIDs)
	}
	if totalLayers != 4 {
		t.Errorf("total layers = %d, want 4", totalLayers)
	}

	// Non-color font: every glyph must return nil.
	roboto := loadFont(t, "Roboto-Regular.ttf")
	if got := roboto.GlyphColorLayers(1); got != nil {
		t.Errorf("Roboto.GlyphColorLayers(1) = %v, want nil", got)
	}
}

func TestFontColorPaletteColors(t *testing.T) {
	f := loadFont(t, "TwemojiMozilla.subset.ttf")

	if got := f.NumColorPalettes(); got != 1 {
		t.Errorf("NumColorPalettes = %d, want 1", got)
	}
	colors := f.ColorPaletteColors(0)
	if len(colors) != 2 {
		t.Errorf("ColorPaletteColors(0) len = %d, want 2", len(colors))
	}

	// HB contract: out-of-range palette returns empty/nil — see
	// CPAL::get_palette_colors at OT/Color/CPAL/CPAL.hh:192-193 returning
	// an empty hb_array_t.
	if got := f.ColorPaletteColors(99); got != nil {
		t.Errorf("ColorPaletteColors(99) = %v, want nil", got)
	}

	// Roboto has no CPAL: must return nil, not an empty slice — matches
	// HB returning 0 from hb_ot_color_palette_get_colors when has_data is
	// false (hb-ot-color.cc:185-189).
	roboto := loadFont(t, "Roboto-Regular.ttf")
	if got := roboto.ColorPaletteColors(0); got != nil {
		t.Errorf("Roboto.ColorPaletteColors(0) = %v, want nil", got)
	}
}

func TestFontColorPaletteFlagsDefaultForV0(t *testing.T) {
	// CPAL v0 fonts must always report PaletteFlagDefault — HB's
	// CPALV1Tail::get_palette_flags falls back to DEFAULT when
	// paletteFlagsZ is null (OT/Color/CPAL/CPAL.hh:54).
	f := loadFont(t, "TwemojiMozilla.subset.ttf")
	if got := f.ColorPaletteFlags(0); got != PaletteFlagDefault {
		t.Errorf("ColorPaletteFlags(0) = %d, want PaletteFlagDefault", got)
	}
}
