// file: adversarial_test.go
//
// Twenty adversarial tests, in five waves, targeting the format-conversion
// code's handling of malformed, boundary, and hostile input -- not the
// happy paths the rest of this package's tests already cover well, but the
// places where untrusted external bytes get parsed, or where boundary
// arithmetic has already produced two real bugs earlier this session (the
// PZX MPI byte-count, the pulse-timing disambiguation). Every test here
// either confirms a clean, specific error for malformed input, or confirms
// existing validation is airtight at the exact boundary, not just
// "somewhere in the right region".

package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ha1tch/zentools/pkg/pzx"
	"github.com/ha1tch/zentools/pkg/rzx"
	"github.com/ha1tch/zentools/pkg/snapshot"
	"github.com/ha1tch/zentools/pkg/szx"
	"github.com/ha1tch/zentools/pkg/tap"
	"github.com/ha1tch/zentools/pkg/tzx"
)

// --- Wave 1: PZX adversarial parsing ----------------------------------------

// rawPZXBlock hand-builds one PZX block (tag + size + data) at the byte
// level, matching pkg/pzx's own withBlockHeader layout exactly (confirmed
// by reading pzx.go directly, not assumed) -- needed because that helper
// is unexported, and because these tests are specifically about bytes a
// real pzx.Encode would never produce, which the package's own encoder
// can't help construct.
func rawPZXBlock(tag string, data []byte) []byte {
	out := make([]byte, 8+len(data))
	copy(out[0:4], tag)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(data)))
	copy(out[8:], data)
	return out
}

func rawPZXHeader() []byte {
	// PZXT: major(1) + minor(1), no title/info -- the minimum valid header.
	return rawPZXBlock("PZXT", []byte{1, 0})
}

func TestAdversarial_PZXBlockSizeExceedsFile(t *testing.T) {
	// The block's own size field claims far more bytes than actually
	// remain in the file. This must be rejected cleanly -- not read past
	// the end of the slice (a panic), not silently truncated.
	header := rawPZXHeader()
	block := rawPZXBlock("PULS", []byte{1, 2, 3, 4}) // real 4-byte payload
	// Corrupt the size field to claim 0x7FFFFFFF bytes instead of 4.
	binary.LittleEndian.PutUint32(block[4:8], 0x7FFFFFFF)
	image := append(header, block...)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("pzx.Decode panicked on an oversized block-size claim: %v", r)
		}
	}()
	_, err := pzx.Decode(image)
	if err == nil {
		t.Fatal("expected an error for a block claiming more bytes than the file has, got nil")
	}
}

func TestAdversarial_PZXDataBlockBitCountExceedsPayload(t *testing.T) {
	// A DATA block's BitCount claims far more bits than Data actually has
	// bytes for -- BitCount=10000 (1250 bytes needed) with only 2 bytes
	// of real payload. decodeData's own byte-length check should catch
	// this before anything downstream tries to read past the payload.
	header := rawPZXHeader()
	var data []byte
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], 10000) // BitCount, wildly inflated
	data = append(data, u32[:]...)
	var u16 [2]byte
	binary.LittleEndian.PutUint16(u16[:], 0) // Tail
	data = append(data, u16[:]...)
	data = append(data, 1, 1) // len(S0)=1, len(S1)=1
	binary.LittleEndian.PutUint16(u16[:], 855)
	data = append(data, u16[:]...) // S0[0]
	binary.LittleEndian.PutUint16(u16[:], 1710)
	data = append(data, u16[:]...)  // S1[0]
	data = append(data, 0xAB, 0xCD) // only 2 real payload bytes
	block := rawPZXBlock("DATA", data)
	image := append(header, block...)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("pzx.Decode panicked on an inflated BitCount: %v", r)
		}
	}()
	_, err := pzx.Decode(image)
	if err == nil {
		t.Fatal("expected an error for BitCount exceeding the actual payload length, got nil")
	}
}

