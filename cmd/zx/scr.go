// file: scr.go
//
// The `zx scr` subcommand converts ordinary images (PNG, JPEG, GIF, BMP) into
// ZX Spectrum screen files, decodes them back to PNG, and works with ZCUT
// asset collections (cut.go). Conversion reduces the image to the Spectrum's
// per-cell two-colour attribute constraint.
//
// An image that is not already 256x192 must be resized with --resize; without
// it, conversion fails and lists the available modes. The padding colour for
// the centre and bestfit modes is given as a CSS-like --fillattr.
//
// --mode and --palette select the ZXMODE screen modes and ZXPAL palettes
// (scr_zx.go).

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
)

func cmdSCR(args []string) error {
	if len(args) == 0 {
		scrUsage()
		return nil
	}
	switch args[0] {
	case "encode":
		return scrEncode(args[1:])
	case "decode":
		return scrDecode(args[1:])
	case "crop":
		return scrCrop(args[1:])
	case "cut":
		return scrCut(args[1:])
	case "paste":
		return scrPaste(args[1:])
	case "ls":
		return scrLs(args[1:])
	case "atlas":
		return scrAtlas(args[1:])
	case "fromsnap":
		return scrFromSnap(args[1:])
	case "ocr":
		return scrOCR(args[1:])
	case "palette":
		return scrPalette(args[1:])
	case "flip":
		return scrFlip(args[1:])
	case "fit":
		return scrFit(args[1:])
	case "fitregs":
		return scrFitRegs(args[1:])
	case "convert":
		return scrConvert(args[1:])
	case "compare":
		return scrCompare(args[1:])
	case "-h", "--help", "help":
		scrUsage()
		return nil
	default:
		// Bare form: `zx scr <image>` is shorthand for `zx scr encode <image>`.
		return scrEncode(args)
	}
}

