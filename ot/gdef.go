// Package ot provides OpenType font table parsing.
package ot

import (
	"encoding/binary"
)

// GlyphClass constants for GDEF glyph classification.
const (
	GlyphClassUnclassified = 0 // Unclassified glyph
	GlyphClassBase         = 1 // Base glyph (single character, spacing glyph)
	GlyphClassLigature     = 2 // Ligature glyph (multiple characters, spacing glyph)
	GlyphClassMark         = 3 // Mark glyph (non-spacing combining glyph)
	GlyphClassComponent    = 4 // Component glyph (part of a ligature)
)

// GDEF represents the Glyph Definition table.
type GDEF struct {

	// Glyph class definitions (optional)
	glyphClassDef *ClassDef

	// Attachment point list (optional)
	attachList *AttachList

	// Ligature caret list (optional)
	ligCaretList *LigCaretList

	// Mark attachment class definitions (optional)
	markAttachClassDef *ClassDef

	// Mark glyph sets (version >= 1.2, optional)
	markGlyphSetsDef *MarkGlyphSetsDef
	data             []byte

	// Version (major.minor)
	versionMajor uint16
	versionMinor uint16
}

// AttachList contains attachment points for glyphs.
type AttachList struct {
	coverage     *Coverage
	attachPoints [][]uint16 // Attachment point indices for each glyph
}

// LigCaretList contains ligature caret positions.
type LigCaretList struct {
	coverage  *Coverage
	ligGlyphs []LigGlyph
}

// LigGlyph contains caret values for a ligature glyph.
type LigGlyph struct {
	caretValues []CaretValue
}

// CaretValue represents a caret position within a ligature.
type CaretValue struct {
	format     uint16
	coordinate int16  // Format 1: X or Y coordinate
	pointIndex uint16 // Format 2: contour point index
	// Format 3: coordinate + Device table (not fully implemented)
}

// MarkGlyphSetsDef contains mark glyph set definitions.
type MarkGlyphSetsDef struct {
	coverages []*Coverage
}

// ParseGDEF parses the GDEF table from raw data.
func ParseGDEF(data []byte) (*GDEF, error) {
	if len(data) < 12 {
		return nil, ErrInvalidTable
	}

	versionMajor := binary.BigEndian.Uint16(data[0:])
	versionMinor := binary.BigEndian.Uint16(data[2:])

	// Validate version
	if versionMajor != 1 || (versionMinor != 0 && versionMinor != 2 && versionMinor != 3) {
		return nil, ErrInvalidFormat
	}

	gdef := &GDEF{
		data:         data,
		versionMajor: versionMajor,
		versionMinor: versionMinor,
	}

	// Parse offsets
	glyphClassDefOffset := int(binary.BigEndian.Uint16(data[4:]))
	attachListOffset := int(binary.BigEndian.Uint16(data[6:]))
	ligCaretListOffset := int(binary.BigEndian.Uint16(data[8:]))
	markAttachClassDefOffset := int(binary.BigEndian.Uint16(data[10:]))

	var markGlyphSetsDefOffset int
	if versionMinor >= 2 && len(data) >= 14 {
		markGlyphSetsDefOffset = int(binary.BigEndian.Uint16(data[12:]))
	}

	// Parse GlyphClassDef
	if glyphClassDefOffset != 0 {
		cd, err := ParseClassDef(data, glyphClassDefOffset)
		if err != nil {
			return nil, err
		}
		gdef.glyphClassDef = cd
	}

	// Parse AttachList
	if attachListOffset != 0 {
		al, err := parseAttachList(data, attachListOffset)
		if err != nil {
			return nil, err
		}
		gdef.attachList = al
	}

	// Parse LigCaretList
	if ligCaretListOffset != 0 {
		lcl, err := parseLigCaretList(data, ligCaretListOffset)
		if err != nil {
			return nil, err
		}
		gdef.ligCaretList = lcl
	}

	// Parse MarkAttachClassDef
	if markAttachClassDefOffset != 0 {
		cd, err := ParseClassDef(data, markAttachClassDefOffset)
		if err != nil {
			return nil, err
		}
		gdef.markAttachClassDef = cd
	}

	// Parse MarkGlyphSetsDef (version >= 1.2)
	if markGlyphSetsDefOffset != 0 {
		mgsd, err := parseMarkGlyphSetsDef(data, markGlyphSetsDefOffset)
		if err != nil {
			return nil, err
		}
		gdef.markGlyphSetsDef = mgsd
	}

	return gdef, nil
}

