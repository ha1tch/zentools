// file: next_bitmap.go
//
// `zx next` sprite attributes, Layer 2 and LoRes bitmaps, Timex screens and
// tiles.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"os"
	"strconv"
	"strings"

	"github.com/ha1tch/zentools/pkg/scr"
	"github.com/ha1tch/zentools/pkg/zxn"
)

// --- sprite attributes ------------------------------------------------------

func nextAttrsEncode(args []string) error {
	fs := flag.NewFlagSet("next attrs encode", flag.ContinueOnError)
	out := fs.String("o", "", "output attribute table")
	fixed5 := fs.Bool("fixed5", false, "five bytes for every sprite")
	if err := fs.Parse(permuteArgs(args, map[string]bool{"fixed5": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("attrs encode needs -o file and exactly one JSON file")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	var attrs []zxn.SpriteAttr
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&attrs); err != nil {
		return fmt.Errorf("%s: %w", fs.Arg(0), err)
	}
	b, err := zxn.EncodeAttrs(attrs, *fixed5)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, b, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d sprites, %d bytes)\n", *out, len(attrs), len(b))
	return nil
}

func nextAttrsDecode(args []string) error {
	fs := flag.NewFlagSet("next attrs decode", flag.ContinueOnError)
	out := fs.String("o", "", "output JSON (default: standard output)")
	fixed5 := fs.Bool("fixed5", false, "five bytes for every sprite")
	if err := fs.Parse(permuteArgs(args, map[string]bool{"fixed5": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("attrs decode needs exactly one attribute file")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	attrs, err := zxn.DecodeAttrs(data, *fixed5)
	if err != nil {
		return fmt.Errorf("%s: %w", fs.Arg(0), err)
	}
	js, err := json.MarshalIndent(attrs, "", "  ")
	if err != nil {
		return err
	}
	js = append(js, '\n')
	if *out == "" {
		_, err := os.Stdout.Write(js)
		return err
	}
	if err := os.WriteFile(*out, js, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d sprites)\n", *out, len(attrs))
	return nil
}

// --- Layer 2 and LoRes ------------------------------------------------------

func bitmapModes() []zxn.Layer2Mode {
	return append(append([]zxn.Layer2Mode(nil), zxn.Layer2Modes...), zxn.LoResModes...)
}

func parseBitmapMode(s string) (zxn.Layer2Mode, error) {
	for _, m := range bitmapModes() {
		if m.Name == strings.ToLower(strings.TrimSpace(s)) {
			return m, nil
		}
	}
	return zxn.Layer2Mode{}, fmt.Errorf("unknown bitmap mode %q (want 256x192, 320x256, 640x256, lores or lores4)", s)
}

// bitmapOfSize finds the mode and embedded palette size of a file of n
// bytes, preferring want when it fits.
func bitmapOfSize(n int, want string) (zxn.Layer2Mode, int, error) {
	pals := []int{0, 512, 32, 256, 16}
	if want != "" {
		m, err := parseBitmapMode(want)
		if err != nil {
			return m, 0, err
		}
		for _, p := range pals {
			if n == m.Bytes()+p {
				return m, p, nil
			}
		}
		return m, 0, fmt.Errorf("%d bytes is not a %s bitmap (%d bytes), with or without a palette", n, m.Name, m.Bytes())
	}
	for _, m := range bitmapModes() {
		for _, p := range pals {
			if n == m.Bytes()+p {
				return m, p, nil
			}
		}
	}
	return zxn.Layer2Mode{}, 0, fmt.Errorf("%d bytes is not the size of a Layer 2 or LoRes screen", n)
}

func nextBitmapInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("bitmap info needs exactly one file")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	m, pal, err := bitmapOfSize(len(data), "")
	if err != nil {
		return fmt.Errorf("%s: %w", args[0], err)
	}
	order := "row by row"
	if m.Order == zxn.ColumnMajor {
		order = "column by column"
	}
	fmt.Printf("%s: %s, %dx%d at %d bits per pixel, %s\n", args[0], m.Name, m.Width, m.Height, m.Depth, order)
	if m.Name == "320x256" {
		fmt.Println("  (640x256 at 4 bits is the same size; decode with -mode 640x256 for that)")
	}
	if pal > 0 {
		fmt.Printf("  palette of %d bytes embedded before the pixels\n", pal)
	} else {
		fmt.Println("  no embedded palette")
	}
	return nil
}

func nextBitmapDecode(args []string) error {
	fs := flag.NewFlagSet("next bitmap decode", flag.ContinueOnError)
	out := fs.String("o", "", "output PNG (default: input base name + .png)")
	mode := fs.String("mode", "", "256x192, 320x256, 640x256, lores or lores4 (default: from the size)")
	palPath := fs.String("palette", "", "palette to draw with (default: the embedded one, else grey levels)")
	layout := fs.String("layout", "rgb333", "layout of -palette and of an embedded palette")
	transparent := fs.String("transparent", "", "palette index drawn transparent")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("bitmap decode needs exactly one file")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	m, palBytes, err := bitmapOfSize(len(data), *mode)
	if err != nil {
		return fmt.Errorf("%s: %w", fs.Arg(0), err)
	}
	b, err := zxn.DecodeNXI(data[palBytes:], m.Width, m.Height, m.Depth, m.Order)
	if err != nil {
		return err
	}
	pal := greyPalette(m.Depth)
	switch {
	case *palPath != "":
		if pal, err = readPaletteFile(*palPath, *layout); err != nil {
			return err
		}
	case palBytes > 0:
		l, err := zxn.ParseLayout(*layout)
		if err != nil {
			return err
		}
		if palBytes == 256 || palBytes == 16 {
			l = zxn.LayoutRGB332
		}
		if pal, err = zxn.DecodePalette(data[:palBytes], l); err != nil {
			return err
		}
	}
	tr, err := parseIndex(*transparent, "-transparent", 255)
	if err != nil {
		return err
	}
	path := *out
	if path == "" {
		path = scrOutBase(fs.Arg(0)) + ".png"
	}
	if err := writePNGFile(path, zxn.BitmapImage(b, pal, tr)); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%s, %dx%d)\n", path, m.Name, m.Width, m.Height)
	return nil
}

// fitImage brings img to w x h, keeping it untouched (and so indexed) when
// it is already that size.
func fitImage(img image.Image, resize string, w, h int) (image.Image, error) {
	if b := img.Bounds(); b.Dx() == w && b.Dy() == h {
		return img, nil
	}
	mode, err := parseResizeMode(resize)
	if err != nil {
		return nil, err
	}
	out, err := scr.FitTo(img, mode, color.Black, w, h)
	if err != nil {
		return nil, fmt.Errorf("%w (give -resize stretch, bestfit, centre or fill)", err)
	}
	return out, nil
}

func nextBitmapEncode(args []string) error {
	fs := flag.NewFlagSet("next bitmap encode", flag.ContinueOnError)
	out := fs.String("o", "", "output .nxi")
	mode := fs.String("mode", "", "256x192, 320x256, 640x256, lores or lores4")
	embed := fs.Bool("embed", false, "write the palette before the pixels")
	resize := fs.String("resize", "", "stretch, bestfit, centre or fill, for an image of another size")
	var cf colourFlags
	cf.register(fs)
	if err := fs.Parse(permuteArgs(args, mergeBools(colourBools, map[string]bool{"embed": true}))); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" || *mode == "" {
		return fmt.Errorf("bitmap encode needs -mode, -o file and exactly one image")
	}
	m, err := parseBitmapMode(*mode)
	if err != nil {
		return err
	}
	img, err := readImageFile(fs.Arg(0))
	if err != nil {
		return err
	}
	if img, err = fitImage(img, *resize, m.Width, m.Height); err != nil {
		return err
	}
	pi, pal, err := cf.index(img, m.Depth)
	if err != nil {
		return err
	}
	b, err := zxn.BitmapFromImage(pi, nil, m.Depth, m.Order, cf.mode)
	if err != nil {
		return err
	}
	data, err := zxn.EncodeNXI(b)
	if err != nil {
		return err
	}
	if *embed {
		l, err := zxn.ParseLayout(cf.layout)
		if err != nil {
			return err
		}
		pb, err := zxn.EncodePalette(pal, l, cf.mode)
		if err != nil {
			return err
		}
		data = append(pb, data...)
	}
	if err := os.WriteFile(*out, data, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%s, %d bytes)\n", *out, m.Name, len(data))
	return cf.writePalette(pal)
}

// --- Timex screens ----------------------------------------------------------

func nextTimexDecode(args []string) error {
	fs := flag.NewFlagSet("next timex decode", flag.ContinueOnError)
	out := fs.String("o", "", "output PNG (default: input base name + .png)")
	mode := fs.String("mode", "", "hires or hicolour")
	ink := fs.Int("ink", 0, "HiRes ink colour 0-7 (paper is its complement)")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("timex decode needs exactly one file")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	var img image.Image
	switch strings.ToLower(*mode) {
	case "hires":
		h, err := zxn.DecodeHiRes(data, uint8(*ink))
		if err != nil {
			return err
		}
		img = h.Image()
	case "hicolour", "hicolor":
		h, err := zxn.DecodeHiColour(data)
		if err != nil {
			return err
		}
		img = h.Image()
	default:
		return fmt.Errorf("timex decode: -mode must be hires or hicolour (the two are the same size)")
	}
	path := *out
	if path == "" {
		path = scrOutBase(fs.Arg(0)) + ".png"
	}
	if err := writePNGFile(path, img); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", path)
	return nil
}

func nextTimexEncode(args []string) error {
	fs := flag.NewFlagSet("next timex encode", flag.ContinueOnError)
	out := fs.String("o", "", "output file")
	mode := fs.String("mode", "", "hires or hicolour")
	ink := fs.Int("ink", 0, "HiRes ink colour 0-7")
	resize := fs.String("resize", "", "stretch, bestfit, centre or fill, for an image of another size")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("timex encode needs -mode, -o file and exactly one image")
	}
	img, err := readImageFile(fs.Arg(0))
	if err != nil {
		return err
	}
	var data []byte
	switch strings.ToLower(*mode) {
	case "hires":
		if *ink < 0 || *ink > 7 {
			return fmt.Errorf("timex encode: -ink must be 0-7")
		}
		if img, err = fitImage(img, *resize, 512, 192); err != nil {
			return err
		}
		h, err := zxn.HiResFromImage(img, uint8(*ink))
		if err != nil {
			return err
		}
		data = h.Bytes()
	case "hicolour", "hicolor":
		if img, err = fitImage(img, *resize, 256, 192); err != nil {
			return err
		}
		h, err := zxn.HiColourFromImage(img)
		if err != nil {
			return err
		}
		data = h.Bytes()
	default:
		return fmt.Errorf("timex encode: -mode must be hires or hicolour")
	}
	if err := os.WriteFile(*out, data, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d bytes)\n", *out, len(data))
	return nil
}

