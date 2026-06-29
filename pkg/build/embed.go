// file: embed.go
//
// Embeds one real booted-machine snapshot per model and decodes it on demand
// into a zentools MachineState. These snapshots were captured by booting each
// model's actual ROM to its start-up prompt, so overlaying user code onto one
// gives the program a fully initialised machine to run in (system variables,
// paging, and the upper RAM pages the 128K editor uses — none of which a
// hand-built state would reproduce).

package build

import (
	_ "embed"
	"fmt"

	"github.com/ha1tch/zentools/pkg/snapshot"
)

//go:embed snapshots/48k_boot.z80
var boot48K []byte

//go:embed snapshots/128k_boot.z80
var boot128K []byte

//go:embed snapshots/plus2_boot.z80
var bootPlus2 []byte

//go:embed snapshots/plus2a_boot.z80
var bootPlus2A []byte

//go:embed snapshots/plus3_boot.z80
var bootPlus3 []byte

// bootImage returns the embedded boot snapshot bytes for a model.
func bootImage(m Model) ([]byte, error) {
	switch m {
	case Model48K:
		return boot48K, nil
	case Model128K:
		return boot128K, nil
	case ModelPlus2:
		return bootPlus2, nil
	case ModelPlus2A:
		return bootPlus2A, nil
	case ModelPlus3:
		return bootPlus3, nil
	default:
		return nil, fmt.Errorf("unknown model %q (want 48k, 128k, plus2, plus2a, or plus3)", m)
	}
}

// bootState decodes a model's embedded boot snapshot into a fresh MachineState.
func bootState(m Model) (*snapshot.MachineState, error) {
	img, err := bootImage(m)
	if err != nil {
		return nil, err
	}
	s, err := snapshot.DecodeZ80(img)
	if err != nil {
		return nil, fmt.Errorf("decoding embedded %s boot snapshot: %w", m, err)
	}
	return s, nil
}
