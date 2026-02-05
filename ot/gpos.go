package ot

// GPOS (Glyph Positioning) Table Implementation
//
// This file implements OpenType GPOS table parsing and application.
// HarfBuzz equivalent files:
//   - hb-ot-layout-gpos-table.hh (Table definitions, positioning logic)
//   - hb-ot-layout-gsubgpos.hh (Shared lookup application logic)
//
// GPOS Lookup Types implemented:
//   Type 1: SinglePos (Lines ~50-200)
//   Type 2: PairPos (Lines ~200-600)
//   Type 3: CursivePos (Lines ~600-800)
//   Type 4: MarkBasePos (Lines ~800-1200)
//   Type 5: MarkLigPos (Lines ~1200-1600)
//   Type 6: MarkMarkPos (Lines ~1600-1900)
//
// Key functions:
//   - PropagateAttachmentOffsets: Propagate cursive/mark offsets (Line ~2270)
//   - ApplyGPOS: Main GPOS application (Line ~1950)
//
// Direction types:
//   - Direction, DirectionLTR, DirectionRTL (Lines ~20-40)

import (
	"encoding/binary"
	"math"
	"sort"
)

// roundAnchor rounds an anchor coordinate value.
// HarfBuzz equivalent: roundf() in AnchorFormat*.get_anchor()
func roundAnchor(v float64) float64 {
	return math.Round(v)
}

// GPOS lookup types
const (
	GPOSTypeSingle       = 1
	GPOSTypePair         = 2
	GPOSTypeCursive      = 3
	GPOSTypeMarkBase     = 4
	GPOSTypeMarkLig      = 5
	GPOSTypeMarkMark     = 6
	GPOSTypeContext      = 7
	GPOSTypeChainContext = 8
	GPOSTypeExtension    = 9
)

// ValueFormat flags - determine which values are present in a ValueRecord
const (
	ValueFormatXPlacement = 0x0001 // Horizontal adjustment for placement
	ValueFormatYPlacement = 0x0002 // Vertical adjustment for placement
	ValueFormatXAdvance   = 0x0004 // Horizontal adjustment for advance
	ValueFormatYAdvance   = 0x0008 // Vertical adjustment for advance
	ValueFormatXPlaDevice = 0x0010 // Device table for horizontal placement
	ValueFormatYPlaDevice = 0x0020 // Device table for vertical placement
	ValueFormatXAdvDevice = 0x0040 // Device table for horizontal advance
	ValueFormatYAdvDevice = 0x0080 // Device table for vertical advance
)

// ValueRecord holds positioning values.
type ValueRecord struct {
	XPlacement int16 // Horizontal adjustment for placement
	YPlacement int16 // Vertical adjustment for placement
	XAdvance   int16 // Horizontal adjustment for advance
	YAdvance   int16 // Vertical adjustment for advance
}

// valueFormatLen returns the number of int16 values in a ValueRecord with the given format.
func valueFormatLen(format uint16) int {
	count := 0
	for f := format & 0xFF; f != 0; f >>= 1 {
		if f&1 != 0 {
			count++
		}
	}
	return count
}

// valueFormatSize returns the byte size of a ValueRecord with the given format.
func valueFormatSize(format uint16) int {
	return valueFormatLen(format) * 2
}

// parseValueRecord parses a ValueRecord from data.
func parseValueRecord(data []byte, offset int, format uint16) (ValueRecord, int) {
	var vr ValueRecord
	off := offset

	if format&ValueFormatXPlacement != 0 {
		vr.XPlacement = int16(binary.BigEndian.Uint16(data[off:]))
		off += 2
	}
	if format&ValueFormatYPlacement != 0 {
		vr.YPlacement = int16(binary.BigEndian.Uint16(data[off:]))
		off += 2
	}
	if format&ValueFormatXAdvance != 0 {
		vr.XAdvance = int16(binary.BigEndian.Uint16(data[off:]))
		off += 2
	}
	if format&ValueFormatYAdvance != 0 {
		vr.YAdvance = int16(binary.BigEndian.Uint16(data[off:]))
		off += 2
	}
	// Skip device tables (we don't support variable fonts yet)
	if format&ValueFormatXPlaDevice != 0 {
		off += 2
	}
	if format&ValueFormatYPlaDevice != 0 {
		off += 2
	}
	if format&ValueFormatXAdvDevice != 0 {
		off += 2
	}
	if format&ValueFormatYAdvDevice != 0 {
		off += 2
	}

	return vr, off - offset
}

// IsZero returns true if all values are zero.
func (vr *ValueRecord) IsZero() bool {
	return vr.XPlacement == 0 && vr.YPlacement == 0 &&
		vr.XAdvance == 0 && vr.YAdvance == 0
}

// GPOS represents the Glyph Positioning table.
type GPOS struct {
	data        []byte
	version     uint32
	scriptList  uint16
	featureList uint16
	lookupList  uint16

	lookups []*GPOSLookup

	// FeatureVariations (GPOS version 1.1+ only)
	// HarfBuzz: hb-ot-layout-common.hh - struct GSUBGPOS
	featureVariations *FeatureVariations
}

// ParseGPOS parses a GPOS table from data.
func ParseGPOS(data []byte) (*GPOS, error) {
	if len(data) < 10 {
		return nil, ErrInvalidTable
	}

	p := NewParser(data)

	major, _ := p.U16()
	minor, _ := p.U16()
	version := uint32(major)<<16 | uint32(minor)

	if major != 1 || (minor != 0 && minor != 1) {
		return nil, ErrInvalidFormat
	}

	scriptList, _ := p.U16()
	featureList, _ := p.U16()
	lookupList, _ := p.U16()

	gpos := &GPOS{
		data:        data,
		version:     version,
		scriptList:  scriptList,
		featureList: featureList,
		lookupList:  lookupList,
	}

	if err := gpos.parseLookupList(); err != nil {
		return nil, err
	}

	// Parse FeatureVariations for version 1.1+
	// HarfBuzz: hb-ot-layout-common.hh - GSUBGPOS::get_feature_variations()
	if version >= 0x00010001 && len(data) >= 14 {
		fvOffset := binary.BigEndian.Uint32(data[10:])
		if fvOffset != 0 && int(fvOffset) < len(data) {
			gpos.featureVariations, _ = ParseFeatureVariations(data, int(fvOffset))
		}
	}

	return gpos, nil
}

func (g *GPOS) parseLookupList() error {
	off := int(g.lookupList)
	if off+2 > len(g.data) {
		return ErrInvalidOffset
	}

	lookupCount := int(binary.BigEndian.Uint16(g.data[off:]))
	if off+2+lookupCount*2 > len(g.data) {
		return ErrInvalidOffset
	}

	g.lookups = make([]*GPOSLookup, lookupCount)

	for i := 0; i < lookupCount; i++ {
		lookupOff := int(binary.BigEndian.Uint16(g.data[off+2+i*2:]))
		lookup, err := parseGPOSLookup(g.data, off+lookupOff)
		if err != nil {
			continue
		}
		g.lookups[i] = lookup
	}

	return nil
}

// NumLookups returns the number of lookups.
func (g *GPOS) NumLookups() int {
	return len(g.lookups)
}

// GetLookup returns the lookup at the given index.
func (g *GPOS) GetLookup(index int) *GPOSLookup {
	if index < 0 || index >= len(g.lookups) {
		return nil
	}
	return g.lookups[index]
}

// FindVariationsIndex finds the matching FeatureVariations record index for the
// given normalized coordinates (in F2DOT14 format).
// Returns VariationsNotFoundIndex if no record matches or no FeatureVariations table exists.
// HarfBuzz: hb_ot_layout_table_find_feature_variations() in hb-ot-layout.cc
func (g *GPOS) FindVariationsIndex(coords []int) uint32 {
	if g.featureVariations == nil {
		return VariationsNotFoundIndex
	}
	return g.featureVariations.FindIndex(coords)
}

// GetFeatureVariations returns the FeatureVariations table, or nil if not present.
func (g *GPOS) GetFeatureVariations() *FeatureVariations {
	return g.featureVariations
}

// GPOSLookup represents a GPOS lookup table.
type GPOSLookup struct {
	Type       uint16
	Flag       uint16
	subtables  []GPOSSubtable
	MarkFilter uint16
}

// GPOSSubtable is the interface for GPOS lookup subtables.
type GPOSSubtable interface {
	// Apply applies the positioning to the glyphs at the current position.
	// Returns true if positioning was applied.
	Apply(ctx *OTApplyContext) bool
}

// Subtables returns the subtables for this lookup.
func (l *GPOSLookup) Subtables() []GPOSSubtable {
	return l.subtables
}

func parseGPOSLookup(data []byte, offset int) (*GPOSLookup, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookupType := binary.BigEndian.Uint16(data[offset:])
	lookupFlag := binary.BigEndian.Uint16(data[offset+2:])
	subtableCount := int(binary.BigEndian.Uint16(data[offset+4:]))

	if offset+6+subtableCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookup := &GPOSLookup{
		Type:      lookupType,
		Flag:      lookupFlag,
		subtables: make([]GPOSSubtable, 0, subtableCount),
	}

	// Check for MarkFilteringSet
	markFilterOff := 6 + subtableCount*2
	if lookupFlag&0x0010 != 0 {
		if offset+markFilterOff+2 > len(data) {
			return nil, ErrInvalidOffset
		}
		lookup.MarkFilter = binary.BigEndian.Uint16(data[offset+markFilterOff:])
	}

	for i := 0; i < subtableCount; i++ {
		subtableOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		actualType := lookupType

		// Handle extension lookups
		if lookupType == GPOSTypeExtension {
			extOff := offset + subtableOff
			if extOff+8 > len(data) {
				continue
			}
			extFormat := binary.BigEndian.Uint16(data[extOff:])
			if extFormat != 1 {
				continue
			}
			actualType = binary.BigEndian.Uint16(data[extOff+2:])
			extOffset := binary.BigEndian.Uint32(data[extOff+4:])
			subtableOff += int(extOffset)
		}

		subtable, err := parseGPOSSubtable(data, offset+subtableOff, actualType)
		if err != nil {
			continue
		}
		if subtable != nil {
			lookup.subtables = append(lookup.subtables, subtable)
		}
	}

	return lookup, nil
}

func parseGPOSSubtable(data []byte, offset int, lookupType uint16) (GPOSSubtable, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	switch lookupType {
	case GPOSTypeSingle:
		return parseSinglePos(data, offset)
	case GPOSTypePair:
		return parsePairPos(data, offset)
	case GPOSTypeCursive:
		return parseCursivePos(data, offset)
	case GPOSTypeMarkBase:
		return parseMarkBasePos(data, offset)
	case GPOSTypeMarkLig:
		return parseMarkLigPos(data, offset)
	case GPOSTypeMarkMark:
		return parseMarkMarkPos(data, offset)
	case GPOSTypeContext:
		return parseContextPos(data, offset)
	case GPOSTypeChainContext:
		return parseChainContextPos(data, offset)
	default:
		return nil, nil
	}
}

// --- Single Positioning ---

// SinglePos represents a Single Positioning subtable.
type SinglePos struct {
	format       uint16
	coverage     *Coverage
	valueFormat  uint16
	valueRecord  ValueRecord   // Format 1: single value for all
	valueRecords []ValueRecord // Format 2: per-glyph values
}

func parseSinglePos(data []byte, offset int) (*SinglePos, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	valueFormat := binary.BigEndian.Uint16(data[offset+4:])

	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	sp := &SinglePos{
		format:      format,
		coverage:    coverage,
		valueFormat: valueFormat,
	}

	switch format {
	case 1:
		// Single ValueRecord for all glyphs
		vr, _ := parseValueRecord(data, offset+6, valueFormat)
		sp.valueRecord = vr
		return sp, nil

	case 2:
		// Per-glyph ValueRecords
		valueCount := int(binary.BigEndian.Uint16(data[offset+6:]))
		vrSize := valueFormatSize(valueFormat)
		if offset+8+valueCount*vrSize > len(data) {
			return nil, ErrInvalidOffset
		}

		sp.valueRecords = make([]ValueRecord, valueCount)
		off := offset + 8
		for i := 0; i < valueCount; i++ {
			vr, size := parseValueRecord(data, off, valueFormat)
			sp.valueRecords[i] = vr
			off += size
		}
		return sp, nil

	default:
		return nil, ErrInvalidFormat
	}
}

// Apply applies single positioning.
func (sp *SinglePos) Apply(ctx *OTApplyContext) bool {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	coverageIndex := sp.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return false
	}

	var vr *ValueRecord
	switch sp.format {
	case 1:
		vr = &sp.valueRecord
	case 2:
		if int(coverageIndex) >= len(sp.valueRecords) {
			return false
		}
		vr = &sp.valueRecords[coverageIndex]
	default:
		return false
	}

	ctx.AdjustPosition(ctx.Buffer.Idx, vr)
	ctx.Buffer.Idx++
	return true
}

// Coverage returns the coverage table for this subtable.
func (sp *SinglePos) Coverage() *Coverage {
	return sp.coverage
}

// Format returns the subtable format (1 or 2).
func (sp *SinglePos) Format() uint16 {
	return sp.format
}

// ValueFormat returns the value format flags.
func (sp *SinglePos) ValueFormat() uint16 {
	return sp.valueFormat
}

// ValueRecord returns the value record (format 1 only).
func (sp *SinglePos) ValueRecord() ValueRecord {
	return sp.valueRecord
}

// ValueRecords returns the per-glyph value records (format 2 only).
func (sp *SinglePos) ValueRecords() []ValueRecord {
	return sp.valueRecords
}

// --- Pair Positioning (Kerning) ---

// PairPos represents a Pair Positioning subtable.
type PairPos struct {
	format       uint16
	coverage     *Coverage
	valueFormat1 uint16
	valueFormat2 uint16

	// Format 1: per-glyph pair sets
	pairSets [][]PairValueRecord

	// Format 2: class-based
	classDef1   *ClassDef
	classDef2   *ClassDef
	class1Count uint16
	class2Count uint16
	classMatrix [][]PairClassRecord // [class1][class2]
}

// PairValueRecord holds a pair of glyphs and their positioning values.
type PairValueRecord struct {
	SecondGlyph GlyphID
	Value1      ValueRecord
	Value2      ValueRecord
}

// PairClassRecord holds positioning values for a class pair.
type PairClassRecord struct {
	Value1 ValueRecord
	Value2 ValueRecord
}

func parsePairPos(data []byte, offset int) (*PairPos, error) {
	if offset+8 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	valueFormat1 := binary.BigEndian.Uint16(data[offset+4:])
	valueFormat2 := binary.BigEndian.Uint16(data[offset+6:])

	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	pp := &PairPos{
		format:       format,
		coverage:     coverage,
		valueFormat1: valueFormat1,
		valueFormat2: valueFormat2,
	}

	switch format {
	case 1:
		return parsePairPosFormat1(data, offset, pp)
	case 2:
		return parsePairPosFormat2(data, offset, pp)
	default:
		return nil, ErrInvalidFormat
	}
}

