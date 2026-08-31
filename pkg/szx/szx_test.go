package szx

import (
	"testing"

	"github.com/ha1tch/zentools/pkg/snapshot"
)

func testState(model snapshot.Model) *snapshot.MachineState {
	s := &snapshot.MachineState{Model: model}
	s.CPU = snapshot.CPU{
		AF: 0x1234, BC: 0x5678, DE: 0x9ABC, HL: 0xDEF0,
		AF_: 0x1111, BC_: 0x2222, DE_: 0x3333, HL_: 0x4444,
		IX: 0x5555, IY: 0x6666, SP: 0xFF00, PC: 0x8000,
		I: 0x3F, R: 0x01, IFF1: true, IFF2: true, IM: 1,
	}
	s.IO.Border = 4
	for b := range s.Memory.RAM {
		s.Memory.RAM[b][0] = byte(0xC0 + b)
	}
	return s
}

func TestSZXRoundTrip48K(t *testing.T) {
	want := testState(snapshot.Model48K)
	image, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if string(image[0:4]) != "ZXST" {
		t.Fatalf("missing ZXST signature, got % X", image[0:4])
	}

	got, err := Decode(image)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Model != snapshot.Model48K {
		t.Errorf("Model = %v, want Model48K", got.Model)
	}
	if got.CPU != want.CPU {
		t.Errorf("CPU = %+v, want %+v", got.CPU, want.CPU)
	}
	if got.IO.Border != want.IO.Border {
		t.Errorf("Border = %d, want %d", got.IO.Border, want.IO.Border)
	}
	// 48K only saves banks 5, 2, 0 -- confirm those round-trip and don't
	// assert on the others, which Encode never wrote for this model.
	for _, b := range []int{5, 2, 0} {
		if got.Memory.RAM[b][0] != want.Memory.RAM[b][0] {
			t.Errorf("bank %d[0] = %#02x, want %#02x", b, got.Memory.RAM[b][0], want.Memory.RAM[b][0])
		}
	}
}

func TestSZXRoundTrip128K(t *testing.T) {
	want := testState(snapshot.Model128K)
	want.Paging.Port7FFD = 0x05
	want.Paging.Port1FFD = 0x02

	image, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(image)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Paging.Port7FFD != 0x05 {
		t.Errorf("Port7FFD = %#02x, want 0x05", got.Paging.Port7FFD)
	}
	if got.Paging.Port1FFD != 0x02 {
		t.Errorf("Port1FFD = %#02x, want 0x02", got.Paging.Port1FFD)
	}
	// 128K saves all eight banks.
	for b := 0; b < 8; b++ {
		if got.Memory.RAM[b][0] != want.Memory.RAM[b][0] {
			t.Errorf("bank %d[0] = %#02x, want %#02x", b, got.Memory.RAM[b][0], want.Memory.RAM[b][0])
		}
	}
}

func TestSZXRoundTripAllModels(t *testing.T) {
	for _, m := range []snapshot.Model{snapshot.Model48K, snapshot.Model128K, snapshot.ModelPlus2, snapshot.ModelPlus2A, snapshot.ModelPlus3} {
		want := testState(m)
		image, err := Encode(want)
		if err != nil {
			t.Fatalf("Model %v: Encode: %v", m, err)
		}
		got, err := Decode(image)
		if err != nil {
			t.Fatalf("Model %v: Decode: %v", m, err)
		}
		if got.Model != m {
			t.Errorf("Model %v: got.Model = %v", m, got.Model)
		}
	}
}

func TestSZXDecode_UnknownBlockSkipped(t *testing.T) {
	// A well-formed file with an extra, unrecognised block (a made-up
	// "JUNK" tag) in the middle must still decode cleanly -- the spec's
	// own rule, not a nice-to-have.
	want := testState(snapshot.Model48K)
	image, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	junk := withBlockHeader(blockID("JUNK"), []byte{1, 2, 3, 4, 5})
	// Insert the junk block right after the 8-byte header.
	spliced := append(append(append([]byte{}, image[:8]...), junk...), image[8:]...)

	got, err := Decode(spliced)
	if err != nil {
		t.Fatalf("Decode with an unknown block present: %v", err)
	}
	if got.CPU.PC != want.CPU.PC {
		t.Errorf("PC = %#04x, want %#04x (decode should still find the real blocks after skipping JUNK)", got.CPU.PC, want.CPU.PC)
	}
}

func TestSZXDecode_MissingSignature(t *testing.T) {
	_, err := Decode([]byte("not an szx file at all"))
	if err == nil {
		t.Fatal("expected an error for missing ZXST signature, got nil")
	}
}

func TestSZXDecode_UnsupportedMachine(t *testing.T) {
	image, _ := Encode(testState(snapshot.Model48K))
	// Corrupt the machine ID to Pentagon 512 (13), out of scope.
	image[6] = 13
	_, err := Decode(image)
	if err == nil {
		t.Fatal("expected an error decoding an unsupported machine ID (Pentagon 512), got nil")
	}
}

func TestSZXDecode_MissingRequiredBlock(t *testing.T) {
	// A header with no ZXSTZ80REGS/ZXSTSPECREGS blocks at all.
	image := []byte{'Z', 'X', 'S', 'T', 1, 4, machine48K, 0}
	_, err := Decode(image)
	if err == nil {
		t.Fatal("expected an error for a file missing required blocks, got nil")
	}
}
