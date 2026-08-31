// Package pzx reads and writes PZX (Perfect ZX Tape) tape files, spec v1.0
// (http://zxds.raxoft.cz/docs/pzx.txt), designed by Patrik Rak as a simpler
// replacement for TZX that still represents any pulse-level tape signal.
//
// Unlike TAP/TZX, which are built around named data blocks with headers,
// PZX is built around raw pulse sequences: a PULS block is literally a list
// of (duration, repeat count) pairs, and a DATA block encodes a byte stream
// by mapping each bit to one of two pulse sequences. A standard ROM-saved
// Spectrum file still decodes to something recognisable (see DecodeStdData
// below), but the format itself has no concept of "header block" or
// "load address" the way TAP does -- those are conventions built on top of
// pulse timing, not something PZX's own block structure names directly.
//
// Scope: all four mandatory block types (PZXT, PULS, DATA, PAUS) and the
// two "should be supported" ones (BRWS, STOP) are implemented. Custom
// lowercase-tag extension blocks are preserved as raw bytes on decode and
// re-emitted verbatim, per the spec's own "must skip it and continue"
// rule for anything unrecognised.
package pzx

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Block is implemented by every PZX block type this package models.
type Block interface{ isPZXBlock() }

// HeaderBlock is PZXT, the mandatory first block of any PZX file. Title is
// the first info string, if any; Info holds the following key/value pairs
// in file order (some keys, like Author and Comment, may legitimately
// repeat, which is why this is a slice of pairs rather than a map).
type HeaderBlock struct {
	Major, Minor byte
	Title        string
	Info         []KV
}

// KV is one key/value pair from a PZXT block's info sequence, e.g.
// {"Publisher", "My Software House"}.
type KV struct{ Key, Value string }

func (HeaderBlock) isPZXBlock() {}

// Pulse is one (duration, repeat count) entry of a PULS block.
type Pulse struct {
	Duration uint32 // T cycles, up to 0x7FFFFFFF
	Count    uint16 // repetitions, 1 or more (never zero)
}

// PulseBlock is PULS: an arbitrary sequence of pulses. The level is low at
// the start of the block by default; see the package doc comment and the
// upstream spec for how a zero-duration pulse can invert that.
type PulseBlock struct {
	Pulses []Pulse
}

func (PulseBlock) isPZXBlock() {}

// DataBlock is DATA: a byte stream encoded as a sequence of pulses, two
// bits at a time via S0 (pulses for a 0 bit) and S1 (pulses for a 1 bit),
// most significant bit first. BitCount may be less than 8*len(Data) if the
// stream doesn't end on a byte boundary.
type DataBlock struct {
	InitialLevelHigh bool
	BitCount         uint32
	Tail             uint16   // duration of the extra pulse after the last bit
	S0, S1           []uint16 // pulse durations encoding a 0 bit / a 1 bit
	Data             []byte
}

func (DataBlock) isPZXBlock() {}

// PauseBlock is PAUS: a single pulse of the given duration and level,
// typically used for the gap between a header and data block.
type PauseBlock struct {
	InitialLevelHigh bool
	Duration         uint32
}

func (PauseBlock) isPZXBlock() {}

// BrowseBlock is BRWS: a named point on the tape suitable for a "jump to"
// UI, with a one-line description.
type BrowseBlock struct{ Text string }

func (BrowseBlock) isPZXBlock() {}

// StopBlock is STOP: an instruction to stop the virtual tape deck. Only48K
// corresponds to flags==1 ("stop only if 48K Spectrum is being emulated");
// false means flags==0, always stop.
type StopBlock struct{ Only48K bool }

func (StopBlock) isPZXBlock() {}

// RawBlock preserves any block type this package doesn't otherwise model --
// a custom lowercase-tag extension, or a standardised tag introduced by a
// future spec revision -- so a decode-then-encode round trip doesn't drop
// it, per the spec's own "skip it and continue" rule for the unrecognised
// case.
type RawBlock struct {
	Tag  string // always 4 bytes
	Data []byte
}

func (RawBlock) isPZXBlock() {}

// File is a decoded PZX file: every block in file order. The first is
// always a HeaderBlock (Decode enforces this, per the spec: "This block
// must be always present as the first block of any PZX file").
type File struct {
	Blocks []Block
}

