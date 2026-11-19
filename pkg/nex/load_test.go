package nex

import "testing"

func TestLoad(t *testing.T) {
	f := &File{}
	// Across the boundary of banks 2 and 0.
	data := make([]byte, 0x30)
	for i := range data {
		data[i] = byte(i + 1)
	}
	if err := f.Load(0xBFF0, data); err != nil {
		t.Fatal(err)
	}
	if len(f.Banks) != 2 || len(f.Banks[2]) != BankLen || len(f.Banks[0]) != BankLen {
		t.Fatalf("banks: %d", len(f.Banks))
	}
	if f.Banks[2][0x3FF0] != 1 || f.Banks[2][0x3FFF] != 16 || f.Banks[0][0] != 17 || f.Banks[0][0x1F] != 48 {
		t.Errorf("bytes are not where the address puts them")
	}
	// Bank 5 at 0x4000, and what is already in a bank stays.
	if err := f.Load(0x4000, []byte{0xAA}); err != nil {
		t.Fatal(err)
	}
	if err := f.Load(0xC010, []byte{0xBB}); err != nil {
		t.Fatal(err)
	}
	if f.Banks[5][0] != 0xAA || f.Banks[0][0x10] != 0xBB || f.Banks[0][0] != 17 {
		t.Errorf("a second Load disturbed the first")
	}
	if err := f.Load(0x3FFF, []byte{1}); err == nil {
		t.Error("loaded into the ROM")
	}
	if err := f.Load(0xFFFF, []byte{1, 2}); err == nil {
		t.Error("loaded past FFFF")
	}
	if err := f.Load(0xFFFF, []byte{1}); err != nil {
		t.Errorf("the last byte of memory: %v", err)
	}
	if err := f.Load(0x0000, nil); err != nil {
		t.Errorf("nothing to load: %v", err)
	}
}
