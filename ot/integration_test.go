package ot

import (
	"os"
	"testing"

	"github.com/boxesandglue/textshape/internal/testutil"
)

// findTestFont locates a test font file.
func findTestFont(name string) string {
	return testutil.FindTestFont(name)
}

func TestRealFontGDEF(t *testing.T) {
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

	// Check for GDEF table
	if !font.HasTable(TagGDEF) {
		t.Skip("Font has no GDEF table")
	}

	gdefData, err := font.TableData(TagGDEF)
	if err != nil {
		t.Fatalf("Failed to get GDEF table: %v", err)
	}

	gdef, err := ParseGDEF(gdefData)
	if err != nil {
		t.Fatalf("Failed to parse GDEF: %v", err)
	}

	major, minor := gdef.Version()
	t.Logf("GDEF version: %d.%d", major, minor)
	t.Logf("Has glyph classes: %v", gdef.HasGlyphClasses())
	t.Logf("Has attach list: %v", gdef.HasAttachList())
	t.Logf("Has lig caret list: %v", gdef.HasLigCaretList())
	t.Logf("Has mark attach classes: %v", gdef.HasMarkAttachClasses())
	t.Logf("Has mark glyph sets: %v", gdef.HasMarkGlyphSets())

	// Test some glyph classes if available
	if gdef.HasGlyphClasses() {
		numGlyphs := font.NumGlyphs()
		t.Logf("Num glyphs: %d", numGlyphs)

		// Count glyphs by class
		counts := make(map[int]int)
		for g := 0; g < numGlyphs; g++ {
			class := gdef.GetGlyphClass(GlyphID(g))
			counts[class]++
		}

		t.Logf("Glyph classes: Unclassified=%d, Base=%d, Ligature=%d, Mark=%d, Component=%d",
			counts[GlyphClassUnclassified],
			counts[GlyphClassBase],
			counts[GlyphClassLigature],
			counts[GlyphClassMark],
			counts[GlyphClassComponent])
	}
}

func TestRealFontGSUB(t *testing.T) {
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

	// Parse GDEF if available
	var gdef *GDEF
	if font.HasTable(TagGDEF) {
		gdefData, err := font.TableData(TagGDEF)
		if err == nil {
			gdef, _ = ParseGDEF(gdefData)
		}
	}

	// Parse GSUB
	if !font.HasTable(TagGSUB) {
		t.Skip("Font has no GSUB table")
	}

	gsubData, err := font.TableData(TagGSUB)
	if err != nil {
		t.Fatalf("Failed to get GSUB table: %v", err)
	}

	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("Failed to parse GSUB: %v", err)
	}

	t.Logf("GSUB: %d lookups", gsub.NumLookups())

	// List lookup types
	lookupTypes := make(map[uint16]int)
	for i := 0; i < gsub.NumLookups(); i++ {
		lookup := gsub.GetLookup(i)
		if lookup != nil {
			lookupTypes[lookup.Type]++
			t.Logf("  Lookup %d: Type=%d, Flag=0x%04x", i, lookup.Type, lookup.Flag)
		}
	}

	// Parse feature list
	featureList, err := gsub.ParseFeatureList()
	if err == nil {
		t.Logf("GSUB Features: %d", featureList.Count())
	}

	// Test ligature substitution with GDEF
	// Get glyph IDs for 'f' and 'i' from cmap
	if font.HasTable(TagCmap) {
		cmapData, err := font.TableData(TagCmap)
		if err == nil {
			cmap, err := ParseCmap(cmapData)
			if err == nil {
				fGlyph, _ := cmap.Lookup('f')
				iGlyph, _ := cmap.Lookup('i')

				if fGlyph != 0 && iGlyph != 0 {
					glyphs := []GlyphID{fGlyph, iGlyph}
					t.Logf("Testing 'fi' ligature: glyphs %v", glyphs)

					// Apply liga feature with GDEF
					result := gsub.ApplyFeatureWithGDEF(TagLiga, glyphs, gdef, nil)
					t.Logf("After 'liga': glyphs %v", result)

					if len(result) == 1 {
						t.Logf("'fi' formed ligature: glyph %d", result[0])
						if gdef != nil && gdef.HasGlyphClasses() {
							class := gdef.GetGlyphClass(result[0])
							t.Logf("Ligature glyph class: %d (expect %d=Ligature)", class, GlyphClassLigature)
						}
					}
				}
			}
		}
	}
}

