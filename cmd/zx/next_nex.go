// file: next_nex.go
//
// `zx next nex`: reading, unpacking and building NEX files through pkg/nex,
// and `zx zx0`: ZX0 compression through pkg/zx0.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ha1tch/zentools/pkg/nex"
	"github.com/ha1tch/zentools/pkg/zx0"
)

var nexScreenNames = []struct {
	name    string
	flag    uint8
	screen2 uint8
	ext     string
}{
	{"l2", nex.ScreenLayer2, 0, "nxi"},
	{"ula", nex.ScreenULA, 0, "scr"},
	{"lores", nex.ScreenLoRes, 0, "slr"},
	{"hires", nex.ScreenHiRes, 0, "shr"},
	{"hicolour", nex.ScreenHiColour, 0, "shc"},
	{"l2_320", nex.ScreenExtended, nex.Screen2Layer2_320, "nxi"},
	{"l2_640", nex.ScreenExtended, nex.Screen2Layer2_640, "nxi"},
}

func readNEX(path string) (*nex.File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := nex.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

func nexBanks(f *nex.File) []int {
	var out []int
	for n := range f.Banks {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

func nexScreenList(f *nex.File) []string {
	var out []string
	for _, s := range nexScreenNames {
		if (s.screen2 == 0 && f.Screens&s.flag != 0) || (s.screen2 != 0 && f.Screen2 == s.screen2) {
			out = append(out, s.name)
		}
	}
	if f.Screen2 == nex.Screen2Tilemap {
		out = append(out, "tilemap (in bank 5)")
	}
	return out
}

func nextNexInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("nex info needs exactly one file")
	}
	f, err := readNEX(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("%s: NEX V1.%d\n", args[0], f.Version)
	fmt.Printf("  start %04X, stack %04X, bank %d at C000, border %d\n", f.PC, f.SP, f.EntryBank, f.Border)
	fmt.Printf("  core %d.%d.%d or later, %s\n", f.CoreVersion[0], f.CoreVersion[1], f.CoreVersion[2], map[bool]string{false: "768 KB", true: "2 MB machine"}[f.RAM2MB])
	scr := nexScreenList(f)
	if len(scr) == 0 {
		scr = []string{"none"}
	}
	pal := "no palette"
	if f.Palette != nil {
		pal = "with palette"
	}
	fmt.Printf("  loading screen: %s, %s\n", strings.Join(scr, ", "), pal)
	if f.LoadBar {
		fmt.Printf("  loading bar colour %d, %d frames per bank, %d after loading\n", f.LoadBarColour, f.LoadDelay, f.StartDelay)
	}
	banks := nexBanks(f)
	var bs []string
	for _, n := range banks {
		bs = append(bs, strconv.Itoa(n))
	}
	fmt.Printf("  %d banks: %s\n", len(banks), strings.Join(bs, " "))
	if f.Copper != nil {
		fmt.Println("  copper code: 2048 bytes")
	}
	if len(f.After) > 0 {
		fmt.Printf("  %d bytes after the banks\n", len(f.After))
	}
	if f.Checksum {
		state := "matches"
		if !f.ChecksumOK {
			state = "does not match (sjasmplus 1.24.0 and earlier write a checksum of a stale header)"
		}
		fmt.Printf("  CRC-32C %08X %s\n", f.StoredChecksum, state)
	}
	return nil
}

// nexHeader is the part of a NEX file nex extract writes as header.json and
// nex build reads back with -header.
type nexHeader struct {
	Version          int      `json:"version"`
	PC               uint16   `json:"pc"`
	SP               uint16   `json:"sp"`
	EntryBank        uint8    `json:"entry_bank"`
	Border           uint8    `json:"border"`
	RAM2MB           bool     `json:"ram_2mb,omitempty"`
	Core             [3]uint8 `json:"core"`
	LoadBar          bool     `json:"load_bar,omitempty"`
	LoadBarColour    uint8    `json:"load_bar_colour,omitempty"`
	LoadDelay        uint8    `json:"load_delay,omitempty"`
	StartDelay       uint8    `json:"start_delay,omitempty"`
	PreserveNextRegs bool     `json:"preserve_nextregs,omitempty"`
	HiResColour      uint8    `json:"hires_colour,omitempty"`
	FileHandle       uint16   `json:"file_handle,omitempty"`
	ExpBusDisable    uint8    `json:"exp_bus_disable,omitempty"`
	Checksum         bool     `json:"checksum,omitempty"`
	CLIBuffer        uint16   `json:"cli_buffer,omitempty"`
	CLIBufferSize    uint16   `json:"cli_buffer_size,omitempty"`
	TileConfig       [4]uint8 `json:"tile_config,omitempty"`
	BigL2BarY        uint8    `json:"big_l2_bar_y,omitempty"`
	Screen2Tilemap   bool     `json:"tilemap_screen,omitempty"`
}

func nextNexExtract(args []string) error {
	fs := flag.NewFlagSet("next nex extract", flag.ContinueOnError)
	out := fs.String("o", "", "output directory")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("nex extract needs -o dir and exactly one file")
	}
	f, err := readNEX(fs.Arg(0))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0755); err != nil {
		return err
	}
	write := func(name string, data []byte) error {
		return os.WriteFile(filepath.Join(*out, name), data, 0644)
	}
	h := nexHeader{Version: f.Version, PC: f.PC, SP: f.SP, EntryBank: f.EntryBank, Border: f.Border, RAM2MB: f.RAM2MB,
		Core: f.CoreVersion, LoadBar: f.LoadBar, LoadBarColour: f.LoadBarColour, LoadDelay: f.LoadDelay, StartDelay: f.StartDelay,
		PreserveNextRegs: f.PreserveNextRegs, HiResColour: f.HiResColour, FileHandle: f.FileHandle, ExpBusDisable: f.ExpBusDisable,
		Checksum: f.Checksum, CLIBuffer: f.CLIBuffer, CLIBufferSize: f.CLIBufferSize, TileConfig: f.TileConfig, BigL2BarY: f.BigL2BarY,
		Screen2Tilemap: f.Screen2 == nex.Screen2Tilemap}
	js, _ := json.MarshalIndent(h, "", "  ")
	if err := write("header.json", append(js, '\n')); err != nil {
		return err
	}
	n := 1
	if f.Palette != nil {
		if err := write("palette.nxp", f.Palette); err != nil {
			return err
		}
		n++
	}
	for _, s := range nexScreenNames {
		key := s.flag
		if s.screen2 != 0 {
			if f.Screen2 != s.screen2 {
				continue
			}
		} else if f.Screens&s.flag == 0 {
			continue
		}
		if err := write("screen_"+s.name+"."+s.ext, f.Screen[key]); err != nil {
			return err
		}
		n++
	}
	if f.Copper != nil {
		if err := write("copper.bin", f.Copper); err != nil {
			return err
		}
		n++
	}
	for _, b := range nexBanks(f) {
		if err := write(fmt.Sprintf("bank_%03d.bin", b), f.Banks[b]); err != nil {
			return err
		}
		n++
	}
	if len(f.After) > 0 {
		if err := write("after.bin", f.After); err != nil {
			return err
		}
		n++
	}
	fmt.Printf("wrote %d files to %s\n", n, *out)
	return nil
}

