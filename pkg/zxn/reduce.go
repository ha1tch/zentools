// file: reduce.go
//
// Colour reduction: from any image to an indexed image whose palette holds
// at most a given number of Next colours, with an optional transparent index
// and optional dithering.

package zxn

import (
	"fmt"
	"image"
	"image/color"
	"sort"
	"strings"
)

// DefaultTransparent is the Next's default global transparency index (and
// RGB332 colour), 0xE3, set by NextReg 0x14 at reset.
const DefaultTransparent = 0xE3

// Dither selects how Reduce spreads the error of each pixel's colour.
type Dither int

const (
	// DitherNone maps each pixel to its nearest palette colour.
	DitherNone Dither = iota
	// DitherFloydSteinberg diffuses each pixel's error to its unvisited
	// neighbours (7/16 right, 3/16, 5/16 and 1/16 on the row below), in
	// 8-bit RGB.
	DitherFloydSteinberg
)

// ParseDither accepts "none" and "fs" (or "floyd-steinberg").
func ParseDither(s string) (Dither, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none":
		return DitherNone, nil
	case "fs", "floyd-steinberg", "floydsteinberg":
		return DitherFloydSteinberg, nil
	}
	return DitherNone, fmt.Errorf("unknown dither %q (want none or fs)", s)
}

// ReduceOptions control Reduce.
type ReduceOptions struct {
	// Colors is the palette size, 2 to 256 (16 for a 4-bit image).
	Colors int
	// Round is how 8-bit channels become RGB333.
	Round RoundMode
	// Dither is how pixel colours are mapped once the palette is chosen.
	Dither Dither
	// Transparent, when 0 or more, is the index that pixels with alpha below
	// half are given. That entry holds the Next's transparent colour (the
	// RGB332 value of the index, as the hardware compares it) and is not
	// used for opaque pixels. -1 treats every pixel as opaque.
	Transparent int
	// Palette, when set, is used as it is instead of choosing colours, and
	// Colors is its length: each pixel takes the nearest of its colours (an
	// exact match, the lowest index of one, when every colour of the image
	// is in it; otherwise by distance, dithered as Dither says). The
	// Transparent entry, if any, is set to the transparent colour.
	Palette *Palette
}

// Reduce returns an indexed version of img in at most opt.Colors Next
// colours, and its palette (always opt.Colors entries, the unused ones
// black). When the image's opaque pixels have no more distinct RGB333
// colours than there is room for, they are all kept, in the order they first
// appear, and no pixel changes beyond its reduction to RGB333; otherwise the
// colours are chosen by median cut over the image's RGB333 histogram.
func Reduce(img image.Image, opt ReduceOptions) (*image.Paletted, *Palette, error) {
	n := opt.Colors
	if opt.Palette != nil {
		n = len(opt.Palette.Colors)
	}
	if n < 2 || n > MaxColors {
		return nil, nil, fmt.Errorf("zxn: reduce to %d colours; want 2 to %d", n, MaxColors)
	}
	tr := opt.Transparent
	if tr >= n {
		return nil, nil, fmt.Errorf("zxn: transparent index %d is outside a %d-colour palette", tr, n)
	}
	room := n
	if tr >= 0 {
		room--
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	// The histogram of opaque RGB333 colours, and their first-seen order.
	var order []Color333
	count := map[Color333]int{}
	clear := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.At(b.Min.X+x, b.Min.Y+y)
			if tr >= 0 {
				if _, _, _, a := c.RGBA(); a < 0x8000 {
					clear[y*w+x] = true
					continue
				}
			}
			k := ToRGB333(c, opt.Round)
			if count[k] == 0 {
				order = append(order, k)
			}
			count[k]++
		}
	}
	var chosen []Color333
	var slot []int // palette index of each chosen colour
	pal := &Palette{Colors: make([]Color333, n)}
	useExact := len(order) <= room
	if opt.Palette != nil {
		copy(pal.Colors, opt.Palette.Colors)
		for i, c := range pal.Colors {
			if i != tr {
				c.Priority = false
				chosen = append(chosen, c)
				slot = append(slot, i)
			}
		}
	} else {
		chosen = order
		if len(order) > room {
			chosen = medianCut(order, count, room)
		}
		next := 0
		for _, c := range chosen {
			if next == tr {
				next++
			}
			pal.Colors[next] = c
			slot = append(slot, next)
			next++
		}
	}
	if tr >= 0 {
		pal.Colors[tr] = rgb332(uint8(tr))
	}

	cp := make(color.Palette, n)
	for i, c := range pal.Colors {
		cp[i] = c.RGBA()
	}
	if tr >= 0 {
		cp[tr] = color.RGBA{}
	}
	out := image.NewPaletted(image.Rect(0, 0, w, h), cp)

	exact := map[Color333]uint8{}
	for i := len(chosen) - 1; i >= 0; i-- { // the lowest index wins
		exact[chosen[i]] = uint8(slot[i])
	}
	for _, c := range order {
		if _, ok := exact[c]; !ok {
			useExact = false
		}
	}
	nearest := nearestFinder(chosen, slot)
	var errs [2][]rgbErr
	if opt.Dither == DitherFloydSteinberg && !useExact {
		errs[0], errs[1] = make([]rgbErr, w+2), make([]rgbErr, w+2)
	}
	for y := 0; y < h; y++ {
		if errs[0] != nil {
			errs[0], errs[1] = errs[1], errs[0]
			for i := range errs[1] {
				errs[1][i] = rgbErr{}
			}
		}
		for x := 0; x < w; x++ {
			i := y*w + x
			if clear[i] {
				out.Pix[i] = uint8(tr)
				continue
			}
			c := img.At(b.Min.X+x, b.Min.Y+y)
			if useExact {
				out.Pix[i] = exact[ToRGB333(c, opt.Round)]
				continue
			}
			r, g, bl, _ := c.RGBA()
			v := rgbErr{float64(r >> 8), float64(g >> 8), float64(bl >> 8)}
			if errs[0] != nil {
				e := errs[0][x+1]
				v = rgbErr{clamp8(v.r + e.r), clamp8(v.g + e.g), clamp8(v.b + e.b)}
			}
			k := nearest(v)
			out.Pix[i] = uint8(slot[k])
			if errs[0] != nil {
				q := chosen[k].RGBA()
				d := rgbErr{v.r - float64(q.R), v.g - float64(q.G), v.b - float64(q.B)}
				errs[0][x+2].add(d, 7.0/16)
				errs[1][x].add(d, 3.0/16)
				errs[1][x+1].add(d, 5.0/16)
				errs[1][x+2].add(d, 1.0/16)
			}
		}
	}
	return out, pal, nil
}

