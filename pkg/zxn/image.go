package zxn

import (
	"fmt"
	"image"
	"image/color"
)

// PaletteFromImage returns the palette an image's pixels index. A paletted
// image (an indexed PNG, say) keeps its own palette in its own order, each
// colour reduced to RGB333 as mode says. Any other image gets its distinct
// RGB333 colours in the order they first appear, row by row; an image with
// more than max of them is refused; Reduce chooses colours for such an image.
func PaletteFromImage(img image.Image, max int, mode RoundMode) (*Palette, error) {
	if pi, ok := img.(*image.Paletted); ok {
		if len(pi.Palette) > max {
			return nil, fmt.Errorf("zxn: the image's palette has %d colours, at most %d fit", len(pi.Palette), max)
		}
		p := &Palette{}
		for _, c := range pi.Palette {
			p.Colors = append(p.Colors, ToRGB333(c, mode))
		}
		return p, nil
	}
	b := img.Bounds()
	p := &Palette{}
	seen := map[Color333]bool{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := ToRGB333(img.At(x, y), mode)
			if seen[c] {
				continue
			}
			if len(p.Colors) == max {
				return nil, fmt.Errorf("zxn: the image has more than %d distinct RGB333 colours; reduce it (Reduce) to %d colours first", max, max)
			}
			seen[c] = true
			p.Colors = append(p.Colors, c)
		}
	}
	return p, nil
}

// SpritesFromImage cuts an image into 16x16 patterns, row by row of patterns,
// left to right. A paletted image's pixel indices are used as they are (at
// depth 4, their low four bits, as gfx2next does); any other image's pixels are
// matched to pal after reduction to RGB333 as mode says, and a colour pal does
// not hold is refused. The image's width and height must be multiples of 16.
func SpritesFromImage(img image.Image, pal *Palette, depth int, mode RoundMode) (*SpriteSheet, error) {
	if err := checkDepth(depth); err != nil {
		return nil, err
	}
	b := img.Bounds()
	if b.Dx()%SpriteSize != 0 || b.Dy()%SpriteSize != 0 || b.Dx() == 0 || b.Dy() == 0 {
		return nil, fmt.Errorf("zxn: the image is %dx%d; sprites need multiples of %d", b.Dx(), b.Dy(), SpriteSize)
	}
	pi, paletted := img.(*image.Paletted)
	index := map[Color333]uint8{}
	if !paletted {
		if pal == nil {
			return nil, fmt.Errorf("zxn: a sprite sheet that is not an indexed image needs a palette")
		}
		for i := len(pal.Colors) - 1; i >= 0; i-- { // the lowest index wins
			c := pal.Colors[i]
			c.Priority = false
			index[c] = uint8(i)
		}
	}
	s := &SpriteSheet{Depth: depth}
	for sy := 0; sy < b.Dy(); sy += SpriteSize {
		for sx := 0; sx < b.Dx(); sx += SpriteSize {
			sp := Bitmap{Width: SpriteSize, Height: SpriteSize, Depth: depth, Indices: make([]uint8, SpriteSize*SpriteSize)}
			for y := 0; y < SpriteSize; y++ {
				for x := 0; x < SpriteSize; x++ {
					px, py := b.Min.X+sx+x, b.Min.Y+sy+y
					var v uint8
					if paletted {
						v = pi.ColorIndexAt(px, py)
					} else {
						c := ToRGB333(img.At(px, py), mode)
						i, ok := index[c]
						if !ok {
							return nil, fmt.Errorf("zxn: pixel (%d, %d) is a colour the palette does not hold", sx+x, sy+y)
						}
						v = i
					}
					if depth == 4 {
						v &= 0x0F
					}
					sp.Indices[y*SpriteSize+x] = v
				}
			}
			s.Sprites = append(s.Sprites, sp)
		}
	}
	return s, nil
}

// SheetImage draws a sprite sheet perRow patterns wide in pal's colours. The
// palette index transparent, if 0 to 255, is drawn fully transparent; -1 draws
// every pixel opaque. Indices beyond the palette are drawn black.
func SheetImage(s *SpriteSheet, pal *Palette, perRow, transparent int) *image.RGBA {
	if perRow < 1 {
		perRow = 1
	}
	n := len(s.Sprites)
	cols := perRow
	if n < cols {
		cols = n
	}
	if cols == 0 {
		cols = 1
	}
	rows := (n + perRow - 1) / perRow
	img := image.NewRGBA(image.Rect(0, 0, cols*SpriteSize, rows*SpriteSize))
	for i := range s.Sprites {
		ox, oy := (i%perRow)*SpriteSize, (i/perRow)*SpriteSize
		for y := 0; y < SpriteSize; y++ {
			for x := 0; x < SpriteSize; x++ {
				v := int(s.Sprites[i].At(x, y))
				c := color.RGBA{A: 0xFF}
				if pal != nil && v < len(pal.Colors) {
					c = pal.Colors[v].RGBA()
				}
				if v == transparent {
					c = color.RGBA{}
				}
				img.SetRGBA(ox+x, oy+y, c)
			}
		}
	}
	return img
}

// PaletteImage draws the palette as 16x16 swatches, 16 to a row.
func PaletteImage(p *Palette) *image.RGBA {
	const sw = 16
	rows := (len(p.Colors) + 15) / 16
	if rows == 0 {
		rows = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, 16*sw, rows*sw))
	for i, c := range p.Colors {
		ox, oy := (i%16)*sw, (i/16)*sw
		for y := 0; y < sw; y++ {
			for x := 0; x < sw; x++ {
				img.SetRGBA(ox+x, oy+y, c.RGBA())
			}
		}
	}
	return img
}
