package harfbuzz_tests

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/boxesandglue/textshape/ot"
)

// TestCase represents a single HarfBuzz shape test
type TestCase struct {
	FontPath     string
	FontHash     string // Expected SHA1 hash of font file (empty = no check)
	Options      TestOptions
	Input        []rune
	Expected     []ExpectedGlyph
	SourceFile   string
	SourceLine   int
	OriginalLine string
}

// TestOptions holds parsed test options
type TestOptions struct {
	Shaper             string // "ot", "fallback", etc.
	ClusterLevel       int    // 0, 1, 2, 3
	FaceIndex          int    // For TTC/DFONT collections
	NoPositions        bool
	NoClusters         bool
	Variations         []ot.Variation
	Features           []ot.Feature
	Direction          ot.Direction
	Script             ot.Tag   // Script tag (e.g., "Deva", "Latn")
	Language           ot.Tag   // Language tag (OpenType, e.g., "ZHS ", "ZHT ")
	LanguageCandidates []ot.Tag // Multiple language tag candidates in priority order
	NoGlyphNames       bool     // Use GID numbers instead of glyph names
	BOT                bool     // Beginning of text flag
	EOT                bool     // End of text flag
	UnicodesBefore     []rune   // Pre-context codepoints (--unicodes-before)
	UnicodesAfter      []rune   // Post-context codepoints (--unicodes-after)
	NotFoundVSGlyph    int      // --not-found-variation-selector-glyph=N (-1 = not set)
	FontSize           int      // --font-size=N (0 = use upem, i.e. 1:1 scaling)
	NED                bool     // --ned: No Extra Data (no clusters, no advances; positions are cumulative)
	FontBold           float64  // --font-bold=V: synthetic bold (embolden_in_place=false)
	FontSlant          float64  // --font-slant=V: synthetic slant (no effect on positions)
}

// ExpectedGlyph represents expected output for one glyph
type ExpectedGlyph struct {
	Name         string
	Cluster      int
	XOffset      int16
	YOffset      int16
	XAdvance     int16
	YAdvance     int16
	HasPositions bool // true if positions were explicitly specified in test file
	HasOffsets   bool // true if offsets (@x,y) were explicitly specified
}

// fontCache caches loaded fonts
var fontCache = make(map[string]*ot.Font)
var shaperCache = make(map[string]*ot.Shaper)

