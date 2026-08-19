// Command gen-use-table generates USE (Universal Shaping Engine) tables from Unicode data.
//
// HarfBuzz equivalent: gen-use-table.py
//
// Usage:
//
//	go run ./cmd/gen-use-table IndicSyllabicCategory.txt IndicPositionalCategory.txt Scripts.txt Blocks.txt IndicSyllabicCategory-Additional.txt IndicPositionalCategory-Additional.txt > ot/use_table.go
//
// Download input files from:
//
//	https://unicode.org/Public/UCD/latest/ucd/IndicSyllabicCategory.txt
//	https://unicode.org/Public/UCD/latest/ucd/IndicPositionalCategory.txt
//	https://unicode.org/Public/UCD/latest/ucd/Scripts.txt
//	https://unicode.org/Public/UCD/latest/ucd/Blocks.txt
//
// The two -Additional files are Microsoft's USE override data, vendored from
// HarfBuzz src/ms-use/ (MIT licensed, see ms-use-COPYING). They override the
// UCD values and add codepoints the UCD does not categorize at all, e.g. the
// Egyptian Hieroglyphs blocks.
//
// HarfBuzz merge semantics: gen-use-table.py:80-108 (Additional files map onto
// the same data slots as the UCD files and win because they are applied last).
package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// useEntry stores USE category information for a codepoint.
type useEntry struct {
	syllabic   string
	positional string
	script     string
	block      string
	codepoint  uint32
}

// disabledScripts are scripts that have their own shapers (not USE).
var disabledScripts = map[string]bool{
	"Arabic":    true,
	"Lao":       true,
	"Samaritan": true,
	"Syriac":    true,
	"Thai":      true,
}

// categoryMapping maps IndicSyllabicCategory to USE category.
var categoryMapping = map[string]string{
	"Other":                       "O",
	"Bindu":                       "VM",
	"Visarga":                     "VM",
	"Avagraha":                    "O",
	"Nukta":                       "CM",
	"Virama":                      "H",
	"Pure_Killer":                 "V",
	"Reordering_Killer":           "RK",
	"Invisible_Stacker":           "IS",
	"Vowel_Independent":           "B",
	"Vowel_Dependent":             "V",
	"Vowel":                       "V",
	"Consonant_Placeholder":       "GB",
	"Consonant":                   "B",
	"Consonant_Dead":              "O",
	"Consonant_With_Stacker":      "CS",
	"Consonant_Prefixed":          "R",
	"Consonant_Preceding_Repha":   "R",
	"Consonant_Succeeding_Repha":  "F",
	"Consonant_Subjoined":         "SUB",
	"Consonant_Medial":            "M",
	"Consonant_Final":             "F",
	"Consonant_Head_Letter":       "B",
	"Consonant_Initial_Postfixed": "M",
	"Modifying_Letter":            "O",
	"Tone_Letter":                 "B",
	"Tone_Mark":                   "VM",
	"Gemination_Mark":             "CM",
	"Cantillation_Mark":           "VM",
	"Register_Shifter":            "VM",
	"Syllable_Modifier":           "FM",
	"Consonant_Killer":            "CM",
	"Non_Joiner":                  "ZWNJ",
	"Joiner":                      "CGJ",
	"Number_Joiner":               "HN",
	"Number":                      "B",
	"Brahmi_Joining_Number":       "N",
	"Symbol_Modifier":             "SM",
	"Hieroglyph":                  "G",
	"Hieroglyph_Joiner":           "J",
	"Hieroglyph_Mark_Begin":       "SB",
	"Hieroglyph_Mark_End":         "SE",
	"Hieroglyph_Mirror":           "HR",
	"Hieroglyph_Modifier":         "HM",
	"Hieroglyph_Segment_Begin":    "SB",
	"Hieroglyph_Segment_End":      "SE",
}

