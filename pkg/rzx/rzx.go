// Package rzx reads and writes RZX (Ramsoft ZX Spectrum Replay) input
// recording files, spec v0.13 (https://worldofspectrum.net/RZXformat.html).
// An RZX file records the result of every CPU IN instruction, frame by
// frame, so a playback emulator reproduces an exact recorded session
// regardless of its own core's timing accuracy -- keypresses, joystick
// input, tape/disk loading, and floating-bus behaviour are all captured
// implicitly as port-read values rather than modelled directly.
//
// Blocks are decoded in file order into a []Block, not separated into
// per-type collections, because a real multiload RZX legitimately
// interleaves Snapshot and Recording blocks (a new snapshot marking where
// a tape reload happened) and that sequence is part of the file's meaning.
//
// Scope: the Creator, Snapshot, and Recording (frame) blocks are fully
// decoded and encoded -- everything needed to read or produce a working,
// unprotected RZX file. The Security Information and Security Signature
// blocks (DSA-signed files, used for game-tournament submissions) are
// decoded into their raw fields and re-encoded verbatim if present, but
// this package does not implement DSA signing or verification -- the spec
// itself calls its own security chapter "obsolete info, needs updating to
// DSA", and implementing verifiable cryptographic signing correctly is a
// substantially different undertaking from reading and writing the
// container format. A protected/signed file's frame data can still be
// decoded structurally; this package just never asserts anything about
// its authenticity.
package rzx

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
)

const magic = "RZX!"

const (
	idCreator           = 0x10
	idSecurityInfo      = 0x20
	idSecuritySignature = 0x21
	idSnapshot          = 0x30
	idInputRecording    = 0x80
)

// Block is implemented by every RZX block type this package models.
type Block interface{ isRZXBlock() }

// CreatorBlock identifies the program that made the recording. Required:
// every valid RZX file has exactly one, and it is conventionally first.
type CreatorBlock struct {
	ID           string // creator's identification string, e.g. "RealSpectrum"
	Major, Minor uint16
	Data         []byte // creator-specific custom data, may be empty
}

func (CreatorBlock) isRZXBlock() {}

// SecurityInfoBlock carries DSA tournament-security parameters. Decoded
// and re-encoded verbatim; see this package's doc comment for why
// verification itself is out of scope.
type SecurityInfoBlock struct {
	KeyID uint32 // low 32 bits of the DSA public key value y
	Week  uint32 // week code, e.g. for game tournaments
}

func (SecurityInfoBlock) isRZXBlock() {}

// SecuritySignatureBlock carries a DSA signature (OpenPGP multi-precision
// integer format) over the blocks it protects. Decoded and re-encoded
// verbatim; not verified -- see this package's doc comment.
type SecuritySignatureBlock struct {
	R, S []byte
}

func (SecuritySignatureBlock) isRZXBlock() {}

// SnapshotBlock embeds (or references) a machine snapshot, marking the
// state a following Recording block starts from. A multiload recording
// has one of these before each Recording block.
type SnapshotBlock struct {
	External   bool   // true: Data is a descriptor (checksum + filename), not the snapshot itself
	Compressed bool   // Data is zlib-compressed (only meaningful when !External)
	Extension  string // snapshot filename extension, e.g. "SNA", "Z80"
	Data       []byte // snapshot image (decompressed) or descriptor bytes
}

func (SnapshotBlock) isRZXBlock() {}

// Frame is one frame of recorded input: the number of instruction fetches
// it lasts for, and the CPU IN results it supplies, in order. A repeated
// frame (Repeated true) reuses the previous frame's PortReads and carries
// none of its own -- see the format's own "idle frame" optimisation.
type Frame struct {
	FetchCount uint16 // R-register increments until the next interrupt (INTA excluded)
	Repeated   bool
	PortReads  []byte
}

// RecordingBlock is the actual input log: a sequence of Frames, each
// supplying the CPU IN results for one frame of playback.
type RecordingBlock struct {
	TStatesStart uint32 // t-states counter at the start of this block
	Protected    bool   // frames are encrypted with a session x-key (not decrypted by this package)
	Frames       []Frame
}

