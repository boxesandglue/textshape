package ot

// Feature mask system for controlling which features apply to which glyphs.
//
// HarfBuzz equivalent: hb-ot-map.cc / hb-ot-map.hh
//
// In HarfBuzz:
// - Each feature gets a unique mask bit (or range of bits for multi-valued features)
// - Glyphs have a mask field that indicates which features should apply
// - During lookup matching: if (!(glyph.mask & lookup.mask)) return NO_MATCH
//
// Our simplified approach:
// - Bit 0 (GlobalMask) is set for all glyphs - global features apply
// - Bits 1-7 are for Arabic positional features (isol, init, medi, fina, med2, fin2, fin3)
// - This allows lookups to be filtered based on the joining action

// Mask constants for feature filtering
const (
	// MaskGlobal is set on all glyphs. Features like ccmp, rlig, calt, liga use this.
	// HarfBuzz: global_mask = 1 << 31 (highest bit, so lower bits can be used for features)
	MaskGlobal uint32 = 1 << 31

	// Arabic positional feature masks
	// HarfBuzz: arabic_features[] in hb-ot-shaper-arabic.cc maps actions to features
	MaskIsol uint32 = 1 << 1 // Isolated form
	MaskFina uint32 = 1 << 2 // Final form
	MaskFin2 uint32 = 1 << 3 // Terminal form (Syriac)
	MaskFin3 uint32 = 1 << 4 // Terminal form (Syriac)
	MaskMedi uint32 = 1 << 5 // Medial form
	MaskMed2 uint32 = 1 << 6 // Medial form (Syriac)
	MaskInit uint32 = 1 << 7 // Initial form

	// Combined mask for all Arabic positional features
	MaskArabicPositional = MaskIsol | MaskFina | MaskFin2 | MaskFin3 | MaskMedi | MaskMed2 | MaskInit

	// Note: Indic feature masks are now generated dynamically in IndicPlan.maskArray
	// See indic.go: newIndicPlan() and indicFeatures[]
)

// FeatureMap maps feature tags to their mask values.
// This is used to determine which mask a lookup should use.
type FeatureMap struct {
	masks map[Tag]uint32
}

// NewFeatureMap creates a new feature map with default mask allocations.
func NewFeatureMap() *FeatureMap {
	fm := &FeatureMap{
		masks: make(map[Tag]uint32),
	}

	// Global features (apply to all glyphs with MaskGlobal)
	globalFeatures := []Tag{
		MakeTag('c', 'c', 'm', 'p'), // Glyph Composition/Decomposition
		MakeTag('l', 'o', 'c', 'l'), // Localized Forms
		MakeTag('r', 'l', 'i', 'g'), // Required Ligatures
		MakeTag('c', 'a', 'l', 't'), // Contextual Alternates
		MakeTag('l', 'i', 'g', 'a'), // Standard Ligatures
		MakeTag('c', 'l', 'i', 'g'), // Contextual Ligatures
		MakeTag('k', 'e', 'r', 'n'), // Kerning
		MakeTag('m', 'a', 'r', 'k'), // Mark Positioning
		MakeTag('m', 'k', 'm', 'k'), // Mark-to-Mark Positioning
		MakeTag('c', 'u', 'r', 's'), // Cursive Positioning
		MakeTag('d', 'i', 's', 't'), // Distances
		MakeTag('a', 'b', 'v', 'm'), // Above-base Mark Positioning
		MakeTag('b', 'l', 'w', 'm'), // Below-base Mark Positioning
		// RTL features
		MakeTag('r', 't', 'l', 'a'), // Right-to-Left Alternates
		MakeTag('r', 't', 'l', 'm'), // Right-to-Left Mirroring
		// LTR features
		MakeTag('l', 't', 'r', 'a'), // Left-to-Right Alternates
		MakeTag('l', 't', 'r', 'm'), // Left-to-Right Mirroring
		// Fraction features
		MakeTag('f', 'r', 'a', 'c'), // Fractions
		MakeTag('n', 'u', 'm', 'r'), // Numerators
		MakeTag('d', 'n', 'o', 'm'), // Denominators
	}
	for _, tag := range globalFeatures {
		fm.masks[tag] = MaskGlobal
	}

	// Arabic positional features (only apply to glyphs with specific action)
	fm.masks[MakeTag('i', 's', 'o', 'l')] = MaskIsol
	fm.masks[MakeTag('f', 'i', 'n', 'a')] = MaskFina
	fm.masks[MakeTag('f', 'i', 'n', '2')] = MaskFin2
	fm.masks[MakeTag('f', 'i', 'n', '3')] = MaskFin3
	fm.masks[MakeTag('m', 'e', 'd', 'i')] = MaskMedi
	fm.masks[MakeTag('m', 'e', 'd', '2')] = MaskMed2
	fm.masks[MakeTag('i', 'n', 'i', 't')] = MaskInit

	return fm
}

// GetMask returns the mask for a feature tag.
// Features not in the map default to MaskGlobal.
func (fm *FeatureMap) GetMask(tag Tag) uint32 {
	if mask, ok := fm.masks[tag]; ok {
		return mask
	}
	return MaskGlobal
}

// IsPositionalFeature returns true if the tag is an Arabic positional feature.
func IsPositionalFeature(tag Tag) bool {
	switch tag {
	case MakeTag('i', 's', 'o', 'l'),
		MakeTag('f', 'i', 'n', 'a'),
		MakeTag('f', 'i', 'n', '2'),
		MakeTag('f', 'i', 'n', '3'),
		MakeTag('m', 'e', 'd', 'i'),
		MakeTag('m', 'e', 'd', '2'),
		MakeTag('i', 'n', 'i', 't'):
		return true
	}
	return false
}

// Buffer mask helper methods

// ResetMasks sets all glyphs to the given mask.
// HarfBuzz equivalent: hb_buffer_t::reset_masks()
func (buf *Buffer) ResetMasks(mask uint32) {
	for i := range buf.Info {
		buf.Info[i].Mask = mask
	}
}

// AddMasks adds mask bits to all glyphs.
// HarfBuzz equivalent: hb_buffer_t::add_masks()
func (buf *Buffer) AddMasks(mask uint32) {
	for i := range buf.Info {
		buf.Info[i].Mask |= mask
	}
}

// SetMasksForClusterRange sets mask bits for glyphs in a cluster range.
// HarfBuzz equivalent: hb_buffer_t::set_masks()
func (buf *Buffer) SetMasksForClusterRange(value, mask uint32, clusterStart, clusterEnd int) {
	for i := range buf.Info {
		cluster := buf.Info[i].Cluster
		if cluster >= clusterStart && cluster < clusterEnd {
			buf.Info[i].Mask = (buf.Info[i].Mask & ^mask) | (value & mask)
		}
	}
}

// arabicActionToMask converts an Arabic joining action to its corresponding mask.
// HarfBuzz equivalent: arabic_plan->mask_array[action] in setup_masks_arabic_plan()
func arabicActionToMask(action ArabicAction) uint32 {
	switch action {
	case arabicActionISOL:
		return MaskIsol
	case arabicActionFINA:
		return MaskFina
	case arabicActionFIN2:
		return MaskFin2
	case arabicActionFIN3:
		return MaskFin3
	case arabicActionMEDI:
		return MaskMedi
	case arabicActionMED2:
		return MaskMed2
	case arabicActionINIT:
		return MaskInit
	default:
		return 0 // No positional feature for this action
	}
}
