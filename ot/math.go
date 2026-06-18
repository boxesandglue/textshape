package ot

import (
	"encoding/binary"
)

// TagMATH is the table tag for the OpenType MATH table.
var TagMATH = MakeTag('M', 'A', 'T', 'H')

// MathKernCorner identifies one of the four corners of a glyph used by the
// MathKernInfo table. The values match the order of the four Offset16 entries
// in MathKernInfoRecord (TopRight, TopLeft, BottomRight, BottomLeft).
type MathKernCorner uint8

const (
	MathKernTopRight    MathKernCorner = 0
	MathKernTopLeft     MathKernCorner = 1
	MathKernBottomRight MathKernCorner = 2
	MathKernBottomLeft  MathKernCorner = 3
)

// MathConstants holds the 57 named values that drive math layout decisions.
// All MathValueRecord fields are stored as raw FUnit int16 values — the device
// table offset is ignored (devices are intended for screen rendering hints,
// not high-quality typesetting).
type MathConstants struct {
	ScriptPercentScaleDown                   int16
	ScriptScriptPercentScaleDown             int16
	DelimitedSubFormulaMinHeight             uint16
	DisplayOperatorMinHeight                 uint16
	MathLeading                              int16
	AxisHeight                               int16
	AccentBaseHeight                         int16
	FlattenedAccentBaseHeight                int16
	SubscriptShiftDown                       int16
	SubscriptTopMax                          int16
	SubscriptBaselineDropMin                 int16
	SuperscriptShiftUp                       int16
	SuperscriptShiftUpCramped                int16
	SuperscriptBottomMin                     int16
	SuperscriptBaselineDropMax               int16
	SubSuperscriptGapMin                     int16
	SuperscriptBottomMaxWithSubscript        int16
	SpaceAfterScript                         int16
	UpperLimitGapMin                         int16
	UpperLimitBaselineRiseMin                int16
	LowerLimitGapMin                         int16
	LowerLimitBaselineDropMin                int16
	StackTopShiftUp                          int16
	StackTopDisplayStyleShiftUp              int16
	StackBottomShiftDown                     int16
	StackBottomDisplayStyleShiftDown         int16
	StackGapMin                              int16
	StackDisplayStyleGapMin                  int16
	StretchStackTopShiftUp                   int16
	StretchStackBottomShiftDown              int16
	StretchStackGapAboveMin                  int16
	StretchStackGapBelowMin                  int16
	FractionNumeratorShiftUp                 int16
	FractionNumeratorDisplayStyleShiftUp     int16
	FractionDenominatorShiftDown             int16
	FractionDenominatorDisplayStyleShiftDown int16
	FractionNumeratorGapMin                  int16
	FractionNumDisplayStyleGapMin            int16
	FractionRuleThickness                    int16
	FractionDenominatorGapMin                int16
	FractionDenomDisplayStyleGapMin          int16
	SkewedFractionHorizontalGap              int16
	SkewedFractionVerticalGap                int16
	OverbarVerticalGap                       int16
	OverbarRuleThickness                     int16
	OverbarExtraAscender                     int16
	UnderbarVerticalGap                      int16
	UnderbarRuleThickness                    int16
	UnderbarExtraDescender                   int16
	RadicalVerticalGap                       int16
	RadicalDisplayStyleVerticalGap           int16
	RadicalRuleThickness                     int16
	RadicalExtraAscender                     int16
	RadicalKernBeforeDegree                  int16
	RadicalKernAfterDegree                   int16
	RadicalDegreeBottomRaisePercent          int16
}

// mathKernTable is the raw correction-height / kern-value array for one corner
// of one glyph. Values come straight from the font in FUnits.
type mathKernTable struct {
	heights []int16 // length n
	kerns   []int16 // length n+1
}

// mathKernEntry holds the four corner kern tables for a single glyph. A nil
// entry per corner means "no math kern for this corner" — the caller treats
// that as zero correction.
type mathKernEntry struct {
	corners [4]*mathKernTable
}

