// file: lanczos.go
//
// Lanczos resampling and the picture fit of the ZXMODE specification (Part 3,
// section 13: "fitted to 512x384 with Lanczos resampling and a centred
// crop"). The specification's tools fit pictures with Pillow's
// ImageOps.fit(image, size, Image.LANCZOS, centering=...), so this follows
// Pillow's resampler (libImaging/Resample.c) step for step: the crop box held
// as 32-bit floats, coefficients computed in double precision, normalised and
// rounded to 22-bit fixed point, a horizontal pass over only the source rows
// the vertical pass needs, then the vertical pass, each output clipped to
// 0-255. Given the same 8-bit RGB input, the output is the same bytes.

package scr

import (
	"fmt"
	"image"
	"math"
)

const lanczosPrecision = 32 - 8 - 2

func lanczosKernel(x float64) float64 {
	if -3.0 <= x && x < 3.0 {
		return sinc(x) * sinc(x/3)
	}
	return 0
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	x *= math.Pi
	return math.Sin(x) / x
}

// lanczosCoeffs is Pillow's precompute_coeffs followed by
// normalize_coeffs_8bpc: for each output position, the first source index,
// the number of taps, and the fixed-point weights.
func lanczosCoeffs(inSize int, in0, in1 float32, outSize int) (bounds []int, ksize int, kk []int32) {
	scale := float64(in1-in0) / float64(outSize)
	filterscale := scale
	if filterscale < 1 {
		filterscale = 1
	}
	support := 3.0 * filterscale
	ksize = int(math.Ceil(support))*2 + 1
	bounds = make([]int, 2*outSize)
	kk = make([]int32, outSize*ksize)
	inv := 1.0 / filterscale
	k := make([]float64, ksize)
	for xx := 0; xx < outSize; xx++ {
		center := float64(in0) + (float64(xx)+0.5)*scale
		ww := 0.0
		xmin := int(center - support + 0.5)
		if xmin < 0 {
			xmin = 0
		}
		xmax := int(center + support + 0.5)
		if xmax > inSize {
			xmax = inSize
		}
		xmax -= xmin
		for x := 0; x < xmax; x++ {
			w := lanczosKernel((float64(x+xmin) - center + 0.5) * inv)
			k[x] = w
			ww += w
		}
		for x := 0; x < ksize; x++ {
			v := 0.0
			if x < xmax {
				v = k[x]
				if ww != 0 {
					v /= ww
				}
			}
			if v < 0 {
				kk[xx*ksize+x] = int32(-0.5 + v*(1<<lanczosPrecision))
			} else {
				kk[xx*ksize+x] = int32(0.5 + v*(1<<lanczosPrecision))
			}
		}
		bounds[2*xx], bounds[2*xx+1] = xmin, xmax
	}
	return bounds, ksize, kk
}

func clip8(v int32) uint8 {
	v >>= lanczosPrecision
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// rgbPlanes holds an image as 8-bit RGB, three bytes per pixel.
type rgbPlanes struct {
	w, h int
	pix  []uint8
}

func toRGBPlanes(src image.Image) *rgbPlanes {
	b := src.Bounds()
	p := &rgbPlanes{w: b.Dx(), h: b.Dy(), pix: make([]uint8, 3*b.Dx()*b.Dy())}
	for y := 0; y < p.h; y++ {
		for x := 0; x < p.w; x++ {
			r, g, bl, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := 3 * (y*p.w + x)
			p.pix[i], p.pix[i+1], p.pix[i+2] = uint8(r>>8), uint8(g>>8), uint8(bl>>8)
		}
	}
	return p
}

func (p *rgbPlanes) image() *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, p.w, p.h))
	for i := 0; i < p.w*p.h; i++ {
		out.Pix[4*i], out.Pix[4*i+1], out.Pix[4*i+2], out.Pix[4*i+3] = p.pix[3*i], p.pix[3*i+1], p.pix[3*i+2], 0xFF
	}
	return out
}

