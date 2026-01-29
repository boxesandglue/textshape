package ot

import (
	"encoding/binary"
	"io"
)

// FontExtents contains font-wide extent values.
// This matches HarfBuzz's hb_font_extents_t.
type FontExtents struct {
	Ascender  int16 // Typographic ascender
	Descender int16 // Typographic descender (usually negative)
	LineGap   int16 // Line spacing gap
}

// GlyphExtents contains glyph extent values.
type GlyphExtents struct {
	XBearing int16 // Left side of glyph from origin
	YBearing int16 // Top side of glyph from origin
	Width    int16 // Width of glyph
	Height   int16 // Height of glyph (usually negative)
}

// Head represents the font header table.
type Head struct {
	Version            uint32
	FontRevision       uint32
	CheckSumAdjustment uint32
	MagicNumber        uint32
	Flags              uint16
	UnitsPerEm         uint16
	Created            int64
	Modified           int64
	XMin               int16
	YMin               int16
	XMax               int16
	YMax               int16
	MacStyle           uint16
	LowestRecPPEM      uint16
	FontDirectionHint  int16
	IndexToLocFormat   int16
	GlyphDataFormat    int16
}

// ParseHead parses the head table.
func ParseHead(data []byte) (*Head, error) {
	if len(data) < 54 {
		return nil, ErrInvalidTable
	}

	h := &Head{
		Version:            binary.BigEndian.Uint32(data[0:]),
		FontRevision:       binary.BigEndian.Uint32(data[4:]),
		CheckSumAdjustment: binary.BigEndian.Uint32(data[8:]),
		MagicNumber:        binary.BigEndian.Uint32(data[12:]),
		Flags:              binary.BigEndian.Uint16(data[16:]),
		UnitsPerEm:         binary.BigEndian.Uint16(data[18:]),
		Created:            int64(binary.BigEndian.Uint64(data[20:])),
		Modified:           int64(binary.BigEndian.Uint64(data[28:])),
		XMin:               int16(binary.BigEndian.Uint16(data[36:])),
		YMin:               int16(binary.BigEndian.Uint16(data[38:])),
		XMax:               int16(binary.BigEndian.Uint16(data[40:])),
		YMax:               int16(binary.BigEndian.Uint16(data[42:])),
		MacStyle:           binary.BigEndian.Uint16(data[44:]),
		LowestRecPPEM:      binary.BigEndian.Uint16(data[46:]),
		FontDirectionHint:  int16(binary.BigEndian.Uint16(data[48:])),
		IndexToLocFormat:   int16(binary.BigEndian.Uint16(data[50:])),
		GlyphDataFormat:    int16(binary.BigEndian.Uint16(data[52:])),
	}

	return h, nil
}

// OS2 represents the OS/2 table.
type OS2 struct {
	Version             uint16
	XAvgCharWidth       int16
	UsWeightClass       uint16
	UsWidthClass        uint16
	FsType              uint16
	YSubscriptXSize     int16
	YSubscriptYSize     int16
	YSubscriptXOffset   int16
	YSubscriptYOffset   int16
	YSuperscriptXSize   int16
	YSuperscriptYSize   int16
	YSuperscriptXOffset int16
	YSuperscriptYOffset int16
	YStrikeoutSize      int16
	YStrikeoutPosition  int16
	SFamilyClass        int16
	Panose              [10]byte
	UlUnicodeRange1     uint32
	UlUnicodeRange2     uint32
	UlUnicodeRange3     uint32
	UlUnicodeRange4     uint32
	AchVendID           [4]byte
	FsSelection         uint16
	UsFirstCharIndex    uint16
	UsLastCharIndex     uint16
	STypoAscender       int16
	STypoDescender      int16
	STypoLineGap        int16
	UsWinAscent         uint16
	UsWinDescent        uint16
	// Version 1+
	UlCodePageRange1 uint32
	UlCodePageRange2 uint32
	// Version 2+
	SxHeight      int16
	SCapHeight    int16
	UsDefaultChar uint16
	UsBreakChar   uint16
	UsMaxContext  uint16
}

