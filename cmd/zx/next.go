// file: next.go
//
// The `zx next` subcommand handles ZX Spectrum Next graphics and program
// files through pkg/zxn and pkg/nex: palettes, hardware sprites and their
// attributes, Layer 2 and LoRes bitmaps, Timex screens, tiles and tilemaps,
// and NEX files. This file holds the dispatcher, the shared colour handling,
// palettes and sprites; next_bitmap.go, next_tiles.go and next_nex.go hold
// the rest.

package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ha1tch/zentools/pkg/scr"
	"github.com/ha1tch/zentools/pkg/zxn"
)

func nextUsage() {
	fmt.Fprint(os.Stderr, `zx next - ZX Spectrum Next graphics and NEX files

Usage:
  zx next palette info    [-layout l] <file>
  zx next palette decode  [-layout l] [-o file.png] <file>      draw a palette as swatches
  zx next palette encode  [colour flags] -o file.nxp <image>   take or choose an image's palette
  zx next palette convert -to l [-from l] [-round m] -o out <file>
  zx next palette std     [-layout l] -o file.nxp              the Next's standard palette
  zx next sprite  info    [-depth 8|4] <file.spr>
  zx next sprite  decode  [flags] -o file.png <file.spr>       draw patterns as a sheet or files
  zx next sprite  encode  [flags] -o file.spr <image>          cut an image into 16x16 patterns
  zx next attrs   encode  [-fixed5] -o file.bin <file.json>    sprite attribute table
  zx next attrs   decode  [-fixed5] [-o file.json] <file.bin>
  zx next bitmap  info    <file.nxi>
  zx next bitmap  decode  [flags] -o file.png <file.nxi>       Layer 2 and LoRes screens
  zx next bitmap  encode  -mode m [flags] -o file.nxi <image>
  zx next timex   decode  -mode hires|hicolour [-ink n] -o file.png <file>
  zx next timex   encode  -mode hires|hicolour [-ink n] [-resize r] -o file <image>
  zx next tiles   encode  [flags] -o base <image>              writes base.nxt, base.nxm (.nxb)
  zx next tiles   decode  -size WxH [flags] -o file.png <file.nxt>
  zx next nex     info    <file.nex>
  zx next nex     extract -o dir <file.nex>
  zx next nex     build   [flags] -o file.nex

Run any of them with -h for its flags. Palette layouts (-layout, -from, -to) are rgb333 (the
2-byte .nxp, the default), rgb332 (1 byte), gbr444 (2 bytes) and bgr222 (1 byte).

Colour flags, for palette, sprite, bitmap and tiles encode:
  -colors <n>        palette size (default 256, or 16 at 4 bits per pixel)
  -round <mode>      nearest (default), floor, ceil or distance (gfx2next's default):
                     how 8-bit channels become 3-bit
  -dither <d>        none (default) or fs (Floyd-Steinberg), when colours must be chosen
  -transparent <n>   pixels less than half opaque take index n (the Next's default is 0xE3),
                     whose entry holds that index's RGB332 colour
  -match <file>      map the image's colours to the nearest of this palette's (dithered as
                     -dither says) instead of choosing colours
  -match-layout <l>  the layout of the -match palette
  -std               map colours onto the standard palette (8 bits only; gfx2next -pal-std)
  -min               sort the palette, drop duplicates and renumber the pixels
                     (gfx2next -pal-min)
  -palette <file>    write the palette here, in -layout's layout
  -layout <l>        layout of the written palette

An indexed image (an indexed PNG or BMP) keeps its own palette in its own order, and its
pixel indices are used as they are, as gfx2next does; at 4 bits the palette is its first 16
colours and an index its low four bits. Any other image is matched to -match, mapped to the
standard palette (-std), or reduced: kept exactly when it has few enough RGB333 colours, in
the order they first appear, and otherwise reduced by median cut. Images may be PNG, BMP,
GIF or JPEG.
`)
}

