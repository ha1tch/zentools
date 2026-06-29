package snapshot

import (
	"encoding/binary"
	"fmt"
)

// SNA 48K layout: a 27-byte header followed by 48 KiB of RAM (banks 5, 2, 0).
//
// Header (offsets):
//   0   I
//   1   HL'  3 DE'  5 BC'  7 AF'   (alternate set, little-endian pairs)
//   9   HL  11 DE  13 BC  15 IY  17 IX
//   19  interrupt flags (bit 2 = IFF2)
//   20  R
//   21  AF  23 SP
//   25  interrupt mode
//   26  border colour
//
// The 48K SNA does not store PC in the header: at save time PC is pushed onto
// the machine stack, so SP in the header points at it. On load, PC is popped
// from RAM at SP and SP is incremented by 2.

const sna48HeaderLen = 27
const sna48RAMLen = 48 * 1024

// EncodeSNA encodes a MachineState as a 48K .sna image. It requires a 48K
// model; the 128K SNA variant is a separate path (not yet implemented).
func EncodeSNA(s *MachineState) ([]byte, error) {
	if s.Model.Is128KFamily() {
		return nil, fmt.Errorf("128K .sna encoding not yet supported")
	}

	out := make([]byte, sna48HeaderLen+sna48RAMLen)

	out[0] = s.CPU.I
	binary.LittleEndian.PutUint16(out[1:], s.CPU.HL_)
	binary.LittleEndian.PutUint16(out[3:], s.CPU.DE_)
	binary.LittleEndian.PutUint16(out[5:], s.CPU.BC_)
	binary.LittleEndian.PutUint16(out[7:], s.CPU.AF_)
	binary.LittleEndian.PutUint16(out[9:], s.CPU.HL)
	binary.LittleEndian.PutUint16(out[11:], s.CPU.DE)
	binary.LittleEndian.PutUint16(out[13:], s.CPU.BC)
	binary.LittleEndian.PutUint16(out[15:], s.CPU.IY)
	binary.LittleEndian.PutUint16(out[17:], s.CPU.IX)

	var iff byte
	if s.CPU.IFF2 {
		iff |= 0x04
	}
	out[19] = iff
	out[20] = s.CPU.R
	binary.LittleEndian.PutUint16(out[21:], s.CPU.AF)

	// Push PC onto the stack: SP decreases by 2 and PC is written there, so the
	// header SP points at the stored PC.
	sp := s.CPU.SP - 2
	binary.LittleEndian.PutUint16(out[23:], sp)
	out[25] = s.CPU.IM
	out[26] = s.IO.Border

	// Lay out RAM banks 5, 2, 0 at 0x4000, 0x8000, 0xC000.
	copy(out[sna48HeaderLen:], s.Memory.RAM[5][:])
	copy(out[sna48HeaderLen+16384:], s.Memory.RAM[2][:])
	copy(out[sna48HeaderLen+32768:], s.Memory.RAM[0][:])

	// Write the pushed PC into RAM at SP (which lives in the 0x4000-0xFFFF range).
	if err := writeWordAtSpectrumAddr(out[sna48HeaderLen:], sp, s.CPU.PC); err != nil {
		return nil, fmt.Errorf("writing pushed PC: %w", err)
	}

	return out, nil
}

