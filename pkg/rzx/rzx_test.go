package rzx

import (
	"testing"
)

func TestRZXRoundTrip_CreatorAndRecording(t *testing.T) {
	want := &File{
		MajorVersion: 0,
		MinorVersion: 13,
		Blocks: []Block{
			CreatorBlock{ID: "zentools", Major: 1, Minor: 0},
			RecordingBlock{
				TStatesStart: 12345,
				Frames: []Frame{
					{FetchCount: 1000, PortReads: []byte{0xFF, 0xFE}},
					{FetchCount: 500, Repeated: true},
					{FetchCount: 2000, PortReads: []byte{0x1F}},
				},
			},
		},
	}

	image, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if string(image[0:4]) != "RZX!" {
		t.Fatalf("missing RZX! signature, got % X", image[0:4])
	}

	got, err := Decode(image)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(got.Blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(got.Blocks))
	}

	creator, ok := got.Blocks[0].(CreatorBlock)
	if !ok {
		t.Fatalf("Blocks[0] is %T, want CreatorBlock", got.Blocks[0])
	}
	if creator.ID != "zentools" || creator.Major != 1 {
		t.Errorf("creator = %+v, want ID=zentools Major=1", creator)
	}

	rec, ok := got.Blocks[1].(RecordingBlock)
	if !ok {
		t.Fatalf("Blocks[1] is %T, want RecordingBlock", got.Blocks[1])
	}
	if rec.TStatesStart != 12345 {
		t.Errorf("TStatesStart = %d, want 12345", rec.TStatesStart)
	}
	if len(rec.Frames) != 3 {
		t.Fatalf("got %d frames, want 3", len(rec.Frames))
	}
	if rec.Frames[0].FetchCount != 1000 || string(rec.Frames[0].PortReads) != "\xFF\xFE" {
		t.Errorf("frame 0 = %+v", rec.Frames[0])
	}
	if !rec.Frames[1].Repeated || rec.Frames[1].FetchCount != 500 {
		t.Errorf("frame 1 = %+v, want Repeated=true FetchCount=500", rec.Frames[1])
	}
	if len(rec.Frames[1].PortReads) != 0 {
		t.Errorf("repeated frame 1 has PortReads = % X, want none", rec.Frames[1].PortReads)
	}
	if rec.Frames[2].FetchCount != 2000 || string(rec.Frames[2].PortReads) != "\x1F" {
		t.Errorf("frame 2 = %+v", rec.Frames[2])
	}
}

func TestRZXRoundTrip_Snapshot(t *testing.T) {
	snapData := []byte("pretend this is a real .z80 image")
	want := &File{
		Blocks: []Block{
			SnapshotBlock{Extension: "Z80", Data: snapData},
		},
	}
	image, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(image)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	snap, ok := got.Blocks[0].(SnapshotBlock)
	if !ok {
		t.Fatalf("Blocks[0] is %T, want SnapshotBlock", got.Blocks[0])
	}
	if snap.Extension != "Z80" {
		t.Errorf("Extension = %q, want Z80", snap.Extension)
	}
	if string(snap.Data) != string(snapData) {
		t.Errorf("Data = %q, want %q", snap.Data, snapData)
	}
	if snap.External {
		t.Error("External = true, want false")
	}
}

func TestRZXRoundTrip_SecurityBlocks(t *testing.T) {
	// Not verified by this package (see its own doc comment) -- just
	// confirm the raw fields survive a round trip intact.
	want := &File{
		Blocks: []Block{
			SecurityInfoBlock{KeyID: 0xDEADBEEF, Week: 202635},
			SecuritySignatureBlock{
				// R declares 8 bits -> ceil(8/8)=1 content byte, so its own
				// encoding is exactly 3 bytes (2-byte prefix + 1 content byte);
				// an earlier version of this fixture gave R a 4th byte, which
				// isn'"'"'t part of R at all -- it'"'"'s the start of S'"'"'s own prefix,
				// and parsing it as such corrupted S entirely. Caught by
				// running this test and reading the actual error, not by
				// eyeballing the byte literal.
				R: []byte{0x00, 0x08, 0xAB},       // 8-bit MPI: 1 content byte
				S: []byte{0x00, 0x10, 0x12, 0x34}, // 16-bit MPI: 2 content bytes
			},
		},
	}
	image, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(image)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	info, ok := got.Blocks[0].(SecurityInfoBlock)
	if !ok || info.KeyID != 0xDEADBEEF || info.Week != 202635 {
		t.Errorf("SecurityInfoBlock = %+v, ok=%v", got.Blocks[0], ok)
	}
	sig, ok := got.Blocks[1].(SecuritySignatureBlock)
	if !ok {
		t.Fatalf("Blocks[1] is %T, want SecuritySignatureBlock", got.Blocks[1])
	}
	if string(sig.R) != string([]byte{0x00, 0x08, 0xAB}) {
		t.Errorf("R = % X, want 00 08 AB", sig.R)
	}
	if string(sig.S) != string([]byte{0x00, 0x10, 0x12, 0x34}) {
		t.Errorf("S = % X, want 00 10 12 34", sig.S)
	}
}

func TestRZXDecode_MissingSignature(t *testing.T) {
	_, err := Decode([]byte("not an rzx file"))
	if err == nil {
		t.Fatal("expected an error for missing RZX! signature, got nil")
	}
}

func TestRZXDecode_UnknownBlockSkipped(t *testing.T) {
	want := &File{Blocks: []Block{CreatorBlock{ID: "x"}}}
	image, _ := Encode(want)

	junk := withBlockHeader(0xF0, []byte{1, 2, 3})
	spliced := append(append(append([]byte{}, image[:10]...), junk...), image[10:]...)

	got, err := Decode(spliced)
	if err != nil {
		t.Fatalf("Decode with an unknown block type present: %v", err)
	}
	if len(got.Blocks) != 1 {
		t.Fatalf("got %d blocks, want 1 (the unknown block should be skipped, not kept)", len(got.Blocks))
	}
}

func TestRZXDecode_TruncatedFrameData(t *testing.T) {
	// A recording block claiming more frames than its data actually holds.
	data := make([]byte, 13)
	data[0] = 5 // claims 5 frames
	// no frame bytes follow at all
	bad := withBlockHeader(idInputRecording, data)
	image := append(append([]byte(magic), 0, 13, 0, 0, 0, 0), bad...)
	_, err := Decode(image)
	if err == nil {
		t.Fatal("expected an error for a recording block with truncated frame data, got nil")
	}
}
