// Package nex reads and writes NEX files, the ZX Spectrum Next's executable
// format: a 512-byte header, an optional loading screen with its palette,
// optional copper code, then 16 KB RAM banks.
//
// The layout follows sjasmplus's SAVENEX implementation (sjasm/io_nex.cpp),
// against whose output the tests check:
//
//	header      512 bytes, "NextV1.2" or "NextV1.3"
//	palette     512 bytes, for a Layer 2, LoRes or extended loading screen
//	            unless the header says it has none
//	screens     in the order Layer 2 (49152 bytes), ULA (6912), LoRes
//	            (12288), Timex HiRes (12288), Timex HiColour (12288), then
//	            the extended screen (Layer 2 320x256 or 640x256, 81920)
//	copper      2048 bytes, if the header says so (V1.3)
//	banks       16 KB each, in the order 5, 2, 0, 1, 3, 4, 6, 7, 8, ... 111
//	appendix    any further bytes
//
// A V1.3 header can carry a CRC-32C (Castagnoli) of the file after the
// header followed by the header's first 508 bytes.
package nex

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"sort"
)

// Sizes of the file's parts.
const (
	HeaderLen  = 512
	PaletteLen = 512
	CopperLen  = 2048
	BankLen    = 16384
	MaxBanks   = 112
)

// Screen flags of header byte 10.
const (
	ScreenLayer2   = 0x01
	ScreenULA      = 0x02
	ScreenLoRes    = 0x04
	ScreenHiRes    = 0x08
	ScreenHiColour = 0x10
	ScreenExtended = 0x40 // the screen is the one header byte 152 names
	ScreenNoPal    = 0x80
)

// Extended screens, header byte 152.
const (
	Screen2Layer2_320 = 1
	Screen2Layer2_640 = 2
	Screen2Tilemap    = 3 // no screen block: the tilemap is in bank 5
)

// screenOrder is the order screen blocks follow one another, with their sizes.
var screenOrder = []struct {
	flag uint8
	size int
}{
	{ScreenLayer2, 49152}, {ScreenULA, 6912}, {ScreenLoRes, 12288},
	{ScreenHiRes, 12288}, {ScreenHiColour, 12288},
}

// bankOrder gives the position of a bank in the file.
func bankOrder(n int) int {
	switch n {
	case 5:
		return 0
	case 2:
		return 1
	case 0:
		return 2
	case 1:
		return 3
	case 3:
		return 4
	case 4:
		return 5
	}
	return n
}

// File is a NEX file.
type File struct {
	// Version is 2 or 3 for "NextV1.2" or "NextV1.3". Encode raises 2 to 3
	// when a V1.3 feature is used; 0 means the lowest that fits.
	Version          int
	RAM2MB           bool  // byte 8: needs the 2 MB machine (banks 48 and up)
	Border           uint8 // byte 11
	SP, PC           uint16
	LoadBar          bool  // byte 130
	LoadBarColour    uint8 // 131
	LoadDelay        uint8 // 132, frames after each bank
	StartDelay       uint8 // 133, frames after loading
	PreserveNextRegs bool  // 134
	CoreVersion      [3]uint8
	// HiResColour is header byte 138: for a HiRes screen the port 0xFF
	// colour bits (colour << 3); for Layer 2 320x256 and 640x256 screens,
	// their palette offset.
	HiResColour uint8
	EntryBank   uint8  // 139, mapped at 0xC000
	FileHandle  uint16 // 140: 0 close, 1 pass in BC, 0x4000+ write there
	// V1.3 fields.
	ExpBusDisable uint8 // 142: 0 disables the expansion bus, 1 leaves it
	Checksum      bool  // 143: bytes 508-511 hold the CRC-32C
	// StoredChecksum is what bytes 508-511 held when the file was read,
	// and ChecksumOK whether it matches the file. Encode always writes the
	// checksum of the file it writes.
	StoredChecksum uint32
	ChecksumOK     bool
	CLIBuffer      uint16
	CLIBufferSize  uint16
	TileConfig     [4]uint8 // NextRegs 0x6B, 0x6C, 0x6E, 0x6F for a tilemap screen
	BigL2BarY      uint8    // loading bar position for 320x256 and 640x256

	// Screens is the loading-screen flags of byte 10, without ScreenNoPal
	// (which Encode sets when Palette is nil) and ScreenExtended (set from
	// Screen2). Screen2 is byte 152.
	Screens uint8
	Screen2 uint8
	Palette []byte           // 512 bytes, or nil
	Screen  map[uint8][]byte // screen blocks by flag; the extended one under ScreenExtended
	Copper  []byte           // 2048 bytes, or nil
	Banks   map[int][]byte   // 16 KB banks by number 0-111
	After   []byte           // bytes after the last bank
}