func (RecordingBlock) isRZXBlock() {}

// File is a decoded RZX file: version, whether the security-signed flag
// is set, and every block in file order.
type File struct {
	MajorVersion, MinorVersion byte
	Signed                     bool // header flags bit 0
	Blocks                     []Block
}

// Decode parses an RZX image into a File.
func Decode(image []byte) (*File, error) {
	if len(image) < 10 {
		return nil, fmt.Errorf("rzx: image too short (%d bytes) to contain a header", len(image))
	}
	if string(image[0:4]) != magic {
		return nil, fmt.Errorf("rzx: missing %q signature", magic)
	}
	f := &File{
		MajorVersion: image[4],
		MinorVersion: image[5],
		Signed:       binary.LittleEndian.Uint32(image[6:10])&1 != 0,
	}

	pos := 10
	for pos < len(image) {
		if pos+5 > len(image) {
			return nil, fmt.Errorf("rzx: truncated block header at offset %d", pos)
		}
		id := image[pos]
		length := binary.LittleEndian.Uint32(image[pos+1:])
		if length < 5 {
			return nil, fmt.Errorf("rzx: block at offset %d has length %d, must be at least 5 (the header itself)", pos, length)
		}
		if pos+int(length) > len(image) {
			return nil, fmt.Errorf("rzx: block at offset %d claims length %d, past end of file", pos, length)
		}
		data := image[pos+5 : pos+int(length)]

		block, err := decodeBlock(id, data)
		if err != nil {
			return nil, fmt.Errorf("rzx: block at offset %d: %w", pos, err)
		}
		if block != nil {
			f.Blocks = append(f.Blocks, block)
		}
		pos += int(length)
	}
	return f, nil
}

func decodeBlock(id byte, data []byte) (Block, error) {
	switch id {
	case idCreator:
		return decodeCreator(data)
	case idSecurityInfo:
		return decodeSecurityInfo(data)
	case idSecuritySignature:
		return decodeSecuritySignature(data)
	case idSnapshot:
		return decodeSnapshot(data)
	case idInputRecording:
		return decodeRecording(data)
	default:
		// Unrecognised block type -- skip, per the format's own "processed
		// as commands" model; nothing later depends on unknown blocks.
		return nil, nil
	}
}

func decodeCreator(data []byte) (Block, error) {
	if len(data) < 24 {
		return nil, fmt.Errorf("creator block is %d bytes, want at least 24", len(data))
	}
	id := string(bytes.TrimRight(data[0:20], "\x00"))
	major := binary.LittleEndian.Uint16(data[20:])
	minor := binary.LittleEndian.Uint16(data[22:])
	custom := append([]byte(nil), data[24:]...)
	return CreatorBlock{ID: id, Major: major, Minor: minor, Data: custom}, nil
}

func encodeCreator(b CreatorBlock) []byte {
	data := make([]byte, 24+len(b.Data))
	copy(data[0:20], b.ID) // truncated if longer than 20; null-padded if shorter
	binary.LittleEndian.PutUint16(data[20:], b.Major)
	binary.LittleEndian.PutUint16(data[22:], b.Minor)
	copy(data[24:], b.Data)
	return withBlockHeader(idCreator, data)
}

func decodeSecurityInfo(data []byte) (Block, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("security info block is %d bytes, want at least 8", len(data))
	}
	return SecurityInfoBlock{
		KeyID: binary.LittleEndian.Uint32(data[0:]),
		Week:  binary.LittleEndian.Uint32(data[4:]),
	}, nil
}

func encodeSecurityInfo(b SecurityInfoBlock) []byte {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:], b.KeyID)
	binary.LittleEndian.PutUint32(data[4:], b.Week)
	return withBlockHeader(idSecurityInfo, data)
}

