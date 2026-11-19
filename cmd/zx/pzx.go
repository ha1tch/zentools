// file: pzx.go
//
// The `zx pzx` subcommand creates PZX images from a TAP (using the standard
// Spectrum ROM pilot/sync/bit timings the PZX spec itself documents as the
// canonical mapping for a TZX ID 0x10 block) and inspects existing PZX files.
//
// Only the standard-ROM-timing direction is implemented: PZX has no block
// concept equivalent to TAP's named header+data pairs (it's pulse-level, not
// block-level), so converting an arbitrary PZX back to TAP would need pulse-
// pattern recognition ("does this DATA block happen to use standard timing")
// this package doesn't attempt. `zx convert` doesn't list pzx as a source or
// target for the same reason -- this is a one-way `make`, not a symmetric
// conversion pair.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ha1tch/zentools/pkg/pzx"
	"github.com/ha1tch/zentools/pkg/tap"
)

func cmdPZX(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, `zx pzx - create and inspect PZX images

Usage:
  zx pzx make <input.tap> -o out.pzx
  zx pzx info <file.pzx>

'make' encodes each TAP block using the standard Spectrum ROM pilot/sync/bit
timings PZX's own spec documents as the canonical TAP-block mapping. There is
no reverse direction: PZX has no block concept equivalent to a TAP header, so
turning an arbitrary PZX back into named TAP blocks isn't attempted.`)
		return nil
	}
	switch args[0] {
	case "make":
		return pzxMake(args[1:])
	case "info":
		return pzxInfo(args[1:])
	default:
		return fmt.Errorf("unknown pzx subcommand %q", args[0])
	}
}

func pzxMake(args []string) error {
	fs := flag.NewFlagSet("pzx make", flag.ContinueOnError)
	var (
		title = fs.String("title", "", "archive title")
		out   = fs.String("o", "", "output file (required)")
	)
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("usage: zx pzx make <input.tap> -o out.pzx")
	}
	tapImage, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	blocks, err := tap.Decode(tapImage)
	if err != nil {
		return fmt.Errorf("decoding %s: %w", fs.Arg(0), err)
	}

	img, err := encodePZXFromTAPBlocks(blocks, *title)
	if err != nil {
		return err
	}
	return writeOut(*out, img)
}

// encodePZXFromTAPBlocks maps each TAP block to a PULS+DATA(+PAUS) triple
// using the standard ROM timings: pzx.EncodeStandard, which has the
// mapping and the timings since 0.16.0.
func encodePZXFromTAPBlocks(blocks []tap.Block, title string) ([]byte, error) {
	std := make([]pzx.StandardBlock, 0, len(blocks))
	for _, b := range blocks {
		// tap.Block.Data is documented as "payload between the flag and
		// the checksum" -- it explicitly excludes both. What actually
		// needs encoding here is the block's full raw bytes (flag,
		// payload, checksum), the same shape a real tape stores and
		// pzxToTAP/tapeBlocksFor (convert.go) expect to get back via
		// tap.DecodeBlock. An earlier version of this function encoded
		// b.Data directly, silently dropping 2 bytes off every block;
		// caught by a TAP -> PZX -> TAP round-trip test failing a byte
		// comparison, not by inspection -- the earlier
		// TestEncodePZXFromTAPBlocks_RoundTripsThroughRealDecoder test
		// only checked len(Data) != 0, which a 2-byte-short payload
		// still satisfies.
		raw := make([]byte, 0, len(b.Data)+2)
		raw = append(raw, b.Flag)
		raw = append(raw, b.Data...)
		raw = append(raw, b.Checksum)
		std = append(std, pzx.StandardBlock{Raw: raw, Header: b.IsHeader})
	}
	return pzx.EncodeStandard(std, title)
}

func pzxInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: zx pzx info <file.pzx>")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	f, err := pzx.Decode(data)
	if err != nil {
		return err
	}
	fmt.Printf("%s: %d blocks\n", args[0], len(f.Blocks))
	for i, block := range f.Blocks {
		switch b := block.(type) {
		case pzx.HeaderBlock:
			fmt.Printf("  [%d] PZXT  v%d.%d", i, b.Major, b.Minor)
			if b.Title != "" {
				fmt.Printf("  title=%q", b.Title)
			}
			fmt.Println()
			for _, kv := range b.Info {
				fmt.Printf("        %s: %s\n", kv.Key, kv.Value)
			}
		case pzx.PulseBlock:
			fmt.Printf("  [%d] PULS  %d pulse entries\n", i, len(b.Pulses))
		case pzx.DataBlock:
			fmt.Printf("  [%d] DATA  %d bits (%d bytes)\n", i, b.BitCount, len(b.Data))
		case pzx.PauseBlock:
			fmt.Printf("  [%d] PAUS  %d T\n", i, b.Duration)
		case pzx.BrowseBlock:
			fmt.Printf("  [%d] BRWS  %q\n", i, b.Text)
		case pzx.StopBlock:
			fmt.Printf("  [%d] STOP  only48K=%v\n", i, b.Only48K)
		case pzx.RawBlock:
			fmt.Printf("  [%d] %s  %d bytes (not decoded)\n", i, b.Tag, len(b.Data))
		}
	}
	return nil
}
