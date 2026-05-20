package ot

import (
	"encoding/binary"
	"sort"
)

// HarfBuzz equivalent: OT/Color/COLR/COLR.hh
// Implements the COLR (Color) table parser for COLRv0 only.
//
// COLR Spec: https://docs.microsoft.com/en-us/typography/opentype/spec/colr
//
// A COLR table maps a base glyph (the one that cmap+GSUB produce) to an
// ordered list of layer glyphs. Each layer references a CPAL color by
// index. To render the base glyph, draw each layer at the same origin in
// its CPAL color, then advance by the base glyph's normal hmtx width.
//
// COLRv1 (paint trees, gradients, transforms, compositing) is OUT OF SCOPE
// here: we parse the v1 BaseGlyphList only enough to detect "has v1 data";
// rendering of v1 glyphs is not provided and callers should fall through to
// the plain glyf/CFF outline.

// TagCOLR is the OpenType tag for the COLR table.
var TagCOLR = MakeTag('C', 'O', 'L', 'R')

// ColorLayer is one entry in the per-glyph layer list returned by
// COLR.GlyphLayers.
//
// HarfBuzz equivalent: struct hb_ot_color_layer_t (hb-ot-color.h:111-114).
//
// A ColorIndex of 0xFFFF is the spec-defined sentinel meaning "use the
// caller's foreground color"; the renderer, not the parser, resolves it.
// HarfBuzz documents this at OT/Color/COLR/COLR.hh:234-243 and exposes
// hb_paint_context_t::foreground (OT/Color/COLR/COLR.hh:80) for the same
// purpose.
type ColorLayer struct {
	GlyphID    GlyphID
	ColorIndex uint16
}

// ForegroundColorIndex is the LayerRecord.colorIdx sentinel meaning "use
// the text foreground color, do not index into CPAL".
//
// HarfBuzz equivalent: special value 0xFFFF documented at
// OT/Color/COLR/COLR.hh:236-242.
const ForegroundColorIndex uint16 = 0xFFFF

// baseGlyphRecord mirrors the on-disk BaseGlyphRecord struct (6 bytes).
//
// HarfBuzz equivalent: struct BaseGlyphRecord (OT/Color/COLR/COLR.hh:248-270).
type baseGlyphRecord struct {
	glyphID       GlyphID
	firstLayerIdx uint16
	numLayers     uint16
}

// layerRecord mirrors the on-disk LayerRecord struct (4 bytes).
//
// HarfBuzz equivalent: struct LayerRecord (OT/Color/COLR/COLR.hh:222-246).
type layerRecord struct {
	glyphID  GlyphID
	colorIdx uint16
}

// COLR holds a parsed COLR table (v0 records only; v1 detection only).
//
// HarfBuzz equivalent: struct COLR (OT/Color/COLR/COLR.hh:2089-2784).
//
// The on-disk fixed header (DEFINE_SIZE_MIN(14) at COLR.hh:2783) is:
//
//	uint16 version
//	uint16 numBaseGlyphs
//	uint32 baseGlyphsZ  (NNOffset32To<SortedUnsizedArrayOf<BaseGlyphRecord>>)
//	uint32 layersZ      (NNOffset32To<UnsizedArrayOf<LayerRecord>>)
//	uint16 numLayers
//	-- v1 additions (we only test for presence) --
//	uint32 baseGlyphList    (Offset32To<BaseGlyphList>)
//	uint32 layerList        (Offset32To<LayerList>)
//	uint32 clipList         (Offset32To<ClipList>; nullable)
//	uint32 varIdxMap        (Offset32To<DeltaSetIndexMap>; nullable)
//	uint32 varStore         (Offset32To<ItemVariationStore>)
//
// baseGlyphRecords is held sorted by glyphID (the on-disk type is
// SortedUnsizedArrayOf<BaseGlyphRecord>), enabling binary search.
type COLR struct {
	baseGlyphRecords []baseGlyphRecord
	layerRecords     []layerRecord
	hasV1            bool

	Version uint16
}

