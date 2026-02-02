package ot

import (
	"fmt"
	"sort"
)

// Debug flag for Indic shaper
var debugIndic = false

// Script tags for Indic scripts
// HarfBuzz equivalent: HB_SCRIPT_MALAYALAM, HB_SCRIPT_TAMIL, etc.
var (
	TagMlym = MakeTag('M', 'l', 'y', 'm') // Malayalam
	TagTaml = MakeTag('T', 'a', 'm', 'l') // Tamil
)

// isHalant checks if a glyph is a halant (virama) and not ligated.
// HarfBuzz equivalent: is_halant() in hb-ot-shaper-indic.cc:85-88
// Uses is_one_of() which returns false for ligated glyphs (line 57).
func isHalant(buf *Buffer, i int) bool {
	// HarfBuzz is_one_of(): if ligated, return false
	if (buf.Info[i].GlyphProps & GlyphPropsLigated) != 0 {
		return false
	}
	return IndicCategory(buf.Info[i].IndicCategory) == ICatH
}

// IndicFeatureIndex is an index into the indicFeatures array and maskArray.
// HarfBuzz equivalent: enum in hb-ot-shaper-indic.cc:202-223
// Must be in the same order as indicFeatures array.
type IndicFeatureIndex int

const (
	indicNukt IndicFeatureIndex = iota
	indicAkhn
	indicRphf
	indicRkrf
	indicPref
	indicBlwf
	indicAbvf
	indicHalf
	indicPstf
	indicVatu
	indicCjct

	indicInit
	indicPres
	indicAbvs
	indicBlws
	indicPsts
	indicHaln

	indicNumFeatures
	indicBasicFeatures = indicInit // Don't forget to update this!
)

// indicFeatureFlags defines how features are applied.
// HarfBuzz equivalent: F_GLOBAL, F_MANUAL_JOINERS, F_PER_SYLLABLE in hb-ot-shaper-indic.cc
type indicFeatureFlags uint8

const (
	indicFlagGlobal      indicFeatureFlags = 1 << 0 // Feature applies globally (mask is 0, always matches)
	indicFlagManualZWNJ  indicFeatureFlags = 1 << 1 // HarfBuzz: F_MANUAL_ZWNJ — don't skip ZWNJ in context matching
	indicFlagManualZWJ   indicFeatureFlags = 1 << 2 // HarfBuzz: F_MANUAL_ZWJ — don't skip ZWJ in input matching
	indicFlagPerSyllable indicFeatureFlags = 1 << 3 // Applied per syllable

	// HarfBuzz: F_MANUAL_JOINERS = F_MANUAL_ZWNJ | F_MANUAL_ZWJ
	indicFlagManualJoiner indicFeatureFlags = indicFlagManualZWNJ | indicFlagManualZWJ
)

// indicFeature describes an Indic feature.
// HarfBuzz equivalent: hb_ot_map_feature_t in hb-ot-shaper-indic.cc:166-197
type indicFeature struct {
	tag   Tag
	flags indicFeatureFlags
}

// indicFeatures is the list of Indic features in application order.
// HarfBuzz equivalent: indic_features[] in hb-ot-shaper-indic.cc:166-197
var indicFeatures = [indicNumFeatures]indicFeature{
	// Basic features - applied in order, one at a time, after initial_reordering
	{MakeTag('n', 'u', 'k', 't'), indicFlagGlobal | indicFlagManualJoiner | indicFlagPerSyllable}, // nukt
	{MakeTag('a', 'k', 'h', 'n'), indicFlagGlobal | indicFlagManualJoiner | indicFlagPerSyllable}, // akhn
	{MakeTag('r', 'p', 'h', 'f'), indicFlagManualJoiner | indicFlagPerSyllable},                   // rphf
	{MakeTag('r', 'k', 'r', 'f'), indicFlagGlobal | indicFlagManualJoiner | indicFlagPerSyllable}, // rkrf
	{MakeTag('p', 'r', 'e', 'f'), indicFlagManualJoiner | indicFlagPerSyllable},                   // pref
	{MakeTag('b', 'l', 'w', 'f'), indicFlagManualJoiner | indicFlagPerSyllable},                   // blwf
	{MakeTag('a', 'b', 'v', 'f'), indicFlagManualJoiner | indicFlagPerSyllable},                   // abvf
	{MakeTag('h', 'a', 'l', 'f'), indicFlagManualJoiner | indicFlagPerSyllable},                   // half
	{MakeTag('p', 's', 't', 'f'), indicFlagManualJoiner | indicFlagPerSyllable},                   // pstf
	{MakeTag('v', 'a', 't', 'u'), indicFlagGlobal | indicFlagManualJoiner | indicFlagPerSyllable}, // vatu
	{MakeTag('c', 'j', 'c', 't'), indicFlagGlobal | indicFlagManualJoiner | indicFlagPerSyllable}, // cjct

	// Other features - applied all at once, after final_reordering
	{MakeTag('i', 'n', 'i', 't'), indicFlagManualJoiner | indicFlagPerSyllable},                   // init
	{MakeTag('p', 'r', 'e', 's'), indicFlagGlobal | indicFlagManualJoiner | indicFlagPerSyllable}, // pres
	{MakeTag('a', 'b', 'v', 's'), indicFlagGlobal | indicFlagManualJoiner | indicFlagPerSyllable}, // abvs
	{MakeTag('b', 'l', 'w', 's'), indicFlagGlobal | indicFlagManualJoiner | indicFlagPerSyllable}, // blws
	{MakeTag('p', 's', 't', 's'), indicFlagGlobal | indicFlagManualJoiner | indicFlagPerSyllable}, // psts
	{MakeTag('h', 'a', 'l', 'n'), indicFlagGlobal | indicFlagManualJoiner | indicFlagPerSyllable}, // haln
}

// IndicPlan holds pre-computed data for Indic shaping.
// HarfBuzz equivalent: indic_shape_plan_t in hb-ot-shaper-indic.cc:289-308
type IndicPlan struct {
	config    *IndicConfig
	isOldSpec bool
	viramaGID GlyphID

	// Feature masks - dynamically generated based on font features
	// HarfBuzz equivalent: mask_array[INDIC_NUM_FEATURES] in hb-ot-shaper-indic.cc:307
	maskArray [indicNumFeatures]uint32

	// Would-substitute feature testers
	// HarfBuzz equivalent: rphf, pref, blwf, pstf, vatu in hb-ot-shaper-indic.cc:299-305
	rphf indicWouldSubstitute
	pref indicWouldSubstitute
	blwf indicWouldSubstitute
	pstf indicWouldSubstitute
	vatu indicWouldSubstitute
}

// indicWouldSubstitute holds data for testing if a feature would substitute glyphs.
// HarfBuzz equivalent: hb_indic_would_substitute_feature_t
type indicWouldSubstitute struct {
	gsub        *GSUB
	tag         Tag
	zeroContext bool
}

// wouldSubstitute tests if the feature would substitute the given glyphs.
func (w *indicWouldSubstitute) wouldSubstitute(glyphs []GlyphID) bool {
	if w.gsub == nil {
		return false
	}
	return w.gsub.WouldSubstituteFeature(w.tag, glyphs, w.zeroContext)
}

// newIndicPlan creates and initializes an IndicPlan for the given script and font.
// HarfBuzz equivalent: data_create_indic() in hb-ot-shaper-indic.cc:312-347
func newIndicPlan(gsub *GSUB, script Tag, config *IndicConfig) *IndicPlan {
	plan := &IndicPlan{
		config:    config,
		viramaGID: 0, // Will be looked up lazily
	}

	// Determine old-spec vs new-spec
	// HarfBuzz: hb-ot-shaper-indic.cc:324
	var chosenTag Tag
	if gsub != nil {
		chosenTag = gsub.FindChosenScriptTag(script)
	}
	plan.isOldSpec = config.HasOldSpec && (byte(chosenTag&0xFF) != '2')

	// Use zero-context would_substitute() matching for new-spec of the main
	// Indic scripts, and scripts with one spec only, but not for old-specs.
	// HarfBuzz: hb-ot-shaper-indic.cc:336
	zeroContext := !plan.isOldSpec && script != MakeTag('M', 'l', 'y', 'm')

	// Initialize would-substitute testers
	plan.rphf = indicWouldSubstitute{gsub, MakeTag('r', 'p', 'h', 'f'), zeroContext}
	plan.pref = indicWouldSubstitute{gsub, MakeTag('p', 'r', 'e', 'f'), zeroContext}
	plan.blwf = indicWouldSubstitute{gsub, MakeTag('b', 'l', 'w', 'f'), zeroContext}
	plan.pstf = indicWouldSubstitute{gsub, MakeTag('p', 's', 't', 'f'), zeroContext}
	plan.vatu = indicWouldSubstitute{gsub, MakeTag('v', 'a', 't', 'u'), zeroContext}

	// Generate masks dynamically
	// HarfBuzz: hb-ot-shaper-indic.cc:343-345
	// For global features, mask is 0 (HarfBuzz: always matches when mask is 0).
	// For non-global features, we allocate a unique bit.
	nextBit := uint(8) // Start after Arabic positional masks (bits 1-7)
	for i := IndicFeatureIndex(0); i < indicNumFeatures; i++ {
		if indicFeatures[i].flags&indicFlagGlobal != 0 {
			plan.maskArray[i] = 0 // Global features: mask=0 means always match
		} else {
			plan.maskArray[i] = 1 << nextBit
			nextBit++
		}
	}

	return plan
}

// getIndicPlan returns the IndicPlan for the given script, creating one if necessary.
// HarfBuzz equivalent: accessing indic_plan via plan->data() in shaper functions
func (s *Shaper) getIndicPlan(script Tag, config *IndicConfig) *IndicPlan {
	if s.indicPlans == nil {
		s.indicPlans = make(map[Tag]*IndicPlan)
	}
	plan, ok := s.indicPlans[script]
	if !ok {
		plan = newIndicPlan(s.gsub, script, config)
		// Load virama glyph ID for halant recovery in final reordering
		// HarfBuzz equivalent: load_virama_glyph() in hb-ot-shaper-indic.cc
		if s.cmap != nil && config.Virama != 0 {
			plan.viramaGID, _ = s.cmap.Lookup(config.Virama)
		}
		s.indicPlans[script] = plan
	}
	return plan
}

// Indic shaper implementation based on HarfBuzz's hb-ot-shaper-indic.cc
//
// This implements the Indic shaping model for scripts like Devanagari, Bengali,
// Tamil, etc. The Indic shaper handles:
// - Syllable detection (via Ragel state machine in indic_machine.go)
// - Character reordering (initial and final)
// - Feature application in specific order

// indicSyllableAccessor implements SyllableAccessor for Indic shaper.
type indicSyllableAccessor struct {
	indicInfo []IndicInfo
}

func (a *indicSyllableAccessor) GetSyllable(i int) uint8 {
	return a.indicInfo[i].Syllable
}

func (a *indicSyllableAccessor) GetCategory(i int) uint8 {
	return uint8(a.indicInfo[i].Category)
}

func (a *indicSyllableAccessor) SetCategory(i int, cat uint8) {
	a.indicInfo[i].Category = IndicCategory(cat)
}

func (a *indicSyllableAccessor) Len() int {
	return len(a.indicInfo)
}

// IndicConfig holds script-specific configuration.
// HarfBuzz equivalent: indic_config_t in hb-ot-shaper-indic.hh
type IndicConfig struct {
	Script Tag

	// Virama codepoint for this script (halant/virama)
	Virama Codepoint

	// HasOldSpec indicates if this script uses old Indic spec shaping
	// (pre-OpenType 1.8 behavior)
	HasOldSpec bool

	// RephPos indicates where reph should be positioned
	RephPos IndicPosition

	// RephMode indicates how reph is formed
	RephMode RephMode

	// BlwfMode indicates how below-forms are handled
	BlwfMode BlwfMode

	// BasePos indicates how to find the base consonant
	BasePos BasePos
}

// RephMode indicates how reph is formed for a script.
// HarfBuzz equivalent: reph_mode_t
type RephMode uint8

const (
	RephModeImplicit RephMode = iota // Reph formed implicitly (Ra+H)
	RephModeExplicit                 // Reph formed explicitly (Ra+H+ZWJ)
	RephModeLogRepha                 // Reph formed by logical repha
)

