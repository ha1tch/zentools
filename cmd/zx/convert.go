// file: convert.go
//
// The `zx convert <source> <target>` subcommand converts between the tape
// formats (tap, tzx, pzx), the snapshot formats (sna, z80, szx), a flat raw
// binary (bin), and -- one direction only -- an RZX input recording's
// embedded snapshot. A second, different mode, `--outdir` in place of `-o`,
// explodes a multi-block tape into a directory of files instead of
// converting to a single one -- see explode.go.
//
// Same-format "conversion" (src == dst) is rejected outright, not treated
// as a no-op copy: zx convert is for converting between formats, and a
// silent same-format pass would risk masking an actual mistake (a typo'd
// extension) rather than surfacing it. Use cp to copy a file.
//
// The formats are of three kinds. Tapes (tap, tzx, pzx) are ordered blocks
// with load addresses but no CPU state. Snapshots (sna, z80, szx) are a
// frozen machine state — full RAM, registers, paging — with no block
// structure. bin is neither: no header, no structure, no metadata, just
// bytes -- both a valid extraction target from either kind, and a valid
// source for building either kind (supplying via --origin/--start what a
// raw binary has no way to carry itself). rzx is a fourth, one-way-only
// kind: it embeds a snapshot rather than being one, so it converts to
// bin, a snapshot format, or a tape format (via the embedded snapshot's
// state, same as a real snap -> tape conversion), but nothing converts
// to rzx (see zx rzx's own doc comment for why -- it records a real
// emulated session, not something built from a static file).
//
//	tap  <-> tzx   lossless: 0x10, 0x11 (Turbo Speed Data), and 0x14 (Pure
//	               Data) blocks all carry byte-resolved payloads and convert
//	               either way; only genuinely non-data blocks (pilot tones,
//	               pulse sequences, direct recordings, structural/metadata
//	               blocks) have nothing to convert and are dropped
//	tap  <-> pzx   lossless both ways: a PZX DATA block's bytes are already
//	               fully resolved by PZX's own decoder regardless of what
//	               pulse timing represented them physically (standard ROM
//	               timing, a turbo loader, anything else) -- only a block
//	               that doesn't parse as a real flag+payload+checksum
//	               structure at all is dropped, which is rare and genuine
//	tzx  <-> pzx   goes via tap as an intermediate; same scope as above on
//	               whichever leg touches each format
//	sna <-> z80 <-> szx   lossless (same MachineState, different container)
//	tape  -> snap  needs --start (a tape carries no entry point)
//	snap  -> tape  lossy: emits RAM as CODE block(s); CPU registers and
//	               interrupt state are lost (a tape format has no field for
//	               them at all -- nothing to extract further here). A
//	               128K-family source gets one block per RAM bank, not just
//	               whichever was paged in at the moment of the snapshot, so
//	               all memory content survives even though paging state
//	               (which bank goes where) does not
//	tape  -> bin   the selected block's raw payload, nothing else
//	snap  -> bin   the selected memory range, nothing else (no registers)
//	bin   -> tape  needs --origin; --name for the block name (default: the
//	               input file's own base name)
//	bin   -> snap  needs --origin and --start
//	rzx   -> bin   the first embedded snapshot's selected memory range
//	rzx   -> snap  the first embedded snapshot, decoded and re-encoded as
//	               dst (a no-op re-encode if dst matches what was embedded)
//	rzx   -> tape  the first embedded snapshot's user RAM as CODE block(s),
//	               same shape and same loss as snap -> tape above

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ha1tch/zentools/pkg/build"
	"github.com/ha1tch/zentools/pkg/pzx"
	"github.com/ha1tch/zentools/pkg/rzx"
	"github.com/ha1tch/zentools/pkg/snapshot"
	"github.com/ha1tch/zentools/pkg/szx"
	"github.com/ha1tch/zentools/pkg/tap"
	"github.com/ha1tch/zentools/pkg/tzx"
)

