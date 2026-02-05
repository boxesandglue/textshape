package ot

import (
	"encoding/binary"
	"sort"
)

// VORG represents the Vertical Origin table (used by CFF/CFF2 fonts).
// It provides the vertical origin Y coordinate for glyphs.
type VORG struct {
	defaultVertOriginY int16
	entries            []vorgEntry // sorted by glyphIndex
}

type vorgEntry struct {
	glyphIndex  GlyphID
	vertOriginY int16
}

// ParseVORG parses the VORG table.
func ParseVORG(data []byte) (*VORG, error) {
	if len(data) < 6 {
		return nil, ErrInvalidTable
	}

	// majorVersion := binary.BigEndian.Uint16(data[0:])
	// minorVersion := binary.BigEndian.Uint16(data[2:])
	defaultY := int16(binary.BigEndian.Uint16(data[4:]))
	numRecords := binary.BigEndian.Uint16(data[6:])

	if len(data) < 8+int(numRecords)*4 {
		return nil, ErrInvalidTable
	}

	v := &VORG{
		defaultVertOriginY: defaultY,
		entries:            make([]vorgEntry, numRecords),
	}

	off := 8
	for i := 0; i < int(numRecords); i++ {
		v.entries[i].glyphIndex = GlyphID(binary.BigEndian.Uint16(data[off:]))
		v.entries[i].vertOriginY = int16(binary.BigEndian.Uint16(data[off+2:]))
		off += 4
	}

	return v, nil
}

// GetVertOriginY returns the vertical origin Y for a glyph.
// Uses binary search on the sorted entries, falls back to defaultVertOriginY.
func (v *VORG) GetVertOriginY(glyph GlyphID) int16 {
	idx := sort.Search(len(v.entries), func(i int) bool {
		return v.entries[i].glyphIndex >= glyph
	})
	if idx < len(v.entries) && v.entries[idx].glyphIndex == glyph {
		return v.entries[idx].vertOriginY
	}
	return v.defaultVertOriginY
}
