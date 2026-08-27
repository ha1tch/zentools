package tzx

import "testing"

// Byte layouts confirmed against SpecIde's own TZXFile.cc and its
// getBlockHeaderLength() table, the same discipline as newblocks_test.go.
// Several of these (0x23, 0x26, 0x27) are marked "Not implemented yet" in
// SpecIde's own source -- even the reference implementation only skips
// them structurally, without acting on their control-flow meaning. This
// package matches that same level of fidelity for those three: correct,
// safe skipping, not a claim of real jump/call semantics neither
// implementation actually has. 0x24/0x25 (Loop Start/End) are the
// exception -- SpecIde does implement real loop semantics there, and
// this package's own version of that lives in a separate test file
// (loop_test.go) since it needs Decode itself to re-emit blocks, not
// just skip bytes.

// decodeOne is a small helper: build a minimal TZX image with exactly one
// block of the given bytes, followed by a distinguishable marker block
// (0x22, Group End -- fixed, zero-length, unambiguous), and return the
// decoded blocks. Confirms both that the block's own fields decode
// correctly AND that pos ends up in the right place afterward.
func decodeOneThenMarker(t *testing.T, blockBytes []byte) []Block {
	t.Helper()
	data := tzxHeader10()
	data = append(data, blockBytes...)
	data = append(data, idGroupEnd) // marker: no body, unambiguous
	blocks, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(blocks) < 1 {
		t.Fatalf("got 0 blocks")
	}
	if blocks[len(blocks)-1].ID != idGroupEnd {
		t.Fatalf("last block ID = 0x%02X, want 0x%02X (marker) -- pos did not end up in the right place", blocks[len(blocks)-1].ID, idGroupEnd)
	}
	return blocks
}

// TestDecode_DeprecatedC64Blocks confirms 0x16 and 0x17 (both explicitly
// deprecated, C64-related, and never emitted by any real Spectrum tool)
// are still recognised and safely skipped by their own u32 length field,
// rather than causing a hard decode failure on the rare file that still
// has one. SpecIde header length 0x05 for both = id(1) + length_u32(4).
func TestDecode_DeprecatedC64Blocks(t *testing.T) {
	for _, id := range []byte{idC64ROM, idC64Turbo} {
		blockBytes := append([]byte{id}, le32(3)...)
		blockBytes = append(blockBytes, 0xAA, 0xBB, 0xCC) // 3 bytes of ignored payload
		blocks := decodeOneThenMarker(t, blockBytes)
		if len(blocks) != 2 || blocks[0].ID != id {
			t.Errorf("id 0x%02X: got %d blocks, first ID 0x%02X", id, len(blocks), blocks[0].ID)
		}
	}
}

// TestDecode_JumpCallReturn confirms 0x23 (Jump To Block), 0x26 (Call
// Sequence), and 0x27 (Return From Sequence) are all skipped safely by
// their own structural length, matching SpecIde's own "not implemented
// yet" stub behaviour for these three specifically -- not a claim that
// this package executes real control flow for them.
func TestDecode_JumpCallReturn(t *testing.T) {
	t.Run("0x23 Jump To Block", func(t *testing.T) {
		blockBytes := append([]byte{idJumpBlock}, le16(1)...) // relative offset, unused
		blocks := decodeOneThenMarker(t, blockBytes)
		if len(blocks) != 2 || blocks[0].ID != idJumpBlock {
			t.Errorf("got %d blocks, first ID 0x%02X", len(blocks), blocks[0].ID)
		}
	})
	t.Run("0x26 Call Sequence", func(t *testing.T) {
		blockBytes := []byte{idCallSeq}
		blockBytes = append(blockBytes, le16(2)...) // 2 nested call offsets follow
		blockBytes = append(blockBytes, le16(1)...)
		blockBytes = append(blockBytes, le16(2)...)
		blocks := decodeOneThenMarker(t, blockBytes)
		if len(blocks) != 2 || blocks[0].ID != idCallSeq {
			t.Errorf("got %d blocks, first ID 0x%02X", len(blocks), blocks[0].ID)
		}
	})
	t.Run("0x27 Return From Sequence", func(t *testing.T) {
		blocks := decodeOneThenMarker(t, []byte{idReturnSeq})
		if len(blocks) != 2 || blocks[0].ID != idReturnSeq {
			t.Errorf("got %d blocks, first ID 0x%02X", len(blocks), blocks[0].ID)
		}
	})
}

// TestDecode_SelectBlock confirms 0x28 skips by its own declared length
// (a u16 immediately after the ID, itself included in what gets skipped
// -- SpecIde: "pointer += getU16(pointer+1) + 3", i.e. length field (2)
// plus the ID (1, already consumed by this package's pos) plus n more
// bytes of selection data).
func TestDecode_SelectBlock(t *testing.T) {
	n := 5
	blockBytes := []byte{idSelectBlock}
	blockBytes = append(blockBytes, le16(uint16(n))...)
	blockBytes = append(blockBytes, []byte{1, 2, 3, 4, 5}...)
	blocks := decodeOneThenMarker(t, blockBytes)
	if len(blocks) != 2 || blocks[0].ID != idSelectBlock {
		t.Errorf("got %d blocks, first ID 0x%02X", len(blocks), blocks[0].ID)
	}
}