// features13 reports whether the file uses anything only V1.3 has.
func (f *File) features13() bool {
	return f.Checksum || f.ExpBusDisable != 0 || f.CLIBuffer != 0 || f.CLIBufferSize != 0 ||
		f.Screen2 != 0 || f.Copper != nil || f.BigL2BarY != 0
}

// palettedScreen reports whether the screen flags call for a palette block.
func palettedScreen(screens uint8) bool {
	return screens&(ScreenLayer2|ScreenLoRes|ScreenExtended) != 0
}

// Encode returns the file's bytes.
func Encode(f *File) ([]byte, error) {
	version := f.Version
	if version == 0 {
		version = 2
	}
	if version != 2 && version != 3 {
		return nil, fmt.Errorf("nex: version %d, want 2 or 3", f.Version)
	}
	if f.features13() {
		if f.Version == 2 {
			return nil, fmt.Errorf("nex: V1.2 was asked for, but the file uses V1.3 features")
		}
		version = 3
	}
	h := make([]byte, HeaderLen)
	copy(h, fmt.Sprintf("NextV1.%d", version))
	if f.RAM2MB {
		h[8] = 1
	}
	screens := f.Screens &^ (ScreenNoPal | ScreenExtended)
	if f.Screen2 != 0 {
		if f.Screen2 > Screen2Tilemap {
			return nil, fmt.Errorf("nex: extended screen %d, want 1-3", f.Screen2)
		}
		screens |= ScreenExtended
	}
	if palettedScreen(screens) && f.Palette == nil {
		screens |= ScreenNoPal
	}
	if f.Palette != nil && len(f.Palette) != PaletteLen {
		return nil, fmt.Errorf("nex: palette is %d bytes, want %d", len(f.Palette), PaletteLen)
	}
	if f.Palette != nil && !palettedScreen(screens) {
		return nil, fmt.Errorf("nex: a palette goes with a Layer 2, LoRes or extended screen")
	}
	h[10] = screens
	h[11] = f.Border & 7
	binary.LittleEndian.PutUint16(h[12:], f.SP)
	binary.LittleEndian.PutUint16(h[14:], f.PC)
	var body bytes.Buffer
	if f.Palette != nil {
		body.Write(f.Palette)
	}
	for _, s := range screenOrder {
		if screens&s.flag == 0 {
			continue
		}
		d := f.Screen[s.flag]
		if len(d) != s.size {
			return nil, fmt.Errorf("nex: screen %#02x is %d bytes, want %d", s.flag, len(d), s.size)
		}
		body.Write(d)
	}
	if f.Screen2 == Screen2Layer2_320 || f.Screen2 == Screen2Layer2_640 {
		d := f.Screen[ScreenExtended]
		if len(d) != 81920 {
			return nil, fmt.Errorf("nex: the extended Layer 2 screen is %d bytes, want 81920", len(d))
		}
		body.Write(d)
	}
	if f.Copper != nil {
		if len(f.Copper) != CopperLen {
			return nil, fmt.Errorf("nex: copper code is %d bytes, want %d", len(f.Copper), CopperLen)
		}
		h[153] = 1
		body.Write(f.Copper)
	}
	banksOffset := HeaderLen + body.Len()
	nums := make([]int, 0, len(f.Banks))
	for n, d := range f.Banks {
		if n < 0 || n >= MaxBanks {
			return nil, fmt.Errorf("nex: bank %d, want 0-%d", n, MaxBanks-1)
		}
		if len(d) != BankLen {
			return nil, fmt.Errorf("nex: bank %d is %d bytes, want %d", n, len(d), BankLen)
		}
		if n >= 48 && !f.RAM2MB {
			return nil, fmt.Errorf("nex: bank %d needs the 2 MB machine; set RAM2MB", n)
		}
		nums = append(nums, n)
	}
	sort.Slice(nums, func(i, j int) bool { return bankOrder(nums[i]) < bankOrder(nums[j]) })
	for _, n := range nums {
		h[18+n] = 1
		body.Write(f.Banks[n])
	}
	h[9] = uint8(len(nums))
	body.Write(f.After)
	if f.LoadBar {
		h[130] = 1
	}
	h[131], h[132], h[133] = f.LoadBarColour, f.LoadDelay, f.StartDelay
	if f.PreserveNextRegs {
		h[134] = 1
	}
	copy(h[135:138], f.CoreVersion[:])
	h[138], h[139] = f.HiResColour, f.EntryBank
	binary.LittleEndian.PutUint16(h[140:], f.FileHandle)
	if version == 3 {
		h[142] = f.ExpBusDisable
		binary.LittleEndian.PutUint32(h[144:], uint32(banksOffset))
		binary.LittleEndian.PutUint16(h[148:], f.CLIBuffer)
		binary.LittleEndian.PutUint16(h[150:], f.CLIBufferSize)
		h[152] = f.Screen2
		copy(h[154:158], f.TileConfig[:])
		h[158] = f.BigL2BarY
	}
	out := append(h, body.Bytes()...)
	if f.Checksum {
		out[143] = 1
		binary.LittleEndian.PutUint32(out[508:], Checksum(out))
	}
	return out, nil
}

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// Checksum is the CRC-32C a V1.3 header stores: of the file from byte 512
// on, then of the header's first 508 bytes.
func Checksum(file []byte) uint32 {
	c := crc32.Update(0, castagnoli, file[HeaderLen:])
	return crc32.Update(c, castagnoli, file[:508])
}

