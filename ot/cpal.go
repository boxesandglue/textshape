package ot

import "encoding/binary"

// HarfBuzz equivalent: OT/Color/CPAL/CPAL.hh
// Implements the CPAL (Color Palette) table parser.
//
// CPAL Spec: https://docs.microsoft.com/en-us/typography/opentype/spec/cpal
//
// The CPAL table provides palettes of colors that COLR-table layers reference
// by index. A font may carry multiple palettes (e.g. light/dark variants); each
// palette contains the same number of colors (numColors).

// TagCPAL is the OpenType tag for the CPAL table.
var TagCPAL = MakeTag('C', 'P', 'A', 'L')

// BGRAColor is one CPAL color record.
//
// HarfBuzz equivalent: typedef HBUINT32 BGRAColor (OT/Color/CPAL/CPAL.hh:167)
//
// On disk a CPAL color record is four bytes in the order Blue, Green, Red,
// Alpha. HarfBuzz exposes it as a packed HBUINT32; we expose the channels
// individually because Go has no convenient packed-byte literal.
type BGRAColor struct {
	Blue  uint8
	Green uint8
	Red   uint8
	Alpha uint8
}

// PaletteFlags mirrors hb_ot_color_palette_flags_t (hb-ot-color.h:73-77).
type PaletteFlags uint32

const (
	// PaletteFlagDefault matches HB_OT_COLOR_PALETTE_FLAG_DEFAULT.
	PaletteFlagDefault PaletteFlags = 0
	// PaletteFlagUsableWithLightBackground matches HB_OT_COLOR_PALETTE_FLAG_USABLE_WITH_LIGHT_BACKGROUND.
	PaletteFlagUsableWithLightBackground PaletteFlags = 0x00000001
	// PaletteFlagUsableWithDarkBackground matches HB_OT_COLOR_PALETTE_FLAG_USABLE_WITH_DARK_BACKGROUND.
	PaletteFlagUsableWithDarkBackground PaletteFlags = 0x00000002
)

// CPAL holds a parsed CPAL table.
//
// HarfBuzz equivalent: struct CPAL (OT/Color/CPAL/CPAL.hh:169-362) and its
// CPALV1Tail (OT/Color/CPAL/CPAL.hh:45-165) when version == 1.
//
// Fields mirror the on-disk layout 1:1:
//   - version, numColors, numPalettes, numColorRecords from the fixed header
//   - colorRecordIndices is the inline uint16 array immediately after the
//     header, length numPalettes
//   - colorRecords is the BGRAColor array reached via colorRecordsZ offset,
//     length numColorRecords (NOT numColors — see get_palette_colors below)
//   - paletteFlags is optional CPAL v1 data, nil if version == 0 or the
//     offset is 0 (v1 spec allows omitting the array even when version == 1)
type CPAL struct {
	colorRecordIndices []uint16
	colorRecords       []BGRAColor
	paletteFlags       []PaletteFlags

	Version         uint16
	numColors       uint16
	numPalettes     uint16
	numColorRecords uint16
}