// resolveSystemFontPath tries fallback paths for macOS system fonts that moved
// between OS versions (e.g. /Library/Fonts → /System/Library/Fonts/Supplemental,
// .dfont → .ttc, renamed files).
func resolveSystemFontPath(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	// /Library/Fonts/X → /System/Library/Fonts/Supplemental/X
	if strings.HasPrefix(path, "/Library/Fonts/") {
		alt := "/System/Library/Fonts/Supplemental/" + path[len("/Library/Fonts/"):]
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	// .dfont → .ttc (macOS modernized font formats)
	if strings.HasSuffix(path, ".dfont") {
		alt := strings.TrimSuffix(path, ".dfont") + ".ttc"
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	// Try Supplemental subdirectory
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if dir == "/System/Library/Fonts" {
		alt := filepath.Join(dir, "Supplemental", base)
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	// Hiragino fonts: "ヒラギノ明朝 ProN W3.ttc" → "ヒラギノ明朝 ProN.ttc" (weight suffix removed)
	if strings.Contains(base, " W") && strings.HasSuffix(base, ".ttc") {
		// Strip " W<digit>" before .ttc
		noExt := strings.TrimSuffix(base, ".ttc")
		if idx := strings.LastIndex(noExt, " W"); idx > 0 {
			alt := filepath.Join(dir, noExt[:idx]+".ttc")
			if _, err := os.Stat(alt); err == nil {
				return alt
			}
		}
	}
	return path
}

// getFont loads a font from cache or disk
func getFont(path string, faceIndex int) (*ot.Font, error) {
	cacheKey := fmt.Sprintf("%s@%d", path, faceIndex)
	if font, ok := fontCache[cacheKey]; ok {
		return font, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	font, err := ot.ParseFont(data, faceIndex)
	if err != nil {
		return nil, err
	}

	fontCache[cacheKey] = font
	return font, nil
}

// getShaper gets a shaper for a font (without variations)
func getShaper(path string, faceIndex int) (*ot.Shaper, error) {
	cacheKey := fmt.Sprintf("%s@%d", path, faceIndex)
	if shaper, ok := shaperCache[cacheKey]; ok {
		return shaper, nil
	}

	font, err := getFont(path, faceIndex)
	if err != nil {
		return nil, err
	}

	shaper, err := ot.NewShaper(font)
	if err != nil {
		return nil, err
	}

	shaperCache[cacheKey] = shaper
	return shaper, nil
}

// parseUnicodeInput parses "U+0041,U+0042" or "0041,0042" into runes
func parseUnicodeInput(s string) ([]rune, error) {
	if s == "" {
		return nil, nil
	}

	parts := strings.Split(s, ",")
	runes := make([]rune, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)

		// Support both "U+XXXX" and bare "XXXX" hex formats
		hexStr := p
		if strings.HasPrefix(p, "U+") || strings.HasPrefix(p, "u+") {
			hexStr = p[2:]
		}

		val, err := strconv.ParseInt(hexStr, 16, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid unicode value: %s", p)
		}
		runes = append(runes, rune(val))
	}

	return runes, nil
}

// parseUnicodesOption parses --unicodes-before/--unicodes-after values.
// Accepts "U+0643,U+0650" or "0627" format.
func parseUnicodesOption(s string) []rune {
	runes, _ := parseUnicodeInput(s)
	return runes
}

// parseExpectedOutput parses HarfBuzz output format
// Supported formats:
//   - Name                              (no-clusters, no-positions)
//   - Name=cluster                      (no-positions)
//   - Name=cluster+xadvance
//   - Name=cluster@xoff,yoff+xadvance
//   - Name=cluster@xoff,yoff+xadvance,yadvance
//   - Name=cluster+xadvance<l,t,w,h>    (extents, ignored)
//   - Name=cluster+xadvance#flags       (flags, ignored)
func parseExpectedOutput(s string) ([]ExpectedGlyph, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("expected output must be wrapped in []")
	}

	s = s[1 : len(s)-1]
	if s == "" {
		return nil, nil
	}

	parts := strings.Split(s, "|")
	glyphs := make([]ExpectedGlyph, 0, len(parts))

	// Patterns for different output formats:
	// Full: Name=cluster[@xoff,yoff]+xadvance[,yadvance][<extents>][#flags]
	reFull := regexp.MustCompile(`^([^=]+)=(\d+)(?:@(-?\d+),(-?\d+))?\+(-?\d+)(?:,(-?\d+))?(?:<[^>]+>)?(?:#\d+)?$`)
	// No positions: Name=cluster
	reNoPos := regexp.MustCompile(`^([^=]+)=(\d+)$`)
	// NED format (--ned): Name[@cumX,cumY] — also covers name-only format (Name without @)
	reNED := regexp.MustCompile(`^([^=@|]+)(?:@(-?\d+),(-?\d+))?$`)

	for _, p := range parts {
		p = strings.TrimSpace(p)

		// Try full format first
		if matches := reFull.FindStringSubmatch(p); matches != nil {
			cluster, _ := strconv.Atoi(matches[2])
			xadvance, _ := strconv.Atoi(matches[5])

			g := ExpectedGlyph{
				Name:         matches[1],
				Cluster:      cluster,
				XAdvance:     int16(xadvance),
				HasPositions: true, // Positions were explicitly specified
			}

			if matches[3] != "" {
				xoff, _ := strconv.Atoi(matches[3])
				yoff, _ := strconv.Atoi(matches[4])
				g.XOffset = int16(xoff)
				g.YOffset = int16(yoff)
				g.HasOffsets = true // Offsets were explicitly specified
			}

			if matches[6] != "" {
				yadvance, _ := strconv.Atoi(matches[6])
				g.YAdvance = int16(yadvance)
			}

			glyphs = append(glyphs, g)
			continue
		}

		// Try no-positions format (Name=cluster)
		if matches := reNoPos.FindStringSubmatch(p); matches != nil {
			cluster, _ := strconv.Atoi(matches[2])
			g := ExpectedGlyph{
				Name:    matches[1],
				Cluster: cluster,
			}
			glyphs = append(glyphs, g)
			continue
		}

		// Try NED format: Name[@cumX,cumY] (--ned: no cluster, no advance, cumulative positions)
		// Also covers name-only format (Name without @)
		if matches := reNED.FindStringSubmatch(p); matches != nil {
			g := ExpectedGlyph{
				Name:    matches[1],
				Cluster: -1, // Mark as "no cluster info"
			}
			if matches[2] != "" {
				// Cumulative position specified — store in XOffset/YOffset,
				// comparison logic handles cumulative vs offset conversion
				cumX, _ := strconv.Atoi(matches[2])
				cumY, _ := strconv.Atoi(matches[3])
				g.XOffset = int16(cumX)
				g.YOffset = int16(cumY)
				g.HasOffsets = true // signals cumulative position data in NED mode
			}
			glyphs = append(glyphs, g)
			continue
		}

		return nil, fmt.Errorf("invalid glyph format: %s", p)
	}

	return glyphs, nil
}

// parseOptions parses test options like "--shaper=ot --variations=wght=700"
func parseOptions(s string) (TestOptions, error) {
	opts := TestOptions{
		Shaper:          "ot",
		ClusterLevel:    0,
		Direction:       0,  // 0 = auto-detect from content
		NotFoundVSGlyph: -1, // -1 = not set (default: remove VS from buffer)
	}

	if s == "" {
		return opts, nil
	}

	// Split by space, but handle quoted strings
	parts := strings.Fields(s)

	// parseOptValue handles both "--opt=value" and "--opt value" forms.
	// Returns the value string for the option at index i, and the next index to process.
	parseOptValue := func(parts []string, i int, prefix string) (string, int) {
		p := parts[i]
		if strings.HasPrefix(p, prefix+"=") {
			return p[len(prefix)+1:], i + 1
		}
		if p == prefix && i+1 < len(parts) && !strings.HasPrefix(parts[i+1], "--") {
			return parts[i+1], i + 2
		}
		return "", i + 1
	}

	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if strings.HasPrefix(p, "--shaper=") || p == "--shaper" {
			val, next := parseOptValue(parts, i, "--shaper")
			opts.Shaper = val
			i = next - 1
		} else if strings.HasPrefix(p, "--cluster-level=") || p == "--cluster-level" {
			val, next := parseOptValue(parts, i, "--cluster-level")
			level, err := strconv.Atoi(val)
			if err != nil {
				return opts, err
			}
			opts.ClusterLevel = level
			i = next - 1
		} else if p == "--no-positions" {
			opts.NoPositions = true
		} else if p == "--no-clusters" {
			opts.NoClusters = true
		} else if p == "--no-glyph-names" {
			opts.NoGlyphNames = true
		} else if p == "--bot" {
			opts.BOT = true
		} else if p == "--eot" {
			opts.EOT = true
		} else if strings.HasPrefix(p, "--unicodes-before=") || p == "--unicodes-before" {
			val, next := parseOptValue(parts, i, "--unicodes-before")
			opts.UnicodesBefore = parseUnicodesOption(val)
			i = next - 1
		} else if strings.HasPrefix(p, "--unicodes-after=") || p == "--unicodes-after" {
			val, next := parseOptValue(parts, i, "--unicodes-after")
			opts.UnicodesAfter = parseUnicodesOption(val)
			i = next - 1
		} else if strings.HasPrefix(p, "--variations=") || p == "--variations" {
			val, next := parseOptValue(parts, i, "--variations")
			vars := strings.Split(val, ",")
			for _, v := range vars {
				vparts := strings.Split(v, "=")
				if len(vparts) == 2 && len(vparts[0]) >= 4 {
					fval, err := strconv.ParseFloat(vparts[1], 32)
					if err != nil {
						return opts, err
					}
					opts.Variations = append(opts.Variations, ot.Variation{
						Tag:   ot.MakeTag(vparts[0][0], vparts[0][1], vparts[0][2], vparts[0][3]),
						Value: float32(fval),
					})
				}
			}
			i = next - 1
		} else if strings.HasPrefix(p, "--features=") || p == "--features" {
			val, next := parseOptValue(parts, i, "--features")
			feats := strings.Split(val, ",")
			for _, f := range feats {
				feat, ok := ot.FeatureFromString(f)
				if ok {
					opts.Features = append(opts.Features, feat)
				}
			}
			i = next - 1
		} else if strings.HasPrefix(p, "--direction=") || p == "--direction" {
			val, next := parseOptValue(parts, i, "--direction")
			switch val {
			case "rtl", "r":
				opts.Direction = ot.DirectionRTL
			case "ltr", "l":
				opts.Direction = ot.DirectionLTR
			case "t", "ttb":
				opts.Direction = ot.DirectionTTB
			case "b", "btt":
				opts.Direction = ot.DirectionBTT
			}
			i = next - 1
		} else if strings.HasPrefix(p, "--face-index=") || p == "--face-index" {
			val, next := parseOptValue(parts, i, "--face-index")
			idx, err := strconv.Atoi(val)
			if err != nil {
				return opts, err
			}
			opts.FaceIndex = idx
			i = next - 1
		} else if strings.HasPrefix(p, "--script=") || p == "--script" {
			val, next := parseOptValue(parts, i, "--script")
			if len(val) >= 4 {
				opts.Script = ot.MakeTag(val[0], val[1], val[2], val[3])
			}
			i = next - 1
		} else if strings.HasPrefix(p, "--language=") || p == "--language" {
			val, next := parseOptValue(parts, i, "--language")
			// Convert BCP47 language tag to OpenType language system tag(s)
			tags := ot.LanguageToTag(val)
			if len(tags) > 0 {
				opts.Language = tags[0]
				opts.LanguageCandidates = tags
			}
			i = next - 1
		} else if strings.HasPrefix(p, "--font-size=") || p == "--font-size" {
			val, next := parseOptValue(parts, i, "--font-size")
			size, err := strconv.Atoi(val)
			if err != nil {
				return opts, err
			}
			opts.FontSize = size
			i = next - 1
		} else if p == "--ned" || p == "-v" {
			opts.NED = true
			opts.NoClusters = true
		} else if strings.HasPrefix(p, "--not-found-variation-selector-glyph=") {
			val, next := parseOptValue(parts, i, "--not-found-variation-selector-glyph")
			nval, err := strconv.Atoi(val)
			if err != nil {
				return opts, err
			}
			opts.NotFoundVSGlyph = nval
			i = next - 1
		} else if strings.HasPrefix(p, "--font-bold=") || p == "--font-bold" {
			val, next := parseOptValue(parts, i, "--font-bold")
			fval, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return opts, err
			}
			opts.FontBold = fval
			i = next - 1
		} else if strings.HasPrefix(p, "--font-slant=") || p == "--font-slant" {
			val, next := parseOptValue(parts, i, "--font-slant")
			fval, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return opts, err
			}
			opts.FontSlant = fval
			i = next - 1
		}
		// Ignore unknown options like --font-funcs=ot
	}

	return opts, nil
}

