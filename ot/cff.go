package ot

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// TagCFF is the table tag for CFF data.
var TagCFF = MakeTag('C', 'F', 'F', ' ')

// CFF represents a parsed CFF (Compact Font Format) table.
type CFF struct {
	data   []byte
	header cffHeader

	Name        string
	TopDict     TopDict
	Strings     []string // Custom strings (SID 391+)
	GlobalSubrs [][]byte // Global subroutines
	CharStrings [][]byte // Per-glyph CharStrings
	PrivateDict PrivateDict
	LocalSubrs  [][]byte  // Local subroutines
	Charset     []GlyphID // Glyph ID to SID mapping

	// CID fonts
	IsCID    bool
	FDArray  []FontDict
	FDSelect []byte
}

type cffHeader struct {
	major   uint8
	minor   uint8
	hdrSize uint8
	offSize uint8
}

// TopDict contains top-level font dictionary data.
type TopDict struct {
	Version     int // SID
	Notice      int // SID
	FullName    int // SID
	FamilyName  int // SID
	Weight      int // SID
	FontBBox    [4]int
	CharStrings int    // Offset to CharStrings INDEX
	Private     [2]int // [size, offset]
	Charset     int    // Offset to Charset
	Encoding    int    // Offset to Encoding

	// CID fonts
	ROS      [3]int // Registry, Ordering, Supplement (SIDs)
	FDArray  int
	FDSelect int
	IsCID    bool
}

// PrivateDict contains private dictionary data.
type PrivateDict struct {
	BlueValues       []int
	OtherBlues       []int
	FamilyBlues      []int
	FamilyOtherBlues []int
	StdHW            int
	StdVW            int
	StemSnapH        []int // Stem snap widths (horizontal)
	StemSnapV        []int // Stem snap widths (vertical)
	Subrs            int   // Offset to Local Subrs (relative to Private DICT)
	DefaultWidthX    int
	NominalWidthX    int
	BlueScale        float64
	BlueShift        int
	BlueFuzz         int
}

// FontDict contains per-font dictionary data (for CID fonts).
type FontDict struct {
	Private [2]int // [size, offset]
}

