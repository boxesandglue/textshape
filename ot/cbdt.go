package ot

import "encoding/binary"

// HarfBuzz equivalent: OT/Color/CBDT/CBDT.hh
// Implements the CBLC (location) + CBDT (data) table pair for color
// bitmap glyphs.
//
// CBLC Spec: https://docs.microsoft.com/en-us/typography/opentype/spec/cblc
// CBDT Spec: https://docs.microsoft.com/en-us/typography/opentype/spec/cbdt
//
// CBLC contains the index structures that map (gid, ppem) → (offset,
// length) in the CBDT data table. We parse CBLC fully at construction
// time; CBDT is held as a raw byte slice and addressed via offsets from
// CBLC. This mirrors HB's CBDT::accelerator_t (CBDT.hh:826-979).
//
// Supported IndexSubtable formats: 1 and 3 (the only ones HB's subsetter
// supports; sufficient for both NotoColorEmoji and AppleColorEmoji).
// Supported image formats: 17 (small metrics + PNG), 18 (big metrics +
// PNG), 19 (no metrics + PNG).

// TagCBLC is the OpenType tag for the CBLC table (color bitmap location).
var TagCBLC = MakeTag('C', 'B', 'L', 'C')

// TagCBDT is the OpenType tag for the CBDT table (color bitmap data).
var TagCBDT = MakeTag('C', 'B', 'D', 'T')

// CBDTGlyph carries one PNG bitmap from a CBDT strike, with associated
// small-metric data and a record of which strike PPEM it came from.
type CBDTGlyph struct {
	PNG     []byte
	PPEM    int  // ppem of the strike the bitmap was drawn from
	Width   int  // pixel width (from glyph metrics)
	Height  int  // pixel height (from glyph metrics)
	XOffset int8 // bearingX in pixels (image's left edge from glyph origin)
	YOffset int8 // bearingY in pixels (image's top edge from baseline)
}

// indexSubtable records one [firstGlyph, lastGlyph] sub-range within a
// strike, pointing at the per-glyph offset table.
//
// HarfBuzz equivalent: struct IndexSubtableRecord (CBDT.hh:366-531)
// combined with the inline IndexSubtableHeader+Format1/3 it points at
// (CBDT.hh:142-364).
type indexSubtable struct {
	// offsetArray holds glyph_count+1 offsets. For Format 1 these are
	// uint32; for Format 3 they are uint16. Both are widened to uint32
	// here for uniform downstream handling.
	offsetArray     []uint32
	firstGlyphIndex GlyphID
	lastGlyphIndex  GlyphID
	indexFormat     uint16 // 1 or 3
	imageFormat     uint16 // 17, 18 or 19
	imageDataOffset uint32 // offset into CBDT (per-strike base)
}

// cblcStrike is one BitmapSizeTable's worth of metadata plus its parsed
// IndexSubtable list.
//
// HarfBuzz equivalent: struct BitmapSizeTable (CBDT.hh:631-704).
type cblcStrike struct {
	subtables []indexSubtable
	PPEMX     uint8
	PPEMY     uint8
}

// CBLC holds a parsed CBLC table. Use it together with a CBDT byte slice
// to fetch per-glyph PNG bitmaps.
//
// HarfBuzz equivalent: struct CBLC (CBDT.hh:734-820).
type CBLC struct {
	strikes []cblcStrike
	Version uint32 // packed FixedVersion: high 16 bits major, low 16 bits minor
}

