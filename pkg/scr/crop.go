// Cropping operations.
//
// Crop extracts a rectangular region of an image. The region can be given in
// pixels or, via CellRect, in 8x8 Spectrum character cells. AutoExtent finds the
// bounding box of an image's non-background pixels, so a sprite drawn on a flat
// field can be tightened to its used area.

package scr

import (
	"fmt"
	"image"
	"image/color"
)

// Rect is a crop rectangle in pixels: origin (X, Y) with width W and height H.
// W and H need not be multiples of 8.
type Rect struct {
	X, Y, W, H int
}

// CellRect converts a rectangle expressed in 8x8 character cells to a pixel
// Rect. A cell rectangle at cell (cx, cy) spanning cw by ch cells covers
// (cx*8, cy*8) with size (cw*8, ch*8).
func CellRect(cx, cy, cw, ch int) Rect {
	return Rect{X: cx * CellSize, Y: cy * CellSize, W: cw * CellSize, H: ch * CellSize}
}

// Crop returns the sub-image of src described by r. The result is a new RGBA
// image with its own pixel storage (not a view into src). r must lie within
// src's bounds and have positive dimensions.
func Crop(src image.Image, r Rect) (*image.RGBA, error) {
	if r.W <= 0 || r.H <= 0 {
		return nil, fmt.Errorf("crop: width and height must be positive (got %dx%d)", r.W, r.H)
	}
	b := src.Bounds()
	if r.X < 0 || r.Y < 0 || r.X+r.W > b.Dx() || r.Y+r.H > b.Dy() {
		return nil, fmt.Errorf("crop: region %d,%d,%d,%d lies outside image %dx%d",
			r.X, r.Y, r.W, r.H, b.Dx(), b.Dy())
	}
	dst := image.NewRGBA(image.Rect(0, 0, r.W, r.H))
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			dst.Set(x, y, src.At(b.Min.X+r.X+x, b.Min.Y+r.Y+y))
		}
	}
	return dst, nil
}

// AutoExtent returns the smallest Rect covering every pixel of src that differs
// from the background colour. If bg is nil the background is inferred as the
// image's most common edge colour (sampling the four borders), which handles the
// usual case of a sprite on a flat field without the caller naming the colour.
//
// A pixel "differs" when any RGB channel is more than tol (0-255) away from the
// background; tol absorbs antialiasing and minor compression noise. If no pixel
// differs, AutoExtent returns an error rather than an empty rectangle.
func AutoExtent(src image.Image, bg color.Color, tol int) (Rect, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return Rect{}, fmt.Errorf("auto-extent: empty image")
	}

	var br, bgn, bb int
	if bg != nil {
		r16, g16, b16, _ := bg.RGBA()
		br, bgn, bb = int(r16>>8), int(g16>>8), int(b16>>8)
	} else {
		br, bgn, bb = inferBackground(src)
	}

	minX, minY := w, h
	maxX, maxY := -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r16, g16, b16, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			r, g, bl := int(r16>>8), int(g16>>8), int(b16>>8)
			if abs(r-br) > tol || abs(g-bgn) > tol || abs(bl-bb) > tol {
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < 0 {
		return Rect{}, fmt.Errorf("auto-extent: image is uniform; nothing to crop")
	}
	return Rect{X: minX, Y: minY, W: maxX - minX + 1, H: maxY - minY + 1}, nil
}

// inferBackground returns the most common colour among the image's edge pixels.
func inferBackground(src image.Image) (r, g, b int) {
	bd := src.Bounds()
	w, h := bd.Dx(), bd.Dy()
	counts := map[[3]int]int{}
	tally := func(x, y int) {
		r16, g16, b16, _ := src.At(bd.Min.X+x, bd.Min.Y+y).RGBA()
		counts[[3]int{int(r16 >> 8), int(g16 >> 8), int(b16 >> 8)}]++
	}
	for x := 0; x < w; x++ {
		tally(x, 0)
		tally(x, h-1)
	}
	for y := 0; y < h; y++ {
		tally(0, y)
		tally(w-1, y)
	}
	best := [3]int{0, 0, 0}
	bestN := -1
	for c, n := range counts {
		if n > bestN {
			bestN = n
			best = c
		}
	}
	return best[0], best[1], best[2]
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// BitmapExtent returns the smallest Rect covering every set bitmap bit of a
// Screen. "Set" means an ink pixel (Ink[y][x] == true) when bits is 1, or a
// paper pixel (Ink[y][x] == false) when bits is 0 - the latter for sprites
// drawn as paper-on-ink. Attributes are ignored entirely: a sprite's extent is
// defined by its bitmap, not by what colours its cells happen to carry.
//
// If no pixel matches, BitmapExtent returns an error rather than an empty
// rectangle.
func BitmapExtent(s *Screen, bits int) (Rect, error) {
	target := bits != 0
	minX, minY := Width, Height
	maxX, maxY := -1, -1
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if s.Ink[y][x] == target {
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < 0 {
		return Rect{}, fmt.Errorf("bitmap-extent: no %s pixels found",
			map[bool]string{true: "set", false: "unset"}[target])
	}
	return Rect{X: minX, Y: minY, W: maxX - minX + 1, H: maxY - minY + 1}, nil
}
