package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ha1tch/zentools/pkg/build"
	"github.com/ha1tch/zentools/pkg/pzx"
	"github.com/ha1tch/zentools/pkg/rzx"
	"github.com/ha1tch/zentools/pkg/snapshot"
	"github.com/ha1tch/zentools/pkg/szx"
	"github.com/ha1tch/zentools/pkg/tap"
	"github.com/ha1tch/zentools/pkg/tzx"
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
		"a.pzx": {"pzx", "tape"},
		"a.sna": {"sna", "snap"},
		"a.z80": {"z80", "snap"},
		"a.szx": {"szx", "snap"},
		"a.bin": {"bin", "bin"}, // now a real target format (plain extraction), not unrecognised
		"a.xyz": {"", ""},       // genuinely unrecognised extension
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

func TestConvertSnapToSZXRoundTrip(t *testing.T) {
	z80, err := build.EncodeZ80(build.Request{
		Code: []byte{0xF3, 0x76}, Origin: 0x8000, Start: 0x8000, SP: 0xFF00, Model: build.Model48K,
	})
	if err != nil {
		t.Fatal(err)
	}
	szxImg, err := convertSnapToSnap(z80, "z80", "szx")
	if err != nil {
		t.Fatal(err)
	}
	back, err := convertSnapToSnap(szxImg, "szx", "z80")
	if err != nil {
		t.Fatal(err)
	}
	s, err := snapshot.DecodeZ80(back)
	if err != nil {
		t.Fatal(err)
	}
	if s.CPU.PC != 0x8000 {
		t.Errorf("PC = 0x%04X after z80->szx->z80, want 0x8000", s.CPU.PC)
	}
}

func TestZXSnapMakeAndInfo_SZX(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "code.bin")
	os.WriteFile(in, []byte{0xF3, 0x76}, 0o644)
	base := filepath.Join(dir, "out")

	if o, err := exec.Command(zx, "snap", "make", in, "--start", "0x8000", "--szx", "-o", base).CombinedOutput(); err != nil {
		t.Fatalf("zx snap make --szx: %v\n%s", err, o)
	}
	szxFile := base + ".szx"
	if _, err := os.Stat(szxFile); err != nil {
		t.Fatalf("no .szx produced: %v", err)
	}
	if o, err := exec.Command(zx, "snap", "info", szxFile).CombinedOutput(); err != nil {
		t.Fatalf("zx snap info: %v\n%s", err, o)
	}
}

