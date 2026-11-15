package scr

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"testing"
)

func decodeFile(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := DecodeImage(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return img
}

// TestBMP decodes BMPs at 1, 4, 8, 24 and 32 bits (one top-down, one with
// a V5 header and an alpha mask) and compares them with PNG twins.
func TestBMP(t *testing.T) {
	for _, c := range []struct {
		bmp, png string
		indexed  bool
		alpha    bool
	}{
		{"m1.bmp", "m1.png", true, false},
		{"p4_topdown.bmp", "p8.png", true, false},
		{"p8.bmp", "p8.png", true, false},
		{"rgb24.bmp", "rgb.png", false, false},
		{"rgba32.bmp", "rgba.png", false, false}, // BI_RGB: the fourth byte is not alpha
		{"argb32_v5.bmp", "rgba.png", false, true},
	} {
		b := decodeFile(t, "testdata/bmp/"+c.bmp)
		p := decodeFile(t, "testdata/bmp/"+c.png)
		if _, ok := b.(*image.Paletted); ok != c.indexed {
			t.Errorf("%s: indexed %v, want %v", c.bmp, ok, c.indexed)
		}
		if b.Bounds() != p.Bounds() {
			t.Fatalf("%s: %v, want %v", c.bmp, b.Bounds(), p.Bounds())
		}
		for y := 0; y < p.Bounds().Dy(); y++ {
			for x := 0; x < p.Bounds().Dx(); x++ {
				got := color.NRGBAModel.Convert(b.At(x, y)).(color.NRGBA)
				want := color.NRGBAModel.Convert(p.At(x, y)).(color.NRGBA)
				if want.A == 0 {
					continue // the PNG keeps no colour under full transparency
				}
				if !c.alpha {
					want.A = 0xFF
				}
				if got != want {
					t.Fatalf("%s: pixel (%d, %d) is %v, want %v", c.bmp, x, y, got, want)
				}
			}
		}
		if c.bmp == "p8.bmp" {
			pb, pp := b.(*image.Paletted), p.(*image.Paletted)
			for i := range pp.Palette {
				if pb.Palette[i] != pp.Palette[i] {
					t.Fatalf("p8.bmp: palette entry %d differs", i)
				}
			}
		}
	}
	if _, err := DecodeImage(bytes.NewReader([]byte("BM\x00\x00"))); err == nil {
		t.Error("a truncated BMP was accepted")
	}
}