// parseTestLine parses a single test line
func parseTestLine(line string, testDir string) (*TestCase, error) {
	// Skip comments and empty lines
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "@") {
		return nil, nil
	}

	parts := strings.Split(line, ";")
	if len(parts) != 4 {
		return nil, fmt.Errorf("expected 4 semicolon-separated fields, got %d", len(parts))
	}

	// Resolve font path: extract @hash suffix (font version verification),
	// handle absolute paths and relative paths.
	fontPath := parts[0]
	var fontHash string
	// Extract @sha1hash suffix used for font version pinning (e.g. "Font.ttc@6a2bc87f...")
	if idx := strings.LastIndex(fontPath, "@"); idx > 0 {
		suffix := fontPath[idx+1:]
		isHash := len(suffix) >= 8
		for _, c := range suffix {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				isHash = false
				break
			}
		}
		if isHash {
			fontHash = suffix
			fontPath = fontPath[:idx]
		}
	}
	// Handle backslash-escaped spaces in font paths (e.g. "Apple\ Color\ Emoji.ttc")
	fontPath = strings.ReplaceAll(fontPath, "\\ ", " ")
	if filepath.IsAbs(fontPath) {
		// Absolute path (e.g. /System/Library/Fonts/...) — use as-is.
		// Try font path fallbacks for macOS version differences.
		fontPath = resolveSystemFontPath(fontPath)
	} else if strings.HasPrefix(fontPath, "../fonts/") {
		fontPath = filepath.Join(testDir, "fonts", fontPath[9:])
	} else {
		fontPath = filepath.Join(testDir, fontPath)
	}

	options, err := parseOptions(parts[1])
	if err != nil {
		return nil, fmt.Errorf("parse options: %w", err)
	}

	input, err := parseUnicodeInput(parts[2])
	if err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}

	expected, err := parseExpectedOutput(parts[3])
	if err != nil {
		return nil, fmt.Errorf("parse expected: %w", err)
	}

	return &TestCase{
		FontPath:     fontPath,
		FontHash:     fontHash,
		Options:      options,
		Input:        input,
		Expected:     expected,
		OriginalLine: line,
	}, nil
}

