package tzx

import (
	"bytes"
	"compress/zlib"
	"testing"
)

// zlibCompress compresses data with Go's own stdlib zlib writer, so the
// CSW compression test round-trips entirely through Go's own zlib
// implementation (both compress and decompress), rather than depending
// on a hardcoded byte sequence produced by some other tool.
func zlibCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatalf("zlib compress: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zlib compress close: %v", err)
	}
	return buf.Bytes()
}

// Byte layout and rescaling formula confirmed against SpecIde's own
// TZXFile.cc and CSWFile.cc, not assumed from a general spec summary.
// SpecIde header length 0x05 (5) = id(1) + dataLength_u32(4), but unlike
// every other block this package has added, dataLength here covers a
// 10-byte sub-header (pause, cswRate, cswCompression, cswExpectedPulses)
// *plus* the CSW pulse data itself, not just the pulse data alone --
// confirmed directly from SpecIde's own slice construction
// ("dataLength - 10" for the CSW buffer length).
//
// CSW compression type 2 is confirmed, from SpecIde's own CSWFile.cc
// comment ("Using ZLIB+RLE compression"), to be standard zlib -- not a
// custom variant -- so Go's stdlib compress/zlib is the exact right tool,
// with no external dependency at all (unlike SpecIde itself, which links
// the real system zlib C library for this).
//
// The CSW pulse-stream encoding itself: each byte is a direct 1-byte
// pulse value, except 0x00 which signals an "extended" pulse -- the
// following 4 bytes (little-endian u32) are the real value instead.
// Every raw sample count is then rescaled from CSW's own sample rate to
// Spectrum T-states: pulse = raw * 3500000.0 / cswRate.

// tzxCSWSubHeader builds the 10-byte CSW sub-header (pause, cswRate,
// compression, expectedPulses) that sits between the block's own
// dataLength field and its CSW pulse data.
func tzxCSWSubHeader(pause uint16, cswRate uint32, compression uint8, expectedPulses uint32) []byte {
	b := le16(pause)
	b = append(b, le24(cswRate)...)
	b = append(b, compression)
	b = append(b, le32(expectedPulses)...)
	return b
}

// TestDecode_CSWRecording_Uncompressed confirms compression type 1 (plain
// RLE, no zlib) decodes correctly: three raw CSW values (one 1-byte,
// one 4-byte extended, one 1-byte again) rescaled from a 44100 Hz CSW
// rate to T-states.
func TestDecode_CSWRecording_Uncompressed(t *testing.T) {
	rawCSW := []byte{50, 0, 200, 1, 0, 0, 75} // pulses: 50, 456 (extended), 75
	subHeader := tzxCSWSubHeader(0, 44100, 1, 3)
	payload := append(subHeader, rawCSW...)

	data := tzxHeader10()
	data = append(data, idCSWRecording)
	data = append(data, le32(uint32(10+len(rawCSW)))...) // dataLength: sub-header(10) + CSW data
	data = append(data, payload...)

	blocks, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	want := []int{3968, 36190, 5952}
	got := blocks[0].Pulses
	if len(got) != len(want) {
		t.Fatalf("Pulses = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Pulses[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

// TestDecode_CSWRecording_ZlibCompressed confirms compression type 2
// decodes identically to the uncompressed case above, once the zlib
// layer is stripped -- the same raw CSW bytes, zlib-compressed this
// time, must produce exactly the same three rescaled pulses.
func TestDecode_CSWRecording_ZlibCompressed(t *testing.T) {
	rawCSW := []byte{50, 0, 200, 1, 0, 0, 75}
	compressed := zlibCompress(t, rawCSW)

	subHeader := tzxCSWSubHeader(0, 44100, 2, 3)
	payload := append(subHeader, compressed...)

	data := tzxHeader10()
	data = append(data, idCSWRecording)
	data = append(data, le32(uint32(10+len(payload)-10))...) // dataLength: sub-header(10) + compressed CSW data
	data = append(data, payload...)

	blocks, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	want := []int{3968, 36190, 5952}
	got := blocks[0].Pulses
	if len(got) != len(want) {
		t.Fatalf("Pulses = %v, want %v (zlib decompression may not have run correctly)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Pulses[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}