func cmdNext(args []string) error {
	if len(args) < 2 {
		nextUsage()
		if len(args) == 0 {
			return nil
		}
		return fmt.Errorf("next %s needs a subcommand", args[0])
	}
	rest := args[2:]
	switch args[0] + " " + args[1] {
	case "palette info":
		return nextPaletteInfo(rest)
	case "palette decode":
		return nextPaletteDecode(rest)
	case "palette encode":
		return nextPaletteEncode(rest)
	case "palette convert":
		return nextPaletteConvert(rest)
	case "palette std":
		return nextPaletteStd(rest)
	case "sprite info":
		return nextSpriteInfo(rest)
	case "sprite decode":
		return nextSpriteDecode(rest)
	case "sprite encode":
		return nextSpriteEncode(rest)
	case "attrs encode":
		return nextAttrsEncode(rest)
	case "attrs decode":
		return nextAttrsDecode(rest)
	case "bitmap info":
		return nextBitmapInfo(rest)
	case "bitmap decode":
		return nextBitmapDecode(rest)
	case "bitmap encode":
		return nextBitmapEncode(rest)
	case "timex decode":
		return nextTimexDecode(rest)
	case "timex encode":
		return nextTimexEncode(rest)
	case "tiles encode":
		return nextTilesEncode(rest)
	case "tiles decode":
		return nextTilesDecode(rest)
	case "nex info":
		return nextNexInfo(rest)
	case "nex extract":
		return nextNexExtract(rest)
	case "nex build":
		return nextNexBuild(rest)
	}
	nextUsage()
	return fmt.Errorf("unknown command: next %s %s", args[0], args[1])
}

// --- shared helpers ------------------------------------------------------------

func writePNGFile(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func readImageFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := scr.DecodeImage(f)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", path, err)
	}
	return img, nil
}

// readPaletteFile reads a palette in a layout as Next colours (the 4- and
// 2-bit layouts reduced to the nearest RGB333).
func readPaletteFile(path, layout string) (*zxn.Palette, error) {
	l, err := zxn.ParseLayout(layout)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if l == zxn.LayoutRGB333 || l == zxn.LayoutRGB332 {
		p, err := zxn.DecodePalette(data, l)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return p, nil
	}
	cs, err := zxn.DecodePaletteRGBA(data, l)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	p := &zxn.Palette{}
	for _, c := range cs {
		p.Colors = append(p.Colors, zxn.ToRGB333(c, zxn.RoundNearest))
	}
	return p, nil
}

func writePaletteFile(path string, p *zxn.Palette, layout string, mode zxn.RoundMode) error {
	l, err := zxn.ParseLayout(layout)
	if err != nil {
		return err
	}
	data, err := zxn.EncodePalette(p, l, mode)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d colours, %s, %d bytes)\n", path, len(p.Colors), l, len(data))
	return nil
}

func parseIndex(s, what string, max int) (int, error) {
	if strings.TrimSpace(s) == "" {
		return -1, nil
	}
	v, err := strconv.ParseInt(strings.TrimSpace(s), 0, 32)
	if err != nil || v < 0 || int(v) > max {
		return 0, fmt.Errorf("%s must be 0-%d", what, max)
	}
	return int(v), nil
}

// greyPalette shows indices as grey levels when no palette is given.
func greyPalette(depth int) *zxn.Palette {
	n := 1 << uint(depth)
	p := &zxn.Palette{}
	for i := 0; i < n; i++ {
		v := uint8(i * 8 / n)
		p.Colors = append(p.Colors, zxn.Color333{R: v, G: v, B: v})
	}
	return p
}

// colourFlags are the flags every encoder shares for turning an image into
// palette indices.
type colourFlags struct {
	colors                     int
	round, dither, transparent string
	match, matchLayout         string
	palette, layout            string
	std, min                   bool
	mode                       zxn.RoundMode
}