func parsePairPosFormat1(data []byte, offset int, pp *PairPos) (*PairPos, error) {
	if offset+10 > len(data) {
		return nil, ErrInvalidOffset
	}

	pairSetCount := int(binary.BigEndian.Uint16(data[offset+8:]))
	if offset+10+pairSetCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	pp.pairSets = make([][]PairValueRecord, pairSetCount)
	recordSize := 2 + valueFormatSize(pp.valueFormat1) + valueFormatSize(pp.valueFormat2)

	for i := 0; i < pairSetCount; i++ {
		pairSetOff := int(binary.BigEndian.Uint16(data[offset+10+i*2:]))
		absOff := offset + pairSetOff

		if absOff+2 > len(data) {
			continue
		}
		pairCount := int(binary.BigEndian.Uint16(data[absOff:]))
		if absOff+2+pairCount*recordSize > len(data) {
			continue
		}

		records := make([]PairValueRecord, pairCount)
		off := absOff + 2
		for j := 0; j < pairCount; j++ {
			records[j].SecondGlyph = GlyphID(binary.BigEndian.Uint16(data[off:]))
			off += 2
			records[j].Value1, _ = parseValueRecord(data, off, pp.valueFormat1)
			off += valueFormatSize(pp.valueFormat1)
			records[j].Value2, _ = parseValueRecord(data, off, pp.valueFormat2)
			off += valueFormatSize(pp.valueFormat2)
		}
		pp.pairSets[i] = records
	}

	return pp, nil
}

func parsePairPosFormat2(data []byte, offset int, pp *PairPos) (*PairPos, error) {
	if offset+16 > len(data) {
		return nil, ErrInvalidOffset
	}

	classDef1Off := int(binary.BigEndian.Uint16(data[offset+8:]))
	classDef2Off := int(binary.BigEndian.Uint16(data[offset+10:]))
	class1Count := binary.BigEndian.Uint16(data[offset+12:])
	class2Count := binary.BigEndian.Uint16(data[offset+14:])

	classDef1, err := ParseClassDef(data, offset+classDef1Off)
	if err != nil {
		return nil, err
	}

	classDef2, err := ParseClassDef(data, offset+classDef2Off)
	if err != nil {
		return nil, err
	}

	pp.classDef1 = classDef1
	pp.classDef2 = classDef2
	pp.class1Count = class1Count
	pp.class2Count = class2Count

	// Parse class matrix
	recordSize := valueFormatSize(pp.valueFormat1) + valueFormatSize(pp.valueFormat2)
	matrixSize := int(class1Count) * int(class2Count) * recordSize
	if offset+16+matrixSize > len(data) {
		return nil, ErrInvalidOffset
	}

	pp.classMatrix = make([][]PairClassRecord, class1Count)
	off := offset + 16
	for c1 := 0; c1 < int(class1Count); c1++ {
		pp.classMatrix[c1] = make([]PairClassRecord, class2Count)
		for c2 := 0; c2 < int(class2Count); c2++ {
			pp.classMatrix[c1][c2].Value1, _ = parseValueRecord(data, off, pp.valueFormat1)
			off += valueFormatSize(pp.valueFormat1)
			pp.classMatrix[c1][c2].Value2, _ = parseValueRecord(data, off, pp.valueFormat2)
			off += valueFormatSize(pp.valueFormat2)
		}
	}

	return pp, nil
}

// Apply applies pair positioning (kerning).
// HarfBuzz: PairPosFormat1/PairPosFormat2::apply in GPOS/PairPos*.hh
// Uses skippy_iter (iter_input) to find the second glyph, skipping marks/bases/ligatures
// per the lookup flag. This is critical for correct kerning in fonts where marks
// (or even space with mark class) appear between kerning pairs.
func (pp *PairPos) Apply(ctx *OTApplyContext) bool {
	if ctx.Buffer.Idx+1 >= len(ctx.Buffer.Info) {
		return false
	}

	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	coverageIndex := pp.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return false
	}

	// Find the next non-skippable glyph (HarfBuzz: skippy_iter.next())
	// This skips glyphs based on the lookup flag (IgnoreMarks, IgnoreBaseGlyphs, etc.)
	nextIdx := -1
	for j := ctx.Buffer.Idx + 1; j < len(ctx.Buffer.Info); j++ {
		if shouldSkipGlyph(ctx.Buffer.Info[j].GlyphID, ctx.LookupFlag, ctx.GDEF, ctx.MarkFilteringSet) {
			continue
		}
		// Also skip default ignorables (HarfBuzz: may_skip checks is_default_ignorable)
		if ctx.Buffer.Info[j].GlyphProps&GlyphPropsDefaultIgnorable != 0 &&
			ctx.Buffer.Info[j].GlyphProps&GlyphPropsSubstituted == 0 {
			continue
		}
		nextIdx = j
		break
	}
	if nextIdx < 0 {
		return false
	}

	nextGlyph := ctx.Buffer.Info[nextIdx].GlyphID

	switch pp.format {
	case 1:
		return pp.applyFormat1(ctx, coverageIndex, nextGlyph, nextIdx)
	case 2:
		return pp.applyFormat2(ctx, glyph, nextGlyph, nextIdx)
	default:
		return false
	}
}

func (pp *PairPos) applyFormat1(ctx *OTApplyContext, coverageIndex uint32, nextGlyph GlyphID, nextIdx int) bool {
	if int(coverageIndex) >= len(pp.pairSets) {
		return false
	}

	pairSet := pp.pairSets[coverageIndex]

	// Binary search for second glyph
	idx := sort.Search(len(pairSet), func(i int) bool {
		return pairSet[i].SecondGlyph >= nextGlyph
	})

	if idx >= len(pairSet) || pairSet[idx].SecondGlyph != nextGlyph {
		return false
	}

	record := &pairSet[idx]
	ctx.AdjustPosition(ctx.Buffer.Idx, &record.Value1)
	ctx.AdjustPosition(nextIdx, &record.Value2)

	// Advance past the second glyph (HarfBuzz: buffer->idx = skippy_iter.idx)
	if pp.valueFormat2 != 0 {
		ctx.Buffer.Idx = nextIdx + 1
	} else {
		ctx.Buffer.Idx = nextIdx
	}
	return true
}

func (pp *PairPos) applyFormat2(ctx *OTApplyContext, glyph, nextGlyph GlyphID, nextIdx int) bool {
	class1 := pp.classDef1.GetClass(glyph)
	class2 := pp.classDef2.GetClass(nextGlyph)

	if class1 >= int(pp.class1Count) || class2 >= int(pp.class2Count) {
		return false
	}

	record := &pp.classMatrix[class1][class2]
	ctx.AdjustPosition(ctx.Buffer.Idx, &record.Value1)
	ctx.AdjustPosition(nextIdx, &record.Value2)

	// HarfBuzz: PairPosFormat2 ALWAYS returns true and advances buffer.idx
	// to skippy_iter.idx (the second glyph's position).
	// Even when values are all zero, the pair is "consumed" and buffer advances.
	if pp.valueFormat2 != 0 {
		ctx.Buffer.Idx = nextIdx + 1
	} else {
		ctx.Buffer.Idx = nextIdx
	}
	return true
}

// Coverage returns the coverage table for this subtable.
func (pp *PairPos) Coverage() *Coverage {
	return pp.coverage
}

// Format returns the subtable format (1 or 2).
func (pp *PairPos) Format() uint16 {
	return pp.format
}

// ValueFormat1 returns the value format for the first glyph.
func (pp *PairPos) ValueFormat1() uint16 {
	return pp.valueFormat1
}

// ValueFormat2 returns the value format for the second glyph.
func (pp *PairPos) ValueFormat2() uint16 {
	return pp.valueFormat2
}

// PairSets returns the pair sets (format 1 only).
func (pp *PairPos) PairSets() [][]PairValueRecord {
	return pp.pairSets
}

// ClassDef1 returns the class definition for first glyphs (format 2 only).
func (pp *PairPos) ClassDef1() *ClassDef {
	return pp.classDef1
}

// ClassDef2 returns the class definition for second glyphs (format 2 only).
func (pp *PairPos) ClassDef2() *ClassDef {
	return pp.classDef2
}

// Class1Count returns the number of classes for first glyphs (format 2 only).
func (pp *PairPos) Class1Count() uint16 {
	return pp.class1Count
}

// Class2Count returns the number of classes for second glyphs (format 2 only).
func (pp *PairPos) Class2Count() uint16 {
	return pp.class2Count
}

// ClassMatrix returns the class matrix (format 2 only).
func (pp *PairPos) ClassMatrix() [][]PairClassRecord {
	return pp.classMatrix
}

// --- Cursive Attachment (Type 3) ---

// LookupFlag bit constants for GSUB/GPOS lookups.
const (
	// LookupFlagRightToLeft indicates right-to-left cursive attachment.
	LookupFlagRightToLeft = 0x0001
	// LookupFlagIgnoreBaseGlyphs causes base glyphs to be skipped.
	LookupFlagIgnoreBaseGlyphs = 0x0002
	// LookupFlagIgnoreLigatures causes ligature glyphs to be skipped.
	LookupFlagIgnoreLigatures = 0x0004
	// LookupFlagIgnoreMarks causes mark glyphs to be skipped.
	LookupFlagIgnoreMarks = 0x0008
	// LookupFlagUseMarkFilteringSet indicates that MarkFilteringSet is used.
	LookupFlagUseMarkFilteringSet = 0x0010
	// LookupFlagMarkAttachTypeMask is the mask for mark attachment type filtering.
	LookupFlagMarkAttachTypeMask = 0xFF00
)

// EntryExitRecord holds entry and exit anchors for cursive attachment.
type EntryExitRecord struct {
	EntryAnchor *Anchor // May be nil
	ExitAnchor  *Anchor // May be nil
}

// CursivePos represents a Cursive Attachment subtable (GPOS Type 3).
// It connects glyphs in cursive scripts (like Arabic) by aligning
// exit anchors with entry anchors of adjacent glyphs.
type CursivePos struct {
	format           uint16
	coverage         *Coverage
	entryExitRecords []EntryExitRecord
}

func parseCursivePos(data []byte, offset int) (*CursivePos, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	entryExitCount := int(binary.BigEndian.Uint16(data[offset+4:]))

	if offset+6+entryExitCount*4 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	cp := &CursivePos{
		format:           format,
		coverage:         coverage,
		entryExitRecords: make([]EntryExitRecord, entryExitCount),
	}

	for i := 0; i < entryExitCount; i++ {
		recOff := offset + 6 + i*4
		entryOff := int(binary.BigEndian.Uint16(data[recOff:]))
		exitOff := int(binary.BigEndian.Uint16(data[recOff+2:]))

		var entryAnchor, exitAnchor *Anchor
		if entryOff != 0 {
			entryAnchor, _ = parseAnchor(data, offset+entryOff)
		}
		if exitOff != 0 {
			exitAnchor, _ = parseAnchor(data, offset+exitOff)
		}

		cp.entryExitRecords[i] = EntryExitRecord{
			EntryAnchor: entryAnchor,
			ExitAnchor:  exitAnchor,
		}
	}

	return cp, nil
}

// reverseCursiveMinorOffset reverses the attachment chain when a child is
// reconnected to a new parent.
// HarfBuzz equivalent: reverse_cursive_minor_offset() in CursivePosFormat1.hh:53-77
//
// When child was already connected to someone else, this walks through its old
// chain and reverses the link direction, such that the whole tree of its
// previous connection now attaches to new parent.
func reverseCursiveMinorOffset(pos []GlyphPos, i int, direction Direction, newParent int) {
	chain := pos[i].AttachChain
	attachType := pos[i].AttachType
	if chain == 0 || (attachType&AttachTypeCursive) == 0 {
		return
	}

	pos[i].AttachChain = 0

	j := i + int(chain)
	if j < 0 || j >= len(pos) {
		return
	}

	// Stop if we see new parent in the chain
	if j == newParent {
		return
	}

	reverseCursiveMinorOffset(pos, j, direction, newParent)

	// Reverse the offset direction
	if direction.IsHorizontal() {
		pos[j].YOffset = -pos[i].YOffset
	} else {
		pos[j].XOffset = -pos[i].XOffset
	}

	// Reverse the chain direction
	pos[j].AttachChain = -chain
	pos[j].AttachType = attachType
}

