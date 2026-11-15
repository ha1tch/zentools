// file: screens.go
//
// The Next's other bitmap screens: LoRes (128x96) and the Timex modes,
// HiRes (512x192 in two colours) and HiColour (256x192 with an attribute per
// 8x1 pixels).

package zxn

import (
	"fmt"
	"image"
	"image/color"

	"github.com/ha1tch/zentools/pkg/scr"
)

// LoRes modes: 128x96 at 8 bits per pixel (12288 bytes, the top half at
// 0x4000 and the bottom at 0x6000, which a file holds one after the other)
// or at 4 bits (6144 bytes, the "Radastan" mode of later cores, the left
// pixel in the high nibble). Both are row by row; encode them with
// EncodeNXI.
var LoResModes = []Layer2Mode{
	{"lores", 128, 96, 8, RowMajor},
	{"lores4", 128, 96, 4, RowMajor},
}

// Timex screen sizes.
const (
	TimexBitmapLen = 6144
	TimexFileLen   = 2 * TimexBitmapLen
)

// timexRow is the offset of pixel row y in a Spectrum bitmap.
func timexRow(y int) int {
	return (y>>6&3)*2048 + (y&7)*256 + (y>>3&7)*32
}

// HiRes is a Timex 512x192 screen: one ink colour for the whole screen, and
// paper its complement (7 - ink), as port 0xFF bits 5-3 set them.
type HiRes struct {
	Ink    uint8
	Pixels [192][512]bool // true is ink
}

// Bytes returns the two bitmaps as the screen holds them, the one at 0x4000
// (even byte columns) then the one at 0x6000 (odd byte columns), each in the
// Spectrum's interleaved row order: 12288 bytes.
func (h *HiRes) Bytes() []byte {
	out := make([]byte, TimexFileLen)
	for y := 0; y < 192; y++ {
		for c := 0; c < 64; c++ {
			var v byte
			for bit := 0; bit < 8; bit++ {
				if h.Pixels[y][c*8+bit] {
					v |= 0x80 >> uint(bit)
				}
			}
			out[(c&1)*TimexBitmapLen+timexRow(y)+c/2] = v
		}
	}
	return out
}

// DecodeHiRes reads 12288 bytes of HiRes bitmap with the given ink.
func DecodeHiRes(data []byte, ink uint8) (*HiRes, error) {
	if len(data) != TimexFileLen {
		return nil, fmt.Errorf("zxn: a HiRes screen is %d bytes; got %d", TimexFileLen, len(data))
	}
	h := &HiRes{Ink: ink & 7}
	for y := 0; y < 192; y++ {
		for c := 0; c < 64; c++ {
			v := data[(c&1)*TimexBitmapLen+timexRow(y)+c/2]
			for bit := 0; bit < 8; bit++ {
				h.Pixels[y][c*8+bit] = v&(0x80>>uint(bit)) != 0
			}
		}
	}
	return h, nil
}

// HiResFromImage takes a 512x192 image as ink and paper: in an indexed image
// any index but 0 is ink, otherwise a pixel darker than mid-grey is ink.
func HiResFromImage(img image.Image, ink uint8) (*HiRes, error) {
	b := img.Bounds()
	if b.Dx() != 512 || b.Dy() != 192 {
		return nil, fmt.Errorf("zxn: a HiRes image is 512x192; got %dx%d", b.Dx(), b.Dy())
	}
	h := &HiRes{Ink: ink & 7}
	pi, indexed := img.(*image.Paletted)
	for y := 0; y < 192; y++ {
		for x := 0; x < 512; x++ {
			if indexed {
				h.Pixels[y][x] = pi.ColorIndexAt(b.Min.X+x, b.Min.Y+y) != 0
			} else {
				h.Pixels[y][x] = luma(img.At(b.Min.X+x, b.Min.Y+y)) < 0x80
			}
		}
	}
	return h, nil
}

