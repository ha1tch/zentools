// file: cut.go
//
// The cut, paste, ls, flip and convert subcommands work with ZCUT asset
// collections and with screens in any ZXMODE mode and ZXPAL palette:
// extracting named regions of a screen into a collection, blitting them back
// (converting attributes between palettes where they differ), mirroring, and
// rewriting assets for another palette.

package main

import (
	"flag"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ha1tch/zentools/pkg/scr"
)

// loadCollection reads a .cut file, or returns an empty collection if the path
// does not exist (so cut can append to a new file).
func loadCollection(path string) (*scr.Collection, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &scr.Collection{}, nil
	}
	if err != nil {
		return nil, err
	}
	return scr.DecodeCollection(raw)
}

func writeCollection(path string, col *scr.Collection) error {
	data, err := scr.EncodeCollection(col)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// picFlags are the flags that say how to read a screen file: its mode
// (default from the extension), palette (default zxpal0) and registers
// (default the palette's reset values).
type picFlags struct {
	mode, palette, registers string
}

func (f *picFlags) register(fs *flag.FlagSet, what string) {
	fs.StringVar(&f.mode, "mode", "", "ZXMODE mode of "+what+" (default: zxmode1 for .zm1, zxmode2 for .zm2, zxmode0 otherwise)")
	fs.StringVar(&f.palette, "palette", "", "ZXPAL palette of "+what+" (default: zxpal0)")
	fs.StringVar(&f.registers, "registers", "", "register table (CSV) "+what+" is meant for (default: the palette's reset values)")
}

// isScreenFile reports whether a path names a screen file by extension.
func isScreenFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".scr", ".zm1", ".zm2":
		return true
	}
	return false
}

func modeFromExt(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zm1":
		return "1"
	case ".zm2":
		return "2"
	}
	return "0"
}

// readRegisters reads a register table for a palette, or returns nil.
func readRegisters(path string, p scr.Palette) (scr.Registers, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	regs, err := scr.ParseRegistersCSV(data, p)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return regs, nil
}

// loadPicture reads a .scr, .zm1 or .zm2 file as the flags describe it.
func loadPicture(path string, f picFlags) (*scr.Picture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	mode := f.mode
	if mode == "" {
		mode = modeFromExt(path)
	}
	m, p, err := parseZXFlags(mode, f.palette)
	if err != nil {
		return nil, err
	}
	if len(data) != scr.PictureLen(m, p) {
		hint := ""
		if f.palette == "" && m != scr.ZXMODE0 && len(data) == scr.PictureLen(m, scr.ZXPAL3) {
			hint = "; it is the size of a two-byte palette: give -palette zxpal3 or zxpal4"
		}
		return nil, fmt.Errorf("%s is %d bytes; %s with %s is %d%s", path, len(data), m, p, scr.PictureLen(m, p), hint)
	}
	pic, err := scr.DecodePicture(data, m, p)
	if err != nil {
		return nil, err
	}
	if pic.Registers, err = readRegisters(f.registers, p); err != nil {
		return nil, err
	}
	return pic, nil
}

// savePicture writes a picture to a file whose extension fits its mode.
func savePicture(path string, pic *scr.Picture) error {
	if want := zxExt(pic.Mode); !strings.EqualFold(filepath.Ext(path), want) {
		return fmt.Errorf("a %s picture is written as %s, not %s", pic.Mode, want, filepath.Base(path))
	}
	return os.WriteFile(path, pic.Bytes(), 0644)
}