func scrUsage() {
	fmt.Fprint(os.Stderr, `zx scr - Spectrum screens: classic .scr, ZXMODE .zm1/.zm2, and ZCUT asset collections

Usage:
  zx scr encode  [flags] <image>          convert PNG/JPEG/GIF/BMP to .scr (or .zm1, .zm2, .cut)
  zx scr decode  [flags] <screen>         render a .scr, .zm1 or .zm2 back to PNG
  zx scr fit     [flags] [-size WxH|zxmodeN] <image>   fit an image as encode would, to PNG
  zx scr crop    [flags] <image|screen>   crop a region (or sprite) to PNG
  zx scr ocr     [flags] <image|screen>   recognise on-screen text as plain text
  zx scr cut     [flags] -o file.cut <screen>        cut a named region into a collection
  zx scr paste   [flags] <file.cut:name> <screen>    paste an asset into a screen
  zx scr ls      <file.cut>               list a collection's assets
  zx scr atlas   [flags] <file.cut>       draw a collection's assets on one sheet
  zx scr flip    -h|-v [flags] <screen|file.cut|image>   mirror a screen, assets or an image
  zx scr convert -to <zxpalN> [flags] <file.cut|screen>  rewrite attributes for another palette
  zx scr palette [-registers f] [-banks] [-o file.csv|file.png] <zxpalN>   list a palette's registers
  zx scr fitregs -add <n> -o <file.csv> [flags] <picture>...   fit colours to add to a palette
  zx scr compare [-metric m] <a> <b>      compare two images or screens
  zx scr fromsnap [flags] <snapshot>      take the screen from a snapshot

Screens: a .scr is ZXMODE0 (the classic layout), a .zm1 ZXMODE1 (linear 256x192) and a
.zm2 ZXMODE2 (linear 512x384, two half-screen pages). The files do not record their
palette, so every command that reads one takes:
  -mode <m>          zxmode0, zxmode1 or zxmode2 (default: from the extension)
  -palette <p>       zxpal0 to zxpal4 (default zxpal0; zxpal3 and zxpal4 need zxmode1/2)
  -registers <csv>   the register colours the screen is meant for, when the program
                     loads its own (default: the palette's reset values); the CSV is the
                     one zx scr palette -o writes, or a spec data/zxpalN_registers.csv

Encode flags:
  -o <file>          output path (default: input base name + .scr, .zm1 or .zm2)
  -resize <mode>     how to fit an image of another size:
                       stretch   scale to fill, ignoring aspect ratio
                       bestfit   scale to fit, preserving aspect, padded border
                       centre    no scaling; centre and crop/pad (alias: center)
                       fill      scale to cover, preserving aspect, crop the
                                 overflow (alias: cover)
  -filter <f>        default (bilinear, or an area filter when fill shrinks) or
                     lanczos (Pillow's; -resize fill -filter lanczos is the ZXMODE
                     specification's picture fit, byte for byte)
  -centering <x,y>   where fill crops, each 0 to 1 (default 0.5,0.5; 0 keeps the
                     left or top edge, 1 the right or bottom)
  -trim <n>          remove n pixels from every edge first (a scan margin)
  -grey              convert to grey after fitting, as the specification does
                     (ITU-R 601 luminance, Pillow's convert("L"))
  -fillattr <spec>   border colour for centre/bestfit, CSS-like:
                       "ink:white; paper:blue; bright:1; flash:0"
                       colours may be names (black, blue, red, magenta, green,
                       cyan, yellow, white) or palette indices 0-7;
                       only paper is visible in a border
  -mode, -palette, -registers   as above; any of them selects the ZXMODE reference
                     encoder (with none, encode works as it always has). An output
                     ending in .cut appends the picture to a collection as one asset.
  -name <n>          asset name for a .cut output (default: input base name)
  Experimental encoder options (Part 4, section 5 of the specification; any of them
  leaves the reference encoder):
  -diffuse <f>       spread each cell's colour error into the cells after it, times f
                     (0-1); the encoding then runs on one CPU
  -flat <n>          fill cells whose pixels spread less than n with one colour
  -linear-pairs      choose each cell's pair in linear light rather than sRGB
  -penalty <p>       weight of the span term in each pair's score (default 0.204, the
                     reference one; larger favours closer pairs); the output is still a screen
  For trying palette designs (the output is a PNG, not a screen file):
  -banks <csv>       encode over these colour banks instead of a palette: columns bank, r, g,
                     b (or hex); each cell takes its two colours from one bank. zx scr palette
                     -banks writes a palette's, zx scr fitregs a fitted one
  -cell <WxH>        cell size (default 8x8; 4 means 4x4); 4x4 cells dither with the 4x4
                     Bayer matrix, other sizes with the 8x8 one laid over the whole picture

Fit flags: -resize, -filter, -centering, -trim, -grey and -fillattr, as for encode;
  -size WxH or a mode (default 512x384); -o <file.png>.

Decode flags:
  -o <file>          output PNG path (default: input base name + .png)
  -mode, -palette, -registers   as above
  -flash             draw ZXPAL0 flashing cells in their second phase

Crop flags (exactly one of --cells / --pixels / --auto):
  -cells <x,y,w,h>   crop region in 8x8 character cells
  -pixels <x,y,w,h>  crop region in pixels (w, h need not be multiples of 8)
  -auto              crop to the sprite extent: for a screen, the bounding box of
                     set bitmap bits; for an image, the non-background box
  -o <file>          output PNG path (default: input base name + "-crop.png")
  For a screen (sprite extraction):
  -bits <1|0>        which bitmap bits are the sprite (default 1; 0 = paper-on-ink)
  -with-attributes   render in the screen's own colours, opaque (default)
  -bitmap-only[=<css>]  render the shape only; paper is transparent unless the
                     attribute spec names a paper, e.g.
                     --bitmap-only="ink:cyan; paper:black"
  For image input, --auto only:
  -bg <colour>       background colour (name or index 0-7; default: inferred)
  -tol <n>           colour tolerance 0-255 (default 16)

Cut flags: -cells or -pixels (as crop), -name <n> (required), -o <file.cut>,
  -bitmap-only, -mask. A cell-aligned region keeps its attributes, in the screen's
  palette, with its register table when -registers was given.

Paste flags: -at <x,y> (required), -op or|and|copy|xor (default or), -mask <name>,
  -set-attr <css> (one-byte palettes), -o <file> (default: the target). An attributed
  asset pasted at a cell-aligned position writes its attributes too, converted to the
  target's palette and registers when they differ (see convert).

Flip flags: -h (left to right), -v (top to bottom), -name <n> (one asset of a .cut;
  default all), -o <file> (default: the input; an image is written as PNG).

Convert flags: -to <zxpalN> (required), -to-registers <csv>, -to-mode <m> (a screen
  may change between zxmode0 and zxmode1), -name <n>, -o <file>. The bitmap is kept;
  each cell takes the target colours nearest its own ink and paper, weighted by how
  many pixels each covers, within one bank for a one-byte palette.

Fitregs flags (the specification's register search, tools/z4_palette_search.py):
  -add <n>           colours to add (required)
  -palette <p>       zxpal3 or zxpal4 to add to (default zxpal4), with -registers; or
  -banks <csv>       a one-bank table to add to
  -o <file.csv>      the base colours and the new ones, as one bank (for encode -banks)
  -sample <n>        pixels drawn from each picture, after a blur of -blur (1.5) (20000)
  -candidates <n>    candidate colours drawn from the sample (1500)
  -subsample <n>     pixels the greedy choice is scored on (12000)
  -iterations <n>    k-means passes that refine the new colours (25)
  -seed <n>          seed of the random draws (1); a fit repeats exactly
  The pictures are used as they are: fit them first (zx scr fit).

Compare flags:
  -metric <m>        pixels (default: count differing pixels, exit non-zero if any),
                     blur (the specification's error: RMS after a Gaussian blur of
                     1.5, as its tools compute it), light (blur 0.75), blur=<sigma>,
                     rms (no blur), ssim (as scikit-image computes it) or all.
                     A 256x192 image is doubled to compare with a 512x384 one.

OCR flags:
  -origin <x,y>      pixel offset, within the image, of the display's
                     top-left corner (default: 0,0; not for a screen file,
                     which is always its native resolution with no border)
  -scale <n>         pixels per emulated pixel (default: 1; not for a screen
                     file). For a captured emulator window, this is
                     usually the window's own render scale -- e.g. CSpect's
                     -w3 renders at 3x, with a further 32-pixel (32*scale)
                     border on every side of the 256x192 display before any
                     window chrome.
  -o <file>          output path for the recognised text (default: stdout)

  Recognition matches each 8x8 character cell against the real ZX Spectrum
  ROM font (codes 32-127). A classic .scr is read by colour; a .zm1 or .zm2
  (or a .scr with -palette) is read from its bitmap, 48 lines of 64 characters
  in ZXMODE2. Content outside that set -- box-drawing borders, UDGs,
  decorative graphics -- is matched to its nearest visual approximation
  rather than skipped, so garbage characters in a non-textual region of the
  screen are expected, not a bug.
`)
}