// ParseCFF parses a CFF table from raw data.
func ParseCFF(data []byte) (*CFF, error) {
	if len(data) < 4 {
		return nil, errors.New("CFF: data too short")
	}

	cff := &CFF{data: data}

	// Parse header
	cff.header.major = data[0]
	cff.header.minor = data[1]
	cff.header.hdrSize = data[2]
	cff.header.offSize = data[3]

	if cff.header.major != 1 {
		return nil, fmt.Errorf("CFF: unsupported version %d.%d", cff.header.major, cff.header.minor)
	}

	offset := int(cff.header.hdrSize)

	// Parse Name INDEX
	names, consumed, err := parseINDEX(data[offset:])
	if err != nil {
		return nil, fmt.Errorf("CFF: parsing Name INDEX: %w", err)
	}
	if len(names) > 0 {
		cff.Name = string(names[0])
	}
	offset += consumed

	// Parse Top DICT INDEX
	topDicts, consumed, err := parseINDEX(data[offset:])
	if err != nil {
		return nil, fmt.Errorf("CFF: parsing Top DICT INDEX: %w", err)
	}
	if len(topDicts) == 0 {
		return nil, errors.New("CFF: no Top DICT found")
	}
	cff.TopDict, err = parseTopDict(topDicts[0])
	if err != nil {
		return nil, fmt.Errorf("CFF: parsing Top DICT: %w", err)
	}
	offset += consumed

	// Parse String INDEX
	strings, consumed, err := parseINDEX(data[offset:])
	if err != nil {
		return nil, fmt.Errorf("CFF: parsing String INDEX: %w", err)
	}
	cff.Strings = make([]string, len(strings))
	for i, s := range strings {
		cff.Strings[i] = string(s)
	}
	offset += consumed

	// Parse Global Subrs INDEX
	cff.GlobalSubrs, _, err = parseINDEX(data[offset:])
	if err != nil {
		return nil, fmt.Errorf("CFF: parsing Global Subrs INDEX: %w", err)
	}

	// Parse CharStrings INDEX (at offset specified in Top DICT)
	if cff.TopDict.CharStrings > 0 && cff.TopDict.CharStrings < len(data) {
		cff.CharStrings, _, err = parseINDEX(data[cff.TopDict.CharStrings:])
		if err != nil {
			return nil, fmt.Errorf("CFF: parsing CharStrings INDEX: %w", err)
		}
	}

	// Check if CID font
	cff.IsCID = cff.TopDict.IsCID
	cff.TopDict.IsCID = cff.IsCID

	// Parse Private DICT
	if cff.TopDict.Private[0] > 0 && cff.TopDict.Private[1] > 0 {
		privOffset := cff.TopDict.Private[1]
		privSize := cff.TopDict.Private[0]
		if privOffset+privSize <= len(data) {
			cff.PrivateDict, err = parsePrivateDict(data[privOffset : privOffset+privSize])
			if err != nil {
				return nil, fmt.Errorf("CFF: parsing Private DICT: %w", err)
			}

			// Parse Local Subrs (offset relative to Private DICT)
			if cff.PrivateDict.Subrs > 0 {
				localSubrsOffset := privOffset + cff.PrivateDict.Subrs
				if localSubrsOffset < len(data) {
					cff.LocalSubrs, _, err = parseINDEX(data[localSubrsOffset:])
					if err != nil {
						// Not fatal - some fonts don't have local subrs
						cff.LocalSubrs = nil
					}
				}
			}
		}
	}

	// Parse Charset
	if cff.TopDict.Charset > 0 && len(cff.CharStrings) > 0 {
		cff.Charset, err = parseCharset(data, cff.TopDict.Charset, len(cff.CharStrings))
		if err != nil {
			// Use default charset (sequential SIDs)
			cff.Charset = make([]GlyphID, len(cff.CharStrings))
			for i := range cff.Charset {
				cff.Charset[i] = GlyphID(i)
			}
		}
	}

	return cff, nil
}

// parseINDEX parses a CFF INDEX structure.
// Returns the data items and bytes consumed.
func parseINDEX(data []byte) ([][]byte, int, error) {
	if len(data) < 2 {
		return nil, 0, errors.New("INDEX: data too short")
	}

	count := int(binary.BigEndian.Uint16(data[0:2]))
	if count == 0 {
		return nil, 2, nil
	}

	if len(data) < 3 {
		return nil, 0, errors.New("INDEX: data too short for offSize")
	}
	offSize := int(data[2])
	if offSize < 1 || offSize > 4 {
		return nil, 0, fmt.Errorf("INDEX: invalid offSize %d", offSize)
	}

	// Calculate header size: 2 (count) + 1 (offSize) + (count+1)*offSize
	headerSize := 3 + (count+1)*offSize
	if len(data) < headerSize {
		return nil, 0, errors.New("INDEX: data too short for offsets")
	}

	// Read offsets
	offsets := make([]int, count+1)
	for i := 0; i <= count; i++ {
		off := 3 + i*offSize
		offsets[i] = readOffset(data[off:], offSize)
	}

	// Calculate total size
	dataStart := headerSize
	dataEnd := dataStart + offsets[count] - 1
	if dataEnd > len(data) {
		return nil, 0, errors.New("INDEX: data extends beyond buffer")
	}

	// Extract items
	items := make([][]byte, count)
	for i := 0; i < count; i++ {
		start := dataStart + offsets[i] - 1
		end := dataStart + offsets[i+1] - 1
		if start < 0 || end > len(data) || start > end {
			return nil, 0, fmt.Errorf("INDEX: invalid item bounds [%d:%d]", start, end)
		}
		items[i] = data[start:end]
	}

	return items, dataEnd, nil
}

