// Command maketap creates a TAP file from a binary, as a single CODE block.
//
// It is a drop-in replacement for the zxgotools tool of the same name: the
// command-line interface is unchanged, but the implementation runs on the
// zentools tap package and emits a correctly formed CODE header (the zxgotools
// original wrote an incorrect param2 of 0; the standard value 0x8000 is used
// here, as zxgotools' own field documentation intended).
//
// Usage:
//
//	maketap [--name NAME] [--address ADDR] input.bin output.tap
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ha1tch/zentools/pkg/tap"
)

func main() {
	var (
		name    = flag.String("name", "", "Name for code block (max 10 chars)")
		address = flag.Uint("address", 32768, "Start address (default: 32768)")
	)
	flag.Parse()

	args := flag.Args()
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s [--name NAME] [--address ADDR] input.bin output.tap\n", os.Args[0])
		os.Exit(1)
	}
	inputFile, outputFile := args[0], args[1]

	data, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: reading input file: %v\n", err)
		os.Exit(1)
	}

	blockName := *name
	if blockName == "" {
		blockName = baseName(inputFile)
	}

	img := tap.EncodeCode(blockName, data, uint16(*address))
	if err := os.WriteFile(outputFile, img, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully converted %s to %s\n", inputFile, outputFile)
}

// baseName derives a default block name from a path: the base name without
// extension, truncated to the 10-character tape-name limit.
func baseName(path string) string {
	n := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if len(n) > 10 {
		n = n[:10]
	}
	return n
}
