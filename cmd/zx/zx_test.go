package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ha1tch/zentools/pkg/build"
	"github.com/ha1tch/zentools/pkg/snapshot"
	"github.com/ha1tch/zentools/pkg/tap"
)

// sampleCodeTAP returns a TAP image holding one CODE block at 0x8000.
func sampleCodeTAP() []byte {
	return tap.EncodeCode("test", []byte{0xF3, 0x21, 0x00, 0x40, 0x76}, 0x8000)
}

// --- permuteArgs ------------------------------------------------------------

func TestPermuteArgsFlagsAfterPositional(t *testing.T) {
	// "input.bin --start 0x8000 --sna" with --sna a bool flag.
	got := permuteArgs(
		[]string{"input.bin", "--start", "0x8000", "--sna"},
		map[string]bool{"sna": true},
	)
	// Flags (and the value-flag's value) come first; positional last.
	want := []string{"--start", "0x8000", "--sna", "input.bin"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestPermuteArgsBoolDoesNotEatPositional(t *testing.T) {
	// "--sna input.bin": --sna is boolean, so input.bin must remain positional,
	// not be consumed as --sna's value.
	got := permuteArgs([]string{"--sna", "input.bin"}, map[string]bool{"sna": true})
	if len(got) != 2 || got[0] != "--sna" || got[1] != "input.bin" {
		t.Fatalf("bool flag ate the positional: %v", got)
	}
}

func TestPermuteArgsEqualsForm(t *testing.T) {
	got := permuteArgs([]string{"in.bin", "--start=0x8000"}, nil)
	if got[0] != "--start=0x8000" || got[1] != "in.bin" {
		t.Fatalf("equals-form mishandled: %v", got)
	}
}

// --- convert matrix ---------------------------------------------------------

func TestConvertTapToTzxRoundTrip(t *testing.T) {
	src := sampleCodeTAP()
	tzxImg, err := convertTapeToTape(src, "tap", "tzx")
	if err != nil {
		t.Fatal(err)
	}
	back, err := convertTapeToTape(tzxImg, "tzx", "tap")
	if err != nil {
		t.Fatal(err)
	}
	if string(back) != string(src) {
		t.Fatalf("tap->tzx->tap not byte-identical (%d vs %d bytes)", len(back), len(src))
	}
}

func TestConvertSnapToSnapRoundTrip(t *testing.T) {
	// Build a snapshot, convert z80->sna->z80, check PC/SP survive.
	z80, err := build.EncodeZ80(build.Request{
		Code: []byte{0xF3, 0x76}, Origin: 0x8000, Start: 0x8000, SP: 0xFF00, Model: build.Model48K,
	})
	if err != nil {
		t.Fatal(err)
	}
	sna, err := convertSnapToSnap(z80, "z80", "sna")
	if err != nil {
		t.Fatal(err)
	}
	back, err := convertSnapToSnap(sna, "sna", "z80")
	if err != nil {
		t.Fatal(err)
	}
	s, err := snapshot.DecodeZ80(back)
	if err != nil {
		t.Fatal(err)
	}
	if s.CPU.PC != 0x8000 {
		t.Errorf("PC = 0x%04X after z80->sna->z80, want 0x8000", s.CPU.PC)
	}
	// The 48K SNA format stores PC by pushing it onto the stack; the decoder
	// pops it back, so SP is restored to its original value. (One stack byte
	// near SP is left perturbed in RAM, which is inherent to the SNA format and
	// does not affect SP or PC.)
	if s.CPU.SP != 0xFF00 {
		t.Errorf("SP = 0x%04X after round-trip through SNA, want 0xFF00", s.CPU.SP)
	}
}

func TestConvertTapeToSnapNeedsStart(t *testing.T) {
	_, err := convertTapeToSnap(sampleCodeTAP(), "tap", "z80", "", "0xFF00", "48k")
	if err == nil {
		t.Fatal("tape->snap without --start should error")
	}
}

func TestConvertTapeToSnapWithStart(t *testing.T) {
	img, err := convertTapeToSnap(sampleCodeTAP(), "tap", "z80", "0x8000", "0xFF00", "48k")
	if err != nil {
		t.Fatal(err)
	}
	s, err := snapshot.DecodeZ80(img)
	if err != nil {
		t.Fatal(err)
	}
	if s.CPU.PC != 0x8000 {
		t.Errorf("converted snapshot PC = 0x%04X, want 0x8000", s.CPU.PC)
	}
}

func TestConvertSnapToTapeProducesValidTAP(t *testing.T) {
	z80, _ := build.EncodeZ80(build.Request{
		Code: []byte{0xF3, 0x76}, Origin: 0x8000, Start: 0x8000, SP: 0xFF00, Model: build.Model48K,
	})
	tapImg, err := convertSnapToTape(z80, "z80", "tap")
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := tap.Decode(tapImg)
	if err != nil {
		t.Fatalf("snap->tape produced an invalid TAP: %v", err)
	}
	// Expect a header + data pair for the memory dump.
	if len(blocks) != 2 || !blocks[0].IsHeader || blocks[0].Type != tap.TypeCode {
		t.Fatalf("unexpected memdump tape structure: %d blocks", len(blocks))
	}
}

func TestFormatOfAndKindOf(t *testing.T) {
	cases := map[string][2]string{
		"a.tap": {"tap", "tape"},
		"a.tzx": {"tzx", "tape"},
		"a.sna": {"sna", "snap"},
		"a.z80": {"z80", "snap"},
		"a.bin": {"", ""},
	}
	for path, want := range cases {
		if f := formatOf(path); f != want[0] {
			t.Errorf("formatOf(%q) = %q, want %q", path, f, want[0])
		}
		if k := kindOf(formatOf(path)); k != want[1] {
			t.Errorf("kindOf(formatOf(%q)) = %q, want %q", path, k, want[1])
		}
	}
}

// --- CLI integration --------------------------------------------------------

func buildZX(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "zx")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building zx: %v\n%s", err, out)
	}
	return bin
}

func TestZXSnapMakeAndInfo(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "code.bin")
	os.WriteFile(in, []byte{0xF3, 0x76}, 0o644)
	base := filepath.Join(dir, "out")

	// Flags after the positional, to exercise permuteArgs end-to-end.
	if o, err := exec.Command(zx, "snap", "make", in, "--start", "0x8000", "--z80", "-o", base).CombinedOutput(); err != nil {
		t.Fatalf("zx snap make: %v\n%s", err, o)
	}
	z80 := base + ".z80"
	if _, err := os.Stat(z80); err != nil {
		t.Fatalf("no .z80 produced: %v", err)
	}
	if o, err := exec.Command(zx, "snap", "info", z80).CombinedOutput(); err != nil {
		t.Fatalf("zx snap info: %v\n%s", err, o)
	}
}
