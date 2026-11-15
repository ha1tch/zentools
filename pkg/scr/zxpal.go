// ZXMODE palettes (the ZXPAL system).
//
// The ZXMODE specification adds two linear screen modes to the Spectrum's
// classic one and five palettes, ZXPAL0 to ZXPAL4. A palette is a table of
// 24-bit registers; an attribute names two registers per 8x8 cell, ink and
// paper. ZXPAL0 is the Spectrum's own palette and attribute. ZXPAL1 and ZXPAL2
// keep one attribute byte but read bits 7-6 as a bank of eight registers.
// ZXPAL3 and ZXPAL4 use two attribute bytes per cell, each a register number.
//
// The register values are generated from the rules of the specification
// (Part 3, sections 7 to 11), not copied from tables; the test suite checks the
// result against the published ZXPAL4 table.

package scr

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// Palette identifies one of the five ZXPAL palettes.
type Palette uint8

const (
	ZXPAL0 Palette = iota // the Spectrum palette: 16 registers, flash
	ZXPAL1                // Spectrum colours in two banks plus 16 greys: 32 registers
	ZXPAL2                // the eight Spectrum colours at four levels: 32 registers
	ZXPAL3                // twelve hues and four greys at four levels: 64 registers, two-byte attribute
	ZXPAL4                // ZXPAL3 plus tones, tints and shades: 112 registers, two-byte attribute
)

// NumPalettes is the number of ZXPAL palettes.
const NumPalettes = 5

// String returns the palette's name, "ZXPAL0" to "ZXPAL4".
func (p Palette) String() string { return fmt.Sprintf("ZXPAL%d", uint8(p)) }

// ParsePalette accepts "zxpal0" to "zxpal4" (any case) or a bare digit 0 to 4.
func ParsePalette(s string) (Palette, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	t = strings.TrimPrefix(t, "zxpal")
	if len(t) == 1 && t[0] >= '0' && t[0] <= '4' {
		return Palette(t[0] - '0'), nil
	}
	return 0, fmt.Errorf("unknown palette %q: use zxpal0 to zxpal4", s)
}

// Valid reports whether p names one of the five palettes.
func (p Palette) Valid() bool { return p < NumPalettes }

// TwoByte reports whether the palette uses two attribute bytes per cell (ink,
// then paper), each holding a register number in bits 6-0.
func (p Palette) TwoByte() bool { return p >= ZXPAL3 }

// Last returns the palette's last register number.
func (p Palette) Last() int { return [...]int{15, 31, 31, 63, 111}[p] }

// Registers returns the palette's registers, register 0 first.
func (p Palette) Registers() []color.RGBA {
	var regs [][3]int
	switch p {
	case ZXPAL0:
		regs = zxpal0()
	case ZXPAL1:
		regs = zxpal1()
	case ZXPAL2:
		regs = zxpal2()
	case ZXPAL3:
		regs = zxpal3()
	case ZXPAL4:
		regs = zxpal4()
	}
	out := make([]color.RGBA, len(regs))
	for i, c := range regs {
		out[i] = color.RGBA{uint8(c[0]), uint8(c[1]), uint8(c[2]), 0xFF}
	}
	return out
}

// CellRegisters returns the ink and paper register numbers an attribute names.
// For a one-byte palette b is unused. flashPhase selects the second half of a
// ZXPAL0 flash cycle, where ink and paper swap; it has no effect elsewhere.
// Two-byte codes beyond the palette's last register show register 0.
func (p Palette) CellRegisters(a, b byte, flashPhase bool) (ink, paper int) {
	switch p {
	case ZXPAL0:
		bright := int(a>>6) & 1
		ink, paper = 8*bright+int(a&7), 8*bright+int(a>>3&7)
		if flashPhase && a&0x80 != 0 {
			ink, paper = paper, ink
		}
	case ZXPAL1, ZXPAL2:
		bank := int(a >> 6)
		ink, paper = 8*bank+int(a&7), 8*bank+int(a>>3&7)
	default:
		ink, paper = int(a&0x7F), int(b&0x7F)
		if ink > p.Last() {
			ink = 0
		}
		if paper > p.Last() {
			paper = 0
		}
	}
	return ink, paper
}

