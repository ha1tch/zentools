package pzx

import (
	"bytes"
	"testing"
)

// A TAP of a header and a data block, by hand: length, flag, payload, checksum.
func tapOf(blocks ...[]byte) []byte {
	var out []byte
	for _, b := range blocks {
		out = append(out, byte(len(b)), byte(len(b)>>8))
		out = append(out, b...)
	}
	return out
}

func TestEncodeFromTAP(t *testing.T) {
	header := append([]byte{0x00, 3}, []byte("code      \x03\x00\x00\x80\x00\x80")...)
	header = append(header, 0x55) // 19 bytes with flag and checksum
	data := []byte{0xFF, 0x01, 0x02, 0x03, 0xFF}
	img, err := EncodeFromTAP(tapOf(header, data), "a title")
	if err != nil {
		t.Fatal(err)
	}
	f, err := Decode(img)
	if err != nil {
		t.Fatal(err)
	}
	// Header, then pulses and data for each block with a pause between.
	if len(f.Blocks) != 6 {
		t.Fatalf("%d blocks, want 6", len(f.Blocks))
	}
	if h, ok := f.Blocks[0].(HeaderBlock); !ok || h.Title != "a title" {
		t.Errorf("first block: %#v", f.Blocks[0])
	}
	for i, want := range []struct {
		leader uint16
		raw    []byte
	}{{stdHeaderPilotCount, header}, {stdDataPilotCount, data}} {
		at := 1 + i*3
		p, ok := f.Blocks[at].(PulseBlock)
		if !ok || len(p.Pulses) != 3 || p.Pulses[0].Count != want.leader || p.Pulses[0].Duration != stdPilotPulse {
			t.Errorf("block %d pulses: %#v", i, f.Blocks[at])
		}
		d, ok := f.Blocks[at+1].(DataBlock)
		if !ok || !bytes.Equal(d.Data, want.raw) || d.BitCount != uint32(len(want.raw))*8 {
			t.Errorf("block %d data: %#v", i, f.Blocks[at+1])
		}
	}
	if _, ok := f.Blocks[3].(PauseBlock); !ok {
		t.Errorf("no pause between the blocks: %#v", f.Blocks[3])
	}

	for _, bad := range [][]byte{{0x05}, {0x05, 0x00, 0x01}, {0x00, 0x00}} {
		if _, err := EncodeFromTAP(bad, ""); err == nil {
			t.Errorf("a broken TAP image % X was accepted", bad)
		}
	}
	if img, err := EncodeFromTAP(nil, ""); err != nil || len(img) == 0 {
		t.Errorf("an empty TAP: %v", err)
	}
}
