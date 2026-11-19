package pzx

import "fmt"

// Standard Spectrum ROM save-routine timings, T cycles at 3.5MHz -- the PZX
// spec's own worked examples (see http://zxds.raxoft.cz/docs/pzx.txt, the
// PULS and DATA block descriptions).
const (
	stdPilotPulse       = 2168
	stdSyncPulse1       = 667
	stdSyncPulse2       = 735
	stdBit0Pulse        = 855
	stdBit1Pulse        = 1710
	stdTailPulse        = 945
	stdHeaderPilotCount = 8063        // the leader of a header block
	stdDataPilotCount   = 3223        // the leader of a data block
	stdInterBlockPause  = 3500000 / 2 // ~0.5s: shorter than the real ROM's ~1s gap, plenty for a loader to detect the pause
)

// StandardBlock is one block of a standard-speed tape, as the ROM saves it:
// the flag byte, the payload and the checksum, in that order. Header says
// whether it gets a header's long leader or a data block's short one.
type StandardBlock struct {
	Raw    []byte
	Header bool
}

// EncodeStandard writes a PZX image of standard-speed blocks: each one a
// PULS block (leader and sync) and a DATA block with the ROM's bit timings,
// with a pause between blocks. This is the PZX spec's own mapping for
// TZX's ID 0x10, the block shape a TAP file holds. title goes in the PZX
// header; it may be empty.
func EncodeStandard(blocks []StandardBlock, title string) ([]byte, error) {
	f := &File{Blocks: []Block{
		HeaderBlock{Major: 1, Minor: 0, Title: title},
	}}
	for i, b := range blocks {
		leader := uint16(stdDataPilotCount)
		if b.Header {
			leader = stdHeaderPilotCount
		}
		f.Blocks = append(f.Blocks,
			PulseBlock{Pulses: []Pulse{
				{Count: leader, Duration: stdPilotPulse},
				{Count: 1, Duration: stdSyncPulse1},
				{Count: 1, Duration: stdSyncPulse2},
			}},
			DataBlock{
				InitialLevelHigh: true,
				BitCount:         uint32(len(b.Raw)) * 8,
				Tail:             stdTailPulse,
				S0:               []uint16{stdBit0Pulse, stdBit0Pulse},
				S1:               []uint16{stdBit1Pulse, stdBit1Pulse},
				Data:             b.Raw,
			},
		)
		if i < len(blocks)-1 {
			f.Blocks = append(f.Blocks, PauseBlock{InitialLevelHigh: false, Duration: stdInterBlockPause})
		}
	}
	return Encode(f)
}

// EncodeFromTAP converts a TAP image to PZX: every TAP block becomes a
// standard-speed block. A block is given a header's leader when it is a
// header as the ROM writes one (flag 0x00 and 19 bytes with flag and
// checksum), a data block's otherwise.
func EncodeFromTAP(tapImage []byte, title string) ([]byte, error) {
	var blocks []StandardBlock
	for pos := 0; pos < len(tapImage); {
		if pos+2 > len(tapImage) {
			return nil, fmt.Errorf("pzx: TAP image ends inside a block length at offset %d", pos)
		}
		n := int(tapImage[pos]) | int(tapImage[pos+1])<<8
		pos += 2
		if n == 0 || pos+n > len(tapImage) {
			return nil, fmt.Errorf("pzx: TAP block of %d bytes at offset %d does not fit in the image", n, pos-2)
		}
		raw := tapImage[pos : pos+n]
		blocks = append(blocks, StandardBlock{Raw: raw, Header: raw[0] == 0x00 && n == 19})
		pos += n
	}
	return EncodeStandard(blocks, title)
}
