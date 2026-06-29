// file: convert.go
//
// The `zx convert <source> <target>` subcommand converts between the tape
// formats (tap, tzx) and the snapshot formats (sna, z80).
//
// The formats are of two kinds. Tapes (tap, tzx) are ordered, named blocks with
// load addresses but no CPU state. Snapshots (sna, z80) are a frozen machine
// state — full RAM, registers, paging — with no block structure. Conversions
// within a kind are clean and reversible; conversions across kinds are
// asymmetric:
//
//	tap  <-> tzx   lossless (standard-speed blocks; custom-loader/structural
//	               blocks have no TAP equivalent and are dropped)
//	sna  <-> z80   lossless (same MachineState, different container)
//	tape  -> snap  needs --start (a tape carries no entry point)
//	snap  -> tape  lossy: emits RAM as a CODE block, losing registers and paging

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ha1tch/zentools/pkg/build"
	"github.com/ha1tch/zentools/pkg/snapshot"
	"github.com/ha1tch/zentools/pkg/tap"
	"github.com/ha1tch/zentools/pkg/tzx"
)

func cmdConvert(args []string) error {
	fs := flag.NewFlagSet("convert", flag.ContinueOnError)
	var (
		out    = fs.String("o", "", "output file (required)")
		startS = fs.String("start", "", "entry point, required for tape -> snapshot")
		spS    = fs.String("sp", "0xFF00", "stack pointer, for tape -> snapshot")
		model  = fs.String("model", "48k", "target model, for tape -> snapshot")
	)
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("usage: zx convert <input> -o <output>\n" +
			"the source and target formats are taken from the file extensions")
	}
	input := fs.Arg(0)
	src := formatOf(input)
	dst := formatOf(*out)
	if src == "" {
		return fmt.Errorf("unrecognised source format for %s", input)
	}
	if dst == "" {
		return fmt.Errorf("unrecognised target format for %s", *out)
	}

	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}

	srcKind, dstKind := kindOf(src), kindOf(dst)
	var result []byte

	switch {
	case srcKind == "tape" && dstKind == "tape":
		result, err = convertTapeToTape(data, src, dst)
	case srcKind == "snap" && dstKind == "snap":
		result, err = convertSnapToSnap(data, src, dst)
	case srcKind == "tape" && dstKind == "snap":
		result, err = convertTapeToSnap(data, src, dst, *startS, *spS, *model)
	case srcKind == "snap" && dstKind == "tape":
		result, err = convertSnapToTape(data, src, dst)
	default:
		return fmt.Errorf("cannot convert %s to %s", src, dst)
	}
	if err != nil {
		return err
	}
	return writeOut(*out, result)
}

// --- tape <-> tape ----------------------------------------------------------

func convertTapeToTape(data []byte, src, dst string) ([]byte, error) {
	// Normalise the source to a TAP image first.
	tapImage, err := toTAP(data, src)
	if err != nil {
		return nil, err
	}
	if dst == "tap" {
		return tapImage, nil
	}
	// dst == "tzx"
	return tzx.EncodeFromTAP(tapImage, tzx.EncodeOptions{})
}

// toTAP returns a TAP image from a tape file, converting from TZX if needed.
func toTAP(data []byte, src string) ([]byte, error) {
	if src == "tap" {
		return data, nil
	}
	// TZX -> TAP: keep standard-speed (0x10) blocks, length-prefix each.
	blocks, err := tzx.Decode(data)
	if err != nil {
		return nil, err
	}
	var out []byte
	dropped := 0
	for _, b := range blocks {
		if b.ID != 0x10 {
			dropped++
			continue
		}
		var prefix [2]byte
		binary.LittleEndian.PutUint16(prefix[:], uint16(len(b.Data)))
		out = append(out, prefix[:]...)
		out = append(out, b.Data...)
	}
	if dropped > 0 {
		fmt.Fprintf(os.Stderr, "Note: dropped %d non-standard-speed block(s) with no TAP equivalent\n", dropped)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the TZX file has no standard-speed data blocks to convert")
	}
	return out, nil
}

// --- snapshot <-> snapshot --------------------------------------------------

