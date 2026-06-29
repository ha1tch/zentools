package snapshot

import (
	"encoding/binary"
	"fmt"
)

// .z80 v1 layout: a 30-byte header then the 48 KiB of RAM (0x4000-0xFFFF) as a
// single block, RLE-compressed when header flag bit 5 is set.
//
// Header fields (v1):
//
//	0   A         1   F
//	2-3 BC        4-5 HL
//	6-7 PC        (zero signals v2/v3 with an extended header)
//	8-9 SP        10  I    11 R (bit 7 cleared; real bit 7 lives in flags1 bit 0)
//	12  flags1    (bit0 = R bit7, bits1-3 = border, bit5 = RAM compressed)
//	13  E    14  D
//	15  C'   16  B'   17 E'  18 D'  19 L'  20 H'  21 A'  22 F'
//	23-24 IY  25-26 IX
//	27  IFF1  28 IFF2
//	29  flags2 (bits 0-1 = interrupt mode)
//
// The v1 RLE scheme: a run of five or more identical bytes is encoded as
// ED ED count value. A literal ED is left as-is unless it would start a false
// marker. The compressed RAM block ends with the four-byte marker 00 ED ED 00.

const z80HeaderV1Len = 30
const z80RAMLen = 48 * 1024

// EncodeZ80 encodes a 48K MachineState as a compressed .z80 v1 image.
func EncodeZ80(s *MachineState) ([]byte, error) {
	if s.Model.Is128KFamily() {
		return nil, fmt.Errorf("128K .z80 encoding not yet supported")
	}

	hdr := make([]byte, z80HeaderV1Len)
	hdr[0] = byte(s.CPU.AF >> 8)   // A
	hdr[1] = byte(s.CPU.AF & 0xFF) // F
	binary.LittleEndian.PutUint16(hdr[2:], s.CPU.BC)
	binary.LittleEndian.PutUint16(hdr[4:], s.CPU.HL)
	binary.LittleEndian.PutUint16(hdr[6:], s.CPU.PC)
	binary.LittleEndian.PutUint16(hdr[8:], s.CPU.SP)
	hdr[10] = s.CPU.I
	hdr[11] = s.CPU.R & 0x7F

	var flags1 byte
	if s.CPU.R&0x80 != 0 {
		flags1 |= 0x01
	}
	flags1 |= (s.IO.Border & 0x07) << 1
	flags1 |= 0x20 // RAM is compressed
	hdr[12] = flags1

	hdr[13] = byte(s.CPU.DE & 0xFF) // E
	hdr[14] = byte(s.CPU.DE >> 8)   // D
	hdr[15] = byte(s.CPU.BC_ & 0xFF)
	hdr[16] = byte(s.CPU.BC_ >> 8)
	hdr[17] = byte(s.CPU.DE_ & 0xFF)
	hdr[18] = byte(s.CPU.DE_ >> 8)
	hdr[19] = byte(s.CPU.HL_ & 0xFF)
	hdr[20] = byte(s.CPU.HL_ >> 8)
	hdr[21] = byte(s.CPU.AF_ >> 8)   // A'
	hdr[22] = byte(s.CPU.AF_ & 0xFF) // F'
	binary.LittleEndian.PutUint16(hdr[23:], s.CPU.IY)
	binary.LittleEndian.PutUint16(hdr[25:], s.CPU.IX)
	if s.CPU.IFF1 {
		hdr[27] = 1
	}
	if s.CPU.IFF2 {
		hdr[28] = 1
	}
	hdr[29] = s.CPU.IM & 0x03

	// Assemble the 48 KiB image (banks 5, 2, 0) and compress it.
	ram := make([]byte, 0, z80RAMLen)
	ram = append(ram, s.Memory.RAM[5][:]...)
	ram = append(ram, s.Memory.RAM[2][:]...)
	ram = append(ram, s.Memory.RAM[0][:]...)

	out := append(hdr, compressZ80(ram)...)
	out = append(out, 0x00, 0xED, 0xED, 0x00) // v1 end marker
	return out, nil
}