var colourBools = map[string]bool{"std": true, "min": true}

func (c *colourFlags) register(fs *flag.FlagSet) {
	fs.IntVar(&c.colors, "colors", 0, "palette size (default 256, or 16 at 4 bits)")
	fs.StringVar(&c.round, "round", "nearest", "nearest, floor, ceil or distance")
	fs.StringVar(&c.dither, "dither", "none", "none or fs")
	fs.StringVar(&c.transparent, "transparent", "", "index for transparent pixels")
	fs.StringVar(&c.match, "match", "", "palette to match colours against")
	fs.StringVar(&c.matchLayout, "match-layout", "rgb333", "layout of the -match palette")
	fs.StringVar(&c.palette, "palette", "", "write the palette here")
	fs.StringVar(&c.layout, "layout", "rgb333", "layout of the written palette")
	fs.BoolVar(&c.std, "std", false, "use the standard palette")
	fs.BoolVar(&c.min, "min", false, "minimise the palette")
}

func mergeBools(maps ...map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// index turns an image into palette indices at a depth (8 or 4), returning
// the indexed image and its palette (of the -colors size). It reports what
// it did on stdout when it chose colours.
func (c *colourFlags) index(img image.Image, depth int) (*image.Paletted, *zxn.Palette, error) {
	var err error
	if c.mode, err = zxn.ParseRoundMode(c.round); err != nil {
		return nil, nil, err
	}
	size := c.colors
	if size == 0 {
		size = 256
		if depth == 4 {
			size = 16
		}
	}
	if size < 2 || size > 256 || (depth == 4 && size > 16) {
		return nil, nil, fmt.Errorf("-colors %d does not fit %d bits per pixel", size, depth)
	}
	tr, err := parseIndex(c.transparent, "-transparent", size-1)
	if err != nil {
		return nil, nil, err
	}
	pi, indexed := img.(*image.Paletted)
	var pal *zxn.Palette
	switch {
	case c.std:
		if depth != 8 {
			return nil, nil, fmt.Errorf("-std is for 8-bit images")
		}
		std := zxn.StandardPalette()
		if indexed {
			pi = zxn.RemapImage(pi, zxn.StandardTable(pi.Palette, c.mode), std)
		} else {
			pi = paletted(img, std, func(col color.Color) uint8 { return zxn.StandardIndex(col, c.mode) })
		}
		pal = std
	case indexed:
		if pal, err = zxn.PaletteFromImage(pi, zxn.MaxColors, c.mode); err != nil {
			return nil, nil, err
		}
	case c.match != "":
		if pal, err = readPaletteFile(c.match, c.matchLayout); err != nil {
			return nil, nil, err
		}
		if len(pal.Colors) > size {
			pal.Colors = pal.Colors[:size]
		}
		d, err := zxn.ParseDither(c.dither)
		if err != nil {
			return nil, nil, err
		}
		if pi, pal, err = zxn.Reduce(img, zxn.ReduceOptions{Palette: pal, Round: c.mode, Dither: d, Transparent: tr}); err != nil {
			return nil, nil, err
		}
	default:
		d, err := zxn.ParseDither(c.dither)
		if err != nil {
			return nil, nil, err
		}
		if pi, pal, err = zxn.Reduce(img, zxn.ReduceOptions{Colors: size, Round: c.mode, Dither: d, Transparent: tr}); err != nil {
			return nil, nil, err
		}
	}
	if c.min && tr >= 0 {
		return nil, nil, fmt.Errorf("-min would move the -transparent entry; use one or the other")
	}
	if c.min && !c.std {
		min, n, table := zxn.Minimize(pal.Padded(zxn.MaxColors))
		fmt.Printf("minimised palette: %d distinct colours\n", n)
		if n > size {
			fmt.Printf("warning: %d colours do not fit %d; pixels of the others become index 0\n", n, size)
		}
		for i, v := range table {
			if int(v) >= size {
				table[i] = 0
			}
		}
		pal = min
		pi = zxn.RemapImage(pi, table, pal)
	}
	if len(pal.Colors) > size {
		pal = &zxn.Palette{Colors: pal.Colors[:size]}
	}
	return pi, pal.Padded(size), nil
}

// writePalette writes the palette if -palette was given.
func (c *colourFlags) writePalette(p *zxn.Palette) error {
	if c.palette == "" {
		return nil
	}
	return writePaletteFile(c.palette, p, c.layout, c.mode)
}

func paletted(img image.Image, pal *zxn.Palette, f func(color.Color) uint8) *image.Paletted {
	b := img.Bounds()
	out := image.NewPaletted(image.Rect(0, 0, b.Dx(), b.Dy()), cpalette(pal))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.SetColorIndex(x, y, f(img.At(b.Min.X+x, b.Min.Y+y)))
		}
	}
	return out
}

