// Conversion from ordinary raster images to a Spectrum Screen.
//
// The Spectrum applies colour per 8x8 cell, not per pixel: each cell has one
// ink and one paper colour drawn from a 15-colour palette (eight hues at two
// brightness levels, with black shared). FromImage reduces an arbitrary image
// to that constraint - the well-known "attribute clash" - by choosing, for each
// cell, the two palette colours that best represent it and assigning every pixel
// to the nearer of the two.
//
// Input decoding uses the standard library only: PNG, JPEG, and GIF are
// recognised because their decoders are registered as blank imports below, so a
// single image.Decode call accepts any of the three.

package scr

import (
	"image"
	"image/color"
	"image/draw"
	"io"

	_ "image/gif"  // register GIF decoder for image.Decode
	_ "image/jpeg" // register JPEG decoder for image.Decode
	_ "image/png"  // register PNG decoder for image.Decode
)

// rgb is an 8-bit-per-channel colour used during reduction.
type rgb struct{ r, g, b int }

// paletteRGB returns the eight Spectrum hues at the given brightness as rgb
// triples, delegating to paletteColor (the single palette source of truth) so
// the reduction code and the attribute helpers can never disagree.
func paletteRGB(bright bool) [8]rgb {
	var out [8]rgb
	for i := 0; i < 8; i++ {
		c := paletteColor(uint8(i), bright)
		out[i] = rgb{int(c.R), int(c.G), int(c.B)}
	}
	return out
}

// legalColour is one selectable Spectrum colour: its RGB, its palette index,
// and whether it belongs to the bright set.
type legalColour struct {
	c      rgb
	index  uint8
	bright bool
}

// legalColours is the full set of distinct attribute colours. Black is identical
// in both brightnesses, so it is included once (as non-bright).
func legalColours() []legalColour {
	var out []legalColour
	for _, bright := range []bool{false, true} {
		pal := paletteRGB(bright)
		for i, c := range pal {
			if bright && i == 0 {
				continue // skip duplicate black
			}
			out = append(out, legalColour{c: c, index: uint8(i), bright: bright})
		}
	}
	return out
}

func dist2(a, b rgb) int {
	dr := a.r - b.r
	dg := a.g - b.g
	db := a.b - b.b
	return dr*dr + dg*dg + db*db
}

func luma(c rgb) int {
	// integer approximation of 0.299R + 0.587G + 0.114B, scaled by 1000
	return (299*c.r + 587*c.g + 114*c.b) / 1000
}

// classify returns the nearest legal colour to c.
func classify(c rgb, legal []legalColour) legalColour {
	best := legal[0]
	bd := dist2(c, best.c)
	for _, lc := range legal[1:] {
		if d := dist2(c, lc.c); d < bd {
			bd = d
			best = lc
		}
	}
	return best
}

// DecodeImage reads a PNG, JPEG, or GIF from r and returns it as an image.Image.
// It is a thin wrapper over image.Decode, exposed so callers can inspect or
// resize an image before conversion.
func DecodeImage(r io.Reader) (image.Image, error) {
	img, _, err := image.Decode(r)
	return img, err
}

// ResizeMode selects how Fit brings a source image to the 256x192 screen size.
type ResizeMode int

const (
	// ResizeNone requires the source to be exactly 256x192 and errors otherwise.
	// It is the default, so a caller that does not opt into a resize is told
	// which modes exist rather than having a policy chosen for it.
	ResizeNone ResizeMode = iota
	// ResizeStretch scales to fill the whole screen, ignoring aspect ratio.
	ResizeStretch
	// ResizeBestFit scales to fit within the screen preserving aspect ratio,
	// centring the result and padding the remaining border with fill.
	ResizeBestFit
	// ResizeCentre does not scale: it centres the source on the screen, cropping
	// any overflow and padding any shortfall with fill.
	ResizeCentre
)

