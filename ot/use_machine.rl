// Copyright 2015 Mozilla Foundation, Google, Inc.
// Ported to Go for textshape.
//
// HarfBuzz equivalent: hb-ot-shaper-use-machine.rl

package ot

%%{
  machine useSyllableMachine;
  alphtype byte;
  write data;
}%%

%%{

# Categories used in the Universal Shaping Engine spec:
# https://docs.microsoft.com/en-us/typography/script-development/use
# Values must match USE_* constants in use_category.go

O     = 0;  # OTHER
B     = 1;  # BASE
N     = 4;  # BASE_NUM
GB    = 5;  # BASE_OTHER
CGJ   = 6;  # CGJ
SUB   = 11; # CONS_SUB
H     = 12; # HALANT
HN    = 13; # HALANT_NUM
ZWNJ  = 14; # Zero width non-joiner
WJ    = 16; # Word joiner
R     = 18; # REPHA
CS    = 43; # CONS_WITH_STACKER
IS    = 44; # INVISIBLE_STACKER
Sk    = 48; # SAKOT
G     = 49; # HIEROGLYPH
J     = 50; # HIEROGLYPH_JOINER
SB    = 51; # HIEROGLYPH_SEGMENT_BEGIN
SE    = 52; # HIEROGLYPH_SEGMENT_END
HVM   = 53; # HALANT_OR_VOWEL_MODIFIER
HM    = 54; # HIEROGLYPH_MOD
HR    = 55; # HIEROGLYPH_MIRROR
RK    = 56; # REORDERING_KILLER

FAbv  = 24; # CONS_FINAL_ABOVE
FBlw  = 25; # CONS_FINAL_BELOW
FPst  = 26; # CONS_FINAL_POST
MAbv  = 27; # CONS_MED_ABOVE
MBlw  = 28; # CONS_MED_BELOW
MPst  = 29; # CONS_MED_POST
MPre  = 30; # CONS_MED_PRE
CMAbv = 31; # CONS_MOD_ABOVE
CMBlw = 32; # CONS_MOD_BELOW
VAbv  = 33; # VOWEL_ABOVE
VBlw  = 34; # VOWEL_BELOW
VPst  = 35; # VOWEL_POST
VPre  = 22; # VOWEL_PRE
VMAbv = 37; # VOWEL_MOD_ABOVE
VMBlw = 38; # VOWEL_MOD_BELOW
VMPst = 39; # VOWEL_MOD_POST
VMPre = 23; # VOWEL_MOD_PRE
SMAbv = 41; # SYM_MOD_ABOVE
SMBlw = 42; # SYM_MOD_BELOW
FMAbv = 45; # CONS_FINAL_MOD_ABOVE
FMBlw = 46; # CONS_FINAL_MOD_BELOW
FMPst = 47; # CONS_FINAL_MOD_POST


h = H | HVM | IS | Sk;

consonant_modifiers = CMAbv* CMBlw* ((h B | SUB) CMAbv* CMBlw*)*;
medial_consonants = MPre? MAbv? MBlw? MPst?;
dependent_vowels = VPre* VAbv* VBlw* VPst* | H;
vowel_modifiers = HVM? VMPre* VMAbv* VMBlw* VMPst*;
final_consonants = FAbv* FBlw* FPst*;
final_modifiers = FMAbv* FMBlw* | FMPst?;

complex_syllable_start = (R | CS)? (B | GB);
complex_syllable_middle =
	consonant_modifiers
	medial_consonants
	dependent_vowels
	vowel_modifiers
	(Sk B)*
;
complex_syllable_tail =
	complex_syllable_middle
	final_consonants
	final_modifiers
;
number_joiner_terminated_cluster_tail = (HN N)* HN;
numeral_cluster_tail = (HN N)+;
symbol_cluster_tail = SMAbv+ SMBlw* | SMBlw+;

virama_terminated_cluster_tail =
	consonant_modifiers
	(IS | RK)
;
virama_terminated_cluster =
	complex_syllable_start
	virama_terminated_cluster_tail
;
sakot_terminated_cluster_tail =
	complex_syllable_middle
	Sk
;
sakot_terminated_cluster =
	complex_syllable_start
	sakot_terminated_cluster_tail
;
standard_cluster =
	complex_syllable_start
	complex_syllable_tail
;
tail = complex_syllable_tail | sakot_terminated_cluster_tail | symbol_cluster_tail | virama_terminated_cluster_tail;
broken_cluster =
	R?
	(tail | number_joiner_terminated_cluster_tail | numeral_cluster_tail)
;

number_joiner_terminated_cluster = N number_joiner_terminated_cluster_tail;
numeral_cluster = N numeral_cluster_tail?;
symbol_cluster = (O | GB | SB) tail?;
hieroglyph_cluster = SB* G HR? HM? SE* (J SB* (G HR? HM? SE*)?)*;
other = any;

main := |*
	virama_terminated_cluster ZWNJ?       => { foundSyllable(USE_ViramaTerminatedCluster) };
	sakot_terminated_cluster ZWNJ?        => { foundSyllable(USE_SakotTerminatedCluster) };
	standard_cluster ZWNJ?                => { foundSyllable(USE_StandardCluster) };
	number_joiner_terminated_cluster ZWNJ? => { foundSyllable(USE_NumberJoinerTerminatedCluster) };
	numeral_cluster ZWNJ?                 => { foundSyllable(USE_NumeralCluster) };
	symbol_cluster ZWNJ?                  => { foundSyllable(USE_SymbolCluster) };
	hieroglyph_cluster ZWNJ?              => { foundSyllable(USE_HieroglyphCluster) };
	FMPst                                 => { foundSyllable(USE_NonCluster) };
	broken_cluster ZWNJ?                  => { foundSyllable(USE_BrokenCluster); hasBroken = true };
	other                                 => { foundSyllable(USE_NonCluster) };
*|;

}%%