type rgbErr struct{ r, g, b float64 }

func (e *rgbErr) add(d rgbErr, f float64) {
	e.r += d.r * f
	e.g += d.g * f
	e.b += d.b * f
}

func clamp8(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// nearestFinder returns a function giving the index into chosen of the
// colour nearest an 8-bit RGB value (squared distance; the first on a tie).
func nearestFinder(chosen []Color333, slot []int) func(rgbErr) int {
	levels := make([]rgbErr, len(chosen))
	for i, c := range chosen {
		q := c.RGBA()
		levels[i] = rgbErr{float64(q.R), float64(q.G), float64(q.B)}
	}
	return func(v rgbErr) int {
		best, bestD := 0, -1.0
		for i, l := range levels {
			dr, dg, db := v.r-l.r, v.g-l.g, v.b-l.b
			if d := dr*dr + dg*dg + db*db; bestD < 0 || d < bestD {
				best, bestD = i, d
			}
		}
		return best
	}
}

// medianCut picks up to k colours for a histogram. It starts from one box
// holding every colour and repeatedly splits the box whose colours lie
// furthest from their mean (the largest pixel-weighted sum of squared
// distances, so a large but even area does not take colours from a small
// varied one), along its widest channel, at the point that leaves the two
// halves the least squared error. Each box gives the RGB333 colour nearest
// its pixels' mean.
func medianCut(colors []Color333, count map[Color333]int, k int) []Color333 {
	type box struct {
		cs  []Color333
		sse float64
	}
	level := func(c Color333, ch int) float64 {
		switch ch {
		case 0:
			return float64(level8[c.R&7])
		case 1:
			return float64(level8[c.G&7])
		}
		return float64(level8[c.B&7])
	}
	// stats returns a set of colours' pixel count, mean and squared error.
	stats := func(cs []Color333) (float64, [3]float64, float64) {
		var n, sse float64
		var sum, sq [3]float64
		for _, c := range cs {
			f := float64(count[c])
			n += f
			for ch := 0; ch < 3; ch++ {
				v := level(c, ch)
				sum[ch] += f * v
				sq[ch] += f * v * v
			}
		}
		var mean [3]float64
		for ch := 0; ch < 3; ch++ {
			mean[ch] = sum[ch] / n
			sse += sq[ch] - sum[ch]*sum[ch]/n
		}
		return n, mean, sse
	}
	newBox := func(cs []Color333) box {
		_, _, sse := stats(cs)
		return box{cs: cs, sse: sse}
	}
	boxes := []box{newBox(append([]Color333(nil), colors...))}
	for len(boxes) < k {
		pick := -1
		for i, bx := range boxes {
			if len(bx.cs) > 1 && bx.sse > 0 && (pick < 0 || bx.sse > boxes[pick].sse) {
				pick = i
			}
		}
		if pick < 0 {
			break
		}
		cs := boxes[pick].cs
		ch, widest := 0, -1.0
		for c := 0; c < 3; c++ {
			lo, hi := 255.0, 0.0
			for _, col := range cs {
				v := level(col, c)
				if v < lo {
					lo = v
				}
				if v > hi {
					hi = v
				}
			}
			if hi-lo > widest {
				ch, widest = c, hi-lo
			}
		}
		sort.SliceStable(cs, func(i, j int) bool { return level(cs[i], ch) < level(cs[j], ch) })
		// Split between two different levels of the channel, where the
		// halves' squared errors add up to the least.
		cut, best := 0, -1.0
		for i := 1; i < len(cs); i++ {
			if level(cs[i], ch) == level(cs[i-1], ch) {
				continue
			}
			_, _, a := stats(cs[:i])
			_, _, b := stats(cs[i:])
			if best < 0 || a+b < best {
				cut, best = i, a+b
			}
		}
		if cut == 0 {
			boxes[pick].sse = 0 // cannot split along this channel; leave it
			continue
		}
		boxes[pick] = newBox(cs[:cut])
		boxes = append(boxes, newBox(cs[cut:]))
	}
	out := make([]Color333, 0, len(boxes))
	for _, bx := range boxes {
		_, m, _ := stats(bx.cs)
		out = append(out, ToRGB333(color.RGBA{R: uint8(m[0] + 0.5), G: uint8(m[1] + 0.5), B: uint8(m[2] + 0.5), A: 0xFF}, RoundNearest))
	}
	return out
}
