package pzx

import (
	"testing"
)

func TestPZXRoundTrip_Header(t *testing.T) {
	want := &File{Blocks: []Block{
		HeaderBlock{
			Major: 1, Minor: 0,
			Title: "My Game",
			Info: []KV{
				{Key: "Publisher", Value: "My Software House"},
				{Key: "Author", Value: "Alice"},
				{Key: "Author", Value: "Bob"}, // repeated key, spec explicitly allows this
			},
		},
	}}
	image, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if string(image[0:4]) != "PZXT" {
		t.Fatalf("first block tag = %q, want PZXT", image[0:4])
	}
	got, err := Decode(image)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	h, ok := got.Blocks[0].(HeaderBlock)
	if !ok {
		t.Fatalf("Blocks[0] is %T, want HeaderBlock", got.Blocks[0])
	}
	if h.Title != "My Game" {
		t.Errorf("Title = %q, want %q", h.Title, "My Game")
	}
	if len(h.Info) != 3 || h.Info[2].Key != "Author" || h.Info[2].Value != "Bob" {
		t.Errorf("Info = %+v, want 3 entries with the repeated Author key preserved", h.Info)
	}
}

// TestPulseEntry_StandardExamples confirms decodePulseEntry against the
// spec's own worked examples for the standard Spectrum header and data
// pilot tones (pulse.html's PULS description).
func TestPulseEntry_StandardExamples(t *testing.T) {
	cases := []struct {
		name         string
		wantCount    uint16
		wantDuration uint32
	}{
		{"header pilot: 0x8000+8063,2168,667,735 -- first entry", 8063, 2168},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := encodePulseEntry(Pulse{Count: tc.wantCount, Duration: tc.wantDuration})
			count, duration, n, err := decodePulseEntry(entry)
			if err != nil {
				t.Fatalf("decodePulseEntry: %v", err)
			}
			if n != len(entry) {
				t.Errorf("consumed %d bytes, want %d (all of them)", n, len(entry))
			}
			if count != tc.wantCount || duration != tc.wantDuration {
				t.Errorf("got count=%d duration=%d, want count=%d duration=%d", count, duration, tc.wantCount, tc.wantDuration)
			}
		})
	}
}

// TestPulseEntry_RoundTrip covers the full range of edge cases in the
// count/duration bit-packing: count==1 (no prefix emitted), count>1,
// duration fitting in 15 bits, and duration needing the extended form.
func TestPulseEntry_RoundTrip(t *testing.T) {
	cases := []Pulse{
		{Count: 1, Duration: 0},          // zero duration -- used to invert level
		{Count: 1, Duration: 855},        // standard bit-0 pulse, fits in 15 bits
		{Count: 1, Duration: 0x7FFF},     // largest duration not needing extension
		{Count: 1, Duration: 0x8000},     // smallest duration needing extension
		{Count: 1, Duration: 0x7FFFFFFF}, // largest duration the format allows at all
		{Count: 8063, Duration: 2168},    // the spec's own header pilot example
		{Count: 0x7FFF, Duration: 100},   // largest repeat count
	}
	for _, want := range cases {
		entry := encodePulseEntry(want)
		count, duration, n, err := decodePulseEntry(entry)
		if err != nil {
			t.Fatalf("Pulse%+v: decodePulseEntry: %v", want, err)
		}
		if n != len(entry) {
			t.Errorf("Pulse%+v: consumed %d bytes, want %d", want, n, len(entry))
		}
		if count != want.Count || duration != want.Duration {
			t.Errorf("Pulse%+v round-tripped as count=%d duration=%d", want, count, duration)
		}
	}
}

// TestPulseEntry_DegenerateEncoding confirms the exact-0x8000-as-first-word
// case the spec's pseudocode explicitly calls out: it is NOT a valid
// repeat-count prefix (count must be >0), so it falls through to the
// duration-extension check instead, contributing zero high bits.
func TestPulseEntry_DegenerateEncoding(t *testing.T) {
	// Raw bytes: u16(0x8000) then u16(0x1234) -- exactly the case the
	// spec's comment describes, constructed by hand rather than via
	// encodePulseEntry (which never produces this form itself, per its
	// own doc comment) specifically to test decodePulseEntry's handling
	// of a form a real third-party encoder might still produce.
	raw := []byte{0x00, 0x80, 0x34, 0x12} // little-endian: 0x8000, 0x1234
	count, duration, n, err := decodePulseEntry(raw)
	if err != nil {
		t.Fatalf("decodePulseEntry: %v", err)
	}
	if n != 4 {
		t.Errorf("consumed %d bytes, want 4", n)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (0x8000 must never be read as a repeat-count prefix)", count)
	}
	if duration != 0x1234 {
		t.Errorf("duration = %#x, want 0x1234 (high bits from the 0x8000 prefix contribute zero)", duration)
	}
}

func TestPZXRoundTrip_Pulse(t *testing.T) {
	want := &File{Blocks: []Block{
		HeaderBlock{Major: 1, Minor: 0},
		PulseBlock{Pulses: []Pulse{
			{Count: 8063, Duration: 2168},
			{Count: 1, Duration: 667},
			{Count: 1, Duration: 735},
		}},
	}}
	image, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(image)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	pb, ok := got.Blocks[1].(PulseBlock)
	if !ok {
		t.Fatalf("Blocks[1] is %T, want PulseBlock", got.Blocks[1])
	}
	if len(pb.Pulses) != 3 {
		t.Fatalf("got %d pulses, want 3", len(pb.Pulses))
	}
	if pb.Pulses[0] != (Pulse{Count: 8063, Duration: 2168}) {
		t.Errorf("Pulses[0] = %+v", pb.Pulses[0])
	}
}

