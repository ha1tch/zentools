package build

import (
	"fmt"

	"github.com/ha1tch/zentools/pkg/nex"
	"github.com/ha1tch/zentools/pkg/pzx"
)

// EncodePZX produces a .pzx image containing the code as a single CODE
// block: EncodeTAP's tape in the PZX format.
func EncodePZX(r Request) ([]byte, error) {
	return EncodePZXFromTAP(EncodeTAP(r), r.Name)
}

// EncodePZXFromTAP converts a TAP image (EncodeTAP's, or
// EncodeTAPWithLoader's) to PZX, with the ROM's standard timings.
func EncodePZXFromTAP(tapImage []byte, title string) ([]byte, error) {
	return pzx.EncodeFromTAP(tapImage, title)
}

// NEXOptions is what a NEX file has that a Request does not say.
type NEXOptions struct {
	// Border is the border colour while loading, 0-7.
	Border uint8
	// Core is the lowest core version the program needs, major, minor and
	// sub-minor. The zero value is DefaultNEXCore.
	Core [3]uint8
}

// DefaultNEXCore is the core version EncodeNEX asks for when NEXOptions
// names none: 2.0.28, the one `zx next nex build` writes by default.
var DefaultNEXCore = [3]uint8{2, 0, 28}

// EncodeNEX produces a .nex file, the Spectrum Next's executable: the code
// in the banks that are paged in when a NEX program starts (bank 5 at
// 0x4000, bank 2 at 0x8000, bank 0 at 0xC000), with Start as the PC and SP
// as the stack pointer. Request.Model is not used: a NEX file carries no
// machine state, the loader sets the machine up. Code must be at 0x4000 or
// above and end by 0xFFFF. A Start of 0 makes a file that is loaded and
// not run, which is the format's own meaning for it.
func EncodeNEX(r Request, o NEXOptions) ([]byte, error) {
	if len(r.Code) == 0 {
		return nil, fmt.Errorf("build: no code to put in a NEX file")
	}
	core := o.Core
	if core == ([3]uint8{}) {
		core = DefaultNEXCore
	}
	f := &nex.File{PC: r.Start, SP: r.SP, Border: o.Border & 7, CoreVersion: core, Screen: map[uint8][]byte{}, Banks: map[int][]byte{}}
	if err := f.Load(r.Origin, r.Code); err != nil {
		return nil, err
	}
	return nex.Encode(f)
}
