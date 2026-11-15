// file: scr_zx.go
//
// ZXMODE pictures for `zx scr encode`, `decode`, `palette` and `compare`: the
// linear modes ZXMODE1 (256x192) and ZXMODE2 (512x384) and the ZXPAL
// palettes. Giving --mode, --palette, --registers or an encoder option selects
// this path; without them, encode and decode behave as before.
//
// Encoding uses the specification's reference encoder (scr.EncodePicture),
// optionally for loaded registers or with the experimental options of Part 4.
// Output goes by file extension: .scr for ZXMODE0 (the classic layout), .zm1
// for a ZXMODE1 screen, .zm2 for the two half-screen pages of a ZXMODE2
// picture, or .cut to append the picture to a ZCUT collection as one asset.

package main

import (
	"errors"
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
)

// zxExt is the default file extension for a picture in mode m.
func zxExt(m scr.Mode) string {
	switch m {
	case scr.ZXMODE1:
		return ".zm1"
	case scr.ZXMODE2:
		return ".zm2"
	}
	return ".scr"
}

// parseZXFlags resolves --mode and --palette; either may be empty.
func parseZXFlags(mode, palette string) (scr.Mode, scr.Palette, error) {
	m, p := scr.ZXMODE0, scr.ZXPAL0
	var err error
	if mode != "" {
		if m, err = scr.ParseMode(mode); err != nil {
			return m, p, err
		}
	}
	if palette != "" {
		if p, err = scr.ParsePalette(palette); err != nil {
			return m, p, err
		}
	}
	if m == scr.ZXMODE0 && p.TwoByte() {
		return m, p, fmt.Errorf("%s takes one-byte palettes only (zxpal0 to zxpal2); %s needs --mode zxmode1 or zxmode2", m, p)
	}
	return m, p, nil
}

// fitFlags are encode's flags for bringing a picture to the screen size.
type fitFlags struct {
	resize, fillattr, filter, centering string
	trim                                int
	grey                                bool
}

func (f *fitFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.resize, "resize", "", "resize mode: stretch | bestfit | centre | fill")
	fs.StringVar(&f.fillattr, "fillattr", "", "border attribute (CSS-like)")
	fs.StringVar(&f.filter, "filter", "", "resampler: default or lanczos")
	fs.StringVar(&f.centering, "centering", "", "crop position for fill: x,y from 0 to 1 (default 0.5,0.5)")
	fs.IntVar(&f.trim, "trim", 0, "pixels to remove from every edge first")
	fs.BoolVar(&f.grey, "grey", false, "convert to grey (luminance) after fitting")
}

// fit decodes inPath and brings it to w x h.
func (f *fitFlags) fit(inPath string, w, h int) (image.Image, error) {
	mode, err := parseResizeMode(f.resize)
	if err != nil {
		return nil, err
	}
	fill, err := scr.ParseAttribute(f.fillattr)
	if err != nil {
		return nil, err
	}
	opt := scr.FitOptions{Trim: f.trim}
	if opt.Filter, err = scr.ParseFilter(f.filter); err != nil {
		return nil, err
	}
	if f.centering != "" {
		a, b, ok := strings.Cut(f.centering, ",")
		x, e1 := strconv.ParseFloat(strings.TrimSpace(a), 64)
		y, e2 := strconv.ParseFloat(strings.TrimSpace(b), 64)
		if !ok || e1 != nil || e2 != nil || x < 0 || x > 1 || y < 0 || y > 1 {
			return nil, fmt.Errorf("-centering %q: want x,y, each from 0 to 1", f.centering)
		}
		opt.Centre, opt.CentreX, opt.CentreY = true, x, y
	}
	file, err := os.Open(inPath)
	if err != nil {
		return nil, err
	}
	img, err := scr.DecodeImage(file)
	file.Close()
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", inPath, err)
	}
	fitted, err := scr.FitToWith(img, mode, fill.PaperRGBA(), w, h, opt)
	if err != nil {
		if _, ok := err.(*scr.SizeError); ok && mode == scr.ResizeNone {
			b := img.Bounds()
			if f.trim != 0 {
				b = b.Inset(f.trim)
			}
			return nil, fmt.Errorf("%s is %dx%d, not %dx%d; choose a resize mode:\n"+
				"  --resize=stretch   scale to fill, ignoring aspect ratio\n"+
				"  --resize=bestfit   scale to fit, preserving aspect, padded border\n"+
				"  --resize=centre    no scaling; centre and crop/pad\n"+
				"  --resize=fill      scale to cover, preserving aspect, cropping the overflow",
				inPath, b.Dx(), b.Dy(), w, h)
		}
		return nil, err
	}
	if f.grey {
		fitted = scr.Grey(fitted)
	}
	return fitted, nil
}

