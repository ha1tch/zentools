package symfile

import (
	"fmt"
	"io"
	"sort"
)

// Table is the names of one or more symbol files, by address, ready to
// be used: what a disassembler or a debugger keeps for as long as a
// program is loaded, where Parse and Labels are the steps that make it.
//
// Files are added in order of preference: where two give a name to the
// same address, the one added first keeps it. Names are made fit for an
// assembler as Labels makes them, and no two addresses get the same
// name: a second one has its address appended. What a format marks as a
// constant is kept apart, by value, since a constant is not a place in
// the program but can be the address of a routine outside it (a ROM
// call is usually written against an EQU).
//
// The zero Table is not ready; use NewTable. A Table is not safe to add
// to from several goroutines at once; reading it from several is.
type Table struct {
	labels    map[uint16]string
	constants map[uint16]string
	names     map[string]bool
}

// NewTable returns an empty Table.
func NewTable() *Table {
	return &Table{labels: map[uint16]string{}, constants: map[uint16]string{}, names: map[string]bool{}}
}

// Add puts the symbols of one file in the table. Addresses and values
// that already have a name keep it.
func (t *Table) Add(symbols []Symbol) {
	labels := Labels(symbols)
	addrs := make([]int, 0, len(labels))
	for addr := range labels {
		addrs = append(addrs, int(addr))
	}
	sort.Ints(addrs)
	for _, a := range addrs {
		addr := uint16(a)
		if _, taken := t.labels[addr]; taken {
			continue
		}
		name := labels[addr]
		if t.names[name] {
			name = fmt.Sprintf("%s_%04X", name, addr)
		}
		t.names[name] = true
		t.labels[addr] = name
	}
	for value, name := range Constants(symbols) {
		if _, taken := t.constants[value]; !taken {
			t.constants[value] = name
		}
	}
}

// AddFile parses a symbol file and adds it. It returns how many symbols
// the file had.
func (t *Table) AddFile(r io.Reader, format Format) (int, error) {
	symbols, err := Parse(r, format)
	if err != nil {
		return 0, err
	}
	t.Add(symbols)
	return len(symbols), nil
}

// Label returns the name of an address in the program, if it has one.
func (t *Table) Label(addr uint16) (string, bool) {
	name, ok := t.labels[addr]
	return name, ok
}

// Constant returns the name of a constant with this value, if there is
// one.
func (t *Table) Constant(value uint16) (string, bool) {
	name, ok := t.constants[value]
	return name, ok
}

// Name returns what to call an address: its label, or failing that a
// constant with that value.
func (t *Table) Name(addr uint16) (string, bool) {
	if name, ok := t.labels[addr]; ok {
		return name, true
	}
	return t.Constant(addr)
}

// Labels returns a copy of the labels, by address.
func (t *Table) Labels() map[uint16]string {
	out := make(map[uint16]string, len(t.labels))
	for addr, name := range t.labels {
		out[addr] = name
	}
	return out
}

// Constants returns a copy of the constants, by value.
func (t *Table) Constants() map[uint16]string {
	out := make(map[uint16]string, len(t.constants))
	for value, name := range t.constants {
		out[value] = name
	}
	return out
}

// Len returns how many labels and constants the table holds.
func (t *Table) Len() int { return len(t.labels) + len(t.constants) }

// Identifier makes a name from a symbol file one an assembler takes as a
// label, the way Labels and Table do: a character that is not a letter,
// a digit or an underscore becomes an underscore (CSpect's START@LOOP,
// sjasmplus's start.loop), and a name that starts with a digit gets an
// underscore in front.
func Identifier(name string) string { return identifier(name) }