// BlwfMode indicates how below-forms are handled.
// HarfBuzz equivalent: blwf_mode_t
type BlwfMode uint8

const (
	BlwfModePreAndPost BlwfMode = iota // Below-forms before and after base
	BlwfModePostOnly                   // Below-forms only after base
)

// BasePos indicates how to find the base consonant.
// HarfBuzz equivalent: base_pos_t
type BasePos uint8

const (
	BasePosLastSinhala BasePos = iota // Last consonant (Sinhala-style)
	BasePosLast                       // Last consonant
	BasePosFirst                      // First consonant (for some scripts)
)

// indicConfigs holds per-script configuration.
// HarfBuzz equivalent: indic_configs[] in hb-ot-shaper-indic.cc:149-162
// Note: has_old_spec is true for all scripts that have dual specs (old and new)
var indicConfigs = map[Tag]IndicConfig{
	MakeTag('D', 'e', 'v', 'a'): { // Devanagari
		Script:     MakeTag('D', 'e', 'v', 'a'),
		Virama:     0x094D,
		HasOldSpec: true, // HarfBuzz: true
		RephPos:    IPosBeforePost,
		RephMode:   RephModeImplicit,
		BlwfMode:   BlwfModePreAndPost,
		BasePos:    BasePosLast,
	},
	MakeTag('B', 'e', 'n', 'g'): { // Bengali
		Script:     MakeTag('B', 'e', 'n', 'g'),
		Virama:     0x09CD,
		HasOldSpec: true, // HarfBuzz: true
		RephPos:    IPosAfterSub,
		RephMode:   RephModeImplicit,
		BlwfMode:   BlwfModePreAndPost,
		BasePos:    BasePosLast,
	},
	MakeTag('G', 'u', 'r', 'u'): { // Gurmukhi
		Script:     MakeTag('G', 'u', 'r', 'u'),
		Virama:     0x0A4D,
		HasOldSpec: true, // HarfBuzz: true
		RephPos:    IPosBeforeSub,
		RephMode:   RephModeImplicit,
		BlwfMode:   BlwfModePreAndPost,
		BasePos:    BasePosLast,
	},
	MakeTag('G', 'u', 'j', 'r'): { // Gujarati
		Script:     MakeTag('G', 'u', 'j', 'r'),
		Virama:     0x0ACD,
		HasOldSpec: true, // HarfBuzz: true
		RephPos:    IPosBeforePost,
		RephMode:   RephModeImplicit,
		BlwfMode:   BlwfModePreAndPost,
		BasePos:    BasePosLast,
	},
	MakeTag('O', 'r', 'y', 'a'): { // Oriya
		Script:     MakeTag('O', 'r', 'y', 'a'),
		Virama:     0x0B4D,
		HasOldSpec: true, // HarfBuzz: true
		RephPos:    IPosAfterMain,
		RephMode:   RephModeImplicit,
		BlwfMode:   BlwfModePreAndPost,
		BasePos:    BasePosLast,
	},
	MakeTag('T', 'a', 'm', 'l'): { // Tamil
		Script:     MakeTag('T', 'a', 'm', 'l'),
		Virama:     0x0BCD,
		HasOldSpec: true, // HarfBuzz: true
		RephPos:    IPosAfterPost,
		RephMode:   RephModeImplicit,
		BlwfMode:   BlwfModePreAndPost,
		BasePos:    BasePosLast,
	},
	MakeTag('T', 'e', 'l', 'u'): { // Telugu
		Script:     MakeTag('T', 'e', 'l', 'u'),
		Virama:     0x0C4D,
		HasOldSpec: true, // HarfBuzz: true
		RephPos:    IPosAfterPost,
		RephMode:   RephModeExplicit,
		BlwfMode:   BlwfModePostOnly,
		BasePos:    BasePosLast,
	},
	MakeTag('K', 'n', 'd', 'a'): { // Kannada
		Script:     MakeTag('K', 'n', 'd', 'a'),
		Virama:     0x0CCD,
		HasOldSpec: true, // HarfBuzz: true
		RephPos:    IPosAfterPost,
		RephMode:   RephModeImplicit,
		BlwfMode:   BlwfModePostOnly,
		BasePos:    BasePosLast,
	},
	MakeTag('M', 'l', 'y', 'm'): { // Malayalam
		Script:     MakeTag('M', 'l', 'y', 'm'),
		Virama:     0x0D4D,
		HasOldSpec: true, // HarfBuzz: true
		RephPos:    IPosAfterMain,
		RephMode:   RephModeLogRepha,
		BlwfMode:   BlwfModePreAndPost,
		BasePos:    BasePosLast,
	},
	MakeTag('S', 'i', 'n', 'h'): { // Sinhala - no old spec
		Script:     MakeTag('S', 'i', 'n', 'h'),
		Virama:     0x0DCA,
		HasOldSpec: false, // HarfBuzz: Sinhala has no old spec
		RephPos:    IPosAfterPost,
		RephMode:   RephModeExplicit,
		BlwfMode:   BlwfModePreAndPost,
		BasePos:    BasePosLastSinhala,
	},
}

// getIndicConfig returns the configuration for a script.
func getIndicConfig(script Tag) *IndicConfig {
	if config, ok := indicConfigs[script]; ok {
		return &config
	}
	// Default config for unknown scripts
	return &IndicConfig{
		Script:     script,
		Virama:     0,
		HasOldSpec: false,
		RephPos:    IPosBeforePost,
		RephMode:   RephModeImplicit,
		BlwfMode:   BlwfModePreAndPost,
		BasePos:    BasePosLast,
	}
}

// isOldSpecIndic returns true if the chosen script tag indicates old-spec shaping.
// HarfBuzz equivalent: hb-ot-shaper-indic.cc:324
// indic_plan->is_old_spec = indic_plan->config->has_old_spec && ((plan->map.chosen_script[0] & 0x000000FFu) != '2');
func isOldSpecIndic(config *IndicConfig, chosenScriptTag Tag) bool {
	if !config.HasOldSpec {
		return false
	}
	// Check if the chosen tag ends with '2' (new spec)
	lastByte := byte(chosenScriptTag & 0xFF)
	return lastByte != '2'
}

// IndicShapingInfo holds per-glyph shaping information.
type IndicShapingInfo struct {
	Category IndicCategory
	Position IndicPosition
	Syllable uint8
}

// hasIndicScript returns true if the buffer contains Indic script characters.
func (s *Shaper) hasIndicScript(buf *Buffer) bool {
	for _, info := range buf.Info {
		if isIndicScript(info.Codepoint) {
			return true
		}
	}
	return false
}

// isIndicScript returns true if the codepoint is in an Indic script.
func isIndicScript(cp Codepoint) bool {
	// Devanagari: U+0900-U+097F
	if cp >= 0x0900 && cp <= 0x097F {
		return true
	}
	// Bengali: U+0980-U+09FF
	if cp >= 0x0980 && cp <= 0x09FF {
		return true
	}
	// Gurmukhi: U+0A00-U+0A7F
	if cp >= 0x0A00 && cp <= 0x0A7F {
		return true
	}
	// Gujarati: U+0A80-U+0AFF
	if cp >= 0x0A80 && cp <= 0x0AFF {
		return true
	}
	// Oriya: U+0B00-U+0B7F
	if cp >= 0x0B00 && cp <= 0x0B7F {
		return true
	}
	// Tamil: U+0B80-U+0BFF
	if cp >= 0x0B80 && cp <= 0x0BFF {
		return true
	}
	// Telugu: U+0C00-U+0C7F
	if cp >= 0x0C00 && cp <= 0x0C7F {
		return true
	}
	// Kannada: U+0C80-U+0CFF
	if cp >= 0x0C80 && cp <= 0x0CFF {
		return true
	}
	// Malayalam: U+0D00-U+0D7F
	if cp >= 0x0D00 && cp <= 0x0D7F {
		return true
	}
	// Sinhala: U+0D80-U+0DFF
	if cp >= 0x0D80 && cp <= 0x0DFF {
		return true
	}
	return false
}

// getIndicScriptTag returns the OpenType script tag for a codepoint.
func getIndicScriptTag(cp Codepoint) Tag {
	switch {
	case cp >= 0x0900 && cp <= 0x097F:
		return MakeTag('D', 'e', 'v', 'a')
	case cp >= 0x0980 && cp <= 0x09FF:
		return MakeTag('B', 'e', 'n', 'g')
	case cp >= 0x0A00 && cp <= 0x0A7F:
		return MakeTag('G', 'u', 'r', 'u')
	case cp >= 0x0A80 && cp <= 0x0AFF:
		return MakeTag('G', 'u', 'j', 'r')
	case cp >= 0x0B00 && cp <= 0x0B7F:
		return MakeTag('O', 'r', 'y', 'a')
	case cp >= 0x0B80 && cp <= 0x0BFF:
		return MakeTag('T', 'a', 'm', 'l')
	case cp >= 0x0C00 && cp <= 0x0C7F:
		return MakeTag('T', 'e', 'l', 'u')
	case cp >= 0x0C80 && cp <= 0x0CFF:
		return MakeTag('K', 'n', 'd', 'a')
	case cp >= 0x0D00 && cp <= 0x0D7F:
		return MakeTag('M', 'l', 'y', 'm')
	case cp >= 0x0D80 && cp <= 0x0DFF:
		return MakeTag('S', 'i', 'n', 'h')
	default:
		return 0
	}
}

// detectIndicScript detects the Indic script from buffer content.
func (s *Shaper) detectIndicScript(buf *Buffer) Tag {
	for _, info := range buf.Info {
		if tag := getIndicScriptTag(info.Codepoint); tag != 0 {
			return tag
		}
	}
	return 0
}

// setupIndicProperties sets up Indic shaping properties for each glyph.
// HarfBuzz equivalent: setup_masks_indic() in hb-ot-shaper-indic.cc:391-406
func (s *Shaper) setupIndicProperties(buf *Buffer, config *IndicConfig) []IndicInfo {
	indicInfo := make([]IndicInfo, len(buf.Info))

	// Get the virama glyph ID for consonant position detection
	var viramaGlyph GlyphID
	if s.cmap != nil {
		viramaGlyph, _ = s.cmap.Lookup(config.Virama)
	}

	for i := range buf.Info {
		cp := buf.Info[i].Codepoint
		cat, pos := GetIndicCategories(cp)

		// Special handling for ZWJ/ZWNJ
		// HarfBuzz: set_indic_properties() in hb-ot-shaper-indic.cc:42-50
		if cp == 0x200D { // ZWJ
			cat = ICatZWJ
		} else if cp == 0x200C { // ZWNJ
			cat = ICatZWNJ
		} else if cp == 0x25CC { // Dotted Circle
			cat = ICatDOTTEDCIRCLE
		}

		// Malayalam Dot Reph (U+0D4E) is a logical Repha
		// HarfBuzz: REPH_MODE_LOG_REPHA in hb-ot-shaper-indic.cc:524-530
		if cp == 0x0D4E {
			cat = ICatRepha
			pos = IPosRaToBeReph
		}

		// Check for Ra (for reph formation)
		if cat == ICatC && isRa(cp, config.Script) {
			cat = ICatRa
		}

		// For consonants, determine position using would_substitute
		// HarfBuzz: set_indic_properties() calls consonant_position_from_face()
		if cat == ICatC || cat == ICatRa {
			pos = s.consonantPositionFromFace(buf.Info[i].GlyphID, viramaGlyph, config)
		}

		indicInfo[i].Category = cat
		indicInfo[i].Position = pos

		// Also store in GlyphInfo for persistence through GSUB substitutions
		// HarfBuzz stores these in var1 via HB_BUFFER_ALLOCATE_VAR
		buf.Info[i].IndicCategory = uint8(cat)
		buf.Info[i].IndicPosition = uint8(pos)
	}

	return indicInfo
}