func cmdConvert(args []string) error {
	fs := flag.NewFlagSet("convert", flag.ContinueOnError)
	var (
		out       = fs.String("o", "", "output file (give exactly one of -o or --outdir)")
		outdir    = fs.String("outdir", "", "explode a multi-block tape (tap/tzx/pzx) into this directory: one .bin per data block, plus manifest.json describing every block in order")
		startS    = fs.String("start", "", "entry point, required for tape -> snapshot")
		spS       = fs.String("sp", "0xFF00", "stack pointer, for tape -> snapshot")
		model     = fs.String("model", "48k", "target model, for tape/bin -> snapshot")
		blockS    = fs.String("block", "", "tape/pzx source -> .bin: select a block by 0-based index (default: first Code-type block)")
		blockName = fs.String("block-name", "", "tape/pzx source -> .bin: select a block by exact header name, overriding --block")
		orgS      = fs.String("org", "", "snapshot/rzx source -> .bin: start address (default: the snapshot's own PC)")
		lengthS   = fs.String("length", "", "-> .bin: number of bytes to extract (default: everything available -- the whole block, or up to the top of the 64K address space for a snapshot)")
		bankS     = fs.String("bank", "", "128K-family snapshot/rzx source -> .bin: select a specific RAM bank (0-7), overriding the snapshot's own paging state")
		originS   = fs.String("origin", "0x8000", "bin source: load address (there's no tape/snapshot to read one from)")
		nameS     = fs.String("name", "", "bin source -> tape: block name (default: the input file's own base name)")
	)
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: zx convert <input> -o <output>\n" +
			"   or: zx convert <tape> --outdir <dir>\n" +
			"the source (and, for -o, target) format is taken from the file extension")
	}
	if (*out == "") == (*outdir == "") {
		return fmt.Errorf("give exactly one of -o <file> or --outdir <dir>")
	}
	input := fs.Arg(0)
	src := formatOf(input)
	if src == "" {
		return fmt.Errorf("unrecognised source format for %s", input)
	}

	if *outdir != "" {
		if kindOf(src) != "tape" {
			return fmt.Errorf("--outdir only applies to a tape source (tap/tzx/pzx); %s is not one", src)
		}
		data, err := os.ReadFile(input)
		if err != nil {
			return err
		}
		return explodeTape(data, src, input, *outdir)
	}

	dst := formatOf(*out)
	if dst == "" {
		return fmt.Errorf("unrecognised target format for %s", *out)
	}
	if src == dst {
		return fmt.Errorf("source and target are both %s -- zx convert is for converting between formats; use cp to copy a file", src)
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
	case srcKind == "tape" && dstKind == "bin":
		result, err = convertTapeToBin(data, src, *blockS, *blockName, *lengthS)
	case srcKind == "snap" && dstKind == "bin":
		result, err = convertSnapToBin(data, src, *orgS, *lengthS, *bankS)
	case srcKind == "bin" && dstKind == "tape":
		result, err = convertBinToTape(data, dst, *nameS, *originS, input)
	case srcKind == "bin" && dstKind == "snap":
		result, err = convertBinToSnap(data, dst, *nameS, *originS, *startS, *spS, *model, input)
	case srcKind == "rzx" && dstKind == "bin":
		result, err = convertRZXToBin(data, *orgS, *lengthS, *bankS)
	case srcKind == "rzx" && dstKind == "snap":
		result, err = convertRZXToSnap(data, dst)
	case srcKind == "rzx" && dstKind == "tape":
		result, err = convertRZXToTape(data, dst)
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
	// src == dst is rejected in cmdConvert before this is ever called, so
	// every path through here is a genuine cross-format conversion.
	tapImage, err := toTAP(data, src)
	if err != nil {
		return nil, err
	}
	switch dst {
	case "tap":
		return tapImage, nil
	case "tzx":
		return tzx.EncodeFromTAP(tapImage, tzx.EncodeOptions{})
	case "pzx":
		blocks, err := tap.Decode(tapImage)
		if err != nil {
			return nil, err
		}
		return encodePZXFromTAPBlocks(blocks, "")
	}
	return nil, fmt.Errorf("unreachable: tape target %q", dst)
}