// Decode parses a NEX file. A checksum that does not match is reported in
// ChecksumOK rather than refused: sjasmplus (1.24.0 and earlier) stores the
// checksum of the header as it stood when the file was opened, not as it is
// written at the end, so its V1.3 checksums do not match their files.
func Decode(data []byte) (*File, error) {
	if len(data) < HeaderLen {
		return nil, fmt.Errorf("nex: %d bytes is shorter than the header", len(data))
	}
	h := data[:HeaderLen]
	if string(h[:6]) != "NextV1" || (h[7] != '0' && h[7] != '1' && h[7] != '2' && h[7] != '3') || h[6] != '.' {
		return nil, fmt.Errorf("nex: not a NEX file (it begins %q)", h[:8])
	}
	f := &File{
		Version:          int(h[7] - '0'),
		RAM2MB:           h[8] != 0,
		Border:           h[11],
		SP:               binary.LittleEndian.Uint16(h[12:]),
		PC:               binary.LittleEndian.Uint16(h[14:]),
		LoadBar:          h[130] != 0,
		LoadBarColour:    h[131],
		LoadDelay:        h[132],
		StartDelay:       h[133],
		PreserveNextRegs: h[134] != 0,
		HiResColour:      h[138],
		EntryBank:        h[139],
		FileHandle:       binary.LittleEndian.Uint16(h[140:]),
		Screen:           map[uint8][]byte{},
		Banks:            map[int][]byte{},
	}
	copy(f.CoreVersion[:], h[135:138])
	screens := h[10]
	if f.Version >= 3 {
		f.ExpBusDisable = h[142]
		f.Checksum = h[143] != 0
		f.CLIBuffer = binary.LittleEndian.Uint16(h[148:])
		f.CLIBufferSize = binary.LittleEndian.Uint16(h[150:])
		if screens&ScreenExtended != 0 {
			f.Screen2 = h[152]
		}
		copy(f.TileConfig[:], h[154:158])
		f.BigL2BarY = h[158]
		if f.Checksum {
			f.StoredChecksum = binary.LittleEndian.Uint32(h[508:])
			f.ChecksumOK = Checksum(data) == f.StoredChecksum
		}
	}
	f.Screens = screens &^ (ScreenNoPal | ScreenExtended)
	off := HeaderLen
	take := func(n int, what string) ([]byte, error) {
		if len(data)-off < n {
			return nil, fmt.Errorf("nex: the file ends in the %s", what)
		}
		b := data[off : off+n]
		off += n
		return append([]byte(nil), b...), nil
	}
	var err error
	if palettedScreen(screens) && screens&ScreenNoPal == 0 {
		if f.Palette, err = take(PaletteLen, "palette"); err != nil {
			return nil, err
		}
	}
	for _, s := range screenOrder {
		if screens&s.flag != 0 {
			if f.Screen[s.flag], err = take(s.size, "loading screen"); err != nil {
				return nil, err
			}
		}
	}
	if f.Screen2 == Screen2Layer2_320 || f.Screen2 == Screen2Layer2_640 {
		if f.Screen[ScreenExtended], err = take(81920, "loading screen"); err != nil {
			return nil, err
		}
	}
	if f.Version >= 3 && h[153] != 0 {
		if f.Copper, err = take(CopperLen, "copper code"); err != nil {
			return nil, err
		}
	}
	if f.Version >= 3 {
		if bo := int(binary.LittleEndian.Uint32(h[144:])); bo != 0 && bo != off {
			return nil, fmt.Errorf("nex: banks are said to start at %d, but the blocks before them end at %d", bo, off)
		}
	}
	var nums []int
	for n := 0; n < MaxBanks; n++ {
		if h[18+n] != 0 {
			nums = append(nums, n)
		}
	}
	if len(nums) != int(h[9]) {
		return nil, fmt.Errorf("nex: the header counts %d banks but marks %d", h[9], len(nums))
	}
	sort.Slice(nums, func(i, j int) bool { return bankOrder(nums[i]) < bankOrder(nums[j]) })
	for _, n := range nums {
		if f.Banks[n], err = take(BankLen, fmt.Sprintf("bank %d", n)); err != nil {
			return nil, err
		}
	}
	if off < len(data) {
		f.After = append([]byte(nil), data[off:]...)
	}
	return f, nil
}