// MathKernEntry is one step of a glyph's math-kern (cut-in) staircase for a
// single corner, as returned by [Math.MathKernEntries]. KernValueFU applies
// while the correction height is below MaxCorrectionHeightFU (and at or above
// the previous entry's). Mirrors HarfBuzz's hb_ot_math_kern_entry_t.
type MathKernEntry struct {
	MaxCorrectionHeightFU int32 // upper height bound in FUnits; MathKernInfinity on the last entry
	KernValueFU           int16 // kern (cut-in) value in FUnits
}

// MathKernInfinity is the MaxCorrectionHeightFU of the final [MathKernEntry]:
// the last kern value applies to every height at or above the previous
// entry, with no upper bound. Matches the open-ended top interval of an
// OpenType MathKern table.
const MathKernInfinity int32 = 1<<31 - 1

// MathVariant is one entry from a MathGlyphConstruction's variant list:
// a successively-larger glyph designed by the font to replace the base
// glyph when a taller (or wider) sign is needed. AdvanceFU is the glyph's
// advance along the stretch axis in font units — for vertical variants
// this is the height (including depth), for horizontal it's the width.
type MathVariant struct {
	GlyphID   GlyphID
	AdvanceFU uint16
}

// MathGlyphPart is one piece of a GlyphAssembly. A vertical assembly is a
// stack of glyph pieces in font-unit dimensions: typically a fixed top, a
// repeatable middle (the "extender"), and a fixed bottom. Horizontal
// assemblies follow the same pattern rotated 90°.
type MathGlyphPart struct {
	GlyphID                GlyphID
	StartConnectorLengthFU uint16 // overlap region with the previous part
	EndConnectorLengthFU   uint16 // overlap region with the next part
	FullAdvanceFU          uint16 // full piece length, no overlap subtracted
	IsExtender             bool   // partFlags bit 0
}

// MathGlyphAssembly is a font-provided recipe for building a stretchable
// glyph out of multiple parts. ItalicsCorrectionFU only applies after the
// assembled glyph (rarely consumed in vertical stretching).
type MathGlyphAssembly struct {
	ItalicsCorrectionFU int16
	Parts               []MathGlyphPart
}

// Math represents a parsed OpenType MATH table.
//
// Phase 1+2 of the math engine consumes:
//   - Constants (MathConstants table)
//   - ItalicCorrection (MathGlyphInfo → MathItalicsCorrectionInfo)
//   - TopAccentAttachment (MathGlyphInfo → MathTopAccentAttachment)
//   - MathKern (MathGlyphInfo → MathKernInfo)
//   - ExtendedShapeCoverage (MathGlyphInfo)
//   - MathVariants + GlyphAssembly (phase 2: stretchy delimiters, big-op
//     variants in display style, radical-size selection)
//
// MathValueRecord device/variation deltas are deliberately NOT resolved.
// Every value here is the plain int16/int32 from the record; the trailing
// Offset16 (a Device table for hinting deltas, or — in a variable font — a
// VariationIndex into the item variation store) is read past but discarded.
// This is correct for static fonts (the value is the design-size metric).
// For a variable math font it yields the default-instance metrics regardless
// of the selected axis position. HarfBuzz resolves these deltas via
// hb_font's get_x/y_delta; matching that would mean capturing the Offset16
// here and resolving it against a normalized coordinate — an additive change
// (the flat int fields would need to grow), not a redesign. See
// parseMathConstants and parseGlyphValueTable.
type Math struct {
	data           []byte
	constants      *MathConstants
	italicsCorr    map[GlyphID]int16 // FUnits
	topAccentAttch map[GlyphID]int16 // FUnits
	mathKerns      map[GlyphID]*mathKernEntry
	extendedShapes map[GlyphID]bool

	minConnectorOverlap uint16
	vertVariants        map[GlyphID][]MathVariant
	horizVariants       map[GlyphID][]MathVariant
	vertAssembly        map[GlyphID]*MathGlyphAssembly
	horizAssembly       map[GlyphID]*MathGlyphAssembly
	hasVariants         bool
}

