package zxn

import (
	"bytes"
	"image"
	"testing"
)

// TestTilesAgainstGfx2Next cuts the tile fixtures as gfx2next did and
// requires its tile, map and block files byte for byte.
func TestTilesAgainstGfx2Next(t *testing.T) {
	for _, c := range []struct {
		name, png string
		opt       TileOptions
		map16     bool
		mapY      bool
		block16   bool
	}{
		{"plain", "t8", TileOptions{Depth: 8}, false, false, false},
		{"rep8", "t8", TileOptions{Depth: 8, Dedup: DedupRepeat}, false, false, false},
		{"mir16", "t8", TileOptions{Depth: 8, Dedup: DedupMirror}, true, false, false},
		{"rot16", "t8", TileOptions{Depth: 8, Dedup: DedupRotate}, true, false, false},
		{"rot16y", "t8", TileOptions{Depth: 8, Dedup: DedupRotate, Offset: 3, PaletteOffset: 1}, true, true, false},
		{"rot4", "t4", TileOptions{Depth: 4, Dedup: DedupRotate}, true, false, false},
		{"rep4p", "t4", TileOptions{Depth: 4, Dedup: DedupRepeat, PaletteOffset: 1}, true, false, false},
		{"one", "t1", TileOptions{Depth: 1, Dedup: DedupRepeat}, false, false, false},
		{"blk", "t8", TileOptions{Depth: 8, Dedup: DedupRepeat, BlockWidth: 2, BlockHeight: 2, BlockDedup: true}, false, false, false},
		{"blk16", "t8", TileOptions{Depth: 8, Dedup: DedupRepeat, BlockWidth: 2, BlockHeight: 2}, false, false, true},
	} {
		img := loadPNG(t, "testdata/"+c.png+".png").(*image.Paletted)
		ts, tm, bs, err := CutTiles(img, c.opt)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		nxt, err := EncodeNXT(ts)
		if err != nil {
			t.Fatal(err)
		}
		if want := mustRead(t, "testdata/tiles_"+c.name+".nxt"); !bytes.Equal(nxt, want) {
			t.Errorf("%s: tiles differ (%d tiles, %d bytes; gfx2next %d bytes)", c.name, len(ts.Tiles), len(nxt), len(want))
		}
		nxm, err := EncodeNXM(tm, c.map16, c.mapY)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if want := mustRead(t, "testdata/tiles_"+c.name+".nxm"); !bytes.Equal(nxm, want) {
			t.Errorf("%s: map differs\n got % X\nwant % X", c.name, nxm, want)
		}
		if bs != nil {
			nxb, err := EncodeNXB(bs, c.block16)
			if err != nil {
				t.Fatal(err)
			}
			if want := mustRead(t, "testdata/tiles_"+c.name+".nxb"); !bytes.Equal(nxb, want) {
				t.Errorf("%s: blocks differ", c.name)
			}
			continue
		}
		// The map, drawn back with its attributes, is the image.
		back, err := DecodeNXT(nxt, ts.Width, ts.Height, ts.Depth)
		if err != nil {
			t.Fatal(err)
		}
		m2, err := DecodeNXM(nxm, tm.Width, tm.Height, c.map16, c.mapY)
		if err != nil {
			t.Fatal(err)
		}
		if c.opt.Depth == 1 {
			continue
		}
		pal, _ := PaletteFromImage(img, MaxColors, RoundNearest)
		drawn := MapImage(back, m2, pal, c.opt.Offset)
		b := img.Bounds()
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				v := img.ColorIndexAt(x, y)
				if c.opt.PaletteOffset != 0 && c.opt.Depth == 4 {
					v |= uint8(c.opt.PaletteOffset << 4)
				} else if c.opt.PaletteOffset != 0 {
					continue // an 8-bit tile's palette offset is not drawn
				}
				if drawn.RGBAAt(x, y) != pal.Colors[v].RGBA() {
					t.Fatalf("%s: pixel (%d, %d) drawn wrong", c.name, x, y)
				}
			}
		}
	}
}

func TestTileRefusals(t *testing.T) {
	img := image.NewPaletted(image.Rect(0, 0, 16, 8), nil)
	if _, _, _, err := CutTiles(img, TileOptions{Depth: 8, Width: 16, Height: 8, Dedup: DedupRotate}); err == nil {
		t.Error("rotation accepted for non-square tiles")
	}
	if _, _, _, err := CutTiles(img, TileOptions{Depth: 8, Dedup: DedupMirror, BlockWidth: 2, BlockHeight: 1}); err == nil {
		t.Error("mirrored tiles accepted in blocks")
	}
	if _, _, _, err := CutTiles(img, TileOptions{Depth: 8, Width: 3, Height: 3}); err == nil {
		t.Error("16x8 accepted as 3x3 cells")
	}
	m := &TileMap{Width: 1, Height: 1, Entries: []uint16{0x0801}}
	if _, err := EncodeNXM(m, false, false); err == nil {
		t.Error("attributes accepted in an 8-bit map")
	}
}
