package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ha1tch/zentools/pkg/scr"
	"github.com/ha1tch/zentools/pkg/snapshot"
	"github.com/ha1tch/zentools/pkg/szx"
)

// --- test fixture helpers ----------------------------------------------------

// writeTestPNG writes a solid-colour w x h PNG to path. Using a single flat
// Spectrum-palette colour (white) means scr.FromImage has no classification
// ambiguity to resolve, so encode/decode round trips are exact and
// predictable -- exactly what a fixture needs to be, not what a real image
// needs to be.
func writeTestPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	white := color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, white)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// writeTestSCR writes a real, valid .scr file to path: solid white ink on
// black paper, so it's visually simple but structurally a genuine Screen,
// not a zero-filled stub.
func writeTestSCR(t *testing.T, path string) *scr.Screen {
	t.Helper()
	s := &scr.Screen{}
	for y := 0; y < scr.Height; y++ {
		for x := 0; x < scr.Width; x++ {
			s.Ink[y][x] = true
		}
	}
	for cy := 0; cy < scr.Rows; cy++ {
		for cx := 0; cx < scr.Cols; cx++ {
			s.Attr[cy][cx] = scr.Attribute{Ink: 7, Paper: 0} // white on black
		}
	}
	if err := os.WriteFile(path, scr.Encode(s), 0o644); err != nil {
		t.Fatal(err)
	}
	return s
}

func decodePNG(t *testing.T, path string) image.Image {
	t.Helper()
	data := mustRead(t, path)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decoding %s as PNG: %v", path, err)
	}
	return img
}

// --- scr encode ---------------------------------------------------------

func TestZXScr_EncodeDecodeRoundTrip(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "in.png")
	writeTestPNG(t, pngPath, scr.Width, scr.Height)

	scrPath := filepath.Join(dir, "out.scr")
	out, err := exec.Command(zx, "scr", "encode", pngPath, "-o", scrPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx scr encode: %v\n%s", err, out)
	}
	data := mustRead(t, scrPath)
	if len(data) != scr.FileLen {
		t.Fatalf("encoded .scr is %d bytes, want %d", len(data), scr.FileLen)
	}

	pngOut := filepath.Join(dir, "back.png")
	out, err = exec.Command(zx, "scr", "decode", scrPath, "-o", pngOut).CombinedOutput()
	if err != nil {
		t.Fatalf("zx scr decode: %v\n%s", err, out)
	}
	img := decodePNG(t, pngOut)
	if img.Bounds().Dx() != scr.Width || img.Bounds().Dy() != scr.Height {
		t.Errorf("decoded PNG is %dx%d, want %dx%d", img.Bounds().Dx(), img.Bounds().Dy(), scr.Width, scr.Height)
	}
	// A solid white source should decode back to a solid, bright pixel
	// somewhere near the middle -- confirms real content survived the
	// round trip, not just that files got written.
	r, g, b, _ := img.At(128, 96).RGBA()
	if r>>8 < 0x80 || g>>8 < 0x80 || b>>8 < 0x80 {
		t.Errorf("centre pixel = %v, want a bright colour (source was solid white)", []uint32{r >> 8, g >> 8, b >> 8})
	}
}

func TestZXScr_EncodeRequiresResizeForWrongSize(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "wrong.png")
	writeTestPNG(t, pngPath, 100, 100)

	out, err := exec.Command(zx, "scr", "encode", pngPath, "-o", filepath.Join(dir, "out.scr")).CombinedOutput()
	if err == nil {
		t.Fatal("expected an error for a non-256x192 image with no --resize, got nil")
	}
	if !strings.Contains(string(out), "resize") {
		t.Errorf("error doesn't mention resize modes, got:\n%s", out)
	}
}

func TestZXScr_EncodeResizeModes(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "wrong.png")
	writeTestPNG(t, pngPath, 100, 100)

	for _, mode := range []string{"stretch", "bestfit", "centre"} {
		outPath := filepath.Join(dir, mode+".scr")
		out, err := exec.Command(zx, "scr", "encode", pngPath, "--resize", mode, "-o", outPath).CombinedOutput()
		if err != nil {
			t.Errorf("zx scr encode --resize=%s: %v\n%s", mode, err, out)
			continue
		}
		if fi, err := os.Stat(outPath); err != nil || fi.Size() != scr.FileLen {
			t.Errorf("--resize=%s: output missing or wrong size", mode)
		}
	}
}

