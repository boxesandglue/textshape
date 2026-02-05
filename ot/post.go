package ot

import "encoding/binary"

// HarfBuzz equivalent: hb-ot-post-table.hh
// Implements the post table for glyph name lookup.

// PostTable represents a fully parsed post table with glyph name support.
// This extends the basic Post struct in metrics.go with full format 2.0 support.
type PostTable struct {
	Version            uint32
	ItalicAngle        int32
	UnderlinePosition  int16
	UnderlineThickness int16
	IsFixedPitch       uint32

	// Format 2.0 specific fields
	numGlyphs      uint16
	glyphNameIndex []uint16 // Maps glyph ID to name index
	stringPool     []byte   // Pascal strings (length byte + string bytes)
	indexToOffset  []int    // Maps custom name index to offset in stringPool
}

// macRomanNames contains the 258 standard glyph names used in post format 1.0 and 2.0.
// HarfBuzz equivalent: hb-ot-post-macroman.hh
var macRomanNames = [258]string{
	".notdef", ".null", "nonmarkingreturn", "space", "exclam",
	"quotedbl", "numbersign", "dollar", "percent", "ampersand",
	"quotesingle", "parenleft", "parenright", "asterisk", "plus",
	"comma", "hyphen", "period", "slash", "zero",
	"one", "two", "three", "four", "five",
	"six", "seven", "eight", "nine", "colon",
	"semicolon", "less", "equal", "greater", "question",
	"at", "A", "B", "C", "D",
	"E", "F", "G", "H", "I",
	"J", "K", "L", "M", "N",
	"O", "P", "Q", "R", "S",
	"T", "U", "V", "W", "X",
	"Y", "Z", "bracketleft", "backslash", "bracketright",
	"asciicircum", "underscore", "grave", "a", "b",
	"c", "d", "e", "f", "g",
	"h", "i", "j", "k", "l",
	"m", "n", "o", "p", "q",
	"r", "s", "t", "u", "v",
	"w", "x", "y", "z", "braceleft",
	"bar", "braceright", "asciitilde", "Adieresis", "Aring",
	"Ccedilla", "Eacute", "Ntilde", "Odieresis", "Udieresis",
	"aacute", "agrave", "acircumflex", "adieresis", "atilde",
	"aring", "ccedilla", "eacute", "egrave", "ecircumflex",
	"edieresis", "iacute", "igrave", "icircumflex", "idieresis",
	"ntilde", "oacute", "ograve", "ocircumflex", "odieresis",
	"otilde", "uacute", "ugrave", "ucircumflex", "udieresis",
	"dagger", "degree", "cent", "sterling", "section",
	"bullet", "paragraph", "germandbls", "registered", "copyright",
	"trademark", "acute", "dieresis", "notequal", "AE",
	"Oslash", "infinity", "plusminus", "lessequal", "greaterequal",
	"yen", "mu", "partialdiff", "summation", "product",
	"pi", "integral", "ordfeminine", "ordmasculine", "Omega",
	"ae", "oslash", "questiondown", "exclamdown", "logicalnot",
	"radical", "florin", "approxequal", "Delta", "guillemotleft",
	"guillemotright", "ellipsis", "nonbreakingspace", "Agrave", "Atilde",
	"Otilde", "OE", "oe", "endash", "emdash",
	"quotedblleft", "quotedblright", "quoteleft", "quoteright", "divide",
	"lozenge", "ydieresis", "Ydieresis", "fraction", "currency",
	"guilsinglleft", "guilsinglright", "fi", "fl", "daggerdbl",
	"periodcentered", "quotesinglbase", "quotedblbase", "perthousand", "Acircumflex",
	"Ecircumflex", "Aacute", "Edieresis", "Egrave", "Iacute",
	"Icircumflex", "Idieresis", "Igrave", "Oacute", "Ocircumflex",
	"apple", "Ograve", "Uacute", "Ucircumflex", "Ugrave",
	"dotlessi", "circumflex", "tilde", "macron", "breve",
	"dotaccent", "ring", "cedilla", "hungarumlaut", "ogonek",
	"caron", "Lslash", "lslash", "Scaron", "scaron",
	"Zcaron", "zcaron", "brokenbar", "Eth", "eth",
	"Yacute", "yacute", "Thorn", "thorn", "minus",
	"multiply", "onesuperior", "twosuperior", "threesuperior", "onehalf",
	"onequarter", "threequarters", "franc", "Gbreve", "gbreve",
	"Idotaccent", "Scedilla", "scedilla", "Cacute", "cacute",
	"Ccaron", "ccaron", "dcroat",
}