// toTAP returns a TAP image from a tape file, converting from TZX or PZX if
// needed.
func toTAP(data []byte, src string) ([]byte, error) {
	if src == "tap" {
		return data, nil
	}
	if src == "pzx" {
		return pzxToTAP(data)
	}
	// TZX -> TAP: 0x10 (Standard Speed Data), 0x11 (Turbo Speed Data), and
	// 0x14 (Pure Data) all carry their payload the same way -- pkg/tzx's
	// own doc comment on Block.Data is explicit that 0x11 "holds the raw
	// payload bytes themselves, the same field 0x10 uses for its own
	// payload", and 0x14 is documented as the same shape again. All three
	// are read uniformly here; only genuinely timing-only or non-data
	// block types (0x12 pilot tone, 0x13 pulse sequence, 0x15 direct
	// recording's raw sample stream, and the structural/metadata blocks)
	// have nothing byte-shaped to extract and are dropped.
	//
	// An earlier version of this function kept only 0x10, which silently
	// dropped every turbo-loaded block on any real TZX using one -- a
	// large fraction of real commercial tapes, which is precisely why
	// turbo loaders exist in the first place (faster loading than the
	// standard ROM routine allows). The pilot/sync/bit-pulse timing
	// those blocks carry describes how the bytes were represented
	// physically on tape; it has no bearing on what the bytes are, which
	// is the same insight that fixed the equivalent PZX limitation.
	blocks, err := tzx.Decode(data)
	if err != nil {
		return nil, err
	}
	var out []byte
	dropped := 0
	for _, b := range blocks {
		switch b.ID {
		case 0x10, 0x11, 0x14:
			out = appendTAPBlock(out, b.Data)
		default:
			dropped++
		}
	}
	if dropped > 0 {
		fmt.Fprintf(os.Stderr, "Note: dropped %d block(s) with no TAP-representable data (pilot tones, pulse sequences, direct recordings, or structural/metadata blocks)\n", dropped)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the TZX file has no data blocks to convert")
	}
	return out, nil
}

// appendTAPBlock appends one TAP block (2-byte little-endian length prefix,
// then the raw flag+payload+checksum bytes) to out.
func appendTAPBlock(out []byte, blockData []byte) []byte {
	var prefix [2]byte
	binary.LittleEndian.PutUint16(prefix[:], uint16(len(blockData)))
	out = append(out, prefix[:]...)
	return append(out, blockData...)
}

// pzxToTAP reconstructs a TAP image from a PZX file's data blocks,
// reusing tapeBlocksFor's own PZX handling -- the same permissive
// decoding already used for PZX -> bin. A DATA block's payload is
// already fully-resolved bytes by the time PZX's own decoder produces
// it, regardless of what pulse timing represented them physically on
// tape (standard Spectrum ROM timing, a turbo loader, anything else) --
// pulse timing only matters for how bytes get loaded, not for what the
// bytes themselves are. The only thing that actually disqualifies a
// block is failing to parse as a valid flag+payload+checksum structure
// at all, which is tap.DecodeBlock's own job (and a vanishingly rare,
// genuinely malformed case) -- not "used non-standard timing".
//
// An earlier version of this function gated on exact pulse-duration
// equality to this package's own encoder constants (see zx pzx make's
// own doc comment, still accurate about the reverse conversion not
// being fully general in the abstract), which was needlessly strict in
// practice: it rejected any real, well-formed PZX file that didn't
// happen to share this package's own arbitrary timing choices --
// including genuinely valid turbo-loader captures, whose data is just
// as real and just as extractable as a standard-speed block's.
func pzxToTAP(data []byte) ([]byte, error) {
	blocks, err := tapeBlocksFor(data, "pzx")
	if err != nil {
		return nil, err
	}
	var out []byte
	for _, b := range blocks {
		raw := make([]byte, 0, len(b.Data)+2)
		raw = append(raw, b.Flag)
		raw = append(raw, b.Data...)
		raw = append(raw, b.Checksum)
		out = appendTAPBlock(out, raw)
	}
	return out, nil
}