// --- tiles ----------------------------------------------------------------------

func parseSize(s, what string) (int, int, error) {
	a, b, ok := strings.Cut(strings.ToLower(s), "x")
	w, err1 := strconv.Atoi(a)
	h, err2 := strconv.Atoi(b)
	if !ok || err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("%s %q: want WxH", what, s)
	}
	return w, h, nil
}

func nextTilesEncode(args []string) error {
	fs := flag.NewFlagSet("next tiles encode", flag.ContinueOnError)
	out := fs.String("o", "", "output base name (default: the input's)")
	size := fs.String("size", "8x8", "tile size")
	depth := fs.Int("depth", 8, "8, 4 or 1")
	dedup := fs.String("dedup", "none", "none, repeat, mirror or rotate")
	map16 := fs.Bool("map16", false, "two bytes per map entry, with attributes")
	mapCols := fs.Bool("map-columns", false, "write the map column by column")
	offset := fs.Int("offset", 0, "add this to every tile index in the map")
	palOffset := fs.Int("pal-offset", 0, "palette offset 0-15 for every map entry")
	block := fs.String("block", "", "group tiles into blocks of WxH tiles")
	blockDedup := fs.Bool("block-dedup", false, "drop repeated blocks")
	block16 := fs.Bool("block16", false, "two bytes per tile index in blocks")
	noMap := fs.Bool("no-map", false, "write no map")
	var cf colourFlags
	cf.register(fs)
	bools := mergeBools(colourBools, map[string]bool{"map16": true, "map-columns": true, "block-dedup": true, "block16": true, "no-map": true})
	if err := fs.Parse(permuteArgs(args, bools)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("tiles encode needs exactly one image")
	}
	tw, th, err := parseSize(*size, "-size")
	if err != nil {
		return err
	}
	d, err := zxn.ParseDedup(*dedup)
	if err != nil {
		return err
	}
	opt := zxn.TileOptions{Width: tw, Height: th, Depth: *depth, Dedup: d, Offset: *offset, PaletteOffset: *palOffset, BlockDedup: *blockDedup}
	if *block != "" {
		if opt.BlockWidth, opt.BlockHeight, err = parseSize(*block, "-block"); err != nil {
			return err
		}
	}
	img, err := readImageFile(fs.Arg(0))
	if err != nil {
		return err
	}
	var pi *image.Paletted
	var pal *zxn.Palette
	if *depth == 1 {
		var ok bool
		if pi, ok = img.(*image.Paletted); !ok {
			pi = paletted(img, &zxn.Palette{Colors: []zxn.Color333{{}, {R: 7, G: 7, B: 7}}}, func(c color.Color) uint8 {
				r, g, b, _ := c.RGBA()
				if (299*(r>>8)+587*(g>>8)+114*(b>>8))/1000 < 0x80 {
					return 1
				}
				return 0
			})
		}
		if cf.palette != "" {
			return fmt.Errorf("tiles encode: 1-bit tiles have no palette to write")
		}
	} else {
		// At depth 4 tiles keep the high nibble of each index as the map's
		// palette offset, so index as 8 bits and let CutTiles split them.
		if _, indexed := img.(*image.Paletted); *depth == 4 && !indexed && (cf.colors == 0 || cf.colors > 16) {
			return fmt.Errorf("tiles encode: a 4-bit tile set from an image that is not indexed needs -colors 16 or fewer (or an indexed image in banks of 16)")
		}
		if pi, pal, err = cf.index(img, 8); err != nil {
			return err
		}
	}
	ts, tm, bs, err := zxn.CutTiles(pi, opt)
	if err != nil {
		return err
	}
	base := *out
	if base == "" {
		base = scrOutBase(fs.Arg(0))
	}
	nxt, err := zxn.EncodeNXT(ts)
	if err != nil {
		return err
	}
	if err := os.WriteFile(base+".nxt", nxt, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %s.nxt (%d tiles of %dx%d at %d bits, %d bytes)\n", base, len(ts.Tiles), tw, th, *depth, len(nxt))
	if bs != nil {
		nxb, err := zxn.EncodeNXB(bs, *block16)
		if err != nil {
			return err
		}
		if err := os.WriteFile(base+".nxb", nxb, 0644); err != nil {
			return err
		}
		fmt.Printf("wrote %s.nxb (%d blocks, %d bytes)\n", base, len(bs.Blocks), len(nxb))
	}
	if !*noMap {
		nxm, err := zxn.EncodeNXM(tm, *map16, *mapCols)
		if err != nil {
			return err
		}
		if err := os.WriteFile(base+".nxm", nxm, 0644); err != nil {
			return err
		}
		fmt.Printf("wrote %s.nxm (%dx%d, %d bytes)\n", base, tm.Width, tm.Height, len(nxm))
	}
	if pal != nil {
		if *depth == 4 {
			size := 16
			if cf.colors > 16 || cf.colors == 0 {
				size = 256 // banks of 16 for the palette offsets
			}
			pal = pal.Padded(size)
			pal.Colors = pal.Colors[:size]
		}
		return cf.writePalette(pal)
	}
	return nil
}

func nextTilesDecode(args []string) error {
	fs := flag.NewFlagSet("next tiles decode", flag.ContinueOnError)
	out := fs.String("o", "", "output PNG (default: input base name + .png)")
	size := fs.String("size", "8x8", "tile size")
	depth := fs.Int("depth", 8, "8, 4 or 1")
	mapPath := fs.String("map", "", "tilemap to draw (default: draw the tiles as a sheet)")
	mapSize := fs.String("map-size", "", "the map's size in tiles, WxH")
	map16 := fs.Bool("map16", false, "two bytes per map entry")
	mapCols := fs.Bool("map-columns", false, "the map is stored column by column")
	offset := fs.Int("offset", 0, "the tile offset the map was written with")
	palPath := fs.String("palette", "", "palette to draw with")
	layout := fs.String("layout", "rgb333", "layout of -palette")
	perRow := fs.Int("per-row", 16, "tiles per row of a sheet")
	if err := fs.Parse(permuteArgs(args, map[string]bool{"map16": true, "map-columns": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("tiles decode needs exactly one .nxt file")
	}
	tw, th, err := parseSize(*size, "-size")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	ts, err := zxn.DecodeNXT(data, tw, th, *depth)
	if err != nil {
		return err
	}
	pal := greyPalette(*depth)
	if *depth == 4 && *mapPath != "" {
		pal = greyPalette(8)
	}
	if *palPath != "" {
		if pal, err = readPaletteFile(*palPath, *layout); err != nil {
			return err
		}
	}
	var img image.Image
	if *mapPath == "" {
		img = zxn.TilesImage(ts, pal, *perRow)
	} else {
		mw, mh, err := parseSize(*mapSize, "-map-size")
		if err != nil {
			return err
		}
		md, err := os.ReadFile(*mapPath)
		if err != nil {
			return err
		}
		m, err := zxn.DecodeNXM(md, mw, mh, *map16, *mapCols)
		if err != nil {
			return err
		}
		img = zxn.MapImage(ts, m, pal, *offset)
	}
	path := *out
	if path == "" {
		path = scrOutBase(fs.Arg(0)) + ".png"
	}
	if err := writePNGFile(path, img); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d tiles)\n", path, len(ts.Tiles))
	return nil
}