// ParseMath parses a MATH table from raw bytes.
//
// On a well-formed table all sub-tables are read; on a malformed sub-table the
// parser returns the partial Math object together with the error, so the
// caller can still use the constants if e.g. the kern table was corrupt.
func ParseMath(data []byte) (*Math, error) {
	if len(data) < 10 {
		return nil, ErrInvalidTable
	}
	major := binary.BigEndian.Uint16(data[0:])
	minor := binary.BigEndian.Uint16(data[2:])
	if major != 1 || minor != 0 {
		return nil, ErrInvalidFormat
	}

	constantsOff := int(binary.BigEndian.Uint16(data[4:]))
	glyphInfoOff := int(binary.BigEndian.Uint16(data[6:]))
	variantsOff := int(binary.BigEndian.Uint16(data[8:]))

	m := &Math{
		data:           data,
		italicsCorr:    map[GlyphID]int16{},
		topAccentAttch: map[GlyphID]int16{},
		mathKerns:      map[GlyphID]*mathKernEntry{},
		extendedShapes: map[GlyphID]bool{},
		vertVariants:   map[GlyphID][]MathVariant{},
		horizVariants:  map[GlyphID][]MathVariant{},
		vertAssembly:   map[GlyphID]*MathGlyphAssembly{},
		horizAssembly:  map[GlyphID]*MathGlyphAssembly{},
		hasVariants:    variantsOff != 0,
	}

	if constantsOff != 0 {
		c, err := parseMathConstants(data, constantsOff)
		if err != nil {
			return m, err
		}
		m.constants = c
	}

	if glyphInfoOff != 0 {
		if err := m.parseMathGlyphInfo(data, glyphInfoOff); err != nil {
			return m, err
		}
	}

	if variantsOff != 0 {
		if err := m.parseMathVariants(data, variantsOff); err != nil {
			return m, err
		}
	}

	return m, nil
}

// HasData reports whether the table contains a usable MathConstants block.
func (m *Math) HasData() bool {
	return m != nil && m.constants != nil
}

// Constants returns the parsed MathConstants block, or nil if the font did not
// embed one (very rare; treat as "this font has no MATH support").
func (m *Math) Constants() *MathConstants {
	if m == nil {
		return nil
	}
	return m.constants
}

// ItalicCorrection returns the italic correction in FUnits for glyph gid, or
// 0 if the glyph has no entry. Used at sub/sup-script placement to shift the
// superscript right past the slanted nucleus.
func (m *Math) ItalicCorrection(gid GlyphID) int16 {
	if m == nil {
		return 0
	}
	return m.italicsCorr[gid]
}

// TopAccentAttachment returns the horizontal accent attachment in FUnits for
// glyph gid, or -1 if the glyph has no entry. -1 is a sentinel: callers use
// the glyph's advance/2 as fallback (matches LuaTeX `compute_accent_skew` in
// mlist.c:2630, where `INT_MIN` signals "absent" and the layout substitutes
// `half(width)`).
func (m *Math) TopAccentAttachment(gid GlyphID) int32 {
	if m == nil {
		return -1
	}
	if v, ok := m.topAccentAttch[gid]; ok {
		return int32(v)
	}
	return -1
}

// MathKern returns the corner-specific kern in FUnits for glyph gid at the
// given correction height (also in FUnits). Returns 0 if the glyph has no
// math-kern entry for that corner.
//
// Lookup follows OT MATH spec: a MathKern of n heights and n+1 kerns is read
// linearly — the first kern whose preceding height exceeds the query height
// wins. This mirrors LuaTeX `math_kern_at` in mlist.c:3517-3551.
func (m *Math) MathKern(gid GlyphID, corner MathKernCorner, height int16) int16 {
	if m == nil {
		return 0
	}
	e := m.mathKerns[gid]
	if e == nil {
		return 0
	}
	t := e.corners[corner]
	if t == nil || len(t.kerns) == 0 {
		return 0
	}
	for i, h := range t.heights {
		if height < h {
			return t.kerns[i]
		}
	}
	return t.kerns[len(t.kerns)-1]
}

