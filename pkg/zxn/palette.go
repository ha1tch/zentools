// Package zxn reads and writes ZX Spectrum Next graphics files: palettes
// (.nxp) and hardware sprite patterns (.spr).
//
// Every Next colour is 9-bit RGB333: three bits each of red, green and blue.
// A palette file holds 16 or 256 entries of two bytes each: the first is the
// colour as RGB332 (RRRGGGBB, the top two blue bits), the second holds the
// lowest blue bit in bit 0. Bit 7 of the second byte is the Layer 2 priority
// bit of NextReg 0x44; the community tools write it as 0, and this package
// keeps whatever a file holds so a palette survives a round trip unchanged.
//
// The conversions follow the established tools (gfx2next and nextbmp), whose
// output the tests check byte for byte: an 8-bit channel c becomes
// c x 7 / 255 rounded to the nearest integer (or down, or up), and a 3-bit
// channel v is shown as round(v x 255 / 7). (For floor and ceil, gfx2next
// reduces a palette colour twice, moving it a further step; zxn reduces once,
// as nextbmp does.)
//
// The package began as zenpal, a separate palette and sprite tool, and was
// folded into zentools.
package zxn

import (
	"fmt"
	"image/color"
	"math"
	"strings"
)

// MaxColors is the number of entries in one Next hardware palette.
const MaxColors = 256

// RoundMode selects how an 8-bit channel is reduced to 3 bits.
type RoundMode int

const (
	// RoundNearest rounds c x 7 / 255 to the nearest integer (gfx2next's
	// -color-round, and nextbmp's default).
	RoundNearest RoundMode = iota
	// RoundFloor rounds down.
	RoundFloor
	// RoundCeil rounds up.
	RoundCeil
	// RoundDistance picks the 3-bit level whose 8-bit value is nearest the
	// channel, the lower one on a tie (gfx2next's default, which searches
	// all 512 colours by distance). It differs from RoundNearest only on ties,
	// such as 164, halfway between levels 146 and 182.
	RoundDistance
)

// ParseRoundMode accepts "nearest" (or "round"), "floor", "ceil" and
// "distance".
func ParseRoundMode(s string) (RoundMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "nearest", "round":
		return RoundNearest, nil
	case "floor":
		return RoundFloor, nil
	case "ceil":
		return RoundCeil, nil
	case "distance":
		return RoundDistance, nil
	}
	return RoundNearest, fmt.Errorf("unknown rounding %q (want nearest, floor, ceil or distance)", s)
}

// Color333 is one Next palette entry: 3-bit channels (0-7) and the Layer 2
// priority bit.
type Color333 struct {
	R, G, B  uint8
	Priority bool
}

// RGBA returns the colour at 8 bits per channel, each 3-bit level v shown as
// round(v x 255 / 7): 0, 36, 73, 109, 146, 182, 219, 255.
func (c Color333) RGBA() color.RGBA {
	return color.RGBA{R: level8[c.R&7], G: level8[c.G&7], B: level8[c.B&7], A: 0xFF}
}

var level8 = [8]uint8{0, 36, 73, 109, 146, 182, 219, 255}

// ToRGB333 reduces a colour to RGB333; the priority bit is clear.
func ToRGB333(c color.Color, mode RoundMode) Color333 {
	r, g, b, _ := c.RGBA()
	return Color333{R: to3(uint8(r>>8), mode), G: to3(uint8(g>>8), mode), B: to3(uint8(b>>8), mode)}
}

// to3 is c x 7 / 255 rounded as mode says, exactly as gfx2next computes it.
func to3(c uint8, mode RoundMode) uint8 {
	v := float64(c) * 7.0 / 255.0
	switch mode {
	case RoundFloor:
		return uint8(math.Floor(v))
	case RoundCeil:
		return uint8(math.Ceil(v))
	case RoundDistance:
		best := uint8(0)
		for l := uint8(1); l < 8; l++ {
			if absDiff(level8[l], c) < absDiff(level8[best], c) {
				best = l
			}
		}
		return best
	}
	return uint8(math.Round(v))
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// Palette is a Next palette of up to MaxColors entries.
type Palette struct {
	Colors []Color333
}

// EncodeNXP returns the palette in the .nxp layout, two bytes per entry.
func EncodeNXP(p *Palette) ([]byte, error) {
	if len(p.Colors) > MaxColors {
		return nil, fmt.Errorf("zxn: palette has %d colours, at most %d", len(p.Colors), MaxColors)
	}
	out := make([]byte, 0, 2*len(p.Colors))
	for _, c := range p.Colors {
		b1 := c.B & 1
		if c.Priority {
			b1 |= 0x80
		}
		out = append(out, (c.R&7)<<5|(c.G&7)<<2|(c.B&7)>>1, b1)
	}
	return out, nil
}

// DecodeNXP parses a .nxp palette. Its length must be even and at most 512
// bytes; 32 bytes is a 16-colour palette.
func DecodeNXP(data []byte) (*Palette, error) {
	if len(data)%2 != 0 || len(data) > 2*MaxColors || len(data) == 0 {
		return nil, fmt.Errorf("zxn: a palette is 2 to %d bytes, two per colour; got %d", 2*MaxColors, len(data))
	}
	p := &Palette{Colors: make([]Color333, len(data)/2)}
	for i := range p.Colors {
		b0, b1 := data[2*i], data[2*i+1]
		p.Colors[i] = Color333{R: b0 >> 5, G: b0 >> 2 & 7, B: (b0&3)<<1 | b1&1, Priority: b1&0x80 != 0}
	}
	return p, nil
}

// Padded returns a copy of the palette extended with black entries to n
// colours, as the tools write a 16- or 256-colour palette whatever the image
// uses.
func (p *Palette) Padded(n int) *Palette {
	out := &Palette{Colors: append([]Color333(nil), p.Colors...)}
	for len(out.Colors) < n {
		out.Colors = append(out.Colors, Color333{})
	}
	return out
}
