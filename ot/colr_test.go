package ot

import (
	"os"
	"testing"

	"github.com/boxesandglue/textshape/internal/testutil"
)

// HarfBuzz reference: the COLRv0 accessors validated here match
// hb_ot_color_has_layers and hb_ot_color_glyph_get_layers
// (hb-ot-color.cc:205 and :263). Test fonts come from HarfBuzz's own
// corpus (/Users/patrick/tmp/harfbuzz/test/api/fonts/); expected values can
// be reproduced by linking against libharfbuzz.

func loadCOLR(t *testing.T, fontName string) *COLR {
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
	if !font.HasTable(TagCOLR) {
		t.Skipf("%s has no COLR table", fontName)
	}
	colrData, err := font.TableData(TagCOLR)
	if err != nil {
		t.Fatalf("COLR data: %v", err)
	}
	colr, err := ParseCOLR(colrData)
	if err != nil {
		t.Fatalf("ParseCOLR: %v", err)
	}
	return colr
}

func TestCOLRChromaCheck(t *testing.T) {
	colr := loadCOLR(t, "chromacheck-colr.ttf")

	if !colr.HasData() {
		t.Fatal("HasData() = false, want true")
	}
	if !colr.HasV0Data() {
		t.Fatal("HasV0Data() = false, want true")
	}
	if colr.HasV1Data() {
		t.Error("HasV1Data() = true, want false for COLRv0 font")
	}
	if got := colr.Version; got != 0 {
		t.Errorf("Version = %d, want 0", got)
	}

	// chromacheck-colr maps exactly one base glyph (gid 1, the only
	// outlined glyph beyond .notdef) to one layer using palette color 0.
	// Verified by dumping the COLR header: numBase=1, numLayers=1.
	layers1 := colr.GlyphLayers(1)
	if len(layers1) != 1 {
		t.Fatalf("GlyphLayers(1) len = %d, want 1", len(layers1))
	}
	if layers1[0].ColorIndex != 0 {
		t.Errorf("layer[0].ColorIndex = %d, want 0", layers1[0].ColorIndex)
	}

	// Glyphs without a record return nil — matches HB's behavior at
	// OT/Color/COLR/COLR.hh:2110-2114 when bsearch returns Null.
	if got := colr.GlyphLayers(99); got != nil {
		t.Errorf("GlyphLayers(99) = %v, want nil for unmapped glyph", got)
	}
	if got := colr.GlyphLayers(0); got != nil {
		t.Errorf("GlyphLayers(0) = %v, want nil for .notdef", got)
	}
}

func TestCOLRTwemojiMozillaSubset(t *testing.T) {
	colr := loadCOLR(t, "TwemojiMozilla.subset.ttf")

	if !colr.HasV0Data() {
		t.Fatal("HasV0Data() = false, want true")
	}

	// TwemojiMozilla.subset header (verified by binary dump):
	// numBaseGlyphs=2, numLayers=4. Each base glyph therefore has on
	// average 2 layers; the actual per-glyph counts depend on which two
	// emoji are present in the subset, but the total must equal 4.
	total := 0
	mapped := 0
	for gid := range 64 {
		layers := colr.GlyphLayers(GlyphID(gid))
		if layers != nil {
			mapped++
			total += len(layers)
			for i, l := range layers {
				// CPAL has numColors=2 in this font, so legal
				// ColorIndex values are 0, 1, or ForegroundColorIndex.
				if l.ColorIndex != ForegroundColorIndex && l.ColorIndex >= 2 {
					t.Errorf("gid %d layer[%d].ColorIndex = %d, want < 2 or 0xFFFF",
						gid, i, l.ColorIndex)
				}
			}
		}
	}
	if mapped != 2 {
		t.Errorf("mapped base glyphs = %d, want 2", mapped)
	}
	if total != 4 {
		t.Errorf("total layers across mapped glyphs = %d, want 4 (matches numLayers header)", total)
	}
}

func TestCOLRInvalid(t *testing.T) {
	if _, err := ParseCOLR([]byte{0, 0, 0, 1, 0, 0, 0, 0}); err == nil {
		t.Error("expected error for truncated header")
	}
}

// TestCOLRBaseGlyphRecordsSortedAssumption documents (and pins) the
// SortedUnsizedArrayOf invariant from OT/Color/COLR/COLR.hh:2771 — our
// GlyphLayers binary search relies on it.
func TestCOLRBaseGlyphRecordsSortedAssumption(t *testing.T) {
	for _, font := range []string{"chromacheck-colr.ttf", "TwemojiMozilla.subset.ttf"} {
		path := testutil.FindTestFont(font)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := ParseFont(data, 0)
		if err != nil {
			t.Fatal(err)
		}
		colrData, err := f.TableData(TagCOLR)
		if err != nil {
			t.Fatal(err)
		}
		colr, err := ParseCOLR(colrData)
		if err != nil {
			t.Fatal(err)
		}
		for i := 1; i < len(colr.baseGlyphRecords); i++ {
			if colr.baseGlyphRecords[i].glyphID <= colr.baseGlyphRecords[i-1].glyphID {
				t.Errorf("%s: baseGlyphRecords not sorted at i=%d (gids %d, %d)",
					font, i,
					colr.baseGlyphRecords[i-1].glyphID,
					colr.baseGlyphRecords[i].glyphID)
			}
		}
	}
}
