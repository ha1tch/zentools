package zxn

import (
	"bytes"
	"reflect"
	"testing"
)

// TestSpriteAttrs checks the layout against the worked examples of the Next
// developer guide (an anchor with unified relatives, scaled 2x) and round
// trips.
func TestSpriteAttrs(t *testing.T) {
	anchor := SpriteAttr{X: 300, Y: 260, Visible: true, Extended: true, Pattern: 1, Unified: true, ScaleX: 1, ScaleY: 1, MirrorX: true, PaletteOffset: 3}
	b, err := anchor.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	// X 300 = 0x12C, Y 260 = 0x104; byte 4 %00101011 is the guide's
	// %00101010 (unified, 2x by 2x) with Y bit 8.
	if want := []byte{0x2C, 0x04, 0x39, 0xC1, 0x2B}; !bytes.Equal(b, want) {
		t.Errorf("anchor % X, want % X", b, want)
	}
	rel := SpriteAttr{X: -16, Y: 16, Visible: true, Extended: true, Relative: true, Pattern: 2, RelativePattern: true, RelativePalette: true}
	b, err = rel.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{0xF0, 0x10, 0x01, 0xC2, 0x41}; !bytes.Equal(b, want) {
		t.Errorf("relative % X, want % X", b, want)
	}
	four := SpriteAttr{X: 10, Y: 20, Visible: true, Extended: true, FourBit: true, N6: true, Pattern: 5}
	plain := SpriteAttr{X: 255, Y: 191, Visible: true, Pattern: 63}
	table := []SpriteAttr{anchor, rel, four, plain}
	for _, fixed := range []bool{false, true} {
		data, err := EncodeAttrs(table, fixed)
		if err != nil {
			t.Fatal(err)
		}
		if n := 5*3 + 4; !fixed && len(data) != n {
			t.Errorf("table is %d bytes, want %d", len(data), n)
		}
		back, err := DecodeAttrs(data, fixed)
		if err != nil {
			t.Fatal(err)
		}
		want := append([]SpriteAttr(nil), table...)
		if fixed {
			want[3].Extended = true
		}
		if !reflect.DeepEqual(back, want) {
			t.Errorf("fixed=%v: round trip\n got %+v\nwant %+v", fixed, back, want)
		}
	}
	for _, bad := range []SpriteAttr{
		{X: 10, Y: 300, Visible: true},                  // Y bit 8 needs byte 4
		{Relative: true, X: 4},                          // a relative sprite needs byte 4
		{X: 600, Extended: true},                        // 9-bit X
		{Relative: true, Extended: true, FourBit: true}, // 4-bit belongs to the anchor
		{Extended: true, RelativePattern: true},         // relative flags on an anchor
		{Extended: true, ScaleX: 4},                     // scale 0-3
		{Extended: true, Pattern: 64},                   // pattern 0-63
	} {
		if _, err := bad.Bytes(); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if _, err := DecodeAttrs([]byte{1, 2, 3, 0x40}, false); err == nil {
		t.Error("a truncated 5-byte sprite accepted")
	}
}
