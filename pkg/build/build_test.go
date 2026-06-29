package build

import (
	"testing"

	"github.com/ha1tch/zentools/pkg/snapshot"
	"github.com/ha1tch/zentools/pkg/tap"
)

// a tiny program: DI / HALT (0xF3 0x76) loaded at 0x8000.
var sampleCode = []byte{0xF3, 0x76}

func TestEncodeTAPRoundTrips(t *testing.T) {
	r := Request{Name: "TEST", Code: sampleCode, Origin: 0x8000}
	img := EncodeTAP(r)
	blocks, err := tap.Decode(img)
	if err != nil {
		t.Fatalf("decode tap: %v", err)
	}
	// A CODE tape is a header block plus a data block.
	if len(blocks) < 2 {
		t.Fatalf("expected at least 2 tap blocks (header+data), got %d", len(blocks))
	}
}

func TestEncodeZ80OverlaysCode(t *testing.T) {
	for _, m := range []Model{Model48K, Model128K, ModelPlus2, ModelPlus2A, ModelPlus3} {
		t.Run(string(m), func(t *testing.T) {
			r := Request{Name: "T", Code: sampleCode, Origin: 0x8000, Start: 0x8000, SP: DefaultSP, Model: m}
			img, err := EncodeZ80(r)
			if err != nil {
				t.Fatalf("EncodeZ80: %v", err)
			}
			s, err := snapshot.DecodeZ80(img)
			if err != nil {
				t.Fatalf("decode result: %v", err)
			}
			// PC and SP set as requested.
			if s.CPU.PC != 0x8000 {
				t.Errorf("PC = 0x%04X, want 0x8000", s.CPU.PC)
			}
			if s.CPU.SP != DefaultSP {
				t.Errorf("SP = 0x%04X, want 0x%04X", s.CPU.SP, DefaultSP)
			}
			// Code landed at 0x8000 = bank 2 offset 0.
			if s.Memory.RAM[2][0] != 0xF3 || s.Memory.RAM[2][1] != 0x76 {
				t.Errorf("code not at 0x8000: got 0x%02X 0x%02X", s.Memory.RAM[2][0], s.Memory.RAM[2][1])
			}
		})
	}
}

func TestWriteToROMRejected(t *testing.T) {
	r := Request{Name: "T", Code: sampleCode, Origin: 0x0000, Start: 0x0000, SP: DefaultSP, Model: Model48K}
	if _, err := EncodeZ80(r); err == nil {
		t.Fatal("expected error writing into ROM, got nil")
	}
}

func TestSPCollisionWarning(t *testing.T) {
	// SP inside the code body.
	r := Request{Code: make([]byte, 0x100), Origin: 0x8000, SP: 0x8080}
	if r.SPWarning() == "" {
		t.Error("expected warning for SP inside code")
	}
	// SP safely high.
	r2 := Request{Code: sampleCode, Origin: 0x8000, SP: 0xFF00}
	if w := r2.SPWarning(); w != "" {
		t.Errorf("unexpected warning for safe layout: %s", w)
	}
}


func TestEncodeTAPWithLoader(t *testing.T) {
	r := Request{Name: "screenfill", Code: sampleCode, Origin: 0x8000, Start: 0x8000}
	img, err := EncodeTAPWithLoader(r)
	if err != nil {
		t.Fatalf("EncodeTAPWithLoader: %v", err)
	}
	blocks, err := tap.Decode(img)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// loader header+data, then code header+data = 4 blocks.
	if len(blocks) != 4 {
		t.Fatalf("expected 4 blocks (loader+code), got %d", len(blocks))
	}
	// First block is a Program header with an autostart line; third is a Code header.
	if !blocks[0].IsHeader || blocks[0].Type != 0x00 {
		t.Errorf("block 0 is not a Program header (IsHeader=%v Type=%d)", blocks[0].IsHeader, blocks[0].Type)
	}
	if blocks[0].Param1 != loaderAutostartLine {
		t.Errorf("loader autostart line = %d, want %d", blocks[0].Param1, loaderAutostartLine)
	}
	if !blocks[2].IsHeader || blocks[2].Type != 0x03 {
		t.Errorf("block 2 is not a Code header (IsHeader=%v Type=%d)", blocks[2].IsHeader, blocks[2].Type)
	}
	if blocks[2].Param1 != 0x8000 {
		t.Errorf("code load address = 0x%04X, want 0x8000", blocks[2].Param1)
	}
}

func TestNameNamespaces(t *testing.T) {
	cases := []struct {
		host, wantTape, wantPlus3 string
	}{
		{"my game.asm", "my game", "MYGAME"},
		{"a-very-long-filename.asm", "a-very-lon", "AVERYLON"},
		{"démo.asm", "d mo", "DMO"}, // é is non-ASCII: space in tape name, dropped in +3 name
	}
	for _, c := range cases {
		base := hostBase(c.host)
		if got := tapeName(base); got != c.wantTape {
			t.Errorf("tapeName(%q) = %q, want %q", base, got, c.wantTape)
		}
		if got := plus3Name(base, "BIN"); got != c.wantPlus3+".BIN" {
			t.Errorf("plus3Name(%q) = %q, want %q", base, got, c.wantPlus3+".BIN")
		}
	}
}
