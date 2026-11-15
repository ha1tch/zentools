// file: jpeg.go
//
// JPEG colour reconstruction as libjpeg-turbo does it. Go's image/jpeg leaves
// a colour JPEG as Y, Cb and Cr planes with the chroma at reduced resolution,
// and image.YCbCr's At() takes each pixel's chroma from the nearest sample.
// libjpeg-turbo (behind Pillow, and so behind the ZXMODE specification's
// tools) instead interpolates chroma with its "fancy" triangle filter and
// converts with fixed-point tables. For 4:2:0 and 4:2:2 pictures this file
// rebuilds the RGB image that way, from Go's planes, so the result differs
// from Pillow's only where the two decoders' inverse DCTs differ (by one level,
// in a few percent of samples) rather than by tens of levels at colour edges.

package scr

import "image"

// yccTables are libjpeg's jdcolor.c conversion tables (SCALEBITS 16).
var yccCrR, yccCbB, yccCrG, yccCbG [256]int32

func init() {
	fix := func(x float64) int32 { return int32(x*65536 + 0.5) }
	const half = 1 << 15
	for i := 0; i < 256; i++ {
		x := int32(i - 128)
		yccCrR[i] = (fix(1.40200)*x + half) >> 16
		yccCbB[i] = (fix(1.77200)*x + half) >> 16
		yccCrG[i] = -fix(0.71414) * x
		yccCbG[i] = -fix(0.34414)*x + half
	}
}

func clampByte(v int32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// fancyRGB rebuilds a 4:2:0 or 4:2:2 YCbCr image as libjpeg-turbo would
// output it; it returns nil for any other layout.
func fancyRGB(m *image.YCbCr) *image.RGBA {
	r := m.Rect
	w, h := r.Dx(), r.Dy()
	var vfactor int
	switch m.SubsampleRatio {
	case image.YCbCrSubsampleRatio420:
		vfactor = 2
	case image.YCbCrSubsampleRatio422:
		vfactor = 1
	default:
		return nil
	}
	if r.Min != (image.Point{}) {
		return nil
	}
	cw := (w + 1) / 2 // libjpeg's downsampled width
	ch := (h + vfactor - 1) / vfactor
	sample := func(plane []uint8, x, y int) int32 {
		if y < 0 {
			y = 0
		}
		if y >= ch {
			y = ch - 1
		}
		return int32(plane[y*m.CStride+x])
	}
	up := func(plane []uint8) []uint8 {
		out := make([]uint8, w*h)
		row := make([]int32, cw)
		for oy := 0; oy < h; oy++ {
			cy := oy / vfactor
			for x := 0; x < cw; x++ {
				if vfactor == 2 {
					// 3/4 of the nearer row, 1/4 of the further one.
					far := cy - 1
					if oy%2 == 1 {
						far = cy + 1
					}
					row[x] = sample(plane, x, cy)*3 + sample(plane, x, far)
				} else {
					row[x] = sample(plane, x, cy)
				}
			}
			o := out[oy*w:]
			put := func(x int, v int32) {
				if x < w {
					o[x] = uint8(v)
				}
			}
			if vfactor == 2 {
				if cw == 1 {
					put(0, (row[0]*4+8)>>4)
					put(1, (row[0]*4+7)>>4)
					continue
				}
				put(0, (row[0]*4+8)>>4)
				put(1, (row[0]*3+row[1]+7)>>4)
				for x := 1; x < cw-1; x++ {
					put(2*x, (row[x]*3+row[x-1]+8)>>4)
					put(2*x+1, (row[x]*3+row[x+1]+7)>>4)
				}
				l := cw - 1
				put(2*l, (row[l]*3+row[l-1]+8)>>4)
				put(2*l+1, (row[l]*4+7)>>4)
			} else {
				if cw == 1 {
					put(0, row[0])
					put(1, row[0])
					continue
				}
				put(0, row[0])
				put(1, (row[0]*3+row[1]+2)>>2)
				for x := 1; x < cw-1; x++ {
					put(2*x, (row[x]*3+row[x-1]+1)>>2)
					put(2*x+1, (row[x]*3+row[x+1]+2)>>2)
				}
				l := cw - 1
				put(2*l, (row[l]*3+row[l-1]+1)>>2)
				put(2*l+1, row[l])
			}
		}
		return out
	}
	cb, cr := up(m.Cb), up(m.Cr)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			yy := int32(m.Y[y*m.YStride+x])
			b, rr := cb[y*w+x], cr[y*w+x]
			i := 4 * (y*w + x)
			out.Pix[i] = clampByte(yy + yccCrR[rr])
			out.Pix[i+1] = clampByte(yy + ((yccCbG[b] + yccCrG[rr]) >> 16))
			out.Pix[i+2] = clampByte(yy + yccCbB[b])
			out.Pix[i+3] = 0xFF
		}
	}
	return out
}