// consonantPositionFromFace determines the position of a consonant based on font features.
// HarfBuzz equivalent: consonant_position_from_face() in hb-ot-shaper-indic.cc:356-389
//
// This checks if the font has blwf/pstf/pref lookups that would substitute the
// consonant+virama sequence, and returns the appropriate position.
func (s *Shaper) consonantPositionFromFace(consonant, virama GlyphID, config *IndicConfig) IndicPosition {
	if s.gsub == nil || virama == 0 {
		return IPosBaseC
	}

	// Build glyph sequences to test: [virama, consonant, virama]
	// We test both [virama, consonant] and [consonant, virama] orders
	// HarfBuzz: hb-ot-shaper-indic.cc:376-377
	glyphs := []GlyphID{virama, consonant, virama}

	// Check for below-base form (blwf or vatu)
	// HarfBuzz: hb-ot-shaper-indic.cc:377-381
	if s.gsub.WouldSubstituteFeature(tagBlwf, glyphs[0:2], true) ||
		s.gsub.WouldSubstituteFeature(tagBlwf, glyphs[1:3], true) ||
		s.gsub.WouldSubstituteFeature(tagVatu, glyphs[0:2], true) ||
		s.gsub.WouldSubstituteFeature(tagVatu, glyphs[1:3], true) {
		return IPosBelowC
	}

	// Check for post-base form (pstf)
	// HarfBuzz: hb-ot-shaper-indic.cc:382-384
	if s.gsub.WouldSubstituteFeature(tagPstf, glyphs[0:2], true) ||
		s.gsub.WouldSubstituteFeature(tagPstf, glyphs[1:3], true) {
		return IPosPostC
	}

	// Check for pre-base-reordering form (pref)
	// HarfBuzz: hb-ot-shaper-indic.cc:385-387
	if s.gsub.WouldSubstituteFeature(tagPref, glyphs[0:2], true) ||
		s.gsub.WouldSubstituteFeature(tagPref, glyphs[1:3], true) {
		return IPosPostC
	}

	// Default: base consonant
	return IPosBaseC
}

// isRa returns true if the codepoint is the Ra consonant for the given script.
func isRa(cp Codepoint, script Tag) bool {
	switch script {
	case MakeTag('D', 'e', 'v', 'a'):
		return cp == 0x0930 // DEVANAGARI LETTER RA
	case MakeTag('B', 'e', 'n', 'g'):
		return cp == 0x09B0 // BENGALI LETTER RA
	case MakeTag('G', 'u', 'r', 'u'):
		return cp == 0x0A30 // GURMUKHI LETTER RA
	case MakeTag('G', 'u', 'j', 'r'):
		return cp == 0x0AB0 // GUJARATI LETTER RA
	case MakeTag('O', 'r', 'y', 'a'):
		return cp == 0x0B30 // ORIYA LETTER RA
	case MakeTag('T', 'a', 'm', 'l'):
		return cp == 0x0BB0 // TAMIL LETTER RA
	case MakeTag('T', 'e', 'l', 'u'):
		return cp == 0x0C30 // TELUGU LETTER RA
	case MakeTag('K', 'n', 'd', 'a'):
		return cp == 0x0CB0 // KANNADA LETTER RA
	case MakeTag('M', 'l', 'y', 'm'):
		return cp == 0x0D30 // MALAYALAM LETTER RA
	case MakeTag('S', 'i', 'n', 'h'):
		return cp == 0x0DBB // SINHALA LETTER RAYANNA
	}
	return false
}

// findSyllablesIndic finds syllable boundaries in the buffer.
// It calls the Ragel-generated state machine.
func (s *Shaper) findSyllablesIndic(indicInfo []IndicInfo) bool {
	return FindSyllablesIndic(indicInfo)
}

// initialReorderingIndic performs initial reordering before GSUB features.
// HarfBuzz equivalent: initial_reordering_indic() in hb-ot-shaper-indic.cc
//
// This function:
// 1. Finds the base consonant in each syllable
// 2. Tags characters with their positions (pre-base, base, post-base, etc.)
// 3. Reorders characters within each syllable
// 4. Sets up feature masks based on position and old-spec/new-spec
func (s *Shaper) initialReorderingIndic(buf *Buffer, indicInfo []IndicInfo, config *IndicConfig, indicPlan *IndicPlan) {
	if len(buf.Info) == 0 {
		return
	}

	// Process each syllable
	start := 0
	for start < len(buf.Info) {
		// Find syllable end
		syllable := indicInfo[start].Syllable
		end := start + 1
		for end < len(buf.Info) && indicInfo[end].Syllable == syllable {
			end++
		}

		// Get syllable type
		syllableType := IndicSyllableType(syllable & 0x0F)

		// Reorder based on syllable type
		switch syllableType {
		case IndicConsonantSyllable:
			s.initialReorderingConsonantSyllable(buf, indicInfo, start, end, config, indicPlan)
		case IndicVowelSyllable:
			s.initialReorderingVowelSyllable(buf, indicInfo, start, end, config)
		case IndicStandaloneCluster:
			s.initialReorderingStandaloneCluster(buf, indicInfo, start, end, config, indicPlan)
		}

		start = end
	}
}

// initialReorderingConsonantSyllable reorders a consonant syllable.
// HarfBuzz equivalent: initial_reordering_consonant_syllable()
func (s *Shaper) initialReorderingConsonantSyllable(buf *Buffer, indicInfo []IndicInfo, start, end int, config *IndicConfig, indicPlan *IndicPlan) {
	// Kannada compatibility: Ra+H+ZWJ → Ra+ZWJ+H
	// HarfBuzz: hb-ot-shaper-indic.cc:466-478
	if config.Script == MakeTag('K', 'n', 'd', 'a') &&
		start+3 <= end &&
		indicInfo[start].Category == ICatRa &&
		indicInfo[start+1].Category == ICatH &&
		indicInfo[start+2].Category == ICatZWJ {
		buf.MergeClusters(start+1, start+3)
		buf.Info[start+1], buf.Info[start+2] = buf.Info[start+2], buf.Info[start+1]
		indicInfo[start+1], indicInfo[start+2] = indicInfo[start+2], indicInfo[start+1]
	}

	// Step 1: Find base consonant
	base := s.findBaseConsonant(buf, indicInfo, start, end, config, indicPlan)
	if base == start && indicInfo[start].Category == ICatRepha {
		// Repha at start - base is the next consonant
		for base = start + 1; base < end; base++ {
			if IsIndicConsonant(indicInfo[base].Category) {
				break
			}
		}
	}

	// Set base position
	if base < end {
		indicInfo[base].Position = IPosBaseC
		buf.Info[base].IndicPosition = uint8(IPosBaseC)
	}

	// Step 2: Classify consonant positions (pre-base clamping)
	// HarfBuzz: hb-ot-shaper-indic.cc:626-627
	s.classifyIndicConsonantPositions(buf, indicInfo, start, end, base, config)

	// Step 3: Handle reph (Ra+H at start) - MUST be before attachMiscMarks!
	// HarfBuzz: hb-ot-shaper-indic.cc:632-634
	s.handleReph(buf, indicInfo, start, end, base, config)

	// Step 3b: Attach misc marks to previous char (position inheritance)
	// HarfBuzz: hb-ot-shaper-indic.cc:685-728
	s.attachMiscMarks(buf, indicInfo, start, end, base, config)

	// Step 4: Pre-base matras - DON'T reorder here!
	// HarfBuzz: Pre-base matras are only positioned via stable_sort in initial_reordering,
	// and then ACTUALLY reordered in final_reordering AFTER pref-blocking check.
	// The stable_sort puts them in position order, but doesn't physically move them.
	// Moving them here would prevent correct pref-blocking behavior.

	// Note: Reph keeps IPosRaToBeReph during initial reordering.
	// The actual reph repositioning happens in final reordering (moveReph).
	// HarfBuzz: initial_reordering does NOT change reph position.

	// Step 5.5: Stable sort by indic_position (HarfBuzz: hb-ot-shaper-indic.cc:731-757)
	// This is CRITICAL for correct glyph ordering. HarfBuzz uses syllable() as a tie-breaker
	// to maintain stability (original order for equal positions).
	// Save syllable for later restore (HarfBuzz: hb-ot-shaper-indic.cc:733)
	syllable := buf.Info[start].Syllable
	base = s.stableSortIndicSyllable(buf, indicInfo, start, end)

	// Step 5.6: Old-spec Halant reordering (HarfBuzz: hb-ot-shaper-indic.cc:664-683)
	// For old-spec fonts, move Halant after the last consonant.
	// This is critical for correct ligature formation in old-spec fonts.
	if indicPlan.isOldSpec {
		disallowDoubleHalants := config.Script == MakeTag('K', 'n', 'd', 'a') // Kannada
		for i := base + 1; i < end; i++ {
			if indicInfo[i].Category == ICatH {
				// Find the last consonant (or halant if disallowed)
				j := end - 1
				for j > i {
					if IsIndicConsonant(indicInfo[j].Category) ||
						(disallowDoubleHalants && indicInfo[j].Category == ICatH) {
						break
					}
					j--
				}
				// Move halant to after last consonant if needed
				if indicInfo[j].Category != ICatH && j > i {
					// Save the halant
					tmpInfo := buf.Info[i]
					tmpIndicInfo := indicInfo[i]
					// Shift elements
					copy(buf.Info[i:j], buf.Info[i+1:j+1])
					copy(indicInfo[i:j], indicInfo[i+1:j+1])
					// Place halant at new position
					buf.Info[j] = tmpInfo
					indicInfo[j] = tmpIndicInfo
				}
				break
			}
		}
	}

	// Step 5.7: Merge clusters (HarfBuzz: hb-ot-shaper-indic.cc:805-826)
	// For old-spec (or very long syllables), merge all clusters from base to end.
	// For new-spec, track glyph movements and merge accordingly.
	if indicPlan.isOldSpec || (end-start) > 127 {
		if base < end {
			buf.MergeClusters(base, end)
		}
	} else {
		// New-spec: track glyph movements using syllable field (which contains original position)
		// HarfBuzz: hb-ot-shaper-indic.cc:810-826
		for i := base; i < end; i++ {
			if buf.Info[i].Syllable != 255 {
				minPos := i
				maxPos := i
				j := start + int(buf.Info[i].Syllable)
				for j != i {
					if j < minPos {
						minPos = j
					}
					if j > maxPos {
						maxPos = j
					}
					next := start + int(buf.Info[j].Syllable)
					buf.Info[j].Syllable = 255 // Mark as processed
					j = next
				}
				// Merge clusters from max(base, minPos) to maxPos+1
				mergeStart := base
				if minPos > base {
					mergeStart = minPos
				}
				buf.MergeClusters(mergeStart, maxPos+1)
			}
		}
	}

	// Restore original syllable value (HarfBuzz: hb-ot-shaper-indic.cc:828-830)
	for i := start; i < end; i++ {
		buf.Info[i].Syllable = syllable
	}

	// Step 6: Set up masks (HarfBuzz: hb-ot-shaper-indic.cc:838-858)
	// Reph mask - only for glyphs with RA_TO_BECOME_REPH position
	// HarfBuzz: hb-ot-shaper-indic.cc:839-840
	for i := start; i < end && indicInfo[i].Position == IPosRaToBeReph; i++ {
		buf.Info[i].Mask |= indicPlan.maskArray[indicRphf]
	}
	// Pre-base masks
	// HarfBuzz: mask = indic_plan->mask_array[INDIC_HALF]
	preBaseMask := indicPlan.maskArray[indicHalf]
	// HarfBuzz: if (!indic_plan->is_old_spec && indic_plan->config->blwf_mode == BLWF_MODE_PRE_AND_POST)
	//   mask |= indic_plan->mask_array[INDIC_BLWF];
	if !indicPlan.isOldSpec && config.BlwfMode == BlwfModePreAndPost {
		preBaseMask |= indicPlan.maskArray[indicBlwf]
	}
	for i := start; i < base; i++ {
		buf.Info[i].Mask |= preBaseMask
	}

	// Post-base masks (HarfBuzz: hb-ot-shaper-indic.cc:854-858)
	// Post-base always gets BLWF | ABVF | PSTF
	postBaseMask := indicPlan.maskArray[indicBlwf] | indicPlan.maskArray[indicAbvf] | indicPlan.maskArray[indicPstf]
	for i := base + 1; i < end; i++ {
		buf.Info[i].Mask |= postBaseMask
	}

	// Special handling for Ra,H before base (HarfBuzz: hb-ot-shaper-indic.cc:862-890)
	// "If the syllable starts with Ra + Halant [...] and has more than one
	// consonant, the first Ra is treated like a below-base consonant."
	// Ra+H gets BLWF mask unless followed by ZWJ
	if !indicPlan.isOldSpec && config.BlwfMode == BlwfModePreAndPost {
		for i := start; i+1 < base; i++ {
			if indicInfo[i].Category == ICatRa &&
				indicInfo[i+1].Category == ICatH &&
				(i+2 == base || indicInfo[i+2].Category != ICatZWJ) {
				buf.Info[i].Mask |= indicPlan.maskArray[indicBlwf]
				buf.Info[i+1].Mask |= indicPlan.maskArray[indicBlwf]
			}
		}
	}

	// Pre-base-reordering forms (pref) - HarfBuzz: hb-ot-shaper-indic.cc:893-908
	// Find a Halant,Ra sequence after base and mark it for pre-base-reordering processing.
	prefLen := 2
	if indicPlan.maskArray[indicPref] != 0 && base+prefLen < end {
		for i := base + 1; i+prefLen-1 < end; i++ {
			glyphs := []GlyphID{buf.Info[i].GlyphID, buf.Info[i+1].GlyphID}
			if indicPlan.pref.wouldSubstitute(glyphs) {
				for j := 0; j < prefLen; j++ {
					buf.Info[i+j].Mask |= indicPlan.maskArray[indicPref]
				}
				break
			}
		}
	}

	// Step 7: Copy positions to GlyphInfo for persistence through GSUB
	// HarfBuzz stores these in var1 via HB_BUFFER_ALLOCATE_VAR, so they survive substitutions.
	// We do the same by copying to buf.Info[i].IndicPosition.
	for i := start; i < end; i++ {
		buf.Info[i].IndicCategory = uint8(indicInfo[i].Category)
		buf.Info[i].IndicPosition = uint8(indicInfo[i].Position)
	}
}

