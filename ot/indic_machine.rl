// Copyright 2011,2012 Google, Inc.
// Ported to Go for textshape.
//
// HarfBuzz equivalent: hb-ot-shaper-indic-machine.rl

package ot

%%{
  machine indicSyllableMachine;
  alphtype byte;
  write data;
}%%

%%{

# Character categories (must match ICat* constants)
X    = 0;
C    = 1;
V    = 2;
N    = 3;
H    = 4;
ZWNJ = 5;
ZWJ  = 6;
M    = 7;
SM   = 8;
A    = 9;
VD   = 9;  # Same as A (Vedic)
PLACEHOLDER = 10;
DOTTEDCIRCLE = 11;
RS    = 12;
MPst  = 13;
Repha = 14;
Ra    = 15;
CM    = 16;
Symbol= 17;
CS    = 18;
SMPst = 57;

# Derived categories
c = (C | Ra);           # is_consonant
n = ((ZWNJ?.RS)? (N.N?)?);  # is_consonant_modifier
z = ZWJ|ZWNJ;           # is_joiner
reph = (Ra H | Repha);  # possible reph
sm = SM | SMPst;

cn = c.ZWJ?.n?;
symbol = Symbol.N?;
matra_group = z*.(M | sm? MPst).N?.H?;
syllable_tail = (z?.sm.sm?.ZWNJ?)? (A | VD)*;
halant_group = (z?.H.(ZWJ.N?)?);
final_halant_group = halant_group | H.ZWNJ;
medial_group = CM?;
halant_or_matra_group = (final_halant_group | matra_group*);

complex_syllable_tail = (halant_group.cn)* medial_group halant_or_matra_group syllable_tail;

consonant_syllable = (Repha|CS)? cn complex_syllable_tail;
vowel_syllable =     reph? V.n? (ZWJ | complex_syllable_tail);
standalone_cluster = ((Repha|CS)? PLACEHOLDER | reph? DOTTEDCIRCLE).n? complex_syllable_tail;
symbol_cluster =     symbol syllable_tail;
broken_cluster =     reph? n? complex_syllable_tail;
other =              any;

main := |*
  consonant_syllable  => { foundSyllable(IndicConsonantSyllable) };
  vowel_syllable      => { foundSyllable(IndicVowelSyllable) };
  standalone_cluster  => { foundSyllable(IndicStandaloneCluster) };
  symbol_cluster      => { foundSyllable(IndicSymbolCluster) };
  SMPst               => { foundSyllable(IndicNonIndicCluster) };
  broken_cluster      => { foundSyllable(IndicBrokenCluster); hasBroken = true };
  other               => { foundSyllable(IndicNonIndicCluster) };
*|;

}%%

// FindSyllablesIndic finds Indic syllable boundaries in the buffer.
// It sets info[i].Syllable to (serial << 4 | type) for each glyph.
// HarfBuzz equivalent: find_syllables_indic() in hb-ot-shaper-indic-machine.hh
func FindSyllablesIndic(info []IndicInfo) (hasBroken bool) {
  if len(info) == 0 {
    return false
  }

  // Build category array
  data := make([]byte, len(info))
  for i := range info {
    data[i] = byte(info[i].Category)
  }

  var cs, p, pe, eof, ts, te, act int
  _ = act // Suppress unused variable warning

  pe = len(data)
  eof = pe

  var syllableSerial uint8 = 1

  foundSyllable := func(syllableType IndicSyllableType) {
    for i := ts; i < te; i++ {
      info[i].Syllable = (syllableSerial << 4) | uint8(syllableType)
    }
    syllableSerial++
    if syllableSerial == 16 {
      syllableSerial = 1
    }
  }

  %%{
    write init;
    write exec;
  }%%

  _ = cs // Suppress unused variable warning
  _ = foundSyllable // May be unused if no syllables found

  return hasBroken
}