// Apply applies cursive positioning.
// It connects the current glyph's entry anchor to the previous glyph's exit anchor.
func (cp *CursivePos) Apply(ctx *OTApplyContext) bool {
	// Check if current glyph is in coverage
	thisIndex := cp.coverage.GetCoverage(ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID)
	if thisIndex == NotCovered {
		return false
	}

	if int(thisIndex) >= len(cp.entryExitRecords) {
		return false
	}

	thisRecord := &cp.entryExitRecords[thisIndex]

	// Current glyph must have an entry anchor
	if thisRecord.EntryAnchor == nil {
		return false
	}

	// Go to the immediately previous glyph (skip according to LookupFlag)
	// HarfBuzz: skippy_iter.prev() - only ONE step back, no loop!
	// CursivePosFormat1.hh:134-141
	prevIdx := ctx.PrevGlyph(ctx.Buffer.Idx)
	if prevIdx < 0 {
		return false
	}

	// Check if previous glyph is in coverage and has an exit anchor
	// HarfBuzz: CursivePosFormat1.hh:143-149
	prevCovIndex := cp.coverage.GetCoverage(ctx.Buffer.Info[prevIdx].GlyphID)
	if prevCovIndex == NotCovered || int(prevCovIndex) >= len(cp.entryExitRecords) {
		return false
	}
	if cp.entryExitRecords[prevCovIndex].ExitAnchor == nil {
		return false
	}
	prevRecord := &cp.entryExitRecords[prevCovIndex]

	i := prevIdx
	j := ctx.Buffer.Idx

	// Get anchor coordinates
	entryX := int32(thisRecord.EntryAnchor.X)
	entryY := int32(thisRecord.EntryAnchor.Y)
	exitX := int32(prevRecord.ExitAnchor.X)
	exitY := int32(prevRecord.ExitAnchor.Y)

	// Main-direction adjustment (affects advance widths)
	switch ctx.Buffer.Direction {
	case DirectionLTR:
		// In LTR, previous glyph's advance is set to exit anchor X
		ctx.Buffer.Pos[i].XAdvance = int16(exitX) + ctx.Buffer.Pos[i].XOffset

		// Current glyph's advance and offset are adjusted by entry anchor X
		d := int16(entryX) + ctx.Buffer.Pos[j].XOffset
		ctx.Buffer.Pos[j].XAdvance -= d
		ctx.Buffer.Pos[j].XOffset -= d

	case DirectionRTL:
		// In RTL, previous glyph's advance and offset are adjusted by exit anchor X
		d := int16(exitX) + ctx.Buffer.Pos[i].XOffset
		ctx.Buffer.Pos[i].XAdvance -= d
		ctx.Buffer.Pos[i].XOffset -= d

		// Current glyph's advance is set to entry anchor X
		ctx.Buffer.Pos[j].XAdvance = int16(entryX) + ctx.Buffer.Pos[j].XOffset

	case DirectionTTB:
		// In TTB, previous glyph's advance is set to exit anchor Y
		ctx.Buffer.Pos[i].YAdvance = int16(exitY) + ctx.Buffer.Pos[i].YOffset

		// Current glyph's advance and offset are adjusted by entry anchor Y
		d := int16(entryY) + ctx.Buffer.Pos[j].YOffset
		ctx.Buffer.Pos[j].YAdvance -= d
		ctx.Buffer.Pos[j].YOffset -= d

	case DirectionBTT:
		// In BTT, previous glyph's advance and offset are adjusted by exit anchor Y
		d := int16(exitY) + ctx.Buffer.Pos[i].YOffset
		ctx.Buffer.Pos[i].YAdvance -= d
		ctx.Buffer.Pos[i].YOffset -= d

		// Current glyph's advance is set to entry anchor Y
		ctx.Buffer.Pos[j].YAdvance = int16(entryY)
	}

	// Cross-direction adjustment
	// We attach child to parent (child moves, parent stays on baseline)
	// RightToLeft flag determines which glyph is the child
	child := i
	parent := j
	xOffset := int16(entryX - exitX)
	yOffset := int16(entryY - exitY)

	if ctx.LookupFlag&LookupFlagRightToLeft == 0 {
		// Not RTL: swap child and parent
		child, parent = parent, child
		xOffset = -xOffset
		yOffset = -yOffset
	}

	// If child was already connected to someone else (i.e., it has a prior
	// attachment chain), reverse that chain so the whole tree attaches to
	// the new parent. HarfBuzz: reverse_cursive_minor_offset()
	reverseCursiveMinorOffset(ctx.Buffer.Pos, child, ctx.Buffer.Direction, parent)

	// Set up the attachment chain
	ctx.Buffer.Pos[child].AttachChain = int16(parent - child)
	ctx.Buffer.Pos[child].AttachType = AttachTypeCursive

	// Apply cross-direction offset
	if ctx.Buffer.Direction.IsHorizontal() {
		ctx.Buffer.Pos[child].YOffset = yOffset
	} else {
		ctx.Buffer.Pos[child].XOffset = xOffset
	}

	// If parent was attached to child, break that attachment
	if ctx.Buffer.Pos[parent].AttachChain == -ctx.Buffer.Pos[child].AttachChain {
		ctx.Buffer.Pos[parent].AttachChain = 0
		if ctx.Buffer.Direction.IsHorizontal() {
			ctx.Buffer.Pos[parent].YOffset = 0
		} else {
			ctx.Buffer.Pos[parent].XOffset = 0
		}
	}

	ctx.Buffer.Idx++
	return true
}

// Coverage returns the coverage table for this subtable.
func (cp *CursivePos) Coverage() *Coverage {
	return cp.coverage
}

// EntryExitRecords returns the entry/exit anchor records.
func (cp *CursivePos) EntryExitRecords() []EntryExitRecord {
	return cp.entryExitRecords
}

// --- ClassDef ---

// ClassDef maps glyph IDs to class values.
type ClassDef struct {
	format uint16
	data   []byte
	offset int

	// Format 1: range starting at startGlyph
	startGlyph  GlyphID
	classValues []uint16

	// Format 2: class ranges
	classRanges []classRange
}

type classRange struct {
	startGlyph GlyphID
	endGlyph   GlyphID
	class      uint16
}

// ParseClassDef parses a ClassDef table.
func ParseClassDef(data []byte, offset int) (*ClassDef, error) {
	if offset+4 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])

	cd := &ClassDef{
		format: format,
		data:   data,
		offset: offset,
	}

	switch format {
	case 1:
		startGlyph := binary.BigEndian.Uint16(data[offset+2:])
		glyphCount := int(binary.BigEndian.Uint16(data[offset+4:]))
		if offset+6+glyphCount*2 > len(data) {
			return nil, ErrInvalidOffset
		}

		cd.startGlyph = GlyphID(startGlyph)
		cd.classValues = make([]uint16, glyphCount)
		for i := 0; i < glyphCount; i++ {
			cd.classValues[i] = binary.BigEndian.Uint16(data[offset+6+i*2:])
		}
		return cd, nil

	case 2:
		rangeCount := int(binary.BigEndian.Uint16(data[offset+2:]))
		if offset+4+rangeCount*6 > len(data) {
			return nil, ErrInvalidOffset
		}

		cd.classRanges = make([]classRange, rangeCount)
		for i := 0; i < rangeCount; i++ {
			off := offset + 4 + i*6
			cd.classRanges[i] = classRange{
				startGlyph: GlyphID(binary.BigEndian.Uint16(data[off:])),
				endGlyph:   GlyphID(binary.BigEndian.Uint16(data[off+2:])),
				class:      binary.BigEndian.Uint16(data[off+4:]),
			}
		}
		return cd, nil

	default:
		return nil, ErrInvalidFormat
	}
}

// GetClass returns the class for a glyph ID.
// Returns 0 (default class) if glyph not found.
func (cd *ClassDef) GetClass(glyph GlyphID) int {
	switch cd.format {
	case 1:
		idx := int(glyph) - int(cd.startGlyph)
		if idx >= 0 && idx < len(cd.classValues) {
			return int(cd.classValues[idx])
		}
		return 0

	case 2:
		// Binary search
		idx := sort.Search(len(cd.classRanges), func(i int) bool {
			return cd.classRanges[i].endGlyph >= glyph
		})
		if idx < len(cd.classRanges) {
			r := &cd.classRanges[idx]
			if glyph >= r.startGlyph && glyph <= r.endGlyph {
				return int(r.class)
			}
		}
		return 0

	default:
		return 0
	}
}

// Mapping returns a map from glyph ID to class for all glyphs in this ClassDef.
func (cd *ClassDef) Mapping() map[GlyphID]uint16 {
	result := make(map[GlyphID]uint16)

	switch cd.format {
	case 1:
		for i, class := range cd.classValues {
			if class != 0 { // Skip class 0 (default)
				glyph := GlyphID(int(cd.startGlyph) + i)
				result[glyph] = class
			}
		}
	case 2:
		for _, r := range cd.classRanges {
			for g := r.startGlyph; g <= r.endGlyph; g++ {
				if r.class != 0 { // Skip class 0 (default)
					result[g] = r.class
				}
			}
		}
	}

	return result
}

// --- Direction ---

// Direction represents text direction.
// HarfBuzz: hb_direction_t in hb-common.h:238-242
// Values use bit-encoding for fast checks:
//   - Bit 2 (0x04): always set for valid directions
//   - Bit 1 (0x02): vertical (0=horizontal, 1=vertical)
//   - Bit 0 (0x01): backward (0=forward, 1=backward)
type Direction int

const (
	DirectionInvalid Direction = 0 // HB_DIRECTION_INVALID
	DirectionLTR     Direction = 4 // HB_DIRECTION_LTR (horizontal, forward)
	DirectionRTL     Direction = 5 // HB_DIRECTION_RTL (horizontal, backward)
	DirectionTTB     Direction = 6 // HB_DIRECTION_TTB (vertical, forward)
	DirectionBTT     Direction = 7 // HB_DIRECTION_BTT (vertical, backward)
)

// IsValid returns true if the direction is valid (LTR, RTL, TTB, or BTT).
// HarfBuzz: HB_DIRECTION_IS_VALID(dir) = ((dir & ~3U) == 4)
func (d Direction) IsValid() bool {
	return (int(d) & ^3) == 4
}

// IsHorizontal returns true if the direction is horizontal (LTR or RTL).
// HarfBuzz: HB_DIRECTION_IS_HORIZONTAL(dir) = ((dir & ~1U) == 4)
func (d Direction) IsHorizontal() bool {
	return (int(d) & ^1) == 4
}

// IsVertical returns true if the direction is vertical (TTB or BTT).
// HarfBuzz: HB_DIRECTION_IS_VERTICAL(dir) = ((dir & ~1U) == 6)
func (d Direction) IsVertical() bool {
	return (int(d) & ^1) == 6
}

// IsForward returns true if the direction is forward (LTR or TTB).
// HarfBuzz: HB_DIRECTION_IS_FORWARD(dir) = ((dir & ~2U) == 4)
func (d Direction) IsForward() bool {
	return (int(d) & ^2) == 4
}

// IsBackward returns true if the direction is backward (RTL or BTT).
// HarfBuzz: HB_DIRECTION_IS_BACKWARD(dir) = ((dir & ~2U) == 5)
func (d Direction) IsBackward() bool {
	return (int(d) & ^2) == 5
}

// Reverse returns the opposite direction (LTR↔RTL, TTB↔BTT).
// HarfBuzz: HB_DIRECTION_REVERSE(dir) = (dir ^ 1)
func (d Direction) Reverse() Direction {
	return Direction(int(d) ^ 1)
}

// shouldSkipGlyph is the core logic for determining if a glyph should be skipped.
func shouldSkipGlyph(glyph GlyphID, lookupFlag uint16, gdef *GDEF, markFilteringSet int) bool {
	// If no GDEF, we can only skip based on IgnoreMarks with no filtering
	if gdef == nil {
		return false
	}

	glyphClass := gdef.GetGlyphClass(glyph)

	// Check IgnoreBaseGlyphs
	if lookupFlag&LookupFlagIgnoreBaseGlyphs != 0 && glyphClass == GlyphClassBase {
		return true
	}

	// Check IgnoreLigatures
	if lookupFlag&LookupFlagIgnoreLigatures != 0 && glyphClass == GlyphClassLigature {
		return true
	}

	// Check IgnoreMarks
	if lookupFlag&LookupFlagIgnoreMarks != 0 && glyphClass == GlyphClassMark {
		return true
	}

	// Check mark attachment type filtering (bits 8-15)
	if glyphClass == GlyphClassMark {
		markAttachType := (lookupFlag & LookupFlagMarkAttachTypeMask) >> 8
		if markAttachType != 0 {
			// Only process marks with matching attachment class
			markClass := gdef.GetMarkAttachClass(glyph)
			if markClass != int(markAttachType) {
				return true
			}
		}
	}

	// Check mark filtering set
	if lookupFlag&LookupFlagUseMarkFilteringSet != 0 && glyphClass == GlyphClassMark {
		if markFilteringSet >= 0 && !gdef.IsInMarkGlyphSet(glyph, markFilteringSet) {
			return true
		}
	}

	return false
}

// --- Apply lookup ---

// ApplyLookupToBuffer applies a single lookup directly to a Buffer.
// HarfBuzz equivalent: apply() in hb-ot-layout-gsubgpos.hh
// Uses MaskGlobal so lookup applies to all glyphs (which have MaskGlobal by default).
func (g *GPOS) ApplyLookupToBuffer(lookupIndex int, buf *Buffer, gdef *GDEF, font *Font) {
	g.ApplyLookupToBufferWithMask(lookupIndex, buf, gdef, MaskGlobal, font)
}

// ApplyLookupToBufferWithMask applies a single lookup directly to a Buffer with mask-based filtering.
// HarfBuzz equivalent: apply() in hb-ot-layout-gsubgpos.hh with mask check in may_match()
//
// The featureMask parameter specifies which glyphs this lookup should apply to.
// A glyph is skipped if (glyph.Mask & featureMask) == 0.
// Use MaskGlobal to apply to all glyphs (which have MaskGlobal set by default).
func (g *GPOS) ApplyLookupToBufferWithMask(lookupIndex int, buf *Buffer, gdef *GDEF, featureMask uint32, font *Font) {
	lookup := g.GetLookup(lookupIndex)
	if lookup == nil {
		return
	}

	// Determine mark filtering set index
	markFilteringSet := -1
	if lookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
		markFilteringSet = int(lookup.MarkFilter)
	}

	ctx := &OTApplyContext{
		Buffer:           buf,
		LookupFlag:       lookup.Flag,
		GDEF:             gdef,
		HasGlyphClasses:  gdef != nil && gdef.HasGlyphClasses(),
		MarkFilteringSet: markFilteringSet,
		FeatureMask:      featureMask,
		Font:             font,
	}

	buf.Idx = 0
	for buf.Idx < len(buf.Info) {
		// Skip glyphs that should be ignored based on LookupFlag, GDEF, and FeatureMask
		if ctx.ShouldSkipGlyph(buf.Idx) {
			buf.Idx++
			continue
		}

		applied := false
		for _, subtable := range lookup.subtables {
			if subtable.Apply(ctx) {
				applied = true
				break
			}
		}
		if !applied {
			buf.Idx++
		}
	}
}

// ApplyLookup applies a single lookup to the glyph/position arrays.
// DEPRECATED: Use ApplyLookupToBuffer for new code.
// This version does not use GDEF for glyph filtering.
func (g *GPOS) ApplyLookup(lookupIndex int, glyphs []GlyphID, positions []GlyphPos) {
	g.ApplyLookupWithGDEF(lookupIndex, glyphs, positions, DirectionLTR, nil)
}

// ApplyLookupWithDirection applies a single lookup with the specified direction.
// DEPRECATED: Use ApplyLookupToBuffer for new code.
// This version does not use GDEF for glyph filtering.
func (g *GPOS) ApplyLookupWithDirection(lookupIndex int, glyphs []GlyphID, positions []GlyphPos, direction Direction) {
	g.ApplyLookupWithGDEF(lookupIndex, glyphs, positions, direction, nil)
}

// ApplyLookupWithGDEF applies a single lookup with GDEF-based glyph filtering.
// DEPRECATED: Use ApplyLookupToBuffer for new code.
func (g *GPOS) ApplyLookupWithGDEF(lookupIndex int, glyphs []GlyphID, positions []GlyphPos, direction Direction, gdef *GDEF) {
	// Create a temporary buffer for the operation
	buf := &Buffer{
		Info:      make([]GlyphInfo, len(glyphs)),
		Pos:       positions,
		Direction: direction,
	}
	for i, gid := range glyphs {
		buf.Info[i] = GlyphInfo{
			GlyphID: gid,
			Cluster: i,
			Mask:    MaskGlobal, // HarfBuzz: glyphs have global_mask
		}
	}

	g.ApplyLookupToBuffer(lookupIndex, buf, gdef, nil)
}