// Decode parses a PZX image into a File.
func Decode(image []byte) (*File, error) {
	pos := 0
	var blocks []Block
	for pos < len(image) {
		if pos+8 > len(image) {
			return nil, fmt.Errorf("pzx: truncated block header at offset %d", pos)
		}
		tag := string(image[pos : pos+4])
		size := binary.LittleEndian.Uint32(image[pos+4:])
		dataStart := pos + 8
		dataEnd := dataStart + int(size)
		if dataEnd > len(image) {
			return nil, fmt.Errorf("pzx: block %q at offset %d claims size %d, past end of file", tag, pos, size)
		}
		data := image[dataStart:dataEnd]

		block, err := decodeBlock(tag, data)
		if err != nil {
			return nil, fmt.Errorf("pzx: block %q at offset %d: %w", tag, pos, err)
		}
		blocks = append(blocks, block)
		pos = dataEnd
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("pzx: empty file, no PZXT header block")
	}
	if _, ok := blocks[0].(HeaderBlock); !ok {
		return nil, fmt.Errorf("pzx: first block is %T, want HeaderBlock (PZXT must be first)", blocks[0])
	}
	return &File{Blocks: blocks}, nil
}

func decodeBlock(tag string, data []byte) (Block, error) {
	switch tag {
	case "PZXT":
		return decodeHeader(data)
	case "PULS":
		return decodePulse(data)
	case "DATA":
		return decodeData(data)
	case "PAUS":
		return decodePause(data)
	case "BRWS":
		return BrowseBlock{Text: string(data)}, nil
	case "STOP":
		if len(data) < 2 {
			return nil, fmt.Errorf("STOP block is %d bytes, want at least 2", len(data))
		}
		flags := binary.LittleEndian.Uint16(data[0:])
		return StopBlock{Only48K: flags == 1}, nil
	default:
		return RawBlock{Tag: tag, Data: append([]byte(nil), data...)}, nil
	}
}

func decodeHeader(data []byte) (Block, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("PZXT block is %d bytes, want at least 2", len(data))
	}
	h := HeaderBlock{Major: data[0], Minor: data[1]}
	strs := splitNulTerminated(data[2:])
	if len(strs) > 0 {
		h.Title = strs[0]
		strs = strs[1:]
	}
	for i := 0; i < len(strs); i += 2 {
		if i+1 < len(strs) {
			h.Info = append(h.Info, KV{Key: strs[i], Value: strs[i+1]})
		} else {
			// Spec: "In case the last value is missing, it should be
			// treated as empty string" -- an odd-length tail key with no
			// following value.
			h.Info = append(h.Info, KV{Key: strs[i], Value: ""})
		}
	}
	return h, nil
}

