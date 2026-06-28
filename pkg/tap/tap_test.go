package tap

import (
	"bytes"
	"testing"
)

// TestEncodeCodeMatchesVerified checks the CODE-block encoding against the bytes
// verified byte-identical to pasmo's --tap output for a 4-byte blob at 0x8000.
// The data block in particular (flag + payload + XOR checksum) must be exact.
func TestEncodeCodeDataBlock(t *testing.T) {
	data := []byte{0x60, 0x0D, 0xF0, 0x0D}
	got := EncodeCode("TEST", data, 0x8000)

	// Header block: length 0x13, flag 0x00, type 0x03, name "TEST______",
	// dataLength 4, param1 0x8000 (load addr), param2 0x8000.
	wantHeader := []byte{
		0x13, 0x00, // block length
		0x00,       // flag (header)
		0x03,       // type CODE
		'T', 'E', 'S', 'T', ' ', ' ', ' ', ' ', ' ', ' ', // name, padded to 10
		0x04, 0x00, // data length
		0x00, 0x80, // param1 load address 0x8000
		0x00, 0x80, // param2 0x8000 (convention)
	}
	wantHeader = append(wantHeader, xorChecksum(wantHeader[2:])) // checksum over payload

	// Data block: length 6, flag 0xFF, the four bytes, XOR checksum.
	dataBody := append([]byte{0xFF}, data...)
	wantData := []byte{0x06, 0x00}
	wantData = append(wantData, dataBody...)
	wantData = append(wantData, xorChecksum(dataBody))

	want := append(wantHeader, wantData...)
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeCode mismatch\n got: % X\nwant: % X", got, want)
	}

	// The data block tail must equal the pasmo-verified bytes: ff 60 0d f0 0d 6f.
	verifiedDataBlock := []byte{0x06, 0x00, 0xFF, 0x60, 0x0D, 0xF0, 0x0D, 0x6F}
	if !bytes.Equal(got[len(got)-len(verifiedDataBlock):], verifiedDataBlock) {
		t.Errorf("data block tail = % X, want % X (pasmo-verified)",
			got[len(got)-len(verifiedDataBlock):], verifiedDataBlock)
	}
}

func TestEncodeProgramRoundsTrip(t *testing.T) {
	data := []byte{0x00, 0x0A, 0x05, 0x00} // arbitrary BASIC-ish bytes
	got := EncodeProgram("PROG", data, 0x8000)
	// Header block on disk = 2 (length prefix) + headerLength (0x13 = payload+checksum).
	// Data block on disk    = 2 (length prefix) + 1 (flag) + len(data) + 1 (checksum).
	wantLen := (2 + headerLength) + (2 + 1 + len(data) + 1)
	if len(got) != wantLen {
		t.Errorf("program TAP length = %d, want %d", len(got), wantLen)
	}
	if got[3] != TypeProgram {
		t.Errorf("type byte = %#x, want TypeProgram", got[3])
	}
}

func TestNamePadding(t *testing.T) {
	// Long names truncate to 10; short names pad with spaces.
	got := EncodeCode("THIS_NAME_IS_TOO_LONG", []byte{0x00}, 0x8000)
	name := got[4:14]
	if string(name) != "THIS_NAME_" {
		t.Errorf("name = %q, want %q", name, "THIS_NAME_")
	}
}

func TestDecodeRoundTrip(t *testing.T) {
	data := []byte{0x60, 0x0D, 0xF0, 0x0D}
	img := EncodeCode("GAME", data, 0x8000)

	blocks, err := Decode(img)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	h := blocks[0]
	if !h.IsHeader || !h.ChecksumOK {
		t.Errorf("block 0: IsHeader=%v ChecksumOK=%v, want both true", h.IsHeader, h.ChecksumOK)
	}
	if h.Type != TypeCode || h.Name != "GAME" || h.DataLength != 4 || h.Param1 != 0x8000 || h.Param2 != 0x8000 {
		t.Errorf("header fields wrong: type=%d name=%q dlen=%d p1=%#x p2=%#x",
			h.Type, h.Name, h.DataLength, h.Param1, h.Param2)
	}
	d := blocks[1]
	if d.IsHeader || !d.ChecksumOK || d.Flag != flagData {
		t.Errorf("block 1: IsHeader=%v ChecksumOK=%v flag=%#x", d.IsHeader, d.ChecksumOK, d.Flag)
	}
	if string(d.Data) != string(data) {
		t.Errorf("data = % X, want % X", d.Data, data)
	}
}

func TestDecodeDetectsBadChecksum(t *testing.T) {
	img := EncodeCode("X", []byte{1, 2, 3}, 0x8000)
	img[len(img)-1] ^= 0xFF // corrupt the data block's checksum
	blocks, err := Decode(img)
	if err != nil {
		t.Fatalf("Decode should not error on bad checksum: %v", err)
	}
	if blocks[1].ChecksumOK {
		t.Error("corrupted data block reported ChecksumOK=true")
	}
}
