package nex

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"reflect"
	"sort"
	"testing"
)

func load(t *testing.T, name string) []byte {
	t.Helper()
	f, err := os.Open("testdata/" + name + ".nex.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func bankList(f *File) []int {
	var out []int
	for n := range f.Banks {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// TestAgainstSjasmplus reads the files sjasmplus wrote from the .asm sources
// in testdata, checks what they hold, and requires Encode to write them back
// byte for byte.
func TestAgainstSjasmplus(t *testing.T) {
	for _, c := range []struct {
		name    string
		version int
		screens uint8
		screen2 uint8
		palette bool
		banks   []int
		check   func(*File) bool
	}{
		{"v12_l2", 2, ScreenLayer2, 0, true, []int{0, 2, 5, 7}, func(f *File) bool {
			return f.Border == 4 && f.FileHandle == 1 && f.EntryBank == 2 && f.LoadBar && f.LoadBarColour == 0xE0 &&
				f.StartDelay == 5 && f.LoadDelay == 2 && f.CoreVersion == [3]uint8{3, 1, 10} && f.SP == 0xFFFE && f.Palette[1] == 1 && f.Palette[3] == 1
		}},
		{"v13_320", 3, 0, Screen2Layer2_320, true, []int{2, 4, 5, 50}, func(f *File) bool {
			return f.RAM2MB && f.Checksum && f.ExpBusDisable == 1 && f.CLIBuffer == 0x6000 && f.CLIBufferSize == 128 &&
				f.BigL2BarY == 200 && f.PreserveNextRegs && f.FileHandle == 0x4000 && len(f.Copper) == CopperLen &&
				bytes.Equal(f.After, mustReadFile(t, "testdata/common.i"))
		}},
		{"scr_ula", 2, ScreenULA, 0, false, []int{1}, nil},
		{"shr", 3, ScreenHiRes, 0, false, []int{5}, func(f *File) bool { return f.HiResColour == 5<<3 }},
		{"tile", 3, 0, Screen2Tilemap, false, []int{3, 5}, func(f *File) bool { return f.TileConfig == [4]uint8{0xA1, 0, 0x6C, 0x0C} }},
		{"lores", 2, ScreenLoRes, 0, false, nil, nil},
	} {
		data := load(t, c.name)
		f, err := Decode(data)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if f.Version != c.version || f.Screens != c.screens || f.Screen2 != c.screen2 || (f.Palette != nil) != c.palette {
			t.Errorf("%s: version %d, screens %#02x/%d, palette %v", c.name, f.Version, f.Screens, f.Screen2, f.Palette != nil)
		}
		if got := bankList(f); !reflect.DeepEqual(got, c.banks) && !(len(got) == 0 && len(c.banks) == 0) {
			t.Errorf("%s: banks %v, want %v", c.name, got, c.banks)
		}
		for n, b := range f.Banks {
			if b[4] != uint8(n) || b[7] != 0x5A { // bytes 0-1 of bank 2 are the code
				t.Errorf("%s: bank %d holds the wrong data", c.name, n)
			}
		}
		if c.check != nil && !c.check(f) {
			h := *f
			h.Palette, h.Screen, h.Copper, h.Banks, h.After = nil, nil, nil, nil, nil
			t.Errorf("%s: header fields %+v", c.name, h)
		}
		back, err := Encode(f)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if f.Checksum {
			// sjasmplus's checksum is of a stale header (see Decode); ours
			// must verify, and everything else must match.
			if again, err := Decode(back); err != nil || !again.ChecksumOK {
				t.Errorf("%s: the re-encoded checksum does not verify (%v)", c.name, err)
			}
			copy(back[508:512], data[508:512])
		}
		if !bytes.Equal(back, data) {
			for i := range data {
				if i >= len(back) || back[i] != data[i] {
					t.Errorf("%s: re-encoded file differs from byte %d (%d bytes, sjasmplus %d)", c.name, i, len(back), len(data))
					break
				}
			}
		}
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	d, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestChecksumAndRefusals(t *testing.T) {
	data := load(t, "v13_320")
	f, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	// sjasmplus checksums the header it wrote at OPEN: "NextV1.2", the
	// start, stack and entry bank, core 2.0.28, nothing else.
	open := make([]byte, HeaderLen)
	copy(open, "NextV1.2")
	open[12], open[13], open[14], open[15], open[139] = 0xF0, 0xBF, 0x00, 0x80, 5
	open[135], open[136], open[137] = 2, 0, 28
	if f.ChecksumOK || f.StoredChecksum != Checksum(append(open, data[HeaderLen:]...)) {
		t.Errorf("sjasmplus checksum %08X: not the stale-header one", f.StoredChecksum)
	}
	ours, _ := Encode(f)
	ours[600] ^= 1
	if g, err := Decode(ours); err != nil || g.ChecksumOK {
		t.Error("a corrupted file passed its checksum")
	}
	bank := make([]byte, BankLen)
	if _, err := Encode(&File{Banks: map[int][]byte{60: bank}}); err == nil {
		t.Error("bank 60 accepted without RAM2MB")
	}
	if _, err := Encode(&File{Version: 2, Checksum: true}); err == nil {
		t.Error("a V1.2 file with a checksum accepted")
	}
	if _, err := Encode(&File{Screens: ScreenULA, Screen: map[uint8][]byte{ScreenULA: make([]byte, 100)}}); err == nil {
		t.Error("a short ULA screen accepted")
	}
	three, err := Encode(&File{Banks: map[int][]byte{5: bank, 0: bank, 2: bank}})
	if err != nil || len(three) != HeaderLen+3*BankLen || three[9] != 3 {
		t.Errorf("three banks: %v, %d bytes", err, len(three))
	}
}