// findBaseConsonant finds the base consonant in a syllable.
// HarfBuzz equivalent: initial_reordering_consonant_syllable() in hb-ot-shaper-indic.cc:480-589
func (s *Shaper) findBaseConsonant(buf *Buffer, indicInfo []IndicInfo, start, end int, config *IndicConfig, indicPlan *IndicPlan) int {
	// For most scripts, the base is the last consonant that doesn't have a below-form
	// For Sinhala, special rules apply
	if config.BasePos == BasePosFirst {
		// Find first consonant
		for i := start; i < end; i++ {
			if IsIndicConsonant(indicInfo[i].Category) && indicInfo[i].Category != ICatRepha {
				return i
			}
		}
		return end
	}

	// Determine limit - skip over reph if present
	// HarfBuzz: hb-ot-shaper-indic.cc:497-531
	limit := start
	hasReph := false

	if config.RephMode == RephModeLogRepha && indicInfo[start].Category == ICatRepha {
		// Malayalam dot reph - limit is after the repha
		limit = start + 1
		for limit < end && isIndicJoiner(indicInfo[limit].Category) {
			limit++
		}
		hasReph = true
	} else if indicPlan.maskArray[indicRphf] != 0 &&
		start+2 < end &&
		indicInfo[start].Category == ICatRa && indicInfo[start+1].Category == ICatH &&
		((config.RephMode == RephModeImplicit && !isIndicJoiner(indicInfo[start+2].Category)) ||
			(config.RephMode == RephModeExplicit && indicInfo[start+2].Category == ICatZWJ)) {
		// Ra+H at start could form reph - verify with would_substitute
		// HarfBuzz: hb-ot-shaper-indic.cc:509-516
		glyphs := []GlyphID{buf.Info[start].GlyphID, buf.Info[start+1].GlyphID}
		if indicPlan.rphf.wouldSubstitute(glyphs) ||
			(config.RephMode == RephModeExplicit &&
				indicPlan.rphf.wouldSubstitute([]GlyphID{buf.Info[start].GlyphID, buf.Info[start+1].GlyphID, buf.Info[start+2].GlyphID})) {
			limit = start + 2
			if config.RephMode == RephModeExplicit {
				limit = start + 3
			}
			for limit < end && isIndicJoiner(indicInfo[limit].Category) {
				limit++
			}
			hasReph = true
		}
	}

	// Find base consonant - starting from end, move backwards
	// HarfBuzz: hb-ot-shaper-indic.cc:533-577
	base := end
	seenBelow := false

	for i := end - 1; i >= limit; i-- {
		cat := indicInfo[i].Category

		if IsIndicConsonant(cat) {
			pos := indicInfo[i].Position

			// A consonant that doesn't have a below-base or post-base form
			// (post-base forms have to follow below-base forms)
			if pos != IPosBelowC && (pos != IPosPostC || seenBelow) {
				base = i
				break
			}
			if pos == IPosBelowC {
				seenBelow = true
			}
			base = i
		} else {
			// A ZWJ after a Halant stops the base search, and requests an explicit half form.
			// HarfBuzz: hb-ot-shaper-indic.cc:567-575
			if i > start && cat == ICatZWJ && indicInfo[i-1].Category == ICatH {
				break
			}
		}
	}

	// If we have reph but no other consonant, reph is not formed
	// HarfBuzz: hb-ot-shaper-indic.cc:580-588
	if hasReph && base == end {
		// Find first consonant
		for i := start; i < end; i++ {
			if IsIndicConsonant(indicInfo[i].Category) {
				return i
			}
		}
	}

	// If no base found, use first consonant
	if base == end {
		for i := start; i < end; i++ {
			if IsIndicConsonant(indicInfo[i].Category) && indicInfo[i].Category != ICatRepha {
				return i
			}
		}
	}

	return base
}

// isIndicJoiner returns true if the category is ZWJ or ZWNJ.
func isIndicJoiner(cat IndicCategory) bool {
	return cat == ICatZWJ || cat == ICatZWNJ
}

// classifyIndicConsonantPositions classifies consonant positions in a syllable (pre-base clamping).
// HarfBuzz equivalent: hb-ot-shaper-indic.cc:626-627
func (s *Shaper) classifyIndicConsonantPositions(buf *Buffer, indicInfo []IndicInfo, start, end, base int, config *IndicConfig) {
	// Pre-base consonants (before base) get IPosPreC
	for i := start; i < base; i++ {
		cat := indicInfo[i].Category
		pos := indicInfo[i].Position
		if IsIndicConsonant(cat) {
			// HarfBuzz: info[i].indic_position() = hb_min (POS_PRE_C, (indic_position_t) info[i].indic_position());
			if pos > IPosPreC {
				indicInfo[i].Position = IPosPreC
			}
		} else if cat == ICatM {
			indicInfo[i].Position = IPosPreM
		}
	}
}

// attachMiscMarks attaches misc marks to previous char and handles post-base ownership.
// HarfBuzz equivalent: hb-ot-shaper-indic.cc:685-728
func (s *Shaper) attachMiscMarks(buf *Buffer, indicInfo []IndicInfo, start, end, base int, config *IndicConfig) {
	// Attach misc marks to previous char to move with them
	// HarfBuzz: hb-ot-shaper-indic.cc:685-713
	lastPos := IPosStart
	for i := start; i < end; i++ {
		cat := indicInfo[i].Category
		pos := indicInfo[i].Position

		// Joiners, Nukta, RS, CM, Halant get position of previous char
		if cat == ICatZWJ || cat == ICatZWNJ || cat == ICatN || cat == ICatRS || cat == ICatCM || cat == ICatH {
			indicInfo[i].Position = lastPos
			// Special case: Halant at pre-base matra position
			if cat == ICatH && indicInfo[i].Position == IPosPreM {
				// HarfBuzz: Uniscribe doesn't move the Halant with Left Matra
				for j := i; j > start; j-- {
					if indicInfo[j-1].Position != IPosPreM {
						indicInfo[i].Position = indicInfo[j-1].Position
						break
					}
				}
			}
		} else if pos != IPosSMVD {
			// MPst after SM: copy position
			if cat == ICatMPst && i > start && indicInfo[i-1].Category == ICatSM {
				indicInfo[i-1].Position = pos
			}
			lastPos = pos
		}
	}

	// For post-base consonants let them own anything before them
	// since the last consonant or matra.
	// HarfBuzz: hb-ot-shaper-indic.cc:715-728
	// NOTE: This does NOT change consonant positions! Only marks between consonants.
	last := base
	for i := base + 1; i < end; i++ {
		cat := indicInfo[i].Category
		if IsIndicConsonant(cat) {
			// Update marks between last and i to have this consonant's position
			for j := last + 1; j < i; j++ {
				if indicInfo[j].Position < IPosSMVD {
					indicInfo[j].Position = indicInfo[i].Position
				}
			}
			last = i
		} else if cat == ICatM || cat == ICatMPst {
			last = i
		}
	}

	// Note: No additional matra position override needed here.
	// Matra positions come directly from the indic table (generated with
	// indic_matra_position mapping from gen-indic-table.py).
	// HarfBuzz: goes straight to stable sort after the "post-base consonants" block.
}

// stableSortIndicSyllable sorts a syllable by indic_position using stable sort.
// HarfBuzz equivalent: hb_stable_sort() call in hb-ot-shaper-indic.cc:738
//
// This is critical for correct Indic reordering. Characters are sorted by their
// indic_position value, with equal positions maintaining their original order.
// Returns the new index of the base consonant after sorting.
func (s *Shaper) stableSortIndicSyllable(buf *Buffer, indicInfo []IndicInfo, start, end int) int {
	// DEBUG
	if debugIndic {
		fmt.Printf("stableSortIndicSyllable: start=%d end=%d\n", start, end)
		for i := start; i < end; i++ {
			fmt.Printf("  BEFORE [%d]: gid=%d cp=U+%04X pos=%d\n", i, buf.Info[i].GlyphID, buf.Info[i].Codepoint, indicInfo[i].Position)
		}
	}

	if end-start <= 1 {
		// Nothing to sort, but still set relative position for cluster tracking
		for i := start; i < end; i++ {
			buf.Info[i].Syllable = uint8(i - start)
			if indicInfo[i].Position == IPosBaseC {
				return i
			}
		}
		return end
	}

	// HarfBuzz uses syllable() to store original index for cluster tracking
	// Store relative position (HarfBuzz: hb-ot-shaper-indic.cc:732-735)
	// Note: The original syllable is saved by the caller (initialReorderingConsonantSyllable)
	n := end - start
	for i := start; i < end; i++ {
		buf.Info[i].Syllable = uint8(i - start)
	}

	// HarfBuzz uses syllable() to store original index for stable sort tie-breaking
	// We use a slice of indices and sort that
	indices := make([]int, n)
	for i := 0; i < n; i++ {
		indices[i] = i
	}

	// Stable sort by position (HarfBuzz: compare_indic_order)
	// compare_indic_order returns: (int) a - (int) b
	// So we sort ascending by position value
	sort.SliceStable(indices, func(i, j int) bool {
		posI := indicInfo[start+indices[i]].Position
		posJ := indicInfo[start+indices[j]].Position
		return posI < posJ
	})

	// Apply the permutation
	// Create temporary copies
	tempInfo := make([]GlyphInfo, n)
	tempIndicInfo := make([]IndicInfo, n)
	for i := 0; i < n; i++ {
		tempInfo[i] = buf.Info[start+i]
		tempIndicInfo[i] = indicInfo[start+i]
	}

	// Apply sorted order
	for i := 0; i < n; i++ {
		buf.Info[start+i] = tempInfo[indices[i]]
		indicInfo[start+i] = tempIndicInfo[indices[i]]
	}

	// Also reorder Pos if allocated
	if len(buf.Pos) >= end {
		tempPos := make([]GlyphPos, n)
		for i := 0; i < n; i++ {
			tempPos[i] = buf.Pos[start+i]
		}
		for i := 0; i < n; i++ {
			buf.Pos[start+i] = tempPos[indices[i]]
		}
	}

	// DEBUG
	if debugIndic {
		fmt.Printf("  AFTER sort:\n")
		for i := start; i < end; i++ {
			fmt.Printf("    [%d]: gid=%d cp=U+%04X pos=%d syllable(origPos)=%d\n", i, buf.Info[i].GlyphID, buf.Info[i].Codepoint, indicInfo[i].Position, buf.Info[i].Syllable)
		}
	}

	// Find base consonant in new positions (HarfBuzz: hb-ot-shaper-indic.cc:743-750)
	base := end
	for i := start; i < end; i++ {
		if indicInfo[i].Position == IPosBaseC {
			base = i
			break
		}
	}

	// Flip left-matra sequence if needed (HarfBuzz: hb-ot-shaper-indic.cc:758-771)
	// Find first and last left-matra
	firstLeftMatra := end
	lastLeftMatra := end
	for i := start; i < end; i++ {
		if indicInfo[i].Position == IPosPreM {
			if firstLeftMatra == end {
				firstLeftMatra = i
			}
			lastLeftMatra = i
		}
	}

	// Reverse left-matra range if there are multiple
	// HarfBuzz: https://github.com/harfbuzz/harfbuzz/issues/3863
	if firstLeftMatra < lastLeftMatra {
		// HarfBuzz: buffer->reverse_range (first_left_matra, last_left_matra + 1);
		buf.ReverseRange(firstLeftMatra, lastLeftMatra+1)
		// Also reverse indicInfo to keep in sync
		for i, j := firstLeftMatra, lastLeftMatra; i < j; i, j = i+1, j-1 {
			indicInfo[i], indicInfo[j] = indicInfo[j], indicInfo[i]
		}

		// Reverse back nuktas etc. within matra groups
		// HarfBuzz: hb-ot-shaper-indic.cc:764-770
		i := firstLeftMatra
		for j := i; j <= lastLeftMatra; j++ {
			cat := indicInfo[j].Category
			if cat == ICatM || cat == ICatMPst {
				if j > i {
					buf.ReverseRange(i, j+1)
					for ii, jj := i, j; ii < jj; ii, jj = ii+1, jj-1 {
						indicInfo[ii], indicInfo[jj] = indicInfo[jj], indicInfo[ii]
					}
				}
				i = j + 1
			}
		}
	}

	// Note: Cluster merging and syllable restore happen in the caller
	// (initialReorderingConsonantSyllable) which has access to indicPlan
	// to determine old-spec vs new-spec behavior.
	// The syllable field still contains the original position for tracking.

	return base
}