func TestZXScr_EncodeRejectsUnknownResizeMode(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "wrong.png")
	writeTestPNG(t, pngPath, 100, 100)

	out, err := exec.Command(zx, "scr", "encode", pngPath, "--resize", "bogus", "-o", filepath.Join(dir, "o.scr")).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error for an unknown resize mode, got nil\n%s", out)
	}
}

// --- scr crop -------------------------------------------------------------

func TestZXScr_CropCells(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	scrPath := filepath.Join(dir, "in.scr")
	writeTestSCR(t, scrPath)

	outPath := filepath.Join(dir, "crop.png")
	out, err := exec.Command(zx, "scr", "crop", "--cells", "1,2,3,4", scrPath, "-o", outPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx scr crop --cells: %v\n%s", err, out)
	}
	img := decodePNG(t, outPath)
	wantW, wantH := 3*8, 4*8
	if img.Bounds().Dx() != wantW || img.Bounds().Dy() != wantH {
		t.Errorf("cropped size = %dx%d, want %dx%d", img.Bounds().Dx(), img.Bounds().Dy(), wantW, wantH)
	}
}

func TestZXScr_CropPixels(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	scrPath := filepath.Join(dir, "in.scr")
	writeTestSCR(t, scrPath)

	outPath := filepath.Join(dir, "crop.png")
	out, err := exec.Command(zx, "scr", "crop", "--pixels", "10,20,17,13", scrPath, "-o", outPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx scr crop --pixels: %v\n%s", err, out)
	}
	img := decodePNG(t, outPath)
	if img.Bounds().Dx() != 17 || img.Bounds().Dy() != 13 {
		t.Errorf("cropped size = %dx%d, want 17x13 (pixels mode need not be cell-aligned)", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestZXScr_CropRejectsMultipleModes(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	scrPath := filepath.Join(dir, "in.scr")
	writeTestSCR(t, scrPath)

	out, err := exec.Command(zx, "scr", "crop", "--cells", "0,0,1,1", "--pixels", "0,0,8,8", scrPath, "-o", filepath.Join(dir, "o.png")).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error for --cells and --pixels together, got nil\n%s", out)
	}
}

func TestZXScr_CropBitsFlagRejectedForImageInput(t *testing.T) {
	// --bits only makes sense for .scr input; on an ordinary image it
	// should be rejected, not silently ignored.
	zx := buildZX(t)
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "in.png")
	writeTestPNG(t, pngPath, scr.Width, scr.Height)

	out, err := exec.Command(zx, "scr", "crop", "--cells", "0,0,1,1", "--bits", "0", pngPath, "-o", filepath.Join(dir, "o.png")).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error for --bits on image input, got nil\n%s", out)
	}
	if !strings.Contains(string(out), "--bits") {
		t.Errorf("error doesn't name --bits, got:\n%s", out)
	}
}

func TestZXScr_CropAutoOnImage(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	// A black background with a white square in the middle -- --auto
	// should find the square, not the whole canvas.
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{0, 0, 0, 0xFF})
		}
	}
	for y := 20; y < 40; y++ {
		for x := 20; x < 40; x++ {
			img.SetRGBA(x, y, color.RGBA{0xFF, 0xFF, 0xFF, 0xFF})
		}
	}
	pngPath := filepath.Join(dir, "in.png")
	f, _ := os.Create(pngPath)
	png.Encode(f, img)
	f.Close()

	outPath := filepath.Join(dir, "crop.png")
	out, err := exec.Command(zx, "scr", "crop", "--auto", pngPath, "-o", outPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx scr crop --auto: %v\n%s", err, out)
	}
	cropped := decodePNG(t, outPath)
	// Should be roughly the 20x20 square, not the full 64x64 canvas.
	if cropped.Bounds().Dx() >= 64 || cropped.Bounds().Dy() >= 64 {
		t.Errorf("cropped size = %dx%d, want smaller than the 64x64 canvas (auto-extent should find the square)", cropped.Bounds().Dx(), cropped.Bounds().Dy())
	}
}

