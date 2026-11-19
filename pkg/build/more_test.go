package build

import (
	"bytes"
	"testing"

	"github.com/ha1tch/zentools/pkg/nex"
	"github.com/ha1tch/zentools/pkg/pzx"
)

func TestEncodeNEX(t *testing.T) {
	code := []byte{0xF3, 0x3E, 0x02, 0xD3, 0xFE, 0x18, 0xFE}
	img, err := EncodeNEX(Request{Name: "t", Code: code, Origin: 0x8000, Start: 0x8000, SP: 0xFFFE}, NEXOptions{Border: 2})
	if err != nil {
		t.Fatal(err)
	}
	f, err := nex.Decode(img)
	if err != nil {
		t.Fatal(err)
	}
	if f.PC != 0x8000 || f.SP != 0xFFFE || f.Border != 2 || f.CoreVersion != DefaultNEXCore {
		t.Errorf("header: PC %04X SP %04X border %d core %v", f.PC, f.SP, f.Border, f.CoreVersion)
	}
	if len(f.Banks) != 1 || !bytes.Equal(f.Banks[2][:len(code)], code) {
		t.Errorf("the code is not at the start of bank 2: %d banks", len(f.Banks))
	}

	// A core of the caller's, and code over two banks.
	long := make([]byte, 0x5000)
	long[0], long[0x4FFF] = 0x11, 0x22
	img, err = EncodeNEX(Request{Code: long, Origin: 0x7000, Start: 0x7000, SP: 0x6FFE}, NEXOptions{Core: [3]uint8{3, 1, 10}})
	if err != nil {
		t.Fatal(err)
	}
	f, _ = nex.Decode(img)
	if f.CoreVersion != ([3]uint8{3, 1, 10}) || f.Banks[5][0x3000] != 0x11 || f.Banks[2][0x3FFF] != 0x22 {
		t.Errorf("core %v, bank 5 %02X, bank 2 %02X", f.CoreVersion, f.Banks[5][0x3000], f.Banks[2][0x3FFF])
	}

	for name, r := range map[string]Request{
		"in the ROM":     {Code: code, Origin: 0x3000, Start: 0x3000},
		"past the top":   {Code: code, Origin: 0xFFFC, Start: 0xFFFC},
		"no code at all": {Origin: 0x8000, Start: 0x8000},
	} {
		if _, err := EncodeNEX(r, NEXOptions{}); err == nil {
			t.Errorf("code %s was accepted", name)
		}
	}
}

func TestEncodePZX(t *testing.T) {
	r := Request{Name: "prog", Code: []byte{1, 2, 3, 4}, Origin: 0x8000, Start: 0x8000}
	img, err := EncodePZX(r)
	if err != nil {
		t.Fatal(err)
	}
	f, err := pzx.Decode(img)
	if err != nil {
		t.Fatal(err)
	}
	// What the tape holds, block by block, is what the TAP holds.
	tapImage := EncodeTAP(r)
	var raw []byte
	for _, b := range f.Blocks {
		if d, ok := b.(pzx.DataBlock); ok {
			raw = append(raw, byte(len(d.Data)), byte(len(d.Data)>>8))
			raw = append(raw, d.Data...)
		}
	}
	if !bytes.Equal(raw, tapImage) {
		t.Errorf("the PZX's data blocks are not the TAP's blocks:\n% X\n% X", raw, tapImage)
	}
	withLoader, err := EncodeTAPWithLoader(r)
	if err != nil {
		t.Fatal(err)
	}
	if img, err := EncodePZXFromTAP(withLoader, "prog"); err != nil || len(img) <= len(withLoader) {
		t.Errorf("a PZX of the tape with its loader: %d bytes, %v", len(img), err)
	}
}
