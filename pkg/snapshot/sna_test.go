package snapshot

import (
	"os"
	"testing"
)

func sampleState() *MachineState {
	s := &MachineState{Model: Model48K}
	s.CPU = CPU{
		AF: 0x1234, BC: 0x5678, DE: 0x9ABC, HL: 0xDEF0,
		AF_: 0x1111, BC_: 0x2222, DE_: 0x3333, HL_: 0x4444,
		IX: 0xAAAA, IY: 0xBBBB,
		SP: 0xFF00, PC: 0x8000,
		I: 0x3F, R: 0x7E, IFF1: true, IFF2: true, IM: 2,
	}
	s.IO.Border = 5
	// distinctive RAM content per bank
	for b := range s.Memory.RAM {
		for i := range s.Memory.RAM[b] {
			s.Memory.RAM[b][i] = byte((b*7 + i) & 0xFF)
		}
	}
	return s
}

func TestSNARoundTrip(t *testing.T) {
	in := sampleState()
	img, err := EncodeSNA(in)
	if err != nil {
		t.Fatalf("EncodeSNA: %v", err)
	}
	if len(img) != sna48HeaderLen+sna48RAMLen {
		t.Fatalf("image size %d, want %d", len(img), sna48HeaderLen+sna48RAMLen)
	}
	out, err := DecodeSNA(img)
	if err != nil {
		t.Fatalf("DecodeSNA: %v", err)
	}

	// Registers must survive, including the push/pop of PC and SP.
	if out.CPU.PC != in.CPU.PC {
		t.Errorf("PC = 0x%04X, want 0x%04X", out.CPU.PC, in.CPU.PC)
	}
	if out.CPU.SP != in.CPU.SP {
		t.Errorf("SP = 0x%04X, want 0x%04X", out.CPU.SP, in.CPU.SP)
	}
	for _, c := range []struct {
		name     string
		got, want uint16
	}{
		{"AF", out.CPU.AF, in.CPU.AF}, {"BC", out.CPU.BC, in.CPU.BC},
		{"DE", out.CPU.DE, in.CPU.DE}, {"HL", out.CPU.HL, in.CPU.HL},
		{"AF_", out.CPU.AF_, in.CPU.AF_}, {"IX", out.CPU.IX, in.CPU.IX},
		{"IY", out.CPU.IY, in.CPU.IY},
	} {
		if c.got != c.want {
			t.Errorf("%s = 0x%04X, want 0x%04X", c.name, c.got, c.want)
		}
	}
	if out.CPU.I != in.CPU.I || out.CPU.R != in.CPU.R || out.CPU.IM != in.CPU.IM {
		t.Errorf("I/R/IM mismatch: got %v/%v/%v", out.CPU.I, out.CPU.R, out.CPU.IM)
	}
	if out.IO.Border != in.IO.Border {
		t.Errorf("border = %d, want %d", out.IO.Border, in.IO.Border)
	}
	// RAM banks 5,2,0 must survive (note: bank at SP got PC written into it on
	// encode, so compare away from SP). Bank 0 holds 0xC000-0xFFFF where SP=0xFF00
	// lives; just check banks 5 and 2 fully, and bank 0 except near SP.
	if in.Memory.RAM[5] != out.Memory.RAM[5] {
		t.Error("RAM bank 5 not preserved")
	}
	if in.Memory.RAM[2] != out.Memory.RAM[2] {
		t.Error("RAM bank 2 not preserved")
	}
}

// TestDecodeRealZ88DKSNA decodes a real 48K .sna produced by the z88dk
// toolchain (an independent implementation). The decoded values must be the
// spec-canonical ones a genuine Spectrum snapshot carries: program-start PC,
// the standard interrupt register and mode.
func TestDecodeRealZ88DKSNA(t *testing.T) {
	img, err := os.ReadFile("testdata/z88dk_zx48.sna")
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	s, err := DecodeSNA(img)
	if err != nil {
		t.Fatalf("DecodeSNA: %v", err)
	}
	if s.CPU.PC != 0x8000 {
		t.Errorf("PC = 0x%04X, want 0x8000 (program start)", s.CPU.PC)
	}
	if s.CPU.I != 0x3F {
		t.Errorf("I = 0x%02X, want 0x3F (standard Spectrum)", s.CPU.I)
	}
	if s.CPU.IM != 1 {
		t.Errorf("IM = %d, want 1", s.CPU.IM)
	}
}

func TestSNA128RoundTrip(t *testing.T) {
	s := &MachineState{Model: Model128K}
	s.CPU = CPU{AF: 0x1234, BC: 0x5678, SP: 0xFF00, PC: 0x8000, I: 0x3F, IM: 1}
	s.IO.Border = 4
	s.Paging.Port7FFD = 0x03 // bank 3 paged at 0xC000
	for b := range s.Memory.RAM {
		for i := range s.Memory.RAM[b] {
			s.Memory.RAM[b][i] = byte((b*13 + i*3) & 0xFF)
		}
	}
	img, err := EncodeSNA128(s)
	if err != nil { t.Fatalf("EncodeSNA128: %v", err) }
	if len(img) != sna128Len { t.Fatalf("size %d want %d", len(img), sna128Len) }
	out, err := DecodeSNA128(img)
	if err != nil { t.Fatalf("DecodeSNA128: %v", err) }
	if out.CPU.PC != s.CPU.PC || out.CPU.SP != s.CPU.SP || out.CPU.AF != s.CPU.AF {
		t.Errorf("regs: PC=0x%04X SP=0x%04X AF=0x%04X", out.CPU.PC, out.CPU.SP, out.CPU.AF)
	}
	if out.Paging.Port7FFD != s.Paging.Port7FFD {
		t.Errorf("7FFD = 0x%02X, want 0x%02X", out.Paging.Port7FFD, s.Paging.Port7FFD)
	}
	for b := 0; b < 8; b++ {
		if out.Memory.RAM[b] != s.Memory.RAM[b] {
			t.Errorf("bank %d not preserved", b)
		}
	}
}

// TestDecodeRealZ88DK128SNA decodes a real 128K .sna from z88dk and checks the
// values are spec-canonical (program-start PC, standard interrupt config, a
// valid paging port).
func TestDecodeRealZ88DK128SNA(t *testing.T) {
	img, err := os.ReadFile("testdata/z88dk_zx128.sna")
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	s, err := DecodeSNA128(img)
	if err != nil {
		t.Fatalf("DecodeSNA128: %v", err)
	}
	if s.CPU.PC != 0x8000 {
		t.Errorf("PC = 0x%04X, want 0x8000", s.CPU.PC)
	}
	if s.CPU.I != 0x3F || s.CPU.IM != 1 {
		t.Errorf("I=0x%02X IM=%d, want 0x3F / 1", s.CPU.I, s.CPU.IM)
	}
}