func TestZXInfo_DetectsSZXBySignature(t *testing.T) {
	// A .szx file with no extension at all -- zx info must still recognise
	// it via the ZXST signature, not just the extension.
	zx := buildZX(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "code.bin")
	os.WriteFile(in, []byte{0xF3, 0x76}, 0o644)
	base := filepath.Join(dir, "out")
	if o, err := exec.Command(zx, "snap", "make", in, "--start", "0x8000", "--szx", "-o", base).CombinedOutput(); err != nil {
		t.Fatalf("zx snap make --szx: %v\n%s", err, o)
	}
	noExt := filepath.Join(dir, "extensionless")
	data, err := os.ReadFile(base + ".szx")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(noExt, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if o, err := exec.Command(zx, "info", noExt).CombinedOutput(); err != nil {
		t.Fatalf("zx info on an extensionless SZX file: %v\n%s", err, o)
	}
}

func TestZXPZXMakeAndInfo(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	binPath := filepath.Join(dir, "code.bin")
	os.WriteFile(binPath, []byte("hello world data"), 0o644)
	tapPath := filepath.Join(dir, "code.tap")

	if o, err := exec.Command(zx, "tap", "make", binPath, "--name", "TEST", "-o", tapPath).CombinedOutput(); err != nil {
		t.Fatalf("zx tap make: %v\n%s", err, o)
	}

	pzxPath := filepath.Join(dir, "code.pzx")
	if o, err := exec.Command(zx, "pzx", "make", tapPath, "--title", "Test", "-o", pzxPath).CombinedOutput(); err != nil {
		t.Fatalf("zx pzx make: %v\n%s", err, o)
	}
	if _, err := os.Stat(pzxPath); err != nil {
		t.Fatalf("no .pzx produced: %v", err)
	}

	out, err := exec.Command(zx, "pzx", "info", pzxPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx pzx info: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PZXT") {
		t.Errorf("zx pzx info output missing PZXT block, got:\n%s", out)
	}

	// zx info must auto-detect PZX via its signature.
	out, err = exec.Command(zx, "info", pzxPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx info on a .pzx file: %v\n%s", err, out)
	}
}

func TestEncodePZXFromTAPBlocks_RoundTripsThroughRealDecoder(t *testing.T) {
	// Build a real TAP with a header+data pair, encode to PZX, decode the
	// PZX back through pzxToTAP, and confirm the result is byte-for-byte
	// identical to the original TAP -- not just "looks non-empty". An
	// earlier version of encodePZXFromTAPBlocks used tap.Block.Data
	// directly as if it were a block's full raw bytes; it's actually
	// documented as "payload between the flag and the checksum",
	// excluding both, so every encoded block came out 2 bytes short.
	// This exact test, run with only a "len(Data) != 0" assertion,
	// passed anyway -- a short-but-nonempty payload still satisfies
	// that check. Only a real byte-for-byte round trip catches it.
	tapImage := tap.EncodeCode("RT", []byte{0x01, 0x02, 0x03}, 0x8000)
	blocks, err := tap.Decode(tapImage)
	if err != nil {
		t.Fatal(err)
	}
	img, err := encodePZXFromTAPBlocks(blocks, "Round Trip")
	if err != nil {
		t.Fatal(err)
	}
	f, err := pzx.Decode(img)
	if err != nil {
		t.Fatalf("pzx.Decode: %v", err)
	}
	h, ok := f.Blocks[0].(pzx.HeaderBlock)
	if !ok || h.Title != "Round Trip" {
		t.Errorf("Blocks[0] = %+v, want HeaderBlock with Title=Round Trip", f.Blocks[0])
	}

	back, err := pzxToTAP(img)
	if err != nil {
		t.Fatalf("pzxToTAP: %v", err)
	}
	if string(back) != string(tapImage) {
		t.Errorf("TAP -> PZX -> TAP not byte-identical:\n  original: % X\n  got:      % X", tapImage, back)
	}
}

func TestZXRZXInfo(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	rzxPath := filepath.Join(dir, "test.rzx")

	f := &rzx.File{
		MajorVersion: 0, MinorVersion: 13,
		Blocks: []rzx.Block{
			rzx.CreatorBlock{ID: "zx_test", Major: 1, Minor: 0},
			rzx.RecordingBlock{
				Frames: []rzx.Frame{
					{FetchCount: 1000, PortReads: []byte{0xFF}},
					{FetchCount: 1000, Repeated: true},
				},
			},
		},
	}
	img, err := rzx.Encode(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rzxPath, img, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(zx, "rzx", "info", rzxPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx rzx info: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "zx_test") || !strings.Contains(string(out), "2 frames") {
		t.Errorf("zx rzx info output missing expected content, got:\n%s", out)
	}

	// zx info must auto-detect RZX via its signature.
	out, err = exec.Command(zx, "info", rzxPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx info on a .rzx file: %v\n%s", err, out)
	}
}

func TestConvertTapToPZX(t *testing.T) {
	// Confirmed as a real bug via the real binary, not just this test:
	// convertTapeToTape had no case for dst=="pzx" at all, so it silently
	// fell through to the tzx.EncodeFromTAP branch -- `zx convert x.tap -o
	// y.pzx` produced a TZX-encoded file mislabelled with a .pzx extension.
	tapImage := sampleCodeTAP()
	img, err := convertTapeToTape(tapImage, "tap", "pzx")
	if err != nil {
		t.Fatal(err)
	}
	f, err := pzx.Decode(img)
	if err != nil {
		t.Fatalf("result is not a valid PZX file: %v", err)
	}
	if _, ok := f.Blocks[0].(pzx.HeaderBlock); !ok {
		t.Errorf("Blocks[0] = %T, want HeaderBlock", f.Blocks[0])
	}
}

func TestConvertRejectsSameFormat(t *testing.T) {
	// zx convert is for converting between formats; same-format "conversion"
	// is rejected outright rather than silently copying the file (which
	// could mask an actual mistake, like a typo'd extension) or -- the
	// earlier, real bug -- silently reprocessing through lossy
	// normalisation for something that was nominally a no-op.
	zx := buildZX(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "code.bin")
	os.WriteFile(in, []byte("hello"), 0o644)
	tapPath := filepath.Join(dir, "code.tap")
	if o, err := exec.Command(zx, "tap", "make", in, "--name", "X", "-o", tapPath).CombinedOutput(); err != nil {
		t.Fatalf("zx tap make: %v\n%s", err, o)
	}

	out, err := exec.Command(zx, "convert", tapPath, "-o", filepath.Join(dir, "copy.tap")).CombinedOutput()
	if err == nil {
		t.Fatal("zx convert tap -> tap should be rejected, got no error")
	}
	if !strings.Contains(string(out), "convert is for converting between formats") {
		t.Errorf("error message doesn't explain why, got:\n%s", out)
	}
}

func TestConvertPZXToTZX(t *testing.T) {
	tapImage := sampleCodeTAP()
	blocks, err := tap.Decode(tapImage)
	if err != nil {
		t.Fatal(err)
	}
	pzxImage, err := encodePZXFromTAPBlocks(blocks, "")
	if err != nil {
		t.Fatal(err)
	}
	tzxImage, err := convertTapeToTape(pzxImage, "pzx", "tzx")
	if err != nil {
		t.Fatal(err)
	}
	tzxBlocks, err := tzx.Decode(tzxImage)
	if err != nil {
		t.Fatalf("result is not a valid TZX file: %v", err)
	}
	if len(tzxBlocks) == 0 || tzxBlocks[0].ID != 0x10 {
		t.Errorf("tzxBlocks = %+v, want at least one 0x10 block", tzxBlocks)
	}
}

func TestZXConvert_BinToTapeAndSnap(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "code.bin")
	os.WriteFile(bin, []byte("raw code"), 0o644)

	tapPath := filepath.Join(dir, "code.tap")
	if o, err := exec.Command(zx, "convert", bin, "-o", tapPath, "--name", "RAW", "--origin", "0x9000").CombinedOutput(); err != nil {
		t.Fatalf("zx convert bin -> tap: %v\n%s", err, o)
	}
	blocks, err := os.ReadFile(tapPath)
	if err != nil || len(blocks) == 0 {
		t.Fatalf("no .tap produced: %v", err)
	}

	snaPath := filepath.Join(dir, "code.sna")
	if o, err := exec.Command(zx, "convert", bin, "-o", snaPath, "--origin", "0x9000", "--start", "0x9000").CombinedOutput(); err != nil {
		t.Fatalf("zx convert bin -> sna: %v\n%s", err, o)
	}
	s, err := snapshot.DecodeSNA(mustRead(t, snaPath))
	if err != nil {
		t.Fatalf("decoding produced .sna: %v", err)
	}
	if s.CPU.PC != 0x9000 {
		t.Errorf("PC = %#04x, want 0x9000", s.CPU.PC)
	}
}

func TestZXConvert_RZXToSnapAndBin(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()

	s := &snapshot.MachineState{Model: snapshot.Model48K}
	s.CPU.PC = 0x8500
	copy(s.Memory.RAM[2][0x500:], []byte{0x3E, 0x99, 0xC9})
	z80Img, err := snapshot.EncodeZ80(s)
	if err != nil {
		t.Fatal(err)
	}
	f := &rzx.File{
		MajorVersion: 0, MinorVersion: 13,
		Blocks: []rzx.Block{
			rzx.CreatorBlock{ID: "test", Major: 1, Minor: 0},
			rzx.SnapshotBlock{Extension: "Z80", Data: z80Img},
		},
	}
	rzxImg, err := rzx.Encode(f)
	if err != nil {
		t.Fatal(err)
	}
	rzxPath := filepath.Join(dir, "session.rzx")
	if err := os.WriteFile(rzxPath, rzxImg, 0o644); err != nil {
		t.Fatal(err)
	}

	// RZX -> bin: exact embedded code bytes.
	binPath := filepath.Join(dir, "code.bin")
	if o, err := exec.Command(zx, "convert", rzxPath, "-o", binPath, "--length", "3").CombinedOutput(); err != nil {
		t.Fatalf("zx convert rzx -> bin: %v\n%s", err, o)
	}
	got := mustRead(t, binPath)
	if string(got) != "\x3e\x99\xc9" {
		t.Errorf("extracted bytes = % X, want 3E 99 C9", got)
	}

	// RZX -> sna: cross-format extraction (embedded is Z80, target is SNA).
	snaPath := filepath.Join(dir, "extracted.sna")
	if o, err := exec.Command(zx, "convert", rzxPath, "-o", snaPath).CombinedOutput(); err != nil {
		t.Fatalf("zx convert rzx -> sna: %v\n%s", err, o)
	}
	snaState, err := snapshot.DecodeSNA(mustRead(t, snaPath))
	if err != nil {
		t.Fatal(err)
	}
	if snaState.CPU.PC != 0x8500 {
		t.Errorf("PC = %#04x after RZX -> sna, want 0x8500", snaState.CPU.PC)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return data
}

func TestConvertSnapToPZX(t *testing.T) {
	// Same bug class as TestConvertTapToPZX, found in the sibling
	// convertSnapToTape function while double-checking before presenting
	// the conversion matrix, not by a fresh report: it only explicitly
	// handled dst=="tap" and silently fell through to TZX-encoding for
	// anything else, including "pzx" -- `zx convert x.sna -o y.pzx`
	// produced a TZX-encoded file mislabelled with a .pzx extension.
	s := &snapshot.MachineState{Model: snapshot.Model48K}
	s.CPU.PC = 0x8000
	snaImage, err := snapshot.EncodeSNA(s)
	if err != nil {
		t.Fatal(err)
	}
	img, err := convertSnapToTape(snaImage, "sna", "pzx")
	if err != nil {
		t.Fatal(err)
	}
	f, err := pzx.Decode(img)
	if err != nil {
		t.Fatalf("result is not a valid PZX file: %v", err)
	}
	if _, ok := f.Blocks[0].(pzx.HeaderBlock); !ok {
		t.Errorf("Blocks[0] = %T, want HeaderBlock", f.Blocks[0])
	}
}

func TestZXConvertOutdir_Explode(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()

	l := filepath.Join(dir, "l.bin")
	m := filepath.Join(dir, "m.bin")
	os.WriteFile(l, []byte("loader"), 0o644)
	os.WriteFile(m, []byte("main game code"), 0o644)
	lTap := filepath.Join(dir, "l.tap")
	mTap := filepath.Join(dir, "m.tap")
	if o, err := exec.Command(zx, "tap", "make", l, "--name", "LOADER", "--origin", "0x8000", "-o", lTap).CombinedOutput(); err != nil {
		t.Fatalf("zx tap make: %v\n%s", err, o)
	}
	if o, err := exec.Command(zx, "tap", "make", m, "--name", "MAIN", "--origin", "0x9000", "-o", mTap).CombinedOutput(); err != nil {
		t.Fatalf("zx tap make: %v\n%s", err, o)
	}

	// Concatenate into a real multi-block tape, then append a bare,
	// headerless data block by hand -- the exact real-world shape found
	// earlier in this project's history (a commercial tape's own custom
	// loader blocks), which the manifest's header attribution must not
	// misattribute to the header two blocks back.
	multiPath := filepath.Join(dir, "multi.tap")
	lData := mustRead(t, lTap)
	mData := mustRead(t, mTap)
	var multi []byte
	multi = append(multi, lData...)
	multi = append(multi, mData...)
	bare := []byte("bareblock")
	flag := byte(0xFF)
	chk := flag
	for _, b := range bare {
		chk ^= b
	}
	block := append([]byte{flag}, bare...)
	block = append(block, chk)
	blockLen := uint16(len(block))
	multi = append(multi, byte(blockLen), byte(blockLen>>8))
	multi = append(multi, block...)
	if err := os.WriteFile(multiPath, multi, 0o644); err != nil {
		t.Fatal(err)
	}

	outdir := filepath.Join(dir, "extracted")
	out, err := exec.Command(zx, "convert", multiPath, "--outdir", outdir).CombinedOutput()
	if err != nil {
		t.Fatalf("zx convert --outdir: %v\n%s", err, out)
	}

	var got tapeManifest
	manifestData := mustRead(t, filepath.Join(outdir, "manifest.json"))
	if err := json.Unmarshal(manifestData, &got); err != nil {
		t.Fatalf("manifest.json is not valid JSON: %v", err)
	}
	if len(got.Blocks) != 5 {
		t.Fatalf("got %d manifest entries, want 5", len(got.Blocks))
	}

	// Index 1 (LOADER's data) must be attributed to header 0.
	if got.Blocks[1].HeaderIndex == nil || *got.Blocks[1].HeaderIndex != 0 || got.Blocks[1].HeaderName != "LOADER" {
		t.Errorf("Blocks[1] = %+v, want header_index=0 header_name=LOADER", got.Blocks[1])
	}
	// Index 3 (MAIN's data) must be attributed to header 2.
	if got.Blocks[3].HeaderIndex == nil || *got.Blocks[3].HeaderIndex != 2 {
		t.Errorf("Blocks[3] = %+v, want header_index=2", got.Blocks[3])
	}
	// Index 4 (the bare block) must NOT be attributed to any header --
	// this is the exact off-by-one case: pendingHeaderIdx must not still
	// read as "valid" from two blocks back.
	if got.Blocks[4].HeaderIndex != nil {
		t.Errorf("Blocks[4].HeaderIndex = %v, want nil (a bare block with no preceding header)", *got.Blocks[4].HeaderIndex)
	}

	if got := mustRead(t, filepath.Join(outdir, "001-LOADER.bin")); string(got) != "loader" {
		t.Errorf("001-LOADER.bin = %q, want %q", got, "loader")
	}
	if got := mustRead(t, filepath.Join(outdir, "003-MAIN.bin")); string(got) != "main game code" {
		t.Errorf("003-MAIN.bin = %q, want %q", got, "main game code")
	}
	if got := mustRead(t, filepath.Join(outdir, "004.bin")); string(got) != "bareblock" {
		t.Errorf("004.bin = %q, want %q", got, "bareblock")
	}
}

func TestZXConvertOutdir_RejectsNonTapeSource(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "code.bin")
	os.WriteFile(bin, []byte("x"), 0o644)

	out, err := exec.Command(zx, "convert", bin, "--outdir", filepath.Join(dir, "out")).CombinedOutput()
	if err == nil {
		t.Fatal("expected an error for --outdir on a non-tape source, got nil")
	}
	if !strings.Contains(string(out), "only applies to a tape source") {
		t.Errorf("error message doesn't explain why, got:\n%s", out)
	}
}

func TestZXConvert_RequiresExactlyOneOfOAndOutdir(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	tapPath := filepath.Join(dir, "x.tap")
	os.WriteFile(tapPath, sampleCodeTAP(), 0o644)

	// Neither given.
	if out, err := exec.Command(zx, "convert", tapPath).CombinedOutput(); err == nil {
		t.Errorf("expected an error with neither -o nor --outdir, got none:\n%s", out)
	}
	// Both given.
	if out, err := exec.Command(zx, "convert", tapPath, "-o", filepath.Join(dir, "y.tzx"), "--outdir", filepath.Join(dir, "z")).CombinedOutput(); err == nil {
		t.Errorf("expected an error with both -o and --outdir, got none:\n%s", out)
	}
}

func TestZXConvert_RZXToTape(t *testing.T) {
	// The one remaining genuinely-buildable gap identified from the
	// conversion matrix: RZX -> TAP/TZX/PZX. Nothing new to invent --
	// firstEmbeddedSnapshot already existed for RZX -> bin/snap, and
	// stateToTape (split out of convertSnapToTape) already existed for
	// snap -> tape. This test just confirms the wiring.
	zx := buildZX(t)
	dir := t.TempDir()

	s := &snapshot.MachineState{Model: snapshot.Model48K}
	s.CPU.PC = 0x8500
	copy(s.Memory.RAM[2][0x500:], []byte{0x3E, 0x99, 0xC9})
	z80Img, err := snapshot.EncodeZ80(s)
	if err != nil {
		t.Fatal(err)
	}
	f := &rzx.File{
		MajorVersion: 0, MinorVersion: 13,
		Blocks: []rzx.Block{
			rzx.CreatorBlock{ID: "test", Major: 1, Minor: 0},
			rzx.SnapshotBlock{Extension: "Z80", Data: z80Img},
		},
	}
	rzxImg, err := rzx.Encode(f)
	if err != nil {
		t.Fatal(err)
	}
	rzxPath := filepath.Join(dir, "session.rzx")
	if err := os.WriteFile(rzxPath, rzxImg, 0o644); err != nil {
		t.Fatal(err)
	}

	for _, ext := range []string{"tap", "tzx", "pzx"} {
		outPath := filepath.Join(dir, "out."+ext)
		if o, err := exec.Command(zx, "convert", rzxPath, "-o", outPath).CombinedOutput(); err != nil {
			t.Fatalf("zx convert rzx -> %s: %v\n%s", ext, err, o)
		}
		if fi, err := os.Stat(outPath); err != nil || fi.Size() == 0 {
			t.Fatalf("no (or empty) .%s produced", ext)
		}
	}

	// Confirm the actual embedded code survives the full chain, not
	// just that files got written: rzx -> tap -> bin, checked at the
	// exact offset the code was placed at (0x8500, inside the 48K
	// memdump that starts at 0x4000).
	tapPath := filepath.Join(dir, "out.tap")
	binPath := filepath.Join(dir, "extracted.bin")
	if o, err := exec.Command(zx, "convert", tapPath, "-o", binPath).CombinedOutput(); err != nil {
		t.Fatalf("zx convert tap -> bin: %v\n%s", err, o)
	}
	extracted := mustRead(t, binPath)
	offset := 0x8500 - 0x4000
	if offset+3 > len(extracted) {
		t.Fatalf("extracted.bin is only %d bytes, want at least %d", len(extracted), offset+3)
	}
	got := extracted[offset : offset+3]
	if string(got) != "\x3e\x99\xc9" {
		t.Errorf("code at offset %#x = % X, want 3E 99 C9", offset, got)
	}
}

func TestPZXToTAP_NonStandardTiming(t *testing.T) {
	// The core improvement: a real, well-formed PZX data block using
	// deliberately non-standard (turbo-loader-style) pulse timing must
	// still convert. An earlier version gated on exact equality to this
	// package's own encoder constants, which rejected any real PZX file
	// that didn't happen to share them -- including genuinely valid
	// turbo-loader captures. The insight: PZX's own decoder already
	// resolves a DATA block's bits into real bytes regardless of what
	// pulse durations represented them physically; the timing has no
	// bearing on what the bytes are.
	tapImg := tap.EncodeCode("TURBO", []byte{0xDE, 0xAD, 0xBE, 0xEF}, 0x9000)
	blocks, err := tap.Decode(tapImg)
	if err != nil {
		t.Fatal(err)
	}
	f := &pzx.File{Blocks: []pzx.Block{pzx.HeaderBlock{Major: 1, Minor: 0}}}
	for _, b := range blocks {
		raw := append([]byte{b.Flag}, b.Data...)
		raw = append(raw, b.Checksum)
		f.Blocks = append(f.Blocks,
			// Deliberately not pilotPulse/syncPulse1/syncPulse2/bit0Pulse/bit1Pulse.
			pzx.PulseBlock{Pulses: []pzx.Pulse{{Count: 2000, Duration: 800}, {Count: 1, Duration: 300}, {Count: 1, Duration: 350}}},
			pzx.DataBlock{InitialLevelHigh: true, BitCount: uint32(len(raw)) * 8, Tail: 400,
				S0: []uint16{300, 300}, S1: []uint16{600, 600}, Data: raw})
	}
	pzxImg, err := pzx.Encode(f)
	if err != nil {
		t.Fatal(err)
	}

	got, err := pzxToTAP(pzxImg)
	if err != nil {
		t.Fatalf("pzxToTAP with non-standard timing: %v", err)
	}
	gotBlocks, err := tap.Decode(got)
	if err != nil {
		t.Fatalf("result is not a valid TAP image: %v", err)
	}
	if len(gotBlocks) != 2 || gotBlocks[0].Name != "TURBO" {
		t.Fatalf("gotBlocks = %+v, want a TURBO header + data pair", gotBlocks)
	}
	if string(gotBlocks[1].Data) != "\xDE\xAD\xBE\xEF" {
		t.Errorf("data = % X, want DE AD BE EF", gotBlocks[1].Data)
	}
}

func TestToTAP_TZXTurboAndPureDataBlocks(t *testing.T) {
	// Real 0x11 (Turbo Speed Data) and 0x14 (Pure Data) blocks, hand-built
	// to match pkg/tzx's own documented layout, must now convert -- an
	// earlier version of toTAP kept only 0x10, silently dropping every
	// turbo-loaded block on any real TZX using one (a large fraction of
	// real commercial tapes, which is the whole reason turbo loaders
	// exist). pkg/tzx's own doc comment on Block.Data confirms 0x11 and
	// 0x14 hold "the same field 0x10 uses for its own payload" --
	// already-resolved bytes, regardless of pulse timing.
	tapImg := tap.EncodeCode("T11", []byte{0x11, 0x11, 0x11}, 0x8000)
	blocks, err := tap.Decode(tapImg)
	if err != nil {
		t.Fatal(err)
	}
	u16 := func(v uint16) []byte { b := make([]byte, 2); b[0] = byte(v); b[1] = byte(v >> 8); return b }

	var image []byte
	image = append(image, "ZXTape!"...)
	image = append(image, 0x1A, 1, 20)

	// 0x11 block: header header, MAIN block (both header & data as one raw TAP block each).
	for _, b := range blocks {
		raw := append([]byte{b.Flag}, b.Data...)
		raw = append(raw, b.Checksum)
		var blk []byte
		blk = append(blk, 0x11)
		blk = append(blk, u16(1200)...) // pilotPulse
		blk = append(blk, u16(400)...)  // syncPulse1
		blk = append(blk, u16(450)...)  // syncPulse2
		blk = append(blk, u16(500)...)  // dataPulse0
		blk = append(blk, u16(1000)...) // dataPulse1
		blk = append(blk, u16(1500)...) // pilotLength
		blk = append(blk, 8)            // bitsInLastByte
		blk = append(blk, u16(0)...)    // pause
		n := len(raw)
		blk = append(blk, byte(n), byte(n>>8), byte(n>>16))
		blk = append(blk, raw...)
		image = append(image, blk...)
	}

	// A standalone 0x14 (Pure Data) block too.
	pureData := []byte{0xAB, 0xCD}
	var blk14 []byte
	blk14 = append(blk14, 0x14)
	blk14 = append(blk14, u16(500)...)  // dataPulse0
	blk14 = append(blk14, u16(1000)...) // dataPulse1
	blk14 = append(blk14, 8)            // bitsInLastByte
	blk14 = append(blk14, u16(0)...)    // pause
	n := len(pureData)
	blk14 = append(blk14, byte(n), byte(n>>8), byte(n>>16))
	blk14 = append(blk14, pureData...)
	image = append(image, blk14...)

	got, err := toTAP(image, "tzx")
	if err != nil {
		t.Fatalf("toTAP with 0x11+0x14 blocks: %v", err)
	}
	gotBlocks, err := tap.Decode(got)
	if err != nil {
		t.Fatalf("result is not a valid TAP image: %v", err)
	}
	if len(gotBlocks) != 3 {
		t.Fatalf("got %d blocks, want 3 (T11 header, T11 data, the 0x14 pure-data block)", len(gotBlocks))
	}
	if gotBlocks[0].Name != "T11" {
		t.Errorf("gotBlocks[0].Name = %q, want T11", gotBlocks[0].Name)
	}
	if string(gotBlocks[1].Data) != "\x11\x11\x11" {
		t.Errorf("gotBlocks[1].Data = % X, want 11 11 11", gotBlocks[1].Data)
	}
}

func TestStateToTape_128KAllBanksPreserved(t *testing.T) {
	// The 128K improvement: the "current view" memdump can only ever
	// show whichever one bank happened to be paged in at 0xC000 -- the
	// other seven were previously not represented on the tape at all.
	s := &snapshot.MachineState{Model: snapshot.Model128K}
	s.CPU.PC = 0xC000
	s.Paging.Port7FFD = 0x04
	s.Paging.Port1FFD = 0x02
	copy(s.Memory.RAM[4][:], []byte{0xAA, 0xAA, 0xAA})
	copy(s.Memory.RAM[7][:], []byte{0xBB, 0xBB, 0xBB}) // NOT paged in

	tapImage, err := stateToTape(s, "tap")
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := tap.Decode(tapImage)
	if err != nil {
		t.Fatal(err)
	}
	// memdump pair + 8 bank pairs + PAGING pair = 20 blocks.
	if len(blocks) != 20 {
		t.Fatalf("got %d blocks, want 20 (memdump + 8 banks + PAGING, header+data each)", len(blocks))
	}
	foundBank7, foundPaging := false, false
	for i, b := range blocks {
		if !b.IsHeader {
			continue
		}
		switch b.Name {
		case "BANK7":
			data := blocks[i+1].Data
			if len(data) < 3 || string(data[:3]) != "\xBB\xBB\xBB" {
				t.Errorf("BANK7 data = % X, want BB BB BB...", data[:3])
			}
			foundBank7 = true
		case "PAGING":
			data := blocks[i+1].Data
			if len(data) != 2 || data[0] != 0x04 || data[1] != 0x02 {
				t.Errorf("PAGING data = % X, want 04 02 (Port7FFD, Port1FFD)", data)
			}
			foundPaging = true
		}
	}
	if !foundBank7 {
		t.Fatal("no BANK7 block found -- bank 7's content (never paged in) would have been lost entirely before this fix")
	}
	if !foundPaging {
		t.Fatal("no PAGING block found -- which bank was actually active has nowhere else to survive on a tape")
	}
}

func TestZXConvert_Port1FFDWarning(t *testing.T) {
	// Can't fix the loss itself -- SNA genuinely has no field for it --
	// but a real, non-zero value should say so on the way out rather
	// than vanish silently. Checked via the real binary's stderr, since
	// that's what a person actually sees.
	zx := buildZX(t)
	dir := t.TempDir()

	s := &snapshot.MachineState{Model: snapshot.ModelPlus3}
	s.Paging.Port7FFD = 0x03
	s.Paging.Port1FFD = 0x04
	img, err := szx.Encode(s)
	if err != nil {
		t.Fatal(err)
	}
	szxPath := filepath.Join(dir, "test.szx")
	if err := os.WriteFile(szxPath, img, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(zx, "convert", szxPath, "-o", filepath.Join(dir, "out.sna")).CombinedOutput()
	if err != nil {
		t.Fatalf("zx convert szx -> sna: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "port 1FFD") {
		t.Errorf("expected a warning about port 1FFD, got:\n%s", out)
	}

	// Confirm it's genuinely gone on the other side, not just warned about.
	snaData := mustRead(t, filepath.Join(dir, "out.sna"))
	back, err := snapshot.DecodeSNA128(snaData)
	if err != nil {
		t.Fatal(err)
	}
	if back.Paging.Port1FFD != 0 {
		t.Errorf("Port1FFD = %#02x after round-trip through SNA, want 0 (SNA has no field for it)", back.Paging.Port1FFD)
	}
	if back.Paging.Port7FFD != 0x03 {
		t.Errorf("Port7FFD = %#02x, want 0x03 (should be unaffected)", back.Paging.Port7FFD)
	}
}

func TestZXConvert_NoPort1FFDWarningWhenZero(t *testing.T) {
	// The warning should be silent for the ordinary case -- most 128K
	// snapshots never touch the +3's special port at all.
	zx := buildZX(t)
	dir := t.TempDir()

	s := &snapshot.MachineState{Model: snapshot.Model128K}
	s.Paging.Port7FFD = 0x03
	// Port1FFD left at its zero value.
	img, err := szx.Encode(s)
	if err != nil {
		t.Fatal(err)
	}
	szxPath := filepath.Join(dir, "test.szx")
	os.WriteFile(szxPath, img, 0o644)

	out, err := exec.Command(zx, "convert", szxPath, "-o", filepath.Join(dir, "out.sna")).CombinedOutput()
	if err != nil {
		t.Fatalf("zx convert szx -> sna: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "port 1FFD") {
		t.Errorf("unexpected port 1FFD warning when it was already zero, got:\n%s", out)
	}
}

func TestZXEdit_ListFlagsAfterPositional(t *testing.T) {
	// Real bug, caught by running this exact invocation: editList used
	// fs.Parse(args) directly instead of permuteArgs, so Go's flag
	// package stopped parsing at the first positional argument and
	// never saw --json when it came after the file path -- the ordinary
	// way anyone would actually type this command.
	zx := buildZX(t)
	dir := t.TempDir()
	tapPath := filepath.Join(dir, "x.tap")
	os.WriteFile(tapPath, sampleCodeTAP(), 0o644)

	out, err := exec.Command(zx, "edit", "list", tapPath, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("zx edit list <file> --json (flags after positional): %v\n%s", err, out)
	}
	var m tapeManifest
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}

func TestZXEdit_ListJSONMatchesExplodeManifestShape(t *testing.T) {
	// Real bug, caught by inspecting actual output: manifestFor used
	// baseName (meant for 10-char TAP block names, truncating and
	// stripping the extension) instead of filepath.Base, which is what
	// explodeTape's own manifest.json actually uses -- "source": "multi"
	// instead of "source": "multi.tap", breaking the "same shape" claim
	// this package's own doc comment makes.
	zx := buildZX(t)
	dir := t.TempDir()
	tapPath := filepath.Join(dir, "multi.tap")
	os.WriteFile(tapPath, sampleCodeTAP(), 0o644)

	out, err := exec.Command(zx, "edit", "list", tapPath, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("zx edit list --json: %v\n%s", err, out)
	}
	var m tapeManifest
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m.Source != "multi.tap" {
		t.Errorf("Source = %q, want %q (full filename, matching explodeTape's own convention)", m.Source, "multi.tap")
	}
}

func TestZXEdit_DeleteHeaderDataPair(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	loader := filepath.Join(dir, "l.bin")
	main := filepath.Join(dir, "m.bin")
	os.WriteFile(loader, []byte("loader"), 0o644)
	os.WriteFile(main, []byte("main game code"), 0o644)
	lTap := filepath.Join(dir, "l.tap")
	mTap := filepath.Join(dir, "m.tap")
	exec.Command(zx, "tap", "make", loader, "--name", "LOADER", "--origin", "0x8000", "-o", lTap).Run()
	exec.Command(zx, "tap", "make", main, "--name", "MAIN", "--origin", "0x9000", "-o", mTap).Run()
	multiPath := filepath.Join(dir, "multi.tap")
	lData := mustRead(t, lTap)
	mData := mustRead(t, mTap)
	os.WriteFile(multiPath, append(append([]byte{}, lData...), mData...), 0o644)

	outPath := filepath.Join(dir, "deleted.tap")
	out, err := exec.Command(zx, "edit", "delete", multiPath, "--block", "2,3", "-o", outPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx edit delete: %v\n%s", err, out)
	}
	blocks, err := tap.Decode(mustRead(t, outPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2 (MAIN's header+data pair should be entirely gone)", len(blocks))
	}
	if blocks[0].Name != "LOADER" {
		t.Errorf("remaining header = %q, want LOADER", blocks[0].Name)
	}
}

func TestZXEdit_ImportAtPosition(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	tapPath := filepath.Join(dir, "orig.tap")
	os.WriteFile(tapPath, sampleCodeTAP(), 0o644)
	newData := filepath.Join(dir, "new.bin")
	os.WriteFile(newData, []byte{0xAA, 0xBB}, 0o644)

	outPath := filepath.Join(dir, "imported.tap")
	out, err := exec.Command(zx, "edit", "import", tapPath, "--data", newData, "--name", "FIRST", "--org", "0x7000", "--at", "0", "-o", outPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx edit import --at 0: %v\n%s", err, out)
	}
	blocks, err := tap.Decode(mustRead(t, outPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 4 || blocks[0].Name != "FIRST" || blocks[0].Param1 != 0x7000 {
		t.Fatalf("blocks[0] = %+v, want a FIRST header at 0x7000, inserted before the original pair", blocks[0])
	}
}

func TestZXEdit_ImportRawDataRoundTrip(t *testing.T) {
	// Extract a raw data block (flag+payload+checksum) and re-import it
	// elsewhere -- confirming the bytes survive exactly, including the
	// checksum, not just that a file got written.
	zx := buildZX(t)
	dir := t.TempDir()
	tapPath := filepath.Join(dir, "orig.tap")
	code := []byte("main game code")
	os.WriteFile(tapPath, tap.EncodeCode("MAIN", code, 0x9000), 0o644)

	rawPath := filepath.Join(dir, "raw.bin")
	if out, err := exec.Command(zx, "edit", "extract", tapPath, "--block", "1", "--raw", "-o", rawPath).CombinedOutput(); err != nil {
		t.Fatalf("zx edit extract --raw: %v\n%s", err, out)
	}

	emptyTap := filepath.Join(dir, "empty_base.tap")
	os.WriteFile(emptyTap, tap.EncodeCode("X", []byte{0}, 0x8000), 0o644)

	resultPath := filepath.Join(dir, "result.tap")
	out, err := exec.Command(zx, "edit", "import", emptyTap, "--data", rawPath, "--raw", "-o", resultPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx edit import --raw: %v\n%s", err, out)
	}
	blocks, err := tap.Decode(mustRead(t, resultPath))
	if err != nil {
		t.Fatal(err)
	}
	last := blocks[len(blocks)-1]
	if last.IsHeader {
		t.Fatalf("last block is a header, want the re-imported bare data block")
	}
	if string(last.Data) != string(code) {
		t.Errorf("re-imported data = %q, want %q", last.Data, code)
	}
	if !last.ChecksumOK {
		t.Error("re-imported block's checksum doesn't validate")
	}
}

func TestZXEdit_DeleteToAllThreeFormats(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	tapPath := filepath.Join(dir, "orig.tap")
	os.WriteFile(tapPath, sampleCodeTAP(), 0o644)

	for _, ext := range []string{"tap", "tzx", "pzx"} {
		outPath := filepath.Join(dir, "out."+ext)
		out, err := exec.Command(zx, "edit", "delete", tapPath, "--block", "1", "-o", outPath).CombinedOutput()
		if err != nil {
			t.Fatalf("zx edit delete -> %s: %v\n%s", ext, err, out)
		}
		if fi, err := os.Stat(outPath); err != nil || fi.Size() == 0 {
			t.Fatalf("no (or empty) .%s produced", ext)
		}
	}
}

func TestZXEdit_DeleteAllBlocksRejected(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	tapPath := filepath.Join(dir, "x.tap")
	os.WriteFile(tapPath, sampleCodeTAP(), 0o644)

	out, err := exec.Command(zx, "edit", "delete", tapPath, "--block", "0,1", "-o", filepath.Join(dir, "empty.tap")).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error deleting every block, got nil\n%s", out)
	}
}

func TestZXBuild_AllThreeKinds(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "loader.bin"), []byte("loader code"), 0o644)
	os.WriteFile(filepath.Join(dir, "basic.bin"), []byte("10 PRINT"), 0o644)

	rawPayload := []byte("raw content")
	flag := byte(0xFF)
	chk := flag
	for _, b := range rawPayload {
		chk ^= b
	}
	raw := append([]byte{flag}, rawPayload...)
	raw = append(raw, chk)
	os.WriteFile(filepath.Join(dir, "raw.bin"), raw, 0o644)

	specPath := filepath.Join(dir, "spec.json")
	os.WriteFile(specPath, []byte(`{
		"title": "Test",
		"blocks": [
			{"kind": "code", "name": "LOADER", "org": 32768, "file": "loader.bin"},
			{"kind": "program", "name": "BASIC", "autostart": 10, "file": "basic.bin"},
			{"kind": "raw", "file": "raw.bin"}
		]
	}`), 0o644)

	outPath := filepath.Join(dir, "out.tap")
	out, err := exec.Command(zx, "build", specPath, "-o", outPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx build: %v\n%s", err, out)
	}
	blocks, err := tap.Decode(mustRead(t, outPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 5 {
		t.Fatalf("got %d blocks, want 5 (code header+data, program header+data, one raw data block)", len(blocks))
	}
	if blocks[0].Name != "LOADER" || blocks[0].Param1 != 0x8000 {
		t.Errorf("blocks[0] = %+v, want LOADER at 0x8000", blocks[0])
	}
	if string(blocks[1].Data) != "loader code" {
		t.Errorf("blocks[1].Data = %q, want %q", blocks[1].Data, "loader code")
	}
	if blocks[2].Name != "BASIC" || blocks[2].Param1 != 10 {
		t.Errorf("blocks[2] = %+v, want BASIC with autostart 10", blocks[2])
	}
	if string(blocks[4].Data) != string(rawPayload) {
		t.Errorf("blocks[4].Data = %q, want %q", blocks[4].Data, rawPayload)
	}
}

func TestZXBuild_FilePathsRelativeToSpecDir(t *testing.T) {
	// A real design decision worth locking in with a test: "file" in the
	// spec resolves relative to the spec file's own directory, not the
	// current working directory -- so a spec and its data files can be
	// moved together as a unit and still work regardless of where zx
	// build is invoked from.
	zx := buildZX(t)
	specDir := t.TempDir()
	cwd := t.TempDir() // deliberately different from specDir

	os.WriteFile(filepath.Join(specDir, "payload.bin"), []byte("relative-to-spec"), 0o644)
	specPath := filepath.Join(specDir, "spec.json")
	os.WriteFile(specPath, []byte(`{"blocks":[{"kind":"code","name":"X","org":32768,"file":"payload.bin"}]}`), 0o644)

	outPath := filepath.Join(cwd, "out.tap")
	cmd := exec.Command(zx, "build", specPath, "-o", outPath)
	cmd.Dir = cwd // run from a directory with no payload.bin of its own
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zx build (run from a different directory than the spec): %v\n%s", err, out)
	}
	blocks, err := tap.Decode(mustRead(t, outPath))
	if err != nil {
		t.Fatal(err)
	}
	if string(blocks[1].Data) != "relative-to-spec" {
		t.Errorf("blocks[1].Data = %q, want %q", blocks[1].Data, "relative-to-spec")
	}
}

func TestZXBuild_PZXTitleCarried(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.bin"), []byte("x"), 0o644)
	specPath := filepath.Join(dir, "spec.json")
	os.WriteFile(specPath, []byte(`{"title":"My Archive","blocks":[{"kind":"code","name":"X","org":32768,"file":"x.bin"}]}`), 0o644)

	outPath := filepath.Join(dir, "out.pzx")
	out, err := exec.Command(zx, "build", specPath, "-o", outPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx build -> pzx: %v\n%s", err, out)
	}
	f, err := pzx.Decode(mustRead(t, outPath))
	if err != nil {
		t.Fatal(err)
	}
	h, ok := f.Blocks[0].(pzx.HeaderBlock)
	if !ok || h.Title != "My Archive" {
		t.Errorf("Blocks[0] = %+v, want HeaderBlock with Title=My Archive", f.Blocks[0])
	}
}

func TestZXBuild_ErrorPaths(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.bin"), []byte("x"), 0o644)

	cases := map[string]string{
		"missing name": `{"blocks":[{"kind":"code","org":32768,"file":"x.bin"}]}`,
		"missing org":  `{"blocks":[{"kind":"code","name":"X","file":"x.bin"}]}`,
		"unknown kind": `{"blocks":[{"kind":"bogus","file":"x.bin"}]}`,
		"empty blocks": `{"blocks":[]}`,
		"missing file": `{"blocks":[{"kind":"raw","file":"doesnotexist.bin"}]}`,
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			specPath := filepath.Join(dir, name+".json")
			os.WriteFile(specPath, []byte(spec), 0o644)
			out, err := exec.Command(zx, "build", specPath, "-o", filepath.Join(dir, name+".tap")).CombinedOutput()
			if err == nil {
				t.Errorf("expected an error, got nil\n%s", out)
			}
		})
	}
}

func TestZXEdit_AppendMixedFormats(t *testing.T) {
	// The actual point of a dedicated append command, not just an alias
	// for shell concatenation: combining sources of DIFFERENT tape
	// formats correctly, which `cat a.tzx b.pzx > c.tzx` could never do
	// (both have their own container structure a raw concatenation
	// would corrupt; only bare TAP happens to survive it).
	zx := buildZX(t)
	dir := t.TempDir()

	loaderBin := filepath.Join(dir, "l.bin")
	mainBin := filepath.Join(dir, "m.bin")
	os.WriteFile(loaderBin, []byte("loader code"), 0o644)
	os.WriteFile(mainBin, []byte("main game code"), 0o644)

	loaderTap := filepath.Join(dir, "loader.tap")
	mainTap := filepath.Join(dir, "main.tap")
	exec.Command(zx, "tap", "make", loaderBin, "--name", "LOADER", "--origin", "0x8000", "-o", loaderTap).Run()
	exec.Command(zx, "tap", "make", mainBin, "--name", "MAIN", "--origin", "0x9000", "-o", mainTap).Run()

	mainTzx := filepath.Join(dir, "main.tzx")
	if out, err := exec.Command(zx, "convert", mainTap, "-o", mainTzx).CombinedOutput(); err != nil {
		t.Fatalf("zx convert (setup): %v\n%s", err, out)
	}

	outPath := filepath.Join(dir, "combined.tap")
	out, err := exec.Command(zx, "edit", "append", loaderTap, mainTzx, "-o", outPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx edit append (tap + tzx): %v\n%s", err, out)
	}
	blocks, err := tap.Decode(mustRead(t, outPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 4 {
		t.Fatalf("got %d blocks, want 4 (LOADER pair + MAIN pair)", len(blocks))
	}
	if blocks[0].Name != "LOADER" || blocks[2].Name != "MAIN" {
		t.Errorf("blocks[0].Name=%q blocks[2].Name=%q, want LOADER then MAIN, in that order", blocks[0].Name, blocks[2].Name)
	}
	if string(blocks[3].Data) != "main game code" {
		t.Errorf("blocks[3].Data = %q, want %q", blocks[3].Data, "main game code")
	}
}

func TestZXEdit_AppendRequiresAtLeastTwoFiles(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	tapPath := filepath.Join(dir, "x.tap")
	os.WriteFile(tapPath, sampleCodeTAP(), 0o644)

	out, err := exec.Command(zx, "edit", "append", tapPath, "-o", filepath.Join(dir, "out.tap")).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error for only one input file, got nil\n%s", out)
	}
}

func TestZXEdit_AppendRejectsNonTapeFile(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	tapPath := filepath.Join(dir, "x.tap")
	os.WriteFile(tapPath, sampleCodeTAP(), 0o644)
	snaPath := filepath.Join(dir, "y.sna")
	os.WriteFile(snaPath, []byte("not a real sna, just wrong extension test"), 0o644)

	out, err := exec.Command(zx, "edit", "append", tapPath, snaPath, "-o", filepath.Join(dir, "out.tap")).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error for a non-tape input, got nil\n%s", out)
	}
}

func TestZXTap_AppendMatchesShellConcatenation(t *testing.T) {
	// TAP has no container structure of its own -- a bare sequence of
	// length-prefixed blocks -- so tapAppend deliberately does raw byte
	// concatenation rather than decoding through []tap.Block and
	// re-encoding, the same as zx edit append does for the general,
	// multi-format case. Confirmed here to be byte-for-byte identical
	// to shell `cat`, which is exactly the property that makes the
	// simpler implementation correct for this specific, common case.
	zx := buildZX(t)
	dir := t.TempDir()
	l := filepath.Join(dir, "l.bin")
	m := filepath.Join(dir, "m.bin")
	os.WriteFile(l, []byte("loader"), 0o644)
	os.WriteFile(m, []byte("main"), 0o644)
	lTap := filepath.Join(dir, "l.tap")
	mTap := filepath.Join(dir, "m.tap")
	exec.Command(zx, "tap", "make", l, "--name", "LOADER", "--origin", "0x8000", "-o", lTap).Run()
	exec.Command(zx, "tap", "make", m, "--name", "MAIN", "--origin", "0x9000", "-o", mTap).Run()

	outPath := filepath.Join(dir, "combined.tap")
	out, err := exec.Command(zx, "tap", "append", lTap, mTap, "-o", outPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx tap append: %v\n%s", err, out)
	}

	expected := append(append([]byte{}, mustRead(t, lTap)...), mustRead(t, mTap)...)
	got := mustRead(t, outPath)
	if string(got) != string(expected) {
		t.Errorf("zx tap append output != shell-cat-equivalent concatenation")
	}
	blocks, err := tap.Decode(got)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 4 {
		t.Fatalf("got %d blocks, want 4", len(blocks))
	}
}

func TestZXTap_AppendRejectsNonTAP(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	tapPath := filepath.Join(dir, "x.tap")
	os.WriteFile(tapPath, sampleCodeTAP(), 0o644)

	tzxPath := filepath.Join(dir, "y.tzx")
	if out, err := exec.Command(zx, "convert", tapPath, "-o", tzxPath).CombinedOutput(); err != nil {
		t.Fatalf("zx convert (setup): %v\n%s", err, out)
	}

	out, err := exec.Command(zx, "tap", "append", tapPath, tzxPath, "-o", filepath.Join(dir, "out.tap")).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error appending a TZX file via zx tap append, got nil\n%s", out)
	}
}

func TestZXTap_AppendRequiresAtLeastTwoFiles(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	tapPath := filepath.Join(dir, "x.tap")
	os.WriteFile(tapPath, sampleCodeTAP(), 0o644)

	out, err := exec.Command(zx, "tap", "append", tapPath, "-o", filepath.Join(dir, "out.tap")).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error for only one input file, got nil\n%s", out)
	}
}

func TestZXTapMake_Basic(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	basPath := filepath.Join(dir, "loader.bas")
	os.WriteFile(basPath, []byte("10 print \"hi\"\n"), 0o644)

	// Default name falls back to the input's own filename, matching
	// binary mode's existing behaviour.
	tapPath := filepath.Join(dir, "a.tap")
	if o, err := exec.Command(zx, "tap", "make", "--basic", basPath, "-o", tapPath).CombinedOutput(); err != nil {
		t.Fatalf("zx tap make --basic: %v\n%s", err, o)
	}
	out, err := exec.Command(zx, "edit", "list", tapPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx edit list: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `type=Program name="loader"`) {
		t.Errorf("expected a Program block named \"loader\", got:\n%s", out)
	}

	// An explicitly empty --name must stay empty, not fall back to the
	// filename -- this is the fix: fs.Visit distinguishes "not passed"
	// from "passed as empty", where a bare *name == "" check cannot.
	tapPath2 := filepath.Join(dir, "b.tap")
	if o, err := exec.Command(zx, "tap", "make", "--basic", "--name", "", basPath, "-o", tapPath2).CombinedOutput(); err != nil {
		t.Fatalf("zx tap make --basic --name \"\": %v\n%s", err, o)
	}
	out2, err := exec.Command(zx, "edit", "list", tapPath2).CombinedOutput()
	if err != nil {
		t.Fatalf("zx edit list: %v\n%s", err, out2)
	}
	if !strings.Contains(string(out2), `name=""`) {
		t.Errorf("expected an empty block name to be preserved, got:\n%s", out2)
	}

	// --autostart sets the Program header's autostart line, and default
	// case-insensitive matching (mirroring zx basic tokenise, not
	// totap's opposite-polarity default) tokenises lowercase "print".
	tapPath3 := filepath.Join(dir, "c.tap")
	if o, err := exec.Command(zx, "tap", "make", "--basic", "--autostart", "10", basPath, "-o", tapPath3).CombinedOutput(); err != nil {
		t.Fatalf("zx tap make --basic --autostart: %v\n%s", err, o)
	}
	out3, err := exec.Command(zx, "edit", "list", tapPath3).CombinedOutput()
	if err != nil {
		t.Fatalf("zx edit list: %v\n%s", err, out3)
	}
	if !strings.Contains(string(out3), "autostart=0x000A") {
		t.Errorf("expected autostart=0x000A, got:\n%s", out3)
	}
	payloadPath := filepath.Join(dir, "payload.bin")
	if o, err := exec.Command(zx, "edit", "extract", tapPath3, "--block", "1", "-o", payloadPath).CombinedOutput(); err != nil {
		t.Fatalf("zx edit extract: %v\n%s", err, o)
	}
	detok, err := exec.Command(zx, "basic", "detokenise", payloadPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx basic detokenise: %v\n%s", err, detok)
	}
	if !strings.Contains(string(detok), "PRINT") {
		t.Errorf("expected lowercase 'print' to tokenise to PRINT, got:\n%s", detok)
	}

	// --basic and --loader are mutually exclusive.
	tapPath4 := filepath.Join(dir, "d.tap")
	out4, err := exec.Command(zx, "tap", "make", "--basic", "--loader", "--start", "0x8000", basPath, "-o", tapPath4).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error combining --basic and --loader, got nil\n%s", out4)
	}
}
