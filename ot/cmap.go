package ot

import (
	"encoding/binary"
	"sort"
)

// Cmap provides Unicode to glyph ID mapping.
type Cmap struct {
	data     []byte
	subtable cmapSubtable
	format14 *cmapFormat14 // For variation selectors (optional)

	// Symbol font support (Platform 3, Encoding 0)
	isSymbol bool
	fontPage uint16 // From OS/2 table (version 0: fsSelection & 0xFF00)
}

// cmapSubtable is the interface for different cmap subtable formats.
type cmapSubtable interface {
	// Lookup returns the glyph ID for a codepoint.
	// Returns 0, false if not found.
	Lookup(cp Codepoint) (GlyphID, bool)
}

// ParseCmap parses a cmap table.
func ParseCmap(data []byte) (*Cmap, error) {
	if len(data) < 4 {
		return nil, ErrInvalidTable
	}

	p := NewParser(data)

	version, _ := p.U16()
	if version != 0 {
		return nil, ErrInvalidFormat
	}

	numTables, _ := p.U16()

	cmap := &Cmap{data: data}

	// Find the best subtable using HarfBuzz's priority order
	var bestSubtable cmapSubtable
	bestPriority := -1

	for i := 0; i < int(numTables); i++ {
		platformID, _ := p.U16()
		encodingID, _ := p.U16()
		offset, _ := p.U32()

		priority := getSubtablePriority(platformID, encodingID)
		if priority > bestPriority {
			st, err := parseCmapSubtable(data, int(offset))
			if err == nil && st != nil {
				bestSubtable = st
				bestPriority = priority
				// Track if this is a symbol font (Platform 3, Encoding 0)
				cmap.isSymbol = (platformID == 3 && encodingID == 0)
			}
		}

		// Also check for format 14 (variation selectors)
		if platformID == 0 && encodingID == 5 {
			if f14, err := parseCmapFormat14(data, int(offset)); err == nil {
				cmap.format14 = f14
			}
		}
	}

	if bestSubtable == nil {
		return nil, ErrInvalidTable
	}

	cmap.subtable = bestSubtable
	return cmap, nil
}

// getSubtablePriority returns the priority for a platform/encoding pair.
// Higher is better. Based on HarfBuzz's find_best_subtable.
func getSubtablePriority(platformID, encodingID uint16) int {
	switch {
	// Symbol (Microsoft platform, Symbol encoding) - highest priority
	case platformID == 3 && encodingID == 0:
		return 100

	// 32-bit subtables (full Unicode)
	case platformID == 3 && encodingID == 10:
		return 90 // Windows UCS-4
	case platformID == 0 && encodingID == 6:
		return 89 // Unicode full
	case platformID == 0 && encodingID == 4:
		return 88 // Unicode 2.0+ full

	// 16-bit subtables (BMP only)
	case platformID == 3 && encodingID == 1:
		return 80 // Windows BMP
	case platformID == 0 && encodingID == 3:
		return 79 // Unicode 2.0 BMP
	case platformID == 0 && encodingID == 2:
		return 78 // Unicode ISO 10646
	case platformID == 0 && encodingID == 1:
		return 77 // Unicode 1.1
	case platformID == 0 && encodingID == 0:
		return 76 // Unicode 1.0

	// Mac subtables (low priority)
	case platformID == 1 && encodingID == 0:
		return 10 // MacRoman

	default:
		return 0
	}
}

// parseCmapSubtable parses a cmap subtable at the given offset.
func parseCmapSubtable(data []byte, offset int) (cmapSubtable, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])

	switch format {
	case 0:
		return parseCmapFormat0(data, offset)
	case 4:
		return parseCmapFormat4(data, offset)
	case 6:
		return parseCmapFormat6(data, offset)
	case 12:
		return parseCmapFormat12(data, offset)
	case 13:
		return parseCmapFormat13(data, offset)
	default:
		return nil, ErrInvalidFormat
	}
}

