package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ha1tch/zentools/pkg/scr"
)

// TestZXScr_ZXMODEEncodeDecode encodes the reference scene in every mode and
// palette combination the modes allow, checks the file size and that decode
// renders exactly what the library renders from the same bytes.
func TestZXScr_ZXMODEEncodeDecode(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	in := filepath.Join("..", "..", "pkg", "scr", "testdata", "zxmode", "scene.png")
	for m := scr.ZXMODE0; m <= scr.ZXMODE2; m++ {
		for p := scr.ZXPAL0; p < scr.NumPalettes; p++ {
			if m == scr.ZXMODE0 && p.TwoByte() {
				continue
			}
			ext := zxExt(m)
			outPath := filepath.Join(dir, "pic_"+m.String()+"_"+p.String()+ext)
			out, err := exec.Command(zx, "scr", "encode", in, "--resize", "stretch", "--mode", m.String(),
				"--palette", p.String(), "-o", outPath).CombinedOutput()
			if err != nil {
				t.Fatalf("encode %s %s: %v\n%s", m, p, err, out)
			}
			data := mustRead(t, outPath)
			if len(data) != scr.PictureLen(m, p) {
				t.Errorf("%s %s: %d bytes, want %d", m, p, len(data), scr.PictureLen(m, p))
			}
			pngPath := strings.TrimSuffix(outPath, ext) + ".png"
			args := []string{"scr", "decode", outPath, "-o", pngPath, "--palette", p.String()}
			if m == scr.ZXMODE0 {
				args = append(args, "--mode", "zxmode0")
			}
			out, err = exec.Command(zx, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("decode %s %s: %v\n%s", m, p, err, out)
			}
			pic, err := scr.DecodePicture(data, m, p)
			if err != nil {
				t.Fatal(err)
			}
			got := decodePNG(t, pngPath)
			want := pic.Render(false)
			w, h := m.Size()
			if got.Bounds().Dx() != w || got.Bounds().Dy() != h {
				t.Errorf("%s %s: decoded %v, want %dx%d", m, p, got.Bounds(), w, h)
				continue
			}
			for y := 0; y < h; y += 7 {
				for x := 0; x < w; x += 5 {
					r1, g1, b1, _ := got.At(x, y).RGBA()
					r2, g2, b2, _ := want.At(x, y).RGBA()
					if r1 != r2 || g1 != g2 || b1 != b2 {
						t.Fatalf("%s %s: decode differs from the library at (%d, %d)", m, p, x, y)
					}
				}
			}
		}
	}
}

func TestZXScr_ZXMODECutOutputAndLs(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	in := filepath.Join("..", "..", "pkg", "scr", "testdata", "zxmode", "scene.png")
	cut := filepath.Join(dir, "pics.cut")
	for _, p := range []string{"zxpal1", "zxpal4"} {
		out, err := exec.Command(zx, "scr", "encode", in, "--mode", "zxmode2", "--palette", p,
			"--name", p, "-o", cut).CombinedOutput()
		if err != nil {
			t.Fatalf("encode to .cut: %v\n%s", err, out)
		}
	}
	out, err := exec.Command(zx, "scr", "ls", cut).CombinedOutput()
	if err != nil {
		t.Fatalf("ls: %v\n%s", err, out)
	}
	for _, want := range []string{"512x384", "color ZXPAL1", "color ZXPAL4"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("ls output lacks %q:\n%s", want, out)
		}
	}
	// The ZXMODE1 stored form and the ZCUT asset carry the same picture.
	zm1 := filepath.Join(dir, "one.zm1")
	cut1 := filepath.Join(dir, "one.cut")
	for _, o := range []string{zm1, cut1} {
		if out, err := exec.Command(zx, "scr", "encode", in, "--resize", "stretch", "--mode", "zxmode1",
			"--palette", "zxpal3", "-o", o).CombinedOutput(); err != nil {
			t.Fatalf("encode: %v\n%s", err, out)
		}
	}
	col, err := scr.DecodeCollection(mustRead(t, cut1))
	if err != nil {
		t.Fatal(err)
	}
	pic, err := scr.PictureFromAsset(&col.Assets[0], scr.ZXMODE1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pic.Bytes(), mustRead(t, zm1)) {
		t.Error("the .cut asset and the .zm1 file hold different pictures")
	}
}

