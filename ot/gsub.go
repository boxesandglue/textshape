package ot

// GSUB (Glyph Substitution) Table Implementation
//
// This file implements OpenType GSUB table parsing and application.
// HarfBuzz equivalent files:
//   - hb-ot-layout-gsub-table.hh (Table definitions, substitution logic)
//   - hb-ot-layout-gsubgpos.hh (Shared lookup application logic)
//
// GSUB Lookup Types implemented:
//   Type 1: SingleSubst (Lines ~50-150)
//   Type 2: MultipleSubst (Lines ~150-300)
//   Type 3: AlternateSubst (Lines ~300-450)
//   Type 4: LigatureSubst (Lines ~450-800)
//   Type 5: ContextSubst (Lines ~800-1200)
//   Type 6: ChainContextSubst (Lines ~2400-2900)
//   Type 8: ReverseChainSingleSubst (Lines ~2900-3000)
//
// Key functions:
//   - ApplyLookupWithMask: Apply lookup with feature mask (Line ~1692)
//   - ApplyLookupToBufferWithMask: Apply to buffer with mask (Line ~1800)
//   - ContextSubst.applyLookups: Context lookup with moveTo pattern (Line ~1040)

import (
	"encoding/binary"
	"sort"
)

// NotCovered is returned when a glyph is not in a coverage table.
const NotCovered = ^uint32(0)

// GSUB lookup types
const (
	GSUBTypeSingle             = 1
	GSUBTypeMultiple           = 2
	GSUBTypeAlternate          = 3
	GSUBTypeLigature           = 4
	GSUBTypeContext            = 5
	GSUBTypeChainContext       = 6
	GSUBTypeExtension          = 7
	GSUBTypeReverseChainSingle = 8
)

// Coverage represents an OpenType Coverage table.
// It maps glyph IDs to coverage indices.
type Coverage struct {
	format uint16
	data   []byte
	offset int // offset to coverage table in data

	// Format 1: sorted array of glyphs
	glyphCount int
	glyphsOff  int

	// Format 2: range records
	rangeCount int
	rangesOff  int
}

// ParseCoverage parses a Coverage table from data at the given offset.
func ParseCoverage(data []byte, offset int) (*Coverage, error) {
	if offset+4 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])

	c := &Coverage{
		format: format,
		data:   data,
		offset: offset,
	}

	switch format {
	case 1:
		// Format 1: Array of GlyphIDs
		glyphCount := int(binary.BigEndian.Uint16(data[offset+2:]))
		if offset+4+glyphCount*2 > len(data) {
			return nil, ErrInvalidOffset
		}
		c.glyphCount = glyphCount
		c.glyphsOff = offset + 4
		return c, nil

	case 2:
		// Format 2: Range records
		rangeCount := int(binary.BigEndian.Uint16(data[offset+2:]))
		if offset+4+rangeCount*6 > len(data) {
			return nil, ErrInvalidOffset
		}
		c.rangeCount = rangeCount
		c.rangesOff = offset + 4
		return c, nil

	default:
		return nil, ErrInvalidFormat
	}
}

// GetCoverage returns the coverage index for a glyph ID, or NotCovered if not found.
func (c *Coverage) GetCoverage(glyph GlyphID) uint32 {
	switch c.format {
	case 1:
		return c.getCoverageFormat1(glyph)
	case 2:
		return c.getCoverageFormat2(glyph)
	default:
		return NotCovered
	}
}

// getCoverageFormat1 performs binary search on sorted glyph array.
func (c *Coverage) getCoverageFormat1(glyph GlyphID) uint32 {
	lo, hi := 0, c.glyphCount
	for lo < hi {
		mid := (lo + hi) / 2
		g := binary.BigEndian.Uint16(c.data[c.glyphsOff+mid*2:])
		if glyph < GlyphID(g) {
			hi = mid
		} else if glyph > GlyphID(g) {
			lo = mid + 1
		} else {
			return uint32(mid)
		}
	}

	return NotCovered
}

// getCoverageFormat2 performs binary search on range records.
func (c *Coverage) getCoverageFormat2(glyph GlyphID) uint32 {
	lo, hi := 0, c.rangeCount
	for lo < hi {
		mid := (lo + hi) / 2
		off := c.rangesOff + mid*6
		startGlyph := binary.BigEndian.Uint16(c.data[off:])
		endGlyph := binary.BigEndian.Uint16(c.data[off+2:])

		if glyph < GlyphID(startGlyph) {
			hi = mid
		} else if glyph > GlyphID(endGlyph) {
			lo = mid + 1
		} else {
			// Found: coverage index = startCoverageIndex + (glyph - startGlyph)
			startCoverageIndex := binary.BigEndian.Uint16(c.data[off+4:])
			return uint32(startCoverageIndex) + uint32(glyph-GlyphID(startGlyph))
		}
	}

	return NotCovered
}

// Glyphs returns all glyphs covered by this coverage table.
func (c *Coverage) Glyphs() []GlyphID {
	var glyphs []GlyphID

	switch c.format {
	case 1:
		// Format 1: sorted array of glyphs
		glyphs = make([]GlyphID, c.glyphCount)
		for i := 0; i < c.glyphCount; i++ {
			glyphs[i] = GlyphID(binary.BigEndian.Uint16(c.data[c.glyphsOff+i*2:]))
		}
	case 2:
		// Format 2: range records
		for i := 0; i < c.rangeCount; i++ {
			off := c.rangesOff + i*6
			startGlyph := GlyphID(binary.BigEndian.Uint16(c.data[off:]))
			endGlyph := GlyphID(binary.BigEndian.Uint16(c.data[off+2:]))
			for g := startGlyph; g <= endGlyph; g++ {
				glyphs = append(glyphs, g)
			}
		}
	}

	return glyphs
}

// GSUB represents the Glyph Substitution table.
type GSUB struct {
	data        []byte
	version     uint32
	scriptList  uint16 // offset to script list
	featureList uint16 // offset to feature list
	lookupList  uint16 // offset to lookup list

	// Parsed lookup list
	lookups []*GSUBLookup

	// FeatureVariations (GSUB version 1.1+ only)
	// HarfBuzz: hb-ot-layout-common.hh - struct GSUBGPOS
	featureVariations *FeatureVariations
}

// ParseGSUB parses a GSUB table from data.
func ParseGSUB(data []byte) (*GSUB, error) {
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

	gsub := &GSUB{
		data:        data,
		version:     version,
		scriptList:  scriptList,
		featureList: featureList,
		lookupList:  lookupList,
	}

	// Parse lookup list
	if err := gsub.parseLookupList(); err != nil {
		return nil, err
	}

	// Parse FeatureVariations for version 1.1+
	// HarfBuzz: hb-ot-layout-common.hh - GSUBGPOS::get_feature_variations()
	if version >= 0x00010001 && len(data) >= 14 {
		fvOffset := binary.BigEndian.Uint32(data[10:])
		if fvOffset != 0 && int(fvOffset) < len(data) {
			gsub.featureVariations, _ = ParseFeatureVariations(data, int(fvOffset))
		}
	}

	return gsub, nil
}

// parseLookupList parses the lookup list.
func (g *GSUB) parseLookupList() error {
	off := int(g.lookupList)
	if off+2 > len(g.data) {
		return ErrInvalidOffset
	}

	lookupCount := int(binary.BigEndian.Uint16(g.data[off:]))
	if off+2+lookupCount*2 > len(g.data) {
		return ErrInvalidOffset
	}

	g.lookups = make([]*GSUBLookup, lookupCount)

	for i := 0; i < lookupCount; i++ {
		lookupOff := int(binary.BigEndian.Uint16(g.data[off+2+i*2:]))
		lookup, err := parseGSUBLookup(g.data, off+lookupOff, g)
		if err != nil {
			// Continue with nil lookup (will be skipped during application)
			continue
		}
		g.lookups[i] = lookup
	}

	return nil
}

// NumLookups returns the number of lookups in the GSUB table.
func (g *GSUB) NumLookups() int {
	return len(g.lookups)
}

// GetLookup returns the lookup at the given index.
func (g *GSUB) GetLookup(index int) *GSUBLookup {
	if index < 0 || index >= len(g.lookups) {
		return nil
	}
	return g.lookups[index]
}

// FindVariationsIndex finds the matching FeatureVariations record index for the
// given normalized coordinates (in F2DOT14 format).
// Returns VariationsNotFoundIndex if no record matches or no FeatureVariations table exists.
// HarfBuzz: hb_ot_layout_table_find_feature_variations() in hb-ot-layout.cc
func (g *GSUB) FindVariationsIndex(coords []int) uint32 {
	if g.featureVariations == nil {
		return VariationsNotFoundIndex
	}
	return g.featureVariations.FindIndex(coords)
}

// GetFeatureVariations returns the FeatureVariations table, or nil if not present.
func (g *GSUB) GetFeatureVariations() *FeatureVariations {
	return g.featureVariations
}

// GSUBLookup represents a GSUB lookup table.
type GSUBLookup struct {
	Type       uint16
	Flag       uint16
	subtables  []GSUBSubtable
	MarkFilter uint16 // For flag & 0x10
}

// Subtables returns the lookup subtables.
func (l *GSUBLookup) Subtables() []GSUBSubtable {
	return l.subtables
}

// GSUBSubtable is the interface for GSUB lookup subtables.
type GSUBSubtable interface {
	// Apply applies the substitution to the glyph at the current position.
	// Returns the number of glyphs consumed (0 if not applied).
	// HarfBuzz equivalent: subtable dispatch in hb-ot-layout-gsubgpos.hh
	Apply(ctx *OTApplyContext) int
}

// parseGSUBLookup parses a single GSUB lookup.
func parseGSUBLookup(data []byte, offset int, gsub *GSUB) (*GSUBLookup, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookupType := binary.BigEndian.Uint16(data[offset:])
	lookupFlag := binary.BigEndian.Uint16(data[offset+2:])
	subtableCount := int(binary.BigEndian.Uint16(data[offset+4:]))

	if offset+6+subtableCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookup := &GSUBLookup{
		Type:      lookupType,
		Flag:      lookupFlag,
		subtables: make([]GSUBSubtable, 0, subtableCount),
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
		if lookupType == GSUBTypeExtension {
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

		subtable, err := parseGSUBSubtable(data, offset+subtableOff, actualType, gsub)
		if err != nil {
			continue
		}
		if subtable != nil {
			lookup.subtables = append(lookup.subtables, subtable)
		}
	}

	return lookup, nil
}

// parseGSUBSubtable parses a GSUB subtable based on its type.
func parseGSUBSubtable(data []byte, offset int, lookupType uint16, gsub *GSUB) (GSUBSubtable, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	switch lookupType {
	case GSUBTypeSingle:
		return parseSingleSubst(data, offset)
	case GSUBTypeLigature:
		return parseLigatureSubst(data, offset)
	case GSUBTypeMultiple:
		return parseMultipleSubst(data, offset)
	case GSUBTypeAlternate:
		return parseAlternateSubst(data, offset)
	case GSUBTypeContext:
		return parseContextSubst(data, offset, gsub)
	case GSUBTypeChainContext:
		return parseChainContextSubst(data, offset, gsub)
	case GSUBTypeReverseChainSingle:
		return parseReverseChainSingleSubst(data, offset)
	default:
		// Unsupported lookup type
		return nil, nil
	}
}

// --- Single Substitution ---

// SingleSubst represents a Single Substitution subtable.
type SingleSubst struct {
	format   uint16
	coverage *Coverage

	// Format 1: delta
	delta int16

	// Format 2: substitute array
	substitutes []GlyphID
}

func parseSingleSubst(data []byte, offset int) (*SingleSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))

	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	s := &SingleSubst{
		format:   format,
		coverage: coverage,
	}

	switch format {
	case 1:
		// Format 1: deltaGlyphID
		s.delta = int16(binary.BigEndian.Uint16(data[offset+4:]))
		return s, nil

	case 2:
		// Format 2: substitute array
		glyphCount := int(binary.BigEndian.Uint16(data[offset+4:]))
		if offset+6+glyphCount*2 > len(data) {
			return nil, ErrInvalidOffset
		}
		s.substitutes = make([]GlyphID, glyphCount)
		for i := 0; i < glyphCount; i++ {
			s.substitutes[i] = GlyphID(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		}
		return s, nil

	default:
		return nil, ErrInvalidFormat
	}
}

// Apply applies the single substitution.
func (s *SingleSubst) Apply(ctx *OTApplyContext) int {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	coverageIndex := s.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return 0
	}

	var newGlyph GlyphID
	switch s.format {
	case 1:
		newGlyph = GlyphID(int(glyph) + int(s.delta))
	case 2:
		if int(coverageIndex) >= len(s.substitutes) {
			return 0
		}
		newGlyph = s.substitutes[coverageIndex]
	default:
		return 0
	}

	ctx.ReplaceGlyph(newGlyph)
	return 1
}

// Mapping returns all input->output glyph mappings for this substitution.
func (s *SingleSubst) Mapping() map[GlyphID]GlyphID {
	result := make(map[GlyphID]GlyphID)
	glyphs := s.coverage.Glyphs()

	switch s.format {
	case 1:
		// Format 1: apply delta to each covered glyph
		for _, glyph := range glyphs {
			result[glyph] = GlyphID(int(glyph) + int(s.delta))
		}
	case 2:
		// Format 2: direct mapping via coverage index
		for i, glyph := range glyphs {
			if i < len(s.substitutes) {
				result[glyph] = s.substitutes[i]
			}
		}
	}
	return result
}

// --- Multiple Substitution ---

// MultipleSubst represents a Multiple Substitution subtable (1 -> n).
type MultipleSubst struct {
	coverage  *Coverage
	sequences [][]GlyphID // Array of replacement sequences
}

func parseMultipleSubst(data []byte, offset int) (*MultipleSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	seqCount := int(binary.BigEndian.Uint16(data[offset+4:]))
	if offset+6+seqCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	m := &MultipleSubst{
		coverage:  coverage,
		sequences: make([][]GlyphID, seqCount),
	}

	for i := 0; i < seqCount; i++ {
		seqOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		absOff := offset + seqOff
		if absOff+2 > len(data) {
			continue
		}
		glyphCount := int(binary.BigEndian.Uint16(data[absOff:]))
		if absOff+2+glyphCount*2 > len(data) {
			continue
		}
		seq := make([]GlyphID, glyphCount)
		for j := 0; j < glyphCount; j++ {
			seq[j] = GlyphID(binary.BigEndian.Uint16(data[absOff+2+j*2:]))
		}
		m.sequences[i] = seq
	}

	return m, nil
}

// Apply applies the multiple substitution.
func (m *MultipleSubst) Apply(ctx *OTApplyContext) int {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	coverageIndex := m.coverage.GetCoverage(glyph)

	if coverageIndex == NotCovered {
		return 0
	}

	if int(coverageIndex) >= len(m.sequences) {
		return 0
	}

	seq := m.sequences[coverageIndex]
	if len(seq) == 0 {
		// Deletion
		ctx.DeleteGlyph()
		return 1
	}

	ctx.ReplaceGlyphs(seq)
	return 1
}

// Mapping returns the input->output mapping for glyph closure computation.
func (m *MultipleSubst) Mapping() map[GlyphID][]GlyphID {
	result := make(map[GlyphID][]GlyphID)
	glyphs := m.coverage.Glyphs()
	for i, glyph := range glyphs {
		if i < len(m.sequences) {
			result[glyph] = m.sequences[i]
		}
	}
	return result
}

// --- Alternate Substitution ---

// AlternateSubst represents an Alternate Substitution subtable (1 -> 1 from set).
// It allows choosing one glyph from a set of alternatives.
type AlternateSubst struct {
	coverage      *Coverage
	alternateSets [][]GlyphID // Array of alternate glyph sets
}