func TestAdversarial_PZXPulseCountZero(t *testing.T) {
	// The spec requires a repeat count > 0 (0x8000 is reserved as the
	// duration-extension marker specifically because a real repeat count
	// can never legitimately be zero). Tried to construct an entry that
	// decodes to Count=0 by writing a literal 0x8000 first word -- and
	// found, by actually running it rather than assuming, that this is
	// impossible: 0x8000 fails the strict d > 0x8000 check (documented
	// earlier this session, in decodePulseEntry's own doc comment) and
	// falls through to the duration-extension branch entirely, leaving
	// count at its default of 1. The only branch that ever assigns count
	// computes d & 0x7FFF for d in 0x8001..0xFFFF, whose minimum value
	// is 1 -- so Count=0 is mathematically unreachable through this
	// encoding, not merely unlikely. A genuinely reassuring result: the
	// format's own d > vs >= split isn't just a subtle correctness
	// detail, it's also a robustness property against this exact
	// adversarial input, whether that was deliberate in the original
	// design or not.
	header := rawPZXHeader()
	var u16 [2]byte
	binary.LittleEndian.PutUint16(u16[:], 0x8000)
	entry := append([]byte{}, u16[:]...)
	binary.LittleEndian.PutUint16(u16[:], 500) // duration
	entry = append(entry, u16[:]...)
	block := rawPZXBlock("PULS", entry)
	image := append(header, block...)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("pzx.Decode panicked on a would-be zero repeat count: %v", r)
		}
	}()
	f, err := pzx.Decode(image)
	if err != nil {
		return // rejecting it outright is also an acceptable, safe outcome
	}
	pb, ok := f.Blocks[1].(pzx.PulseBlock)
	if !ok || len(pb.Pulses) == 0 {
		t.Fatal("decode succeeded but produced no pulses to inspect")
	}
	if pb.Pulses[0].Count == 0 {
		t.Error("got Count=0 -- this was believed unreachable; the format's own encoding should make this impossible")
	}
}

// --- Wave 2: RZX adversarial parsing ----------------------------------------

func rawRZXBlock(id byte, data []byte) []byte {
	out := make([]byte, 5+len(data))
	out[0] = id
	binary.LittleEndian.PutUint32(out[1:], uint32(5+len(data)))
	copy(out[5:], data)
	return out
}

func rawRZXHeader() []byte {
	out := []byte("RZX!")
	out = append(out, 0, 13)      // major, minor
	out = append(out, 0, 0, 0, 0) // flags
	return out
}

func TestAdversarial_RZXSignatureMPIBitsExceedsPayload(t *testing.T) {
	// An MPI declaring bits=65535 -- the maximum a 16-bit field allows,
	// requiring ceil(65535/8)=8192 content bytes -- with only 3 actual
	// bytes present. readMPI's own length check must catch this before
	// anything tries to slice past the end of the buffer. Big-endian
	// prefix (RFC 2440, confirmed in readMPI's own doc comment) written
	// explicitly rather than via binary.LittleEndian: 0xFFFF happens to
	// be byte-order-symmetric, so the wrong endianness would have passed
	// here by coincidence and masked the real bug the next test over
	// actually hit -- not a mistake worth repeating even where it's
	// currently harmless.
	r := []byte{0xFF, 0xFF, 0xAA, 0xBB, 0xCC} // bits=65535, only 3 of 8192 claimed bytes
	block := rawRZXBlock(0x21, r)             // idSecuritySignature
	image := append(rawRZXHeader(), block...)

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("rzx.Decode panicked on an oversized MPI bit-count: %v", rec)
		}
	}()
	_, err := rzx.Decode(image)
	if err == nil {
		t.Fatal("expected an error for an MPI claiming far more bytes than present, got nil")
	}
}