func scrCut(args []string) error {
	fs := flag.NewFlagSet("scr cut", flag.ContinueOnError)
	cells := fs.String("cells", "", "region in cells: x,y,w,h")
	pixels := fs.String("pixels", "", "region in pixels: x,y,w,h")
	name := fs.String("name", "", "asset name (ASCII, required)")
	out := fs.String("o", "", "output .cut path (appends if it exists)")
	bitmapOnly := fs.Bool("bitmap-only", false, "store bitmap only, drop attributes")
	asMask := fs.Bool("mask", false, "flag the asset as a mask (implies bitmap-only)")
	var pf picFlags
	pf.register(fs, "the screen")
	args = permuteArgs(args, map[string]bool{"bitmap-only": true, "mask": true})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("cut needs exactly one input screen (.scr, .zm1 or .zm2)")
	}
	if *name == "" {
		return fmt.Errorf("cut: --name is required")
	}
	if *out == "" {
		return fmt.Errorf("cut: -o <file.cut> is required")
	}
	if (*cells == "") == (*pixels == "") {
		return fmt.Errorf("cut: specify exactly one of --cells or --pixels")
	}
	pic, err := loadPicture(fs.Arg(0), pf)
	if err != nil {
		return err
	}
	var region scr.Rect
	if *cells != "" {
		x, y, w, h, err := parseQuad(*cells)
		if err != nil {
			return fmt.Errorf("cut --cells: %w", err)
		}
		region = scr.CellRect(x, y, w, h)
	} else {
		x, y, w, h, err := parseQuad(*pixels)
		if err != nil {
			return fmt.Errorf("cut --pixels: %w", err)
		}
		region = scr.Rect{X: x, Y: y, W: w, H: h}
	}
	asset, err := scr.CutPicture(pic, region, *name, !*bitmapOnly && !*asMask)
	if err != nil {
		return err
	}
	asset.IsMask = *asMask
	if !asset.HasAttrs() {
		asset.Palette, asset.Registers = scr.ZXPAL0, nil
	}
	col, err := loadCollection(*out)
	if err != nil {
		return err
	}
	if col.Find(*name) != nil {
		return fmt.Errorf("cut: collection already has an asset named %q", *name)
	}
	col.Assets = append(col.Assets, *asset)
	if err := writeCollection(*out, col); err != nil {
		return err
	}
	fmt.Printf("cut %q (%dx%d, %s) -> %s [%d assets]\n", *name, asset.Width, asset.Height, pixmapLabel(asset), *out, len(col.Assets))
	return nil
}

// pixmapLabel describes an asset's pixels for cut and ls.
func pixmapLabel(a *scr.Asset) string {
	switch {
	case a.IsMask:
		return "mask"
	case !a.HasAttrs():
		return "mono"
	}
	s := "color"
	if a.Palette != scr.ZXPAL0 || a.Registers != nil {
		s += " " + a.Palette.String()
	}
	if a.Registers != nil {
		s += ", own registers"
	}
	return s
}

func parseOp(s string) (scr.PasteOp, error) {
	switch strings.ToLower(s) {
	case "or":
		return scr.PasteOR, nil
	case "and":
		return scr.PasteAND, nil
	case "copy":
		return scr.PasteCOPY, nil
	case "xor":
		return scr.PasteXOR, nil
	}
	return 0, fmt.Errorf("paste: --op must be one of or, and, copy, xor (got %q)", s)
}

// findAsset resolves a <file.cut>:<name> reference.
func findAsset(ref string) (*scr.Collection, *scr.Asset, error) {
	colPath, assetName, ok := strings.Cut(ref, ":")
	if !ok || assetName == "" {
		return nil, nil, fmt.Errorf("reference must be <file.cut>:<name>")
	}
	col, err := loadCollection(colPath)
	if err != nil {
		return nil, nil, err
	}
	asset := col.Find(assetName)
	if asset == nil {
		return nil, nil, fmt.Errorf("no asset named %q in %s", assetName, colPath)
	}
	return col, asset, nil
}

