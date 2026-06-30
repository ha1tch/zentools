// file: scr.go
//
// The `zx scr` subcommand converts ordinary images (PNG, JPEG, GIF) into ZX
// Spectrum SCR screen files, and decodes SCR files back to PNG. Conversion
// reduces the image to the Spectrum's per-cell two-colour attribute constraint.
//
// An image that is not already 256x192 must be resized with --resize; without
// it, conversion fails and lists the available modes. The padding colour for
// the centre and bestfit modes is given as a CSS-like --fillattr.

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
	case "-h", "--help", "help":
		scrUsage()
		return nil
	default:
		// Bare form: `zx scr <image>` is shorthand for `zx scr encode <image>`.
		return scrEncode(args)
	}
}

func scrUsage() {
	fmt.Fprint(os.Stderr, `zx scr - convert images to/from ZX Spectrum SCR screens

Usage:
  zx scr encode [flags] <image>        convert PNG/JPEG/GIF to .scr
  zx scr decode [flags] <file.scr>     render an .scr back to PNG
  zx scr crop   [flags] <image|.scr>   crop a region (or sprite) to PNG

Encode flags:
  -o <file>          output path (default: input base name + .scr)
  -resize <mode>     how to fit a non-256x192 image:
                       stretch   scale to fill, ignoring aspect ratio
                       bestfit   scale to fit, preserving aspect, padded border
                       centre    no scaling; centre and crop/pad (alias: center)
  -fillattr <spec>   border colour for centre/bestfit, CSS-like:
                       "ink:white; paper:blue; bright:1; flash:0"
                       colours may be names (black, blue, red, magenta, green,
                       cyan, yellow, white) or palette indices 0-7;
                       spaces around : and ; are allowed; braces optional;
                       only paper is visible in a border

Decode flags:
  -o <file>          output PNG path (default: input base name + .png)

Crop flags (exactly one of --cells / --pixels / --auto):
  -cells <x,y,w,h>   crop region in 8x8 character cells
  -pixels <x,y,w,h>  crop region in pixels (w, h need not be multiples of 8)
  -auto              crop to the sprite extent: for a .scr, the bounding box of
                     set bitmap bits; for an image, the non-background box
  -o <file>          output PNG path (default: input base name + "-crop.png")

  For .scr input (sprite extraction):
  -bits <1|0>        which bitmap bits are the sprite (default 1; 0 = paper-on-ink)
  -with-attributes   render in the screen's own colours, opaque (default)
  -bitmap-only[=<css>]  render the shape only; paper is transparent unless the
                     attribute spec names a paper, e.g.
                     --bitmap-only="ink:cyan; paper:black"

  For image input, --auto only:
  -bg <colour>       background colour (name or index 0-7; default: inferred)
  -tol <n>           colour tolerance 0-255 (default 16)

  Output is always PNG.

An image that is not 256x192 requires -resize; without it, encode fails.
`)
}

func scrEncode(args []string) error {
	fs := flag.NewFlagSet("scr encode", flag.ContinueOnError)
	out := fs.String("o", "", "output .scr path")
	resize := fs.String("resize", "", "resize mode: stretch | bestfit | centre")
	fillattr := fs.String("fillattr", "", "border attribute (CSS-like)")
	args = permuteArgs(args, map[string]bool{})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		scrUsage()
		return fmt.Errorf("encode needs exactly one input image")
	}
	inPath := fs.Arg(0)

	mode, err := parseResizeMode(*resize)
	if err != nil {
		return err
	}

	fill, err := scr.ParseAttribute(*fillattr)
	if err != nil {
		return err
	}

	f, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer f.Close()

	img, err := scr.DecodeImage(f)
	if err != nil {
		return fmt.Errorf("decoding %s: %w", inPath, err)
	}

	fitted, err := scr.Fit(img, mode, fill.PaperRGBA())
	if err != nil {
		// Turn a size mismatch with no chosen mode into actionable guidance.
		if _, ok := err.(*scr.SizeError); ok && mode == scr.ResizeNone {
			return fmt.Errorf("%s is %dx%d, not %dx%d; choose a resize mode:\n"+
				"  --resize=stretch   scale to fill, ignoring aspect ratio\n"+
				"  --resize=bestfit   scale to fit, preserving aspect, padded border\n"+
				"  --resize=centre    no scaling; centre and crop/pad",
				inPath, img.Bounds().Dx(), img.Bounds().Dy(), scr.Width, scr.Height)
		}
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
	args = permuteArgs(args, map[string]bool{})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		scrUsage()
		return fmt.Errorf("decode needs exactly one .scr file")
	}
	inPath := fs.Arg(0)

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

	// Load the source. A 6912-byte file is an SCR; otherwise decode as an image.
	raw, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	isSCRInput := len(raw) == scr.FileLen

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
	var screen *scr.Screen
	if isSCRInput {
		screen, err = scr.Decode(raw)
		if err != nil {
			return err
		}
		if bitmapMode {
			ink, paper, paperTransparent, err := bitmapColours(*bitmapOnly)
			if err != nil {
				return err
			}
			srcImg = scr.ToImageBitmap(screen, *bits, ink, paper, paperTransparent)
		} else {
			srcImg = scr.ToImage(screen)
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
			region, err = scr.BitmapExtent(screen, *bits)
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
	default:
		return scr.ResizeNone, fmt.Errorf("unknown resize mode %q (want stretch, bestfit, or centre)", s)
	}
}

func scrOutBase(path string) string {
	b := filepath.Base(path)
	return strings.TrimSuffix(b, filepath.Ext(b))
}
