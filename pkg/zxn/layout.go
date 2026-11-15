// file: layout.go
//
// Palette layouts other than the 2-byte .nxp entry, the Next's standard
// palette, and palette minimising, all as gfx2next writes them.

package zxn

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strings"
)

// Layout is the byte layout of one palette entry in a file.
type Layout int

const (
	// LayoutRGB333 is the .nxp layout: two bytes, RGB332 then the low blue
	// bit (and the priority bit in bit 7). This is the default.
	LayoutRGB333 Layout = iota
	// LayoutRGB332 is one byte, RRRGGGBB, the top two bits of the 3-bit
	// blue. The Next shows the missing low blue bit as the OR of the two.
	LayoutRGB332
	// LayoutGBR444 is two bytes, GGGGBBBB then xxxxRRRR (gfx2next's
	// -pal-gbr444), 4-bit channels as the eZ80 and some FPGA cores use.
	LayoutGBR444
	// LayoutBGR222 is one byte, xxBBGGRR (gfx2next's -pal-bgr222), the
	// Sega Master System palette layout.
	LayoutBGR222
)

// ParseLayout accepts "rgb333" (or "nxp"), "rgb332", "gbr444" and "bgr222".
func ParseLayout(s string) (Layout, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "rgb333", "nxp":
		return LayoutRGB333, nil
	case "rgb332":
		return LayoutRGB332, nil
	case "gbr444":
		return LayoutGBR444, nil
	case "bgr222":
		return LayoutBGR222, nil
	}
	return LayoutRGB333, fmt.Errorf("unknown palette layout %q (want rgb333, rgb332, gbr444 or bgr222)", s)
}

func (l Layout) String() string {
	switch l {
	case LayoutRGB332:
		return "rgb332"
	case LayoutGBR444:
		return "gbr444"
	case LayoutBGR222:
		return "bgr222"
	}
	return "rgb333"
}

// EntrySize is the number of bytes per palette entry.
func (l Layout) EntrySize() int {
	if l == LayoutRGB332 || l == LayoutBGR222 {
		return 1
	}
	return 2
}

// EncodePalette writes the palette in a layout. The 4-bit and 2-bit layouts
// are taken from each colour's 8-bit level, reduced as mode says, which is
// how gfx2next derives them from an RGB333 palette.
func EncodePalette(p *Palette, l Layout, mode RoundMode) ([]byte, error) {
	if l == LayoutRGB333 {
		return EncodeNXP(p)
	}
	if len(p.Colors) > MaxColors {
		return nil, fmt.Errorf("zxn: palette has %d colours, at most %d", len(p.Colors), MaxColors)
	}
	out := make([]byte, 0, l.EntrySize()*len(p.Colors))
	for _, c := range p.Colors {
		switch l {
		case LayoutRGB332:
			out = append(out, (c.R&7)<<5|(c.G&7)<<2|(c.B&7)>>1)
		case LayoutGBR444:
			r, g, b := toBits(level8[c.R&7], 15, mode), toBits(level8[c.G&7], 15, mode), toBits(level8[c.B&7], 15, mode)
			out = append(out, g<<4|b, r)
		case LayoutBGR222:
			r, g, b := toBits(level8[c.R&7], 3, mode), toBits(level8[c.G&7], 3, mode), toBits(level8[c.B&7], 3, mode)
			out = append(out, b<<4|g<<2|r)
		}
	}
	return out, nil
}

// DecodePaletteRGBA reads a palette file in any layout as 8-bit colours: an
// RGB333 or RGB332 level shown as Color333.RGBA shows it, a 4-bit level v as
// v x 17 and a 2-bit level as v x 85.
func DecodePaletteRGBA(data []byte, l Layout) ([]color.RGBA, error) {
	if l == LayoutRGB333 || l == LayoutRGB332 {
		p, err := DecodePalette(data, l)
		if err != nil {
			return nil, err
		}
		out := make([]color.RGBA, len(p.Colors))
		for i, c := range p.Colors {
			out[i] = c.RGBA()
		}
		return out, nil
	}
	n := l.EntrySize()
	if len(data) == 0 || len(data)%n != 0 || len(data) > n*MaxColors {
		return nil, fmt.Errorf("zxn: a %s palette is 1 to %d entries of %d bytes; got %d bytes", l, MaxColors, n, len(data))
	}
	out := make([]color.RGBA, len(data)/n)
	for i := range out {
		if l == LayoutGBR444 {
			b0, b1 := data[2*i], data[2*i+1]
			out[i] = color.RGBA{R: (b1 & 15) * 17, G: (b0 >> 4) * 17, B: (b0 & 15) * 17, A: 0xFF}
		} else {
			v := data[i]
			out[i] = color.RGBA{R: (v & 3) * 85, G: (v >> 2 & 3) * 85, B: (v >> 4 & 3) * 85, A: 0xFF}
		}
	}
	return out, nil
}

