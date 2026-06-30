package scr

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func TestAttributeRoundTrip(t *testing.T) {
	for b := 0; b < 256; b++ {
		a := AttributeFromByte(byte(b))
		if got := a.Byte(); got != byte(b) {
			t.Fatalf("attribute byte %#02x round-tripped to %#02x", b, got)
		}
	}
}

// TestRowAddrKnownValues checks the screen interleave against hand-computed
// offsets. Row 0 starts at 0; row 1 (same char row, next pixel row) is +256;
// row 8 (next char row) is +32; row 64 (second third) is +2048.
func TestRowAddrKnownValues(t *testing.T) {
	cases := []struct {
		y, want int
	}{
		{0, 0},
		{1, 256},
		{7, 7 * 256},
		{8, 32},
		{9, 256 + 32},
		{64, 2048},
		{128, 4096},
		{191, 2048*2 + 7*256 + 7*32},
	}
	for _, c := range cases {
		if got := rowAddr(c.y); got != c.want {
			t.Errorf("rowAddr(%d) = %d, want %d", c.y, got, c.want)
		}
	}
}

// TestRowAddrCoversBitmap verifies the interleave is a bijection over the 6144
// bitmap bytes: every row maps to a distinct 32-byte span and together they
// tile the region exactly once.
func TestRowAddrCoversBitmap(t *testing.T) {
	seen := make([]bool, bitmapLen)
	for y := 0; y < Height; y++ {
		base := rowAddr(y)
		for cx := 0; cx < Cols; cx++ {
			idx := base + cx
			if idx < 0 || idx >= bitmapLen {
				t.Fatalf("row %d col %d -> offset %d out of range", y, cx, idx)
			}
			if seen[idx] {
				t.Fatalf("offset %d written twice (row %d col %d)", idx, y, cx)
			}
			seen[idx] = true
		}
	}
	for i, ok := range seen {
		if !ok {
			t.Fatalf("bitmap offset %d never written", i)
		}
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	src := &Screen{}
	// Deterministic pseudo-pattern across pixels and attributes.
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			src.Ink[y][x] = (x*31+y*17)%5 == 0
		}
	}
	for cy := 0; cy < Rows; cy++ {
		for cx := 0; cx < Cols; cx++ {
			src.Attr[cy][cx] = Attribute{
				Ink:    uint8((cx + cy) % 8),
				Paper:  uint8((cx*2 + cy) % 8),
				Bright: (cx+cy)%2 == 0,
				Flash:  cy%3 == 0,
			}
		}
	}

	enc := Encode(src)
	if len(enc) != FileLen {
		t.Fatalf("Encode produced %d bytes, want %d", len(enc), FileLen)
	}

	dec, err := Decode(enc)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if *dec != *src {
		t.Fatal("decoded screen differs from source")
	}

	// Encoding the decoded screen reproduces the bytes exactly.
	if !bytes.Equal(Encode(dec), enc) {
		t.Fatal("re-encode not byte-identical")
	}
}

func TestDecodeRejectsWrongSize(t *testing.T) {
	if _, err := Decode(make([]byte, 100)); err == nil {
		t.Fatal("expected error for short data")
	}
	if _, err := Decode(make([]byte, FileLen+1)); err == nil {
		t.Fatal("expected error for oversized data")
	}
}

// TestFromImageConvertsAndRoundTrips builds a synthetic 256x192 image using only
// legal Spectrum colours laid out one solid colour per cell, converts it, and
// confirms ToImage reproduces it. With one colour per cell there is no clash, so
// conversion must be exact.
func TestFromImageConvertsAndRoundTrips(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	bright := paletteRGB(true)
	for cy := 0; cy < Rows; cy++ {
		for cx := 0; cx < Cols; cx++ {
			c := bright[(cx+cy)%8]
			rc := color.RGBA{uint8(c.r), uint8(c.g), uint8(c.b), 0xFF}
			for yy := 0; yy < CellSize; yy++ {
				for xx := 0; xx < CellSize; xx++ {
					img.Set(cx*8+xx, cy*8+yy, rc)
				}
			}
		}
	}

	s, err := FromImage(img)
	if err != nil {
		t.Fatalf("FromImage: %v", err)
	}
	// A solid cell: every distinct colour equal, so darkest==brightest==ink;
	// paper equals ink and the whole cell is paper (no ink bits). The rendered
	// colour must match the source cell colour.
	out := ToImage(s)
	for cy := 0; cy < Rows; cy++ {
		for cx := 0; cx < Cols; cx++ {
			want := img.RGBAAt(cx*8, cy*8)
			got := out.RGBAAt(cx*8, cy*8)
			if want != got {
				t.Fatalf("cell (%d,%d): got %v want %v", cx, cy, got, want)
			}
		}
	}

	// And the SCR bytes round-trip.
	enc := Encode(s)
	dec, err := Decode(enc)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(Encode(dec), enc) {
		t.Fatal("scr re-encode not byte-identical")
	}
}

func TestFromImageRejectsWrongSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	if _, err := FromImage(img); err == nil {
		t.Fatal("expected SizeError for non-256x192 image")
	}
}

func TestParseColour(t *testing.T) {
	cases := []struct {
		in   string
		want uint8
		ok   bool
	}{
		{"black", Black, true},
		{"WHITE", White, true},
		{" red ", Red, true},
		{"0", Black, true},
		{"7", White, true},
		{"3", Magenta, true},
		{"8", 0, false},
		{"-1", 0, false},
		{"teal", 0, false},
	}
	for _, c := range cases {
		got, err := ParseColour(c.in)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("ParseColour(%q) = %d,%v; want %d,nil", c.in, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("ParseColour(%q): expected error", c.in)
		}
	}
}

func TestParseBool(t *testing.T) {
	for _, s := range []string{"1", "true", "yes", "on", " 1 "} {
		if b, err := ParseBool(s); err != nil || !b {
			t.Errorf("ParseBool(%q) = %v,%v; want true", s, b, err)
		}
	}
	for _, s := range []string{"0", "false", "no", "off"} {
		if b, err := ParseBool(s); err != nil || b {
			t.Errorf("ParseBool(%q) = %v,%v; want false", s, b, err)
		}
	}
	if _, err := ParseBool("maybe"); err == nil {
		t.Error("ParseBool(maybe): expected error")
	}
}

func TestParseAttribute(t *testing.T) {
	a, err := ParseAttribute("{ ink: white ; paper:1; bright:1; flash:0 }")
	if err != nil {
		t.Fatal(err)
	}
	want := Attribute{Ink: White, Paper: Blue, Bright: true, Flash: false}
	if a != want {
		t.Errorf("got %+v, want %+v", a, want)
	}

	def, err := ParseAttribute("")
	if err != nil {
		t.Fatal(err)
	}
	if def != (Attribute{Ink: Black, Paper: Black}) {
		t.Errorf("empty spec = %+v, want all black", def)
	}

	idx, err := ParseAttribute("ink:2;paper:6")
	if err != nil {
		t.Fatal(err)
	}
	if idx.Ink != Red || idx.Paper != Yellow {
		t.Errorf("index spec = %+v", idx)
	}

	for _, bad := range []string{"paper:teal", "wibble:1", "paper", "bright:maybe", "paper:9"} {
		if _, err := ParseAttribute(bad); err == nil {
			t.Errorf("ParseAttribute(%q): expected error", bad)
		}
	}
}

func TestAttributeStringRoundTrips(t *testing.T) {
	a := Attribute{Ink: Cyan, Paper: Magenta, Bright: true, Flash: true}
	s := a.String()
	back, err := ParseAttribute(s)
	if err != nil {
		t.Fatalf("re-parse %q: %v", s, err)
	}
	if back != a {
		t.Errorf("round-trip: %+v -> %q -> %+v", a, s, back)
	}
}

func TestAttributeRGBAHelpers(t *testing.T) {
	a := Attribute{Ink: White, Paper: Blue, Bright: true}
	ink := a.InkRGBA()
	if ink != (color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Errorf("bright white ink = %v", ink)
	}
	paper := a.PaperRGBA()
	if paper != (color.RGBA{0, 0, 0xFF, 0xFF}) {
		t.Errorf("bright blue paper = %v", paper)
	}
	dim := Attribute{Paper: Red}.PaperRGBA()
	if dim != (color.RGBA{0xC8, 0, 0, 0xFF}) {
		t.Errorf("dim red paper = %v", dim)
	}
}

