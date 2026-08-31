// file: edit.go
//
// `zx edit` operates on a multi-block tape file (tap/tzx/pzx) at the level
// of its flat logical block list -- the same []tap.Block shape
// tapeBlocksFor already produces for extraction, and explodeTape already
// produces for manifest.json. list/extract are read-only; delete/import
// decode the source into that list, apply one change, and re-encode to
// whichever target format -o names (defaulting to the source's own
// format), via the same encoders convert.go and pzx.go already use --
// tzx.EncodeFromTAP, encodePZXFromTAPBlocks, or plain concatenation for
// tap. No new encoding logic exists here, only list manipulation.
//
// This works at the logical-block level deliberately, the same scope
// boundary toTAP/tapeBlocksFor already draw: a TZX's own structural
// blocks (PAUSE, GROUP START/END, STOP, TEXT DESCRIPTION, and the rest)
// and a PZX's own PAUS/BRWS/STOP blocks carry no editable "block" concept
// of their own and are dropped on re-encode, the same as they already are
// converting to TAP. Editing a tape's data content doesn't currently
// preserve that surrounding structure; only the loadable code/data blocks
// themselves round-trip.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ha1tch/zentools/pkg/tap"
	"github.com/ha1tch/zentools/pkg/tzx"
)

func cmdEdit(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, `zx edit - list, extract, delete, or import blocks in a multi-block tape

Usage:
  zx edit list <file> [--json]
  zx edit extract <file> --block N [--raw] -o out.bin
  zx edit delete <file> --block N[,M,...] -o out.tap|out.tzx|out.pzx
  zx edit import <file> --data new.bin --name NAME [--kind code|program]
                 [--org ADDR] [--autostart N] [--at N] [--raw]
                 -o out.tap|out.tzx|out.pzx
  zx edit append <file1> <file2> [<file3>...] -o out.tap|out.tzx|out.pzx

'delete'/'import'/'append' always require -o (never edit in place); the
target format is taken from -o's own extension, defaulting to the first
input file's own format if -o's extension isn't tap/tzx/pzx. This operates
on the flat logical block list only -- a TZX's own structural blocks
(pauses, group markers, stop commands) and a PZX's own PAUS/BRWS/STOP
blocks are not part of that list and don't survive a delete/import/append
round trip, the same scope boundary zx convert's own tape normalisation
already has.

'append' concatenates each input file's own block list, in the order
given, regardless of what tape format each one is -- unlike shell-level
concatenation (cat a.tap b.tap > c.tap), which only happens to work
because TAP is nothing but a bare sequence of length-prefixed blocks;
TZX and PZX both have their own container structure that a raw
concatenation would corrupt.`)
		return nil
	}
	switch args[0] {
	case "list":
		return editList(args[1:])
	case "extract":
		return editExtract(args[1:])
	case "delete":
		return editDelete(args[1:])
	case "import":
		return editImport(args[1:])
	case "append":
		return editAppend(args[1:])
	default:
		return fmt.Errorf("unknown edit subcommand %q", args[0])
	}
}

// tapBlocksToImage concatenates raw tap blocks (flag+payload+checksum,
// each length-prefixed) into a TAP image -- the same shape appendTAPBlock
// already builds one block at a time, generalised across a whole list.
func tapBlocksToImage(blocks []tap.Block) []byte {
	var out []byte
	for _, b := range blocks {
		raw := make([]byte, 0, len(b.Data)+2)
		raw = append(raw, b.Flag)
		raw = append(raw, b.Data...)
		raw = append(raw, b.Checksum)
		out = appendTAPBlock(out, raw)
	}
	return out
}