// --- scr cut / paste / ls --------------------------------------------------

func TestZXScr_CutPasteLs(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	scrPath := filepath.Join(dir, "in.scr")
	writeTestSCR(t, scrPath)

	cutPath := filepath.Join(dir, "assets.cut")
	out, err := exec.Command(zx, "scr", "cut", "--cells", "0,0,2,2", "--name", "sprite1", "-o", cutPath, scrPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx scr cut: %v\n%s", err, out)
	}

	out, err = exec.Command(zx, "scr", "ls", cutPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx scr ls: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "sprite1") {
		t.Errorf("ls output doesn't mention sprite1, got:\n%s", out)
	}
	if !strings.Contains(string(out), "16x16") {
		t.Errorf("ls output doesn't show 16x16 (2 cells = 16px), got:\n%s", out)
	}

	targetPath := filepath.Join(dir, "target.scr")
	writeTestSCR(t, targetPath)
	out, err = exec.Command(zx, "scr", "paste", cutPath+":sprite1", targetPath, "--at", "32,32").CombinedOutput()
	if err != nil {
		t.Fatalf("zx scr paste: %v\n%s", err, out)
	}
	// Pasted in place (no -o given): target.scr itself should still be a
	// valid, correctly-sized .scr file afterward.
	if fi, err := os.Stat(targetPath); err != nil || fi.Size() != scr.FileLen {
		t.Errorf("target.scr after paste: missing or wrong size")
	}
}

func TestZXScr_CutRejectsDuplicateNames(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	scrPath := filepath.Join(dir, "in.scr")
	writeTestSCR(t, scrPath)
	cutPath := filepath.Join(dir, "assets.cut")

	if out, err := exec.Command(zx, "scr", "cut", "--cells", "0,0,1,1", "--name", "a", "-o", cutPath, scrPath).CombinedOutput(); err != nil {
		t.Fatalf("first cut: %v\n%s", err, out)
	}
	out, err := exec.Command(zx, "scr", "cut", "--cells", "1,1,1,1", "--name", "a", "-o", cutPath, scrPath).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error cutting a duplicate name, got nil\n%s", out)
	}
}

func TestZXScr_PasteRequiresAt(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	scrPath := filepath.Join(dir, "in.scr")
	writeTestSCR(t, scrPath)
	cutPath := filepath.Join(dir, "assets.cut")
	exec.Command(zx, "scr", "cut", "--cells", "0,0,1,1", "--name", "a", "-o", cutPath, scrPath).Run()

	out, err := exec.Command(zx, "scr", "paste", cutPath+":a", scrPath).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error for paste with no --at, got nil\n%s", out)
	}
}

// --- scr atlas --------------------------------------------------------------

func TestZXScr_Atlas(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	scrPath := filepath.Join(dir, "in.scr")
	writeTestSCR(t, scrPath)
	cutPath := filepath.Join(dir, "assets.cut")
	if out, err := exec.Command(zx, "scr", "cut", "--cells", "0,0,2,2", "--name", "a", "-o", cutPath, scrPath).CombinedOutput(); err != nil {
		t.Fatalf("cut: %v\n%s", err, out)
	}
	if out, err := exec.Command(zx, "scr", "cut", "--cells", "3,0,3,3", "--name", "b", "-o", cutPath, scrPath).CombinedOutput(); err != nil {
		t.Fatalf("cut: %v\n%s", err, out)
	}

	atlasPath := filepath.Join(dir, "sheet.png")
	out, err := exec.Command(zx, "scr", "atlas", cutPath, "-o", atlasPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx scr atlas: %v\n%s", err, out)
	}
	img := decodePNG(t, atlasPath)
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Error("atlas image has zero size")
	}
}

func TestZXScr_AtlasRejectsEmptyCollection(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	emptyCut := filepath.Join(dir, "empty.cut")
	col := &scr.Collection{}
	data, err := scr.EncodeCollection(col)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(emptyCut, data, 0o644)

	out, err := exec.Command(zx, "scr", "atlas", emptyCut, "-o", filepath.Join(dir, "o.png")).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error for an empty collection, got nil\n%s", out)
	}
}

