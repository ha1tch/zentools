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
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	signature = "ZXTape!"
	eofMarker = 0x1A
	majorVer  = 1
	minorVer  = 20

	idStandardSpeed = 0x10
	idTurboSpeed    = 0x11
	idPureTone      = 0x12
	idPulseSeq      = 0x13
	idPureData      = 0x14
	idDirectRec     = 0x15
	idC64ROM        = 0x16
	idC64Turbo      = 0x17
	idCSWRecording  = 0x18
	idGeneralized   = 0x19
	idPause         = 0x20
	idJumpBlock     = 0x23
	idLoopStart     = 0x24
	idLoopEnd       = 0x25
	idCallSeq       = 0x26
	idReturnSeq     = 0x27
	idSelectBlock   = 0x28
	idSetSignalLvl  = 0x2B
	idTextDesc      = 0x30
	idMessage       = 0x31
	idArchiveInfo   = 0x32
	idStopThe48K    = 0x2A
	idCustomInfo    = 0x35
	idGlue          = 0x5A

	// defaultPause is the pause after a block, in milliseconds, that a normal
	// loading scheme expects between header and data.
	defaultPause = 1000
)

// EncodeOptions carries optional metadata written ahead of the tape data.
// All fields are optional; the zero value produces a bare, valid TZX file.
type EncodeOptions struct {
	Title       string         // archive-info title (0x32)
	Author      string         // archive-info author
	Year        string         // archive-info year
	Description string         // a text-description block (0x30)
	StopIn48K   bool           // emit a "stop the tape if in 48K mode" block (0x2A)
	Pause       uint16         // pause after each data block, ms (0 uses the default)
	Hardware    []HardwareInfo // hardware-type block (0x33); empty means none
	Group       string         // if set, the data blocks are bracketed in a named group (0x21/0x22)
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
	w := NewWriter(opts.Pause)
	w.ArchiveInfo(opts)
	w.Description(opts.Description)
	w.Hardware(opts.Hardware)
	// A group brackets the data blocks: start before, end after. Groups cannot
	// nest, and every start needs an end; encoding a single group around the
	// data here satisfies both rules by construction.
	if opts.Group != "" {
		w.GroupStart(opts.Group)
	}
	if err := w.AddTAP(tapImage); err != nil {
		return nil, err
	}
	if opts.Group != "" {
		w.GroupEnd()
	}
	if opts.StopIn48K {
		w.StopIn48K()
	}
	return w.Bytes(), nil
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
	Pause uint16 // for 0x10 (pause after block) and 0x20 (the pause/stop value itself, reported as-is -- see Decode's own doc comment on 0x20 for why zero is not special-cased here)
	Data  []byte // for 0x10: the wrapped TAP block bytes

	// PilotPulse and PilotLength describe a 0x12 (Pure Tone) block: a
	// pilot tone of PilotLength pulses, each PilotPulse T-states long.
	PilotPulse  uint16 // for 0x12
	PilotLength uint16 // for 0x12

	// Pulses holds a 0x13 (Sequence Of Pulses) block's literal,
	// individually specified pulse durations, in T-states, in order.
	Pulses []int // for 0x13

	// SyncPulse1, SyncPulse2, DataPulse0, DataPulse1, and BitsInLastByte
	// describe a 0x11 (Turbo Speed Data) block's custom timing: its own
	// pilot tone (PilotPulse/PilotLength, shared fields with 0x12 above,
	// same meaning), its own two sync pulses, its own two data-bit pulse
	// lengths (0 vs 1), and how many bits of the final data byte are
	// actually significant (a partial last byte, not always a full 8).
	// Data holds the raw payload bytes themselves, the same field 0x10
	// uses for its own payload.
	SyncPulse1     uint16 // for 0x11
	SyncPulse2     uint16 // for 0x11
	DataPulse0     uint16 // for 0x11 and 0x14 (Pure Data reuses these two)
	DataPulse1     uint16 // for 0x11 and 0x14
	BitsInLastByte uint8  // for 0x11, 0x14, and 0x15

	// SampleStep is 0x15 (Direct Recording)'s own sample-length unit, in
	// T-states: consecutive same-level bits accumulate SampleStep each,
	// with a pulse emitted on every level change -- run-length encoding
	// of a bitstream, not a fixed per-bit pulse pair the way 0x10/0x11/
	// 0x14 work. Genuine pulse expansion for this block type is left to
	// a caller (the same separation of concerns as everywhere else in
	// this package), so only the raw fields are reported here.
	SampleStep uint16 // for 0x15

	// SignalLevel is 0x2B (Set Signal Level)'s requested EAR level: true
	// for high, false for low. Interpreting it (nudging the pulse stream
	// to actually reach that level) is a pulse-expansion concern and a
	// caller's job, matching zenzx's own polarity-reset handling for
	// 0x20 in tape_types.go.
	SignalLevel bool // for 0x2B

	// The following six fields describe a 0x19 (Generalized Data Block)'s
	// structure: an "alphabet" of named pulse-symbols (separate for the
	// pilot and data portions), referenced by index from a pilot stream
	// (byte-aligned) and a data stream (bit-packed, DataBitsPerSymbol
	// bits per symbol). This is confirmed the most structurally complex
	// block in the whole format -- even SpecIde's own source treats it
	// as effectively future-proofing (it is exceptionally rare in
	// real-world files; ZEsarUX does not implement it at all). This
	// package parses the structure precisely enough to always advance
	// past a 0x19 block correctly, matching SpecIde's own byte-counting
	// exactly, but deliberately stops short of alphabet-driven pulse
	// expansion (decoding AlphabetData into actual T-state pulses) --
	// SpecIde's own pushSymbol has real polarity-continuation logic
	// (its "Continue"/"Force low"/"Force high" symbol types) that is
	// genuinely easy to get subtly wrong, and deserves its own dedicated
	// verification pass rather than being rushed alongside everything
	// else. AlphabetData holds the raw, still-undecoded bytes covering
	// both alphabet tables and both symbol streams, in file order, for
	// a future caller to finish decoding.
	SymbolsInPilot    uint32 // for 0x19
	MaxPilotSymLen    uint32 // for 0x19
	PilotAlphabetSize uint32 // for 0x19
	SymbolsInData     uint32 // for 0x19
	MaxDataSymLen     uint32 // for 0x19
	DataAlphabetSize  uint32 // for 0x19
	AlphabetData      []byte // for 0x19: raw, undecoded alphabet tables + symbol streams
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

	// Loop Start/End (0x24/0x25) state. TZX loops do not nest -- SpecIde
	// itself tracks only a single loopStart/loopCounter pair, and this
	// package matches that for behavioural parity. loopActive distinguishes
	// "no loop in progress" from "a loop with counter 0 in progress" (the
	// zero value of loopCounter alone cannot, since a legitimately-just-
	// finished loop also has counter 0).
	var loopActive bool
	var loopStart int
	var loopCounter int

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
		case idTurboSpeed:
			// SpecIde header length 0x13 (19, including the ID byte its
			// own table counts) = id(1) + pilotPulse(2) + syncPulse1(2)
			// + syncPulse2(2) + dataPulse0(2) + dataPulse1(2) +
			// pilotLength(2) + bitsInLastByte(1) + pause(2) +
			// dataLength(3) -- 18 bytes after this package's pos has
			// already consumed the ID byte, then dataLength bytes of
			// payload.
			if pos+18 > len(image) {
				return nil, errors.New("truncated 0x11 block header")
			}
			pilotPulse := binary.LittleEndian.Uint16(image[pos : pos+2])
			syncPulse1 := binary.LittleEndian.Uint16(image[pos+2 : pos+4])
			syncPulse2 := binary.LittleEndian.Uint16(image[pos+4 : pos+6])
			dataPulse0 := binary.LittleEndian.Uint16(image[pos+6 : pos+8])
			dataPulse1 := binary.LittleEndian.Uint16(image[pos+8 : pos+10])
			pilotLength := binary.LittleEndian.Uint16(image[pos+10 : pos+12])
			bitsInLastByte := image[pos+12]
			pause := binary.LittleEndian.Uint16(image[pos+13 : pos+15])
			n := int(image[pos+15]) | int(image[pos+16])<<8 | int(image[pos+17])<<16
			pos += 18
			if pos+n > len(image) {
				return nil, errors.New("truncated 0x11 block data")
			}
			blocks = append(blocks, Block{
				ID:             id,
				PilotPulse:     pilotPulse,
				PilotLength:    pilotLength,
				SyncPulse1:     syncPulse1,
				SyncPulse2:     syncPulse2,
				DataPulse0:     dataPulse0,
				DataPulse1:     dataPulse1,
				BitsInLastByte: bitsInLastByte,
				Pause:          pause,
				Data:           image[pos : pos+n],
			})
			pos += n
		case idPureTone:
			// SpecIde header length 0x05 = id(1) + pilotPulse(2) +
			// pilotLength(2); no further payload.
			if pos+4 > len(image) {
				return nil, errors.New("truncated 0x12 block")
			}
			pilotPulse := binary.LittleEndian.Uint16(image[pos : pos+2])
			pilotLength := binary.LittleEndian.Uint16(image[pos+2 : pos+4])
			pos += 4
			blocks = append(blocks, Block{ID: id, PilotPulse: pilotPulse, PilotLength: pilotLength})
		case idPulseSeq:
			// SpecIde header length 0x02 = id(1) + count(1); count*2
			// bytes of individual u16 pulse durations follow.
			if pos >= len(image) {
				return nil, errors.New("truncated 0x13 block")
			}
			n := int(image[pos])
			pos++
			if pos+2*n > len(image) {
				return nil, errors.New("truncated 0x13 block data")
			}
			pulses := make([]int, n)
			for i := 0; i < n; i++ {
				pulses[i] = int(binary.LittleEndian.Uint16(image[pos+2*i : pos+2*i+2]))
			}
			pos += 2 * n
			blocks = append(blocks, Block{ID: id, Pulses: pulses})
		case idPause:
			// SpecIde header length 0x03 = id(1) + pause(2). A pause of
			// zero is TZX's own "stop the tape" signal -- a genuine,
			// distinct meaning from "no pause", used by real multi-part
			// loaders to mark an intentional halt point -- not something
			// this decode step drops or special-cases silently. It is
			// reported via Pause exactly like any other value; a caller
			// expanding blocks into pulse timings (as zenzx's own
			// tape_types.go does, ported from this same SpecIde source)
			// is responsible for treating zero specially, the same
			// separation of concerns this package already has between
			// parsing block structure and generating pulses for 0x10.
			if pos+2 > len(image) {
				return nil, errors.New("truncated 0x20 block")
			}
			pause := binary.LittleEndian.Uint16(image[pos : pos+2])
			pos += 2
			blocks = append(blocks, Block{ID: id, Pause: pause})
		case idPureData:
			// SpecIde header length 0x0B (11) = id(1) + dataPulse0(2) +
			// dataPulse1(2) + bitsInLastByte(1) + pause(2) +
			// dataLength(3) -- 10 bytes after this package's pos has
			// consumed the ID, then dataLength bytes of payload. Same
			// shape as 0x11 minus the pilot/sync fields -- used for a
			// continuation block after a separate 0x12/0x13 pilot/sync,
			// reusing the same DataPulse0/DataPulse1/BitsInLastByte/
			// Pause/Data fields 0x11 already has.
			if pos+10 > len(image) {
				return nil, errors.New("truncated 0x14 block header")
			}
			dataPulse0 := binary.LittleEndian.Uint16(image[pos : pos+2])
			dataPulse1 := binary.LittleEndian.Uint16(image[pos+2 : pos+4])
			bitsInLastByte := image[pos+4]
			pause := binary.LittleEndian.Uint16(image[pos+5 : pos+7])
			n := int(image[pos+7]) | int(image[pos+8])<<8 | int(image[pos+9])<<16
			pos += 10
			if pos+n > len(image) {
				return nil, errors.New("truncated 0x14 block data")
			}
			blocks = append(blocks, Block{
				ID:             id,
				DataPulse0:     dataPulse0,
				DataPulse1:     dataPulse1,
				BitsInLastByte: bitsInLastByte,
				Pause:          pause,
				Data:           image[pos : pos+n],
			})
			pos += n
		case idDirectRec:
			// SpecIde header length 0x09 (9) = id(1) + sampleStep(2) +
			// pause(2) + bitsInLastByte(1) + dataLength(3) -- 8 bytes
			// after the ID, then dataLength bytes of raw sample data.
			// Pulse expansion (run-length encoding the bitstream against
			// sampleStep) is left to a caller -- see SampleStep's own
			// doc comment on Block.
			if pos+8 > len(image) {
				return nil, errors.New("truncated 0x15 block header")
			}
			sampleStep := binary.LittleEndian.Uint16(image[pos : pos+2])
			pause := binary.LittleEndian.Uint16(image[pos+2 : pos+4])
			bitsInLastByte := image[pos+4]
			n := int(image[pos+5]) | int(image[pos+6])<<8 | int(image[pos+7])<<16
			pos += 8
			if pos+n > len(image) {
				return nil, errors.New("truncated 0x15 block data")
			}
			blocks = append(blocks, Block{
				ID:             id,
				SampleStep:     sampleStep,
				BitsInLastByte: bitsInLastByte,
				Pause:          pause,
				Data:           image[pos : pos+n],
			})
			pos += n
		case idCSWRecording:
			// SpecIde header length 0x05 = id(1) + dataLength_u32(4).
			// Unlike every other block in this package, dataLength here
			// covers a 10-byte sub-header (pause, cswRate, compression,
			// expectedPulses) *plus* the CSW pulse data itself, not
			// just the pulse data alone -- confirmed directly from
			// SpecIde's own slice construction, which subtracts 10 from
			// dataLength to get the CSW buffer's own length.
			if pos+4 > len(image) {
				return nil, errors.New("truncated 0x18 block header")
			}
			dataLength := int(binary.LittleEndian.Uint32(image[pos : pos+4]))
			pos += 4
			if pos+dataLength > len(image) {
				return nil, errors.New("truncated 0x18 block data")
			}
			if dataLength < 10 {
				return nil, errors.New("0x18 block dataLength too short for its own sub-header")
			}
			pause := binary.LittleEndian.Uint16(image[pos : pos+2])
			cswRate := uint32(image[pos+2]) | uint32(image[pos+3])<<8 | uint32(image[pos+4])<<16
			compression := image[pos+5]
			cswBytes := image[pos+10 : pos+dataLength]
			pos += dataLength

			if cswRate == 0 {
				return nil, errors.New("0x18 block has a zero CSW sample rate")
			}

			// Compression type 2 is confirmed (SpecIde's CSWFile.cc:
			// "Using ZLIB+RLE compression") to be standard zlib, not a
			// custom variant -- Go's stdlib compress/zlib is the exact
			// right tool, no external dependency needed (unlike SpecIde
			// itself, which links the real system zlib C library here).
			if compression == 2 {
				r, err := zlib.NewReader(bytes.NewReader(cswBytes))
				if err != nil {
					return nil, fmt.Errorf("0x18 block: zlib init: %w", err)
				}
				inflated, err := io.ReadAll(r)
				if err != nil {
					return nil, fmt.Errorf("0x18 block: zlib inflate: %w", err)
				}
				cswBytes = inflated
			}

			// CSW pulse-stream encoding: each byte is a direct 1-byte
			// pulse value, except 0x00, which signals an "extended"
			// pulse -- the following 4 bytes (little-endian u32) are
			// the real value instead. Every raw sample count is then
			// rescaled from CSW's own sample rate to Spectrum T-states.
			var pulses []int
			for i := 0; i < len(cswBytes); i++ {
				var raw uint32
				if cswBytes[i] == 0x00 {
					if i+4 >= len(cswBytes) {
						return nil, errors.New("0x18 block: truncated extended CSW pulse value")
					}
					raw = binary.LittleEndian.Uint32(cswBytes[i+1 : i+5])
					i += 4
				} else {
					raw = uint32(cswBytes[i])
				}
				pulse := int(float64(raw) * 3500000.0 / float64(cswRate))
				pulses = append(pulses, pulse)
			}

			blocks = append(blocks, Block{ID: id, Pause: pause, Pulses: pulses})
		case idGeneralized:
			// SpecIde header up to the alphabet/stream tables ("table
			// Index") is a fixed 19 bytes = id(1) + dataLength_u32(4) +
			// pause(2) + [pilot: numSym_u32(4)+maxLen(1)+alphaSize(1)]
			// + [data: numSym_u32(4)+maxLen(1)+alphaSize(1)] -- 18
			// bytes after this package's pos has consumed the ID.
			if pos+18 > len(image) {
				return nil, errors.New("truncated 0x19 block header")
			}
			dataLength := int(binary.LittleEndian.Uint32(image[pos : pos+4]))
			pause := binary.LittleEndian.Uint16(image[pos+4 : pos+6])
			symbolsInPilot := binary.LittleEndian.Uint32(image[pos+6 : pos+10])
			maxPilotSymLen := uint32(image[pos+10])
			pilotAlphabetSize := uint32(image[pos+11])
			if pilotAlphabetSize == 0 {
				pilotAlphabetSize = 0x100 // SpecIde: "if (0 == alphaSize) alphaSize = 0x100"
			}
			symbolsInData := binary.LittleEndian.Uint32(image[pos+12 : pos+16])
			maxDataSymLen := uint32(image[pos+16])
			dataAlphabetSize := uint32(image[pos+17])
			if dataAlphabetSize == 0 {
				dataAlphabetSize = 0x100
			}
			tablesStart := pos + 18

			// Walk the alphabet tables and symbol streams to compute
			// their real combined length independently, rather than
			// trusting dataLength blindly -- matching the spirit of
			// SpecIde's own "assert(pointer == tableIndex)" sanity
			// check (a debug-only assert there; a real, returned error
			// here, since malformed input should not be UB).
			tableLen := 0
			if symbolsInPilot > 0 {
				tableLen += int(pilotAlphabetSize) * (1 + 2*int(maxPilotSymLen))
				tableLen += int(symbolsInPilot) * 3 // dumpPilotStream: 1-byte symbol + 2-byte repeat, per entry
			}
			if symbolsInData > 0 {
				tableLen += int(dataAlphabetSize) * (1 + 2*int(maxDataSymLen))
				bps := int(math.Ceil(math.Log2(float64(dataAlphabetSize))))
				if bps < 1 {
					bps = 1
				}
				totalBits := int(symbolsInData) * bps
				tableLen += (totalBits + 7) / 8 // dumpDataStream: bit-packed, byte-ceiling
			}

			if tablesStart+tableLen > len(image) {
				return nil, errors.New("truncated 0x19 block: alphabet/stream tables run past end of image")
			}
			wantLen := tableLen + 14 // tableIndex(19) - pos-relative-start(pos, i.e. -1 for id) - dataLength/pause fields(6) = 14; see comment below
			if dataLength != wantLen {
				return nil, fmt.Errorf(
					"0x19 block: declared dataLength (%d) does not match the alphabet/stream tables' own computed length (%d) -- likely corrupt",
					dataLength, wantLen)
			}

			blocks = append(blocks, Block{
				ID:                id,
				Pause:             pause,
				SymbolsInPilot:    symbolsInPilot,
				MaxPilotSymLen:    maxPilotSymLen,
				PilotAlphabetSize: pilotAlphabetSize,
				SymbolsInData:     symbolsInData,
				MaxDataSymLen:     maxDataSymLen,
				DataAlphabetSize:  dataAlphabetSize,
				AlphabetData:      image[tablesStart : tablesStart+tableLen],
			})
			pos = tablesStart + tableLen
		case idC64ROM, idC64Turbo:
			// Both deprecated, never emitted by any real Spectrum tool.
			// SpecIde header length 0x05 (5) for both = id(1) +
			// dataLength_u32(4); recognised and safely skipped rather
			// than causing a hard decode failure on the rare file that
			// still has one, matching SpecIde's own comment ("Skipped
			// for the moment") for 0x17 specifically.
			if pos+4 > len(image) {
				return nil, errors.New("truncated 0x16/0x17 block header")
			}
			n := int(binary.LittleEndian.Uint32(image[pos : pos+4]))
			pos += 4
			if pos+n > len(image) {
				return nil, errors.New("truncated 0x16/0x17 block data")
			}
			pos += n
			blocks = append(blocks, Block{ID: id})
		case idLoopStart:
			// SpecIde header length 0x03 = id(1) + count_u16(2). Records
			// where the loop body begins (right after this header) and
			// how many total times it should run -- see this test
			// file's own loop_test.go doc comment for the exact
			// counter semantics, traced through SpecIde rather than
			// assumed. TZX loops do not nest (matching SpecIde): a
			// Loop Start while one is already active simply overwrites
			// the previous loopStart/loopCounter, the same behaviour
			// SpecIde itself has.
			if pos+2 > len(image) {
				return nil, errors.New("truncated 0x24 block")
			}
			loopCounter = int(binary.LittleEndian.Uint16(image[pos : pos+2]))
			pos += 2
			loopStart = pos
			loopActive = true
		case idLoopEnd:
			// No body (SpecIde header length 0x01 = the ID alone).
			// Decrement the counter; if still nonzero, jump back to
			// loopStart to run the body again; once it reaches zero,
			// just fall through. A Loop End with no active loop (a
			// malformed file, or one this package's non-nesting model
			// cannot represent) is treated as a safe no-op rather than
			// replicating the unsigned-underflow hang SpecIde's own C++
			// would suffer here (--loopCounter on an already-zero
			// unsigned counter wraps to a huge value, not an error) --
			// an intentional, documented safety improvement, not an
			// unexamined behavioural difference.
			if loopActive {
				loopCounter--
				if loopCounter > 0 {
					pos = loopStart
					continue
				}
				loopActive = false
			}
		case idJumpBlock:
			// "Not implemented yet" in SpecIde's own source -- matched
			// here at the same fidelity: recognised and safely skipped,
			// not a claim of real relative-jump semantics neither
			// implementation actually has. SpecIde header length 0x03 =
			// id(1) + relative offset(2).
			if pos+2 > len(image) {
				return nil, errors.New("truncated 0x23 block")
			}
			pos += 2
			blocks = append(blocks, Block{ID: id})
		case idCallSeq:
			// "Not implemented yet" in SpecIde -- same fidelity as
			// idJumpBlock above. SpecIde's own skip formula: pointer +=
			// 2*count + 3, i.e. (in this package's pos convention,
			// already past the ID) 2 bytes for count plus count*2 bytes
			// of call-offset entries.
			if pos+2 > len(image) {
				return nil, errors.New("truncated 0x26 block header")
			}
			count := int(binary.LittleEndian.Uint16(image[pos : pos+2]))
			pos += 2
			if pos+2*count > len(image) {
				return nil, errors.New("truncated 0x26 block data")
			}
			pos += 2 * count
			blocks = append(blocks, Block{ID: id})
		case idReturnSeq:
			// "Not implemented yet" in SpecIde -- same fidelity as
			// idJumpBlock above. No body at all: SpecIde header length
			// 0x01 = the ID byte alone.
			blocks = append(blocks, Block{ID: id})
		case idSelectBlock:
			// SpecIde's own skip formula: pointer += getU16(pointer+1) +
			// 3, i.e. (in this package's pos convention) 2 bytes for the
			// length field itself plus n more bytes of selection data.
			if pos+2 > len(image) {
				return nil, errors.New("truncated 0x28 block header")
			}
			n := int(binary.LittleEndian.Uint16(image[pos : pos+2]))
			pos += 2
			if pos+n > len(image) {
				return nil, errors.New("truncated 0x28 block data")
			}
			pos += n
			blocks = append(blocks, Block{ID: id})
		case idSetSignalLvl:
			// SpecIde header length 0x06 (6) = id(1) + blockLength_u32(4,
			// always 1, effectively fixed) + level_u8(1) -- 5 bytes
			// after the ID. Interpreting the level (nudging the pulse
			// stream to reach it) is a caller's job -- see SignalLevel's
			// own doc comment on Block.
			if pos+5 > len(image) {
				return nil, errors.New("truncated 0x2B block")
			}
			level := image[pos+4] != 0
			pos += 5
			blocks = append(blocks, Block{ID: id, SignalLevel: level})
		case idMessage:
			// Structurally identical to idTextDesc (0x30) but with an
			// extra leading "seconds to display" byte. SpecIde header
			// length 0x03 = id(1) + displayTime(1) + textLength(1), then
			// textLength bytes of text (not retained, matching
			// idTextDesc's own existing precedent of not storing text).
			if pos+2 > len(image) {
				return nil, errors.New("truncated 0x31 block header")
			}
			n := int(image[pos+1])
			pos += 2 + n
			blocks = append(blocks, Block{ID: id})
		case idCustomInfo:
			// SpecIde header length 0x15 (21) = id(1) + identifier(16) +
			// dataLength_u32(4) -- 20 bytes after the ID, then
			// dataLength bytes of payload (not retained; this block is
			// tool-specific and its content has no standard meaning).
			if pos+20 > len(image) {
				return nil, errors.New("truncated 0x35 block header")
			}
			n := int(binary.LittleEndian.Uint32(image[pos+16 : pos+20]))
			pos += 20
			if pos+n > len(image) {
				return nil, errors.New("truncated 0x35 block data")
			}
			pos += n
			blocks = append(blocks, Block{ID: id})
		case idGlue:
			// Fixed 9 bytes after the ID -- the historical trick that
			// lets two TZX files be concatenated byte-for-byte: the ID
			// (0x5A, ASCII 'Z') plus its own 9 data bytes spell out
			// "ZXTape!" plus version bytes again. SpecIde header length
			// 0x0A (10) = id(1) + 9 fixed bytes.
			if pos+9 > len(image) {
				return nil, errors.New("truncated 0x5A block")
			}
			pos += 9
			blocks = append(blocks, Block{ID: id})
		case idArchiveInfo:
			if pos+2 > len(image) {
				return nil, errors.New("truncated 0x32 block")
			}
			n := int(binary.LittleEndian.Uint16(image[pos : pos+2]))
			if pos+2+n > len(image) {
				return nil, errors.New("truncated 0x32 block data")
			}
			pos += 2 + n
			blocks = append(blocks, Block{ID: id})
		case idTextDesc:
			if pos >= len(image) {
				return nil, errors.New("truncated 0x30 block")
			}
			n := int(image[pos])
			if pos+1+n > len(image) {
				return nil, errors.New("truncated 0x30 block data")
			}
			pos += 1 + n
			blocks = append(blocks, Block{ID: id})
		case idStopThe48K:
			if pos+4 > len(image) {
				return nil, errors.New("truncated 0x2A block")
			}
			pos += 4 // 4-byte length field, always zero
			blocks = append(blocks, Block{ID: id})
		case idHardwareTyp:
			if pos >= len(image) {
				return nil, errors.New("truncated 0x33 block")
			}
			n := int(image[pos]) // number of HWINFO entries
			if pos+1+n*3 > len(image) {
				return nil, errors.New("truncated 0x33 block data")
			}
			pos += 1 + n*3 // each entry is 3 bytes
			blocks = append(blocks, Block{ID: id})
		case idGroupStart:
			if pos >= len(image) {
				return nil, errors.New("truncated 0x21 block")
			}
			n := int(image[pos]) // group-name length
			if pos+1+n > len(image) {
				return nil, errors.New("truncated 0x21 block data")
			}
			pos += 1 + n
			blocks = append(blocks, Block{ID: id})
		case idGroupEnd:
			// No body.
			blocks = append(blocks, Block{ID: id})
		default:
			return blocks, fmt.Errorf("unsupported TZX block id 0x%02X at offset %d", id, pos-1)
		}
	}
	return blocks, nil
}