// loadTestFile loads all test cases from a .tests file
func loadTestFile(path string) ([]*TestCase, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	testDir := filepath.Dir(filepath.Dir(path)) // Go up from tests/ to harfbuzz-tests/
	var tests []*TestCase
	scanner := bufio.NewScanner(file)
	lineNum := 0
	// Track @shapers directive: persistent, applies to ALL following test lines
	// until another @shapers= appears. HarfBuzz test format.
	currentShapers := "" // "" means no restriction (all shapers)

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Handle @shapers directive (persistent)
		if strings.HasPrefix(line, "@shapers=") {
			shaperStr := line[9:]
			// Strip trailing comments (e.g., "@shapers=ot # comment")
			if idx := strings.Index(shaperStr, "#"); idx >= 0 {
				shaperStr = strings.TrimSpace(shaperStr[:idx])
			}
			currentShapers = shaperStr
			continue
		}

		// Skip other @ directives (e.g., @face-loaders=...)
		if strings.HasPrefix(line, "@") {
			continue
		}

		tc, err := parseTestLine(line, testDir)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNum, err)
		}
		if tc != nil {
			// Apply current @shapers restriction
			if currentShapers != "" {
				// Check if "ot" is in the allowed shapers
				shapers := strings.Split(currentShapers, ",")
				otAllowed := false
				for _, s := range shapers {
					if strings.TrimSpace(s) == "ot" {
						otAllowed = true
						break
					}
				}
				if !otAllowed {
					tc.Options.Shaper = currentShapers // Will cause skip in runTest
				}
			}

			tc.SourceFile = filepath.Base(path)
			tc.SourceLine = lineNum
			tests = append(tests, tc)
		}
	}

	return tests, scanner.Err()
}

