package tzx

import (
	"bytes"
	"testing"
)

// A minimal TAP image (header block + data block) to wrap. Built by hand so the
// test does not depend on the tap package.
func sampleTAP() []byte {
	// header block: len 0x13, flag 0x00, type 0x03, name (10), dlen, p1, p2, cksum
	hdr := []byte{0x00, 0x03}
	hdr = append(hdr, []byte("CODE      ")...)      // 10-char name
	hdr = append(hdr, 0x04, 0x00, 0x00, 0x80, 0x00, 0x80) // dlen=4 p1=0x8000 p2=0x8000
	var hc byte
	for _, b := range hdr {
		hc ^= b
	}
	header := append([]byte{0x13, 0x00}, append(hdr, hc)...)

	// data block: len 6, flag 0xFF, 4 bytes, cksum
	body := []byte{0xFF, 0x60, 0x0D, 0xF0, 0x0D}
	var dc byte
	for _, b := range body {
		dc ^= b
	}
	data := append([]byte{0x06, 0x00}, append(body, dc)...)
	return append(header, data...)
}

func TestEncodeFromTAPStructure(t *testing.T) {
	tapImg := sampleTAP()
	out, err := EncodeFromTAP(tapImg, EncodeOptions{})
	if err != nil {
		t.Fatalf("EncodeFromTAP: %v", err)
	}
	// signature + EOF + version
	if string(out[:7]) != "ZXTape!" || out[7] != 0x1A || out[8] != 1 || out[9] != 20 {
		t.Errorf("bad header: % X", out[:10])
	}
	blocks, err := Decode(out)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2 standard-speed", len(blocks))
	}
	for i, b := range blocks {
		if b.ID != idStandardSpeed {
			t.Errorf("block %d id = 0x%02X, want 0x10", i, b.ID)
		}
		if b.Pause != defaultPause {
			t.Errorf("block %d pause = %d, want %d", i, b.Pause, defaultPause)
		}
	}
	// The wrapped data must be byte-identical to the original TAP blocks.
	if !bytes.Equal(blocks[0].Data, tapImg[2:2+0x13]) {
		t.Errorf("header block payload not preserved")
	}
}

func TestEncodeWithMetadata(t *testing.T) {
	out, err := EncodeFromTAP(sampleTAP(), EncodeOptions{
		Title: "Test", Author: "Nobody", StopIn48K: true,
	})
	if err != nil {
		t.Fatalf("EncodeFromTAP: %v", err)
	}
	blocks, err := Decode(out)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	// Expect: archive-info (0x32), two 0x10, stop-48K (0x2A) = 4 blocks.
	var haveArchive, haveStop bool
	for _, b := range blocks {
		if b.ID == idArchiveInfo {
			haveArchive = true
		}
		if b.ID == idStopThe48K {
			haveStop = true
		}
	}
	if !haveArchive || !haveStop {
		t.Errorf("metadata blocks missing: archive=%v stop=%v", haveArchive, haveStop)
	}
}