// usePositions maps a USE category to its position suffixes and the
// Indic_Positional_Category values selecting each suffix.
// HarfBuzz equivalent: use_positions in gen-use-table.py:334-380
var usePositions = map[string][]struct {
	suffix string
	uipc   []string
}{
	"F": {
		{"Abv", []string{"Top"}},
		{"Blw", []string{"Bottom"}},
		{"Pst", []string{"Right"}},
	},
	"M": {
		{"Abv", []string{"Top"}},
		{"Blw", []string{"Bottom", "Bottom_And_Left", "Bottom_And_Right"}},
		{"Pst", []string{"Right"}},
		{"Pre", []string{"Left", "Top_And_Bottom_And_Left"}},
	},
	"CM": {
		{"Abv", []string{"Top"}},
		{"Blw", []string{"Bottom", "Overstruck"}},
	},
	"V": {
		{"Abv", []string{"Top", "Top_And_Bottom", "Top_And_Bottom_And_Right", "Top_And_Right"}},
		{"Blw", []string{"Bottom", "Overstruck", "Bottom_And_Right"}},
		{"Pst", []string{"Right"}},
		{"Pre", []string{"Left", "Top_And_Left", "Top_And_Left_And_Right", "Left_And_Right"}},
	},
	"VM": {
		{"Abv", []string{"Top"}},
		{"Blw", []string{"Bottom", "Overstruck"}},
		{"Pst", []string{"Right"}},
		{"Pre", []string{"Left"}},
	},
	"SM": {
		{"Abv", []string{"Top"}},
		{"Blw", []string{"Bottom"}},
	},
	"FM": {
		{"Abv", []string{"Top"}},
		{"Blw", []string{"Bottom"}},
		{"Pst", []string{"Not_Applicable"}},
	},
}

// getUSECategory returns the USE category for a syllabic/positional combination.
// The codepoint is needed for special-case overrides (e.g. SAKOT).
// HarfBuzz equivalent: map_to_use() in gen-use-table.py:382-421
func getUSECategory(cp uint32, syllabic, positional string) string {
	// UIPC overrides, gen-use-table.py:407-409
	// (harfbuzz#1037 and harfbuzz#1631)
	switch cp {
	case 0x11302, 0x11303, 0x114C1:
		positional = "Top"
	}

	// U+1A60 TAI THAM SIGN SAKOT is SAKOT, not IS.
	// See is_SAKOT() and is_INVISIBLE_STACKER() in gen-use-table.py.
	if cp == 0x1A60 {
		return "Sk"
	}

	// U+0DCA SINHALA SIGN AL-LAKUNA is HVM, split off of HALANT.
	// See is_HALANT_OR_VOWEL_MODIFIER() in gen-use-table.py:247-249.
	if cp == 0x0DCA {
		return "HVM"
	}

	cat := categoryMapping[syllabic]
	if cat == "" {
		cat = "O"
	}

	// Visual_Order_Left vowels are all General_Category Lo, which is_BASE()
	// in gen-use-table.py classifies as B before the vowel rules apply.
	// (We do not read UnicodeData.txt, so special-case it here.)
	if cat == "V" && positional == "Visual_Order_Left" {
		return "B"
	}

	if positions, ok := usePositions[cat]; ok {
		if positional == "" {
			positional = "Not_Applicable"
		}
		for _, pos := range positions {
			for _, uipc := range pos.uipc {
				if positional == uipc {
					return cat + pos.suffix
				}
			}
		}
	}

	return cat
}

func main() {
	if len(os.Args) != 7 {
		fmt.Fprintln(os.Stderr, "usage: gen-use-table IndicSyllabicCategory.txt IndicPositionalCategory.txt Scripts.txt Blocks.txt IndicSyllabicCategory-Additional.txt IndicPositionalCategory-Additional.txt")
		os.Exit(1)
	}

	syllabicData, err := parseUnicodeData(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing IndicSyllabicCategory.txt: %v\n", err)
		os.Exit(1)
	}

	positionalData, err := parseUnicodeData(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing IndicPositionalCategory.txt: %v\n", err)
		os.Exit(1)
	}

	scriptData, err := parseUnicodeData(os.Args[3])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing Scripts.txt: %v\n", err)
		os.Exit(1)
	}

	blocks, err := parseBlocks(os.Args[4])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing Blocks.txt: %v\n", err)
		os.Exit(1)
	}

	syllabicAdd, err := parseUnicodeData(os.Args[5])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing IndicSyllabicCategory-Additional.txt: %v\n", err)
		os.Exit(1)
	}

	positionalAdd, err := parseUnicodeData(os.Args[6])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing IndicPositionalCategory-Additional.txt: %v\n", err)
		os.Exit(1)
	}

	// The ms-use override data maps onto the same slots as the UCD files and
	// wins because it is applied last.
	// HarfBuzz equivalent: gen-use-table.py:80-108
	for cp, v := range syllabicAdd {
		if v == "Consonant_Final_Modifier" {
			// gen-use-table.py:84-86, MicrosoftDocs/typography-issues#336
			v = "Syllable_Modifier"
		}
		syllabicData[cp] = v
	}
	for cp, v := range positionalAdd {
		if v == "NA" {
			v = "Not_Applicable"
		}
		positionalData[cp] = v
	}

	// Codepoints with UIPC but no UISC in the UCD, gen-use-table.py:389-397
	for cp := uint32(0x1CE2); cp <= 0x1CE8; cp++ {
		syllabicData[cp] = "Cantillation_Mark"
	}
	for _, cp := range []uint32{0x0F18, 0x0F19, 0x0F3E, 0x0F3F} {
		syllabicData[cp] = "Vowel_Dependent"
	}
	syllabicData[0x1CED] = "Tone_Mark"

	entries := combineData(syllabicData, positionalData, scriptData, blocks)
	generateCode(entries, blocks)
}