// SetFontPage sets the font page from OS/2 table for Arabic PUA mapping.
// For OS/2 version 0, this is fsSelection & 0xFF00.
// Source: HarfBuzz hb-ot-os2-table.hh:333-342
func (c *Cmap) SetFontPage(fontPage uint16) {
	c.fontPage = fontPage
}

// IsSymbol returns true if this is a Symbol-encoded font (Platform 3, Encoding 0).
func (c *Cmap) IsSymbol() bool {
	return c.isSymbol
}

// Lookup returns the glyph ID for a codepoint.
// For Symbol fonts, applies PUA mapping based on font page.
// Source: HarfBuzz hb-ot-cmap-table.hh accelerator_t constructor
func (c *Cmap) Lookup(cp Codepoint) (GlyphID, bool) {
	if c.isSymbol {
		// Apply PUA mapping for symbol fonts
		var mapped Codepoint
		switch c.fontPage {
		case FontPageNone:
			// Standard Symbol font - use simple PUA mapping
			mapped = SymbolPUAMap(cp)
		case FontPageSimpArabic:
			mapped = ArabicPUASimpMap(cp)
		case FontPageTradArabic:
			mapped = ArabicPUATradMap(cp)
		default:
			// Other font pages (Hebrew, Farsi, Thai, etc.) - no PUA mapping
			// Fall through to direct lookup
		}
		if mapped != 0 {
			return c.subtable.Lookup(mapped)
		}
		// Fall through to try direct lookup
	}
	return c.subtable.Lookup(cp)
}

// LookupVariation returns the glyph ID for a codepoint with variation selector.
// Returns the glyph ID and true ONLY if a specific variant was found in cmap format 14.
// If no variant is found (or no format 14 table), returns (0, false).
// The caller should fall back to GSUB for combining base + VS when this returns false.
func (c *Cmap) LookupVariation(cp Codepoint, vs Codepoint) (GlyphID, bool) {
	if c.format14 != nil {
		// Check for non-default UVS (specific variant glyph)
		if gid, found := c.format14.lookup(cp, vs); found {
			return gid, true
		}
		// Check for default UVS (use base glyph, variation is valid)
		if c.format14.hasDefaultVariant(cp, vs) {
			gid, _ := c.subtable.Lookup(cp)
			return gid, true
		}
	}
	// No format 14 or no variant found - let GSUB handle it
	return 0, false
}

// --- Format 0: Byte encoding table (legacy, 8-bit only) ---

type cmapFormat0 struct {
	glyphIDs [256]byte
}

func parseCmapFormat0(data []byte, offset int) (*cmapFormat0, error) {
	if offset+262 > len(data) { // 6 header + 256 glyphs
		return nil, ErrInvalidOffset
	}
	f := &cmapFormat0{}
	copy(f.glyphIDs[:], data[offset+6:offset+262])
	return f, nil
}

func (f *cmapFormat0) Lookup(cp Codepoint) (GlyphID, bool) {
	if cp >= 256 {
		return 0, false
	}
	gid := f.glyphIDs[cp]
	if gid == 0 {
		return 0, false
	}
	return GlyphID(gid), true
}

// --- Format 4: Segment mapping to delta values (BMP) ---

type cmapFormat4 struct {
	data     []byte // Raw subtable data
	segCount int
	// Offsets into data for each array
	endCodeOff      int
	startCodeOff    int
	idDeltaOff      int
	idRangeOffOff   int
	glyphIdArrayOff int
	glyphIdArrayLen int
}

