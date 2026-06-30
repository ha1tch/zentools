// Package scr reads and writes ZX Spectrum SCR screen files.
//
// An SCR file is a verbatim 6912-byte dump of the Spectrum's display memory:
// a 6144-byte pixel bitmap followed by a 768-byte attribute map. It carries no
// machine state - no registers, no entry point - so it is a pure framebuffer
// format, closer in spirit to a tape block than to a snapshot.
//
// The bitmap is not stored in linear row order. The Spectrum interleaves screen
// rows so that a row's byte address is built from three separate fields of the
// y coordinate. Writing rows linearly produces the classic scrambled-thirds
// image; this package handles the interleave in both directions.
//
// The core of this package works in memory. A Screen is a plain 256x192 grid of
// palette indices plus per-cell attributes; Encode turns it into the 6912 bytes
// of an SCR file and Decode reverses that. Conversion from ordinary images
// (PNG, JPEG, GIF) lives in convert.go.
package scr

import (
	"fmt"
	"image/color"
)

const (
	// Width and Height are the Spectrum display dimensions in pixels.
	Width  = 256
	Height = 192

	// Cols and Rows are the attribute grid dimensions, in 8x8 character cells.
	Cols = 32
	Rows = 24

	// CellSize is the side of one attribute cell, in pixels.
	CellSize = 8

	// bitmapLen is the size of the pixel bitmap region (256*192/8).
	bitmapLen = 6144
	// attrLen is the size of the attribute region (32*24).
	attrLen = 768
	// FileLen is the total size of an SCR file.
	FileLen = bitmapLen + attrLen
)

// Palette colour indices, in the order the attribute byte encodes them.
const (
	Black uint8 = iota
	Blue
	Red
	Magenta
	Green
	Cyan
	Yellow
	White
)

// paletteColor returns the rendered colour for a palette index (0..7) at a
// brightness. The dim level uses 0xC8 and the bright level 0xFF for non-zero
// channels; black is (0,0,0) at either brightness. These values match the zenzx
// emulator's ZXPaletteRGBA table, so an SCR rendered here matches what the
// emulator displays. This is the single source of truth for Spectrum colours,
// shared by the attribute helpers and the image conversion code.
func paletteColor(index uint8, bright bool) color.RGBA {
	v := uint8(0xC8)
	if bright {
		v = 0xFF
	}
	r := func(on bool) uint8 {
		if on {
			return v
		}
		return 0
	}
	// bit 1 = red, bit 2 = green, bit 0 = blue (GRB ordering of the index bits)
	red := index&0x02 != 0
	green := index&0x04 != 0
	blue := index&0x01 != 0
	return color.RGBA{R: r(red), G: r(green), B: r(blue), A: 0xFF}
}

// Attribute is one attribute cell: an ink and paper colour (each 0..7), a
// bright flag, and a flash flag. It maps to a single byte as
// ink | paper<<3 | bright<<6 | flash<<7.
type Attribute struct {
	Ink    uint8
	Paper  uint8
	Bright bool
	Flash  bool
}

// Byte packs the attribute into its stored form.
func (a Attribute) Byte() byte {
	var b byte
	b |= a.Ink & 0x07
	b |= (a.Paper & 0x07) << 3
	if a.Bright {
		b |= 0x40
	}
	if a.Flash {
		b |= 0x80
	}
	return b
}

// AttributeFromByte unpacks a stored attribute byte.
func AttributeFromByte(b byte) Attribute {
	return Attribute{
		Ink:    b & 0x07,
		Paper:  (b >> 3) & 0x07,
		Bright: b&0x40 != 0,
		Flash:  b&0x80 != 0,
	}
}

// PaperRGBA returns the colour this attribute's paper renders as, at its
// brightness. It lets callers map an attribute to a concrete colour without
// reaching into the conversion internals - for instance to use an attribute as
// a fill colour when resizing an image before conversion.
func (a Attribute) PaperRGBA() color.RGBA {
	return paletteColor(a.Paper&0x07, a.Bright)
}

// InkRGBA returns the colour this attribute's ink renders as, at its brightness.
func (a Attribute) InkRGBA() color.RGBA {
	return paletteColor(a.Ink&0x07, a.Bright)
}

// Screen is an in-memory ZX Spectrum screen: a pixel grid where each pixel is
// either ink (true) or paper (false), plus one Attribute per 8x8 cell. This is
// the neutral representation the codec encodes from and decodes to; callers
// adapt their own pixel data to a Screen rather than to the SCR byte layout.
type Screen struct {
	// Ink is row-major, Ink[y][x]; true means the pixel shows its cell's ink
	// colour, false its paper colour.
	Ink [Height][Width]bool
	// Attr is the attribute grid, Attr[cellY][cellX].
	Attr [Rows][Cols]Attribute
}

// rowAddr returns the byte offset within the 6144-byte bitmap region at which
// pixel row y begins. The Spectrum splits y into three fields:
//
//	y = SS CCC RRR  (bit 7..6 unused; bits 5..0 carry the three fields)
//	  third (SS)   = y bits 6..7  -> 2048 bytes per third
//	  charRow (CCC)= y bits 3..5  -> 32 bytes per character row
//	  pixRow (RRR) = y bits 0..2  -> 256 bytes per pixel row within a char
//
// and weights them non-uniformly. This interleave is the whole subtlety of the
// format.
func rowAddr(y int) int {
	third := (y >> 6) & 0x03
	pixRow := y & 0x07
	charRow := (y >> 3) & 0x07
	return third*2048 + pixRow*256 + charRow*32
}

// Encode serialises a Screen into the 6912 bytes of an SCR file.
func Encode(s *Screen) []byte {
	out := make([]byte, FileLen)
	bitmap := out[:bitmapLen]
	attrs := out[bitmapLen:]

	for y := 0; y < Height; y++ {
		base := rowAddr(y)
		for cx := 0; cx < Cols; cx++ {
			var b byte
			for bit := 0; bit < 8; bit++ {
				if s.Ink[y][cx*8+bit] {
					b |= 1 << (7 - bit)
				}
			}
			bitmap[base+cx] = b
		}
	}

	for cy := 0; cy < Rows; cy++ {
		for cx := 0; cx < Cols; cx++ {
			attrs[cy*Cols+cx] = s.Attr[cy][cx].Byte()
		}
	}

	return out
}

// Decode parses the 6912 bytes of an SCR file into a Screen.
func Decode(data []byte) (*Screen, error) {
	if len(data) != FileLen {
		return nil, fmt.Errorf("scr: image is %d bytes, want %d", len(data), FileLen)
	}
	bitmap := data[:bitmapLen]
	attrs := data[bitmapLen:]

	s := &Screen{}

	for y := 0; y < Height; y++ {
		base := rowAddr(y)
		for cx := 0; cx < Cols; cx++ {
			b := bitmap[base+cx]
			for bit := 0; bit < 8; bit++ {
				s.Ink[y][cx*8+bit] = b&(1<<(7-bit)) != 0
			}
		}
	}

	for cy := 0; cy < Rows; cy++ {
		for cx := 0; cx < Cols; cx++ {
			s.Attr[cy][cx] = AttributeFromByte(attrs[cy*Cols+cx])
		}
	}

	return s, nil
}