// runTest runs a single test case and returns whether it passed
func runTest(tc *TestCase, t *testing.T) (passed bool, skipReason string, err error) {
	// Skip non-OT shapers for now
	if tc.Options.Shaper != "ot" && tc.Options.Shaper != "" {
		return false, fmt.Sprintf("shaper %q not supported", tc.Options.Shaper), nil
	}

	// Verify font hash if specified
	if tc.FontHash != "" {
		fontData, err := os.ReadFile(tc.FontPath)
		if err != nil {
			return false, fmt.Sprintf("font file not readable for hash check: %s", tc.FontPath), nil
		}
		h := sha1.Sum(fontData)
		actualHash := hex.EncodeToString(h[:])
		if actualHash != tc.FontHash {
			return false, fmt.Sprintf("font hash mismatch: expected %s, got %s", tc.FontHash, actualHash), nil
		}
	}

	// Load font
	font, err := getFont(tc.FontPath, tc.Options.FaceIndex)
	if err != nil {
		return false, "", fmt.Errorf("load font: %w", err)
	}

	// Create shaper (need new one for variations)
	shaper, err := ot.NewShaper(font)
	if err != nil {
		return false, "", fmt.Errorf("create shaper: %w", err)
	}

	// Apply variations if any
	if len(tc.Options.Variations) > 0 {
		shaper.SetVariations(tc.Options.Variations)
	}

	// Create buffer and add input
	buf := ot.NewBuffer()
	codepoints := make([]ot.Codepoint, len(tc.Input))
	for i, r := range tc.Input {
		codepoints[i] = ot.Codepoint(r)
	}
	buf.AddCodepoints(codepoints)
	buf.NotFoundVSGlyph = tc.Options.NotFoundVSGlyph
	// Set BOT|EOT flags only if explicitly requested (hb-shape defaults to false)
	if tc.Options.BOT {
		buf.Flags |= ot.BufferFlagBOT
	}
	if tc.Options.EOT {
		buf.Flags |= ot.BufferFlagEOT
	}
	// Set pre/post context for Arabic joining
	if len(tc.Options.UnicodesBefore) > 0 {
		buf.PreContext = make([]ot.Codepoint, len(tc.Options.UnicodesBefore))
		for i, r := range tc.Options.UnicodesBefore {
			buf.PreContext[i] = ot.Codepoint(r)
		}
	}
	if len(tc.Options.UnicodesAfter) > 0 {
		buf.PostContext = make([]ot.Codepoint, len(tc.Options.UnicodesAfter))
		for i, r := range tc.Options.UnicodesAfter {
			buf.PostContext[i] = ot.Codepoint(r)
		}
	}
	buf.Direction = tc.Options.Direction
	buf.ClusterLevel = tc.Options.ClusterLevel
	if tc.Options.Script != 0 {
		buf.Script = tc.Options.Script
	}
	if tc.Options.Language != 0 {
		buf.Language = tc.Options.Language
		buf.LanguageCandidates = tc.Options.LanguageCandidates
	}
	buf.GuessSegmentProperties()

	// Set synthetic bold/slant on the shaper before shaping
	if tc.Options.FontBold != 0 {
		shaper.SetSyntheticBold(float32(tc.Options.FontBold), float32(tc.Options.FontBold), false)
	}
	if tc.Options.FontSlant != 0 {
		shaper.SetSyntheticSlant(float32(tc.Options.FontSlant))
	}

	// Shape
	shaper.Shape(buf, tc.Options.Features)

	// Apply font-size scaling if specified
	// HarfBuzz: --font-size=N scales all positions by N/upem using roundf.
	// For vertical text, HarfBuzz computes x_origin = scale(h_advance) / 2 (scale first,
	// then divide). To match this, we undo the unscaled origin, scale, then reapply
	// the scaled origin.
	if tc.Options.FontSize > 0 {
		face, faceErr := ot.NewFace(font)
		if faceErr == nil {
			upem := float64(face.Upem())
			if upem > 0 {
				fontSize := float64(tc.Options.FontSize)
				scaleVal := func(v int16) int16 {
					scaled := float64(v) * fontSize / upem
					if scaled >= 0 {
						return int16(scaled + 0.5)
					}
					return int16(scaled - 0.5)
				}
				if buf.Direction.IsVertical() {
					// HarfBuzz scales h_advance BEFORE dividing by 2 for x_origin.
					// Our shaper computes origin in font units (advance/2).
					// To match HarfBuzz: undo unscaled origin, scale GPOS part,
					// then subtract scaled origin (scale(advance)/2).
					for i := range buf.Pos {
						gid := buf.Info[i].GlyphID
						xOrig, yOrig := shaper.GetGlyphVOrigin(gid)

						// Undo unscaled origin to get GPOS-only contributions
						gposX := buf.Pos[i].XOffset + xOrig
						gposY := buf.Pos[i].YOffset + yOrig

						// Scale GPOS contributions
						scaledGposX := scaleVal(gposX)
						scaledGposY := scaleVal(gposY)

						// Compute scaled origin (HarfBuzz way):
						// x = scale(h_advance) / 2, y = scale(y_origin)
						hAdv := shaper.GetGlyphHAdvanceVar(gid)
						scaledXOrig := scaleVal(int16(hAdv)) / 2
						scaledYOrig := scaleVal(yOrig)

						buf.Pos[i].XOffset = scaledGposX - int16(scaledXOrig)
						buf.Pos[i].YOffset = scaledGposY - scaledYOrig
						buf.Pos[i].XAdvance = scaleVal(buf.Pos[i].XAdvance)
						buf.Pos[i].YAdvance = scaleVal(buf.Pos[i].YAdvance)
					}
				} else {
					for i := range buf.Pos {
						buf.Pos[i].XAdvance = scaleVal(buf.Pos[i].XAdvance)
						buf.Pos[i].YAdvance = scaleVal(buf.Pos[i].YAdvance)
						buf.Pos[i].XOffset = scaleVal(buf.Pos[i].XOffset)
						buf.Pos[i].YOffset = scaleVal(buf.Pos[i].YOffset)
					}
				}
			}
		}
	}

	// Debug output for specific tests
	if t != nil && os.Getenv("HB_DEBUG") == "1" {
		t.Logf("DEBUG: direction=%d, input=%v, variations=%v", buf.Direction, tc.Input, tc.Options.Variations)
		for i, info := range buf.Info {
			t.Logf("  glyph[%d]: gid=%d cluster=%d pos={xadv=%d, yadv=%d, xoff=%d, yoff=%d}",
				i, info.GlyphID, info.Cluster, buf.Pos[i].XAdvance, buf.Pos[i].YAdvance, buf.Pos[i].XOffset, buf.Pos[i].YOffset)
		}
	}

	// Compare results
	if len(buf.Info) != len(tc.Expected) {
		return false, "", fmt.Errorf("glyph count mismatch: got %d, want %d", len(buf.Info), len(tc.Expected))
	}

	for i, exp := range tc.Expected {
		info := buf.Info[i]
		pos := buf.Pos[i]

		// Check glyph name (if expected name is provided)
		// HarfBuzz equivalent: run-tests.py lines 307-322
		// Strategy: Convert both names to glyph IDs and compare the IDs.
		// This handles cases where the font has no post table names but the
		// test file uses names like "uni0622" (which can be resolved via cmap).
		// With CFF support, we can now get glyph names even for post v3.0 fonts!
		if exp.Name != "" {
			if tc.Options.NoGlyphNames {
				// --no-glyph-names: expected name is a GID number
				expGID, err := strconv.Atoi(exp.Name)
				if err == nil && ot.GlyphID(expGID) != info.GlyphID {
					return false, "", fmt.Errorf("glyph %d gid: got %d, want %d",
						i, info.GlyphID, expGID)
				}
			} else {
				gotName := font.GetGlyphName(info.GlyphID)

				// If names differ, try converting both to GIDs and compare
				if gotName != exp.Name {
					// Convert got name to GID
					gotGID := info.GlyphID // We already have the actual GID

					// Convert expected name to GID
					expGID, expOK := font.GetGlyphFromName(exp.Name)

					if !expOK {
						// Can't resolve expected name, report error
						return false, "", fmt.Errorf("glyph %d name: got %s (gid=%d), want %s (cannot resolve expected name)",
							i, gotName, gotGID, exp.Name)
					}

					// Compare the resolved GIDs
					if gotGID != expGID {
						return false, "", fmt.Errorf("glyph %d name: got %s (gid=%d), want %s (gid=%d)",
							i, gotName, gotGID, exp.Name, expGID)
					}
				}
			}
		}

		// Check cluster (skip if --no-clusters or cluster is -1 meaning not specified)
		if !tc.Options.NoClusters && exp.Cluster != -1 {
			if info.Cluster != exp.Cluster {
				return false, "", fmt.Errorf("glyph %d cluster: got %d, want %d", i, info.Cluster, exp.Cluster)
			}
		}

		// Check positions (advances and offsets) if explicitly specified in test file
		// Previously we skipped checks when expected value was 0, which could hide bugs
		if !tc.Options.NoPositions && exp.HasPositions {
			// Check XAdvance (always check if positions were specified)
			if pos.XAdvance != exp.XAdvance {
				return false, "", fmt.Errorf("glyph %d (%s) xadvance: got %d, want %d",
					i, exp.Name, pos.XAdvance, exp.XAdvance)
			}
			// Check YAdvance (for vertical text)
			if pos.YAdvance != exp.YAdvance {
				return false, "", fmt.Errorf("glyph %d (%s) yadvance: got %d, want %d",
					i, exp.Name, pos.YAdvance, exp.YAdvance)
			}
		}

		// Check offsets if explicitly specified (@x,y syntax)
		if !tc.Options.NoPositions && exp.HasOffsets {
			if tc.Options.NED {
				// NED mode: expected values are cumulative positions (sum of advances + offsets)
				// Calculate cumulative position from our output
				var cumX, cumY int16
				for j := 0; j < i; j++ {
					cumX += buf.Pos[j].XAdvance
					cumY += buf.Pos[j].YAdvance
				}
				cumX += pos.XOffset
				cumY += pos.YOffset
				if cumX != exp.XOffset {
					return false, "", fmt.Errorf("glyph %d (%s) cumulative x: got %d, want %d",
						i, exp.Name, cumX, exp.XOffset)
				}
				if cumY != exp.YOffset {
					return false, "", fmt.Errorf("glyph %d (%s) cumulative y: got %d, want %d",
						i, exp.Name, cumY, exp.YOffset)
				}
			} else {
				if pos.XOffset != exp.XOffset {
					return false, "", fmt.Errorf("glyph %d (%s) xoffset: got %d, want %d",
						i, exp.Name, pos.XOffset, exp.XOffset)
				}
				if pos.YOffset != exp.YOffset {
					return false, "", fmt.Errorf("glyph %d (%s) yoffset: got %d, want %d",
						i, exp.Name, pos.YOffset, exp.YOffset)
				}
			}
		}
	}

	return true, "", nil
}

