// file: atlas.go
//
// The atlas subcommand renders a ZCUT collection as a labelled contact sheet: a
// grid of panels, one per asset, each showing the asset and its name. It is the
// visual counterpart of `ls`. Layout, labelling, and PNG output are presentation
// concerns and live here in the command, not in pkg/scr; the per-asset rendering
// is done by scr.AssetToImage.

package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"

	"github.com/ha1tch/zentools/pkg/scr"
)

func scrAtlas(args []string) error {
	fs := flag.NewFlagSet("scr atlas", flag.ContinueOnError)
	out := fs.String("o", "", "output PNG path (default: input base + -atlas.png)")
	scale := fs.Int("scale", 4, "integer pixel scale for each asset")
	cols := fs.Int("cols", 0, "panels per row (0 = auto)")
	args = permuteArgs(args, nil)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("atlas needs exactly one .cut file")
	}
	if *scale < 1 {
		return fmt.Errorf("atlas: --scale must be >= 1")
	}
	inPath := fs.Arg(0)

	raw, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	col, err := scr.DecodeCollection(raw)
	if err != nil {
		return err
	}
	if len(col.Assets) == 0 {
		return fmt.Errorf("atlas: collection is empty")
	}

	// Panel geometry: each panel holds the scaled asset plus a label strip,
	// padded, sized to the widest/tallest asset so the grid is uniform.
	const pad = 8
	const labelH = 14
	maxW, maxH := 0, 0
	for i := range col.Assets {
		if col.Assets[i].Width > maxW {
			maxW = col.Assets[i].Width
		}
		if col.Assets[i].Height > maxH {
			maxH = col.Assets[i].Height
		}
	}
	panelW := maxW**scale + pad*2
	panelH := maxH**scale + pad*2 + labelH

	n := len(col.Assets)
	ncols := *cols
	if ncols <= 0 {
		ncols = int(math.Ceil(math.Sqrt(float64(n))))
	}
	nrows := (n + ncols - 1) / ncols

	atlasW := ncols * panelW
	atlasH := nrows * panelH
	atlas := image.NewRGBA(image.Rect(0, 0, atlasW, atlasH))
	draw.Draw(atlas, atlas.Bounds(), &image.Uniform{color.RGBA{0x20, 0x20, 0x20, 0xFF}}, image.Point{}, draw.Src)

	silver := color.RGBA{0xC8, 0xC8, 0xC8, 0xFF}
	for i := range col.Assets {
		a := &col.Assets[i]
		px := (i % ncols) * panelW
		py := (i / ncols) * panelH

		// panel background (checkerboard so transparency is visible)
		drawChecker(atlas, image.Rect(px, py, px+panelW, py+panelH-labelH))

		// render asset; bitmap-only assets drawn silver on transparent
		ai := scr.AssetToImage(a, silver, color.RGBA{}, true)
		scaled := scaleNearest(ai, *scale)
		ox := px + (panelW-scaled.Bounds().Dx())/2
		oy := py + pad
		draw.Draw(atlas, scaled.Bounds().Add(image.Pt(ox, oy)), scaled, image.Point{}, draw.Over)

		// label strip
		ly := py + panelH - labelH
		draw.Draw(atlas, image.Rect(px, ly, px+panelW, ly+labelH),
			&image.Uniform{color.RGBA{0x10, 0x10, 0x10, 0xFF}}, image.Point{}, draw.Src)
		label := fmt.Sprintf("%s %dx%d", a.Name, a.Width, a.Height)
		drawLabel(atlas, label, px+4, ly+3, color.RGBA{0xE0, 0xE0, 0xE0, 0xFF})
	}

	outPath := *out
	if outPath == "" {
		outPath = scrOutBase(inPath) + "-atlas.png"
	}
	if !isPNG(outPath) {
		return fmt.Errorf("atlas: output must be a .png file")
	}
	of, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer of.Close()
	if err := png.Encode(of, atlas); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d assets, %dx%d)\n", outPath, n, atlasW, atlasH)
	return nil
}

func drawChecker(img *image.RGBA, r image.Rectangle) {
	const sq = 8
	light := color.RGBA{0x50, 0x50, 0x50, 0xFF}
	dark := color.RGBA{0x40, 0x40, 0x40, 0xFF}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := dark
			if ((x-r.Min.X)/sq+(y-r.Min.Y)/sq)%2 == 0 {
				c = light
			}
			img.SetRGBA(x, y, c)
		}
	}
}

func scaleNearest(src *image.RGBA, s int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*s, b.Dy()*s))
	for y := 0; y < b.Dy()*s; y++ {
		for x := 0; x < b.Dx()*s; x++ {
			dst.Set(x, y, src.At(b.Min.X+x/s, b.Min.Y+y/s))
		}
	}
	return dst
}