// ApplyKerning is a convenience method to apply pair positioning (kerning).
// DEPRECATED: Use ApplyLookupToBuffer for new code.
// This version does not use GDEF for glyph filtering.
func (g *GPOS) ApplyKerning(glyphs []GlyphID) []GlyphPos {
	return g.ApplyKerningWithGDEF(glyphs, nil)
}

// ApplyKerningWithGDEF applies pair positioning with GDEF-based glyph filtering.
// DEPRECATED: Use ApplyLookupToBuffer for new code.
func (g *GPOS) ApplyKerningWithGDEF(glyphs []GlyphID, gdef *GDEF) []GlyphPos {
	positions := make([]GlyphPos, len(glyphs))

	// Find and apply all PairPos lookups
	for i := 0; i < g.NumLookups(); i++ {
		lookup := g.GetLookup(i)
		if lookup != nil && lookup.Type == GPOSTypePair {
			g.ApplyLookupWithGDEF(i, glyphs, positions, DirectionLTR, gdef)
		}
	}

	return positions
}

// ParseFeatureList parses a FeatureList from a GPOS table.
func (g *GPOS) ParseFeatureList() (*FeatureList, error) {
	off := int(g.featureList)
	if off+2 > len(g.data) {
		return nil, ErrInvalidOffset
	}

	count := int(binary.BigEndian.Uint16(g.data[off:]))
	if off+2+count*6 > len(g.data) {
		return nil, ErrInvalidOffset
	}

	return &FeatureList{
		data:   g.data,
		offset: off,
		count:  count,
	}, nil
}

// ParseScriptList parses the ScriptList from a GPOS table.
func (g *GPOS) ParseScriptList() (*ScriptList, error) {
	off := int(g.scriptList)
	if off+2 > len(g.data) {
		return nil, ErrInvalidOffset
	}

	count := int(binary.BigEndian.Uint16(g.data[off:]))
	if off+2+count*6 > len(g.data) {
		return nil, ErrInvalidOffset
	}

	return &ScriptList{
		data:   g.data,
		offset: off,
		count:  count,
	}, nil
}

// Common GPOS feature tags
var (
	TagKern = MakeTag('k', 'e', 'r', 'n') // Kerning
	TagCurs = MakeTag('c', 'u', 'r', 's') // Cursive Positioning
	TagMark = MakeTag('m', 'a', 'r', 'k') // Mark Positioning
	TagMkmk = MakeTag('m', 'k', 'm', 'k') // Mark-to-Mark Positioning
	TagAbvm = MakeTag('a', 'b', 'v', 'm') // Above-base Mark Positioning
	TagBlwm = MakeTag('b', 'l', 'w', 'm') // Below-base Mark Positioning
	TagDist = MakeTag('d', 'i', 's', 't') // Distances
)

// --- Anchor ---

// Anchor represents an anchor point for mark positioning.
// It stores x,y coordinates in design units.
type Anchor struct {
	Format uint16
	X      int16 // X coordinate in design units
	Y      int16 // Y coordinate in design units
	// Format 2 adds: anchorPoint (contour point index)
	AnchorPoint uint16
	// Format 3 adds: device table offsets (not yet implemented)
}

// parseAnchor parses an Anchor table from data at the given offset.
func parseAnchor(data []byte, offset int) (*Anchor, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	x := int16(binary.BigEndian.Uint16(data[offset+2:]))
	y := int16(binary.BigEndian.Uint16(data[offset+4:]))

	anchor := &Anchor{
		Format: format,
		X:      x,
		Y:      y,
	}

	if format == 2 {
		if offset+8 > len(data) {
			return nil, ErrInvalidOffset
		}
		anchor.AnchorPoint = binary.BigEndian.Uint16(data[offset+6:])
	}
	// Format 3 with device tables could be added here

	return anchor, nil
}

// --- MarkRecord ---

// MarkRecord associates a mark glyph with a class and anchor.
type MarkRecord struct {
	Class  uint16  // Mark class
	Anchor *Anchor // Anchor for this mark
}

// --- MarkArray ---

// MarkArray contains an array of MarkRecords.
type MarkArray struct {
	Records []MarkRecord
}

// parseMarkArray parses a MarkArray table from data at the given offset.
func parseMarkArray(data []byte, offset int) (*MarkArray, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	count := int(binary.BigEndian.Uint16(data[offset:]))
	if offset+2+count*4 > len(data) {
		return nil, ErrInvalidOffset
	}

	ma := &MarkArray{
		Records: make([]MarkRecord, count),
	}

	for i := 0; i < count; i++ {
		recOff := offset + 2 + i*4
		class := binary.BigEndian.Uint16(data[recOff:])
		anchorOff := int(binary.BigEndian.Uint16(data[recOff+2:]))

		anchor, err := parseAnchor(data, offset+anchorOff)
		if err != nil {
			return nil, err
		}

		ma.Records[i] = MarkRecord{
			Class:  class,
			Anchor: anchor,
		}
	}

	return ma, nil
}

// --- BaseArray (AnchorMatrix) ---

// BaseArray contains anchors for base glyphs, organized as a matrix.
// Rows correspond to base glyphs (in BaseCoverage order).
// Columns correspond to mark classes (0 to classCount-1).
type BaseArray struct {
	Rows       int
	ClassCount int
	Anchors    [][]*Anchor // [row][class] -> Anchor (may be nil)
}

// parseBaseArray parses a BaseArray (AnchorMatrix) from data.
func parseBaseArray(data []byte, offset int, classCount int) (*BaseArray, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	rows := int(binary.BigEndian.Uint16(data[offset:]))
	totalAnchors := rows * classCount

	if offset+2+totalAnchors*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	ba := &BaseArray{
		Rows:       rows,
		ClassCount: classCount,
		Anchors:    make([][]*Anchor, rows),
	}

	for row := 0; row < rows; row++ {
		ba.Anchors[row] = make([]*Anchor, classCount)
		for col := 0; col < classCount; col++ {
			idx := row*classCount + col
			anchorOff := int(binary.BigEndian.Uint16(data[offset+2+idx*2:]))

			if anchorOff == 0 {
				// NULL offset - no anchor for this combination
				continue
			}

			anchor, err := parseAnchor(data, offset+anchorOff)
			if err != nil {
				// Skip invalid anchors
				continue
			}
			ba.Anchors[row][col] = anchor
		}
	}

	return ba, nil
}

// GetAnchor returns the anchor for a given base glyph index and mark class.
func (ba *BaseArray) GetAnchor(baseIndex, markClass int) *Anchor {
	if baseIndex < 0 || baseIndex >= ba.Rows {
		return nil
	}
	if markClass < 0 || markClass >= ba.ClassCount {
		return nil
	}
	return ba.Anchors[baseIndex][markClass]
}

// --- MarkBasePos ---

// MarkBasePos represents a Mark-to-Base Attachment subtable (GPOS Type 4).
// It positions mark glyphs relative to base glyphs using anchor points.
type MarkBasePos struct {
	format       uint16
	markCoverage *Coverage
	baseCoverage *Coverage
	classCount   uint16
	markArray    *MarkArray
	baseArray    *BaseArray
}

func parseMarkBasePos(data []byte, offset int) (*MarkBasePos, error) {
	if offset+12 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	markCoverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	baseCoverageOff := int(binary.BigEndian.Uint16(data[offset+4:]))
	classCount := binary.BigEndian.Uint16(data[offset+6:])
	markArrayOff := int(binary.BigEndian.Uint16(data[offset+8:]))
	baseArrayOff := int(binary.BigEndian.Uint16(data[offset+10:]))

	markCoverage, err := ParseCoverage(data, offset+markCoverageOff)
	if err != nil {
		return nil, err
	}

	baseCoverage, err := ParseCoverage(data, offset+baseCoverageOff)
	if err != nil {
		return nil, err
	}

	markArray, err := parseMarkArray(data, offset+markArrayOff)
	if err != nil {
		return nil, err
	}

	baseArray, err := parseBaseArray(data, offset+baseArrayOff, int(classCount))
	if err != nil {
		return nil, err
	}

	return &MarkBasePos{
		format:       format,
		markCoverage: markCoverage,
		baseCoverage: baseCoverage,
		classCount:   classCount,
		markArray:    markArray,
		baseArray:    baseArray,
	}, nil
}

// Apply applies the mark-to-base positioning.
// It finds the preceding base glyph and positions the current mark relative to it.
// acceptMarkBase checks if a glyph is acceptable as a base for mark attachment.
// HarfBuzz equivalent: MarkBasePosFormat1_2::accept() in OT/Layout/GPOS/MarkBasePosFormat1.hh:93-110
//
// We only want to attach to the first of a MultipleSubst sequence.
// https://github.com/harfbuzz/harfbuzz/issues/740
// Reject others... but stop if we find a mark in the MultipleSubst sequence:
// https://github.com/harfbuzz/harfbuzz/issues/1020
func acceptMarkBase(buf *Buffer, idx int) bool {
	info := &buf.Info[idx]

	// Not multiplied → accept
	if !info.IsMultiplied() {
		return true
	}

	// Multiplied but lig_comp == 0 → accept (first of sequence)
	if info.GetLigComp() == 0 {
		return true
	}

	// Check for sequence break conditions
	if idx == 0 {
		return true
	}

	prevInfo := &buf.Info[idx-1]

	// Previous is a mark → accept (sequence interrupted)
	if prevInfo.IsMark() {
		return true
	}

	// Previous is not multiplied → accept (sequence interrupted)
	if !prevInfo.IsMultiplied() {
		return true
	}

	// Different lig_id → accept (different sequence)
	if info.GetLigID() != prevInfo.GetLigID() {
		return true
	}

	// Not consecutive lig_comp → accept (sequence interrupted)
	if info.GetLigComp() != prevInfo.GetLigComp()+1 {
		return true
	}

	// All checks failed - this is a non-first glyph in an uninterrupted MultipleSubst sequence
	return false
}

// HarfBuzz equivalent: MarkBasePosFormat1_2::apply in OT/Layout/GPOS/MarkBasePosFormat1.hh
func (m *MarkBasePos) Apply(ctx *OTApplyContext) bool {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID

	// Check if current glyph is a mark
	markIndex := m.markCoverage.GetCoverage(glyph)

	if markIndex == NotCovered {
		return false
	}

	if int(markIndex) >= len(m.markArray.Records) {
		return false
	}

	// Search backwards for a non-mark glyph.
	// HarfBuzz equivalent: MarkBasePosFormat1.hh lines 119-152
	// Uses last_base/last_base_until cache to avoid O(n^2) behavior.

	// Reset cache if we've moved backwards in buffer
	// HarfBuzz: lines 125-129
	if ctx.LastBaseUntil > ctx.Buffer.Idx {
		ctx.LastBaseUntil = 0
		ctx.LastBase = -1
	}

	// Search from current position down to last_base_until
	// HarfBuzz: lines 130-146
	for j := ctx.Buffer.Idx; j > ctx.LastBaseUntil; j-- {
		info := &ctx.Buffer.Info[j-1]
		candidateGlyph := info.GlyphID

		// Check if this glyph should be skipped
		// HarfBuzz equivalent: skippy_iter.match() which calls may_skip()
		skip := false

		// 1. Skip marks (GDEF class 3)
		// HarfBuzz: check_glyph_property with LookupFlag::IgnoreMarks
		if ctx.GDEF != nil {
			glyphClass := ctx.GDEF.GetGlyphClass(candidateGlyph)
			if glyphClass == 3 { // Mark
				skip = true
			}
		}

		// 2. Skip ALL default ignorables for GPOS (including ZWNJ and ZWJ)
		// HarfBuzz equivalent: may_skip() in hb-ot-layout-gsubgpos.hh:463-467
		// For GPOS (table_index==1): ignore_zwnj=true, ignore_hidden=true
		// So ALL default ignorables are skipped unconditionally.
		if !skip && (info.GlyphProps&GlyphPropsDefaultIgnorable) != 0 &&
			(info.GlyphProps&GlyphPropsSubstituted) == 0 {
			skip = true
		}

		if !skip {
			// Found a non-mark, non-ignorable glyph - check accept() and coverage
			// HarfBuzz: if (!accept(buffer, j-1) && NOT_COVERED == baseCoverage.get_coverage(...))
			//           then treat as SKIP
			// https://github.com/harfbuzz/harfbuzz/issues/4124
			if !acceptMarkBase(ctx.Buffer, j-1) {
				if m.baseCoverage.GetCoverage(candidateGlyph) == NotCovered {
					skip = true
				}
			}
		}

		if !skip {
			// This is our base candidate
			ctx.LastBase = j - 1
			break
		}
	}

	// Update cache: we've searched up to current index
	// HarfBuzz: line 147
	ctx.LastBaseUntil = ctx.Buffer.Idx

	// Check if we found a base
	// HarfBuzz: lines 148-152
	if ctx.LastBase == -1 {
		return false
	}

	baseIdx := ctx.LastBase

	// Check if the candidate base is in baseCoverage
	// CRITICAL: If not in coverage, return false - DO NOT keep searching!
	// This matches HarfBuzz behavior (lines 159-164 of MarkBasePosFormat1.hh)
	baseGlyph := ctx.Buffer.Info[baseIdx].GlyphID
	baseIndex := m.baseCoverage.GetCoverage(baseGlyph)
	if baseIndex == NotCovered {
		return false
	}

	// Get the mark record
	markRecord := m.markArray.Records[markIndex]
	markAnchor := markRecord.Anchor
	markClass := int(markRecord.Class)

	// Get the base anchor for this mark class
	baseAnchor := m.baseArray.GetAnchor(int(baseIndex), markClass)
	if baseAnchor == nil {
		return false
	}

	// Calculate position offset: mark should be placed at baseAnchor - markAnchor
	// HarfBuzz: Scales anchor coordinates with em_fscale_x/y then rounds
	// For now, we assume scale = upem (1:1) and just round the coordinates
	// TODO: Implement proper scaling when Font has scale information

	xOffset := int16(roundAnchor(float64(baseAnchor.X) - float64(markAnchor.X)))
	yOffset := int16(roundAnchor(float64(baseAnchor.Y) - float64(markAnchor.Y)))

	// Apply the positioning - use = not += to match HarfBuzz behavior
	// When multiple lookups position the same mark, later lookups override earlier ones
	ctx.Buffer.Pos[ctx.Buffer.Idx].XOffset = xOffset
	ctx.Buffer.Pos[ctx.Buffer.Idx].YOffset = yOffset

	// Store attachment info
	ctx.Buffer.Pos[ctx.Buffer.Idx].AttachType = AttachTypeMark
	ctx.Buffer.Pos[ctx.Buffer.Idx].AttachChain = int16(baseIdx - ctx.Buffer.Idx)

	ctx.Buffer.Idx++
	return true
}

// MarkCoverage returns the mark coverage table.
func (m *MarkBasePos) MarkCoverage() *Coverage {
	return m.markCoverage
}

