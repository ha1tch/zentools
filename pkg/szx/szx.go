// Package szx reads and writes ZX Spectrum zx-state (.szx) snapshot files, the
// modern, extensible snapshot format used by Spectaculator and Fuse (spec v1.5,
// https://www.spectaculator.com/docs/zx-state/intro.html). It decodes into and
// encodes from the same neutral snapshot.MachineState that pkg/snapshot's SNA
// and .z80 codecs use -- SZX is a third codec for the same type, not a
// separate parallel representation.
//
// Scope: the four blocks that carry the CPU/paging/memory state every
// snapshot needs -- ZXSTCREATOR, ZXSTZ80REGS, ZXSTSPECREGS, ZXSTRAMPAGE -- are
// fully decoded and encoded. The zx-state spec defines over thirty further
// block types for peripheral state (AY sound, joysticks, disk controllers,
// speech synthesis, and so on -- see block_types.html in the spec). Per the
// spec's own rule ("If your parser does not recognise the block type, it can
// simply skip the remaining dwSize bytes"), Decode skips every block it
// doesn't implement rather than erroring. This is a deliberate scope
// boundary, not an oversight: MachineState has no home for peripheral state,
// and round-tripping a file through Decode then Encode will not preserve
// those blocks. A file's CPU/paging/memory content, and any RAM page or
// machine model this package doesn't support (Pentagon 512/1024, Scorpion,
// Timex variants, and any page number beyond 7 -- none of which
// snapshot.Model or snapshot.Memory represent), causes a clear decode error
// rather than a silent partial read.
package szx

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/ha1tch/zentools/pkg/snapshot"
)

const magic = "ZXST"

// Machine IDs, from the zx-state header's chMachineId (header.html). Only
// the models snapshot.Model itself represents are accepted by Decode; the
// rest (Pentagon/Scorpion/Timex/SE variants) are named here only so a clear
// "unsupported model" error can name what was actually found.
const (
	machine16K          = 0
	machine48K          = 1
	machine128K         = 2
	machinePlus2        = 3
	machinePlus2A       = 4
	machinePlus3        = 5
	machinePlus3E       = 6
	machinePentagon128  = 7
	machineTC2048       = 8
	machineTC2068       = 9
	machineScorpion     = 10
	machineSE           = 11
	machineTS2068       = 12
	machinePentagon512  = 13
	machinePentagon1024 = 14
	machineNTSC48K      = 15
	machine128Ke        = 16
)

// Block IDs are 4-byte ASCII sequences stored as a little-endian DWORD, per
// block.html ("e.g. the ZXSTCREATOR block has a block id of 'C','R','T','R'").
// blockID below reverses the natural ASCII order to match: the DWORD's low
// byte is the first letter.
func blockID(s string) uint32 {
	return uint32(s[0]) | uint32(s[1])<<8 | uint32(s[2])<<16 | uint32(s[3])<<24
}

var (
	idCreator  = blockID("CRTR")
	idZ80Regs  = blockID("Z80R")
	idSpecRegs = blockID("SPCR")
	idRAMPage  = blockID("RAMP")
)

const (
	ramPageCompressed = 1 // ZXSTRF_COMPRESSED, rampage.html
)

// modelToMachineID and machineIDToModel translate between snapshot.Model and
// the zx-state chMachineId values it corresponds to. Only the five models
// snapshot.Model defines have a mapping.
func modelToMachineID(m snapshot.Model) (byte, error) {
	switch m {
	case snapshot.Model48K:
		return machine48K, nil
	case snapshot.Model128K:
		return machine128K, nil
	case snapshot.ModelPlus2:
		return machinePlus2, nil
	case snapshot.ModelPlus2A:
		return machinePlus2A, nil
	case snapshot.ModelPlus3:
		return machinePlus3, nil
	default:
		return 0, fmt.Errorf("szx: unsupported Model %v", m)
	}
}