func parseAlternateSubst(data []byte, offset int) (*AlternateSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	altSetCount := int(binary.BigEndian.Uint16(data[offset+4:]))
	if offset+6+altSetCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	a := &AlternateSubst{
		coverage:      coverage,
		alternateSets: make([][]GlyphID, altSetCount),
	}

	for i := 0; i < altSetCount; i++ {
		altSetOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		absOff := offset + altSetOff
		if absOff+2 > len(data) {
			continue
		}
		glyphCount := int(binary.BigEndian.Uint16(data[absOff:]))
		if absOff+2+glyphCount*2 > len(data) {
			continue
		}
		alts := make([]GlyphID, glyphCount)
		for j := 0; j < glyphCount; j++ {
			alts[j] = GlyphID(binary.BigEndian.Uint16(data[absOff+2+j*2:]))
		}
		a.alternateSets[i] = alts
	}

	return a, nil
}

// Apply applies the alternate substitution.
// By default, it selects the first alternative (index 0).
// Use ApplyWithIndex to select a specific alternative.
func (a *AlternateSubst) Apply(ctx *OTApplyContext) int {
	return a.ApplyWithIndex(ctx, 0)
}

// ApplyWithIndex applies the alternate substitution with a specific alternate index.
// altIndex is 0-based (0 = first alternate, 1 = second, etc.)
func (a *AlternateSubst) ApplyWithIndex(ctx *OTApplyContext, altIndex int) int {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	coverageIndex := a.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return 0
	}

	if int(coverageIndex) >= len(a.alternateSets) {
		return 0
	}

	alts := a.alternateSets[coverageIndex]
	if len(alts) == 0 {
		return 0
	}

	// Clamp altIndex to valid range
	if altIndex < 0 {
		altIndex = 0
	}
	if altIndex >= len(alts) {
		altIndex = len(alts) - 1
	}

	ctx.ReplaceGlyph(alts[altIndex])
	return 1
}

// GetAlternates returns the available alternates for a glyph.
// Returns nil if the glyph is not covered.
func (a *AlternateSubst) GetAlternates(glyph GlyphID) []GlyphID {
	coverageIndex := a.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return nil
	}
	if int(coverageIndex) >= len(a.alternateSets) {
		return nil
	}
	return a.alternateSets[coverageIndex]
}

// Mapping returns the input->alternates mapping for glyph closure computation.
func (a *AlternateSubst) Mapping() map[GlyphID][]GlyphID {
	result := make(map[GlyphID][]GlyphID)
	glyphs := a.coverage.Glyphs()
	for i, glyph := range glyphs {
		if i < len(a.alternateSets) {
			result[glyph] = a.alternateSets[i]
		}
	}
	return result
}

// --- Context Substitution ---

// ContextSubst represents a Context Substitution subtable (GSUB Type 5).
// It matches input sequences and applies nested lookups.
type ContextSubst struct {
	format uint16
	gsub   *GSUB

	// Format 1: Simple glyph contexts
	coverage *Coverage
	ruleSets [][]ContextRule // Indexed by coverage index

	// Format 2: Class-based contexts
	classDef *ClassDef
	// ruleSets also used for format 2 (indexed by class)

	// Format 3: Coverage-based contexts
	inputCoverages []*Coverage
	lookupRecords  []LookupRecord
}

// ContextRule represents a single context rule.
type ContextRule struct {
	Input         []GlyphID      // Input sequence (starting from second glyph)
	LookupRecords []LookupRecord // Lookups to apply
}

func parseContextSubst(data []byte, offset int, gsub *GSUB) (*ContextSubst, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])

	switch format {
	case 1:
		return parseContextFormat1(data, offset, gsub)
	case 2:
		return parseContextFormat2(data, offset, gsub)
	case 3:
		return parseContextFormat3(data, offset, gsub)
	default:
		return nil, ErrInvalidFormat
	}
}

// parseContextFormat1 parses ContextSubstFormat1 (simple glyph context).
func parseContextFormat1(data []byte, offset int, gsub *GSUB) (*ContextSubst, error) {
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

	cs := &ContextSubst{
		format:   1,
		gsub:     gsub,
		coverage: coverage,
		ruleSets: make([][]ContextRule, ruleSetCount),
	}

	for i := 0; i < ruleSetCount; i++ {
		ruleSetOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		if ruleSetOff == 0 {
			continue
		}
		rules, err := parseContextRuleSet(data, offset+ruleSetOff)
		if err != nil {
			continue
		}
		cs.ruleSets[i] = rules
	}

	return cs, nil
}

// parseContextRuleSet parses a RuleSet (array of Rules).
func parseContextRuleSet(data []byte, offset int) ([]ContextRule, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	ruleCount := int(binary.BigEndian.Uint16(data[offset:]))
	if offset+2+ruleCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	rules := make([]ContextRule, 0, ruleCount)

	for i := 0; i < ruleCount; i++ {
		ruleOff := int(binary.BigEndian.Uint16(data[offset+2+i*2:]))
		rule, err := parseContextRule(data, offset+ruleOff)
		if err != nil {
			continue
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// parseContextRule parses a single Rule.
func parseContextRule(data []byte, offset int) (ContextRule, error) {
	var rule ContextRule

	if offset+4 > len(data) {
		return rule, ErrInvalidOffset
	}

	// inputCount includes first glyph
	inputCount := int(binary.BigEndian.Uint16(data[offset:]))
	lookupCount := int(binary.BigEndian.Uint16(data[offset+2:]))

	inputLen := inputCount - 1
	if inputLen < 0 {
		inputLen = 0
	}

	off := offset + 4
	if off+inputLen*2 > len(data) {
		return rule, ErrInvalidOffset
	}

	rule.Input = make([]GlyphID, inputLen)
	for i := 0; i < inputLen; i++ {
		rule.Input[i] = GlyphID(binary.BigEndian.Uint16(data[off+i*2:]))
	}
	off += inputLen * 2

	if off+lookupCount*4 > len(data) {
		return rule, ErrInvalidOffset
	}

	rule.LookupRecords = make([]LookupRecord, lookupCount)
	for i := 0; i < lookupCount; i++ {
		rule.LookupRecords[i].SequenceIndex = binary.BigEndian.Uint16(data[off+i*4:])
		rule.LookupRecords[i].LookupIndex = binary.BigEndian.Uint16(data[off+i*4+2:])
	}

	return rule, nil
}

// parseContextFormat2 parses ContextSubstFormat2 (class-based context).
func parseContextFormat2(data []byte, offset int, gsub *GSUB) (*ContextSubst, error) {
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

	cs := &ContextSubst{
		format:   2,
		gsub:     gsub,
		coverage: coverage,
		classDef: classDef,
		ruleSets: make([][]ContextRule, ruleSetCount),
	}

	for i := 0; i < ruleSetCount; i++ {
		ruleSetOff := int(binary.BigEndian.Uint16(data[offset+8+i*2:]))
		if ruleSetOff == 0 {
			continue
		}
		rules, err := parseContextRuleSet(data, offset+ruleSetOff)
		if err != nil {
			continue
		}
		cs.ruleSets[i] = rules
	}

	return cs, nil
}

// parseContextFormat3 parses ContextSubstFormat3 (coverage-based context).
func parseContextFormat3(data []byte, offset int, gsub *GSUB) (*ContextSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	glyphCount := int(binary.BigEndian.Uint16(data[offset+2:]))
	lookupCount := int(binary.BigEndian.Uint16(data[offset+4:]))

	if offset+6+glyphCount*2+lookupCount*4 > len(data) {
		return nil, ErrInvalidOffset
	}

	inputCoverages := make([]*Coverage, glyphCount)
	off := offset + 6
	for i := 0; i < glyphCount; i++ {
		covOff := int(binary.BigEndian.Uint16(data[off+i*2:]))
		cov, err := ParseCoverage(data, offset+covOff)
		if err != nil {
			return nil, err
		}
		inputCoverages[i] = cov
	}
	off += glyphCount * 2

	lookupRecords := make([]LookupRecord, lookupCount)
	for i := 0; i < lookupCount; i++ {
		lookupRecords[i].SequenceIndex = binary.BigEndian.Uint16(data[off+i*4:])
		lookupRecords[i].LookupIndex = binary.BigEndian.Uint16(data[off+i*4+2:])
	}

	return &ContextSubst{
		format:         3,
		gsub:           gsub,
		inputCoverages: inputCoverages,
		lookupRecords:  lookupRecords,
	}, nil
}

// Apply applies the context substitution.
func (cs *ContextSubst) Apply(ctx *OTApplyContext) int {
	switch cs.format {
	case 1:
		return cs.applyFormat1(ctx)
	case 2:
		return cs.applyFormat2(ctx)
	case 3:
		return cs.applyFormat3(ctx)
	default:
		return 0
	}
}

// applyFormat1 applies ContextSubstFormat1 (simple glyph context).
func (cs *ContextSubst) applyFormat1(ctx *OTApplyContext) int {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	coverageIndex := cs.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return 0
	}

	if int(coverageIndex) >= len(cs.ruleSets) {
		return 0
	}

	rules := cs.ruleSets[coverageIndex]
	for _, rule := range rules {
		if cs.matchRuleFormat1(ctx, &rule) {
			cs.applyLookups(ctx, rule.LookupRecords, len(rule.Input)+1)
			return 1
		}
	}

	return 0
}

// matchRuleFormat1 checks if a ContextRule matches at the current position (Format 1).
// This version uses skippy iteration to respect LookupFlags (IgnoreMarks, etc).
func (cs *ContextSubst) matchRuleFormat1(ctx *OTApplyContext, rule *ContextRule) bool {
	// Find match positions for each input glyph, skipping ignored glyphs
	matchPositions := make([]int, len(rule.Input)+1)
	matchPositions[0] = ctx.Buffer.Idx

	pos := ctx.Buffer.Idx
	for i, g := range rule.Input {
		// Find next non-skipped glyph
		// NextGlyph(pos) searches from pos+1, so we pass the current position
		pos = ctx.NextGlyph(pos)
		if pos == -1 {
			return false
		}
		if ctx.Buffer.Info[pos].GlyphID != g {
			return false
		}
		matchPositions[i+1] = pos
	}

	// Store match positions for applyLookups
	ctx.MatchPositions = matchPositions
	return true
}

// applyFormat2 applies ContextSubstFormat2 (class-based context).
func (cs *ContextSubst) applyFormat2(ctx *OTApplyContext) int {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	if cs.coverage.GetCoverage(glyph) == NotCovered {
		return 0
	}

	inputClass := cs.classDef.GetClass(glyph)
	if inputClass < 0 || inputClass >= len(cs.ruleSets) {
		return 0
	}

	rules := cs.ruleSets[inputClass]
	for _, rule := range rules {
		if cs.matchRuleFormat2(ctx, &rule) {
			cs.applyLookups(ctx, rule.LookupRecords, len(rule.Input)+1)
			return 1
		}
	}

	return 0
}

// matchRuleFormat2 checks if a ContextRule matches at the current position (Format 2).
// This version uses skippy iteration to respect LookupFlags (IgnoreMarks, etc).
func (cs *ContextSubst) matchRuleFormat2(ctx *OTApplyContext, rule *ContextRule) bool {
	// Find match positions for each input glyph, skipping ignored glyphs
	matchPositions := make([]int, len(rule.Input)+1)
	matchPositions[0] = ctx.Buffer.Idx

	pos := ctx.Buffer.Idx
	for i, classID := range rule.Input {
		// Find next non-skipped glyph
		// NextGlyph(pos) searches from pos+1, so we pass the current position
		pos = ctx.NextGlyph(pos)
		if pos == -1 {
			return false
		}
		glyphClass := cs.classDef.GetClass(ctx.Buffer.Info[pos].GlyphID)
		if glyphClass != int(classID) {
			return false
		}
		matchPositions[i+1] = pos
	}

	// Store match positions for applyLookups
	ctx.MatchPositions = matchPositions
	return true
}

// applyFormat3 applies ContextSubstFormat3 (coverage-based context).
// HarfBuzz equivalent: ContextFormat3::apply() which calls context_apply_lookup() -> match_input()
func (cs *ContextSubst) applyFormat3(ctx *OTApplyContext) int {
	inputLen := len(cs.inputCoverages)
	if inputLen == 0 {
		return 0
	}

	// Check first coverage (current glyph)
	if cs.inputCoverages[0].GetCoverage(ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID) == NotCovered {
		return 0
	}

	// Build match positions using skippy-iteration (like HarfBuzz match_input)
	matchPositions := make([]int, inputLen)
	matchPositions[0] = ctx.Buffer.Idx

	// Match remaining input sequence by coverage using skippy-iteration
	pos := ctx.Buffer.Idx
	for i := 1; i < inputLen; i++ {
		pos = ctx.NextGlyph(pos)
		if pos < 0 {
			return 0
		}
		if cs.inputCoverages[i].GetCoverage(ctx.Buffer.Info[pos].GlyphID) == NotCovered {
			return 0
		}
		matchPositions[i] = pos
	}

	// Store match positions for use in applyLookups
	ctx.MatchPositions = matchPositions

	cs.applyLookups(ctx, cs.lookupRecords, inputLen)
	return 1
}

// applyLookups applies lookup records to matched input positions.
// HarfBuzz equivalent: apply_lookup() in hb-ot-layout-gsubgpos.hh:1772-1912
func (cs *ContextSubst) applyLookups(ctx *OTApplyContext, lookupRecords []LookupRecord, count int) {
	if cs.gsub == nil {
		ctx.Buffer.Idx += count
		return
	}

	// Check recursion limit
	if ctx.NestingLevel >= MaxNestingLevel {
		ctx.Buffer.Idx += count
		return
	}

	buffer := ctx.Buffer
	if !buffer.haveOutput {
		buffer.Idx += count
		return
	}

	// Validate MatchPositions
	if ctx.MatchPositions == nil || len(ctx.MatchPositions) < count {
		// Fallback: create consecutive positions
		ctx.MatchPositions = make([]int, count)
		for i := 0; i < count; i++ {
			ctx.MatchPositions[i] = buffer.Idx + i
		}
	}

	// HarfBuzz: apply_lookup() lines 1781-1791
	// "All positions are distance from beginning of *output* buffer. Adjust."
	bl := buffer.outLen // backtrack_len()
	matchEnd := ctx.MatchPositions[count-1] + 1
	end := bl + matchEnd - buffer.Idx

	delta := bl - buffer.Idx
	// Convert positions to new indexing (in-place modification like HarfBuzz)
	for j := 0; j < count; j++ {
		ctx.MatchPositions[j] += delta
	}

	// Apply each lookup record
	for _, record := range lookupRecords {
		idx := int(record.SequenceIndex)
		if idx >= count {
			continue
		}

		// HarfBuzz: orig_len = backtrack_len() + lookahead_len()
		origLen := buffer.outLen + (len(buffer.Info) - buffer.Idx)

		// HarfBuzz: "This can happen if earlier recursed lookups deleted many entries."
		if ctx.MatchPositions[idx] >= origLen {
			continue
		}

		// Move to the target position
		if !buffer.moveTo(ctx.MatchPositions[idx]) {
			break
		}

		lookup := cs.gsub.GetLookup(int(record.LookupIndex))
		if lookup == nil {
			continue
		}

		// Determine mark filtering set for nested lookup
		nestedMarkFilteringSet := -1
		if lookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
			nestedMarkFilteringSet = int(lookup.MarkFilter)
		}

		// Apply nested lookup
		nestedCtx := &OTApplyContext{
			Buffer:           buffer,
			LookupFlag:       lookup.Flag,
			GDEF:             ctx.GDEF,
			HasGlyphClasses:  ctx.HasGlyphClasses,
			MarkFilteringSet: nestedMarkFilteringSet,
			NestingLevel:     ctx.NestingLevel + 1,
			FeatureMask:      ctx.FeatureMask,
			Font:             ctx.Font,
		}

		applied := false
		for _, subtable := range lookup.subtables {
			if subtable.Apply(nestedCtx) > 0 {
				applied = true
				break
			}
		}

		if !applied {
			continue
		}

		// HarfBuzz: new_len = backtrack_len() + lookahead_len()
		newLen := buffer.outLen + (len(buffer.Info) - buffer.Idx)
		delta := newLen - origLen

		if delta == 0 {
			continue
		}

		// HarfBuzz: "Recursed lookup changed buffer len. Adjust."
		end += delta
		if end < ctx.MatchPositions[idx] {
			// HarfBuzz: "End might end up being smaller than match_positions[idx]..."
			delta += ctx.MatchPositions[idx] - end
			end = ctx.MatchPositions[idx]
		}

		next := idx + 1 // next now is the position after the recursed lookup

		if delta > 0 {
			// Ensure MatchPositions has enough capacity
			if count+delta > len(ctx.MatchPositions) {
				newPositions := make([]int, count+delta)
				copy(newPositions, ctx.MatchPositions)
				ctx.MatchPositions = newPositions
			}
		} else {
			// NOTE: delta is non-positive
			if next-count > delta {
				delta = next - count
			}
			next -= delta
		}

		// Shift subsequent positions
		if next < count {
			copy(ctx.MatchPositions[next+delta:], ctx.MatchPositions[next:count])
		}
		next += delta
		count += delta

		// Fill in new entries
		for j := idx + 1; j < next; j++ {
			ctx.MatchPositions[j] = ctx.MatchPositions[j-1] + 1
		}

		// Fixup the rest
		for ; next < count; next++ {
			ctx.MatchPositions[next] += delta
		}
	}

	if end < 0 {
		end = 0
	}
	// Ensure end doesn't exceed the virtual buffer length
	maxEnd := buffer.outLen + (len(buffer.Info) - buffer.Idx)
	if end > maxEnd {
		end = maxEnd
	}
	if !buffer.moveTo(end) {
		// moveTo failed - advance Idx by count to prevent infinite loop
		buffer.Idx += count
	}
}

// --- Ligature Substitution ---

// LigatureSubst represents a Ligature Substitution subtable.
type LigatureSubst struct {
	coverage     *Coverage
	ligatureSets [][]Ligature
}

// Coverage returns the coverage table.
func (l *LigatureSubst) Coverage() *Coverage {
	return l.coverage
}

// LigatureSets returns the ligature sets.
func (l *LigatureSubst) LigatureSets() [][]Ligature {
	return l.ligatureSets
}

// Ligature represents a single ligature rule.
type Ligature struct {
	LigGlyph   GlyphID   // The resulting ligature glyph
	Components []GlyphID // Component glyphs (starting from second)
}

func parseLigatureSubst(data []byte, offset int) (*LigatureSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	ligSetCount := int(binary.BigEndian.Uint16(data[offset+4:]))
	if offset+6+ligSetCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	l := &LigatureSubst{
		coverage:     coverage,
		ligatureSets: make([][]Ligature, ligSetCount),
	}

	for i := 0; i < ligSetCount; i++ {
		ligSetOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		ligatures, err := parseLigatureSet(data, offset+ligSetOff)
		if err != nil {
			continue
		}
		l.ligatureSets[i] = ligatures
	}

	return l, nil
}

func parseLigatureSet(data []byte, offset int) ([]Ligature, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	ligCount := int(binary.BigEndian.Uint16(data[offset:]))
	if offset+2+ligCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	ligatures := make([]Ligature, 0, ligCount)

	for i := 0; i < ligCount; i++ {
		ligOff := int(binary.BigEndian.Uint16(data[offset+2+i*2:]))
		lig, err := parseLigature(data, offset+ligOff)
		if err != nil {
			continue
		}
		ligatures = append(ligatures, lig)
	}

	return ligatures, nil
}

func parseLigature(data []byte, offset int) (Ligature, error) {
	if offset+4 > len(data) {
		return Ligature{}, ErrInvalidOffset
	}

	ligGlyph := GlyphID(binary.BigEndian.Uint16(data[offset:]))
	compCount := int(binary.BigEndian.Uint16(data[offset+2:]))

	// compCount includes first glyph (which is in coverage), so components are compCount-1
	numComponents := compCount - 1
	if numComponents < 0 {
		numComponents = 0
	}

	if offset+4+numComponents*2 > len(data) {
		return Ligature{}, ErrInvalidOffset
	}

	lig := Ligature{
		LigGlyph:   ligGlyph,
		Components: make([]GlyphID, numComponents),
	}

	for i := 0; i < numComponents; i++ {
		lig.Components[i] = GlyphID(binary.BigEndian.Uint16(data[offset+4+i*2:]))
	}

	return lig, nil
}

// Apply applies the ligature substitution.
func (l *LigatureSubst) Apply(ctx *OTApplyContext) int {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	coverageIndex := l.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return 0
	}

	if int(coverageIndex) >= len(l.ligatureSets) {
		return 0
	}

	ligSet := l.ligatureSets[coverageIndex]

	// Try each ligature in order of preference
	for _, lig := range ligSet {
		matchedPositions := l.matchLigature(ctx, &lig)
		if matchedPositions != nil {
			// Apply ligature - replace all matched positions with the ligature glyph
			// The matchedPositions includes all positions that should be consumed
			// (including default ignorables that were skipped during matching)
			ctx.LigatePositions(lig.LigGlyph, matchedPositions)
			return 1
		}
	}

	return 0
}

