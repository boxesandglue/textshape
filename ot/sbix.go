package ot

import "encoding/binary"

// HarfBuzz equivalent: OT/Color/sbix/sbix.hh
// Implements the sbix (Standard Bitmap Graphics) table parser.
//
// sbix Spec: https://docs.microsoft.com/en-us/typography/opentype/spec/sbix
// Apple TrueType Reference Manual:
// https://developer.apple.com/fonts/TrueType-Reference-Manual/RM06/Chap6sbix.html
//
// An sbix table carries one or more "strikes", each containing a PNG (or
// JPEG/TIFF) image per glyph at a fixed PPEM size. The renderer picks the
// best-matching strike for the current font size, scales the bitmap to
// match, and draws it at the glyph origin offset by (xOffset, yOffset).

// TagSbix is the OpenType tag for the sbix table.
var TagSbix = MakeTag('s', 'b', 'i', 'x')

// SbixStrike describes one resolution-specific bitmap strike inside an
// sbix table. Each strike has a fixed PPEM (pixels-per-em) and contains
// at most one glyph blob per glyph in the font.
//
// HarfBuzz equivalent: struct SBIXStrike (OT/Color/sbix/sbix.hh:80-191).
type SbixStrike struct {
	// imageOffsets has numGlyphs+1 entries; entry i+1 minus entry i is
	// the byte length of glyph i's SBIXGlyph record (including the
	// 8-byte glyph header). A zero-length range means "glyph absent
	// from this strike".
	// HarfBuzz equivalent: UnsizedArrayOf<Offset32To<SBIXGlyph>>
	// imageOffsetsZ (OT/Color/sbix/sbix.hh:186-188).
	imageOffsets []uint32

	// rawData is the strike's bytes starting at the strike header,
	// indexed by imageOffsets. We keep a reference rather than slicing
	// to avoid copying; sbix tables can be very large (multi-MB).
	rawData []byte

	PPEM       uint16
	Resolution uint16
}

// SbixGlyph represents one glyph blob inside a strike.
//
// HarfBuzz equivalent: struct SBIXGlyph (OT/Color/sbix/sbix.hh:45-78). The
// GraphicType tag distinguishes 'png ' (most common), 'jpg ', 'tiff', or
// 'dupe' (an indirection: see Sbix.GlyphBlob for 'dupe' resolution).
type SbixGlyph struct {
	Data        []byte
	GraphicType Tag
	XOffset     int16
	YOffset     int16
}

// Sbix holds a parsed sbix table.
//
// HarfBuzz equivalent: struct sbix (OT/Color/sbix/sbix.hh:193-440).
//
// On-disk header (8 bytes, DEFINE_SIZE_ARRAY(8, strikes) at sbix.hh:439):
//
//	uint16  version  (= 1)
//	uint16  flags    (bit 0 = 1, bit 1 = "draw outlines as well")
//	uint32  numStrikes
//	uint32  strikeOffsets[numStrikes]  (offsets from the start of sbix)
type Sbix struct {
	Strikes []SbixStrike
	Version uint16
	Flags   uint16
}

// ParseSbix parses an sbix table. numGlyphs is the font-wide glyph count
// from the maxp table, needed to size each strike's imageOffsets array.
//
// HarfBuzz equivalent: sbix::sanitize (OT/Color/sbix/sbix.hh:368-375) and
// SBIXStrike::sanitize (OT/Color/sbix/sbix.hh:85-90); HB sanitizes
// shallowly here and reads the actual offsets lazily via get_glyph_blob.
// We materialize the strike-level offset arrays at parse time but leave
// the SBIXGlyph payloads where they are (slice views into rawData).
func ParseSbix(data []byte, numGlyphs int) (*Sbix, error) {
	if len(data) < 8 || numGlyphs < 0 {
		return nil, ErrInvalidTable
	}
	s := &Sbix{
		Version: binary.BigEndian.Uint16(data[0:]),
		Flags:   binary.BigEndian.Uint16(data[2:]),
	}
	numStrikes := binary.BigEndian.Uint32(data[4:])
	if 8+int(numStrikes)*4 > len(data) {
		return nil, ErrInvalidTable
	}
	s.Strikes = make([]SbixStrike, numStrikes)
	for i := range int(numStrikes) {
		strikeOff := binary.BigEndian.Uint32(data[8+i*4:])
		if int(strikeOff)+4 > len(data) {
			return nil, ErrInvalidTable
		}
		strikeData := data[strikeOff:]
		st := SbixStrike{
			PPEM:       binary.BigEndian.Uint16(strikeData[0:]),
			Resolution: binary.BigEndian.Uint16(strikeData[2:]),
			rawData:    strikeData,
		}
		// numGlyphs+1 entries (last entry is sentinel marking end of
		// the data array — HB explicitly checks
		// imageOffsetsZ[gid+1] > imageOffsetsZ[gid] at
		// OT/Color/sbix/sbix.hh:109).
		offEnd := 4 + (numGlyphs+1)*4
		if offEnd > len(strikeData) {
			return nil, ErrInvalidTable
		}
		st.imageOffsets = make([]uint32, numGlyphs+1)
		for g := range numGlyphs + 1 {
			st.imageOffsets[g] = binary.BigEndian.Uint32(strikeData[4+g*4:])
		}
		s.Strikes[i] = st
	}
	return s, nil
}

