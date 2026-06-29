// Package snapshot reads and writes ZX Spectrum snapshot files (.sna and .z80)
// via a neutral MachineState. The codecs never reference an emulator: a consumer
// fills a MachineState from its own live state to save, and applies a decoded
// MachineState to restore. This keeps the format logic reusable across the
// emulator, tooling, and tests.
//
// Supported formats:
//   - .sna: 48K (the classic 27-byte header + 48 KiB) and 128K (header + paged
//     banks + the extra 128K trailer).
//   - .z80: versioned, optionally compressed (planned).
//
// The package depends only on the standard library.
package snapshot

// Model identifies the Spectrum variant a snapshot targets.
type Model uint8

const (
	Model48K Model = iota
	Model128K
	ModelPlus2
	ModelPlus2A
	ModelPlus3
)

func (m Model) Is128KFamily() bool { return m != Model48K }

// CPU holds the Z80 register file and interrupt state. Register pairs are stored
// as 16-bit values; the underscore-suffixed fields are the alternate set.
type CPU struct {
	AF, BC, DE, HL     uint16
	AF_, BC_, DE_, HL_ uint16 // alternate registers
	IX, IY             uint16
	SP, PC             uint16
	I, R               uint8
	IFF1, IFF2         bool
	IM                 uint8 // interrupt mode 0, 1, or 2
}

// Paging holds 128K/+3 memory banking state. It is meaningful only for the
// 128K family; for a 48K snapshot it is left zero.
type Paging struct {
	Port7FFD uint8 // 128K paging port
	Port1FFD uint8 // +3 secondary paging port
	Locked   bool  // paging disabled (bit 5 of 0x7FFD)
}

// IO holds the small amount of I/O state a snapshot carries.
type IO struct {
	Border uint8 // border colour 0-7
}

// Memory holds the Spectrum's RAM as eight 16 KiB banks. For a 48K machine only
// banks 5, 2 and 0 are meaningful (mapped at 0x4000, 0x8000, 0xC000); for the
// 128K family all eight may be used, with Paging selecting which is visible.
type Memory struct {
	RAM [8][16384]byte
}

// BankFor48K returns the RAM bank mapped to a 48K address region. 48K memory is
// banks 5 (0x4000-0x7FFF), 2 (0x8000-0xBFFF) and 0 (0xC000-0xFFFF).
func (m *Memory) bank48(region int) *[16384]byte {
	switch region {
	case 0:
		return &m.RAM[5]
	case 1:
		return &m.RAM[2]
	default:
		return &m.RAM[0]
	}
}

// MachineState is the neutral snapshot pivot: everything a snapshot format needs
// to save or restore, with no reference to any emulator's types.
type MachineState struct {
	Model  Model
	CPU    CPU
	Paging Paging
	IO     IO
	Memory Memory
}
