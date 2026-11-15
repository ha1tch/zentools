package scr

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
	"testing"
)

func TestPaletteRegisters(t *testing.T) {
	// Register and distinct-colour counts from Part 3, sections 7 to 11.
	want := []struct{ regs, distinct int }{{16, 15}, {32, 29}, {32, 29}, {64, 59}, {112, 107}}
	for p := ZXPAL0; p < NumPalettes; p++ {
		regs := p.Registers()
		seen := map[color.RGBA]bool{}
		for _, c := range regs {
			seen[c] = true
		}
		if len(regs) != want[p].regs || len(seen) != want[p].distinct || p.Last() != len(regs)-1 {
			t.Errorf("%s: %d registers, %d distinct, last %d; want %d, %d", p, len(regs), len(seen), p.Last(), want[p].regs, want[p].distinct)
		}
	}
	// ZXPAL0 matches the classic palette used by the rest of the package.
	regs := ZXPAL0.Registers()
	for i := 0; i < 16; i++ {
		if regs[i] != paletteColor(uint8(i&7), i >= 8) {
			t.Errorf("ZXPAL0 register %d = %v, classic %v", i, regs[i], paletteColor(uint8(i&7), i >= 8))
		}
	}
}

func TestZXPAL4PublishedTable(t *testing.T) {
	f, err := os.Open("testdata/zxmode/zxpal4_registers.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	regs := ZXPAL4.Registers()
	if len(rows)-1 != len(regs) {
		t.Fatalf("published table has %d registers, rule gives %d", len(rows)-1, len(regs))
	}
	for i, row := range rows[1:] {
		var c [3]int
		for k := 0; k < 3; k++ {
			c[k], _ = strconv.Atoi(row[4+k])
		}
		if got := regs[i]; int(got.R) != c[0] || int(got.G) != c[1] || int(got.B) != c[2] {
			t.Errorf("register %d: rule %v, published %v", i, got, c)
		}
	}
}

func TestCellRegisters(t *testing.T) {
	cases := []struct {
		p          Palette
		a, b       byte
		flash      bool
		ink, paper int
	}{
		{ZXPAL0, 0x47, 0, false, 15, 8},      // bright, paper 0, ink 7
		{ZXPAL0, 0x8A, 0, true, 1, 2},        // flash swaps in the second phase
		{ZXPAL0, 0x8A, 0, false, 2, 1},       //
		{ZXPAL1, 0xC5, 0, false, 29, 24},     // bank 3: odd greys
		{ZXPAL2, 0x8A, 0, true, 18, 17},      // bit 7 is a bank bit, not flash
		{ZXPAL3, 63, 0x80 | 5, false, 63, 5}, // bit 7 ignored
		{ZXPAL3, 64, 70, false, 0, 0},        // beyond the last register: register 0
		{ZXPAL4, 111, 112, false, 111, 0},    //
	}
	for _, c := range cases {
		ink, paper := c.p.CellRegisters(c.a, c.b, c.flash)
		if ink != c.ink || paper != c.paper {
			t.Errorf("%s %02x %02x flash %v: ink %d paper %d, want %d %d", c.p, c.a, c.b, c.flash, ink, paper, c.ink, c.paper)
		}
	}
}

func TestPictureLayout(t *testing.T) {
	// One set pixel and one attribute per picture, checked at the offsets Part 2 gives.
	type probe struct {
		mode        Mode
		pal         Palette
		x, y        int
		bitmap, ink int // byte offsets
		paper       int // -1 for one-byte palettes
		length      int
	}
	probes := []probe{
		{ZXMODE1, ZXPAL0, 13, 100, 32*100 + 1, 6144 + 32*12 + 1, -1, 6912},
		{ZXMODE1, ZXPAL3, 13, 100, 32*100 + 1, 6144 + 32*12 + 1, 6912 + 32*12 + 1, 7680},
		{ZXMODE2, ZXPAL2, 300, 50, 64*50 + 37, 12288 + 64*6 + 37, -1, 27648},
		// row 250 is row 58 of the bottom half-screen page, which starts at 15,360
		{ZXMODE2, ZXPAL4, 300, 250, 15360 + 64*58 + 37, 15360 + 12288 + 64*7 + 37, 15360 + 13824 + 64*7 + 37, 30720},
	}
	for _, pr := range probes {
		p, err := NewPicture(pr.mode, pr.pal)
		if err != nil {
			t.Fatal(err)
		}
		p.Ink[pr.y][pr.x] = true
		p.Attr[pr.y/8][pr.x/8] = CellAttr{A: 0x2A, B: 0x15}
		data := p.Bytes()
		if len(data) != pr.length || PictureLen(pr.mode, pr.pal) != pr.length {
			t.Errorf("%s %s: %d bytes, want %d", pr.mode, pr.pal, len(data), pr.length)
			continue
		}
		want := make([]byte, pr.length)
		want[pr.bitmap] = 0x80 >> uint(pr.x%8)
		want[pr.ink] = 0x2A
		if pr.paper >= 0 {
			want[pr.paper] = 0x15
		}
		if !bytes.Equal(data, want) {
			for i := range data {
				if data[i] != want[i] {
					t.Errorf("%s %s: byte %d is %02x, want %02x", pr.mode, pr.pal, i, data[i], want[i])
				}
			}
		}
		back, err := DecodePicture(data, pr.mode, pr.pal)
		if err != nil {
			t.Fatal(err)
		}
		if !back.Ink[pr.y][pr.x] || back.Attr[pr.y/8][pr.x/8].A != 0x2A {
			t.Errorf("%s %s: round trip lost the pixel or attribute", pr.mode, pr.pal)
		}
	}
	if _, err := NewPicture(ZXMODE0, ZXPAL3); err == nil {
		t.Error("ZXMODE0 accepted a two-byte palette")
	}
}

func TestZXMODE0MatchesSCR(t *testing.T) {
	p, _ := NewPicture(ZXMODE0, ZXPAL1)
	p.Ink[77][201] = true
	p.Attr[9][25] = CellAttr{A: 0xC5}
	data := p.Bytes()
	s, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Ink[77][201] || s.Attr[9][25].Byte() != 0xC5 {
		t.Error("ZXMODE0 bytes are not a classic SCR")
	}
}

func TestZCUTVersions(t *testing.T) {
	// A ZXPAL0 collection stays version 1.
	p0, _ := NewPicture(ZXMODE1, ZXPAL0)
	data, err := EncodeCollection(&Collection{Assets: []Asset{*p0.Asset("classic")}})
	if err != nil {
		t.Fatal(err)
	}
	if data[4] != 1 {
		t.Errorf("ZXPAL0 collection written as version %d", data[4])
	}
	// Any other palette makes it version 2, and survives a round trip.
	for pal := ZXPAL1; pal < NumPalettes; pal++ {
		p, _ := NewPicture(ZXMODE2, pal)
		p.Ink[383][511] = true
		p.Attr[47][63] = CellAttr{A: 0x6F, B: 0x21}
		if !pal.TwoByte() {
			p.Attr[47][63].B = 0
		}
		data, err := EncodeCollection(&Collection{Assets: []Asset{*p0.Asset("a"), *p.Asset("b")}})
		if err != nil {
			t.Fatal(err)
		}
		if data[4] != 2 {
			t.Errorf("%s collection written as version %d", pal, data[4])
		}
		col, err := DecodeCollection(data)
		if err != nil {
			t.Fatalf("%s: %v", pal, err)
		}
		if col.Assets[0].Palette != ZXPAL0 || col.Assets[1].Palette != pal {
			t.Errorf("%s: palettes read back as %s, %s", pal, col.Assets[0].Palette, col.Assets[1].Palette)
		}
		back, err := PictureFromAsset(&col.Assets[1], ZXMODE2)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(back.Bytes(), p.Bytes()) {
			t.Errorf("%s: picture changed through ZCUT", pal)
		}
	}
	// Version 1 cannot carry two-byte attributes.
	p3, _ := NewPicture(ZXMODE1, ZXPAL3)
	data, _ = EncodeCollection(&Collection{Assets: []Asset{*p3.Asset("x")}})
	data[4] = 1
	if _, err := DecodeCollection(data); err == nil {
		t.Error("version 1 collection with two-byte attributes accepted")
	}
}

func loadPNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// TestReferenceEncoder checks the encoder against pictures rendered by the
// specification's Python reference (tools/zxfast.py) from the same input: a
// synthetic scene drawn by the specification's tools, every palette, pixel for
// pixel.
func TestReferenceEncoder(t *testing.T) {
	in := loadPNG(t, "testdata/zxmode/scene.png")
	for pal := ZXPAL0; pal < NumPalettes; pal++ {
		pic, err := EncodePicture(in, ZXMODE2, pal)
		if err != nil {
			t.Fatal(err)
		}
		got := pic.Render(false)
		want := loadPNG(t, fmt.Sprintf("testdata/zxmode/scene_zxpal%d.png", pal))
		diff := 0
		for y := 0; y < 384; y++ {
			for x := 0; x < 512; x++ {
				r1, g1, b1, _ := got.At(x, y).RGBA()
				r2, g2, b2, _ := want.At(x, y).RGBA()
				if r1 != r2 || g1 != g2 || b1 != b2 {
					diff++
				}
			}
		}
		if diff != 0 {
			t.Errorf("%s: %d pixels differ from the reference", pal, diff)
		}
	}
}

func TestFitToFill(t *testing.T) {
	// A 400x100 source, left half red and right half blue, filled into 512x384:
	// the crop keeps the middle 133 columns, so the result is red then blue,
	// split down the middle, with nothing padded.
	src := image.NewRGBA(image.Rect(0, 0, 400, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 400; x++ {
			c := color.RGBA{255, 0, 0, 255}
			if x >= 200 {
				c = color.RGBA{0, 0, 255, 255}
			}
			src.SetRGBA(x, y, c)
		}
	}
	out, err := FitTo(src, ResizeFill, color.Black, 512, 384)
	if err != nil {
		t.Fatal(err)
	}
	if b := out.Bounds(); b.Dx() != 512 || b.Dy() != 384 {
		t.Fatalf("fill gave %v", b)
	}
	for _, pt := range []struct {
		x, y int
		red  bool
	}{{0, 0, true}, {250, 383, true}, {262, 0, false}, {511, 383, false}} {
		r, _, b, _ := out.At(pt.x, pt.y).RGBA()
		if (r>>8 == 255 && b == 0) != pt.red || (b>>8 == 255 && r == 0) == pt.red {
			t.Errorf("pixel (%d, %d): r %d b %d, want red %v", pt.x, pt.y, r>>8, b>>8, pt.red)
		}
	}
	// Shrinking averages: a 2x2 checkerboard of black and white tiles becomes grey.
	chk := image.NewRGBA(image.Rect(0, 0, 1024, 768))
	for y := 0; y < 768; y++ {
		for x := 0; x < 1024; x++ {
			if (x+y)%2 == 0 {
				chk.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
			} else {
				chk.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}
	out, _ = FitTo(chk, ResizeFill, color.Black, 512, 384)
	if r, _, _, _ := out.At(100, 100).RGBA(); r>>8 < 126 || r>>8 > 129 {
		t.Errorf("area filter gave %d, want about 128", r>>8)
	}
}
