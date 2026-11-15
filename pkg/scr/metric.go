// file: metric.go
//
// The error measures of the ZXMODE specification (Part 3, section 15): the
// RMS difference between two pictures after each is blurred with a Gaussian
// of 1.5 pixels per channel (0.75 for the lighter measure), and SSIM.
//
// The specification's tools blur with Pillow's ImageFilter.GaussianBlur, which
// approximates a Gaussian with three passes of an extended box filter in 8-bit
// fixed point (libImaging/BoxBlur.c); BlurPillow follows it step for step, so
// the blurred pictures are the same bytes and the errors the same numbers.
// SSIM follows scikit-image's structural_similarity with its defaults (a 7x7
// uniform window, K1 0.01, K2 0.03, sample covariance, data range 255),
// averaged over the three channels.

package scr

import (
	"fmt"
	"image"
	"math"
)

// gaussianBoxRadius is Pillow's _gaussian_blur_radius, in 32-bit floats.
func gaussianBoxRadius(radius float32, passes int) float32 {
	sigma2 := radius * radius / float32(passes)
	L := float32(math.Sqrt(12.0*float64(sigma2) + 1.0))
	l := float32(math.Floor((float64(L) - 1.0) / 2.0))
	a := (2*l + 1) * (l*(l+1) - 3*sigma2)
	a /= 6 * (sigma2 - (l+1)*(l+1))
	return l + a
}

// boxBlurLine is Pillow's ImagingLineBoxBlur8 over one line of n samples
// taken every stride bytes from in, written likewise to out.
func boxBlurLine(out, in []uint8, n, radius int, ww, fw uint32) {
	lastx := n - 1
	edgeA := radius + 1
	if n < edgeA {
		edgeA = n
	}
	edgeB := n - radius - 1
	if edgeB < 0 {
		edgeB = 0
	}
	at := func(i int) uint32 { return uint32(in[i]) }
	acc := at(0) * uint32(radius+1)
	for x := 0; x < edgeA-1; x++ {
		acc += at(x)
	}
	acc += at(lastx) * uint32(radius-edgeA+1)
	move := func(sub, add int) { acc += at(add) - at(sub) }
	save := func(x, left, right int) {
		bulk := acc*ww + (at(left)+at(right))*fw
		out[x] = uint8((bulk + 1<<23) >> 24)
	}
	if edgeA <= edgeB {
		for x := 0; x < edgeA; x++ {
			move(0, x+radius)
			save(x, 0, x+radius+1)
		}
		for x := edgeA; x < edgeB; x++ {
			move(x-radius-1, x+radius)
			save(x, x-radius-1, x+radius+1)
		}
		for x := edgeB; x <= lastx; x++ {
			move(x-radius-1, lastx)
			save(x, x-radius-1, lastx)
		}
		return
	}
	for x := 0; x < edgeB; x++ {
		move(0, x+radius)
		save(x, 0, x+radius+1)
	}
	for x := edgeB; x < edgeA; x++ {
		move(0, lastx)
		save(x, 0, lastx)
	}
	for x := edgeA; x <= lastx; x++ {
		move(x-radius-1, lastx)
		save(x, x-radius-1, lastx)
	}
}

// horizontalBoxBlur is Pillow's ImagingHorizontalBoxBlur, in place.
func horizontalBoxBlur(p []uint8, w, h int, r float32) {
	radius := int(r)
	ww := uint32(float32(1<<24) / (r*2 + 1))
	fw := ((1 << 24) - uint32(radius*2+1)*ww) / 2
	line := make([]uint8, w)
	for y := 0; y < h; y++ {
		row := p[y*w : (y+1)*w]
		boxBlurLine(line, row, w, radius, ww, fw)
		copy(row, line)
	}
}

func transpose(p []uint8, w, h int) []uint8 {
	out := make([]uint8, len(p))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out[x*h+y] = p[y*w+x]
		}
	}
	return out
}

// BlurPillow blurs one 8-bit channel of w x h samples as Pillow's
// ImageFilter.GaussianBlur(radius) does, and returns the result.
func BlurPillow(plane []uint8, w, h int, radius float64) []uint8 {
	out := append([]uint8(nil), plane...)
	if radius == 0 {
		return out
	}
	const passes = 3
	r := gaussianBoxRadius(float32(radius), passes)
	if r != 0 {
		for i := 0; i < passes; i++ {
			horizontalBoxBlur(out, w, h, r)
		}
		t := transpose(out, w, h)
		for i := 0; i < passes; i++ {
			horizontalBoxBlur(t, h, w, r)
		}
		out = transpose(t, h, w)
	}
	return out
}