func parseUnicodeData(filename string) (map[uint32]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data := make(map[uint32]string)
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Split(line, ";")
		if len(fields) < 2 {
			continue
		}

		rangePart := strings.TrimSpace(fields[0])
		value := strings.TrimSpace(fields[1])

		var start, end uint64
		if strings.Contains(rangePart, "..") {
			parts := strings.Split(rangePart, "..")
			start, _ = strconv.ParseUint(parts[0], 16, 32)
			end, _ = strconv.ParseUint(parts[1], 16, 32)
		} else {
			start, _ = strconv.ParseUint(rangePart, 16, 32)
			end = start
		}

		for cp := start; cp <= end; cp++ {
			data[uint32(cp)] = value
		}
	}

	return data, scanner.Err()
}

func parseBlocks(filename string) (map[uint32]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	blocks := make(map[uint32]string)
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Split(line, ";")
		if len(fields) < 2 {
			continue
		}

		rangePart := strings.TrimSpace(fields[0])
		name := strings.TrimSpace(fields[1])

		rangeParts := strings.Split(rangePart, "..")
		if len(rangeParts) != 2 {
			continue
		}

		start, _ := strconv.ParseUint(rangeParts[0], 16, 32)
		end, _ := strconv.ParseUint(rangeParts[1], 16, 32)

		for cp := start; cp <= end; cp++ {
			blocks[uint32(cp)] = name
		}
	}

	return blocks, scanner.Err()
}

func combineData(syllabicData, positionalData map[uint32]string, scriptData map[uint32]string, blocks map[uint32]string) []useEntry {
	var entries []useEntry

	for cp, syllabic := range syllabicData {
		script := scriptData[cp]
		if disabledScripts[script] {
			continue
		}

		positional := positionalData[cp]
		if positional == "" {
			positional = "Not_Applicable"
		}

		block := blocks[cp]

		entries = append(entries, useEntry{
			codepoint:  cp,
			syllabic:   syllabic,
			positional: positional,
			script:     script,
			block:      block,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].codepoint < entries[j].codepoint
	})

	return entries
}

