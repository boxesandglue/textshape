package ot

import (
	"os"
	"testing"
)

func TestBufferBasic(t *testing.T) {
	buf := NewBuffer()

	if buf.Len() != 0 {
		t.Errorf("Empty buffer should have length 0, got %d", buf.Len())
	}

	buf.AddString("Hello")
	if buf.Len() != 5 {
		t.Errorf("Buffer with 'Hello' should have length 5, got %d", buf.Len())
	}

	// Check codepoints
	expected := []Codepoint{'H', 'e', 'l', 'l', 'o'}
	for i, cp := range expected {
		if buf.Info[i].Codepoint != cp {
			t.Errorf("Codepoint[%d] = %d, want %d", i, buf.Info[i].Codepoint, cp)
		}
		if buf.Info[i].Cluster != i {
			t.Errorf("Cluster[%d] = %d, want %d", i, buf.Info[i].Cluster, i)
		}
	}

	buf.Clear()
	if buf.Len() != 0 {
		t.Errorf("Cleared buffer should have length 0, got %d", buf.Len())
	}
}

func TestBufferCodepoints(t *testing.T) {
	buf := NewBuffer()
	buf.AddCodepoints([]Codepoint{0x0041, 0x0042, 0x0043}) // ABC

	if buf.Len() != 3 {
		t.Errorf("Buffer length = %d, want 3", buf.Len())
	}

	if buf.Info[0].Codepoint != 0x0041 {
		t.Errorf("Codepoint[0] = %d, want 0x0041", buf.Info[0].Codepoint)
	}
}

func TestBufferDirection(t *testing.T) {
	buf := NewBuffer()

	// Default direction is 0 (unset), not LTR
	// Direction is set by GuessSegmentProperties or explicitly
	if buf.Direction != 0 {
		t.Error("Default direction should be unset (0)")
	}

	buf.SetDirection(DirectionRTL)
	if buf.Direction != DirectionRTL {
		t.Error("Direction should be RTL after SetDirection")
	}
}

func TestGuessDirection(t *testing.T) {
	tests := []struct {
		text     string
		expected Direction
	}{
		{"Hello", DirectionLTR},
		{"مرحبا", DirectionRTL},       // Arabic
		{"שלום", DirectionRTL},        // Hebrew
		{"Hello مرحبا", DirectionLTR}, // Mixed, first letter wins
		{"123", DirectionLTR},         // Numbers default to LTR
		{"", DirectionLTR},            // Empty defaults to LTR
	}

	for _, tt := range tests {
		got := GuessDirection(tt.text)
		if got != tt.expected {
			t.Errorf("GuessDirection(%q) = %d, want %d", tt.text, got, tt.expected)
		}
	}
}

func TestShaperWithRealFont(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	shaper, err := NewShaper(font)
	if err != nil {
		t.Fatalf("Failed to create shaper: %v", err)
	}

	// Test basic shaping
	buf := NewBuffer()
	buf.AddString("Hello")
	shaper.Shape(buf, nil) // Use default features

	t.Logf("Shaped 'Hello': %d glyphs", buf.Len())
	for i, info := range buf.Info {
		t.Logf("  [%d] glyph=%d cluster=%d class=%d",
			i, info.GlyphID, info.Cluster, info.GlyphClass)
	}

	// Verify we got glyphs (not .notdef)
	for i, info := range buf.Info {
		if info.GlyphID == 0 {
			t.Errorf("Glyph[%d] is .notdef (0), expected valid glyph", i)
		}
	}
}

func TestShaperLigature(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	shaper, err := NewShaper(font)
	if err != nil {
		t.Fatalf("Failed to create shaper: %v", err)
	}

	// Test 'fi' ligature
	buf := NewBuffer()
	buf.AddString("fi")

	glyphsBefore := make([]GlyphID, len(buf.Info))
	copy(glyphsBefore, buf.GlyphIDs())

	shaper.Shape(buf, nil) // Use default features

	t.Logf("Shaped 'fi': %d glyphs (was 2 codepoints)", buf.Len())
	for i, info := range buf.Info {
		t.Logf("  [%d] glyph=%d cluster=%d class=%d",
			i, info.GlyphID, info.Cluster, info.GlyphClass)
	}

	// Note: Roboto might form an 'fi' ligature depending on features
	// If ligature formed, we'd have 1 glyph instead of 2
	if buf.Len() == 1 {
		t.Logf("'fi' formed ligature")
		if shaper.HasGDEF() && shaper.GDEF().HasGlyphClasses() {
			if buf.Info[0].GlyphClass == GlyphClassLigature {
				t.Logf("Ligature glyph correctly classified as Ligature")
			}
		}
	}
}

func TestShaperKerning(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	shaper, err := NewShaper(font)
	if err != nil {
		t.Fatalf("Failed to create shaper: %v", err)
	}

	// Test kerning pairs
	buf := NewBuffer()
	buf.AddString("AV")
	shaper.Shape(buf, nil) // Use default features

	t.Logf("Shaped 'AV':")
	for i, info := range buf.Info {
		t.Logf("  [%d] glyph=%d pos=(%d, %d, %d, %d)",
			i, info.GlyphID,
			buf.Pos[i].XAdvance, buf.Pos[i].YAdvance,
			buf.Pos[i].XOffset, buf.Pos[i].YOffset)
	}

	// AV typically has negative kerning
	if buf.Pos[0].XAdvance < 0 {
		t.Logf("'AV' has negative kerning: %d", buf.Pos[0].XAdvance)
	}
}