func scrPaste(args []string) error {
	fs := flag.NewFlagSet("scr paste", flag.ContinueOnError)
	at := fs.String("at", "", "paste position in pixels: x,y")
	opName := fs.String("op", "or", "bit operation: or, and, copy, xor")
	setAttr := fs.String("set-attr", "", "recolour the asset's attributes before pasting (CSS-like; one-byte palettes)")
	maskName := fs.String("mask", "", "name of a mask asset in the same collection to filter through")
	out := fs.String("o", "", "output path (default: the target)")
	var pf picFlags
	pf.register(fs, "the target")
	args = permuteArgs(args, nil)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("paste needs <collection.cut:name> and a target screen (.scr, .zm1 or .zm2)")
	}
	ref, target := fs.Arg(0), fs.Arg(1)
	if *at == "" {
		return fmt.Errorf("paste: --at x,y is required")
	}
	x, y, err := parsePair(*at)
	if err != nil {
		return fmt.Errorf("paste --at: %w", err)
	}
	col, asset, err := findAsset(ref)
	if err != nil {
		return fmt.Errorf("paste: %w", err)
	}
	if *setAttr != "" {
		if !asset.HasAttrs() {
			return fmt.Errorf("paste --set-attr: asset %q has no attributes", asset.Name)
		}
		if asset.Attr2 != nil {
			return fmt.Errorf("paste --set-attr: asset %q is in %s, whose cells are register numbers, not attribute bytes", asset.Name, asset.Palette)
		}
		na, err := scr.ParseAttribute(*setAttr)
		if err != nil {
			return fmt.Errorf("paste --set-attr: %w", err)
		}
		setHasInk := specHasField(*setAttr, "ink")
		setHasPaper := specHasField(*setAttr, "paper")
		setHasBright := specHasField(*setAttr, "bright")
		setHasFlash := specHasField(*setAttr, "flash")
		asset.MapAttributes(func(a scr.Attribute) scr.Attribute {
			if setHasInk {
				a.Ink = na.Ink
			}
			if setHasPaper {
				a.Paper = na.Paper
			}
			if setHasBright {
				a.Bright = na.Bright
			}
			if setHasFlash {
				a.Flash = na.Flash
			}
			return a
		})
	}
	if *maskName != "" {
		m := col.Find(*maskName)
		if m == nil {
			return fmt.Errorf("paste --mask: no asset named %q", *maskName)
		}
		if err := scr.ApplyMask(asset, m); err != nil {
			return fmt.Errorf("paste --mask: %w", err)
		}
	}
	pic, err := loadPicture(target, pf)
	if err != nil {
		return err
	}
	op, err := parseOp(*opName)
	if err != nil {
		return err
	}
	converted, err := scr.PastePicture(pic, asset, x, y, op)
	if err != nil {
		return err
	}
	outPath := *out
	if outPath == "" {
		outPath = target
	}
	if err := savePicture(outPath, pic); err != nil {
		return err
	}
	note := ""
	if converted {
		note = fmt.Sprintf(" (attributes converted from %s to %s)", regLabel(asset.Palette, asset.Registers), regLabel(pic.Palette, pic.Registers))
	}
	fmt.Printf("pasted %q at %d,%d -> %s%s\n", asset.Name, x, y, outPath, note)
	return nil
}

func regLabel(p scr.Palette, regs scr.Registers) string {
	if regs != nil {
		return p.String() + " with its own registers"
	}
	return p.String()
}

func scrLs(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("ls needs exactly one .cut file")
	}
	col, err := loadCollection(args[0])
	if err != nil {
		return err
	}
	if len(col.Assets) == 0 {
		fmt.Println("(empty collection)")
		return nil
	}
	fmt.Printf("%-3s %-20s %-9s %s\n", "#", "NAME", "SIZE", "PIXMAP")
	for i := range col.Assets {
		a := &col.Assets[i]
		fmt.Printf("%-3d %-20s %-9s %s\n", i, a.Name, fmt.Sprintf("%dx%d", a.Width, a.Height), pixmapLabel(a))
	}
	return nil
}

// selectAssets returns the assets of a collection a -name flag picks: one, or
// all when name is empty.
func selectAssets(col *scr.Collection, name string) ([]*scr.Asset, error) {
	if name == "" {
		out := make([]*scr.Asset, len(col.Assets))
		for i := range col.Assets {
			out[i] = &col.Assets[i]
		}
		return out, nil
	}
	a := col.Find(name)
	if a == nil {
		return nil, fmt.Errorf("no asset named %q", name)
	}
	return []*scr.Asset{a}, nil
}