// Fit returns a 256x192 image produced from src according to mode. The fill
// colour is used for padding in ResizeBestFit and ResizeCentre (it has no effect
// on ResizeStretch, which leaves no border). ResizeNone returns src unchanged
// when it is already 256x192 and a *SizeError otherwise.
//
// The scaler used for ResizeStretch and ResizeBestFit is a simple bilinear
// resample. That is deliberate: the conversion that follows collapses every 8x8
// cell to two colours, so a higher-order resampler's extra fidelity would be
// destroyed by the attribute reduction and is not worth a dependency.
func Fit(src image.Image, mode ResizeMode, fill color.Color) (image.Image, error) {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()

	if mode == ResizeNone {
		if sw != Width || sh != Height {
			return nil, &SizeError{Got: image.Pt(sw, sh)}
		}
		return src, nil
	}

	canvas := image.NewRGBA(image.Rect(0, 0, Width, Height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(fill), image.Point{}, draw.Src)

	switch mode {
	case ResizeStretch:
		bilinear(canvas, canvas.Bounds(), src)

	case ResizeBestFit:
		// Largest scale that fits both dimensions, preserving aspect.
		scale := float64(Width) / float64(sw)
		if s := float64(Height) / float64(sh); s < scale {
			scale = s
		}
		dw := int(float64(sw) * scale)
		dh := int(float64(sh) * scale)
		if dw < 1 {
			dw = 1
		}
		if dh < 1 {
			dh = 1
		}
		dst := image.Rect(0, 0, dw, dh).Add(image.Pt((Width-dw)/2, (Height-dh)/2))
		bilinear(canvas, dst, src)

	case ResizeCentre:
		// No scaling: copy src 1:1, centred. Overflow is cropped, shortfall is
		// already painted with fill above.
		off := image.Pt((Width-sw)/2, (Height-sh)/2)
		dst := b.Add(off.Sub(b.Min))
		draw.Draw(canvas, dst, src, b.Min, draw.Src)

	default:
		return nil, &SizeError{Got: image.Pt(sw, sh)}
	}

	return canvas, nil
}

// bilinear resamples src into the rectangle dst within dstImg using bilinear
// interpolation. Pixels of dstImg outside dst are left untouched.
func bilinear(dstImg *image.RGBA, dst image.Rectangle, src image.Image) {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dw, dh := dst.Dx(), dst.Dy()
	if dw == 0 || dh == 0 || sw == 0 || sh == 0 {
		return
	}
	for dy := 0; dy < dh; dy++ {
		// map destination row to a fractional source row
		fy := (float64(dy) + 0.5) * float64(sh) / float64(dh)
		sy := int(fy - 0.5)
		ty := fy - 0.5 - float64(sy)
		sy0 := clampi(sy, 0, sh-1)
		sy1 := clampi(sy+1, 0, sh-1)
		for dx := 0; dx < dw; dx++ {
			fx := (float64(dx) + 0.5) * float64(sw) / float64(dw)
			sx := int(fx - 0.5)
			tx := fx - 0.5 - float64(sx)
			sx0 := clampi(sx, 0, sw-1)
			sx1 := clampi(sx+1, 0, sw-1)

			c00 := rgbaOf(src, sb.Min.X+sx0, sb.Min.Y+sy0)
			c10 := rgbaOf(src, sb.Min.X+sx1, sb.Min.Y+sy0)
			c01 := rgbaOf(src, sb.Min.X+sx0, sb.Min.Y+sy1)
			c11 := rgbaOf(src, sb.Min.X+sx1, sb.Min.Y+sy1)

			r := lerp2(c00[0], c10[0], c01[0], c11[0], tx, ty)
			g := lerp2(c00[1], c10[1], c01[1], c11[1], tx, ty)
			bl := lerp2(c00[2], c10[2], c01[2], c11[2], tx, ty)
			dstImg.SetRGBA(dst.Min.X+dx, dst.Min.Y+dy,
				color.RGBA{uint8(r), uint8(g), uint8(bl), 0xFF})
		}
	}
}

func rgbaOf(img image.Image, x, y int) [3]float64 {
	r, g, b, _ := img.At(x, y).RGBA()
	return [3]float64{float64(r >> 8), float64(g >> 8), float64(b >> 8)}
}

func lerp2(c00, c10, c01, c11, tx, ty float64) float64 {
	top := c00 + (c10-c00)*tx
	bot := c01 + (c11-c01)*tx
	return top + (bot-top)*ty
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// FromImage reduces img to a Spectrum Screen.
//
// The image must already be 256x192; FromImage does not resize, because the
// right resizing strategy (fit, fill, letterbox) is a caller decision and a
// naive resize here would silently distort logos and screenshots. Callers that
// need scaling should resize to 256x192 first. An image of the wrong size
// returns an error.
//
// For each 8x8 cell, the two colours are chosen by luminance: the darkest
// distinct colour present becomes paper, the brightest becomes ink. A cell holds
// a single bright bit, decided from the ink colour (the visually dominant
// foreground); a black ink or paper, being brightness-agnostic, does not force
// the bit. Every pixel is then set to ink or paper by whichever it is nearer.
func FromImage(img image.Image) (*Screen, error) {
	b := img.Bounds()
	if b.Dx() != Width || b.Dy() != Height {
		return nil, &SizeError{Got: image.Pt(b.Dx(), b.Dy())}
	}

	legal := legalColours()
	at := func(x, y int) rgb {
		// image colours are 16-bit per channel; bring down to 8-bit.
		r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
		return rgb{int(r >> 8), int(g >> 8), int(bl >> 8)}
	}

	s := &Screen{}

	for cy := 0; cy < Rows; cy++ {
		for cx := 0; cx < Cols; cx++ {
			s.encodeCell(cx, cy, at, legal)
		}
	}

	return s, nil
}

// posteriseLevels is the number of steps each colour channel is reduced to
// before palette mapping. Posterising flattens gradients and resize
// interpolation into broad bands, so a coherent region (e.g. a green area of a
// gem) snaps to one palette colour as a whole instead of fragmenting across
// luminance. Empirically 3 retains mid-luma hues (notably green) far better than
// the old darkest/brightest selection while staying faithful elsewhere.
const posteriseLevels = 3

// posteriseChannel reduces a single 0-255 channel to posteriseLevels steps.
func posteriseChannel(v int) int {
	if posteriseLevels < 2 {
		return v
	}
	step := 255.0 / float64(posteriseLevels-1)
	return int(float64(int(float64(v)/step+0.5))*step + 0.5)
}

func posterise(c rgb) rgb {
	return rgb{posteriseChannel(c.r), posteriseChannel(c.g), posteriseChannel(c.b)}
}

// encodeCell reduces one 8x8 cell to two Spectrum colours and sets the cell's
// attribute and bitmap bits. The pipeline is: posterise each pixel, snap to the
// legal palette, keep the two most frequently occurring legal colours, then
// assign each pixel to the nearer of the two. A cell that resolves to a single
// colour becomes solid paper with no set bits.
func (s *Screen) encodeCell(cx, cy int, at func(x, y int) rgb, legal []legalColour) {
	// Posterise + snap every pixel, tallying by palette INDEX (collapsing the
	// brightness bit). Tallying by index is essential: a feature whose pixels
	// straddle both brightness sets (e.g. an antialiased white stroke posterised
	// into bright-white and dim-white) must count as one colour, otherwise its
	// vote splits in two and can crowd out a genuinely distinct colour such as
	// the black background - which previously wiped thin strokes to empty.
	var counts [8]int
	// brightVote[index] tallies how many of that index's pixels were bright, to
	// decide the cell's single brightness bit afterward.
	var brightVote [8]int
	for yy := 0; yy < CellSize; yy++ {
		for xx := 0; xx < CellSize; xx++ {
			lc := classify(posterise(at(cx*8+xx, cy*8+yy)), legal)
			counts[lc.index]++
			if lc.bright {
				brightVote[lc.index]++
			}
		}
	}

	// Two most frequent indices (first-seen order breaks ties).
	firstIdx, secondIdx := -1, -1
	for i := 0; i < 8; i++ {
		if counts[i] == 0 {
			continue
		}
		if firstIdx < 0 || counts[i] > counts[firstIdx] {
			secondIdx = firstIdx
			firstIdx = i
		} else if secondIdx < 0 || counts[i] > counts[secondIdx] {
			secondIdx = i
		}
	}

	// Brightness: a Spectrum cell shares one bright bit. Take it from the
	// foreground (ink) index's pixels unless that index is black, then follow the
	// other. brightVote decides by majority of that index's own pixels.
	idxBright := func(i int) bool { return i >= 0 && brightVote[i]*2 >= counts[i] }

	inkIdx, paperIdx := firstIdx, secondIdx

	// Uniform cell (only one colour present): solid paper, no set bits.
	if secondIdx < 0 || inkIdx == paperIdx {
		bright := idxBright(firstIdx)
		s.Attr[cy][cx] = Attribute{Ink: uint8(firstIdx), Paper: uint8(firstIdx), Bright: bright}
		return
	}

	// Order ink/paper by luma so the brighter colour is ink.
	if luma(paletteRGB(false)[paperIdx]) > luma(paletteRGB(false)[inkIdx]) {
		inkIdx, paperIdx = paperIdx, inkIdx
	}

	bright := idxBright(inkIdx)
	if inkIdx == 0 { // ink black: take brightness from paper
		bright = idxBright(paperIdx)
	}

	s.Attr[cy][cx] = Attribute{
		Ink:    uint8(inkIdx),
		Paper:  uint8(paperIdx),
		Bright: bright,
	}

	// Reference colours rendered at the cell's actual brightness, so the
	// per-pixel nearest test matches what will display.
	pal := paletteRGB(bright)
	inkRef := pal[inkIdx]
	paperRef := pal[paperIdx]
	for yy := 0; yy < CellSize; yy++ {
		for xx := 0; xx < CellSize; xx++ {
			px := at(cx*8+xx, cy*8+yy)
			if dist2(px, inkRef) <= dist2(px, paperRef) {
				s.Ink[cy*8+yy][cx*8+xx] = true
			}
		}
	}
}

// ToImage renders a Screen back to an RGBA image, for previewing or for
// round-trip verification. The result is 256x192.
func ToImage(s *Screen) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	for cy := 0; cy < Rows; cy++ {
		for cx := 0; cx < Cols; cx++ {
			a := s.Attr[cy][cx]
			pal := paletteRGB(a.Bright)
			ink := pal[a.Ink&7]
			paper := pal[a.Paper&7]
			for yy := 0; yy < CellSize; yy++ {
				for xx := 0; xx < CellSize; xx++ {
					c := paper
					if s.Ink[cy*8+yy][cx*8+xx] {
						c = ink
					}
					img.Set(cx*8+xx, cy*8+yy, color.RGBA{uint8(c.r), uint8(c.g), uint8(c.b), 0xFF})
				}
			}
		}
	}
	return img
}

// ToImageBitmap renders a Screen's bitmap, ignoring its attributes, into a fresh
// 256x192 RGBA image. Set bits (per the bits argument: 1 means ink pixels, 0
// means paper pixels) are painted with ink; the other pixels are painted with
// paper. If paperTransparent is true the non-set pixels are left fully
// transparent instead, so an extracted sprite can be composited over any
// background.
//
// This is the rendering path for bitmap-only sprite extraction: the shape comes
// from the bitmap, and the colours come from the caller (typically a parsed
// attribute) rather than from the screen's own attributes.
func ToImageBitmap(s *Screen, bits int, ink, paper color.Color, paperTransparent bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	ir, ig, ib, ia := rgba8(ink)
	pr, pg, pb, pa := rgba8(paper)
	if paperTransparent {
		pr, pg, pb, pa = 0, 0, 0, 0
	}
	target := bits != 0
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if s.Ink[y][x] == target {
				img.SetRGBA(x, y, color.RGBA{ir, ig, ib, ia})
			} else {
				img.SetRGBA(x, y, color.RGBA{pr, pg, pb, pa})
			}
		}
	}
	return img
}

