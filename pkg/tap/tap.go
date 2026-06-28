// Package tap reads and writes ZX Spectrum TAP files.
//
// A TAP file is a sequence of blocks, each prefixed with a little-endian 16-bit
// length. A standard Spectrum block is a flag byte (0x00 header, 0xFF data)
// followed by the payload and a single XOR checksum byte over the flag and
// payload.
//
// The core of this package works in memory: EncodeCode and EncodeProgram return
// the bytes of a complete TAP file, and the file/stream helpers are thin wrappers
// over them.
package tap

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// headerLength is the byte count of a Spectrum header block's payload
	// (flag + type + 10-char name + 4 params), i.e. the value written as the
	// header block's length field.
	headerLength = 0x13

	flagHeader = 0x00
	flagData   = 0xFF

	// Block (file) types, as stored in a header block's type byte.
	TypeProgram = 0x00
	TypeNumArray = 0x01
	TypeCharArray = 0x02
	TypeCode    = 0x03
)

// xorChecksum returns the XOR of all bytes, the checksum used by Spectrum tape
// blocks.
func xorChecksum(data []byte) byte {
	var c byte
	for _, b := range data {
		c ^= b
	}
	return c
}

// padName returns the filename padded or truncated to exactly 10 bytes with
// trailing spaces, as a header block requires.
func padName(name string) [10]byte {
	var out [10]byte
	for i := range out {
		out[i] = ' '
	}
	copy(out[:], []byte(name))
	return out
}

// headerBlock builds a complete header block (length prefix + payload + checksum).
// param1 and param2 carry type-specific values (see EncodeCode / EncodeProgram).
func headerBlock(blockType byte, name string, dataLength, param1, param2 uint16) []byte {
	nameBytes := padName(name)

	// Payload over which the checksum is computed: flag, type, name, and the
	// three 16-bit parameters.
	payload := make([]byte, 0, headerLength)
	payload = append(payload, flagHeader)
	payload = append(payload, blockType)
	payload = append(payload, nameBytes[:]...)
	payload = appendU16(payload, dataLength)
	payload = appendU16(payload, param1)
	payload = appendU16(payload, param2)

	out := make([]byte, 0, headerLength+3)
	out = appendU16(out, headerLength) // block length prefix
	out = append(out, payload...)
	out = append(out, xorChecksum(payload))
	return out
}

// dataBlock builds a complete data block (length prefix + flag + data + checksum).
func dataBlock(data []byte) []byte {
	blockLen := uint16(len(data) + 2) // flag + checksum

	body := make([]byte, 0, len(data)+1)
	body = append(body, flagData)
	body = append(body, data...)

	out := make([]byte, 0, int(blockLen)+2)
	out = appendU16(out, blockLen)
	out = append(out, body...)
	out = append(out, xorChecksum(body))
	return out
}

func appendU16(b []byte, v uint16) []byte {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	return append(b, buf[:]...)
}

// Block is one decoded TAP block: a header (with its fields parsed) or a data
// block. IsHeader distinguishes them. For a header block the Header fields are
// populated; for a data block Data holds the payload (without the flag or
// checksum) and Flag is the block's flag byte.
type Block struct {
	IsHeader   bool
	Flag       byte   // 0x00 for a standard header, 0xFF for standard data
	Data       []byte // payload between the flag and the checksum
	Checksum   byte   // the stored checksum byte
	ChecksumOK bool   // whether the stored checksum matches the computed one

	// Header fields, valid only when IsHeader is true.
	Type       byte   // TypeProgram / TypeCode / ...
	Name       string // 10-char name, trailing spaces trimmed
	DataLength uint16
	Param1     uint16 // load address (Code) or autostart line (Program)
	Param2     uint16 // 0x8000 (Code) or program length (Program)
}

// Decode parses a TAP image into its blocks. Each block's checksum is verified
// (ChecksumOK), and header blocks have their fields parsed. Decode returns an
// error only for structural problems (truncated length, block past end); a bad
// checksum is reported per-block via ChecksumOK rather than failing the whole
// parse, so a caller can decide how strict to be.
func Decode(image []byte) ([]Block, error) {
	var blocks []Block
	pos := 0
	for pos < len(image) {
		if pos+2 > len(image) {
			return blocks, fmt.Errorf("truncated block length at offset %d", pos)
		}
		blockLen := int(binary.LittleEndian.Uint16(image[pos : pos+2]))
		pos += 2
		if blockLen < 2 {
			return blocks, fmt.Errorf("block at offset %d too short (%d bytes)", pos-2, blockLen)
		}
		if pos+blockLen > len(image) {
			return blocks, fmt.Errorf("block at offset %d claims %d bytes, only %d remain", pos-2, blockLen, len(image)-pos)
		}
		raw := image[pos : pos+blockLen]
		pos += blockLen

		flag := raw[0]
		stored := raw[blockLen-1]
		body := raw[1 : blockLen-1] // between flag and checksum
		calc := xorChecksum(raw[:blockLen-1])

		b := Block{
			Flag:       flag,
			Checksum:   stored,
			ChecksumOK: calc == stored,
			Data:       body,
		}

		// A standard header is flag 0x00 with a 0x13-length block.
		if flag == flagHeader && blockLen == headerLength {
			b.IsHeader = true
			b.Type = body[0]
			b.Name = strings.TrimRight(string(body[1:11]), " ")
			b.DataLength = binary.LittleEndian.Uint16(body[11:13])
			b.Param1 = binary.LittleEndian.Uint16(body[13:15])
			b.Param2 = binary.LittleEndian.Uint16(body[15:17])
		}
		blocks = append(blocks, b)
	}
	return blocks, nil
}

// EncodeCode returns the bytes of a TAP file holding a single CODE block: a
// header describing a byte block loaded at loadAddress, followed by the data.
// This is the in-memory entry point an assembler uses: it has the bytes in hand
// and wants the TAP image back.
//
// The header's param1 is the load address; param2 is 0x8000 by convention for a
// CODE block (the value a real Spectrum SAVE uses).
func EncodeCode(name string, data []byte, loadAddress uint16) []byte {
	out := headerBlock(TypeCode, clampName(name), uint16(len(data)), loadAddress, 0x8000)
	return append(out, dataBlock(data)...)
}

// EncodeProgram returns the bytes of a TAP file holding a BASIC program block.
// autostart is the auto-run line (use a value >= 0x8000, e.g. 0x8000, for "no
// autostart"); param2 is the program length, matching the data length here.
func EncodeProgram(name string, data []byte, autostart uint16) []byte {
	length := uint16(len(data))
	out := headerBlock(TypeProgram, clampName(name), length, autostart, length)
	return append(out, dataBlock(data)...)
}

// clampName trims a name to the 10-character header limit.
func clampName(name string) string {
	if len(name) > 10 {
		return name[:10]
	}
	return name
}

// WriteCodeFile assembles a CODE TAP for the given binary file and writes it to
// outputPath. If name is empty, the input file's base name (without extension)
// is used.
func WriteCodeFile(inputPath, outputPath, name string, loadAddress uint16) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading input file: %w", err)
	}
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	}
	if err := os.WriteFile(outputPath, EncodeCode(name, data, loadAddress), 0o644); err != nil {
		return fmt.Errorf("writing output file: %w", err)
	}
	return nil
}