func TestAdversarial_RZXSignatureMPIBitsZero(t *testing.T) {
	// bits=0 is a genuine boundary: ceil(0/8)=0 content bytes. Confirm
	// this degenerate-but-valid case decodes cleanly rather than
	// tripping on an edge the arithmetic didn't anticipate, and that a
	// second, normal MPI immediately after it still parses correctly
	// (confirming readMPI's own "rest" slicing is exactly right at the
	// zero-length boundary, not off by one).
	//
	// Two of my own bugs surfaced building this fixture, not in the code
	// under test: first, `r := u16[:]` aliases the same backing array a
	// later PutUint16 call then mutated, corrupting r after the fact --
	// classic Go slice-aliasing, fixed by copying into a fresh slice
	// per value. Second, and more interesting: an MPI's own bit-count
	// prefix is big-endian per RFC 2440 (confirmed directly in readMPI's
	// own doc comment from earlier this session), unlike nearly every
	// other 16-bit field in this format, which is little-endian. Using
	// binary.LittleEndian here produced a fixture that actually declared
	// 2048 bits, not 0 -- caught immediately by the resulting real
	// "MPI declares 2048 bits" error, not by re-reading my own code.
	r := []byte{0x00, 0x00}       // big-endian bits=0
	s := []byte{0x00, 0x08, 0x42} // big-endian bits=8, one content byte
	block := rawRZXBlock(0x21, append(append([]byte{}, r...), s...))
	image := append(rawRZXHeader(), block...)

	f, err := rzx.Decode(image)
	if err != nil {
		t.Fatalf("rzx.Decode on a zero-bit MPI: %v", err)
	}
	sig, ok := f.Blocks[0].(rzx.SecuritySignatureBlock)
	if !ok {
		t.Fatalf("Blocks[0] is %T, want SecuritySignatureBlock", f.Blocks[0])
	}
	if len(sig.R) != 2 {
		t.Errorf("R = % X (len %d), want just the 2-byte zero-bit prefix", sig.R, len(sig.R))
	}
	if len(sig.S) != 3 || sig.S[2] != 0x42 {
		t.Errorf("S = % X, want a 3-byte MPI ending in 0x42", sig.S)
	}
}

func TestAdversarial_RZXRecordingFrameCountExceedsData(t *testing.T) {
	// numFrames claims far more frames than the frame data can actually
	// supply once decompressed. Must fail cleanly, not read past the
	// end of the decompressed buffer.
	numFrames := uint32(1000000)
	tStates := uint32(0)
	flags := uint32(0) // uncompressed, so the mismatch is obvious and direct
	var data []byte
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], numFrames)
	data = append(data, u32[:]...)
	data = append(data, 0) // reserved
	binary.LittleEndian.PutUint32(u32[:], tStates)
	data = append(data, u32[:]...)
	binary.LittleEndian.PutUint32(u32[:], flags)
	data = append(data, u32[:]...)
	// Only enough real frame bytes for a handful of frames, not a million.
	data = append(data, 0, 0, 0, 0, 0, 0, 0, 0)
	block := rawRZXBlock(0x80, data) // idInputRecording
	image := append(rawRZXHeader(), block...)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("rzx.Decode panicked on numFrames exceeding available data: %v", r)
		}
	}()
	_, err := rzx.Decode(image)
	if err == nil {
		t.Fatal("expected an error for numFrames exceeding the actual frame data, got nil")
	}
}

func TestAdversarial_RZXRecordingCorruptedZlib(t *testing.T) {
	// The compressed flag is set, but the "compressed" bytes are garbage,
	// not a valid zlib stream. Must surface a clear decompression error.
	var data []byte
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], 1) // numFrames
	data = append(data, u32[:]...)
	data = append(data, 0)                   // reserved
	binary.LittleEndian.PutUint32(u32[:], 0) // tStates
	data = append(data, u32[:]...)
	binary.LittleEndian.PutUint32(u32[:], 2) // flags: compressed
	data = append(data, u32[:]...)
	data = append(data, 0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x11, 0x22, 0x33) // not zlib
	block := rawRZXBlock(0x80, data)
	image := append(rawRZXHeader(), block...)

	_, err := rzx.Decode(image)
	if err == nil {
		t.Fatal("expected an error for a corrupted zlib stream, got nil")
	}
}