// listFlag collects a repeatable flag.
type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(s string) error { *l = append(*l, s); return nil }

func nextNexBuild(args []string) error {
	fs := flag.NewFlagSet("next nex build", flag.ContinueOnError)
	out := fs.String("o", "", "output .nex")
	header := fs.String("header", "", "header.json from nex extract, as the starting header")
	pc := fs.String("pc", "", "start address (0 returns to the caller)")
	sp := fs.String("sp", "", "stack pointer")
	entry := fs.Int("entry-bank", -1, "16 KB bank mapped at C000 on start")
	border := fs.Int("border", -1, "border colour 0-7")
	core := fs.String("core", "", "lowest core version, a.b.c")
	version := fs.Int("version", 0, "2 or 3 for V1.2 or V1.3 (default: the lowest that fits)")
	ram2 := fs.Bool("2mb", false, "needs the 2 MB machine (for banks 48 and up)")
	checksum := fs.Bool("checksum", false, "write a CRC-32C (V1.3)")
	palette := fs.String("palette", "", "512-byte .nxp for the loading screen")
	copper := fs.String("copper", "", "2048 bytes of copper code (V1.3)")
	appendF := fs.String("append", "", "file to put after the banks")
	hires := fs.Int("hires-colour", -1, "HiRes ink 0-7 (or palette offset of a 320/640 screen)")
	var banks, bins, screens listFlag
	fs.Var(&banks, "bank", "N=file: a 16 KB bank (shorter files are padded with zeros; repeatable)")
	fs.Var(&bins, "bin", "ADDR=file: load a file at a 64 KB address, banks 5, 2 and 0 at 4000, 8000 and C000 (repeatable)")
	fs.Var(&screens, "screen", "kind=file: loading screen, kind l2, l2_320, l2_640, lores, ula, hires or hicolour (repeatable)")
	if err := fs.Parse(permuteArgs(args, map[string]bool{"2mb": true, "checksum": true})); err != nil {
		return err
	}
	if fs.NArg() != 0 || *out == "" {
		return fmt.Errorf("nex build needs -o file and flags only")
	}
	f := &nex.File{CoreVersion: [3]uint8{2, 0, 28}, Screen: map[uint8][]byte{}, Banks: map[int][]byte{}}
	if *header != "" {
		data, err := os.ReadFile(*header)
		if err != nil {
			return err
		}
		var h nexHeader
		if err := json.Unmarshal(data, &h); err != nil {
			return fmt.Errorf("%s: %w", *header, err)
		}
		f.Version, f.PC, f.SP, f.EntryBank, f.Border, f.RAM2MB, f.CoreVersion = h.Version, h.PC, h.SP, h.EntryBank, h.Border, h.RAM2MB, h.Core
		f.LoadBar, f.LoadBarColour, f.LoadDelay, f.StartDelay = h.LoadBar, h.LoadBarColour, h.LoadDelay, h.StartDelay
		f.PreserveNextRegs, f.HiResColour, f.FileHandle, f.ExpBusDisable = h.PreserveNextRegs, h.HiResColour, h.FileHandle, h.ExpBusDisable
		f.Checksum, f.CLIBuffer, f.CLIBufferSize, f.TileConfig, f.BigL2BarY = h.Checksum, h.CLIBuffer, h.CLIBufferSize, h.TileConfig, h.BigL2BarY
		if h.Screen2Tilemap {
			f.Screen2 = nex.Screen2Tilemap
		}
	}
	num := func(s, what string, max int64) (int64, error) {
		v, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(s), "$"), 0, 32)
		if strings.HasPrefix(strings.TrimSpace(s), "$") {
			v, err = strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(s), "$"), 16, 32)
		}
		if err != nil || v < 0 || v > max {
			return 0, fmt.Errorf("%s %q: want 0-%d", what, s, max)
		}
		return v, nil
	}
	if *pc != "" {
		v, err := num(*pc, "-pc", 0xFFFF)
		if err != nil {
			return err
		}
		f.PC = uint16(v)
	}
	if *sp != "" {
		v, err := num(*sp, "-sp", 0xFFFF)
		if err != nil {
			return err
		}
		f.SP = uint16(v)
	}
	if *entry >= 0 {
		if *entry >= nex.MaxBanks {
			return fmt.Errorf("-entry-bank must be 0-%d", nex.MaxBanks-1)
		}
		f.EntryBank = uint8(*entry)
	}
	if *border >= 0 {
		f.Border = uint8(*border & 7)
	}
	if *core != "" {
		parts := strings.Split(*core, ".")
		if len(parts) != 3 {
			return fmt.Errorf("-core %q: want a.b.c", *core)
		}
		for i, p := range parts {
			v, err := strconv.Atoi(p)
			if err != nil || v < 0 || v > 255 {
				return fmt.Errorf("-core %q: want a.b.c", *core)
			}
			f.CoreVersion[i] = uint8(v)
		}
	}
	if *version != 0 {
		f.Version = *version
	}
	f.RAM2MB = f.RAM2MB || *ram2
	f.Checksum = f.Checksum || *checksum
	if *hires >= 0 {
		f.HiResColour = uint8(*hires)
	}
	readF := func(path string) ([]byte, error) { return os.ReadFile(path) }
	if *palette != "" {
		d, err := readF(*palette)
		if err != nil {
			return err
		}
		f.Palette = d
	}
	if *copper != "" {
		d, err := readF(*copper)
		if err != nil {
			return err
		}
		f.Copper = d
	}
	if *appendF != "" {
		d, err := readF(*appendF)
		if err != nil {
			return err
		}
		f.After = d
	}
	for _, s := range screens {
		kind, path, ok := strings.Cut(s, "=")
		if !ok {
			return fmt.Errorf("-screen %q: want kind=file", s)
		}
		d, err := readF(path)
		if err != nil {
			return err
		}
		found := false
		for _, n := range nexScreenNames {
			if n.name == strings.ToLower(kind) {
				found = true
				if n.screen2 != 0 {
					f.Screen2 = n.screen2
				} else {
					f.Screens |= n.flag
				}
				f.Screen[n.flag] = d
			}
		}
		if !found {
			return fmt.Errorf("-screen %q: kind must be l2, l2_320, l2_640, lores, ula, hires or hicolour", s)
		}
	}
	bank := func(n int) []byte {
		if f.Banks[n] == nil {
			f.Banks[n] = make([]byte, nex.BankLen)
		}
		return f.Banks[n]
	}
	for _, b := range banks {
		ns, path, ok := strings.Cut(b, "=")
		n, err := strconv.Atoi(ns)
		if !ok || err != nil || n < 0 || n >= nex.MaxBanks {
			return fmt.Errorf("-bank %q: want N=file with N 0-%d", b, nex.MaxBanks-1)
		}
		d, err := readF(path)
		if err != nil {
			return err
		}
		if len(d) > nex.BankLen {
			return fmt.Errorf("-bank %q: %d bytes is more than a 16 KB bank", b, len(d))
		}
		copy(bank(n), d)
	}
	for _, b := range bins {
		as, path, ok := strings.Cut(b, "=")
		if !ok {
			return fmt.Errorf("-bin %q: want ADDR=file", b)
		}
		addr, err := num(as, "-bin address", 0xFFFF)
		if err != nil {
			return err
		}
		if addr < 0x4000 {
			return fmt.Errorf("-bin %q: the ROM is at 0000-3FFF", b)
		}
		d, err := readF(path)
		if err != nil {
			return err
		}
		if int(addr)+len(d) > 0x10000 {
			return fmt.Errorf("-bin %q: %d bytes from %04X pass FFFF", b, len(d), addr)
		}
		if err := f.Load(uint16(addr), d); err != nil {
			return err
		}
	}
	data, err := nex.Encode(f)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, data, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (NEX V1.%c, %d banks, %d bytes)\n", *out, data[7], len(f.Banks), len(data))
	return nil
}