// ParseOS2 parses the OS/2 table.
func ParseOS2(data []byte) (*OS2, error) {
	if len(data) < 78 {
		return nil, ErrInvalidTable
	}

	o := &OS2{
		Version:             binary.BigEndian.Uint16(data[0:]),
		XAvgCharWidth:       int16(binary.BigEndian.Uint16(data[2:])),
		UsWeightClass:       binary.BigEndian.Uint16(data[4:]),
		UsWidthClass:        binary.BigEndian.Uint16(data[6:]),
		FsType:              binary.BigEndian.Uint16(data[8:]),
		YSubscriptXSize:     int16(binary.BigEndian.Uint16(data[10:])),
		YSubscriptYSize:     int16(binary.BigEndian.Uint16(data[12:])),
		YSubscriptXOffset:   int16(binary.BigEndian.Uint16(data[14:])),
		YSubscriptYOffset:   int16(binary.BigEndian.Uint16(data[16:])),
		YSuperscriptXSize:   int16(binary.BigEndian.Uint16(data[18:])),
		YSuperscriptYSize:   int16(binary.BigEndian.Uint16(data[20:])),
		YSuperscriptXOffset: int16(binary.BigEndian.Uint16(data[22:])),
		YSuperscriptYOffset: int16(binary.BigEndian.Uint16(data[24:])),
		YStrikeoutSize:      int16(binary.BigEndian.Uint16(data[26:])),
		YStrikeoutPosition:  int16(binary.BigEndian.Uint16(data[28:])),
		SFamilyClass:        int16(binary.BigEndian.Uint16(data[30:])),
		FsSelection:         binary.BigEndian.Uint16(data[62:]),
		UsFirstCharIndex:    binary.BigEndian.Uint16(data[64:]),
		UsLastCharIndex:     binary.BigEndian.Uint16(data[66:]),
		STypoAscender:       int16(binary.BigEndian.Uint16(data[68:])),
		STypoDescender:      int16(binary.BigEndian.Uint16(data[70:])),
		STypoLineGap:        int16(binary.BigEndian.Uint16(data[72:])),
		UsWinAscent:         binary.BigEndian.Uint16(data[74:]),
		UsWinDescent:        binary.BigEndian.Uint16(data[76:]),
	}

	copy(o.Panose[:], data[32:42])
	o.UlUnicodeRange1 = binary.BigEndian.Uint32(data[42:])
	o.UlUnicodeRange2 = binary.BigEndian.Uint32(data[46:])
	o.UlUnicodeRange3 = binary.BigEndian.Uint32(data[50:])
	o.UlUnicodeRange4 = binary.BigEndian.Uint32(data[54:])
	copy(o.AchVendID[:], data[58:62])

	// Version 1+ fields
	if len(data) >= 86 {
		o.UlCodePageRange1 = binary.BigEndian.Uint32(data[78:])
		o.UlCodePageRange2 = binary.BigEndian.Uint32(data[82:])
	}

	// Version 2+ fields
	if len(data) >= 96 && o.Version >= 2 {
		o.SxHeight = int16(binary.BigEndian.Uint16(data[86:]))
		o.SCapHeight = int16(binary.BigEndian.Uint16(data[88:]))
		o.UsDefaultChar = binary.BigEndian.Uint16(data[90:])
		o.UsBreakChar = binary.BigEndian.Uint16(data[92:])
		o.UsMaxContext = binary.BigEndian.Uint16(data[94:])
	}

	return o, nil
}

// Post represents the post table.
type Post struct {
	Version            uint32
	ItalicAngle        int32 // Fixed-point 16.16
	UnderlinePosition  int16
	UnderlineThickness int16
	IsFixedPitch       uint32
}

// ParsePost parses the post table (minimal parsing for metrics).
func ParsePost(data []byte) (*Post, error) {
	if len(data) < 32 {
		return nil, ErrInvalidTable
	}

	p := &Post{
		Version:            binary.BigEndian.Uint32(data[0:]),
		ItalicAngle:        int32(binary.BigEndian.Uint32(data[4:])),
		UnderlinePosition:  int16(binary.BigEndian.Uint16(data[8:])),
		UnderlineThickness: int16(binary.BigEndian.Uint16(data[10:])),
		IsFixedPitch:       binary.BigEndian.Uint32(data[12:]),
	}

	return p, nil
}

// ItalicAngleDegrees returns the italic angle in degrees.
func (p *Post) ItalicAngleDegrees() float64 {
	return float64(p.ItalicAngle) / 65536.0
}

// Name represents the name table.
type Name struct {
	entries map[uint16]string // nameID -> string
}