func cpalette(p *zxn.Palette) color.Palette {
	cp := make(color.Palette, len(p.Colors))
	for i, c := range p.Colors {
		cp[i] = c.RGBA()
	}
	if len(cp) == 0 {
		cp = color.Palette{color.Black}
	}
	return cp
}

// --- palettes ------------------------------------------------------------------

func nextPaletteInfo(args []string) error {
	fs := flag.NewFlagSet("next palette info", flag.ContinueOnError)
	layout := fs.String("layout", "rgb333", "palette layout")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("palette info needs exactly one palette file")
	}
	l, err := zxn.ParseLayout(*layout)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	if l == zxn.LayoutRGB333 || l == zxn.LayoutRGB332 {
		p, err := zxn.DecodePalette(data, l)
		if err != nil {
			return fmt.Errorf("%s: %w", fs.Arg(0), err)
		}
		fmt.Printf("%s: %d colours (%s)\n", fs.Arg(0), len(p.Colors), l)
		for i, c := range p.Colors {
			rgb := c.RGBA()
			pri := ""
			if c.Priority {
				pri = "  priority"
			}
			fmt.Printf("%3d  RGB333 %d%d%d  #%02X%02X%02X%s\n", i, c.R, c.G, c.B, rgb.R, rgb.G, rgb.B, pri)
		}
		return nil
	}
	cs, err := zxn.DecodePaletteRGBA(data, l)
	if err != nil {
		return fmt.Errorf("%s: %w", fs.Arg(0), err)
	}
	fmt.Printf("%s: %d colours (%s)\n", fs.Arg(0), len(cs), l)
	for i, c := range cs {
		fmt.Printf("%3d  #%02X%02X%02X\n", i, c.R, c.G, c.B)
	}
	return nil
}

func nextPaletteDecode(args []string) error {
	fs := flag.NewFlagSet("next palette decode", flag.ContinueOnError)
	out := fs.String("o", "", "output PNG (default: input base name + .png)")
	layout := fs.String("layout", "rgb333", "palette layout")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("palette decode needs exactly one palette file")
	}
	p, err := readPaletteFile(fs.Arg(0), *layout)
	if err != nil {
		return err
	}
	path := *out
	if path == "" {
		path = scrOutBase(fs.Arg(0)) + ".png"
	}
	if err := writePNGFile(path, zxn.PaletteImage(p)); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d colours)\n", path, len(p.Colors))
	return nil
}

func nextPaletteEncode(args []string) error {
	fs := flag.NewFlagSet("next palette encode", flag.ContinueOnError)
	out := fs.String("o", "", "output palette file")
	var cf colourFlags
	cf.register(fs)
	if err := fs.Parse(permuteArgs(args, colourBools)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("palette encode needs -o file and exactly one image")
	}
	if cf.colors != 0 && cf.colors != 256 && cf.colors != 16 {
		return fmt.Errorf("palette encode: -colors must be 256 or 16")
	}
	img, err := readImageFile(fs.Arg(0))
	if err != nil {
		return err
	}
	depth := 8
	if cf.colors == 16 {
		depth = 4
	}
	cf.palette = ""
	_, pal, err := cf.index(img, depth)
	if err != nil {
		return err
	}
	return writePaletteFile(*out, pal, cf.layout, cf.mode)
}

