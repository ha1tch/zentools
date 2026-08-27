package tzx

import "testing"

// Byte layouts for all three block types below are confirmed against
// MartianGirl's SpecIde (github.com/MartianGirl/SpecIde,
// source/src/TZXFile.cc), specifically its getBlockHeaderLength() table
// and each block's own case body -- not assumed from a general TZX spec
// summary. Header lengths there are measured including the block's own
// ID byte; this package's own pos convention has already consumed the ID
// byte by the time each case body runs, so each header length below is
// one less than SpecIde's own table value for the same block ID.

// tzxHeader10 returns a minimal valid 10-byte TZX header, matching this
// package's own decode entry point (pos starts at 10).
func tzxHeader10() []byte {
	return []byte{'Z', 'X', 'T', 'a', 'p', 'e', '!', eofMarker, 1, 20}
}

// TestDecode_PureTone confirms 0x12 (Pure Tone) decodes its two u16
// fields correctly. SpecIde header length 0x05 = id(1) + pilotPulse(2) +
// pilotLength(2); no further payload.
func TestDecode_PureTone(t *testing.T) {
	data := tzxHeader10()
	data = append(data, idPureTone)
	data = append(data, le16(2168)...) // pilotPulse
	data = append(data, le16(4032)...) // pilotLength

	blocks, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	b := blocks[0]
	if b.ID != idPureTone {
		t.Errorf("ID = 0x%02X, want 0x%02X", b.ID, idPureTone)
	}
	if b.PilotPulse != 2168 {
		t.Errorf("PilotPulse = %d, want 2168", b.PilotPulse)
	}
	if b.PilotLength != 4032 {
		t.Errorf("PilotLength = %d, want 4032", b.PilotLength)
	}
}

// TestDecode_PulseSequence confirms 0x13 (Sequence Of Pulses) decodes a
// count byte followed by that many u16 pulse durations. SpecIde header
// length 0x02 = id(1) + count(1); count * 2 bytes of pulse data follow.
func TestDecode_PulseSequence(t *testing.T) {
	data := tzxHeader10()
	data = append(data, idPulseSeq)
	data = append(data, 3) // 3 pulses follow
	data = append(data, le16(667)...)
	data = append(data, le16(735)...)
	data = append(data, le16(954)...)

	blocks, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	want := []int{667, 735, 954}
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

// TestDecode_Pause confirms 0x20 (Pause/Stop The Tape) decodes its single
// u16 pause field. SpecIde header length 0x03 = id(1) + pause(2). The
// pause==0 "stop the tape" semantics (distinct from an ordinary pause --
// see zenzx's own tape_types.go for the full reasoning, ported from the
// same SpecIde source) are a pulse-expansion concern, not a structural
// decode one: this package's Decode reports the raw pause value
// (including zero) unconditionally; interpreting zero as "stop" is the
// caller's job, the same separation this package already has between
// parsing block structure and generating pulse timings for 0x10.
func TestDecode_Pause(t *testing.T) {
	cases := []struct {
		name  string
		pause uint16
	}{
		{"ordinary pause", 1000},
		{"zero -- stop the tape signal, reported as-is", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := tzxHeader10()
			data = append(data, idPause)
			data = append(data, le16(c.pause)...)

			blocks, err := Decode(data)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if len(blocks) != 1 {
				t.Fatalf("got %d blocks, want 1", len(blocks))
			}
			if blocks[0].Pause != c.pause {
				t.Errorf("Pause = %d, want %d", blocks[0].Pause, c.pause)
			}
		})
	}
}

// TestDecode_MixedRealWorldSequence is the actual point of this
// increment: a TZX image using several block types together, the shape
// a real commercial-game tape commonly takes (pilot tone, then a custom
// pulse sequence, then a standard block, then a pause) -- confirming the
// parser advances `pos` correctly across mixed block types in sequence,
// not just that each type parses correctly in isolation.
func TestDecode_MixedRealWorldSequence(t *testing.T) {
	data := tzxHeader10()

	data = append(data, idPureTone)
	data = append(data, le16(2168)...)
	data = append(data, le16(100)...)

	data = append(data, idPulseSeq)
	data = append(data, 2)
	data = append(data, le16(667)...)
	data = append(data, le16(735)...)

	data = append(data, idPause)
	data = append(data, le16(500)...)

	blocks, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(blocks))
	}
	if blocks[0].ID != idPureTone || blocks[0].PilotLength != 100 {
		t.Errorf("block 0: got %+v", blocks[0])
	}
	if blocks[1].ID != idPulseSeq || len(blocks[1].Pulses) != 2 {
		t.Errorf("block 1: got %+v", blocks[1])
	}
	if blocks[2].ID != idPause || blocks[2].Pause != 500 {
		t.Errorf("block 2: got %+v", blocks[2])
	}
}