// ParseCBLC parses the CBLC table. The CBDT table is not needed here —
// it is supplied to GlyphPNG below.
//
// HarfBuzz equivalent: CBLC::sanitize (CBDT.hh:740-748) combined with
// the materialization that accelerator_t::accelerator_t triggers
// indirectly via choose_strike/find_table calls.
//
// On-disk header (8 bytes, DEFINE_SIZE_ARRAY(8, sizeTables) at CBDT.hh:819):
//
//	uint16 version.major   (= 2 or 3)
//	uint16 version.minor
//	uint32 numSizes
//	BitmapSizeTable sizeTables[numSizes]   (each 48 bytes, inline)
func ParseCBLC(data []byte) (*CBLC, error) {
	if len(data) < 8 {
		return nil, ErrInvalidTable
	}
	c := &CBLC{
		Version: binary.BigEndian.Uint32(data[0:]),
	}
	numSizes := binary.BigEndian.Uint32(data[4:])
	if 8+int(numSizes)*48 > len(data) {
		return nil, ErrInvalidTable
	}
	c.strikes = make([]cblcStrike, numSizes)
	for i := range int(numSizes) {
		base := 8 + i*48
		// BitmapSizeTable layout (CBDT.hh:688-703):
		//   uint32 indexSubtableArrayOffset (from CBLC table start)
		//   uint32 indexTablesSize
		//   uint32 numberOfIndexSubtables
		//   uint32 colorRef
		//   SBitLineMetrics horizontal (12 bytes)
		//   SBitLineMetrics vertical   (12 bytes)
		//   uint16 startGlyphIndex
		//   uint16 endGlyphIndex
		//   uint8  ppemX
		//   uint8  ppemY
		//   uint8  bitDepth
		//   int8   flags
		idxArrOff := binary.BigEndian.Uint32(data[base+0:])
		numSubtables := binary.BigEndian.Uint32(data[base+8:])
		ppemX := data[base+44]
		ppemY := data[base+45]

		strike := cblcStrike{
			PPEMX: ppemX,
			PPEMY: ppemY,
		}

		// IndexSubtableArray = UnsizedArrayOf<IndexSubtableRecord>.
		// Each IndexSubtableRecord is 8 bytes (CBDT.hh:530):
		//   uint16 firstGlyphIndex
		//   uint16 lastGlyphIndex
		//   uint32 offsetToSubtable  (from start of IndexSubtableArray)
		arrEnd := int(idxArrOff) + int(numSubtables)*8
		if arrEnd > len(data) {
			return nil, ErrInvalidTable
		}
		strike.subtables = make([]indexSubtable, numSubtables)
		for j := range int(numSubtables) {
			recBase := int(idxArrOff) + j*8
			first := binary.BigEndian.Uint16(data[recBase+0:])
			last := binary.BigEndian.Uint16(data[recBase+2:])
			subOff := binary.BigEndian.Uint32(data[recBase+4:])
			// subOff is from the start of IndexSubtableArray, not CBLC.
			subAbs := int(idxArrOff) + int(subOff)
			if subAbs+8 > len(data) {
				return nil, ErrInvalidTable
			}
			// IndexSubtableHeader (CBDT.hh:142-154): uint16 indexFormat,
			// uint16 imageFormat, uint32 imageDataOffset (into CBDT).
			indexFormat := binary.BigEndian.Uint16(data[subAbs+0:])
			imageFormat := binary.BigEndian.Uint16(data[subAbs+2:])
			imageDataOff := binary.BigEndian.Uint32(data[subAbs+4:])

			sub := indexSubtable{
				firstGlyphIndex: first,
				lastGlyphIndex:  last,
				indexFormat:     indexFormat,
				imageFormat:     imageFormat,
				imageDataOffset: imageDataOff,
			}

			// glyph_count+1 entries. The offset width depends on
			// indexFormat: 32-bit for format 1, 16-bit for format 3.
			// HarfBuzz equivalent: IndexSubtableFormat1Or3<OffsetType>
			// at CBDT.hh:157-196.
			glyphCount := int(last) - int(first) + 1
			arrayStart := subAbs + 8
			switch indexFormat {
			case 1:
				if arrayStart+(glyphCount+1)*4 > len(data) {
					return nil, ErrInvalidTable
				}
				sub.offsetArray = make([]uint32, glyphCount+1)
				for k := range glyphCount + 1 {
					sub.offsetArray[k] = binary.BigEndian.Uint32(data[arrayStart+k*4:])
				}
			case 3:
				if arrayStart+(glyphCount+1)*2 > len(data) {
					return nil, ErrInvalidTable
				}
				sub.offsetArray = make([]uint32, glyphCount+1)
				for k := range glyphCount + 1 {
					sub.offsetArray[k] = uint32(binary.BigEndian.Uint16(data[arrayStart+k*2:]))
				}
			default:
				// Format 2, 4, 5: not supported here, matching HB's
				// subsetter scope (CBDT.hh:235-238). Skip the subtable
				// — glyphs in this range will return no bitmap.
				continue
			}
			strike.subtables[j] = sub
		}
		c.strikes[i] = strike
	}
	return c, nil
}

// HasData returns true if the table carries at least one strike.
//
// HarfBuzz equivalent: CBDT::accelerator_t::has_data (CBDT.hh:944) which
// inspects cbdt->version.major; we check via numSizes which is equivalent
// for our purposes (a valid CBLC with zero strikes is degenerate).
func (c *CBLC) HasData() bool {
	return c.Version != 0 && len(c.strikes) > 0
}

// chooseStrike returns the best matching strike for the given PPEM, or
// nil if no strikes exist.
//
// HarfBuzz equivalent: CBLC::choose_strike (CBDT.hh:789-813). Same
// preference order as sbix: smallest strike at least as large as the
// requested PPEM, falling back to the largest available if all are
// smaller.
func (c *CBLC) chooseStrike(requestedPPEM int) *cblcStrike {
	if len(c.strikes) == 0 {
		return nil
	}
	if requestedPPEM <= 0 {
		requestedPPEM = 1 << 30
	}
	bestI := 0
	maxPPEM := func(s *cblcStrike) int {
		if s.PPEMX > s.PPEMY {
			return int(s.PPEMX)
		}
		return int(s.PPEMY)
	}
	bestPPEM := maxPPEM(&c.strikes[0])
	for i := 1; i < len(c.strikes); i++ {
		ppem := maxPPEM(&c.strikes[i])
		if (requestedPPEM <= ppem && ppem < bestPPEM) ||
			(requestedPPEM > bestPPEM && ppem > bestPPEM) {
			bestI = i
			bestPPEM = ppem
		}
	}
	return &c.strikes[bestI]
}

