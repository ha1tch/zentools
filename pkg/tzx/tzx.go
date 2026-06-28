// Package tzx reads and writes ZX Spectrum TZX tape images.
//
// TZX is a richer tape container than TAP: a signature and version header
// followed by typed blocks. The most common block, 0x10 (standard-speed data),
// carries the same payload a TAP block would, so a TAP image converts to TZX by
// wrapping each of its blocks in a 0x10 block.
//
// This package works in memory and depends only on the standard library. The
// metadata that a command-line tool might read from a YAML config (titles,
// hardware flags) is passed in as a plain EncodeOptions struct; parsing config
// files is the caller's job, not this package's.
package tzx

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	signature = "ZXTape!"
	eofMarker = 0x1A
	majorVer  = 1
	minorVer  = 20

	idStandardSpeed = 0x10
	idTextDesc      = 0x30
	idArchiveInfo   = 0x32
	idStopThe48K    = 0x2A

	// defaultPause is the pause after a block, in milliseconds, that a normal
	// loading scheme expects between header and data.
	defaultPause = 1000
)

// EncodeOptions carries optional metadata written ahead of the tape data.
// All fields are optional; the zero value produces a bare, valid TZX file.
type EncodeOptions struct {
	Title       string // archive-info title (0x32)
	Author      string // archive-info author
	Year        string // archive-info year
	Description string // a text-description block (0x30)
	StopIn48K   bool   // emit a "stop the tape if in 48K mode" block (0x2A)
	Pause       uint16 // pause after each data block, ms (0 uses the default)
}

// rawTAPBlock is the minimal view of a TAP block this package needs: the full
// block bytes (flag + payload + checksum) that go verbatim into a 0x10 block.
// Callers usually obtain these from tap.Decode and adapt; EncodeFromTAP does
// that adaptation itself so callers can pass a TAP image directly.
type rawTAPBlock []byte

func appendU16(b []byte, v uint16) []byte {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	return append(b, buf[:]...)
}

// header returns the TZX signature, EOF marker, and version bytes.
func header() []byte {
	out := make([]byte, 0, 10)
	out = append(out, []byte(signature)...)
	out = append(out, eofMarker, majorVer, minorVer)
	return out
}

// standardSpeedBlock wraps a full TAP block (flag+payload+checksum) in a 0x10
// block with the given pause.
func standardSpeedBlock(block []byte, pause uint16) []byte {
	out := make([]byte, 0, len(block)+5)
	out = append(out, idStandardSpeed)
	out = appendU16(out, pause)
	out = appendU16(out, uint16(len(block)))
	out = append(out, block...)
	return out
}

// archiveInfoBlock builds a 0x32 archive-info block from the title/author/year.
// Returns nil if there is nothing to write.
func archiveInfoBlock(opts EncodeOptions) []byte {
	type field struct {
		id   byte
		text string
	}
	var fields []field
	if opts.Title != "" {
		fields = append(fields, field{0x00, opts.Title})
	}
	if opts.Author != "" {
		fields = append(fields, field{0x01, opts.Author})
	}
	if opts.Year != "" {
		fields = append(fields, field{0x02, opts.Year})
	}
	if len(fields) == 0 {
		return nil
	}
	// Body: number-of-strings, then for each: id, length, text.
	body := []byte{byte(len(fields))}
	for _, f := range fields {
		body = append(body, f.id, byte(len(f.text)))
		body = append(body, []byte(f.text)...)
	}
	out := []byte{idArchiveInfo}
	out = appendU16(out, uint16(len(body)))
	return append(out, body...)
}

// textDescriptionBlock builds a 0x30 text-description block.
func textDescriptionBlock(desc string) []byte {
	if desc == "" {
		return nil
	}
	if len(desc) > 255 {
		desc = desc[:255]
	}
	out := []byte{idTextDesc, byte(len(desc))}
	return append(out, []byte(desc)...)
}

