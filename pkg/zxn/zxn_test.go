package zxn

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

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

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestAgainstGfx2Next checks palettes and sprites byte for byte against what
// gfx2next and nextbmp/nextraw write for the same images (testdata/README.md).
func TestAgainstGfx2Next(t *testing.T) {
	img8 := loadPNG(t, "testdata/s8.png")
	for name, mode := range map[string]RoundMode{"round": RoundNearest, "floor": RoundFloor, "ceil": RoundCeil} {
		pal, err := PaletteFromImage(img8, MaxColors, mode)
		if err != nil {
			t.Fatal(err)
		}
		got, err := EncodeNXP(pal.Padded(MaxColors))
		if err != nil {
			t.Fatal(err)
		}
		if want := mustRead(t, "testdata/s8_"+name+".nxp"); !bytes.Equal(got, want) {
			t.Errorf("%s: palette differs from gfx2next", name)
		}
	}
	sheet, err := SpritesFromImage(img8, nil, 8, RoundNearest)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := EncodeSPR(sheet)
	if want := mustRead(t, "testdata/s8.spr"); !bytes.Equal(got, want) {
		t.Errorf("8-bit sprites differ from gfx2next")
	}

	img4 := loadPNG(t, "testdata/s4.png")
	pal4, err := PaletteFromImage(img4, MaxColors, RoundNearest)
	if err != nil {
		t.Fatal(err)
	}
	pal4.Colors = pal4.Padded(16).Colors[:16] // a 4-bit palette is the first 16 colours
	gotPal, _ := EncodeNXP(pal4)
	if want := mustRead(t, "testdata/s4.nxp"); !bytes.Equal(gotPal, want) {
		t.Errorf("4-bit palette differs from gfx2next")
	}
	sheet4, err := SpritesFromImage(img4, nil, 4, RoundNearest)
	if err != nil {
		t.Fatal(err)
	}
	got4, _ := EncodeSPR(sheet4)
	if want := mustRead(t, "testdata/s4.spr"); !bytes.Equal(got4, want) {
		t.Errorf("4-bit sprites differ from gfx2next")
	}
}

func TestRoundTrips(t *testing.T) {
	for _, path := range []string{"testdata/s8_round.nxp", "testdata/s4.nxp"} {
		data := mustRead(t, path)
		p, err := DecodeNXP(data)
		if err != nil {
			t.Fatal(err)
		}
		back, _ := EncodeNXP(p)
		if !bytes.Equal(back, data) {
			t.Errorf("%s: palette changed through a round trip", path)
		}
	}
	// The priority bit survives.
	p, _ := DecodeNXP([]byte{0xFF, 0x81, 0x00, 0x00})
	if !p.Colors[0].Priority || p.Colors[0] != (Color333{7, 7, 7, true}) || p.Colors[1].Priority {
		t.Errorf("priority bit read as %+v", p.Colors)
	}
	if b, _ := EncodeNXP(p); !bytes.Equal(b, []byte{0xFF, 0x81, 0x00, 0x00}) {
		t.Errorf("priority bit written as %x", b)
	}
	for depth, path := range map[int]string{8: "testdata/s8.spr", 4: "testdata/s4.spr"} {
		data := mustRead(t, path)
		s, err := DecodeSPR(data, depth)
		if err != nil {
			t.Fatal(err)
		}
		back, _ := EncodeSPR(s)
		if !bytes.Equal(back, data) {
			t.Errorf("%s: sprites changed through a round trip", path)
		}
	}
}

func TestConversions(t *testing.T) {
	// Every 3-bit level shows as round(v x 255 / 7) and reduces back to itself.
	for v := uint8(0); v < 8; v++ {
		c := Color333{R: v, G: v, B: v}
		if back := ToRGB333(c.RGBA(), RoundNearest); back != c {
			t.Errorf("level %d: %v reduces to %v", v, c.RGBA(), back)
		}
	}
	// Nearest, not truncation: 0xC8 (200) is closer to 182 (level 5) than to 219 (6).
	if got := ToRGB333(color.RGBA{200, 200, 200, 255}, RoundNearest); got.R != 5 {
		t.Errorf("0xC8 rounds to level %d, want 5", got.R)
	}
	if got := ToRGB333(color.RGBA{200, 0, 0, 255}, RoundFloor); got.R != 5 {
		t.Errorf("0xC8 floors to level %d, want 5", got.R)
	}
	if got := ToRGB333(color.RGBA{37, 0, 0, 255}, RoundCeil); got.R != 2 {
		t.Errorf("37 ceils to level %d, want 2", got.R)
	}
}

func TestRefusals(t *testing.T) {
	if _, err := DecodeNXP(make([]byte, 3)); err == nil {
		t.Error("odd-length palette accepted")
	}
	if _, err := DecodeNXP(make([]byte, 514)); err == nil {
		t.Error("257-colour palette accepted")
	}
	if _, err := DecodeSPR(make([]byte, 100), 8); err == nil {
		t.Error("partial pattern accepted")
	}
	s := &SpriteSheet{Depth: 4, Sprites: []Bitmap{{Width: 16, Height: 16, Indices: make([]uint8, 256)}}}
	s.Sprites[0].Indices[5] = 16
	if _, err := EncodeSPR(s); err == nil {
		t.Error("index 16 accepted at depth 4")
	}
	// A true-colour image with more than 256 RGB333 colours is refused.
	big := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for i := 0; i < 32*32; i++ { // all 512 RGB333 colours, twice
		big.SetRGBA(i%32, i/32, Color333{R: uint8(i & 7), G: uint8(i >> 3 & 7), B: uint8(i >> 6 & 7)}.RGBA())
	}
	if _, err := PaletteFromImage(big, MaxColors, RoundNearest); err == nil {
		t.Error("image with more than 256 colours accepted")
	}
	// A true-colour sprite sheet is matched against the palette.
	rgb := image.NewRGBA(image.Rect(0, 0, 16, 16))
	pal := &Palette{Colors: []Color333{{0, 0, 0, false}, {7, 0, 0, false}}}
	rgb.SetRGBA(3, 2, color.RGBA{255, 0, 0, 255})
	for i := range rgb.Pix {
		if i%4 == 3 {
			rgb.Pix[i] = 255
		}
	}
	sh, err := SpritesFromImage(rgb, pal, 8, RoundNearest)
	if err != nil {
		t.Fatal(err)
	}
	if sh.Sprites[0].At(3, 2) != 1 || sh.Sprites[0].At(0, 0) != 0 {
		t.Error("true-colour sprite matched to the wrong indices")
	}
	rgb.SetRGBA(0, 0, color.RGBA{0, 255, 0, 255})
	if _, err := SpritesFromImage(rgb, pal, 8, RoundNearest); err == nil {
		t.Error("colour missing from the palette accepted")
	}
}
