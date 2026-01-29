package ot

import (
	"encoding/binary"
	"fmt"
)

// Font represents an OpenType font.
type Font struct {
	data   []byte
	tables map[Tag]tableRecord
}

type tableRecord struct {
	offset uint32
	length uint32
}

// ParseFont parses an OpenType font from data.
// For TrueType Collections (.ttc) or DFONTs, use index to select a font.
func ParseFont(data []byte, index int) (*Font, error) {
	if len(data) < 12 {
		return nil, ErrInvalidFont
	}

	p := NewParser(data)

	// Check for TTC or DFONT
	magic, _ := p.U32()
	if magic == 0x74746366 { // 'ttcf'
		return parseTTC(data, index)
	}
	if magic == 0x00000100 { // DFONT resource data offset
		return parseDfont(data, index)
	}

	// Single font
	if index != 0 {
		return nil, ErrInvalidFont
	}

	// Single font: table offsets are absolute (start at 0)
	return parseOffsetTable(data, 0, 0)
}

func parseTTC(data []byte, index int) (*Font, error) {
	p := NewParser(data)
	p.Skip(4) // 'ttcf'

	_, err := p.U32() // version
	if err != nil {
		return nil, ErrInvalidFont
	}

	numFonts, err := p.U32()
	if err != nil {
		return nil, ErrInvalidFont
	}

	if index < 0 || index >= int(numFonts) {
		return nil, ErrInvalidFont
	}

	// Read offset for requested font
	p.Skip(index * 4)
	offset, err := p.U32()
	if err != nil {
		return nil, ErrInvalidFont
	}

	// TTC: table offsets are absolute to the file
	return parseOffsetTable(data, int(offset), 0)
}

// parseDfont parses a dfont resource map (Apple format).
// See https://github.com/kreativekorp/ksfl/wiki/Macintosh-Resource-File-Format
func parseDfont(data []byte, index int) (*Font, error) {
	if len(data) < 16 {
		return nil, ErrInvalidFont
	}

	const dfontResourceDataOffset = 0x100

	resourceMapOffset := binary.BigEndian.Uint32(data[4:8])
	resourceMapLength := binary.BigEndian.Uint32(data[12:16])

	const maxOffset = 1 << 29
	if resourceMapOffset > maxOffset || resourceMapLength > maxOffset {
		return nil, ErrInvalidFont
	}

	// Read type list offset from resource map (at offset 24)
	const headerSize = 28
	if resourceMapLength < headerSize {
		return nil, ErrInvalidFont
	}
	if int(resourceMapOffset)+26 > len(data) {
		return nil, ErrInvalidFont
	}
	typeListOffset := int16(binary.BigEndian.Uint16(data[resourceMapOffset+24:]))
	if typeListOffset < headerSize || resourceMapLength < uint32(typeListOffset)+2 {
		return nil, ErrInvalidFont
	}

	// Read type count
	typeCountOffset := int(resourceMapOffset) + int(typeListOffset)
	if typeCountOffset+2 > len(data) {
		return nil, ErrInvalidFont
	}
	typeCount := int(binary.BigEndian.Uint16(data[typeCountOffset:])) + 1 // Count is stored minus one

	// Find "sfnt" type entry
	const tSize = 8
	typeListStart := typeCountOffset + 2
	numFonts, resourceListOffset := 0, 0

	for i := 0; i < typeCount; i++ {
		entryOffset := typeListStart + tSize*i
		if entryOffset+8 > len(data) {
			return nil, ErrInvalidFont
		}
		typeTag := binary.BigEndian.Uint32(data[entryOffset:])
		if typeTag == 0x73666e74 { // "sfnt"
			numFonts = int(int16(binary.BigEndian.Uint16(data[entryOffset+4:]))) + 1
			resourceListOffset = int(int16(binary.BigEndian.Uint16(data[entryOffset+6:])))
			break
		}
	}

	if numFonts == 0 {
		return nil, ErrInvalidFont
	}
	if index < 0 || index >= numFonts {
		return nil, ErrInvalidFont
	}

	// Read resource list entry
	const rSize = 12
	resourceOffset := int(resourceMapOffset) + int(typeListOffset) + resourceListOffset + rSize*index
	if resourceOffset+8 > len(data) {
		return nil, ErrInvalidFont
	}

	// Resource data offset is at bytes 4-7, but only lower 24 bits are the offset
	fontOffset := int(0xffffff & binary.BigEndian.Uint32(data[resourceOffset+4:]))
	// Add dfont header offset and skip 4-byte length prefix
	fontOffset += dfontResourceDataOffset + 4

	// DFONT: table offsets are relative to the sfnt start, not absolute to file
	return parseOffsetTable(data, fontOffset, fontOffset)
}

