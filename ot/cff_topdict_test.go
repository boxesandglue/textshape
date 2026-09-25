package ot

import (
	"bytes"
	"testing"
)

// fontMatrix4000 encodes [0.00025 0 0 0.00025 0 0], the FontMatrix of a CFF
// font with 4000 units per em. 0.00025 is the real number 30 0a 00 02 5f.
var fontMatrix4000 = []byte{
	30, 0x0a, 0x00, 0x02, 0x5f, 139, 139,
	30, 0x0a, 0x00, 0x02, 0x5f, 139, 139,
}

// TestTopDictFontMatrix checks that the parser keeps the FontMatrix operands
// as they are. They are real numbers the DICT parser decodes as 0, and
// without them a subset falls back to the 1000-units default.
func TestTopDictFontMatrix(t *testing.T) {
	var data []byte
	data = append(data, fontMatrix4000...)
	data = append(data, 12, 7)
	data = append(data, 139, 139, 247, 0, 247, 0, 5) // FontBBox 0 0 108 108

	dict, err := parseTopDict(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dict.FontMatrix, fontMatrix4000) {
		t.Errorf("FontMatrix = % x, want % x", dict.FontMatrix, fontMatrix4000)
	}
	if dict.FontBBox != [4]int{0, 0, 108, 108} {
		t.Errorf("FontBBox = %v, want [0 0 108 108]", dict.FontBBox)
	}

	dict, err = parseTopDict([]byte{139, 139, 247, 0, 247, 0, 5})
	if err != nil {
		t.Fatal(err)
	}
	if dict.FontMatrix != nil {
		t.Errorf("FontMatrix = % x, want nil for the default", dict.FontMatrix)
	}
}