// TestHarfBuzzShapeTests runs all HarfBuzz shape tests
func TestHarfBuzzShapeTests(t *testing.T) {
	testsDir := "tests"

	entries, err := os.ReadDir(testsDir)
	if err != nil {
		t.Fatalf("Failed to read tests directory: %v", err)
	}

	var totalTests, passedTests, skippedTests, failedTests int
	failedByFile := make(map[string]int)
	passedByFile := make(map[string]int)
	skippedByFile := make(map[string]int)

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".tests") {
			continue
		}

		testFile := filepath.Join(testsDir, entry.Name())
		tests, err := loadTestFile(testFile)
		if err != nil {
			t.Errorf("Failed to load %s: %v", entry.Name(), err)
			continue
		}

		for _, tc := range tests {
			totalTests++
			passed, skipReason, err := runTest(tc, t)

			if skipReason != "" {
				skippedTests++
				skippedByFile[entry.Name()]++
				continue
			}

			if err != nil {
				failedTests++
				failedByFile[entry.Name()]++
				if testing.Verbose() {
					t.Logf("FAIL %s:%d: %v\n  Input: %s", tc.SourceFile, tc.SourceLine, err, tc.OriginalLine)
				}
				continue
			}

			if passed {
				passedTests++
				passedByFile[entry.Name()]++
			}
		}
	}

	// Collect all file names in order
	var allFiles []string
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".tests") {
			continue
		}
		allFiles = append(allFiles, entry.Name())
	}

	// Table output mode for TESTPROGRESS.md
	if os.Getenv("HB_TABLE_OUTPUT") == "1" {
		printMarkdownTable(t, allFiles, passedByFile, failedByFile, skippedByFile, passedTests, totalTests)
	} else {
		// Print summary
		t.Logf("\n=== HarfBuzz Shape Test Results ===")
		t.Logf("Total:   %d", totalTests)
		t.Logf("Passed:  %d (%.1f%%)", passedTests, float64(passedTests)*100/float64(totalTests))
		t.Logf("Failed:  %d (%.1f%%)", failedTests, float64(failedTests)*100/float64(totalTests))
		t.Logf("Skipped: %d (%.1f%%)", skippedTests, float64(skippedTests)*100/float64(totalTests))

		// Print per-file breakdown for files with failures
		if len(failedByFile) > 0 {
			t.Logf("\n=== Failed Tests by File ===")
			for file, count := range failedByFile {
				total := passedByFile[file] + failedByFile[file] + skippedByFile[file]
				t.Logf("  %s: %d/%d failed", file, count, total)
			}
		}
	}

	// Report overall failure
	if failedTests > 0 {
		t.Errorf("%d tests failed", failedTests)
	}
}