// handleReph handles reph formation (Ra+H at start of syllable).
// HarfBuzz equivalent: initial_reordering_consonant_syllable() in hb-ot-shaper-indic.cc:632-634
func (s *Shaper) handleReph(buf *Buffer, indicInfo []IndicInfo, start, end, base int, config *IndicConfig) {
	if start >= end {
		return
	}

	// For LOG_REPHA mode (Malayalam), the repha is already encoded as U+0D4E
	// HarfBuzz: hb-ot-shaper-indic.cc:524-530
	if config.RephMode == RephModeLogRepha {
		if indicInfo[start].Category == ICatRepha {
			indicInfo[start].Position = IPosRaToBeReph
		}
		return
	}

	// Check for Ra+H at start
	if indicInfo[start].Category != ICatRa {
		return
	}
	if start+1 >= end || indicInfo[start+1].Category != ICatH {
		return
	}

	// For explicit reph mode, need ZWJ after halant
	// HarfBuzz: hb-ot-shaper-indic.cc:506
	if config.RephMode == RephModeExplicit {
		if start+2 >= end || indicInfo[start+2].Category != ICatZWJ {
			return
		}
	}

	// For implicit reph mode, there should be no joiner after halant
	// HarfBuzz: hb-ot-shaper-indic.cc:505
	if config.RephMode == RephModeImplicit {
		if start+2 < end && isIndicJoiner(indicInfo[start+2].Category) {
			return
		}
	}

	// Mark Ra as reph
	indicInfo[start].Position = IPosRaToBeReph
}

// reorderPreBaseMatras reorders pre-base matras to their correct position.
func (s *Shaper) reorderPreBaseMatras(buf *Buffer, indicInfo []IndicInfo, start, end, base int) {
	// Find pre-base matras after base and move them to the start of the syllable.
	// HarfBuzz: Pre-base matras are reordered before basic features.
	// This implements the visual reordering where pre-base matras appear
	// visually to the left of the base consonant.

	// Find insertion point (start of syllable)
	insertPoint := start

	// Process matras from the end to avoid index shifting issues
	for i := end - 1; i > base; i-- {
		if indicInfo[i].Position == IPosPreM {
			// Move this matra to the insertion point
			s.moveGlyph(buf, indicInfo, i, insertPoint)
			// After moving, the insertion point stays the same
			// (the matra was moved to insertPoint, pushing others right)
		}
	}
}

// initialReorderingVowelSyllable reorders a vowel syllable.
func (s *Shaper) initialReorderingVowelSyllable(buf *Buffer, indicInfo []IndicInfo, start, end int, config *IndicConfig) {
	// Vowel syllables don't have a base consonant to reorder around
	// Just classify positions
	for i := start; i < end; i++ {
		cat := indicInfo[i].Category
		if cat == ICatSM || cat == ICatSMPst {
			indicInfo[i].Position = IPosSMVD
		}
		if cat == ICatA {
			indicInfo[i].Position = IPosSMVD
		}
	}
}

// initialReorderingStandaloneCluster reorders a standalone cluster.
func (s *Shaper) initialReorderingStandaloneCluster(buf *Buffer, indicInfo []IndicInfo, start, end int, config *IndicConfig, indicPlan *IndicPlan) {
	// Standalone clusters (with placeholder/dotted circle) are similar to consonant syllables
	s.initialReorderingConsonantSyllable(buf, indicInfo, start, end, config, indicPlan)
}

// finalReorderingIndic performs final reordering after GSUB features.
// HarfBuzz equivalent: final_reordering_indic() in hb-ot-shaper-indic.cc
func (s *Shaper) finalReorderingIndic(buf *Buffer, indicInfo []IndicInfo, config *IndicConfig, indicPlan *IndicPlan) {
	// Final reordering handles:
	// 1. Actual reph repositioning (after rphf feature has been applied)
	// 2. Pre-base-reordering consonant repositioning
	// 3. Pre-base matra repositioning

	// Process each syllable
	// Use buf.Info syllable data (survives GSUB) instead of indicInfo (stale after GSUB)
	start := 0
	for start < len(buf.Info) {
		syllable := buf.Info[start].Syllable
		end := start + 1
		for end < len(buf.Info) && buf.Info[end].Syllable == syllable {
			end++
		}

		syllableType := IndicSyllableType(syllable & 0x0F)

		if syllableType == IndicConsonantSyllable || syllableType == IndicStandaloneCluster {
			s.finalReorderingSyllable(buf, indicInfo, start, end, config, indicPlan)
		}

		start = end
	}
}

// finalReorderingSyllable performs final reordering for a single syllable.
// HarfBuzz equivalent: final_reordering_syllable_indic() in hb-ot-shaper-indic.cc:994-1435
func (s *Shaper) finalReorderingSyllable(buf *Buffer, indicInfo []IndicInfo, start, end int, config *IndicConfig, indicPlan *IndicPlan) {
	// HarfBuzz: hb-ot-shaper-indic.cc:1002-1021
	// Recover halant category for virama glyphs that were ligated then multiplied.
	// After ligation + multiple substitution, the virama glyph may have lost its
	// I_Cat(H) category. If it still has the virama glyph ID and is both ligated
	// and multiplied, restore the halant category and clear those flags so
	// is_halant() will return true.
	viramaGlyph := indicPlan.viramaGID
	if viramaGlyph != 0 {
		for i := start; i < end; i++ {
			if buf.Info[i].GlyphID == viramaGlyph &&
				buf.Info[i].IsLigated() &&
				buf.Info[i].IsMultiplied() {
				buf.Info[i].IndicCategory = uint8(ICatH)
				buf.Info[i].GlyphProps &^= GlyphPropsLigated | GlyphPropsMultiplied
			}
		}
	}

	if debugIndic {
		fmt.Printf("finalReorderingSyllable [%d,%d):\n", start, end)
		for i := start; i < end; i++ {
			fmt.Printf("  [%d] gid=%d pos=%d cat=%d mask=0x%X props=0x%X ligated=%v multiplied=%v subst=%v\n",
				i, buf.Info[i].GlyphID, buf.Info[i].IndicPosition, buf.Info[i].IndicCategory,
				buf.Info[i].Mask, buf.Info[i].GlyphProps,
				buf.Info[i].IsLigated(), buf.Info[i].IsMultiplied(),
				(buf.Info[i].GlyphProps&GlyphPropsSubstituted) != 0)
		}
	}
	// HarfBuzz: bool try_pref = !!indic_plan->mask_array[INDIC_PREF];
	tryPref := indicPlan.maskArray[indicPref] != 0

	// Find base consonant with pref-blocking logic
	// HarfBuzz: hb-ot-shaper-indic.cc:1032-1085
	base := -1
	for i := start; i < end; i++ {
		if buf.Info[i].IndicPosition >= uint8(IPosBaseC) {
			base = i

			// HarfBuzz: pref-blocking check (lines 1039-1058)
			// If a glyph has the pref mask but didn't actually get substituted/ligated,
			// then it was a pref candidate that didn't form - adjust base accordingly.
			if tryPref && base+1 < end {
				for j := base + 1; j < end; j++ {
					if (buf.Info[j].Mask & indicPlan.maskArray[indicPref]) != 0 {
						// HarfBuzz: if (!(_hb_glyph_info_substituted(&info[i]) &&
						//                _hb_glyph_info_ligated_and_didnt_multiply(&info[i])))
						// Check if pref actually formed (substituted AND ligated AND not multiplied)
						ligatedAndDidntMultiply := (buf.Info[j].GlyphProps&GlyphPropsLigated) != 0 &&
							(buf.Info[j].GlyphProps&GlyphPropsMultiplied) == 0
						substituted := (buf.Info[j].GlyphProps & GlyphPropsSubstituted) != 0

						if !(substituted && ligatedAndDidntMultiply) {
							// Ok, this was a 'pref' candidate but didn't form any.
							// Base is around here...
							base = j
							for base < end && isHalant(buf, base) {
								base++
							}
							if base < end {
								buf.Info[base].IndicPosition = uint8(IPosBaseC)
							}
							tryPref = false
						}
						break
					}
				}
				if base == end {
					break
				}
			}

			// HarfBuzz: Malayalam special handling (lines 1062-1080)
			// For Malayalam, skip over unformed below- (but NOT post-) forms.
			if buf.Script == TagMlym {
				for i := base + 1; i < end; {
					// Skip joiners
					for i < end && isIndicJoiner(IndicCategory(buf.Info[i].IndicCategory)) {
						i++
					}
					if i == end || !isHalant(buf, i) {
						break
					}
					i++ // Skip halant
					// Skip joiners
					for i < end && isIndicJoiner(IndicCategory(buf.Info[i].IndicCategory)) {
						i++
					}
					if i < end && IsIndicConsonant(IndicCategory(buf.Info[i].IndicCategory)) &&
						buf.Info[i].IndicPosition == uint8(IPosBelowC) {
						base = i
						buf.Info[base].IndicPosition = uint8(IPosBaseC)
					}
				}
			}

			// HarfBuzz: lines 1082-1083
			if start < base && buf.Info[base].IndicPosition > uint8(IPosBaseC) {
				base--
			}
			break
		}
	}

	// HarfBuzz: if no base found, base = end (the loop naturally stops there)
	if base < 0 {
		base = end
	}

	// HarfBuzz: lines 1086-1092 (handle trailing ZWJ/N/H)
	if base == end && start < base {
		if (buf.Info[base-1].GlyphProps & GlyphPropsZWJ) != 0 {
			base--
		}
	}
	if base < end {
		for start < base {
			cat := IndicCategory(buf.Info[base].IndicCategory)
			if cat != ICatN && !isHalant(buf, base) {
				break
			}
			base--
		}
	}

	// Reorder pre-base matras (move them before the base cluster)
	// HarfBuzz: lines 1095-1203
	_ = s.movePreBaseMatras(buf, indicInfo, start, end, base)

	// Reorder reph to its final position
	// HarfBuzz: lines 1206-1356
	base = s.moveReph(buf, indicInfo, start, end, base, config)

	// Reorder pre-base-reordering consonants (pref)
	// HarfBuzz: lines 1359-1422
	if tryPref && base+1 < end {
		for i := base + 1; i < end; i++ {
			if (buf.Info[i].Mask & indicPlan.maskArray[indicPref]) != 0 {
				// Only reorder if pref actually ligated
				// HarfBuzz: if (_hb_glyph_info_ligated_and_didnt_multiply(&info[i]))
				ligatedAndDidntMultiply := (buf.Info[i].GlyphProps&GlyphPropsLigated) != 0 &&
					(buf.Info[i].GlyphProps&GlyphPropsMultiplied) == 0

				if ligatedAndDidntMultiply {
					// Find target position (same logic as pre-base matra)
					newPos := base
					// Malayalam / Tamil don't have half forms
					if buf.Script != TagMlym && buf.Script != TagTaml {
						for newPos > start {
							prevCat := IndicCategory(buf.Info[newPos-1].IndicCategory)
							if prevCat != ICatM && prevCat != ICatMPst && !isHalant(buf, newPos-1) {
								break
							}
							newPos--
						}
					}

					// If halant before new_pos, check for joiners
					if newPos > start && isHalant(buf, newPos-1) {
						if newPos < end && isIndicJoiner(IndicCategory(buf.Info[newPos].IndicCategory)) {
							newPos++
						}
					}

					// Move pref glyph from i to newPos
					if newPos < i {
						oldPos := i
						buf.MergeClusters(newPos, oldPos+1)
						tmp := buf.Info[oldPos]
						tmpPos := buf.Pos[oldPos]
						copy(buf.Info[newPos+1:oldPos+1], buf.Info[newPos:oldPos])
						copy(buf.Pos[newPos+1:oldPos+1], buf.Pos[newPos:oldPos])
						buf.Info[newPos] = tmp
						buf.Pos[newPos] = tmpPos

						if newPos <= base && base < oldPos {
							base++
						}
					}
				}
				break
			}
		}
	}

	// Merge clusters in the syllable, respecting ZWNJ boundaries
	s.mergeIndicClusters(buf, indicInfo, start, end)
}

