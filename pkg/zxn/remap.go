// file: remap.go
//
// Re-indexing an indexed image after its palette has been rearranged.

package zxn

import (
	"image"
	"image/color"
)

// RemapImage returns a copy of an indexed image whose pixel index i becomes
// table[i] (an index past the table's end is kept), drawn with pal. Use it
// with the table Minimize returns, or one built with StandardTable.
func RemapImage(img *image.Paletted, table []uint8, pal *Palette) *image.Paletted {
	cp := make(color.Palette, len(pal.Colors))
	for i, c := range pal.Colors {
		cp[i] = c.RGBA()
	}
	if len(cp) == 0 {
		cp = color.Palette{color.Black}
	}
	b := img.Bounds()
	out := image.NewPaletted(b, cp)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			v := img.ColorIndexAt(x, y)
			if int(v) < len(table) {
				v = table[v]
			}
			out.SetColorIndex(x, y, v)
		}
	}
	return out
}

// StandardTable takes each colour of an indexed image's palette to its
// standard-palette index (StandardIndex), for RemapImage with
// StandardPalette.
func StandardTable(pal color.Palette, mode RoundMode) []uint8 {
	t := make([]uint8, len(pal))
	for i, c := range pal {
		t[i] = StandardIndex(c, mode)
	}
	return t
}
