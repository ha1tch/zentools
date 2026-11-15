package zxn

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func meanColour(img image.Image) [3]float64 {
	var s [3]float64
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			s[0] += float64(r >> 8)
			s[1] += float64(g >> 8)
			s[2] += float64(bl >> 8)
		}
	}
	n := float64(b.Dx() * b.Dy())
	return [3]float64{s[0] / n, s[1] / n, s[2] / n}
}

func TestReduceScene(t *testing.T) {
	scene := loadPNG(t, "../scr/testdata/zxmode/scene.png")
	want := meanColour(scene)
	for _, n := range []int{256, 16, 2} {
		for _, d := range []Dither{DitherNone, DitherFloydSteinberg} {
			img, pal, err := Reduce(scene, ReduceOptions{Colors: n, Dither: d, Transparent: -1})
			if err != nil {
				t.Fatal(err)
			}
			if len(pal.Colors) != n || len(img.Palette) != n {
				t.Fatalf("%d colours: palette has %d entries", n, len(pal.Colors))
			}
			used := map[uint8]bool{}
			for _, v := range img.Pix {
				used[v] = true
				if int(v) >= n {
					t.Fatalf("index %d in a %d-colour image", v, n)
				}
			}
			// Floyd-Steinberg keeps the image's average colour close, given
			// enough colours to span it (two cannot).
			if d == DitherFloydSteinberg && n >= 16 {
				got := meanColour(img)
				for c := 0; c < 3; c++ {
					if math.Abs(got[c]-want[c]) > 6 {
						t.Errorf("%d colours dithered: mean channel %d is %.1f, source %.1f", n, c, got[c], want[c])
					}
				}
			}
			// The same input gives the same output.
			again, _, _ := Reduce(scene, ReduceOptions{Colors: n, Dither: d, Transparent: -1})
			for i := range img.Pix {
				if img.Pix[i] != again.Pix[i] {
					t.Fatalf("%d colours: Reduce is not deterministic", n)
				}
			}
		}
	}
}

func TestReduceExactAndTransparent(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	cols := []color.NRGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 0, 0}, {255, 0, 0, 255}}
	for x, c := range cols {
		img.SetNRGBA(x, 0, c)
		img.SetNRGBA(x, 1, cols[len(cols)-1-x])
	}
	out, pal, err := Reduce(img, ReduceOptions{Colors: 256, Transparent: 0})
	if err != nil {
		t.Fatal(err)
	}
	// Index 0 is transparent; red and green follow in first-seen order.
	if out.Pix[2] != 0 || out.Pix[0] != 1 || out.Pix[1] != 2 || out.Pix[3] != 1 {
		t.Errorf("first row indices %v", out.Pix[:4])
	}
	if pal.Colors[1] != (Color333{R: 7}) || pal.Colors[2] != (Color333{G: 7}) {
		t.Errorf("palette starts %v", pal.Colors[:3])
	}
	if _, _, _, a := out.Palette[0].RGBA(); a != 0 {
		t.Error("the transparent entry is drawn opaque")
	}
	// The default transparent index skips 0xE3 and holds its RGB332 colour.
	_, pal, _ = Reduce(img, ReduceOptions{Colors: 256, Transparent: DefaultTransparent})
	if pal.Colors[DefaultTransparent] != rgb332(DefaultTransparent) || pal.Colors[0] != (Color333{R: 7}) {
		t.Errorf("transparent entry %v, first %v", pal.Colors[DefaultTransparent], pal.Colors[0])
	}
	if _, _, err := Reduce(img, ReduceOptions{Colors: 16, Transparent: DefaultTransparent}); err == nil {
		t.Error("transparent index 0xE3 accepted in a 16-colour palette")
	}
	if _, _, err := Reduce(img, ReduceOptions{Colors: 1, Transparent: -1}); err == nil {
		t.Error("a 1-colour palette accepted")
	}
}

func TestReduceToGivenPalette(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	img.SetNRGBA(0, 0, color.NRGBA{250, 10, 10, 255}) // nearest red
	img.SetNRGBA(1, 0, color.NRGBA{0, 0, 0, 255})     // exactly black
	img.SetNRGBA(2, 0, color.NRGBA{0, 0, 0, 0})       // transparent
	pal := &Palette{Colors: []Color333{{}, {R: 7}, {G: 7}, {}}}
	out, p, err := Reduce(img, ReduceOptions{Palette: pal, Transparent: 3})
	if err != nil {
		t.Fatal(err)
	}
	if out.Pix[0] != 1 || out.Pix[1] != 0 || out.Pix[2] != 3 || len(p.Colors) != 4 || p.Colors[3] != rgb332(3) {
		t.Errorf("indices %v, palette %v", out.Pix, p.Colors)
	}
}