// splitNulTerminated splits data on 0x00 bytes, per the PZXT spec ("each
// terminated either by character 0x00 or end of the block, whichever comes
// first ... the last string in the sequence may or may not be
// terminated"). An empty trailing segment after a final terminator is not
// emitted as an extra empty string.
func splitNulTerminated(data []byte) []string {
	var out []string
	start := 0
	for i, b := range data {
		if b == 0 {
			out = append(out, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, string(data[start:]))
	}
	return out
}

func encodeHeader(h HeaderBlock) []byte {
	var buf bytes.Buffer
	buf.WriteByte(h.Major)
	buf.WriteByte(h.Minor)
	if h.Title != "" || len(h.Info) > 0 {
		buf.WriteString(h.Title)
		buf.WriteByte(0)
	}
	for _, kv := range h.Info {
		buf.WriteString(kv.Key)
		buf.WriteByte(0)
		buf.WriteString(kv.Value)
		buf.WriteByte(0)
	}
	return withBlockHeader("PZXT", buf.Bytes())
}

// decodePulse parses a PULS block per the spec's own decode pseudocode --
// see pulseDurationHigh's doc comment for why the two threshold checks
// (">" for the count prefix, ">=" for duration extension) are genuinely
// different and both required.
func decodePulse(data []byte) (Block, error) {
	var pulses []Pulse
	pos := 0
	for pos < len(data) {
		count, duration, n, err := decodePulseEntry(data[pos:])
		if err != nil {
			return nil, err
		}
		pulses = append(pulses, Pulse{Duration: duration, Count: count})
		pos += n
	}
	return PulseBlock{Pulses: pulses}, nil
}

// decodePulseEntry decodes one (count, duration) entry from the start of
// data, returning how many bytes it consumed. This implements the spec's
// own pseudocode exactly:
//
//	count = 1
//	duration = fetch_u16()
//	if duration > 0x8000 { count = duration & 0x7FFF; duration = fetch_u16() }
//	if duration >= 0x8000 { duration = ((duration & 0x7FFF) << 16) | fetch_u16() }
//
// The first comparison is strictly-greater, the second is greater-or-equal
// -- not a typo. A first u16 of exactly 0x8000 can never be a valid count
// prefix (the spec: "the stored repeat count must be always greater than
// zero, so ... a value 0x8000 is not a zero repeat count, but prefix
// indicating the presence of extended duration"), so it falls through to
// the second check instead, which then reads a following u16 as the
// duration's low bits with zero high bits contributed. Implemented as
// written, not "corrected", since a decoder for real PZX files must match
// whatever a real encoder actually produces.
func decodePulseEntry(data []byte) (count uint16, duration uint32, consumed int, err error) {
	if len(data) < 2 {
		return 0, 0, 0, fmt.Errorf("PULS: truncated pulse entry")
	}
	count = 1
	d := binary.LittleEndian.Uint16(data[0:])
	pos := 2
	if d > 0x8000 {
		count = d & 0x7FFF
		if len(data) < pos+2 {
			return 0, 0, 0, fmt.Errorf("PULS: truncated duration after repeat count")
		}
		d = binary.LittleEndian.Uint16(data[pos:])
		pos += 2
	}
	duration = uint32(d)
	if d >= 0x8000 {
		if len(data) < pos+2 {
			return 0, 0, 0, fmt.Errorf("PULS: truncated low bits of extended duration")
		}
		low := binary.LittleEndian.Uint16(data[pos:])
		duration = (uint32(d&0x7FFF) << 16) | uint32(low)
		pos += 2
	}
	return count, duration, pos, nil
}

// encodePulseEntry writes one (count, duration) pair in the simplest form
// that decodePulseEntry will read back exactly: a repeat-count prefix is
// written whenever count != 1, and also -- per the spec's own explicit
// warning -- whenever duration needs the full 2-word extended form AND
// its high 15 bits are non-zero (duration > 0xFFFF). In that case the
// extension prefix word itself is 0x8000|highBits with highBits != 0,
// which is > 0x8000, indistinguishable on decode from a genuine
// repeat-count prefix unless an explicit count word (even count==1)
// precedes it. This was found by a round-trip test failing on
// Duration=0x7FFFFFFF, not by reading the spec closely enough the first
// time: "the repeat count must be present unless the duration fits
// within 16 bits, otherwise the decoding implementation would treat the
// high bits as a repeat count." When duration fits in 16 bits (0x8000 to
// 0xFFFF), its high bits are zero, the prefix word is exactly 0x8000
// (not > 0x8000), and no count prefix is needed for disambiguation --
// that's the "fits within 16 bits" exception the spec names.
func encodePulseEntry(p Pulse) []byte {
	needsExtension := p.Duration >= 0x8000
	highBitsNonZero := p.Duration>>16 != 0
	mustDisambiguate := needsExtension && highBitsNonZero

	var buf bytes.Buffer
	var u16 [2]byte
	if p.Count != 1 || mustDisambiguate {
		binary.LittleEndian.PutUint16(u16[:], 0x8000|p.Count)
		buf.Write(u16[:])
	}
	if p.Duration < 0x8000 {
		binary.LittleEndian.PutUint16(u16[:], uint16(p.Duration))
		buf.Write(u16[:])
	} else {
		binary.LittleEndian.PutUint16(u16[:], 0x8000|uint16(p.Duration>>16))
		buf.Write(u16[:])
		binary.LittleEndian.PutUint16(u16[:], uint16(p.Duration&0xFFFF))
		buf.Write(u16[:])
	}
	return buf.Bytes()
}

func encodePulse(b PulseBlock) []byte {
	var buf bytes.Buffer
	for _, p := range b.Pulses {
		buf.Write(encodePulseEntry(p))
	}
	return withBlockHeader("PULS", buf.Bytes())
}

func decodeData(data []byte) (Block, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("DATA block is %d bytes, want at least 8", len(data))
	}
	countField := binary.LittleEndian.Uint32(data[0:])
	bitCount := countField &^ 0x80000000
	initialHigh := countField&0x80000000 != 0
	tail := binary.LittleEndian.Uint16(data[4:])
	p0 := int(data[6])
	p1 := int(data[7])

	pos := 8
	need := pos + 2*p0 + 2*p1
	if len(data) < need {
		return nil, fmt.Errorf("DATA block too short for %d+%d pulse durations", p0, p1)
	}
	s0 := make([]uint16, p0)
	for i := range s0 {
		s0[i] = binary.LittleEndian.Uint16(data[pos:])
		pos += 2
	}
	s1 := make([]uint16, p1)
	for i := range s1 {
		s1[i] = binary.LittleEndian.Uint16(data[pos:])
		pos += 2
	}

	byteLen := (int(bitCount) + 7) / 8
	if len(data) < pos+byteLen {
		return nil, fmt.Errorf("DATA block declares %d bits (%d bytes) but only %d bytes remain", bitCount, byteLen, len(data)-pos)
	}
	stream := append([]byte(nil), data[pos:pos+byteLen]...)

	return DataBlock{InitialLevelHigh: initialHigh, BitCount: bitCount, Tail: tail, S0: s0, S1: s1, Data: stream}, nil
}

