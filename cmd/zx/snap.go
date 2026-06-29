// file: snap.go
//
// The `zx snap` subcommand creates .sna / .z80 snapshots from a raw binary and
// inspects existing ones. Creation overlays the binary onto a real booted
// machine state (per model) via pkg/build — the same procedure zenas uses for
// its `build` command — so the snapshot loads and runs at the given entry point.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ha1tch/zentools/pkg/build"
	"github.com/ha1tch/zentools/pkg/snapshot"
)

// permuteArgs reorders args so that all flags precede positional arguments,
// letting a modern CLI accept flags in any position (the standard flag package
// otherwise stops at the first non-flag token). It is arity-aware: boolFlags
// names the flags that take no value, so a positional following a boolean flag
// is not mistakenly consumed as its value.
func permuteArgs(args []string, boolFlags map[string]bool) []string {
	isFlag := func(tok string) (name string, ok bool) {
		if len(tok) < 2 || tok[0] != '-' {
			return "", false
		}
		name = strings.TrimLeft(tok, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
		}
		return name, true
	}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		name, ok := isFlag(args[i])
		if !ok {
			positional = append(positional, args[i])
			continue
		}
		flags = append(flags, args[i])
		// A value-taking flag in "--flag value" form carries its next token.
		if !strings.Contains(args[i], "=") && !boolFlags[name] && i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return append(flags, positional...)
}

func cmdSnap(args []string) error {
	if len(args) == 0 {
		snapUsage()
		return nil
	}
	switch args[0] {
	case "make":
		return snapMake(args[1:])
	case "info":
		return snapInfo(args[1:])
	default:
		snapUsage()
		return fmt.Errorf("unknown snap subcommand %q", args[0])
	}
}

func snapUsage() {
	fmt.Fprintln(os.Stderr, `zx snap - create and inspect snapshots

Usage:
  zx snap make <input.bin> --start <addr> [--sna] [--z80] [options]
  zx snap info <file.sna|file.z80>

Make options:
  --origin <addr>   load address of the binary (default 0x8000)
  --start  <addr>   entry point / program counter (required)
  --sp     <addr>   stack pointer (default 0xFF00)
  --model  <name>   48k, 128k, plus2, plus2a, plus3 (default 48k)
  --sna             write a .sna snapshot
  --z80             write a .z80 (v3) snapshot
  -o <basename>     output basename (default: input name)

Addresses may be hex (0x8000, $8000) or decimal (32768).`)
}

func snapMake(args []string) error {
	fs := flag.NewFlagSet("snap make", flag.ContinueOnError)
	var (
		originS = fs.String("origin", "0x8000", "load address of the binary")
		startS  = fs.String("start", "", "entry point (required)")
		spS     = fs.String("sp", "0xFF00", "stack pointer")
		model   = fs.String("model", "48k", "target model")
		wantSNA = fs.Bool("sna", false, "write .sna")
		wantZ80 = fs.Bool("z80", false, "write .z80")
		outBase = fs.String("o", "", "output basename")
	)
	if err := fs.Parse(permuteArgs(args, map[string]bool{"sna": true, "z80": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("expected one input binary; see 'zx snap'")
	}
	if !*wantSNA && !*wantZ80 {
		return fmt.Errorf("choose at least one of --sna, --z80")
	}
	if *startS == "" {
		return fmt.Errorf("--start <addr> is required")
	}

	input := fs.Arg(0)
	code, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("reading %s: %w", input, err)
	}
	origin, err := parseAddr(*originS)
	if err != nil {
		return fmt.Errorf("invalid --origin: %w", err)
	}
	start, err := parseAddr(*startS)
	if err != nil {
		return fmt.Errorf("invalid --start: %w", err)
	}
	sp, err := parseAddr(*spS)
	if err != nil {
		return fmt.Errorf("invalid --sp: %w", err)
	}

	req := build.Request{
		Name:   baseName(input),
		Code:   code,
		Origin: origin,
		Start:  start,
		SP:     sp,
		Model:  build.Model(strings.ToLower(*model)),
	}
	if w := req.SPWarning(); w != "" {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	base := *outBase
	if base == "" {
		base = strings.TrimSuffix(input, filepath.Ext(input))
	}

	if *wantSNA {
		img, err := build.EncodeSNA(req)
		if err != nil {
			return err
		}
		if err := writeOut(base+".sna", img); err != nil {
			return err
		}
	}
	if *wantZ80 {
		img, err := build.EncodeZ80(req)
		if err != nil {
			return err
		}
		if err := writeOut(base+".z80", img); err != nil {
			return err
		}
	}
	return nil
}

func snapInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("expected one snapshot file")
	}
	path := args[0]
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var s *snapshot.MachineState
	switch strings.ToLower(filepath.Ext(path)) {
	case ".z80":
		s, err = snapshot.DecodeZ80(data)
	case ".sna":
		if len(data) > 49179 {
			s, err = snapshot.DecodeSNA128(data)
		} else {
			s, err = snapshot.DecodeSNA(data)
		}
	default:
		return fmt.Errorf("unrecognised snapshot extension %q", filepath.Ext(path))
	}
	if err != nil {
		return fmt.Errorf("decoding snapshot: %w", err)
	}

	fmt.Printf("File:  %s\n", path)
	fmt.Printf("Model: %s\n", modelName(s.Model))
	fmt.Printf("PC:    0x%04X\n", s.CPU.PC)
	fmt.Printf("SP:    0x%04X\n", s.CPU.SP)
	fmt.Printf("IM:    %d   IFF1: %v\n", s.CPU.IM, s.CPU.IFF1)
	if s.Model != snapshot.Model48K {
		fmt.Printf("Paging: port 0x7FFD = 0x%02X\n", s.Paging.Port7FFD)
	}
	return nil
}

func modelName(m snapshot.Model) string {
	switch m {
	case snapshot.Model48K:
		return "48K"
	case snapshot.Model128K:
		return "128K"
	case snapshot.ModelPlus2:
		return "+2"
	case snapshot.ModelPlus2A:
		return "+2A"
	case snapshot.ModelPlus3:
		return "+3"
	default:
		return "unknown"
	}
}

// parseAddr parses a 16-bit address in hex (0x.., $..) or decimal.
func parseAddr(s string) (uint16, error) {
	s = strings.TrimSpace(s)
	base := 10
	switch {
	case strings.HasPrefix(s, "0x"), strings.HasPrefix(s, "0X"):
		s, base = s[2:], 16
	case strings.HasPrefix(s, "$"):
		s, base = s[1:], 16
	}
	v, err := strconv.ParseUint(s, base, 16)
	if err != nil {
		return 0, err
	}
	return uint16(v), nil
}

func baseName(path string) string {
	n := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if len(n) > 10 {
		n = n[:10]
	}
	return n
}

func writeOut(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Printf("Wrote %s (%d bytes)\n", path, len(data))
	return nil
}