// spectrumRGB is Spectrum colour index i (bit 0 blue, bit 1 red, bit 2 green)
// with every lit channel at level.
func spectrumRGB(i, level int) [3]int {
	var c [3]int
	if i&2 != 0 {
		c[0] = level
	}
	if i&4 != 0 {
		c[1] = level
	}
	if i&1 != 0 {
		c[2] = level
	}
	return c
}

func hueBank(level int) [][3]int {
	out := make([][3]int, 8)
	for i := range out {
		out[i] = spectrumRGB(i, level)
	}
	return out
}

func zxpal0() [][3]int { return append(hueBank(0xC8), hueBank(0xFF)...) }

func zxpal1() [][3]int {
	regs := zxpal0()
	for i := 0; i < 8; i++ {
		v := 0x22 * i
		regs = append(regs, [3]int{v, v, v})
	}
	for i := 0; i < 8; i++ {
		v := 0x11 + 0x22*i
		regs = append(regs, [3]int{v, v, v})
	}
	return regs
}

func zxpal2() [][3]int {
	var regs [][3]int
	for _, lv := range []int{0x40, 0x80, 0xBF, 0xFF} {
		regs = append(regs, hueBank(lv)...)
	}
	return regs
}

func zxpal3() [][3]int {
	var regs [][3]int
	for _, L := range []int{0x40, 0x80, 0xBF, 0xFF} {
		h := L / 2
		if L == 0xFF {
			h = 0x80
		}
		regs = append(regs,
			[3]int{0, 0, 0}, [3]int{L / 3, L / 3, L / 3}, [3]int{2 * L / 3, 2 * L / 3, 2 * L / 3}, [3]int{L, L, L},
			[3]int{L, 0, 0}, [3]int{L, h, 0}, [3]int{L, L, 0}, [3]int{h, L, 0},
			[3]int{0, L, 0}, [3]int{0, L, h}, [3]int{0, L, L}, [3]int{0, h, L},
			[3]int{0, 0, L}, [3]int{h, 0, L}, [3]int{L, 0, L}, [3]int{L, 0, h})
	}
	return regs
}

