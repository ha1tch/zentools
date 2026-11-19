// Package symfile reads and writes the symbol files of other Z80 tools,
// so that names can travel between an assembler, a disassembler and a
// debugger: zenas writes and reads them, zendis reads and writes them, and
// a program that embeds both passes Symbols from one to the other.
//
// Four formats are read:
//
//   - cspect: the map file CSpect's debugger loads with -map, which SNASM
//     and sjasmplus (CSPECTMAP) write. One symbol a line:
//     "<16-bit address> <physical address> <type> <name>", the first two as
//     eight hex digits. The physical address is the 8K page times 8192
//     plus the offset in it. The type is 00 for a label, 01 for an EQU,
//     02 for a redefinable value (sjasmplus's DEFL) and FFFFFFFF for
//     unknown. A local label is PRIMARY@LOCAL. SNASM's files open with a
//     [SYMBOLS] line and carry [FILES] and [LINES] sections after the
//     symbols; those are skipped.
//   - pasmo: "NAME<tabs>EQU 0XXXXH", which is also what zenas writes
//     with --sym.
//   - sjasmplus: "NAME: EQU 0xXXXXXXXX", its --sym output.
//   - z88dk: the .map file, "name = $XXXX ; addr, public, ..." with
//     "const" in place of "addr" for a constant.
//
// One is written: cspect.
//
// The formats were taken from files the tools themselves wrote (the maps
// shipped with CSpect 4.5.1.0, sjasmplus 1.24.1's CSPECTMAP and --sym
// output, zenas 0.11.2's --sym output) and from CSpect's ReadMe. z88dk's
// is written from its documented shape and was not checked against a
// file made by z88dk.
package symfile

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Kind says what a symbol is, where the format tells.
type Kind int

const (
	KindUnknown Kind = iota // the format does not say (pasmo, sjasmplus), or says unknown
	KindLabel               // an address in the program
	KindEqu                 // a constant
	KindDefl                // a value that can be set again (sjasmplus's DEFL or =)
)

// Symbol is one name from a symbol file.
type Symbol struct {
	Name    string
	Value   uint32 // as the file gives it; an address when it is 0xFFFF or less
	Phys    uint32 // the physical address, where the format has one
	HasPhys bool
	Kind    Kind
}

// Format names a symbol file format.
type Format string

const (
	FormatAuto      Format = "auto"
	FormatCSpect    Format = "cspect"
	FormatPasmo     Format = "pasmo"
	FormatSjasmplus Format = "sjasmplus"
	FormatZ88dk     Format = "z88dk"
)

// ParseFormat turns a name given on a command line into a Format. "zenas"
// is pasmo's format, "snasm" and "map" are cspect's, and "" is auto.
func ParseFormat(name string) (Format, error) {
	switch strings.ToLower(name) {
	case "", "auto":
		return FormatAuto, nil
	case "cspect", "snasm", "map":
		return FormatCSpect, nil
	case "pasmo", "zenas":
		return FormatPasmo, nil
	case "sjasmplus":
		return FormatSjasmplus, nil
	case "z88dk":
		return FormatZ88dk, nil
	}
	return "", fmt.Errorf("unknown symbol file format %q (want auto, cspect, pasmo, zenas, sjasmplus or z88dk)", name)
}

var (
	reCSpect    = regexp.MustCompile(`^([0-9A-Fa-f]{1,8})\s+([0-9A-Fa-f]{1,8})\s+([0-9A-Fa-f]{1,8})\s+(\S+)\s*$`)
	rePasmo     = regexp.MustCompile(`^(\S+)\s+EQU\s+0([0-9A-Fa-f]+)H\s*$`)
	reSjasmplus = regexp.MustCompile(`^(\S+):\s+EQU\s+0x([0-9A-Fa-f]+)\s*$`)
	reZ88dk     = regexp.MustCompile(`^(\S+)\s*=\s*\$([0-9A-Fa-f]+)\s*(?:;\s*(\w+))?`)
)

