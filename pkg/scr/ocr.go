// file: pkg/scr/ocr.go
//
// OCR reads the text on a rendered ZX Spectrum screen by matching each 8x8
// character cell against the real ROM font, rather than requiring a human
// to read it visually off a captured image.
//
// It exists specifically for driving emulators through screenshots: an
// automated session can send input, capture the screen, and read back
// exactly what printed -- as text -- rather than a human inspecting each
// image by eye. A .scr file needs no geometry at all (it is always exactly
// the native 256x192 pixels); an arbitrary screenshot (an emulator window,
// scaled and offset within a larger capture) needs Geometry to say where
// the 256x192 display actually sits.

package scr

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"strings"

	"github.com/ha1tch/zentools/pkg/bdf"
)

// sinclairFont is the ZX Spectrum 48K ROM character set (codes 32-127),
// embedded directly so OCR has no runtime dependency on an external font
// file. Sourced from github.com/ha1tch/bdf-fonts; see LICENSE-Sinclair.txt
// for provenance (extracted from a real 48K ROM, redistributed under
// Amstrad's own permission grant).
//
//go:embed sinclair.bdf
var sinclairFont []byte

// Geometry describes where a 256x192 Spectrum display sits within a larger
// captured image, and at what scale it was rendered. A plain .scr file (or
// any image already cropped to exactly the native resolution) needs
// Geometry{0, 0, 1}; an emulator window screenshot usually needs both a
// non-zero origin (past the window's own border/chrome) and a scale factor
// greater than 1.
type Geometry struct {
	OriginX int // pixel x, within the source image, of the display's top-left corner
	OriginY int // pixel y, within the source image, of the display's top-left corner
	Scale   int // pixels per emulated pixel (an 8x8 font cell is Scale*8 square)
}

// NativeGeometry is the geometry of an image that already *is* the raw
// 256x192 Spectrum display, pixel for pixel -- what ToImage produces, and
// what a correctly-cropped screenshot should be resized or cropped to
// before OCR if a caller does not want to specify Geometry directly.
var NativeGeometry = Geometry{OriginX: 0, OriginY: 0, Scale: 1}

// glyphBits is an 8x8 boolean bitmap: true where the ROM font sets ink.
type glyphBits [8][8]bool

// glyphTable is built once (via newGlyphTable) and matched against many
// times; building it is not free (rendering 96 glyphs), so callers doing
// repeated recognition should build it once and reuse it via
// RecognizeTextWithTable.
type glyphTable map[rune]glyphBits

// newGlyphTable renders every printable character (32..127) from the
// Sinclair ROM font and reduces each to an 8x8 boolean bitmap, keyed by
// rune.
func newGlyphTable() (glyphTable, error) {
	font, err := bdf.Parse(bytes.NewReader(sinclairFont))
	if err != nil {
		return nil, err
	}
	table := make(glyphTable)
	ink := color.NRGBA{
		R: 0,
		G: 0,
		B: 0,
		A: 255,
	}
	for r := rune(32); r < 128; r++ {
		img, ok := font.GlyphImage(r, ink)
		if !ok {
			continue
		}
		var bits glyphBits
		b := img.Bounds()
		for y := 0; y < 8 && y < b.Dy(); y++ {
			for x := 0; x < 8 && x < b.Dx(); x++ {
				_, _, _, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
				bits[y][x] = a != 0
			}
		}
		table[r] = bits
	}
	return table, nil
}