// ParseCOLR parses a COLR table. Only the v0 portion is decoded; v1 data is
// detected (so callers can fall back to glyf/CFF) but its paint trees are
// not parsed.
//
// HarfBuzz equivalent: COLR::sanitize (OT/Color/COLR/COLR.hh:2333-2349) plus
// the array materialization at OT/Color/COLR/COLR.hh:2110-2114.
func ParseCOLR(data []byte) (*COLR, error) {
	if len(data) < 14 {
		return nil, ErrInvalidTable
	}

	c := &COLR{
		Version: binary.BigEndian.Uint16(data[0:]),
	}
	numBaseGlyphs := binary.BigEndian.Uint16(data[2:])
	baseGlyphsOff := binary.BigEndian.Uint32(data[4:])
	layersOff := binary.BigEndian.Uint32(data[8:])
	numLayers := binary.BigEndian.Uint16(data[12:])

	// Read layer records (4 bytes each).
	// HarfBuzz equivalent: (this+layersZ).as_array(numLayers) at
	// OT/Color/COLR/COLR.hh:2112.
	layersEnd := int(layersOff) + int(numLayers)*4
	if layersEnd > len(data) {
		return nil, ErrInvalidTable
	}
	c.layerRecords = make([]layerRecord, numLayers)
	for i := range int(numLayers) {
		base := int(layersOff) + i*4
		c.layerRecords[i] = layerRecord{
			glyphID:  binary.BigEndian.Uint16(data[base:]),
			colorIdx: binary.BigEndian.Uint16(data[base+2:]),
		}
	}

	// Read base-glyph records (6 bytes each, sorted by glyphID).
	// HarfBuzz equivalent: (this+baseGlyphsZ).as_array(numBaseGlyphs) at
	// OT/Color/COLR/COLR.hh:2255 — the on-disk type is
	// SortedUnsizedArrayOf<BaseGlyphRecord> (OT/Color/COLR/COLR.hh:2771).
	baseEnd := int(baseGlyphsOff) + int(numBaseGlyphs)*6
	if baseEnd > len(data) {
		return nil, ErrInvalidTable
	}
	c.baseGlyphRecords = make([]baseGlyphRecord, numBaseGlyphs)
	for i := range int(numBaseGlyphs) {
		base := int(baseGlyphsOff) + i*6
		c.baseGlyphRecords[i] = baseGlyphRecord{
			glyphID:       binary.BigEndian.Uint16(data[base:]),
			firstLayerIdx: binary.BigEndian.Uint16(data[base+2:]),
			numLayers:     binary.BigEndian.Uint16(data[base+4:]),
		}
	}

	// Detect COLRv1: presence of a non-zero baseGlyphList offset whose
	// target has non-empty numBaseGlyphPaintRecords.
	// HarfBuzz equivalent: COLR::has_v1_data (OT/Color/COLR/COLR.hh:2096-2103).
	// We do NOT parse the v1 paint tree here.
	if c.Version >= 1 && len(data) >= 18 {
		baseGlyphListOff := binary.BigEndian.Uint32(data[14:])
		if baseGlyphListOff != 0 && int(baseGlyphListOff)+4 <= len(data) {
			// BaseGlyphList starts with a uint32 numBaseGlyphPaintRecords.
			n := binary.BigEndian.Uint32(data[int(baseGlyphListOff):])
			c.hasV1 = n > 0
		}
	}

	return c, nil
}

// HasV0Data returns true if the table carries at least one COLRv0 base
// glyph record.
//
// HarfBuzz equivalent: COLR::has_v0_data (OT/Color/COLR/COLR.hh:2095).
func (c *COLR) HasV0Data() bool {
	return len(c.baseGlyphRecords) > 0
}

// HasV1Data returns true if the table carries v1 paint-tree base glyphs.
// The v1 paint tree itself is NOT parsed by this package; callers that see
// HasV1Data() == true and HasV0Data() == false for a given glyph should
// fall back to the plain outline.
//
// HarfBuzz equivalent: COLR::has_v1_data (OT/Color/COLR/COLR.hh:2096-2103).
func (c *COLR) HasV1Data() bool {
	return c.hasV1
}

// HasData returns true if the table has any color data at all (v0 or v1).
//
// HarfBuzz equivalent: COLR::has_data (OT/Color/COLR/COLR.hh:2093).
func (c *COLR) HasData() bool {
	return c.HasV0Data() || c.HasV1Data()
}

// GlyphLayers returns the v0 layer list for the given base glyph, or nil if
// no v0 record covers that glyph. Each returned ColorLayer carries the
// layer glyph ID and a CPAL color index (which may be ForegroundColorIndex
// = 0xFFFF, see ColorLayer doc).
//
// HarfBuzz equivalent: COLR::get_glyph_layers
// (OT/Color/COLR/COLR.hh:2105-2122). The HB version paginates output via a
// start_offset/count pair for C-API memory management; we return a slice
// because Go callers do not need that.
//
// Algorithm (HB lines 2110-2114):
//  1. Binary-search baseGlyphRecords (sorted by glyphID) for the record
//     whose glyphID == gid.
//  2. The record's [firstLayerIdx, firstLayerIdx+numLayers) sub-range of
//     layerRecords is this glyph's layer list.
func (c *COLR) GlyphLayers(gid GlyphID) []ColorLayer {
	if len(c.baseGlyphRecords) == 0 {
		return nil
	}
	// sort.Search returns the smallest index i such that the predicate is
	// true; combined with the explicit equality check this is binary
	// search on a sorted array — matching HB's hb_array_t::bsearch at
	// OT/Color/COLR/COLR.hh:2110.
	idx := sort.Search(len(c.baseGlyphRecords), func(i int) bool {
		return c.baseGlyphRecords[i].glyphID >= gid
	})
	if idx >= len(c.baseGlyphRecords) || c.baseGlyphRecords[idx].glyphID != gid {
		return nil
	}
	rec := c.baseGlyphRecords[idx]
	if int(rec.firstLayerIdx)+int(rec.numLayers) > len(c.layerRecords) {
		return nil
	}
	out := make([]ColorLayer, rec.numLayers)
	for i := uint16(0); i < rec.numLayers; i++ {
		lr := c.layerRecords[int(rec.firstLayerIdx)+int(i)]
		out[i] = ColorLayer{GlyphID: lr.glyphID, ColorIndex: lr.colorIdx}
	}
	return out
}