// parseOffsetTable parses an sfnt offset table.
// offset is where the sfnt starts in data.
// tableBaseOffset is added to all table offsets (needed for DFONT where
// table offsets are relative to sfnt start, not absolute to file).
func parseOffsetTable(data []byte, offset int, tableBaseOffset int) (*Font, error) {
	if offset+12 > len(data) {
		return nil, ErrInvalidFont
	}

	p := NewParser(data)
	p.SetOffset(offset)

	sfntVersion, _ := p.U32()
	// Valid: 0x00010000 (TrueType), 'OTTO' (CFF), 'true', 'typ1'
	if sfntVersion != 0x00010000 &&
		sfntVersion != 0x4F54544F && // OTTO
		sfntVersion != 0x74727565 && // true
		sfntVersion != 0x74797031 { // typ1
		return nil, ErrInvalidFont
	}

	numTables, _ := p.U16()
	p.Skip(6) // searchRange, entrySelector, rangeShift

	font := &Font{
		data:   data,
		tables: make(map[Tag]tableRecord, numTables),
	}

	for i := 0; i < int(numTables); i++ {
		tag, _ := p.Tag()
		p.Skip(4) // checksum
		tableOffset, _ := p.U32()
		tableLength, _ := p.U32()

		font.tables[tag] = tableRecord{
			offset: tableOffset + uint32(tableBaseOffset),
			length: tableLength,
		}
	}

	return font, nil
}

// HasTable returns true if the font has the given table.
func (f *Font) HasTable(tag Tag) bool {
	_, ok := f.tables[tag]
	return ok
}

// TableData returns the raw data for a table.
func (f *Font) TableData(tag Tag) ([]byte, error) {
	rec, ok := f.tables[tag]
	if !ok {
		return nil, ErrTableNotFound
	}

	end := rec.offset + rec.length
	if end > uint32(len(f.data)) {
		return nil, ErrInvalidTable
	}

	return f.data[rec.offset:end], nil
}

// TableParser returns a parser for the given table.
func (f *Font) TableParser(tag Tag) (*Parser, error) {
	data, err := f.TableData(tag)
	if err != nil {
		return nil, err
	}
	return NewParser(data), nil
}

// NumGlyphs returns the number of glyphs in the font.
// Returns 0 if maxp table is missing or invalid.
func (f *Font) NumGlyphs() int {
	data, err := f.TableData(TagMaxp)
	if err != nil || len(data) < 6 {
		return 0
	}
	return int(binary.BigEndian.Uint16(data[4:]))
}

// HasGlyph returns true if the font has a glyph for the given codepoint.
// HarfBuzz equivalent: hb_font_t::has_glyph() (hb-font.hh)
func (f *Font) HasGlyph(cp Codepoint) bool {
	data, err := f.TableData(TagCmap)
	if err != nil {
		return false
	}

	cmap, err := ParseCmap(data)
	if err != nil {
		return false
	}

	_, ok := cmap.Lookup(cp)
	return ok
}