// mixInt is c + floor(((g - c) * k + 8) / 16) per channel, with a floor
// division as in the specification's reference implementation.
func mixInt(c [3]int, g, k int) [3]int {
	var out [3]int
	for i, x := range c {
		out[i] = x + floorDiv((g-x)*k+8, 16)
	}
	return out
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func zxpal4() [][3]int {
	regs := zxpal3()
	tone1 := make([][3]int, 0, 12)
	for _, c := range regs[52:64] {
		tone1 = append(tone1, mixInt(c, 128, 7))
	}
	for _, k := range []func([3]int) [3]int{
		func(t [3]int) [3]int { return t },
		func(t [3]int) [3]int { return mixInt(t, 128, 9) },
		func(t [3]int) [3]int { return mixInt(t, 255, 4) },
		func(t [3]int) [3]int { return mixInt(t, 0, 7) },
	} {
		for _, t := range tone1 {
			regs = append(regs, k(t))
		}
	}
	return regs
}

// Registers is a set of register colours for a palette, register 0 first.
// The ZXMODE palette registers are loadable (Part 5): software may overwrite
// them, so a picture or an asset can be meant for registers other than the
// palette's reset values. A nil Registers stands for the reset values.
type Registers []color.RGBA

// Or returns r, or the palette's reset values when r is nil.
func (r Registers) Or(p Palette) Registers {
	if r == nil {
		return p.Registers()
	}
	return r
}

// Equal reports whether two register sets hold the same colours.
func (r Registers) Equal(o Registers) bool {
	if len(r) != len(o) {
		return false
	}
	for i := range r {
		if r[i] != o[i] {
			return false
		}
	}
	return true
}

// CheckRegisters reports whether regs fits palette p: one colour for each of
// its registers.
func CheckRegisters(p Palette, regs Registers) error {
	if regs != nil && len(regs) != p.Last()+1 {
		return fmt.Errorf("scr: %s has %d registers; got %d", p, p.Last()+1, len(regs))
	}
	return nil
}

// RegistersCSV writes registers as `zx scr palette -o file.csv` does: a
// header, then register,hex,r,g,b per row.
func RegistersCSV(regs Registers) []byte {
	var b strings.Builder
	b.WriteString("register,hex,r,g,b\n")
	for i, c := range regs {
		fmt.Fprintf(&b, "%d,%02X%02X%02X,%d,%d,%d\n", i, c.R, c.G, c.B, c.R, c.G, c.B)
	}
	return []byte(b.String())
}

// ParseRegistersCSV reads a register table for palette p from CSV with a
// header row. The colour comes from columns named r, g and b (or, failing
// those, hex as RRGGBB); the register number from a column named register,
// slot or index, or else from the row's position. Every register of p must
// appear exactly once. It reads the tables `zx scr palette` writes and the
// specification's data/zxpalN_registers.csv files.
func ParseRegistersCSV(data []byte, p Palette) (Registers, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var rows [][]string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			rows = append(rows, strings.Split(l, ","))
		}
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("scr: a register table needs a header row and one row per register")
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	idx := -1
	for _, name := range []string{"register", "slot", "index"} {
		if i, ok := col[name]; ok {
			idx = i
			break
		}
	}
	ri, okR := col["r"]
	gi, okG := col["g"]
	bi, okB := col["b"]
	hi, okHex := col["hex"]
	if !(okR && okG && okB) && !okHex {
		return nil, fmt.Errorf("scr: a register table needs r, g and b columns, or a hex column")
	}
	n := p.Last() + 1
	regs := make(Registers, n)
	seen := make([]bool, n)
	field := func(row []string, i int) string {
		if i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	for k, row := range rows[1:] {
		reg := k
		if idx >= 0 {
			v, err := strconv.Atoi(field(row, idx))
			if err != nil {
				return nil, fmt.Errorf("scr: register table row %d: register %q", k+2, field(row, idx))
			}
			reg = v
		}
		if reg < 0 || reg >= n {
			return nil, fmt.Errorf("scr: register table row %d: register %d is outside %s (0-%d)", k+2, reg, p, n-1)
		}
		if seen[reg] {
			return nil, fmt.Errorf("scr: register table row %d: register %d appears twice", k+2, reg)
		}
		var c [3]int
		if okR && okG && okB {
			for j, i := range []int{ri, gi, bi} {
				v, err := strconv.Atoi(field(row, i))
				if err != nil || v < 0 || v > 255 {
					return nil, fmt.Errorf("scr: register table row %d: channel %q is not 0-255", k+2, field(row, i))
				}
				c[j] = v
			}
		} else {
			v, err := strconv.ParseUint(strings.TrimPrefix(field(row, hi), "#"), 16, 32)
			if err != nil || len(strings.TrimPrefix(field(row, hi), "#")) != 6 {
				return nil, fmt.Errorf("scr: register table row %d: hex %q is not RRGGBB", k+2, field(row, hi))
			}
			c = [3]int{int(v >> 16), int(v >> 8 & 0xFF), int(v & 0xFF)}
		}
		regs[reg] = color.RGBA{uint8(c[0]), uint8(c[1]), uint8(c[2]), 0xFF}
		seen[reg] = true
	}
	for i, s := range seen {
		if !s {
			return nil, fmt.Errorf("scr: register table has no register %d; %s needs all %d", i, p, n)
		}
	}
	return regs, nil
}
