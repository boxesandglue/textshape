// Copyright 2011,2012 Google, Inc.
// Ported to Go for textshape.
//
// HarfBuzz equivalent: hb-ot-shaper-myanmar-machine.rl

package ot

%%{
  machine myanmarSyllableMachine;
  alphtype byte;
  write data;
}%%

%%{

# Categories from HarfBuzz hb-ot-shaper-myanmar-machine.rl
# Values must match MyanmarCategory constants in myanmar.go

C    = 1;   # Consonant
IV   = 2;   # Independent vowel
DB   = 3;   # Dot below
H    = 4;   # Halant (Myanmar virama)
ZWNJ = 5;   # Zero-width non-joiner
ZWJ  = 6;   # Zero-width joiner
SM   = 8;   # Visarga and Shan tones
A    = 9;   # Anusvara
GB   = 10;  # Placeholder (generic base)
DOTTEDCIRCLE = 11;
Ra   = 15;  # Myanmar Ra
CS   = 18;  # Consonant preceding Kinzi
VAbv = 20;  # Vowel above
VBlw = 21;  # Vowel below
VPre = 22;  # Vowel pre (left)
VPst = 23;  # Vowel post (right)
As   = 32;  # Asat
MH   = 35;  # Medial Ha
MR   = 36;  # Medial Ra
MW   = 37;  # Medial Wa, Shan Wa
MY   = 38;  # Medial Ya, Mon Na, Mon Ma
PT   = 39;  # Pwo and other tones
VS   = 40;  # Variation selectors
ML   = 41;  # Medial Mon La
SMPst = 57; # Post-syllable SM

j = ZWJ|ZWNJ;                    # Joiners
k = (Ra As H);                   # Kinzi
sm = SM | SMPst;
c = C|Ra;                        # is_consonant

medial_group = MY? As? MR? ((MW MH? ML? | MH ML? | ML) As?)?;
main_vowel_group = (VPre.VS?)* VAbv* VBlw* A* (DB As?)?;
post_vowel_group = VPst MH? ML? As* VAbv* A* (DB As?)?;
tone_group = sm | PT A* DB? As?;

complex_syllable_tail = As* medial_group main_vowel_group post_vowel_group* tone_group* j?;
syllable_tail = (H (c|IV).VS?)* (H | complex_syllable_tail);

consonant_syllable = (k|CS)? (c|IV|GB|DOTTEDCIRCLE).VS? syllable_tail;
broken_cluster = k? VS? syllable_tail;
other = any;

main := |*
    consonant_syllable  => { foundSyllable(MyanmarConsonantSyllable) };
    j | SMPst           => { foundSyllable(MyanmarNonMyanmarCluster) };
    broken_cluster      => { foundSyllable(MyanmarBrokenCluster); hasBroken = true };
    other               => { foundSyllable(MyanmarNonMyanmarCluster) };
*|;

}%%

// FindSyllablesMyanmar finds Myanmar syllable boundaries in the buffer.
// It sets the syllable field in Info[i].Syllable to (serial << 4 | type) for each glyph.
// HarfBuzz equivalent: find_syllables_myanmar() in hb-ot-shaper-myanmar-machine.hh
func FindSyllablesMyanmar(buf *Buffer, categories []MyanmarCategory) (hasBroken bool) {
  if len(buf.Info) == 0 {
    return false
  }

  n := len(buf.Info)
  data := make([]byte, n)
  for i := 0; i < n; i++ {
    data[i] = byte(categories[i])
  }

  var cs, p, pe, eof, ts, te, act int
  _ = act // Suppress unused variable warning

  pe = n
  eof = pe

  var syllableSerial uint8 = 1

  foundSyllable := func(syllableType MyanmarSyllableType) {
    for i := ts; i < te; i++ {
      buf.Info[i].Syllable = (syllableSerial << 4) | uint8(syllableType)
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