// parseAttachList parses the AttachList subtable.
func parseAttachList(data []byte, offset int) (*AttachList, error) {
	if offset+4 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverageOffset := int(binary.BigEndian.Uint16(data[offset:]))
	glyphCount := int(binary.BigEndian.Uint16(data[offset+2:]))

	if offset+4+glyphCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	// Parse coverage
	cov, err := ParseCoverage(data, offset+coverageOffset)
	if err != nil {
		return nil, err
	}

	al := &AttachList{
		coverage:     cov,
		attachPoints: make([][]uint16, glyphCount),
	}

	// Parse attachment point tables
	for i := 0; i < glyphCount; i++ {
		attachPointOffset := int(binary.BigEndian.Uint16(data[offset+4+i*2:]))
		if attachPointOffset == 0 {
			continue
		}

		apOff := offset + attachPointOffset
		if apOff+2 > len(data) {
			return nil, ErrInvalidOffset
		}

		pointCount := int(binary.BigEndian.Uint16(data[apOff:]))
		if apOff+2+pointCount*2 > len(data) {
			return nil, ErrInvalidOffset
		}

		al.attachPoints[i] = make([]uint16, pointCount)
		for j := 0; j < pointCount; j++ {
			al.attachPoints[i][j] = binary.BigEndian.Uint16(data[apOff+2+j*2:])
		}
	}

	return al, nil
}

// parseLigCaretList parses the LigCaretList subtable.
func parseLigCaretList(data []byte, offset int) (*LigCaretList, error) {
	if offset+4 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverageOffset := int(binary.BigEndian.Uint16(data[offset:]))
	ligGlyphCount := int(binary.BigEndian.Uint16(data[offset+2:]))

	if offset+4+ligGlyphCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	// Parse coverage
	cov, err := ParseCoverage(data, offset+coverageOffset)
	if err != nil {
		return nil, err
	}

	lcl := &LigCaretList{
		coverage:  cov,
		ligGlyphs: make([]LigGlyph, ligGlyphCount),
	}

	// Parse LigGlyph tables
	for i := 0; i < ligGlyphCount; i++ {
		ligGlyphOffset := int(binary.BigEndian.Uint16(data[offset+4+i*2:]))
		if ligGlyphOffset == 0 {
			continue
		}

		lgOff := offset + ligGlyphOffset
		if lgOff+2 > len(data) {
			return nil, ErrInvalidOffset
		}

		caretCount := int(binary.BigEndian.Uint16(data[lgOff:]))
		if lgOff+2+caretCount*2 > len(data) {
			return nil, ErrInvalidOffset
		}

		lcl.ligGlyphs[i].caretValues = make([]CaretValue, caretCount)

		// Parse CaretValue tables
		for j := 0; j < caretCount; j++ {
			caretOffset := int(binary.BigEndian.Uint16(data[lgOff+2+j*2:]))
			cvOff := lgOff + caretOffset
			if cvOff+4 > len(data) {
				return nil, ErrInvalidOffset
			}

			format := binary.BigEndian.Uint16(data[cvOff:])
			cv := CaretValue{format: format}

			switch format {
			case 1:
				cv.coordinate = int16(binary.BigEndian.Uint16(data[cvOff+2:]))
			case 2:
				cv.pointIndex = binary.BigEndian.Uint16(data[cvOff+2:])
			case 3:
				cv.coordinate = int16(binary.BigEndian.Uint16(data[cvOff+2:]))
				// Device table offset at cvOff+4 (not fully implemented)
			}

			lcl.ligGlyphs[i].caretValues[j] = cv
		}
	}

	return lcl, nil
}

// parseMarkGlyphSetsDef parses the MarkGlyphSetsDef subtable.
func parseMarkGlyphSetsDef(data []byte, offset int) (*MarkGlyphSetsDef, error) {
	if offset+4 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	markSetCount := int(binary.BigEndian.Uint16(data[offset+2:]))
	if offset+4+markSetCount*4 > len(data) {
		return nil, ErrInvalidOffset
	}

	mgsd := &MarkGlyphSetsDef{
		coverages: make([]*Coverage, markSetCount),
	}

	// Parse coverage offsets (32-bit offsets)
	for i := 0; i < markSetCount; i++ {
		covOffset := int(binary.BigEndian.Uint32(data[offset+4+i*4:]))
		if covOffset == 0 {
			continue
		}

		cov, err := ParseCoverage(data, offset+covOffset)
		if err != nil {
			return nil, err
		}
		mgsd.coverages[i] = cov
	}

	return mgsd, nil
}

// Version returns the GDEF table version as (major, minor).
func (g *GDEF) Version() (uint16, uint16) {
	return g.versionMajor, g.versionMinor
}

// HasGlyphClasses returns true if the GDEF table has glyph class definitions.
func (g *GDEF) HasGlyphClasses() bool {
	return g.glyphClassDef != nil
}

// GetGlyphClass returns the glyph class for a glyph ID.
// Returns GlyphClassUnclassified (0) if no class is defined.
func (g *GDEF) GetGlyphClass(glyph GlyphID) int {
	if g.glyphClassDef == nil {
		return GlyphClassUnclassified
	}
	return g.glyphClassDef.GetClass(glyph)
}

