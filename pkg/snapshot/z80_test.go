package snapshot

import (
	"bytes"
	"os"
	"testing"
)

func TestZ80CompressRoundTrip(t *testing.T) {
	// Tricky inputs: long runs, ED bytes, ED pairs, mixed.
	cases := [][]byte{
		bytes.Repeat([]byte{0x00}, 1000),
		bytes.Repeat([]byte{0xED}, 10),
		{0xED, 0xED, 0x01, 0x02, 0xED},
		append(bytes.Repeat([]byte{0xAA}, 7), 0xED, 0xED, 0xED, 0xED),
		{1, 2, 3, 4, 5}, // no runs
	}
	for i, in := range cases {
		comp := compressZ80(in)
		back := decompressZ80(comp)
		if !bytes.Equal(back, in) {
			t.Errorf("case %d round trip failed\n in:  % X\n out: % X", i, in, back)
		}
	}
}

func TestZ80RoundTrip(t *testing.T) {
	s := &MachineState{Model: Model48K}
	s.CPU = CPU{AF: 0x1234, BC: 0x5678, DE: 0x9ABC, HL: 0xDEF0,
		AF_: 0x1111, BC_: 0x2222, DE_: 0x3333, HL_: 0x4444,
		IX: 0xAAAA, IY: 0xBBBB, SP: 0xFF00, PC: 0x8000,
		I: 0x3F, R: 0xFE, IFF1: true, IFF2: true, IM: 1}
	s.IO.Border = 5
	for b := range s.Memory.RAM {
		for i := range s.Memory.RAM[b] {
			s.Memory.RAM[b][i] = byte((b*7 + i) & 0xFF)
		}
	}
	img, err := EncodeZ80(s)
	if err != nil { t.Fatalf("EncodeZ80: %v", err) }
	out, err := DecodeZ80(img)
	if err != nil { t.Fatalf("DecodeZ80: %v", err) }

	if out.CPU.AF != s.CPU.AF || out.CPU.PC != s.CPU.PC || out.CPU.SP != s.CPU.SP {
		t.Errorf("regs: AF=0x%04X PC=0x%04X SP=0x%04X", out.CPU.AF, out.CPU.PC, out.CPU.SP)
	}
	if out.CPU.R != s.CPU.R {
		t.Errorf("R = 0x%02X, want 0x%02X (bit 7 via flags1)", out.CPU.R, s.CPU.R)
	}
	if out.IO.Border != s.IO.Border || out.CPU.IM != s.CPU.IM {
		t.Errorf("border=%d IM=%d", out.IO.Border, out.CPU.IM)
	}
	for b := range []int{5, 2, 0} {
		_ = b
	}
	if s.Memory.RAM[5] != out.Memory.RAM[5] || s.Memory.RAM[2] != out.Memory.RAM[2] || s.Memory.RAM[0] != out.Memory.RAM[0] {
		t.Error("RAM banks not preserved through compress/decompress")
	}
}

func TestDecodeRealZenzxZ80(t *testing.T) {
	img, err := os.ReadFile("/tmp/zenzx_real.z80")
	if err != nil {
		t.Skipf("no zenzx .z80: %v", err)
	}
	s, err := DecodeZ80(img)
	if err != nil {
		t.Fatalf("DecodeZ80: %v", err)
	}
	// zenzx set PC=0x8000, SP=0xFF00, border=3, and 0xAB at 0x8000 / 0x11 at 0x4000.
	if s.CPU.PC != 0x8000 || s.CPU.SP != 0xFF00 || s.IO.Border != 3 {
		t.Errorf("PC=0x%04X SP=0x%04X border=%d", s.CPU.PC, s.CPU.SP, s.IO.Border)
	}
	if s.Memory.RAM[2][0] != 0xAB { // 0x8000 -> bank 2 offset 0
		t.Errorf("0x8000 = 0x%02X, want 0xAB", s.Memory.RAM[2][0])
	}
	if s.Memory.RAM[5][0] != 0x11 { // 0x4000 -> bank 5 offset 0
		t.Errorf("0x4000 = 0x%02X, want 0x11", s.Memory.RAM[5][0])
	}
}

