package subset

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/textshape/ot"
)

// TestSerializeCFFTopDictOffsets checks that the offsets in the Top DICT
// point at the charset and the CharStrings INDEX for every glyph count up
// to past the encoding boundaries at 1131/1132. The Top DICT holds these
// offsets and grows when one of them needs a longer encoding, which moves
// the data behind it again.
func TestSerializeCFFTopDictOffsets(t *testing.T) {
	original := &ot.CFF{Name: "Test"}
	for n := 1; n < 700; n++ {
		charStrings := make([][]byte, n)
		charset := []byte{0}
		for i := range charStrings {
			charStrings[i] = []byte{14} // endchar
			if i > 0 {
				charset = append(charset, 0, byte(i))
			}
		}
		out, err := serializeCFFWithSIDMap(original, charStrings, nil, nil, charset, newSIDRemap(), topDictSIDs{})
		if err != nil {
			t.Fatalf("%d glyphs: %v", n, err)
		}
		cff, err := ot.ParseCFF(out)
		if err != nil {
			t.Fatalf("%d glyphs: parse: %v", n, err)
		}
		if off := cff.TopDict.Charset; !bytes.HasPrefix(out[off:], charset) {
			t.Fatalf("%d glyphs: charset offset %d does not point at the charset", n, off)
		}
		if off := cff.TopDict.CharStrings; !bytes.HasPrefix(out[off:], buildINDEX(charStrings)) {
			t.Fatalf("%d glyphs: CharStrings offset %d does not point at the CharStrings INDEX", n, off)
		}
	}
}
