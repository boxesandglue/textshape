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
// Note: 'locl' (Localized Forms) is NOT included here because it should only
// be applied when the font has it registered for the current script/language.
// HarfBuzz applies features based on script/language registration, which
// textshape doesn't fully implement yet.
func DefaultFeatures() []Feature {
	return []Feature{
		// GSUB features
		NewFeatureOn(TagCcmp), // Glyph Composition/Decomposition
		NewFeatureOn(TagRlig), // Required Ligatures
		NewFeatureOn(TagCalt), // Contextual Alternates
		NewFeatureOn(TagLiga), // Standard Ligatures
		NewFeatureOn(TagClig), // Contextual Ligatures
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
