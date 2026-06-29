// file: hardware.go
//
// TZX group blocks (0x21/0x22) and the hardware-type block (0x33), per the
// TZX v1.20 specification. These describe the tape rather than carrying tape
// data: a group brackets a set of blocks under a name, and a hardware-type
// block records which machines and peripherals the tape runs on or uses.

package tzx

const (
	idGroupStart  = 0x21
	idGroupEnd    = 0x22
	idHardwareTyp = 0x33
)

// Hardware type categories (the first byte of a HWINFO entry), per the TZX
// hardware-information reference.
const (
	HWComputers      = 0x00
	HWExternalStore  = 0x01
	HWRAMROMAddOn    = 0x02
	HWSoundDevices   = 0x03
	HWJoysticks      = 0x04
	HWMice           = 0x05
	HWControllers    = 0x06
	HWSerialPorts    = 0x07
	HWParallelPorts  = 0x08
	HWPrinters       = 0x09
	HWModems         = 0x0A
	HWDigitizers     = 0x0B
	HWNetworkAdapter = 0x0C
	HWKeyboards      = 0x0D
	HWADDAConverters = 0x0E
	HWEPROMProgram   = 0x0F
	HWGraphics       = 0x10
)

// Hardware IDs within the Computers category (HWComputers).
const (
	HWIDSpectrum16K       = 0x00
	HWIDSpectrum48K       = 0x01 // 48k, Plus
	HWIDSpectrum48KIssue1 = 0x02
	HWIDSpectrum128K      = 0x03 // 128k + (Sinclair)
	HWIDSpectrum128KPlus2 = 0x04 // 128k +2 (grey case)
	HWIDSpectrumPlus2APlus3 = 0x05 // 128k +2A, +3
)

// Hardware IDs within the Sound devices category (HWSoundDevices).
const (
	HWIDClassicAY = 0x00 // compatible with 128k ZXs
)

// Hardware information byte: whether the tape runs on / uses a given machine
// or peripheral.
const (
	HWInfoRuns       = 0x00 // runs, may or may not use the hardware
	HWInfoUses       = 0x01 // uses the hardware or special features
	HWInfoRunsUnused = 0x02 // runs but does not use the hardware
	HWInfoDoesntRun  = 0x03 // does not run on this machine/hardware
)

// HardwareInfo is one entry in a hardware-type (0x33) block: a machine or
// peripheral and the tape's relationship to it.
type HardwareInfo struct {
	Type byte // category, e.g. HWComputers
	ID   byte // item within the category, e.g. HWIDSpectrum128K
	Info byte // HWInfoRuns / HWInfoUses / HWInfoRunsUnused / HWInfoDoesntRun
}

// groupStartBlock builds a 0x21 group-start block. The name is clamped to 30
// characters, as the spec recommends. An empty name still produces a valid
// (zero-length-name) group start.
func groupStartBlock(name string) []byte {
	if len(name) > 30 {
		name = name[:30]
	}
	out := []byte{idGroupStart, byte(len(name))}
	return append(out, []byte(name)...)
}

// groupEndBlock builds a 0x22 group-end block, which has no body.
func groupEndBlock() []byte {
	return []byte{idGroupEnd}
}

// hardwareTypeBlock builds a 0x33 hardware-type block from a list of entries.
// Returns nil if the list is empty. A maximum of 255 entries is encodable; any
// beyond that are dropped (the count field is a single byte).
func hardwareTypeBlock(entries []HardwareInfo) []byte {
	if len(entries) == 0 {
		return nil
	}
	if len(entries) > 255 {
		entries = entries[:255]
	}
	out := []byte{idHardwareTyp, byte(len(entries))}
	for _, e := range entries {
		out = append(out, e.Type, e.ID, e.Info)
	}
	return out
}