// le16 encodes v as two little-endian bytes.
func le16(v uint16) []byte {
	return []byte{byte(v), byte(v >> 8)}
}

// le24 encodes v (must fit in 24 bits) as three little-endian bytes, for
// building 0x11's 24-bit data-length field by hand.
func le24(v uint32) []byte {
	return []byte{byte(v), byte(v >> 8), byte(v >> 16)}
}

// TestDecode_TurboSpeedData confirms 0x11 (Turbo Speed Data) decodes all
// nine numeric header fields and the variable-length payload correctly.
// This is the block type most commercial games' custom loaders actually
// use (faster-than-standard timing, often with a non-8-bit final byte for
// copy-protection schemes), and the most complex of this package's new
// additions: nine fields packed tightly before a 24-bit length and the
// payload itself. Byte layout confirmed against SpecIde's own
// TZXFile.cc: pilotPulse(2) syncPulse1(2) syncPulse2(2) dataPulse0(2)
// dataPulse1(2) pilotLength(2) bitsInLastByte(1) pause(2) dataLength(3),
// header length 0x13 (19, including the ID byte SpecIde's own table
// counts but this package's pos convention has already consumed).
func TestDecode_TurboSpeedData(t *testing.T) {
	data := tzxHeader10()
	data = append(data, idTurboSpeed)
	data = append(data, le16(2168)...) // pilotPulse
	data = append(data, le16(667)...)  // syncPulse1
	data = append(data, le16(735)...)  // syncPulse2
	data = append(data, le16(855)...)  // dataPulse0
	data = append(data, le16(1710)...) // dataPulse1
	data = append(data, le16(3223)...) // pilotLength
	data = append(data, 4)             // bitsInLastByte -- deliberately not 8, a real, common case
	data = append(data, le16(1000)...) // pause
	data = append(data, le24(3)...)    // dataLength
	data = append(data, 0xAA, 0xBB, 0xCC)

	// A following marker block, to prove pos ends up in the right place
	// after a 0x11 block -- not just that the fields inside it decode
	// correctly in isolation.
	data = append(data, idPause)
	data = append(data, le16(50)...)

	blocks, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2 (the 0x11 block plus the following marker)", len(blocks))
	}

	b := blocks[0]
	check := func(name string, got, want any) {
		if got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	check("PilotPulse", b.PilotPulse, uint16(2168))
	check("SyncPulse1", b.SyncPulse1, uint16(667))
	check("SyncPulse2", b.SyncPulse2, uint16(735))
	check("DataPulse0", b.DataPulse0, uint16(855))
	check("DataPulse1", b.DataPulse1, uint16(1710))
	check("PilotLength", b.PilotLength, uint16(3223))
	check("BitsInLastByte", b.BitsInLastByte, uint8(4))
	check("Pause", b.Pause, uint16(1000))
	if string(b.Data) != string([]byte{0xAA, 0xBB, 0xCC}) {
		t.Errorf("Data = %v, want [0xAA 0xBB 0xCC]", b.Data)
	}

	if blocks[1].ID != idPause || blocks[1].Pause != 50 {
		t.Errorf("marker block after 0x11: got %+v, want ID=0x20 Pause=50", blocks[1])
	}
}