// zxEncodeFlags are encode's ZXMODE flags.
type zxEncodeFlags struct {
	mode, palette, registers, name string
	banks, cell, penalty           string
	diffuse, flat                  float64
	linearPairs                    bool
}

func (z *zxEncodeFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&z.mode, "mode", "", "ZXMODE screen mode: zxmode0 | zxmode1 | zxmode2")
	fs.StringVar(&z.palette, "palette", "", "ZXPAL palette: zxpal0 to zxpal4")
	fs.StringVar(&z.registers, "registers", "", "register table (CSV) to encode for")
	fs.StringVar(&z.name, "name", "", "asset name when the output is a .cut (default: input base name)")
	fs.Float64Var(&z.diffuse, "diffuse", 0, "experimental: spread each cell's colour error to its neighbours, times this (0-1)")
	fs.Float64Var(&z.flat, "flat", 0, "experimental: fill cells whose pixels spread less than this with one colour")
	fs.BoolVar(&z.linearPairs, "linear-pairs", false, "experimental: choose each cell's pair in linear light")
	fs.StringVar(&z.banks, "banks", "", "encode over the colour banks in this CSV (writes a PNG)")
	fs.StringVar(&z.cell, "cell", "", "cell size WxH (default 8x8; any other writes a PNG)")
	fs.StringVar(&z.penalty, "penalty", "", "span penalty of the pair score (default 0.204, the reference one)")
}

func (z *zxEncodeFlags) used() bool {
	return z.mode != "" || z.palette != "" || z.registers != "" || z.diffuse != 0 || z.flat != 0 || z.linearPairs ||
		z.banks != "" || z.cell != "" || z.penalty != ""
}

// scrEncodeZX encodes an image as a ZXMODE picture.
func scrEncodeZX(inPath string, ff *fitFlags, z *zxEncodeFlags, out string) error {
	m, p, err := parseZXFlags(z.mode, z.palette)
	if err != nil {
		return err
	}
	if z.diffuse < 0 || z.diffuse > 1 {
		return fmt.Errorf("encode: -diffuse must be from 0 to 1")
	}
	penalty := scr.RefPenalty
	if z.penalty != "" {
		if penalty, err = strconv.ParseFloat(z.penalty, 64); err != nil || penalty < 0 {
			return fmt.Errorf("encode: -penalty %q: want a number, 0 or more", z.penalty)
		}
	}
	cw, ch := 8, 8
	if z.cell != "" {
		if cw, ch, err = scr.ParseCellSize(z.cell); err != nil {
			return err
		}
	}
	regs, err := readRegisters(z.registers, p)
	if err != nil {
		return err
	}
	w, h := m.Size()
	fitted, err := ff.fit(inPath, w, h)
	if err != nil {
		return err
	}
	if z.banks != "" || cw != 8 || ch != 8 {
		return scrEncodeBanks(inPath, fitted, z, p, regs, cw, ch, penalty, out)
	}
	var pen *float64
	if z.penalty != "" {
		pen = &penalty
	}
	pic, err := scr.EncodePictureWith(fitted, m, p, scr.EncodeOptions{Registers: regs, Penalty: pen, Diffuse: z.diffuse, FlatSpread: z.flat, LinearPairs: z.linearPairs})
	if err != nil {
		return err
	}
	outPath := out
	if outPath == "" {
		outPath = scrOutBase(inPath) + zxExt(m)
	}
	if strings.EqualFold(filepath.Ext(outPath), ".cut") {
		name := z.name
		if name == "" {
			name = scrOutBase(inPath)
		}
		col, err := loadCollection(outPath)
		if err != nil {
			return err
		}
		if col.Find(name) != nil {
			return fmt.Errorf("encode: %s already has an asset named %q", outPath, name)
		}
		col.Assets = append(col.Assets, *pic.Asset(name))
		if err := writeCollection(outPath, col); err != nil {
			return err
		}
		fmt.Printf("encoded %q (%dx%d, %s, %s) -> %s [%d assets]\n", name, pic.Width, pic.Height, m, regLabel(p, regs), outPath, len(col.Assets))
		return nil
	}
	if err := savePicture(outPath, pic); err != nil {
		return fmt.Errorf("encode: %w (or give a .cut output)", err)
	}
	fmt.Printf("wrote %s (%s, %s, %d bytes)\n", outPath, m, regLabel(p, regs), scr.PictureLen(m, p))
	return nil
}

