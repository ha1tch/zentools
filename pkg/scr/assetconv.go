// file: assetconv.go
//
// Assets and ZXMODE pictures together: cutting an asset out of a picture in
// any mode and palette, pasting one in, converting an asset's attributes to
// another palette or register set, and flipping assets and pictures.

package scr

import (
	"fmt"
	"image"
	"image/color"
	"strings"
)

// cellColours returns the ink and paper colours of every cell of an
// attributed asset, in its own registers.
func (a *Asset) cellColours() [][][2][3]int {
	regs := a.Registers.Or(a.Palette)
	ch, cw := a.Height/8, a.Width/8
	out := make([][][2][3]int, ch)
	for cy := 0; cy < ch; cy++ {
		out[cy] = make([][2][3]int, cw)
		for cx := 0; cx < cw; cx++ {
			var c CellAttr
			if a.Attr2 != nil {
				c = a.Attr2[cy][cx]
			} else {
				c = CellAttr{A: a.Attr[cy][cx].Byte()}
			}
			ink, paper := a.Palette.CellRegisters(c.A, c.B, false)
			i, p := regs[ink], regs[paper]
			out[cy][cx] = [2][3]int{{int(i.R), int(i.G), int(i.B)}, {int(p.R), int(p.G), int(p.B)}}
		}
	}
	return out
}

func colourDist2(a, b [3]int) int {
	d0, d1, d2 := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return d0*d0 + d1*d1 + d2*d2
}

// ConvertAsset returns a copy of a rewritten for palette to with registers
// regs (nil for its reset values), keeping the bitmap: every cell takes the
// ink and paper the target can name that are nearest, in sRGB, to the cell's
// own ink and paper colours, each weighted by how many of the cell's pixels
// it covers. A one-byte palette's ink and paper must share a bank (ZXPAL0's
// bright level, ZXPAL1's and ZXPAL2's bank), so the bank with the least
// weighted error wins; a two-byte palette chooses each freely. A colour the
// target holds exactly is kept exactly, at its lowest register. Flash is
// kept only from ZXPAL0 to ZXPAL0. An asset without attributes only changes
// its palette label.
func ConvertAsset(a *Asset, to Palette, regs Registers) (*Asset, error) {
	if !to.Valid() {
		return nil, fmt.Errorf("scr: unknown palette %d", to)
	}
	if err := CheckRegisters(to, regs); err != nil {
		return nil, err
	}
	out := a.clone()
	out.Palette, out.Registers = to, regs
	out.Attr, out.Attr2 = nil, nil
	if !a.HasAttrs() {
		out.Registers = nil
		return out, nil
	}
	target := regs.Or(to)
	tc := make([][3]int, len(target))
	for i, c := range target {
		tc[i] = [3]int{int(c.R), int(c.G), int(c.B)}
	}
	// nearest returns the register in [lo, hi) nearest c, and its distance.
	nearest := func(c [3]int, lo, hi int) (int, int) {
		best, bd := lo, -1
		for r := lo; r < hi; r++ {
			if d := colourDist2(c, tc[r]); bd < 0 || d < bd {
				best, bd = r, d
			}
		}
		return best, bd
	}
	cols := a.cellColours()
	ch, cw := a.Height/8, a.Width/8
	if to.TwoByte() {
		out.Attr2 = make([][]CellAttr, ch)
	} else {
		out.Attr = make([][]Attribute, ch)
	}
	for cy := 0; cy < ch; cy++ {
		if to.TwoByte() {
			out.Attr2[cy] = make([]CellAttr, cw)
		} else {
			out.Attr[cy] = make([]Attribute, cw)
		}
		for cx := 0; cx < cw; cx++ {
			nInk := 0
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					if a.Ink[cy*8+y][cx*8+x] {
						nInk++
					}
				}
			}
			// A colour no pixel shows still converts, at a token weight, so
			// the cell stays sensible if its bitmap is edited later.
			wInk, wPaper := 1+1000*nInk, 1+1000*(64-nInk)
			ink, paper := cols[cy][cx][0], cols[cy][cx][1]
			if to.TwoByte() {
				ri, _ := nearest(ink, 0, len(tc))
				rp, _ := nearest(paper, 0, len(tc))
				out.Attr2[cy][cx] = CellAttr{A: byte(ri), B: byte(rp)}
				continue
			}
			bestBank, bestI, bestP, bestE := 0, 0, 0, -1
			for bank := 0; bank < len(tc)/8; bank++ {
				ri, di := nearest(ink, 8*bank, 8*bank+8)
				rp, dp := nearest(paper, 8*bank, 8*bank+8)
				if e := wInk*di + wPaper*dp; bestE < 0 || e < bestE {
					bestBank, bestI, bestP, bestE = bank, ri-8*bank, rp-8*bank, e
				}
			}
			b := byte(bestBank<<6 | bestP<<3 | bestI)
			if to == ZXPAL0 && a.Palette == ZXPAL0 && a.Attr[cy][cx].Flash {
				b |= 0x80
			}
			out.Attr[cy][cx] = AttributeFromByte(b)
		}
	}
	return out, nil
}