// DecodeZ80 decodes a .z80 v1 image into a MachineState. v2/v3 (extended
// header, signalled by PC == 0) is not yet supported.
func DecodeZ80(image []byte) (*MachineState, error) {
	if len(image) < z80HeaderV1Len {
		return nil, fmt.Errorf(".z80 too short (%d bytes)", len(image))
	}
	pc := binary.LittleEndian.Uint16(image[6:])
	if pc == 0 {
		return decodeZ80Extended(image)
	}

	s := &MachineState{Model: Model48K}
	s.CPU.AF = uint16(image[0])<<8 | uint16(image[1])
	s.CPU.BC = binary.LittleEndian.Uint16(image[2:])
	s.CPU.HL = binary.LittleEndian.Uint16(image[4:])
	s.CPU.PC = pc
	s.CPU.SP = binary.LittleEndian.Uint16(image[8:])
	s.CPU.I = image[10]

	flags1 := image[12]
	r := image[11] & 0x7F
	if flags1&0x01 != 0 {
		r |= 0x80
	}
	s.CPU.R = r
	s.IO.Border = (flags1 >> 1) & 0x07
	compressed := flags1&0x20 != 0

	s.CPU.DE = uint16(image[14])<<8 | uint16(image[13])
	s.CPU.BC_ = uint16(image[16])<<8 | uint16(image[15])
	s.CPU.DE_ = uint16(image[18])<<8 | uint16(image[17])
	s.CPU.HL_ = uint16(image[20])<<8 | uint16(image[19])
	s.CPU.AF_ = uint16(image[21])<<8 | uint16(image[22])
	s.CPU.IY = binary.LittleEndian.Uint16(image[23:])
	s.CPU.IX = binary.LittleEndian.Uint16(image[25:])
	s.CPU.IFF1 = image[27] != 0
	s.CPU.IFF2 = image[28] != 0
	s.CPU.IM = image[29] & 0x03

	body := image[z80HeaderV1Len:]
	var ram []byte
	if compressed {
		ram = decompressZ80(body)
	} else {
		ram = body
	}
	if len(ram) < z80RAMLen {
		return nil, fmt.Errorf("decompressed RAM is %d bytes, want %d", len(ram), z80RAMLen)
	}
	copy(s.Memory.RAM[5][:], ram[0:16384])
	copy(s.Memory.RAM[2][:], ram[16384:32768])
	copy(s.Memory.RAM[0][:], ram[32768:49152])

	return s, nil
}

// compressZ80 applies the v1 RLE scheme: runs of 5+ identical bytes become
// ED ED count value. A run of ED bytes is always encoded (even a pair) to avoid
// emitting a literal ED ED that a decoder would misread as a marker.
func compressZ80(data []byte) []byte {
	var out []byte
	i := 0
	for i < len(data) {
		b := data[i]
		// Measure the full run of this byte (uncapped; emitRuns splits at 255).
		run := 1
		for i+run < len(data) && data[i+run] == b {
			run++
		}

		switch {
		case b == 0xED && run >= 2:
			// Encode ED runs of two or more so no literal ED ED is produced.
			out = emitRuns(out, run, b)
		case b == 0xED && run == 1:
			out = append(out, 0xED)
		case run >= 5:
			out = emitRuns(out, run, b)
		default:
			// Short run of a non-ED byte: emit literally.
			for k := 0; k < run; k++ {
				out = append(out, b)
			}
		}
		i += run
	}
	return out
}

// emitRuns writes one or more "ED ED count value" groups for a run of `run`
// bytes of value b, splitting at the 255-per-group limit.
func emitRuns(out []byte, run int, b byte) []byte {
	for run > 0 {
		n := run
		if n > 255 {
			n = 255
		}
		out = append(out, 0xED, 0xED, byte(n), b)
		run -= n
	}
	return out
}

// decompressZ80 reverses the v1 RLE scheme. It stops at the end of input or at
// the 00 ED ED 00 end marker if present.
func decompressZ80(data []byte) []byte {
	var out []byte
	i := 0
	for i < len(data) {
		// End marker: 00 ED ED 00.
		if i+3 < len(data) && data[i] == 0x00 && data[i+1] == 0xED && data[i+2] == 0xED && data[i+3] == 0x00 {
			break
		}
		if i+1 < len(data) && data[i] == 0xED && data[i+1] == 0xED {
			if i+3 >= len(data) {
				break
			}
			count := int(data[i+2])
			value := data[i+3]
			for k := 0; k < count; k++ {
				out = append(out, value)
			}
			i += 4
			continue
		}
		out = append(out, data[i])
		i++
	}
	return out
}

