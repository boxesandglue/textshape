package ot

import (
	"strconv"
	"strings"
)

// tagFromString converts a 4-character string to a Tag.
// If the string is shorter than 4 characters, it's padded with spaces.
func tagFromString(s string) Tag {
	var b [4]byte
	for i := 0; i < 4; i++ {
		if i < len(s) {
			b[i] = s[i]
		} else {
			b[i] = ' '
		}
	}
	return MakeTag(b[0], b[1], b[2], b[3])
}

// Feature represents an OpenType feature with optional range.
// This matches HarfBuzz's hb_feature_t structure.
type Feature struct {
	Tag   Tag    // Feature tag (e.g., TagKern, TagLiga)
	Value uint32 // 0 = off, 1 = on, >1 for alternates
	Start uint   // Cluster start (inclusive), FeatureGlobalStart for beginning
	End   uint   // Cluster end (exclusive), FeatureGlobalEnd for end

	// Internal flags matching HarfBuzz F_PER_SYLLABLE and F_MANUAL_ZWJ.
	// HarfBuzz: hb_ot_map_feature_flags_t in hb-ot-map.hh
	PerSyllable bool // F_PER_SYLLABLE: constrain lookup application to syllable boundaries
	ManualZWJ   bool // F_MANUAL_ZWJ: disable automatic ZWJ handling (AutoZWJ = !ManualZWJ)
	Random      bool // F_RANDOM: use random alternate selection (for 'rand' feature)
}

const (
	// FeatureGlobalStart indicates feature applies from buffer start.
	FeatureGlobalStart uint = 0
	// FeatureGlobalEnd indicates feature applies to buffer end.
	FeatureGlobalEnd uint = ^uint(0)
)

// NewFeature creates a feature that applies globally (entire buffer).
func NewFeature(tag Tag, value uint32) Feature {
	return Feature{
		Tag:   tag,
		Value: value,
		Start: FeatureGlobalStart,
		End:   FeatureGlobalEnd,
	}
}

// NewFeatureOn creates a feature that is enabled globally.
func NewFeatureOn(tag Tag) Feature {
	return NewFeature(tag, 1)
}

// NewFeatureOff creates a feature that is disabled globally.
func NewFeatureOff(tag Tag) Feature {
	return NewFeature(tag, 0)
}

// IsGlobal returns true if the feature applies to the entire buffer.
func (f Feature) IsGlobal() bool {
	return f.Start == FeatureGlobalStart && f.End == FeatureGlobalEnd
}

// FeatureFromString parses a feature string like HarfBuzz.
// Supported formats:
//   - "kern"           -> kern=1 (on)
//   - "kern=1"         -> kern=1 (on)
//   - "kern=0"         -> kern=0 (off)
//   - "-kern"          -> kern=0 (off)
//   - "+kern"          -> kern=1 (on)
//   - "aalt=2"         -> aalt=2 (alternate #2)
//   - "kern[3:5]"      -> kern=1 for clusters 3-5
//   - "kern[3:5]=0"    -> kern=0 for clusters 3-5
//   - "kern[3:]"       -> kern=1 from cluster 3 to end
//   - "kern[:5]"       -> kern=1 from start to cluster 5
//
// Returns false if the string cannot be parsed.
func FeatureFromString(s string) (Feature, bool) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return Feature{}, false
	}

	f := Feature{
		Value: 1,
		Start: FeatureGlobalStart,
		End:   FeatureGlobalEnd,
	}

	// Handle +/- prefix
	if s[0] == '-' {
		f.Value = 0
		s = s[1:]
	} else if s[0] == '+' {
		f.Value = 1
		s = s[1:]
	}

	if len(s) == 0 {
		return Feature{}, false
	}

	// Find tag (4 chars or until special char)
	tagEnd := 0
	for tagEnd < len(s) && tagEnd < 4 {
		c := s[tagEnd]
		if c == '=' || c == '[' {
			break
		}
		tagEnd++
	}

	if tagEnd == 0 {
		return Feature{}, false
	}

	// Parse tag - pad with spaces if shorter than 4 chars
	tagStr := s[:tagEnd]
	f.Tag = tagFromString(tagStr)
	s = s[tagEnd:]

	// Parse optional range [start:end]
	if len(s) > 0 && s[0] == '[' {
		endBracket := strings.Index(s, "]")
		if endBracket == -1 {
			return Feature{}, false
		}
		rangeStr := s[1:endBracket]
		s = s[endBracket+1:]

		colonIdx := strings.Index(rangeStr, ":")
		if colonIdx == -1 {
			// Single index [n] means [n:n+1]
			n, err := strconv.ParseUint(rangeStr, 10, 64)
			if err != nil {
				return Feature{}, false
			}
			f.Start = uint(n)
			f.End = uint(n + 1)
		} else {
			// Range [start:end]
			startStr := rangeStr[:colonIdx]
			endStr := rangeStr[colonIdx+1:]

			if startStr != "" {
				n, err := strconv.ParseUint(startStr, 10, 64)
				if err != nil {
					return Feature{}, false
				}
				f.Start = uint(n)
			}
			if endStr != "" {
				n, err := strconv.ParseUint(endStr, 10, 64)
				if err != nil {
					return Feature{}, false
				}
				f.End = uint(n)
			}
		}
	}

	// Parse optional =value
	if len(s) > 0 && s[0] == '=' {
		s = s[1:]
		// Handle "on"/"off" or numeric value
		switch strings.ToLower(s) {
		case "on", "true", "yes":
			f.Value = 1
		case "off", "false", "no":
			f.Value = 0
		default:
			n, err := strconv.ParseUint(s, 10, 32)
			if err != nil {
				return Feature{}, false
			}
			f.Value = uint32(n)
		}
	}

	return f, true
}