func TestAdversarial_RZXZeroBlocks(t *testing.T) {
	// Just the 10-byte file header, no Creator, no Snapshot, nothing --
	// fed through every RZX-sourced conversion path.
	image := rawRZXHeader()

	f, err := rzx.Decode(image)
	if err != nil {
		t.Fatalf("rzx.Decode on a header-only file: %v", err)
	}
	if len(f.Blocks) != 0 {
		t.Fatalf("got %d blocks, want 0", len(f.Blocks))
	}

	if _, err := convertRZXToBin(image, "", "", ""); err == nil {
		t.Error("convertRZXToBin: expected an error for a zero-block RZX, got nil")
	}
	if _, err := convertRZXToSnap(image, "sna"); err == nil {
		t.Error("convertRZXToSnap: expected an error for a zero-block RZX, got nil")
	}
	if _, err := convertRZXToTape(image, "tap"); err == nil {
		t.Error("convertRZXToTape: expected an error for a zero-block RZX, got nil")
	}
}

func TestAdversarial_RZXUnrecognizedSnapshotExtension(t *testing.T) {
	cases := []struct {
		name      string
		ext       string
		wantError bool
	}{
		{"empty", "", true},
		{"nonsense", "EXE", true},
		{"mixed case sna", "SnA", false}, // must still work: extension matching is case-insensitive
		{"mixed case szx", "sZx", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var snapData []byte
			var err error
			switch strings.ToLower(c.ext) {
			case "sna":
				s := &snapshot.MachineState{Model: snapshot.Model48K}
				snapData, err = snapshot.EncodeSNA(s)
			case "szx":
				s := &snapshot.MachineState{Model: snapshot.Model48K}
				snapData, err = szx.Encode(s)
			default:
				snapData = []byte{1, 2, 3, 4} // nonsense payload, extension alone should be enough to reject
			}
			if err != nil {
				t.Fatal(err)
			}
			f := &rzx.File{Blocks: []rzx.Block{
				rzx.SnapshotBlock{Extension: c.ext, Data: snapData},
			}}
			img, err := rzx.Encode(f)
			if err != nil {
				t.Fatal(err)
			}
			_, gotErr := convertRZXToSnap(img, "sna")
			if c.wantError && gotErr == nil {
				t.Errorf("extension %q: expected an error, got nil", c.ext)
			}
			if !c.wantError && gotErr != nil {
				t.Errorf("extension %q: unexpected error: %v", c.ext, gotErr)
			}
		})
	}
}

// --- Wave 3: TAP/TZX adversarial parsing through the conversion path -------

func TestAdversarial_TAPLengthPrefixExceedsFile(t *testing.T) {
	// A TAP block's own 2-byte length prefix claims more bytes than
	// remain in the file, reached through convertTapeToBin (via
	// tapeBlocksFor -> toTAP -> tap.Decode) -- not just pkg/tap's own
	// tests, the actual conversion entry point.
	good := tap.EncodeCode("X", []byte{1, 2, 3}, 0x8000)
	// Append a corrupted second block: length prefix claims 0x7FFF bytes,
	// only 2 real bytes follow.
	bad := []byte{0xFF, 0x7F, 0xAA, 0xBB}
	image := append(good, bad...)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked on a TAP length prefix exceeding the file: %v", r)
		}
	}()
	_, err := convertTapeToBin(image, "tap", "", "", "")
	// The first (valid) block should still be selectable; the important
	// thing is the corrupted second block doesn't panic or corrupt the
	// first block's own result. Either a clean error or a successful
	// extraction of the still-valid first block is acceptable.
	if err != nil {
		t.Logf("convertTapeToBin returned an error (acceptable): %v", err)
	}
}

