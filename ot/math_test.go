package ot

import (
	"encoding/binary"
	"testing"
)

// TestParseMath_Synthetic builds a small but spec-shaped MATH table in memory
// and round-trips it through ParseMath. No font file required.
//
// Layout (all offsets are spec-relative to the *owning* sub-table, not to the
// MATH header — that's the easiest mistake to make when hand-building bytes):
//
//	  0  Header (10 B)
//	 10  MathConstants (214 B)
//	224  MathGlyphInfo (8 B)
//	232  MathItalicsCorrectionInfo (4 B + 2×4 B values)
//	244  MathTopAccentAttachment (4 B + 2×4 B values)
//	256  MathKernInfo (4 B + 1×8 B record)
//	268  MathKern for glyph 5 corner TR (2 B + 1×4 B height + 2×4 B kerns)
//	282  Coverage tables (italic, topaccent, mathkern)
//	304  ExtendedShapeCoverage (Format 1, glyph 5)
func TestParseMath_Synthetic(t *testing.T) {
	const total = 310
	data := make([]byte, total)
	put16 := func(at, v int) {
		binary.BigEndian.PutUint16(data[at:], uint16(v))
	}

	// --- Header ---
	put16(0, 1)   // majorVersion
	put16(2, 0)   // minorVersion
	put16(4, 10)  // mathConstantsOffset
	put16(6, 224) // mathGlyphInfoOffset
	put16(8, 0)   // mathVariantsOffset (none in this test)

	// --- MathConstants at 10 ---
	// Initial fields:
	put16(10, 80) // ScriptPercentScaleDown
	put16(12, 60) // ScriptScriptPercentScaleDown
	put16(14, 0)  // DelimitedSubFormulaMinHeight
	put16(16, 0)  // DisplayOperatorMinHeight
	// 51 MathValueRecords (4 bytes each, only the int16 value matters).
	// mv(i) lives at offset 18 + i*4.
	mv := func(i, value int) { put16(18+i*4, value) }
	mv(0, 150)  // MathLeading
	mv(1, 250)  // AxisHeight
	mv(4, 200)  // SubscriptShiftDown
	mv(7, 350)  // SuperscriptShiftUp
	mv(34, 40)  // FractionRuleThickness
	mv(45, 120) // RadicalVerticalGap
	// RadicalDegreeBottomRaisePercent is the trailing int16, NOT a MathValueRecord.
	// Lives at 10 + 8 + 51*4 = 222 .. 223.
	put16(222, 75)

	// --- MathGlyphInfo at 224 (8 B) ---
	put16(224, 8)  // italicsCorrectionOffset (relative to 224 → 232)
	put16(226, 20) // topAccentAttachmentOffset (relative to 224 → 244)
	put16(228, 80) // extendedShapeCoverageOffset (relative to 224 → 304)
	put16(230, 32) // mathKernInfoOffset (relative to 224 → 256)

	// --- MathItalicsCorrectionInfo at 232 ---
	put16(232, 50) // coverageOffset (relative to 232 → 282)
	put16(234, 2)  // italicsCorrectionCount
	// value records — first 2 bytes value, last 2 bytes device offset (0).
	put16(236, 30) // glyph 5: italic 30 FUnit
	put16(238, 0)
	put16(240, 50) // glyph 7: italic 50
	put16(242, 0)

	// --- MathTopAccentAttachment at 244 ---
	put16(244, 46) // coverageOffset (relative to 244 → 290)
	put16(246, 2)  // topAccentAttachmentCount
	put16(248, 400)
	put16(250, 0)
	put16(252, 500)
	put16(254, 0)

	// --- MathKernInfo at 256 ---
	put16(256, 42) // coverageOffset (relative to 256 → 298)
	put16(258, 1)  // mathKernCount
	// MathKernInfoRecord: 4 Offset16 in order TR, TL, BR, BL (relative to 256).
	put16(260, 12) // TR → 268
	put16(262, 0)  // TL
	put16(264, 0)  // BR
	put16(266, 0)  // BL

	// --- MathKern table at 268 (1 height, 2 kern values) ---
	put16(268, 1) // heightCount
	put16(270, 100)
	put16(272, 0) // height device
	put16(274, 10)
	put16(276, 0) // kern 0 device
	put16(278, 20)
	put16(280, 0) // kern 1 device

	// --- Coverage tables ---
	// Italics coverage (Format 1, count=2, glyphs 5, 7) at 282
	put16(282, 1)
	put16(284, 2)
	put16(286, 5)
	put16(288, 7)
	// TopAccent coverage (Format 1, count=2, glyphs 5, 7) at 290
	put16(290, 1)
	put16(292, 2)
	put16(294, 5)
	put16(296, 7)
	// MathKern coverage (Format 1, count=1, glyph 5) at 298
	put16(298, 1)
	put16(300, 1)
	put16(302, 5)
	// ExtendedShapeCoverage (Format 1, count=1, glyph 5) at 304
	put16(304, 1)
	put16(306, 1)
	put16(308, 5)

	m, err := ParseMath(data)
	if err != nil {
		t.Fatalf("ParseMath: %v", err)
	}
	if !m.HasData() {
		t.Fatalf("expected HasData() to be true")
	}

	// Constants spot-checks
	c := m.Constants()
	if c.ScriptPercentScaleDown != 80 {
		t.Errorf("ScriptPercentScaleDown = %d, want 80", c.ScriptPercentScaleDown)
	}
	if c.ScriptScriptPercentScaleDown != 60 {
		t.Errorf("ScriptScriptPercentScaleDown = %d, want 60", c.ScriptScriptPercentScaleDown)
	}
	if c.AxisHeight != 250 {
		t.Errorf("AxisHeight = %d, want 250", c.AxisHeight)
	}
	if c.SubscriptShiftDown != 200 {
		t.Errorf("SubscriptShiftDown = %d, want 200", c.SubscriptShiftDown)
	}
	if c.SuperscriptShiftUp != 350 {
		t.Errorf("SuperscriptShiftUp = %d, want 350", c.SuperscriptShiftUp)
	}
	if c.FractionRuleThickness != 40 {
		t.Errorf("FractionRuleThickness = %d, want 40", c.FractionRuleThickness)
	}
	if c.RadicalVerticalGap != 120 {
		t.Errorf("RadicalVerticalGap = %d, want 120", c.RadicalVerticalGap)
	}
	if c.RadicalDegreeBottomRaisePercent != 75 {
		t.Errorf("RadicalDegreeBottomRaisePercent = %d, want 75", c.RadicalDegreeBottomRaisePercent)
	}

	// Italic correction
	if got := m.ItalicCorrection(5); got != 30 {
		t.Errorf("ItalicCorrection(5) = %d, want 30", got)
	}
	if got := m.ItalicCorrection(7); got != 50 {
		t.Errorf("ItalicCorrection(7) = %d, want 50", got)
	}
	if got := m.ItalicCorrection(99); got != 0 {
		t.Errorf("ItalicCorrection(99) = %d, want 0 (no entry)", got)
	}

	// Top accent attachment — 5/7 set, 99 absent. -1 sentinel for absent.
	if got := m.TopAccentAttachment(5); got != 400 {
		t.Errorf("TopAccentAttachment(5) = %d, want 400", got)
	}
	if got := m.TopAccentAttachment(7); got != 500 {
		t.Errorf("TopAccentAttachment(7) = %d, want 500", got)
	}
	if got := m.TopAccentAttachment(99); got != -1 {
		t.Errorf("TopAccentAttachment(99) = %d, want -1 (absent)", got)
	}

	// Math kerns for glyph 5 corner TR — one correctionHeight=100, kerns [10, 20].
	// height < 100 → kerns[0] = 10
	// height >= 100 → kerns[1] = 20
	if got := m.MathKern(5, MathKernTopRight, 50); got != 10 {
		t.Errorf("MathKern(5, TR, 50) = %d, want 10", got)
	}
	if got := m.MathKern(5, MathKernTopRight, 150); got != 20 {
		t.Errorf("MathKern(5, TR, 150) = %d, want 20", got)
	}
	// Other corners have no table → 0.
	if got := m.MathKern(5, MathKernTopLeft, 50); got != 0 {
		t.Errorf("MathKern(5, TL, 50) = %d, want 0", got)
	}
	// Glyph without any math kern entry → 0.
	if got := m.MathKern(99, MathKernTopRight, 50); got != 0 {
		t.Errorf("MathKern(99, TR, 50) = %d, want 0 (no entry)", got)
	}

	// MathKernEntries returns the full staircase: 1 height (100) → 2 entries,
	// the last with the open-ended MathKernInfinity bound.
	entries := m.MathKernEntries(5, MathKernTopRight)
	wantEntries := []MathKernEntry{
		{MaxCorrectionHeightFU: 100, KernValueFU: 10},
		{MaxCorrectionHeightFU: MathKernInfinity, KernValueFU: 20},
	}
	if len(entries) != len(wantEntries) {
		t.Errorf("MathKernEntries(5, TR) len = %d, want %d", len(entries), len(wantEntries))
	} else {
		for i, w := range wantEntries {
			if entries[i] != w {
				t.Errorf("MathKernEntries(5, TR)[%d] = %+v, want %+v", i, entries[i], w)
			}
		}
	}
	if got := m.MathKernEntries(5, MathKernTopLeft); got != nil {
		t.Errorf("MathKernEntries(5, TL) = %+v, want nil (no table)", got)
	}
	if got := m.MathKernEntries(99, MathKernTopRight); got != nil {
		t.Errorf("MathKernEntries(99, TR) = %+v, want nil (no entry)", got)
	}

	// ExtendedShapeCoverage lists glyph 5 only.
	if !m.IsExtendedShape(5) {
		t.Errorf("IsExtendedShape(5) = false, want true")
	}
	if m.IsExtendedShape(7) {
		t.Errorf("IsExtendedShape(7) = true, want false")
	}

	if m.HasMathVariants() {
		t.Errorf("HasMathVariants() = true, want false (variants offset was 0)")
	}
}