// TestDecode_Message confirms 0x31 (Message), structurally identical to
// the already-supported 0x30 (Text Description) but with an extra
// leading "seconds to display" byte: SpecIde header length 0x03 = id(1)
// + displayTime(1) + textLength(1), then textLength bytes of text.
func TestDecode_Message(t *testing.T) {
	blockBytes := []byte{idMessage, 3} // display for 3 seconds
	text := "Loading..."
	blockBytes = append(blockBytes, byte(len(text)))
	blockBytes = append(blockBytes, []byte(text)...)
	blocks := decodeOneThenMarker(t, blockBytes)
	if len(blocks) != 2 || blocks[0].ID != idMessage {
		t.Errorf("got %d blocks, first ID 0x%02X", len(blocks), blocks[0].ID)
	}
}

// TestDecode_CustomInfo confirms 0x35 skips a 16-byte identifier plus a
// u32 length field plus that many bytes of payload. SpecIde header
// length 0x15 (21) = id(1) + identifier(16) + length_u32(4).
func TestDecode_CustomInfo(t *testing.T) {
	blockBytes := []byte{idCustomInfo}
	ident := make([]byte, 16)
	copy(ident, "Test ID Block")
	blockBytes = append(blockBytes, ident...)
	blockBytes = append(blockBytes, le32(2)...)
	blockBytes = append(blockBytes, 0xDE, 0xAD)
	blocks := decodeOneThenMarker(t, blockBytes)
	if len(blocks) != 2 || blocks[0].ID != idCustomInfo {
		t.Errorf("got %d blocks, first ID 0x%02X", len(blocks), blocks[0].ID)
	}
}

// TestDecode_Glue confirms 0x5A (Glue Block), a fixed 9 bytes of payload
// after the ID -- the historical trick that lets two TZX files be
// concatenated byte-for-byte: the ID (0x5A, ASCII 'Z') plus its own 9
// data bytes spell out "ZXTape!" plus version bytes again, so a glued
// file's continuation still starts with what looks like a fresh header.
// SpecIde header length 0x0A (10) = id(1) + 9 fixed bytes.
func TestDecode_Glue(t *testing.T) {
	blockBytes := []byte{idGlue}
	blockBytes = append(blockBytes, []byte("XTape!")...)           // 6
	blockBytes = append(blockBytes, eofMarker, majorVer, minorVer) // 3 -- 9 total
	blocks := decodeOneThenMarker(t, blockBytes)
	if len(blocks) != 2 || blocks[0].ID != idGlue {
		t.Errorf("got %d blocks, first ID 0x%02X", len(blocks), blocks[0].ID)
	}
}

// le32 encodes v as four little-endian bytes.
func le32(v uint32) []byte {
	return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
}

// TestDecode_OriginalHandlers_RejectOverflowingLength is a regression
// test surfaced by cross-checking this package against libspectrum's own
// TZX test fixtures (test/invalid-hardwareinfo.tzx specifically): five
// of the six block handlers already present before this package's own
// extension work (idArchiveInfo, idTextDesc, idStopThe48K, idHardwareTyp,
// idGroupStart) checked that a length field itself was safely readable,
// but never checked that the *computed* advancement (length field value
// applied) stayed within the image bounds -- unlike every block type
// added during this extension work, which all have that check. Not a
// crash risk (the outer `for pos < len(image)` loop terminates safely
// either way), but a real, inconsistent gap: a malformed file with an
// inflated count could silently overshoot pos with no error, unlike the
// newer handlers, which correctly report truncation.
func TestDecode_OriginalHandlers_RejectOverflowingLength(t *testing.T) {
	cases := []struct {
		name       string
		blockBytes []byte
	}{
		// 0x32: 2-byte length field claims more bytes than actually follow.
		{"idArchiveInfo", []byte{idArchiveInfo, 0xFF, 0x00}},
		// 0x30: 1-byte length claims more bytes than actually follow.
		{"idTextDesc", []byte{idTextDesc, 0xFF}},
		// 0x2A: always claims a fixed 4-byte body; none follow here.
		{"idStopThe48K", []byte{idStopThe48K}},
		// 0x33: claims 5 HWINFO entries (15 bytes); none follow -- the
		// exact shape of libspectrum's own invalid-hardwareinfo.tzx.
		{"idHardwareTyp", []byte{idHardwareTyp, 5}},
		// 0x21: 1-byte group-name length claims more bytes than follow.
		{"idGroupStart", []byte{idGroupStart, 0xFF}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := tzxHeader10()
			data = append(data, c.blockBytes...)
			_, err := Decode(data)
			if err == nil {
				t.Errorf("expected a truncation error, got none")
			}
		})
	}
}