// TestDecode_PureData confirms 0x14 (Pure Data) decodes its five fields
// correctly -- structurally identical to 0x11 minus the pilot/sync
// fields, reusing DataPulse0/DataPulse1/BitsInLastByte/Pause/Data.
// SpecIde header length 0x0B (11) = id(1) + dataPulse0(2) +
// dataPulse1(2) + bitsInLastByte(1) + pause(2) + dataLength(3).
func TestDecode_PureData(t *testing.T) {
	data := tzxHeader10()
	data = append(data, idPureData)
	data = append(data, le16(855)...)  // dataPulse0
	data = append(data, le16(1710)...) // dataPulse1
	data = append(data, 6)             // bitsInLastByte
	data = append(data, le16(200)...)  // pause
	data = append(data, le24(2)...)    // dataLength
	data = append(data, 0x11, 0x22)

	blocks, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	b := blocks[0]
	if b.DataPulse0 != 855 || b.DataPulse1 != 1710 || b.BitsInLastByte != 6 || b.Pause != 200 {
		t.Errorf("got DataPulse0=%d DataPulse1=%d BitsInLastByte=%d Pause=%d, want 855/1710/6/200",
			b.DataPulse0, b.DataPulse1, b.BitsInLastByte, b.Pause)
	}
	if string(b.Data) != string([]byte{0x11, 0x22}) {
		t.Errorf("Data = %v, want [0x11 0x22]", b.Data)
	}
}

// TestDecode_DirectRecording confirms 0x15 (Direct Recording) decodes
// its four fields correctly. SpecIde header length 0x09 (9) = id(1) +
// sampleStep(2) + pause(2) + bitsInLastByte(1) + dataLength(3).
func TestDecode_DirectRecording(t *testing.T) {
	data := tzxHeader10()
	data = append(data, idDirectRec)
	data = append(data, le16(79)...) // sampleStep
	data = append(data, le16(0)...)  // pause
	data = append(data, 8)           // bitsInLastByte
	data = append(data, le24(4)...)  // dataLength
	data = append(data, 0xFF, 0x00, 0xFF, 0x00)

	blocks, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	b := blocks[0]
	if b.SampleStep != 79 || b.BitsInLastByte != 8 || b.Pause != 0 {
		t.Errorf("got SampleStep=%d BitsInLastByte=%d Pause=%d, want 79/8/0",
			b.SampleStep, b.BitsInLastByte, b.Pause)
	}
	if string(b.Data) != string([]byte{0xFF, 0x00, 0xFF, 0x00}) {
		t.Errorf("Data = %v, want [0xFF 0x00 0xFF 0x00]", b.Data)
	}
}

// TestDecode_SetSignalLevel confirms 0x2B decodes both the fixed
// (always-1) length field correctly consumed and the level byte itself,
// for both level values -- not just that it doesn't crash.
func TestDecode_SetSignalLevel(t *testing.T) {
	for _, want := range []bool{true, false} {
		data := tzxHeader10()
		data = append(data, idSetSignalLvl)
		data = append(data, le32(1)...) // block length, always 1
		if want {
			data = append(data, 1)
		} else {
			data = append(data, 0)
		}

		blocks, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if len(blocks) != 1 {
			t.Fatalf("got %d blocks, want 1", len(blocks))
		}
		if blocks[0].SignalLevel != want {
			t.Errorf("SignalLevel = %v, want %v", blocks[0].SignalLevel, want)
		}
	}
}