// ParseName parses the name table.
func ParseName(data []byte) (*Name, error) {
	if len(data) < 6 {
		return nil, ErrInvalidTable
	}

	format := binary.BigEndian.Uint16(data[0:])
	count := binary.BigEndian.Uint16(data[2:])
	storageOffset := binary.BigEndian.Uint16(data[4:])

	n := &Name{entries: make(map[uint16]string)}

	if format > 1 {
		return n, nil // Unsupported format
	}

	recordOffset := 6
	for i := 0; i < int(count); i++ {
		if recordOffset+12 > len(data) {
			break
		}

		platformID := binary.BigEndian.Uint16(data[recordOffset:])
		encodingID := binary.BigEndian.Uint16(data[recordOffset+2:])
		// languageID := binary.BigEndian.Uint16(data[recordOffset+4:])
		nameID := binary.BigEndian.Uint16(data[recordOffset+6:])
		length := binary.BigEndian.Uint16(data[recordOffset+8:])
		offset := binary.BigEndian.Uint16(data[recordOffset+10:])

		recordOffset += 12

		// Prefer Unicode (platform 0 or 3) or Mac Roman (platform 1)
		stringOffset := int(storageOffset) + int(offset)
		if stringOffset+int(length) > len(data) {
			continue
		}

		stringData := data[stringOffset : stringOffset+int(length)]

		var str string
		if platformID == 3 || platformID == 0 {
			// Windows/Unicode: UTF-16BE
			str = decodeUTF16BE(stringData)
		} else if platformID == 1 && encodingID == 0 {
			// Mac Roman
			str = string(stringData)
		}

		if str != "" {
			n.entries[nameID] = str
		}
	}

	return n, nil
}

func decodeUTF16BE(data []byte) string {
	if len(data)%2 != 0 {
		return ""
	}
	runes := make([]rune, len(data)/2)
	for i := 0; i < len(data)/2; i++ {
		runes[i] = rune(binary.BigEndian.Uint16(data[i*2:]))
	}
	return string(runes)
}

// Get returns the string for a nameID.
func (n *Name) Get(nameID uint16) string {
	return n.entries[nameID]
}

// PostScriptName returns the PostScript name (nameID 6).
func (n *Name) PostScriptName() string {
	return n.entries[6]
}

// FamilyName returns the font family name (nameID 1).
func (n *Name) FamilyName() string {
	return n.entries[1]
}

// FullName returns the full font name (nameID 4).
func (n *Name) FullName() string {
	return n.entries[4]
}

// Face represents a font face with parsed tables for metrics.
// This is a higher-level abstraction that caches parsed tables.
type Face struct {
	Font  *Font
	head  *Head
	hhea  *Hhea
	hmtx  *Hmtx
	os2   *OS2
	post  *Post
	name  *Name
	cmap  *Cmap
	fvar  *Fvar
	upem  uint16
	isCFF bool
}

// NewFace creates a new Face from a Font, parsing required tables.
func NewFace(font *Font) (*Face, error) {
	f := &Face{Font: font}

	// Parse head (required)
	if data, err := font.TableData(TagHead); err == nil {
		f.head, _ = ParseHead(data)
	}
	if f.head != nil {
		f.upem = f.head.UnitsPerEm
	}
	if f.upem == 0 {
		f.upem = 1000 // Default for CFF
	}

	// Parse hhea (required)
	if data, err := font.TableData(TagHhea); err == nil {
		f.hhea, _ = ParseHhea(data)
	}

	// Parse hmtx
	if f.hhea != nil {
		if data, err := font.TableData(TagHmtx); err == nil {
			f.hmtx, _ = ParseHmtx(data, int(f.hhea.NumberOfHMetrics), font.NumGlyphs())
		}
	}

	// Parse OS/2 (optional but common)
	if data, err := font.TableData(TagOS2); err == nil {
		f.os2, _ = ParseOS2(data)
	}

	// Parse post (optional)
	if data, err := font.TableData(TagPost); err == nil {
		f.post, _ = ParsePost(data)
	}

	// Parse name
	if data, err := font.TableData(TagName); err == nil {
		f.name, _ = ParseName(data)
	}

	// Parse cmap
	if data, err := font.TableData(TagCmap); err == nil {
		f.cmap, _ = ParseCmap(data)
		// For Symbol fonts, set font page from OS/2 for Arabic PUA mapping
		if f.cmap != nil && f.cmap.IsSymbol() && f.os2 != nil && f.os2.Version == 0 {
			f.cmap.SetFontPage(f.os2.FsSelection & 0xFF00)
		}
	}

	// Check if CFF font
	f.isCFF = font.HasTable(TagCFF)

	// Parse fvar (variable fonts)
	if data, err := font.TableData(TagFvar); err == nil {
		f.fvar, _ = ParseFvar(data)
	}

	return f, nil
}

// Upem returns the units per em.
func (f *Face) Upem() uint16 {
	return f.upem
}

// IsCFF returns true if the font uses CFF outlines.
func (f *Face) IsCFF() bool {
	return f.isCFF
}