// --- scr fromsnap -----------------------------------------------------------

func TestZXScr_FromSnap(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()

	s := &snapshot.MachineState{Model: snapshot.Model48K}
	// A real, running-state PC -- not left at its zero default. Found via
	// this test, not assumed: PC==0 is the .z80 format's own genuine,
	// documented v1/v2/v3 disambiguation signal ("zero signals v2/v3
	// with an extended header", pkg/snapshot's own z80.go comment,
	// confirmed against the real format's design, not a zentools bug).
	// A v1-encoded fixture that leaves PC at its zero default gets
	// mis-decoded as a v2/v3 header with garbage extended-header-length
	// bytes. Real captured snapshots essentially never have PC==0 (that's
	// the reset vector, not a running state), so a realistic fixture
	// sidesteps this rather than working around it.
	s.CPU.PC = 0x8000
	// Distinctive marker at the very start of the display file (0x4000)
	// so we can confirm this is really the display, not zeroed memory.
	s.Memory.RAM[5][0] = 0xAA

	cases := []struct {
		name string
		ext  string
		data func() ([]byte, error)
	}{
		{"sna", "sna", func() ([]byte, error) { return snapshot.EncodeSNA(s) }},
		{"z80", "z80", func() ([]byte, error) { return snapshot.EncodeZ80(s) }},
		{"szx", "szx", func() ([]byte, error) { return szx.Encode(s) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := c.data()
			if err != nil {
				t.Fatal(err)
			}
			inPath := filepath.Join(dir, "test."+c.ext)
			if err := os.WriteFile(inPath, data, 0o644); err != nil {
				t.Fatal(err)
			}
			outPath := filepath.Join(dir, c.name+"-out.scr")
			out, err := exec.Command(zx, "scr", "fromsnap", inPath, "-o", outPath).CombinedOutput()
			if err != nil {
				t.Fatalf("zx scr fromsnap (%s): %v\n%s", c.ext, err, out)
			}
			got := mustRead(t, outPath)
			if len(got) != scr.FileLen {
				t.Fatalf("extracted .scr is %d bytes, want %d", len(got), scr.FileLen)
			}
			if got[0] != 0xAA {
				t.Errorf("first byte = %#02x, want 0xAA (the marker written at 0x4000)", got[0])
			}
		})
	}
}

func TestZXScr_FromSnapRejectsNonSnapshot(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	tapPath := filepath.Join(dir, "in.tap")
	os.WriteFile(tapPath, sampleCodeTAP(), 0o644)

	out, err := exec.Command(zx, "scr", "fromsnap", tapPath, "-o", filepath.Join(dir, "o.scr")).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error for a non-snapshot input, got nil\n%s", out)
	}
}

// --- scr ocr ----------------------------------------------------------------

func TestZXScr_OCRRejectsGeometryFlagsForSCRInput(t *testing.T) {
	zx := buildZX(t)
	dir := t.TempDir()
	scrPath := filepath.Join(dir, "in.scr")
	writeTestSCR(t, scrPath)

	out, err := exec.Command(zx, "scr", "ocr", "--scale", "2", scrPath).CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error for --scale on .scr input, got nil\n%s", out)
	}
	if !strings.Contains(string(out), "do not apply") {
		t.Errorf("error doesn't explain why, got:\n%s", out)
	}
}

func TestZXScr_OCRSmokeOnRealSCR(t *testing.T) {
	// Not testing recognition accuracy (pkg/scr's own tests cover that) --
	// just that the CLI path (flag parsing, .scr-vs-image dispatch, output
	// handling) runs cleanly end to end on a real, valid .scr file.
	zx := buildZX(t)
	dir := t.TempDir()
	scrPath := filepath.Join(dir, "in.scr")
	writeTestSCR(t, scrPath)

	outPath := filepath.Join(dir, "text.txt")
	out, err := exec.Command(zx, "scr", "ocr", scrPath, "-o", outPath).CombinedOutput()
	if err != nil {
		t.Fatalf("zx scr ocr: %v\n%s", err, out)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Error("no output text file produced")
	}
}