// scrDecodeZX renders a .scr, .zm1 or .zm2 file.
func scrDecodeZX(inPath string, pf picFlags, out string, flash bool) error {
	pic, err := loadPicture(inPath, pf)
	if err != nil {
		return err
	}
	outPath := out
	if outPath == "" {
		outPath = scrOutBase(inPath) + ".png"
	}
	of, err := os.Create(outPath)
	if err != nil {
		return err
	}
	if err := png.Encode(of, pic.Render(flash)); err != nil {
		of.Close()
		return err
	}
	if err := of.Close(); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%s, %s)\n", outPath, pic.Mode, regLabel(pic.Palette, pic.Registers))
	return nil
}

// scrPalette prints a ZXPAL palette's registers, or writes them as CSV or as
// a PNG of swatches (one row per bank or level).
func scrPalette(args []string) error {
	fs := flag.NewFlagSet("scr palette", flag.ContinueOnError)
	out := fs.String("o", "", "output .csv or .png (default: a table on standard output)")
	regPath := fs.String("registers", "", "show this register table instead of the reset values")
	asBanks := fs.Bool("banks", false, "write the colour banks the encoder pairs within (for encode -banks), as CSV")
	args = permuteArgs(args, map[string]bool{"banks": true})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("palette needs exactly one palette name (zxpal0 to zxpal4)")
	}
	p, err := scr.ParsePalette(fs.Arg(0))
	if err != nil {
		return err
	}
	regs, err := readRegisters(*regPath, p)
	if err != nil {
		return err
	}
	if *asBanks {
		if !strings.EqualFold(filepath.Ext(*out), ".csv") {
			return fmt.Errorf("palette: -banks writes a CSV: give -o file.csv")
		}
		banks := scr.PaletteBanks(p, regs)
		if err := os.WriteFile(*out, scr.BanksCSV(banks), 0644); err != nil {
			return err
		}
		fmt.Printf("wrote %s (%s, %d colours in %d bank(s))\n", *out, p, banks.Len(), len(banks))
		return nil
	}
	regs = regs.Or(p)
	switch strings.ToLower(filepath.Ext(*out)) {
	case "":
		fmt.Printf("%s: %d registers, attribute %s\n", p, len(regs), map[bool]string{true: "two bytes (ink, paper)", false: "one byte"}[p.TwoByte()])
		for i, c := range regs {
			fmt.Printf("%3d  %02X%02X%02X  %3d %3d %3d\n", i, c.R, c.G, c.B, c.R, c.G, c.B)
		}
		return nil
	case ".csv":
		if err := os.WriteFile(*out, scr.RegistersCSV(regs), 0644); err != nil {
			return err
		}
	case ".png":
		per := 8
		if p.TwoByte() {
			per = 16
		}
		const sw = 32
		img := image.NewRGBA(image.Rect(0, 0, per*sw, (len(regs)+per-1)/per*sw))
		for i, c := range regs {
			x0, y0 := (i%per)*sw, (i/per)*sw
			for y := y0; y < y0+sw; y++ {
				for x := x0; x < x0+sw; x++ {
					img.SetRGBA(x, y, c)
				}
			}
		}
		if err := writePNGFile(*out, img); err != nil {
			return err
		}
	default:
		return fmt.Errorf("palette: -o must end in .csv or .png")
	}
	fmt.Printf("wrote %s (%s, %d registers)\n", *out, p, len(regs))
	return nil
}