// detect picks the format from the first line that is not blank.
func detect(lines []string) (Format, error) {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "["), reCSpect.MatchString(line):
			return FormatCSpect, nil
		case reSjasmplus.MatchString(line):
			return FormatSjasmplus, nil
		case rePasmo.MatchString(line):
			return FormatPasmo, nil
		case reZ88dk.MatchString(line):
			return FormatZ88dk, nil
		}
		return "", fmt.Errorf("symbol file: cannot tell the format from its first line, %q; name it (cspect, pasmo, zenas, sjasmplus or z88dk)", line)
	}
	return FormatAuto, nil // an empty file: no symbols in any format
}

// Parse reads a symbol file. With FormatAuto the format is taken from the
// first line that is not blank. A line that does not fit the format is an
// error naming its number; nothing is skipped silently.
func Parse(r io.Reader, format Format) ([]Symbol, error) {
	var lines []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if format == FormatAuto || format == "" {
		f, err := detect(lines)
		if err != nil {
			return nil, err
		}
		format = f
	}

	var out []Symbol
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		bad := func() error {
			return fmt.Errorf("symbol file:%d: not a %s symbol line: %q", i+1, format, line)
		}
		switch format {
		case FormatCSpect:
			if strings.HasPrefix(line, "[") {
				if strings.EqualFold(line, "[SYMBOLS]") {
					continue
				}
				return out, nil // [FILES], [LINES], [END]: the symbols are over
			}
			m := reCSpect.FindStringSubmatch(line)
			if m == nil {
				return nil, bad()
			}
			value, _ := strconv.ParseUint(m[1], 16, 32)
			phys, _ := strconv.ParseUint(m[2], 16, 32)
			kind := KindUnknown
			switch t, _ := strconv.ParseUint(m[3], 16, 32); t {
			case 0:
				kind = KindLabel
			case 1:
				kind = KindEqu
			case 2:
				kind = KindDefl
			}
			out = append(out, Symbol{Name: m[4], Value: uint32(value), Phys: uint32(phys), HasPhys: true, Kind: kind})
		case FormatPasmo, FormatSjasmplus:
			re := rePasmo
			if format == FormatSjasmplus {
				re = reSjasmplus
			}
			m := re.FindStringSubmatch(line)
			if m == nil {
				return nil, bad()
			}
			value, err := strconv.ParseUint(m[2], 16, 32)
			if err != nil {
				return nil, bad()
			}
			out = append(out, Symbol{Name: m[1], Value: uint32(value)})
		case FormatZ88dk:
			m := reZ88dk.FindStringSubmatch(line)
			if m == nil {
				return nil, bad()
			}
			value, err := strconv.ParseUint(m[2], 16, 32)
			if err != nil {
				return nil, bad()
			}
			kind := KindUnknown
			switch strings.ToLower(m[3]) {
			case "addr":
				kind = KindLabel
			case "const":
				kind = KindEqu
			}
			out = append(out, Symbol{Name: m[1], Value: uint32(value), Kind: kind})
		}
	}
	return out, nil
}

// Labels picks the symbols that can name an address in a disassembly and
// returns them by address. A constant (KindEqu, KindDefl) is left out, and
// so is a value above 0xFFFF. Where the format does not say what a symbol
// is (pasmo, zenas, sjasmplus) every value that fits is taken, so a
// constant there names the address equal to it.
//
// A name is made fit for an assembler: any character that is not a letter,
// a digit or an underscore becomes an underscore (START@LOOP and
// start.loop both become a name with an underscore), and a name that
// starts with a digit gets one in front. When two symbols have the same
// address the first by name is kept; when two addresses would get the
// same name the later one has its address appended.
func Labels(symbols []Symbol) map[uint16]string {
	sorted := make([]Symbol, len(symbols))
	copy(sorted, symbols)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	labels := map[uint16]string{}
	used := map[string]bool{}
	for _, s := range sorted {
		if s.Kind == KindEqu || s.Kind == KindDefl || s.Value > 0xFFFF {
			continue
		}
		addr := uint16(s.Value)
		if _, taken := labels[addr]; taken {
			continue
		}
		name := identifier(s.Name)
		if used[name] {
			name = fmt.Sprintf("%s_%04X", name, addr)
		}
		used[name] = true
		labels[addr] = name
	}
	return labels
}

