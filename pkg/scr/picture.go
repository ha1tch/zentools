// ZXMODE pictures: a screen in any ZXMODE mode and ZXPAL palette.
//
// ZXMODE0 is the classic screen, 256x192 in the interleaved SCR layout, and
// takes the one-byte palettes ZXPAL0 to ZXPAL2 only. ZXMODE1 (256x192) and
// ZXMODE2 (512x384) are linear: pixel rows follow one another, and the
// attributes follow the bitmap, one plane per attribute byte (Part 2 of the
// specification, "Bitmap layout" and "Two-byte attributes", arrangement A).
// A ZXMODE2 picture is two half-screen pages, the top half then the bottom
// half, each laid out like a 512x192 ZXMODE1 screen.

package scr

import (
	"fmt"
	"image"
	"image/color"
	"strings"
)

// Mode identifies a ZXMODE screen mode.
type Mode uint8

const (
	ZXMODE0 Mode = iota // classic 256x192, interleaved, main RAM
	ZXMODE1             // linear 256x192, VRAM
	ZXMODE2             // linear 512x384, VRAM, two half-screen pages
)

// String returns the mode's name, "ZXMODE0" to "ZXMODE2".
func (m Mode) String() string { return fmt.Sprintf("ZXMODE%d", uint8(m)) }

// ParseMode accepts "zxmode0" to "zxmode2" (any case) or a bare digit 0 to 2.
func ParseMode(s string) (Mode, error) {
	t := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "zxmode")
	if len(t) == 1 && t[0] >= '0' && t[0] <= '2' {
		return Mode(t[0] - '0'), nil
	}
	return 0, fmt.Errorf("unknown mode %q: use zxmode0 to zxmode2", s)
}

// Size returns the mode's picture size in pixels.
func (m Mode) Size() (w, h int) {
	if m == ZXMODE2 {
		return 512, 384
	}
	return Width, Height
}

// CellAttr is one cell's attribute. For a one-byte palette A is the attribute
// byte and B is unused; for a two-byte palette A is the ink register and B the
// paper register.
type CellAttr struct {
	A, B byte
}

// Picture is a screen in a ZXMODE mode and ZXPAL palette: a pixel grid where
// each pixel is ink (true) or paper (false), and one CellAttr per 8x8 cell.
type Picture struct {
	Mode    Mode
	Palette Palette
	Width   int
	Height  int
	Ink     [][]bool     // [y][x]
	Attr    [][]CellAttr // [cellY][cellX]
	// Registers are the register colours the picture is meant for; nil
	// means the palette's reset values.
	Registers Registers
}

// NewPicture returns an all-paper picture with zero attributes.
func NewPicture(mode Mode, pal Palette) (*Picture, error) {
	if err := checkModePalette(mode, pal); err != nil {
		return nil, err
	}
	w, h := mode.Size()
	p := &Picture{Mode: mode, Palette: pal, Width: w, Height: h}
	p.Ink = make([][]bool, h)
	for y := range p.Ink {
		p.Ink[y] = make([]bool, w)
	}
	p.Attr = make([][]CellAttr, h/CellSize)
	for cy := range p.Attr {
		p.Attr[cy] = make([]CellAttr, w/CellSize)
	}
	return p, nil
}

func checkModePalette(mode Mode, pal Palette) error {
	if mode > ZXMODE2 {
		return fmt.Errorf("scr: unknown mode %d", mode)
	}
	if !pal.Valid() {
		return fmt.Errorf("scr: unknown palette %d", pal)
	}
	if mode == ZXMODE0 && pal.TwoByte() {
		return fmt.Errorf("scr: %s takes one-byte palettes only (ZXPAL0 to ZXPAL2), not %s", mode, pal)
	}
	return nil
}

// PictureLen returns the length in bytes of a picture's stored form: 6912 for
// ZXMODE0 (an SCR file); for ZXMODE1 one screen, 6912 or 7680 bytes; for
// ZXMODE2 two half-screen pages, 27648 or 30720 bytes.
func PictureLen(mode Mode, pal Palette) int {
	planes := 1
	if pal.TwoByte() {
		planes = 2
	}
	switch mode {
	case ZXMODE1:
		return 6144 + 768*planes
	case ZXMODE2:
		return 2 * (12288 + 1536*planes)
	}
	return FileLen
}