func TestShaperShapeString(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	shaper, err := NewShaper(font)
	if err != nil {
		t.Fatalf("Failed to create shaper: %v", err)
	}

	// Test convenience method
	glyphs, positions := shaper.ShapeString("Test")

	if len(glyphs) != 4 {
		t.Errorf("ShapeString('Test') returned %d glyphs, want 4", len(glyphs))
	}

	if len(positions) != len(glyphs) {
		t.Errorf("positions length %d != glyphs length %d", len(positions), len(glyphs))
	}

	t.Logf("ShapeString('Test'): %d glyphs", len(glyphs))
	for i, g := range glyphs {
		t.Logf("  [%d] glyph=%d xAdv=%d", i, g, positions[i].XAdvance)
	}
}

func TestShaperWithFeatures(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	shaper, err := NewShaper(font)
	if err != nil {
		t.Fatalf("Failed to create shaper: %v", err)
	}

	// Shape with liga feature enabled
	buf := NewBuffer()
	buf.AddString("fi")
	shaper.Shape(buf, []Feature{
		NewFeatureOn(TagLiga),
		NewFeatureOn(TagKern),
	})

	t.Logf("Shaped 'fi' with liga+kern: %d glyphs", buf.Len())

	// Shape without liga (disabled)
	buf2 := NewBuffer()
	buf2.AddString("fi")
	shaper.Shape(buf2, []Feature{
		NewFeatureOff(TagLiga), // Explicitly disable liga
		NewFeatureOn(TagKern),
	})

	t.Logf("Shaped 'fi' with -liga+kern: %d glyphs", buf2.Len())

	// Without liga, should have 2 glyphs
	if buf2.Len() != 2 {
		t.Logf("Note: Expected 2 glyphs without liga, got %d", buf2.Len())
	}
}

func TestFeatureFromString(t *testing.T) {
	tests := []struct {
		input   string
		wantTag Tag
		wantVal uint32
		wantOK  bool
	}{
		{"kern", tagFromString("kern"), 1, true},
		{"kern=1", tagFromString("kern"), 1, true},
		{"kern=0", tagFromString("kern"), 0, true},
		{"-kern", tagFromString("kern"), 0, true},
		{"+kern", tagFromString("kern"), 1, true},
		{"aalt=2", tagFromString("aalt"), 2, true},
		{"liga", tagFromString("liga"), 1, true},
		{"", Tag(0), 0, false},
	}

	for _, tt := range tests {
		f, ok := FeatureFromString(tt.input)
		if ok != tt.wantOK {
			t.Errorf("FeatureFromString(%q) ok=%v, want %v", tt.input, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if f.Tag != tt.wantTag {
			t.Errorf("FeatureFromString(%q).Tag = %v, want %v", tt.input, f.Tag, tt.wantTag)
		}
		if f.Value != tt.wantVal {
			t.Errorf("FeatureFromString(%q).Value = %d, want %d", tt.input, f.Value, tt.wantVal)
		}
	}
}

func TestConvenienceShapeFunction(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Use convenience function (should cache shaper internally)
	buf := NewBuffer()
	buf.AddString("Hello")
	buf.GuessSegmentProperties()

	err = Shape(font, buf, nil)
	if err != nil {
		t.Fatalf("Shape failed: %v", err)
	}

	t.Logf("Shaped 'Hello' with convenience function: %d glyphs", buf.Len())

	// Shape again (should use cached shaper)
	buf2 := NewBuffer()
	buf2.AddString("World")
	buf2.GuessSegmentProperties()

	err = Shape(font, buf2, nil)
	if err != nil {
		t.Fatalf("Shape failed: %v", err)
	}

	t.Logf("Shaped 'World' with convenience function: %d glyphs", buf2.Len())
}

func TestShaperMultipleWords(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	shaper, err := NewShaper(font)
	if err != nil {
		t.Fatalf("Failed to create shaper: %v", err)
	}

	text := "The quick brown fox"
	glyphs, positions := shaper.ShapeString(text)

	t.Logf("Shaped %q: %d glyphs", text, len(glyphs))

	// Calculate total advance
	totalAdvance := int16(0)
	for _, p := range positions {
		totalAdvance += p.XAdvance
	}
	t.Logf("Total advance: %d units", totalAdvance)

	// Verify all glyphs are valid
	for i, g := range glyphs {
		if g == 0 && text[i] != ' ' { // Space might map to glyph 0 in some fonts
			t.Errorf("Glyph[%d] for '%c' is .notdef", i, text[i])
		}
	}
}

func TestShaperHasTables(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	shaper, err := NewShaper(font)
	if err != nil {
		t.Fatalf("Failed to create shaper: %v", err)
	}

	t.Logf("HasGSUB: %v", shaper.HasGSUB())
	t.Logf("HasGPOS: %v", shaper.HasGPOS())
	t.Logf("HasGDEF: %v", shaper.HasGDEF())

	if !shaper.HasGSUB() {
		t.Error("Expected GSUB to be present")
	}
	if !shaper.HasGPOS() {
		t.Error("Expected GPOS to be present")
	}
	if !shaper.HasGDEF() {
		t.Error("Expected GDEF to be present")
	}
}