// Constants picks the symbols a format marks as constants (KindEqu,
// KindDefl) whose value fits in 16 bits, by value, with names made fit as
// Labels makes them and the first by name kept for each value. A constant
// is not a label, but one can be the address of a routine outside the
// program: a ROM call is usually written against an EQU. A caller that
// knows which addresses are branched to and lie outside the code can name
// those from here.
func Constants(symbols []Symbol) map[uint16]string {
	sorted := make([]Symbol, len(symbols))
	copy(sorted, symbols)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	out := map[uint16]string{}
	for _, s := range sorted {
		if (s.Kind != KindEqu && s.Kind != KindDefl) || s.Value > 0xFFFF {
			continue
		}
		if _, taken := out[uint16(s.Value)]; !taken {
			out[uint16(s.Value)] = identifier(s.Name)
		}
	}
	return out
}

// identifier makes a name an assembler takes as a label.
func identifier(name string) string {
	var b strings.Builder
	for i, r := range name {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_'
		digit := r >= '0' && r <= '9'
		if i == 0 && digit {
			b.WriteByte('_')
		}
		if letter || digit {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}

// nextPages is the 8K page in each of the eight slots of a Next's 64K as
// a program finds them: the ROM's two slots are numbered as sjasmplus
// numbers them for its ZXSPECTRUMNEXT device (pages 14 and 15), then bank
// 5, bank 2 and bank 0.
var nextPages = [8]uint32{14, 15, 10, 11, 4, 5, 0, 1}

// NextPhys gives the physical address a CSpect map wants for a 16-bit
// address, with the Next's memory paged as it is when a program starts:
// the page in the address's slot times 8192, plus the offset in the page.
// It is what sjasmplus writes for a ZXSPECTRUMNEXT device whose paging the
// source does not change.
func NextPhys(addr uint16) uint32 {
	return nextPages[addr>>13]*0x2000 + uint32(addr&0x1FFF)
}

// WriteCSpectMap writes symbols as a CSpect map file, sorted by name, in
// the shape sjasmplus writes: no section lines, eight hex digits for each
// address, two for the type (FFFFFFFF for unknown, as SNASM writes it).
// A symbol without a physical address gets NextPhys of its value when it
// is a label or unknown, and its value's offset in an 8K page when it is a
// constant (page 0), which is what sjasmplus writes for an EQU.
func WriteCSpectMap(w io.Writer, symbols []Symbol) error {
	sorted := make([]Symbol, len(symbols))
	copy(sorted, symbols)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	for _, s := range sorted {
		phys := s.Phys
		if !s.HasPhys {
			phys = s.Value & 0x1FFF
			if (s.Kind == KindLabel || s.Kind == KindUnknown) && s.Value <= 0xFFFF {
				phys = NextPhys(uint16(s.Value))
			}
		}
		kind := "FFFFFFFF"
		switch s.Kind {
		case KindLabel:
			kind = "00"
		case KindEqu:
			kind = "01"
		case KindDefl:
			kind = "02"
		}
		if _, err := fmt.Fprintf(w, "%08X %08X %s %s\n", s.Value, phys, kind, s.Name); err != nil {
			return err
		}
	}
	return nil
}

// FromLabels turns a disassembly's name-to-address set into symbols for
// WriteCSpectMap: every one a label.
func FromLabels(labels map[string]uint16) []Symbol {
	out := make([]Symbol, 0, len(labels))
	for name, addr := range labels {
		out = append(out, Symbol{Name: name, Value: uint32(addr), Kind: KindLabel})
	}
	return out
}