// hasZWJ checks if a syllable contains a ZWJ (Zero Width Joiner).
// Uses GlyphPropsZWJ flag which is preserved even after substitution
// (when Codepoint might have changed to 0 or another value).
func hasZWJ(buf *Buffer, start, end int) bool {
	for i := start; i < end; i++ {
		// Check GlyphPropsZWJ flag (preserved through substitution)
		// instead of Codepoint which may have changed
		if (buf.Info[i].GlyphProps & GlyphPropsZWJ) != 0 {
			return true
		}
	}
	return false
}

// mergeIndicClusters merges clusters in a syllable, respecting joiner boundaries.
// HarfBuzz equivalent: cluster merging logic in hb-ot-shaper-indic.cc
//
// Rules based on HarfBuzz behavior:
// - If syllable has NO joiners (ZWJ/ZWNJ): NO automatic cluster merging
// - If syllable contains only ZWJ (no ZWNJ): merge entire syllable to minimum cluster
// - If syllable contains ZWNJ: split at ZWNJ positions
//   - Before ZWNJ: merge if there's a ZWJ, otherwise keep original
//   - ZWNJ itself: keeps its own cluster
//
// HarfBuzz equivalent: cluster merging in hb-ot-shaper-indic.cc
// ZWJ merges BACKWARDS to the segment start or previous joiner.
// ZWNJ creates cluster boundaries.
// Uses GlyphPropsZWNJ/GlyphPropsZWJ flags which are preserved even after substitution.
func (s *Shaper) mergeIndicClusters(buf *Buffer, indicInfo []IndicInfo, start, end int) {
	// Check for ZWNJ - only ZWNJ creates cluster boundaries
	// Use GlyphPropsZWNJ flag instead of Codepoint (which may have changed)
	hasZWNJ := false
	for i := start; i < end; i++ {
		if (buf.Info[i].GlyphProps & GlyphPropsZWNJ) != 0 {
			hasZWNJ = true
			break
		}
	}

	if !hasZWNJ && !hasZWJ(buf, start, end) {
		// No ZWJ/ZWNJ - no cluster merging needed
		// HarfBuzz only merges clusters for specific operations (reordering, etc.),
		// not automatically for all syllables
		return
	}

	// If there's a ZWJ but no ZWNJ, merge the entire syllable
	// (ZWJ requests joining and cluster merging)
	if !hasZWNJ {
		buf.MergeClusters(start, end)
		return
	}

	// Process joiners: ZWJ merges backwards to segment start, ZWNJ creates boundaries
	// A segment is defined as: characters between joiners (or syllable start/end)
	// - ZWJ at end of segment: merge entire segment
	// - ZWNJ at end of segment: no merge, creates boundary
	//
	// Example: TTA, VIRAMA, ZWJ, TTA, VIRAMA, ZWNJ
	//          Segment 1: [TTA, VIRAMA, ZWJ] -> merge to cluster 0
	//          Segment 2: [TTA, VIRAMA, ZWNJ] -> no merge, clusters 3, 4, 5
	segStart := start
	lastJoinerWasZWJ := false
	for i := start; i < end; i++ {
		// Use GlyphProps flags instead of Codepoint
		isZWJ := (buf.Info[i].GlyphProps & GlyphPropsZWJ) != 0
		isZWNJ := (buf.Info[i].GlyphProps & GlyphPropsZWNJ) != 0
		if isZWJ {
			// ZWJ merges backwards from segStart to ZWJ position (inclusive)
			buf.MergeClusters(segStart, i+1)
			// New segment starts after ZWJ
			segStart = i + 1
			lastJoinerWasZWJ = true
		} else if isZWNJ {
			// ZWNJ creates a boundary - no merge, new segment starts after it
			segStart = i + 1
			lastJoinerWasZWJ = false
		}
	}

	// Handle remaining segment after last joiner
	// If last joiner was ZWJ, merge remaining elements with ZWJ's cluster
	if lastJoinerWasZWJ && segStart < end {
		// Find the cluster of the ZWJ (which is at segStart-1)
		// and merge the remaining elements with it
		buf.MergeClusters(segStart-1, end)
	}
}

// mergeSyllableClusters sets all glyphs in a syllable to the minimum cluster value.
// HarfBuzz equivalent: buffer->merge_clusters() in hb-buffer.cc
func (s *Shaper) mergeSyllableClusters(buf *Buffer, start, end int) {
	if start >= end {
		return
	}

	// Find minimum cluster value in the syllable
	minCluster := buf.Info[start].Cluster
	for i := start + 1; i < end; i++ {
		if buf.Info[i].Cluster < minCluster {
			minCluster = buf.Info[i].Cluster
		}
	}

	// Set all glyphs to the minimum cluster
	for i := start; i < end; i++ {
		buf.Info[i].Cluster = minCluster
	}
}

// movePreBaseMatras moves pre-base matras to their correct position.
// Returns true if any matras were moved.
// HarfBuzz equivalent: final_reordering_syllable_indic() in hb-ot-shaper-indic.cc:1176-1202
func (s *Shaper) movePreBaseMatras(buf *Buffer, indicInfo []IndicInfo, start, end, base int) bool {
	// HarfBuzz equivalent: hb-ot-shaper-indic.cc:1123-1202
	// Reorder pre-base matra like best Indic shaper in town!
	// This is O(n^2), but there are only so many matras...

	// HarfBuzz: if (start + 1 < end && start < base)
	// Otherwise there can't be any pre-base matra characters.
	if !(start+1 < end && start < base) {
		return false
	}

	// HarfBuzz: unsigned int new_pos = base == end ? base - 2 : base - 1;
	// If we lost track of base, alas, position before last thingy.
	newPos := base - 1
	if base == end {
		newPos = base - 2
	}

	// Malayalam / Tamil do not have "half" forms or explicit virama forms.
	// The glyphs formed by 'half' are Chillus or ligated explicit viramas.
	// We want to position matra after them.
	if buf.Script != TagMlym && buf.Script != TagTaml {
		// For other scripts, search backwards for Halant/Matra/MPst
		for newPos > start {
			cat := IndicCategory(buf.Info[newPos].IndicCategory)
			if cat != ICatM && cat != ICatMPst && !isHalant(buf, newPos) {
				break
			}
			newPos--
		}

		// If we found a Halant that doesn't belong to a pre-base matra
		if isHalant(buf, newPos) && buf.Info[newPos].IndicPosition != uint8(IPosPreM) {
			// HarfBuzz: Handle ZWJ/ZWNJ after halant
			if newPos+1 < end {
				// If ZWJ follows this halant, matra is NOT repositioned after this halant.
				if IndicCategory(buf.Info[newPos+1].IndicCategory) == ICatZWJ {
					// Keep searching backwards
					if newPos > start {
						newPos--
						// Continue search (simplified - HarfBuzz uses goto)
						for newPos > start {
							cat := IndicCategory(buf.Info[newPos].IndicCategory)
							if cat != ICatM && cat != ICatMPst && !isHalant(buf, newPos) {
								break
							}
							newPos--
						}
					}
				}
				// ZWNJ is handled by state machine - any pre-base matras after H,ZWNJ
				// belong to subsequent syllable.
			}
		} else {
			// No suitable Halant found, don't move
			newPos = start
		}
	}

	// HarfBuzz: if (start < new_pos && info[new_pos].indic_position () != POS_PRE_M)
	if start < newPos && buf.Info[newPos].IndicPosition != uint8(IPosPreM) {
		// Now go see if there's actually any matras...
		// Search backwards from new_pos to start
		for i := newPos; i > start; i-- {
			if buf.Info[i-1].IndicPosition == uint8(IPosPreM) {
				oldPos := i - 1

				// HarfBuzz: if (old_pos < base && base <= new_pos) base--;
				// Shouldn't actually happen, but handle it
				if oldPos < base && base <= newPos {
					base--
				}

				// Move matra from oldPos to newPos (shift right)
				// HarfBuzz: memmove(&info[old_pos], &info[old_pos + 1], (new_pos - old_pos) * sizeof(info[0]));
				//           info[new_pos] = tmp;
				tmp := buf.Info[oldPos]
				tmpPos := buf.Pos[oldPos]
				copy(buf.Info[oldPos:newPos], buf.Info[oldPos+1:newPos+1])
				copy(buf.Pos[oldPos:newPos], buf.Pos[oldPos+1:newPos+1])
				buf.Info[newPos] = tmp
				buf.Pos[newPos] = tmpPos

				// Note: this merge_clusters() is intentionally *after* the reordering.
				// Indic matra reordering is special and tricky...
				mergeEnd := min(end, base+1)
				buf.MergeClusters(newPos, mergeEnd)

				newPos--
			}
		}
		return true
	}

	// Else branch: just merge clusters for matras already before base
	// HarfBuzz: for (unsigned int i = start; i < base; i++)
	//             if (info[i].indic_position () == POS_PRE_M) {
	//               buffer->merge_clusters (i, hb_min (end, base + 1));
	//               break;
	//             }
	for i := start; i < base; i++ {
		if buf.Info[i].IndicPosition == uint8(IPosPreM) {
			mergeEnd := min(end, base+1)
			buf.MergeClusters(i, mergeEnd)
			break
		}
	}

	return false
}