// DecodeSNA decodes a 48K .sna image into a MachineState.
func DecodeSNA(image []byte) (*MachineState, error) {
	if len(image) != sna48HeaderLen+sna48RAMLen {
		return nil, fmt.Errorf(".sna size %d is not a 48K snapshot (want %d)", len(image), sna48HeaderLen+sna48RAMLen)
	}
	s := &MachineState{Model: Model48K}

	s.CPU.I = image[0]
	s.CPU.HL_ = binary.LittleEndian.Uint16(image[1:])
	s.CPU.DE_ = binary.LittleEndian.Uint16(image[3:])
	s.CPU.BC_ = binary.LittleEndian.Uint16(image[5:])
	s.CPU.AF_ = binary.LittleEndian.Uint16(image[7:])
	s.CPU.HL = binary.LittleEndian.Uint16(image[9:])
	s.CPU.DE = binary.LittleEndian.Uint16(image[11:])
	s.CPU.BC = binary.LittleEndian.Uint16(image[13:])
	s.CPU.IY = binary.LittleEndian.Uint16(image[15:])
	s.CPU.IX = binary.LittleEndian.Uint16(image[17:])

	iff := image[19]
	s.CPU.IFF2 = iff&0x04 != 0
	s.CPU.IFF1 = s.CPU.IFF2 // 48K SNA stores only IFF2; restore IFF1 to match
	s.CPU.R = image[20]
	s.CPU.AF = binary.LittleEndian.Uint16(image[21:])
	sp := binary.LittleEndian.Uint16(image[23:])
	s.CPU.IM = image[25]
	s.IO.Border = image[26]

	ram := image[sna48HeaderLen:]
	copy(s.Memory.RAM[5][:], ram[0:16384])
	copy(s.Memory.RAM[2][:], ram[16384:32768])
	copy(s.Memory.RAM[0][:], ram[32768:49152])

	// Pop PC from the stack at SP, then advance SP by 2.
	pc, err := readWordAtSpectrumAddr(ram, sp)
	if err != nil {
		return nil, fmt.Errorf("reading pushed PC: %w", err)
	}
	s.CPU.PC = pc
	s.CPU.SP = sp + 2

	return s, nil
}

// writeWordAtSpectrumAddr writes a little-endian word at a Spectrum address in
// the 0x4000-0xFFFF range, given a slice that begins at 0x4000.
func writeWordAtSpectrumAddr(ram48 []byte, addr, val uint16) error {
	off := int(addr) - 0x4000
	if off < 0 || off+1 >= len(ram48) {
		return fmt.Errorf("address 0x%04X outside 48K RAM", addr)
	}
	binary.LittleEndian.PutUint16(ram48[off:], val)
	return nil
}

// readWordAtSpectrumAddr reads a little-endian word at a Spectrum address in the
// 0x4000-0xFFFF range, given a slice that begins at 0x4000.
func readWordAtSpectrumAddr(ram48 []byte, addr uint16) (uint16, error) {
	off := int(addr) - 0x4000
	if off < 0 || off+1 >= len(ram48) {
		return 0, fmt.Errorf("address 0x%04X outside 48K RAM", addr)
	}
	return binary.LittleEndian.Uint16(ram48[off:]), nil
}

// 128K SNA layout: the 27-byte header, then the three banks visible in the 48K
// window (bank 5 at 0x4000, bank 2 at 0x8000, the currently paged bank at
// 0xC000), then a trailer:
//
//	PC          2 bytes, little-endian (stored explicitly, NOT pushed to stack)
//	port 0x7FFD 1 byte  (paging state; bits 0-2 select the bank at 0xC000)
//	TR-DOS flag 1 byte  (1 if a TR-DOS ROM is paged; 0 otherwise)
//
// then the remaining five RAM banks (every bank except 5, 2 and the paged one)
// in ascending bank-number order. Total: 27 + 3*16384 + 4 + 5*16384 = 131103.

const sna128Len = 27 + 8*16384 + 4 // 131103

