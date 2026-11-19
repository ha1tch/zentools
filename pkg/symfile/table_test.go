package symfile

import (
	"strings"
	"testing"
)

func TestTable(t *testing.T) {
	tab := NewTable()
	n, err := tab.AddFile(strings.NewReader("0000229B 0000029B 01 BORDER\n00008000 00008000 00 START\n00008002 00008002 00 START@LOOP\n00000005 00000005 01 COUNT\n"), FormatAuto)
	if err != nil || n != 4 {
		t.Fatalf("first file: %d symbols, %v", n, err)
	}
	// A second file: another name for 8000 (the first keeps it), the same
	// name at another address (made unique), a new label and a new value
	// for a constant's name.
	if _, err := tab.AddFile(strings.NewReader("main\t\tEQU 08000H\nSTART\t\tEQU 09000H\nhelper\t\tEQU 08100H\n"), FormatAuto); err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[uint16]string{0x8000: "START", 0x8002: "START_LOOP", 0x9000: "START_9000", 0x8100: "helper"} {
		if got, ok := tab.Label(addr); !ok || got != want {
			t.Errorf("label at %04X is %q (%v), want %q", addr, got, ok, want)
		}
	}
	if _, ok := tab.Label(0x229B); ok {
		t.Error("the constant BORDER is a label")
	}
	if name, ok := tab.Constant(0x229B); !ok || name != "BORDER" {
		t.Errorf("constant 229B: %q %v", name, ok)
	}
	if name, ok := tab.Name(0x229B); !ok || name != "BORDER" {
		t.Errorf("Name(229B): %q %v", name, ok)
	}
	if name, _ := tab.Name(0x8000); name != "START" {
		t.Errorf("Name(8000): %q", name)
	}
	if _, ok := tab.Name(0x1234); ok {
		t.Error("a name for an address nothing names")
	}
	if tab.Len() != 6 {
		t.Errorf("Len %d, want 4 labels and 2 constants", tab.Len())
	}
	labels := tab.Labels()
	labels[0x8000] = "changed"
	if name, _ := tab.Label(0x8000); name != "START" {
		t.Error("Labels did not return a copy")
	}
	if _, err := tab.AddFile(strings.NewReader("not a symbol file\n"), FormatAuto); err == nil {
		t.Error("a bad file was added")
	}
}