// fileCategory maps test file names to their display category for TESTPROGRESS.md.
// Files are listed in display order within each category.
var fileCategories = []struct {
	category string
	files    []string
}{
	{"Arabic", []string{
		"arabic-fallback-shaping", "arabic-feature-order", "arabic-like-joining",
		"arabic-mark-order", "arabic-normalization", "arabic-phags-pa", "arabic-stch",
	}},
	{"Indic", []string{
		"indic-consonant-with-stacker", "indic-decompose", "indic-feature-order",
		"indic-init", "indic-joiner-candrabindu", "indic-joiners",
		"indic-malayalam-dot-reph", "indic-misc", "indic-old-spec",
		"indic-pref-blocking", "indic-script-extensions", "indic-special-cases",
		"indic-syllable", "indic-vowel-letter-spoofing",
	}},
	{"Myanmar", []string{
		"myanmar-misc", "myanmar-syllable", "myanmar-zawgyi",
	}},
	{"USE (Universal Shaping)", []string{
		"use", "use-indic3", "use-javanese", "use-marchen", "use-syllable",
		"use-vowel-letter-spoofing",
	}},
	{"Tibetan", []string{
		"tibetan-contractions-1", "tibetan-contractions-2", "tibetan-vowels",
	}},
	{"Khmer", []string{
		"khmer-mark-order", "khmer-misc",
	}},
	{"Hebrew", []string{
		"hebrew-diacritics",
	}},
	{"Thai", []string{
		"sara-am",
	}},
	{"Hangul", []string{
		"hangul-jamo",
	}},
	{"Mongolian", []string{
		"mongolian-variation-selector",
	}},
	{"Emoji", []string{
		"emoji", "emoji-clusters",
	}},
	{"Variations", []string{
		"variation-selectors", "variations", "variations-rvrn",
	}},
	{"Positioning", []string{
		"cursive-positioning", "fallback-positioning", "kern-format2",
		"mark-attachment", "mark-filtering-sets", "nested-mark-filtering-sets",
		"per-script-kern-fallback", "positioning-features", "tt-kern-gpos",
		"zero-width-marks",
	}},
	{"AAT (Apple)", []string{
		"aat-morx", "aat-trak",
	}},
	{"Latin/Basic", []string{
		"automatic-fractions", "collections", "context-matching", "hyphens",
		"language-tags", "none-directional", "simple", "spaces",
	}},
	{"Sonstige", nil}, // Catch-all for uncategorized files
}