func machineIDToModel(id byte) (snapshot.Model, error) {
	switch id {
	case machine48K, machineNTSC48K:
		return snapshot.Model48K, nil
	case machine128K, machine128Ke:
		return snapshot.Model128K, nil
	case machinePlus2:
		return snapshot.ModelPlus2, nil
	case machinePlus2A:
		return snapshot.ModelPlus2A, nil
	case machinePlus3, machinePlus3E:
		return snapshot.ModelPlus3, nil
	case machine16K:
		return 0, fmt.Errorf("szx: 16K Spectrum has no snapshot.Model equivalent (RAM banking assumes 48K/128K-family)")
	default:
		return 0, fmt.Errorf("szx: machine ID %d is not a model this package represents (Pentagon/Scorpion/Timex/SE variants are out of scope -- see this package's own doc comment)", id)
	}
}

// Decode parses a zx-state (.szx) image into a MachineState. Unrecognised
// blocks are skipped, per the spec's own rule; see this package's doc
// comment for exactly which blocks are understood.
func Decode(image []byte) (*snapshot.MachineState, error) {
	if len(image) < 8 {
		return nil, fmt.Errorf("szx: image too short (%d bytes) to contain a header", len(image))
	}
	if string(image[0:4]) != magic {
		return nil, fmt.Errorf("szx: missing %q signature", magic)
	}
	machineID := image[6]
	model, err := machineIDToModel(machineID)
	if err != nil {
		return nil, err
	}
	s := &snapshot.MachineState{Model: model}

	pos := 8
	sawZ80Regs, sawSpecRegs := false, false
	for pos+8 <= len(image) {
		id := binary.LittleEndian.Uint32(image[pos:])
		size := binary.LittleEndian.Uint32(image[pos+4:])
		dataStart := pos + 8
		dataEnd := dataStart + int(size)
		if dataEnd > len(image) {
			return nil, fmt.Errorf("szx: block at offset %d claims size %d, past end of file", pos, size)
		}
		data := image[dataStart:dataEnd]

		switch id {
		case idZ80Regs:
			if err := decodeZ80Regs(data, s); err != nil {
				return nil, err
			}
			sawZ80Regs = true
		case idSpecRegs:
			if err := decodeSpecRegs(data, s); err != nil {
				return nil, err
			}
			sawSpecRegs = true
		case idRAMPage:
			if err := decodeRAMPage(data, s); err != nil {
				return nil, err
			}
		default:
			// Unrecognised (or recognised-but-unimplemented, e.g. ZXSTAYBLOCK,
			// ZXSTJOYSTICK, disk-image blocks) -- skip, per the spec.
		}
		pos = dataEnd
	}

	if !sawZ80Regs {
		return nil, fmt.Errorf("szx: missing required ZXSTZ80REGS block")
	}
	if !sawSpecRegs {
		return nil, fmt.Errorf("szx: missing required ZXSTSPECREGS block")
	}
	return s, nil
}

// decodeZ80Regs reads a ZXSTZ80REGS block's data (z80regs.html) into s.CPU.
// The block also carries dwCyclesStart/chHoldIntReqCycles/chFlags/wMemPtr --
// fine-grained mid-instruction emulator-resume state with no home in
// snapshot.CPU. This package reads the registers, not that state; see this
// package's own doc comment.
func decodeZ80Regs(data []byte, s *snapshot.MachineState) error {
	const wantLen = 37
	if len(data) < wantLen {
		return fmt.Errorf("szx: ZXSTZ80REGS block is %d bytes, want at least %d", len(data), wantLen)
	}
	c := &s.CPU
	c.AF = binary.LittleEndian.Uint16(data[0:])
	c.BC = binary.LittleEndian.Uint16(data[2:])
	c.DE = binary.LittleEndian.Uint16(data[4:])
	c.HL = binary.LittleEndian.Uint16(data[6:])
	c.AF_ = binary.LittleEndian.Uint16(data[8:])
	c.BC_ = binary.LittleEndian.Uint16(data[10:])
	c.DE_ = binary.LittleEndian.Uint16(data[12:])
	c.HL_ = binary.LittleEndian.Uint16(data[14:])
	c.IX = binary.LittleEndian.Uint16(data[16:])
	c.IY = binary.LittleEndian.Uint16(data[18:])
	c.SP = binary.LittleEndian.Uint16(data[20:])
	c.PC = binary.LittleEndian.Uint16(data[22:])
	c.I = data[24]
	c.R = data[25]
	c.IFF1 = data[26] != 0
	c.IFF2 = data[27] != 0
	c.IM = data[28]
	// data[29:33] dwCyclesStart, data[33] chHoldIntReqCycles,
	// data[34] chFlags, data[35:37] wMemPtr -- not modelled, see above.
	return nil
}