func parseCmapFormat4(data []byte, offset int) (*cmapFormat4, error) {
	if offset+14 > len(data) {
		return nil, ErrInvalidOffset
	}

	length := int(binary.BigEndian.Uint16(data[offset+2:]))
	if offset+length > len(data) {
		return nil, ErrInvalidOffset
	}

	segCountX2 := int(binary.BigEndian.Uint16(data[offset+6:]))
	segCount := segCountX2 / 2

	f := &cmapFormat4{
		data:     data[offset : offset+length],
		segCount: segCount,
	}

	// Calculate array offsets (relative to subtable start)
	f.endCodeOff = 14
	f.startCodeOff = f.endCodeOff + segCountX2 + 2 // +2 for reservedPad
	f.idDeltaOff = f.startCodeOff + segCountX2
	f.idRangeOffOff = f.idDeltaOff + segCountX2
	f.glyphIdArrayOff = f.idRangeOffOff + segCountX2
	f.glyphIdArrayLen = (length - f.glyphIdArrayOff) / 2

	return f, nil
}

func (f *cmapFormat4) Lookup(cp Codepoint) (GlyphID, bool) {
	if cp > 0xFFFF {
		return 0, false
	}

	// Binary search for segment
	segIdx := f.searchSegment(uint16(cp))
	if segIdx < 0 {
		return 0, false
	}

	startCode := f.startCodeAt(segIdx)
	if uint16(cp) < startCode {
		return 0, false
	}

	idRangeOffset := f.idRangeOffsetAt(segIdx)
	idDelta := f.idDeltaAt(segIdx)

	var gid uint16
	if idRangeOffset == 0 {
		gid = uint16(int(cp) + int(idDelta))
	} else {
		// glyphId = *(idRangeOffset[i]/2 + (c - startCode[i]) + &idRangeOffset[i])
		// This is the crazy pointer arithmetic from the spec
		index := int(idRangeOffset)/2 + int(uint16(cp)-startCode) + segIdx - f.segCount
		if index < 0 || index >= f.glyphIdArrayLen {
			return 0, false
		}
		gid = binary.BigEndian.Uint16(f.data[f.glyphIdArrayOff+index*2:])
		if gid == 0 {
			return 0, false
		}
		gid = uint16(int(gid) + int(idDelta))
	}

	gid &= 0xFFFF
	if gid == 0 {
		return 0, false
	}
	return GlyphID(gid), true
}