// BaseCoverage returns the base coverage table.
func (m *MarkBasePos) BaseCoverage() *Coverage {
	return m.baseCoverage
}

// ClassCount returns the number of mark classes.
func (m *MarkBasePos) ClassCount() uint16 {
	return m.classCount
}

// MarkArray returns the mark array.
func (m *MarkBasePos) MarkArray() *MarkArray {
	return m.markArray
}

// BaseArray returns the base array.
func (m *MarkBasePos) BaseArray() *BaseArray {
	return m.baseArray
}

// AttachType constants for glyph attachment
const (
	AttachTypeNone    = 0
	AttachTypeMark    = 1
	AttachTypeCursive = 2
)

// PropagateAttachmentOffsets propagates positioning offsets along the attachment chain.
// This must be called after all GPOS lookups have been applied.
// It handles both mark attachments (marks inherit base offsets) and cursive attachments
// (cursive children inherit parent's cross-direction offset).
func PropagateAttachmentOffsets(positions []GlyphPos, direction Direction) {
	// Process order depends on direction (matches HarfBuzz behavior)
	// See https://github.com/harfbuzz/harfbuzz/issues/5514
	if direction.IsForward() {
		for i := 0; i < len(positions); i++ {
			if positions[i].AttachChain != 0 {
				propagateAttachmentOffsetsRecursive(positions, i, direction)
			}
		}
	} else {
		for i := len(positions) - 1; i >= 0; i-- {
			if positions[i].AttachChain != 0 {
				propagateAttachmentOffsetsRecursive(positions, i, direction)
			}
		}
	}
}

// propagateAttachmentOffsetsRecursive recursively propagates offsets from parent to child.
// This matches HarfBuzz's propagate_attachment_offsets in GPOS.hh
func propagateAttachmentOffsetsRecursive(positions []GlyphPos, i int, direction Direction) {
	chain := positions[i].AttachChain
	attachType := positions[i].AttachType
	if chain == 0 {
		return
	}

	// Clear the chain to mark as processed and prevent infinite loops
	positions[i].AttachChain = 0

	// Find parent index
	j := i + int(chain)
	if j < 0 || j >= len(positions) {
		return
	}

	// First, recursively process the parent
	propagateAttachmentOffsetsRecursive(positions, j, direction)

	// Cursive and mark attachments are handled differently:
	// - Cursive: only cross-direction offset is inherited
	// - Mark: both offsets are inherited, plus advance adjustments
	if attachType == AttachTypeCursive {
		// For cursive, only inherit cross-direction offset
		if direction.IsHorizontal() {
			positions[i].YOffset += positions[j].YOffset
		} else {
			positions[i].XOffset += positions[j].XOffset
		}
	} else {
		// For mark attachments, inherit both offsets
		positions[i].XOffset += positions[j].XOffset
		positions[i].YOffset += positions[j].YOffset

		// Marks also need advance adjustments based on position relative to base.
		// This compensates for cursor movement between mark and base positions.
		// Note: Mark advances are already zeroed by zero_mark_widths_by_gdef()
		// before this function is called, matching HarfBuzz behavior.
		if j < i {
			// Mark follows base (common case in OpenType)
			if direction.IsForward() {
				for k := j; k < i; k++ {
					positions[i].XOffset -= positions[k].XAdvance
					positions[i].YOffset -= positions[k].YAdvance
				}
			} else {
				for k := j + 1; k < i+1; k++ {
					positions[i].XOffset += positions[k].XAdvance
					positions[i].YOffset += positions[k].YAdvance
				}
			}
		} else if j > i {
			// Mark precedes base (rare, can happen with kerx)
			// HarfBuzz: for (unsigned int k = i; k < j; k++)
			if direction.IsForward() {
				for k := i; k < j; k++ {
					positions[i].XOffset += positions[k].XAdvance
					positions[i].YOffset += positions[k].YAdvance
				}
			} else {
				for k := i + 1; k < j+1; k++ {
					positions[i].XOffset -= positions[k].XAdvance
					positions[i].YOffset -= positions[k].YAdvance
				}
			}
		}
	}
}

// --- MarkLigPos ---

// LigatureAttach contains anchors for one ligature glyph.
// It's organized as a matrix where:
// - Rows correspond to ligature components (in writing order)
// - Columns correspond to mark classes
type LigatureAttach struct {
	ComponentCount int         // Number of ligature components
	ClassCount     int         // Number of mark classes
	Anchors        [][]*Anchor // [component][class] -> Anchor (may be nil)
}

// parseLigatureAttach parses a LigatureAttach table (same structure as AnchorMatrix).
func parseLigatureAttach(data []byte, offset int, classCount int) (*LigatureAttach, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	componentCount := int(binary.BigEndian.Uint16(data[offset:]))
	totalAnchors := componentCount * classCount

	if offset+2+totalAnchors*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	la := &LigatureAttach{
		ComponentCount: componentCount,
		ClassCount:     classCount,
		Anchors:        make([][]*Anchor, componentCount),
	}

	for comp := 0; comp < componentCount; comp++ {
		la.Anchors[comp] = make([]*Anchor, classCount)
		for class := 0; class < classCount; class++ {
			idx := comp*classCount + class
			anchorOff := int(binary.BigEndian.Uint16(data[offset+2+idx*2:]))

			if anchorOff == 0 {
				continue
			}

			anchor, err := parseAnchor(data, offset+anchorOff)
			if err != nil {
				continue
			}
			la.Anchors[comp][class] = anchor
		}
	}

	return la, nil
}

// GetAnchor returns the anchor for a given component index and mark class.
func (la *LigatureAttach) GetAnchor(componentIndex, markClass int) *Anchor {
	if componentIndex < 0 || componentIndex >= la.ComponentCount {
		return nil
	}
	if markClass < 0 || markClass >= la.ClassCount {
		return nil
	}
	return la.Anchors[componentIndex][markClass]
}

// LigatureArray contains LigatureAttach tables for multiple ligatures.
type LigatureArray struct {
	Attachments []*LigatureAttach
}

// parseLigatureArray parses a LigatureArray table.
func parseLigatureArray(data []byte, offset int, classCount int) (*LigatureArray, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	ligCount := int(binary.BigEndian.Uint16(data[offset:]))
	if offset+2+ligCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	la := &LigatureArray{
		Attachments: make([]*LigatureAttach, ligCount),
	}

	for i := 0; i < ligCount; i++ {
		attachOff := int(binary.BigEndian.Uint16(data[offset+2+i*2:]))
		if attachOff == 0 {
			continue
		}

		attach, err := parseLigatureAttach(data, offset+attachOff, classCount)
		if err != nil {
			continue
		}
		la.Attachments[i] = attach
	}

	return la, nil
}

// MarkLigPos represents a Mark-to-Ligature Attachment subtable (GPOS Type 5).
// It positions mark glyphs relative to ligature glyphs.
// Each ligature can have multiple components, and each component has its own anchor points.
type MarkLigPos struct {
	format           uint16
	markCoverage     *Coverage
	ligatureCoverage *Coverage
	classCount       uint16
	markArray        *MarkArray
	ligatureArray    *LigatureArray
}

func parseMarkLigPos(data []byte, offset int) (*MarkLigPos, error) {
	if offset+12 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	markCoverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	ligatureCoverageOff := int(binary.BigEndian.Uint16(data[offset+4:]))
	classCount := binary.BigEndian.Uint16(data[offset+6:])
	markArrayOff := int(binary.BigEndian.Uint16(data[offset+8:]))
	ligatureArrayOff := int(binary.BigEndian.Uint16(data[offset+10:]))

	markCoverage, err := ParseCoverage(data, offset+markCoverageOff)
	if err != nil {
		return nil, err
	}

	ligatureCoverage, err := ParseCoverage(data, offset+ligatureCoverageOff)
	if err != nil {
		return nil, err
	}

	markArray, err := parseMarkArray(data, offset+markArrayOff)
	if err != nil {
		return nil, err
	}

	ligatureArray, err := parseLigatureArray(data, offset+ligatureArrayOff, int(classCount))
	if err != nil {
		return nil, err
	}

	return &MarkLigPos{
		format:           format,
		markCoverage:     markCoverage,
		ligatureCoverage: ligatureCoverage,
		classCount:       classCount,
		markArray:        markArray,
		ligatureArray:    ligatureArray,
	}, nil
}

// Apply applies the mark-to-ligature positioning.
// HarfBuzz equivalent: MarkLigPosFormat1_2::apply() in OT/Layout/GPOS/MarkLigPosFormat1.hh:95-166
//
// This searches backwards for a non-mark glyph (the ligature base), determines
// which ligature component the mark belongs to using lig_id/lig_comp, then
// positions the mark at the appropriate component anchor.
func (m *MarkLigPos) Apply(ctx *OTApplyContext) bool {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID

	// HarfBuzz: markCoverage.get_coverage(buffer->cur().codepoint)
	markIndex := m.markCoverage.GetCoverage(glyph)
	if markIndex == NotCovered {
		return false
	}
	if int(markIndex) >= len(m.markArray.Records) {
		return false
	}

	// Now we search backwards for a non-mark glyph.
	// HarfBuzz: skippy_iter.set_lookup_props(LookupFlag::IgnoreMarks) + last_base cache
	// Uses the same last_base/last_base_until cache as MarkBasePos.

	// Reset cache if we've moved backwards in buffer
	// HarfBuzz: lines 107-111
	if ctx.LastBaseUntil > ctx.Buffer.Idx {
		ctx.LastBaseUntil = 0
		ctx.LastBase = -1
	}

	// Search from current position down to last_base_until
	// HarfBuzz: lines 113-121
	for j := ctx.Buffer.Idx; j > ctx.LastBaseUntil; j-- {
		info := &ctx.Buffer.Info[j-1]
		candidateGlyph := info.GlyphID

		// Skip marks (GDEF class 3)
		// HarfBuzz: skippy_iter with LookupFlag::IgnoreMarks
		skip := false
		if ctx.GDEF != nil {
			glyphClass := ctx.GDEF.GetGlyphClass(candidateGlyph)
			if glyphClass == 3 { // Mark
				skip = true
			}
		}

		// Skip ALL default ignorables for GPOS
		// HarfBuzz: may_skip() with ignore_zwnj=true, ignore_hidden=true for GPOS
		if !skip && (info.GlyphProps&GlyphPropsDefaultIgnorable) != 0 &&
			(info.GlyphProps&GlyphPropsSubstituted) == 0 {
			skip = true
		}

		if !skip {
			ctx.LastBase = j - 1
			break
		}
	}

	// Update cache
	// HarfBuzz: line 122
	ctx.LastBaseUntil = ctx.Buffer.Idx

	if ctx.LastBase == -1 {
		return false
	}

	idx := ctx.LastBase

	// HarfBuzz: "Checking that matched glyph is actually a ligature by GDEF is too strong; disabled"
	// (line 131-132)

	// Check ligature coverage
	// HarfBuzz: lines 134-139
	ligIndex := m.ligatureCoverage.GetCoverage(ctx.Buffer.Info[idx].GlyphID)
	if ligIndex == NotCovered {
		return false
	}

	if int(ligIndex) >= len(m.ligatureArray.Attachments) {
		return false
	}

	ligAttach := m.ligatureArray.Attachments[ligIndex]
	if ligAttach == nil || ligAttach.ComponentCount == 0 {
		return false
	}

	// Find component to attach to.
	// HarfBuzz: lines 152-163
	// "We must now check whether the ligature ID of the current mark glyph
	//  is identical to the ligature ID of the found ligature. If yes, we
	//  can directly use the component index. If not, we attach the mark
	//  glyph to the last component of the ligature."
	compCount := ligAttach.ComponentCount
	ligID := ctx.Buffer.Info[idx].GetLigID()
	markID := ctx.Buffer.Info[ctx.Buffer.Idx].GetLigID()
	markComp := ctx.Buffer.Info[ctx.Buffer.Idx].GetLigComp()

	var compIndex int
	if ligID != 0 && ligID == markID && markComp > 0 {
		// Mark belongs to this ligature - use its component
		// HarfBuzz: comp_index = hb_min(comp_count, mark_comp) - 1
		compIndex = int(min(int(compCount), int(markComp))) - 1
	} else {
		// Mark doesn't belong to this ligature - attach to last component
		// HarfBuzz: comp_index = comp_count - 1
		compIndex = compCount - 1
	}

	// Get the mark record
	markRecord := m.markArray.Records[markIndex]
	markAnchor := markRecord.Anchor
	markClass := int(markRecord.Class)

	// Get the ligature anchor for this component and mark class
	ligAnchor := ligAttach.GetAnchor(compIndex, markClass)
	if ligAnchor == nil {
		return false
	}

	// Calculate position offset: mark placed at ligAnchor - markAnchor
	// HarfBuzz: markArray.apply() computes baseAnchor - markAnchor
	xOffset := int16(roundAnchor(float64(ligAnchor.X) - float64(markAnchor.X)))
	yOffset := int16(roundAnchor(float64(ligAnchor.Y) - float64(markAnchor.Y)))
	// Apply the positioning - use = not += to match HarfBuzz behavior
	ctx.Buffer.Pos[ctx.Buffer.Idx].XOffset = xOffset
	ctx.Buffer.Pos[ctx.Buffer.Idx].YOffset = yOffset

	// Store attachment info
	ctx.Buffer.Pos[ctx.Buffer.Idx].AttachType = AttachTypeMark
	ctx.Buffer.Pos[ctx.Buffer.Idx].AttachChain = int16(idx - ctx.Buffer.Idx)

	ctx.Buffer.Idx++
	return true
}

// MarkCoverage returns the mark coverage table.
func (m *MarkLigPos) MarkCoverage() *Coverage {
	return m.markCoverage
}

// LigatureCoverage returns the ligature coverage table.
func (m *MarkLigPos) LigatureCoverage() *Coverage {
	return m.ligatureCoverage
}

// ClassCount returns the number of mark classes.
func (m *MarkLigPos) ClassCount() uint16 {
	return m.classCount
}

// MarkArray returns the mark array.
func (m *MarkLigPos) MarkArray() *MarkArray {
	return m.markArray
}

// LigatureArray returns the ligature array.
func (m *MarkLigPos) LigatureArray() *LigatureArray {
	return m.ligatureArray
}

// --- MarkMarkPos ---

// MarkMarkPos represents a Mark-to-Mark Attachment subtable (GPOS Type 6).
// It positions mark glyphs (mark1) relative to preceding mark glyphs (mark2).
// This is used for stacking diacritics, e.g., placing an accent on top of another accent.
type MarkMarkPos struct {
	format        uint16
	mark1Coverage *Coverage // Coverage for the attaching mark (mark1)
	mark2Coverage *Coverage // Coverage for the base mark (mark2)
	classCount    uint16
	mark1Array    *MarkArray // Anchor information for mark1 glyphs
	mark2Array    *BaseArray // Anchor matrix for mark2 glyphs (same structure as BaseArray)
}