func TestFitNoneRequiresExactSize(t *testing.T) {
	exact := image.NewRGBA(image.Rect(0, 0, Width, Height))
	if _, err := Fit(exact, ResizeNone, color.Black); err != nil {
		t.Errorf("ResizeNone on 256x192: unexpected error %v", err)
	}
	wrong := image.NewRGBA(image.Rect(0, 0, 320, 240))
	_, err := Fit(wrong, ResizeNone, color.Black)
	if err == nil {
		t.Fatal("ResizeNone on wrong size: expected SizeError")
	}
	if _, ok := err.(*SizeError); !ok {
		t.Errorf("expected *SizeError, got %T", err)
	}
}

func TestFitProducesScreenSize(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 64, 64))
	// fill src with a solid colour so we can check stretch covers the frame
	draw := func(im *image.RGBA, c color.RGBA) {
		for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
			for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
				im.SetRGBA(x, y, c)
			}
		}
	}
	draw(src, color.RGBA{0xFF, 0, 0, 0xFF})

	for _, mode := range []ResizeMode{ResizeStretch, ResizeBestFit, ResizeCentre} {
		out, err := Fit(src, mode, color.RGBA{0, 0, 0xFF, 0xFF})
		if err != nil {
			t.Fatalf("mode %d: %v", mode, err)
		}
		if out.Bounds().Dx() != Width || out.Bounds().Dy() != Height {
			t.Errorf("mode %d: size %v, want 256x192", mode, out.Bounds())
		}
	}
}

func TestFitCentreUsesFillForBorder(t *testing.T) {
	// A 16x16 red source centred on the screen: corners must be the fill colour,
	// centre must be the source colour.
	src := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			src.SetRGBA(x, y, color.RGBA{0xFF, 0, 0, 0xFF})
		}
	}
	fill := color.RGBA{0, 0xFF, 0, 0xFF}
	out, err := Fit(src, ResizeCentre, fill)
	if err != nil {
		t.Fatal(err)
	}
	rgba := out.(*image.RGBA)
	if got := rgba.RGBAAt(0, 0); got != fill {
		t.Errorf("corner = %v, want fill %v", got, fill)
	}
	if got := rgba.RGBAAt(Width/2, Height/2); got != (color.RGBA{0xFF, 0, 0, 0xFF}) {
		t.Errorf("centre = %v, want red source", got)
	}
}

func TestFitBestFitPreservesAspect(t *testing.T) {
	// A wide 200x50 source: best-fit should scale to full width (256) and a
	// proportional height (64), leaving fill bars top and bottom.
	src := image.NewRGBA(image.Rect(0, 0, 200, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 200; x++ {
			src.SetRGBA(x, y, color.RGBA{0xFF, 0xFF, 0xFF, 0xFF})
		}
	}
	fill := color.RGBA{0, 0, 0, 0xFF}
	out, err := Fit(src, ResizeBestFit, fill)
	if err != nil {
		t.Fatal(err)
	}
	rgba := out.(*image.RGBA)
	// Top row should be fill (letterbox bar); middle row should be white content.
	if got := rgba.RGBAAt(Width/2, 2); got != fill {
		t.Errorf("top bar = %v, want fill", got)
	}
	if got := rgba.RGBAAt(Width/2, Height/2); got != (color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Errorf("centre = %v, want white content", got)
	}
}

func solidImage(w, h int, c color.RGBA) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.SetRGBA(x, y, c)
		}
	}
	return im
}

func TestCellRect(t *testing.T) {
	r := CellRect(5, 10, 5, 6)
	want := Rect{X: 40, Y: 80, W: 40, H: 48}
	if r != want {
		t.Errorf("CellRect(5,10,5,6) = %+v, want %+v", r, want)
	}
}