// ResizeLanczos resamples the region box (x0, y0, x1, y1, in source pixels,
// fractions allowed) of src to w x h with Pillow's Lanczos filter, as
// Image.resize(size, Image.LANCZOS, box=box) does. Alpha is ignored: the
// result is opaque.
func ResizeLanczos(src image.Image, w, h int, box [4]float64) (*image.RGBA, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("scr: resize to %dx%d", w, h)
	}
	in := toRGBPlanes(src)
	fb := [4]float32{float32(box[0]), float32(box[1]), float32(box[2]), float32(box[3])}
	if fb[0] < 0 || fb[1] < 0 || fb[2] > float32(in.w) || fb[3] > float32(in.h) || fb[2]-fb[0] <= 0 || fb[3]-fb[1] <= 0 {
		return nil, fmt.Errorf("scr: resize box %v is outside the %dx%d image", box, in.w, in.h)
	}
	needH := w != in.w || fb[0] != 0 || fb[2] != float32(w)
	needV := h != in.h || fb[1] != 0 || fb[3] != float32(h)
	vb, vk, vkk := lanczosCoeffs(in.h, fb[1], fb[3], h)
	first := vb[0]
	last := vb[2*h-2] + vb[2*h-1]
	cur := in
	if needH {
		hb, hk, hkk := lanczosCoeffs(in.w, fb[0], fb[2], w)
		for i := 0; i < h; i++ {
			vb[2*i] -= first
		}
		tmp := &rgbPlanes{w: w, h: last - first, pix: make([]uint8, 3*w*(last-first))}
		for yy := 0; yy < tmp.h; yy++ {
			row := in.pix[3*(yy+first)*in.w:]
			for xx := 0; xx < w; xx++ {
				xmin, xmax := hb[2*xx], hb[2*xx+1]
				k := hkk[xx*hk:]
				s0, s1, s2 := int32(1<<(lanczosPrecision-1)), int32(1<<(lanczosPrecision-1)), int32(1<<(lanczosPrecision-1))
				for x := 0; x < xmax; x++ {
					o := 3 * (x + xmin)
					s0 += int32(row[o]) * k[x]
					s1 += int32(row[o+1]) * k[x]
					s2 += int32(row[o+2]) * k[x]
				}
				o := 3 * (yy*w + xx)
				tmp.pix[o], tmp.pix[o+1], tmp.pix[o+2] = clip8(s0), clip8(s1), clip8(s2)
			}
		}
		cur = tmp
	}
	if needV {
		out := &rgbPlanes{w: cur.w, h: h, pix: make([]uint8, 3*cur.w*h)}
		for yy := 0; yy < h; yy++ {
			ymin, ymax := vb[2*yy], vb[2*yy+1]
			k := vkk[yy*vk:]
			for xx := 0; xx < cur.w; xx++ {
				s0, s1, s2 := int32(1<<(lanczosPrecision-1)), int32(1<<(lanczosPrecision-1)), int32(1<<(lanczosPrecision-1))
				for y := 0; y < ymax; y++ {
					o := 3 * ((y+ymin)*cur.w + xx)
					s0 += int32(cur.pix[o]) * k[y]
					s1 += int32(cur.pix[o+1]) * k[y]
					s2 += int32(cur.pix[o+2]) * k[y]
				}
				o := 3 * (yy*cur.w + xx)
				out.pix[o], out.pix[o+1], out.pix[o+2] = clip8(s0), clip8(s1), clip8(s2)
			}
		}
		cur = out
	}
	return cur.image(), nil
}

// FitLanczos crops src to the aspect ratio of w x h and resizes it to w x h
// with ResizeLanczos, as Pillow's ImageOps.fit does: the crop removes the
// excess along one axis, cx and cy (0 to 1) placing it (0.5 centres the
// crop; 0 keeps the left or top edge; 1 the right or bottom).
func FitLanczos(src image.Image, w, h int, cx, cy float64) (*image.RGBA, error) {
	if cx < 0 || cx > 1 {
		cx = 0.5
	}
	if cy < 0 || cy > 1 {
		cy = 0.5
	}
	b := src.Bounds()
	lw, lh := float64(b.Dx()), float64(b.Dy())
	liveRatio := lw / lh
	outRatio := float64(w) / float64(h)
	cropW, cropH := lw, lh
	if liveRatio != outRatio {
		if liveRatio >= outRatio {
			cropW = outRatio * lh
		} else {
			cropH = lw / outRatio
		}
	}
	left := (lw - cropW) * cx
	top := (lh - cropH) * cy
	return ResizeLanczos(src, w, h, [4]float64{left, top, left + cropW, top + cropH})
}

// Trim removes n pixels from every edge of src, as the specification's
// tools remove a scan margin before fitting.
func Trim(src image.Image, n int) (image.Image, error) {
	b := src.Bounds()
	if n < 0 || 2*n >= b.Dx() || 2*n >= b.Dy() {
		return nil, fmt.Errorf("scr: cannot trim %d pixels from a %dx%d image", n, b.Dx(), b.Dy())
	}
	if n == 0 {
		return src, nil
	}
	return subImage(src, image.Rect(b.Min.X+n, b.Min.Y+n, b.Max.X-n, b.Max.Y-n)), nil
}

// Grey returns img as luminance in all three channels, computed as Pillow's
// convert("L") does (ITU-R 601: (19595 R + 38470 G + 7471 B + 32768) >> 16),
// which is how the specification makes the grey versions of its pictures.
func Grey(img image.Image) *image.RGBA {
	p := toRGBPlanes(img)
	for i := 0; i < p.w*p.h; i++ {
		r, g, b := uint32(p.pix[3*i]), uint32(p.pix[3*i+1]), uint32(p.pix[3*i+2])
		l := uint8((r*19595 + g*38470 + b*7471 + 0x8000) >> 16)
		p.pix[3*i], p.pix[3*i+1], p.pix[3*i+2] = l, l, l
	}
	return p.image()
}
