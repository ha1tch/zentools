package nex

import "fmt"

// Load puts data in the file's banks at a 64K address, the way memory is
// paged when a NEX program starts: bank 5 at 0x4000, bank 2 at 0x8000 and
// bank 0 at 0xC000. Banks it touches are made if the file does not have
// them yet; what was in them stays where data does not reach. Addresses
// below 0x4000 are the ROM's and nothing can be loaded there, and data may
// not run past 0xFFFF.
func (f *File) Load(addr uint16, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if addr < 0x4000 {
		return fmt.Errorf("nex: cannot load at %04X: the ROM is at 0000-3FFF", addr)
	}
	if int(addr)+len(data) > 0x10000 {
		return fmt.Errorf("nex: %d bytes from %04X pass FFFF", len(data), addr)
	}
	if f.Banks == nil {
		f.Banks = map[int][]byte{}
	}
	slots := [4]int{-1, 5, 2, 0}
	for i, v := range data {
		a := int(addr) + i
		n := slots[a>>14]
		if f.Banks[n] == nil {
			f.Banks[n] = make([]byte, BankLen)
		}
		f.Banks[n][a&0x3FFF] = v
	}
	return nil
}