// GetHExtents returns horizontal font extents.
func (f *Face) GetHExtents() FontExtents {
	var ext FontExtents
	if f.hhea != nil {
		ext.Ascender = f.hhea.Ascender
		ext.Descender = f.hhea.Descender
		ext.LineGap = f.hhea.LineGap
	}
	return ext
}

// HorizontalAdvance returns the horizontal advance for a glyph in font units.
func (f *Face) HorizontalAdvance(glyph GlyphID) float32 {
	if f.hmtx != nil {
		return float32(f.hmtx.GetAdvanceWidth(glyph))
	}
	return float32(f.upem)
}

// Cmap returns the cmap table.
func (f *Face) Cmap() *Cmap {
	return f.cmap
}

// PostscriptName returns the PostScript name of the font.
func (f *Face) PostscriptName() string {
	if f.name != nil {
		return f.name.PostScriptName()
	}
	return "Unknown"
}

// FamilyName returns the font family name.
func (f *Face) FamilyName() string {
	if f.name != nil {
		return f.name.FamilyName()
	}
	return "Unknown"
}

// --- Raw metric accessors (no PDF formatting) ---

// Ascender returns the typographic ascender in font units.
func (f *Face) Ascender() int16 {
	if f.hhea != nil {
		return f.hhea.Ascender
	}
	return 800
}

// Descender returns the typographic descender in font units (usually negative).
func (f *Face) Descender() int16 {
	if f.hhea != nil {
		return f.hhea.Descender
	}
	return -200
}

// CapHeight returns the cap height in font units.
func (f *Face) CapHeight() int16 {
	if f.os2 != nil && f.os2.SCapHeight != 0 {
		return f.os2.SCapHeight
	}
	return f.Ascender()
}

// XHeight returns the x-height in font units.
func (f *Face) XHeight() int16 {
	if f.os2 != nil && f.os2.SxHeight != 0 {
		return f.os2.SxHeight
	}
	return f.Ascender() / 2
}

// BBox returns the font bounding box.
func (f *Face) BBox() (xMin, yMin, xMax, yMax int16) {
	if f.head != nil {
		return f.head.XMin, f.head.YMin, f.head.XMax, f.head.YMax
	}
	return 0, -200, 1000, 800
}

// IsFixedPitch returns true if the font is monospaced.
func (f *Face) IsFixedPitch() bool {
	return f.post != nil && f.post.IsFixedPitch != 0
}

// IsItalic returns true if the font is italic.
func (f *Face) IsItalic() bool {
	return f.head != nil && f.head.MacStyle&2 != 0
}

// ItalicAngle returns the italic angle in degrees (fixed-point 16.16).
func (f *Face) ItalicAngle() int32 {
	if f.post != nil {
		return f.post.ItalicAngle
	}
	return 0
}

// WeightClass returns the font weight class (100-900).
func (f *Face) WeightClass() uint16 {
	if f.os2 != nil {
		return f.os2.UsWeightClass
	}
	return 400
}

// LineGap returns the line gap in font units.
func (f *Face) LineGap() int16 {
	if f.hhea != nil {
		return f.hhea.LineGap
	}
	return 0
}

// LoadFace loads a font from an io.Reader and returns a Face.
func LoadFace(r io.Reader, index int) (*Face, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	font, err := ParseFont(data, index)
	if err != nil {
		return nil, err
	}
	return NewFace(font)
}

// --- Variable Font Methods ---

// HasVariations returns true if the font is a variable font.
func (f *Face) HasVariations() bool {
	return f.fvar.HasData()
}

// VariationAxes returns information about all variation axes.
// Returns nil for non-variable fonts.
func (f *Face) VariationAxes() []AxisInfo {
	if f.fvar == nil {
		return nil
	}
	return f.fvar.AxisInfos()
}

// FindVariationAxis finds a variation axis by its tag.
func (f *Face) FindVariationAxis(tag Tag) (AxisInfo, bool) {
	if f.fvar == nil {
		return AxisInfo{}, false
	}
	return f.fvar.FindAxis(tag)
}

// NamedInstances returns all named instances (e.g., "Bold", "Light").
// Returns nil for non-variable fonts.
func (f *Face) NamedInstances() []NamedInstance {
	if f.fvar == nil {
		return nil
	}
	return f.fvar.NamedInstances()
}

// Fvar returns the parsed fvar table, or nil if not present.
func (f *Face) Fvar() *Fvar {
	return f.fvar
}

// LoadFaceFromData loads a font from byte data and returns a Face.
func LoadFaceFromData(data []byte, index int) (*Face, error) {
	font, err := ParseFont(data, index)
	if err != nil {
		return nil, err
	}
	return NewFace(font)
}