// --- .z80 v2/v3 (extended header) ----------------------------------------
//
// When the v1 PC field (offset 6) is zero, the file is v2 or v3: the 30-byte
// base header is followed by a 2-byte extended-header length and that many more
// header bytes, then memory as a sequence of per-page blocks.
//
// Extended header (from offset 30):
//	30-31 extended header length (23 = v2; 54 or 55 = v3)
//	32-33 real PC
//	34    hardware mode (interpreted per version; see hwIs128K)
//	35    port 0x7FFD value (128K paging)
//	36    Interface I / other (ignored here)
//	37    flags (bit7 doubles bits 0-1 of port 0x7FFD on some writers; ignored)
//	38    port 0xFFFD (AY register select; not modelled)
//	39-54 AY register values (not modelled)
//	(v3 adds low/high T-state counters and more; not needed to restore RAM)
//
// Memory blocks, each: [length:2][page:1][data]. length 0xFFFF means the 16384
// bytes are stored uncompressed; otherwise the data is v1-RLE compressed and
// `length` is its compressed size. Page numbers map to RAM banks depending on
// machine type.

// hwIs128K reports whether a v2/v3 hardware-mode byte denotes a 128K-family
// machine. The mapping differs between v2 and v3.
func hwIs128K(version, mode byte) bool {
	if version == 2 {
		// v2: 0,1 = 48K; 3,4 = 128K (+ variants).
		return mode >= 3
	}
	// v3: 0,1,3 = 48K family; 4,5,6 = 128K; 7,12 = +2/+2A/+3; 9 Pentagon; etc.
	switch mode {
	case 0, 1, 3:
		return false
	default:
		return true
	}
}

// pageToBank maps a .z80 memory-block page number to a RAM bank index. For 128K
// machines page N holds RAM bank N-3 (pages 3..10 = banks 0..7). For 48K
// machines only pages 4, 5 and 8 carry RAM (0x8000, 0xC000, 0x4000 = banks 2, 0,
// 5); other pages (ROM) are skipped by returning ok=false.
func pageToBank(page byte, is128K bool) (bank int, ok bool) {
	if is128K {
		if page >= 3 && page <= 10 {
			return int(page) - 3, true
		}
		return 0, false
	}
	switch page {
	case 8:
		return 5, true // 0x4000
	case 4:
		return 2, true // 0x8000
	case 5:
		return 0, true // 0xC000
	default:
		return 0, false // ROM pages 0..3 etc.
	}
}

// decodeZ80Extended decodes a v2/v3 .z80 image (PC at offset 6 is zero).
func decodeZ80Extended(image []byte) (*MachineState, error) {
	if len(image) < 32 {
		return nil, fmt.Errorf(".z80 extended header truncated")
	}
	extLen := int(binary.LittleEndian.Uint16(image[30:]))
	var version byte
	switch extLen {
	case 23:
		version = 2
	case 54, 55:
		version = 3
	default:
		return nil, fmt.Errorf("unsupported .z80 extended header length %d", extLen)
	}
	headerEnd := 32 + extLen
	if len(image) < headerEnd {
		return nil, fmt.Errorf(".z80 extended header claims %d bytes, file too short", extLen)
	}

	s := &MachineState{}
	// Base-header registers (same offsets as v1, except PC which is in the ext header).
	s.CPU.AF = uint16(image[0])<<8 | uint16(image[1])
	s.CPU.BC = binary.LittleEndian.Uint16(image[2:])
	s.CPU.HL = binary.LittleEndian.Uint16(image[4:])
	s.CPU.SP = binary.LittleEndian.Uint16(image[8:])
	s.CPU.I = image[10]
	flags1 := image[12]
	r := image[11] & 0x7F
	if flags1&0x01 != 0 {
		r |= 0x80
	}
	s.CPU.R = r
	s.IO.Border = (flags1 >> 1) & 0x07
	s.CPU.DE = uint16(image[14])<<8 | uint16(image[13])
	s.CPU.BC_ = uint16(image[16])<<8 | uint16(image[15])
	s.CPU.DE_ = uint16(image[18])<<8 | uint16(image[17])
	s.CPU.HL_ = uint16(image[20])<<8 | uint16(image[19])
	s.CPU.AF_ = uint16(image[21])<<8 | uint16(image[22])
	s.CPU.IY = binary.LittleEndian.Uint16(image[23:])
	s.CPU.IX = binary.LittleEndian.Uint16(image[25:])
	s.CPU.IFF1 = image[27] != 0
	s.CPU.IFF2 = image[28] != 0
	s.CPU.IM = image[29] & 0x03

	// Extended header: real PC, hardware mode, paging.
	s.CPU.PC = binary.LittleEndian.Uint16(image[32:])
	hwMode := image[34]
	is128K := hwIs128K(version, hwMode)
	if is128K {
		s.Model = Model128K
		s.Paging.Port7FFD = image[35]
		if extLen >= 24 {
			s.Paging.Port1FFD = image[36]
		}
	} else {
		s.Model = Model48K
	}

	// Memory blocks.
	pos := headerEnd
	for pos+3 <= len(image) {
		blockLen := int(binary.LittleEndian.Uint16(image[pos:]))
		page := image[pos+2]
		pos += 3

		var raw []byte
		if blockLen == 0xFFFF {
			// Uncompressed 16 KiB.
			if pos+16384 > len(image) {
				return nil, fmt.Errorf("uncompressed page %d runs past end", page)
			}
			raw = image[pos : pos+16384]
			pos += 16384
		} else {
			if pos+blockLen > len(image) {
				return nil, fmt.Errorf("page %d block (%d bytes) runs past end", page, blockLen)
			}
			raw = decompressZ80(image[pos : pos+blockLen])
			pos += blockLen
		}
		if len(raw) != 16384 {
			return nil, fmt.Errorf("page %d decompressed to %d bytes, want 16384", page, len(raw))
		}
		if bank, ok := pageToBank(page, is128K); ok {
			copy(s.Memory.RAM[bank][:], raw)
		}
		// Pages that don't map to RAM (ROM) are silently skipped.
	}

	return s, nil
}