func encodeData(b DataBlock) ([]byte, error) {
	if len(b.S0) > 255 || len(b.S1) > 255 {
		return nil, fmt.Errorf("DATA: pulse sequence too long (S0=%d S1=%d, max 255 each)", len(b.S0), len(b.S1))
	}
	if b.BitCount&0x80000000 != 0 {
		return nil, fmt.Errorf("DATA: BitCount %d exceeds 31 bits", b.BitCount)
	}
	wantLen := (int(b.BitCount) + 7) / 8
	if len(b.Data) != wantLen {
		return nil, fmt.Errorf("DATA: Data is %d bytes, want %d for a %d-bit stream", len(b.Data), wantLen, b.BitCount)
	}

	var buf bytes.Buffer
	var u32 [4]byte
	count := b.BitCount
	if b.InitialLevelHigh {
		count |= 0x80000000
	}
	binary.LittleEndian.PutUint32(u32[:], count)
	buf.Write(u32[:])
	var u16 [2]byte
	binary.LittleEndian.PutUint16(u16[:], b.Tail)
	buf.Write(u16[:])
	buf.WriteByte(byte(len(b.S0)))
	buf.WriteByte(byte(len(b.S1)))
	for _, d := range b.S0 {
		binary.LittleEndian.PutUint16(u16[:], d)
		buf.Write(u16[:])
	}
	for _, d := range b.S1 {
		binary.LittleEndian.PutUint16(u16[:], d)
		buf.Write(u16[:])
	}
	buf.Write(b.Data)
	return withBlockHeader("DATA", buf.Bytes()), nil
}

func decodePause(data []byte) (Block, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("PAUS block is %d bytes, want at least 4", len(data))
	}
	v := binary.LittleEndian.Uint32(data[0:])
	return PauseBlock{InitialLevelHigh: v&0x80000000 != 0, Duration: v &^ 0x80000000}, nil
}

func encodePause(b PauseBlock) []byte {
	v := b.Duration &^ 0x80000000
	if b.InitialLevelHigh {
		v |= 0x80000000
	}
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], v)
	return withBlockHeader("PAUS", u32[:])
}

func encodeBrowse(b BrowseBlock) []byte {
	return withBlockHeader("BRWS", []byte(b.Text))
}

func encodeStop(b StopBlock) []byte {
	var u16 [2]byte
	if b.Only48K {
		binary.LittleEndian.PutUint16(u16[:], 1)
	}
	return withBlockHeader("STOP", u16[:])
}

func encodeRaw(b RawBlock) []byte {
	return withBlockHeader(b.Tag, b.Data)
}

// withBlockHeader prepends the 4-byte tag and 4-byte size PZX blocks share
// (size excludes the tag and size fields themselves, per the spec).
func withBlockHeader(tag string, data []byte) []byte {
	out := make([]byte, 8+len(data))
	copy(out[0:4], tag)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(data)))
	copy(out[8:], data)
	return out
}

// Encode writes f as a PZX image.
func Encode(f *File) ([]byte, error) {
	if len(f.Blocks) == 0 {
		return nil, fmt.Errorf("pzx: no blocks to encode")
	}
	if _, ok := f.Blocks[0].(HeaderBlock); !ok {
		return nil, fmt.Errorf("pzx: first block is %T, want HeaderBlock (PZXT must be first)", f.Blocks[0])
	}

	var out bytes.Buffer
	for _, block := range f.Blocks {
		switch b := block.(type) {
		case HeaderBlock:
			out.Write(encodeHeader(b))
		case PulseBlock:
			out.Write(encodePulse(b))
		case DataBlock:
			enc, err := encodeData(b)
			if err != nil {
				return nil, err
			}
			out.Write(enc)
		case PauseBlock:
			out.Write(encodePause(b))
		case BrowseBlock:
			out.Write(encodeBrowse(b))
		case StopBlock:
			out.Write(encodeStop(b))
		case RawBlock:
			out.Write(encodeRaw(b))
		default:
			return nil, fmt.Errorf("pzx: unknown block type %T", block)
		}
	}
	return out.Bytes(), nil
}