// --- snapshot <-> snapshot --------------------------------------------------

func convertSnapToSnap(data []byte, src, dst string) ([]byte, error) {
	state, err := decodeSnapshot(data, src)
	if err != nil {
		return nil, err
	}
	warnIfPort1FFDWillBeLost(state, dst)
	return encodeSnap(state, dst)
}

// warnIfPort1FFDWillBeLost reports to stderr when state carries a real
// (non-zero) port 1FFD value and dst is "sna" -- SNA's own 128K format
// has no field for it at all (confirmed by reading pkg/snapshot's own
// SNA encoder/decoder, not assumed -- neither ever touches
// Paging.Port1FFD). Port7FFD, the actual bank selection, is unaffected;
// this is narrowly the +3's secondary paging port. Nothing to do about
// the loss itself -- there is no spare field in the documented .sna
// format to smuggle it into without breaking every other reader of the
// format -- but a value that's about to be silently dropped should say
// so rather than disappear without a trace. Shared by every path that
// can reach an "sna" target with a state that didn't just come from
// SNA itself: convertSnapToSnap and convertRZXToSnap.
func warnIfPort1FFDWillBeLost(state *snapshot.MachineState, dst string) {
	if dst == "sna" && state.Paging.Port1FFD != 0 {
		fmt.Fprintf(os.Stderr,
			"Warning: port 1FFD is %#02x; the .sna format has no field for it and this "+
				"value will not survive the conversion (port 7FFD is unaffected)\n",
			state.Paging.Port1FFD)
	}
}