// clone returns a deep copy of the asset.
func (a *Asset) clone() *Asset {
	out := *a
	out.Ink = make([][]bool, len(a.Ink))
	for y := range a.Ink {
		out.Ink[y] = append([]bool(nil), a.Ink[y]...)
	}
	if a.Attr != nil {
		out.Attr = make([][]Attribute, len(a.Attr))
		for y := range a.Attr {
			out.Attr[y] = append([]Attribute(nil), a.Attr[y]...)
		}
	}
	if a.Attr2 != nil {
		out.Attr2 = make([][]CellAttr, len(a.Attr2))
		for y := range a.Attr2 {
			out.Attr2[y] = append([]CellAttr(nil), a.Attr2[y]...)
		}
	}
	if a.Registers != nil {
		out.Registers = append(Registers(nil), a.Registers...)
	}
	return &out
}

// Matches reports whether the asset's attributes are already in palette p
// with registers regs (nil: the reset values).
func (a *Asset) Matches(p Palette, regs Registers) bool {
	return a.Palette == p && a.Registers.Or(a.Palette).Equal(regs.Or(p))
}

// CutPicture extracts a pixel region of a picture as an asset in the
// picture's palette and registers. With keepAttrs and a cell-aligned region
// the asset carries the region's attributes (two planes for a two-byte
// palette); otherwise it is bitmap only.
func CutPicture(p *Picture, r Rect, name string, keepAttrs bool) (*Asset, error) {
	if r.W <= 0 || r.H <= 0 {
		return nil, fmt.Errorf("cut: region must be positive (got %dx%d)", r.W, r.H)
	}
	if r.X < 0 || r.Y < 0 || r.X+r.W > p.Width || r.Y+r.H > p.Height {
		return nil, fmt.Errorf("cut: region %d,%d,%d,%d is outside the %dx%d picture", r.X, r.Y, r.W, r.H, p.Width, p.Height)
	}
	a := &Asset{Name: name, Width: r.W, Height: r.H, Palette: p.Palette}
	a.Ink = make([][]bool, r.H)
	for y := 0; y < r.H; y++ {
		a.Ink[y] = append([]bool(nil), p.Ink[r.Y+y][r.X:r.X+r.W]...)
	}
	if keepAttrs && r.X%8 == 0 && r.Y%8 == 0 && r.W%8 == 0 && r.H%8 == 0 {
		cw, ch := r.W/8, r.H/8
		a.Registers = p.Registers
		if p.Palette.TwoByte() {
			a.Attr2 = make([][]CellAttr, ch)
		} else {
			a.Attr = make([][]Attribute, ch)
		}
		for cy := 0; cy < ch; cy++ {
			row := p.Attr[r.Y/8+cy][r.X/8 : r.X/8+cw]
			if p.Palette.TwoByte() {
				a.Attr2[cy] = append([]CellAttr(nil), row...)
				continue
			}
			a.Attr[cy] = make([]Attribute, cw)
			for cx, c := range row {
				a.Attr[cy][cx] = AttributeFromByte(c.A)
			}
		}
	}
	return a, nil
}

// PastePicture blits an asset onto a picture at pixel (x, y), combining the
// bitmaps as op says. An attributed asset pasted at a cell-aligned position
// also writes its attributes, first converted with ConvertAsset when it is
// in another palette or another register set; converted reports whether that
// happened.
func PastePicture(p *Picture, a *Asset, x, y int, op PasteOp) (converted bool, err error) {
	if x < 0 || y < 0 || x+a.Width > p.Width || y+a.Height > p.Height {
		return false, fmt.Errorf("paste: asset %dx%d at %d,%d does not fit the %dx%d picture", a.Width, a.Height, x, y, p.Width, p.Height)
	}
	src := a
	if a.HasAttrs() && x%8 == 0 && y%8 == 0 && !a.Matches(p.Palette, p.Registers) {
		if src, err = ConvertAsset(a, p.Palette, p.Registers); err != nil {
			return false, err
		}
		converted = true
	}
	for ay := 0; ay < a.Height; ay++ {
		for ax := 0; ax < a.Width; ax++ {
			s, t := src.Ink[ay][ax], &p.Ink[y+ay][x+ax]
			switch op {
			case PasteOR:
				*t = *t || s
			case PasteAND:
				*t = *t && s
			case PasteCOPY:
				*t = s
			case PasteXOR:
				*t = *t != s
			}
		}
	}
	if src.HasAttrs() && x%8 == 0 && y%8 == 0 {
		for cy := 0; cy < src.Height/8; cy++ {
			for cx := 0; cx < src.Width/8; cx++ {
				if src.Attr2 != nil {
					p.Attr[y/8+cy][x/8+cx] = src.Attr2[cy][cx]
				} else {
					p.Attr[y/8+cy][x/8+cx] = CellAttr{A: src.Attr[cy][cx].Byte()}
				}
			}
		}
	}
	return converted, nil
}