// IsBaseGlyph returns true if the glyph is classified as a base glyph.
func (g *GDEF) IsBaseGlyph(glyph GlyphID) bool {
	return g.GetGlyphClass(glyph) == GlyphClassBase
}

// IsLigatureGlyph returns true if the glyph is classified as a ligature glyph.
func (g *GDEF) IsLigatureGlyph(glyph GlyphID) bool {
	return g.GetGlyphClass(glyph) == GlyphClassLigature
}

// IsMarkGlyph returns true if the glyph is classified as a mark glyph.
func (g *GDEF) IsMarkGlyph(glyph GlyphID) bool {
	return g.GetGlyphClass(glyph) == GlyphClassMark
}

// IsComponentGlyph returns true if the glyph is classified as a component glyph.
func (g *GDEF) IsComponentGlyph(glyph GlyphID) bool {
	return g.GetGlyphClass(glyph) == GlyphClassComponent
}

// HasMarkAttachClasses returns true if the GDEF table has mark attachment class definitions.
func (g *GDEF) HasMarkAttachClasses() bool {
	return g.markAttachClassDef != nil
}

// GetMarkAttachClass returns the mark attachment class for a glyph ID.
// Returns 0 if no class is defined.
func (g *GDEF) GetMarkAttachClass(glyph GlyphID) int {
	if g.markAttachClassDef == nil {
		return 0
	}
	return g.markAttachClassDef.GetClass(glyph)
}

// HasAttachList returns true if the GDEF table has an attachment point list.
func (g *GDEF) HasAttachList() bool {
	return g.attachList != nil
}

// GetAttachPoints returns the attachment point indices for a glyph.
// Returns nil if the glyph has no attachment points defined.
func (g *GDEF) GetAttachPoints(glyph GlyphID) []uint16 {
	if g.attachList == nil {
		return nil
	}
	idx := g.attachList.coverage.GetCoverage(glyph)
	if idx == NotCovered || int(idx) >= len(g.attachList.attachPoints) {
		return nil
	}
	return g.attachList.attachPoints[idx]
}

// HasLigCaretList returns true if the GDEF table has a ligature caret list.
func (g *GDEF) HasLigCaretList() bool {
	return g.ligCaretList != nil
}

// GetLigCaretCount returns the number of caret positions for a ligature glyph.
// Returns 0 if the glyph has no caret positions defined.
func (g *GDEF) GetLigCaretCount(glyph GlyphID) int {
	if g.ligCaretList == nil {
		return 0
	}
	idx := g.ligCaretList.coverage.GetCoverage(glyph)
	if idx == NotCovered || int(idx) >= len(g.ligCaretList.ligGlyphs) {
		return 0
	}
	return len(g.ligCaretList.ligGlyphs[idx].caretValues)
}

// GetLigCarets returns the caret values for a ligature glyph.
// Returns nil if the glyph has no caret positions defined.
func (g *GDEF) GetLigCarets(glyph GlyphID) []CaretValue {
	if g.ligCaretList == nil {
		return nil
	}
	idx := g.ligCaretList.coverage.GetCoverage(glyph)
	if idx == NotCovered || int(idx) >= len(g.ligCaretList.ligGlyphs) {
		return nil
	}
	return g.ligCaretList.ligGlyphs[idx].caretValues
}

// HasMarkGlyphSets returns true if the GDEF table has mark glyph sets (version >= 1.2).
func (g *GDEF) HasMarkGlyphSets() bool {
	return g.markGlyphSetsDef != nil
}

// MarkGlyphSetCount returns the number of mark glyph sets.
func (g *GDEF) MarkGlyphSetCount() int {
	if g.markGlyphSetsDef == nil {
		return 0
	}
	return len(g.markGlyphSetsDef.coverages)
}

// IsInMarkGlyphSet returns true if the glyph is in the specified mark glyph set.
func (g *GDEF) IsInMarkGlyphSet(glyph GlyphID, setIndex int) bool {
	if g.markGlyphSetsDef == nil {
		return false
	}
	if setIndex < 0 || setIndex >= len(g.markGlyphSetsDef.coverages) {
		return false
	}
	cov := g.markGlyphSetsDef.coverages[setIndex]
	if cov == nil {
		return false
	}
	return cov.GetCoverage(glyph) != NotCovered
}

// Coordinate returns the coordinate value for a CaretValue (format 1 or 3).
func (cv *CaretValue) Coordinate() int16 {
	return cv.coordinate
}

// PointIndex returns the contour point index for a CaretValue (format 2).
func (cv *CaretValue) PointIndex() uint16 {
	return cv.pointIndex
}

// Format returns the CaretValue format (1, 2, or 3).
func (cv *CaretValue) Format() uint16 {
	return cv.format
}