// MathKernEntries returns the full cut-in staircase for glyph gid at the
// given corner, as a slice of [MathKernEntry] in ascending height order. For
// an OpenType MathKern with n correction heights there are n+1 entries; the
// last one carries MaxCorrectionHeightFU == [MathKernInfinity] (it applies to
// every height above the last correction height). Returns nil if the glyph
// has no kern for that corner.
//
// This is the raw-table counterpart to [Math.MathKern] (which resolves a
// single height to its kern), mirroring HarfBuzz's
// hb_ot_math_get_glyph_kernings.
func (m *Math) MathKernEntries(gid GlyphID, corner MathKernCorner) []MathKernEntry {
	if m == nil {
		return nil
	}
	e := m.mathKerns[gid]
	if e == nil {
		return nil
	}
	t := e.corners[corner]
	if t == nil || len(t.kerns) == 0 {
		return nil
	}
	out := make([]MathKernEntry, len(t.kerns))
	for i := range t.kerns {
		h := MathKernInfinity
		if i < len(t.heights) {
			h = int32(t.heights[i])
		}
		out[i] = MathKernEntry{MaxCorrectionHeightFU: h, KernValueFU: t.kerns[i]}
	}
	return out
}

// IsExtendedShape reports whether gid is listed in the MATH table's
// ExtendedShapeCoverage — a tall glyph (e.g. an integral) whose design
// already extends well above and below the math axis. Layout engines use
// this to position super/subscripts on such glyphs differently. Mirrors
// HarfBuzz's hb_ot_math_is_glyph_extended_shape.
func (m *Math) IsExtendedShape(gid GlyphID) bool {
	if m == nil {
		return false
	}
	return m.extendedShapes[gid]
}

// HasMathVariants reports whether the font advertises MathVariants —
// stretchy delimiters, big-op variants, etc.
func (m *Math) HasMathVariants() bool {
	return m != nil && m.hasVariants
}

// MinConnectorOverlap returns the minimum overlap (in font units) between
// adjacent glyph pieces in a GlyphAssembly. Used by the assembly builder
// to avoid "transparent seams" between pieces.
func (m *Math) MinConnectorOverlap() uint16 {
	if m == nil {
		return 0
	}
	return m.minConnectorOverlap
}

// VerticalVariants returns the size-ordered list of pre-designed variants
// for the given vertically-stretchable glyph (e.g. (, [, {, √, ∑, ∫). The
// list is sorted by AdvanceFU ascending — callers pick the smallest
// variant whose AdvanceFU meets or exceeds the required height. An empty
// slice means the font ships no variants for this glyph (use the base
// glyph or fall back to assembly).
func (m *Math) VerticalVariants(gid GlyphID) []MathVariant {
	if m == nil {
		return nil
	}
	return m.vertVariants[gid]
}

// HorizontalVariants returns the size-ordered variants for a horizontally-
// stretchable glyph (e.g. →, =, brace-like accents that grow with width).
func (m *Math) HorizontalVariants(gid GlyphID) []MathVariant {
	if m == nil {
		return nil
	}
	return m.horizVariants[gid]
}

// VerticalAssembly returns the assembly recipe for a vertically-stretchable
// glyph that the font can extend beyond its pre-built variants by chaining
// top / middle / bottom pieces. Returns nil if the font has no assembly
// for the glyph (typical for big-op variants — they're usually limited to
// the pre-built variant set).
func (m *Math) VerticalAssembly(gid GlyphID) *MathGlyphAssembly {
	if m == nil {
		return nil
	}
	return m.vertAssembly[gid]
}

// HorizontalAssembly returns the horizontal assembly recipe (for the
// occasional growable arrow or accent).
func (m *Math) HorizontalAssembly(gid GlyphID) *MathGlyphAssembly {
	if m == nil {
		return nil
	}
	return m.horizAssembly[gid]
}

// --- internals ---------------------------------------------------------