// moveReph moves reph to its final position.
// Returns the (possibly updated) base index.
// HarfBuzz equivalent: hb-ot-shaper-indic.cc:1223-1373
func (s *Shaper) moveReph(buf *Buffer, indicInfo []IndicInfo, start, end, base int, config *IndicConfig) int {
	info := buf.Info

	// HarfBuzz: Two cases for reph detection (lines 1241-1244)
	// XOR condition: (category == Repha) XOR (ligated_and_didnt_multiply)
	if start+1 >= end {
		return base
	}
	if IndicPosition(info[start].IndicPosition) != IPosRaToBeReph {
		return base
	}
	isRepha := IndicCategory(info[start].IndicCategory) == ICatRepha
	ligatedAndDidntMultiply := (info[start].GlyphProps&GlyphPropsLigated) != 0 &&
		(info[start].GlyphProps&GlyphPropsMultiplied) == 0
	// debug removed
	// XOR: only proceed if exactly one is true
	if !(isRepha != ligatedAndDidntMultiply) {
		return base
	}

	var newRephPos int
	rephPos := config.RephPos

	// Step 1: If reph should be positioned after post-base consonant forms, jump to step 5
	if rephPos == IPosAfterPost {
		goto reph_step_5
	}

	// Step 2: Find first explicit halant between first post-reph consonant and last main consonant
	{
		newRephPos = start + 1
		for newRephPos < base && !isHalant(buf, newRephPos) {
			newRephPos++
		}
		if newRephPos < base && isHalant(buf, newRephPos) {
			// If ZWJ or ZWNJ follows this halant, position is moved after it
			if newRephPos+1 < base && isIndicJoiner(IndicCategory(info[newRephPos+1].IndicCategory)) {
				newRephPos++
			}
			goto reph_move
		}
	}

	// Step 3: If reph should be repositioned after the main consonant
	if rephPos == IPosAfterMain {
		newRephPos = base
		for newRephPos+1 < end && IndicPosition(info[newRephPos+1].IndicPosition) <= IPosAfterMain {
			newRephPos++
		}
		if newRephPos < end {
			goto reph_move
		}
	}

	// Step 4: If reph should be positioned after sub-joined consonant
	if rephPos == IPosAfterSub {
		newRephPos = base
		for newRephPos+1 < end {
			pos := IndicPosition(info[newRephPos+1].IndicPosition)
			if pos == IPosPostC || pos == IPosAfterPost || pos == IPosSMVD {
				break
			}
			newRephPos++
		}
		if newRephPos < end {
			goto reph_move
		}
	}

	// Step 5: Fallback halant search (copied from step 2)
reph_step_5:
	{
		newRephPos = start + 1
		for newRephPos < base && !isHalant(buf, newRephPos) {
			newRephPos++
		}
		if newRephPos < base && isHalant(buf, newRephPos) {
			if newRephPos+1 < base && isIndicJoiner(IndicCategory(info[newRephPos+1].IndicCategory)) {
				newRephPos++
			}
			goto reph_move
		}
	}

	// Step 6: Otherwise, reorder reph to end of syllable
	{
		newRephPos = end - 1
		for newRephPos > start && IndicPosition(info[newRephPos].IndicPosition) == IPosSMVD {
			newRephPos--
		}

		// If the Reph is ending up after a Matra,Halant sequence,
		// position it before that Halant so it can interact with the Matra.
		// HarfBuzz: lines 1349-1357
		if isHalant(buf, newRephPos) {
			for i := base + 1; i < newRephPos; i++ {
				cat := indicInfo[i].Category
				if cat == ICatM || cat == ICatMPst {
					newRephPos--
				}
			}
		}

		goto reph_move
	}

reph_move:
	{
		// Merge clusters and memmove
		// HarfBuzz: buffer->merge_clusters(start, new_reph_pos + 1)
		buf.MergeClusters(start, newRephPos+1)

		reph := info[start]
		rephInd := indicInfo[start]
		rephP := buf.Pos[start]
		copy(info[start:newRephPos], info[start+1:newRephPos+1])
		copy(indicInfo[start:newRephPos], indicInfo[start+1:newRephPos+1])
		copy(buf.Pos[start:newRephPos], buf.Pos[start+1:newRephPos+1])
		info[newRephPos] = reph
		indicInfo[newRephPos] = rephInd
		buf.Pos[newRephPos] = rephP

		// HarfBuzz: if (start < base && base <= new_reph_pos) base--;
		if start < base && base <= newRephPos {
			base--
		}
	}

	return base
}

// moveGlyph moves a glyph from src to dst position, shifting others.
func (s *Shaper) moveGlyph(buf *Buffer, indicInfo []IndicInfo, src, dst int) {
	if src == dst {
		return
	}

	// Adjust dst if it's at the end (means "after the last element")
	// In this case, we actually want to insert at dst-1 position
	if dst >= len(buf.Info) {
		dst = len(buf.Info) - 1
	}

	if src == dst {
		return
	}

	// Save the glyph to move
	glyph := buf.Info[src]
	info := indicInfo[src]

	if src < dst {
		// Moving forward: shift elements left
		copy(buf.Info[src:dst], buf.Info[src+1:dst+1])
		copy(indicInfo[src:dst], indicInfo[src+1:dst+1])
		buf.Info[dst] = glyph
		indicInfo[dst] = info
	} else {
		// Moving backward: shift elements right
		copy(buf.Info[dst+1:src+1], buf.Info[dst:src])
		copy(indicInfo[dst+1:src+1], indicInfo[dst:src])
		buf.Info[dst] = glyph
		indicInfo[dst] = info
	}
}

// Indic feature tags
var (
	tagNukt = MakeTag('n', 'u', 'k', 't') // Nukta forms
	tagAkhn = MakeTag('a', 'k', 'h', 'n') // Akhand ligatures
	tagRphf = MakeTag('r', 'p', 'h', 'f') // Reph forms
	tagRkrf = MakeTag('r', 'k', 'r', 'f') // Rakaar forms
	tagPref = MakeTag('p', 'r', 'e', 'f') // Pre-base forms
	tagBlwf = MakeTag('b', 'l', 'w', 'f') // Below-base forms
	tagAbvf = MakeTag('a', 'b', 'v', 'f') // Above-base forms
	tagHalf = MakeTag('h', 'a', 'l', 'f') // Half forms
	tagPstf = MakeTag('p', 's', 't', 'f') // Post-base forms
	tagVatu = MakeTag('v', 'a', 't', 'u') // Vattu variants
	tagCjct = MakeTag('c', 'j', 'c', 't') // Conjunct forms
	tagPres = MakeTag('p', 'r', 'e', 's') // Pre-base substitutions
	tagAbvs = MakeTag('a', 'b', 'v', 's') // Above-base substitutions
	tagBlws = MakeTag('b', 'l', 'w', 's') // Below-base substitutions
	tagPsts = MakeTag('p', 's', 't', 's') // Post-base substitutions
	tagHaln = MakeTag('h', 'a', 'l', 'n') // Halant forms
	tagDist = MakeTag('d', 'i', 's', 't') // Distances
	tagAbvm = MakeTag('a', 'b', 'v', 'm') // Above-base mark positioning
	tagBlwm = MakeTag('b', 'l', 'w', 'm') // Below-base mark positioning
)

// shapeIndic shapes text using the Indic shaper.
// HarfBuzz equivalent: hb_ot_shape_internal() with indic shaper
func (s *Shaper) shapeIndic(buf *Buffer, features []Feature) {
	// Set direction to LTR if not set
	if buf.Direction == 0 {
		buf.Direction = DirectionLTR
	}

	// Step 0: Preprocess vowel constraints (insert dotted circles)
	// HarfBuzz equivalent: _hb_preprocess_text_vowel_constraints() in hb-ot-shaper-vowel-constraints.cc
	// This is called BEFORE normalization in HarfBuzz's preprocess_text hook
	PreprocessVowelConstraints(buf)

	// Detect script from buffer content
	script := s.detectIndicScript(buf)
	config := getIndicConfig(script)

	// Get or create IndicPlan for this script
	// HarfBuzz equivalent: data_create_indic() and accessing plan->data()
	indicPlan := s.getIndicPlan(script, config)

	// DEBUG
	if debugIndic {
		fmt.Printf("isOldSpec: script=%s, isOldSpec=%v\n",
			script.String(), indicPlan.isOldSpec)
	}

	// Step 1: Normalize Unicode
	// HarfBuzz equivalent: _hb_ot_shape_normalize() in hb-ot-shape-normalize.cc
	// Indic uses COMPOSED_DIACRITICS mode like USE
	s.normalizeBuffer(buf, NormalizationModeComposedDiacritics)

	// Step 2: Initialize masks after normalization
	// HarfBuzz equivalent: hb_ot_shape_initialize_masks()
	buf.ResetMasks(MaskGlobal)

	// Step 3: Map codepoints to glyphs
	// HarfBuzz equivalent: hb_ot_map_glyphs_fast() in hb-ot-shape.cc
	s.mapCodepointsToGlyphs(buf)

	// Step 4: Set up Indic properties
	indicInfo := s.setupIndicProperties(buf, config)

	// Step 5: Find syllables
	hasBroken := s.findSyllablesIndic(indicInfo)

	// Step 5.5: Insert dotted circles for broken clusters
	// HarfBuzz equivalent: hb_syllabic_insert_dotted_circles() in initial_reordering_indic()
	if hasBroken {
		accessor := &indicSyllableAccessor{indicInfo: indicInfo}
		// ICatDOTTEDCIRCLE = 11, ICatRepha = 14
		// HarfBuzz: hb-ot-shaper-indic.cc:978-982
		s.insertSyllabicDottedCircles(buf, accessor,
			uint8(IndicBrokenCluster), // broken syllable type
			uint8(ICatDOTTEDCIRCLE),   // dotted circle category
			int(ICatRepha))            // repha category
		// Update indicInfo after insertion (buffer length may have changed)
		indicInfo = s.setupIndicProperties(buf, config)
		s.findSyllablesIndic(indicInfo)
	}

	// Copy syllable info to GlyphInfo for per-syllable GSUB application
	// HarfBuzz equivalent: info.syllable() in hb-buffer.hh
	for i := range buf.Info {
		buf.Info[i].Syllable = indicInfo[i].Syllable
	}

	// Step 6: Set up base masks BEFORE initial reordering
	// HarfBuzz: Masks are set in initial_reordering_consonant_syllable (hb-ot-shaper-indic.cc:843-848)
	// setupIndicMasksFromPositions sets MaskGlobal | indicPlan.maskArray[indicCjct] on all glyphs
	s.setupIndicMasksFromPositions(buf, indicInfo, indicPlan)

	// Step 6.5: Initial reordering (before GSUB)
	// This also adds feature masks to glyphs before the base consonant
	// and sets position-dependent feature masks (BLWF, ABVF, PSTF)
	s.initialReorderingIndic(buf, indicInfo, config, indicPlan)

	// Step 7: Apply basic shaping features
	// HarfBuzz applies these in specific order with pauses
	s.applyIndicBasicFeatures(buf, indicPlan)

	// Rebuild indicInfo from buf.Info after GSUB may have changed buffer length
	// (e.g. rphf ligature Ra+Halant → rephdeva shrinks buffer by 1)
	indicInfo = make([]IndicInfo, len(buf.Info))
	for i, info := range buf.Info {
		indicInfo[i] = IndicInfo{
			Category: IndicCategory(info.IndicCategory),
			Position: IndicPosition(info.IndicPosition),
			Syllable: info.Syllable,
		}
	}

	// Step 8: Final reordering (after basic features, before other features)
	s.finalReorderingIndic(buf, indicInfo, config, indicPlan)

	// Step 8.5: Set init mask on first glyph of buffer (after reordering!)
	// HarfBuzz: 'init' feature only applies to buffer-initial glyph
	s.setIndicInitMask(buf, indicPlan)

	// Step 9: Apply user-requested GSUB features (e.g., ss03, salt) BEFORE other features
	// HarfBuzz: Lookups are sorted by index. User features like ss03 often have lower
	// lookup indices than standard features like psts, so they need to be applied first.
	userGSUB, _ := s.categorizeFeatures(features)
	s.applyUserIndicGSUBFeatures(buf, userGSUB)

	// Step 9.5: Apply other GSUB features
	s.applyIndicOtherFeatures(buf, indicPlan)

	// Step 10: Ensure buf.Pos is allocated (may not be if glyph count didn't change during substitutions)
	if len(buf.Pos) != len(buf.Info) {
		buf.Pos = make([]GlyphPos, len(buf.Info))
	}

	// Step 11: Set base advances
	s.setBaseAdvances(buf)

	// Step 12: Apply GPOS features
	// For Indic, we need to apply standard GPOS features even if none were explicitly requested
	// HarfBuzz: These are applied as part of the Indic shaper's positioning phase
	gposFeatures := s.getIndicGPOSFeatures(features)
	s.applyGPOS(buf, gposFeatures)

	// Note: Indic uses ZeroWidthMarksNone, so NO zeroMarkWidthsByGDEF call here
	// HarfBuzz: _hb_ot_shaper_indic has zero_width_marks = HB_OT_SHAPE_ZERO_WIDTH_MARKS_NONE
}

// getIndicGPOSFeatures returns GPOS features to apply for Indic shaping.
// HarfBuzz equivalent: positioning features are always applied for Indic scripts.
// The Indic-specific features (dist, abvm, blwm) are always required, plus
// standard positioning features (kern, mark, mkmk).
func (s *Shaper) getIndicGPOSFeatures(features []Feature) []Feature {
	// Indic-specific positioning features are ALWAYS applied
	// These are not optional - they are required for correct Indic rendering
	// HarfBuzz: hb-ot-shaper-indic.cc applies these unconditionally
	requiredIndicFeatures := []Tag{
		tagDist, // Distances - Indic-specific
		tagAbvm, // Above-base mark positioning - Indic-specific
		tagBlwm, // Below-base mark positioning - Indic-specific
	}

	// Standard positioning features
	standardFeatures := []Tag{
		MakeTag('k', 'e', 'r', 'n'), // Kerning
		MakeTag('m', 'a', 'r', 'k'), // Mark positioning
		MakeTag('m', 'k', 'm', 'k'), // Mark-to-mark positioning
	}

	// Build result: required Indic features + standard features + user-requested features
	result := make([]Feature, 0, len(requiredIndicFeatures)+len(standardFeatures))

	// Add required Indic-specific features first
	for _, tag := range requiredIndicFeatures {
		result = append(result, Feature{Tag: tag, Value: 1})
	}

	// Add standard features
	for _, tag := range standardFeatures {
		result = append(result, Feature{Tag: tag, Value: 1})
	}

	// Add any explicit GPOS features from user (they may override defaults)
	_, userGPOS := s.categorizeFeatures(features)
	for _, f := range userGPOS {
		// Only add if not already in result
		found := false
		for _, existing := range result {
			if existing.Tag == f.Tag {
				found = true
				break
			}
		}
		if !found {
			result = append(result, f)
		}
	}

	return result
}