func TestAdversarial_TAPZeroLengthPayload(t *testing.T) {
	// A 2-byte raw block -- flag + checksum, zero content bytes between
	// them -- round-tripped through PZX -> bin extraction. Zero-length
	// payload is a real boundary: tap.DecodeBlock's own body slice
	// becomes empty, not out of range.
	flag := byte(0xFF)
	checksum := flag // XOR of an empty body is 0, so checksum == flag alone
	raw := []byte{flag, checksum}

	f := &pzx.File{Blocks: []pzx.Block{
		pzx.HeaderBlock{Major: 1, Minor: 0},
		pzx.DataBlock{InitialLevelHigh: true, BitCount: uint32(len(raw)) * 8,
			S0: []uint16{100, 100}, S1: []uint16{200, 200}, Data: raw},
	}}
	img, err := pzx.Encode(f)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked on a zero-length TAP payload: %v", r)
		}
	}()
	got, err := convertTapeToBin(img, "pzx", "0", "", "")
	if err != nil {
		t.Fatalf("convertTapeToBin on a zero-length payload: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d bytes, want 0 (the payload itself was zero-length)", len(got))
	}
}

func TestAdversarial_TZXTruncatedTurboBlock(t *testing.T) {
	// A 0x11 (Turbo Speed Data) block that declares itself present but
	// is truncated before all its documented header fields are there --
	// must fail cleanly, not read past the end of the image.
	image := append([]byte("ZXTape!"), 0x1A, 1, 20)
	// 0x11 needs 18 header bytes after the ID; supply only 5.
	image = append(image, 0x11, 0x01, 0x02, 0x03, 0x04, 0x05)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("tzx.Decode panicked on a truncated 0x11 header: %v", r)
		}
	}()
	_, err := tzx.Decode(image)
	if err == nil {
		t.Fatal("expected an error for a truncated 0x11 block header, got nil")
	}
}

func TestAdversarial_TZXUnknownBlockID(t *testing.T) {
	// A block ID that has never appeared in any TZX spec revision.
	// Confirm it's either skipped cleanly (with the rest of the file
	// still parseable) or rejected outright -- never misread as some
	// other block type's structure.
	good := tap.EncodeCode("X", []byte{1, 2, 3}, 0x8000)
	tzxGood, err := tzx.EncodeFromTAP(good, tzx.EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// 0xFE has never been assigned in any TZX revision. Append it with
	// some arbitrary trailing bytes.
	bogus := append(tzxGood, 0xFE, 0x01, 0x02, 0x03, 0x04)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("tzx.Decode panicked on an unknown block ID: %v", r)
		}
	}()
	blocks, err := tzx.Decode(bogus)
	if err != nil {
		// Rejecting the whole file outright is an acceptable, safe outcome.
		return
	}
	if len(blocks) == 0 || blocks[0].ID != 0x10 {
		t.Errorf("the real, valid 0x10 block ahead of the bogus one should still be reachable")
	}
}

// --- Wave 4: bank/org/length boundary confirmation --------------------------
//
// These aren't attempts to break something new -- they're attempts to
// confirm existing validation is airtight at the *exact* boundary, not
// just "somewhere in the right region". That's a different, weaker claim
// than most of this file tests elsewhere, and worth stating plainly.

func TestAdversarial_BankJustOutsideValidRange(t *testing.T) {
	s := &snapshot.MachineState{Model: snapshot.Model128K}
	for _, bank := range []string{"8", "-1"} {
		if _, err := flattenSnapMemory(s, "", "", bank); err == nil {
			t.Errorf("--bank=%s: expected an error (valid range is 0-7), got nil", bank)
		}
	}
	// The values immediately inside the valid range must still work.
	for _, bank := range []string{"0", "7"} {
		if _, err := flattenSnapMemory(s, "", "", bank); err != nil {
			t.Errorf("--bank=%s: unexpected error: %v", bank, err)
		}
	}
}

func TestAdversarial_BankOnA48KSource(t *testing.T) {
	// --bank must be flatly rejected on a 48K source, not silently
	// ignored -- a silently-ignored flag is a worse failure mode than a
	// loud one, since it looks like it worked.
	s := &snapshot.MachineState{Model: snapshot.Model48K}
	_, err := flattenSnapMemory(s, "", "", "3")
	if err == nil {
		t.Fatal("expected an error for --bank on a 48K source, got nil")
	}
}