// parseMathConstants reads the 214-byte MathConstants block at the given
// offset. Each MathValueRecord is 4 bytes (int16 value + Offset16 device); we
// only consume the value.
func parseMathConstants(data []byte, off int) (*MathConstants, error) {
	const sz = 4 + 4 + 51*4 + 2 // = 214
	if off < 0 || off+sz > len(data) {
		return nil, ErrInvalidOffset
	}
	b := data[off:]
	c := &MathConstants{}

	c.ScriptPercentScaleDown = int16(binary.BigEndian.Uint16(b[0:]))
	c.ScriptScriptPercentScaleDown = int16(binary.BigEndian.Uint16(b[2:]))
	c.DelimitedSubFormulaMinHeight = binary.BigEndian.Uint16(b[4:])
	c.DisplayOperatorMinHeight = binary.BigEndian.Uint16(b[6:])

	// 51 MathValueRecords — value is the first 2 bytes of each 4-byte record.
	mv := func(i int) int16 {
		return int16(binary.BigEndian.Uint16(b[8+i*4:]))
	}
	c.MathLeading = mv(0)
	c.AxisHeight = mv(1)
	c.AccentBaseHeight = mv(2)
	c.FlattenedAccentBaseHeight = mv(3)
	c.SubscriptShiftDown = mv(4)
	c.SubscriptTopMax = mv(5)
	c.SubscriptBaselineDropMin = mv(6)
	c.SuperscriptShiftUp = mv(7)
	c.SuperscriptShiftUpCramped = mv(8)
	c.SuperscriptBottomMin = mv(9)
	c.SuperscriptBaselineDropMax = mv(10)
	c.SubSuperscriptGapMin = mv(11)
	c.SuperscriptBottomMaxWithSubscript = mv(12)
	c.SpaceAfterScript = mv(13)
	c.UpperLimitGapMin = mv(14)
	c.UpperLimitBaselineRiseMin = mv(15)
	c.LowerLimitGapMin = mv(16)
	c.LowerLimitBaselineDropMin = mv(17)
	c.StackTopShiftUp = mv(18)
	c.StackTopDisplayStyleShiftUp = mv(19)
	c.StackBottomShiftDown = mv(20)
	c.StackBottomDisplayStyleShiftDown = mv(21)
	c.StackGapMin = mv(22)
	c.StackDisplayStyleGapMin = mv(23)
	c.StretchStackTopShiftUp = mv(24)
	c.StretchStackBottomShiftDown = mv(25)
	c.StretchStackGapAboveMin = mv(26)
	c.StretchStackGapBelowMin = mv(27)
	c.FractionNumeratorShiftUp = mv(28)
	c.FractionNumeratorDisplayStyleShiftUp = mv(29)
	c.FractionDenominatorShiftDown = mv(30)
	c.FractionDenominatorDisplayStyleShiftDown = mv(31)
	c.FractionNumeratorGapMin = mv(32)
	c.FractionNumDisplayStyleGapMin = mv(33)
	c.FractionRuleThickness = mv(34)
	c.FractionDenominatorGapMin = mv(35)
	c.FractionDenomDisplayStyleGapMin = mv(36)
	c.SkewedFractionHorizontalGap = mv(37)
	c.SkewedFractionVerticalGap = mv(38)
	c.OverbarVerticalGap = mv(39)
	c.OverbarRuleThickness = mv(40)
	c.OverbarExtraAscender = mv(41)
	c.UnderbarVerticalGap = mv(42)
	c.UnderbarRuleThickness = mv(43)
	c.UnderbarExtraDescender = mv(44)
	c.RadicalVerticalGap = mv(45)
	c.RadicalDisplayStyleVerticalGap = mv(46)
	c.RadicalRuleThickness = mv(47)
	c.RadicalExtraAscender = mv(48)
	c.RadicalKernBeforeDegree = mv(49)
	c.RadicalKernAfterDegree = mv(50)

	c.RadicalDegreeBottomRaisePercent = int16(binary.BigEndian.Uint16(b[8+51*4:]))
	return c, nil
}