// ParsePostTable parses the post table with full glyph name support.
// HarfBuzz equivalent: hb-ot-post-table.hh accelerator_t::init()
func ParsePostTable(data []byte) (*PostTable, error) {
	if len(data) < 32 {
		return nil, ErrInvalidTable
	}

	p := &PostTable{
		Version:            binary.BigEndian.Uint32(data[0:]),
		ItalicAngle:        int32(binary.BigEndian.Uint32(data[4:])),
		UnderlinePosition:  int16(binary.BigEndian.Uint16(data[8:])),
		UnderlineThickness: int16(binary.BigEndian.Uint16(data[10:])),
		IsFixedPitch:       binary.BigEndian.Uint32(data[12:]),
	}

	// Version 1.0 (0x00010000): Uses standard MacRoman names
	if p.Version == 0x00010000 {
		return p, nil
	}

	// Version 2.0 (0x00020000): Custom glyph names
	if p.Version == 0x00020000 {
		if len(data) < 34 {
			return p, nil // Not enough data, but return what we have
		}

		p.numGlyphs = binary.BigEndian.Uint16(data[32:])

		// Read glyphNameIndex array
		indexOffset := 34
		indexEnd := indexOffset + int(p.numGlyphs)*2
		if indexEnd > len(data) {
			return p, nil // Truncated table
		}

		p.glyphNameIndex = make([]uint16, p.numGlyphs)
		for i := uint16(0); i < p.numGlyphs; i++ {
			offset := indexOffset + int(i)*2
			p.glyphNameIndex[i] = binary.BigEndian.Uint16(data[offset:])
		}

		// Read string pool (Pascal strings)
		poolStart := indexEnd
		if poolStart < len(data) {
			p.stringPool = data[poolStart:]

			// Build index-to-offset map for custom names (index >= 258)
			p.buildIndexToOffsetMap()
		}

		return p, nil
	}

	// Version 3.0 (0x00030000): No glyph names
	// Version 2.5 (0x00025000): Deprecated
	return p, nil
}

// buildIndexToOffsetMap builds a map from custom name index (0-based) to offset in stringPool.
// HarfBuzz equivalent: hb-ot-post-table.hh accelerator_t::init() (index_to_offset building)
func (p *PostTable) buildIndexToOffsetMap() {
	offset := 0
	p.indexToOffset = []int{}

	for offset < len(p.stringPool) {
		// Pascal string: first byte is length
		if offset >= len(p.stringPool) {
			break
		}

		length := int(p.stringPool[offset])
		p.indexToOffset = append(p.indexToOffset, offset)

		// Move to next string
		offset += 1 + length
	}
}

// GetGlyphName returns the glyph name for a given glyph ID.
// Returns empty string if no name is available.
// HarfBuzz equivalent: hb-ot-post-table.hh accelerator_t::get_glyph_name()
func (p *PostTable) GetGlyphName(glyph GlyphID) string {
	// Version 1.0: Use MacRoman names directly
	if p.Version == 0x00010000 {
		if int(glyph) < len(macRomanNames) {
			return macRomanNames[glyph]
		}
		return ""
	}

	// Version 2.0: Use glyphNameIndex
	if p.Version == 0x00020000 {
		if int(glyph) >= len(p.glyphNameIndex) {
			return ""
		}

		index := p.glyphNameIndex[glyph]

		// Standard MacRoman name
		if index < 258 {
			return macRomanNames[index]
		}

		// Custom name from string pool
		customIndex := int(index) - 258
		if customIndex >= len(p.indexToOffset) {
			return ""
		}

		offset := p.indexToOffset[customIndex]
		if offset >= len(p.stringPool) {
			return ""
		}

		// Read Pascal string
		length := int(p.stringPool[offset])
		offset++

		if offset+length > len(p.stringPool) {
			return ""
		}

		return string(p.stringPool[offset : offset+length])
	}

	// Version 3.0 or other: No names
	return ""
}

// HasGlyphNames returns true if the post table contains glyph names.
func (p *PostTable) HasGlyphNames() bool {
	return p.Version == 0x00010000 || p.Version == 0x00020000
}

// GetGlyphFromName returns the glyph ID for a given glyph name.
// Returns false if the name is not found.
// HarfBuzz equivalent: hb-ot-post-table.hh accelerator_t::get_glyph_from_name()
func (p *PostTable) GetGlyphFromName(name string) (GlyphID, bool) {
	// Version 1.0: Search MacRoman names
	if p.Version == 0x00010000 {
		for gid, macName := range macRomanNames {
			if macName == name {
				return GlyphID(gid), true
			}
		}
		return 0, false
	}

	// Version 2.0: Search through glyphNameIndex
	if p.Version == 0x00020000 {
		for gid := uint16(0); gid < p.numGlyphs; gid++ {
			if p.GetGlyphName(GlyphID(gid)) == name {
				return GlyphID(gid), true
			}
		}
		return 0, false
	}

	// Version 3.0 or other: No names
	return 0, false
}