// channels splits an image into three 8-bit planes.
func channels(img image.Image) ([3][]uint8, int, int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	var c [3][]uint8
	for i := range c {
		c[i] = make([]uint8, w*h)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			c[0][y*w+x], c[1][y*w+x], c[2][y*w+x] = uint8(r>>8), uint8(g>>8), uint8(bl>>8)
		}
	}
	return c, w, h
}

func sameSize(a, b image.Image) (int, int, error) {
	if a.Bounds().Size() != b.Bounds().Size() {
		return 0, 0, fmt.Errorf("scr: images are %v and %v", a.Bounds().Size(), b.Bounds().Size())
	}
	return a.Bounds().Dx(), a.Bounds().Dy(), nil
}

// BlurredRMSE is the specification's error measure: the RMS difference, over
// every pixel and channel on a 0-255 scale, between ref and img after each
// channel of each is blurred with BlurPillow(sigma). With sigma 0 it is the
// plain RMS difference.
func BlurredRMSE(ref, img image.Image, sigma float64) (float64, error) {
	w, h, err := sameSize(ref, img)
	if err != nil {
		return 0, err
	}
	a, _, _ := channels(ref)
	b, _, _ := channels(img)
	sum := 0.0
	for c := 0; c < 3; c++ {
		ba, bb := BlurPillow(a[c], w, h, sigma), BlurPillow(b[c], w, h, sigma)
		for i := range ba {
			d := float64(ba[i]) - float64(bb[i])
			sum += d * d
		}
	}
	return math.Sqrt(sum / float64(3*w*h)), nil
}

// SSIM is scikit-image's structural_similarity(ref, img, channel_axis=2,
// data_range=255) with its defaults: the mean over the three channels of the
// mean SSIM over every 7x7 window that lies inside the picture.
func SSIM(ref, img image.Image) (float64, error) {
	w, h, err := sameSize(ref, img)
	if err != nil {
		return 0, err
	}
	const win = 7
	if w < win || h < win {
		return 0, fmt.Errorf("scr: SSIM needs a picture of at least 7x7")
	}
	a, _, _ := channels(ref)
	b, _, _ := channels(img)
	const np = win * win
	covNorm := float64(np) / float64(np-1)
	c1 := (0.01 * 255) * (0.01 * 255)
	c2 := (0.03 * 255) * (0.03 * 255)
	total := 0.0
	for c := 0; c < 3; c++ {
		x, y := a[c], b[c]
		// Box sums over each window, through summed-area tables.
		sat := func(f func(i int) float64) []float64 {
			t := make([]float64, (w+1)*(h+1))
			for yy := 0; yy < h; yy++ {
				row := 0.0
				for xx := 0; xx < w; xx++ {
					row += f(yy*w + xx)
					t[(yy+1)*(w+1)+xx+1] = t[yy*(w+1)+xx+1] + row
				}
			}
			return t
		}
		sx := sat(func(i int) float64 { return float64(x[i]) })
		sy := sat(func(i int) float64 { return float64(y[i]) })
		sxx := sat(func(i int) float64 { return float64(x[i]) * float64(x[i]) })
		syy := sat(func(i int) float64 { return float64(y[i]) * float64(y[i]) })
		sxy := sat(func(i int) float64 { return float64(x[i]) * float64(y[i]) })
		box := func(t []float64, x0, y0 int) float64 {
			x1, y1 := x0+win, y0+win
			return (t[y1*(w+1)+x1] - t[y0*(w+1)+x1] - t[y1*(w+1)+x0] + t[y0*(w+1)+x0]) / np
		}
		sum, n := 0.0, 0
		for y0 := 0; y0+win <= h; y0++ {
			for x0 := 0; x0+win <= w; x0++ {
				ux, uy := box(sx, x0, y0), box(sy, x0, y0)
				vx := covNorm * (box(sxx, x0, y0) - ux*ux)
				vy := covNorm * (box(syy, x0, y0) - uy*uy)
				vxy := covNorm * (box(sxy, x0, y0) - ux*uy)
				sum += ((2*ux*uy + c1) * (2*vxy + c2)) / ((ux*ux + uy*uy + c1) * (vx + vy + c2))
				n++
			}
		}
		total += sum / float64(n)
	}
	return total / 3, nil
}