func (m *Math) parseMathGlyphInfo(data []byte, off int) error {
	if off < 0 || off+8 > len(data) {
		return ErrInvalidOffset
	}
	b := data[off:]
	italicOff := int(binary.BigEndian.Uint16(b[0:]))
	topAccOff := int(binary.BigEndian.Uint16(b[2:]))
	extShapeOff := int(binary.BigEndian.Uint16(b[4:]))
	kernInfoOff := int(binary.BigEndian.Uint16(b[6:]))

	if italicOff != 0 {
		if err := m.parseGlyphValueTable(data, off+italicOff, m.italicsCorr); err != nil {
			return err
		}
	}
	if topAccOff != 0 {
		if err := m.parseGlyphValueTable(data, off+topAccOff, m.topAccentAttch); err != nil {
			return err
		}
	}
	if extShapeOff != 0 {
		// ExtendedShapeCoverage is a bare Coverage table: membership marks a
		// glyph as an "extended shape" (a tall glyph like an integral whose
		// design already spans well above/below the axis). Layout engines use
		// this to position scripts on such glyphs differently.
		gids, err := parseCoverage(data, off+extShapeOff)
		if err != nil {
			return err
		}
		for _, gid := range gids {
			m.extendedShapes[gid] = true
		}
	}
	if kernInfoOff != 0 {
		if err := m.parseMathKernInfo(data, off+kernInfoOff); err != nil {
			return err
		}
	}
	return nil
}

// parseGlyphValueTable handles MathItalicsCorrectionInfo and
// MathTopAccentAttachment — both share the same layout: a coverage table
// followed by a parallel array of MathValueRecord. The coverage's i-th glyph
// maps to the i-th value.
func (m *Math) parseGlyphValueTable(data []byte, off int, dst map[GlyphID]int16) error {
	if off < 0 || off+4 > len(data) {
		return ErrInvalidOffset
	}
	b := data[off:]
	covOff := int(binary.BigEndian.Uint16(b[0:]))
	count := int(binary.BigEndian.Uint16(b[2:]))
	if 4+count*4 > len(b) {
		return ErrInvalidOffset
	}
	values := make([]int16, count)
	for i := 0; i < count; i++ {
		values[i] = int16(binary.BigEndian.Uint16(b[4+i*4:]))
	}
	gids, err := parseCoverage(data, off+covOff)
	if err != nil {
		return err
	}
	for i, gid := range gids {
		if i >= count {
			break
		}
		dst[gid] = values[i]
	}
	return nil
}

func (m *Math) parseMathKernInfo(data []byte, off int) error {
	if off < 0 || off+4 > len(data) {
		return ErrInvalidOffset
	}
	b := data[off:]
	covOff := int(binary.BigEndian.Uint16(b[0:]))
	count := int(binary.BigEndian.Uint16(b[2:]))
	if 4+count*8 > len(b) {
		return ErrInvalidOffset
	}
	// Per-record: 4 Offset16 (TR, TL, BR, BL).
	records := make([][4]int, count)
	for i := 0; i < count; i++ {
		base := 4 + i*8
		records[i][0] = int(binary.BigEndian.Uint16(b[base+0:]))
		records[i][1] = int(binary.BigEndian.Uint16(b[base+2:]))
		records[i][2] = int(binary.BigEndian.Uint16(b[base+4:]))
		records[i][3] = int(binary.BigEndian.Uint16(b[base+6:]))
	}
	gids, err := parseCoverage(data, off+covOff)
	if err != nil {
		return err
	}
	for i, gid := range gids {
		if i >= count {
			break
		}
		entry := &mathKernEntry{}
		for c := 0; c < 4; c++ {
			if records[i][c] == 0 {
				continue
			}
			t, err := parseMathKern(data, off+records[i][c])
			if err != nil {
				return err
			}
			entry.corners[c] = t
		}
		m.mathKerns[gid] = entry
	}
	return nil
}