func decodeSecuritySignature(data []byte) (Block, error) {
	// r and s are OpenPGP multi-precision integers (RFC 2440 3.2): a 2-byte
	// bit-length prefix followed by ceil(bits/8) bytes. There is no other
	// length field in the block, so r's own prefix is what delimits it
	// from s -- read r first, whatever remains is s.
	r, rest, err := readMPI(data)
	if err != nil {
		return nil, fmt.Errorf("signature block, parameter r: %w", err)
	}
	s, rest, err := readMPI(rest)
	if err != nil {
		return nil, fmt.Errorf("signature block, parameter s: %w", err)
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("signature block has %d trailing bytes after r and s", len(rest))
	}
	return SecuritySignatureBlock{R: r, S: s}, nil
}

// readMPI reads one OpenPGP multi-precision integer (RFC 2440 section
// 3.2): a big-endian 16-bit bit count, then ceil(bits/8) content bytes.
// Returns the MPI's own bytes (bit-count prefix included, since encodeMPI
// needs nothing more than to re-emit exactly what was read) and whatever
// of data follows it.
func readMPI(data []byte) (mpi, rest []byte, err error) {
	if len(data) < 2 {
		return nil, nil, fmt.Errorf("truncated MPI bit-count prefix")
	}
	bits := int(data[0])<<8 | int(data[1])
	n := (bits + 7) / 8
	if len(data) < 2+n {
		return nil, nil, fmt.Errorf("MPI declares %d bits (%d bytes) but only %d bytes remain", bits, n, len(data)-2)
	}
	return data[:2+n], data[2+n:], nil
}

func encodeSecuritySignature(b SecuritySignatureBlock) []byte {
	data := append(append([]byte{}, b.R...), b.S...)
	return withBlockHeader(idSecuritySignature, data)
}

func decodeSnapshot(data []byte) (Block, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("snapshot block is %d bytes, want at least 12", len(data))
	}
	flags := binary.LittleEndian.Uint32(data[0:])
	ext := string(bytes.TrimRight(data[4:8], "\x00"))
	payload := data[12:]
	external := flags&1 != 0
	compressed := flags&2 != 0
	if compressed && !external {
		r, err := zlib.NewReader(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("snapshot data: zlib: %w", err)
		}
		defer r.Close()
		decompressed, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("snapshot data: zlib: %w", err)
		}
		payload = decompressed
	}
	return SnapshotBlock{External: external, Compressed: false, Extension: ext, Data: append([]byte(nil), payload...)}, nil
}

func encodeSnapshot(b SnapshotBlock) ([]byte, error) {
	payload := b.Data
	var flags uint32
	if b.External {
		flags |= 1
	}
	uncompressedLen := uint32(len(payload))
	if !b.External {
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		if _, err := w.Write(payload); err != nil {
			return nil, fmt.Errorf("snapshot data: zlib: %w", err)
		}
		if err := w.Close(); err != nil {
			return nil, fmt.Errorf("snapshot data: zlib: %w", err)
		}
		payload = buf.Bytes()
		flags |= 2 // always write compressed, since we can always read it back
	}

	data := make([]byte, 12, 12+len(payload))
	binary.LittleEndian.PutUint32(data[0:], flags)
	copy(data[4:8], b.Extension) // truncated/padded to 4 bytes
	binary.LittleEndian.PutUint32(data[8:], uncompressedLen)
	data = append(data, payload...)
	return withBlockHeader(idSnapshot, data), nil
}