// --- zx zx0 -----------------------------------------------------------------

func zx0Usage() {
	fmt.Fprint(os.Stderr, `zx zx0 - ZX0 compression

Usage:
  zx zx0 [flags] [-o out] <file>

Flags:
  -d          decompress (default: compress)
  -classic    the v1 format (zx0 -c), as gfx2next writes
  -back       backwards, for in-place decompression from the end (zx0 -b)
  -quick      quick: offsets up to 2176 only, faster, compresses less (zx0 -q)
  -o <file>   output (default: the input with .zx0 added, or removed with -d)

The compressor is the reference one's optimal parse, so the bytes are the same as zx0's.
It takes time in proportion to the file's length times the offsets it searches (up to
32640): about 3 seconds for 13 KB on an ordinary machine, 45 seconds for 80 KB, and
much less with -quick.
`)
}

func cmdZX0(args []string) error {
	fs := flag.NewFlagSet("zx0", flag.ContinueOnError)
	fs.Usage = zx0Usage
	d := fs.Bool("d", false, "decompress")
	classic := fs.Bool("classic", false, "v1 format")
	back := fs.Bool("back", false, "backwards")
	quick := fs.Bool("quick", false, "quick mode")
	out := fs.String("o", "", "output")
	if err := fs.Parse(permuteArgs(args, map[string]bool{"d": true, "classic": true, "back": true, "quick": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		zx0Usage()
		return fmt.Errorf("zx0 needs exactly one file")
	}
	in, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	opt := zx0.Options{Classic: *classic, Backwards: *back, Quick: *quick}
	path := *out
	var data []byte
	if *d {
		if data, err = zx0.Decompress(in, opt, 0); err != nil {
			return fmt.Errorf("%s: %w", fs.Arg(0), err)
		}
		if path == "" {
			path = strings.TrimSuffix(fs.Arg(0), ".zx0")
			if path == fs.Arg(0) {
				path += ".out"
			}
		}
	} else {
		if data, err = zx0.Compress(in, opt); err != nil {
			return err
		}
		if path == "" {
			path = fs.Arg(0) + ".zx0"
		}
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d bytes from %d)\n", path, len(data), len(in))
	return nil
}