// String returns a string representation of the feature.
func (f Feature) String() string {
	var sb strings.Builder

	// Tag
	sb.WriteString(f.Tag.String())

	// Range (only if not global)
	if !f.IsGlobal() {
		sb.WriteByte('[')
		if f.Start != FeatureGlobalStart {
			sb.WriteString(strconv.FormatUint(uint64(f.Start), 10))
		}
		sb.WriteByte(':')
		if f.End != FeatureGlobalEnd {
			sb.WriteString(strconv.FormatUint(uint64(f.End), 10))
		}
		sb.WriteByte(']')
	}

	// Value (only if not 1)
	if f.Value != 1 {
		sb.WriteByte('=')
		sb.WriteString(strconv.FormatUint(uint64(f.Value), 10))
	}

	return sb.String()
}

// mergedFeatureInfo contains the result of HarfBuzz-style feature merging.
// HarfBuzz equivalent: feature_info_t after compile() merging in hb-ot-map.cc:213-240
type mergedFeatureInfo struct {
	MaxValue     uint32 // 0 = disabled globally
	DefaultValue uint32 // default value for glyphs (from earlier global entry)
	IsGlobal     bool   // true if feature applies uniformly to all glyphs
}

// mergeFeatureInfo determines the effective feature parameters after applying
// HarfBuzz-style merging of a default (on) feature with user overrides.
// HarfBuzz equivalent: feature merging in hb-ot-map.cc:213-240
func mergeFeatureInfo(tag Tag, userFeatures []Feature) mergedFeatureInfo {
	// Default: feature is on (global, value=1)
	info := mergedFeatureInfo{
		MaxValue:     1,
		DefaultValue: 1,
		IsGlobal:     true,
	}

	for _, f := range userFeatures {
		if f.Tag != tag {
			continue
		}
		fIsGlobal := f.IsGlobal() || (f.Start == 0 && f.End == 0)
		if fIsGlobal {
			// Later global overrides earlier: take its value
			// HarfBuzz: hb-ot-map.cc:226-228
			info.IsGlobal = true
			info.MaxValue = f.Value
			info.DefaultValue = f.Value
		} else {
			// Later non-global: remove F_GLOBAL, keep default_value
			// HarfBuzz: hb-ot-map.cc:230-237
			info.IsGlobal = false
			if f.Value > info.MaxValue {
				info.MaxValue = f.Value
			}
			// default_value stays from the earlier entry
		}
	}

	return info
}

// mergedFeatureValue is a convenience wrapper that returns the effective global
// value of a feature. Returns 0 if the feature is globally disabled.
func mergedFeatureValue(tag Tag, userFeatures []Feature) uint32 {
	info := mergeFeatureInfo(tag, userFeatures)
	if info.MaxValue == 0 {
		return 0
	}
	return info.DefaultValue
}

