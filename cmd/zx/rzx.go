// file: rzx.go
//
// The `zx rzx` subcommand inspects RZX input-recording files. Unlike TAP,
// TZX, and SZX, there's no `make` here: an RZX file records the exact result
// of every CPU IN instruction during a real emulated session (see
// pkg/rzx's own doc comment), which is an emulator's job to produce while
// running, not something meaningfully synthesised from a static binary.

package main

import (
	"fmt"
	"os"

	"github.com/ha1tch/zentools/pkg/rzx"
)

func cmdRZX(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, `zx rzx - inspect RZX input-recording files

Usage:
  zx rzx info <file.rzx>

There is no 'make': an RZX file records the result of every CPU IN
instruction during a real emulated session, which is an emulator's job to
produce, not something built from a static binary.`)
		return nil
	}
	switch args[0] {
	case "info":
		return rzxInfo(args[1:])
	default:
		return fmt.Errorf("unknown rzx subcommand %q", args[0])
	}
}

func rzxInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: zx rzx info <file.rzx>")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	f, err := rzx.Decode(data)
	if err != nil {
		return err
	}
	fmt.Printf("%s: RZX v%d.%d, signed=%v, %d blocks\n", args[0], f.MajorVersion, f.MinorVersion, f.Signed, len(f.Blocks))
	for i, block := range f.Blocks {
		switch b := block.(type) {
		case rzx.CreatorBlock:
			fmt.Printf("  [%d] Creator  %q v%d.%d", i, b.ID, b.Major, b.Minor)
			if len(b.Data) > 0 {
				fmt.Printf("  (+%d bytes custom data)", len(b.Data))
			}
			fmt.Println()
		case rzx.SecurityInfoBlock:
			fmt.Printf("  [%d] SecurityInfo  keyID=%#08x week=%d\n", i, b.KeyID, b.Week)
		case rzx.SecuritySignatureBlock:
			fmt.Printf("  [%d] SecuritySignature  (not verified by this tool)\n", i)
		case rzx.SnapshotBlock:
			kind := "embedded"
			if b.External {
				kind = "external reference"
			}
			fmt.Printf("  [%d] Snapshot  .%s, %s, %d bytes\n", i, b.Extension, kind, len(b.Data))
		case rzx.RecordingBlock:
			totalReads := 0
			repeated := 0
			for _, fr := range b.Frames {
				if fr.Repeated {
					repeated++
					continue
				}
				totalReads += len(fr.PortReads)
			}
			protected := ""
			if b.Protected {
				protected = " (protected)"
			}
			fmt.Printf("  [%d] Recording%s  %d frames (%d repeated), t-states start=%d, %d total port reads\n",
				i, protected, len(b.Frames), repeated, b.TStatesStart, totalReads)
		}
	}
	return nil
}
