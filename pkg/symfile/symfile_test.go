package symfile

import (
	"bytes"
	"strings"
	"testing"
)

// What sjasmplus 1.24.1 wrote with CSPECTMAP for a ZXSPECTRUMNEXT device:
// a constant, a DEFL, labels in each part of memory with the default
// paging, and one in a page put in the top slot with MMU 7 n, 30.
const sjasmplusMap = `00000005 00000005 01 CONST
00000007 00000007 02 VAR
0000E000 0003C000 00 BANKED
0000E001 0003C001 00 BANKED@INNER
0000C000 00000000 00 HIGH
00000100 0001C100 00 ROM
00004000 00014000 00 SCR
00008000 00008000 00 START
00008002 00008002 00 START@LOOP
`

func TestCSpectMapAsSjasmplusWritesIt(t *testing.T) {
	syms, err := Parse(strings.NewReader(sjasmplusMap), FormatAuto)
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 9 {
		t.Fatalf("%d symbols, want 9", len(syms))
	}
	if s := syms[0]; s.Name != "CONST" || s.Value != 5 || s.Kind != KindEqu {
		t.Errorf("first symbol: %+v", s)
	}
	if s := syms[1]; s.Kind != KindDefl {
		t.Errorf("VAR's kind is %v, want a DEFL", s.Kind)
	}
	if s := syms[2]; s.Value != 0xE000 || s.Phys != 0x3C000 || !s.HasPhys || s.Kind != KindLabel {
		t.Errorf("BANKED: %+v", s)
	}
	labels := Labels(syms)
	want := map[uint16]string{0xE000: "BANKED", 0xE001: "BANKED_INNER", 0xC000: "HIGH", 0x0100: "ROM", 0x4000: "SCR", 0x8000: "START", 0x8002: "START_LOOP"}
	if len(labels) != len(want) {
		t.Errorf("%d labels, want %d: %v", len(labels), len(want), labels)
	}
	for addr, name := range want {
		if labels[addr] != name {
			t.Errorf("label at %04X is %q, want %q", addr, labels[addr], name)
		}
	}
	if _, ok := labels[5]; ok {
		t.Error("the constant CONST became a label at address 5")
	}
	if c := Constants(syms); len(c) != 2 || c[5] != "CONST" || c[7] != "VAR" {
		t.Errorf("constants: %v", c)
	}

	// Written back, the file is the one that was read.
	var out bytes.Buffer
	if err := WriteCSpectMap(&out, syms); err != nil {
		t.Fatal(err)
	}
	if lines(out.String()) != lines(sjasmplusMap) {
		t.Errorf("written back:\n%s\nwant (in any order):\n%s", out.String(), sjasmplusMap)
	}
}

// lines sorts a file's lines, to compare two files whatever their order.
func lines(s string) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	for i := range l {
		for j := i + 1; j < len(l); j++ {
			if l[j] < l[i] {
				l[i], l[j] = l[j], l[i]
			}
		}
	}
	return strings.Join(l, "\n")
}

func TestNextPhysIsWhatSjasmplusGives(t *testing.T) {
	// The labels of sjasmplusMap that are in the default paging.
	for addr, want := range map[uint16]uint32{0xC000: 0, 0x0100: 0x1C100, 0x4000: 0x14000, 0x8000: 0x8000, 0x8002: 0x8002, 0xFFFF: 0x3FFF, 0xA000: 0xA000, 0x6000: 0x16000} {
		if got := NextPhys(addr); got != want {
			t.Errorf("NextPhys(%04X) = %06X, want %06X", addr, got, want)
		}
	}
	var out bytes.Buffer
	WriteCSpectMap(&out, FromLabels(map[string]uint16{"start": 0x8000, "high": 0xC000, "scr": 0x4000}))
	want := "0000C000 00000000 00 high\n00004000 00014000 00 scr\n00008000 00008000 00 start\n"
	if out.String() != want {
		t.Errorf("a map from labels:\n%swant:\n%s", out.String(), want)
	}
	// A constant with no physical address given: sjasmplus writes
	// "0000229B 0000029B 01 BORDER" for BORDER EQU 0x229B.
	out.Reset()
	WriteCSpectMap(&out, []Symbol{{Name: "BORDER", Value: 0x229B, Kind: KindEqu}, {Name: "COUNT", Value: 5, Kind: KindEqu}})
	if want := "0000229B 0000029B 01 BORDER\n00000005 00000005 01 COUNT\n"; out.String() != want {
		t.Errorf("constants:\n%swant:\n%s", out.String(), want)
	}
}