func scrEncode(args []string) error {
	fs := flag.NewFlagSet("scr encode", flag.ContinueOnError)
	out := fs.String("o", "", "output .scr path")
	var ff fitFlags
	ff.register(fs)
	var z zxEncodeFlags
	z.register(fs)
	args = permuteArgs(args, map[string]bool{"grey": true, "linear-pairs": true})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		scrUsage()
		return fmt.Errorf("encode needs exactly one input image")
	}
	inPath := fs.Arg(0)
	if z.used() {
		return scrEncodeZX(inPath, &ff, &z, *out)
	}
	fitted, err := ff.fit(inPath, scr.Width, scr.Height)
	if err != nil {
		return err
	}
	screen, err := scr.FromImage(fitted)
	if err != nil {
		return err
	}
	data := scr.Encode(screen)

	outPath := *out
	if outPath == "" {
		outPath = scrOutBase(inPath) + ".scr"
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d bytes)\n", outPath, len(data))
	return nil
}

func scrDecode(args []string) error {
	fs := flag.NewFlagSet("scr decode", flag.ContinueOnError)
	out := fs.String("o", "", "output PNG path")
	var pf picFlags
	pf.register(fs, "the screen")
	flash := fs.Bool("flash", false, "render ZXPAL0 flashing cells in their second phase")
	args = permuteArgs(args, map[string]bool{"flash": true})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		scrUsage()
		return fmt.Errorf("decode needs exactly one .scr file")
	}
	inPath := fs.Arg(0)
	if ext := strings.ToLower(filepath.Ext(inPath)); pf.mode != "" || pf.palette != "" || pf.registers != "" || *flash || ext == ".zm1" || ext == ".zm2" {
		return scrDecodeZX(inPath, pf, *out, *flash)
	}

	data, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	screen, err := scr.Decode(data)
	if err != nil {
		return err
	}

	outPath := *out
	if outPath == "" {
		outPath = scrOutBase(inPath) + ".png"
	}
	of, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer of.Close()
	if err := png.Encode(of, scr.ToImage(screen)); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", outPath)
	return nil
}

