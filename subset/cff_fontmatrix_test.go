package subset

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/textshape/ot"
)

// TestSubsetKeepsFontMatrix checks that the subset's Top DICT carries the
// FontMatrix of the original. Without it a CFF font with 4000 units per em
// is drawn four times too large.
func TestSubsetKeepsFontMatrix(t *testing.T) {
	matrix := []byte{
		30, 0x0a, 0x00, 0x02, 0x5f, 139, 139,
		30, 0x0a, 0x00, 0x02, 0x5f, 139, 139,
	}
	want := append(append([]byte(nil), matrix...), 12, 7)

	original := &ot.CFF{TopDict: ot.TopDict{FontMatrix: matrix}}
	got := buildTopDictWithSIDs(original, topDictSIDs{}, 100, 200, 30, 300)
	if !bytes.Contains(got, want) {
		t.Errorf("Top DICT % x does not contain FontMatrix % x", got, want)
	}

	original = &ot.CFF{}
	got = buildTopDictWithSIDs(original, topDictSIDs{}, 100, 200, 30, 300)
	if bytes.Contains(got, []byte{12, 7}) {
		t.Errorf("Top DICT % x has a FontMatrix, want the default", got)
	}
}