func scrFlip(args []string) error {
	fs := flag.NewFlagSet("scr flip", flag.ContinueOnError)
	h := fs.Bool("h", false, "mirror left to right")
	v := fs.Bool("v", false, "mirror top to bottom")
	name := fs.String("name", "", "for a .cut: the asset to flip (default: all)")
	out := fs.String("o", "", "output path (default: the input)")
	var pf picFlags
	pf.register(fs, "the screen")
	if err := fs.Parse(permuteArgs(args, map[string]bool{"h": true, "v": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("flip needs exactly one input: a screen, a .cut or an image")
	}
	if !*h && !*v {
		return fmt.Errorf("flip: give -h, -v or both")
	}
	in := fs.Arg(0)
	outPath := *out
	if outPath == "" {
		outPath = in
	}
	dir := strings.TrimSpace(map[bool]string{true: "horizontally"}[*h] + map[bool]string{true: " vertically"}[*v])
	switch {
	case strings.EqualFold(filepath.Ext(in), ".cut"):
		col, err := loadCollection(in)
		if err != nil {
			return err
		}
		assets, err := selectAssets(col, *name)
		if err != nil {
			return fmt.Errorf("flip: %w", err)
		}
		for _, a := range assets {
			a.Flip(*h, *v)
		}
		if err := writeCollection(outPath, col); err != nil {
			return err
		}
		fmt.Printf("flipped %d asset(s) %s -> %s\n", len(assets), dir, outPath)
	case isScreenFile(in):
		pic, err := loadPicture(in, pf)
		if err != nil {
			return err
		}
		pic.Flip(*h, *v)
		if err := savePicture(outPath, pic); err != nil {
			return err
		}
		fmt.Printf("flipped %s %s -> %s\n", in, dir, outPath)
	default:
		img, err := readImageFile(in)
		if err != nil {
			return err
		}
		if !isPNG(outPath) {
			return fmt.Errorf("flip: an image is written as PNG; give -o file.png")
		}
		if err := writePNGFile(outPath, flipImage(img, *h, *v)); err != nil {
			return err
		}
		fmt.Printf("flipped %s %s -> %s\n", in, dir, outPath)
	}
	return nil
}

func flipImage(img image.Image, h, v bool) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			sx, sy := x, y
			if h {
				sx = b.Dx() - 1 - x
			}
			if v {
				sy = b.Dy() - 1 - y
			}
			out.Set(x, y, img.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	return out
}

func scrConvert(args []string) error {
	fs := flag.NewFlagSet("scr convert", flag.ContinueOnError)
	toPal := fs.String("to", "", "target palette: zxpal0 to zxpal4")
	toRegs := fs.String("to-registers", "", "register table (CSV) of the target (default: its reset values)")
	toMode := fs.String("to-mode", "", "for a screen: the target mode (default: the source's; zxmode0 and zxmode1 share a size)")
	name := fs.String("name", "", "for a .cut: the asset to convert (default: all)")
	out := fs.String("o", "", "output path (default: the input)")
	var pf picFlags
	pf.register(fs, "the screen")
	if err := fs.Parse(permuteArgs(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *toPal == "" {
		return fmt.Errorf("convert needs -to palette and exactly one .cut or screen")
	}
	to, err := scr.ParsePalette(*toPal)
	if err != nil {
		return err
	}
	regs, err := readRegisters(*toRegs, to)
	if err != nil {
		return err
	}
	in := fs.Arg(0)
	outPath := *out
	if outPath == "" {
		outPath = in
	}
	if strings.EqualFold(filepath.Ext(in), ".cut") {
		col, err := loadCollection(in)
		if err != nil {
			return err
		}
		assets, err := selectAssets(col, *name)
		if err != nil {
			return fmt.Errorf("convert: %w", err)
		}
		for _, a := range assets {
			c, err := scr.ConvertAsset(a, to, regs)
			if err != nil {
				return fmt.Errorf("convert %q: %w", a.Name, err)
			}
			*a = *c
		}
		if err := writeCollection(outPath, col); err != nil {
			return err
		}
		fmt.Printf("converted %d asset(s) to %s -> %s\n", len(assets), regLabel(to, regs), outPath)
		return nil
	}
	if !isScreenFile(in) {
		return fmt.Errorf("convert: %s is neither a .cut nor a screen (.scr, .zm1, .zm2)", in)
	}
	pic, err := loadPicture(in, pf)
	if err != nil {
		return err
	}
	mode := pic.Mode
	if *toMode != "" {
		if mode, err = scr.ParseMode(*toMode); err != nil {
			return err
		}
	}
	a, err := scr.ConvertAsset(pic.Asset("screen"), to, regs)
	if err != nil {
		return err
	}
	res, err := scr.PictureFromAsset(a, mode)
	if err != nil {
		return fmt.Errorf("convert: %w", err)
	}
	if *out == "" && mode != pic.Mode {
		outPath = scrOutBase(in) + zxExt(mode)
	}
	if err := savePicture(outPath, res); err != nil {
		return err
	}
	fmt.Printf("converted %s (%s, %s) to %s, %s -> %s\n", in, pic.Mode, regLabel(pic.Palette, pic.Registers), mode, regLabel(to, regs), outPath)
	if regs != nil {
		fmt.Printf("  the registers are not stored in %s; pass the same table with -registers to read it\n", filepath.Ext(outPath))
	}
	return nil
}

// parsePair parses "x,y" into two ints, tolerating spaces.
func parsePair(s string) (x, y int, err error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("want x,y, got %q", s)
	}
	x, e1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	y, e2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if e1 != nil || e2 != nil {
		return 0, 0, fmt.Errorf("non-integer in %q", s)
	}
	return x, y, nil
}