// EncodeSNA128 encodes a 128K MachineState as a 128K .sna image.
func EncodeSNA128(s *MachineState) ([]byte, error) {
	if !s.Model.Is128KFamily() {
		return nil, fmt.Errorf("EncodeSNA128 requires a 128K-family model")
	}
	pagedBank := s.Paging.Port7FFD & 0x07

	out := make([]byte, 0, sna128Len)

	// 27-byte header, same field layout as 48K but SP is stored as-is (PC is in
	// the trailer, so there is no pushed-PC convention here).
	hdr := make([]byte, sna48HeaderLen)
	hdr[0] = s.CPU.I
	binary.LittleEndian.PutUint16(hdr[1:], s.CPU.HL_)
	binary.LittleEndian.PutUint16(hdr[3:], s.CPU.DE_)
	binary.LittleEndian.PutUint16(hdr[5:], s.CPU.BC_)
	binary.LittleEndian.PutUint16(hdr[7:], s.CPU.AF_)
	binary.LittleEndian.PutUint16(hdr[9:], s.CPU.HL)
	binary.LittleEndian.PutUint16(hdr[11:], s.CPU.DE)
	binary.LittleEndian.PutUint16(hdr[13:], s.CPU.BC)
	binary.LittleEndian.PutUint16(hdr[15:], s.CPU.IY)
	binary.LittleEndian.PutUint16(hdr[17:], s.CPU.IX)
	if s.CPU.IFF2 {
		hdr[19] |= 0x04
	}
	hdr[20] = s.CPU.R
	binary.LittleEndian.PutUint16(hdr[21:], s.CPU.AF)
	binary.LittleEndian.PutUint16(hdr[23:], s.CPU.SP)
	hdr[25] = s.CPU.IM
	hdr[26] = s.IO.Border
	out = append(out, hdr...)

	// The three banks in the 48K window.
	out = append(out, s.Memory.RAM[5][:]...)
	out = append(out, s.Memory.RAM[2][:]...)
	out = append(out, s.Memory.RAM[pagedBank][:]...)

	// Trailer.
	out = appendU16le(out, s.CPU.PC)
	out = append(out, s.Paging.Port7FFD)
	out = append(out, 0) // TR-DOS flag

	// Remaining banks in ascending order.
	for b := 0; b < 8; b++ {
		if b == 5 || b == 2 || b == int(pagedBank) {
			continue
		}
		out = append(out, s.Memory.RAM[b][:]...)
	}
	return out, nil
}

// DecodeSNA128 decodes a 128K .sna image into a MachineState.
func DecodeSNA128(image []byte) (*MachineState, error) {
	if len(image) != sna128Len {
		return nil, fmt.Errorf(".sna size %d is not a 128K snapshot (want %d)", len(image), sna128Len)
	}
	s := &MachineState{Model: Model128K}

	s.CPU.I = image[0]
	s.CPU.HL_ = binary.LittleEndian.Uint16(image[1:])
	s.CPU.DE_ = binary.LittleEndian.Uint16(image[3:])
	s.CPU.BC_ = binary.LittleEndian.Uint16(image[5:])
	s.CPU.AF_ = binary.LittleEndian.Uint16(image[7:])
	s.CPU.HL = binary.LittleEndian.Uint16(image[9:])
	s.CPU.DE = binary.LittleEndian.Uint16(image[11:])
	s.CPU.BC = binary.LittleEndian.Uint16(image[13:])
	s.CPU.IY = binary.LittleEndian.Uint16(image[15:])
	s.CPU.IX = binary.LittleEndian.Uint16(image[17:])
	s.CPU.IFF2 = image[19]&0x04 != 0
	s.CPU.IFF1 = s.CPU.IFF2
	s.CPU.R = image[20]
	s.CPU.AF = binary.LittleEndian.Uint16(image[21:])
	s.CPU.SP = binary.LittleEndian.Uint16(image[23:])
	s.CPU.IM = image[25]
	s.IO.Border = image[26]

	pos := sna48HeaderLen
	// Trailer sits after the three window banks.
	trailer := pos + 3*16384
	s.CPU.PC = binary.LittleEndian.Uint16(image[trailer:])
	port := image[trailer+2]
	s.Paging.Port7FFD = port
	pagedBank := int(port & 0x07)

	// Place the three window banks.
	copy(s.Memory.RAM[5][:], image[pos:pos+16384])
	copy(s.Memory.RAM[2][:], image[pos+16384:pos+32768])
	copy(s.Memory.RAM[pagedBank][:], image[pos+32768:pos+49152])

	// Remaining banks follow the 4-byte trailer, in ascending order.
	rp := trailer + 4
	for b := 0; b < 8; b++ {
		if b == 5 || b == 2 || b == pagedBank {
			continue
		}
		copy(s.Memory.RAM[b][:], image[rp:rp+16384])
		rp += 16384
	}
	return s, nil
}

func appendU16le(b []byte, v uint16) []byte {
	return append(b, byte(v), byte(v>>8))
}