// encodeSnap encodes state as dst ("sna", "z80", or "szx"). Shared by
// every path that ends in a snapshot target, regardless of what the
// source was (another snapshot, a tape, a raw binary, or an RZX's
// embedded snapshot).
func encodeSnap(state *snapshot.MachineState, dst string) ([]byte, error) {
	switch dst {
	case "sna":
		if state.Model.Is128KFamily() {
			return snapshot.EncodeSNA128(state)
		}
		return snapshot.EncodeSNA(state)
	case "z80":
		return snapshot.EncodeZ80v3(state)
	case "szx":
		return szx.Encode(state)
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

	req, err := buildSnapRequest(name, code, fmt.Sprintf("%#04x", origin), startS, spS, model)
	if err != nil {
		return nil, err
	}
	state, err := build.Overlay(req)
	if err != nil {
		return nil, err
	}
	return encodeSnap(state, dst)
}

// buildSnapRequest parses origin/start/sp and assembles a build.Request --
// shared by convertTapeToSnap (name/code/origin come from a decoded tape
// block) and convertBinToSnap (name/code/origin come from the raw file and
// --name/--origin directly).
func buildSnapRequest(name string, code []byte, originS, startS, spS, model string) (build.Request, error) {
	if startS == "" {
		return build.Request{}, fmt.Errorf("--start <addr> is required")
	}
	origin, err := parseAddr(originS)
	if err != nil {
		return build.Request{}, fmt.Errorf("invalid --origin: %w", err)
	}
	start, err := parseAddr(startS)
	if err != nil {
		return build.Request{}, fmt.Errorf("invalid --start: %w", err)
	}
	sp, err := parseAddr(spS)
	if err != nil {
		return build.Request{}, fmt.Errorf("invalid --sp: %w", err)
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
	return req, nil
}

// --- snapshot -> tape -------------------------------------------------------

func convertSnapToTape(data []byte, src, dst string) ([]byte, error) {
	state, err := decodeSnapshot(data, src)
	if err != nil {
		return nil, err
	}
	return stateToTape(state, dst)
}

// stateToTape emits state's user RAM as a single CODE tape block, encoded
// as dst. Shared by convertSnapToTape (state comes from decoding the
// source file directly) and convertRZXToTape (state comes from an RZX's
// embedded snapshot) -- the same split already used for
// convertSnapToBin/flattenSnapMemory.
func stateToTape(state *snapshot.MachineState, dst string) ([]byte, error) {
	fmt.Fprintln(os.Stderr,
		"Warning: snapshot -> tape emits RAM as CODE block(s); CPU registers and "+
			"interrupt state are lost, so the tape is a memory dump, not a runnable program")

	// The contiguous 48K user-RAM view (banks 5, 2, and whichever bank is
	// currently paged at 0xC000) as one CODE block at 0x4000 -- unchanged
	// from before, the quick-reference shape for the common case.
	tapImage := tap.EncodeCode("memdump", userRAM48K(state), 0x4000)

	// For a 128K-family source, additionally emit every individual RAM
	// bank as its own named block, plus which one was actually paged in.
	// The memdump above can only ever show whichever one bank happened
	// to be paged in at 0xC000 the moment the snapshot was taken -- the
	// other seven were previously not represented on the tape at all,
	// silently gone. Nominal load address 0xC000 for all eight (matching
	// zendis's own --bank convention): that's the only real address
	// range any one of them could ever occupy when paged in, even though
	// they can't all be resident there at once -- the block name, not
	// the load address, is what actually identifies which bank this is.
	//
	// The PAGING block closes what was still a real gap after that: all
	// eight banks' *content* was recoverable, but which one was actually
	// active -- the paging register state itself -- had nowhere to go.
	// A tape block has no field for it, but it doesn't need one: a tiny
	// dedicated 2-byte block (Port7FFD, then Port1FFD) is unambiguous
	// and easy for anything reading the tape back to find, the same way
	// TZX's own metadata blocks work. Unsophisticated consumers can
	// simply ignore it and use memdump/BANK0-7 exactly as before.
	if state.Model.Is128KFamily() {
		for bank := 0; bank < 8; bank++ {
			name := fmt.Sprintf("BANK%d", bank)
			tapImage = append(tapImage, tap.EncodeCode(name, state.Memory.RAM[bank][:], 0xC000)...)
		}
		paging := []byte{state.Paging.Port7FFD, state.Paging.Port1FFD}
		tapImage = append(tapImage, tap.EncodeCode("PAGING", paging, 0)...)
	}

	switch dst {
	case "tap":
		return tapImage, nil
	case "tzx":
		return tzx.EncodeFromTAP(tapImage, tzx.EncodeOptions{})
	case "pzx":
		blocks, err := tap.Decode(tapImage)
		if err != nil {
			return nil, err
		}
		return encodePZXFromTAPBlocks(blocks, "")
	}
	return nil, fmt.Errorf("unreachable: tape target %q", dst)
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
	case "szx":
		return szx.Decode(data)
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
	case ".pzx":
		return "pzx"
	case ".sna":
		return "sna"
	case ".z80":
		return "z80"
	case ".szx":
		return "szx"
	case ".bin":
		return "bin"
	case ".rzx":
		return "rzx"
	}
	return ""
}

// kindOf groups a format into "tape", "snap", "bin", or "rzx".
func kindOf(format string) string {
	switch format {
	case "tap", "tzx", "pzx":
		return "tape"
	case "sna", "z80", "szx":
		return "snap"
	case "bin":
		return "bin"
	case "rzx":
		return "rzx"
	}
	return ""
}

// --- tape/pzx -> .bin (plain extraction) ------------------------------------

// convertTapeToBin extracts one block's raw payload from a tap/tzx/pzx
// source as a flat binary: no length prefix, no flag byte, no checksum --
// just the code or data itself. blockS/blockName select which block, the
// same way pkg/load's TapeOptions does in the sibling zendis project (kept
// consistent deliberately, not reinvented): blockName takes precedence,
// blockS is a 0-based index into the block list, and if neither is given
// the first Code-type header's block is used.
func convertTapeToBin(data []byte, src string, blockS, blockName, lengthS string) ([]byte, error) {
	blocks, err := tapeBlocksFor(data, src)
	if err != nil {
		return nil, err
	}
	payload, err := selectBlockPayload(blocks, blockS, blockName)
	if err != nil {
		return nil, err
	}
	return clampLength(payload, lengthS)
}

// tapeBlocksFor returns src's blocks as []tap.Block regardless of which
// tape format it actually is. For pzx, each DataBlock's own payload is
// decoded via tap.DecodeBlock directly -- unlike pzxToTAP, this does not
// require standard timing first: a DataBlock's Data is already the raw
// flag+payload+checksum bytes regardless of what pulse timing represents
// them physically, so extraction can be more permissive than reconstructing
// a loadable TAP has to be.
func tapeBlocksFor(data []byte, src string) ([]tap.Block, error) {
	if src == "pzx" {
		f, err := pzx.Decode(data)
		if err != nil {
			return nil, err
		}
		var blocks []tap.Block
		for _, block := range f.Blocks {
			db, ok := block.(pzx.DataBlock)
			if !ok {
				continue
			}
			tb, err := tap.DecodeBlock(db.Data)
			if err != nil {
				continue // not a TAP-shaped payload (e.g. a non-standard-timing block's own bytes) -- not selectable, not an error for the file as a whole
			}
			blocks = append(blocks, tb)
		}
		if len(blocks) == 0 {
			return nil, fmt.Errorf("no TAP-shaped blocks found in this PZX file's DATA blocks")
		}
		return blocks, nil
	}
	tapImage, err := toTAP(data, src)
	if err != nil {
		return nil, err
	}
	return tap.Decode(tapImage)
}

// selectBlockPayload picks one block from blocks per blockS (index) /
// blockName, mirroring pkg/load's TapeOptions selection in zendis: a
// header's own following data block is what actually gets selected, a
// bare data block with no header is selectable by index but has no name.
func selectBlockPayload(blocks []tap.Block, blockS, blockName string) ([]byte, error) {
	switch {
	case blockName != "":
		for i, b := range blocks {
			if b.IsHeader && strings.TrimRight(b.Name, " ") == strings.TrimRight(blockName, " ") {
				return dataFollowing(blocks, i)
			}
		}
		return nil, fmt.Errorf("no block named %q found", blockName)

	case blockS != "":
		idx, err := strconv.Atoi(blockS)
		if err != nil {
			return nil, fmt.Errorf("invalid --block %q: %w", blockS, err)
		}
		if idx < 0 || idx >= len(blocks) {
			return nil, fmt.Errorf("--block %d out of range (%d block(s) available)", idx, len(blocks))
		}
		if blocks[idx].IsHeader {
			return dataFollowing(blocks, idx)
		}
		return blocks[idx].Data, nil

	default:
		for i, b := range blocks {
			if b.IsHeader && b.Type == tap.TypeCode {
				return dataFollowing(blocks, i)
			}
		}
		return nil, fmt.Errorf("no Code-type block found; use --block or --block-name to select one explicitly")
	}
}

func dataFollowing(blocks []tap.Block, headerIdx int) ([]byte, error) {
	if headerIdx+1 >= len(blocks) || blocks[headerIdx+1].IsHeader {
		return nil, fmt.Errorf("header block %d has no following data block", headerIdx)
	}
	return blocks[headerIdx+1].Data, nil
}

// clampLength truncates payload to lengthS bytes if given, erroring if
// lengthS asks for more than payload actually has -- silently truncating a
// mistaken over-request would hide it rather than surface it.
func clampLength(payload []byte, lengthS string) ([]byte, error) {
	if lengthS == "" {
		return payload, nil
	}
	n, err := strconv.Atoi(lengthS)
	if err != nil {
		return nil, fmt.Errorf("invalid --length %q: %w", lengthS, err)
	}
	if n < 0 || n > len(payload) {
		return nil, fmt.Errorf("--length %d exceeds the %d bytes available", n, len(payload))
	}
	return payload[:n], nil
}

// --- snapshot -> .bin (plain extraction) ------------------------------------

// convertSnapToBin extracts a flat memory range from a sna/z80/szx source:
// no registers, no paging metadata, just bytes -- starting at org (default:
// the snapshot's own PC, matching zendis's pkg/load convention) for length
// bytes (default: everything up to the top of the 64K address space).
// bankS, for a 128K-family snapshot, selects a specific RAM bank to appear
// at 0xC000 instead of whatever the snapshot's own paging state has there.
func convertSnapToBin(data []byte, src, orgS, lengthS, bankS string) ([]byte, error) {
	state, err := decodeSnapshot(data, src)
	if err != nil {
		return nil, err
	}
	return flattenSnapMemory(state, orgS, lengthS, bankS)
}

// flattenSnapMemory extracts a flat memory range from an already-decoded
// state -- shared by convertSnapToBin (state comes from decoding the
// source file directly) and convertRZXToBin (state comes from decoding
// an RZX's embedded snapshot).
func flattenSnapMemory(state *snapshot.MachineState, orgS, lengthS, bankS string) ([]byte, error) {
	var bank *int
	if bankS != "" {
		b, err := strconv.Atoi(bankS)
		if err != nil {
			return nil, fmt.Errorf("invalid --bank %q: %w", bankS, err)
		}
		if b < 0 || b > 7 {
			return nil, fmt.Errorf("--bank must be 0-7, got %d", b)
		}
		bank = &b
	}
	if bank != nil && !state.Model.Is128KFamily() {
		return nil, fmt.Errorf("--bank only applies to a 128K-family snapshot")
	}

	pagedBank := 0
	if state.Model.Is128KFamily() {
		pagedBank = int(state.Paging.Port7FFD & 0x07)
		if bank != nil {
			pagedBank = *bank
		}
	}

	var org uint16
	switch {
	case orgS != "":
		v, err := parseAddr(orgS)
		if err != nil {
			return nil, fmt.Errorf("invalid --org: %w", err)
		}
		org = v
	case bank != nil:
		org = 0xC000 // no meaningful PC-based default when explicitly inspecting an arbitrary bank
	default:
		org = state.CPU.PC
	}
	if org < 0x4000 {
		return nil, fmt.Errorf("--org %#04x is in ROM (0x0000-0x3FFF), which is never captured in a snapshot", org)
	}
	if bank != nil && org < 0xC000 {
		return nil, fmt.Errorf("--org %#04x is outside 0xC000-0xFFFF, the only range an explicitly selected --bank can occupy", org)
	}

	var flat [0x10000]byte
	copy(flat[0x4000:0x8000], state.Memory.RAM[5][:])
	copy(flat[0x8000:0xC000], state.Memory.RAM[2][:])
	copy(flat[0xC000:0x10000], state.Memory.RAM[pagedBank][:])

	return clampLength(flat[org:], lengthS)
}

// --- raw binary -> tape/snapshot ---------------------------------------------
//
// The reverse of tape/snap -> .bin: wrapping a flat binary that has no
// header, no load address, and no entry point of its own into a real
// format, using --origin (and, for a snapshot target, --start) to supply
// what the file itself can't. This is the same job zx tap make / zx snap
// make already do as dedicated commands; wiring bin in as a source here
// makes it reachable through zx convert's own dispatch too, for symmetry
// with tape/snap -> bin.

func convertBinToTape(data []byte, dst, nameS, originS, inputPath string) ([]byte, error) {
	origin, err := parseAddr(originS)
	if err != nil {
		return nil, fmt.Errorf("invalid --origin: %w", err)
	}
	name := nameS
	if name == "" {
		name = baseName(inputPath)
	}
	req := build.Request{Name: name, Code: data, Origin: origin}

	switch dst {
	case "tap":
		return build.EncodeTAP(req), nil
	case "tzx":
		return build.EncodeTZX(req)
	case "pzx":
		blocks, err := tap.Decode(build.EncodeTAP(req))
		if err != nil {
			return nil, err
		}
		return encodePZXFromTAPBlocks(blocks, "")
	}
	return nil, fmt.Errorf("unreachable: tape target %q", dst)
}

func convertBinToSnap(data []byte, dst, nameS, originS, startS, spS, model, inputPath string) ([]byte, error) {
	name := nameS
	if name == "" {
		name = baseName(inputPath)
	}
	req, err := buildSnapRequest(name, data, originS, startS, spS, model)
	if err != nil {
		return nil, err
	}
	state, err := build.Overlay(req)
	if err != nil {
		return nil, err
	}
	return encodeSnap(state, dst)
}

// --- RZX -> .bin / snapshot (extracting an embedded snapshot) ---------------
//
// An RZX has no `make` (see zx rzx's own doc comment: it records a real
// emulated session, which isn't something to construct from a static
// file), but going the other way -- pulling a snapshot back out of one --
// is a real, well-scoped operation with real prior art: fuse-emulator-
// utils' rzxtool does the equivalent extraction. A multiload recording
// can carry more than one embedded Snapshot block; the first is used, the
// same way a tape's first Code block is the sensible default elsewhere in
// this file.

func convertRZXToBin(data []byte, orgS, lengthS, bankS string) ([]byte, error) {
	state, err := firstEmbeddedSnapshot(data)
	if err != nil {
		return nil, err
	}
	return flattenSnapMemory(state, orgS, lengthS, bankS)
}

func convertRZXToSnap(data []byte, dst string) ([]byte, error) {
	state, err := firstEmbeddedSnapshot(data)
	if err != nil {
		return nil, err
	}
	warnIfPort1FFDWillBeLost(state, dst)
	return encodeSnap(state, dst)
}

// convertRZXToTape emits the first embedded snapshot's user RAM as a
// tape, the same way convertSnapToTape does for a real snapshot source
// -- registers and paging are lost, same warning, same limitation. This
// was the one genuinely buildable gap left after RZX -> bin/snap
// shipped: nothing new to invent, just wiring stateToTape's existing
// logic to a state that happens to come from an RZX instead of a
// standalone snapshot file.
func convertRZXToTape(data []byte, dst string) ([]byte, error) {
	state, err := firstEmbeddedSnapshot(data)
	if err != nil {
		return nil, err
	}
	return stateToTape(state, dst)
}

// firstEmbeddedSnapshot decodes an RZX file and returns the MachineState
// from its first Snapshot block. An External snapshot block (a reference
// to a snapshot file rather than the embedded bytes themselves -- see
// pkg/rzx's SnapshotBlock.External doc) has nothing to decode here and is
// rejected with a clear error rather than silently producing an empty
// result.
func firstEmbeddedSnapshot(data []byte) (*snapshot.MachineState, error) {
	f, err := rzx.Decode(data)
	if err != nil {
		return nil, err
	}
	for _, block := range f.Blocks {
		sb, ok := block.(rzx.SnapshotBlock)
		if !ok {
			continue
		}
		if sb.External {
			return nil, fmt.Errorf("this RZX's snapshot block is external (a filename reference, not embedded data) -- nothing to extract")
		}
		ext := strings.ToLower(sb.Extension)
		switch ext {
		case "sna":
			if len(sb.Data) > 49179 {
				return snapshot.DecodeSNA128(sb.Data)
			}
			return snapshot.DecodeSNA(sb.Data)
		case "z80":
			return snapshot.DecodeZ80(sb.Data)
		case "szx":
			return szx.Decode(sb.Data)
		default:
			return nil, fmt.Errorf("embedded snapshot has extension %q, not a format this package decodes", sb.Extension)
		}
	}
	return nil, fmt.Errorf("this RZX has no embedded snapshot block")
}
