package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// zxRunner runs the built zx and compares files.
type zxRunner struct {
	t   *testing.T
	zx  string
	dir string
}

func newRunner(t *testing.T) *zxRunner {
	return &zxRunner{t: t, zx: buildZX(t), dir: t.TempDir()}
}

func (r *zxRunner) run(args ...string) string {
	r.t.Helper()
	out, err := exec.Command(r.zx, args...).CombinedOutput()
	if err != nil {
		r.t.Fatalf("zx %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func (r *zxRunner) fails(want string, args ...string) {
	r.t.Helper()
	out, err := exec.Command(r.zx, args...).CombinedOutput()
	if err == nil || !strings.Contains(string(out), want) {
		r.t.Errorf("zx %s: want a failure mentioning %q, got %v\n%s", strings.Join(args, " "), want, err, out)
	}
}

func (r *zxRunner) same(got, want string) {
	r.t.Helper()
	if !bytes.Equal(mustRead(r.t, got), mustRead(r.t, want)) {
		r.t.Errorf("%s differs from %s", filepath.Base(got), filepath.Base(want))
	}
}

func (r *zxRunner) tmp(name string) string { return filepath.Join(r.dir, name) }

var zxnTD = filepath.Join("..", "..", "pkg", "zxn", "testdata")

func td(name string) string { return filepath.Join(zxnTD, name) }

// TestZXNext_EncodeMatchesTools runs `zx next` on the pkg/zxn fixtures and
// requires the bytes gfx2next and nextbmp wrote for the same images.
func TestZXNext_EncodeMatchesTools(t *testing.T) {
	r := newRunner(t)
	r.run("next", "sprite", "encode", td("s8.png"), "-o", r.tmp("s8.spr"), "-palette", r.tmp("s8.nxp"))
	r.same(r.tmp("s8.spr"), td("s8.spr"))
	r.same(r.tmp("s8.nxp"), td("s8_round.nxp"))
	r.run("next", "sprite", "encode", td("s4.png"), "-depth", "4", "-o", r.tmp("s4.spr"), "-palette", r.tmp("s4.nxp"))
	r.same(r.tmp("s4.spr"), td("s4.spr"))
	r.same(r.tmp("s4.nxp"), td("s4.nxp"))
	// The same from the BMP gfx2next read.
	r.run("next", "sprite", "encode", td("s4.bmp"), "-depth", "4", "-o", r.tmp("s4b.spr"))
	r.same(r.tmp("s4b.spr"), td("s4.spr"))
	for _, m := range []string{"floor", "ceil"} {
		out := r.tmp(m + ".nxp")
		r.run("next", "palette", "encode", td("s8.png"), "-round", m, "-o", out)
		r.same(out, td("s8_"+m+".nxp"))
	}
	// Layouts, the standard palette and minimising, with gfx2next's
	// default (distance) rounding.
	r.run("next", "palette", "encode", td("s8.png"), "-round", "distance", "-layout", "rgb332", "-o", r.tmp("332.nxp"))
	r.same(r.tmp("332.nxp"), td("s8_rgb332.nxp"))
	r.run("next", "palette", "encode", td("s8.png"), "-round", "floor", "-layout", "gbr444", "-o", r.tmp("444.nxp"))
	r.same(r.tmp("444.nxp"), td("s8_gbr444_floor.nxp"))
	r.run("next", "palette", "std", "-o", r.tmp("std.nxp"))
	r.same(r.tmp("std.nxp"), td("s8_std.nxp"))
	r.run("next", "sprite", "encode", td("s8.png"), "-std", "-round", "distance", "-o", r.tmp("std.spr"))
	r.same(r.tmp("std.spr"), td("s8_std.spr"))
	r.run("next", "sprite", "encode", td("s8.png"), "-min", "-round", "distance", "-o", r.tmp("min.spr"), "-palette", r.tmp("min.nxp"))
	r.same(r.tmp("min.spr"), td("s8_min.spr"))
	r.same(r.tmp("min.nxp"), td("s8_min.nxp"))
	r.run("next", "sprite", "encode", td("s4.png"), "-depth", "4", "-min", "-round", "distance", "-o", r.tmp("min4.spr"), "-palette", r.tmp("min4.nxp"))
	r.same(r.tmp("min4.spr"), td("s4_min.spr"))
	r.same(r.tmp("min4.nxp"), td("s4_min.nxp"))
	// Converting a palette between layouts.
	r.run("next", "palette", "convert", td("s8_round.nxp"), "-to", "rgb332", "-o", r.tmp("conv.nxp"))
	if n := len(mustRead(t, r.tmp("conv.nxp"))); n != 256 {
		t.Errorf("converted palette is %d bytes, want 256", n)
	}
	if out := r.run("next", "palette", "info", "-layout", "bgr222", td("s4_bgr222.nxp")); !strings.Contains(out, "16 colours (bgr222)") {
		t.Errorf("palette info: %s", out)
	}

	// decode draws a sheet of the right size, or one file per pattern.
	sheet := r.tmp("sheet.png")
	r.run("next", "sprite", "decode", td("s8.spr"), "-palette", td("s8_round.nxp"), "-per-row", "4", "-transparent", "0xE3", "-o", sheet)
	if img := decodePNG(t, sheet); img.Bounds().Dx() != 64 || img.Bounds().Dy() != 32 {
		t.Errorf("sheet is %v, want 64x32", img.Bounds())
	}
	r.run("next", "sprite", "decode", td("s8.spr"), "-range", "2-4", "-split", r.tmp("split"))
	if names, _ := filepath.Glob(filepath.Join(r.dir, "split", "*.png")); len(names) != 3 || filepath.Base(names[0]) != "002.png" {
		t.Errorf("split wrote %v", names)
	}
	if out := r.run("info", td("s4.nxp")); !strings.Contains(out, "16 colours") {
		t.Errorf("zx info .nxp: %s", out)
	}
	if out := r.run("next", "sprite", "info", "-depth", "4", td("s4.spr")); !strings.Contains(out, "3 patterns") {
		t.Errorf("sprite info: %s", out)
	}
	// Deduplication and padding.
	if out := r.run("next", "sprite", "encode", td("t8.png"), "-dedup", "-pad", "10", "-transparent", "0xE3", "-o", r.tmp("d.spr")); !strings.Contains(out, "10 patterns") {
		t.Errorf("dedup and pad: %s", out)
	}

	// A true-colour image is reduced, or mapped to a palette with -match.
	scene := filepath.Join("..", "..", "pkg", "scr", "testdata", "zxmode", "scene.png")
	r.run("next", "sprite", "encode", scene, "-dither", "fs", "-colors", "64", "-o", r.tmp("x.spr"), "-palette", r.tmp("x.nxp"))
	if n := len(mustRead(t, r.tmp("x.nxp"))); n != 128 {
		t.Errorf("a 64-colour palette is %d bytes, want 128", n)
	}
	r.run("next", "sprite", "encode", scene, "-o", r.tmp("y.spr"), "-match", r.tmp("x.nxp"), "-round", "nearest", "-dither", "none")
	r.fails("-min", "next", "sprite", "encode", scene, "-o", r.tmp("z.spr"), "-min", "-transparent", "0")
}

func TestZXNext_BitmapsTilesAttrs(t *testing.T) {
	r := newRunner(t)
	r.run("next", "bitmap", "encode", td("l2_256.png"), "-mode", "256x192", "-embed", "-o", r.tmp("a.nxi"))
	r.same(r.tmp("a.nxi"), td("l2_256_embed.nxi"))
	r.run("next", "bitmap", "encode", td("l2_320.png"), "-mode", "320x256", "-o", r.tmp("b.nxi"), "-palette", r.tmp("b.nxp"))
	r.same(r.tmp("b.nxi"), td("l2_320.nxi"))
	r.same(r.tmp("b.nxp"), td("l2_320.nxp"))
	r.run("next", "bitmap", "encode", td("l2_640.png"), "-mode", "640x256", "-o", r.tmp("c.nxi"), "-palette", r.tmp("c.nxp"))
	r.same(r.tmp("c.nxi"), td("l2_640.nxi"))
	r.same(r.tmp("c.nxp"), td("l2_640.nxp"))
	if out := r.run("info", td("l2_256_embed.nxi")); !strings.Contains(out, "256x192") || !strings.Contains(out, "512 bytes") {
		t.Errorf("zx info .nxi: %s", out)
	}
	r.run("next", "bitmap", "decode", td("l2_640.nxi"), "-mode", "640x256", "-palette", td("l2_640.nxp"), "-o", r.tmp("c.png"))
	if img := decodePNG(t, r.tmp("c.png")); img.Bounds().Dx() != 640 {
		t.Errorf("640x256 decodes to %v", img.Bounds())
	}
	// A true-colour picture resized into LoRes.
	scene := filepath.Join("..", "..", "pkg", "scr", "testdata", "zxmode", "scene.png")
	r.run("next", "bitmap", "encode", scene, "-mode", "lores", "-resize", "fill", "-o", r.tmp("l.slr"))
	if n := len(mustRead(t, r.tmp("l.slr"))); n != 12288 {
		t.Errorf("LoRes is %d bytes", n)
	}
	r.fails("-resize", "next", "bitmap", "encode", scene, "-mode", "lores", "-o", r.tmp("m.slr"))
	// Timex screens.
	r.run("next", "timex", "encode", scene, "-mode", "hicolour", "-resize", "fill", "-o", r.tmp("s.shc"))
	r.run("next", "timex", "decode", r.tmp("s.shc"), "-mode", "hicolour", "-o", r.tmp("s.png"))
	r.run("next", "timex", "encode", scene, "-mode", "hires", "-resize", "stretch", "-ink", "1", "-o", r.tmp("s.shr"))
	if n := len(mustRead(t, r.tmp("s.shr"))); n != 12288 {
		t.Errorf("HiRes is %d bytes", n)
	}

	// Tiles, as gfx2next cut them.
	tiles := filepath.Join(zxnTD, "tiles_rot16y")
	r.run("next", "tiles", "encode", td("t8.png"), "-dedup", "rotate", "-map16", "-map-columns", "-offset", "3", "-pal-offset", "1", "-o", r.tmp("rot"))
	r.same(r.tmp("rot.nxt"), tiles+".nxt")
	r.same(r.tmp("rot.nxm"), tiles+".nxm")
	blk := filepath.Join(zxnTD, "tiles_blk")
	r.run("next", "tiles", "encode", td("t8.png"), "-dedup", "repeat", "-block", "2x2", "-block-dedup", "-o", r.tmp("blk"))
	r.same(r.tmp("blk.nxb"), blk+".nxb")
	r.same(r.tmp("blk.nxm"), blk+".nxm")
	r.run("next", "tiles", "encode", td("t1.png"), "-depth", "1", "-dedup", "repeat", "-o", r.tmp("one"))
	r.same(r.tmp("one.nxt"), filepath.Join(zxnTD, "tiles_one.nxt"))
	r.run("next", "tiles", "decode", r.tmp("rot.nxt"), "-map", r.tmp("rot.nxm"), "-map-size", "8x4", "-map16", "-map-columns", "-offset", "3", "-o", r.tmp("map.png"))
	if img := decodePNG(t, r.tmp("map.png")); img.Bounds().Dx() != 64 || img.Bounds().Dy() != 32 {
		t.Errorf("map drawn %v, want 64x32", img.Bounds())
	}
	r.fails("16-bit", "next", "tiles", "encode", td("t8.png"), "-dedup", "mirror", "-o", r.tmp("bad"))

	// Sprite attributes through JSON.
	js := r.tmp("a.json")
	if err := os.WriteFile(js, []byte(`[{"x": 300, "y": 20, "visible": true, "pattern": 3, "extended": true, "unified": true},
 {"x": -16, "y": 0, "visible": true, "relative": true, "extended": true, "relative_pattern": true}]`), 0644); err != nil {
		t.Fatal(err)
	}
	r.run("next", "attrs", "encode", js, "-o", r.tmp("a.bin"))
	if got := mustRead(t, r.tmp("a.bin")); !bytes.Equal(got, []byte{0x2C, 20, 0x01, 0xC3, 0x20, 0xF0, 0, 0, 0xC0, 0x41}) {
		t.Errorf("attrs % X", got)
	}
	r.run("next", "attrs", "decode", r.tmp("a.bin"), "-o", r.tmp("b.json"))
	r.run("next", "attrs", "encode", r.tmp("b.json"), "-o", r.tmp("b.bin"))
	r.same(r.tmp("b.bin"), r.tmp("a.bin"))
}

// TestZXNext_NexAndZX0 unpacks the sjasmplus NEX fixtures and builds them
// back from the parts, and checks zx zx0 against the reference compressor.
func TestZXNext_NexAndZX0(t *testing.T) {
	r := newRunner(t)
	nexTD := filepath.Join("..", "..", "pkg", "nex", "testdata")
	for _, name := range []string{"v12_l2", "v13_320", "scr_ula", "shr", "tile", "lores"} {
		gz, err := os.Open(filepath.Join(nexTD, name+".nex.gz"))
		if err != nil {
			t.Fatal(err)
		}
		z, _ := gzip.NewReader(gz)
		data, _ := io.ReadAll(z)
		gz.Close()
		in := r.tmp(name + ".nex")
		if err := os.WriteFile(in, data, 0644); err != nil {
			t.Fatal(err)
		}
		if out := r.run("info", in); !strings.Contains(out, "NEX V1.") {
			t.Errorf("zx info %s: %s", name, out)
		}
		dir := r.tmp(name)
		r.run("next", "nex", "extract", in, "-o", dir)
		args := []string{"next", "nex", "build", "-header", filepath.Join(dir, "header.json"), "-o", r.tmp(name + "_b.nex")}
		parts, _ := filepath.Glob(filepath.Join(dir, "*"))
		for _, p := range parts {
			b := filepath.Base(p)
			switch {
			case strings.HasPrefix(b, "bank_"):
				n := strings.TrimLeft(strings.TrimSuffix(strings.TrimPrefix(b, "bank_"), ".bin"), "0")
				if n == "" {
					n = "0"
				}
				args = append(args, "-bank", n+"="+p)
			case strings.HasPrefix(b, "screen_"):
				args = append(args, "-screen", strings.TrimSuffix(strings.TrimPrefix(b, "screen_"), filepath.Ext(b))+"="+p)
			case b == "palette.nxp":
				args = append(args, "-palette", p)
			case b == "copper.bin":
				args = append(args, "-copper", p)
			case b == "after.bin":
				args = append(args, "-append", p)
			}
		}
		r.run(args...)
		got := mustRead(t, r.tmp(name+"_b.nex"))
		if len(got) == len(data) && got[143] != 0 {
			copy(got[508:], data[508:512]) // sjasmplus's stale checksum
		}
		if !bytes.Equal(got, data) {
			t.Errorf("%s: rebuilt file differs (%d bytes, sjasmplus %d)", name, len(got), len(data))
		}
	}
	// A program loaded at an address lands in the right bank.
	code := r.tmp("code.bin")
	if err := os.WriteFile(code, []byte{0x18, 0xFE}, 0644); err != nil {
		t.Fatal(err)
	}
	r.run("next", "nex", "build", "-bin", "0x8000="+code, "-pc", "0x8000", "-sp", "0xFF00", "-o", r.tmp("p.nex"))
	if out := r.run("next", "nex", "info", r.tmp("p.nex")); !strings.Contains(out, "1 banks: 2") || !strings.Contains(out, "start 8000") {
		t.Errorf("nex info: %s", out)
	}

	zx0TD := filepath.Join("..", "..", "pkg", "zx0", "testdata")
	r.run("zx0", "-classic", filepath.Join(zx0TD, "a.bin"), "-o", r.tmp("a.zx0"))
	r.same(r.tmp("a.zx0"), filepath.Join(zx0TD, "a.v1"))
	r.run("zx0", "-d", filepath.Join(zx0TD, "c.v2"), "-o", r.tmp("c.out"))
	r.same(r.tmp("c.out"), filepath.Join(zx0TD, "c.bin"))
}