// --- CLI-only helper functions (unique to this layer, not delegated to pkg/scr) --

func TestParseQuad(t *testing.T) {
	x, y, w, h, err := parseQuad("1, 2,3 ,4")
	if err != nil || x != 1 || y != 2 || w != 3 || h != 4 {
		t.Errorf("parseQuad = %d,%d,%d,%d err=%v, want 1,2,3,4 nil", x, y, w, h, err)
	}
	if _, _, _, _, err := parseQuad("1,2,3"); err == nil {
		t.Error("expected an error for only 3 values, got nil")
	}
	if _, _, _, _, err := parseQuad("1,2,x,4"); err == nil {
		t.Error("expected an error for a non-integer value, got nil")
	}
}

func TestParsePair(t *testing.T) {
	x, y, err := parsePair(" 10, 20 ")
	if err != nil || x != 10 || y != 20 {
		t.Errorf("parsePair = %d,%d err=%v, want 10,20 nil", x, y, err)
	}
	if _, _, err := parsePair("10"); err == nil {
		t.Error("expected an error for a single value, got nil")
	}
}

func TestParseResizeMode(t *testing.T) {
	cases := map[string]scr.ResizeMode{
		"":         scr.ResizeNone,
		"stretch":  scr.ResizeStretch,
		"bestfit":  scr.ResizeBestFit,
		"best-fit": scr.ResizeBestFit,
		"centre":   scr.ResizeCentre,
		"center":   scr.ResizeCentre,
		"CENTRE":   scr.ResizeCentre,
	}
	for input, want := range cases {
		got, err := parseResizeMode(input)
		if err != nil || got != want {
			t.Errorf("parseResizeMode(%q) = %v, %v; want %v, nil", input, got, err, want)
		}
	}
	if _, err := parseResizeMode("bogus"); err == nil {
		t.Error("expected an error for an unknown mode, got nil")
	}
}

func TestIsPNG(t *testing.T) {
	if !isPNG("foo.png") || !isPNG("foo.PNG") {
		t.Error("isPNG should accept .png case-insensitively")
	}
	if isPNG("foo.jpg") {
		t.Error("isPNG should reject non-png extensions")
	}
}

func TestScrOutBase(t *testing.T) {
	if got := scrOutBase("/a/b/game.scr"); got != "game" {
		t.Errorf("scrOutBase = %q, want game", got)
	}
}

func TestSpecHasField(t *testing.T) {
	if !specHasField("ink:white; paper:blue", "ink") {
		t.Error("specHasField should find ink")
	}
	if specHasField("ink:white", "paper") {
		t.Error("specHasField should not find paper when absent")
	}
}

func TestNormaliseOptionalValue(t *testing.T) {
	got := normaliseOptionalValue([]string{"--bitmap-only", "x.scr"}, "bitmap-only")
	if got[0] != "--bitmap-only=" {
		t.Errorf("got[0] = %q, want --bitmap-only=", got[0])
	}
	// Already-valued form should pass through unchanged.
	got = normaliseOptionalValue([]string{"--bitmap-only=ink:red", "x.scr"}, "bitmap-only")
	if got[0] != "--bitmap-only=ink:red" {
		t.Errorf("got[0] = %q, want unchanged", got[0])
	}
}

func TestBitmapColours(t *testing.T) {
	ink, paper, transparent, err := bitmapColours(scrFlagUnset)
	if err != nil || !transparent {
		t.Errorf("bare bitmap-only: transparent=%v err=%v, want true nil", transparent, err)
	}
	_ = ink
	_ = paper

	ink2, paper2, transparent2, err := bitmapColours("ink:red; paper:blue")
	if err != nil {
		t.Fatal(err)
	}
	if transparent2 {
		t.Error("paper explicitly named should not be transparent")
	}
	r, g, b, _ := ink2.RGBA()
	if r>>8 < g>>8 { // red should dominate green for a red ink
		t.Errorf("ink colour doesn't look red: r=%d g=%d b=%d", r>>8, g>>8, b>>8)
	}
	_ = paper2
}