func convertSnapToSnap(data []byte, src, dst string) ([]byte, error) {
	state, err := decodeSnapshot(data, src)
	if err != nil {
		return nil, err
	}
	switch dst {
	case "sna":
		if state.Model.Is128KFamily() {
			return snapshot.EncodeSNA128(state)
		}
		return snapshot.EncodeSNA(state)
	case "z80":
		return snapshot.EncodeZ80v3(state)
	}
	return nil, fmt.Errorf("unreachable: snap target %q", dst)
}

// --- tape -> snapshot -------------------------------------------------------

func convertTapeToSnap(data []byte, src, dst, startS, spS, model string) ([]byte, error) {
	if startS == "" {
		return nil, fmt.Errorf("tape -> snapshot needs --start <addr>: a tape carries no entry point")
	}
	tapImage, err := toTAP(data, src)
	if err != nil {
		return nil, err
	}
	blocks, err := tap.Decode(tapImage)
	if err != nil {
		return nil, err
	}
	// Find the first CODE block: that is the machine code to place in memory.
	var code []byte
	var origin uint16
	var name string
	found := false
	for _, b := range blocks {
		if b.IsHeader && b.Type == tap.TypeCode {
			origin = b.Param1
			name = b.Name
			continue
		}
		if !b.IsHeader && found == false && origin != 0 {
			code = b.Data
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("no CODE block found in the tape to place in memory")
	}

	start, err := parseAddr(startS)
	if err != nil {
		return nil, fmt.Errorf("invalid --start: %w", err)
	}
	sp, err := parseAddr(spS)
	if err != nil {
		return nil, fmt.Errorf("invalid --sp: %w", err)
	}
	req := build.Request{
		Name:   name,
		Code:   code,
		Origin: origin,
		Start:  start,
		SP:     sp,
		Model:  build.Model(strings.ToLower(model)),
	}
	if w := req.SPWarning(); w != "" {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}
	if dst == "sna" {
		return build.EncodeSNA(req)
	}
	return build.EncodeZ80(req)
}

// --- snapshot -> tape -------------------------------------------------------

func convertSnapToTape(data []byte, src, dst string) ([]byte, error) {
	state, err := decodeSnapshot(data, src)
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(os.Stderr,
		"Warning: snapshot -> tape emits RAM as a CODE block; CPU registers and paging "+
			"are lost, so the tape is a memory dump, not a runnable program")

	// Emit the contiguous 48K user RAM (0x4000-0xFFFF) as one CODE block at
	// 0x4000. For 128K snapshots this is the currently-paged view; the other
	// banks are not represented (a tape cannot express paging).
	ram := userRAM48K(state)
	tapImage := tap.EncodeCode("memdump", ram, 0x4000)
	if dst == "tap" {
		return tapImage, nil
	}
	return tzx.EncodeFromTAP(tapImage, tzx.EncodeOptions{})
}

// userRAM48K returns the 48K of user RAM (0x4000-0xFFFF) as seen through the
// current paging: bank 5 at 0x4000, bank 2 at 0x8000, and the paged bank at
// 0xC000 (bank 0 for a 48K machine).
func userRAM48K(s *snapshot.MachineState) []byte {
	out := make([]byte, 0, 3*16384)
	out = append(out, s.Memory.RAM[5][:]...)
	out = append(out, s.Memory.RAM[2][:]...)
	pagedBank := 0
	if s.Model.Is128KFamily() {
		pagedBank = int(s.Paging.Port7FFD & 0x07)
	}
	out = append(out, s.Memory.RAM[pagedBank][:]...)
	return out
}

// --- shared helpers ---------------------------------------------------------

func decodeSnapshot(data []byte, src string) (*snapshot.MachineState, error) {
	switch src {
	case "z80":
		return snapshot.DecodeZ80(data)
	case "sna":
		if len(data) > 49179 {
			return snapshot.DecodeSNA128(data)
		}
		return snapshot.DecodeSNA(data)
	}
	return nil, fmt.Errorf("not a snapshot format: %s", src)
}

// formatOf returns the canonical format name for a path's extension.
func formatOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".tap":
		return "tap"
	case ".tzx":
		return "tzx"
	case ".sna":
		return "sna"
	case ".z80":
		return "z80"
	}
	return ""
}

// kindOf groups a format into "tape" or "snap".
func kindOf(format string) string {
	switch format {
	case "tap", "tzx":
		return "tape"
	case "sna", "z80":
		return "snap"
	}
	return ""
}