// matchLigature checks if the ligature matches at the current position.
// Returns the positions of the matched glyphs (components only), or nil if no match.
// HarfBuzz behavior: Glyphs that should be skipped based on LookupFlag (e.g., marks when
// IgnoreMarks is set) are skipped during matching but KEPT in the output.
func (l *LigatureSubst) matchLigature(ctx *OTApplyContext, lig *Ligature) []int {
	// positions contains only the component positions (not skipped glyphs)
	positions := make([]int, 0, len(lig.Components)+1)
	positions = append(positions, ctx.Buffer.Idx) // First glyph is always included

	// For per-syllable matching, get the syllable of the starting glyph
	// HarfBuzz equivalent: per_syllable check in may_match() (hb-ot-layout-gsubgpos.hh:436-439)
	var startSyllable uint8
	if ctx.PerSyllable {
		startSyllable = ctx.Buffer.Info[ctx.Buffer.Idx].Syllable
	}

	pos := ctx.Buffer.Idx + 1
	for _, comp := range lig.Components {
		// Skip glyphs based on LookupFlag (marks, ligatures, etc.)
		// HarfBuzz: uses skippy_iter to skip based on lookup flags
		for pos < len(ctx.Buffer.Info) && ctx.ShouldSkipGlyph(pos) {
			pos++
		}

		if pos >= len(ctx.Buffer.Info) {
			return nil // Not enough glyphs
		}

		// Per-syllable check: all matched glyphs must be in the same syllable
		// HarfBuzz equivalent: per_syllable && syllable != info.syllable() in may_match()
		// HarfBuzz: if (per_syllable && syllable && syllable != info.syllable()) return MATCH_NO
		// Note: syllable==0 means no syllable assigned → skip the check
		if ctx.PerSyllable && startSyllable != 0 && ctx.Buffer.Info[pos].Syllable != startSyllable {
			return nil // Cross-syllable match not allowed
		}

		if ctx.Buffer.Info[pos].GlyphID != comp {
			return nil // Component doesn't match
		}

		positions = append(positions, pos)
		pos++
	}

	return positions
}

// --- GSUBContext ---

// GSUBContext provides context for GSUB application.
// MaxNestingLevel is the maximum recursion depth for nested lookups.
// HarfBuzz equivalent: HB_MAX_NESTING_LEVEL
const MaxNestingLevel = 64

// IsDefaultIgnorable returns true if the codepoint is a Unicode Default Ignorable.
// Based on HarfBuzz hb-unicode.hh is_default_ignorable().
func IsDefaultIgnorable(cp Codepoint) bool {
	plane := cp >> 16
	if plane == 0 {
		// BMP
		page := cp >> 8
		switch page {
		case 0x00:
			return cp == 0x00AD // SOFT HYPHEN
		case 0x03:
			return cp == 0x034F // COMBINING GRAPHEME JOINER
		case 0x06:
			return cp == 0x061C // ARABIC LETTER MARK
		case 0x17:
			return cp >= 0x17B4 && cp <= 0x17B5 // Khmer vowels
		case 0x18:
			return cp >= 0x180B && cp <= 0x180E // Mongolian FVS
		case 0x20:
			return (cp >= 0x200B && cp <= 0x200F) || // Zero-width, directional
				(cp >= 0x202A && cp <= 0x202E) || // Directional overrides
				(cp >= 0x2060 && cp <= 0x206F) // Word joiner, etc.
		case 0xFE:
			return (cp >= 0xFE00 && cp <= 0xFE0F) || cp == 0xFEFF // Variation Selectors, BOM
		case 0xFF:
			return cp >= 0xFFF0 && cp <= 0xFFF8 // Specials
		}
		return false
	}
	if plane == 1 {
		// Plane 1: Variation Selectors Supplement
		return cp >= 0xE0100 && cp <= 0xE01EF
	}
	if plane == 14 {
		// Plane 14: Tags
		return cp >= 0xE0000 && cp <= 0xE0FFF
	}
	return false
}

// isHiddenDefaultIgnorable returns true if the codepoint should have the HIDDEN flag.
// HarfBuzz: UPROPS_MASK_HIDDEN in hb-ot-layout.hh:199, 233-243
// These characters should NOT be skipped during GSUB context matching (ignore_hidden=false).
// Set for: CGJ (U+034F), Mongolian FVS (U+180B-U+180D, U+180F), TAG chars (U+E0020-U+E007F)
func isHiddenDefaultIgnorable(cp Codepoint) bool {
	// CGJ (Combining Grapheme Joiner)
	// https://github.com/harfbuzz/harfbuzz/issues/554
	if cp == 0x034F {
		return true
	}
	// Mongolian Free Variation Selectors
	// https://github.com/harfbuzz/harfbuzz/issues/234
	if (cp >= 0x180B && cp <= 0x180D) || cp == 0x180F {
		return true
	}
	// TAG characters
	// https://github.com/harfbuzz/harfbuzz/issues/463
	if cp >= 0xE0020 && cp <= 0xE007F {
		return true
	}
	return false
}

// IsVariationSelector returns true if the codepoint is a Variation Selector.
// Includes: U+180B-U+180D, U+180F (Mongolian FVS), U+FE00-U+FE0F (VS1-VS16),
// and U+E0100-U+E01EF (VS17-VS256).
func IsVariationSelector(cp Codepoint) bool {
	// Mongolian Free Variation Selectors: U+180B-U+180D, U+180F
	// These are used for Mongolian script variant forms
	if cp >= 0x180B && cp <= 0x180D {
		return true
	}
	if cp == 0x180F {
		return true
	}
	// VS1-VS16 in BMP
	if cp >= 0xFE00 && cp <= 0xFE0F {
		return true
	}
	// VS17-VS256 in Plane 14 (Supplementary Special-purpose Plane)
	if cp >= 0xE0100 && cp <= 0xE01EF {
		return true
	}
	return false
}

// --- Feature/Script lookup ---

// FeatureList represents a GSUB/GPOS FeatureList.
type FeatureList struct {
	data   []byte
	offset int
	count  int
}