// setIndicInitMask sets the init feature mask on the first glyph of the buffer.
// HarfBuzz: 'init' feature applies only to buffer-initial position.
// Despite F_PER_SYLLABLE flag, init only applies to the very first glyph.
func (s *Shaper) setIndicInitMask(buf *Buffer, indicPlan *IndicPlan) {
	if len(buf.Info) == 0 {
		return
	}
	// Only first glyph gets the init mask
	buf.Info[0].Mask |= indicPlan.maskArray[indicInit]
}

// setupIndicMasksFromPositions sets up feature masks based on Indic positions.
// HarfBuzz equivalent: initial_reordering_consonant_syllable() lines 843-848
// Note: HALF mask is now set per-syllable in initialReorderingConsonantSyllable
func (s *Shaper) setupIndicMasksFromPositions(buf *Buffer, indicInfo []IndicInfo, indicPlan *IndicPlan) {
	for i := range buf.Info {
		// Start with global mask and CJCT (which is applied to most glyphs)
		// HarfBuzz: mask_array[INDIC_CJCT] is 0 for global features
		buf.Info[i].Mask = MaskGlobal | indicPlan.maskArray[indicCjct]
		// Note: HALF mask is added per-syllable in initialReorderingConsonantSyllable
	}

	// Apply ZWJ/ZWNJ effects on masks
	// HarfBuzz: hb-ot-shaper-indic.cc:910-928
	s.applyIndicJoinerEffects(buf, indicPlan)
}

// applyIndicBasicFeatures applies basic Indic GSUB features.
// HarfBuzz equivalent: basic_features[] in hb-ot-shaper-indic.cc
func (s *Shaper) applyIndicBasicFeatures(buf *Buffer, indicPlan *IndicPlan) {
	if s.gsub == nil {
		return
	}

	// DEBUG
	if debugIndic {
		fmt.Println("applyIndicBasicFeatures: BEFORE any features:")
		for i, info := range buf.Info {
			fmt.Printf("  [%d] gid=%d cp=U+%04X mask=0x%X\n", i, info.GlyphID, info.Codepoint, info.Mask)
		}
	}

	// Basic features in order (HarfBuzz: hb-ot-shaper-indic.cc basic_features[])
	// Note: These are applied with pauses between some of them in HarfBuzz
	// ALL basic features have F_PER_SYLLABLE flag - must be applied per-syllable
	// HarfBuzz: hb-ot-shaper-indic.cc:173-183
	// Apply each basic feature with the correct auto_zwnj/auto_zwj flags.
	// HarfBuzz: indic_features[] in hb-ot-shaper-indic.cc:166-183
	// All Indic basic features have F_MANUAL_JOINERS (auto_zwnj=false, auto_zwj=false).
	basicIndices := []IndicFeatureIndex{
		indicNukt, indicAkhn, indicRphf, indicRkrf, indicPref,
		indicBlwf, indicAbvf, indicHalf, indicPstf, indicVatu, indicCjct,
	}
	for _, idx := range basicIndices {
		feat := indicFeatures[idx]
		// HarfBuzz: auto_zwnj = !(flags & F_MANUAL_ZWNJ), auto_zwj = !(flags & F_MANUAL_ZWJ)
		autoZWNJ := feat.flags&indicFlagManualZWNJ == 0
		autoZWJ := feat.flags&indicFlagManualZWJ == 0
		s.applyFeaturePerSyllableWithOpts(buf, feat.tag, indicPlan.maskArray[idx], autoZWNJ, autoZWJ)
	}
}

// applyIndicJoinerEffects applies ZWJ/ZWNJ effects on glyph masks.
// HarfBuzz equivalent: hb-ot-shaper-indic.cc:910-928
//
// Rules from HarfBuzz:
//   - ZWNJ disables HALF feature for preceding glyphs (explicit virama form)
//   - ZWJ does NOT disable HALF (allows explicit half form)
//   - ZWJ/ZWNJ disable CJCT feature by being present (F_MANUAL_ZWJ)
func (s *Shaper) applyIndicJoinerEffects(buf *Buffer, indicPlan *IndicPlan) {
	for i := 1; i < len(buf.Info); i++ {
		cp := buf.Info[i].Codepoint
		isZWNJ := cp == 0x200C
		isZWJ := cp == 0x200D

		if !isZWNJ && !isZWJ {
			continue
		}

		// Walk backwards from joiner position
		j := i - 1
		for j >= 0 {
			// HarfBuzz: "A ZWNJ disables HALF."
			// Only ZWNJ disables HALF, not ZWJ!
			// ZWJ requests explicit half form, ZWNJ requests explicit virama form.
			if isZWNJ {
				buf.Info[j].Mask &^= indicPlan.maskArray[indicHalf]
			}

			// ZWJ/ZWNJ disable CJCT by simply being there
			// (we don't skip them for CJCT feature, ie. F_MANUAL_ZWJ)
			buf.Info[j].Mask &^= indicPlan.maskArray[indicCjct]

			// Stop at consonant
			cat := GetIndicCategory(buf.Info[j].Codepoint)
			if cat == ICatC || cat == ICatRa {
				break
			}
			j--
		}
	}
}

// GetIndicCategory returns the Indic category for a codepoint.
func GetIndicCategory(cp Codepoint) IndicCategory {
	cat, _ := GetIndicCategories(cp)
	return cat
}

// applyIndicOtherFeatures applies other Indic GSUB features.
// HarfBuzz equivalent: other_features[] in hb-ot-shaper-indic.cc
// All these features have F_PER_SYLLABLE flag in HarfBuzz, meaning they only
// operate within syllable boundaries to prevent cross-syllable substitutions.
func (s *Shaper) applyIndicOtherFeatures(buf *Buffer, indicPlan *IndicPlan) {
	if s.gsub == nil {
		return
	}

	// Apply 'init' feature (only first glyph has the init mask set)
	// HarfBuzz: 'init' has F_MANUAL_JOINERS | F_PER_SYLLABLE → autoZWNJ=false, autoZWJ=false
	s.applyFeaturePerSyllableWithOpts(buf, MakeTag('i', 'n', 'i', 't'), indicPlan.maskArray[indicInit], false, false)

	// Other features (HarfBuzz: hb-ot-shaper-indic.cc other_features[])
	// All have F_MANUAL_JOINERS | F_PER_SYLLABLE → autoZWNJ=false, autoZWJ=false
	otherIndicFeatures := []Tag{
		tagPres, // Pre-base substitutions
		tagAbvs, // Above-base substitutions
		tagBlws, // Below-base substitutions
		tagPsts, // Post-base substitutions
		tagHaln, // Halant forms
	}

	for _, tag := range otherIndicFeatures {
		s.applyFeaturePerSyllableWithOpts(buf, tag, MaskGlobal, false, false)
	}

	// Standard horizontal features (HarfBuzz: hb-ot-shape.cc horizontal_features[])
	// These use default flags: autoZWNJ=true, autoZWJ=true
	standardFeatures := []Tag{
		tagCalt, // Contextual alternates
		tagClig, // Contextual ligatures
	}

	for _, tag := range standardFeatures {
		s.applyFeaturePerSyllable(buf, tag, MaskGlobal)
	}
}

// applyFeaturePerSyllable applies a GSUB feature respecting syllable boundaries.
// HarfBuzz equivalent: F_PER_SYLLABLE flag in hb-ot-map.hh
// This ensures that context-based lookups (ligatures, etc.) only match glyphs
// within the same syllable, preventing cross-syllable substitutions.
// Uses HarfBuzz defaults for auto_zwnj=true, auto_zwj=true.
func (s *Shaper) applyFeaturePerSyllable(buf *Buffer, tag Tag, featureMask uint32) {
	s.applyFeaturePerSyllableWithOpts(buf, tag, featureMask, true, true)
}

// applyFeaturePerSyllableWithOpts applies a GSUB feature per syllable with explicit
// auto_zwnj/auto_zwj flags.
// HarfBuzz equivalent: Feature application with F_PER_SYLLABLE combined with
// F_MANUAL_ZWNJ (auto_zwnj=false) and/or F_MANUAL_ZWJ (auto_zwj=false).
// See hb-ot-map.cc:308-309:
//
//	map->auto_zwnj = !(info->flags & F_MANUAL_ZWNJ);
//	map->auto_zwj  = !(info->flags & F_MANUAL_ZWJ);
func (s *Shaper) applyFeaturePerSyllableWithOpts(buf *Buffer, tag Tag, featureMask uint32, autoZWNJ, autoZWJ bool) {
	if s.gsub == nil || len(buf.Info) == 0 {
		return
	}

	// Find syllable boundaries and apply feature to each syllable separately
	start := 0
	for start < len(buf.Info) {
		syllable := buf.Info[start].Syllable
		end := start + 1
		for end < len(buf.Info) && buf.Info[end].Syllable == syllable {
			end++
		}

		// Apply feature to this syllable range only
		s.gsub.ApplyFeatureToBufferRangeWithOpts(tag, buf, s.gdef, featureMask, s.font, start, end, autoZWNJ, autoZWJ)

		// Adjust end for next iteration (buffer length may have changed)
		newEnd := start
		for newEnd < len(buf.Info) && buf.Info[newEnd].Syllable == syllable {
			newEnd++
		}
		start = newEnd
	}
}

// tagClig is the contextual ligatures feature tag
var tagClig = MakeTag('c', 'l', 'i', 'g')

// applyUserIndicGSUBFeatures applies user-requested GSUB features that are not
// standard Indic features. Standard Indic features are already applied by
// applyIndicBasicFeatures and applyIndicOtherFeatures.
// HarfBuzz: User features are applied through the same map, after standard features.
func (s *Shaper) applyUserIndicGSUBFeatures(buf *Buffer, userFeatures []Feature) {
	if s.gsub == nil || len(userFeatures) == 0 {
		return
	}

	for _, f := range userFeatures {
		if f.Value == 0 {
			continue
		}
		// Skip standard Indic features that are already applied
		if isStandardIndicGSUBFeature(f.Tag) {
			continue
		}
		// Apply the user feature
		s.gsub.ApplyFeatureToBufferWithMask(f.Tag, buf, s.gdef, MaskGlobal, s.font)
	}
}

// isStandardIndicGSUBFeature returns true if the tag is a standard Indic GSUB feature
// that is already applied by applyIndicBasicFeatures or applyIndicOtherFeatures.
func isStandardIndicGSUBFeature(tag Tag) bool {
	switch tag {
	// Basic features (from applyIndicBasicFeatures)
	case tagNukt, // nukt
		tagAkhn,                     // akhn
		tagRphf,                     // rphf
		tagRkrf,                     // rkrf
		tagPref,                     // pref
		tagBlwf,                     // blwf
		tagAbvf,                     // abvf
		tagHalf,                     // half
		tagPstf,                     // pstf
		tagVatu,                     // vatu
		tagCjct,                     // cjct
		MakeTag('c', 'f', 'a', 'r'): // cfar
		return true
	// Other features (from applyIndicOtherFeatures)
	case MakeTag('i', 'n', 'i', 't'), // init
		tagPres,                     // pres
		tagAbvs,                     // abvs
		tagBlws,                     // blws
		tagPsts,                     // psts
		tagHaln,                     // haln
		tagCalt,                     // calt
		tagClig:                     // clig
		return true
	// Common features applied in basic/other phases
	case MakeTag('l', 'o', 'c', 'l'), // locl
		MakeTag('c', 'c', 'm', 'p'), // ccmp
		MakeTag('r', 'l', 'i', 'g'), // rlig
		MakeTag('l', 'i', 'g', 'a'): // liga
		return true
	}
	return false
}
