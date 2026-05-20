package ot

import (
	"encoding/binary"
	"fmt"
)

// Font represents an OpenType font.
type Font struct {
	tables map[Tag]tableRecord
	data   []byte
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
//
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

// HasColorPalettes returns true if the font carries a CPAL table with at
// least one palette.
//
// HarfBuzz equivalent: hb_ot_color_has_palettes (hb-ot-color.cc:68-83)
// delegating to CPAL::has_data (OT/Color/CPAL/CPAL.hh:173).
func (f *Font) HasColorPalettes() bool {
	data, err := f.TableData(TagCPAL)
	if err != nil {
		return false
	}
	cpal, err := ParseCPAL(data)
	if err != nil {
		return false
	}
	return cpal.HasData()
}

// NumColorPalettes returns the number of color palettes in the font, or 0
// if no CPAL table is present.
//
// HarfBuzz equivalent: hb_ot_color_palette_get_count (hb-ot-color.cc:84-103)
// delegating to CPAL::get_palette_count (OT/Color/CPAL/CPAL.hh:178).
func (f *Font) NumColorPalettes() int {
	data, err := f.TableData(TagCPAL)
	if err != nil {
		return 0
	}
	cpal, err := ParseCPAL(data)
	if err != nil {
		return 0
	}
	return cpal.NumPalettes()
}

// ColorPaletteFlags returns the flags for the given palette index.
// Returns PaletteFlagDefault if the CPAL table is missing, the index is
// out of range, or the table is CPAL version 0 (no flags array).
//
// HarfBuzz equivalent: hb_ot_color_palette_get_flags (hb-ot-color.cc:146-176)
// delegating to CPAL::get_palette_flags (OT/Color/CPAL/CPAL.hh:181-182).
func (f *Font) ColorPaletteFlags(paletteIndex int) PaletteFlags {
	data, err := f.TableData(TagCPAL)
	if err != nil {
		return PaletteFlagDefault
	}
	cpal, err := ParseCPAL(data)
	if err != nil {
		return PaletteFlagDefault
	}
	return cpal.PaletteFlags(paletteIndex)
}

// ColorPaletteColors returns the colors of the given palette, or nil if
// the palette index is out of range or no CPAL table is present.
//
// HarfBuzz equivalent: hb_ot_color_palette_get_colors (hb-ot-color.cc:178-200)
// delegating to CPAL::get_palette_colors (OT/Color/CPAL/CPAL.hh:190-197).
// HB's variant paginates via start_offset+count for C-API memory
// management; we return a slice because Go callers do not need that.
func (f *Font) ColorPaletteColors(paletteIndex int) []BGRAColor {
	data, err := f.TableData(TagCPAL)
	if err != nil {
		return nil
	}
	cpal, err := ParseCPAL(data)
	if err != nil {
		return nil
	}
	return cpal.PaletteColors(paletteIndex)
}

// HasColorLayers returns true if the font has a COLR table that contains
// at least one COLRv0 base-glyph record. Returns false for fonts that only
// carry COLRv1 paint trees — see GlyphColorLayers commentary for why.
//
// HarfBuzz equivalent: hb_ot_color_has_layers (hb-ot-color.cc:204-218)
// delegating to COLR::has_v0_data (OT/Color/COLR/COLR.hh:2095). Note that
// HB returns true ONLY for v0 data here — separate accessors
// (hb_ot_color_has_paint at hb-ot-color.cc:222) report v1 presence.
func (f *Font) HasColorLayers() bool {
	data, err := f.TableData(TagCOLR)
	if err != nil {
		return false
	}
	colr, err := ParseCOLR(data)
	if err != nil {
		return false
	}
	return colr.HasV0Data()
}

// GlyphColorLayers returns the COLRv0 layer list for the given base glyph,
// or nil if the glyph has no v0 record (or no COLR table exists at all).
// Callers that get nil should fall back to the plain outline.
//
// HarfBuzz equivalent: hb_ot_color_glyph_get_layers
// (hb-ot-color.cc:262-270) delegating to COLR::get_glyph_layers
// (OT/Color/COLR/COLR.hh:2105-2122). HB paginates output via
// start_offset+count for C-API memory management; we return a slice.
//
// Each returned ColorLayer carries a CPAL color index that may be
// ForegroundColorIndex (0xFFFF) — see ColorLayer's documentation for the
// "use foreground color" sentinel.
func (f *Font) GlyphColorLayers(gid GlyphID) []ColorLayer {
	data, err := f.TableData(TagCOLR)
	if err != nil {
		return nil
	}
	colr, err := ParseCOLR(data)
	if err != nil {
		return nil
	}
	return colr.GlyphLayers(gid)
}

// ColorPNG is the PNG payload of a color-bitmap glyph as returned by
// GlyphColorPNG. The fields combine HB's sbix and CBDT result shapes
// into one type: callers do not need to know which table the bitmap
// came from. Width/Height are in pixels; XOffset/YOffset are the
// per-glyph bearing carried in the source table (sbix: font units of
// 1/UnitsPerEm; CBDT: pixels).
//
// HarfBuzz equivalent: the combined outputs of
// sbix::accelerator_t::reference_png (sbix.hh:221-231) and
// CBDT::accelerator_t::reference_png (CBDT.hh:894-942), unified.
type ColorPNG struct {
	PNG     []byte
	PPEM    int
	Width   int
	Height  int
	XOffset int
	YOffset int
	Source  Tag // TagSbix or TagCBDT — useful for offset-unit interpretation
}

// HasColorPNG returns true if the font carries color bitmap data in
// either an sbix or a CBDT/CBLC table.
//
// HarfBuzz equivalent: hb_ot_color_has_png (hb-ot-color.cc:327-331)
// which is true if face->table.CBDT->has_data() OR
// face->table.sbix->has_data().
func (f *Font) HasColorPNG() bool {
	if f.HasTable(TagSbix) {
		data, err := f.TableData(TagSbix)
		if err == nil {
			sbix, err := ParseSbix(data, f.NumGlyphs())
			if err == nil && sbix.HasData() {
				return true
			}
		}
	}
	if f.HasTable(TagCBLC) && f.HasTable(TagCBDT) {
		data, err := f.TableData(TagCBLC)
		if err == nil {
			cblc, err := ParseCBLC(data)
			if err == nil && cblc.HasData() {
				return true
			}
		}
	}
	return false
}

// GlyphColorPNG returns the color bitmap for the given glyph at the
// strike best matching requestedPPEM, trying sbix first and then CBDT.
// Returns nil if no bitmap is available for this glyph.
//
// HarfBuzz equivalent: hb_ot_color_glyph_reference_png
// (hb-ot-color.cc:348-360). HB takes hb_font_t because PPEM is part of
// the font instance; in textshape font size is not bound to ot.Font,
// so the caller passes PPEM explicitly. requestedPPEM == 0 means
// "give me the largest strike" — mirrors HB's behavior when PPEM is
// unset (hb-ot-color.cc:339 documents this).
func (f *Font) GlyphColorPNG(gid GlyphID, requestedPPEM int) *ColorPNG {
	// Try sbix first — matches HB ordering at hb-ot-color.cc:353-354.
	if f.HasTable(TagSbix) {
		data, err := f.TableData(TagSbix)
		if err == nil {
			sbix, err := ParseSbix(data, f.NumGlyphs())
			if err == nil {
				if g := sbix.GlyphBlob(gid, requestedPPEM); g != nil && g.GraphicType == MakeTag('p', 'n', 'g', ' ') {
					// sbix offsets are int16 font units, not pixels.
					strike := sbix.chooseStrike(requestedPPEM)
					ppem := 0
					if strike != nil {
						ppem = int(strike.PPEM)
					}
					return &ColorPNG{
						PNG:     g.Data,
						PPEM:    ppem,
						XOffset: int(g.XOffset),
						YOffset: int(g.YOffset),
						Source:  TagSbix,
					}
				}
			}
		}
	}
	// Then try CBDT.
	if f.HasTable(TagCBLC) && f.HasTable(TagCBDT) {
		cblcData, err := f.TableData(TagCBLC)
		if err == nil {
			cblc, err := ParseCBLC(cblcData)
			if err == nil {
				cbdtData, err := f.TableData(TagCBDT)
				if err == nil {
					if g := cblc.GlyphPNG(gid, requestedPPEM, cbdtData); g != nil {
						return &ColorPNG{
							PNG:     g.PNG,
							PPEM:    g.PPEM,
							Width:   g.Width,
							Height:  g.Height,
							XOffset: int(g.XOffset),
							YOffset: int(g.YOffset),
							Source:  TagCBDT,
						}
					}
				}
			}
		}
	}
	return nil
}

// GlyphID represents a glyph index.
type GlyphID = uint16

// Codepoint represents a Unicode codepoint.
type Codepoint = uint32
