package scr

import (
	"image"
	"os"
	"testing"
)

func loadTestImage(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := DecodeImage(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// channelDiffs counts the channel values that differ between two images of
// one size, and the largest difference.
func channelDiffs(a, b image.Image) (n, max int) {
	r := a.Bounds()
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			r1, g1, b1, _ := a.At(r.Min.X+x, r.Min.Y+y).RGBA()
			r2, g2, b2, _ := b.At(b.Bounds().Min.X+x, b.Bounds().Min.Y+y).RGBA()
			for _, d := range []int{int(r1>>8) - int(r2>>8), int(g1>>8) - int(g2>>8), int(b1>>8) - int(b2>>8)} {
				if d < 0 {
					d = -d
				}
				if d != 0 {
					n++
				}
				if d > max {
					max = d
				}
			}
		}
	}
	return n, max
}

// TestLanczosMatchesPillow compares FitLanczos and ResizeLanczos with what
// Pillow 12.3 wrote (ImageOps.fit and Image.resize with Image.LANCZOS) for
// the same input: an upscale with an off-centre crop, a 2x downscale, a trim
// before an upscale, and a plain resize. Every byte must match.
func TestLanczosMatchesPillow(t *testing.T) {
	src := loadTestImage(t, "testdata/fit/src.png")
	scene := loadTestImage(t, "testdata/zxmode/scene.png")
	trimmed, err := Trim(src, 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		got  func() (*image.RGBA, error)
	}{
		{"fit_256_c02.png", func() (*image.RGBA, error) { return FitLanczos(src, 256, 192, 0.2, 0.5) }},
		{"fit_scene_256.png", func() (*image.RGBA, error) { return FitLanczos(scene, 256, 192, 0.5, 0.5) }},
		{"fit_trim5_512.png", func() (*image.RGBA, error) { return FitLanczos(trimmed, 512, 384, 0.5, 0.9) }},
		{"resize_160x120.png", func() (*image.RGBA, error) { return ResizeLanczos(scene, 160, 120, [4]float64{0, 0, 512, 384}) }},
	} {
		got, err := c.got()
		if err != nil {
			t.Fatal(err)
		}
		want := loadTestImage(t, "testdata/fit/"+c.name)
		if got.Bounds().Size() != want.Bounds().Size() {
			t.Fatalf("%s: %v, want %v", c.name, got.Bounds(), want.Bounds())
		}
		if n, max := channelDiffs(got, want); n != 0 {
			t.Errorf("%s: %d channel values differ from Pillow's (max %d)", c.name, n, max)
		}
	}
	// FitToWith reaches the same fit.
	fit, err := FitToWith(src, ResizeFill, nil, 512, 384, FitOptions{Filter: FilterLanczos, Trim: 5, Centre: true, CentreX: 0.5, CentreY: 0.9})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := channelDiffs(fit, loadTestImage(t, "testdata/fit/fit_trim5_512.png")); n != 0 {
		t.Errorf("FitToWith: %d channel values differ", n)
	}
}

// TestJPEGChromaLikeLibjpeg decodes 4:2:0 and 4:2:2 JPEGs and compares them
// with Pillow's decoding (libjpeg-turbo). Chroma is interpolated as
// libjpeg-turbo does, so only the two decoders' inverse DCTs differ: a few
// percent of values, by a few levels, where nearest-sample chroma would be
// out by tens.
func TestJPEGChromaLikeLibjpeg(t *testing.T) {
	for _, c := range []struct{ jpg, png string }{
		{"src.jpg", "src_jpg_pillow.png"},
		{"src422.jpg", "src422_jpg_pillow.png"},
	} {
		got := loadTestImage(t, "testdata/fit/"+c.jpg)
		if _, ok := got.(*image.RGBA); !ok {
			t.Fatalf("%s decoded as %T, not rebuilt", c.jpg, got)
		}
		want := loadTestImage(t, "testdata/fit/"+c.png)
		n, max := channelDiffs(got, want)
		total := 3 * want.Bounds().Dx() * want.Bounds().Dy()
		if max > 4 || n*100 > 6*total {
			t.Errorf("%s: %d of %d values differ from libjpeg-turbo's, max %d", c.jpg, n, total, max)
		}
	}
}