func readOffset(data []byte, size int) int {
	switch size {
	case 1:
		return int(data[0])
	case 2:
		return int(binary.BigEndian.Uint16(data))
	case 3:
		return int(data[0])<<16 | int(data[1])<<8 | int(data[2])
	case 4:
		return int(binary.BigEndian.Uint32(data))
	}
	return 0
}

// parseTopDict parses a Top DICT.
func parseTopDict(data []byte) (TopDict, error) {
	dict := TopDict{
		Charset:  0, // Default charset
		Encoding: 0, // Default encoding
	}

	operands := make([]int, 0, 16)
	pos := 0

	for pos < len(data) {
		b := data[pos]

		// Operand
		if b >= 32 && b <= 254 || b == 28 || b == 29 || b == 30 {
			val, consumed := decodeDictOperand(data[pos:])
			operands = append(operands, val)
			pos += consumed
			continue
		}

		// Operator
		op := int(b)
		pos++
		if b == 12 && pos < len(data) {
			op = 12<<8 | int(data[pos])
			pos++
		}

		switch op {
		case dictVersion:
			if len(operands) > 0 {
				dict.Version = operands[len(operands)-1]
			}
		case dictNotice:
			if len(operands) > 0 {
				dict.Notice = operands[len(operands)-1]
			}
		case dictFullName:
			if len(operands) > 0 {
				dict.FullName = operands[len(operands)-1]
			}
		case dictFamilyName:
			if len(operands) > 0 {
				dict.FamilyName = operands[len(operands)-1]
			}
		case dictWeight:
			if len(operands) > 0 {
				dict.Weight = operands[len(operands)-1]
			}
		case dictFontBBox:
			if len(operands) >= 4 {
				copy(dict.FontBBox[:], operands[len(operands)-4:])
			}
		case dictCharset:
			if len(operands) > 0 {
				dict.Charset = operands[len(operands)-1]
			}
		case dictEncoding:
			if len(operands) > 0 {
				dict.Encoding = operands[len(operands)-1]
			}
		case dictCharStrings:
			if len(operands) > 0 {
				dict.CharStrings = operands[len(operands)-1]
			}
		case dictPrivate:
			if len(operands) >= 2 {
				dict.Private[0] = operands[len(operands)-2] // size
				dict.Private[1] = operands[len(operands)-1] // offset
			}
		case dictROS:
			if len(operands) >= 3 {
				dict.ROS[0] = operands[len(operands)-3]
				dict.ROS[1] = operands[len(operands)-2]
				dict.ROS[2] = operands[len(operands)-1]
				dict.IsCID = true
			}
		case dictFDArray:
			if len(operands) > 0 {
				dict.FDArray = operands[len(operands)-1]
			}
		case dictFDSelect:
			if len(operands) > 0 {
				dict.FDSelect = operands[len(operands)-1]
			}
		}

		operands = operands[:0]
	}

	return dict, nil
}