func flipBools(rows [][]bool, h, v bool) {
	if v {
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	}
	if h {
		for _, r := range rows {
			for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
				r[i], r[j] = r[j], r[i]
			}
		}
	}
}

func flipGrid[T any](rows [][]T, h, v bool) {
	if v {
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	}
	if h {
		for _, r := range rows {
			for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
				r[i], r[j] = r[j], r[i]
			}
		}
	}
}

// Flip mirrors the asset left to right (h) and/or top to bottom (v), its
// attribute cells with it. An attributed asset is cell-aligned, so its cells
// stay whole.
func (a *Asset) Flip(h, v bool) {
	flipBools(a.Ink, h, v)
	flipGrid(a.Attr, h, v)
	flipGrid(a.Attr2, h, v)
}

// Flip mirrors the picture left to right (h) and/or top to bottom (v), its
// attribute cells with it.
func (p *Picture) Flip(h, v bool) {
	flipBools(p.Ink, h, v)
	flipGrid(p.Attr, h, v)
}

// PictureExtent is BitmapExtent for a picture in any mode: the smallest
// rectangle covering every pixel whose bitmap bit equals bits (1 or 0).
func PictureExtent(p *Picture, bits int) (Rect, error) {
	target := bits != 0
	minX, minY, maxX, maxY := p.Width, p.Height, -1, -1
	for y := 0; y < p.Height; y++ {
		for x := 0; x < p.Width; x++ {
			if p.Ink[y][x] == target {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < 0 {
		return Rect{}, fmt.Errorf("bitmap-extent: no %s pixels found", map[bool]string{true: "set", false: "unset"}[target])
	}
	return Rect{X: minX, Y: minY, W: maxX - minX + 1, H: maxY - minY + 1}, nil
}

// BitmapImage draws the picture's bitmap only: pixels whose bit equals bits
// in ink, the others in paper (or transparent).
func (p *Picture) BitmapImage(bits int, ink, paper color.Color, paperTransparent bool) *image.RGBA {
	target := bits != 0
	ir, ig, ib, ia := rgba8(ink)
	pr, pg, pb, pa := rgba8(paper)
	if paperTransparent {
		pr, pg, pb, pa = 0, 0, 0, 0
	}
	img := image.NewRGBA(image.Rect(0, 0, p.Width, p.Height))
	for y := 0; y < p.Height; y++ {
		for x := 0; x < p.Width; x++ {
			c := color.RGBA{pr, pg, pb, pa}
			if p.Ink[y][x] == target {
				c = color.RGBA{ir, ig, ib, ia}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// RecognizePicture reads text from a picture's bitmap, one line per row of
// cells (24 in ZXMODE0 and ZXMODE1, 48 in ZXMODE2, 32 or 64 characters each),
// matching each cell's pixels against the Spectrum ROM font as
// RecognizeScreen does, but from the bits themselves rather than from
// colours, so any palette reads the same. As RecognizeScreen takes a cell's
// commonest colour as its paper, a cell with more set bits than clear ones is
// read inverted, so inverse text reads too.
func RecognizePicture(p *Picture) ([]string, error) {
	table, err := newGlyphTable()
	if err != nil {
		return nil, err
	}
	lines := make([]string, p.Height/8)
	for row := range lines {
		var b strings.Builder
		for col := 0; col < p.Width/8; col++ {
			var g glyphBits
			set := 0
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					g[y][x] = p.Ink[row*8+y][col*8+x]
					if g[y][x] {
						set++
					}
				}
			}
			if set > 32 {
				for y := range g {
					for x := range g[y] {
						g[y][x] = !g[y][x]
					}
				}
			}
			b.WriteRune(bestMatch(table, g))
		}
		lines[row] = strings.TrimRight(b.String(), " ")
	}
	return lines, nil
}
