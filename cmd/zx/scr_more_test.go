package main

import (
	"bytes"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ha1tch/zentools/pkg/scr"
)

var scrTD = filepath.Join("..", "..", "pkg", "scr", "testdata")

func std(name string) string { return filepath.Join(scrTD, name) }

// TestZXScr_SpecFitAndMetrics checks the specification's picture fit and
// error measures through the command line.
func TestZXScr_SpecFitAndMetrics(t *testing.T) {
	r := newRunner(t)
	// Fitting inside zx gives the picture Pillow's fit gives.
	r.run("scr", "encode", std("fit/src.png"), "-mode", "zxmode2", "-palette", "zxpal4", "-resize", "fill", "-filter", "lanczos",
		"-trim", "5", "-centering", "0.5,0.9", "-o", r.tmp("a.zm2"))
	r.run("scr", "encode", std("fit/fit_trim5_512.png"), "-mode", "zxmode2", "-palette", "zxpal4", "-o", r.tmp("b.zm2"))
	r.same(r.tmp("a.zm2"), r.tmp("b.zm2"))
	// The error measures give the numbers the spec's tools give.
	for _, c := range []struct{ metric, want string }{
		{"blur", "14.4357"}, {"light", "16.3351"}, {"ssim", "0.4041"},
	} {
		if out := r.run("scr", "compare", "-metric", c.metric, std("zxmode/scene.png"), std("zxmode/scene_zxpal4.png")); strings.TrimSpace(out) != c.want {
			t.Errorf("compare -metric %s: %q, want %s", c.metric, out, c.want)
		}
	}
	if out := r.run("scr", "compare", "-metric", "all", std("zxmode/scene.png"), r.tmp("b.zm2"), "-palette", "zxpal4"); !strings.Contains(out, "SSIM") {
		t.Errorf("compare -metric all: %s", out)
	}
	// -grey is the luminance of the fitted picture.
	grey := scr.Grey(decodePNG(t, std("zxmode/scene.png")))
	if err := writePNGFile(r.tmp("grey.png"), grey); err != nil {
		t.Fatal(err)
	}
	r.run("scr", "encode", std("zxmode/scene.png"), "-grey", "-mode", "zxmode2", "-palette", "zxpal2", "-o", r.tmp("g1.zm2"))
	r.run("scr", "encode", r.tmp("grey.png"), "-mode", "zxmode2", "-palette", "zxpal2", "-o", r.tmp("g2.zm2"))
	r.same(r.tmp("g1.zm2"), r.tmp("g2.zm2"))
	// Error diffusion lowers the error on the scene.
	r.run("scr", "encode", std("zxmode/scene.png"), "-mode", "zxmode2", "-palette", "zxpal0", "-o", r.tmp("ref.zm2"))
	r.run("scr", "encode", std("zxmode/scene.png"), "-mode", "zxmode2", "-palette", "zxpal0", "-diffuse", "0.5", "-o", r.tmp("dif.zm2"))
	ref := r.run("scr", "compare", "-metric", "blur", std("zxmode/scene.png"), r.tmp("ref.zm2"))
	dif := r.run("scr", "compare", "-metric", "blur", std("zxmode/scene.png"), r.tmp("dif.zm2"))
	if dif >= ref {
		t.Errorf("diffusion error %s is not below the reference's %s", strings.TrimSpace(dif), strings.TrimSpace(ref))
	}
}