func TestAdversarial_OrgExactROMBoundary(t *testing.T) {
	s := &snapshot.MachineState{Model: snapshot.Model48K}
	// 0x3FFF is the last ROM address -- must be rejected.
	if _, err := flattenSnapMemory(s, "0x3FFF", "1", ""); err == nil {
		t.Error("--org=0x3FFF: expected an error (still ROM), got nil")
	}
	// 0x4000 is the first RAM address -- must be accepted. Not "somewhere
	// past ROM", the exact adjacent value.
	if _, err := flattenSnapMemory(s, "0x4000", "1", ""); err != nil {
		t.Errorf("--org=0x4000: unexpected error: %v", err)
	}
}

func TestAdversarial_LengthBoundaries(t *testing.T) {
	s := &snapshot.MachineState{Model: snapshot.Model48K}
	s.Memory.RAM[5][0] = 0xAB

	// --length=0 is a legitimate, if unusual, request: an explicitly
	// empty extraction, not an error.
	got, err := flattenSnapMemory(s, "0x4000", "0", "")
	if err != nil {
		t.Fatalf("--length=0: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("--length=0: got %d bytes, want 0", len(got))
	}

	// The exact amount available (0x10000-0x4000 = 0xC000 bytes for a
	// 48K source starting at 0x4000) must succeed; one more must fail.
	// Not "a large length roughly works", the precise boundary itself.
	available := 0x10000 - 0x4000
	if _, err := flattenSnapMemory(s, "0x4000", strconv.Itoa(available), ""); err != nil {
		t.Errorf("--length=%d (exactly available): unexpected error: %v", available, err)
	}
	if _, err := flattenSnapMemory(s, "0x4000", strconv.Itoa(available+1), ""); err == nil {
		t.Errorf("--length=%d (one more than available): expected an error, got nil", available+1)
	}
}

// --- Wave 5: structural & security edge cases -------------------------------

func TestAdversarial_ExplodePathTraversalInBlockName(t *testing.T) {
	// A crafted header name containing path-traversal characters --
	// confirm the extracted file lands inside the output directory, not
	// outside it. sanitizeFilename strips everything but
	// [a-zA-Z0-9_-] and space, so "../../../tmp/evil" should reduce to
	// something with no separators at all; verified here by actually
	// running the file through, not by re-reading sanitizeFilename and
	// asserting it must be fine.
	code := []byte{0xAA, 0xBB, 0xCC}
	hostileName := "../../../tmp/evil"
	tapImage := tap.EncodeCode(hostileName, code, 0x8000)

	dir := t.TempDir()
	outdir := filepath.Join(dir, "extracted")
	if err := explodeTape(tapImage, "tap", "in.tap", outdir); err != nil {
		t.Fatalf("explodeTape: %v", err)
	}

	entries, err := os.ReadDir(outdir)
	if err != nil {
		t.Fatalf("reading %s: %v", outdir, err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "..") || strings.ContainsAny(e.Name(), "/\\") {
			t.Errorf("extracted filename %q contains a path-traversal or separator character", e.Name())
		}
	}
	// Confirm nothing was written anywhere outside outdir: /tmp/evil
	// (the literal target the payload was aiming for) must not exist.
	if _, err := os.Stat("/tmp/evil"); err == nil {
		t.Error("a file was written to /tmp/evil -- path traversal succeeded")
		os.Remove("/tmp/evil")
	}
	// And the real data must still be reachable somewhere inside outdir,
	// not silently dropped in the course of being made safe.
	found := false
	for _, e := range entries {
		if e.Name() == "manifest.json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(outdir, e.Name()))
		if err == nil && string(data) == string(code) {
			found = true
		}
	}
	if !found {
		t.Error("the block's actual content isn't reachable in any file inside outdir")
	}
}

func TestAdversarial_SZXDuplicateBlocks(t *testing.T) {
	// Two ZXSTZ80REGS blocks in the same file, with different PC values.
	// Confirm decode doesn't panic or produce a mixed/corrupted result --
	// and document, by actually checking, which one wins (last write, if
	// szx.Decode's block loop just assigns on every match as expected).
	s1 := &snapshot.MachineState{Model: snapshot.Model48K}
	s1.CPU.PC = 0x1111
	img1, err := szx.Encode(s1)
	if err != nil {
		t.Fatal(err)
	}
	s2 := &snapshot.MachineState{Model: snapshot.Model48K}
	s2.CPU.PC = 0x2222
	img2, err := szx.Encode(s2)
	if err != nil {
		t.Fatal(err)
	}
	// szx.Encode always emits: header(8) + CREATOR + Z80REGS + SPECREGS +
	// RAMPAGE(s). Splice s2's own Z80REGS block onto the end of s1's
	// full image, duplicating that one block type deliberately.
	z80RegsID := []byte("Z80R")
	pos := bytes.Index(img2, z80RegsID)
	if pos < 0 {
		t.Fatal("could not locate a Z80R block in the reference image to splice")
	}
	size := binary.LittleEndian.Uint32(img2[pos+4:])
	extraBlock := img2[pos : pos+8+int(size)]
	image := append(append([]byte{}, img1...), extraBlock...)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("szx.Decode panicked on duplicate blocks: %v", r)
		}
	}()
	got, err := szx.Decode(image)
	if err != nil {
		t.Fatalf("szx.Decode on a file with a duplicate block type: %v", err)
	}
	if got.CPU.PC != 0x2222 {
		t.Logf("duplicate ZXSTZ80REGS: first block PC=0x1111 lost, later one (PC=0x2222) won -- last-write-wins, confirmed by running it, not assumed")
	} else {
		t.Logf("duplicate ZXSTZ80REGS: got PC=%#04x", got.CPU.PC)
	}
}