func TestRealFontGPOS(t *testing.T) {
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

	// Parse GDEF if available
	var gdef *GDEF
	if font.HasTable(TagGDEF) {
		gdefData, err := font.TableData(TagGDEF)
		if err == nil {
			gdef, _ = ParseGDEF(gdefData)
		}
	}

	// Parse GPOS
	if !font.HasTable(TagGPOS) {
		t.Skip("Font has no GPOS table")
	}

	gposData, err := font.TableData(TagGPOS)
	if err != nil {
		t.Fatalf("Failed to get GPOS table: %v", err)
	}

	gpos, err := ParseGPOS(gposData)
	if err != nil {
		t.Fatalf("Failed to parse GPOS: %v", err)
	}

	t.Logf("GPOS: %d lookups", gpos.NumLookups())

	// List lookup types
	for i := 0; i < gpos.NumLookups(); i++ {
		lookup := gpos.GetLookup(i)
		if lookup != nil {
			t.Logf("  Lookup %d: Type=%d, Flag=0x%04x", i, lookup.Type, lookup.Flag)
		}
	}

	// Test kerning with GDEF
	if font.HasTable(TagCmap) {
		cmapData, err := font.TableData(TagCmap)
		if err == nil {
			cmap, err := ParseCmap(cmapData)
			if err == nil {
				// Test common kerning pairs
				pairs := [][2]rune{
					{'A', 'V'},
					{'T', 'o'},
					{'V', 'A'},
					{'W', 'a'},
					{'Y', 'o'},
				}

				for _, pair := range pairs {
					g1, _ := cmap.Lookup(Codepoint(pair[0]))
					g2, _ := cmap.Lookup(Codepoint(pair[1]))

					if g1 != 0 && g2 != 0 {
						glyphs := []GlyphID{g1, g2}
						positions := gpos.ApplyKerningWithGDEF(glyphs, gdef)

						if positions[0].XAdvance != 0 {
							t.Logf("Kerning '%c%c' (glyphs %d,%d): XAdvance=%d",
								pair[0], pair[1], g1, g2, positions[0].XAdvance)
						}
					}
				}
			}
		}
	}
}

func TestRealFontGDEFIntegration(t *testing.T) {
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

	// Parse all tables
	var gdef *GDEF
	var gsub *GSUB
	var gpos *GPOS
	var cmap *Cmap

	if font.HasTable(TagGDEF) {
		gdefData, _ := font.TableData(TagGDEF)
		gdef, _ = ParseGDEF(gdefData)
	}
	if font.HasTable(TagGSUB) {
		gsubData, _ := font.TableData(TagGSUB)
		gsub, _ = ParseGSUB(gsubData)
	}
	if font.HasTable(TagGPOS) {
		gposData, _ := font.TableData(TagGPOS)
		gpos, _ = ParseGPOS(gposData)
	}
	if font.HasTable(TagCmap) {
		cmapData, _ := font.TableData(TagCmap)
		cmap, _ = ParseCmap(cmapData)
	}

	if gdef == nil {
		t.Skip("No GDEF table")
	}
	if gsub == nil {
		t.Skip("No GSUB table")
	}
	if cmap == nil {
		t.Skip("No cmap table")
	}

	t.Logf("Testing GDEF integration with GSUB/GPOS")

	// Find a lookup with IgnoreMarks flag
	foundIgnoreMarks := false
	for i := 0; i < gsub.NumLookups(); i++ {
		lookup := gsub.GetLookup(i)
		if lookup != nil && lookup.Flag&LookupFlagIgnoreMarks != 0 {
			t.Logf("GSUB Lookup %d has IgnoreMarks flag (0x%04x)", i, lookup.Flag)
			foundIgnoreMarks = true
		}
	}

	if gpos != nil {
		for i := 0; i < gpos.NumLookups(); i++ {
			lookup := gpos.GetLookup(i)
			if lookup != nil && lookup.Flag&LookupFlagIgnoreMarks != 0 {
				t.Logf("GPOS Lookup %d has IgnoreMarks flag (0x%04x)", i, lookup.Flag)
				foundIgnoreMarks = true
			}
		}
	}

	if !foundIgnoreMarks {
		t.Log("No lookups with IgnoreMarks flag found")
	}

	// Test that mark glyphs exist and are classified correctly
	if gdef.HasGlyphClasses() {
		markCount := 0
		numGlyphs := font.NumGlyphs()
		for g := 0; g < numGlyphs; g++ {
			if gdef.GetGlyphClass(GlyphID(g)) == GlyphClassMark {
				markCount++
			}
		}
		t.Logf("Font has %d mark glyphs", markCount)

		// If we have marks, test that IgnoreMarks actually skips them
		if markCount > 0 {
			// Find a mark glyph
			var markGlyph GlyphID
			for g := 0; g < numGlyphs; g++ {
				if gdef.GetGlyphClass(GlyphID(g)) == GlyphClassMark {
					markGlyph = GlyphID(g)
					break
				}
			}

			// Test shouldSkipGlyph
			if shouldSkipGlyph(markGlyph, LookupFlagIgnoreMarks, gdef, -1) {
				t.Logf("shouldSkipGlyph correctly skips mark glyph %d with IgnoreMarks", markGlyph)
			} else {
				t.Errorf("shouldSkipGlyph should skip mark glyph %d with IgnoreMarks", markGlyph)
			}

			if !shouldSkipGlyph(markGlyph, 0, gdef, -1) {
				t.Logf("shouldSkipGlyph correctly does NOT skip mark glyph %d without flags", markGlyph)
			} else {
				t.Errorf("shouldSkipGlyph should NOT skip mark glyph %d without flags", markGlyph)
			}
		}
	}
}