// parseMathKern reads a single MathKern table: heightCount + n correction
// heights + n+1 kern values. Each value is a MathValueRecord (4 bytes), we
// take the int16 value.
func parseMathKern(data []byte, off int) (*mathKernTable, error) {
	if off < 0 || off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	b := data[off:]
	n := int(binary.BigEndian.Uint16(b[0:]))
	need := 2 + n*4 + (n+1)*4
	if need > len(b) {
		return nil, ErrInvalidOffset
	}
	heights := make([]int16, n)
	for i := 0; i < n; i++ {
		heights[i] = int16(binary.BigEndian.Uint16(b[2+i*4:]))
	}
	kerns := make([]int16, n+1)
	base := 2 + n*4
	for i := 0; i <= n; i++ {
		kerns[i] = int16(binary.BigEndian.Uint16(b[base+i*4:]))
	}
	return &mathKernTable{heights: heights, kerns: kerns}, nil
}

// parseMathVariants reads the MathVariants block at the given offset
// (relative to the MATH table start). Layout per OT-MATH spec:
//
//	  0  uint16   minConnectorOverlap
//	  2  Offset16 vertGlyphCoverageOffset    (relative to MathVariants)
//	  4  Offset16 horizGlyphCoverageOffset
//	  6  uint16   vertGlyphCount
//	  8  uint16   horizGlyphCount
//	 10  Offset16 vertGlyphConstruction[vertGlyphCount]
//	     Offset16 horizGlyphConstruction[horizGlyphCount]
//
// Each MathGlyphConstruction inhabits its own sub-table with variants and
// optionally a glyph assembly. We materialize everything into Go maps so
// the layout engine does O(1) lookup by glyph id.
func (m *Math) parseMathVariants(data []byte, off int) error {
	if off < 0 || off+10 > len(data) {
		return ErrInvalidOffset
	}
	b := data[off:]
	m.minConnectorOverlap = binary.BigEndian.Uint16(b[0:])
	vertCovOff := int(binary.BigEndian.Uint16(b[2:]))
	horizCovOff := int(binary.BigEndian.Uint16(b[4:]))
	vertCount := int(binary.BigEndian.Uint16(b[6:]))
	horizCount := int(binary.BigEndian.Uint16(b[8:]))

	need := 10 + 2*vertCount + 2*horizCount
	if need > len(b) {
		return ErrInvalidOffset
	}
	vertConstrOff := make([]int, vertCount)
	for i := 0; i < vertCount; i++ {
		vertConstrOff[i] = int(binary.BigEndian.Uint16(b[10+i*2:]))
	}
	horizConstrOff := make([]int, horizCount)
	for i := 0; i < horizCount; i++ {
		horizConstrOff[i] = int(binary.BigEndian.Uint16(b[10+vertCount*2+i*2:]))
	}

	if vertCount > 0 {
		vertGids, err := parseCoverage(data, off+vertCovOff)
		if err != nil {
			return err
		}
		for i, gid := range vertGids {
			if i >= vertCount {
				break
			}
			variants, assembly, err := parseMathGlyphConstruction(data, off+vertConstrOff[i])
			if err != nil {
				return err
			}
			if len(variants) > 0 {
				m.vertVariants[gid] = variants
			}
			if assembly != nil {
				m.vertAssembly[gid] = assembly
			}
		}
	}

	if horizCount > 0 {
		horizGids, err := parseCoverage(data, off+horizCovOff)
		if err != nil {
			return err
		}
		for i, gid := range horizGids {
			if i >= horizCount {
				break
			}
			variants, assembly, err := parseMathGlyphConstruction(data, off+horizConstrOff[i])
			if err != nil {
				return err
			}
			if len(variants) > 0 {
				m.horizVariants[gid] = variants
			}
			if assembly != nil {
				m.horizAssembly[gid] = assembly
			}
		}
	}

	return nil
}

