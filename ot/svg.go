package ot

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"sort"
)

// HarfBuzz equivalent: OT/Color/svg/SVG.hh
// Implements the SVG (Scalable Vector Graphics) table parser.
//
// SVG Spec: https://docs.microsoft.com/en-us/typography/opentype/spec/svg
//
// An SVG table maps glyph-ID ranges to embedded SVG documents. Each
// document is a complete <svg> element that draws one or more glyphs;
// the glyph drawing is selected by its `id="glyphN"` attribute. The
// blob may be gzip-compressed (magic 1F 8B).
//
// Unlike HarfBuzz which only returns the raw blob and delegates
// rendering to a backend (hb_ot_color_glyph_reference_svg at
// hb-ot-color.cc:307-310), we keep the same boundary here — the
// rendering is the consumer's problem (in boxesandglue's case the
// boxesandglue/svgreader pair).

// TagSVG is the OpenType tag for the SVG table.
var TagSVG = MakeTag('S', 'V', 'G', ' ')

// svgDocumentIndexEntry mirrors the on-disk SVGDocumentIndexEntry
// struct (12 bytes, OT/Color/svg/SVG.hh:43-75).
type svgDocumentIndexEntry struct {
	startGlyphID GlyphID
	endGlyphID   GlyphID
	docOffset    uint32 // relative to svgDocEntries (the index, not the table)
	docLength    uint32
}

// SVG holds a parsed SVG color-glyph table.
//
// HarfBuzz equivalent: struct SVG (OT/Color/svg/SVG.hh:77-144).
//
// On-disk header (10 bytes, DEFINE_SIZE_STATIC(10) at SVG.hh:143):
//
//	uint16 version
//	uint32 svgDocEntriesOffset  (Offset32To<SortedArray16Of<…>>)
//	uint32 reserved
//
// At svgDocEntriesOffset (the index base):
//
//	uint16 numEntries
//	SVGDocumentIndexEntry entries[numEntries]  (each 12 bytes, sorted by startGlyphID)
//
// The per-entry docOffset is relative to the index base (the byte at
// svgDocEntriesOffset), NOT to the table start. HB documents this at
// SVG.hh:48-53 where reference_blob adds `index_offset + svgDoc`.
type SVG struct {
	entries      []svgDocumentIndexEntry
	rawData      []byte // entire SVG table data; entry docs are slices into this
	indexBaseOff uint32 // where the index starts inside rawData

	Version uint16
}

// ParseSVG parses an SVG table.
//
// HarfBuzz equivalent: SVG::sanitize (OT/Color/svg/SVG.hh:128-133). HB
// also exposes its accessor via SVG::accelerator_t::reference_blob_for_glyph
// (SVG.hh:89-93) and the public C wrapper at hb-ot-color.cc:307-310.
func ParseSVG(data []byte) (*SVG, error) {
	if len(data) < 10 {
		return nil, ErrInvalidTable
	}
	s := &SVG{
		Version:      binary.BigEndian.Uint16(data[0:]),
		indexBaseOff: binary.BigEndian.Uint32(data[2:]),
		rawData:      data,
	}
	if s.indexBaseOff == 0 || int(s.indexBaseOff)+2 > len(data) {
		// Spec mandates non-zero (SVG.hh:138 "Must be non-zero"); a
		// zero offset means "no SVG data" per HB's has_data check.
		return s, nil
	}
	numEntries := binary.BigEndian.Uint16(data[s.indexBaseOff:])
	entriesEnd := int(s.indexBaseOff) + 2 + int(numEntries)*12
	if entriesEnd > len(data) {
		return nil, ErrInvalidTable
	}
	s.entries = make([]svgDocumentIndexEntry, numEntries)
	for i := range int(numEntries) {
		base := int(s.indexBaseOff) + 2 + i*12
		s.entries[i] = svgDocumentIndexEntry{
			startGlyphID: binary.BigEndian.Uint16(data[base:]),
			endGlyphID:   binary.BigEndian.Uint16(data[base+2:]),
			docOffset:    binary.BigEndian.Uint32(data[base+4:]),
			docLength:    binary.BigEndian.Uint32(data[base+8:]),
		}
	}
	return s, nil
}

// HasData returns true if the table carries at least one document
// entry.
//
// HarfBuzz equivalent: SVG::has_data (OT/Color/svg/SVG.hh:81).
func (s *SVG) HasData() bool {
	return len(s.entries) > 0
}

// GlyphSVG returns the SVG document bytes covering the given glyph, or
// nil if no entry covers it. The returned bytes are decompressed if the
// on-disk blob was gzip-compressed (magic 1F 8B).
//
// HarfBuzz equivalent: SVG::accelerator_t::reference_blob_for_glyph
// (OT/Color/svg/SVG.hh:89-93) combined with SVG::get_glyph_entry's
// bsearch (SVG.hh:125-126). HB returns the raw blob with no
// decompression; the caller (a renderer like Skia) is responsible for
// inflating it. We inflate eagerly because every consumer would have
// to do it anyway and the SVG XML payload is the natural unit at this
// API boundary.
//
// Range entries cover [startGlyphID, endGlyphID]; the same SVG document
// can describe multiple glyphs, each addressed by its `id="glyphN"`
// attribute. We return the SAME document for every glyph in a range —
// the consumer picks the right `<g id="glyphN">` subtree.
func (s *SVG) GlyphSVG(gid GlyphID) []byte {
	if len(s.entries) == 0 {
		return nil
	}
	// SortedArray16Of: binary-search the sorted entries by glyph
	// range. SVG.hh:46 documents the comparator: glyph < start → -1,
	// glyph > end → +1, otherwise 0.
	idx := sort.Search(len(s.entries), func(i int) bool {
		return s.entries[i].endGlyphID >= gid
	})
	if idx >= len(s.entries) || s.entries[idx].startGlyphID > gid {
		return nil
	}
	e := s.entries[idx]
	absStart := int(s.indexBaseOff) + int(e.docOffset)
	absEnd := absStart + int(e.docLength)
	if absStart >= len(s.rawData) || absEnd > len(s.rawData) {
		return nil
	}
	blob := s.rawData[absStart:absEnd]
	if len(blob) >= 2 && blob[0] == 0x1F && blob[1] == 0x8B {
		gz, err := gzip.NewReader(bytes.NewReader(blob))
		if err != nil {
			return nil
		}
		defer gz.Close()
		out, err := io.ReadAll(gz)
		if err != nil {
			return nil
		}
		return out
	}
	return blob
}