func parseMarkMarkPos(data []byte, offset int) (*MarkMarkPos, error) {
	if offset+12 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	mark1CoverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	mark2CoverageOff := int(binary.BigEndian.Uint16(data[offset+4:]))
	classCount := binary.BigEndian.Uint16(data[offset+6:])
	mark1ArrayOff := int(binary.BigEndian.Uint16(data[offset+8:]))
	mark2ArrayOff := int(binary.BigEndian.Uint16(data[offset+10:]))

	mark1Coverage, err := ParseCoverage(data, offset+mark1CoverageOff)
	if err != nil {
		return nil, err
	}

	mark2Coverage, err := ParseCoverage(data, offset+mark2CoverageOff)
	if err != nil {
		return nil, err
	}

	mark1Array, err := parseMarkArray(data, offset+mark1ArrayOff)
	if err != nil {
		return nil, err
	}

	mark2Array, err := parseBaseArray(data, offset+mark2ArrayOff, int(classCount))
	if err != nil {
		return nil, err
	}

	return &MarkMarkPos{
		format:        format,
		mark1Coverage: mark1Coverage,
		mark2Coverage: mark2Coverage,
		classCount:    classCount,
		mark1Array:    mark1Array,
		mark2Array:    mark2Array,
	}, nil
}

// Apply applies the mark-to-mark positioning.
// It finds the preceding mark glyph (mark2) and positions the current mark (mark1) relative to it.
func (m *MarkMarkPos) Apply(ctx *OTApplyContext) bool {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID

	// Check if current glyph is mark1
	mark1Index := m.mark1Coverage.GetCoverage(glyph)
	if mark1Index == NotCovered {
		return false
	}

	if int(mark1Index) >= len(m.mark1Array.Records) {
		return false
	}

	// Search backwards for a mark2 glyph
	// Unlike MarkBasePos, we're looking for another mark, not a base glyph
	// HarfBuzz equivalent: MarkMarkPosFormat1.hh lines 97-130
	mark2Idx := -1
	for j := ctx.Buffer.Idx - 1; j >= 0; j-- {
		info := &ctx.Buffer.Info[j]

		// Skip ALL default ignorables (not just hidden ones) for GPOS
		// HarfBuzz: may_skip() with ignore_hidden=true (GPOS table)
		// See hb-ot-layout-gsubgpos.hh lines 463-467
		// For GPOS: ignore_hidden=true, so (ignore_hidden || !is_hidden) is ALWAYS true
		// This means ALL default ignorables are skipped, regardless of hidden flag
		// Note: ZWNJ/ZWJ are handled separately via GlyphProps flags
		if (info.GlyphProps&GlyphPropsDefaultIgnorable) != 0 &&
			(info.GlyphProps&GlyphPropsSubstituted) == 0 &&
			(info.GlyphProps&GlyphPropsZWNJ) == 0 &&
			(info.GlyphProps&GlyphPropsZWJ) == 0 {
			continue
		}

		// Check if this glyph is a mark2 (in mark2Coverage)
		if m.mark2Coverage.GetCoverage(info.GlyphID) != NotCovered {
			mark2Idx = j
			break
		}

		// Stop if we hit a non-mark glyph (base or ligature)
		// HarfBuzz: _hb_glyph_info_is_mark() in hb-ot-layout.hh:547-550
		// Uses GlyphProps which works even without GDEF (props are set from coverage)
		if !info.IsMark() {
			break
		}
	}

	if mark2Idx < 0 {
		return false
	}

	// Ligature ID/component check: ensure mark1 and mark2 belong together.
	// HarfBuzz: MarkMarkPosFormat1.hh lines 121-143
	// Two marks can attach only if:
	//   - Both have ligID==0 (marks on same non-ligature base), OR
	//   - Both have same ligID and same ligComp (same ligature component), OR
	//   - One of them is itself a ligature (ligID>0 && ligComp==0)
	id1 := ctx.Buffer.Info[ctx.Buffer.Idx].GetLigID()
	id2 := ctx.Buffer.Info[mark2Idx].GetLigID()
	comp1 := ctx.Buffer.Info[ctx.Buffer.Idx].GetLigComp()
	comp2 := ctx.Buffer.Info[mark2Idx].GetLigComp()

	good := false
	if id1 == id2 {
		if id1 == 0 {
			good = true // Marks belonging to the same base
		} else if comp1 == comp2 {
			good = true // Same ligature component
		}
	} else {
		// If ligature IDs don't match, one may be a ligature itself
		if (id1 > 0 && comp1 == 0) || (id2 > 0 && comp2 == 0) {
			good = true
		}
	}
	if !good {
		return false
	}

	mark2Glyph := ctx.Buffer.Info[mark2Idx].GlyphID
	mark2Index := m.mark2Coverage.GetCoverage(mark2Glyph)
	if mark2Index == NotCovered {
		return false
	}

	// Get the mark1 record
	mark1Record := m.mark1Array.Records[mark1Index]
	mark1Anchor := mark1Record.Anchor
	mark1Class := int(mark1Record.Class)

	// Get the mark2 anchor for this mark1 class
	mark2Anchor := m.mark2Array.GetAnchor(int(mark2Index), mark1Class)
	if mark2Anchor == nil {
		return false
	}

	// Calculate position offset: mark1 should be placed at mark2Anchor - mark1Anchor
	// HarfBuzz: Scales anchor coordinates with em_fscale_x/y then rounds
	xOffset := int16(roundAnchor(float64(mark2Anchor.X) - float64(mark1Anchor.X)))
	yOffset := int16(roundAnchor(float64(mark2Anchor.Y) - float64(mark1Anchor.Y)))

	// Apply the positioning - use = not += to match HarfBuzz behavior
	// When multiple lookups position the same mark, later lookups override earlier ones
	ctx.Buffer.Pos[ctx.Buffer.Idx].XOffset = xOffset
	ctx.Buffer.Pos[ctx.Buffer.Idx].YOffset = yOffset

	// Store attachment info
	ctx.Buffer.Pos[ctx.Buffer.Idx].AttachType = AttachTypeMark
	ctx.Buffer.Pos[ctx.Buffer.Idx].AttachChain = int16(mark2Idx - ctx.Buffer.Idx)

	ctx.Buffer.Idx++
	return true
}

// Mark1Coverage returns the coverage table for the attaching mark (mark1).
func (m *MarkMarkPos) Mark1Coverage() *Coverage {
	return m.mark1Coverage
}

// Mark2Coverage returns the coverage table for the base mark (mark2).
func (m *MarkMarkPos) Mark2Coverage() *Coverage {
	return m.mark2Coverage
}

// ClassCount returns the number of mark classes.
func (m *MarkMarkPos) ClassCount() uint16 {
	return m.classCount
}

// Mark1Array returns the mark array for mark1 glyphs.
func (m *MarkMarkPos) Mark1Array() *MarkArray {
	return m.mark1Array
}

// Mark2Array returns the anchor array for mark2 glyphs.
func (m *MarkMarkPos) Mark2Array() *BaseArray {
	return m.mark2Array
}

// --- Context Positioning (Type 7) ---

// GPOSLookupRecord represents a lookup to apply at a specific position in a context.
type GPOSLookupRecord struct {
	SequenceIndex uint16 // Index into current glyph sequence (0-based)
	LookupIndex   uint16 // Lookup to apply
}

// GPOSContextRule represents a single rule in a context positioning rule set.
type GPOSContextRule struct {
	Input         []GlyphID          // Input sequence (starting from second glyph)
	LookupRecords []GPOSLookupRecord // Lookups to apply
}

// ContextPos represents a Context Positioning subtable (GPOS Type 7).
// It matches input sequences and applies nested positioning lookups.
// HarfBuzz equivalent: ContextPos in OT/Layout/GPOS/ContextPos.hh
type ContextPos struct {
	format uint16

	// Format 1: Simple glyph contexts
	coverage *Coverage
	ruleSets [][]GPOSContextRule

	// Format 2: Class-based contexts
	classDef *ClassDef

	// Format 3: Coverage-based contexts
	inputCoverages []*Coverage
	lookupRecords  []GPOSLookupRecord
}

func parseContextPos(data []byte, offset int) (*ContextPos, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])

	switch format {
	case 1:
		return parseContextPosFormat1(data, offset)
	case 2:
		return parseContextPosFormat2(data, offset)
	case 3:
		return parseContextPosFormat3(data, offset)
	default:
		return nil, ErrInvalidFormat
	}
}

// parseContextPosFormat1 parses ContextPosFormat1 (simple glyph context).
func parseContextPosFormat1(data []byte, offset int) (*ContextPos, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	ruleSetCount := int(binary.BigEndian.Uint16(data[offset+4:]))

	if offset+6+ruleSetCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	cp := &ContextPos{
		format:   1,
		coverage: coverage,
		ruleSets: make([][]GPOSContextRule, ruleSetCount),
	}

	for i := 0; i < ruleSetCount; i++ {
		ruleSetOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		if ruleSetOff == 0 {
			continue
		}

		absOff := offset + ruleSetOff
		if absOff+2 > len(data) {
			continue
		}

		ruleCount := int(binary.BigEndian.Uint16(data[absOff:]))
		if absOff+2+ruleCount*2 > len(data) {
			continue
		}

		rules := make([]GPOSContextRule, 0, ruleCount)
		for j := 0; j < ruleCount; j++ {
			ruleOff := int(binary.BigEndian.Uint16(data[absOff+2+j*2:]))
			if ruleOff == 0 {
				continue
			}

			ruleAbsOff := absOff + ruleOff
			if ruleAbsOff+4 > len(data) {
				continue
			}

			glyphCount := int(binary.BigEndian.Uint16(data[ruleAbsOff:]))
			lookupCount := int(binary.BigEndian.Uint16(data[ruleAbsOff+2:]))

			inputCount := glyphCount - 1
			if ruleAbsOff+4+inputCount*2+lookupCount*4 > len(data) {
				continue
			}

			rule := GPOSContextRule{
				Input:         make([]GlyphID, inputCount),
				LookupRecords: make([]GPOSLookupRecord, lookupCount),
			}

			for k := 0; k < inputCount; k++ {
				rule.Input[k] = GlyphID(binary.BigEndian.Uint16(data[ruleAbsOff+4+k*2:]))
			}

			lookupOff := ruleAbsOff + 4 + inputCount*2
			for k := 0; k < lookupCount; k++ {
				rule.LookupRecords[k] = GPOSLookupRecord{
					SequenceIndex: binary.BigEndian.Uint16(data[lookupOff+k*4:]),
					LookupIndex:   binary.BigEndian.Uint16(data[lookupOff+k*4+2:]),
				}
			}

			rules = append(rules, rule)
		}
		cp.ruleSets[i] = rules
	}

	return cp, nil
}

// parseContextPosFormat2 parses ContextPosFormat2 (class-based context).
func parseContextPosFormat2(data []byte, offset int) (*ContextPos, error) {
	if offset+8 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	classDefOff := int(binary.BigEndian.Uint16(data[offset+4:]))
	ruleSetCount := int(binary.BigEndian.Uint16(data[offset+6:]))

	if offset+8+ruleSetCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	classDef, err := ParseClassDef(data, offset+classDefOff)
	if err != nil {
		return nil, err
	}

	cp := &ContextPos{
		format:   2,
		coverage: coverage,
		classDef: classDef,
		ruleSets: make([][]GPOSContextRule, ruleSetCount),
	}

	for i := 0; i < ruleSetCount; i++ {
		ruleSetOff := int(binary.BigEndian.Uint16(data[offset+8+i*2:]))
		if ruleSetOff == 0 {
			continue
		}

		absOff := offset + ruleSetOff
		if absOff+2 > len(data) {
			continue
		}

		ruleCount := int(binary.BigEndian.Uint16(data[absOff:]))
		if absOff+2+ruleCount*2 > len(data) {
			continue
		}

		rules := make([]GPOSContextRule, 0, ruleCount)
		for j := 0; j < ruleCount; j++ {
			ruleOff := int(binary.BigEndian.Uint16(data[absOff+2+j*2:]))
			if ruleOff == 0 {
				continue
			}

			ruleAbsOff := absOff + ruleOff
			if ruleAbsOff+4 > len(data) {
				continue
			}

			glyphCount := int(binary.BigEndian.Uint16(data[ruleAbsOff:]))
			lookupCount := int(binary.BigEndian.Uint16(data[ruleAbsOff+2:]))

			inputCount := glyphCount - 1
			if ruleAbsOff+4+inputCount*2+lookupCount*4 > len(data) {
				continue
			}

			rule := GPOSContextRule{
				Input:         make([]GlyphID, inputCount),
				LookupRecords: make([]GPOSLookupRecord, lookupCount),
			}

			// For Format 2, Input contains class values, not glyph IDs
			for k := 0; k < inputCount; k++ {
				rule.Input[k] = GlyphID(binary.BigEndian.Uint16(data[ruleAbsOff+4+k*2:]))
			}

			lookupOff := ruleAbsOff + 4 + inputCount*2
			for k := 0; k < lookupCount; k++ {
				rule.LookupRecords[k] = GPOSLookupRecord{
					SequenceIndex: binary.BigEndian.Uint16(data[lookupOff+k*4:]),
					LookupIndex:   binary.BigEndian.Uint16(data[lookupOff+k*4+2:]),
				}
			}

			rules = append(rules, rule)
		}
		cp.ruleSets[i] = rules
	}

	return cp, nil
}

// parseContextPosFormat3 parses ContextPosFormat3 (coverage-based context).
func parseContextPosFormat3(data []byte, offset int) (*ContextPos, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	glyphCount := int(binary.BigEndian.Uint16(data[offset+2:]))
	lookupCount := int(binary.BigEndian.Uint16(data[offset+4:]))

	if glyphCount == 0 {
		return nil, ErrInvalidFormat
	}

	if offset+6+glyphCount*2+lookupCount*4 > len(data) {
		return nil, ErrInvalidOffset
	}

	inputCoverages := make([]*Coverage, glyphCount)
	for i := 0; i < glyphCount; i++ {
		covOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		cov, err := ParseCoverage(data, offset+covOff)
		if err != nil {
			return nil, err
		}
		inputCoverages[i] = cov
	}

	lookupRecords := make([]GPOSLookupRecord, lookupCount)
	lookupOff := offset + 6 + glyphCount*2
	for i := 0; i < lookupCount; i++ {
		lookupRecords[i] = GPOSLookupRecord{
			SequenceIndex: binary.BigEndian.Uint16(data[lookupOff+i*4:]),
			LookupIndex:   binary.BigEndian.Uint16(data[lookupOff+i*4+2:]),
		}
	}

	return &ContextPos{
		format:         3,
		inputCoverages: inputCoverages,
		lookupRecords:  lookupRecords,
	}, nil
}