// ParseFeatureList parses a FeatureList from a GSUB/GPOS table.
func (g *GSUB) ParseFeatureList() (*FeatureList, error) {
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

// FeatureRecord represents a parsed feature record with its lookup indices.
// This is the internal representation from the font's FeatureList table.
type FeatureRecord struct {
	Tag     Tag
	Lookups []uint16
}

// GetFeature returns the feature record at the given index.
func (f *FeatureList) GetFeature(index int) (*FeatureRecord, error) {
	if index < 0 || index >= f.count {
		return nil, ErrInvalidOffset
	}

	recordOff := f.offset + 2 + index*6
	tag := Tag(binary.BigEndian.Uint32(f.data[recordOff:]))
	featureOff := int(binary.BigEndian.Uint16(f.data[recordOff+4:]))

	absOff := f.offset + featureOff
	if absOff+4 > len(f.data) {
		return nil, ErrInvalidOffset
	}

	// Skip featureParams offset
	lookupCount := int(binary.BigEndian.Uint16(f.data[absOff+2:]))
	if absOff+4+lookupCount*2 > len(f.data) {
		return nil, ErrInvalidOffset
	}

	feat := &FeatureRecord{
		Tag:     tag,
		Lookups: make([]uint16, lookupCount),
	}

	for i := 0; i < lookupCount; i++ {
		feat.Lookups[i] = binary.BigEndian.Uint16(f.data[absOff+4+i*2:])
	}

	return feat, nil
}

// FindFeature finds a feature by tag and returns its lookup indices.
func (f *FeatureList) FindFeature(tag Tag) []uint16 {
	// Collect unique lookup indices from all features with matching tag
	lookupSet := make(map[uint16]bool)
	for i := 0; i < f.count; i++ {
		feat, err := f.GetFeature(i)
		if err != nil {
			continue
		}
		if feat.Tag == tag {
			for _, idx := range feat.Lookups {
				lookupSet[idx] = true
			}
		}
	}

	if len(lookupSet) == 0 {
		return nil
	}

	// Convert to sorted slice
	lookups := make([]uint16, 0, len(lookupSet))
	for idx := range lookupSet {
		lookups = append(lookups, idx)
	}
	// Sort to ensure consistent application order
	for i := 0; i < len(lookups)-1; i++ {
		for j := i + 1; j < len(lookups); j++ {
			if lookups[j] < lookups[i] {
				lookups[i], lookups[j] = lookups[j], lookups[i]
			}
		}
	}
	return lookups
}

// Count returns the number of features.
func (f *FeatureList) Count() int {
	return f.count
}

// FindFirstFeature finds the first feature with the given tag and returns its lookup indices.
// This is useful when a font has multiple features with the same tag (e.g., for different scripts)
// and you want to use the default/first one (typically the default script's feature).
func (f *FeatureList) FindFirstFeature(tag Tag) []uint16 {
	for i := 0; i < f.count; i++ {
		feat, err := f.GetFeature(i)
		if err != nil {
			continue
		}
		if feat.Tag == tag {
			return feat.Lookups
		}
	}
	return nil
}

// FindFeatureWithVariations returns lookup indices for a feature tag, considering FeatureVariations.
// This is a global search that considers all features with the matching tag.
func (f *FeatureList) FindFeatureWithVariations(tag Tag, fv *FeatureVariations, variationsIndex uint32) []uint16 {
	lookupSet := make(map[uint16]bool)

	for i := 0; i < f.count; i++ {
		feat, err := f.GetFeature(i)
		if err != nil {
			continue
		}
		if feat.Tag == tag {
			// Check if this feature index has a substitution
			var lookups []uint16
			if variationsIndex != VariationsNotFoundIndex && fv != nil {
				lookups = fv.GetSubstituteLookups(variationsIndex, uint16(i))
			}
			// Use original lookups if no substitution
			if lookups == nil {
				lookups = feat.Lookups
			}
			for _, lookupIdx := range lookups {
				lookupSet[lookupIdx] = true
			}
		}
	}

	if len(lookupSet) == 0 {
		return nil
	}

	// Convert to sorted slice
	lookups := make([]uint16, 0, len(lookupSet))
	for idx := range lookupSet {
		lookups = append(lookups, idx)
	}
	// Sort to ensure consistent application order
	for i := 0; i < len(lookups)-1; i++ {
		for j := i + 1; j < len(lookups); j++ {
			if lookups[j] < lookups[i] {
				lookups[i], lookups[j] = lookups[j], lookups[i]
			}
		}
	}
	return lookups
}

// FindFeatureByIndices returns lookup indices for a feature tag, considering only the specified feature indices.
// This is the correct way to query features when respecting Script/Language systems.
func (f *FeatureList) FindFeatureByIndices(tag Tag, featureIndices []uint16) []uint16 {
	lookupSet := make(map[uint16]bool)

	for _, idx := range featureIndices {
		if int(idx) >= f.count {
			continue
		}
		feat, err := f.GetFeature(int(idx))
		if err != nil {
			continue
		}
		if feat.Tag == tag {
			for _, lookupIdx := range feat.Lookups {
				lookupSet[lookupIdx] = true
			}
		}
	}

	if len(lookupSet) == 0 {
		return nil
	}

	lookups := make([]uint16, 0, len(lookupSet))
	for idx := range lookupSet {
		lookups = append(lookups, idx)
	}
	sort.Slice(lookups, func(i, j int) bool { return lookups[i] < lookups[j] })
	return lookups
}

// FindFeatureByIndicesWithVariations returns lookup indices for a feature tag,
// considering FeatureVariations substitutions.
// HarfBuzz: hb_ot_map_t::collect_lookups() with feature_substitutes_map
func (f *FeatureList) FindFeatureByIndicesWithVariations(tag Tag, featureIndices []uint16, fv *FeatureVariations, variationsIndex uint32) []uint16 {
	lookupSet := make(map[uint16]bool)

	for _, idx := range featureIndices {
		if int(idx) >= f.count {
			continue
		}
		feat, err := f.GetFeature(int(idx))
		if err != nil {
			continue
		}
		if feat.Tag == tag {
			// Check if this feature index has a substitution
			var lookups []uint16
			if variationsIndex != VariationsNotFoundIndex && fv != nil {
				lookups = fv.GetSubstituteLookups(variationsIndex, idx)
			}
			// Use original lookups if no substitution
			if lookups == nil {
				lookups = feat.Lookups
			}
			for _, lookupIdx := range lookups {
				lookupSet[lookupIdx] = true
			}
		}
	}

	if len(lookupSet) == 0 {
		return nil
	}

	lookups := make([]uint16, 0, len(lookupSet))
	for idx := range lookupSet {
		lookups = append(lookups, idx)
	}
	sort.Slice(lookups, func(i, j int) bool { return lookups[i] < lookups[j] })
	return lookups
}

// --- Script/Language System ---

// ScriptList represents the ScriptList table.
type ScriptList struct {
	data   []byte
	offset int
	count  int
}

// ParseScriptList parses the ScriptList from a GSUB table.
func (g *GSUB) ParseScriptList() (*ScriptList, error) {
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

// FindChosenScriptTag returns the actual script tag found in the font's GSUB table.
// This is needed for Indic old-spec vs new-spec detection.
// HarfBuzz equivalent: plan->map.chosen_script[0] in hb-ot-shaper-indic.cc:324
// Returns 0 if no matching script is found.
func (g *GSUB) FindChosenScriptTag(scriptTag Tag) Tag {
	sl, err := g.ParseScriptList()
	if err != nil {
		return 0
	}
	return sl.FindChosenScriptTag(scriptTag)
}

// FindBestLanguage finds the first language tag from a list of candidates that the font
// actually supports in its GSUB table. Returns the matching Tag, or the first candidate if none match.
func (g *GSUB) FindBestLanguage(scriptTag Tag, languageTags []Tag) Tag {
	sl, err := g.ParseScriptList()
	if err != nil || sl == nil {
		if len(languageTags) > 0 {
			return languageTags[0]
		}
		return 0
	}
	return sl.FindBestLanguage(scriptTag, languageTags)
}

// LangSys represents a Language System table with its feature indices.
type LangSys struct {
	RequiredFeature int      // -1 if none
	FeatureIndices  []uint16 // indices into FeatureList
}

// GetScript returns the LangSys for a script tag (using default language).
// Returns nil if the script is not found.
func (sl *ScriptList) GetScript(scriptTag Tag) *LangSys {
	for i := 0; i < sl.count; i++ {
		recOff := sl.offset + 2 + i*6
		tag := Tag(binary.BigEndian.Uint32(sl.data[recOff:]))
		if tag == scriptTag {
			scriptOff := int(binary.BigEndian.Uint16(sl.data[recOff+4:]))
			return sl.parseScript(sl.offset + scriptOff)
		}
	}
	return nil
}

// GetLangSys returns the LangSys for a specific script and language combination.
// HarfBuzz equivalent: hb_ot_layout_collect_features_map() in hb-ot-map.cc:244-248
//   const OT::LangSys &l = g.get_script (script_index).get_lang_sys (language_index);
//
// If languageTag is 0 or the language is not found, returns the default LangSys.
// Returns nil if the script is not found.
func (sl *ScriptList) GetLangSys(scriptTag Tag, languageTag Tag) *LangSys {
	// Convert scriptTag to OpenType format (lowercase first char)
	// HarfBuzz: hb_ot_old_tag_from_script() in hb-ot-tag.cc:58-59 does:
	//   return ((hb_tag_t) script) | 0x20000000u;
	// ISO 15924 uses 'Phag', OpenType uses 'phag'
	otScriptTag := scriptTag | 0x20000000 // Force first char lowercase

	// HarfBuzz prefers the new script tags (dev2, bng2, knd3, etc.) over the old ones (deva, beng, knda, etc.)
	// These are the "new" OpenType script tags for Indic scripts used with USE (Universal Shaping Engine)
	// HarfBuzz: hb_ot_tags_from_script_and_language() in hb-ot-tag.cc
	newScriptTags := getNewScriptTags(otScriptTag)

	// Try new script tags first, but only for Indic scripts where this matters.
	// For other scripts, prefer the original tag to avoid regressions.
	// HarfBuzz: hb_ot_layout_table_select_script() in hb-ot-layout.cc:572-580
	tagsToTry := make([]Tag, 0, len(newScriptTags)+2)
	tagsToTry = append(tagsToTry, newScriptTags...)
	tagsToTry = append(tagsToTry, otScriptTag, scriptTag)

	// Find the script
	for _, tryTag := range tagsToTry {
		for i := 0; i < sl.count; i++ {
			recOff := sl.offset + 2 + i*6
			tag := Tag(binary.BigEndian.Uint32(sl.data[recOff:]))
			if tag == tryTag {
				scriptOff := sl.offset + int(binary.BigEndian.Uint16(sl.data[recOff+4:]))
				return sl.parseScriptWithLanguage(scriptOff, languageTag)
			}
		}
	}
	return nil
}

// FindChosenScriptTag returns the actual script tag found in the font.
// This is needed for Indic old-spec vs new-spec detection.
// HarfBuzz equivalent: plan->map.chosen_script[0] in hb-ot-shaper-indic.cc:324
// Returns 0 if no matching script is found.
func (sl *ScriptList) FindChosenScriptTag(scriptTag Tag) Tag {
	// Convert scriptTag to OpenType format (lowercase first char)
	otScriptTag := scriptTag | 0x20000000

	// Get new script tags if available (v3 and v2 versions)
	newScriptTags := getNewScriptTags(otScriptTag)

	// Build list of tags to try: new tags first (v3, v2), then old tags
	tagsToTry := make([]Tag, 0, len(newScriptTags)+2)
	tagsToTry = append(tagsToTry, newScriptTags...)
	tagsToTry = append(tagsToTry, otScriptTag, scriptTag)

	// Find which tag actually exists in the font
	for _, tryTag := range tagsToTry {
		for i := 0; i < sl.count; i++ {
			recOff := sl.offset + 2 + i*6
			tag := Tag(binary.BigEndian.Uint32(sl.data[recOff:]))
			if tag == tryTag {
				return tag // Return the tag that was actually found
			}
		}
	}
	return 0
}

// getNewScriptTags returns all "new" OpenType script tags for Indic scripts.
// HarfBuzz: hb_ot_tags_from_script_and_language() in hb-ot-tag.cc
// The new tags (dev2, bng2, knd3, etc.) are used for the Universal Shaping Engine (USE).
// Note: Some fonts use v3 tags (like knd3) instead of v2 tags.
func getNewScriptTags(oldTag Tag) []Tag {
	switch oldTag {
	case MakeTag('d', 'e', 'v', 'a'):
		return []Tag{MakeTag('d', 'e', 'v', '3'), MakeTag('d', 'e', 'v', '2')} // Devanagari
	case MakeTag('b', 'e', 'n', 'g'):
		return []Tag{MakeTag('b', 'n', 'g', '3'), MakeTag('b', 'n', 'g', '2')} // Bengali
	case MakeTag('g', 'u', 'r', 'u'):
		return []Tag{MakeTag('g', 'u', 'r', '3'), MakeTag('g', 'u', 'r', '2')} // Gurmukhi
	case MakeTag('g', 'u', 'j', 'r'):
		return []Tag{MakeTag('g', 'j', 'r', '3'), MakeTag('g', 'j', 'r', '2')} // Gujarati
	case MakeTag('o', 'r', 'y', 'a'):
		return []Tag{MakeTag('o', 'r', 'y', '3'), MakeTag('o', 'r', 'y', '2')} // Oriya
	case MakeTag('t', 'a', 'm', 'l'):
		return []Tag{MakeTag('t', 'm', 'l', '3'), MakeTag('t', 'm', 'l', '2')} // Tamil
	case MakeTag('t', 'e', 'l', 'u'):
		return []Tag{MakeTag('t', 'e', 'l', '3'), MakeTag('t', 'e', 'l', '2')} // Telugu
	case MakeTag('k', 'n', 'd', 'a'):
		return []Tag{MakeTag('k', 'n', 'd', '3'), MakeTag('k', 'n', 'd', '2')} // Kannada
	case MakeTag('m', 'l', 'y', 'm'):
		return []Tag{MakeTag('m', 'l', 'm', '3'), MakeTag('m', 'l', 'm', '2')} // Malayalam
	case MakeTag('m', 'y', 'a', 'n'):
		return []Tag{MakeTag('m', 'y', 'm', '3'), MakeTag('m', 'y', 'm', '2')} // Myanmar
	default:
		return nil
	}
}

// FindBestLanguage finds the first language tag from a list of candidates that the font's
// script table actually supports. Returns the matching Tag, or 0 if none match.
// This is used to resolve BCP47 language tags that map to multiple OT tags (e.g., "zh-mo" → [ZHTM, ZHH]).
func (sl *ScriptList) FindBestLanguage(scriptTag Tag, languageTags []Tag) Tag {
	if len(languageTags) == 0 {
		return 0
	}

	// Convert scriptTag to OpenType format (lowercase first char)
	otScriptTag := scriptTag | 0x20000000
	newScriptTags := getNewScriptTags(otScriptTag)

	tagsToTry := make([]Tag, 0, len(newScriptTags)+2)
	tagsToTry = append(tagsToTry, newScriptTags...)
	tagsToTry = append(tagsToTry, otScriptTag, scriptTag)

	// Find the script record
	for _, tryTag := range tagsToTry {
		for i := 0; i < sl.count; i++ {
			recOff := sl.offset + 2 + i*6
			tag := Tag(binary.BigEndian.Uint32(sl.data[recOff:]))
			if tag == tryTag {
				scriptOff := sl.offset + int(binary.BigEndian.Uint16(sl.data[recOff+4:]))
				// Now check which language tag exists in this script
				if scriptOff+4 > len(sl.data) {
					continue
				}
				langSysCount := int(binary.BigEndian.Uint16(sl.data[scriptOff+2:]))
				for _, langTag := range languageTags {
					for j := 0; j < langSysCount; j++ {
						lrOff := scriptOff + 4 + j*6
						if lrOff+6 > len(sl.data) {
							break
						}
						lt := Tag(binary.BigEndian.Uint32(sl.data[lrOff:]))
						if lt == langTag {
							return langTag
						}
					}
				}
				// No language candidate found in this script, return first candidate
				// (will fall back to default LangSys in GetLangSys)
				return languageTags[0]
			}
		}
	}
	return languageTags[0]
}

// parseScriptWithLanguage parses a Script table and returns the LangSys for the specified language.
// If languageTag is 0 or not found, returns the default LangSys.
func (sl *ScriptList) parseScriptWithLanguage(off int, languageTag Tag) *LangSys {
	if off+4 > len(sl.data) {
		return nil
	}

	defaultLangSysOff := int(binary.BigEndian.Uint16(sl.data[off:]))
	langSysCount := int(binary.BigEndian.Uint16(sl.data[off+2:]))

	// If no specific language requested, or no language systems available, use default
	if languageTag == 0 || langSysCount == 0 {
		if defaultLangSysOff == 0 {
			return nil
		}
		return sl.parseLangSys(off + defaultLangSysOff)
	}

	// Search for the specific language
	for i := 0; i < langSysCount; i++ {
		recOff := off + 4 + i*6
		if recOff+6 > len(sl.data) {
			break
		}
		tag := Tag(binary.BigEndian.Uint32(sl.data[recOff:]))
		if tag == languageTag {
			langSysOff := int(binary.BigEndian.Uint16(sl.data[recOff+4:]))
			return sl.parseLangSys(off + langSysOff)
		}
	}

	// Language not found, fall back to default
	if defaultLangSysOff == 0 {
		return nil
	}
	return sl.parseLangSys(off + defaultLangSysOff)
}

// GetDefaultScript returns the default LangSys using HarfBuzz fallback order.
// HarfBuzz equivalent: hb_ot_layout_table_select_script() in hb-ot-layout.cc:561-608
// Tries: DFLT, dflt, latn in that order.
func (sl *ScriptList) GetDefaultScript() *LangSys {
	// Try DFLT first
	// HarfBuzz: hb-ot-layout.cc:582-587
	if langSys := sl.GetScript(MakeTag('D', 'F', 'L', 'T')); langSys != nil {
		return langSys
	}

	// Try dflt (MS site had typos and many fonts use it)
	// HarfBuzz: hb-ot-layout.cc:589-594
	if langSys := sl.GetScript(MakeTag('d', 'f', 'l', 't')); langSys != nil {
		return langSys
	}

	// Try latn (some old fonts put features there for non-Latin scripts)
	// HarfBuzz: hb-ot-layout.cc:596-602
	if langSys := sl.GetScript(MakeTag('l', 'a', 't', 'n')); langSys != nil {
		return langSys
	}

	return nil
}

// parseScript parses a Script table and returns its default LangSys.
func (sl *ScriptList) parseScript(off int) *LangSys {
	if off+4 > len(sl.data) {
		return nil
	}

	defaultLangSysOff := int(binary.BigEndian.Uint16(sl.data[off:]))
	// langSysCount := int(binary.BigEndian.Uint16(sl.data[off+2:]))

	if defaultLangSysOff == 0 {
		// No default LangSys - unusual but possible
		return nil
	}

	return sl.parseLangSys(off + defaultLangSysOff)
}

// parseLangSys parses a LangSys table.
func (sl *ScriptList) parseLangSys(off int) *LangSys {
	if off+6 > len(sl.data) {
		return nil
	}

	// lookupOrder := binary.BigEndian.Uint16(sl.data[off:]) // reserved, always 0
	reqFeatureIndex := int(binary.BigEndian.Uint16(sl.data[off+2:]))
	featureIndexCount := int(binary.BigEndian.Uint16(sl.data[off+4:]))

	if off+6+featureIndexCount*2 > len(sl.data) {
		return nil
	}

	ls := &LangSys{
		RequiredFeature: reqFeatureIndex,
		FeatureIndices:  make([]uint16, featureIndexCount),
	}

	// 0xFFFF means no required feature
	if reqFeatureIndex == 0xFFFF {
		ls.RequiredFeature = -1
	}

	for i := 0; i < featureIndexCount; i++ {
		ls.FeatureIndices[i] = binary.BigEndian.Uint16(sl.data[off+6+i*2:])
	}

	return ls
}

// --- Apply lookup ---

// ApplyLookupWithMask applies a single lookup with mask-based filtering.
// This is the HarfBuzz-style approach where each glyph has a mask, and
// the lookup is only applied to glyphs where (mask & featureMask) != 0.
//
// HarfBuzz equivalent: The mask check in match_glyph() / may_match() in hb-ot-layout-gsubgpos.hh
//
// Parameters:
//   - lookupIndex: The GSUB lookup index
//   - glyphs: The current glyph sequence
//   - masks: Mask for each glyph (must have same length as glyphs)
//   - featureMask: The mask for the current feature
//   - gdef: GDEF table for glyph classification (may be nil)
//   - font: Font object for scaling information (may be nil)
//
// Returns the modified glyph sequence and updated masks.
func (g *GSUB) ApplyLookupWithMask(lookupIndex int, glyphs []GlyphID, masks []uint32, featureMask uint32, gdef *GDEF, font *Font) ([]GlyphID, []uint32) {
	lookup := g.GetLookup(lookupIndex)
	if lookup == nil {
		return glyphs, masks
	}

	// Determine mark filtering set index
	markFilteringSet := -1
	if lookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
		markFilteringSet = int(lookup.MarkFilter)
	}

	// Create temporary buffer from glyphs
	buf := &Buffer{
		Info: make([]GlyphInfo, len(glyphs)),
	}
	for i, gid := range glyphs {
		buf.Info[i] = GlyphInfo{
			GlyphID: gid,
			Cluster: i,
		}
		if masks != nil && i < len(masks) {
			buf.Info[i].Mask = masks[i]
		}
	}

	ctx := &OTApplyContext{
		Buffer:           buf,
		LookupFlag:       lookup.Flag,
		GDEF:             gdef,
		MarkFilteringSet: markFilteringSet,
		FeatureMask:      featureMask,
		Font:             font,
	}
	// Buffer.Idx starts at 0 by default

	// Initialize output buffer (HarfBuzz pattern: clearOutput before GSUB)
	// This is CRITICAL for Context/ChainContext lookups to work correctly!
	buf.clearOutput()

	// Type 8 (Reverse Chain Single Substitution) must be applied in reverse order
	// Note: Reverse substitution doesn't use output buffer pattern
	if lookup.Type == GSUBTypeReverseChainSingle {
		for ctx.Buffer.Idx = len(ctx.Buffer.Info) - 1; ctx.Buffer.Idx >= 0; ctx.Buffer.Idx-- {
			if ctx.ShouldSkipGlyph(ctx.Buffer.Idx) {
				continue
			}
			for _, subtable := range lookup.subtables {
				if subtable.Apply(ctx) > 0 {
					break
				}
			}
		}
		return extractGlyphsAndMasks(buf)
	}

	// Normal forward iteration for all other lookup types
	// HarfBuzz: When a glyph is not matched or skipped, next_glyph() is called
	// to copy it to output and advance idx. If we only do Idx++, the glyph
	// is lost when sync() is called.
	for ctx.Buffer.Idx < len(ctx.Buffer.Info) {
		if ctx.ShouldSkipGlyph(ctx.Buffer.Idx) {
			// HarfBuzz: skipped glyphs are passed through via next_glyph()
			buf.nextGlyph()
			continue
		}

		applied := false
		for _, subtable := range lookup.subtables {
			if subtable.Apply(ctx) > 0 {
				applied = true
				break
			}
		}
		if !applied {
			// HarfBuzz: non-matching glyphs are passed through via next_glyph()
			buf.nextGlyph()
		}
	}

	// Sync output buffer back to main buffer (HarfBuzz pattern)
	buf.sync()

	return extractGlyphsAndMasks(buf)
}

// extractGlyphsAndMasks extracts GlyphIDs and Masks from a Buffer.
func extractGlyphsAndMasks(buf *Buffer) ([]GlyphID, []uint32) {
	glyphs := make([]GlyphID, len(buf.Info))
	masks := make([]uint32, len(buf.Info))
	for i, info := range buf.Info {
		glyphs[i] = info.GlyphID
		masks[i] = info.Mask
	}
	return glyphs, masks
}

// ApplyLookupWithGDEF applies a single lookup with GDEF-based glyph filtering.
func (g *GSUB) ApplyLookupWithGDEF(lookupIndex int, glyphs []GlyphID, gdef *GDEF, font *Font) []GlyphID {
	lookup := g.GetLookup(lookupIndex)
	if lookup == nil {
		return glyphs
	}

	// Determine mark filtering set index
	markFilteringSet := -1
	if lookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
		markFilteringSet = int(lookup.MarkFilter)
	}

	// Create temporary buffer from glyphs
	// HarfBuzz: All glyphs start with global_mask set
	buf := &Buffer{
		Info: make([]GlyphInfo, len(glyphs)),
	}
	for i, gid := range glyphs {
		buf.Info[i] = GlyphInfo{
			GlyphID: gid,
			Cluster: i,
			Mask:    MaskGlobal, // HarfBuzz: glyphs have global_mask
		}
	}

	ctx := &OTApplyContext{
		Buffer:           buf,
		LookupFlag:       lookup.Flag,
		GDEF:             gdef,
		MarkFilteringSet: markFilteringSet,
		FeatureMask:      MaskGlobal, // HarfBuzz: lookup_mask is never 0
		Font:             font,
	}
	// Buffer.Idx starts at 0 by default

	// Initialize output buffer (HarfBuzz pattern: clearOutput before GSUB)
	buf.clearOutput()

	// Type 8 (Reverse Chain Single Substitution) must be applied in reverse order
	if lookup.Type == GSUBTypeReverseChainSingle {
		for ctx.Buffer.Idx = len(ctx.Buffer.Info) - 1; ctx.Buffer.Idx >= 0; ctx.Buffer.Idx-- {
			if ctx.ShouldSkipGlyph(ctx.Buffer.Idx) {
				buf.nextGlyph()
				continue
			}
			for _, subtable := range lookup.subtables {
				if subtable.Apply(ctx) > 0 {
					break
				}
			}
		}
		buf.sync()
		return extractGlyphs(buf)
	}

	// Normal forward iteration for all other lookup types
	for ctx.Buffer.Idx < len(ctx.Buffer.Info) {
		if ctx.ShouldSkipGlyph(ctx.Buffer.Idx) {
			buf.nextGlyph()
			continue
		}

		applied := false
		for _, subtable := range lookup.subtables {
			if subtable.Apply(ctx) > 0 {
				applied = true
				break
			}
		}
		if !applied {
			buf.nextGlyph()
		}
	}

	buf.sync()
	return extractGlyphs(buf)
}

