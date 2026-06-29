// Command zx is the modern, unified front-end to the zentools format library.
//
// Where the individual tools (maketap, totap, loadtap, tap2tzx) preserve the
// historical zxgotools interfaces, zx is organised by format: each subcommand
// names a noun (tap, tzx, basic, snap) or an action over formats (info), and
// exposes capabilities those older tools could not, such as creating snapshots
// and converting between formats.
//
// Usage:
//
//	zx <command> [arguments]
//
// Commands:
//
//	tap     create and inspect TAP tape images
//	tzx     create and inspect TZX tape images
//	basic   tokenise and detokenise ZX BASIC
//	snap    create and inspect .sna / .z80 snapshots
//	info    auto-detect a file's format and summarise it
//	version print the zentools version
package main

import (
	"fmt"
	"os"

	"github.com/ha1tch/zentools/pkg/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "tap":
		err = cmdTAP(args)
	case "tzx":
		err = cmdTZX(args)
	case "basic":
		err = cmdBASIC(args)
	case "snap":
		err = cmdSnap(args)
	case "info":
		err = cmdInfo(args)
	case "convert":
		err = cmdConvert(args)
	case "version", "-v", "--version":
		fmt.Printf("zx (zentools) %s\n", version.Version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "zx: unknown command %q\n\n", cmd)
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "zx %s: %v\n", cmd, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `zx - ZX Spectrum format tools (zentools %s)

Usage: zx <command> [arguments]

Commands:
  tap     create and inspect TAP tape images
  tzx     create and inspect TZX tape images
  basic   tokenise and detokenise ZX BASIC
  snap    create and inspect .sna / .z80 snapshots
  convert convert between tape and snapshot formats
  info    auto-detect a file's format and summarise it
  version print the zentools version

Run 'zx <command>' with no arguments for command-specific help.
`, version.Version)
}