// Apply applies context positioning.
func (cp *ContextPos) Apply(ctx *OTApplyContext) bool {
	switch cp.format {
	case 1:
		return cp.applyFormat1(ctx)
	case 2:
		return cp.applyFormat2(ctx)
	case 3:
		return cp.applyFormat3(ctx)
	default:
		return false
	}
}

// applyFormat1 applies ContextPosFormat1 (simple glyph context).
func (cp *ContextPos) applyFormat1(ctx *OTApplyContext) bool {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	coverageIndex := cp.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return false
	}

	if int(coverageIndex) >= len(cp.ruleSets) {
		return false
	}

	ruleSet := cp.ruleSets[coverageIndex]
	for _, rule := range ruleSet {
		if cp.matchRuleFormat1(ctx, &rule) {
			inputLen := len(rule.Input) + 1
			cp.applyLookups(ctx, rule.LookupRecords, inputLen)
			matchEnd := ctx.MatchPositions[inputLen-1] + 1
			ctx.MatchPositions = nil
			ctx.Buffer.Idx = matchEnd
			return true
		}
	}

	return false
}

func (cp *ContextPos) matchRuleFormat1(ctx *OTApplyContext, rule *GPOSContextRule) bool {
	inputLen := len(rule.Input) + 1
	bufLen := len(ctx.Buffer.Info)

	// Match input sequence using skippy-iteration
	// HarfBuzz: match_input() in hb-ot-layout-gsubgpos.hh:1311-1400
	matchPositions := make([]int, inputLen)
	matchPositions[0] = ctx.Buffer.Idx
	pos := ctx.Buffer.Idx
	for i, glyph := range rule.Input {
		found := false
		for pos < bufLen-1 {
			pos++
			skip := ctx.MaySkip(pos, false)
			if skip == SkipYes {
				continue
			}
			if ctx.Buffer.Info[pos].GlyphID == glyph {
				found = true
				break
			}
			if skip == SkipMaybe {
				continue
			}
			return false
		}
		if !found {
			return false
		}
		matchPositions[i+1] = pos
	}

	ctx.MatchPositions = matchPositions
	return true
}

// applyFormat2 applies ContextPosFormat2 (class-based context).
func (cp *ContextPos) applyFormat2(ctx *OTApplyContext) bool {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	if cp.coverage.GetCoverage(glyph) == NotCovered {
		return false
	}

	classIndex := cp.classDef.GetClass(glyph)
	if classIndex >= len(cp.ruleSets) {
		return false
	}

	ruleSet := cp.ruleSets[classIndex]
	for _, rule := range ruleSet {
		if cp.matchRuleFormat2(ctx, &rule) {
			inputLen := len(rule.Input) + 1
			cp.applyLookups(ctx, rule.LookupRecords, inputLen)
			matchEnd := ctx.MatchPositions[inputLen-1] + 1
			ctx.MatchPositions = nil
			ctx.Buffer.Idx = matchEnd
			return true
		}
	}

	return false
}

func (cp *ContextPos) matchRuleFormat2(ctx *OTApplyContext, rule *GPOSContextRule) bool {
	inputLen := len(rule.Input) + 1
	bufLen := len(ctx.Buffer.Info)

	// Match input sequence using skippy-iteration
	// HarfBuzz: match_input() in hb-ot-layout-gsubgpos.hh:1311-1400
	matchPositions := make([]int, inputLen)
	matchPositions[0] = ctx.Buffer.Idx
	pos := ctx.Buffer.Idx
	for i, classValue := range rule.Input {
		found := false
		for pos < bufLen-1 {
			pos++
			skip := ctx.MaySkip(pos, false)
			if skip == SkipYes {
				continue
			}
			glyphClass := cp.classDef.GetClass(ctx.Buffer.Info[pos].GlyphID)
			if glyphClass == int(classValue) {
				found = true
				break
			}
			if skip == SkipMaybe {
				continue
			}
			return false
		}
		if !found {
			return false
		}
		matchPositions[i+1] = pos
	}

	ctx.MatchPositions = matchPositions
	return true
}

// applyFormat3 applies ContextPosFormat3 (coverage-based context).
func (cp *ContextPos) applyFormat3(ctx *OTApplyContext) bool {
	inputLen := len(cp.inputCoverages)
	if inputLen == 0 {
		return false
	}

	if ctx.Buffer.Idx+inputLen > len(ctx.Buffer.Info) {
		return false
	}

	// Check all input coverages
	for i, cov := range cp.inputCoverages {
		if cov.GetCoverage(ctx.Buffer.Info[ctx.Buffer.Idx+i].GlyphID) == NotCovered {
			return false
		}
	}

	cp.applyLookups(ctx, cp.lookupRecords, inputLen)
	ctx.Buffer.Idx += inputLen
	return true
}

// applyLookups applies nested lookups for ContextPos.
// HarfBuzz equivalent: apply_lookup() in hb-ot-layout-gsubgpos.hh:1788-1909
// Uses ctx.Recurse() for HarfBuzz-conformant nested lookup application.
func (cp *ContextPos) applyLookups(ctx *OTApplyContext, lookupRecords []GPOSLookupRecord, inputLen int) {
	// Save original buffer index
	originalIdx := ctx.Buffer.Idx

	for _, record := range lookupRecords {
		idx := int(record.SequenceIndex)
		if idx >= inputLen {
			continue
		}

		// HarfBuzz: buffer->move_to(c->match_positions.arrayZ[idx])
		// For Format1/2, match positions are consecutive
		ctx.Buffer.Idx = originalIdx + idx

		// Apply nested lookup using ctx.Recurse()
		// HarfBuzz equivalent: recurse(r.lookupListIndex)
		ctx.Recurse(int(record.LookupIndex))
	}

	// Restore original buffer index
	ctx.Buffer.Idx = originalIdx
}

// --- Chaining Context Positioning (Type 8) ---

// GPOSChainRule represents a single chaining context positioning rule.
type GPOSChainRule struct {
	Backtrack     []GlyphID          // Backtrack sequence (in reverse order)
	Input         []GlyphID          // Input sequence (starting from second glyph)
	Lookahead     []GlyphID          // Lookahead sequence
	LookupRecords []GPOSLookupRecord // Lookups to apply
}

// ChainContextPos represents a Chaining Context Positioning subtable (GPOS Type 8).
// It matches backtrack, input, and lookahead sequences, then applies nested lookups.
// HarfBuzz equivalent: ChainContextPos in OT/Layout/GPOS/ChainContextPos.hh
type ChainContextPos struct {
	format uint16

	// Format 1: Simple glyph contexts
	coverage      *Coverage
	chainRuleSets [][]GPOSChainRule // Indexed by coverage index

	// Format 2: Class-based contexts
	backtrackClassDef *ClassDef
	inputClassDef     *ClassDef
	lookaheadClassDef *ClassDef
	// chainRuleSets also used for format 2 (indexed by input class)

	// Format 3: Coverage-based contexts
	backtrackCoverages []*Coverage
	inputCoverages     []*Coverage
	lookaheadCoverages []*Coverage
	lookupRecords      []GPOSLookupRecord
}

func parseChainContextPos(data []byte, offset int) (*ChainContextPos, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])

	switch format {
	case 1:
		return parseChainContextPosFormat1(data, offset)
	case 2:
		return parseChainContextPosFormat2(data, offset)
	case 3:
		return parseChainContextPosFormat3(data, offset)
	default:
		return nil, ErrInvalidFormat
	}
}

// parseChainContextPosFormat1 parses ChainContextPosFormat1 (simple glyph context).
func parseChainContextPosFormat1(data []byte, offset int) (*ChainContextPos, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	chainRuleSetCount := int(binary.BigEndian.Uint16(data[offset+4:]))

	if offset+6+chainRuleSetCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	ccp := &ChainContextPos{
		format:        1,
		coverage:      coverage,
		chainRuleSets: make([][]GPOSChainRule, chainRuleSetCount),
	}

	for i := 0; i < chainRuleSetCount; i++ {
		chainRuleSetOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		if chainRuleSetOff == 0 {
			continue
		}

		absOff := offset + chainRuleSetOff
		if absOff+2 > len(data) {
			continue
		}

		chainRuleCount := int(binary.BigEndian.Uint16(data[absOff:]))
		if absOff+2+chainRuleCount*2 > len(data) {
			continue
		}

		rules := make([]GPOSChainRule, 0, chainRuleCount)
		for j := 0; j < chainRuleCount; j++ {
			chainRuleOff := int(binary.BigEndian.Uint16(data[absOff+2+j*2:]))
			if chainRuleOff == 0 {
				continue
			}

			rule, err := parseGPOSChainRule(data, absOff+chainRuleOff)
			if err != nil {
				continue
			}
			rules = append(rules, *rule)
		}
		ccp.chainRuleSets[i] = rules
	}

	return ccp, nil
}

func parseGPOSChainRule(data []byte, offset int) (*GPOSChainRule, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	off := offset

	// Backtrack count and glyphs
	backtrackCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+backtrackCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	backtrack := make([]GlyphID, backtrackCount)
	for i := 0; i < backtrackCount; i++ {
		backtrack[i] = GlyphID(binary.BigEndian.Uint16(data[off+i*2:]))
	}
	off += backtrackCount * 2

	// Input count and glyphs
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	inputCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2

	inputGlyphCount := inputCount - 1 // First glyph is matched by coverage
	if off+inputGlyphCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	input := make([]GlyphID, inputGlyphCount)
	for i := 0; i < inputGlyphCount; i++ {
		input[i] = GlyphID(binary.BigEndian.Uint16(data[off+i*2:]))
	}
	off += inputGlyphCount * 2

	// Lookahead count and glyphs
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	lookaheadCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+lookaheadCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookahead := make([]GlyphID, lookaheadCount)
	for i := 0; i < lookaheadCount; i++ {
		lookahead[i] = GlyphID(binary.BigEndian.Uint16(data[off+i*2:]))
	}
	off += lookaheadCount * 2

	// Lookup records
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	lookupCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+lookupCount*4 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookupRecords := make([]GPOSLookupRecord, lookupCount)
	for i := 0; i < lookupCount; i++ {
		lookupRecords[i] = GPOSLookupRecord{
			SequenceIndex: binary.BigEndian.Uint16(data[off+i*4:]),
			LookupIndex:   binary.BigEndian.Uint16(data[off+i*4+2:]),
		}
	}

	return &GPOSChainRule{
		Backtrack:     backtrack,
		Input:         input,
		Lookahead:     lookahead,
		LookupRecords: lookupRecords,
	}, nil
}

// parseChainContextPosFormat2 parses ChainContextPosFormat2 (class-based context).
func parseChainContextPosFormat2(data []byte, offset int) (*ChainContextPos, error) {
	if offset+12 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	backtrackClassDefOff := int(binary.BigEndian.Uint16(data[offset+4:]))
	inputClassDefOff := int(binary.BigEndian.Uint16(data[offset+6:]))
	lookaheadClassDefOff := int(binary.BigEndian.Uint16(data[offset+8:]))
	chainRuleSetCount := int(binary.BigEndian.Uint16(data[offset+10:]))

	if offset+12+chainRuleSetCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	// HarfBuzz: NULL offset (0) for ClassDef means all glyphs are class 0.
	var backtrackClassDef *ClassDef
	if backtrackClassDefOff != 0 {
		backtrackClassDef, err = ParseClassDef(data, offset+backtrackClassDefOff)
		if err != nil {
			return nil, err
		}
	} else {
		backtrackClassDef = &ClassDef{}
	}

	var inputClassDef *ClassDef
	if inputClassDefOff != 0 {
		inputClassDef, err = ParseClassDef(data, offset+inputClassDefOff)
		if err != nil {
			return nil, err
		}
	} else {
		inputClassDef = &ClassDef{}
	}

	var lookaheadClassDef *ClassDef
	if lookaheadClassDefOff != 0 {
		lookaheadClassDef, err = ParseClassDef(data, offset+lookaheadClassDefOff)
		if err != nil {
			return nil, err
		}
	} else {
		lookaheadClassDef = &ClassDef{}
	}

	ccp := &ChainContextPos{
		format:            2,
		coverage:          coverage,
		backtrackClassDef: backtrackClassDef,
		inputClassDef:     inputClassDef,
		lookaheadClassDef: lookaheadClassDef,
		chainRuleSets:     make([][]GPOSChainRule, chainRuleSetCount),
	}

	for i := 0; i < chainRuleSetCount; i++ {
		chainRuleSetOff := int(binary.BigEndian.Uint16(data[offset+12+i*2:]))
		if chainRuleSetOff == 0 {
			continue
		}

		absOff := offset + chainRuleSetOff
		if absOff+2 > len(data) {
			continue
		}

		chainRuleCount := int(binary.BigEndian.Uint16(data[absOff:]))
		if absOff+2+chainRuleCount*2 > len(data) {
			continue
		}

		rules := make([]GPOSChainRule, 0, chainRuleCount)
		for j := 0; j < chainRuleCount; j++ {
			chainRuleOff := int(binary.BigEndian.Uint16(data[absOff+2+j*2:]))
			if chainRuleOff == 0 {
				continue
			}

			rule, err := parseGPOSChainRule(data, absOff+chainRuleOff)
			if err != nil {
				continue
			}
			rules = append(rules, *rule)
		}
		ccp.chainRuleSets[i] = rules
	}

	return ccp, nil
}

// parseChainContextPosFormat3 parses ChainContextPosFormat3 (coverage-based context).
func parseChainContextPosFormat3(data []byte, offset int) (*ChainContextPos, error) {
	off := offset + 2 // Skip format

	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	// Backtrack coverages
	backtrackCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+backtrackCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	backtrackCoverages := make([]*Coverage, backtrackCount)
	for i := 0; i < backtrackCount; i++ {
		covOff := int(binary.BigEndian.Uint16(data[off+i*2:]))
		cov, err := ParseCoverage(data, offset+covOff)
		if err != nil {
			return nil, err
		}
		backtrackCoverages[i] = cov
	}
	off += backtrackCount * 2

	// Input coverages
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	inputCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if inputCount == 0 {
		return nil, ErrInvalidFormat
	}
	if off+inputCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	inputCoverages := make([]*Coverage, inputCount)
	for i := 0; i < inputCount; i++ {
		covOff := int(binary.BigEndian.Uint16(data[off+i*2:]))
		cov, err := ParseCoverage(data, offset+covOff)
		if err != nil {
			return nil, err
		}
		inputCoverages[i] = cov
	}
	off += inputCount * 2

	// Lookahead coverages
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	lookaheadCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+lookaheadCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookaheadCoverages := make([]*Coverage, lookaheadCount)
	for i := 0; i < lookaheadCount; i++ {
		covOff := int(binary.BigEndian.Uint16(data[off+i*2:]))
		cov, err := ParseCoverage(data, offset+covOff)
		if err != nil {
			return nil, err
		}
		lookaheadCoverages[i] = cov
	}
	off += lookaheadCount * 2

	// Lookup records
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	lookupCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+lookupCount*4 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookupRecords := make([]GPOSLookupRecord, lookupCount)
	for i := 0; i < lookupCount; i++ {
		lookupRecords[i] = GPOSLookupRecord{
			SequenceIndex: binary.BigEndian.Uint16(data[off+i*4:]),
			LookupIndex:   binary.BigEndian.Uint16(data[off+i*4+2:]),
		}
	}

	return &ChainContextPos{
		format:             3,
		backtrackCoverages: backtrackCoverages,
		inputCoverages:     inputCoverages,
		lookaheadCoverages: lookaheadCoverages,
		lookupRecords:      lookupRecords,
	}, nil
}