// EncodeZ80v3 encodes a MachineState as a v3 .z80 image, using the extended
// header and per-page compressed memory blocks. Works for both 48K and 128K
// states. This is the format modern tools prefer; the v1 EncodeZ80 remains for
// simple 48K output.
func EncodeZ80v3(s *MachineState) ([]byte, error) {
	const extLen = 54 // v3
	hdr := make([]byte, 32+extLen)

	hdr[0] = byte(s.CPU.AF >> 8)
	hdr[1] = byte(s.CPU.AF & 0xFF)
	binary.LittleEndian.PutUint16(hdr[2:], s.CPU.BC)
	binary.LittleEndian.PutUint16(hdr[4:], s.CPU.HL)
	// hdr[6:8] PC = 0 signals extended header.
	binary.LittleEndian.PutUint16(hdr[8:], s.CPU.SP)
	hdr[10] = s.CPU.I
	hdr[11] = s.CPU.R & 0x7F
	var flags1 byte
	if s.CPU.R&0x80 != 0 {
		flags1 |= 0x01
	}
	flags1 |= (s.IO.Border & 0x07) << 1
	hdr[12] = flags1
	hdr[13] = byte(s.CPU.DE & 0xFF)
	hdr[14] = byte(s.CPU.DE >> 8)
	hdr[15] = byte(s.CPU.BC_ & 0xFF)
	hdr[16] = byte(s.CPU.BC_ >> 8)
	hdr[17] = byte(s.CPU.DE_ & 0xFF)
	hdr[18] = byte(s.CPU.DE_ >> 8)
	hdr[19] = byte(s.CPU.HL_ & 0xFF)
	hdr[20] = byte(s.CPU.HL_ >> 8)
	hdr[21] = byte(s.CPU.AF_ >> 8)
	hdr[22] = byte(s.CPU.AF_ & 0xFF)
	binary.LittleEndian.PutUint16(hdr[23:], s.CPU.IY)
	binary.LittleEndian.PutUint16(hdr[25:], s.CPU.IX)
	if s.CPU.IFF1 {
		hdr[27] = 1
	}
	if s.CPU.IFF2 {
		hdr[28] = 1
	}
	hdr[29] = s.CPU.IM & 0x03

	binary.LittleEndian.PutUint16(hdr[30:], extLen)
	binary.LittleEndian.PutUint16(hdr[32:], s.CPU.PC)
	if s.Model.Is128KFamily() {
		hdr[34] = 4 // hardware mode 4 = 128K
		hdr[35] = s.Paging.Port7FFD
		hdr[36] = s.Paging.Port1FFD
	} else {
		hdr[34] = 0 // 48K
	}

	out := hdr

	writePage := func(page byte, bank int) {
		comp := compressZ80(s.Memory.RAM[bank][:])
		// If compression did not shrink it, store uncompressed (length 0xFFFF).
		if len(comp) >= 16384 {
			out = appendU16le(out, 0xFFFF)
			out = append(out, page)
			out = append(out, s.Memory.RAM[bank][:]...)
		} else {
			out = appendU16le(out, uint16(len(comp)))
			out = append(out, page)
			out = append(out, comp...)
		}
	}

	if s.Model.Is128KFamily() {
		// All eight banks, pages 3..10 = banks 0..7.
		for bank := 0; bank < 8; bank++ {
			writePage(byte(bank+3), bank)
		}
	} else {
		// 48K: pages 8,4,5 = banks 5,2,0.
		writePage(8, 5)
		writePage(4, 2)
		writePage(5, 0)
	}
	return out, nil
}
