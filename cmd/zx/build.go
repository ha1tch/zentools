// file: build.go
//
// `zx build <spec.json> -o out.tap|out.tzx|out.pzx` builds a multi-block
// tape file from a declarative JSON list of blocks -- the "describe the
// whole tape at once" counterpart to zx edit's incremental list/import/
// delete. Reuses the same []tap.Block representation and the same
// encodeBlocksAs encoder edit.go already has; the only new logic here is
// parsing the spec and turning each entry into blocks via tap.EncodeCode/
// EncodeProgram/DecodeBlock, exactly the primitives zx edit import already
// uses for a single block.
//
// Deliberately JSON, not the YAML format an older, since-lost tool in this
// project's own history apparently used: a fresh design, not an attempt to
// reconstruct or emulate whatever that one did.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ha1tch/zentools/pkg/tap"
)

// buildSpec is the top-level JSON document zx build reads.
type buildSpec struct {
	// Title is used only when the target format is pzx (its PZXT header
	// block's own title field); ignored for tap/tzx targets.
	Title  string       `json:"title,omitempty"`
	Blocks []buildBlock `json:"blocks"`
}

// buildBlock is one entry in a buildSpec's block list. File is always
// required, resolved relative to the spec file's own directory, not the
// current working directory -- so a spec and its data files can be moved
// together as a unit.
type buildBlock struct {
	// Kind is "code", "program", or "raw". code/program build a fresh
	// header+data pair via tap.EncodeCode/EncodeProgram, the same as zx
	// edit import without --raw. raw inserts File's own bytes directly,
	// which must already be a complete flag+payload+checksum block, the
	// same as zx edit import --raw.
	Kind string `json:"kind"`
	// Name is required for kind=code/program; ignored for kind=raw.
	Name string `json:"name,omitempty"`
	// Org is the load address, required for kind=code; ignored otherwise.
	// A JSON number (decimal), not a "0x..." string -- keeping the spec
	// format itself simple rather than teaching it the CLI's own hex-
	// string flag convention too.
	Org *int `json:"org,omitempty"`
	// Autostart is the BASIC auto-run line, for kind=program only.
	// Omitted (or null) defaults to 0x8000, the conventional "no
	// autostart line" sentinel -- the same default zx edit import uses.
	Autostart *int `json:"autostart,omitempty"`
	// File holds this block's payload bytes (kind=code/program) or, for
	// kind=raw, the complete already-encoded block.
	File string `json:"file"`
}

func cmdBuild(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, `zx build - build a multi-block tape file from a JSON specification

Usage:
  zx build <spec.json> -o out.tap|out.tzx|out.pzx

Spec format:
  {
    "title": "optional, pzx target only",
    "blocks": [
      {"kind": "code", "name": "LOADER", "org": 32768, "file": "loader.bin"},
      {"kind": "program", "name": "BASIC", "autostart": 10, "file": "basic.bin"},
      {"kind": "raw", "file": "already_encoded_block.bin"}
    ]
  }

"file" paths are resolved relative to the spec file's own directory, not
the current working directory. "org" is a plain JSON number (decimal),
not a "0x..." string. "autostart", for kind=program, defaults to 0x8000
(the conventional "no autostart line" sentinel) if omitted.`)
		return nil
	}
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	out := fs.String("o", "", "output file (required; target format taken from its own extension: .tap, .tzx, or .pzx)")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("usage: zx build <spec.json> -o out.tap|out.tzx|out.pzx")
	}

	dst := formatOf(*out)
	if dst != "tap" && dst != "tzx" && dst != "pzx" {
		return fmt.Errorf("unrecognised target format for %s (want .tap, .tzx, or .pzx)", *out)
	}

	specPath := fs.Arg(0)
	specData, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}
	var spec buildSpec
	if err := json.Unmarshal(specData, &spec); err != nil {
		return fmt.Errorf("parsing %s: %w", specPath, err)
	}
	if len(spec.Blocks) == 0 {
		return fmt.Errorf("%s has no blocks", specPath)
	}

	specDir := filepath.Dir(specPath)
	var blocks []tap.Block
	for i, bb := range spec.Blocks {
		built, err := buildOneBlock(bb, specDir)
		if err != nil {
			return fmt.Errorf("block %d: %w", i, err)
		}
		blocks = append(blocks, built...)
	}

	result, err := encodeBlocksAs(blocks, dst, spec.Title)
	if err != nil {
		return err
	}
	return writeOut(*out, result)
}

// buildOneBlock turns one buildBlock spec entry into the tap.Block(s) it
// represents -- one header+data pair for code/program, or a single block
// for raw. Shares its three cases with zx edit import's own --kind/--raw
// handling exactly; kept separate rather than factored into one function
// both call, since the two have different input shapes (a parsed JSON
// struct here, individual CLI flags there) that would need translating
// into a common shape either way -- not enough duplication to justify the
// indirection.
func buildOneBlock(bb buildBlock, specDir string) ([]tap.Block, error) {
	if bb.File == "" {
		return nil, fmt.Errorf(`"file" is required`)
	}
	payload, err := os.ReadFile(filepath.Join(specDir, bb.File))
	if err != nil {
		return nil, err
	}

	switch bb.Kind {
	case "code":
		if bb.Name == "" {
			return nil, fmt.Errorf(`"name" is required for kind=code`)
		}
		if bb.Org == nil {
			return nil, fmt.Errorf(`"org" is required for kind=code`)
		}
		if *bb.Org < 0 || *bb.Org > 0xFFFF {
			return nil, fmt.Errorf(`"org" %d is outside the 16-bit address range`, *bb.Org)
		}
		return tap.Decode(tap.EncodeCode(bb.Name, payload, uint16(*bb.Org)))

	case "program":
		if bb.Name == "" {
			return nil, fmt.Errorf(`"name" is required for kind=program`)
		}
		autostart := 0x8000
		if bb.Autostart != nil {
			autostart = *bb.Autostart
		}
		if autostart < 0 || autostart > 0xFFFF {
			return nil, fmt.Errorf(`"autostart" %d is outside the 16-bit range`, autostart)
		}
		return tap.Decode(tap.EncodeProgram(bb.Name, payload, uint16(autostart)))

	case "raw":
		tb, err := tap.DecodeBlock(payload)
		if err != nil {
			return nil, fmt.Errorf("kind=raw file must already be a valid flag+payload+checksum block: %w", err)
		}
		return []tap.Block{tb}, nil

	default:
		return nil, fmt.Errorf("unknown kind %q (want code, program, or raw)", bb.Kind)
	}
}