// compareImage reads an image or renders a screen file for compare.
func compareImage(path string, pf picFlags) (image.Image, error) {
	if isScreenFile(path) {
		pic, err := loadPicture(path, pf)
		if err != nil {
			return nil, err
		}
		return pic.Render(false), nil
	}
	return readImageFile(path)
}

// scrCompare compares two images, either of which may be a screen file. By
// default it counts the pixels that differ, after doubling a 256x192 image to
// compare it with a 512x384 one (as ZXMODE1 shows on the ZXMODE2 raster), and
// exits non-zero when any does; -metric gives the specification's error
// measures instead.
func scrCompare(args []string) error {
	fs := flag.NewFlagSet("scr compare", flag.ContinueOnError)
	metric := fs.String("metric", "pixels", "pixels, rms, blur, blur=<sigma>, light (blur 0.75), ssim or all")
	var pf picFlags
	pf.register(fs, "a screen file")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("compare needs exactly two images")
	}
	var imgs [2]image.Image
	for i, path := range fs.Args() {
		img, err := compareImage(path, pf)
		if err != nil {
			return err
		}
		imgs[i] = img
	}
	a, b := imgs[0], imgs[1]
	ab, bb := a.Bounds(), b.Bounds()
	sa, sb := 1, 1
	switch {
	case ab.Dx()*2 == bb.Dx() && ab.Dy()*2 == bb.Dy():
		sa = 2
	case bb.Dx()*2 == ab.Dx() && bb.Dy()*2 == ab.Dy():
		sb = 2
	case ab.Dx() != bb.Dx() || ab.Dy() != bb.Dy():
		return fmt.Errorf("compare: sizes differ (%dx%d and %dx%d)", ab.Dx(), ab.Dy(), bb.Dx(), bb.Dy())
	}
	if sa == 2 {
		a = doubled(a)
	}
	if sb == 2 {
		b = doubled(b)
	}
	m := strings.ToLower(strings.TrimSpace(*metric))
	if m == "pixels" {
		r := a.Bounds()
		w, h := r.Dx(), r.Dy()
		diff, first := 0, ""
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r1, g1, b1, _ := a.At(r.Min.X+x, r.Min.Y+y).RGBA()
				r2, g2, b2, _ := b.At(b.Bounds().Min.X+x, b.Bounds().Min.Y+y).RGBA()
				if r1>>8 != r2>>8 || g1>>8 != g2>>8 || b1>>8 != b2>>8 {
					if diff == 0 {
						first = fmt.Sprintf(", first at (%d, %d)", x, y)
					}
					diff++
				}
			}
		}
		fmt.Printf("%d of %d pixels differ%s\n", diff, w*h, first)
		if diff != 0 {
			return errPixelsDiffer
		}
		return nil
	}
	type measure struct {
		name string
		f    func() (float64, error)
	}
	blur := func(s float64) func() (float64, error) {
		return func() (float64, error) { return scr.BlurredRMSE(a, b, s) }
	}
	var ms []measure
	switch {
	case m == "rms":
		ms = []measure{{"rms error", blur(0)}}
	case m == "blur":
		ms = []measure{{"error (blur 1.5)", blur(1.5)}}
	case m == "light":
		ms = []measure{{"lighter-blur error (blur 0.75)", blur(0.75)}}
	case strings.HasPrefix(m, "blur="):
		s, err := strconv.ParseFloat(strings.TrimPrefix(m, "blur="), 64)
		if err != nil || s < 0 {
			return fmt.Errorf("compare: -metric blur=<sigma> needs a sigma of 0 or more")
		}
		ms = []measure{{fmt.Sprintf("error (blur %g)", s), blur(s)}}
	case m == "ssim":
		ms = []measure{{"SSIM", func() (float64, error) { return scr.SSIM(a, b) }}}
	case m == "all":
		ms = []measure{{"error (blur 1.5)", blur(1.5)}, {"lighter-blur error (blur 0.75)", blur(0.75)},
			{"rms error", blur(0)}, {"SSIM", func() (float64, error) { return scr.SSIM(a, b) }}}
	default:
		return fmt.Errorf("compare: unknown -metric %q (want pixels, rms, blur, blur=<sigma>, light, ssim or all)", *metric)
	}
	for _, x := range ms {
		v, err := x.f()
		if err != nil {
			return err
		}
		if len(ms) == 1 {
			fmt.Printf("%.4f\n", v)
		} else {
			fmt.Printf("%-32s %.4f\n", x.name, v)
		}
	}
	return nil
}