func decodeRecording(data []byte) (Block, error) {
	if len(data) < 13 {
		return nil, fmt.Errorf("input recording block is %d bytes, want at least 13", len(data))
	}
	numFrames := binary.LittleEndian.Uint32(data[0:])
	// data[4] reserved
	tStates := binary.LittleEndian.Uint32(data[5:])
	flags := binary.LittleEndian.Uint32(data[9:])
	frameData := data[13:]

	protected := flags&1 != 0
	compressed := flags&2 != 0
	if compressed {
		r, err := zlib.NewReader(bytes.NewReader(frameData))
		if err != nil {
			return nil, fmt.Errorf("recording frames: zlib: %w", err)
		}
		defer r.Close()
		decompressed, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("recording frames: zlib: %w", err)
		}
		frameData = decompressed
	}

	frames := make([]Frame, 0, numFrames)
	pos := 0
	for uint32(len(frames)) < numFrames {
		if pos+4 > len(frameData) {
			return nil, fmt.Errorf("recording declares %d frames but frame data ends after %d", numFrames, len(frames))
		}
		ic := binary.LittleEndian.Uint16(frameData[pos:])
		in := binary.LittleEndian.Uint16(frameData[pos+2:])
		pos += 4
		if in == 0xFFFF {
			frames = append(frames, Frame{FetchCount: ic, Repeated: true})
			continue
		}
		if pos+int(in) > len(frameData) {
			return nil, fmt.Errorf("frame declares %d port reads, past end of frame data", in)
		}
		reads := append([]byte(nil), frameData[pos:pos+int(in)]...)
		pos += int(in)
		frames = append(frames, Frame{FetchCount: ic, PortReads: reads})
	}

	return RecordingBlock{TStatesStart: tStates, Protected: protected, Frames: frames}, nil
}

func encodeRecording(b RecordingBlock) ([]byte, error) {
	var raw bytes.Buffer
	for _, fr := range b.Frames {
		var hdr [4]byte
		binary.LittleEndian.PutUint16(hdr[0:], fr.FetchCount)
		if fr.Repeated {
			binary.LittleEndian.PutUint16(hdr[2:], 0xFFFF)
			raw.Write(hdr[:])
			continue
		}
		binary.LittleEndian.PutUint16(hdr[2:], uint16(len(fr.PortReads)))
		raw.Write(hdr[:])
		raw.Write(fr.PortReads)
	}

	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	if _, err := w.Write(raw.Bytes()); err != nil {
		return nil, fmt.Errorf("recording frames: zlib: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("recording frames: zlib: %w", err)
	}

	data := make([]byte, 13, 13+compressed.Len())
	binary.LittleEndian.PutUint32(data[0:], uint32(len(b.Frames)))
	data[4] = 0 // reserved
	binary.LittleEndian.PutUint32(data[5:], b.TStatesStart)
	flags := uint32(2) // always write compressed
	if b.Protected {
		flags |= 1
	}
	binary.LittleEndian.PutUint32(data[9:], flags)
	data = append(data, compressed.Bytes()...)
	return withBlockHeader(idInputRecording, data), nil
}

// withBlockHeader prepends the 1-byte ID and 4-byte length RZX blocks
// share -- unlike SZX, the RZX length field includes this 5-byte header
// itself (confirmed against the spec's own worked example: the Creator
// block's documented length is "29+N", which only matches its total size,
// header included, when N is the custom-data length).
func withBlockHeader(id byte, data []byte) []byte {
	out := make([]byte, 5+len(data))
	out[0] = id
	binary.LittleEndian.PutUint32(out[1:], uint32(5+len(data)))
	copy(out[5:], data)
	return out
}

// Encode writes f as an RZX image.
func Encode(f *File) ([]byte, error) {
	var out bytes.Buffer
	out.WriteString(magic)
	out.WriteByte(f.MajorVersion)
	out.WriteByte(f.MinorVersion)
	var flags uint32
	if f.Signed {
		flags |= 1
	}
	var flagBytes [4]byte
	binary.LittleEndian.PutUint32(flagBytes[:], flags)
	out.Write(flagBytes[:])

	for _, block := range f.Blocks {
		switch b := block.(type) {
		case CreatorBlock:
			out.Write(encodeCreator(b))
		case SecurityInfoBlock:
			out.Write(encodeSecurityInfo(b))
		case SecuritySignatureBlock:
			out.Write(encodeSecuritySignature(b))
		case SnapshotBlock:
			enc, err := encodeSnapshot(b)
			if err != nil {
				return nil, err
			}
			out.Write(enc)
		case RecordingBlock:
			enc, err := encodeRecording(b)
			if err != nil {
				return nil, err
			}
			out.Write(enc)
		default:
			return nil, fmt.Errorf("rzx: unknown block type %T", block)
		}
	}
	return out.Bytes(), nil
}
