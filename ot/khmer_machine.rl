// Copyright 2011,2012 Google, Inc.
// Ported to Go for textshape.
//
// HarfBuzz equivalent: hb-ot-shaper-khmer-machine.rl

package ot

%%{
  machine khmerSyllableMachine;
  alphtype byte;
  write data;
}%%

%%{

# Categories from HarfBuzz (use H for Coeng)
# Values must match KhmerCategory constants in khmer.go

C    = 1;
V    = 2;
H    = 4;  # Coeng
ZWNJ = 5;
ZWJ  = 6;
PLACEHOLDER = 10;
DOTTEDCIRCLE = 11;
Ra   = 15;

VAbv = 20;
VBlw = 21;
VPre = 22;
VPst = 23;

Robatic = 25;
Xgroup  = 26;
Ygroup  = 27;


c = (C | Ra | V);
cn = c.((ZWJ|ZWNJ)?.Robatic)?;
joiner = (ZWJ | ZWNJ);
xgroup = (joiner*.Xgroup)*;
ygroup = Ygroup*;

# This grammar was experimentally extracted from what Uniscribe allows.

matra_group = VPre? xgroup VBlw? xgroup (joiner?.VAbv)? xgroup VPst?;
syllable_tail = xgroup matra_group xgroup (H.c)? ygroup;


broken_cluster =	Robatic? (H.cn)* (H | syllable_tail);
consonant_syllable =	(cn|PLACEHOLDER|DOTTEDCIRCLE) broken_cluster;
other =			any;

main := |*
	consonant_syllable	=> { foundSyllable(KhmerConsonantSyllable) };
	broken_cluster		=> { foundSyllable(KhmerBrokenCluster); hasBroken = true };
	other			=> { foundSyllable(KhmerNonKhmerCluster) };
*|;


}%%

// FindSyllablesKhmer finds Khmer syllable boundaries in the buffer.
// It sets the syllable field in Info[i].Mask to (serial << 4 | type) for each glyph.
// HarfBuzz equivalent: find_syllables_khmer() in hb-ot-shaper-khmer-machine.hh
func FindSyllablesKhmer(buf *Buffer, categories []KhmerCategory) (hasBroken bool) {
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

  foundSyllable := func(syllableType KhmerSyllableType) {
    syllableValue := uint32(syllableSerial<<4) | uint32(syllableType)
    for i := ts; i < te; i++ {
      buf.Info[i].Mask = (buf.Info[i].Mask & 0xFFFF0000) | syllableValue
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