func generateCode(entries []useEntry, blocks map[uint32]string) {
	// Build entry map for quick lookup
	entryMap := make(map[uint32]useEntry)
	for _, e := range entries {
		entryMap[e.codepoint] = e
	}

	// Find contiguous ranges
	type rangeInfo struct {
		name    string
		varName string
		start   uint32
		end     uint32
	}

	var ranges []rangeInfo

	if len(entries) > 0 {
		rangeStart := entries[0].codepoint
		prevCp := entries[0].codepoint
		prevBlock := entries[0].block

		for i := 1; i <= len(entries); i++ {
			var cp uint32
			var block string
			atEnd := i == len(entries)

			if !atEnd {
				cp = entries[i].codepoint
				block = entries[i].block
			}

			// End current range if gap too large or different block
			if atEnd || cp-prevCp > 256 || block != prevBlock {
				if prevCp-rangeStart >= 8 { // Only include ranges with enough entries
					varName := fmt.Sprintf("useTable%04X", rangeStart)
					ranges = append(ranges, rangeInfo{
						start:   rangeStart,
						end:     prevCp,
						name:    prevBlock,
						varName: varName,
					})
				}

				if !atEnd {
					rangeStart = cp
				}
			}

			if !atEnd {
				prevCp = cp
				prevBlock = block
			}
		}
	}

	// Codepoints that do not make it into any emitted range (isolated
	// entries, or ranges with fewer than 8 entries) become explicit switch
	// cases so they are not silently dropped, e.g. U+25CC DOTTED CIRCLE.
	covered := func(cp uint32) bool {
		for _, r := range ranges {
			if cp >= r.start && cp <= r.end {
				return true
			}
		}
		return false
	}

	// U+034F and U+2060 are categorized via Default_Ignorable_Code_Point and
	// General_Category in gen-use-table.py (is_CGJ, is_Word_Joiner); we do not
	// read those data files, so they are hardcoded here.
	specials := map[uint32]string{
		0x034F: "CGJ", // COMBINING GRAPHEME JOINER
		0x2060: "WJ",  // WORD JOINER
	}
	for _, e := range entries {
		if covered(e.codepoint) {
			continue
		}
		if cat := getUSECategory(e.codepoint, e.syllabic, e.positional); cat != "O" {
			specials[e.codepoint] = cat
		}
	}
	var specialCps []uint32
	for cp := range specials {
		specialCps = append(specialCps, cp)
	}
	sort.Slice(specialCps, func(i, j int) bool { return specialCps[i] < specialCps[j] })

	fmt.Print(`// Code generated by cmd/gen-use-table. DO NOT EDIT.
// Source: IndicSyllabicCategory.txt, IndicPositionalCategory.txt, Scripts.txt, Blocks.txt
// from https://unicode.org/Public/UCD/latest/ucd/ plus the ms-use override
// files IndicSyllabicCategory-Additional.txt and
// IndicPositionalCategory-Additional.txt vendored from HarfBuzz src/ms-use/.
//
// HarfBuzz equivalent: hb-ot-shaper-use-table.hh

package ot

// USE table - maps codepoints to USE categories
//
// This table is generated from Unicode data files.
// Scripts with their own shapers (Arabic, Thai, etc.) are excluded.

// getUSECategory returns the USE category for a codepoint.
// HarfBuzz equivalent: hb_use_get_category() in hb-ot-shaper-use-table.hh
func getUSECategory(cp Codepoint) USECategory {
	// Special characters and codepoints outside the emitted ranges
	switch cp {
`)
	for _, cp := range specialCps {
		fmt.Printf("\tcase 0x%04X:\n\t\treturn USE_%s\n", cp, specials[cp])
	}
	fmt.Print(`	}
`)

	// Generate range lookups
	for _, r := range ranges {
		fmt.Printf("\tif cp >= 0x%04X && cp <= 0x%04X {\n", r.start, r.end)
		fmt.Printf("\t\treturn %s[cp-0x%04X]\n", r.varName, r.start)
		fmt.Println("\t}")
		fmt.Println()
	}

	fmt.Println("\treturn USE_O")
	fmt.Println("}")
	fmt.Println()

	// Generate tables
	for _, r := range ranges {
		size := r.end - r.start + 1
		fmt.Printf("// %s (U+%04X-U+%04X)\n", r.name, r.start, r.end)
		fmt.Printf("var %s = [%d]USECategory{\n", r.varName, size)

		for cp := r.start; cp <= r.end; {
			lineEnd := min(cp+8, r.end+1)
			fmt.Print("\t")

			for i := cp; i < lineEnd; i++ {
				if e, ok := entryMap[i]; ok {
					cat := getUSECategory(i, e.syllabic, e.positional)
					fmt.Printf("USE_%s", cat)
				} else {
					fmt.Print("USE_O")
				}
				if i < lineEnd-1 {
					fmt.Print(", ")
				}
			}
			fmt.Println(",")
			cp = lineEnd
		}

		fmt.Println("}")
		fmt.Println()
	}

	// Generate isUSEScript function
	generateIsUSEScript(entries)
}

func generateIsUSEScript(entries []useEntry) {
	// Collect all script ranges
	scriptRanges := make(map[string][][2]uint32)

	for _, e := range entries {
		if e.script == "" || e.script == "Common" || e.script == "Inherited" {
			continue
		}

		script := e.script
		ranges := scriptRanges[script]

		if len(ranges) == 0 {
			scriptRanges[script] = [][2]uint32{{e.codepoint, e.codepoint}}
		} else {
			last := &ranges[len(ranges)-1]
			if e.codepoint-last[1] <= 256 {
				last[1] = e.codepoint
			} else {
				scriptRanges[script] = append(ranges, [2]uint32{e.codepoint, e.codepoint})
			}
		}
	}

	fmt.Println(`// isUSEScript returns true if the codepoint belongs to a USE-handled script.
// These scripts use the Universal Shaping Engine instead of script-specific shapers.
func isUSEScript(cp Codepoint) bool {`)

	// Sort scripts for consistent output
	var scripts []string
	for script := range scriptRanges {
		scripts = append(scripts, script)
	}
	sort.Strings(scripts)

	for _, script := range scripts {
		ranges := scriptRanges[script]
		if len(ranges) == 0 {
			continue
		}

		fmt.Printf("\t// %s\n", script)
		for _, r := range ranges {
			if r[0] == r[1] {
				fmt.Printf("\tif cp == 0x%04X {\n", r[0])
			} else {
				fmt.Printf("\tif cp >= 0x%04X && cp <= 0x%04X {\n", r[0], r[1])
			}
			fmt.Println("\t\treturn true")
			fmt.Println("\t}")
		}
	}

	fmt.Println("\treturn false")
	fmt.Println("}")
}