// FindSyllablesUSE finds USE syllable boundaries in the buffer.
// It sets syllables[i].Syllable to (serial << 4 | type) for each glyph.
// HarfBuzz equivalent: find_syllables_use() in hb-ot-shaper-use-machine.hh
func FindSyllablesUSE(syllables []USESyllableInfo) (hasBroken bool) {
  if len(syllables) == 0 {
    return false
  }

  // Build category array, filtering out CGJ
  // HarfBuzz: not_ccs_default_ignorable filters CGJ
  n := len(syllables)
  data := make([]byte, n)
  mapping := make([]int, 0, n) // Maps filtered index to original index

  for i := 0; i < n; i++ {
    cat := syllables[i].Category
    if cat == USE_CGJ {
      continue // Skip CGJ like HarfBuzz
    }
    data[len(mapping)] = byte(cat)
    mapping = append(mapping, i)
  }

  if len(mapping) == 0 {
    return false
  }

  filteredData := data[:len(mapping)]

  var cs, p, pe, eof, ts, te, act int
  _ = act // Suppress unused variable warning

  pe = len(filteredData)
  eof = pe

  var syllableSerial uint8 = 1

  foundSyllable := func(syllableType USESyllableType) {
    // Map filtered indices back to original indices
    origStart := mapping[ts]
    origEnd := n
    if te < len(mapping) {
      origEnd = mapping[te]
    }

    for i := origStart; i < origEnd; i++ {
      syllables[i].Syllable = (syllableSerial << 4) | uint8(syllableType)
      syllables[i].SyllableType = syllableType
    }
    syllableSerial++
    if syllableSerial == 16 {
      syllableSerial = 1
    }
  }

  // Use filtered data for Ragel
  data = filteredData

  %%{
    write init;
    write exec;
  }%%

  _ = cs // Suppress unused variable warning
  _ = foundSyllable // May be unused if no syllables found

  return hasBroken
}