// Apply applies chaining context positioning.
func (ccp *ChainContextPos) Apply(ctx *OTApplyContext) bool {
	var result bool
	switch ccp.format {
	case 1:
		result = ccp.applyFormat1(ctx)
	case 2:
		result = ccp.applyFormat2(ctx)
	case 3:
		result = ccp.applyFormat3(ctx)
	default:
		result = false
	}
	return result
}

// applyFormat1 applies ChainContextPosFormat1 (simple glyph context).
func (ccp *ChainContextPos) applyFormat1(ctx *OTApplyContext) bool {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	coverageIndex := ccp.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return false
	}

	if int(coverageIndex) >= len(ccp.chainRuleSets) {
		return false
	}

	ruleSet := ccp.chainRuleSets[coverageIndex]
	for _, rule := range ruleSet {
		if ccp.matchRuleFormat1(ctx, &rule) {
			inputLen := len(rule.Input) + 1
			ccp.applyLookups(ctx, rule.LookupRecords, inputLen)
			matchEnd := ctx.MatchPositions[inputLen-1] + 1
			ctx.MatchPositions = nil
			ctx.Buffer.Idx = matchEnd
			return true
		}
	}

	return false
}

func (ccp *ChainContextPos) matchRuleFormat1(ctx *OTApplyContext, rule *GPOSChainRule) bool {
	inputLen := len(rule.Input) + 1
	bufLen := len(ctx.Buffer.Info)

	// Match input sequence using skippy-iteration (starting from second glyph)
	// HarfBuzz: match_input() in hb-ot-layout-gsubgpos.hh:1311-1400
	matchPositions := make([]int, inputLen)
	matchPositions[0] = ctx.Buffer.Idx
	pos := ctx.Buffer.Idx
	for i, glyph := range rule.Input {
		found := false
		for pos < bufLen-1 {
			pos++
			skip := ctx.MaySkip(pos, false) // context_match=false for input matching
			if skip == SkipYes {
				continue
			}
			if ctx.Buffer.Info[pos].GlyphID == glyph {
				found = true
				break
			}
			if skip == SkipMaybe {
				continue
			}
			return false
		}
		if !found {
			return false
		}
		matchPositions[i+1] = pos
	}

	// Check lookahead using skippy-iteration
	// HarfBuzz: match_lookahead() in hb-ot-layout-gsubgpos.hh:1607-1637
	lookaheadPos := pos
	for _, glyph := range rule.Lookahead {
		found := false
		for lookaheadPos < bufLen-1 {
			lookaheadPos++
			skip := ctx.MaySkip(lookaheadPos, true) // context_match=true
			if skip == SkipYes {
				continue
			}
			if ctx.Buffer.Info[lookaheadPos].GlyphID == glyph {
				found = true
				break
			}
			if skip == SkipMaybe {
				continue
			}
			return false
		}
		if !found {
			return false
		}
	}

	// Check backtrack using skippy-iteration (in reverse order)
	// HarfBuzz: match_backtrack() in hb-ot-layout-gsubgpos.hh:1569-1601
	backtrackPos := ctx.Buffer.Idx
	for _, glyph := range rule.Backtrack {
		found := false
		for backtrackPos > 0 {
			backtrackPos--
			info := &ctx.Buffer.Info[backtrackPos]
			skip := ctx.MaySkipInfo(info, true) // context_match=true
			if skip == SkipYes {
				continue
			}
			if info.GlyphID == glyph {
				found = true
				break
			}
			if skip == SkipMaybe {
				continue
			}
			return false
		}
		if !found {
			return false
		}
	}

	// Store match positions for use in applyLookups
	ctx.MatchPositions = matchPositions
	return true
}

// applyFormat2 applies ChainContextPosFormat2 (class-based context).
func (ccp *ChainContextPos) applyFormat2(ctx *OTApplyContext) bool {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	if ccp.coverage.GetCoverage(glyph) == NotCovered {
		return false
	}

	classIndex := ccp.inputClassDef.GetClass(glyph)
	if classIndex >= len(ccp.chainRuleSets) {
		return false
	}

	ruleSet := ccp.chainRuleSets[classIndex]
	for _, rule := range ruleSet {
		if ccp.matchRuleFormat2(ctx, &rule) {
			inputLen := len(rule.Input) + 1
			ccp.applyLookups(ctx, rule.LookupRecords, inputLen)
			// Advance past matched glyphs (use matchPositions for skippy-aware end)
			matchEnd := ctx.MatchPositions[inputLen-1] + 1
			ctx.MatchPositions = nil
			ctx.Buffer.Idx = matchEnd
			return true
		}
	}

	return false
}

func (ccp *ChainContextPos) matchRuleFormat2(ctx *OTApplyContext, rule *GPOSChainRule) bool {
	inputLen := len(rule.Input) + 1
	bufLen := len(ctx.Buffer.Info)

	// Match input sequence using skippy-iteration (starting from second glyph)
	// HarfBuzz: match_input() in hb-ot-layout-gsubgpos.hh:1311-1400
	matchPositions := make([]int, inputLen)
	matchPositions[0] = ctx.Buffer.Idx
	pos := ctx.Buffer.Idx
	for i := 0; i < len(rule.Input); i++ {
		found := false
		for pos < bufLen-1 {
			pos++
			skip := ctx.MaySkip(pos, false) // context_match=false for input matching
			if skip == SkipYes {
				continue
			}
			glyphClass := ccp.inputClassDef.GetClass(ctx.Buffer.Info[pos].GlyphID)
			if glyphClass == int(rule.Input[i]) {
				found = true
				break
			}
			if skip == SkipMaybe {
				continue
			}
			return false
		}
		if !found {
			return false
		}
		matchPositions[i+1] = pos
	}
	matchEnd := pos + 1

	// Check lookahead classes using skippy-iteration
	// HarfBuzz: match_lookahead() in hb-ot-layout-gsubgpos.hh:1607-1637
	lookaheadPos := pos
	for _, classValue := range rule.Lookahead {
		found := false
		for lookaheadPos < bufLen-1 {
			lookaheadPos++
			skip := ctx.MaySkip(lookaheadPos, true) // context_match=true
			if skip == SkipYes {
				continue
			}
			glyphClass := ccp.lookaheadClassDef.GetClass(ctx.Buffer.Info[lookaheadPos].GlyphID)
			if glyphClass == int(classValue) {
				found = true
				break
			}
			if skip == SkipMaybe {
				continue
			}
			return false
		}
		if !found {
			return false
		}
	}

	// Check backtrack classes using skippy-iteration (in reverse order)
	// HarfBuzz: match_backtrack() in hb-ot-layout-gsubgpos.hh:1569-1601
	backtrackPos := ctx.Buffer.Idx
	for _, classValue := range rule.Backtrack {
		found := false
		for backtrackPos > 0 {
			backtrackPos--
			info := &ctx.Buffer.Info[backtrackPos]
			skip := ctx.MaySkipInfo(info, true) // context_match=true
			if skip == SkipYes {
				continue
			}
			glyphClass := ccp.backtrackClassDef.GetClass(info.GlyphID)
			if glyphClass == int(classValue) {
				found = true
				break
			}
			if skip == SkipMaybe {
				continue
			}
			return false
		}
		if !found {
			return false
		}
	}

	// Store match positions for use in applyLookups
	ctx.MatchPositions = matchPositions
	_ = matchEnd
	return true
}

// applyFormat3 applies ChainContextPosFormat3 (coverage-based context).
// HarfBuzz equivalent: ChainContextFormat3::apply() in hb-ot-layout-gsubgpos.hh
// Uses skippy-iteration for matching (skips marks/default ignorables per LookupFlag).
func (ccp *ChainContextPos) applyFormat3(ctx *OTApplyContext) bool {
	inputLen := len(ccp.inputCoverages)
	if inputLen == 0 {
		return false
	}

	// Check first coverage (current glyph)
	if ccp.inputCoverages[0].GetCoverage(ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID) == NotCovered {
		return false
	}

	// Build match positions using skippy-iteration (like HarfBuzz match_input)
	// HarfBuzz: hb-ot-layout-gsubgpos.hh:1311-1400 (match_input)
	// MatchPositions[0] = ctx.Buffer.Idx (current position)
	// MatchPositions[1..] = positions of subsequent input glyphs (skipping marks if needed)
	//
	// HarfBuzz match() logic:
	// 1. may_skip == SKIP_YES -> continue (skip)
	// 2. may_match (coverage check) == YES -> return MATCH
	// 3. may_match == MAYBE && may_skip == SKIP_NO -> return MATCH
	// 4. may_skip == SKIP_NO -> return NOT_MATCH (fail)
	// 5. else (SKIP_MAYBE with non-match) -> continue (skip)
	matchPositions := make([]int, inputLen)
	matchPositions[0] = ctx.Buffer.Idx

	// Match remaining input sequence by coverage using skippy-iteration
	// HarfBuzz: uses iter_input (context_match=false) with coverage matching
	bufLen := len(ctx.Buffer.Info)
	pos := ctx.Buffer.Idx
	for i := 1; i < inputLen; i++ {
		cov := ccp.inputCoverages[i]
		found := false
		for pos < bufLen-1 {
			pos++
			skip := ctx.MaySkip(pos, false) // context_match=false for input matching
			if skip == SkipYes {
				continue // Definitely skip (e.g., ignored by LookupFlag)
			}
			glyph := ctx.Buffer.Info[pos].GlyphID
			// Check if glyph is in coverage
			if cov.GetCoverage(glyph) != NotCovered {
				found = true
				break // Found matching glyph in coverage
			}
			// Not in coverage - can we skip it?
			if skip == SkipMaybe {
				continue // Skip default ignorables if not in coverage
			}
			// Not in coverage and can't skip -> fail
			return false
		}
		if !found {
			return false
		}
		matchPositions[i] = pos
	}
	matchEnd := pos + 1 // End position after last matched input glyph

	// Check lookahead by coverage (continue from last input position)
	// HarfBuzz: hb-ot-layout-gsubgpos.hh:1607-1637 (match_lookahead)
	// HarfBuzz: uses iter_context (context_match=true) with coverage matching
	//
	// For GPOS: ignore_hidden=true, so CGJ and other hidden default ignorables
	// are skipped automatically via MaySkip with contextMatch=true.
	lookaheadPos := pos
	for _, cov := range ccp.lookaheadCoverages {
		found := false
		for lookaheadPos < bufLen-1 {
			lookaheadPos++
			skip := ctx.MaySkip(lookaheadPos, true) // context_match=true
			if skip == SkipYes {
				continue // Definitely skip (e.g., ignored by LookupFlag)
			}
			glyph := ctx.Buffer.Info[lookaheadPos].GlyphID
			// First check if glyph is in coverage
			if cov.GetCoverage(glyph) != NotCovered {
				found = true
				break // Found matching glyph in coverage
			}
			// Not in coverage - can we skip it?
			if skip == SkipMaybe {
				continue // Skip default ignorables (like CGJ) if not in coverage
			}
			// Not in coverage and can't skip -> fail
			return false
		}
		if !found {
			return false
		}
	}

	// Check backtrack by coverage (in reverse order, starting before current position)
	// HarfBuzz: hb-ot-layout-gsubgpos.hh:1569-1601 (match_backtrack)
	// HarfBuzz: uses iter_context (context_match=true) with coverage matching
	//
	// Note: For GPOS, we use input buffer (not output buffer like GSUB)
	// because GPOS doesn't modify glyphs, only positions.
	backtrackPos := ctx.Buffer.Idx
	for _, cov := range ccp.backtrackCoverages {
		found := false
		for backtrackPos > 0 {
			backtrackPos--
			info := &ctx.Buffer.Info[backtrackPos]
			skip := ctx.MaySkipInfo(info, true) // context_match=true
			if skip == SkipYes {
				continue // Definitely skip (e.g., ignored by LookupFlag)
			}
			// First check if glyph is in coverage
			if cov.GetCoverage(info.GlyphID) != NotCovered {
				found = true
				break // Found matching glyph in coverage
			}
			// Not in coverage - can we skip it?
			if skip == SkipMaybe {
				continue // Skip default ignorables (like CGJ) if not in coverage
			}
			// Not in coverage and can't skip -> fail
			return false
		}
		if !found {
			return false
		}
	}

	// Store match positions for use in applyLookups
	ctx.MatchPositions = matchPositions

	ccp.applyLookups(ctx, ccp.lookupRecords, inputLen)

	// Clear match positions to avoid side effects on subsequent lookups
	ctx.MatchPositions = nil

	ctx.Buffer.Idx = matchEnd
	return true
}

// applyLookups applies nested lookups for ChainContextPos.
// HarfBuzz equivalent: apply_lookup() in hb-ot-layout-gsubgpos.hh:1788-1909
// Uses ctx.Recurse() for HarfBuzz-conformant nested lookup application.
func (ccp *ChainContextPos) applyLookups(ctx *OTApplyContext, lookupRecords []GPOSLookupRecord, inputLen int) {
	// Save original buffer index
	originalIdx := ctx.Buffer.Idx

	for _, record := range lookupRecords {
		idx := int(record.SequenceIndex)
		if idx >= inputLen {
			continue
		}

		// HarfBuzz: buffer->move_to(c->match_positions.arrayZ[idx])
		// Use stored match positions from context matching (accounts for skipped glyphs)
		if ctx.MatchPositions != nil && idx < len(ctx.MatchPositions) {
			ctx.Buffer.Idx = ctx.MatchPositions[idx]
		} else {
			// Fallback for Format1/2 where match positions may not be stored
			ctx.Buffer.Idx = originalIdx + idx
		}

		// Apply nested lookup using ctx.Recurse()
		// HarfBuzz equivalent: recurse(r.lookupListIndex)
		ctx.Recurse(int(record.LookupIndex))
	}

	// Restore original buffer index
	ctx.Buffer.Idx = originalIdx
}