func TestCSpectMapAsSNASMWritesIt(t *testing.T) {
	// The shape of the maps that come with CSpect: a [SYMBOLS] line, types
	// of two digits or FFFFFFFF, constants with no physical address, then
	// sections that are not symbols.
	const snasm = "[SYMBOLS]\r\n" +
		"00000001 00000000 01 MOUSE_LB\r\n" +
		"000001ED 00000000 FFFFFFFF LOADFILE\r\n" +
		"00005C5C 00005C5C 00 IM2ROUTINE\r\n" +
		"00005E22 00005E22 00 DOIRQ@SECONDSPLIT\r\n" +
		"[FILES]\r\n" +
		"C:\\source\\irq.asm\r\n" +
		"[LINES]\r\n" +
		"00005C5C 8 0\r\n" +
		"[END]\r\n"
	syms, err := Parse(strings.NewReader(snasm), FormatAuto)
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 4 {
		t.Fatalf("%d symbols, want 4: the [FILES] and [LINES] sections are not symbols", len(syms))
	}
	if syms[1].Kind != KindUnknown || syms[0].Kind != KindEqu || syms[2].Kind != KindLabel {
		t.Errorf("kinds: %v %v %v", syms[0].Kind, syms[1].Kind, syms[2].Kind)
	}
	labels := Labels(syms)
	if labels[0x5E22] != "DOIRQ_SECONDSPLIT" || labels[0x01ED] != "LOADFILE" || len(labels) != 3 {
		t.Errorf("labels: %v", labels)
	}
}

func TestPasmoSjasmplusAndZ88dk(t *testing.T) {
	for _, c := range []struct {
		name   string
		format Format
		text   string
		kind   Kind
	}{
		{"pasmo and zenas", FormatPasmo, "main\t\tEQU 08000H\nkernel_end\tEQU 09A3CH\na_very_long_symbol_name EQU 0C000H\n", KindUnknown},
		{"sjasmplus", FormatSjasmplus, "main: EQU 0x00008000\nkernel_end: EQU 0x00009A3C\nstart.loop: EQU 0x0000C000\n", KindUnknown},
		{"z88dk", FormatZ88dk, "_main                           = $8000 ; addr, public, , main_c, code_compiler, main.c:5\nkernel_end = $9A3C ; addr, local, , k, code, k.asm:9\n__top = $C000 ; addr, public, , , ,\n", KindLabel},
	} {
		for _, f := range []Format{FormatAuto, c.format} {
			syms, err := Parse(strings.NewReader(c.text), f)
			if err != nil {
				t.Errorf("%s (%s): %v", c.name, f, err)
				continue
			}
			if len(syms) != 3 || syms[0].Value != 0x8000 || syms[1].Value != 0x9A3C || syms[2].Value != 0xC000 || syms[0].Kind != c.kind {
				t.Errorf("%s (%s): %+v", c.name, f, syms)
			}
		}
	}
	// A z88dk constant is not a label; a value above 64K is not an address.
	syms, err := Parse(strings.NewReader("SIZE = $0010 ; const, local, , m, , m.asm:1\nfar = $12345 ; addr, public, , m, , m.asm:2\n"), FormatAuto)
	if err != nil {
		t.Fatal(err)
	}
	if l := Labels(syms); len(l) != 0 {
		t.Errorf("labels from a constant and a 20-bit value: %v", l)
	}
}

func TestLabelsAreDeterministicAndUnique(t *testing.T) {
	syms := []Symbol{
		{Name: "zeta", Value: 0x8000}, {Name: "alpha", Value: 0x8000}, // two names, one address
		{Name: "a.b", Value: 0x9000}, {Name: "a@b", Value: 0x9001}, {Name: "a_b", Value: 0x9002}, // three names that become one
		{Name: "1st", Value: 0xA000},
	}
	l := Labels(syms)
	if l[0x8000] != "alpha" {
		t.Errorf("at 8000: %q, want the first by name", l[0x8000])
	}
	seen := map[string]bool{}
	for _, n := range l {
		if seen[n] {
			t.Errorf("the name %q is given to two addresses: %v", n, l)
		}
		seen[n] = true
	}
	if l[0xA000] != "_1st" {
		t.Errorf("a name that starts with a digit became %q", l[0xA000])
	}
}

func TestBadLinesAreErrors(t *testing.T) {
	if _, err := Parse(strings.NewReader("main\t\tEQU 08000H\nthis is not a symbol\n"), FormatAuto); err == nil || !strings.Contains(err.Error(), ":2:") {
		t.Errorf("a bad second line: %v", err)
	}
	if _, err := Parse(strings.NewReader("what is this\n"), FormatAuto); err == nil {
		t.Error("a file in no known format was accepted")
	}
	if syms, err := Parse(strings.NewReader("\n\n"), FormatAuto); err != nil || len(syms) != 0 {
		t.Errorf("an empty file: %v, %v", syms, err)
	}
	if _, err := ParseFormat("vice"); err == nil {
		t.Error("an unknown format name was accepted")
	}
	if f, _ := ParseFormat("zenas"); f != FormatPasmo {
		t.Errorf("zenas is %q, want pasmo's format", f)
	}
}