// DecodePalette reads a palette in one of the two layouts that hold Next
// colours exactly, RGB333 (.nxp) and RGB332 (whose low blue bit becomes the
// OR of the other two, as the hardware shows it). Other layouts are read with
// DecodePaletteRGBA.
func DecodePalette(data []byte, l Layout) (*Palette, error) {
	switch l {
	case LayoutRGB333:
		return DecodeNXP(data)
	case LayoutRGB332:
		if len(data) == 0 || len(data) > MaxColors {
			return nil, fmt.Errorf("zxn: an RGB332 palette is 1 to %d bytes; got %d", MaxColors, len(data))
		}
		p := &Palette{Colors: make([]Color333, len(data))}
		for i, v := range data {
			p.Colors[i] = rgb332(v)
		}
		return p, nil
	}
	return nil, fmt.Errorf("zxn: a %s palette does not hold Next colours exactly; read it as 8-bit colours", l)
}

// rgb332 is the Next colour an 8-bit RRRGGGBB value stands for: the low blue
// bit is the OR of the two given.
func rgb332(v uint8) Color333 {
	b2 := v & 3
	return Color333{R: v >> 5, G: v >> 2 & 7, B: b2<<1 | (b2>>1|b2)&1}
}

// toBits is c x max / 255 rounded as mode says, as gfx2next computes its
// 4-bit and 2-bit channels.
func toBits(c uint8, max float64, mode RoundMode) uint8 {
	v := float64(c) * max / 255.0
	switch mode {
	case RoundFloor:
		return uint8(math.Floor(v))
	case RoundCeil:
		return uint8(math.Ceil(v))
	}
	return uint8(math.Round(v))
}

// StandardPalette is the Next's default palette: entry i is the RGB332 value
// i, with the low blue bit the OR of the other two.
func StandardPalette() *Palette {
	p := &Palette{Colors: make([]Color333, MaxColors)}
	for i := range p.Colors {
		p.Colors[i] = rgb332(uint8(i))
	}
	return p
}

// StandardIndex is the standard-palette index for a colour: its RGB332
// value, red and green reduced to 3 bits and blue to 2, as mode says
// (gfx2next's -pal-std). With RoundDistance the colour is first taken to
// its nearest RGB333 colour, whose blue level is then rounded to 2 bits, as
// gfx2next does.
func StandardIndex(c color.Color, mode RoundMode) uint8 {
	if mode == RoundDistance {
		n := ToRGB333(c, mode)
		return n.R<<5 | n.G<<2 | toBits(level8[n.B], 3, RoundNearest)
	}
	r, g, b, _ := c.RGBA()
	return to3(uint8(r>>8), mode)<<5 | to3(uint8(g>>8), mode)<<2 | toBits(uint8(b>>8), 3, mode)
}

// Minimize sorts a palette's colours in ascending order (red, then green,
// then blue), drops duplicates and pads the end with black to the original
// length, as gfx2next's -pal-min does. It returns the new palette, the number
// of distinct colours, and a table taking each old index to its new one.
// gfx2next minimises all 256 entries, so to match it pass a palette padded
// to 256 (Padded): the padding's black then takes part, as it does there.
func Minimize(p *Palette) (*Palette, int, []uint8) {
	key := func(c Color333) int {
		k := int(c.R&7)<<6 | int(c.G&7)<<3 | int(c.B&7)
		if c.Priority {
			k |= 1 << 9
		}
		return k
	}
	sorted := append([]Color333(nil), p.Colors...)
	sort.SliceStable(sorted, func(i, j int) bool { return key(sorted[i]) < key(sorted[j]) })
	uniq := sorted[:0:0]
	for _, c := range sorted {
		if len(uniq) == 0 || uniq[len(uniq)-1] != c {
			uniq = append(uniq, c)
		}
	}
	n := len(uniq)
	pos := map[Color333]uint8{}
	for i, c := range uniq {
		pos[c] = uint8(i)
	}
	table := make([]uint8, len(p.Colors))
	for i, c := range p.Colors {
		table[i] = pos[c]
	}
	out := &Palette{Colors: uniq}
	return out.Padded(len(p.Colors)), n, table
}