// parsePrivateDict parses a Private DICT.
func parsePrivateDict(data []byte) (PrivateDict, error) {
	dict := PrivateDict{
		BlueScale: 0.039625, // Default
		BlueShift: 7,        // Default
		BlueFuzz:  1,        // Default
	}

	operands := make([]int, 0, 16)
	pos := 0

	for pos < len(data) {
		b := data[pos]

		// Operand
		if b >= 32 && b <= 254 || b == 28 || b == 29 || b == 30 {
			val, consumed := decodeDictOperand(data[pos:])
			operands = append(operands, val)
			pos += consumed
			continue
		}

		// Operator
		op := int(b)
		pos++
		if b == 12 && pos < len(data) {
			op = 12<<8 | int(data[pos])
			pos++
		}

		switch op {
		case dictBlueValues:
			dict.BlueValues = append([]int{}, operands...)
		case dictOtherBlues:
			dict.OtherBlues = append([]int{}, operands...)
		case dictFamilyBlues:
			dict.FamilyBlues = append([]int{}, operands...)
		case dictFamilyOtherBlues:
			dict.FamilyOtherBlues = append([]int{}, operands...)
		case dictStdHW:
			if len(operands) > 0 {
				dict.StdHW = operands[len(operands)-1]
			}
		case dictStdVW:
			if len(operands) > 0 {
				dict.StdVW = operands[len(operands)-1]
			}
		case dictSubrs:
			if len(operands) > 0 {
				dict.Subrs = operands[len(operands)-1]
			}
		case dictDefaultWidthX:
			if len(operands) > 0 {
				dict.DefaultWidthX = operands[len(operands)-1]
			}
		case dictNominalWidthX:
			if len(operands) > 0 {
				dict.NominalWidthX = operands[len(operands)-1]
			}
		case dictBlueShift:
			if len(operands) > 0 {
				dict.BlueShift = operands[len(operands)-1]
			}
		case dictBlueFuzz:
			if len(operands) > 0 {
				dict.BlueFuzz = operands[len(operands)-1]
			}
		case dictStemSnapH:
			dict.StemSnapH = append([]int{}, operands...)
		case dictStemSnapV:
			dict.StemSnapV = append([]int{}, operands...)
		}

		operands = operands[:0]
	}

	return dict, nil
}

// decodeDictOperand decodes a single DICT operand.
// Returns the value and bytes consumed.
func decodeDictOperand(data []byte) (int, int) {
	if len(data) == 0 {
		return 0, 0
	}

	b0 := data[0]

	// 1-byte integer
	if b0 >= 32 && b0 <= 246 {
		return int(b0) - 139, 1
	}

	// 2-byte positive integer
	if b0 >= 247 && b0 <= 250 {
		if len(data) < 2 {
			return 0, 1
		}
		return (int(b0)-247)*256 + int(data[1]) + 108, 2
	}

	// 2-byte negative integer
	if b0 >= 251 && b0 <= 254 {
		if len(data) < 2 {
			return 0, 1
		}
		return -(int(b0)-251)*256 - int(data[1]) - 108, 2
	}

	// 3-byte integer (operator 28)
	if b0 == 28 {
		if len(data) < 3 {
			return 0, 1
		}
		v := int(int16(binary.BigEndian.Uint16(data[1:3])))
		return v, 3
	}

	// 5-byte integer (operator 29)
	if b0 == 29 {
		if len(data) < 5 {
			return 0, 1
		}
		v := int(int32(binary.BigEndian.Uint32(data[1:5])))
		return v, 5
	}

	// Real number (operator 30) - skip for simplicity, return 0
	if b0 == 30 {
		// Find end of BCD sequence (nibble 0xf)
		pos := 1
		for pos < len(data) {
			b := data[pos]
			if b&0x0f == 0x0f || b>>4 == 0x0f {
				break
			}
			pos++
		}
		return 0, pos + 1
	}

	return 0, 1
}

