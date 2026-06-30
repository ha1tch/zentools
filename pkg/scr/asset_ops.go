// Conversions between a full Screen and individual assets: cutting a region out
// of a screen, pasting an asset back into a screen, and transforming the
// attributes of an asset.

package scr

import "fmt"

// CutRegion extracts a pixel region of a screen as an Asset. If keepAttrs is
// true and the region is cell-aligned, the asset carries the region's
// attributes; otherwise it is a bitmap-only asset. A region that is not
// cell-aligned always produces a bitmap-only asset regardless of keepAttrs.
func CutRegion(s *Screen, r Rect, name string, keepAttrs bool) (*Asset, error) {
	if r.W <= 0 || r.H <= 0 {
		return nil, fmt.Errorf("cut: region must be positive (got %dx%d)", r.W, r.H)
	}
	if r.X < 0 || r.Y < 0 || r.X+r.W > Width || r.Y+r.H > Height {
		return nil, fmt.Errorf("cut: region %d,%d,%d,%d outside screen", r.X, r.Y, r.W, r.H)
	}

	a := &Asset{Name: name, Width: r.W, Height: r.H}
	a.Ink = make([][]bool, r.H)
	for y := 0; y < r.H; y++ {
		a.Ink[y] = make([]bool, r.W)
		for x := 0; x < r.W; x++ {
			a.Ink[y][x] = s.Ink[r.Y+y][r.X+x]
		}
	}

	cellAligned := r.X%8 == 0 && r.Y%8 == 0 && r.W%8 == 0 && r.H%8 == 0
	if keepAttrs && cellAligned {
		cw, ch := r.W/8, r.H/8
		ocx, ocy := r.X/8, r.Y/8
		a.Attr = make([][]Attribute, ch)
		for cy := 0; cy < ch; cy++ {
			a.Attr[cy] = make([]Attribute, cw)
			for cx := 0; cx < cw; cx++ {
				a.Attr[cy][cx] = s.Attr[ocy+cy][ocx+cx]
			}
		}
	}
	return a, nil
}

// CutCells extracts a cell-aligned region given in character cells.
func CutCells(s *Screen, cx, cy, cw, ch int, name string, keepAttrs bool) (*Asset, error) {
	return CutRegion(s, CellRect(cx, cy, cw, ch), name, keepAttrs)
}

// PasteOp selects how an asset's bitmap bits combine with the target screen's
// existing bits, following the classic Spectrum sprite blit operations.
type PasteOp int

const (
	// PasteOR sets target bits where the asset bit is set and leaves the target
	// unchanged elsewhere (paint the sprite on). This is the default and the
	// natural operation for a bitmask or data blit.
	PasteOR PasteOp = iota
	// PasteAND clears target bits where the asset bit is clear and leaves the
	// target unchanged where the asset bit is set (punch a hole with a mask).
	PasteAND
	// PasteCOPY overwrites every target bit with the asset bit, set or clear.
	PasteCOPY
	// PasteXOR toggles target bits where the asset bit is set (cheap reversible
	// draw/erase).
	PasteXOR
)

// Paste blits an asset onto a screen at pixel position (x, y), combining the
// asset's bitmap with the target according to op. If the asset carries
// attributes and (x, y) is cell-aligned, the attributes are written too;
// otherwise attributes are skipped. The asset must fit within the screen.
func Paste(s *Screen, a *Asset, x, y int, op PasteOp) error {
	if x < 0 || y < 0 || x+a.Width > Width || y+a.Height > Height {
		return fmt.Errorf("paste: asset %dx%d at %d,%d does not fit screen", a.Width, a.Height, x, y)
	}
	for ay := 0; ay < a.Height; ay++ {
		for ax := 0; ax < a.Width; ax++ {
			src := a.Ink[ay][ax]
			tx, ty := x+ax, y+ay
			switch op {
			case PasteOR:
				if src {
					s.Ink[ty][tx] = true
				}
			case PasteAND:
				if !src {
					s.Ink[ty][tx] = false
				}
			case PasteCOPY:
				s.Ink[ty][tx] = src
			case PasteXOR:
				if src {
					s.Ink[ty][tx] = !s.Ink[ty][tx]
				}
			}
		}
	}
	if a.HasAttrs() && x%8 == 0 && y%8 == 0 {
		cw, ch := a.Width/8, a.Height/8
		ocx, ocy := x/8, y/8
		for cy := 0; cy < ch; cy++ {
			for cx := 0; cx < cw; cx++ {
				s.Attr[ocy+cy][ocx+cx] = a.Attr[cy][cx]
			}
		}
	}
	return nil
}

// ApplyMask filters an asset's bitmap through a mask asset of identical
// dimensions: a target bit is cleared wherever the mask bit is unset. This is
// the standalone-mask filtering operation. Both assets must share dimensions.
func ApplyMask(a, mask *Asset) error {
	if a.Width != mask.Width || a.Height != mask.Height {
		return fmt.Errorf("mask: dimensions %dx%d != asset %dx%d", mask.Width, mask.Height, a.Width, a.Height)
	}
	for y := 0; y < a.Height; y++ {
		for x := 0; x < a.Width; x++ {
			if !mask.Ink[y][x] {
				a.Ink[y][x] = false
			}
		}
	}
	return nil
}

// MapAttributes applies f to every attribute cell of an asset, leaving the
// bitmap untouched. It is a no-op on a bitmap-only asset.
func (a *Asset) MapAttributes(f func(Attribute) Attribute) {
	if !a.HasAttrs() {
		return
	}
	for cy := range a.Attr {
		for cx := range a.Attr[cy] {
			a.Attr[cy][cx] = f(a.Attr[cy][cx])
		}
	}
}
