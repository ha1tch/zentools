// Command totap converts a binary or a BASIC text file into a TAP file.
//
// It is a drop-in replacement for the zxgotools tool of the same name. The
// command-line interface is unchanged; the implementation runs on the zentools
// tap and basic packages. Two zxgotools defects are corrected here: the BASIC
// conversion works (the original hung), and the binary CODE header carries the
// standard param2 of 0x8000.
//
// Usage:
//
//	totap [--basic|--binary] [--name NAME] [--address ADDR]
//	      [--autostart LINE] [-c] input output.tap
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ha1tch/zentools/pkg/basic"
	"github.com/ha1tch/zentools/pkg/tap"
)

func main() {
	var (
		basicMode       = flag.Bool("basic", false, "Convert BASIC text file")
		binMode         = flag.Bool("binary", false, "Convert binary file")
		name            = flag.String("name", "", "Name for TAP block (max 10 chars)")
		address         = flag.Uint("address", 32768, "Start address for binary files (default: 32768)")
		autostart       = flag.Uint("autostart", 0, "Auto-start line for BASIC programs")
		caseIndependent = flag.Bool("c", false, "Case independent token matching")
	)
	flag.Parse()

	if !*basicMode && !*binMode {
		fmt.Fprintf(os.Stderr, "Error: Must specify either --basic or --binary mode\n")
		flag.Usage()
		os.Exit(1)
	}
	if *basicMode && *binMode {
		fmt.Fprintf(os.Stderr, "Error: Cannot specify both --basic and --binary\n")
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s [--basic|--binary] [options] input output.tap\n", os.Args[0])
		flag.Usage()
		os.Exit(1)
	}
	inputFile, outputFile := args[0], args[1]

	var err error
	if *basicMode {
		err = convertBasic(inputFile, outputFile, *name, uint16(*autostart), *caseIndependent)
	} else {
		err = convertBinary(inputFile, outputFile, *name, uint16(*address))
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully created %s\n", outputFile)
}

func convertBinary(inputFile, outputFile, name string, startAddress uint16) error {
	data, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("reading input file: %w", err)
	}
	if name == "" {
		name = defaultName(inputFile)
	}
	img := tap.EncodeCode(name, data, startAddress)
	return os.WriteFile(outputFile, img, 0o644)
}

func convertBasic(inputFile, outputFile, name string, autostart uint16, caseIndependent bool) error {
	src, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("reading input file: %w", err)
	}

	// zxgotools' -c flag selected case-independent token matching. The zentools
	// tokeniser is case-independent by default, so -c maps to the default and
	// its absence maps to the explicit CaseSensitive option.
	var opts []basic.Option
	if !caseIndependent {
		opts = append(opts, basic.CaseSensitive())
	}

	tokenised, err := basic.Tokenise(string(src), opts...)
	if err != nil {
		return fmt.Errorf("tokenising BASIC: %w", err)
	}

	if name == "" {
		name = defaultName(inputFile)
	}

	img := tap.EncodeProgram(name, tokenised, autostart)
	if err := os.WriteFile(outputFile, img, 0o644); err != nil {
		return fmt.Errorf("writing output file: %w", err)
	}

	if requires128K(tokenised) {
		fmt.Println("Note: Program requires 128K")
	}
	return nil
}

// requires128K reports whether the tokenised program uses a 128K-only keyword
// token: SPECTRUM (0xA3) or PLAY (0xA4).
func requires128K(tokenised []byte) bool {
	for _, b := range tokenised {
		if b == 0xA3 || b == 0xA4 {
			return true
		}
	}
	return false
}

// defaultName derives a tape block name from a path: base name without
// extension, truncated to 10 characters.
func defaultName(path string) string {
	n := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if len(n) > 10 {
		n = n[:10]
	}
	return n
}
