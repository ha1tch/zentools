// Package build turns assembled Z80 machine code into loadable ZX Spectrum
// artifacts: tape images (.tap, .tzx) and snapshots (.sna, .z80).
//
// Tape output is a direct encoding of the code as a CODE block. Snapshot output
// overlays the code onto a real booted-machine state (captured per model) so
// that ROM routines the code may call find the system in a sane, initialised
// posture — then sets the program counter and stack pointer so the snapshot
// runs the user code on load. The boot snapshots are embedded; see embed.go.
package build

import (
	"fmt"

	"github.com/ha1tch/zentools/pkg/snapshot"
	"github.com/ha1tch/zentools/pkg/tap"
	"github.com/ha1tch/zentools/pkg/tzx"
)

// Model identifies the target Spectrum for snapshot output.
type Model string

const (
	Model48K    Model = "48k"
	Model128K   Model = "128k"
	ModelPlus2  Model = "plus2"
	ModelPlus2A Model = "plus2a"
	ModelPlus3  Model = "plus3"
)

// Default stack pointer for snapshot output when the caller does not specify
// one. 0xFF00 sits high in RAM, clear of the ROM and of typical code/data
// loaded at 0x8000, leaving headroom for the stack to grow downward.
const DefaultSP uint16 = 0xFF00

// Request describes one build: the assembled bytes, where they load, and the
// entry/stack configuration for snapshot output.
type Request struct {
	Name    string // program name embedded in tape/snapshot (<= 10 chars on tape)
	Code    []byte // assembled machine code
	Origin  uint16 // load address of the first byte of Code
	Start   uint16 // PC entry point for snapshot output (required for snapshots)
	SP      uint16 // stack pointer for snapshot output
	Model   Model  // target model for snapshot output
}

// EncodeTAP produces a .tap image containing the code as a single CODE block.
func EncodeTAP(r Request) []byte {
	return tap.EncodeCode(tapeName(r.Name), r.Code, r.Origin)
}

// EncodeTZX produces a .tzx image by wrapping the TAP encoding.
func EncodeTZX(r Request) ([]byte, error) {
	return tzx.EncodeFromTAP(EncodeTAP(r), tzx.EncodeOptions{})
}

// EncodeTZXFromTAP wraps an already-built TAP image (which may include a loader
// block) as a TZX image.
func EncodeTZXFromTAP(tapImage []byte) ([]byte, error) {
	return tzx.EncodeFromTAP(tapImage, tzx.EncodeOptions{})
}

// EncodeSNA overlays the code onto the model's boot state and emits a .sna.
func EncodeSNA(r Request) ([]byte, error) {
	s, err := overlay(r)
	if err != nil {
		return nil, err
	}
	if s.Model.Is128KFamily() {
		return snapshot.EncodeSNA128(s)
	}
	return snapshot.EncodeSNA(s)
}

// EncodeZ80 overlays the code onto the model's boot state and emits a v3 .z80.
func EncodeZ80(r Request) ([]byte, error) {
	s, err := overlay(r)
	if err != nil {
		return nil, err
	}
	return snapshot.EncodeZ80v3(s)
}

// overlay loads the model's boot snapshot, writes the code into memory at its
// origin, and sets PC and SP so the snapshot runs the user program on load.
func overlay(r Request) (*snapshot.MachineState, error) {
	s, err := bootState(r.Model)
	if err != nil {
		return nil, err
	}
	if err := writeMemory(s, r.Origin, r.Code); err != nil {
		return nil, err
	}
	s.CPU.PC = r.Start
	s.CPU.SP = r.SP
	return s, nil
}

// CodeEnd returns the address just past the last byte of the loaded code.
func (r Request) CodeEnd() int { return int(r.Origin) + len(r.Code) }

// SPWarning returns a human-readable warning if the stack pointer sits inside,
// or dangerously close below, the loaded code — meaning the stack could grow
// down into the program. It returns an empty string when the layout is safe.
// The stack grows downward from SP, so the risk is the code occupying memory
// just below SP.
func (r Request) SPWarning() string {
	codeStart := int(r.Origin)
	codeEnd := r.CodeEnd() // exclusive
	sp := int(r.SP)

	// SP pointing directly into the code body: the stack's first pushes land on
	// program bytes.
	if sp > codeStart && sp <= codeEnd {
		return fmt.Sprintf("stack pointer 0x%04X is inside the code (0x%04X-0x%04X); "+
			"the first stack writes will overwrite the program", r.SP, codeStart, codeEnd-1)
	}
	// SP just above the code: a deep stack will grow down into it. 256 bytes is a
	// conservative margin for typical ROM/IM1 stack usage.
	const margin = 256
	if sp > codeEnd && sp-codeEnd < margin {
		return fmt.Sprintf("stack pointer 0x%04X is only %d bytes above the code end "+
			"(0x%04X); a deep stack may grow down into the program", r.SP, sp-codeEnd, codeEnd-1)
	}
	return ""
}

// writeMemory writes data into the machine's RAM starting at guest address
// origin, mapping each address to the correct RAM bank for the model's current
// paging. Writing into ROM (below 0x4000) is an error.
func writeMemory(s *snapshot.MachineState, origin uint16, data []byte) error {
	for i, b := range data {
		addr := int(origin) + i
		if addr > 0xFFFF {
			return fmt.Errorf("code overflows the 64K address space at offset %d (0x%X)", i, addr)
		}
		bank, off, err := mapAddress(s, uint16(addr))
		if err != nil {
			return err
		}
		s.Memory.RAM[bank][off] = b
	}
	return nil
}

// mapAddress resolves a guest address to (RAM bank, offset within bank) for the
// machine's current paging. The lower 16K (0x0000-0x3FFF) is ROM and cannot be
// written. 0x4000 is always bank 5, 0x8000 always bank 2, and 0xC000 is the
// bank selected by the 128K paging port (bank 0 on a 48K machine).
func mapAddress(s *snapshot.MachineState, addr uint16) (bank, off int, err error) {
	switch {
	case addr < 0x4000:
		return 0, 0, fmt.Errorf("address 0x%04X is in ROM; code must load at 0x4000 or above", addr)
	case addr < 0x8000:
		return 5, int(addr - 0x4000), nil
	case addr < 0xC000:
		return 2, int(addr - 0x8000), nil
	default:
		top := 0
		if s.Model.Is128KFamily() {
			top = int(s.Paging.Port7FFD & 0x07)
		}
		return top, int(addr - 0xC000), nil
	}
}
