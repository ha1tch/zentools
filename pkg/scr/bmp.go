// file: bmp.go
//
// A decoder for uncompressed Windows BMP files, registered with image.Decode
// so DecodeImage reads them. The Spectrum Next tools (gfx2next, nextbmp)
// work in 8-bit indexed BMPs, and an indexed BMP decodes to an
// image.Paletted with its palette in file order.

package scr

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
)

func init() {
	image.RegisterFormat("bmp", "BM", decodeBMP, decodeBMPConfig)
}

type bmpHeader struct {
	offset         uint32
	width, height  int
	topDown        bool
	bpp            int
	compression    uint32
	colours        int
	masks          [4]uint32 // red, green, blue, alpha for 16/32-bit bitfields
	headerSize     uint32
	paletteEntries []color.Color
}

func readBMPHeader(r io.Reader) (*bmpHeader, []byte, error) {
	var file [14]byte
	if _, err := io.ReadFull(r, file[:]); err != nil {
		return nil, nil, err
	}
	if file[0] != 'B' || file[1] != 'M' {
		return nil, nil, fmt.Errorf("bmp: not a BMP file")
	}
	h := &bmpHeader{offset: binary.LittleEndian.Uint32(file[10:])}
	var sz [4]byte
	if _, err := io.ReadFull(r, sz[:]); err != nil {
		return nil, nil, err
	}
	h.headerSize = binary.LittleEndian.Uint32(sz[:])
	if h.headerSize < 40 || h.headerSize > 1024 {
		return nil, nil, fmt.Errorf("bmp: header of %d bytes is not supported (OS/2 BMPs are not)", h.headerSize)
	}
	info := make([]byte, h.headerSize)
	copy(info, sz[:])
	if _, err := io.ReadFull(r, info[4:]); err != nil {
		return nil, nil, err
	}
	w := int32(binary.LittleEndian.Uint32(info[4:]))
	ht := int32(binary.LittleEndian.Uint32(info[8:]))
	h.bpp = int(binary.LittleEndian.Uint16(info[14:]))
	h.compression = binary.LittleEndian.Uint32(info[16:])
	h.colours = int(binary.LittleEndian.Uint32(info[32:]))
	if ht < 0 {
		h.topDown, ht = true, -ht
	}
	h.width, h.height = int(w), int(ht)
	if h.width <= 0 || h.height <= 0 || h.width > 1<<15 || h.height > 1<<15 {
		return nil, nil, fmt.Errorf("bmp: image is %dx%d", h.width, h.height)
	}
	read := 14 + int(h.headerSize)
	switch h.compression {
	case 0: // BI_RGB
		switch h.bpp {
		case 16:
			h.masks = [4]uint32{0x7C00, 0x03E0, 0x001F, 0}
		case 32:
			h.masks = [4]uint32{0xFF0000, 0xFF00, 0xFF, 0}
		}
	case 3, 6: // BI_BITFIELDS, BI_ALPHABITFIELDS
		if h.bpp != 16 && h.bpp != 32 {
			return nil, nil, fmt.Errorf("bmp: bit fields at %d bits per pixel", h.bpp)
		}
		if h.headerSize >= 52 {
			for i := 0; i < 3; i++ {
				h.masks[i] = binary.LittleEndian.Uint32(info[40+4*i:])
			}
			if h.headerSize >= 56 {
				h.masks[3] = binary.LittleEndian.Uint32(info[52:])
			}
		} else {
			var m [12]byte
			if _, err := io.ReadFull(r, m[:]); err != nil {
				return nil, nil, err
			}
			read += 12
			for i := 0; i < 3; i++ {
				h.masks[i] = binary.LittleEndian.Uint32(m[4*i:])
			}
		}
	default:
		return nil, nil, fmt.Errorf("bmp: compression %d is not supported (only uncompressed BMPs are)", h.compression)
	}
	switch h.bpp {
	case 1, 4, 8:
		n := h.colours
		if n == 0 {
			n = 1 << uint(h.bpp)
		}
		if n > 1<<uint(h.bpp) {
			return nil, nil, fmt.Errorf("bmp: %d palette entries at %d bits per pixel", n, h.bpp)
		}
		pal := make([]byte, 4*n)
		if _, err := io.ReadFull(r, pal); err != nil {
			return nil, nil, err
		}
		read += 4 * n
		for i := 0; i < n; i++ {
			h.paletteEntries = append(h.paletteEntries, color.RGBA{R: pal[4*i+2], G: pal[4*i+1], B: pal[4*i], A: 0xFF})
		}
	case 16, 24, 32:
	default:
		return nil, nil, fmt.Errorf("bmp: %d bits per pixel is not supported", h.bpp)
	}
	if int(h.offset) < read {
		return nil, nil, fmt.Errorf("bmp: pixel data at %d overlaps the header", h.offset)
	}
	skip := make([]byte, int(h.offset)-read)
	if _, err := io.ReadFull(r, skip); err != nil {
		return nil, nil, err
	}
	return h, info, nil
}