// extractGlyphs extracts GlyphIDs from a Buffer.
func extractGlyphs(buf *Buffer) []GlyphID {
	glyphs := make([]GlyphID, len(buf.Info))
	for i, info := range buf.Info {
		glyphs[i] = info.GlyphID
	}
	return glyphs
}

// ApplyLookupWithCodepoints applies a single lookup with GDEF-based filtering
// and codepoint tracking for default ignorable handling.
func (g *GSUB) ApplyLookupWithCodepoints(lookupIndex int, glyphs []GlyphID, codepoints []Codepoint, gdef *GDEF, font *Font) ([]GlyphID, []Codepoint) {
	lookup := g.GetLookup(lookupIndex)
	if lookup == nil {
		return glyphs, codepoints
	}

	// Determine mark filtering set index
	markFilteringSet := -1
	if lookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
		markFilteringSet = int(lookup.MarkFilter)
	}

	// Create temporary buffer from glyphs and codepoints
	// HarfBuzz: All glyphs start with global_mask set
	buf := &Buffer{
		Info: make([]GlyphInfo, len(glyphs)),
	}
	for i, gid := range glyphs {
		buf.Info[i] = GlyphInfo{
			GlyphID: gid,
			Cluster: i,
			Mask:    MaskGlobal, // HarfBuzz: glyphs have global_mask
		}
		if codepoints != nil && i < len(codepoints) {
			buf.Info[i].Codepoint = codepoints[i]
		}
	}

	ctx := &OTApplyContext{
		Buffer:           buf,
		LookupFlag:       lookup.Flag,
		GDEF:             gdef,
		MarkFilteringSet: markFilteringSet,
		FeatureMask:      MaskGlobal, // HarfBuzz: lookup_mask is never 0
		Font:             font,
	}
	// Buffer.Idx starts at 0 by default

	// CRITICAL: Initialize output buffer before applying lookups
	// HarfBuzz equivalent: clear_output() in hb-buffer.hh:232
	// Context-lookups check haveOutput and return immediately if false!
	buf.clearOutput()

	// Type 8 (Reverse Chain Single Substitution) must be applied in reverse order
	// Note: Reverse substitution doesn't use output buffer pattern
	if lookup.Type == GSUBTypeReverseChainSingle {
		for ctx.Buffer.Idx = len(ctx.Buffer.Info) - 1; ctx.Buffer.Idx >= 0; ctx.Buffer.Idx-- {
			if ctx.ShouldSkipGlyph(ctx.Buffer.Idx) {
				continue
			}
			for _, subtable := range lookup.subtables {
				if subtable.Apply(ctx) > 0 {
					break
				}
			}
		}
		// Sync output buffer back to input buffer
		// HarfBuzz equivalent: sync() in hb-buffer.cc:416
		buf.sync()
		return extractGlyphsAndCodepoints(buf)
	}

	// Normal forward iteration for all other lookup types
	// HarfBuzz: When a glyph is not matched or skipped, next_glyph() is called
	// to copy it to output and advance idx.
	for ctx.Buffer.Idx < len(ctx.Buffer.Info) {
		if ctx.ShouldSkipGlyph(ctx.Buffer.Idx) {
			buf.nextGlyph()
			continue
		}

		applied := false
		for _, subtable := range lookup.subtables {
			if subtable.Apply(ctx) > 0 {
				applied = true
				break
			}
		}
		if !applied {
			buf.nextGlyph()
		}
	}

	// Sync output buffer back to input buffer
	// HarfBuzz equivalent: sync() in hb-buffer.cc:416
	buf.sync()

	return extractGlyphsAndCodepoints(buf)
}

// extractGlyphsAndCodepoints extracts GlyphIDs and Codepoints from a Buffer.
func extractGlyphsAndCodepoints(buf *Buffer) ([]GlyphID, []Codepoint) {
	glyphs := make([]GlyphID, len(buf.Info))
	codepoints := make([]Codepoint, len(buf.Info))
	for i, info := range buf.Info {
		glyphs[i] = info.GlyphID
		codepoints[i] = info.Codepoint
	}
	return glyphs, codepoints
}

// ApplyLookupToBuffer applies a single GSUB lookup directly to a Buffer.
// HarfBuzz equivalent: hb_ot_layout_substitute_lookup() in hb-ot-layout.cc:2093-2098
// Uses MaskGlobal so lookup applies to all glyphs (which have MaskGlobal by default).
// This preserves cluster information during substitution.
func (g *GSUB) ApplyLookupToBuffer(lookupIndex int, buf *Buffer, gdef *GDEF, font *Font) {
	g.ApplyLookupToBufferWithMask(lookupIndex, buf, gdef, MaskGlobal, font)
}

// ApplyLookupToBufferWithMask applies a single GSUB lookup directly to a Buffer with mask-based filtering.
//
// HarfBuzz equivalent: Combines multiple HarfBuzz functions:
//   - hb_ot_layout_substitute_lookup() in hb-ot-layout.cc:2093-2098
//   - apply_string() / apply_forward() in hb-ot-layout.cc:1917-1947
//   - Mask filtering: (info[j].mask & c->lookup_mask) in hb-ot-layout.cc:1930
//
// The featureMask parameter corresponds to lookup_mask in hb_ot_apply_context_t.
// A glyph is skipped if (glyph.Mask & featureMask) == 0.
// Use MaskGlobal to apply to all glyphs (which have MaskGlobal set by default).
// This preserves cluster information during substitution (unlike array-based methods).
func (g *GSUB) ApplyLookupToBufferWithMask(lookupIndex int, buf *Buffer, gdef *GDEF, featureMask uint32, font *Font) {
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
		TableType:        TableGSUB,
		Font:             font,
	}

	// Type 8 (Reverse Chain Single Substitution) must be applied in reverse order
	// HarfBuzz: Reverse lookups are always in-place (don't use output buffer)
	if lookup.Type == GSUBTypeReverseChainSingle {
		for buf.Idx = len(buf.Info) - 1; buf.Idx >= 0; buf.Idx-- {
			if ctx.ShouldSkipGlyph(buf.Idx) {
				continue
			}
			for _, subtable := range lookup.subtables {
				if subtable.Apply(ctx) > 0 {
					break
				}
			}
		}
		return
	}

	// Normal forward iteration for all other lookup types
	// HarfBuzz equivalent: apply_string() with clear_output/sync pattern
	// (hb-ot-layout.cc:1986-1996)
	//
	// GSUB uses output buffer to properly propagate cluster information:
	// - clear_output() before substitution
	// - outputGlyph() / nextGlyph() during substitution (copies all properties)
	// - sync() after substitution
	buf.clearOutput()

	buf.Idx = 0
	for buf.Idx < len(buf.Info) {
		// Skip glyphs that should be ignored based on LookupFlag, GDEF, and FeatureMask
		if ctx.ShouldSkipGlyph(buf.Idx) {
			buf.nextGlyph()
			continue
		}

		applied := false
		for _, subtable := range lookup.subtables {
			if subtable.Apply(ctx) > 0 {
				applied = true
				break
			}
		}
		if !applied {
			buf.nextGlyph()
		}
	}

	buf.sync()
}

// ApplyFeatureToBuffer applies all lookups for a feature directly to a Buffer.
// HarfBuzz equivalent: hb_ot_layout_substitute_lookup() loop
// This preserves cluster information during substitution.
func (g *GSUB) ApplyFeatureToBuffer(tag Tag, buf *Buffer, gdef *GDEF, font *Font) {
	g.ApplyFeatureToBufferWithMaskAndVariations(tag, buf, gdef, MaskGlobal, font, VariationsNotFoundIndex)
}

// ApplyFeatureToBufferWithMask applies all lookups for a feature directly to a Buffer with mask filtering.
// This preserves cluster information during substitution.
// CRITICAL: Uses script/language-specific feature selection (like HarfBuzz)
func (g *GSUB) ApplyFeatureToBufferWithMask(tag Tag, buf *Buffer, gdef *GDEF, featureMask uint32, font *Font) {
	g.ApplyFeatureToBufferWithMaskAndVariations(tag, buf, gdef, featureMask, font, VariationsNotFoundIndex)
}

// ApplyFeatureToBufferLangSysOnly applies a feature only if it's found in the current
// script/language LangSys. Does NOT fall back to global feature search.
// This is used for 'locl' which should only apply when the font has it for the specific language.
// Tries the buffer's script first, then DFLT/dflt as fallback scripts.
func (g *GSUB) ApplyFeatureToBufferLangSysOnly(tag Tag, buf *Buffer, gdef *GDEF, featureMask uint32, font *Font, variationsIndex uint32) {
	if buf.Language == 0 {
		return // No language set - locl not applicable
	}

	featureList, err := g.ParseFeatureList()
	if err != nil {
		return
	}

	scriptList, err := g.ParseScriptList()
	if err != nil || scriptList == nil {
		return
	}

	// Try buffer's script first, then DFLT/dflt as fallback
	// HarfBuzz: hb_ot_layout_table_select_script() tries DFLT/dflt when script not found
	langSys := scriptList.GetLangSys(buf.Script, buf.Language)
	if langSys == nil {
		// Try DFLT script as fallback
		langSys = scriptList.GetLangSys(MakeTag('D', 'F', 'L', 'T'), buf.Language)
	}
	if langSys == nil {
		// Try dflt script as fallback
		langSys = scriptList.GetLangSys(MakeTag('d', 'f', 'l', 't'), buf.Language)
	}
	if langSys == nil {
		// Also try with language candidates if available
		if len(buf.LanguageCandidates) > 1 {
			for _, langTag := range buf.LanguageCandidates[1:] {
				langSys = scriptList.GetLangSys(buf.Script, langTag)
				if langSys != nil {
					break
				}
				langSys = scriptList.GetLangSys(MakeTag('D', 'F', 'L', 'T'), langTag)
				if langSys != nil {
					break
				}
			}
		}
	}
	if langSys == nil {
		return // No LangSys found - don't apply
	}

	lookups := featureList.FindFeatureByIndicesWithVariations(tag, langSys.FeatureIndices, g.featureVariations, variationsIndex)
	if lookups == nil {
		return
	}

	sorted := make([]int, len(lookups))
	for i, l := range lookups {
		sorted[i] = int(l)
	}
	sort.Ints(sorted)

	for _, lookupIdx := range sorted {
		g.ApplyLookupToBufferWithMask(lookupIdx, buf, gdef, featureMask, font)
	}
}