func TestCrop(t *testing.T) {
	src := solidImage(64, 64, color.RGBA{0, 0, 0xC8, 0xFF})
	// paint a 10x10 white square at (20,20)
	for y := 20; y < 30; y++ {
		for x := 20; x < 30; x++ {
			src.SetRGBA(x, y, color.RGBA{0xFF, 0xFF, 0xFF, 0xFF})
		}
	}
	out, err := Crop(src, Rect{X: 20, Y: 20, W: 10, H: 10})
	if err != nil {
		t.Fatal(err)
	}
	if out.Bounds().Dx() != 10 || out.Bounds().Dy() != 10 {
		t.Fatalf("crop size %v", out.Bounds())
	}
	if out.RGBAAt(0, 0) != (color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Errorf("crop corner = %v, want white", out.RGBAAt(0, 0))
	}

	// Non-multiple-of-8 dimensions are allowed.
	if _, err := Crop(src, Rect{X: 1, Y: 1, W: 34, H: 15}); err != nil {
		t.Errorf("non-mult-of-8 crop: %v", err)
	}
	// Out of bounds rejected.
	if _, err := Crop(src, Rect{X: 60, Y: 60, W: 10, H: 10}); err == nil {
		t.Error("out-of-bounds crop: expected error")
	}
	// Non-positive rejected.
	if _, err := Crop(src, Rect{X: 0, Y: 0, W: 0, H: 5}); err == nil {
		t.Error("zero-width crop: expected error")
	}
}

func TestAutoExtent(t *testing.T) {
	src := solidImage(64, 64, color.RGBA{0, 0, 0xC8, 0xFF})
	for y := 20; y < 38; y++ {
		for x := 16; x < 40; x++ {
			src.SetRGBA(x, y, color.RGBA{0xC8, 0, 0, 0xFF})
		}
	}
	r, err := AutoExtent(src, nil, 16)
	if err != nil {
		t.Fatal(err)
	}
	want := Rect{X: 16, Y: 20, W: 24, H: 18}
	if r != want {
		t.Errorf("AutoExtent = %+v, want %+v", r, want)
	}

	// Uniform image errors.
	if _, err := AutoExtent(solidImage(16, 16, color.RGBA{0, 0, 0, 0xFF}), nil, 16); err == nil {
		t.Error("uniform image: expected error")
	}

	// Explicit background.
	r2, err := AutoExtent(src, color.RGBA{0, 0, 0xC8, 0xFF}, 16)
	if err != nil {
		t.Fatal(err)
	}
	if r2 != want {
		t.Errorf("AutoExtent with explicit bg = %+v, want %+v", r2, want)
	}
}

func TestBitmapExtent(t *testing.T) {
	s := &Screen{}
	// set a 16x16 block of ink at (40,48)
	for y := 48; y < 64; y++ {
		for x := 40; x < 56; x++ {
			s.Ink[y][x] = true
		}
	}
	r, err := BitmapExtent(s, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := Rect{X: 40, Y: 48, W: 16, H: 16}
	if r != want {
		t.Errorf("BitmapExtent(bits=1) = %+v, want %+v", r, want)
	}

	// bits=0 inverts: unset pixels span the whole screen.
	r0, err := BitmapExtent(s, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r0 != (Rect{X: 0, Y: 0, W: Width, H: Height}) {
		t.Errorf("BitmapExtent(bits=0) = %+v, want full screen", r0)
	}

	// All-unset screen with bits=1 errors.
	if _, err := BitmapExtent(&Screen{}, 1); err == nil {
		t.Error("BitmapExtent on empty screen (bits=1): expected error")
	}
}

func TestToImageBitmap(t *testing.T) {
	s := &Screen{}
	s.Ink[0][0] = true // one set bit at origin

	// bitmap-only, white ink, transparent paper
	img := ToImageBitmap(s, 1, color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}, color.RGBA{}, true)
	if got := img.RGBAAt(0, 0); got != (color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Errorf("set bit = %v, want opaque white", got)
	}
	if got := img.RGBAAt(1, 1); got != (color.RGBA{0, 0, 0, 0}) {
		t.Errorf("unset bit = %v, want transparent", got)
	}

	// opaque paper
	img2 := ToImageBitmap(s, 1, color.RGBA{0xFF, 0, 0, 0xFF}, color.RGBA{0, 0, 0xFF, 0xFF}, false)
	if got := img2.RGBAAt(1, 1); got != (color.RGBA{0, 0, 0xFF, 0xFF}) {
		t.Errorf("opaque paper = %v, want blue", got)
	}

	// bits=0 inverts which pixels are "set"
	img3 := ToImageBitmap(s, 0, color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}, color.RGBA{}, true)
	if got := img3.RGBAAt(0, 0); got != (color.RGBA{0, 0, 0, 0}) {
		t.Errorf("bits=0 at set pixel = %v, want transparent", got)
	}
	if got := img3.RGBAAt(1, 1); got != (color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Errorf("bits=0 at unset pixel = %v, want white", got)
	}
}