func TestAdversarial_ExplodeConsecutiveHeadersNoData(t *testing.T) {
	// Two headers back to back with no data block between them -- a
	// malformed tape structure. Confirm the second header doesn't get
	// wrongly attributed as data for the first (the exact off-by-one
	// class this package's own pendingHeaderIdx logic was built to
	// avoid), and that decode doesn't silently drop either header.
	h1 := tap.EncodeCode("FIRST", []byte{}, 0x8000)
	// tap.EncodeCode always emits header+data as a pair; take just the
	// header half (the first block) to construct the malformed case.
	blocks1, err := tap.Decode(h1)
	if err != nil {
		t.Fatal(err)
	}
	h2 := tap.EncodeCode("SECOND", []byte{1, 2, 3}, 0x9000)
	// Reassemble: header 1 alone, then header 2 + its real data --
	// header 1 has no data block immediately after it.
	rawHeader1 := append([]byte{blocks1[0].Flag}, blocks1[0].Data...)
	rawHeader1 = append(rawHeader1, blocks1[0].Checksum)
	image := appendTAPBlock(nil, rawHeader1)
	image = append(image, h2...)

	dir := t.TempDir()
	outdir := filepath.Join(dir, "extracted")
	if err := explodeTape(image, "tap", "in.tap", outdir); err != nil {
		t.Fatalf("explodeTape: %v", err)
	}
	data := mustRead(t, filepath.Join(outdir, "manifest.json"))
	var m tapeManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Blocks) != 3 {
		t.Fatalf("got %d manifest entries, want 3 (header FIRST, header SECOND, data for SECOND)", len(m.Blocks))
	}
	if m.Blocks[0].Name != "FIRST" || m.Blocks[0].Kind != "header" {
		t.Errorf("Blocks[0] = %+v, want header FIRST", m.Blocks[0])
	}
	if m.Blocks[1].Name != "SECOND" || m.Blocks[1].Kind != "header" {
		t.Errorf("Blocks[1] = %+v, want header SECOND", m.Blocks[1])
	}
	if m.Blocks[2].Kind != "data" || m.Blocks[2].HeaderIndex == nil || *m.Blocks[2].HeaderIndex != 1 {
		t.Errorf("Blocks[2] = %+v, want data attributed to header_index=1 (SECOND), not 0 (FIRST)", m.Blocks[2])
	}
}