func nextPaletteConvert(args []string) error {
	fs := flag.NewFlagSet("next palette convert", flag.ContinueOnError)
	out := fs.String("o", "", "output palette file")
	from := fs.String("from", "rgb333", "layout of the input")
	to := fs.String("to", "", "layout of the output")
	round := fs.String("round", "nearest", "nearest, floor, ceil or distance")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" || *to == "" {
		return fmt.Errorf("palette convert needs -to layout, -o file and exactly one palette file")
	}
	mode, err := zxn.ParseRoundMode(*round)
	if err != nil {
		return err
	}
	p, err := readPaletteFile(fs.Arg(0), *from)
	if err != nil {
		return err
	}
	return writePaletteFile(*out, p, *to, mode)
}

func nextPaletteStd(args []string) error {
	fs := flag.NewFlagSet("next palette std", flag.ContinueOnError)
	out := fs.String("o", "", "output palette file")
	layout := fs.String("layout", "rgb333", "layout of the output")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 0 || *out == "" {
		return fmt.Errorf("palette std needs -o file and no other argument")
	}
	return writePaletteFile(*out, zxn.StandardPalette(), *layout, zxn.RoundNearest)
}

// --- sprites -------------------------------------------------------------------

func readSPR(path string, depth int) (*zxn.SpriteSheet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s, err := zxn.DecodeSPR(data, depth)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

func nextSpriteInfo(args []string) error {
	fs := flag.NewFlagSet("next sprite info", flag.ContinueOnError)
	depth := fs.Int("depth", 8, "8 or 4")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("sprite info needs exactly one .spr file")
	}
	s, err := readSPR(fs.Arg(0), *depth)
	if err != nil {
		return err
	}
	fmt.Printf("%s: %d patterns of 16x16 at %d bits per pixel\n", fs.Arg(0), len(s.Sprites), s.Depth)
	for i, sp := range s.Sprites {
		used := map[uint8]bool{}
		for _, v := range sp.Indices {
			used[v] = true
		}
		fmt.Printf("%3d  %d colours used\n", i, len(used))
	}
	return nil
}

// parseRange reads "a-b" or "a" as an inclusive range of pattern numbers.
func parseRange(s string, n int) (int, int, error) {
	if s == "" {
		return 0, n - 1, nil
	}
	lo, hi, found := strings.Cut(s, "-")
	a, err1 := strconv.Atoi(strings.TrimSpace(lo))
	b := a
	var err2 error
	if found {
		b, err2 = strconv.Atoi(strings.TrimSpace(hi))
	}
	if err1 != nil || err2 != nil || a < 0 || b < a || b >= n {
		return 0, 0, fmt.Errorf("-range %q: want a or a-b within 0-%d", s, n-1)
	}
	return a, b, nil
}