// doubled scales an image up twofold by repeating pixels.
func doubled(img image.Image) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, 2*b.Dx(), 2*b.Dy()))
	for y := 0; y < 2*b.Dy(); y++ {
		for x := 0; x < 2*b.Dx(); x++ {
			out.Set(x, y, img.At(b.Min.X+x/2, b.Min.Y+y/2))
		}
	}
	return out
}

// errPixelsDiffer makes compare exit non-zero after its count.
var errPixelsDiffer = errors.New("images differ")

// zxmodeInfo summarises a .zm1 or .zm2 file for zx info: its mode, and which
// palettes its size fits (the file does not record the palette).
func zxmodeInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("info needs exactly one file")
	}
	path := args[0]
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	m, _ := scr.ParseMode(modeFromExt(path))
	w, h := m.Size()
	n := int(st.Size())
	var fits string
	switch n {
	case scr.PictureLen(m, scr.ZXPAL0):
		fits = "one attribute byte per cell: ZXPAL0, ZXPAL1 or ZXPAL2"
	case scr.PictureLen(m, scr.ZXPAL3):
		fits = "two attribute bytes per cell (ink, then paper planes): ZXPAL3 or ZXPAL4"
	default:
		return fmt.Errorf("%s: %d bytes is not a %s picture (%d or %d bytes)", path, n, m, scr.PictureLen(m, scr.ZXPAL0), scr.PictureLen(m, scr.ZXPAL3))
	}
	fmt.Printf("%s: %s picture, %dx%d, %d bytes\n", path, m, w, h, n)
	if m == scr.ZXMODE2 {
		fmt.Printf("  two half-screen pages of %d bytes (rows 0-191, then 192-383)\n", n/2)
	}
	fmt.Printf("  %s\n", fits)
	fmt.Println("  the palette is not recorded in the file: decode with -palette (and -registers for loaded registers)")
	return nil
}

// scrFit writes an image fitted to a screen size as encode would fit it, so
// the fitted picture can be inspected, or used as the reference that an
// encoded screen is compared with.
func scrFit(args []string) error {
	fs := flag.NewFlagSet("scr fit", flag.ContinueOnError)
	out := fs.String("o", "", "output PNG path (default: input base name + -fit.png)")
	size := fs.String("size", "512x384", "target size WxH, or a mode: zxmode0, zxmode1, zxmode2")
	var ff fitFlags
	ff.register(fs)
	if err := fs.Parse(permuteArgs(args, map[string]bool{"grey": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("fit needs exactly one input image")
	}
	w, h, err := parseFitSize(*size)
	if err != nil {
		return err
	}
	img, err := ff.fit(fs.Arg(0), w, h)
	if err != nil {
		return err
	}
	path := *out
	if path == "" {
		path = scrOutBase(fs.Arg(0)) + "-fit.png"
	}
	if err := writePNGFile(path, img); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%dx%d)\n", path, w, h)
	return nil
}

func parseFitSize(s string) (int, int, error) {
	if m, err := scr.ParseMode(s); err == nil {
		w, h := m.Size()
		return w, h, nil
	}
	a, b, ok := strings.Cut(strings.ToLower(s), "x")
	w, e1 := strconv.Atoi(a)
	h, e2 := strconv.Atoi(b)
	if !ok || e1 != nil || e2 != nil || w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("-size %q: want WxH or a mode", s)
	}
	return w, h, nil
}

