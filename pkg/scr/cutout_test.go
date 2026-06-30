package scr

import (
	"bytes"
	"image/color"
	"testing"
)

func sampleScreen() *Screen {
	s := &Screen{}
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			s.Ink[y][x] = (x*7+y*13)%5 == 0
		}
	}
	for cy := 0; cy < Rows; cy++ {
		for cx := 0; cx < Cols; cx++ {
			s.Attr[cy][cx] = Attribute{Ink: uint8((cx + cy) % 8), Paper: uint8(cx % 8), Bright: cx%2 == 0}
		}
	}
	return s
}

func TestCollectionRoundTrip(t *testing.T) {
	s := sampleScreen()
	// one cell-aligned attributed asset, one bitmap-only pixel asset, one mask
	a1, err := CutCells(s, 2, 3, 5, 3, "opal", true)
	if err != nil {
		t.Fatal(err)
	}
	if !a1.HasAttrs() {
		t.Fatal("cell-aligned cut should keep attrs")
	}
	a2, err := CutRegion(s, Rect{X: 10, Y: 5, W: 13, H: 11}, "icon", true)
	if err != nil {
		t.Fatal(err)
	}
	if a2.HasAttrs() {
		t.Fatal("non-cell-aligned cut must be bitmap-only")
	}
	a3, _ := CutRegion(s, Rect{X: 0, Y: 0, W: 16, H: 16}, "stencil", false)
	a3.IsMask = true

	col := &Collection{Assets: []Asset{*a1, *a2, *a3}}
	enc, err := EncodeCollection(col)
	if err != nil {
		t.Fatal(err)
	}
	// every chunk boundary 8-aligned
	if len(enc)%8 != 0 {
		t.Errorf("encoded length %d not multiple of 8", len(enc))
	}

	dec, err := DecodeCollection(enc)
	if err != nil {
		t.Fatal(err)
	}
	if len(dec.Assets) != 3 {
		t.Fatalf("got %d assets, want 3", len(dec.Assets))
	}
	// re-encode identical
	enc2, _ := EncodeCollection(dec)
	if !bytes.Equal(enc, enc2) {
		t.Fatal("re-encode not byte-identical")
	}
	// named lookup
	if dec.Find("opal") == nil || dec.Find("icon") == nil || dec.Find("nope") != nil {
		t.Fatal("Find broken")
	}
	if !dec.Find("stencil").IsMask {
		t.Fatal("mask flag lost")
	}
	// content equality on the attributed asset
	o := dec.Find("opal")
	if o.Width != 40 || o.Height != 24 || !o.HasAttrs() {
		t.Errorf("opal decoded wrong: %dx%d attrs=%v", o.Width, o.Height, o.HasAttrs())
	}
}

func TestScreenCutPasteRoundTrip(t *testing.T) {
	s := sampleScreen()
	// cut a cell-aligned region with attrs, paste into a blank screen at the
	// same cell position: that region must match exactly.
	a, err := CutCells(s, 4, 2, 6, 4, "blk", true)
	if err != nil {
		t.Fatal(err)
	}
	dst := &Screen{}
	if err := Paste(dst, a, 4*8, 2*8, PasteCOPY); err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 4*8; y++ {
		for x := 0; x < 6*8; x++ {
			if dst.Ink[2*8+y][4*8+x] != s.Ink[2*8+y][4*8+x] {
				t.Fatalf("bitmap mismatch at %d,%d", x, y)
			}
		}
	}
	for cy := 0; cy < 4; cy++ {
		for cx := 0; cx < 6; cx++ {
			if dst.Attr[2+cy][4+cx] != s.Attr[2+cy][4+cx] {
				t.Fatalf("attr mismatch at cell %d,%d", cx, cy)
			}
		}
	}
}

func TestPasteOps(t *testing.T) {
	// asset: single set bit at (0,0), rest clear, 8x8
	mkAsset := func() *Asset {
		a := &Asset{Name: "m", Width: 8, Height: 8, Ink: make([][]bool, 8)}
		for y := range a.Ink {
			a.Ink[y] = make([]bool, 8)
		}
		a.Ink[0][0] = true
		return a
	}
	allSet := func() *Screen {
		s := &Screen{}
		for y := 0; y < Height; y++ {
			for x := 0; x < Width; x++ {
				s.Ink[y][x] = true
			}
		}
		return s
	}

	// OR: set bit writes; unset asset bits leave target untouched.
	dst := allSet()
	if err := Paste(dst, mkAsset(), 0, 0, PasteOR); err != nil {
		t.Fatal(err)
	}
	if !dst.Ink[0][0] || !dst.Ink[1][1] {
		t.Error("OR: set bit must write, unset must leave target")
	}

	// AND: clears target where asset bit is clear; leaves it where asset is set.
	dst = allSet()
	if err := Paste(dst, mkAsset(), 0, 0, PasteAND); err != nil {
		t.Fatal(err)
	}
	if !dst.Ink[0][0] {
		t.Error("AND: target under set asset bit must stay set")
	}
	if dst.Ink[1][1] {
		t.Error("AND: target under unset asset bit must be cleared")
	}

	// COPY: overwrites wholesale.
	dst = allSet()
	if err := Paste(dst, mkAsset(), 0, 0, PasteCOPY); err != nil {
		t.Fatal(err)
	}
	if !dst.Ink[0][0] {
		t.Error("COPY: set asset bit must write set")
	}
	if dst.Ink[1][1] {
		t.Error("COPY: unset asset bit must overwrite to clear")
	}

	// XOR: toggles where asset bit is set.
	dst = allSet()
	if err := Paste(dst, mkAsset(), 0, 0, PasteXOR); err != nil {
		t.Fatal(err)
	}
	if dst.Ink[0][0] {
		t.Error("XOR: set asset bit over set target must toggle to clear")
	}
	if !dst.Ink[1][1] {
		t.Error("XOR: unset asset bit must leave target")
	}
}