func scrCrop(args []string) error {
	fs := flag.NewFlagSet("scr crop", flag.ContinueOnError)
	cells := fs.String("cells", "", "crop region in cells: x,y,w,h")
	pixels := fs.String("pixels", "", "crop region in pixels: x,y,w,h")
	auto := fs.Bool("auto", false, "crop to the sprite's extent")
	bg := fs.String("bg", "", "background colour for --auto on image input")
	tol := fs.Int("tol", 16, "colour tolerance for --auto on image input (0-255)")
	bits := fs.Int("bits", 1, "which bitmap bits are the sprite (1 or 0); .scr input only")
	bitmapOnly := fs.String("bitmap-only", scrFlagUnset, "render the shape only; optional attribute spec for ink/paper colours")
	fs.Bool("with-attributes", false, "render in the screen's colours (default)")
	out := fs.String("o", "", "output PNG path")
	var pf picFlags
	pf.register(fs, "a screen")
	args = permuteArgs(args, map[string]bool{"auto": true, "with-attributes": true, "bitmap-only": true})
	// --bitmap-only is value-optional. Go's flag package would otherwise consume
	// the following token as its value, so rewrite a bare "--bitmap-only" (with no
	// "=") into the explicit empty-value form before parsing.
	args = normaliseOptionalValue(args, "bitmap-only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		scrUsage()
		return fmt.Errorf("crop needs exactly one input image")
	}
	inPath := fs.Arg(0)

	// Which flags did the user actually set?
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	// Exactly one geometry mode.
	modes := 0
	if *cells != "" {
		modes++
	}
	if *pixels != "" {
		modes++
	}
	if *auto {
		modes++
	}
	if modes != 1 {
		return fmt.Errorf("crop: specify exactly one of --cells, --pixels, or --auto")
	}

	// Rendering mode: --with-attributes (default) xor --bitmap-only.
	bitmapMode := set["bitmap-only"]
	if set["with-attributes"] && bitmapMode {
		return fmt.Errorf("crop: --with-attributes and --bitmap-only are mutually exclusive")
	}

	// Load the source. A .zm1 or .zm2 file, or a 6912-byte file, is a screen;
	// otherwise decode as an image.
	raw, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	isSCRInput := len(raw) == scr.FileLen || isScreenFile(inPath)

	// Flags that only make sense for .scr input.
	if !isSCRInput {
		if set["bits"] {
			return fmt.Errorf("crop: --bits applies only to .scr input")
		}
		if bitmapMode {
			return fmt.Errorf("crop: --bitmap-only applies only to .scr input")
		}
		if set["with-attributes"] {
			return fmt.Errorf("crop: --with-attributes applies only to .scr input")
		}
	}
	if *bits != 0 && *bits != 1 {
		return fmt.Errorf("crop: --bits must be 0 or 1 (got %d)", *bits)
	}

	// Produce the full-frame source image to crop from.
	var srcImg image.Image
	var pic *scr.Picture
	if isSCRInput {
		pic, err = loadPicture(inPath, pf)
		if err != nil {
			return err
		}
		if bitmapMode {
			ink, paper, paperTransparent, err := bitmapColours(*bitmapOnly)
			if err != nil {
				return err
			}
			srcImg = pic.BitmapImage(*bits, ink, paper, paperTransparent)
		} else {
			srcImg = pic.Render(false)
		}
	} else {
		img, _, derr := image.Decode(strings.NewReader(string(raw)))
		if derr != nil {
			return fmt.Errorf("decoding %s: %w", inPath, derr)
		}
		srcImg = img
	}

	// Resolve the crop region.
	var region scr.Rect
	switch {
	case *cells != "":
		x, y, w, h, err := parseQuad(*cells)
		if err != nil {
			return fmt.Errorf("crop --cells: %w", err)
		}
		region = scr.CellRect(x, y, w, h)
	case *pixels != "":
		x, y, w, h, err := parseQuad(*pixels)
		if err != nil {
			return fmt.Errorf("crop --pixels: %w", err)
		}
		region = scr.Rect{X: x, Y: y, W: w, H: h}
	case *auto:
		if isSCRInput {
			// Bitmap-aware: the sprite's extent is its set bits, not its colours.
			region, err = scr.PictureExtent(pic, *bits)
			if err != nil {
				return fmt.Errorf("crop --auto: %w", err)
			}
		} else {
			var bgColour color.Color
			if *bg != "" {
				idx, err := scr.ParseColour(*bg)
				if err != nil {
					return fmt.Errorf("crop --bg: %w", err)
				}
				bgColour = scr.Attribute{Paper: idx}.PaperRGBA()
			}
			region, err = scr.AutoExtent(srcImg, bgColour, *tol)
			if err != nil {
				return fmt.Errorf("crop --auto: %w", err)
			}
		}
	}

	cropped, err := scr.Crop(srcImg, region)
	if err != nil {
		return err
	}

	outPath := *out
	if outPath == "" {
		outPath = scrOutBase(inPath) + "-crop.png"
	}
	if !isPNG(outPath) {
		return fmt.Errorf("crop: output must be a .png file (got %q)", filepath.Ext(outPath))
	}
	of, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer of.Close()
	if err := png.Encode(of, cropped); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%dx%d at %d,%d)\n", outPath, region.W, region.H, region.X, region.Y)
	return nil
}