func rgba8(c color.Color) (r, g, b, a uint8) {
	r16, g16, b16, a16 := c.RGBA()
	return uint8(r16 >> 8), uint8(g16 >> 8), uint8(b16 >> 8), uint8(a16 >> 8)
}

// AssetToImage renders an asset to an RGBA image at its own dimensions. An
// attributed asset is drawn in its stored colours (ink on paper). A bitmap-only
// asset is drawn with the supplied ink and paper; if paperTransparent is true,
// its unset pixels are left fully transparent so it can be composited. The ink
// and paper arguments are ignored for an attributed asset.
func AssetToImage(a *Asset, ink, paper color.Color, paperTransparent bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, a.Width, a.Height))
	if a.HasAttrs() {
		for y := 0; y < a.Height; y++ {
			for x := 0; x < a.Width; x++ {
				at := a.Attr[y/8][x/8]
				pal := paletteRGB(at.Bright)
				var c rgb
				if a.Ink[y][x] {
					c = pal[at.Ink&7]
				} else {
					c = pal[at.Paper&7]
				}
				img.SetRGBA(x, y, color.RGBA{uint8(c.r), uint8(c.g), uint8(c.b), 0xFF})
			}
		}
		return img
	}
	ir, ig, ib, ia := rgba8(ink)
	pr, pg, pb, pa := rgba8(paper)
	if paperTransparent {
		pr, pg, pb, pa = 0, 0, 0, 0
	}
	for y := 0; y < a.Height; y++ {
		for x := 0; x < a.Width; x++ {
			if a.Ink[y][x] {
				img.SetRGBA(x, y, color.RGBA{ir, ig, ib, ia})
			} else {
				img.SetRGBA(x, y, color.RGBA{pr, pg, pb, pa})
			}
		}
	}
	return img
}