func TestApplyMask(t *testing.T) {
	a := &Asset{Width: 4, Height: 1, Ink: [][]bool{{true, true, true, true}}}
	m := &Asset{Width: 4, Height: 1, Ink: [][]bool{{true, false, true, false}}}
	if err := ApplyMask(a, m); err != nil {
		t.Fatal(err)
	}
	want := []bool{true, false, true, false}
	for i, b := range want {
		if a.Ink[0][i] != b {
			t.Errorf("pixel %d = %v, want %v", i, a.Ink[0][i], b)
		}
	}
}

func TestMapAttributes(t *testing.T) {
	s := sampleScreen()
	a, _ := CutCells(s, 0, 0, 4, 2, "t", true)
	a.MapAttributes(func(at Attribute) Attribute {
		at.Ink = White
		at.Bright = false
		return at
	})
	for cy := range a.Attr {
		for cx := range a.Attr[cy] {
			if a.Attr[cy][cx].Ink != White || a.Attr[cy][cx].Bright {
				t.Fatal("MapAttributes did not apply")
			}
		}
	}
}

func TestDecodeRejectsMalformed(t *testing.T) {
	s := sampleScreen()
	a, _ := CutCells(s, 0, 0, 2, 2, "x", true)
	good, _ := EncodeCollection(&Collection{Assets: []Asset{*a}})

	// bad magic
	bad := bytes.Clone(good)
	bad[0] = 'X'
	if _, err := DecodeCollection(bad); err == nil {
		t.Error("bad magic accepted")
	}
	// bad version
	bad = bytes.Clone(good)
	bad[4] = 9
	if _, err := DecodeCollection(bad); err == nil {
		t.Error("bad version accepted")
	}
	// truncated
	if _, err := DecodeCollection(good[:len(good)-4]); err == nil {
		t.Error("truncated accepted")
	}
	// corrupt payloadLen so derived size mismatches
	bad = bytes.Clone(good)
	bad[filePreambleLen+4] = 0xFF // payloadLen low byte
	if _, err := DecodeCollection(bad); err == nil {
		t.Error("inconsistent payloadLen accepted")
	}
}

func TestEncodeRejectsInvalidAsset(t *testing.T) {
	// attributed but not cell-aligned
	a := &Asset{Name: "x", Width: 5, Height: 5, Ink: make([][]bool, 5), Attr: [][]Attribute{{{}}}}
	for y := range a.Ink {
		a.Ink[y] = make([]bool, 5)
	}
	if _, err := EncodeCollection(&Collection{Assets: []Asset{*a}}); err == nil {
		t.Error("non-cell-aligned attributed asset accepted")
	}
	// mask with attrs
	b := &Asset{Name: "m", Width: 8, Height: 8, IsMask: true, Ink: make([][]bool, 8), Attr: [][]Attribute{{{}}}}
	for y := range b.Ink {
		b.Ink[y] = make([]bool, 8)
	}
	if _, err := EncodeCollection(&Collection{Assets: []Asset{*b}}); err == nil {
		t.Error("mask with attrs accepted")
	}
	// non-ASCII name
	cAsset := &Asset{Name: "caf\xe9", Width: 8, Height: 8, Ink: make([][]bool, 8)}
	for y := range cAsset.Ink {
		cAsset.Ink[y] = make([]bool, 8)
	}
	if _, err := EncodeCollection(&Collection{Assets: []Asset{*cAsset}}); err == nil {
		t.Error("non-ASCII name accepted")
	}
}

func TestAssetToImage(t *testing.T) {
	// attributed asset: drawn in its own colours
	s := sampleScreen()
	a, _ := CutCells(s, 0, 0, 1, 1, "c", true)
	img := AssetToImage(a, color.RGBA{}, color.RGBA{}, false)
	if img.Bounds().Dx() != 8 || img.Bounds().Dy() != 8 {
		t.Fatalf("attributed asset image %v, want 8x8", img.Bounds())
	}
	// bitmap-only asset: ink/paper supplied, paper transparent
	b := &Asset{Name: "m", Width: 2, Height: 1, Ink: [][]bool{{true, false}}}
	im2 := AssetToImage(b, color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}, color.RGBA{}, true)
	if im2.RGBAAt(0, 0) != (color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Error("set bit not white")
	}
	if im2.RGBAAt(1, 0) != (color.RGBA{0, 0, 0, 0}) {
		t.Error("unset bit not transparent")
	}
}