// ParseCPAL parses a CPAL table.
//
// HarfBuzz equivalent: CPAL::sanitize (OT/Color/CPAL/CPAL.hh:336-344) plus the
// CPALV1Tail::sanitize at OT/Color/CPAL/CPAL.hh:136-146. Sanitize in HB
// validates structure; we parse and validate in one pass.
func ParseCPAL(data []byte) (*CPAL, error) {
	if len(data) < 12 {
		return nil, ErrInvalidTable
	}

	c := &CPAL{
		Version:         binary.BigEndian.Uint16(data[0:]),
		numColors:       binary.BigEndian.Uint16(data[2:]),
		numPalettes:     binary.BigEndian.Uint16(data[4:]),
		numColorRecords: binary.BigEndian.Uint16(data[6:]),
	}
	colorRecordsOff := binary.BigEndian.Uint32(data[8:])

	// colorRecordIndicesZ is the inline uint16 array immediately after the
	// 12-byte header. Length is numPalettes.
	// HarfBuzz equivalent: UnsizedArrayOf<HBUINT16> colorRecordIndicesZ
	// (OT/Color/CPAL/CPAL.hh:356-358) accessed via colorRecordIndicesZ[i]
	// at OT/Color/CPAL/CPAL.hh:194.
	indicesEnd := 12 + int(c.numPalettes)*2
	if indicesEnd > len(data) {
		return nil, ErrInvalidTable
	}
	c.colorRecordIndices = make([]uint16, c.numPalettes)
	for i := uint16(0); i < c.numPalettes; i++ {
		c.colorRecordIndices[i] = binary.BigEndian.Uint16(data[12+int(i)*2:])
	}

	// colorRecords is the BGRAColor array at colorRecordsOff with
	// numColorRecords entries (each 4 bytes).
	// HarfBuzz equivalent: (this+colorRecordsZ).arrayZ as_array(numColorRecords)
	// at OT/Color/CPAL/CPAL.hh:195.
	recordsEnd := int(colorRecordsOff) + int(c.numColorRecords)*4
	if recordsEnd > len(data) {
		return nil, ErrInvalidTable
	}
	c.colorRecords = make([]BGRAColor, c.numColorRecords)
	for i := uint16(0); i < c.numColorRecords; i++ {
		base := int(colorRecordsOff) + int(i)*4
		c.colorRecords[i] = BGRAColor{
			Blue:  data[base],
			Green: data[base+1],
			Red:   data[base+2],
			Alpha: data[base+3],
		}
	}

	// Optional CPAL v1 tail: paletteFlagsZ, paletteLabelsZ, colorLabelsZ —
	// each a uint32 offset that may be 0 (meaning "not provided"). We only
	// read paletteFlagsZ because that is what hb_ot_color_palette_get_flags
	// exposes; palette labels and color labels are nameID indirections that
	// are not needed for rendering.
	//
	// HarfBuzz equivalent: CPALV1Tail::get_palette_flags
	// (OT/Color/CPAL/CPAL.hh:50-57). The tail sits immediately after the
	// colorRecordIndicesZ array.
	if c.Version >= 1 {
		tailOff := indicesEnd
		if tailOff+4 <= len(data) {
			paletteFlagsOff := binary.BigEndian.Uint32(data[tailOff:])
			if paletteFlagsOff != 0 {
				flagsEnd := int(paletteFlagsOff) + int(c.numPalettes)*4
				if flagsEnd <= len(data) {
					c.paletteFlags = make([]PaletteFlags, c.numPalettes)
					for i := uint16(0); i < c.numPalettes; i++ {
						c.paletteFlags[i] = PaletteFlags(binary.BigEndian.Uint32(data[int(paletteFlagsOff)+int(i)*4:]))
					}
				}
			}
		}
	}

	return c, nil
}

// HasData returns true if the table carries at least one palette.
//
// HarfBuzz equivalent: CPAL::has_data (OT/Color/CPAL/CPAL.hh:173).
func (c *CPAL) HasData() bool {
	return c.numPalettes > 0
}

// NumPalettes returns the number of palettes in the table.
//
// HarfBuzz equivalent: CPAL::get_palette_count (OT/Color/CPAL/CPAL.hh:178)
// and the public C wrapper hb_ot_color_palette_get_count (hb-ot-color.cc:85).
func (c *CPAL) NumPalettes() int {
	return int(c.numPalettes)
}

// NumColors returns the number of colors per palette. Every palette in a
// CPAL table has the same color count.
//
// HarfBuzz equivalent: CPAL::get_color_count (OT/Color/CPAL/CPAL.hh:179).
func (c *CPAL) NumColors() int {
	return int(c.numColors)
}

// PaletteColors returns the color slice for the given palette index, or nil
// if the index is out of range.
//
// HarfBuzz equivalent: CPAL::get_palette_colors returning an
// hb_array_t<const BGRAColor> at OT/Color/CPAL/CPAL.hh:190-197. The C-API
// variant at lines 198-219 paginates via start_offset; we return a plain
// slice because Go callers do not need the C buffer dance.
//
// Note that the on-disk colorRecords array carries numColorRecords entries
// total, shared across palettes; each palette is the sub-array
// [colorRecordIndices[i] : colorRecordIndices[i] + numColors].
func (c *CPAL) PaletteColors(paletteIndex int) []BGRAColor {
	if paletteIndex < 0 || paletteIndex >= int(c.numPalettes) {
		return nil
	}
	start := int(c.colorRecordIndices[paletteIndex])
	end := start + int(c.numColors)
	if end > len(c.colorRecords) {
		return nil
	}
	return c.colorRecords[start:end]
}

// PaletteFlags returns the flags for the given palette, or
// PaletteFlagDefault if version == 0 or no paletteFlags array is present.
//
// HarfBuzz equivalent: CPAL::get_palette_flags (OT/Color/CPAL/CPAL.hh:181-182)
// delegating to CPALV1Tail::get_palette_flags
// (OT/Color/CPAL/CPAL.hh:50-57). HB returns HB_OT_COLOR_PALETTE_FLAG_DEFAULT
// when paletteFlagsZ is null; we mirror that.
func (c *CPAL) PaletteFlags(paletteIndex int) PaletteFlags {
	if paletteIndex < 0 || paletteIndex >= int(c.numPalettes) {
		return PaletteFlagDefault
	}
	if c.paletteFlags == nil {
		return PaletteFlagDefault
	}
	return c.paletteFlags[paletteIndex]
}