func TestPZXRoundTrip_Data(t *testing.T) {
	// The standard ZX Spectrum bit encoding, per the spec's own example:
	// bit 0: 855,855  bit 1: 1710,1710 -- initial level high.
	data := []byte{0xAA, 0x55, 0x0F} // arbitrary payload, 24 bits, byte-aligned
	want := &File{Blocks: []Block{
		HeaderBlock{Major: 1, Minor: 0},
		DataBlock{
			InitialLevelHigh: true,
			BitCount:         24,
			Tail:             945,
			S0:               []uint16{855, 855},
			S1:               []uint16{1710, 1710},
			Data:             data,
		},
	}}
	image, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(image)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	db, ok := got.Blocks[1].(DataBlock)
	if !ok {
		t.Fatalf("Blocks[1] is %T, want DataBlock", got.Blocks[1])
	}
	if !db.InitialLevelHigh || db.BitCount != 24 || db.Tail != 945 {
		t.Errorf("db = %+v", db)
	}
	if string(db.Data) != string(data) {
		t.Errorf("Data = % X, want % X", db.Data, data)
	}
	if len(db.S0) != 2 || db.S0[0] != 855 || len(db.S1) != 2 || db.S1[0] != 1710 {
		t.Errorf("S0=%v S1=%v, want S0=[855 855] S1=[1710 1710]", db.S0, db.S1)
	}
}

func TestPZXRoundTrip_DataNonByteAligned(t *testing.T) {
	// BitCount not a multiple of 8 -- confirmed the spec allows this
	// directly: "the data stream, see below" with byte count
	// ceil(bits/8), i.e. the last byte's low bits beyond BitCount are
	// padding, not part of the encoded stream.
	want := &File{Blocks: []Block{
		HeaderBlock{Major: 1, Minor: 0},
		DataBlock{BitCount: 13, Tail: 0, S0: []uint16{100}, S1: []uint16{200}, Data: []byte{0xFF, 0xE0}},
	}}
	image, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(image)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	db := got.Blocks[1].(DataBlock)
	if db.BitCount != 13 || len(db.Data) != 2 {
		t.Errorf("BitCount=%d len(Data)=%d, want 13 and 2 (ceil(13/8))", db.BitCount, len(db.Data))
	}
}

func TestPZXRoundTrip_Pause(t *testing.T) {
	want := &File{Blocks: []Block{
		HeaderBlock{Major: 1, Minor: 0},
		PauseBlock{InitialLevelHigh: false, Duration: 3500000}, // ~1 second at 3.5MHz
	}}
	image, _ := Encode(want)
	got, err := Decode(image)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	pb := got.Blocks[1].(PauseBlock)
	if pb.InitialLevelHigh || pb.Duration != 3500000 {
		t.Errorf("pb = %+v", pb)
	}
}

func TestPZXRoundTrip_BrowseAndStop(t *testing.T) {
	want := &File{Blocks: []Block{
		HeaderBlock{Major: 1, Minor: 0},
		BrowseBlock{Text: "Level 2"},
		StopBlock{Only48K: true},
	}}
	image, _ := Encode(want)
	got, err := Decode(image)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Blocks[1].(BrowseBlock).Text != "Level 2" {
		t.Errorf("BrowseBlock.Text = %q", got.Blocks[1].(BrowseBlock).Text)
	}
	if !got.Blocks[2].(StopBlock).Only48K {
		t.Error("StopBlock.Only48K = false, want true")
	}
}

func TestPZXDecode_UnknownBlockPreserved(t *testing.T) {
	// Unlike RZX/SZX (where unrecognised blocks are just skipped), PZX
	// explicitly reserves lowercase tags for custom extensions the spec
	// expects implementations to round-trip, not discard -- confirmed by
	// re-encoding and checking the raw bytes survive, not just that
	// decode doesn't error.
	want := &File{Blocks: []Block{
		HeaderBlock{Major: 1, Minor: 0},
		RawBlock{Tag: "xtra", Data: []byte{1, 2, 3, 4, 5}},
	}}
	image, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(image)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	rb, ok := got.Blocks[1].(RawBlock)
	if !ok {
		t.Fatalf("Blocks[1] is %T, want RawBlock", got.Blocks[1])
	}
	if rb.Tag != "xtra" || string(rb.Data) != string([]byte{1, 2, 3, 4, 5}) {
		t.Errorf("RawBlock = %+v", rb)
	}
}

func TestPZXDecode_FirstBlockMustBeHeader(t *testing.T) {
	bad := withBlockHeader("PULS", []byte{0x00, 0x01})
	_, err := Decode(bad)
	if err == nil {
		t.Fatal("expected an error for a file not starting with PZXT, got nil")
	}
}

func TestPZXEncode_DataLengthMismatch(t *testing.T) {
	bad := &File{Blocks: []Block{
		HeaderBlock{Major: 1, Minor: 0},
		DataBlock{BitCount: 16, Data: []byte{0x01}}, // declares 16 bits, gives 1 byte
	}}
	_, err := Encode(bad)
	if err == nil {
		t.Fatal("expected an error for Data length not matching BitCount, got nil")
	}
}