// HasData returns true if the table carries at least one strike.
//
// HarfBuzz equivalent: sbix::has_data (OT/Color/sbix/sbix.hh:197).
func (s *Sbix) HasData() bool {
	return s.Version > 0 && len(s.Strikes) > 0
}

// chooseStrike picks the best-matching strike for the requested PPEM.
//
// HarfBuzz equivalent: sbix::accelerator_t::choose_strike
// (OT/Color/sbix/sbix.hh:267-292). The selection prefers the smallest
// strike that is at least as large as requestedPPEM; if all strikes are
// smaller, it picks the largest. A requested PPEM of 0 means "give me
// the largest" (HB uses 1<<30 as the upper sentinel).
func (s *Sbix) chooseStrike(requestedPPEM int) *SbixStrike {
	if len(s.Strikes) == 0 {
		return nil
	}
	if requestedPPEM <= 0 {
		requestedPPEM = 1 << 30
	}
	bestI := 0
	bestPPEM := int(s.Strikes[0].PPEM)
	for i := 1; i < len(s.Strikes); i++ {
		ppem := int(s.Strikes[i].PPEM)
		if (requestedPPEM <= ppem && ppem < bestPPEM) ||
			(requestedPPEM > bestPPEM && ppem > bestPPEM) {
			bestI = i
			bestPPEM = ppem
		}
	}
	return &s.Strikes[bestI]
}

// GlyphBlob returns the bitmap blob for the given glyph at the strike
// best matching requestedPPEM. Returns nil if the glyph is absent from
// the chosen strike, the graphic type is unsupported, or a 'dupe' chain
// loops too deep.
//
// HarfBuzz equivalent: SBIXStrike::get_glyph_blob
// (OT/Color/sbix/sbix.hh:92-137). The 'dupe' redirect is implemented
// with the same 8-deep retry cap HB uses (sbix.hh:102).
func (s *Sbix) GlyphBlob(gid GlyphID, requestedPPEM int) *SbixGlyph {
	strike := s.chooseStrike(requestedPPEM)
	if strike == nil {
		return nil
	}
	return strike.glyphBlob(gid, 8)
}

// glyphBlob walks one strike's imageOffsets to find the glyph's data,
// resolving 'dupe' redirects up to retryLimit times.
//
// HarfBuzz equivalent: the inner loop of SBIXStrike::get_glyph_blob
// (OT/Color/sbix/sbix.hh:107-127).
func (st *SbixStrike) glyphBlob(gid GlyphID, retryLimit int) *SbixGlyph {
	for range retryLimit {
		if int(gid)+1 >= len(st.imageOffsets) {
			return nil
		}
		startOff := st.imageOffsets[gid]
		endOff := st.imageOffsets[gid+1]
		// HB: imageOffsetsZ[gid+1] <= imageOffsetsZ[gid] ||
		//     imageOffsetsZ[gid+1] - imageOffsetsZ[gid] <= SBIXGlyph::min_size
		// (OT/Color/sbix/sbix.hh:108-110). min_size = 8.
		if endOff <= startOff || endOff-startOff <= 8 {
			return nil
		}
		if int(endOff) > len(st.rawData) {
			return nil
		}
		// SBIXGlyph header (8 bytes): int16 xOffset, int16 yOffset,
		// Tag graphicType. Data follows at offset 8.
		glyphData := st.rawData[startOff:endOff]
		gt := Tag(binary.BigEndian.Uint32(glyphData[4:]))
		if gt == MakeTag('d', 'u', 'p', 'e') {
			// First two bytes of data are a GlyphID to redirect to.
			// HarfBuzz equivalent: sbix.hh:119-128.
			if len(glyphData) < 10 {
				return nil
			}
			gid = GlyphID(binary.BigEndian.Uint16(glyphData[8:]))
			continue
		}
		return &SbixGlyph{
			XOffset:     int16(binary.BigEndian.Uint16(glyphData[0:])),
			YOffset:     int16(binary.BigEndian.Uint16(glyphData[2:])),
			GraphicType: gt,
			Data:        glyphData[8:],
		}
	}
	return nil
}