func (f *cmapFormat4) searchSegment(cp uint16) int {
	// Binary search for the segment containing cp
	lo, hi := 0, f.segCount
	for lo < hi {
		mid := (lo + hi) / 2
		endCode := f.endCodeAt(mid)
		if cp > endCode {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo >= f.segCount {
		return -1
	}
	return lo
}

func (f *cmapFormat4) endCodeAt(i int) uint16 {
	return binary.BigEndian.Uint16(f.data[f.endCodeOff+i*2:])
}

func (f *cmapFormat4) startCodeAt(i int) uint16 {
	return binary.BigEndian.Uint16(f.data[f.startCodeOff+i*2:])
}

func (f *cmapFormat4) idDeltaAt(i int) int16 {
	return int16(binary.BigEndian.Uint16(f.data[f.idDeltaOff+i*2:]))
}

func (f *cmapFormat4) idRangeOffsetAt(i int) uint16 {
	return binary.BigEndian.Uint16(f.data[f.idRangeOffOff+i*2:])
}

// --- Format 6: Trimmed table mapping (BMP, contiguous range) ---

type cmapFormat6 struct {
	firstCode uint16
	glyphIDs  []uint16
}

func parseCmapFormat6(data []byte, offset int) (*cmapFormat6, error) {
	if offset+10 > len(data) {
		return nil, ErrInvalidOffset
	}

	length := int(binary.BigEndian.Uint16(data[offset+2:]))
	if offset+length > len(data) {
		return nil, ErrInvalidOffset
	}

	firstCode := binary.BigEndian.Uint16(data[offset+6:])
	entryCount := int(binary.BigEndian.Uint16(data[offset+8:]))

	if offset+10+entryCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	f := &cmapFormat6{
		firstCode: firstCode,
		glyphIDs:  make([]uint16, entryCount),
	}

	for i := 0; i < entryCount; i++ {
		f.glyphIDs[i] = binary.BigEndian.Uint16(data[offset+10+i*2:])
	}

	return f, nil
}

func (f *cmapFormat6) Lookup(cp Codepoint) (GlyphID, bool) {
	if cp > 0xFFFF {
		return 0, false
	}
	idx := int(cp) - int(f.firstCode)
	if idx < 0 || idx >= len(f.glyphIDs) {
		return 0, false
	}
	gid := f.glyphIDs[idx]
	if gid == 0 {
		return 0, false
	}
	return GlyphID(gid), true
}

// --- Format 12: Segmented coverage (full Unicode) ---

type cmapFormat12 struct {
	groups []cmapGroup12
}

type cmapGroup12 struct {
	startCharCode uint32
	endCharCode   uint32
	startGlyphID  uint32
}

func parseCmapFormat12(data []byte, offset int) (*cmapFormat12, error) {
	if offset+16 > len(data) {
		return nil, ErrInvalidOffset
	}

	length := binary.BigEndian.Uint32(data[offset+4:])
	if uint32(offset)+length > uint32(len(data)) {
		return nil, ErrInvalidOffset
	}

	numGroups := int(binary.BigEndian.Uint32(data[offset+12:]))
	if offset+16+numGroups*12 > len(data) {
		return nil, ErrInvalidOffset
	}

	f := &cmapFormat12{
		groups: make([]cmapGroup12, numGroups),
	}

	off := offset + 16
	for i := 0; i < numGroups; i++ {
		f.groups[i] = cmapGroup12{
			startCharCode: binary.BigEndian.Uint32(data[off:]),
			endCharCode:   binary.BigEndian.Uint32(data[off+4:]),
			startGlyphID:  binary.BigEndian.Uint32(data[off+8:]),
		}
		off += 12
	}

	return f, nil
}

func (f *cmapFormat12) Lookup(cp Codepoint) (GlyphID, bool) {
	// Binary search for group
	idx := sort.Search(len(f.groups), func(i int) bool {
		return f.groups[i].endCharCode >= cp
	})

	if idx >= len(f.groups) {
		return 0, false
	}

	g := &f.groups[idx]
	if cp < g.startCharCode || cp > g.endCharCode {
		return 0, false
	}

	gid := g.startGlyphID + (cp - g.startCharCode)
	if gid == 0 || gid > 0xFFFF {
		return 0, false
	}

	return GlyphID(gid), true
}

// --- Format 13: Many-to-one range mappings (full Unicode) ---

type cmapFormat13 struct {
	groups []cmapGroup12 // Same structure as format 12
}

func parseCmapFormat13(data []byte, offset int) (*cmapFormat13, error) {
	if offset+16 > len(data) {
		return nil, ErrInvalidOffset
	}

	length := binary.BigEndian.Uint32(data[offset+4:])
	if uint32(offset)+length > uint32(len(data)) {
		return nil, ErrInvalidOffset
	}

	numGroups := int(binary.BigEndian.Uint32(data[offset+12:]))
	if offset+16+numGroups*12 > len(data) {
		return nil, ErrInvalidOffset
	}

	f := &cmapFormat13{
		groups: make([]cmapGroup12, numGroups),
	}

	off := offset + 16
	for i := 0; i < numGroups; i++ {
		f.groups[i] = cmapGroup12{
			startCharCode: binary.BigEndian.Uint32(data[off:]),
			endCharCode:   binary.BigEndian.Uint32(data[off+4:]),
			startGlyphID:  binary.BigEndian.Uint32(data[off+8:]),
		}
		off += 12
	}

	return f, nil
}

func (f *cmapFormat13) Lookup(cp Codepoint) (GlyphID, bool) {
	// Binary search for group
	idx := sort.Search(len(f.groups), func(i int) bool {
		return f.groups[i].endCharCode >= cp
	})

	if idx >= len(f.groups) {
		return 0, false
	}

	g := &f.groups[idx]
	if cp < g.startCharCode || cp > g.endCharCode {
		return 0, false
	}

	// Format 13: all codepoints in range map to same glyph
	gid := g.startGlyphID
	if gid == 0 || gid > 0xFFFF {
		return 0, false
	}

	return GlyphID(gid), true
}

// --- Format 14: Unicode Variation Sequences ---

type cmapFormat14 struct {
	records []variationRecord
	data    []byte
}

type variationRecord struct {
	varSelector      uint32
	defaultUVSOff    uint32
	nonDefaultUVSOff uint32
}

func parseCmapFormat14(data []byte, offset int) (*cmapFormat14, error) {
	if offset+10 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 14 {
		return nil, ErrInvalidFormat
	}

	length := binary.BigEndian.Uint32(data[offset+2:])
	if uint32(offset)+length > uint32(len(data)) {
		return nil, ErrInvalidOffset
	}

	numRecords := int(binary.BigEndian.Uint32(data[offset+6:]))
	if offset+10+numRecords*11 > len(data) {
		return nil, ErrInvalidOffset
	}

	f := &cmapFormat14{
		records: make([]variationRecord, numRecords),
		data:    data[offset:],
	}

	off := 10
	for i := 0; i < numRecords; i++ {
		// varSelector is 3 bytes (UINT24)
		vs := uint32(data[offset+off])<<16 | uint32(data[offset+off+1])<<8 | uint32(data[offset+off+2])
		f.records[i] = variationRecord{
			varSelector:      vs,
			defaultUVSOff:    binary.BigEndian.Uint32(data[offset+off+3:]),
			nonDefaultUVSOff: binary.BigEndian.Uint32(data[offset+off+7:]),
		}
		off += 11
	}

	return f, nil
}

func (f *cmapFormat14) lookup(cp Codepoint, vs Codepoint) (GlyphID, bool) {
	// Find variation selector record
	idx := sort.Search(len(f.records), func(i int) bool {
		return f.records[i].varSelector >= vs
	})

	if idx >= len(f.records) || f.records[idx].varSelector != vs {
		return 0, false
	}

	rec := &f.records[idx]

	// Check non-default UVS
	if rec.nonDefaultUVSOff != 0 {
		if gid, found := f.lookupNonDefault(int(rec.nonDefaultUVSOff), cp); found {
			return gid, true
		}
	}

	return 0, false
}

func (f *cmapFormat14) hasDefaultVariant(cp Codepoint, vs Codepoint) bool {
	// Find variation selector record
	idx := sort.Search(len(f.records), func(i int) bool {
		return f.records[i].varSelector >= vs
	})

	if idx >= len(f.records) || f.records[idx].varSelector != vs {
		return false
	}

	rec := &f.records[idx]

	// Check default UVS
	if rec.defaultUVSOff != 0 {
		return f.lookupDefault(int(rec.defaultUVSOff), cp)
	}

	return false
}

func (f *cmapFormat14) lookupDefault(offset int, cp Codepoint) bool {
	if offset+4 > len(f.data) {
		return false
	}

	numRanges := int(binary.BigEndian.Uint32(f.data[offset:]))
	offset += 4

	if offset+numRanges*4 > len(f.data) {
		return false
	}

	// Binary search for range containing cp
	idx := sort.Search(numRanges, func(i int) bool {
		rangeOff := offset + i*4
		startUnicode := uint32(f.data[rangeOff])<<16 | uint32(f.data[rangeOff+1])<<8 | uint32(f.data[rangeOff+2])
		additionalCount := uint32(f.data[rangeOff+3])
		return startUnicode+additionalCount >= cp
	})

	if idx >= numRanges {
		return false
	}

	rangeOff := offset + idx*4
	startUnicode := uint32(f.data[rangeOff])<<16 | uint32(f.data[rangeOff+1])<<8 | uint32(f.data[rangeOff+2])
	additionalCount := uint32(f.data[rangeOff+3])

	return cp >= startUnicode && cp <= startUnicode+additionalCount
}

func (f *cmapFormat14) lookupNonDefault(offset int, cp Codepoint) (GlyphID, bool) {
	if offset+4 > len(f.data) {
		return 0, false
	}

	numMappings := int(binary.BigEndian.Uint32(f.data[offset:]))
	offset += 4

	if offset+numMappings*5 > len(f.data) {
		return 0, false
	}

	// Binary search for mapping
	idx := sort.Search(numMappings, func(i int) bool {
		mapOff := offset + i*5
		unicodeValue := uint32(f.data[mapOff])<<16 | uint32(f.data[mapOff+1])<<8 | uint32(f.data[mapOff+2])
		return unicodeValue >= cp
	})

	if idx >= numMappings {
		return 0, false
	}

	mapOff := offset + idx*5
	unicodeValue := uint32(f.data[mapOff])<<16 | uint32(f.data[mapOff+1])<<8 | uint32(f.data[mapOff+2])

	if unicodeValue != cp {
		return 0, false
	}

	gid := binary.BigEndian.Uint16(f.data[mapOff+3:])
	return GlyphID(gid), true
}

// --- Cmap Collection Methods (HarfBuzz-style) ---

// cmapCollector is the interface for collecting cmap mappings.
type cmapCollector interface {
	collectMapping(mapping map[rune]GlyphID)
}

// CollectMapping returns a map of all Unicode codepoints to glyph IDs.
func (c *Cmap) CollectMapping() map[rune]GlyphID {
	mapping := make(map[rune]GlyphID)
	if c.subtable == nil {
		return mapping
	}

	if collector, ok := c.subtable.(cmapCollector); ok {
		collector.collectMapping(mapping)
	}
	return mapping
}

// CollectReverseMapping returns a map of glyph IDs to Unicode codepoints.
// If multiple codepoints map to the same glyph, the last one wins.
func (c *Cmap) CollectReverseMapping() map[GlyphID]rune {
	mapping := c.CollectMapping()
	reverse := make(map[GlyphID]rune, len(mapping))
	for r, gid := range mapping {
		reverse[gid] = r
	}
	return reverse
}

// Format 0 collection
func (f *cmapFormat0) collectMapping(mapping map[rune]GlyphID) {
	for i := 0; i < 256; i++ {
		if gid := f.glyphIDs[i]; gid != 0 {
			mapping[rune(i)] = GlyphID(gid)
		}
	}
}

// Format 4 collection
func (f *cmapFormat4) collectMapping(mapping map[rune]GlyphID) {
	for segIdx := 0; segIdx < f.segCount; segIdx++ {
		startCode := f.startCodeAt(segIdx)
		endCode := f.endCodeAt(segIdx)

		// Skip 0xFFFF terminator segment
		if startCode == 0xFFFF {
			continue
		}

		for cp := startCode; cp <= endCode; cp++ {
			if gid, ok := f.Lookup(Codepoint(cp)); ok && gid != 0 {
				mapping[rune(cp)] = gid
			}
		}
	}
}

// Format 6 collection
func (f *cmapFormat6) collectMapping(mapping map[rune]GlyphID) {
	for i, gid := range f.glyphIDs {
		if gid != 0 {
			mapping[rune(int(f.firstCode)+i)] = GlyphID(gid)
		}
	}
}

// Format 12 collection
func (f *cmapFormat12) collectMapping(mapping map[rune]GlyphID) {
	for _, g := range f.groups {
		for cp := g.startCharCode; cp <= g.endCharCode; cp++ {
			gid := g.startGlyphID + (cp - g.startCharCode)
			if gid != 0 && gid <= 0xFFFF {
				mapping[rune(cp)] = GlyphID(gid)
			}
		}
	}
}

// Format 13 collection (many-to-one: all codepoints in range map to same glyph)
func (f *cmapFormat13) collectMapping(mapping map[rune]GlyphID) {
	for _, g := range f.groups {
		if g.startGlyphID == 0 || g.startGlyphID > 0xFFFF {
			continue
		}
		gid := GlyphID(g.startGlyphID)
		for cp := g.startCharCode; cp <= g.endCharCode; cp++ {
			mapping[rune(cp)] = gid
		}
	}
}