// scrEncodeBanks encodes over colour banks, or with cells other than 8x8:
// the result is a picture to look at and measure, written as PNG.
func scrEncodeBanks(inPath string, fitted image.Image, z *zxEncodeFlags, p scr.Palette, regs scr.Registers, cw, ch int, penalty float64, out string) error {
	if z.diffuse != 0 || z.flat != 0 || z.linearPairs {
		return fmt.Errorf("encode: -diffuse, -flat and -linear-pairs work on 8x8 cells of a palette, not with -banks or -cell")
	}
	banks := scr.PaletteBanks(p, regs)
	what := regLabel(p, regs)
	if z.banks != "" {
		data, err := os.ReadFile(z.banks)
		if err != nil {
			return err
		}
		if banks, err = scr.ParseBanksCSV(data); err != nil {
			return fmt.Errorf("%s: %w", z.banks, err)
		}
		what = fmt.Sprintf("%d colours in %d bank(s) from %s", banks.Len(), len(banks), z.banks)
	}
	path := out
	if path == "" {
		path = scrOutBase(inPath) + ".png"
	}
	if !strings.EqualFold(filepath.Ext(path), ".png") {
		return fmt.Errorf("encode: with -banks or a cell other than 8x8 the output is a picture, not a screen file: give -o file.png")
	}
	img, _, err := scr.EncodeBanks(fitted, banks, scr.BankOptions{CellW: cw, CellH: ch, Penalty: penalty})
	if err != nil {
		return err
	}
	if err := writePNGFile(path, img); err != nil {
		return err
	}
	b := img.Bounds()
	fmt.Printf("wrote %s (%dx%d, %dx%d cells, %s, penalty %g)\n", path, b.Dx(), b.Dy(), cw, ch, what, penalty)
	return nil
}

// scrFitRegs fits colours to add to a one-bank palette, from pictures.
func scrFitRegs(args []string) error {
	fs := flag.NewFlagSet("scr fitregs", flag.ContinueOnError)
	out := fs.String("o", "", "output bank table (.csv; required)")
	add := fs.Int("add", 0, "number of colours to add (required)")
	palName := fs.String("palette", "zxpal4", "palette to add to: zxpal3 or zxpal4 (one bank)")
	regPath := fs.String("registers", "", "the palette's register colours (default: the reset values)")
	banksPath := fs.String("banks", "", "add to the one bank in this CSV instead of a palette")
	var opt scr.FitRegistersOptions
	fs.IntVar(&opt.PerPicture, "sample", 20000, "pixels sampled from each picture")
	fs.IntVar(&opt.Candidates, "candidates", 1500, "candidate colours drawn from the sample")
	fs.IntVar(&opt.Subsample, "subsample", 12000, "pixels the greedy choice is scored on")
	fs.IntVar(&opt.Iterations, "iterations", 25, "k-means refinement passes")
	fs.Float64Var(&opt.Blur, "blur", 1.5, "Gaussian blur before sampling")
	seed := fs.Uint64("seed", 1, "seed of the random draws")
	if err := fs.Parse(permuteArgs(args, map[string]bool{})); err != nil {
		return err
	}
	if *add < 1 || *out == "" || fs.NArg() == 0 {
		return fmt.Errorf("fitregs needs -add n, -o file.csv and one or more pictures")
	}
	opt.Add, opt.Seed = *add, *seed
	var base []color.RGBA
	if *banksPath != "" {
		data, err := os.ReadFile(*banksPath)
		if err != nil {
			return err
		}
		banks, err := scr.ParseBanksCSV(data)
		if err != nil {
			return fmt.Errorf("%s: %w", *banksPath, err)
		}
		if len(banks) != 1 {
			return fmt.Errorf("fitregs: %s has %d banks; colours are added to a one-bank table", *banksPath, len(banks))
		}
		base = banks[0]
	} else {
		p, err := scr.ParsePalette(*palName)
		if err != nil {
			return err
		}
		if !p.TwoByte() {
			return fmt.Errorf("fitregs: %s has more than one bank; colours are added to ZXPAL3, ZXPAL4 or a one-bank table (-banks)", p)
		}
		regs, err := readRegisters(*regPath, p)
		if err != nil {
			return err
		}
		base = regs.Or(p)
	}
	var pics []image.Image
	for _, path := range fs.Args() {
		img, err := readImageFile(path)
		if err != nil {
			return err
		}
		pics = append(pics, img)
	}
	added, before, after, err := scr.FitRegisters(pics, base, opt)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, scr.BanksCSV(scr.Banks{append(append([]color.RGBA{}, base...), added...)}), 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d colours: %d + %d fitted to %d picture(s))\n", *out, len(base)+len(added), len(base), len(added), len(pics))
	fmt.Printf("sample distance to the nearest colour: mean %.2f -> %.2f, share over 50: %.3f -> %.3f\n",
		before.MeanDistance, after.MeanDistance, before.FarShare, after.FarShare)
	return nil
}