func decodeBMPConfig(r io.Reader) (image.Config, error) {
	h, _, err := readBMPHeader(r)
	if err != nil {
		return image.Config{}, err
	}
	var m color.Model = color.RGBAModel
	if h.paletteEntries != nil {
		m = color.Palette(h.paletteEntries)
	}
	return image.Config{ColorModel: m, Width: h.width, Height: h.height}, nil
}

// maskValue extracts a channel under a bit mask and scales it to 8 bits.
func maskValue(v, mask uint32) uint8 {
	if mask == 0 {
		return 0
	}
	shift := 0
	for mask&1 == 0 {
		mask >>= 1
		shift++
	}
	x := (v >> uint(shift)) & mask
	return uint8(x * 255 / mask)
}

func decodeBMP(r io.Reader) (image.Image, error) {
	h, _, err := readBMPHeader(r)
	if err != nil {
		return nil, err
	}
	stride := (h.width*h.bpp + 31) / 32 * 4
	row := make([]byte, stride)
	var pi *image.Paletted
	var ri *image.NRGBA
	rect := image.Rect(0, 0, h.width, h.height)
	if h.paletteEntries != nil {
		pi = image.NewPaletted(rect, color.Palette(h.paletteEntries))
	} else {
		ri = image.NewNRGBA(rect)
	}
	for i := 0; i < h.height; i++ {
		if _, err := io.ReadFull(r, row); err != nil {
			return nil, fmt.Errorf("bmp: pixel data: %w", err)
		}
		y := h.height - 1 - i
		if h.topDown {
			y = i
		}
		for x := 0; x < h.width; x++ {
			switch h.bpp {
			case 1, 4, 8:
				bit := x * h.bpp
				v := row[bit/8] >> uint(8-h.bpp-bit%8) & uint8(1<<uint(h.bpp)-1)
				if int(v) >= len(h.paletteEntries) {
					return nil, fmt.Errorf("bmp: pixel index %d past a %d-colour palette", v, len(h.paletteEntries))
				}
				pi.SetColorIndex(x, y, v)
			case 24:
				o := 3 * x
				ri.SetNRGBA(x, y, color.NRGBA{R: row[o+2], G: row[o+1], B: row[o], A: 0xFF})
			case 16, 32:
				var v uint32
				if h.bpp == 16 {
					v = uint32(binary.LittleEndian.Uint16(row[2*x:]))
				} else {
					v = binary.LittleEndian.Uint32(row[4*x:])
				}
				a := uint8(0xFF)
				if h.masks[3] != 0 {
					a = maskValue(v, h.masks[3])
				}
				ri.SetNRGBA(x, y, color.NRGBA{R: maskValue(v, h.masks[0]), G: maskValue(v, h.masks[1]), B: maskValue(v, h.masks[2]), A: a})
			}
		}
	}
	if pi != nil {
		return pi, nil
	}
	return ri, nil
}