// TestZXScr_RegistersCutPasteConvert loads registers, cuts and pastes
// between palettes, converts, flips, and reads ZXMODE files with info, crop
// and ocr.
func TestZXScr_RegistersCutPasteConvert(t *testing.T) {
	r := newRunner(t)
	// A register table with red and blue swapped.
	regs := scr.ZXPAL4.Registers()
	for i := range regs {
		regs[i] = color.RGBA{regs[i].B, regs[i].G, regs[i].R, 0xFF}
	}
	csv := r.tmp("swap.csv")
	if err := os.WriteFile(csv, scr.RegistersCSV(regs), 0644); err != nil {
		t.Fatal(err)
	}
	r.run("scr", "palette", "zxpal4", "-registers", csv, "-o", r.tmp("again.csv"))
	r.same(r.tmp("again.csv"), csv)
	scene := std("zxmode/scene.png")
	r.run("scr", "encode", scene, "-mode", "zxmode2", "-palette", "zxpal4", "-registers", csv, "-o", r.tmp("s.zm2"))
	r.run("scr", "decode", r.tmp("s.zm2"), "-palette", "zxpal4", "-registers", csv, "-o", r.tmp("s.png"))
	r.run("scr", "decode", r.tmp("s.zm2"), "-palette", "zxpal4", "-o", r.tmp("s_reset.png"))
	if bytes.Equal(mustRead(t, r.tmp("s.png")), mustRead(t, r.tmp("s_reset.png"))) {
		t.Error("decoding with and without the register table gives the same picture")
	}
	if out := r.run("info", r.tmp("s.zm2")); !strings.Contains(out, "ZXMODE2") || !strings.Contains(out, "ZXPAL3 or ZXPAL4") {
		t.Errorf("zx info .zm2: %s", out)
	}

	// Cut keeps the palette and registers; paste into a classic screen converts.
	cut := r.tmp("a.cut")
	r.run("scr", "cut", r.tmp("s.zm2"), "-palette", "zxpal4", "-registers", csv, "-cells", "4,4,8,6", "-name", "piece", "-o", cut)
	if out := r.run("scr", "ls", cut); !strings.Contains(out, "ZXPAL4, own registers") {
		t.Errorf("ls: %s", out)
	}
	if out := r.run("info", cut); !strings.Contains(out, "piece") {
		t.Errorf("zx info .cut: %s", out)
	}
	r.run("scr", "encode", scene, "-resize", "fill", "-o", r.tmp("c.scr"))
	if out := r.run("scr", "paste", cut+":piece", r.tmp("c.scr"), "-at", "8,8", "-op", "copy"); !strings.Contains(out, "converted from ZXPAL4 with its own registers to ZXPAL0") {
		t.Errorf("paste into .scr: %s", out)
	}
	// Into a .zm2 with the same registers: no conversion.
	if out := r.run("scr", "paste", cut+":piece", r.tmp("s.zm2"), "-palette", "zxpal4", "-registers", csv, "-at", "64,64", "-o", r.tmp("s2.zm2")); strings.Contains(out, "converted") {
		t.Errorf("paste into the same palette: %s", out)
	}
	// Convert the collection to ZXPAL1, and a screen to a classic one.
	r.run("scr", "convert", cut, "-to", "zxpal1", "-o", r.tmp("b.cut"))
	if out := r.run("scr", "ls", r.tmp("b.cut")); !strings.Contains(out, "color ZXPAL1") || strings.Contains(out, "own registers") {
		t.Errorf("converted ls: %s", out)
	}
	r.run("scr", "encode", scene, "-resize", "fill", "-filter", "lanczos", "-mode", "zxmode1", "-palette", "zxpal3", "-o", r.tmp("m.zm1"))
	r.run("scr", "convert", r.tmp("m.zm1"), "-palette", "zxpal3", "-to", "zxpal0", "-to-mode", "zxmode0")
	if n := len(mustRead(t, "m.scr")); n != scr.FileLen {
		t.Errorf("converted screen is %d bytes", n)
	}
	os.Remove("m.scr")

	// Flipping twice restores a screen; a .cut and an image flip too.
	r.run("scr", "flip", r.tmp("m.zm1"), "-palette", "zxpal3", "-h", "-o", r.tmp("f.zm1"))
	r.run("scr", "flip", r.tmp("f.zm1"), "-palette", "zxpal3", "-h")
	r.same(r.tmp("f.zm1"), r.tmp("m.zm1"))
	r.run("scr", "flip", cut, "-v", "-o", r.tmp("fv.cut"))
	r.run("scr", "flip", scene, "-h", "-v", "-o", r.tmp("fl.png"))
	if img := decodePNG(t, r.tmp("fl.png")); img.Bounds().Dx() != 512 {
		t.Errorf("flipped image is %v", img.Bounds())
	}
	r.fails("-h, -v", "scr", "flip", scene, "-o", r.tmp("x.png"))

	// Crop and OCR read ZXMODE screens: a classic text screen converted to
	// ZXMODE1 reads the same.
	r.run("scr", "convert", std("ocr-basic-output.scr"), "-to", "zxpal0", "-to-mode", "zxmode1", "-o", r.tmp("t.zm1"))
	a := r.run("scr", "ocr", std("ocr-basic-output.scr"))
	b := r.run("scr", "ocr", r.tmp("t.zm1"))
	if a != b {
		t.Errorf("OCR of the .zm1 differs:\n%s\nfrom the .scr:\n%s", b, a)
	}
	r.run("scr", "crop", r.tmp("t.zm1"), "-auto", "-o", r.tmp("t.png"))
	r.run("scr", "crop", r.tmp("s.zm2"), "-palette", "zxpal4", "-cells", "0,0,8,8", "-o", r.tmp("u.png"))
	if img := decodePNG(t, r.tmp("u.png")); img.Bounds().Dx() != 64 {
		t.Errorf("crop of .zm2 is %v", img.Bounds())
	}
}

