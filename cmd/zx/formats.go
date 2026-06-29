// file: formats.go
//
// The tap, tzx, basic, and info subcommands. These reuse the zentools format
// packages directly, and pkg/build for the shared overlay/encode procedure.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ha1tch/zentools/pkg/basic"
	"github.com/ha1tch/zentools/pkg/build"
	"github.com/ha1tch/zentools/pkg/tap"
	"github.com/ha1tch/zentools/pkg/tzx"
)

// --- zx tap -----------------------------------------------------------------

func cmdTAP(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, `zx tap - create and inspect TAP images

Usage:
  zx tap make <input.bin> [--name N] [--origin <addr>] [--loader --start <addr>] -o out.tap
  zx tap info <file.tap>`)
		return nil
	}
	switch args[0] {
	case "make":
		return tapMake(args[1:])
	case "info":
		return tapInfo(args[1:])
	default:
		return fmt.Errorf("unknown tap subcommand %q", args[0])
	}
}

func tapMake(args []string) error {
	fs := flag.NewFlagSet("tap make", flag.ContinueOnError)
	var (
		name     = fs.String("name", "", "tape block name (<=10 chars)")
		originS  = fs.String("origin", "0x8000", "load address")
		loader   = fs.Bool("loader", false, "prepend a BASIC auto-run loader")
		startS   = fs.String("start", "", "entry point (required with --loader)")
		out      = fs.String("o", "", "output file (required)")
	)
	if err := fs.Parse(permuteArgs(args, map[string]bool{"loader": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("usage: zx tap make <input.bin> -o out.tap")
	}
	code, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	origin, err := parseAddr(*originS)
	if err != nil {
		return fmt.Errorf("invalid --origin: %w", err)
	}
	nm := *name
	if nm == "" {
		nm = baseName(fs.Arg(0))
	}
	req := build.Request{Name: nm, Code: code, Origin: origin}

	var img []byte
	if *loader {
		if *startS == "" {
			return fmt.Errorf("--loader requires --start")
		}
		start, err := parseAddr(*startS)
		if err != nil {
			return fmt.Errorf("invalid --start: %w", err)
		}
		req.Start = start
		img, err = build.EncodeTAPWithLoader(req)
		if err != nil {
			return err
		}
	} else {
		img = build.EncodeTAP(req)
	}
	return writeOut(*out, img)
}

func tapInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: zx tap info <file.tap>")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	blocks, err := tap.Decode(data)
	if err != nil {
		return err
	}
	fmt.Printf("%s: %d blocks\n", args[0], len(blocks))
	for i, b := range blocks {
		kind := "data"
		if b.IsHeader {
			kind = "header"
		}
		fmt.Printf("  [%d] %-6s %d bytes", i, kind, len(b.Data)+2)
		if b.IsHeader {
			fmt.Printf("  type=%d name=%q load=0x%04X", b.Type, b.Name, b.Param1)
		}
		if !b.ChecksumOK {
			fmt.Print("  CHECKSUM BAD")
		}
		fmt.Println()
	}
	return nil
}

// --- zx tzx -----------------------------------------------------------------

func cmdTZX(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, `zx tzx - create and inspect TZX images

Usage:
  zx tzx make <input.tap> [--title T] [--author A] [--year Y] -o out.tzx
  zx tzx info <file.tzx>`)
		return nil
	}
	switch args[0] {
	case "make":
		return tzxMake(args[1:])
	case "info":
		return tzxInfo(args[1:])
	default:
		return fmt.Errorf("unknown tzx subcommand %q", args[0])
	}
}

func tzxMake(args []string) error {
	fs := flag.NewFlagSet("tzx make", flag.ContinueOnError)
	var (
		title  = fs.String("title", "", "archive title")
		author = fs.String("author", "", "archive author")
		year   = fs.String("year", "", "publication year")
		out    = fs.String("o", "", "output file (required)")
	)
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("usage: zx tzx make <input.tap> -o out.tzx")
	}
	tapImg, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	img, err := tzx.EncodeFromTAP(tapImg, tzx.EncodeOptions{
		Title:  *title,
		Author: *author,
		Year:   *year,
	})
	if err != nil {
		return err
	}
	return writeOut(*out, img)
}

func tzxInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: zx tzx info <file.tzx>")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	blocks, err := tzx.Decode(data)
	if err != nil {
		return err
	}
	fmt.Printf("%s: %d blocks\n", args[0], len(blocks))
	for i, b := range blocks {
		fmt.Printf("  [%d] 0x%02X\n", i, b.ID)
	}
	return nil
}

// --- zx basic ---------------------------------------------------------------

func cmdBASIC(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, `zx basic - tokenise and detokenise ZX BASIC

Usage:
  zx basic tokenise   <input.bas> -o out.bin   [--case-sensitive]
  zx basic detokenise <input.bin> [-o out.bas]`)
		return nil
	}
	switch args[0] {
	case "tokenise", "tokenize":
		return basicTokenise(args[1:])
	case "detokenise", "detokenize":
		return basicDetokenise(args[1:])
	default:
		return fmt.Errorf("unknown basic subcommand %q", args[0])
	}
}

func basicTokenise(args []string) error {
	fs := flag.NewFlagSet("basic tokenise", flag.ContinueOnError)
	var (
		out           = fs.String("o", "", "output file (required)")
		caseSensitive = fs.Bool("case-sensitive", false, "require exact keyword case")
	)
	if err := fs.Parse(permuteArgs(args, map[string]bool{"case-sensitive": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("usage: zx basic tokenise <input.bas> -o out.bin")
	}
	src, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	var opts []basic.Option
	if *caseSensitive {
		opts = append(opts, basic.CaseSensitive())
	}
	tok, err := basic.Tokenise(string(src), opts...)
	if err != nil {
		return err
	}
	return writeOut(*out, tok)
}

func basicDetokenise(args []string) error {
	fs := flag.NewFlagSet("basic detokenise", flag.ContinueOnError)
	out := fs.String("o", "", "output file (default: stdout)")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: zx basic detokenise <input.bin> [-o out.bas]")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	text, err := basic.Detokenise(data)
	if err != nil {
		return err
	}
	if *out == "" {
		fmt.Print(text)
		return nil
	}
	return writeOut(*out, []byte(text))
}

// --- zx info ----------------------------------------------------------------

func cmdInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: zx info <file>")
	}
	path := args[0]
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	switch detectFormat(path, data) {
	case "tzx":
		return tzxInfo(args)
	case "tap":
		return tapInfo(args)
	case "snap":
		return snapInfo(args)
	default:
		return fmt.Errorf("could not identify the format of %s", path)
	}
}

// detectFormat identifies a file by signature first, then extension.
func detectFormat(path string, data []byte) string {
	if len(data) >= 7 && string(data[:7]) == "ZXTape!" {
		return "tzx"
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".tzx":
		return "tzx"
	case ".tap":
		return "tap"
	case ".sna", ".z80":
		return "snap"
	}
	return ""
}