// TestDecode_GeneralizedData confirms 0x19's six structural fields
// decode correctly and pos advances exactly past the alphabet tables and
// symbol streams -- computed independently by walking their own
// self-describing lengths, matching SpecIde's own "assert(pointer ==
// tableIndex)" sanity check (a real returned error here, not a
// debug-only assert). Full alphabet-driven pulse expansion is
// deliberately out of scope for this package for now -- see the
// SymbolsInPilot field's own doc comment on Block for why -- so this
// test checks structure and correct advancement, not decoded pulses.
//
// Fixture: 2 pilot symbols (alphabet size 2, max symbol length 1) in the
// pilot stream, 4 data symbols (alphabet size 2, so bps=1: 4 bits,
// packed into a single byte) in the data stream. Sized by hand from the
// same formulas the implementation uses, so the test is a genuine check
// against an independently-reasoned expectation, not the implementation
// checking itself.
func TestDecode_GeneralizedData(t *testing.T) {
	// Pilot alphabet: 2 symbols, each type(1) + 1 pulse value(2) = 3 bytes/symbol, 6 bytes total.
	pilotAlphabet := []byte{
		0x00, 0x00, 0x00, // symbol 0: type=Edge, pulse=0 (unused/terminator)
		0x01, 0x00, 0x00, // symbol 1: type=Continue, pulse=0
	}
	// Pilot stream: 2 entries, each symbol(1) + repeat_u16(2) = 3 bytes/entry, 6 bytes total.
	pilotStream := []byte{
		0, 0, 10, // symbol 0, repeat 10
		1, 0, 5, // symbol 1, repeat 5
	}
	// Data alphabet: 3 symbols (deliberately not a power of two -- with
	// an alphabet size where log2 lands exactly on an integer, such as
	// 2, dropping the ceiling in bps=ceil(log2(size)) would not actually
	// change the result, and a test built on that size would not catch
	// the bug it is meant to catch. Confirmed directly: an earlier draft
	// of this test used size 2 and passed even with the ceiling removed
	// from the implementation, silently. Size 3 needs bps=2 (log2(3)~=
	// 1.585, ceil=2); a truncating bps=1 would compute a different,
	// wrong data-stream byte length below, which this test's own
	// AlphabetData length check would then catch), same 3-byte shape
	// per symbol, 9 bytes total.
	dataAlphabet := []byte{
		0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
	}
	// Data stream: 5 symbols at bps=2 (ceil(log2(3))=2) = 10 bits,
	// packed into 2 bytes (ceil(10/8)=2) -- a bps=1 (wrong) computation
	// would instead need ceil(5/8)=1 byte, a different, checkable length.
	dataStream := []byte{0b01101100, 0b10000000}

	tableLen := len(pilotAlphabet) + len(pilotStream) + len(dataAlphabet) + len(dataStream)
	dataLength := tableLen + 14

	data := tzxHeader10()
	data = append(data, idGeneralized)
	data = append(data, le32(uint32(dataLength))...)
	data = append(data, le16(0)...) // pause
	data = append(data, le32(2)...) // symbolsInPilot
	data = append(data, 1)          // maxPilotSymLen
	data = append(data, 2)          // pilotAlphabetSize
	data = append(data, le32(5)...) // symbolsInData
	data = append(data, 1)          // maxDataSymLen
	data = append(data, 3)          // dataAlphabetSize
	data = append(data, pilotAlphabet...)
	data = append(data, pilotStream...)
	data = append(data, dataAlphabet...)
	data = append(data, dataStream...)

	// Trailing marker, to confirm pos ends up exactly right afterward.
	data = append(data, idGroupEnd)

	blocks, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2 (the 0x19 block plus the trailing marker)", len(blocks))
	}
	b := blocks[0]
	if b.SymbolsInPilot != 2 || b.MaxPilotSymLen != 1 || b.PilotAlphabetSize != 2 {
		t.Errorf("pilot fields: got SymbolsInPilot=%d MaxPilotSymLen=%d PilotAlphabetSize=%d, want 2/1/2",
			b.SymbolsInPilot, b.MaxPilotSymLen, b.PilotAlphabetSize)
	}
	if b.SymbolsInData != 5 || b.MaxDataSymLen != 1 || b.DataAlphabetSize != 3 {
		t.Errorf("data fields: got SymbolsInData=%d MaxDataSymLen=%d DataAlphabetSize=%d, want 5/1/3",
			b.SymbolsInData, b.MaxDataSymLen, b.DataAlphabetSize)
	}
	if len(b.AlphabetData) != tableLen {
		t.Errorf("AlphabetData length = %d, want %d", len(b.AlphabetData), tableLen)
	}
	if blocks[1].ID != idGroupEnd {
		t.Errorf("trailing block ID = 0x%02X, want 0x%02X -- pos did not advance to the right place", blocks[1].ID, idGroupEnd)
	}
}

// TestDecode_GeneralizedData_LengthMismatchRejected confirms a declared
// dataLength that does not match the alphabet/stream tables' own
// computed length is a real, returned decode error -- not silently
// accepted with pos left in the wrong place for whatever follows.
func TestDecode_GeneralizedData_LengthMismatchRejected(t *testing.T) {
	data := tzxHeader10()
	data = append(data, idGeneralized)
	data = append(data, le32(999)...) // deliberately wrong -- does not match the real table length
	data = append(data, le16(0)...)
	data = append(data, le32(0)...) // symbolsInPilot=0
	data = append(data, 0, 0)
	data = append(data, le32(0)...) // symbolsInData=0
	data = append(data, 0, 0)

	_, err := Decode(data)
	if err == nil {
		t.Fatal("expected an error for a dataLength that does not match the computed table length, got none")
	}
}
