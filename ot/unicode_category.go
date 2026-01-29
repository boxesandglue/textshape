package ot

import "unicode"

// GeneralCategory represents the Unicode General_Category property.
// HarfBuzz equivalent: hb_unicode_general_category_t in hb-unicode.h
//
// The values are ordered to match HarfBuzz's enum for compatibility.
type GeneralCategory uint8

const (
	GCControl            GeneralCategory = iota // Cc
	GCFormat                                    // Cf
	GCUnassigned                                // Cn
	GCPrivateUse                                // Co
	GCSurrogate                                 // Cs
	GCLowercaseLetter                           // Ll
	GCModifierLetter                            // Lm
	GCOtherLetter                               // Lo
	GCTitlecaseLetter                           // Lt
	GCUppercaseLetter                           // Lu
	GCSpacingMark                               // Mc
	GCEnclosingMark                             // Me
	GCNonSpacingMark                            // Mn
	GCDecimalNumber                             // Nd
	GCLetterNumber                              // Nl
	GCOtherNumber                               // No
	GCConnectPunctuation                        // Pc
	GCDashPunctuation                           // Pd
	GCClosePunctuation                          // Pe
	GCFinalPunctuation                          // Pf
	GCInitialPunctuation                        // Pi
	GCOtherPunctuation                          // Po
	GCOpenPunctuation                           // Ps
	GCCurrencySymbol                            // Sc
	GCModifierSymbol                            // Sk
	GCMathSymbol                                // Sm
	GCOtherSymbol                               // So
	GCLineSeparator                             // Zl
	GCParagraphSeparator                        // Zp
	GCSpaceSeparator                            // Zs
)

// getGeneralCategory returns the Unicode General_Category for a codepoint.
// HarfBuzz equivalent: hb_unicode_general_category() in hb-unicode.h
func getGeneralCategory(cp Codepoint) GeneralCategory {
	r := rune(cp)

	// Letters
	if unicode.Is(unicode.Lu, r) {
		return GCUppercaseLetter
	}
	if unicode.Is(unicode.Ll, r) {
		return GCLowercaseLetter
	}
	if unicode.Is(unicode.Lt, r) {
		return GCTitlecaseLetter
	}
	if unicode.Is(unicode.Lm, r) {
		return GCModifierLetter
	}
	if unicode.Is(unicode.Lo, r) {
		return GCOtherLetter
	}

	// Marks
	if unicode.Is(unicode.Mn, r) {
		return GCNonSpacingMark
	}
	if unicode.Is(unicode.Mc, r) {
		return GCSpacingMark
	}
	if unicode.Is(unicode.Me, r) {
		return GCEnclosingMark
	}

	// Numbers
	if unicode.Is(unicode.Nd, r) {
		return GCDecimalNumber
	}
	if unicode.Is(unicode.Nl, r) {
		return GCLetterNumber
	}
	if unicode.Is(unicode.No, r) {
		return GCOtherNumber
	}

	// Punctuation
	if unicode.Is(unicode.Pc, r) {
		return GCConnectPunctuation
	}
	if unicode.Is(unicode.Pd, r) {
		return GCDashPunctuation
	}
	if unicode.Is(unicode.Ps, r) {
		return GCOpenPunctuation
	}
	if unicode.Is(unicode.Pe, r) {
		return GCClosePunctuation
	}
	if unicode.Is(unicode.Pi, r) {
		return GCInitialPunctuation
	}
	if unicode.Is(unicode.Pf, r) {
		return GCFinalPunctuation
	}
	if unicode.Is(unicode.Po, r) {
		return GCOtherPunctuation
	}

	// Symbols
	if unicode.Is(unicode.Sm, r) {
		return GCMathSymbol
	}
	if unicode.Is(unicode.Sc, r) {
		return GCCurrencySymbol
	}
	if unicode.Is(unicode.Sk, r) {
		return GCModifierSymbol
	}
	if unicode.Is(unicode.So, r) {
		return GCOtherSymbol
	}

	// Separators
	if unicode.Is(unicode.Zs, r) {
		return GCSpaceSeparator
	}
	if unicode.Is(unicode.Zl, r) {
		return GCLineSeparator
	}
	if unicode.Is(unicode.Zp, r) {
		return GCParagraphSeparator
	}

	// Other
	if unicode.Is(unicode.Cc, r) {
		return GCControl
	}
	if unicode.Is(unicode.Cf, r) {
		return GCFormat
	}
	if unicode.Is(unicode.Cs, r) {
		return GCSurrogate
	}
	if unicode.Is(unicode.Co, r) {
		return GCPrivateUse
	}

	return GCUnassigned
}

// IsUnicodeMark returns true if the codepoint is a Unicode mark (Mn, Mc, or Me).
// HarfBuzz equivalent: _hb_glyph_info_is_unicode_mark() in hb-ot-layout.hh:272
func IsUnicodeMark(cp Codepoint) bool {
	gc := getGeneralCategory(cp)
	return gc == GCNonSpacingMark || gc == GCSpacingMark || gc == GCEnclosingMark
}