func nextSpriteDecode(args []string) error {
	fs := flag.NewFlagSet("next sprite decode", flag.ContinueOnError)
	out := fs.String("o", "", "output PNG (default: input base name + .png)")
	split := fs.String("split", "", "write each pattern as its own PNG in this directory")
	depth := fs.Int("depth", 8, "8 or 4")
	palPath := fs.String("palette", "", "palette to draw with")
	layout := fs.String("layout", "rgb333", "layout of -palette")
	transparent := fs.String("transparent", "", "palette index drawn transparent")
	perRow := fs.Int("per-row", 16, "patterns per row")
	rng := fs.String("range", "", "patterns a-b only")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("sprite decode needs exactly one .spr file")
	}
	s, err := readSPR(fs.Arg(0), *depth)
	if err != nil {
		return err
	}
	if len(s.Sprites) == 0 {
		return fmt.Errorf("%s holds no patterns", fs.Arg(0))
	}
	a, b, err := parseRange(*rng, len(s.Sprites))
	if err != nil {
		return err
	}
	first := a
	s.Sprites = s.Sprites[a : b+1]
	pal := greyPalette(*depth)
	if *palPath != "" {
		if pal, err = readPaletteFile(*palPath, *layout); err != nil {
			return err
		}
	}
	tr, err := parseIndex(*transparent, "-transparent", 255)
	if err != nil {
		return err
	}
	if *split != "" {
		if err := os.MkdirAll(*split, 0755); err != nil {
			return err
		}
		for i := range s.Sprites {
			one := &zxn.SpriteSheet{Depth: s.Depth, Sprites: s.Sprites[i : i+1]}
			path := filepath.Join(*split, fmt.Sprintf("%03d.png", first+i))
			if err := writePNGFile(path, zxn.SheetImage(one, pal, 1, tr)); err != nil {
				return err
			}
		}
		fmt.Printf("wrote %d patterns to %s\n", len(s.Sprites), *split)
		return nil
	}
	path := *out
	if path == "" {
		path = scrOutBase(fs.Arg(0)) + ".png"
	}
	if err := writePNGFile(path, zxn.SheetImage(s, pal, *perRow, tr)); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d patterns)\n", path, len(s.Sprites))
	return nil
}

func nextSpriteEncode(args []string) error {
	fs := flag.NewFlagSet("next sprite encode", flag.ContinueOnError)
	out := fs.String("o", "", "output .spr")
	depth := fs.Int("depth", 8, "8 or 4")
	dedup := fs.Bool("dedup", false, "drop patterns identical to an earlier one")
	pad := fs.Int("pad", 0, "pad the sheet to this many patterns")
	var cf colourFlags
	cf.register(fs)
	if err := fs.Parse(permuteArgs(args, mergeBools(colourBools, map[string]bool{"dedup": true}))); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("sprite encode needs -o file.spr and exactly one image")
	}
	if *depth != 8 && *depth != 4 {
		return fmt.Errorf("sprite encode: -depth must be 8 or 4")
	}
	img, err := readImageFile(fs.Arg(0))
	if err != nil {
		return err
	}
	pi, pal, err := cf.index(img, *depth)
	if err != nil {
		return err
	}
	s, err := zxn.SpritesFromImage(pi, nil, *depth, cf.mode)
	if err != nil {
		return err
	}
	if *dedup {
		var kept []zxn.Bitmap
	next:
		for i, sp := range s.Sprites {
			for j, k := range kept {
				if string(k.Indices) == string(sp.Indices) {
					fmt.Printf("pattern %d repeats kept pattern %d\n", i, j)
					continue next
				}
			}
			kept = append(kept, sp)
		}
		s.Sprites = kept
	}
	if *pad > 0 {
		max := 64
		if *depth == 4 {
			max = 128
		}
		if *pad > max || *pad < len(s.Sprites) {
			return fmt.Errorf("sprite encode: -pad %d: the sheet has %d patterns and the Next holds %d", *pad, len(s.Sprites), max)
		}
		fill := uint8(0)
		if t, _ := parseIndex(cf.transparent, "-transparent", 255); t >= 0 {
			fill = uint8(t)
		}
		for len(s.Sprites) < *pad {
			sp := zxn.Bitmap{Width: zxn.SpriteSize, Height: zxn.SpriteSize, Depth: *depth, Indices: make([]uint8, zxn.SpriteSize*zxn.SpriteSize)}
			for i := range sp.Indices {
				sp.Indices[i] = fill
			}
			s.Sprites = append(s.Sprites, sp)
		}
	}
	data, err := zxn.EncodeSPR(s)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, data, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d patterns, %d bytes)\n", *out, len(s.Sprites), len(data))
	return cf.writePalette(pal)
}