// TestEncoderPreservesGreen guards the posterise+frequency cell reducer against
// regression to luma-extremes selection, which discarded mid-luma hues. A cell
// dominated by green flanked by black and a brighter colour must encode as
// green, not snap to the brighter colour.
func TestEncoderPreservesGreen(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	// fill black
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			img.SetRGBA(x, y, color.RGBA{0, 0, 0, 0xFF})
		}
	}
	// one cell: majority vivid green, a few yellow pixels, rest black.
	green := color.RGBA{0x3D, 0xFF, 0x00, 0xFF} // the gem's yellow-green
	yellow := color.RGBA{0xFF, 0xFF, 0x00, 0xFF}
	for yy := 0; yy < 8; yy++ {
		for xx := 0; xx < 8; xx++ {
			c := green
			if yy == 0 && xx < 2 {
				c = yellow // minority
			}
			img.SetRGBA(8+xx, 8+yy, c)
		}
	}
	s, err := FromImage(img)
	if err != nil {
		t.Fatal(err)
	}
	a := s.Attr[1][1] // cell (1,1)
	if a.Ink != Green && a.Paper != Green {
		t.Errorf("green-dominant cell encoded without green: ink=%d paper=%d", a.Ink, a.Paper)
	}
	if a.Ink == Yellow && a.Paper != Green {
		t.Errorf("green flipped to yellow (the original defect): ink=%d paper=%d", a.Ink, a.Paper)
	}
}

// TestUniformCellSolidPaper guards the uniform-cell fix: a flat single-colour
// cell must become solid paper with no set bits, not all-ink-set.
func TestUniformCellSolidPaper(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			img.SetRGBA(x, y, color.RGBA{0, 0, 0, 0xFF}) // all black
		}
	}
	s, err := FromImage(img)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if s.Ink[y][x] {
				t.Fatalf("uniform black image has a set bit at %d,%d", x, y)
			}
		}
	}
}

// TestEncoderKeepsSplitBrightnessStroke guards against a thin feature being
// wiped when posterising splits its pixels across both brightness sets. A cell
// of mostly white stroke (some bright, some dim after antialiasing) over a black
// background must keep the stroke, not collapse to empty. Regression for the
// broken-O-corner defect: tallying by (index,brightness) let white's two
// brightness buckets crowd out black and trigger the uniform-cell wipe.
func TestEncoderKeepsSplitBrightnessStroke(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			img.SetRGBA(x, y, color.RGBA{0, 0, 0, 0xFF})
		}
	}
	// cell (1,1): mix of bright white, mid-grey (antialiasing), and black.
	for yy := 0; yy < 8; yy++ {
		for xx := 0; xx < 8; xx++ {
			var c color.RGBA
			switch {
			case yy < 4:
				c = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF} // bright white stroke
			case yy < 6:
				c = color.RGBA{0x90, 0x90, 0x90, 0xFF} // antialiased grey
			default:
				c = color.RGBA{0, 0, 0, 0xFF} // black background
			}
			img.SetRGBA(8+xx, 8+yy, c)
		}
	}
	s, err := FromImage(img)
	if err != nil {
		t.Fatal(err)
	}
	// the cell must NOT be wiped empty: some bits set for the white stroke.
	set := 0
	for yy := 0; yy < 8; yy++ {
		for xx := 0; xx < 8; xx++ {
			if s.Ink[8+yy][8+xx] {
				set++
			}
		}
	}
	if set == 0 {
		t.Fatal("split-brightness white stroke was wiped to empty (the broken-O defect)")
	}
	// and the cell should carry both white and black, not white-on-white.
	a := s.Attr[1][1]
	if a.Ink == a.Paper {
		t.Errorf("cell collapsed to a single colour (ink==paper==%d)", a.Ink)
	}
}