func TestParseMath_TruncatedRejected(t *testing.T) {
	// Header shorter than 10 bytes — must error rather than panic.
	short := []byte{0, 1, 0, 0}
	if _, err := ParseMath(short); err == nil {
		t.Fatalf("expected error on truncated header, got nil")
	}
}

func TestParseMath_VersionRejected(t *testing.T) {
	bad := make([]byte, 10)
	binary.BigEndian.PutUint16(bad[0:], 2) // major=2 — unknown
	if _, err := ParseMath(bad); err == nil {
		t.Fatalf("expected error on unknown major version, got nil")
	}
}

// TestParseMath_Variants constructs a synthetic MATH table with a
// MathVariants block: one vertical glyph (gid 7) with two pre-built
// variants and a 3-part GlyphAssembly. Verifies the parser exposes both
// via the public API.
func TestParseMath_Variants(t *testing.T) {
	// Layout:
	//   0   Header (10 B)
	//  10   MathVariants (12 B header + 2 B vert construction offset)
	//  24   MathGlyphConstruction for gid 7 (4 B + 2*4 B variants = 12 B)
	//  36   GlyphAssembly (6 B header + 3*10 B parts = 36 B)
	//  72   Coverage for vert (4+2 = 6 B)
	const total = 78
	data := make([]byte, total)
	put16 := func(at, v int) { binary.BigEndian.PutUint16(data[at:], uint16(v)) }

	// Header — only mathVariantsOffset matters.
	put16(0, 1)  // major
	put16(2, 0)  // minor
	put16(4, 0)  // constantsOffset
	put16(6, 0)  // glyphInfoOffset
	put16(8, 10) // variantsOffset

	// MathVariants at offset 10
	put16(10, 5)  // minConnectorOverlap
	put16(12, 62) // vertCoverageOffset (relative to 10 → 72)
	put16(14, 0)  // horizCoverageOffset (no horiz variants)
	put16(16, 1)  // vertGlyphCount
	put16(18, 0)  // horizGlyphCount
	put16(20, 14) // vertConstructionOffset[0] (relative to 10 → 24)
	// No horiz construction offsets (count=0).

	// MathGlyphConstruction at offset 24
	put16(24, 12) // glyphAssemblyOffset (relative to 24 → 36)
	put16(26, 2)  // variantCount
	// Variant 0: gid 8, advance 400
	put16(28, 8)
	put16(30, 400)
	// Variant 1: gid 9, advance 600
	put16(32, 9)
	put16(34, 600)

	// GlyphAssembly at offset 36
	put16(36, 50) // italicsCorrection value
	put16(38, 0)  // italicsCorrection device offset
	put16(40, 3)  // partCount
	// Part 0 (top, gid 10): start 0, end 30, advance 200, flags 0
	put16(42, 10)
	put16(44, 0)
	put16(46, 30)
	put16(48, 200)
	put16(50, 0)
	// Part 1 (extender, gid 11): start 30, end 30, advance 100, flags 1
	put16(52, 11)
	put16(54, 30)
	put16(56, 30)
	put16(58, 100)
	put16(60, 1)
	// Part 2 (bottom, gid 12): start 30, end 0, advance 200, flags 0
	put16(62, 12)
	put16(64, 30)
	put16(66, 0)
	put16(68, 200)
	put16(70, 0)

	// Vertical coverage at offset 72 (format 1, count 1, glyph 7)
	put16(72, 1)
	put16(74, 1)
	put16(76, 7)

	m, err := ParseMath(data)
	if err != nil {
		t.Fatalf("ParseMath: %v", err)
	}
	if !m.HasMathVariants() {
		t.Fatalf("HasMathVariants() = false, want true")
	}
	if got := m.MinConnectorOverlap(); got != 5 {
		t.Errorf("MinConnectorOverlap = %d, want 5", got)
	}

	variants := m.VerticalVariants(7)
	if len(variants) != 2 {
		t.Fatalf("VerticalVariants(7) returned %d variants, want 2", len(variants))
	}
	if variants[0].GlyphID != 8 || variants[0].AdvanceFU != 400 {
		t.Errorf("variant 0 = (gid=%d, adv=%d), want (8, 400)", variants[0].GlyphID, variants[0].AdvanceFU)
	}
	if variants[1].GlyphID != 9 || variants[1].AdvanceFU != 600 {
		t.Errorf("variant 1 = (gid=%d, adv=%d), want (9, 600)", variants[1].GlyphID, variants[1].AdvanceFU)
	}

	assembly := m.VerticalAssembly(7)
	if assembly == nil {
		t.Fatalf("VerticalAssembly(7) returned nil, want a 3-part assembly")
	}
	if assembly.ItalicsCorrectionFU != 50 {
		t.Errorf("italics correction = %d, want 50", assembly.ItalicsCorrectionFU)
	}
	if len(assembly.Parts) != 3 {
		t.Fatalf("assembly has %d parts, want 3", len(assembly.Parts))
	}
	if assembly.Parts[0].IsExtender || !assembly.Parts[1].IsExtender || assembly.Parts[2].IsExtender {
		t.Errorf("extender flags wrong: got [%v,%v,%v], want [false,true,false]",
			assembly.Parts[0].IsExtender, assembly.Parts[1].IsExtender, assembly.Parts[2].IsExtender)
	}
	if assembly.Parts[1].FullAdvanceFU != 100 {
		t.Errorf("extender FullAdvanceFU = %d, want 100", assembly.Parts[1].FullAdvanceFU)
	}

	// A non-covered glyph returns empty.
	if got := m.VerticalVariants(99); len(got) != 0 {
		t.Errorf("VerticalVariants(99) returned %d variants, want 0", len(got))
	}
	if got := m.VerticalAssembly(99); got != nil {
		t.Errorf("VerticalAssembly(99) returned %v, want nil", got)
	}
}
