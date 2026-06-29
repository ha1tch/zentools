// file: loader.go
//
// A CODE tape is loaded but not run: a real Spectrum needs a BASIC program to
// LOAD the code and jump to it. This file generates that loader as ASCII BASIC
// and tokenises it with the zentools tokeniser — no hand-assembled byte tables.
// The loader is emitted as a Program block ahead of the CODE block so the tape
// auto-runs on LOAD "".

package build

import (
	"fmt"

	"github.com/ha1tch/zentools/pkg/basic"
	"github.com/ha1tch/zentools/pkg/tap"
)

// loaderAutostartLine is the BASIC line the tape's Program header points at, so
// LOAD "" runs the loader immediately rather than just listing it.
const loaderAutostartLine = 10

// loaderSource builds the ASCII BASIC auto-run loader. It clears the machine
// just below the code's load address (so BASIC's workspace cannot collide with
// the code), loads the CODE block, and jumps to the entry point.
//
// CLEAR sets RAMTOP to its argument and is given codeOrigin-1 so that memory
// from codeOrigin upward is outside BASIC's reach. The USR target is the
// program's entry point.
func loaderSource(codeOrigin, start uint16) string {
	clearAddr := int(codeOrigin) - 1
	if clearAddr < 0 {
		clearAddr = 0
	}
	return fmt.Sprintf("10 CLEAR %d\n20 LOAD \"\"CODE\n30 RANDOMIZE USR %d\n", clearAddr, start)
}

// encodeLoaderProgram tokenises the loader and wraps it as a Program tape block
// with an autostart line, so LOAD "" runs it.
func encodeLoaderProgram(name string, codeOrigin, start uint16) ([]byte, error) {
	tokenised, err := basic.Tokenise(loaderSource(codeOrigin, start))
	if err != nil {
		return nil, fmt.Errorf("tokenising loader: %w", err)
	}
	return tap.EncodeProgram(tapeName(name), tokenised, loaderAutostartLine), nil
}

// EncodeTAPWithLoader produces a .tap that auto-runs: a BASIC loader Program
// block followed by the CODE block. Requires r.Start (the entry point) to be set.
func EncodeTAPWithLoader(r Request) ([]byte, error) {
	if r.Start == 0 {
		return nil, fmt.Errorf("a self-running tape needs --start (the entry point) set")
	}
	loader, err := encodeLoaderProgram(r.Name, r.Origin, r.Start)
	if err != nil {
		return nil, err
	}
	code := tap.EncodeCode(tapeName(r.Name), r.Code, r.Origin)
	return append(loader, code...), nil
}