// applyFeatureWithMergedMask applies a single feature with HarfBuzz-style per-cluster
// mask support. It allocates a mask bit starting at nextBit for non-global features,
// sets up per-glyph masks, and applies the feature with the correct mask.
// HarfBuzz equivalent: mask allocation in hb-ot-map.cc:250-324 + setup_masks in hb-ot-shape.cc:771-779
//
// Returns the updated nextBit value.
func applyFeatureWithMergedMask(
	tag Tag,
	userFeatures []Feature,
	nextBit uint,
	buf *Buffer,
	gsub *GSUB,
	gdef *GDEF,
	font *Font,
) uint {
	info := mergeFeatureInfo(tag, userFeatures)

	if info.MaxValue == 0 {
		// Feature globally disabled (e.g., -calt)
		return nextBit
	}

	if info.IsGlobal && info.MaxValue == 1 {
		// Simple case: feature applies to all glyphs with MaskGlobal
		// HarfBuzz: only use global bit when max_value == 1 (hb-ot-map.cc:312)
		gsub.ApplyFeatureToBufferWithMask(tag, buf, gdef, MaskGlobal, font)
		return nextBit
	}

	// Non-global: allocate dedicated mask bit for per-cluster ranges
	// HarfBuzz: hb-ot-map.cc:316-321
	bitsNeeded := bitStorage(info.MaxValue)
	if bitsNeeded > 8 {
		bitsNeeded = 8
	}
	if nextBit+bitsNeeded >= 31 {
		// Not enough bits, fall back to global application
		gsub.ApplyFeatureToBufferWithMask(tag, buf, gdef, MaskGlobal, font)
		return nextBit
	}

	mask := uint32((1<<(nextBit+bitsNeeded) - 1) &^ (1<<nextBit - 1))
	shift := nextBit

	// Set default value on all glyphs
	// HarfBuzz: global_mask |= (default_value << shift) & mask
	if info.DefaultValue > 0 {
		defaultMaskValue := (info.DefaultValue << shift) & mask
		for i := range buf.Info {
			buf.Info[i].Mask |= defaultMaskValue
		}
	}

	// Apply per-cluster range overrides from user features
	// HarfBuzz: hb-ot-shape.cc:771-779
	for _, f := range userFeatures {
		if f.Tag != tag {
			continue
		}
		fIsGlobal := f.IsGlobal() || (f.Start == 0 && f.End == 0)
		if fIsGlobal {
			continue
		}
		buf.SetMasksForClusterRange(f.Value<<shift, mask, int(f.Start), int(f.End))
	}

	// Apply feature with the allocated mask
	gsub.ApplyFeatureToBufferWithMask(tag, buf, gdef, mask, font)

	return nextBit + bitsNeeded
}

// uniqueFeatureTags returns the unique tags from a feature list (preserving order of first occurrence).
func uniqueFeatureTags(features []Feature) []Tag {
	seen := make(map[Tag]bool)
	var tags []Tag
	for _, f := range features {
		if !seen[f.Tag] {
			seen[f.Tag] = true
			tags = append(tags, f.Tag)
		}
	}
	return tags
}

// ParseFeatures parses a comma-separated list of features.
func ParseFeatures(s string) []Feature {
	if s == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	features := make([]Feature, 0, len(parts))

	for _, part := range parts {
		if f, ok := FeatureFromString(part); ok {
			features = append(features, f)
		}
	}

	return features
}

// DefaultFeatures returns the default features for shaping.
// HarfBuzz equivalent: common_features[] in hb-ot-shape.cc:295-305
// Note: 'locl' is applied separately in applyGSUB to ensure it only uses LangSys features.
func DefaultFeatures() []Feature {
	return []Feature{
		// GSUB features
		NewFeatureOn(TagCcmp), // Glyph Composition/Decomposition
		NewFeatureOn(TagRlig), // Required Ligatures
		NewFeatureOn(TagCalt), // Contextual Alternates
		NewFeatureOn(TagLiga), // Standard Ligatures
		NewFeatureOn(TagClig), // Contextual Ligatures
		// HarfBuzz: enable_feature('rand', F_RANDOM, HB_OT_MAP_MAX_VALUE)
		// Random alternate selection — global with value=MAX_VALUE.
		{Tag: MakeTag('r', 'a', 'n', 'd'), Value: otMapMaxValue, Random: true,
			Start: FeatureGlobalStart, End: FeatureGlobalEnd},
		// GPOS features - HarfBuzz common_features[] and horizontal_features[]
		// See hb-ot-shape.cc:295-318
		NewFeatureOn(TagAbvm), // Above-base Mark Positioning
		NewFeatureOn(TagBlwm), // Below-base Mark Positioning
		NewFeatureOn(TagMark), // Mark Positioning
		NewFeatureOn(TagMkmk), // Mark to Mark Positioning
		NewFeatureOn(TagCurs), // Cursive Positioning (for Arabic, etc.)
		NewFeatureOn(TagDist), // Distances
		NewFeatureOn(TagKern), // Kerning
	}
}