func TestZXScr_ZXMODERejects(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	in := filepath.Join("..", "..", "pkg", "scr", "testdata", "zxmode", "scene.png")
	cases := [][]string{
		{"scr", "encode", in, "--mode", "zxmode0", "--palette", "zxpal4", "-o", filepath.Join(dir, "a.scr")},
		{"scr", "encode", in, "--mode", "zxmode1", "-o", filepath.Join(dir, "a.zm1")}, // 512x384 needs --resize
		{"scr", "encode", in, "--mode", "zxmode2", "-o", filepath.Join(dir, "a.scr")}, // wrong extension
		{"scr", "encode", in, "--mode", "zxmode2", "--palette", "zxpal9", "-o", filepath.Join(dir, "a.zm2")},
	}
	for _, args := range cases {
		if out, err := exec.Command(zx, args...).CombinedOutput(); err == nil {
			t.Errorf("zx %s succeeded, want an error:\n%s", strings.Join(args, " "), out)
		}
	}
	// A two-byte .zm2 decoded without --palette names the palettes to use.
	zm2 := filepath.Join(dir, "b.zm2")
	if out, err := exec.Command(zx, "scr", "encode", in, "--mode", "zxmode2", "--palette", "zxpal3", "-o", zm2).CombinedOutput(); err != nil {
		t.Fatalf("encode: %v\n%s", err, out)
	}
	out, err := exec.Command(zx, "scr", "decode", zm2).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "zxpal3 or zxpal4") {
		t.Errorf("decode of a two-byte .zm2 without --palette: %v\n%s", err, out)
	}
}

func TestZXScr_PaletteAndCompare(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "p.csv")
	if out, err := exec.Command(zx, "scr", "palette", "zxpal4", "-o", csvPath).CombinedOutput(); err != nil {
		t.Fatalf("palette: %v\n%s", err, out)
	}
	published := mustRead(t, filepath.Join("..", "..", "pkg", "scr", "testdata", "zxmode", "zxpal4_registers.csv"))
	lines := strings.Split(strings.TrimSpace(string(mustRead(t, csvPath))), "\n")
	pub := strings.Split(strings.TrimSpace(string(published)), "\n")
	if len(lines) != 113 || len(pub) != 113 {
		t.Fatalf("csv has %d lines, published %d", len(lines), len(pub))
	}
	for i := 1; i < len(lines); i++ {
		// the published table has CRLF line ends
		a, b := strings.Split(lines[i], ","), strings.Split(strings.TrimRight(pub[i], "\r"), ",")
		if strings.Join(a[2:], ",") != strings.Join(b[4:], ",") {
			t.Errorf("register %d: %s, published %s", i-1, lines[i], pub[i])
		}
	}
	pngPath := filepath.Join(dir, "p.png")
	if out, err := exec.Command(zx, "scr", "palette", "zxpal1", "-o", pngPath).CombinedOutput(); err != nil {
		t.Fatalf("palette png: %v\n%s", err, out)
	}
	if img := decodePNG(t, pngPath); img.Bounds().Dx() != 256 || img.Bounds().Dy() != 128 {
		t.Errorf("ZXPAL1 swatches are %v, want 256x128", img.Bounds())
	}

	// compare: equal images, a doubled 256x192 against 512x384, and a difference.
	in := filepath.Join("..", "..", "pkg", "scr", "testdata", "zxmode", "scene.png")
	if out, err := exec.Command(zx, "scr", "compare", in, in).CombinedOutput(); err != nil || !strings.Contains(string(out), "0 of 196608") {
		t.Errorf("compare with itself: %v\n%s", err, out)
	}
	zm1 := filepath.Join(dir, "s.zm1")
	small := filepath.Join(dir, "s.png")
	exec.Command(zx, "scr", "encode", in, "--resize", "fill", "--mode", "zxmode1", "-o", zm1).Run()
	exec.Command(zx, "scr", "decode", zm1, "-o", small).Run()
	if out, err := exec.Command(zx, "scr", "compare", small, in).CombinedOutput(); err == nil || !strings.Contains(string(out), "of 196608 pixels differ") {
		t.Errorf("compare of different pictures: %v\n%s", err, out)
	}
	if out, err := exec.Command(zx, "scr", "compare", small, small).CombinedOutput(); err != nil {
		t.Errorf("compare of a 256x192 picture with itself: %v\n%s", err, out)
	}
}