// encodeZ80Regs writes a ZXSTZ80REGS block from s.CPU. The emulator-resume
// fields this package doesn't model (dwCyclesStart, chHoldIntReqCycles,
// chFlags, wMemPtr) are written as zero.
func encodeZ80Regs(s *snapshot.MachineState) []byte {
	data := make([]byte, 37)
	c := &s.CPU
	binary.LittleEndian.PutUint16(data[0:], c.AF)
	binary.LittleEndian.PutUint16(data[2:], c.BC)
	binary.LittleEndian.PutUint16(data[4:], c.DE)
	binary.LittleEndian.PutUint16(data[6:], c.HL)
	binary.LittleEndian.PutUint16(data[8:], c.AF_)
	binary.LittleEndian.PutUint16(data[10:], c.BC_)
	binary.LittleEndian.PutUint16(data[12:], c.DE_)
	binary.LittleEndian.PutUint16(data[14:], c.HL_)
	binary.LittleEndian.PutUint16(data[16:], c.IX)
	binary.LittleEndian.PutUint16(data[18:], c.IY)
	binary.LittleEndian.PutUint16(data[20:], c.SP)
	binary.LittleEndian.PutUint16(data[22:], c.PC)
	data[24] = c.I
	data[25] = c.R
	if c.IFF1 {
		data[26] = 1
	}
	if c.IFF2 {
		data[27] = 1
	}
	data[28] = c.IM
	// data[29:37] left zero: dwCyclesStart, chHoldIntReqCycles, chFlags, wMemPtr.
	return withBlockHeader(idZ80Regs, data)
}

// decodeSpecRegs reads a ZXSTSPECREGS block's data (specregs.html) into
// s.Paging and s.IO.Border.
func decodeSpecRegs(data []byte, s *snapshot.MachineState) error {
	const wantLen = 8
	if len(data) < wantLen {
		return fmt.Errorf("szx: ZXSTSPECREGS block is %d bytes, want at least %d", len(data), wantLen)
	}
	s.IO.Border = data[0]
	s.Paging.Port7FFD = data[1]
	s.Paging.Port1FFD = data[2] // shares its offset with chEff7 (Pentagon 1024, out of scope)
	s.Paging.Locked = data[1]&0x20 != 0
	return nil
}

func encodeSpecRegs(s *snapshot.MachineState) []byte {
	data := make([]byte, 8)
	data[0] = s.IO.Border
	data[1] = s.Paging.Port7FFD
	data[2] = s.Paging.Port1FFD
	// data[3] chFe, data[4:8] chReserved left zero.
	return withBlockHeader(idSpecRegs, data)
}