// isInk reports whether a screenshot pixel counts as foreground ("ink")
// rather than background ("paper"), given this cell's own dominant colour
// as the paper reference.
//
// An absolute darkness threshold (treat anything dark as ink) is the
// obvious first approach, and it is exactly right for plain black-on-grey
// text -- but it silently drops any *coloured* ink, such as the blue
// shortcut-letter highlighting NextZXOS menus use, since a saturated blue
// has a high blue channel even though it is visually nothing like grey
// paper. The correct test is distance from the actual local paper colour,
// not absolute darkness, so any non-paper colour -- black, blue, or
// otherwise -- counts as ink.
func isInk(c, paper color.Color) bool {
	pr, pg, pb, _ := paper.RGBA()
	r, g, b, _ := c.RGBA()
	return absDiff(r, pr)+absDiff(g, pg)+absDiff(b, pb) > 0x6000
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// sampleCell extracts one 8x8 boolean bitmap from the source image at
// character-cell column,row (0-based), by majority-voting each emulated
// pixel's Scale x Scale block of real source pixels. Majority voting
// (rather than sampling a single pixel per cell) makes this robust to the
// geometry being off by a pixel or two.
//
// The paper reference is this cell's own dominant colour, not a single
// screen-wide assumption: a real Spectrum/Next screen can have
// differently-coloured regions (a NextZXOS menu box is white on a grey
// desktop background, for instance), so a single global paper colour
// handles a uniform-background screen correctly but produces blank cells
// throughout a screen with mixed backgrounds.
func sampleCell(img image.Image, geom Geometry, col, row int) glyphBits {
	cellX := geom.OriginX + col*8*geom.Scale
	cellY := geom.OriginY + row*8*geom.Scale
	cellW := 8 * geom.Scale

	counts := make(map[color.RGBA]int)
	for y := 0; y < cellW; y++ {
		for x := 0; x < cellW; x++ {
			r, g, b, a := img.At(cellX+x, cellY+y).RGBA()
			counts[color.RGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: uint8(a >> 8),
			}]++
		}
	}
	var paper color.RGBA
	bestCount := 0
	for c, n := range counts {
		if n > bestCount {
			bestCount = n
			paper = c
		}
	}

	var bits glyphBits
	for py := 0; py < 8; py++ {
		for px := 0; px < 8; px++ {
			ink := 0
			paperVotes := 0
			for sy := 0; sy < geom.Scale; sy++ {
				for sx := 0; sx < geom.Scale; sx++ {
					x := cellX + px*geom.Scale + sx
					y := cellY + py*geom.Scale + sy
					if isInk(img.At(x, y), paper) {
						ink++
					} else {
						paperVotes++
					}
				}
			}
			bits[py][px] = ink > paperVotes
		}
	}
	return bits
}

// hammingDistance counts differing pixels between two 8x8 bitmaps.
func hammingDistance(a, b glyphBits) int {
	d := 0
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if a[y][x] != b[y][x] {
				d++
			}
		}
	}
	return d
}

// bestMatch finds the glyph table entry closest to the sampled cell. An
// all-paper cell is recognised as a space directly, rather than via
// distance matching, since several real glyphs (space itself, and a couple
// of punctuation marks) are sparse enough to otherwise be confused with a
// genuinely blank cell.
func bestMatch(table glyphTable, sampled glyphBits) rune {
	empty := true
	for y := 0; y < 8 && empty; y++ {
		for x := 0; x < 8; x++ {
			if sampled[y][x] {
				empty = false
				break
			}
		}
	}
	if empty {
		return ' '
	}

	best := rune(' ')
	bestDist := 65 // worse than any possible 8x8 distance (max 64)
	for r, bits := range table {
		if d := hammingDistance(bits, sampled); d < bestDist {
			bestDist = d
			best = r
		}
	}
	return best
}

// RecognizeText reads every one of the 32x24 character cells of img at the
// given geometry, and returns the result as 24 lines of text, each
// right-trimmed of trailing spaces (matching how the real screen would
// print -- trailing blank cells carry no information). Text outside the
// standard 96-glyph ROM set (box-drawing borders, UDGs, decorative
// graphics) is matched to its nearest visual approximation rather than
// skipped; callers reading known-textual regions can ignore this, and
// callers who need to tell text from graphics should crop to the region
// that is actually text first.
func RecognizeText(img image.Image, geom Geometry) ([]string, error) {
	table, err := newGlyphTable()
	if err != nil {
		return nil, err
	}
	return recognizeWithTable(img, geom, table), nil
}

// RecognizeScreen is RecognizeText specialised for an already-decoded
// Screen: no Geometry is needed, since a Screen is always exactly the
// native 256x192 pixels with no border or scaling to account for.
func RecognizeScreen(s *Screen) ([]string, error) {
	return RecognizeText(ToImage(s), NativeGeometry)
}

func recognizeWithTable(img image.Image, geom Geometry, table glyphTable) []string {
	lines := make([]string, 24)
	for row := 0; row < 24; row++ {
		var b strings.Builder
		for col := 0; col < 32; col++ {
			cell := sampleCell(img, geom, col, row)
			b.WriteRune(bestMatch(table, cell))
		}
		lines[row] = strings.TrimRight(b.String(), " ")
	}
	return lines
}