// parseMathGlyphConstruction reads one MathGlyphConstruction sub-table:
//
//	  0  Offset16 glyphAssemblyOffset (relative to this table; 0 = none)
//	  2  uint16   variantCount
//	  4  MathGlyphVariantRecord variants[variantCount]   // 4 bytes each
//	     uint16 variantGlyph
//	     uint16 advanceMeasurement
func parseMathGlyphConstruction(data []byte, off int) ([]MathVariant, *MathGlyphAssembly, error) {
	if off < 0 || off+4 > len(data) {
		return nil, nil, ErrInvalidOffset
	}
	b := data[off:]
	assemblyOff := int(binary.BigEndian.Uint16(b[0:]))
	count := int(binary.BigEndian.Uint16(b[2:]))
	if 4+count*4 > len(b) {
		return nil, nil, ErrInvalidOffset
	}
	variants := make([]MathVariant, count)
	for i := 0; i < count; i++ {
		variants[i] = MathVariant{
			GlyphID:   GlyphID(binary.BigEndian.Uint16(b[4+i*4:])),
			AdvanceFU: binary.BigEndian.Uint16(b[4+i*4+2:]),
		}
	}
	var assembly *MathGlyphAssembly
	if assemblyOff != 0 {
		a, err := parseGlyphAssembly(data, off+assemblyOff)
		if err != nil {
			return variants, nil, err
		}
		assembly = a
	}
	return variants, assembly, nil
}

// parseGlyphAssembly reads a GlyphAssembly sub-table:
//
//	  0  MathValueRecord italicsCorrection   (4 bytes; only value used)
//	  4  uint16          partCount
//	  6  GlyphPartRecord parts[partCount]     // 10 bytes each
//	     uint16 glyph
//	     uint16 startConnectorLength
//	     uint16 endConnectorLength
//	     uint16 fullAdvance
//	     uint16 partFlags                    // bit 0 = EXTENDER
func parseGlyphAssembly(data []byte, off int) (*MathGlyphAssembly, error) {
	if off < 0 || off+6 > len(data) {
		return nil, ErrInvalidOffset
	}
	b := data[off:]
	italic := int16(binary.BigEndian.Uint16(b[0:]))
	count := int(binary.BigEndian.Uint16(b[4:]))
	if 6+count*10 > len(b) {
		return nil, ErrInvalidOffset
	}
	parts := make([]MathGlyphPart, count)
	for i := 0; i < count; i++ {
		base := 6 + i*10
		flags := binary.BigEndian.Uint16(b[base+8:])
		parts[i] = MathGlyphPart{
			GlyphID:                GlyphID(binary.BigEndian.Uint16(b[base+0:])),
			StartConnectorLengthFU: binary.BigEndian.Uint16(b[base+2:]),
			EndConnectorLengthFU:   binary.BigEndian.Uint16(b[base+4:]),
			FullAdvanceFU:          binary.BigEndian.Uint16(b[base+6:]),
			IsExtender:             flags&1 != 0,
		}
	}
	return &MathGlyphAssembly{
		ItalicsCorrectionFU: italic,
		Parts:               parts,
	}, nil
}

// parseCoverage reads an OT Coverage table at the given offset and returns the
// glyph IDs it covers, in coverage-index order. Both Format 1 (glyph list) and
// Format 2 (range records) are supported — the math sub-tables index parallel
// arrays by coverage index, so we materialize the full list.
func parseCoverage(data []byte, off int) ([]GlyphID, error) {
	if off < 0 || off+4 > len(data) {
		return nil, ErrInvalidOffset
	}
	b := data[off:]
	format := binary.BigEndian.Uint16(b[0:])
	count := int(binary.BigEndian.Uint16(b[2:]))
	switch format {
	case 1:
		if 4+count*2 > len(b) {
			return nil, ErrInvalidOffset
		}
		out := make([]GlyphID, count)
		for i := 0; i < count; i++ {
			out[i] = GlyphID(binary.BigEndian.Uint16(b[4+i*2:]))
		}
		return out, nil
	case 2:
		if 4+count*6 > len(b) {
			return nil, ErrInvalidOffset
		}
		var out []GlyphID
		for i := 0; i < count; i++ {
			start := int(binary.BigEndian.Uint16(b[4+i*6:]))
			end := int(binary.BigEndian.Uint16(b[4+i*6+2:]))
			startIdx := int(binary.BigEndian.Uint16(b[4+i*6+4:]))
			for j := start; j <= end; j++ {
				idx := startIdx + (j - start)
				for len(out) <= idx {
					out = append(out, 0)
				}
				out[idx] = GlyphID(j)
			}
		}
		return out, nil
	default:
		return nil, ErrInvalidFormat
	}
}