// Image draws the screen in ULA colours, not bright.
func (h *HiRes) Image() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 512, 192))
	ink, paper := scr.ULAColor(h.Ink, false), scr.ULAColor(7-h.Ink, false)
	for y := 0; y < 192; y++ {
		for x := 0; x < 512; x++ {
			c := paper
			if h.Pixels[y][x] {
				c = ink
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// HiColour is a Timex 256x192 screen with an attribute byte for every 8x1
// pixels.
type HiColour struct {
	Pixels [192][256]bool // true is ink
	Attr   [192][32]uint8 // ink | paper<<3 | bright<<6 | flash<<7
}

// Bytes returns the bitmap (at 0x4000) then the attributes (at 0x6000),
// each byte's attribute at the same interleaved offset as its pixels:
// 12288 bytes.
func (h *HiColour) Bytes() []byte {
	out := make([]byte, TimexFileLen)
	for y := 0; y < 192; y++ {
		for c := 0; c < 32; c++ {
			var v byte
			for bit := 0; bit < 8; bit++ {
				if h.Pixels[y][c*8+bit] {
					v |= 0x80 >> uint(bit)
				}
			}
			out[timexRow(y)+c] = v
			out[TimexBitmapLen+timexRow(y)+c] = h.Attr[y][c]
		}
	}
	return out
}

// DecodeHiColour reads 12288 bytes of HiColour screen.
func DecodeHiColour(data []byte) (*HiColour, error) {
	if len(data) != TimexFileLen {
		return nil, fmt.Errorf("zxn: a HiColour screen is %d bytes; got %d", TimexFileLen, len(data))
	}
	h := &HiColour{}
	for y := 0; y < 192; y++ {
		for c := 0; c < 32; c++ {
			v := data[timexRow(y)+c]
			for bit := 0; bit < 8; bit++ {
				h.Pixels[y][c*8+bit] = v&(0x80>>uint(bit)) != 0
			}
			h.Attr[y][c] = data[TimexBitmapLen+timexRow(y)+c]
		}
	}
	return h, nil
}

// Image draws the screen in ULA colours (flash ignored).
func (h *HiColour) Image() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 256, 192))
	for y := 0; y < 192; y++ {
		for c := 0; c < 32; c++ {
			a := scr.AttributeFromByte(h.Attr[y][c])
			ink, paper := scr.ULAColor(a.Ink, a.Bright), scr.ULAColor(a.Paper, a.Bright)
			for bit := 0; bit < 8; bit++ {
				col := paper
				if h.Pixels[y][c*8+bit] {
					col = ink
				}
				img.SetRGBA(c*8+bit, y, col)
			}
		}
	}
	return img
}

// HiColourFromImage chooses, for every 8x1 span of a 256x192 image, the ink,
// paper and brightness (shared by both, as an attribute has one bright bit)
// that draw it with the least squared error, each pixel taking the nearer of
// the two. On a tie the pair with the lower ink, then paper, wins, and a
// pixel equally near both is paper. Flash is never set.
func HiColourFromImage(img image.Image) (*HiColour, error) {
	b := img.Bounds()
	if b.Dx() != 256 || b.Dy() != 192 {
		return nil, fmt.Errorf("zxn: a HiColour image is 256x192; got %dx%d", b.Dx(), b.Dy())
	}
	var ula [2][8][3]int
	for br := 0; br < 2; br++ {
		for i := 0; i < 8; i++ {
			c := scr.ULAColor(uint8(i), br == 1)
			ula[br][i] = [3]int{int(c.R), int(c.G), int(c.B)}
		}
	}
	h := &HiColour{}
	var px [8][3]int
	for y := 0; y < 192; y++ {
		for c := 0; c < 32; c++ {
			for i := 0; i < 8; i++ {
				r, g, bl, _ := img.At(b.Min.X+c*8+i, b.Min.Y+y).RGBA()
				px[i] = [3]int{int(r >> 8), int(g >> 8), int(bl >> 8)}
			}
			best, bestErr := uint8(0), -1
			var bestBits [8]bool
			for br := 0; br < 2; br++ {
				for ink := 0; ink < 8; ink++ {
					for paper := 0; paper < 8; paper++ {
						e := 0
						var bits [8]bool
						for i := 0; i < 8; i++ {
							di, dp := dist2(px[i], ula[br][ink]), dist2(px[i], ula[br][paper])
							if di < dp {
								e += di
								bits[i] = true
							} else {
								e += dp
							}
						}
						if bestErr < 0 || e < bestErr {
							bestErr, bestBits = e, bits
							best = uint8(ink) | uint8(paper)<<3 | uint8(br)<<6
						}
					}
				}
			}
			h.Attr[y][c] = best
			for i := 0; i < 8; i++ {
				h.Pixels[y][c*8+i] = bestBits[i]
			}
		}
	}
	return h, nil
}

func dist2(a, b [3]int) int {
	dr, dg, db := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return dr*dr + dg*dg + db*db
}

func luma(c color.Color) uint32 {
	r, g, b, _ := c.RGBA()
	return (299*(r>>8) + 587*(g>>8) + 114*(b>>8)) / 1000
}