// TestZXScr_BanksCellsFitRegs encodes over colour banks and other cell
// sizes, with another penalty, and fits registers, through the command line.
func TestZXScr_BanksCellsFitRegs(t *testing.T) {
	r := newRunner(t)
	scene := std("zxmode/scene.png")
	// The specification's tools drew these.
	r.run("scr", "encode", scene, "-mode", "zxmode2", "-banks", std("banks/zxpal1_banks.csv"), "-cell", "4x8", "-o", r.tmp("a.png"))
	if out := r.run("scr", "compare", r.tmp("a.png"), std("banks/c4x8_zxpal1.png")); !strings.HasPrefix(out, "0 of") {
		t.Errorf("4x8 cells: %s", out)
	}
	r.run("scr", "encode", scene, "-mode", "zxmode2", "-palette", "zxpal2", "-cell", "4", "-o", r.tmp("b.png"))
	if out := r.run("scr", "compare", r.tmp("b.png"), std("banks/c4_zxpal2.png")); !strings.HasPrefix(out, "0 of") {
		t.Errorf("4x4 cells: %s", out)
	}
	// -penalty on a palette writes a screen that draws as the bank encoding.
	r.run("scr", "encode", scene, "-mode", "zxmode2", "-palette", "zxpal3", "-penalty", "0.5", "-o", r.tmp("c.zm2"))
	if out := r.run("scr", "compare", r.tmp("c.zm2"), std("banks/c8_z64_p05.png"), "-palette", "zxpal3"); !strings.HasPrefix(out, "0 of") {
		t.Errorf("penalty 0.5: %s", out)
	}
	// A palette's bank table encodes as the palette does.
	r.run("scr", "palette", "zxpal1", "-banks", "-o", r.tmp("p1.csv"))
	r.run("scr", "encode", scene, "-mode", "zxmode2", "-palette", "zxpal1", "-o", r.tmp("d.zm2"))
	r.run("scr", "encode", scene, "-mode", "zxmode2", "-banks", r.tmp("p1.csv"), "-o", r.tmp("d.png"))
	if out := r.run("scr", "compare", r.tmp("d.zm2"), r.tmp("d.png"), "-palette", "zxpal1"); !strings.HasPrefix(out, "0 of") {
		t.Errorf("bank table of ZXPAL1: %s", out)
	}
	r.fails("give -o file.png", "scr", "encode", scene, "-mode", "zxmode2", "-cell", "4", "-o", r.tmp("x.zm2"))
	r.fails("-diffuse", "scr", "encode", scene, "-mode", "zxmode2", "-cell", "4", "-diffuse", "0.5", "-o", r.tmp("x.png"))

	// Fitted registers: the base palette plus the new colours, as one bank.
	out := r.run("scr", "fitregs", "-palette", "zxpal3", "-add", "4", "-sample", "3000", "-candidates", "200", "-subsample", "2000", "-o", r.tmp("f.csv"), scene)
	if !strings.Contains(out, "68 colours: 64 + 4") {
		t.Errorf("fitregs: %s", out)
	}
	r.run("scr", "encode", scene, "-mode", "zxmode2", "-banks", r.tmp("f.csv"), "-o", r.tmp("f.png"))
	r.fails("one-bank", "scr", "fitregs", "-palette", "zxpal1", "-add", "4", "-o", r.tmp("g.csv"), scene)
}