// ApplyFeatureToBufferWithMaskAndVariations applies all lookups for a feature directly to a Buffer
// with mask filtering and FeatureVariations support.
// variationsIndex should be obtained from FindVariationsIndex() or VariationsNotFoundIndex if not applicable.
// HarfBuzz: hb_ot_layout_substitute_lookup() with feature_substitutes_map
func (g *GSUB) ApplyFeatureToBufferWithMaskAndVariations(tag Tag, buf *Buffer, gdef *GDEF, featureMask uint32, font *Font, variationsIndex uint32) {
	featureList, err := g.ParseFeatureList()
	if err != nil {
		return
	}

	// Get script/language-specific feature indices
	// HarfBuzz: hb_ot_layout_collect_features_map() in hb-ot-map.cc:244-248
	//   const OT::LangSys &l = g.get_script (script_index).get_lang_sys (language_index);
	var lookups []uint16
	scriptList, err := g.ParseScriptList()
	if err == nil && scriptList != nil {
		// CRITICAL FIX: Use GetLangSys() instead of GetScript() to respect both Script AND Language
		// This ensures we only use features from the correct Script/Language combination
		langSys := scriptList.GetLangSys(buf.Script, buf.Language)
		if langSys != nil {
			// Use script/language-specific search with FeatureVariations support
			lookups = featureList.FindFeatureByIndicesWithVariations(tag, langSys.FeatureIndices, g.featureVariations, variationsIndex)
		} else {
			// Try default script as fallback
			langSys = scriptList.GetDefaultScript()
			if langSys != nil {
				lookups = featureList.FindFeatureByIndicesWithVariations(tag, langSys.FeatureIndices, g.featureVariations, variationsIndex)
			}
		}
	}

	// Fallback to global search if no script/language found
	// Also apply FeatureVariations substitution for global features
	if lookups == nil {
		lookups = featureList.FindFeatureWithVariations(tag, g.featureVariations, variationsIndex)
	}

	if lookups == nil {
		return
	}

	// Sort lookups by index (they should be applied in order)
	sorted := make([]int, len(lookups))
	for i, l := range lookups {
		sorted[i] = int(l)
	}
	sort.Ints(sorted)

	for _, lookupIdx := range sorted {
		g.ApplyLookupToBufferWithMask(lookupIdx, buf, gdef, featureMask, font)
	}
}

// ApplyFeatureToBufferRangeWithMask applies all lookups for a feature to a range of the Buffer.
// This is used for per-syllable feature application (F_PER_SYLLABLE in HarfBuzz).
// Only glyphs in the range [start, end) are considered for substitution.
// HarfBuzz equivalent: per_syllable flag in hb_ot_apply_context_t
func (g *GSUB) ApplyFeatureToBufferRangeWithMask(tag Tag, buf *Buffer, gdef *GDEF, featureMask uint32, font *Font, start, end int) {
	g.ApplyFeatureToBufferRangeWithOpts(tag, buf, gdef, featureMask, font, start, end, true, true)
}

// ApplyFeatureToBufferRangeWithOpts is like ApplyFeatureToBufferRangeWithMask but allows
// setting AutoZWNJ and AutoZWJ flags. These control whether ZWNJ/ZWJ are automatically
// skipped during matching (HarfBuzz: auto_zwnj, auto_zwj from F_MANUAL_ZWNJ/F_MANUAL_ZWJ).
func (g *GSUB) ApplyFeatureToBufferRangeWithOpts(tag Tag, buf *Buffer, gdef *GDEF, featureMask uint32, font *Font, start, end int, autoZWNJ, autoZWJ bool) {
	if start >= end || start < 0 || end > len(buf.Info) {
		return
	}

	featureList, err := g.ParseFeatureList()
	if err != nil {
		return
	}

	// Get script/language-specific feature indices
	var lookups []uint16
	scriptList, err := g.ParseScriptList()
	if err == nil && scriptList != nil {
		langSys := scriptList.GetLangSys(buf.Script, buf.Language)
		if langSys != nil {
			lookups = featureList.FindFeatureByIndicesWithVariations(tag, langSys.FeatureIndices, g.featureVariations, VariationsNotFoundIndex)
		} else {
			langSys = scriptList.GetDefaultScript()
			if langSys != nil {
				lookups = featureList.FindFeatureByIndicesWithVariations(tag, langSys.FeatureIndices, g.featureVariations, VariationsNotFoundIndex)
			}
		}
	}

	if lookups == nil {
		lookups = featureList.FindFeatureWithVariations(tag, g.featureVariations, VariationsNotFoundIndex)
	}

	if lookups == nil {
		return
	}

	// Sort lookups by index
	sorted := make([]int, len(lookups))
	for i, l := range lookups {
		sorted[i] = int(l)
	}
	sort.Ints(sorted)

	for _, lookupIdx := range sorted {
		g.applyLookupToBufferRangeWithOpts(lookupIdx, buf, gdef, featureMask, font, start, end, autoZWNJ, autoZWJ)
		// Update end if buffer length changed (e.g., ligature or multiple substitution)
		// We need to find where this syllable ends now
		if start < len(buf.Info) {
			syllable := buf.Info[start].Syllable
			end = start
			for end < len(buf.Info) && buf.Info[end].Syllable == syllable {
				end++
			}
		}
	}
}

// ApplyLookupToBufferRangeWithMask applies a single lookup to a range of the Buffer.
// Only positions in [start, end) are considered as starting positions for matches.
// Uses HarfBuzz defaults for auto_zwnj=true, auto_zwj=true.
func (g *GSUB) ApplyLookupToBufferRangeWithMask(lookupIndex int, buf *Buffer, gdef *GDEF, featureMask uint32, font *Font, start, end int) {
	g.applyLookupToBufferRangeWithOpts(lookupIndex, buf, gdef, featureMask, font, start, end, true, true)
}

// applyLookupToBufferRangeWithOpts applies a single lookup to a range of the Buffer
// with explicit auto_zwnj/auto_zwj settings.
// HarfBuzz equivalent: The per-lookup application in hb_ot_map_t::apply(),
// where auto_zwnj and auto_zwj come from the lookup_map_t flags.
func (g *GSUB) applyLookupToBufferRangeWithOpts(lookupIndex int, buf *Buffer, gdef *GDEF, featureMask uint32, font *Font, start, end int, autoZWNJ, autoZWJ bool) {
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
		TableType:        TableGSUB,
		Font:             font,
		AutoZWNJ:         autoZWNJ, // HarfBuzz: lookup.auto_zwnj from F_MANUAL_ZWNJ
		AutoZWJ:          autoZWJ,  // HarfBuzz: lookup.auto_zwj from F_MANUAL_ZWJ
		// Per-syllable range constraints
		RangeStart: start,
		RangeEnd:   end,
		PerSyllable: true,
	}

	// Type 8 (Reverse Chain Single Substitution) must be applied in reverse order
	if lookup.Type == GSUBTypeReverseChainSingle {
		// For per-syllable, only iterate within the range
		for buf.Idx = end - 1; buf.Idx >= start; buf.Idx-- {
			if ctx.ShouldSkipGlyph(buf.Idx) {
				continue
			}
			for _, subtable := range lookup.subtables {
				if subtable.Apply(ctx) > 0 {
					break
				}
			}
		}
		return
	}

	// Normal forward iteration - but only for the specified range
	// We use a modified approach: iterate through all but only apply within range
	buf.clearOutput()

	buf.Idx = 0
	for buf.Idx < len(buf.Info) {
		// Skip glyphs that should be ignored
		if ctx.ShouldSkipGlyph(buf.Idx) {
			buf.nextGlyph()
			continue
		}

		// For per-syllable: only apply substitutions if current position is within range
		// and all context glyphs are within the same syllable
		inRange := buf.Idx >= start && buf.Idx < end

		applied := false
		if inRange {
			for _, subtable := range lookup.subtables {
				if subtable.Apply(ctx) > 0 {
					applied = true
					break
				}
			}
		}
		if !applied {
			buf.nextGlyph()
		}
	}

	buf.sync()
}

// ApplyFeatureWithGDEF applies all lookups for a feature with GDEF-based glyph filtering.
// Note: This unions all features with the matching tag, which may not be correct for fonts
// with multiple features for different scripts/languages.
func (g *GSUB) ApplyFeatureWithGDEF(tag Tag, glyphs []GlyphID, gdef *GDEF, font *Font) []GlyphID {
	result, _ := g.ApplyFeatureWithCodepoints(tag, glyphs, nil, gdef, font)
	return result
}

// ApplyFeatureWithCodepoints applies all lookups for a feature with GDEF-based filtering
// and support for default ignorable handling (like Variation Selectors).
// The codepoints slice should match glyphs in length, or can be nil if not needed.
func (g *GSUB) ApplyFeatureWithCodepoints(tag Tag, glyphs []GlyphID, codepoints []Codepoint, gdef *GDEF, font *Font) ([]GlyphID, []Codepoint) {
	featureList, err := g.ParseFeatureList()
	if err != nil {
		return glyphs, codepoints
	}

	lookups := featureList.FindFeature(tag)
	if lookups == nil {
		return glyphs, codepoints
	}

	// Sort lookups by index (they should be applied in order)
	sorted := make([]int, len(lookups))
	for i, l := range lookups {
		sorted[i] = int(l)
	}
	sort.Ints(sorted)

	for _, lookupIdx := range sorted {
		glyphs, codepoints = g.ApplyLookupWithCodepoints(lookupIdx, glyphs, codepoints, gdef, font)
	}

	return glyphs, codepoints
}

// ApplyFeatureWithMask applies all lookups for a feature with mask-based filtering.
// This is the HarfBuzz-style approach where each glyph has a mask, and
// the lookup is only applied to glyphs where (mask & featureMask) != 0.
//
// HarfBuzz equivalent: The mask check in hb-ot-layout-gsubgpos.hh
//
// Parameters:
//   - tag: The feature tag (e.g., 'isol', 'init', 'medi', 'fina')
//   - glyphs: The current glyph sequence
//   - masks: Mask for each glyph (must have same length as glyphs)
//   - featureMask: The mask for this feature (used to filter which glyphs are affected)
//   - gdef: GDEF table for glyph classification (may be nil)
//
// Returns the modified glyph sequence and updated masks.
func (g *GSUB) ApplyFeatureWithMask(tag Tag, glyphs []GlyphID, masks []uint32, featureMask uint32, gdef *GDEF, font *Font) ([]GlyphID, []uint32) {
	featureList, err := g.ParseFeatureList()
	if err != nil {
		return glyphs, masks
	}

	lookups := featureList.FindFeature(tag)
	if lookups == nil {
		return glyphs, masks
	}

	// Sort lookups by index (they should be applied in order)
	sorted := make([]int, len(lookups))
	for i, l := range lookups {
		sorted[i] = int(l)
	}
	sort.Ints(sorted)

	for _, lookupIdx := range sorted {
		glyphs, masks = g.ApplyLookupWithMask(lookupIdx, glyphs, masks, featureMask, gdef, font)
	}

	return glyphs, masks
}

// Common feature tags
var (
	TagLiga = MakeTag('l', 'i', 'g', 'a') // Standard Ligatures
	TagClig = MakeTag('c', 'l', 'i', 'g') // Contextual Ligatures
	TagDlig = MakeTag('d', 'l', 'i', 'g') // Discretionary Ligatures
	TagHlig = MakeTag('h', 'l', 'i', 'g') // Historical Ligatures
	TagCcmp = MakeTag('c', 'c', 'm', 'p') // Glyph Composition/Decomposition
	TagLocl = MakeTag('l', 'o', 'c', 'l') // Localized Forms
	TagRlig = MakeTag('r', 'l', 'i', 'g') // Required Ligatures
	TagSmcp = MakeTag('s', 'm', 'c', 'p') // Small Capitals
	TagCalt = MakeTag('c', 'a', 'l', 't') // Contextual Alternates
)

// --- LookupRecord ---

// LookupRecord specifies a lookup to apply at a specific position.
type LookupRecord struct {
	SequenceIndex uint16 // Index into current glyph sequence (0-based)
	LookupIndex   uint16 // Lookup to apply
}

// --- ChainContextSubst ---

// ChainContextSubst represents a Chaining Context Substitution subtable (GSUB Type 6).
// It enables substitution based on surrounding context (backtrack, input, lookahead).
type ChainContextSubst struct {
	format uint16
	gsub   *GSUB // Reference to parent GSUB for nested lookup application

	// Format 1: Simple glyph contexts
	coverage      *Coverage
	chainRuleSets [][]ChainRule // Indexed by coverage index

	// Format 2: Class-based contexts
	backtrackClassDef *ClassDef
	inputClassDef     *ClassDef
	lookaheadClassDef *ClassDef
	// chainRuleSets also used for format 2 (indexed by input class)

	// Format 3: Coverage-based contexts
	backtrackCoverages []*Coverage
	inputCoverages     []*Coverage
	lookaheadCoverages []*Coverage
	lookupRecords      []LookupRecord
}

// ChainRule represents a single chaining context rule.
type ChainRule struct {
	Backtrack     []GlyphID      // Backtrack sequence (in reverse order)
	Input         []GlyphID      // Input sequence (starting from second glyph)
	Lookahead     []GlyphID      // Lookahead sequence
	LookupRecords []LookupRecord // Lookups to apply
}

func parseChainContextSubst(data []byte, offset int, gsub *GSUB) (*ChainContextSubst, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])

	switch format {
	case 1:
		return parseChainContextFormat1(data, offset, gsub)
	case 2:
		return parseChainContextFormat2(data, offset, gsub)
	case 3:
		return parseChainContextFormat3(data, offset, gsub)
	default:
		return nil, ErrInvalidFormat
	}
}

// parseChainContextFormat1 parses ChainContextSubstFormat1 (simple glyph context).
func parseChainContextFormat1(data []byte, offset int, gsub *GSUB) (*ChainContextSubst, error) {
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

	ccs := &ChainContextSubst{
		format:        1,
		gsub:          gsub,
		coverage:      coverage,
		chainRuleSets: make([][]ChainRule, chainRuleSetCount),
	}

	for i := 0; i < chainRuleSetCount; i++ {
		ruleSetOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		if ruleSetOff == 0 {
			continue // NULL offset
		}
		rules, err := parseChainRuleSet(data, offset+ruleSetOff)
		if err != nil {
			continue
		}
		ccs.chainRuleSets[i] = rules
	}

	return ccs, nil
}