// encodeBlocksAs re-encodes a flat block list as dst ("tap", "tzx", or
// "pzx"), reusing exactly the encoders convert.go and pzx.go already use
// for every other tape-producing path -- no new encoding logic here.
// title is only meaningful for a "pzx" target (its PZXT header block);
// ignored otherwise.
func encodeBlocksAs(blocks []tap.Block, dst, title string) ([]byte, error) {
	switch dst {
	case "tap":
		return tapBlocksToImage(blocks), nil
	case "tzx":
		return tzx.EncodeFromTAP(tapBlocksToImage(blocks), tzx.EncodeOptions{})
	case "pzx":
		return encodePZXFromTAPBlocks(blocks, title)
	}
	return nil, fmt.Errorf("unreachable: tape target %q", dst)
}

// editTargetFormat picks the output format for delete/import: -o's own
// extension if it's a recognised tape format, otherwise the source
// file's own format (so `zx edit delete game.tzx --block 3 -o
// game.tzx` -- the ordinary case, editing and keeping the same
// format -- doesn't require spelling the format out twice).
func editTargetFormat(outPath, srcFormat string) (string, error) {
	if f := formatOf(outPath); f == "tap" || f == "tzx" || f == "pzx" {
		return f, nil
	}
	if srcFormat == "tap" || srcFormat == "tzx" || srcFormat == "pzx" {
		return srcFormat, nil
	}
	return "", fmt.Errorf("cannot determine a tape target format for %s (and the source isn't a recognised tape format either)", outPath)
}

