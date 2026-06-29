// Command loadtap reads and analyses a ZX Spectrum TAP file.
//
// It is a drop-in replacement for the zxgotools tool of the same name: the
// command-line interface and the printed output format are unchanged, but the
// implementation decodes via the zentools tap package.
//
// Usage:
//
//	loadtap [-d] [-r] <tap-file>
//
//	-d  Dump block data as hex
//	-r  Output raw block data (data blocks only, to stdout)
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ha1tch/zentools/pkg/tap"
)

const headerFlag = 0x00

func main() {
	dump := flag.Bool("d", false, "Dump block data as hex")
	raw := flag.Bool("r", false, "Output raw block data")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [-d] [-r] <tap-file>\n", os.Args[0])
		os.Exit(1)
	}

	filename := flag.Arg(0)
	image, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: opening file: %v\n", err)
		os.Exit(1)
	}

	blocks, err := tap.Decode(image)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *raw {
		// Output just the raw data blocks (skip headers).
		for _, block := range blocks {
			if block.Flag != headerFlag {
				os.Stdout.Write(block.Data)
			}
		}
		return
	}

	fmt.Printf("Found %d blocks in %s\n", len(blocks), filename)
	for i, block := range blocks {
		printBlockInfo(block, i)
		if *dump {
			dumpHex(block.Data)
		}
	}
}

// printBlockInfo prints information about a TAP block, matching the original
// loadtap layout. The block length is the on-tape length: flag + data +
// checksum, i.e. len(Data) + 2.
func printBlockInfo(block tap.Block, index int) {
	length := len(block.Data) + 2
	fmt.Printf("\nBlock %d:\n", index)
	fmt.Printf("  Length: %d\n", length)
	fmt.Printf("  Flag: 0x%02X (%s)\n", block.Flag, flagTypeString(block.Flag))

	if block.IsHeader {
		fmt.Println("  Header Information:")
		fmt.Printf("    Type: %d\n", block.Type)
		fmt.Printf("    Filename: %s\n", block.Name)
		fmt.Printf("    Data Length: %d\n", block.DataLength)
		fmt.Printf("    Param1: %d\n", block.Param1)
		fmt.Printf("    Param2: %d\n", block.Param2)
	}
	fmt.Printf("  Checksum: 0x%02X\n", block.Checksum)
	fmt.Printf("  Data Length: %d bytes\n", len(block.Data))
}

func flagTypeString(flag byte) string {
	if flag == headerFlag {
		return "Header"
	}
	return "Data"
}

// dumpHex prints a hexadecimal dump of the data, 16 bytes per line.
func dumpHex(data []byte) {
	fmt.Println("  Data:")
	const bytesPerLine = 16
	for i := 0; i < len(data); i += bytesPerLine {
		end := i + bytesPerLine
		if end > len(data) {
			end = len(data)
		}
		line := data[i:end]
		hexBytes := make([]string, len(line))
		for j, b := range line {
			hexBytes[j] = fmt.Sprintf("%02X", b)
		}
		fmt.Printf("    %s\n", strings.Join(hexBytes, " "))
	}
}
