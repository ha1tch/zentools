package zxn

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/ha1tch/zentools/pkg/scr"
)

func TestHiColour(t *testing.T) {
	// A standard screen is a HiColour screen whose 8x1 attributes repeat
	// down each cell: the bitmaps must match byte for byte.
	f, err := scr.Decode(func() []byte {
		d := make([]byte, scr.FileLen)
		for i := range d {
			d[i] = byte(i*37 + i>>5)
		}
		return d
	}())
	if err != nil {
		t.Fatal(err)
	}
	std := scr.Encode(f)
	h := &HiColour{}
	for y := 0; y < 192; y++ {
		for x := 0; x < 256; x++ {
			h.Pixels[y][x] = f.Ink[y][x]
		}
		for c := 0; c < 32; c++ {
			h.Attr[y][c] = f.Attr[y/8][c].Byte()
		}
	}
	data := h.Bytes()
	if !bytes.Equal(data[:TimexBitmapLen], std[:TimexBitmapLen]) {
		t.Fatal("HiColour bitmap differs from the standard screen's")
	}
	// Each attribute sits at its pixels' offset in the second screen.
	if data[TimexBitmapLen+256] != f.Attr[0][0].Byte() {
		t.Error("attribute for row 1 is not at offset 256 of the attribute screen")
	}
	back, err := DecodeHiColour(data)
	if err != nil || *back != *h {
		t.Fatalf("round trip: %v", err)
	}
	// Choosing attributes for the drawn screen gives a screen that draws
	// the same picture.
	img := scr.ToImage(f)
	again, err := HiColourFromImage(img)
	if err != nil {
		t.Fatal(err)
	}
	drawn := again.Image()
	for y := 0; y < 192; y++ {
		for x := 0; x < 256; x++ {
			if drawn.RGBAAt(x, y) != img.RGBAAt(x, y) {
				t.Fatalf("pixel (%d, %d) drawn differently", x, y)
			}
		}
	}
}

func TestHiRes(t *testing.T) {
	h := &HiRes{Ink: 1}
	h.Pixels[0][0] = true   // byte column 0: first screen, offset 0
	h.Pixels[0][8] = true   // byte column 1: second screen, offset 0
	h.Pixels[1][511] = true // byte column 63: second screen, row 1, offset 256+31
	data := h.Bytes()
	if data[0] != 0x80 || data[TimexBitmapLen] != 0x80 || data[TimexBitmapLen+256+31] != 0x01 {
		t.Fatalf("HiRes layout: % X / % X / % X", data[0], data[TimexBitmapLen], data[TimexBitmapLen+256+31])
	}
	back, err := DecodeHiRes(data, 1)
	if err != nil || *back != *h {
		t.Fatalf("round trip: %v", err)
	}
	img := image.NewPaletted(image.Rect(0, 0, 512, 192), color.Palette{color.White, color.Black})
	img.SetColorIndex(5, 7, 1)
	g, err := HiResFromImage(img, 2)
	if err != nil || !g.Pixels[7][5] || g.Pixels[0][0] {
		t.Fatalf("from image: %v", err)
	}
	if _, err := HiResFromImage(image.NewRGBA(image.Rect(0, 0, 256, 192)), 0); err == nil {
		t.Error("a 256x192 image accepted as HiRes")
	}
}

func TestLoRes(t *testing.T) {
	for _, m := range LoResModes {
		b := &Bitmap{Width: m.Width, Height: m.Height, Depth: m.Depth, Order: m.Order, Indices: make([]uint8, m.Width*m.Height)}
		for i := range b.Indices {
			b.Indices[i] = uint8(i*7) & uint8(1<<uint(m.Depth)-1)
		}
		data, err := EncodeNXI(b)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != m.Bytes() {
			t.Errorf("%s: %d bytes, want %d", m.Name, len(data), m.Bytes())
		}
		back, err := DecodeNXI(data, m.Width, m.Height, m.Depth, m.Order)
		if err != nil || !bytes.Equal(back.Indices, b.Indices) {
			t.Errorf("%s: round trip %v", m.Name, err)
		}
	}
}