// scrFlagUnset is the sentinel default for --bitmap-only, distinguishing "flag
// absent" from "flag present with empty value" (e.g. bare --bitmap-only).
const scrFlagUnset = "\x00unset"

// bitmapColours resolves the ink/paper colours and paper transparency for
// --bitmap-only from its optional CSS spec. With no spec (bare --bitmap-only),
// ink is white and paper is transparent. A spec recolours: ink defaults to white
// if unspecified, and paper is transparent unless the spec explicitly names a
// "paper" field.
func bitmapColours(spec string) (ink, paper color.Color, paperTransparent bool, err error) {
	ink = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF} // default ink: white
	paper = color.RGBA{0, 0, 0, 0}           // default paper: transparent
	paperTransparent = true

	if spec == scrFlagUnset || strings.TrimSpace(spec) == "" {
		return ink, paper, paperTransparent, nil
	}

	a, err := scr.ParseAttribute(spec)
	if err != nil {
		return nil, nil, false, fmt.Errorf("crop --bitmap-only: %w", err)
	}
	// ink is always taken from the spec (defaults to black via ParseAttribute if
	// the user wrote only paper; but our render default is white, so only
	// override when the spec actually carried an ink).
	if specHasField(spec, "ink") {
		ink = a.InkRGBA()
	}
	// paper becomes opaque only if explicitly named.
	if specHasField(spec, "paper") {
		paper = a.PaperRGBA()
		paperTransparent = false
	}
	return ink, paper, paperTransparent, nil
}

// specHasField reports whether a CSS-like attribute spec contains the named
// field, so the command can distinguish an explicit value from a default.
func specHasField(spec, field string) bool {
	s := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(spec), "{"), "}"))
	for _, part := range strings.Split(s, ";") {
		k, _, ok := strings.Cut(part, ":")
		if ok && strings.EqualFold(strings.TrimSpace(k), field) {
			return true
		}
	}
	return false
}

