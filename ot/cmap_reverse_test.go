package ot

import "testing"

func TestCollectReverseMappingIsDeterministic(t *testing.T) {
	f := &cmapFormat0{}
	f.glyphIDs[0x20] = 3 // space
	f.glyphIDs[0xA0] = 3 // no-break space, same glyph
	f.glyphIDs[0x41] = 5
	c := &Cmap{subtable: f}

	for i := 0; i < 100; i++ {
		rev := c.CollectReverseMapping()
		if got := rev[3]; got != 0x20 {
			t.Fatalf("run %d: glyph 3 maps to U+%04X, want U+0020", i, got)
		}
		if got := rev[5]; got != 0x41 {
			t.Fatalf("run %d: glyph 5 maps to U+%04X, want U+0041", i, got)
		}
	}
}