func TestArabicFont(t *testing.T) {
	// Try to find an Arabic font for more complex testing
	fontPath := findTestFont("AnekBangla-subset.ttf")
	if fontPath == "" {
		t.Skip("AnekBangla-subset.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Parse GDEF
	if !font.HasTable(TagGDEF) {
		t.Skip("Font has no GDEF table")
	}

	gdefData, err := font.TableData(TagGDEF)
	if err != nil {
		t.Fatalf("Failed to get GDEF table: %v", err)
	}

	gdef, err := ParseGDEF(gdefData)
	if err != nil {
		t.Fatalf("Failed to parse GDEF: %v", err)
	}

	major, minor := gdef.Version()
	t.Logf("GDEF version: %d.%d", major, minor)

	if gdef.HasGlyphClasses() {
		numGlyphs := font.NumGlyphs()
		counts := make(map[int]int)
		for g := 0; g < numGlyphs; g++ {
			class := gdef.GetGlyphClass(GlyphID(g))
			counts[class]++
		}
		t.Logf("Glyph classes: Unclassified=%d, Base=%d, Ligature=%d, Mark=%d, Component=%d",
			counts[GlyphClassUnclassified],
			counts[GlyphClassBase],
			counts[GlyphClassLigature],
			counts[GlyphClassMark],
			counts[GlyphClassComponent])
	}

	if gdef.HasMarkAttachClasses() {
		t.Log("Font has mark attachment classes")
	}

	if gdef.HasMarkGlyphSets() {
		t.Logf("Font has %d mark glyph sets", gdef.MarkGlyphSetCount())
	}
}

// TestHebrewDiacriticsDebug debugs the Hebrew diacritics test case line 18.
func TestHebrewDiacriticsDebug(t *testing.T) {
	// Load the Hebrew test font
	fontPath := "../harfbuzz-tests/fonts/b895f8ff06493cc893ec44de380690ca0074edfa.ttf"
	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Skip("Hebrew font not found:", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	shaper, err := NewShaper(font)
	if err != nil {
		t.Fatalf("Failed to create shaper: %v", err)
	}

	// Input: U+05D9,U+05B0,U+05E8,U+05D5,U+05BC,U+05E9,U+05C1,U+05B8,U+05DC,U+05B7,U+05B4,U+05DD
	// yod, sheva, resh, vav, dagesh, shin, shin-dot, qamats, lamed, patah, hiriq, finalmem
	input := []Codepoint{0x05D9, 0x05B0, 0x05E8, 0x05D5, 0x05BC, 0x05E9, 0x05C1, 0x05B8, 0x05DC, 0x05B7, 0x05B4, 0x05DD}

	buf := NewBuffer()
	buf.AddCodepoints(input)
	buf.Direction = DirectionRTL
	buf.Script = MakeTag('H', 'e', 'b', 'r')

	// Print GPOS lookup info
	if shaper.gpos != nil {
		t.Logf("GPOS has %d lookups", shaper.gpos.NumLookups())
		for i := 0; i < shaper.gpos.NumLookups(); i++ {
			lookup := shaper.gpos.GetLookup(i)
			if lookup != nil {
				if i == 31 || i == 32 {
					t.Logf("  Lookup %d: Type=%d Flag=0x%04X MarkFilter=%d", i, lookup.Type, lookup.Flag, lookup.MarkFilter)
				} else {
					t.Logf("  Lookup %d: Type=%d (1=Single, 2=Pair, 3=Cursive, 4=MarkBase, 5=MarkLig, 6=MarkMark)", i, lookup.Type)
				}
			}
		}
	}

	// Shape
	shaper.Shape(buf, nil)

	t.Logf("After shaping: %d glyphs", len(buf.Info))
	for i, info := range buf.Info {
		pos := buf.Pos[i]
		t.Logf("  [%d] gid=%d cluster=%d class=%d xoff=%d yoff=%d xadv=%d yadv=%d attach=%d",
			i, info.GlyphID, info.Cluster, info.GlyphClass,
			pos.XOffset, pos.YOffset, pos.XAdvance, pos.YAdvance, pos.AttachChain)
	}

	// Expected: hiriq (glyph at some index) should have xoffset -97
	// We're looking for the hiriq glyph and checking its offset
	for i, info := range buf.Info {
		// Find hiriq by checking if it's attached to lamed (cluster 8)
		if info.Cluster == 8 && buf.Pos[i].XAdvance == 0 {
			pos := buf.Pos[i]
			t.Logf("Mark at cluster 8: gid=%d xoffset=%d (expected: hiriq=-97, patah=499)",
				info.GlyphID, pos.XOffset)
		}
	}
}
