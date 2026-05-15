package subset

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/boxesandglue/textshape/ot"
)

// TestSubsetDeterminismBaselinePDFStyle replicates the exact call pattern
// baseline-pdf uses: GIDs gathered from a map, sorted, AddGlyph, with
// FlagDropLayoutTables. This is the path that actually drifts in real
// production runs per the TODO at baseline-pdf/pdffont.go:453.
func TestSubsetDeterminismBaselinePDFStyle(t *testing.T) {
	cases := []struct {
		name   string
		font   string
		gidSet []ot.GlyphID
	}{
		// Simulate a large mixed-script document: a wide spread of GIDs
		// including composites (Latin Extended accented forms).
		{"Roboto-wide", "Roboto-Regular.ttf", wideGIDSet()},
		{"SourceSansPro-wide", "SourceSansPro-Regular.otf", wideGIDSet()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := findTestFont(tc.font)
			if path == "" {
				t.Skipf("%s not found", tc.font)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			const runs = 5
			hashes := make([]string, runs)
			sizes := make([]int, runs)
			outputs := make([][]byte, runs)

			for i := 0; i < runs; i++ {
				font, err := ot.ParseFont(data, 0)
				if err != nil {
					t.Fatal(err)
				}

				// Replicate baseline-pdf/pdffont.go:418-436 exactly.
				input := NewInput()
				input.Flags = FlagDropLayoutTables
				for _, gid := range tc.gidSet {
					input.AddGlyph(gid)
				}

				plan, err := CreatePlan(font, input)
				if err != nil {
					t.Fatal(err)
				}
				out, err := plan.Execute()
				if err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256(out)
				hashes[i] = hex.EncodeToString(sum[:8])
				sizes[i] = len(out)
				outputs[i] = out
			}

			t.Logf("hashes: %v", hashes)
			t.Logf("sizes:  %v", sizes)

			for i := 1; i < runs; i++ {
				if hashes[i] != hashes[0] {
					t.Errorf("run %d differs from run 0", i)
					diffOffsets(t, outputs[0], outputs[i])
				}
			}
		})
	}
}

func wideGIDSet() []ot.GlyphID {
	// Synthesise a wide GID range that the font likely has —
	// numerals, basic latin, latin-1 supplement, latin extended,
	// punctuation, common symbols. This mimics a slide deck or
	// long-form document.
	var gids []ot.GlyphID
	// Use a deterministic spread; not all will exist in every font.
	for i := 1; i <= 600; i++ {
		gids = append(gids, ot.GlyphID(i))
	}
	return gids
}

// TestSubsetDeterminism verifies that Execute() produces byte-identical output
// for identical inputs across runs. Tracks reproducible-build progress.
func TestSubsetDeterminism(t *testing.T) {
	cases := []struct {
		name string
		font string
		text string
	}{
		{"Roboto-short", "Roboto-Regular.ttf", "Hello, World!"},
		{"Roboto-medium", "Roboto-Regular.ttf",
			"The quick brown fox jumps over the lazy dog. " +
				"Sphinx of black quartz, judge my vow. " +
				"How vexingly quick daft zebras jump! " +
				"Pack my box with five dozen liquor jugs."},
		{"Roboto-large", "Roboto-Regular.ttf", largeText()},
		{"SourceSansPro-short", "SourceSansPro-Regular.otf", "Hello, World!"},
		{"SourceSansPro-large", "SourceSansPro-Regular.otf", largeText()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := findTestFont(tc.font)
			if path == "" {
				t.Skipf("%s not found", tc.font)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			const runs = 5
			hashes := make([]string, runs)
			sizes := make([]int, runs)
			outputs := make([][]byte, runs)

			for i := 0; i < runs; i++ {
				font, err := ot.ParseFont(data, 0)
				if err != nil {
					t.Fatal(err)
				}

				out, err := SubsetString(font, tc.text)
				if err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256(out)
				hashes[i] = hex.EncodeToString(sum[:8])
				sizes[i] = len(out)
				outputs[i] = out
			}

			t.Logf("hashes: %v", hashes)
			t.Logf("sizes:  %v", sizes)

			for i := 1; i < runs; i++ {
				if hashes[i] != hashes[0] {
					t.Errorf("run %d differs from run 0", i)
					diffOffsets(t, outputs[0], outputs[i])
				}
			}
		})
	}
}

func diffOffsets(t *testing.T, a, b []byte) {
	if len(a) != len(b) {
		t.Logf("length differs: %d vs %d (delta %d)", len(a), len(b), len(b)-len(a))
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	diffs := 0
	first := -1
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			if first == -1 {
				first = i
			}
			diffs++
			if diffs <= 10 {
				t.Logf("  offset 0x%x: %02x vs %02x", i, a[i], b[i])
			}
		}
	}
	t.Logf("total differing bytes (in common range): %d, first at 0x%x", diffs, first)
}

func largeText() string {
	// A larger text exercising more GSUB lookups (kerning pairs, ligatures, etc.)
	// Include common ligature contexts and accented characters.
	var buf bytes.Buffer
	corpus := []string{
		"The fjord-finder offices flew official fluffy flags.",
		"Affirmative — efficient fffj sequences trigger ligatures.",
		"Various accented forms: café, naïve, façade, jalapeño, smörgåsbord.",
		"Greek: αβγδε; Cyrillic: абвгде; Hebrew: שלום; Arabic: مرحبا (subset may skip RTL shaping).",
		"Numerics & symbols: 0123456789 — fi fl ff ffi ffl — &@#$%^*()[]{}<>?!.,;:'\"",
	}
	for i := 0; i < 20; i++ {
		for _, s := range corpus {
			buf.WriteString(s)
			buf.WriteString(" ")
			buf.WriteString(fmt.Sprintf("iteration-%d ", i))
		}
	}
	return buf.String()
}