// decodeRAMPage reads one ZXSTRAMPAGE block (rampage.html) into the
// corresponding bank of s.Memory.RAM, decompressing with zlib if the
// ZXSTRF_COMPRESSED flag is set.
func decodeRAMPage(data []byte, s *snapshot.MachineState) error {
	if len(data) < 3 {
		return fmt.Errorf("szx: ZXSTRAMPAGE block is %d bytes, want at least 3", len(data))
	}
	flags := binary.LittleEndian.Uint16(data[0:])
	pageNo := data[2]
	if pageNo >= 8 {
		return fmt.Errorf("szx: RAM page %d is outside the 0-7 range snapshot.Memory represents (Pentagon 512/1024 and Scorpion 256 pages are out of scope)", pageNo)
	}
	payload := data[3:]
	if flags&ramPageCompressed != 0 {
		r, err := zlib.NewReader(bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("szx: RAM page %d: zlib: %w", pageNo, err)
		}
		defer r.Close()
		decompressed, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("szx: RAM page %d: zlib: %w", pageNo, err)
		}
		payload = decompressed
	}
	if len(payload) != 16384 {
		return fmt.Errorf("szx: RAM page %d decompresses to %d bytes, want 16384", pageNo, len(payload))
	}
	copy(s.Memory.RAM[pageNo][:], payload)
	return nil
}

// encodeRAMPage writes one ZXSTRAMPAGE block for bank pageNo, zlib-compressed.
func encodeRAMPage(pageNo int, bank *[16384]byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(bank[:]); err != nil {
		return nil, fmt.Errorf("szx: RAM page %d: zlib: %w", pageNo, err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("szx: RAM page %d: zlib: %w", pageNo, err)
	}
	data := make([]byte, 3, 3+buf.Len())
	binary.LittleEndian.PutUint16(data[0:], ramPageCompressed)
	data[2] = byte(pageNo)
	data = append(data, buf.Bytes()...)
	return withBlockHeader(idRAMPage, data), nil
}

// withBlockHeader prepends a ZXSTBLOCK header (block.html: dwId, dwSize --
// dwSize excludes the header itself) to data.
func withBlockHeader(id uint32, data []byte) []byte {
	out := make([]byte, 8+len(data))
	binary.LittleEndian.PutUint32(out[0:], id)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(data)))
	copy(out[8:], data)
	return out
}

// banksForModel returns which RAM banks are meaningful for m and so should
// be written on encode, per rampage.html: "For 48k Spectrums ... pages 5, 2
// ... and 0 ... are saved. For 128k Spectrums ... all pages (0-7) are saved."
func banksForModel(m snapshot.Model) []int {
	if m == snapshot.Model48K {
		return []int{5, 2, 0}
	}
	return []int{0, 1, 2, 3, 4, 5, 6, 7}
}

// Encode writes s as a zx-state (.szx) image: the header, a ZXSTCREATOR
// block naming this package, ZXSTZ80REGS, ZXSTSPECREGS, and one ZXSTRAMPAGE
// block per bank banksForModel says is meaningful for s.Model.
func Encode(s *snapshot.MachineState) ([]byte, error) {
	machineID, err := modelToMachineID(s.Model)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString(magic)
	out.WriteByte(1) // chMajorVersion
	out.WriteByte(4) // chMinorVersion
	out.WriteByte(machineID)
	out.WriteByte(0) // chFlags

	out.Write(encodeCreator())
	out.Write(encodeZ80Regs(s))
	out.Write(encodeSpecRegs(s))
	for _, bank := range banksForModel(s.Model) {
		block, err := encodeRAMPage(bank, &s.Memory.RAM[bank])
		if err != nil {
			return nil, err
		}
		out.Write(block)
	}
	return out.Bytes(), nil
}

// encodeCreator writes a ZXSTCREATOR block (creator.html) identifying this
// package as the file's creator.
func encodeCreator() []byte {
	const name = "github.com/ha1tch/zentools/pkg/szx"
	data := make([]byte, 32+2+2) // szCreator[32] + chMajorVersion + chMinorVersion
	copy(data[:32], name)        // truncated to 32 bytes if longer; null-padded if shorter
	// version fields left zero: this package doesn't track its own release
	// number internally, and a fixed placeholder would be actively misleading.
	return withBlockHeader(idCreator, data)
}