// HasGlyphNames returns true if the font has a post table with glyph names.
// Returns false for post table version 3.0 (which has no glyph names).
func (f *Font) HasGlyphNames() bool {
	data, err := f.TableData(TagPost)
	if err != nil {
		return false
	}

	post, err := ParsePostTable(data)
	if err != nil {
		return false
	}

	// post version 3.0 has no glyph names
	// Version is stored as fixed-point: 0x00030000 = 3.0
	return post.Version != 0x00030000
}

// GetGlyphName returns the glyph name for a given glyph ID.
// Tries post table first, then CFF table, then falls back to "gidNNN".
// HarfBuzz equivalent: hb_ot_get_glyph_name() in hb-ot-font.cc:833-846
func (f *Font) GetGlyphName(glyph GlyphID) string {
	// Try post table first
	data, err := f.TableData(TagPost)
	if err == nil {
		post, err := ParsePostTable(data)
		if err == nil {
			name := post.GetGlyphName(glyph)
			if name != "" {
				return name
			}
		}
	}

	// Try CFF table second (for fonts with post v3.0)
	// HarfBuzz equivalent: #ifndef HB_NO_OT_FONT_CFF
	data, err = f.TableData(TagCFF)
	if err == nil {
		cff, err := ParseCFF(data)
		if err == nil {
			name := cff.GetGlyphName(glyph)
			if name != "" {
				return name
			}
		}
	}

	// Fallback: generate "gidDDD" format (HarfBuzz behavior)
	return fmt.Sprintf("gid%d", glyph)
}

// GetGlyphFromName converts a glyph name to a glyph ID.
// Supports multiple formats:
//   - Direct post/CFF table lookup ("A", "B", "uni0622.fina")
//   - Numeric glyph index ("123")
//   - gidDDD syntax ("gid123")
//   - uniUUUU syntax with cmap lookup ("uni0622")
// Returns false if the name cannot be resolved.
// HarfBuzz equivalent: hb_ot_get_glyph_from_name() in hb-ot-font.cc:849-872
func (f *Font) GetGlyphFromName(name string) (GlyphID, bool) {
	// 1. Try post table lookup first
	if data, err := f.TableData(TagPost); err == nil {
		if post, err := ParsePostTable(data); err == nil {
			if gid, ok := post.GetGlyphFromName(name); ok {
				return gid, true
			}
		}
	}

	// 2. Try CFF table lookup (for fonts with post v3.0)
	// HarfBuzz equivalent: #ifndef HB_NO_OT_FONT_CFF
	if data, err := f.TableData(TagCFF); err == nil {
		if cff, err := ParseCFF(data); err == nil {
			if gid, ok := cff.GetGlyphFromName(name); ok {
				return gid, true
			}
		}
	}

	// 3. Try parsing as straight glyph index ("123")
	var gid uint64
	if n, err := fmt.Sscanf(name, "%d", &gid); err == nil && n == 1 {
		return GlyphID(gid), true
	}

	// 4. Try gidDDD syntax ("gid123")
	if len(name) > 3 && name[:3] == "gid" {
		if n, err := fmt.Sscanf(name[3:], "%d", &gid); err == nil && n == 1 {
			return GlyphID(gid), true
		}
	}

	// 5. Try uniUUUU syntax with cmap lookup ("uni0622")
	if len(name) > 3 && name[:3] == "uni" {
		var unichar uint32
		if n, err := fmt.Sscanf(name[3:], "%x", &unichar); err == nil && n == 1 {
			// Lookup in cmap table
			if data, err := f.TableData(TagCmap); err == nil {
				if cmap, err := ParseCmap(data); err == nil {
					if gid, ok := cmap.Lookup(Codepoint(unichar)); ok {
						return gid, true
					}
				}
			}
		}
	}

	return 0, false
}

// GlyphID represents a glyph index.
type GlyphID = uint16

// Codepoint represents a Unicode codepoint.
type Codepoint = uint32