// parseCharset parses the charset table.
func parseCharset(data []byte, offset int, numGlyphs int) ([]GlyphID, error) {
	if offset >= len(data) {
		return nil, errors.New("charset offset out of bounds")
	}

	// Predefined charsets
	if offset == 0 {
		// ISOAdobe charset
		charset := make([]GlyphID, numGlyphs)
		for i := range charset {
			charset[i] = GlyphID(i)
		}
		return charset, nil
	}
	if offset == 1 {
		// Expert charset
		charset := make([]GlyphID, numGlyphs)
		for i := range charset {
			charset[i] = GlyphID(i)
		}
		return charset, nil
	}
	if offset == 2 {
		// ExpertSubset charset
		charset := make([]GlyphID, numGlyphs)
		for i := range charset {
			charset[i] = GlyphID(i)
		}
		return charset, nil
	}

	format := data[offset]
	charset := make([]GlyphID, numGlyphs)
	charset[0] = 0 // .notdef

	pos := offset + 1
	gid := 1

	switch format {
	case 0:
		// Format 0: array of SIDs
		for gid < numGlyphs && pos+1 < len(data) {
			sid := binary.BigEndian.Uint16(data[pos:])
			charset[gid] = GlyphID(sid)
			gid++
			pos += 2
		}
	case 1:
		// Format 1: ranges with 1-byte count
		for gid < numGlyphs && pos+2 < len(data) {
			first := int(binary.BigEndian.Uint16(data[pos:]))
			nLeft := int(data[pos+2])
			for i := 0; i <= nLeft && gid < numGlyphs; i++ {
				charset[gid] = GlyphID(first + i)
				gid++
			}
			pos += 3
		}
	case 2:
		// Format 2: ranges with 2-byte count
		for gid < numGlyphs && pos+3 < len(data) {
			first := int(binary.BigEndian.Uint16(data[pos:]))
			nLeft := int(binary.BigEndian.Uint16(data[pos+2:]))
			for i := 0; i <= nLeft && gid < numGlyphs; i++ {
				charset[gid] = GlyphID(first + i)
				gid++
			}
			pos += 4
		}
	}

	return charset, nil
}

// NumGlyphs returns the number of glyphs in the CFF font.
func (c *CFF) NumGlyphs() int {
	return len(c.CharStrings)
}

// GetString returns the string for a given SID.
func (c *CFF) GetString(sid int) string {
	if sid < cffStdStringCount {
		return getStdString(sid)
	}
	idx := sid - cffStdStringCount
	if idx >= 0 && idx < len(c.Strings) {
		return c.Strings[idx]
	}
	return ""
}

// getStdString returns the CFF Standard String for a given SID (0-390).
// HarfBuzz equivalent: cff1_std_strings() in hb-ot-cff1-std-str.hh
func getStdString(sid int) string {
	if sid >= 0 && sid < len(cffStdStrings) {
		return cffStdStrings[sid]
	}
	return fmt.Sprintf("sid%d", sid)
}

// GetGlyphName returns the glyph name for a given glyph ID.
// Returns empty string if the glyph has no name (CID fonts or invalid glyph).
// HarfBuzz equivalent: hb_ot_cff1_table_t::get_glyph_name() in hb-ot-cff1-table.hh:1379-1406
func (c *CFF) GetGlyphName(glyph GlyphID) string {
	// CID fonts have no glyph names
	if c.IsCID {
		return ""
	}

	// Check glyph range
	if int(glyph) >= len(c.Charset) {
		return ""
	}

	// Get SID from charset
	// Note: Charset is typed as []GlyphID but actually contains SIDs
	sid := int(c.Charset[glyph])

	// Get string for SID
	if sid < cffStdStringCount {
		return getStdString(sid)
	}

	idx := sid - cffStdStringCount
	if idx >= 0 && idx < len(c.Strings) {
		return c.Strings[idx]
	}

	return ""
}

// GetGlyphFromName returns the glyph ID for a given glyph name.
// Returns (0, false) if the name cannot be found.
// HarfBuzz equivalent: hb_ot_cff1_table_t::get_glyph_from_name() in hb-ot-cff1-table.hh:1408-1464
func (c *CFF) GetGlyphFromName(name string) (GlyphID, bool) {
	// CID fonts have no glyph names
	if c.IsCID {
		return 0, false
	}

	if name == "" {
		return 0, false
	}

	// Linear search through all glyphs
	// HarfBuzz does binary search on a sorted array, but for simplicity
	// we do linear search. Can be optimized later with caching if needed.
	for gid := 0; gid < len(c.Charset); gid++ {
		sid := int(c.Charset[gid])

		// Get string for this SID
		var str string
		if sid < cffStdStringCount {
			str = getStdString(sid)
		} else {
			idx := sid - cffStdStringCount
			if idx >= 0 && idx < len(c.Strings) {
				str = c.Strings[idx]
			}
		}

		if str == name {
			return GlyphID(gid), true
		}
	}

	return 0, false
}
