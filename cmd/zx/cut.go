// file: cut.go
//
// The cut/paste/ls subcommands work with ZCUT asset collections: extracting
// named regions of a screen into a collection, blitting them back, and listing
// a collection's contents.

package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ha1tch/zentools/pkg/scr"
)

// loadScreen reads a .scr file into a Screen.
func loadScreen(path string) (*scr.Screen, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) != scr.FileLen {
		return nil, fmt.Errorf("%s is not a .scr (%d bytes, want %d)", path, len(raw), scr.FileLen)
	}
	return scr.Decode(raw)
}

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

func scrCut(args []string) error {
	fs := flag.NewFlagSet("scr cut", flag.ContinueOnError)
	cells := fs.String("cells", "", "region in cells: x,y,w,h")
	pixels := fs.String("pixels", "", "region in pixels: x,y,w,h")
	name := fs.String("name", "", "asset name (ASCII, required)")
	out := fs.String("o", "", "output .cut path (appends if it exists)")
	bitmapOnly := fs.Bool("bitmap-only", false, "store bitmap only, drop attributes")
	asMask := fs.Bool("mask", false, "flag the asset as a mask (implies bitmap-only)")
	args = permuteArgs(args, map[string]bool{"bitmap-only": true, "mask": true})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("cut needs exactly one input .scr")
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

	s, err := loadScreen(fs.Arg(0))
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

	keepAttrs := !*bitmapOnly && !*asMask
	asset, err := scr.CutRegion(s, region, *name, keepAttrs)
	if err != nil {
		return err
	}
	if *asMask {
		asset.IsMask = true
	}

	col, err := loadCollection(*out)
	if err != nil {
		return err
	}
	if col.Find(*name) != nil {
		return fmt.Errorf("cut: collection already has an asset named %q", *name)
	}
	col.Assets = append(col.Assets, *asset)

	data, err := scr.EncodeCollection(col)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, data, 0644); err != nil {
		return err
	}
	pixmap := "color"
	if asset.IsMask {
		pixmap = "mask"
	} else if !asset.HasAttrs() {
		pixmap = "mono"
	}
	fmt.Printf("cut %q (%dx%d, %s) -> %s [%d assets]\n", *name, asset.Width, asset.Height, pixmap, *out, len(col.Assets))
	return nil
}

func scrPaste(args []string) error {
	fs := flag.NewFlagSet("scr paste", flag.ContinueOnError)
	at := fs.String("at", "", "paste position in pixels: x,y")
	opName := fs.String("op", "or", "bit operation: or, and, copy, xor")
	setAttr := fs.String("set-attr", "", "recolour the asset's attributes before pasting (CSS-like)")
	maskName := fs.String("mask", "", "name of a mask asset in the same collection to filter through")
	out := fs.String("o", "", "output .scr path")
	args = permuteArgs(args, nil)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("paste needs <collection.cut:name> and a target .scr")
	}
	ref, target := fs.Arg(0), fs.Arg(1)
	if *at == "" {
		return fmt.Errorf("paste: --at x,y is required")
	}
	x, y, err := parsePair(*at)
	if err != nil {
		return fmt.Errorf("paste --at: %w", err)
	}

	colPath, assetName, ok := strings.Cut(ref, ":")
	if !ok || assetName == "" {
		return fmt.Errorf("paste: reference must be <file.cut>:<name>")
	}
	col, err := loadCollection(colPath)
	if err != nil {
		return err
	}
	asset := col.Find(assetName)
	if asset == nil {
		return fmt.Errorf("paste: no asset named %q in %s", assetName, colPath)
	}

	// optional recolour
	if *setAttr != "" {
		if !asset.HasAttrs() {
			return fmt.Errorf("paste --set-attr: asset %q has no attributes", assetName)
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

	// optional mask filtering
	if *maskName != "" {
		m := col.Find(*maskName)
		if m == nil {
			return fmt.Errorf("paste --mask: no asset named %q", *maskName)
		}
		if err := scr.ApplyMask(asset, m); err != nil {
			return fmt.Errorf("paste --mask: %w", err)
		}
	}

	s, err := loadScreen(target)
	if err != nil {
		return err
	}
	var op scr.PasteOp
	switch strings.ToLower(*opName) {
	case "or":
		op = scr.PasteOR
	case "and":
		op = scr.PasteAND
	case "copy":
		op = scr.PasteCOPY
	case "xor":
		op = scr.PasteXOR
	default:
		return fmt.Errorf("paste: --op must be one of or, and, copy, xor (got %q)", *opName)
	}
	if err := scr.Paste(s, asset, x, y, op); err != nil {
		return err
	}

	outPath := *out
	if outPath == "" {
		outPath = target
	}
	if err := os.WriteFile(outPath, scr.Encode(s), 0644); err != nil {
		return err
	}
	fmt.Printf("pasted %q at %d,%d -> %s\n", assetName, x, y, outPath)
	return nil
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
		pixmap := "color"
		if a.IsMask {
			pixmap = "mask"
		} else if !a.HasAttrs() {
			pixmap = "mono"
		}
		fmt.Printf("%-3d %-20s %-9s %s\n", i, a.Name, fmt.Sprintf("%dx%d", a.Width, a.Height), pixmap)
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
