// file: fromsnap.go
//
// The fromsnap subcommand extracts the Spectrum display file (the first 6912
// bytes of user RAM, 0x4000-0x5AFF) from a .sna or .z80 snapshot and writes it
// as a standalone .scr screen.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ha1tch/zentools/pkg/scr"
)

func scrFromSnap(args []string) error {
	fs := flag.NewFlagSet("scr fromsnap", flag.ContinueOnError)
	out := fs.String("o", "", "output .scr path (default: input base + .scr)")
	args = permuteArgs(args, nil)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("fromsnap needs exactly one .sna or .z80 snapshot")
	}
	inPath := fs.Arg(0)

	format := formatOf(inPath)
	if kindOf(format) != "snap" {
		return fmt.Errorf("fromsnap: input must be .sna or .z80 (got %q)", inPath)
	}

	data, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	state, err := decodeSnapshot(data, format)
	if err != nil {
		return err
	}

	ram := userRAM48K(state)
	if len(ram) < scr.FileLen {
		return fmt.Errorf("fromsnap: snapshot RAM too small (%d bytes)", len(ram))
	}
	screen := ram[:scr.FileLen]

	outPath := *out
	if outPath == "" {
		outPath = scrOutBase(inPath) + ".scr"
	}
	if err := os.WriteFile(outPath, screen, 0644); err != nil {
		return err
	}
	fmt.Printf("extracted screen from %s -> %s\n", inPath, outPath)
	return nil
}
