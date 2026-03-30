package subset

import (
	"os"
	"testing"

	"github.com/boxesandglue/textshape/internal/testutil"
	"github.com/boxesandglue/textshape/ot"
)

func TestCFFParsing(t *testing.T) {
	fontPath := testutil.FindTestFont("SourceSansPro-Regular.otf")
	if fontPath == "" {
		t.Skip("SourceSansPro-Regular.otf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	if !font.HasTable(ot.TagCFF) {
		t.Fatal("Font does not have CFF table")
	}

	cffData, err := font.TableData(ot.TagCFF)
	if err != nil {
		t.Fatalf("Failed to get CFF table: %v", err)
	}

	cff, err := ot.ParseCFF(cffData)
	if err != nil {
		t.Fatalf("Failed to parse CFF: %v", err)
	}

	t.Logf("Font name: %s", cff.Name)
	t.Logf("Number of glyphs: %d", cff.NumGlyphs())
	t.Logf("Global subroutines: %d", len(cff.GlobalSubrs))
	t.Logf("Local subroutines: %d", len(cff.LocalSubrs))
	t.Logf("CharStrings offset: %d", cff.TopDict.CharStrings)
	t.Logf("Private DICT: size=%d, offset=%d", cff.TopDict.Private[0], cff.TopDict.Private[1])

	if cff.NumGlyphs() == 0 {
		t.Error("Expected non-zero glyph count")
	}
}

func TestCFFSubsetBasic(t *testing.T) {
	fontPath := testutil.FindTestFont("SourceSansPro-Regular.otf")
	if fontPath == "" {
		t.Skip("SourceSansPro-Regular.otf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Subset to "Hello"
	result, err := SubsetString(font, "Hello")
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}

	t.Logf("Original size: %d bytes", len(data))
	t.Logf("Subset size: %d bytes", len(result))
	t.Logf("Reduction: %.1f%%", 100*(1-float64(len(result))/float64(len(data))))

	// Parse the subset font
	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset font: %v", err)
	}

	// Verify CFF table exists
	if !subFont.HasTable(ot.TagCFF) {
		t.Error("Subset font missing CFF table")
	}

	// Parse subset CFF
	subCFFData, err := subFont.TableData(ot.TagCFF)
	if err != nil {
		t.Fatalf("Failed to get subset CFF table: %v", err)
	}

	subCFF, err := ot.ParseCFF(subCFFData)
	if err != nil {
		t.Fatalf("Failed to parse subset CFF: %v", err)
	}

	t.Logf("Subset glyph count: %d", subCFF.NumGlyphs())

	// "Hello" has 4 unique characters (H, e, l, o) + .notdef
	// GSUB closure may add more glyphs (ligatures, alternates, etc.)
	minExpectedGlyphs := 5
	if subCFF.NumGlyphs() < minExpectedGlyphs {
		t.Errorf("Expected at least %d glyphs, got %d", minExpectedGlyphs, subCFF.NumGlyphs())
	}

	// Verify size reduction
	if len(result) >= len(data) {
		t.Error("Subset should be smaller than original")
	}
}

func TestCFFSubsetWithSubroutines(t *testing.T) {
	fontPath := testutil.FindTestFont("SourceSansPro-Regular.otf")
	if fontPath == "" {
		t.Skip("SourceSansPro-Regular.otf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Get original CFF to check subroutine counts
	origCFFData, _ := font.TableData(ot.TagCFF)
	origCFF, _ := ot.ParseCFF(origCFFData)

	t.Logf("Original global subrs: %d", len(origCFF.GlobalSubrs))
	t.Logf("Original local subrs: %d", len(origCFF.LocalSubrs))

	// Subset to a larger set of characters to test subroutine handling
	result, err := SubsetString(font, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}

	// Parse subset
	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset: %v", err)
	}

	subCFFData, _ := subFont.TableData(ot.TagCFF)
	subCFF, err := ot.ParseCFF(subCFFData)
	if err != nil {
		t.Fatalf("Failed to parse subset CFF: %v", err)
	}

	t.Logf("Subset global subrs: %d", len(subCFF.GlobalSubrs))
	t.Logf("Subset local subrs: %d", len(subCFF.LocalSubrs))
	t.Logf("Subset glyph count: %d", subCFF.NumGlyphs())

	// Subset should have fewer or equal subroutines
	if len(subCFF.GlobalSubrs) > len(origCFF.GlobalSubrs) {
		t.Error("Subset has more global subroutines than original")
	}
	if len(subCFF.LocalSubrs) > len(origCFF.LocalSubrs) {
		t.Error("Subset has more local subroutines than original")
	}

	t.Logf("Original size: %d bytes", len(data))
	t.Logf("Subset size: %d bytes", len(result))
}

func TestCFFCharStringInterpreter(t *testing.T) {
	fontPath := testutil.FindTestFont("SourceSansPro-Regular.otf")
	if fontPath == "" {
		t.Skip("SourceSansPro-Regular.otf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	cffData, _ := font.TableData(ot.TagCFF)
	cff, _ := ot.ParseCFF(cffData)

	// Test interpreter on first few glyphs
	interp := ot.NewCharStringInterpreter(cff.GlobalSubrs, cff.LocalSubrs)

	for i := 0; i < min(10, len(cff.CharStrings)); i++ {
		err := interp.FindUsedSubroutines(cff.CharStrings[i])
		if err != nil {
			t.Errorf("Failed to interpret glyph %d: %v", i, err)
		}
	}

	t.Logf("Used global subrs: %d", len(interp.UsedGlobalSubrs))
	t.Logf("Used local subrs: %d", len(interp.UsedLocalSubrs))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestCFFSubsetRetainGIDs(t *testing.T) {
	fontPath := testutil.FindTestFont("SourceSansPro-Regular.otf")
	if fontPath == "" {
		t.Skip("SourceSansPro-Regular.otf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Get GID for 'A'
	face, _ := ot.NewFace(font)
	cmap := face.Cmap()
	aGlyph, _ := cmap.Lookup('A')
	t.Logf("'A' GlyphID: %d", aGlyph)

	// Subset with RetainGIDs
	input := NewInput()
	input.Flags = FlagRetainGIDs
	input.AddGlyph(0)      // .notdef
	input.AddGlyph(aGlyph) // 'A'

	plan, err := CreatePlan(font, input)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	result, err := plan.Execute()
	if err != nil {
		t.Fatalf("Failed to execute subset: %v", err)
	}

	// Verify glyph mapping: with RetainGIDs, old GID should equal new GID
	glyphMap := plan.GlyphMap()
	newGID, ok := glyphMap[aGlyph]
	if !ok {
		t.Fatalf("'A' not in glyph map")
	}
	if newGID != aGlyph {
		t.Errorf("With FlagRetainGIDs, GID should be unchanged: got %d, want %d", newGID, aGlyph)
	}
	t.Logf("'A' mapping with RetainGIDs: %d -> %d", aGlyph, newGID)

	// Parse subset font and verify CFF
	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset font: %v", err)
	}

	subCFFData, _ := subFont.TableData(ot.TagCFF)
	subCFF, err := ot.ParseCFF(subCFFData)
	if err != nil {
		t.Fatalf("Failed to parse subset CFF: %v", err)
	}

	// With RetainGIDs, glyph count should be at least aGlyph+1 (slots 0 through aGlyph)
	// Note: GSUB closure may add more glyphs, increasing the max GID
	minExpectedGlyphs := int(aGlyph) + 1
	if subCFF.NumGlyphs() < minExpectedGlyphs {
		t.Errorf("Expected at least %d glyphs with RetainGIDs, got %d", minExpectedGlyphs, subCFF.NumGlyphs())
	}
	t.Logf("Subset glyph count with RetainGIDs: %d (min expected: %d)", subCFF.NumGlyphs(), minExpectedGlyphs)
}

// TestCFFClosureSyntheticSharedStack uses hand-crafted charstrings to verify that
// collectSubrClosure and computeTotalHintCount handle shared operand stacks correctly.
// The bug pattern: a subroutine pushes stem hint operands but has no stem operator;
// the caller consumes them. Without shared stack, hintCount is wrong → hintmask mask
// bytes miscounted → parser loses alignment → subsequent callsubr missed → incomplete closure.
func TestCFFClosureSyntheticSharedStack(t *testing.T) {
	// With 2 local subroutines, bias = 107.
	// callsubr 0: push (0-107)=-107 encoded as byte 32, then op 10
	// callsubr 1: push (1-107)=-106 encoded as byte 33, then op 10
	//
	// CFF single-byte number encoding: byte b (32..246) → value b-139

	// localSubrs[0]: pushes 4 hint values (2 pairs), no stem operator, returns.
	// The caller is expected to consume these with hstem.
	localSubr0 := []byte{
		149, // push 10   (149-139)
		159, // push 20   (159-139)
		169, // push 30   (169-139)
		179, // push 40   (179-139)
		11,  // return
	}

	// localSubrs[1]: simple drawing, returns.
	// This subroutine must appear in the closure for a correct subset.
	localSubr1 := []byte{
		139, // push 0
		139, // push 0
		21,  // rmoveto
		11,  // return
	}

	localSubrs := [][]byte{localSubr0, localSubr1}
	var globalSubrs [][]byte

	// Charstring: callsubr 0 → pushes 4 hint values onto shared stack
	//             hstem      → consumes 4 values = 2 stem hints
	//             hintmask   → skip ceil(2/8) = 1 mask byte
	//             0x80       → the mask byte
	//             callsubr 1 → drawing subroutine
	//             endchar
	//
	// With the old bug (per-call stack): subr 0's values are lost on return,
	// hstem sees empty stack → 0 hints, hintmask skips 0 bytes, the 0x80 mask
	// byte is parsed as a number, and the subsequent callsubr 1 is either
	// missed or misinterpreted → subr 1 not in closure.
	charstring := []byte{
		32, // push -107 (subr 0 biased index)
		10, // callsubr
		1,  // hstem
		19, // hintmask
		0x80,
		33, // push -106 (subr 1 biased index)
		10, // callsubr
		14, // endchar
	}

	_, localClosure := collectSubrClosure([][]byte{charstring}, globalSubrs, localSubrs)

	if !localClosure[0] {
		t.Error("subr 0 missing from closure")
	}
	if !localClosure[1] {
		t.Error("subr 1 missing from closure (shared-stack bug: hintmask byte count wrong → callsubr missed)")
	}

	// Verify computeTotalHintCount with shared stack
	localBias := calcSubrBias(len(localSubrs))  // 107
	globalBias := calcSubrBias(len(globalSubrs)) // 107
	hintCount := computeTotalHintCount(charstring, globalSubrs, localSubrs, globalBias, localBias)
	if hintCount != 2 {
		t.Errorf("computeTotalHintCount = %d, want 2", hintCount)
	}

	// Second pattern: nested subroutines. Subr 0 calls subr 2 which pushes
	// hint values; subr 0 has the stem operator. This tests two levels of
	// stack sharing.
	localSubr2 := []byte{
		149, // push 10
		159, // push 20
		11,  // return
	}
	// subr 0 now: calls subr 2 (which pushes 2 values), pushes 2 more, hstem, return
	localSubr0nested := []byte{
		34, // push -105 (subr 2 biased index: 2-107=-105, byte=139+(-105)=34)
		10, // callsubr (calls subr 2, which pushes 10, 20)
		169, 179, // push 30, 40
		1,  // hstem → consumes all 4 values = 2 hints
		11, // return
	}

	// subr 3: drawing target that must be in closure
	localSubr3 := []byte{
		139, 139, 21, // push 0, 0, rmoveto
		11, // return
	}

	nestedLocalSubrs := [][]byte{localSubr0nested, localSubr1, localSubr2, localSubr3}

	// charstring: callsubr 0 (which calls subr 2 inside), hintmask, callsubr 3, endchar
	nestedCharstring := []byte{
		32,   // push -107 (subr 0)
		10,   // callsubr
		19,   // hintmask
		0x80, // mask byte (ceil(2/8) = 1)
		35,   // push -104 (subr 3 biased: 3-107=-104, byte=139-104=35)
		10,   // callsubr
		14,   // endchar
	}

	_, nestedLocalClosure := collectSubrClosure([][]byte{nestedCharstring}, globalSubrs, nestedLocalSubrs)

	for _, idx := range []int{0, 2, 3} {
		if !nestedLocalClosure[idx] {
			t.Errorf("nested case: subr %d missing from closure", idx)
		}
	}

	nestedLocalBias := calcSubrBias(len(nestedLocalSubrs))
	nestedHintCount := computeTotalHintCount(nestedCharstring, globalSubrs, nestedLocalSubrs, globalBias, nestedLocalBias)
	if nestedHintCount != 2 {
		t.Errorf("nested computeTotalHintCount = %d, want 2", nestedHintCount)
	}
}

// TestCFFSubrClosureWithHintsInSubroutines tests that subroutine closure collection
// and charstring remapping work correctly when stem hints are defined in subroutines
// rather than in the top-level charstring. CFF subroutines share the operand stack
// with their caller, so hint values pushed by subroutines must be counted when
// computing hintmask byte sizes. Without correct stack sharing, the hintmask byte
// count is wrong, causing the byte stream parser to lose alignment and miss
// subsequent callsubr/callgsubr operators.
func TestCFFSubrClosureWithHintsInSubroutines(t *testing.T) {
	fontPath := testutil.FindTestFont("SourceSansPro-Regular.otf")
	if fontPath == "" {
		t.Skip("SourceSansPro-Regular.otf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	face, _ := ot.NewFace(font)
	cmap := face.Cmap()

	cffData, _ := font.TableData(ot.TagCFF)
	cff, err := ot.ParseCFF(cffData)
	if err != nil {
		t.Fatalf("Failed to parse CFF: %v", err)
	}

	globalBias := calcSubrBias(len(cff.GlobalSubrs))
	localBias := calcSubrBias(len(cff.LocalSubrs))

	// Find a glyph that has hints defined in subroutines (not in the top-level charstring).
	// SourceSansPro-Regular uses this pattern for most glyphs.
	rGlyph, ok := cmap.Lookup('R')
	if !ok {
		t.Fatal("'R' not in cmap")
	}

	// Verify pre-condition: 'R' has hints only in subroutines
	totalHints := computeTotalHintCount(cff.CharStrings[rGlyph], cff.GlobalSubrs, cff.LocalSubrs, globalBias, localBias)
	if totalHints == 0 {
		t.Fatal("Expected 'R' to have stem hints (defined in subroutines)")
	}
	t.Logf("'R' GID=%d total_hints=%d", rGlyph, totalHints)

	// Subset with RetainGIDs (the mode used by the PDF writer)
	input := NewInput()
	input.Flags = FlagRetainGIDs | FlagDropLayoutTables
	input.AddGlyph(0)
	input.AddGlyph(rGlyph)

	plan, err := CreatePlan(font, input)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	result, err := plan.Execute()
	if err != nil {
		t.Fatalf("Failed to execute subset: %v", err)
	}

	// Parse subset CFF and verify the R charstring references only valid subroutines
	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset font: %v", err)
	}

	subCFFData, _ := subFont.TableData(ot.TagCFF)
	subCFF, err := ot.ParseCFF(subCFFData)
	if err != nil {
		t.Fatalf("Failed to parse subset CFF: %v", err)
	}

	if int(rGlyph) >= len(subCFF.CharStrings) {
		t.Fatalf("R glyph (GID %d) not in subset charstrings (len %d)", rGlyph, len(subCFF.CharStrings))
	}

	// Verify that ALL subroutine calls in the subset R charstring reference
	// valid subroutines. This catches the bug where wrong hintmask byte counts
	// caused the closure to miss subroutines.
	newGlobalBias := calcSubrBias(len(subCFF.GlobalSubrs))
	newLocalBias := calcSubrBias(len(subCFF.LocalSubrs))
	verifySubrRefs(t, "R", subCFF.CharStrings[rGlyph], subCFF.GlobalSubrs, subCFF.LocalSubrs, newGlobalBias, newLocalBias)

	// Also verify hint count is preserved
	subTotalHints := computeTotalHintCount(subCFF.CharStrings[rGlyph], subCFF.GlobalSubrs, subCFF.LocalSubrs, newGlobalBias, newLocalBias)
	if subTotalHints != totalHints {
		t.Errorf("Hint count mismatch: original=%d, subset=%d", totalHints, subTotalHints)
	}
}

// verifySubrRefs recursively checks that all callsubr/callgsubr calls in a charstring
// reference valid subroutine indices. It follows subroutines with a shared stack
// (as the CFF spec requires).
func verifySubrRefs(t *testing.T, name string, data []byte, globalSubrs, localSubrs [][]byte, globalBias, localBias int) {
	t.Helper()
	stack := make([]int, 0, 48)
	hintCount := 0
	verifySubrRefsRecursive(t, name, data, globalSubrs, localSubrs, globalBias, localBias, &stack, &hintCount, 0)
}

func verifySubrRefsRecursive(t *testing.T, name string, data []byte,
	globalSubrs, localSubrs [][]byte, globalBias, localBias int,
	stack *[]int, hintCount *int, depth int) {
	t.Helper()
	if depth > 10 {
		return
	}

	pos := 0
	for pos < len(data) {
		b := data[pos]
		if b >= 32 && b <= 246 {
			*stack = append(*stack, int(b)-139)
			pos++
		} else if b >= 247 && b <= 250 {
			if pos+1 >= len(data) {
				break
			}
			*stack = append(*stack, (int(b)-247)*256+int(data[pos+1])+108)
			pos += 2
		} else if b >= 251 && b <= 254 {
			if pos+1 >= len(data) {
				break
			}
			*stack = append(*stack, -(int(b)-251)*256-int(data[pos+1])-108)
			pos += 2
		} else if b == 28 {
			if pos+2 >= len(data) {
				break
			}
			*stack = append(*stack, int(int16(uint16(data[pos+1])<<8|uint16(data[pos+2]))))
			pos += 3
		} else if b == 255 {
			if pos+4 >= len(data) {
				break
			}
			*stack = append(*stack, 0)
			pos += 5
		} else {
			op := int(b)
			pos++
			if b == 12 && pos < len(data) {
				op = 12<<8 | int(data[pos])
				pos++
			}
			switch op {
			case 1, 18, 3, 23: // stem operators
				*hintCount += len(*stack) / 2
				*stack = (*stack)[:0]
			case 19, 20: // hintmask, cntrmask
				*hintCount += len(*stack) / 2
				*stack = (*stack)[:0]
				pos += (*hintCount + 7) / 8
			case 10: // callsubr
				if len(*stack) == 0 {
					t.Errorf("%s: callsubr with empty stack at depth %d", name, depth)
					return
				}
				subrNum := (*stack)[len(*stack)-1] + localBias
				*stack = (*stack)[:len(*stack)-1]
				if subrNum < 0 || subrNum >= len(localSubrs) {
					t.Errorf("%s: callsubr references invalid local subr %d (have %d) at depth %d",
						name, subrNum, len(localSubrs), depth)
					return
				}
				verifySubrRefsRecursive(t, name, localSubrs[subrNum], globalSubrs, localSubrs,
					globalBias, localBias, stack, hintCount, depth+1)
			case 29: // callgsubr
				if len(*stack) == 0 {
					t.Errorf("%s: callgsubr with empty stack at depth %d", name, depth)
					return
				}
				subrNum := (*stack)[len(*stack)-1] + globalBias
				*stack = (*stack)[:len(*stack)-1]
				if subrNum < 0 || subrNum >= len(globalSubrs) {
					t.Errorf("%s: callgsubr references invalid global subr %d (have %d) at depth %d",
						name, subrNum, len(globalSubrs), depth)
					return
				}
				verifySubrRefsRecursive(t, name, globalSubrs[subrNum], globalSubrs, localSubrs,
					globalBias, localBias, stack, hintCount, depth+1)
			case 11, 14: // return, endchar
				return
			default:
				*stack = (*stack)[:0]
			}
		}
	}
}