func statusEmoji(passed, total int) string {
	if total == 0 {
		return ""
	}
	rate := float64(passed) * 100 / float64(total)
	switch {
	case rate >= 100:
		return "🟢🟢"
	case rate >= 70:
		return "🟢"
	case rate >= 30:
		return "🟡"
	default:
		return "🔴"
	}
}

func rateString(passed, total int) string {
	if total == 0 {
		return ""
	}
	pct := float64(passed) * 100 / float64(total)
	if pct == float64(int(pct)) {
		return fmt.Sprintf("%d%%", int(pct))
	}
	return fmt.Sprintf("%.1f%%", pct)
}

func printMarkdownTable(t *testing.T, allFiles []string, passedByFile, failedByFile, skippedByFile map[string]int, totalPassed, totalTests int) {
	// Build set of categorized files
	categorized := make(map[string]bool)
	for _, cat := range fileCategories {
		for _, f := range cat.files {
			categorized[f] = true
		}
	}

	// Collect uncategorized files for "Sonstige"
	var uncategorized []string
	for _, f := range allFiles {
		name := strings.TrimSuffix(f, ".tests")
		if !categorized[name] {
			uncategorized = append(uncategorized, name)
		}
	}

	// Build lookup: filename -> (passed, total)
	type fileResult struct {
		passed, total int
	}
	results := make(map[string]fileResult)
	for _, f := range allFiles {
		name := strings.TrimSuffix(f, ".tests")
		p := passedByFile[f]
		total := passedByFile[f] + failedByFile[f] + skippedByFile[f]
		results[name] = fileResult{p, total}
	}

	var sb strings.Builder
	sb.WriteString("# HarfBuzz Test-Übersicht\n\n")
	sb.WriteString(fmt.Sprintf("| Kategorie  | Bestanden | Gesamt   | Rate      |\n"))
	sb.WriteString(fmt.Sprintf("| ---------- | --------- | -------- | --------- |\n"))
	sb.WriteString(fmt.Sprintf("| **Gesamt** | **%d**  | **%d** | **%s** |\n", totalPassed, totalTests, rateString(totalPassed, totalTests)))
	sb.WriteString("\n## Ergebnisse nach Testdatei\n\n")
	sb.WriteString("| Testdatei                    | Bestanden | Gesamt | Rate | Status |\n")
	sb.WriteString("| ---------------------------- | --------- | ------ | ---- | ------ |\n")

	for _, cat := range fileCategories {
		files := cat.files
		if cat.category == "Sonstige" {
			files = uncategorized
		}
		if len(files) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("| **%s** |  |  |  |  |\n", cat.category))
		for _, name := range files {
			r, ok := results[name]
			if !ok {
				sb.WriteString(fmt.Sprintf("| %-28s | ?         | ?      | ?    | ?      |\n", name))
				continue
			}
			sb.WriteString(fmt.Sprintf("| %-28s | %-9d | %-6d | %-4s | %-6s |\n",
				name, r.passed, r.total, rateString(r.passed, r.total), statusEmoji(r.passed, r.total)))
		}
	}

	sb.WriteString("\n## Legende\n\n")
	sb.WriteString("- 🟢🟢 100%\n")
	sb.WriteString("- 🟢 70-99%\n")
	sb.WriteString("- 🟡 30-69%\n")
	sb.WriteString("- 🔴 <30%\n")

	t.Logf("\n%s", sb.String())
}

// TestSingleFile runs tests from a single file (useful for debugging)
func TestSingleFile(t *testing.T) {
	testFile := os.Getenv("HB_TEST_FILE")
	if testFile == "" {
		t.Skip("Set HB_TEST_FILE env var to run specific test file")
	}

	testPath := filepath.Join("tests", testFile)
	tests, err := loadTestFile(testPath)
	if err != nil {
		t.Fatalf("Failed to load %s: %v", testFile, err)
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("line%d", tc.SourceLine), func(t *testing.T) {
			passed, skipReason, err := runTest(tc, t)
			if skipReason != "" {
				t.Skip(skipReason)
			}
			if err != nil {
				t.Errorf("Failed: %v\nInput: %s", err, tc.OriginalLine)
			}
			if !passed && err == nil {
				t.Error("Test did not pass")
			}
		})
	}
}