// findSubtable locates the IndexSubtable that covers gid, or nil if not
// present in this strike.
//
// HarfBuzz equivalent: IndexSubtableArray::find_table (CBDT.hh:615-625).
// Linear search; the subtable count is typically tiny (~1-3 per strike).
func (st *cblcStrike) findSubtable(gid GlyphID) *indexSubtable {
	for i := range st.subtables {
		s := &st.subtables[i]
		if s.firstGlyphIndex <= gid && gid <= s.lastGlyphIndex && s.offsetArray != nil {
			return s
		}
	}
	return nil
}

// GlyphPNG returns the color bitmap for the given glyph at the strike
// best matching requestedPPEM, sourced from the supplied CBDT byte
// slice. Returns nil if no matching bitmap exists or the image format is
// unsupported.
//
// HarfBuzz equivalent: CBDT::accelerator_t::reference_png
// (CBDT.hh:894-942). HB returns a heap-managed blob; we return a Go
// slice that is a view into cbdtData.
func (c *CBLC) GlyphPNG(gid GlyphID, requestedPPEM int, cbdtData []byte) *CBDTGlyph {
	strike := c.chooseStrike(requestedPPEM)
	if strike == nil {
		return nil
	}
	sub := strike.findSubtable(gid)
	if sub == nil {
		return nil
	}
	idx := int(gid) - int(sub.firstGlyphIndex)
	if idx+1 >= len(sub.offsetArray) {
		return nil
	}
	startOff := sub.offsetArray[idx]
	endOff := sub.offsetArray[idx+1]
	// HB: offsetArrayZ[idx + 1] <= offsetArrayZ[idx] → gap glyph.
	if endOff <= startOff {
		return nil
	}
	absOff := int(sub.imageDataOffset) + int(startOff)
	imgLen := int(endOff - startOff)
	if absOff+imgLen > len(cbdtData) {
		return nil
	}

	// Image-format dispatch. CBDT.hh:911-941.
	out := &CBDTGlyph{
		PPEM: int(strike.PPEMY),
	}
	switch sub.imageFormat {
	case 17:
		// GlyphBitmapDataFormat17 (CBDT.hh:711-717):
		//   SmallGlyphMetrics (5 bytes)
		//   Array32Of<HBUINT8> data (uint32 length + bytes)
		// min_size = 9.
		if imgLen < 9 {
			return nil
		}
		// SmallGlyphMetrics (CBDT.hh:76-101):
		//   uint8 height, uint8 width, int8 BearingX, int8 BearingY, uint8 Advance
		out.Height = int(cbdtData[absOff+0])
		out.Width = int(cbdtData[absOff+1])
		out.XOffset = int8(cbdtData[absOff+2])
		out.YOffset = int8(cbdtData[absOff+3])
		dataLen := int(binary.BigEndian.Uint32(cbdtData[absOff+5:]))
		dataStart := absOff + 9
		if dataStart+dataLen > absOff+imgLen {
			return nil
		}
		out.PNG = cbdtData[dataStart : dataStart+dataLen]
	case 18:
		// GlyphBitmapDataFormat18 (CBDT.hh:719-725):
		//   BigGlyphMetrics (8 bytes)
		//   Array32Of<HBUINT8> data
		// min_size = 12.
		if imgLen < 12 {
			return nil
		}
		// BigGlyphMetrics (CBDT.hh:104-110): SmallGlyphMetrics + 3 more
		// fields. Only the first 4 fields (height, width, hBearingX,
		// hBearingY) are relevant for image extents.
		out.Height = int(cbdtData[absOff+0])
		out.Width = int(cbdtData[absOff+1])
		out.XOffset = int8(cbdtData[absOff+2])
		out.YOffset = int8(cbdtData[absOff+3])
		dataLen := int(binary.BigEndian.Uint32(cbdtData[absOff+8:]))
		dataStart := absOff + 12
		if dataStart+dataLen > absOff+imgLen {
			return nil
		}
		out.PNG = cbdtData[dataStart : dataStart+dataLen]
	case 19:
		// GlyphBitmapDataFormat19 (CBDT.hh:727-732): only
		// Array32Of<HBUINT8> data. min_size = 4. Image metrics must
		// come from the IndexSubtable's metrics block — Format 2 / 5
		// embed them there, but we only support index formats 1 + 3,
		// which carry no metrics. Format 19 with no metrics is
		// useless to us; HB also returns the blob but the caller
		// would lack extents. Return PNG only; caller can decode
		// dimensions from the PNG IHDR if needed.
		dataLen := int(binary.BigEndian.Uint32(cbdtData[absOff:]))
		dataStart := absOff + 4
		if dataStart+dataLen > absOff+imgLen {
			return nil
		}
		out.PNG = cbdtData[dataStart : dataStart+dataLen]
	default:
		return nil
	}
	return out
}
