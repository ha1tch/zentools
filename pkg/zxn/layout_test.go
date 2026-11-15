package zxn

import (
	"bytes"
	"image"
	"testing"
)

func encodeSprites(t *testing.T, img image.Image, pal *Palette, depth int) []byte {
	t.Helper()
	s, err := SpritesFromImage(img, pal, depth, RoundNearest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := EncodeSPR(s)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func samePalette(t *testing.T, name string, p *Palette, l Layout, mode RoundMode) {
	t.Helper()
	got, err := EncodePalette(p, l, mode)
	if err != nil {
		t.Fatal(err)
	}
	if want := mustRead(t, "testdata/"+name); !bytes.Equal(got, want) {
		t.Errorf("%s: %d bytes differ from gfx2next's %d", name, len(got), len(want))
	}
}

// TestLayoutsAgainstGfx2Next checks the 1-byte and 4-bit palette layouts.
func TestLayoutsAgainstGfx2Next(t *testing.T) {
	s8 := loadPNG(t, "testdata/s8.png")
	s4 := loadPNG(t, "testdata/s4.png")
	for _, c := range []struct {
		file string
		img  image.Image
		size int
		l    Layout
		mode RoundMode
	}{
		{"s8_rgb332.nxp", s8, 256, LayoutRGB332, RoundDistance},
		{"s8_gbr444.nxp", s8, 256, LayoutGBR444, RoundDistance},
		{"s8_gbr444_floor.nxp", s8, 256, LayoutGBR444, RoundFloor},
		{"s8_gbr444_ceil.nxp", s8, 256, LayoutGBR444, RoundCeil},
		{"s4_bgr222.nxp", s4, 16, LayoutBGR222, RoundDistance},
	} {
		p, err := PaletteFromImage(c.img, MaxColors, c.mode)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Colors) > c.size {
			p.Colors = p.Colors[:c.size]
		}
		samePalette(t, c.file, p.Padded(c.size), c.l, c.mode)
	}
}

// TestStandardAgainstGfx2Next checks -pal-std: the standard palette and the
// pixels mapped onto it.
func TestStandardAgainstGfx2Next(t *testing.T) {
	img := loadPNG(t, "testdata/s8.png").(*image.Paletted)
	std := StandardPalette()
	samePalette(t, "s8_std.nxp", std, LayoutRGB333, RoundNearest)
	for _, c := range []struct {
		file string
		mode RoundMode
	}{{"s8_std.spr", RoundDistance}, {"s8_std_floor.spr", RoundFloor}} {
		m := RemapImage(img, StandardTable(img.Palette, c.mode), std)
		if got := encodeSprites(t, m, std, 8); !bytes.Equal(got, mustRead(t, "testdata/"+c.file)) {
			t.Errorf("%s differs", c.file)
		}
	}
}

// TestMinimizeAgainstGfx2Next checks -pal-min at both depths, including the
// 4-bit case where black padding makes a seventeenth colour that is dropped.
func TestMinimizeAgainstGfx2Next(t *testing.T) {
	for _, c := range []struct {
		name   string
		depth  int
		size   int
		unique int
	}{{"s8", 8, 256, 197}, {"s4", 4, 16, 17}} {
		img := loadPNG(t, "testdata/"+c.name+".png").(*image.Paletted)
		p, err := PaletteFromImage(img, MaxColors, RoundDistance)
		if err != nil {
			t.Fatal(err)
		}
		min, n, table := Minimize(p.Padded(MaxColors))
		if n != c.unique {
			t.Errorf("%s: %d distinct colours, want %d", c.name, n, c.unique)
		}
		if c.size < MaxColors {
			min.Colors = min.Colors[:c.size]
			for i, v := range table {
				if int(v) >= c.size {
					table[i] = 0
				}
			}
		}
		samePalette(t, c.name+"_min.nxp", min, LayoutRGB333, RoundNearest)
		m := RemapImage(img, table, min)
		if got := encodeSprites(t, m, min, c.depth); !bytes.Equal(got, mustRead(t, "testdata/"+c.name+"_min.spr")) {
			t.Errorf("%s_min.spr differs", c.name)
		}
	}
}

func TestLayoutRoundTrips(t *testing.T) {
	std := StandardPalette()
	data, _ := EncodePalette(std, LayoutRGB332, RoundNearest)
	for i, v := range data {
		if int(v) != i {
			t.Fatalf("standard palette entry %d encodes as %d", i, v)
		}
	}
	back, err := DecodePalette(data, LayoutRGB332)
	if err != nil {
		t.Fatal(err)
	}
	for i := range back.Colors {
		if back.Colors[i] != std.Colors[i] {
			t.Fatalf("RGB332 entry %d: %v, want %v", i, back.Colors[i], std.Colors[i])
		}
	}
	// 4-bit levels survive GBR444 exactly; so do 2-bit levels BGR222.
	g, err := DecodePaletteRGBA(mustRead(t, "testdata/s8_gbr444.nxp"), LayoutGBR444)
	if err != nil || len(g) != 256 {
		t.Fatalf("gbr444 decode: %v, %d colours", err, len(g))
	}
	b, err := DecodePaletteRGBA(mustRead(t, "testdata/s4_bgr222.nxp"), LayoutBGR222)
	if err != nil || len(b) != 16 {
		t.Fatalf("bgr222 decode: %v, %d colours", err, len(b))
	}
	for _, c := range b {
		if c.R%85 != 0 || c.G%85 != 0 || c.B%85 != 0 {
			t.Fatalf("bgr222 colour %v is not on 2-bit levels", c)
		}
	}
	if _, err := DecodePalette([]byte{1, 2}, LayoutGBR444); err == nil {
		t.Error("DecodePalette accepted a GBR444 palette")
	}
	if _, err := DecodePaletteRGBA([]byte{1, 2, 3}, LayoutGBR444); err == nil {
		t.Error("an odd-length GBR444 palette was accepted")
	}
}