func editList(args []string) error {
	fs := flag.NewFlagSet("edit list", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit the same manifest.json shape zx convert --outdir produces")
	if err := fs.Parse(permuteArgs(args, map[string]bool{"json": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: zx edit list <file> [--json]")
	}
	path := fs.Arg(0)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := formatOf(path)
	if kindOf(src) != "tape" {
		return fmt.Errorf("zx edit only operates on a tape source (tap/tzx/pzx); %s is not one", src)
	}
	blocks, err := tapeBlocksFor(data, src)
	if err != nil {
		return err
	}

	if *asJSON {
		m := manifestFor(blocks, path, src)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(m)
	}

	fmt.Printf("%s: %d block(s)\n", path, len(blocks))
	for i, b := range blocks {
		if b.IsHeader {
			// Param1 means different things per header type: a load
			// address for Code, an autostart line for Program, a
			// reserved/unused value for NumArray/CharArray -- labelling
			// it "load=" unconditionally would be actively wrong for a
			// Program header, not just imprecise.
			label := "load"
			if b.Type == tap.TypeProgram {
				label = "autostart"
			}
			fmt.Printf("  [%d] header  type=%s name=%q %s=0x%04X\n", i, tapTypeName(b.Type), b.Name, label, b.Param1)
		} else {
			fmt.Printf("  [%d] data    %d bytes  checksum_ok=%v\n", i, len(b.Data), b.ChecksumOK)
		}
	}
	return nil
}

// manifestFor builds the same tapeManifest explodeTape's own JSON output
// uses, but without writing any files -- editList --json and zx convert
// --outdir share one shape rather than two independently-maintained ones.
func manifestFor(blocks []tap.Block, sourcePath, format string) tapeManifest {
	m := tapeManifest{Source: filepath.Base(sourcePath), SourceFormat: format}
	pendingHeaderIdx := -1
	pendingHeaderName := ""
	for i, b := range blocks {
		if b.IsHeader {
			typ, dataLen, p1, p2 := b.Type, b.DataLength, b.Param1, b.Param2
			m.Blocks = append(m.Blocks, blockManifestEntry{
				Index: i, Kind: "header", Flag: b.Flag, ChecksumOK: b.ChecksumOK,
				Type: &typ, TypeName: tapTypeName(b.Type), Name: b.Name,
				DataLength: &dataLen, Param1: &p1, Param2: &p2,
			})
			pendingHeaderIdx = i
			pendingHeaderName = b.Name
			continue
		}
		entry := blockManifestEntry{Index: i, Kind: "data", Flag: b.Flag, ChecksumOK: b.ChecksumOK, Length: len(b.Data)}
		if pendingHeaderIdx == i-1 && pendingHeaderIdx >= 0 {
			idx := pendingHeaderIdx
			entry.HeaderIndex = &idx
			entry.HeaderName = pendingHeaderName
		}
		m.Blocks = append(m.Blocks, entry)
		pendingHeaderIdx = -1
	}
	return m
}

func editExtract(args []string) error {
	fs := flag.NewFlagSet("edit extract", flag.ContinueOnError)
	blockStr := fs.String("block", "", "0-based index of the block to extract (required)")
	raw := fs.Bool("raw", false, "emit the full raw block (flag+payload+checksum) instead of just the payload -- for re-importing elsewhere via zx edit import --raw")
	out := fs.String("o", "", "output file (required)")
	if err := fs.Parse(permuteArgs(args, map[string]bool{"raw": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 || *blockStr == "" || *out == "" {
		return fmt.Errorf("usage: zx edit extract <file> --block N [--raw] -o out.bin")
	}
	path := fs.Arg(0)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := formatOf(path)
	blocks, err := tapeBlocksFor(data, src)
	if err != nil {
		return err
	}
	idx, err := strconv.Atoi(*blockStr)
	if err != nil {
		return fmt.Errorf("invalid --block %q: %w", *blockStr, err)
	}
	if idx < 0 || idx >= len(blocks) {
		return fmt.Errorf("--block %d out of range (%d block(s) available)", idx, len(blocks))
	}
	b := blocks[idx]
	var result []byte
	if *raw {
		result = make([]byte, 0, len(b.Data)+2)
		result = append(result, b.Flag)
		result = append(result, b.Data...)
		result = append(result, b.Checksum)
	} else {
		result = b.Data
	}
	return writeOut(*out, result)
}

func editDelete(args []string) error {
	fs := flag.NewFlagSet("edit delete", flag.ContinueOnError)
	blockStr := fs.String("block", "", "0-based index (or comma-separated indices) to delete, all against the original numbering (required)")
	out := fs.String("o", "", "output file (required)")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *blockStr == "" || *out == "" {
		return fmt.Errorf("usage: zx edit delete <file> --block N[,M,...] -o out.tap|out.tzx|out.pzx")
	}
	path := fs.Arg(0)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := formatOf(path)
	blocks, err := tapeBlocksFor(data, src)
	if err != nil {
		return err
	}

	toDelete := map[int]bool{}
	for _, s := range strings.Split(*blockStr, ",") {
		idx, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return fmt.Errorf("invalid --block %q: %w", s, err)
		}
		if idx < 0 || idx >= len(blocks) {
			return fmt.Errorf("--block %d out of range (%d block(s) available)", idx, len(blocks))
		}
		toDelete[idx] = true
	}

	var kept []tap.Block
	for i, b := range blocks {
		if !toDelete[i] {
			kept = append(kept, b)
		}
	}
	if len(kept) == 0 {
		return fmt.Errorf("deleting block(s) %s would leave the tape with no blocks at all", *blockStr)
	}

	dst, err := editTargetFormat(*out, src)
	if err != nil {
		return err
	}
	result, err := encodeBlocksAs(kept, dst, "")
	if err != nil {
		return err
	}
	return writeOut(*out, result)
}

func editImport(args []string) error {
	fs := flag.NewFlagSet("edit import", flag.ContinueOnError)
	var (
		dataPath  = fs.String("data", "", "file whose bytes become the new block's payload (required)")
		name      = fs.String("name", "", "block name (required unless --raw)")
		kind      = fs.String("kind", "code", "code or program -- which header type to build (ignored with --raw)")
		originStr = fs.String("org", "0x8000", "load address, for --kind=code")
		autostart = fs.Int("autostart", 0x8000, "autostart line, for --kind=program (0x8000 is the conventional \"no autostart\" sentinel)")
		atStr     = fs.String("at", "", "insert before this 0-based index (default: append at the end)")
		raw       = fs.Bool("raw", false, "treat --data as an already-complete raw block (flag+payload+checksum) to insert directly, not a payload to wrap in a fresh header+data pair")
		out       = fs.String("o", "", "output file (required)")
	)
	if err := fs.Parse(permuteArgs(args, map[string]bool{"raw": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 || *dataPath == "" || *out == "" {
		return fmt.Errorf("usage: zx edit import <file> --data new.bin [--name NAME] [--kind code|program] [--org ADDR] [--at N] [--raw] -o out.tap|out.tzx|out.pzx")
	}
	if !*raw && *name == "" {
		return fmt.Errorf("--name is required unless --raw is given")
	}
	path := fs.Arg(0)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := formatOf(path)
	blocks, err := tapeBlocksFor(data, src)
	if err != nil {
		return err
	}

	payload, err := os.ReadFile(*dataPath)
	if err != nil {
		return err
	}

	var newBlocks []tap.Block
	if *raw {
		tb, err := tap.DecodeBlock(payload)
		if err != nil {
			return fmt.Errorf("--data with --raw must already be a valid flag+payload+checksum block: %w", err)
		}
		newBlocks = []tap.Block{tb}
	} else {
		var built []byte
		switch *kind {
		case "code":
			origin, err := parseAddr(*originStr)
			if err != nil {
				return fmt.Errorf("invalid --org: %w", err)
			}
			built = tap.EncodeCode(*name, payload, origin)
		case "program":
			built = tap.EncodeProgram(*name, payload, uint16(*autostart))
		default:
			return fmt.Errorf("unknown --kind %q (want code or program)", *kind)
		}
		builtBlocks, err := tap.Decode(built)
		if err != nil {
			return err
		}
		newBlocks = builtBlocks
	}

	at := len(blocks)
	if *atStr != "" {
		at, err = strconv.Atoi(*atStr)
		if err != nil {
			return fmt.Errorf("invalid --at %q: %w", *atStr, err)
		}
		if at < 0 || at > len(blocks) {
			return fmt.Errorf("--at %d out of range (0-%d)", at, len(blocks))
		}
	}
	result := make([]tap.Block, 0, len(blocks)+len(newBlocks))
	result = append(result, blocks[:at]...)
	result = append(result, newBlocks...)
	result = append(result, blocks[at:]...)

	dst, err := editTargetFormat(*out, src)
	if err != nil {
		return err
	}
	outImage, err := encodeBlocksAs(result, dst, "")
	if err != nil {
		return err
	}
	return writeOut(*out, outImage)
}

// editAppend concatenates two or more tape files' own block lists, in
// the order given, into one output file. Unlike shell-level
// concatenation, this works across any combination of tap/tzx/pzx
// inputs -- each is decoded via tapeBlocksFor (auto-detecting its own
// format), so the result is correct regardless of whether the sources
// share a format or not.
func editAppend(args []string) error {
	fs := flag.NewFlagSet("edit append", flag.ContinueOnError)
	out := fs.String("o", "", "output file (required)")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() < 2 || *out == "" {
		return fmt.Errorf("usage: zx edit append <file1> <file2> [<file3>...] -o out.tap|out.tzx|out.pzx")
	}

	var allBlocks []tap.Block
	firstFormat := ""
	for i := 0; i < fs.NArg(); i++ {
		path := fs.Arg(i)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := formatOf(path)
		if kindOf(src) != "tape" {
			return fmt.Errorf("%s is not a recognised tape format (tap/tzx/pzx)", path)
		}
		if i == 0 {
			firstFormat = src
		}
		blocks, err := tapeBlocksFor(data, src)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		allBlocks = append(allBlocks, blocks...)
	}

	dst, err := editTargetFormat(*out, firstFormat)
	if err != nil {
		return err
	}
	result, err := encodeBlocksAs(allBlocks, dst, "")
	if err != nil {
		return err
	}
	return writeOut(*out, result)
}
