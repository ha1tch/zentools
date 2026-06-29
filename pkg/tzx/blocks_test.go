package tzx

import (
	"testing"
)

// findBlockIDs walks a TZX image and returns the sequence of block IDs after
// the 10-byte header. It understands exactly the blocks this package emits.
func findBlockIDs(t *testing.T, image []byte) []byte {
	t.Helper()
	var ids []byte
	pos := 10
	for pos < len(image) {
		id := image[pos]
		ids = append(ids, id)
		pos++
		switch id {
		case idStandardSpeed:
			n := int(image[pos+2]) | int(image[pos+3])<<8
			pos += 4 + n
		case idArchiveInfo:
			n := int(image[pos]) | int(image[pos+1])<<8
			pos += 2 + n
		case idTextDesc:
			pos += 1 + int(image[pos])
		case idStopThe48K:
			pos += 4
		case idHardwareTyp:
			pos += 1 + int(image[pos])*3
		case idGroupStart:
			pos += 1 + int(image[pos])
		case idGroupEnd:
			// no body
		default:
			t.Fatalf("unexpected block id 0x%02X at offset %d", id, pos-1)
		}
	}
	return ids
}

func TestHardwareBlockBytes(t *testing.T) {
	out, err := EncodeFromTAP(sampleTAP(), EncodeOptions{
		Hardware: []HardwareInfo{
			{Type: HWComputers, ID: HWIDSpectrum128K, Info: HWInfoUses},
			{Type: HWSoundDevices, ID: HWIDClassicAY, Info: HWInfoUses},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Locate the 0x33 block and check its entries byte-for-byte.
	idx := -1
	for i := 10; i < len(out); i++ {
		if out[i] == idHardwareTyp {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("no 0x33 hardware block emitted")
	}
	if out[idx+1] != 2 {
		t.Fatalf("hardware entry count = %d, want 2", out[idx+1])
	}
	want := []byte{HWComputers, HWIDSpectrum128K, HWInfoUses, HWSoundDevices, HWIDClassicAY, HWInfoUses}
	got := out[idx+2 : idx+2+6]
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("hardware byte %d = 0x%02X, want 0x%02X", i, got[i], want[i])
		}
	}
}

func TestGroupBracketsData(t *testing.T) {
	out, err := EncodeFromTAP(sampleTAP(), EncodeOptions{Group: "Main"})
	if err != nil {
		t.Fatal(err)
	}
	ids := findBlockIDs(t, out)
	// Expect: group-start, data, data, group-end.
	if len(ids) < 2 || ids[0] != idGroupStart || ids[len(ids)-1] != idGroupEnd {
		t.Fatalf("group does not bracket data: ids = % X", ids)
	}
	// No data block should fall outside the group.
	for i, id := range ids {
		if id == idStandardSpeed && (i == 0 || i == len(ids)-1) {
			t.Errorf("data block at position %d is outside the group", i)
		}
	}
}

func TestBlockOrdering(t *testing.T) {
	out, err := EncodeFromTAP(sampleTAP(), EncodeOptions{
		Title:    "T",
		Hardware: []HardwareInfo{{Type: HWComputers, ID: HWIDSpectrum128K, Info: HWInfoUses}},
		Group:    "G",
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := findBlockIDs(t, out)
	// Metadata (archive, hardware) must precede the group and data.
	posOf := func(want byte) int {
		for i, id := range ids {
			if id == want {
				return i
			}
		}
		return -1
	}
	if !(posOf(idArchiveInfo) < posOf(idHardwareTyp) &&
		posOf(idHardwareTyp) < posOf(idGroupStart) &&
		posOf(idGroupStart) < posOf(idStandardSpeed) &&
		posOf(idStandardSpeed) < posOf(idGroupEnd)) {
		t.Fatalf("blocks out of spec order: % X", ids)
	}
}

func TestWriterMultiTAP(t *testing.T) {
	w := NewWriter(0)
	w.ArchiveInfo(EncodeOptions{Title: "Multi"})
	w.GroupStart("Parts")
	if err := w.AddTAP(sampleTAP()); err != nil {
		t.Fatal(err)
	}
	w.StopIn48K()
	if err := w.AddTAP(sampleTAP()); err != nil {
		t.Fatal(err)
	}
	w.GroupEnd()

	out := w.Bytes()
	blocks, err := Decode(out)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// archive + group-start + 2 data + stop + 2 data + group-end = 8 blocks.
	if len(blocks) != 8 {
		t.Fatalf("decoded %d blocks, want 8", len(blocks))
	}
}

func TestDecodeRoundTripsNewBlocks(t *testing.T) {
	out, err := EncodeFromTAP(sampleTAP(), EncodeOptions{
		Title:     "RT",
		Hardware:  []HardwareInfo{{Type: HWComputers, ID: HWIDSpectrumPlus2APlus3, Info: HWInfoUses}},
		Group:     "RoundTrip",
		StopIn48K: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(out); err != nil {
		t.Fatalf("Decode rejected an image this package produced: %v", err)
	}
}