// Render draws the picture in its registers' colours. flashPhase selects the
// second half of a ZXPAL0 flash cycle.
func (p *Picture) Render(flashPhase bool) *image.RGBA {
	regs := p.Registers.Or(p.Palette)
	img := image.NewRGBA(image.Rect(0, 0, p.Width, p.Height))
	for y := 0; y < p.Height; y++ {
		for x := 0; x < p.Width; x++ {
			a := p.Attr[y/CellSize][x/CellSize]
			ink, paper := p.Palette.CellRegisters(a.A, a.B, flashPhase)
			c := regs[paper]
			if p.Ink[y][x] {
				c = regs[ink]
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// Bytes returns the picture's stored form (see PictureLen).
func (p *Picture) Bytes() []byte {
	if p.Mode == ZXMODE0 {
		s := &Screen{}
		for y := 0; y < Height; y++ {
			copy(s.Ink[y][:], p.Ink[y])
		}
		for cy := 0; cy < Rows; cy++ {
			for cx := 0; cx < Cols; cx++ {
				s.Attr[cy][cx] = AttributeFromByte(p.Attr[cy][cx].A)
			}
		}
		return Encode(s)
	}
	out := make([]byte, 0, PictureLen(p.Mode, p.Palette))
	halves := 1
	if p.Mode == ZXMODE2 {
		halves = 2
	}
	rows := p.Height / halves
	for half := 0; half < halves; half++ {
		out = append(out, p.page(half*rows, rows)...)
	}
	return out
}

// page lays out rows y0 to y0+rows-1 as one linear screen: the bitmap, then the
// ink (or only) attribute plane, then the paper plane for a two-byte palette.
func (p *Picture) page(y0, rows int) []byte {
	stride := p.Width / 8
	bmp := make([]byte, stride*rows)
	for y := 0; y < rows; y++ {
		for x := 0; x < p.Width; x++ {
			if p.Ink[y0+y][x] {
				bmp[stride*y+x/8] |= 0x80 >> uint(x%8)
			}
		}
	}
	cells := rows / CellSize
	planeA := make([]byte, stride*cells)
	planeB := make([]byte, stride*cells)
	for cy := 0; cy < cells; cy++ {
		for cx := 0; cx < stride; cx++ {
			a := p.Attr[y0/CellSize+cy][cx]
			planeA[stride*cy+cx] = a.A
			planeB[stride*cy+cx] = a.B
		}
	}
	out := append(bmp, planeA...)
	if p.Palette.TwoByte() {
		out = append(out, planeB...)
	}
	return out
}

// DecodePicture parses a picture's stored form (see PictureLen) in the given
// mode and palette.
func DecodePicture(data []byte, mode Mode, pal Palette) (*Picture, error) {
	p, err := NewPicture(mode, pal)
	if err != nil {
		return nil, err
	}
	if want := PictureLen(mode, pal); len(data) != want {
		return nil, fmt.Errorf("scr: %s with %s is %d bytes, got %d", mode, pal, want, len(data))
	}
	if mode == ZXMODE0 {
		s, err := Decode(data)
		if err != nil {
			return nil, err
		}
		for y := 0; y < Height; y++ {
			copy(p.Ink[y], s.Ink[y][:])
		}
		for cy := 0; cy < Rows; cy++ {
			for cx := 0; cx < Cols; cx++ {
				p.Attr[cy][cx] = CellAttr{A: s.Attr[cy][cx].Byte()}
			}
		}
		return p, nil
	}
	halves := 1
	if mode == ZXMODE2 {
		halves = 2
	}
	rows := p.Height / halves
	pageLen := len(data) / halves
	stride := p.Width / 8
	cells := rows / CellSize
	for half := 0; half < halves; half++ {
		pg := data[half*pageLen : (half+1)*pageLen]
		for y := 0; y < rows; y++ {
			for x := 0; x < p.Width; x++ {
				p.Ink[half*rows+y][x] = pg[stride*y+x/8]&(0x80>>uint(x%8)) != 0
			}
		}
		planeA := pg[stride*rows:]
		for cy := 0; cy < cells; cy++ {
			for cx := 0; cx < stride; cx++ {
				a := CellAttr{A: planeA[stride*cy+cx]}
				if pal.TwoByte() {
					a.B = planeA[stride*cells+stride*cy+cx]
				}
				p.Attr[half*cells+cy][cx] = a
			}
		}
	}
	return p, nil
}

// regColour is a register colour as the encoder handles it.
func regColour(c color.RGBA) [3]float64 {
	return [3]float64{float64(c.R), float64(c.G), float64(c.B)}
}

// Asset returns the picture as one ZCUT asset with the given name.
func (p *Picture) Asset(name string) *Asset {
	a := &Asset{Name: name, Width: p.Width, Height: p.Height, Palette: p.Palette, Registers: p.Registers}
	a.Ink = make([][]bool, p.Height)
	for y := range a.Ink {
		a.Ink[y] = append([]bool(nil), p.Ink[y]...)
	}
	if p.Palette.TwoByte() {
		a.Attr2 = make([][]CellAttr, len(p.Attr))
		for cy := range p.Attr {
			a.Attr2[cy] = append([]CellAttr(nil), p.Attr[cy]...)
		}
	} else {
		a.Attr = make([][]Attribute, len(p.Attr))
		for cy := range p.Attr {
			a.Attr[cy] = make([]Attribute, len(p.Attr[cy]))
			for cx, c := range p.Attr[cy] {
				a.Attr[cy][cx] = AttributeFromByte(c.A)
			}
		}
	}
	return a
}

// PictureFromAsset makes a picture in the given mode from an attributed asset
// of exactly the mode's size, in the asset's palette.
func PictureFromAsset(a *Asset, mode Mode) (*Picture, error) {
	p, err := NewPicture(mode, a.Palette)
	if err != nil {
		return nil, err
	}
	if a.Width != p.Width || a.Height != p.Height {
		return nil, fmt.Errorf("scr: asset %q is %dx%d, %s needs %dx%d", a.Name, a.Width, a.Height, mode, p.Width, p.Height)
	}
	if !a.HasAttrs() {
		return nil, fmt.Errorf("scr: asset %q has no attributes", a.Name)
	}
	p.Registers = a.Registers
	for y := range p.Ink {
		copy(p.Ink[y], a.Ink[y])
	}
	for cy := range p.Attr {
		for cx := range p.Attr[cy] {
			if a.Attr2 != nil {
				p.Attr[cy][cx] = a.Attr2[cy][cx]
			} else {
				p.Attr[cy][cx] = CellAttr{A: a.Attr[cy][cx].Byte()}
			}
		}
	}
	return p, nil
}
