package scr

import (
	"image"
	"image/color"
	"testing"
)

func encodeScene(t *testing.T, mode Mode, pal Palette, regs Registers) *Picture {
	t.Helper()
	img := loadTestImage(t, "testdata/zxmode/scene.png")
	w, h := mode.Size()
	if w != 512 {
		r, err := ResizeLanczos(img, w, h, [4]float64{0, 0, 512, 384})
		if err != nil {
			t.Fatal(err)
		}
		img = r
	}
	p, err := EncodePictureWith(img, mode, pal, EncodeOptions{Registers: regs})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func sameRender(a, b image.Image) bool {
	n, _ := channelDiffs(a, b)
	return n == 0
}

// TestConvertAssetExact converts assets to palettes that hold all their
// colours and requires the same picture: ZXPAL0 into ZXPAL1 (whose first 16
// registers are ZXPAL0's), ZXPAL3 into ZXPAL4 and back (ZXPAL4 extends
// ZXPAL3), and any palette into itself.
func TestConvertAssetExact(t *testing.T) {
	for _, c := range []struct{ from, to Palette }{
		{ZXPAL0, ZXPAL1}, {ZXPAL3, ZXPAL4}, {ZXPAL2, ZXPAL2}, {ZXPAL4, ZXPAL4},
	} {
		mode := ZXMODE1
		p := encodeScene(t, mode, c.from, nil)
		a := p.Asset("scene")
		b, err := ConvertAsset(a, c.to, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !sameRender(AssetToImage(a, nil, nil, false), AssetToImage(b, nil, nil, false)) {
			t.Errorf("%s to %s changes the picture", c.from, c.to)
		}
		if c.from == ZXPAL3 {
			back, _ := ConvertAsset(b, ZXPAL3, nil)
			for cy := range back.Attr2 {
				for cx := range back.Attr2[cy] {
					if back.Attr2[cy][cx] != a.Attr2[cy][cx] {
						t.Fatalf("ZXPAL3 to ZXPAL4 and back changes cell (%d, %d)", cx, cy)
					}
				}
			}
		}
	}
}

// TestConvertAssetNearest converts a two-colour cell into palettes that lack
// its colours and checks the nearest ones are chosen, within one bank for a
// one-byte palette.
func TestConvertAssetNearest(t *testing.T) {
	regs := ZXPAL4.Registers()
	regs[1] = color.RGBA{0xF0, 0x10, 0x10, 0xFF} // a red ZXPAL4 lacks
	a := &Asset{Name: "x", Width: 8, Height: 8, Palette: ZXPAL4, Registers: regs,
		Attr2: [][]CellAttr{{{A: 1, B: 0}}}}
	a.Ink = make([][]bool, 8)
	for y := range a.Ink {
		a.Ink[y] = make([]bool, 8)
		for x := 0; x < 4; x++ {
			a.Ink[y][x] = true
		}
	}
	b, err := ConvertAsset(a, ZXPAL0, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Red on black: bright red (0xFF0000) is nearer than 0xC80000, and
	// black exists in both banks, so bank 1 (bright) wins.
	if got := b.Attr[0][0]; got.Ink != Red || got.Paper != Black || !got.Bright {
		t.Errorf("ZXPAL0 cell %+v, want bright red on black", got)
	}
	c, _ := ConvertAsset(a, ZXPAL4, nil)
	if want := (CellAttr{A: 0x34, B: 0}); c.Attr2[0][0] != want { // register 52: FF0000
		t.Errorf("ZXPAL4 cell %+v, want %+v", c.Attr2[0][0], want)
	}
	// Flash survives ZXPAL0 to ZXPAL0 only.
	f := &Asset{Name: "f", Width: 8, Height: 8, Ink: a.Ink, Palette: ZXPAL0, Attr: [][]Attribute{{{Ink: 2, Paper: 7, Flash: true}}}}
	if g, _ := ConvertAsset(f, ZXPAL0, nil); !g.Attr[0][0].Flash {
		t.Error("flash lost from ZXPAL0 to ZXPAL0")
	}
	if g, _ := ConvertAsset(f, ZXPAL1, nil); g.Attr[0][0].Byte()&0xC0 != 0 {
		t.Errorf("ZXPAL1 cell %02X keeps a flash bit as a bank", g.Attr[0][0].Byte())
	}
}

// TestCutPastePictures cuts from a ZXMODE2 picture in ZXPAL4 with custom
// registers and pastes into pictures of other modes and palettes.
func TestCutPastePictures(t *testing.T) {
	regs := ZXPAL4.Registers()
	for i := range regs {
		regs[i].R = 255 - regs[i].R
	}
	src := encodeScene(t, ZXMODE2, ZXPAL4, regs)
	a, err := CutPicture(src, Rect{X: 64, Y: 32, W: 128, H: 96}, "piece", true)
	if err != nil {
		t.Fatal(err)
	}
	if a.Attr2 == nil || !a.Registers.Equal(regs) {
		t.Fatal("cut lost the attributes or registers")
	}
	// Back into a picture with the same registers: no conversion, same cells.
	same, _ := NewPicture(ZXMODE2, ZXPAL4)
	same.Registers = regs
	conv, err := PastePicture(same, a, 64, 32, PasteCOPY)
	if err != nil || conv {
		t.Fatalf("paste into the same palette: converted %v, %v", conv, err)
	}
	if same.Attr[4][8] != src.Attr[4][8] || same.Ink[40][70] != src.Ink[40][70] {
		t.Error("paste did not copy cells")
	}
	// Into a ZXMODE1 ZXPAL1 picture and a classic screen: converted.
	other, _ := NewPicture(ZXMODE1, ZXPAL1)
	if conv, err := PastePicture(other, a, 8, 8, PasteCOPY); err != nil || !conv {
		t.Fatalf("paste into ZXPAL1: converted %v, %v", conv, err)
	}
	s := &Screen{}
	if err := Paste(s, a, 0, 0, PasteCOPY); err != nil {
		t.Fatalf("paste into a classic screen: %v", err)
	}
	// Off-cell positions paste the bitmap only.
	if conv, err := PastePicture(other, a, 3, 5, PasteOR); err != nil || conv {
		t.Fatalf("off-cell paste: converted %v, %v", conv, err)
	}
	if _, err := PastePicture(other, a, 200, 0, PasteOR); err == nil {
		t.Error("a paste past the edge was accepted")
	}
	// A ZCUT round trip keeps the register table, as version 3.
	data, err := EncodeCollection(&Collection{Assets: []Asset{*a}})
	if err != nil {
		t.Fatal(err)
	}
	if data[4] != 3 {
		t.Errorf("collection version %d, want 3", data[4])
	}
	c, err := DecodeCollection(data)
	if err != nil || !c.Assets[0].Registers.Equal(regs) || c.Assets[0].Attr2[2][3] != a.Attr2[2][3] {
		t.Fatalf("round trip: %v", err)
	}
	a.Registers = nil
	if data, _ := EncodeCollection(&Collection{Assets: []Asset{*a}}); data[4] != 2 {
		t.Errorf("without registers the version is %d, want 2", data[4])
	}
}

func TestFlip(t *testing.T) {
	p := encodeScene(t, ZXMODE1, ZXPAL3, nil)
	orig := p.Render(false)
	p.Flip(true, false)
	r := p.Render(false)
	for y := 0; y < 192; y += 7 {
		for x := 0; x < 256; x += 5 {
			if r.RGBAAt(x, y) != orig.RGBAAt(255-x, y) {
				t.Fatalf("horizontal flip: pixel (%d, %d)", x, y)
			}
		}
	}
	p.Flip(true, true)
	r = p.Render(false)
	for y := 0; y < 192; y += 7 {
		for x := 0; x < 256; x += 5 {
			if r.RGBAAt(x, y) != orig.RGBAAt(x, 191-y) {
				t.Fatalf("vertical flip: pixel (%d, %d)", x, y)
			}
		}
	}
	a := p.Asset("a")
	a.Flip(false, true)
	if !sameRender(AssetToImage(a, nil, nil, false), orig) {
		t.Error("flipping the asset back does not give the original")
	}
}
