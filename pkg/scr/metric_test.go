package scr

import (
	"image"
	"math"
	"testing"
)

// TestMetricsMatchSpecTools compares the error measures with what the
// specification's Python tools give for the same pictures:
// zxcommon.blurred_rmse at sigma 1.5 and 0.75, and scikit-image's
// structural_similarity, on the reference scene and its ZXPAL0, 2 and 4
// encodings. The blur itself must give Pillow's bytes.
func TestMetricsMatchSpecTools(t *testing.T) {
	scene := loadTestImage(t, "testdata/zxmode/scene.png")
	for _, c := range []struct {
		file      string
		e15, e075 float64
		ssim      float64
	}{
		{"scene_zxpal0.png", 28.58318967199229, 33.735675047964286, -1},
		{"scene_zxpal2.png", 26.412605270269836, 30.32608951806842, -1},
		{"scene_zxpal4.png", 14.435693669130846, 16.335055513479965, 0.40406251721851644},
	} {
		img := loadTestImage(t, "testdata/zxmode/"+c.file)
		for _, m := range []struct {
			sigma, want float64
		}{{1.5, c.e15}, {0.75, c.e075}} {
			got, err := BlurredRMSE(scene, img, m.sigma)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-m.want) > 1e-9 {
				t.Errorf("%s: blurred RMSE at %.2f is %.12f, the tools give %.12f", c.file, m.sigma, got, m.want)
			}
		}
		if c.ssim >= 0 {
			got, err := SSIM(scene, img)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-c.ssim) > 1e-9 {
				t.Errorf("%s: SSIM %.12f, scikit-image gives %.12f", c.file, got, c.ssim)
			}
		}
	}
	// The red channel blurred at 1.5, as Pillow wrote it.
	ch, w, h := channels(scene)
	got := BlurPillow(ch[0], w, h, 1.5)
	want := loadTestImage(t, "testdata/fit/scene_red_blur15.png").(*image.Gray)
	for i := range got {
		if got[i] != want.Pix[i] {
			t.Fatalf("blurred sample %d is %d, Pillow's %d", i, got[i], want.Pix[i])
		}
	}
	if e, _ := BlurredRMSE(scene, scene, 1.5); e != 0 {
		t.Errorf("a picture against itself: %v", e)
	}
}