// parseChainRuleSet parses a ChainRuleSet (array of ChainRules).
func parseChainRuleSet(data []byte, offset int) ([]ChainRule, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	ruleCount := int(binary.BigEndian.Uint16(data[offset:]))
	if offset+2+ruleCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	rules := make([]ChainRule, 0, ruleCount)

	for i := 0; i < ruleCount; i++ {
		ruleOff := int(binary.BigEndian.Uint16(data[offset+2+i*2:]))
		rule, err := parseChainRule(data, offset+ruleOff)
		if err != nil {
			continue
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// parseChainRule parses a single ChainRule.
func parseChainRule(data []byte, offset int) (ChainRule, error) {
	var rule ChainRule
	off := offset

	if off+2 > len(data) {
		return rule, ErrInvalidOffset
	}

	// Backtrack count and array
	backtrackCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+backtrackCount*2 > len(data) {
		return rule, ErrInvalidOffset
	}
	rule.Backtrack = make([]GlyphID, backtrackCount)
	for i := 0; i < backtrackCount; i++ {
		rule.Backtrack[i] = GlyphID(binary.BigEndian.Uint16(data[off+i*2:]))
	}
	off += backtrackCount * 2

	// Input count and array (count includes first glyph)
	if off+2 > len(data) {
		return rule, ErrInvalidOffset
	}
	inputCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	inputLen := inputCount - 1 // First glyph is covered by coverage table
	if inputLen < 0 {
		inputLen = 0
	}
	if off+inputLen*2 > len(data) {
		return rule, ErrInvalidOffset
	}
	rule.Input = make([]GlyphID, inputLen)
	for i := 0; i < inputLen; i++ {
		rule.Input[i] = GlyphID(binary.BigEndian.Uint16(data[off+i*2:]))
	}
	off += inputLen * 2

	// Lookahead count and array
	if off+2 > len(data) {
		return rule, ErrInvalidOffset
	}
	lookaheadCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+lookaheadCount*2 > len(data) {
		return rule, ErrInvalidOffset
	}
	rule.Lookahead = make([]GlyphID, lookaheadCount)
	for i := 0; i < lookaheadCount; i++ {
		rule.Lookahead[i] = GlyphID(binary.BigEndian.Uint16(data[off+i*2:]))
	}
	off += lookaheadCount * 2

	// Lookup records
	if off+2 > len(data) {
		return rule, ErrInvalidOffset
	}
	lookupCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+lookupCount*4 > len(data) {
		return rule, ErrInvalidOffset
	}
	rule.LookupRecords = make([]LookupRecord, lookupCount)
	for i := 0; i < lookupCount; i++ {
		rule.LookupRecords[i].SequenceIndex = binary.BigEndian.Uint16(data[off+i*4:])
		rule.LookupRecords[i].LookupIndex = binary.BigEndian.Uint16(data[off+i*4+2:])
	}

	return rule, nil
}

// parseChainContextFormat2 parses ChainContextSubstFormat2 (class-based context).
func parseChainContextFormat2(data []byte, offset int, gsub *GSUB) (*ChainContextSubst, error) {
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

	backtrackClassDef, err := ParseClassDef(data, offset+backtrackClassDefOff)
	if err != nil {
		return nil, err
	}

	inputClassDef, err := ParseClassDef(data, offset+inputClassDefOff)
	if err != nil {
		return nil, err
	}

	lookaheadClassDef, err := ParseClassDef(data, offset+lookaheadClassDefOff)
	if err != nil {
		return nil, err
	}

	ccs := &ChainContextSubst{
		format:            2,
		gsub:              gsub,
		coverage:          coverage,
		backtrackClassDef: backtrackClassDef,
		inputClassDef:     inputClassDef,
		lookaheadClassDef: lookaheadClassDef,
		chainRuleSets:     make([][]ChainRule, chainRuleSetCount),
	}

	for i := 0; i < chainRuleSetCount; i++ {
		ruleSetOff := int(binary.BigEndian.Uint16(data[offset+12+i*2:]))
		if ruleSetOff == 0 {
			continue // NULL offset
		}
		rules, err := parseChainRuleSet(data, offset+ruleSetOff)
		if err != nil {
			continue
		}
		ccs.chainRuleSets[i] = rules
	}

	return ccs, nil
}

// parseChainContextFormat3 parses ChainContextSubstFormat3 (coverage-based context).
func parseChainContextFormat3(data []byte, offset int, gsub *GSUB) (*ChainContextSubst, error) {
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

	lookupRecords := make([]LookupRecord, lookupCount)
	for i := 0; i < lookupCount; i++ {
		lookupRecords[i].SequenceIndex = binary.BigEndian.Uint16(data[off+i*4:])
		lookupRecords[i].LookupIndex = binary.BigEndian.Uint16(data[off+i*4+2:])
	}

	return &ChainContextSubst{
		format:             3,
		gsub:               gsub,
		backtrackCoverages: backtrackCoverages,
		inputCoverages:     inputCoverages,
		lookaheadCoverages: lookaheadCoverages,
		lookupRecords:      lookupRecords,
	}, nil
}

// Apply applies the chaining context substitution.
func (ccs *ChainContextSubst) Apply(ctx *OTApplyContext) int {
	switch ccs.format {
	case 1:
		return ccs.applyFormat1(ctx)
	case 2:
		return ccs.applyFormat2(ctx)
	case 3:
		return ccs.applyFormat3(ctx)
	default:
		return 0
	}
}

// wouldApply checks if this ChainContextSubst would apply to the given glyph sequence.
// HarfBuzz equivalent: chain_context_would_apply_lookup() in hb-ot-layout-gsubgpos.hh:3126-3141
//
// For zeroContext=true: only matches if there's no backtrack AND no lookahead context.
// This is used by consonant_position_from_face() to determine consonant positions.
func (ccs *ChainContextSubst) wouldApply(glyphs []GlyphID, zeroContext bool) bool {
	switch ccs.format {
	case 1:
		return ccs.wouldApplyFormat1(glyphs, zeroContext)
	case 2:
		return ccs.wouldApplyFormat2(glyphs, zeroContext)
	case 3:
		return ccs.wouldApplyFormat3(glyphs, zeroContext)
	default:
		return false
	}
}

// wouldApplyFormat1 checks if format 1 (simple glyph context) would apply.
func (ccs *ChainContextSubst) wouldApplyFormat1(glyphs []GlyphID, zeroContext bool) bool {
	if len(glyphs) == 0 {
		return false
	}

	coverageIndex := ccs.coverage.GetCoverage(glyphs[0])
	if coverageIndex == NotCovered {
		return false
	}

	if int(coverageIndex) >= len(ccs.chainRuleSets) {
		return false
	}

	ruleSet := ccs.chainRuleSets[coverageIndex]
	for _, rule := range ruleSet {
		// HarfBuzz: For zero_context, require no backtrack and no lookahead
		if zeroContext && (len(rule.Backtrack) > 0 || len(rule.Lookahead) > 0) {
			continue
		}

		// Check if input matches
		// Input array includes first glyph implicitly (covered by coverage)
		// So we need len(glyphs) == len(rule.Input) + 1
		if len(glyphs) != len(rule.Input)+1 {
			continue
		}

		match := true
		for i, inputGlyph := range rule.Input {
			if glyphs[i+1] != inputGlyph {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// wouldApplyFormat2 checks if format 2 (class-based context) would apply.
func (ccs *ChainContextSubst) wouldApplyFormat2(glyphs []GlyphID, zeroContext bool) bool {
	if len(glyphs) == 0 || ccs.inputClassDef == nil {
		return false
	}

	coverageIndex := ccs.coverage.GetCoverage(glyphs[0])
	if coverageIndex == NotCovered {
		return false
	}

	firstClass := ccs.inputClassDef.GetClass(glyphs[0])
	if int(firstClass) >= len(ccs.chainRuleSets) {
		return false
	}

	ruleSet := ccs.chainRuleSets[firstClass]
	for _, rule := range ruleSet {
		// HarfBuzz: For zero_context, require no backtrack and no lookahead
		if zeroContext && (len(rule.Backtrack) > 0 || len(rule.Lookahead) > 0) {
			continue
		}

		// Check if input classes match
		if len(glyphs) != len(rule.Input)+1 {
			continue
		}

		match := true
		for i, inputClass := range rule.Input {
			// rule.Input contains class IDs stored as GlyphID
			if ccs.inputClassDef.GetClass(glyphs[i+1]) != int(inputClass) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// wouldApplyFormat3 checks if format 3 (coverage-based context) would apply.
func (ccs *ChainContextSubst) wouldApplyFormat3(glyphs []GlyphID, zeroContext bool) bool {
	// HarfBuzz: For zero_context, require no backtrack and no lookahead
	if zeroContext && (len(ccs.backtrackCoverages) > 0 || len(ccs.lookaheadCoverages) > 0) {
		return false
	}

	// Check if input coverages match
	if len(glyphs) != len(ccs.inputCoverages) {
		return false
	}

	for i, cov := range ccs.inputCoverages {
		if cov.GetCoverage(glyphs[i]) == NotCovered {
			return false
		}
	}

	return true
}

// applyFormat1 applies ChainContextSubstFormat1 (simple glyph context).
func (ccs *ChainContextSubst) applyFormat1(ctx *OTApplyContext) int {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	coverageIndex := ccs.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return 0
	}

	if int(coverageIndex) >= len(ccs.chainRuleSets) {
		return 0
	}

	rules := ccs.chainRuleSets[coverageIndex]
	for _, rule := range rules {
		if ccs.matchRuleFormat1(ctx, &rule) {
			ccs.applyLookups(ctx, rule.LookupRecords, len(rule.Input)+1)
			return 1
		}
	}

	return 0
}

// matchRuleFormat1 checks if a ChainRule matches at the current position (Format 1).
func (ccs *ChainContextSubst) matchRuleFormat1(ctx *OTApplyContext, rule *ChainRule) bool {
	// Check if enough glyphs for input sequence
	inputLen := len(rule.Input) + 1 // +1 for first glyph (covered by coverage)
	if ctx.Buffer.Idx+inputLen > len(ctx.Buffer.Info) {
		return false
	}

	// Match input sequence (starting from second glyph)
	for i, g := range rule.Input {
		if ctx.Buffer.Info[ctx.Buffer.Idx+1+i].GlyphID != g {
			return false
		}
	}

	// Check lookahead
	lookaheadStart := ctx.Buffer.Idx + inputLen
	if lookaheadStart+len(rule.Lookahead) > len(ctx.Buffer.Info) {
		return false
	}
	for i, g := range rule.Lookahead {
		if ctx.Buffer.Info[lookaheadStart+i].GlyphID != g {
			return false
		}
	}

	// Check backtrack (in reverse order)
	// HarfBuzz: backtrack matching uses out_info (output buffer) when have_output is true.
	// This is critical because earlier substitutions change the backtrack context.
	// HarfBuzz: match_backtrack() in hb-ot-layout-gsubgpos.hh:1569-1601
	backtrackLen := ctx.Buffer.BacktrackLen()
	if backtrackLen < len(rule.Backtrack) {
		return false
	}
	for i, g := range rule.Backtrack {
		// Backtrack[0] is immediately before current position
		info := ctx.Buffer.BacktrackInfo(backtrackLen - 1 - i)
		if info == nil || info.GlyphID != g {
			return false
		}
	}

	// Store match positions for use in applyLookups
	// Format 1 uses consecutive positions (no skippy-iteration)
	ctx.MatchPositions = make([]int, inputLen)
	for i := 0; i < inputLen; i++ {
		ctx.MatchPositions[i] = ctx.Buffer.Idx + i
	}

	return true
}

// applyFormat2 applies ChainContextSubstFormat2 (class-based context).
func (ccs *ChainContextSubst) applyFormat2(ctx *OTApplyContext) int {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	if ccs.coverage.GetCoverage(glyph) == NotCovered {
		return 0
	}

	// Get class of current glyph
	inputClass := ccs.inputClassDef.GetClass(glyph)
	if inputClass < 0 || inputClass >= len(ccs.chainRuleSets) {
		return 0
	}

	rules := ccs.chainRuleSets[inputClass]
	for _, rule := range rules {
		if ccs.matchRuleFormat2(ctx, &rule) {
			ccs.applyLookups(ctx, rule.LookupRecords, len(rule.Input)+1)
			return 1
		}
	}

	return 0
}

// matchRuleFormat2 checks if a ChainRule matches at the current position (Format 2).
// In Format 2, rule values are class IDs, not glyph IDs.
// This function uses skippy-iteration to skip glyphs according to LookupFlag (e.g., IgnoreMarks).
func (ccs *ChainContextSubst) matchRuleFormat2(ctx *OTApplyContext, rule *ChainRule) bool {
	inputLen := len(rule.Input) + 1

	// Build array of match positions using skippy-iteration
	// MatchPositions[0] = ctx.Buffer.Idx (current position)
	// MatchPositions[1..] = positions of subsequent input glyphs (skipping marks if needed)
	matchPositions := make([]int, inputLen)
	matchPositions[0] = ctx.Buffer.Idx

	// Match input sequence by class (starting from second glyph)
	// NextGlyph(pos) searches from pos+1, so we pass the current position
	pos := ctx.Buffer.Idx
	for i, classID := range rule.Input {
		pos = ctx.NextGlyph(pos)
		if pos < 0 {
			return false
		}
		glyphClass := ccs.inputClassDef.GetClass(ctx.Buffer.Info[pos].GlyphID)
		if glyphClass != int(classID) {
			return false
		}
		matchPositions[i+1] = pos
	}

	// Check lookahead by class (continue from last input position)
	// HarfBuzz: uses iter_context (context_match=true) with 3-way match logic.
	// SKIP_MAYBE glyphs (like ZWJ) that don't match the expected class are skipped,
	// while SKIP_NO glyphs that don't match cause the rule to fail.
	lookaheadPos := pos
	for _, classID := range rule.Lookahead {
		expectedClass := int(classID)
		lookaheadPos = ctx.NextContextMatch(lookaheadPos, func(info *GlyphInfo) bool {
			return ccs.lookaheadClassDef.GetClass(info.GlyphID) == expectedClass
		})
		if lookaheadPos < 0 {
			return false
		}
	}

	// Check backtrack by class (in reverse order, starting before current position)
	// HarfBuzz: uses iter_context (context_match=true) with 3-way match logic.
	backtrackPos := ctx.Buffer.Idx
	for _, classID := range rule.Backtrack {
		expectedClass := int(classID)
		backtrackPos = ctx.PrevContextMatch(backtrackPos, func(info *GlyphInfo) bool {
			return ccs.backtrackClassDef.GetClass(info.GlyphID) == expectedClass
		})
		if backtrackPos < 0 {
			return false
		}
	}

	// Store match positions for use in applyLookups
	ctx.MatchPositions = matchPositions
	return true
}

// applyFormat3 applies ChainContextSubstFormat3 (coverage-based context).
// HarfBuzz equivalent: ChainContextFormat3::apply() in hb-ot-layout-gsubgpos.hh:4218-4237
// which calls chain_context_apply_lookup() -> match_input() for skippy-iteration.
func (ccs *ChainContextSubst) applyFormat3(ctx *OTApplyContext) int {
	inputLen := len(ccs.inputCoverages)
	if inputLen == 0 {
		return 0
	}

	// Check first coverage (current glyph)
	if ccs.inputCoverages[0].GetCoverage(ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID) == NotCovered {
		return 0
	}

	// Build match positions using skippy-iteration (like HarfBuzz match_input)
	// MatchPositions[0] = ctx.Buffer.Idx (current position)
	// MatchPositions[1..] = positions of subsequent input glyphs (skipping marks if needed)
	matchPositions := make([]int, inputLen)
	matchPositions[0] = ctx.Buffer.Idx

	// Match remaining input sequence by coverage using skippy-iteration
	pos := ctx.Buffer.Idx
	for i := 1; i < inputLen; i++ {
		pos = ctx.NextGlyph(pos)
		if pos < 0 {
			return 0
		}
		if ccs.inputCoverages[i].GetCoverage(ctx.Buffer.Info[pos].GlyphID) == NotCovered {
			return 0
		}
		matchPositions[i] = pos
	}

	// Check lookahead by coverage (continue from last input position)
	// HarfBuzz: uses iter_context (context_match=true) with coverage matching
	// HarfBuzz: hb-ot-layout-gsubgpos.hh:1603-1635 (match_lookahead)
	//
	// HarfBuzz logic in skipping_iterator_t::next():
	// 1. may_skip == SKIP_YES -> continue (definitely skip)
	// 2. may_match != MATCH_NO -> return true (this glyph is potential match)
	// 3. may_skip == SKIP_MAYBE -> continue (skip if no match)
	// 4. else -> fail (glyph doesn't match and can't be skipped)
	//
	// Then match_func checks coverage. If not in coverage -> fail.
	//
	// SPECIAL CASE: In HarfBuzz, CGJ (U+034F) is marked as "continuation" and has
	// mask=0 set in the Arabic shaper, causing may_match to return MATCH_NO.
	// This allows CGJ to be skipped (with SKIP_MAYBE) during context matching.
	// We handle this by checking if the glyph is a "transparent" default ignorable
	// before checking may_match.
	bufLen := len(ctx.Buffer.Info)
	lookaheadPos := pos
	for _, cov := range ccs.lookaheadCoverages {
		found := false
		for lookaheadPos < bufLen-1 {
			lookaheadPos++
			skip := ctx.MaySkip(lookaheadPos, true) // context_match=true
			if skip == SkipYes {
				continue // Definitely skip
			}

			glyph := ctx.Buffer.Info[lookaheadPos].GlyphID
			// Check if glyph is in coverage
			if cov.GetCoverage(glyph) != NotCovered {
				found = true
				break // Found matching glyph in coverage
			}
			// Not in coverage - can we skip it?
			if skip == SkipMaybe {
				continue // Skip default ignorables (like CGJ) if not in coverage
			}
			// Not in coverage and can't skip -> fail
			return 0
		}
		if !found {
			return 0
		}
	}

	// Check backtrack by coverage (in reverse order, starting before current position)
	// HarfBuzz: uses iter_context (context_match=true) with coverage matching
	// HarfBuzz: hb-ot-layout-gsubgpos.hh:1569-1601 (match_backtrack)
	//
	// IMPORTANT: HarfBuzz uses out_info (output buffer) for backtrack matching!
	// HarfBuzz: backtrack_len() returns out_len when have_output is true
	// HarfBuzz: prev() iterates over out_info, not info
	//
	// When have_output is true:
	// - out_info[0:out_len] contains already-processed glyphs
	// - info[idx:] contains not-yet-processed glyphs
	// Backtrack matching should use out_info, not info!
	backtrackPos := ctx.Buffer.BacktrackLen()
	for _, cov := range ccs.backtrackCoverages {
		found := false
		for backtrackPos > 0 {
			backtrackPos--
			// Get glyph info from the correct buffer (output for backtrack)
			info := ctx.Buffer.BacktrackInfo(backtrackPos)
			if info == nil {
				return 0
			}
			skip := ctx.MaySkipInfo(info, true) // context_match=true
			if skip == SkipYes {
				continue // Definitely skip (e.g., ignored by LookupFlag)
			}

			// Check if glyph is in coverage
			if cov.GetCoverage(info.GlyphID) != NotCovered {
				found = true
				break // Found matching glyph in coverage
			}
			// Not in coverage - can we skip it?
			if skip == SkipMaybe {
				continue // Skip default ignorables (like CGJ) if not in coverage
			}
			// Not in coverage and can't skip -> fail
			return 0
		}
		if !found {
			return 0
		}
	}

	// Store match positions for use in applyLookups
	ctx.MatchPositions = matchPositions

	// Apply lookups
	ccs.applyLookups(ctx, ccs.lookupRecords, inputLen)
	return 1
}

// applyLookups applies lookup records to matched input positions.
// HarfBuzz equivalent: apply_lookup() in hb-ot-layout-gsubgpos.hh:1772-1912
func (ccs *ChainContextSubst) applyLookups(ctx *OTApplyContext, lookupRecords []LookupRecord, count int) {
	if ccs.gsub == nil {
		ctx.Buffer.Idx += count
		return
	}

	// Check recursion limit
	if ctx.NestingLevel >= MaxNestingLevel {
		ctx.Buffer.Idx += count
		return
	}

	buffer := ctx.Buffer
	if !buffer.haveOutput {
		buffer.Idx += count
		return
	}

	// Validate MatchPositions
	if ctx.MatchPositions == nil || len(ctx.MatchPositions) < count {
		// Fallback: create consecutive positions
		ctx.MatchPositions = make([]int, count)
		for i := 0; i < count; i++ {
			ctx.MatchPositions[i] = buffer.Idx + i
		}
	}

	// HarfBuzz: apply_lookup() lines 1781-1791
	// "All positions are distance from beginning of *output* buffer. Adjust."
	bl := buffer.outLen // backtrack_len()
	matchEnd := ctx.MatchPositions[count-1] + 1
	end := bl + matchEnd - buffer.Idx

	delta := bl - buffer.Idx
	// Convert positions to new indexing (in-place modification like HarfBuzz)
	for j := 0; j < count; j++ {
		ctx.MatchPositions[j] += delta
	}

	// Apply each lookup record
	for _, record := range lookupRecords {
		idx := int(record.SequenceIndex)
		if idx >= count {
			continue
		}

		// HarfBuzz: orig_len = backtrack_len() + lookahead_len()
		origLen := buffer.outLen + (len(buffer.Info) - buffer.Idx)

		// HarfBuzz: "This can happen if earlier recursed lookups deleted many entries."
		if ctx.MatchPositions[idx] >= origLen {
			continue
		}

		// Move to the target position
		if !buffer.moveTo(ctx.MatchPositions[idx]) {
			break
		}

		lookup := ccs.gsub.GetLookup(int(record.LookupIndex))
		if lookup == nil {
			continue
		}

		// Determine mark filtering set for nested lookup
		nestedMarkFilteringSet := -1
		if lookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
			nestedMarkFilteringSet = int(lookup.MarkFilter)
		}

		// Apply nested lookup
		nestedCtx := &OTApplyContext{
			Buffer:           buffer,
			LookupFlag:       lookup.Flag,
			GDEF:             ctx.GDEF,
			HasGlyphClasses:  ctx.HasGlyphClasses,
			MarkFilteringSet: nestedMarkFilteringSet,
			NestingLevel:     ctx.NestingLevel + 1,
			FeatureMask:      ctx.FeatureMask,
			Font:             ctx.Font,
		}

		applied := false
		for _, subtable := range lookup.subtables {
			if subtable.Apply(nestedCtx) > 0 {
				applied = true
				break
			}
		}

		if !applied {
			continue
		}

		// HarfBuzz: new_len = backtrack_len() + lookahead_len()
		newLen := buffer.outLen + (len(buffer.Info) - buffer.Idx)
		delta := newLen - origLen

		if delta == 0 {
			continue
		}

		// HarfBuzz: "Recursed lookup changed buffer len. Adjust."
		end += delta
		if end < ctx.MatchPositions[idx] {
			// HarfBuzz: "End might end up being smaller than match_positions[idx]..."
			delta += ctx.MatchPositions[idx] - end
			end = ctx.MatchPositions[idx]
		}

		next := idx + 1 // next now is the position after the recursed lookup

		if delta > 0 {
			// Ensure MatchPositions has enough capacity
			if count+delta > len(ctx.MatchPositions) {
				newPositions := make([]int, count+delta)
				copy(newPositions, ctx.MatchPositions)
				ctx.MatchPositions = newPositions
			}
		} else {
			// NOTE: delta is non-positive
			if next-count > delta {
				delta = next - count
			}
			next -= delta
		}

		// Shift subsequent positions
		if next < count {
			copy(ctx.MatchPositions[next+delta:], ctx.MatchPositions[next:count])
		}
		next += delta
		count += delta

		// Fill in new entries
		for j := idx + 1; j < next; j++ {
			ctx.MatchPositions[j] = ctx.MatchPositions[j-1] + 1
		}

		// Fixup the rest
		for ; next < count; next++ {
			ctx.MatchPositions[next] += delta
		}
	}

	if end < 0 {
		end = 0
	}
	// Ensure end doesn't exceed the virtual buffer length
	maxEnd := buffer.outLen + (len(buffer.Info) - buffer.Idx)
	if end > maxEnd {
		end = maxEnd
	}
	if !buffer.moveTo(end) {
		// moveTo failed - advance Idx by count to prevent infinite loop
		buffer.Idx += count
	}
}

// --- Reverse Chain Single Substitution ---

// ReverseChainSingleSubst represents a Reverse Chaining Context Single Substitution subtable (GSUB Type 8).
// It is designed to be applied in reverse (from end to beginning of buffer).
// Unlike ChainContextSubst, it only performs single glyph substitution (no nested lookups).
type ReverseChainSingleSubst struct {
	coverage           *Coverage
	backtrackCoverages []*Coverage
	lookaheadCoverages []*Coverage
	substitutes        []GlyphID
}

func parseReverseChainSingleSubst(data []byte, offset int) (*ReverseChainSingleSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	off := offset + 4

	// Backtrack coverages
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
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

	// Substitute glyphs
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	substituteCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2

	if off+substituteCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	substitutes := make([]GlyphID, substituteCount)
	for i := 0; i < substituteCount; i++ {
		substitutes[i] = GlyphID(binary.BigEndian.Uint16(data[off+i*2:]))
	}

	return &ReverseChainSingleSubst{
		coverage:           coverage,
		backtrackCoverages: backtrackCoverages,
		lookaheadCoverages: lookaheadCoverages,
		substitutes:        substitutes,
	}, nil
}

// Apply applies the reverse chaining context single substitution.
// This lookup is intended to be applied in reverse (from end to beginning of buffer).
// It replaces the current glyph if it matches the coverage and context.
func (r *ReverseChainSingleSubst) Apply(ctx *OTApplyContext) int {
	glyph := ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID
	coverageIndex := r.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return 0
	}

	if int(coverageIndex) >= len(r.substitutes) {
		return 0
	}

	// Match backtrack (in reverse order, looking backwards from current position)
	if ctx.Buffer.Idx < len(r.backtrackCoverages) {
		return 0
	}
	for i, cov := range r.backtrackCoverages {
		if cov.GetCoverage(ctx.Buffer.Info[ctx.Buffer.Idx-1-i].GlyphID) == NotCovered {
			return 0
		}
	}

	// Match lookahead (looking forward from current position)
	lookaheadStart := ctx.Buffer.Idx + 1
	if lookaheadStart+len(r.lookaheadCoverages) > len(ctx.Buffer.Info) {
		return 0
	}
	for i, cov := range r.lookaheadCoverages {
		if cov.GetCoverage(ctx.Buffer.Info[lookaheadStart+i].GlyphID) == NotCovered {
			return 0
		}
	}

	// Replace glyph in place (don't advance index - reverse lookup handles this)
	ctx.Buffer.Info[ctx.Buffer.Idx].GlyphID = r.substitutes[coverageIndex]
	return 1
}

// ApplyLookupReverseWithGDEF applies this lookup in reverse order with GDEF-based glyph filtering.
// This is the intended way to use ReverseChainSingleSubst (GSUB Type 8).
func (g *GSUB) ApplyLookupReverseWithGDEF(lookupIndex int, glyphs []GlyphID, gdef *GDEF, font *Font) []GlyphID {
	lookup := g.GetLookup(lookupIndex)
	if lookup == nil {
		return glyphs
	}

	// Only Type 8 should be applied in reverse
	if lookup.Type != GSUBTypeReverseChainSingle {
		return g.ApplyLookupWithGDEF(lookupIndex, glyphs, gdef, font)
	}

	// Determine mark filtering set index
	markFilteringSet := -1
	if lookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
		markFilteringSet = int(lookup.MarkFilter)
	}

	// Create temporary buffer from glyphs
	// HarfBuzz: All glyphs start with global_mask set
	buf := &Buffer{
		Info: make([]GlyphInfo, len(glyphs)),
	}
	for i, gid := range glyphs {
		buf.Info[i] = GlyphInfo{
			GlyphID: gid,
			Cluster: i,
			Mask:    MaskGlobal, // HarfBuzz: glyphs have global_mask
		}
	}

	ctx := &OTApplyContext{
		Buffer:           buf,
		LookupFlag:       lookup.Flag,
		GDEF:             gdef,
		MarkFilteringSet: markFilteringSet,
		FeatureMask:      MaskGlobal, // HarfBuzz: lookup_mask is never 0
		Font:             font,
	}

	// Apply in reverse order
	for ctx.Buffer.Idx = len(ctx.Buffer.Info) - 1; ctx.Buffer.Idx >= 0; ctx.Buffer.Idx-- {
		if ctx.ShouldSkipGlyph(ctx.Buffer.Idx) {
			continue
		}
		for _, subtable := range lookup.subtables {
			if subtable.Apply(ctx) > 0 {
				break
			}
		}
	}

	return extractGlyphs(buf)
}

// WouldSubstitute checks if a lookup would apply to the given glyph sequence.
// HarfBuzz equivalent: hb_ot_layout_lookup_would_substitute() in hb-ot-layout.cc:1547-1560
//
// Parameters:
// - glyphs: The sequence of glyph IDs to test
// - zeroContext: If true, ignore context rules (only match direct substitutions)
//
// Returns true if any subtable in the lookup would match the input sequence.
func (l *GSUBLookup) WouldSubstitute(glyphs []GlyphID, zeroContext bool) bool {
	if len(glyphs) == 0 {
		return false
	}

	for _, subtable := range l.subtables {
		if wouldApply(subtable, glyphs, zeroContext) {
			return true
		}
	}
	return false
}

// wouldApply checks if a single subtable would apply to the glyph sequence.
// HarfBuzz equivalent: would_apply() methods in hb-ot-layout-gsubgpos.hh
func wouldApply(subtable GSUBSubtable, glyphs []GlyphID, zeroContext bool) bool {
	switch st := subtable.(type) {
	case *SingleSubst:
		// SingleSub: matches if first glyph is in coverage
		if len(glyphs) == 1 {
			return st.coverage.GetCoverage(glyphs[0]) != NotCovered
		}
		return false

	case *LigatureSubst:
		// LigatureSub: matches if first glyph is in coverage AND
		// remaining glyphs match ligature components
		if len(glyphs) < 2 {
			return false
		}
		covIdx := st.Coverage().GetCoverage(glyphs[0])
		if covIdx == NotCovered {
			return false
		}
		// Check if any ligature in the set matches
		ligSets := st.LigatureSets()
		if int(covIdx) >= len(ligSets) {
			return false
		}
		for _, lig := range ligSets[covIdx] {
			if wouldMatchLigature(glyphs[1:], lig.Components) {
				return true
			}
		}
		return false

	case *MultipleSubst:
		// MultipleSub: matches if first glyph is in coverage
		if len(glyphs) == 1 {
			return st.coverage.GetCoverage(glyphs[0]) != NotCovered
		}
		return false

	case *ContextSubst:
		// Context: for now, if zeroContext is true, don't match contextual rules
		if zeroContext {
			return false
		}
		// TODO: implement full context matching
		return false

	case *ChainContextSubst:
		// ChainContext: HarfBuzz chain_context_would_apply_lookup()
		// For zero_context: only match if no backtrack AND no lookahead context
		// Then check if input glyphs match
		return st.wouldApply(glyphs, zeroContext)

	default:
		return false
	}
}

// wouldMatchLigature checks if the input glyphs match the ligature components.
// HarfBuzz equivalent: would_match_input() in hb-ot-layout-gsubgpos.hh:1288-1306
func wouldMatchLigature(input []GlyphID, components []GlyphID) bool {
	if len(input) != len(components) {
		return false
	}
	for i, comp := range components {
		if input[i] != comp {
			return false
		}
	}
	return true
}

// WouldSubstituteFeature checks if a feature would substitute the given glyph sequence.
// HarfBuzz equivalent: hb_indic_would_substitute_feature_t::would_substitute() in hb-ot-shaper-indic.cc:99-107
func (g *GSUB) WouldSubstituteFeature(featureTag Tag, glyphs []GlyphID, zeroContext bool) bool {
	featureList, err := g.ParseFeatureList()
	if err != nil {
		return false
	}

	lookupIndices := featureList.FindFeature(featureTag)
	if lookupIndices == nil {
		return false
	}

	for _, lookupIdx := range lookupIndices {
		lookup := g.GetLookup(int(lookupIdx))
		if lookup == nil {
			continue
		}
		if lookup.WouldSubstitute(glyphs, zeroContext) {
			return true
		}
	}
	return false
}