// parseQuad parses "x,y,w,h" into four ints, tolerating spaces around commas.
func parseQuad(s string) (x, y, w, h int, err error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return 0, 0, 0, 0, fmt.Errorf("want x,y,w,h (four values), got %q", s)
	}
	vals := make([]int, 4)
	for i, p := range parts {
		n, e := strconv.Atoi(strings.TrimSpace(p))
		if e != nil {
			return 0, 0, 0, 0, fmt.Errorf("%q is not an integer", strings.TrimSpace(p))
		}
		vals[i] = n
	}
	return vals[0], vals[1], vals[2], vals[3], nil
}

func isPNG(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".png")
}

// normaliseOptionalValue rewrites a bare "--name" (or "-name") token, not already
// in "--name=value" form, into "--name=" so a value-optional string flag does not
// consume the following argument as its value.
func normaliseOptionalValue(args []string, name string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		trimmed := strings.TrimLeft(a, "-")
		if trimmed == name { // exactly "--name" with no "=value"
			out[i] = "--" + name + "="
		} else {
			out[i] = a
		}
	}
	return out
}

func parseResizeMode(s string) (scr.ResizeMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return scr.ResizeNone, nil
	case "stretch":
		return scr.ResizeStretch, nil
	case "bestfit", "best-fit", "fit":
		return scr.ResizeBestFit, nil
	case "centre", "center":
		return scr.ResizeCentre, nil
	case "fill", "cover":
		return scr.ResizeFill, nil
	default:
		return scr.ResizeNone, fmt.Errorf("unknown resize mode %q (want stretch, bestfit, centre or fill)", s)
	}
}

func scrOCR(args []string) error {
	fs := flag.NewFlagSet("scr ocr", flag.ContinueOnError)
	origin := fs.String("origin", "", "pixel offset x,y of the display's top-left corner (default 0,0)")
	scale := fs.Int("scale", 1, "pixels per emulated pixel")
	out := fs.String("o", "", "output path for recognised text (default: stdout)")
	var pf picFlags
	pf.register(fs, "a screen")
	args = permuteArgs(args, map[string]bool{})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		scrUsage()
		return fmt.Errorf("ocr needs exactly one input image")
	}
	inPath := fs.Arg(0)

	raw, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}

	var lines []string
	if len(raw) == scr.FileLen || isScreenFile(inPath) {
		// A screen file is always exactly its native pixels, so no
		// geometry flags apply -- flag them as a mistake rather than
		// silently ignoring what the user asked for.
		if *origin != "" || *scale != 1 {
			return fmt.Errorf("ocr: --origin and --scale do not apply to a screen file (already native resolution)")
		}
		pic, derr := loadPicture(inPath, pf)
		if derr != nil {
			return derr
		}
		if pic.Mode == scr.ZXMODE0 && pic.Palette == scr.ZXPAL0 && pic.Registers == nil {
			// A classic screen reads by colour, as it always has.
			s, derr := scr.Decode(raw)
			if derr != nil {
				return derr
			}
			lines, err = scr.RecognizeScreen(s)
		} else {
			lines, err = scr.RecognizePicture(pic)
		}
	} else {
		img, _, derr := image.Decode(strings.NewReader(string(raw)))
		if derr != nil {
			return fmt.Errorf("decoding %s: %w", inPath, derr)
		}
		geom := scr.Geometry{OriginX: 0, OriginY: 0, Scale: *scale}
		if *origin != "" {
			x, y, oerr := parsePair(*origin)
			if oerr != nil {
				return fmt.Errorf("ocr --origin: %w", oerr)
			}
			geom.OriginX, geom.OriginY = x, y
		}
		lines, err = scr.RecognizeText(img, geom)
	}
	if err != nil {
		return err
	}

	text := strings.Join(lines, "\n") + "\n"
	if *out == "" {
		fmt.Print(text)
		return nil
	}
	if werr := os.WriteFile(*out, []byte(text), 0644); werr != nil {
		return werr
	}
	fmt.Printf("wrote %s\n", *out)
	return nil
}

func scrOutBase(path string) string {
	b := filepath.Base(path)
	return strings.TrimSuffix(b, filepath.Ext(b))
}