func TestDecodeRealV2_48K(t *testing.T) {
	img, err := os.ReadFile("testdata/manicminer_v2_48k.z80")
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	s, err := DecodeZ80(img)
	if err != nil {
		t.Fatalf("DecodeZ80 v2: %v", err)
	}
	if s.Model != Model48K {
		t.Errorf("model = %v, want 48K", s.Model)
	}
	if s.CPU.PC != 0x0038 || s.CPU.I != 0x3F || s.CPU.IM != 1 {
		t.Errorf("PC=0x%04X I=0x%02X IM=%d, want 0x0038/0x3F/1", s.CPU.PC, s.CPU.I, s.CPU.IM)
	}
	// Real game: the display file (bank 5) must be populated.
	nz := 0
	for i := 0; i < 6144; i++ {
		if s.Memory.RAM[5][i] != 0 {
			nz++
		}
	}
	if nz < 50 {
		t.Errorf("display file looks empty (%d non-zero bytes); decompression likely failed", nz)
	}
}

func TestDecodeRealV3_128K(t *testing.T) {
	img, err := os.ReadFile("testdata/z80attack_v3_128k.z80")
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	s, err := DecodeZ80(img)
	if err != nil {
		t.Fatalf("DecodeZ80 v3: %v", err)
	}
	if !s.Model.Is128KFamily() {
		t.Errorf("model = %v, want 128K family", s.Model)
	}
	if s.CPU.PC != 0xBE2F {
		t.Errorf("PC = 0x%04X, want 0xBE2F", s.CPU.PC)
	}
	if s.Paging.Port7FFD != 0x10 {
		t.Errorf("7FFD = 0x%02X, want 0x10", s.Paging.Port7FFD)
	}
}

// TestV3EncodeDecodeCross decodes a real v3 file, re-encodes it with our v3
// encoder, decodes that, and checks the machine state is identical. This proves
// our encoder writes what our decoder reads, anchored to a real file's content.
func TestV3EncodeDecodeCross(t *testing.T) {
	img, err := os.ReadFile("testdata/z80attack_v3_128k.z80")
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	s1, err := DecodeZ80(img)
	if err != nil {
		t.Fatalf("decode original: %v", err)
	}
	reenc, err := EncodeZ80v3(s1)
	if err != nil {
		t.Fatalf("EncodeZ80v3: %v", err)
	}
	s2, err := DecodeZ80(reenc)
	if err != nil {
		t.Fatalf("decode re-encoded: %v", err)
	}
	if s1.CPU != s2.CPU {
		t.Errorf("CPU state changed across encode/decode")
	}
	if s1.Paging != s2.Paging {
		t.Errorf("paging changed: %+v vs %+v", s1.Paging, s2.Paging)
	}
	for b := 0; b < 8; b++ {
		if s1.Memory.RAM[b] != s2.Memory.RAM[b] {
			t.Errorf("RAM bank %d changed across encode/decode", b)
		}
	}
}

// TestDecodeRealV1_48K decodes a real v1 .z80 (Jet Set Willy, Software Projects,
// 1984) and checks for spec-canonical register values and a populated game
// screen, confirming the v1 RLE decompression reconstructs real content.
func TestDecodeRealV1_48K(t *testing.T) {
	img, err := os.ReadFile("testdata/jetsetwilly_v1_48k.z80")
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	s, err := DecodeZ80(img)
	if err != nil {
		t.Fatalf("DecodeZ80 v1: %v", err)
	}
	if s.Model != Model48K {
		t.Errorf("model = %v, want 48K", s.Model)
	}
	if s.CPU.PC != 0x96AC || s.CPU.I != 0x3F || s.CPU.IM != 1 {
		t.Errorf("PC=0x%04X I=0x%02X IM=%d, want 0x96AC/0x3F/1", s.CPU.PC, s.CPU.I, s.CPU.IM)
	}
	// A real loaded game: most of 48K RAM is populated.
	nz := 0
	for _, bank := range []int{5, 2, 0} {
		for _, v := range s.Memory.RAM[bank] {
			if v != 0 {
				nz++
			}
		}
	}
	if nz < 10000 {
		t.Errorf("48K RAM only %d non-zero bytes; decompression likely wrong", nz)
	}
}