// EncodeFromTAP converts a TAP image into a TZX image, wrapping each TAP block
// in a standard-speed (0x10) block and prepending any requested metadata.
func EncodeFromTAP(tapImage []byte, opts EncodeOptions) ([]byte, error) {
	blocks, err := splitTAP(tapImage)
	if err != nil {
		return nil, err
	}
	pause := opts.Pause
	if pause == 0 {
		pause = defaultPause
	}

	out := header()
	if b := archiveInfoBlock(opts); b != nil {
		out = append(out, b...)
	}
	if b := textDescriptionBlock(opts.Description); b != nil {
		out = append(out, b...)
	}
	for _, blk := range blocks {
		out = append(out, standardSpeedBlock(blk, pause)...)
	}
	if opts.StopIn48K {
		// 0x2A block: ID then a 4-byte length field that is always zero.
		out = append(out, idStopThe48K, 0, 0, 0, 0)
	}
	return out, nil
}

// splitTAP cuts a TAP image into its raw blocks (flag+payload+checksum each),
// without interpreting them. This is the minimal parse EncodeFromTAP needs; the
// tap package's Decode is used by callers who want the parsed fields.
func splitTAP(image []byte) ([]rawTAPBlock, error) {
	var blocks []rawTAPBlock
	pos := 0
	for pos < len(image) {
		if pos+2 > len(image) {
			return nil, fmt.Errorf("truncated TAP block length at offset %d", pos)
		}
		n := int(binary.LittleEndian.Uint16(image[pos : pos+2]))
		pos += 2
		if n < 2 || pos+n > len(image) {
			return nil, fmt.Errorf("TAP block at offset %d has invalid length %d", pos-2, n)
		}
		blocks = append(blocks, rawTAPBlock(image[pos:pos+n]))
		pos += n
	}
	return blocks, nil
}

// Block is a decoded TZX block. Only the fields relevant to the block's ID are
// populated. Data holds the payload of a 0x10 standard-speed block (the full
// flag+payload+checksum bytes).
type Block struct {
	ID    byte
	Pause uint16 // for 0x10
	Data  []byte // for 0x10: the wrapped TAP block bytes
}

// Decode parses a TZX image into its blocks. It validates the signature and
// version header and understands the block IDs this package emits; it skips
// over other known block IDs by their length where it can, and returns an error
// on an unrecognised or malformed block.
func Decode(image []byte) ([]Block, error) {
	if len(image) < 10 {
		return nil, errors.New("too short to be a TZX file")
	}
	if string(image[:7]) != signature || image[7] != eofMarker {
		return nil, errors.New("missing TZX signature")
	}
	pos := 10 // signature(7) + eof(1) + version(2)
	var blocks []Block
	for pos < len(image) {
		id := image[pos]
		pos++
		switch id {
		case idStandardSpeed:
			if pos+4 > len(image) {
				return nil, errors.New("truncated 0x10 block header")
			}
			pause := binary.LittleEndian.Uint16(image[pos : pos+2])
			n := int(binary.LittleEndian.Uint16(image[pos+2 : pos+4]))
			pos += 4
			if pos+n > len(image) {
				return nil, errors.New("truncated 0x10 block data")
			}
			blocks = append(blocks, Block{ID: id, Pause: pause, Data: image[pos : pos+n]})
			pos += n
		case idArchiveInfo:
			if pos+2 > len(image) {
				return nil, errors.New("truncated 0x32 block")
			}
			n := int(binary.LittleEndian.Uint16(image[pos : pos+2]))
			pos += 2 + n
			blocks = append(blocks, Block{ID: id})
		case idTextDesc:
			if pos >= len(image) {
				return nil, errors.New("truncated 0x30 block")
			}
			n := int(image[pos])
			pos += 1 + n
			blocks = append(blocks, Block{ID: id})
		case idStopThe48K:
			pos += 4 // 4-byte length field, always zero
			blocks = append(blocks, Block{ID: id})
		default:
			return blocks, fmt.Errorf("unsupported TZX block id 0x%02X at offset %d", id, pos-1)
		}
	}
	return blocks, nil
}
