// file: layer2.go
//
// Layer 2 bitmaps (.nxi): raw pixel indices with no header, at 8 or 4 bits
// per pixel, row by row or column by column, optionally preceded by their
// palette.

package zxn

import (
	"fmt"
	"image"
	"image/color"
	"strings"
)

// Layer2Mode is one of the Next's Layer 2 screen modes.
type Layer2Mode struct {
	Name          string
	Width, Height int
	Depth         int
	Order         PixelOrder
}

// Layer2Modes are the three Layer 2 modes: 256x192 at 8 bits stored row by
// row (48 KB), and 320x256 at 8 bits and 640x256 at 4 bits, both stored
// column by column (80 KB).
var Layer2Modes = []Layer2Mode{
	{"256x192", 256, 192, 8, RowMajor},
	{"320x256", 320, 256, 8, ColumnMajor},
	{"640x256", 640, 256, 4, ColumnMajor},
}

// Bytes is the size of a screen in this mode, without a palette.
func (m Layer2Mode) Bytes() int { return m.Width * m.Height * m.Depth / 8 }

// ParseLayer2Mode accepts a mode's name ("256x192", "320x256", "640x256").
func ParseLayer2Mode(s string) (Layer2Mode, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, m := range Layer2Modes {
		if m.Name == s {
			return m, nil
		}
	}
	return Layer2Mode{}, fmt.Errorf("unknown Layer 2 mode %q (want 256x192, 320x256 or 640x256)", s)
}

// Layer2ModeOfSize finds the mode of a file of n bytes, a bare screen or one
// with an embedded palette, and returns the palette's size (0, 32 or 512 for
// the two-byte layout, 16 or 256 for a one-byte one). A 4-bit 640x256 screen
// is the same size as a 320x256 one, so that size is reported as 320x256.
func Layer2ModeOfSize(n int) (Layer2Mode, int, bool) {
	for _, m := range Layer2Modes {
		for _, pal := range []int{0, 512, 32, 256, 16} {
			if n == m.Bytes()+pal {
				return m, pal, true
			}
		}
	}
	return Layer2Mode{}, 0, false
}

func checkBitmap(w, h, depth int) error {
	if depth != 8 && depth != 4 {
		return fmt.Errorf("zxn: bitmap depth %d, want 8 or 4", depth)
	}
	if w <= 0 || h <= 0 {
		return fmt.Errorf("zxn: bitmap is %dx%d", w, h)
	}
	if depth == 4 && w%2 != 0 {
		return fmt.Errorf("zxn: a 4-bit bitmap needs an even width; got %d", w)
	}
	return nil
}

// BitmapFromImage takes an image's pixels as palette indices: a paletted
// image's own indices (at depth 4 their low four bits, as gfx2next does),
// any other image's pixels matched to pal after reduction to RGB333 as mode
// says, refusing a colour pal does not hold.
func BitmapFromImage(img image.Image, pal *Palette, depth int, order PixelOrder, mode RoundMode) (*Bitmap, error) {
	b := img.Bounds()
	if err := checkBitmap(b.Dx(), b.Dy(), depth); err != nil {
		return nil, err
	}
	pi, paletted := img.(*image.Paletted)
	index := map[Color333]uint8{}
	if !paletted {
		if pal == nil {
			return nil, fmt.Errorf("zxn: an image that is not indexed needs a palette to match its colours against")
		}
		for i := len(pal.Colors) - 1; i >= 0; i-- { // the lowest index wins
			c := pal.Colors[i]
			c.Priority = false
			index[c] = uint8(i)
		}
	}
	out := &Bitmap{Width: b.Dx(), Height: b.Dy(), Depth: depth, Order: order, Indices: make([]uint8, b.Dx()*b.Dy())}
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			var v uint8
			if paletted {
				v = pi.ColorIndexAt(b.Min.X+x, b.Min.Y+y)
			} else {
				i, ok := index[ToRGB333(img.At(b.Min.X+x, b.Min.Y+y), mode)]
				if !ok {
					return nil, fmt.Errorf("zxn: pixel (%d, %d) is a colour the palette does not hold", x, y)
				}
				v = i
			}
			if depth == 4 {
				v &= 0x0F
			}
			out.Indices[y*b.Dx()+x] = v
		}
	}
	return out, nil
}

// EncodeNXI returns a bitmap's pixels as a Layer 2 file holds them. Row by
// row, an 8-bit bitmap is its indices in order and a 4-bit one packs each
// pair of pixels into a byte, the left one in the high nibble. Column by
// column, an 8-bit bitmap stores each column top to bottom, and a 4-bit one
// each column of pixel pairs, so a byte still holds two horizontal pixels.
func EncodeNXI(b *Bitmap) ([]byte, error) {
	if err := checkBitmap(b.Width, b.Height, b.Depth); err != nil {
		return nil, err
	}
	if len(b.Indices) != b.Width*b.Height {
		return nil, fmt.Errorf("zxn: bitmap holds %d indices for %dx%d", len(b.Indices), b.Width, b.Height)
	}
	cols := b.Width // stored columns: pixels, or pixel pairs at depth 4
	if b.Depth == 4 {
		cols /= 2
	}
	out := make([]byte, cols*b.Height)
	for y := 0; y < b.Height; y++ {
		for c := 0; c < cols; c++ {
			var v uint8
			if b.Depth == 8 {
				v = b.Indices[y*b.Width+c]
			} else {
				l, r := b.Indices[y*b.Width+2*c], b.Indices[y*b.Width+2*c+1]
				if l > 15 || r > 15 {
					return nil, fmt.Errorf("zxn: an index at row %d, pixel %d does not fit in 4 bits", y, 2*c)
				}
				v = l<<4 | r
			}
			if b.Order == ColumnMajor {
				out[c*b.Height+y] = v
			} else {
				out[y*cols+c] = v
			}
		}
	}
	return out, nil
}

// DecodeNXI reads Layer 2 pixels of a known size, depth and order; data must
// be exactly that size (split off an embedded palette first).
func DecodeNXI(data []byte, w, h, depth int, order PixelOrder) (*Bitmap, error) {
	if err := checkBitmap(w, h, depth); err != nil {
		return nil, err
	}
	cols := w
	if depth == 4 {
		cols /= 2
	}
	if len(data) != cols*h {
		return nil, fmt.Errorf("zxn: a %dx%d %d-bit bitmap is %d bytes; got %d", w, h, depth, cols*h, len(data))
	}
	b := &Bitmap{Width: w, Height: h, Depth: depth, Order: order, Indices: make([]uint8, w*h)}
	for y := 0; y < h; y++ {
		for c := 0; c < cols; c++ {
			var v uint8
			if order == ColumnMajor {
				v = data[c*h+y]
			} else {
				v = data[y*cols+c]
			}
			if depth == 8 {
				b.Indices[y*w+c] = v
			} else {
				b.Indices[y*w+2*c], b.Indices[y*w+2*c+1] = v>>4, v&0x0F
			}
		}
	}
	return b, nil
}

// BitmapImage draws a bitmap in pal's colours; the index transparent, if 0
// to 255, is drawn fully transparent, and indices beyond the palette black.
func BitmapImage(b *Bitmap, pal *Palette, transparent int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, b.Width, b.Height))
	for y := 0; y < b.Height; y++ {
		for x := 0; x < b.Width; x++ {
			v := int(b.At(x, y))
			c := color.RGBA{A: 0xFF}
			if pal != nil && v < len(pal.Colors) {
				c = pal.Colors[v].RGBA()
			}
			if v == transparent {
				c = color.RGBA{}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}
