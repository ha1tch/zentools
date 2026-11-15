# zentools library manual

The zentools library is a set of Go packages for reading, writing, and
converting ZX Spectrum file formats in memory. Every package depends only on the
Go standard library. This manual describes each package, its principal types and
functions, and how they compose.

For the command-line tools built on these packages, see the
[CLI tools manual](CLI.md).

## Contents

- [Overview](#overview)
- [pkg/tap](#pkgtap)
- [pkg/tzx](#pkgtzx)
- [pkg/pzx](#pkgpzx)
- [pkg/basic](#pkgbasic)
- [pkg/scr](#pkgscr)
- [pkg/zxn](#pkgzxn)
- [pkg/nex](#pkgnex)
- [pkg/zx0](#pkgzx0)
- [pkg/snapshot](#pkgsnapshot)
- [pkg/szx](#pkgszx)
- [pkg/rzx](#pkgrzx)
- [pkg/build](#pkgbuild)
- [pkg/version](#pkgversion)
- [Composing the packages](#composing-the-packages)

## Overview

The import path root is `github.com/ha1tch/zentools`. Each package owns one
format family:

| Package         | Responsibility                                                |
| --------------- | -------------------------------------------------------------- |
| `pkg/tap`       | TAP tape images.                                                |
| `pkg/tzx`       | TZX tape images.                                                |
| `pkg/pzx`       | PZX tape images, the simpler TZX successor.                     |
| `pkg/basic`     | ZX BASIC tokenisation and detokenisation.                       |
| `pkg/scr`       | `.scr` screens, ZXMODE pictures and ZXPAL palettes, image conversion, `.cut` asset collections, OCR. |
| `pkg/zxn`       | ZX Spectrum Next graphics: palettes, sprites and their attributes, Layer 2, LoRes and Timex screens, tiles, and a colour reducer. |
| `pkg/nex`       | NEX files, the Spectrum Next executable format. |
| `pkg/zx0`       | ZX0 compression and decompression. |
| `pkg/snapshot`  | `.sna` and `.z80` snapshots, via a neutral state type.           |
| `pkg/szx`       | `.szx` (zx-state) snapshots, via the same neutral state type.   |
| `pkg/rzx`       | `.rzx` input-recording files.                                    |
| `pkg/build`     | Overlaying code onto a boot state to emit artifacts.             |
| `pkg/bdf`       | BDF bitmap fonts, read for `pkg/scr`'s text recognition.          |
| `pkg/version`   | The library version constant.                                    |

The packages do not import one another except that `pkg/build` builds on `tap`,
`tzx`, `basic`, `snapshot` and `szx`, `pkg/rzx` uses `pkg/snapshot`/`pkg/szx` to
decode an embedded snapshot block, `pkg/szx` shares `pkg/snapshot`'s machine
state, `pkg/scr`'s OCR reads its font with `pkg/bdf`, and `pkg/zxn` takes the
Spectrum ULA colours from `pkg/scr`. A typical flow runs from source material (a
binary or BASIC text), through `build`, out to a tape or snapshot format.

Install with:

```sh
go get github.com/ha1tch/zentools@latest
```

Requires Go 1.25 or later.

## pkg/tap

Reads and writes TAP tape images. A TAP image is a sequence of length-prefixed
blocks; each program is represented by a header block followed by a data block.

### Encoding

```go
func EncodeCode(name string, data []byte, loadAddress uint16) []byte
func EncodeProgram(name string, data []byte, autostart uint16) []byte
func WriteCodeFile(inputPath, outputPath, name string, loadAddress uint16) error
```

`EncodeCode` returns the complete TAP bytes for a CODE block — a header naming
the block and recording its load address, followed by the data. `EncodeProgram`
does the same for a BASIC Program block, recording an auto-run line in place of a
load address. `WriteCodeFile` is a file-to-file convenience wrapper around
`EncodeCode`.

The CODE-block encoding is verified byte-identical to pasmo's `--tap` output.

### Decoding

```go
func Decode(image []byte) ([]Block, error)
func DecodeBlock(raw []byte) (Block, error)
```

`Decode` parses a TAP image into its blocks. `DecodeBlock` parses one
block's raw bytes (flag, payload and checksum, without TAP's length
prefix), as a TZX carries them:

```go
type Block struct {
    IsHeader   bool
    Flag       byte   // 0x00 for a standard header, 0xFF for standard data
    Data       []byte // payload between the flag and the checksum
    Checksum   byte
    ChecksumOK bool   // whether the stored checksum matches the computed one
    Type       byte   // TypeProgram / TypeCode / ...
    Name       string // ten-character name, trailing spaces trimmed
    DataLength uint16
    Param1     uint16 // load address (Code) or autostart line (Program)
    Param2     uint16 // 0x8000 (Code) or program length (Program)
}
```

The header fields (`Type`, `Name`, `DataLength`, `Param1`, `Param2`) are
populated only when `IsHeader` is true. The block type constants are
`TypeProgram` (0x00), `TypeNumArray` (0x01), `TypeCharArray` (0x02), and
`TypeCode` (0x03).

### Example

```go
import "github.com/ha1tch/zentools/pkg/tap"

image := tap.EncodeCode("mygame", code, 0x8000)
os.WriteFile("mygame.tap", image, 0o644)

blocks, err := tap.Decode(image)
if err != nil {
    return err
}
for _, b := range blocks {
    if b.IsHeader && b.Type == tap.TypeCode {
        fmt.Printf("%s loads at 0x%04X\n", b.Name, b.Param1)
    }
}
```

## pkg/tzx

Reads and writes TZX tape images, following the TZX v1.20 specification. TZX is a
richer container than TAP: a signature and version header, then typed blocks. A
TAP image converts to TZX by wrapping each block as a standard-speed (0x10)
block; metadata, hardware-type, and group blocks can be added around the data.

### Encoding from a TAP image

```go
func EncodeFromTAP(tapImage []byte, opts EncodeOptions) ([]byte, error)
```

`EncodeFromTAP` wraps a TAP image as TZX, with optional metadata:

```go
type EncodeOptions struct {
    Title       string         // archive title (0x32)
    Author      string         // archive author
    Year        string         // year of publication
    Description  string         // a text-description block (0x30)
    StopIn48K    bool           // emit a "stop in 48K mode" block (0x2A)
    Pause        uint16         // pause after each data block, ms
    Hardware     []HardwareInfo // hardware-type block (0x33)
    Group        string         // bracket the data in a named group (0x21/0x22)
}
```

A `HardwareInfo` entry names a machine or peripheral and the tape's relationship
to it:

```go
type HardwareInfo struct {
    Type byte // category, e.g. HWComputers
    ID   byte // item within the category, e.g. HWIDSpectrum128K
    Info byte // HWInfoRuns / HWInfoUses / HWInfoRunsUnused / HWInfoDoesntRun
}
```

The package defines the full set of hardware type, identifier, and information
constants from the specification's reference table.

### The incremental writer

For assembling several tape images into one file with structural blocks between
them, `Writer` builds a TZX block by block:

```go
type Writer struct { /* ... */ }

func NewWriter(pause uint16) *Writer
func (w *Writer) ArchiveInfo(opts EncodeOptions)
func (w *Writer) Description(desc string)
func (w *Writer) Hardware(entries []HardwareInfo)
func (w *Writer) GroupStart(name string)
func (w *Writer) GroupEnd()
func (w *Writer) StopIn48K()
func (w *Writer) AddTAP(tapImage []byte) error
func (w *Writer) Bytes() []byte
```

`EncodeFromTAP` is itself implemented on top of `Writer`, so the two share one
code path. Groups cannot nest; the caller pairs each `GroupStart` with a
`GroupEnd`.

### Decoding

```go
func Decode(image []byte) ([]Block, error)
```

`Decode` returns the blocks of a TZX image. The package round-trips every block
type it writes, so a TZX produced by `EncodeFromTAP` or `Writer` decodes
cleanly.

### Example

```go
import "github.com/ha1tch/zentools/pkg/tzx"

image, err := tzx.EncodeFromTAP(tapImage, tzx.EncodeOptions{
    Title:  "My Game",
    Author: "haitch",
    Year:   "2026",
    Hardware: []tzx.HardwareInfo{
        {Type: tzx.HWComputers, ID: tzx.HWIDSpectrum128K, Info: tzx.HWInfoUses},
    },
    Group: "Main Program",
})
```

## pkg/pzx

Reads and writes PZX tape images (spec v1.0), Patrik Rak's simpler
successor to TZX. A `File` is an ordered list of `Block`s: `HeaderBlock`
(always first), `PulseBlock`, `DataBlock` (a byte stream as pulse-encoded
bits), `PauseBlock`, `BrowseBlock`, `StopBlock`, and `RawBlock` for any
tag this package does not otherwise model -- preserved verbatim on a
round trip, per the spec's own rule for unrecognised tags.

```go
func Decode(image []byte) (*File, error)
func Encode(f *File) ([]byte, error)
```

See the package's own doc comments for each block type's fields, and
[the CLI manual's `zx pzx` section](CLI.md#zx-pzx) for the command-line
surface.

## pkg/basic

Tokenises ZX BASIC source text into the Spectrum's internal byte representation,
and detokenises it back. Both the 48K and 128K keyword sets are supported; the
128K-only keywords `SPECTRUM` and `PLAY` are recognised.

```go
func Tokenise(src string, opts ...Option) ([]byte, error)
func Detokenise(prog []byte) (string, error)
func LooksTokenised(data []byte) bool
func CaseSensitive() Option
```

`Tokenise` converts source text to tokens. By default, keyword matching is
case-independent; passing `CaseSensitive()` requires exact keyword case, so that
lowercase words are kept as literal text rather than tokenised. `Detokenise`
reverses the process. `LooksTokenised` reports whether a byte slice appears to be
tokenised BASIC rather than plain text.

### Example

```go
import "github.com/ha1tch/zentools/pkg/basic"

tokens, err := basic.Tokenise("10 PRINT \"HELLO\"\n")
text, err := basic.Detokenise(tokens)
```

## pkg/scr

Reads and writes ZX Spectrum `.scr` screen files (the raw display: 6912
bytes, pixel bitmap plus per-cell colour attributes, exactly as they sit in
Spectrum RAM), converts between `.scr` and ordinary images, and manages
`.cut` asset collections (named sub-regions cut from one or more screens,
for sprite/tile reuse). It also handles the ZXMODE screen modes and ZXPAL
palettes, below. The [CLI manual's `zx scr` section](CLI.md#zx-scr) and the
[sprite](SPRITE-HANDLING.md) and [ZXMODE](ZXMODE-HANDLING.md) handling
manuals show the same operations from the command line.

### Classic screens

```go
type Attribute struct {
	Ink, Paper    uint8 // 0-7
	Bright, Flash bool
}
func (a Attribute) Byte() byte
func AttributeFromByte(b byte) Attribute
func (a Attribute) InkRGBA() color.RGBA
func (a Attribute) PaperRGBA() color.RGBA
func ParseAttribute(s string) (Attribute, error) // "ink:white; paper:blue; bright:1"
func ParseColour(s string) (uint8, error)
func ULAColor(index uint8, bright bool) color.RGBA

type Screen struct {
	Ink  [192][256]bool        // [y][x]; true shows the cell's ink
	Attr [24][32]Attribute     // [cellY][cellX]
}
func Encode(s *Screen) []byte              // 6912 bytes, in the display's own interleave
func Decode(data []byte) (*Screen, error)
```

`Encode` and `Decode` convert between a `Screen` and the 6912-byte file.
A wrong size is reported as a `*SizeError`.

```go
func DecodeImage(r io.Reader) (image.Image, error) // PNG, JPEG, GIF or BMP
type ResizeMode int // ResizeNone, ResizeStretch, ResizeBestFit, ResizeCentre, ResizeFill
func Fit(src image.Image, mode ResizeMode, fill color.Color) (image.Image, error)
func FromImage(img image.Image) (*Screen, error) // a 256x192 image
func ToImage(s *Screen) *image.RGBA
func ToImageBitmap(s *Screen, bits int, ink, paper color.Color, paperTransparent bool) *image.RGBA
```

`FromImage` is the classic converter: for each cell it takes the darkest
colour present as paper and the brightest as ink, decides one bright bit
for the cell, and sets each pixel to the nearer of the two.
`Fit` brings an image of any size to 256x192 first; `FitTo` and `FitToWith`
(below) fit to other sizes. `ToImageBitmap` draws the shape alone, with the
given ink and paper.

```go
type Rect struct{ X, Y, W, H int }
func CellRect(cx, cy, cw, ch int) Rect
func Crop(src image.Image, r Rect) (*image.RGBA, error)
func AutoExtent(src image.Image, bg color.Color, tol int) (Rect, error)
func BitmapExtent(s *Screen, bits int) (Rect, error)
```

`AutoExtent` finds the box around an image's non-background pixels, and
`BitmapExtent` the box around a screen's set (or, with `bits` 0, clear)
bitmap bits.

### Asset collections

```go
type Asset struct {
	Name          string
	Width, Height int
	IsMask        bool
	Ink           [][]bool      // [y][x]
	Attr          [][]Attribute // one-byte palettes, or nil
	Palette       Palette
	Attr2         [][]CellAttr  // ZXPAL3 and ZXPAL4, or nil
	Registers     Registers     // the asset's own register table, or nil
}
type Collection struct{ Assets []Asset }
func (c *Collection) Find(name string) *Asset
func EncodeCollection(c *Collection) ([]byte, error) // a .cut file (ZCUT)
func DecodeCollection(data []byte) (*Collection, error)

func CutRegion(s *Screen, r Rect, name string, keepAttrs bool) (*Asset, error)
func CutCells(s *Screen, cx, cy, cw, ch int, name string, keepAttrs bool) (*Asset, error)
type PasteOp int // PasteOR, PasteAND, PasteCOPY, PasteXOR
func Paste(s *Screen, a *Asset, x, y int, op PasteOp) error
func ApplyMask(a, mask *Asset) error
func (a *Asset) MapAttributes(f func(Attribute) Attribute)
func AssetToImage(a *Asset, ink, paper color.Color, paperTransparent bool) *image.RGBA
```

An asset keeps its attributes only when cut on cell boundaries with
`keepAttrs`. `Paste` combines the asset's bits with the screen's by the
given operation and writes its attributes when the position is
cell-aligned. `ApplyMask` clears an asset's bits outside a mask asset, and
`MapAttributes` rewrites every cell's attribute. The ZCUT format, its
three versions and assets in other palettes are described under
[Assets across palettes](#assets-across-palettes).

### ZXMODE pictures and ZXPAL palettes

The ZXMODE screen modes and ZXPAL palettes (see [the CLI manual](CLI.md#zxmode-modes-and-zxpal-palettes)
for what they are) have their own types alongside `Screen`.

```go
type Palette uint8 // ZXPAL0 .. ZXPAL4
func ParsePalette(s string) (Palette, error)
func (p Palette) Registers() []color.RGBA
func (p Palette) Last() int
func (p Palette) TwoByte() bool
func (p Palette) CellRegisters(a, b byte, flashPhase bool) (ink, paper int)

type Registers []color.RGBA // a loaded register table; nil means the reset values
func (r Registers) Or(p Palette) Registers
func (r Registers) Equal(o Registers) bool
func CheckRegisters(p Palette, regs Registers) error
func RegistersCSV(regs Registers) []byte
func ParseRegistersCSV(data []byte, p Palette) (Registers, error)

type Mode uint8 // ZXMODE0 .. ZXMODE2
func ParseMode(s string) (Mode, error)
func (m Mode) Size() (w, h int)

type CellAttr struct{ A, B byte }
type Picture struct {
	Mode      Mode
	Palette   Palette
	Registers Registers
	Width     int
	Height    int
	Ink       [][]bool     // [y][x]
	Attr      [][]CellAttr // [cellY][cellX]
}
func NewPicture(mode Mode, pal Palette) (*Picture, error)
func EncodePicture(img image.Image, mode Mode, pal Palette) (*Picture, error)
func EncodePictureWith(img image.Image, mode Mode, pal Palette, opt EncodeOptions) (*Picture, error)
type EncodeOptions struct {
	Registers   Registers
	LinearPairs bool    // experimental
	Diffuse     float64 // experimental
	FlatSpread  float64 // experimental
}
func DecodePicture(data []byte, mode Mode, pal Palette) (*Picture, error)
func PictureLen(mode Mode, pal Palette) int
func (p *Picture) Bytes() []byte
func (p *Picture) Render(flashPhase bool) *image.RGBA
func (p *Picture) Flip(h, v bool)
func (p *Picture) BitmapImage(bits int, ink, paper color.Color, paperTransparent bool) *image.RGBA
func PictureExtent(p *Picture, bits int) (Rect, error)
func RecognizePicture(p *Picture) ([]string, error)
```

`Registers` generates a palette from the specification's rules; the tests
check ZXPAL4 against its published table. A `CellAttr` holds the attribute
byte in `A` for a one-byte palette, and the ink and paper registers in `A`
and `B` for ZXPAL3 and ZXPAL4. A picture's `Registers`, when set, are the
loaded register colours it is drawn in (the files do not store them).
`EncodePicture` is the specification's reference encoder, pixel for pixel,
and needs an image of exactly the mode's size; `EncodePictureWith` encodes
for loaded registers and carries the experimental options of Part 4 of the
specification (with all of them zero it is the reference encoder). `Bytes`
and `DecodePicture` convert to and from the stored form (`PictureLen`
bytes: a `.scr` for ZXMODE0, the linear layout for ZXMODE1 and ZXMODE2).
`RecognizePicture` reads text from the bitmap, as `RecognizeScreen` reads it
from a classic screen's colours.

#### Fitting and measuring

```go
func FitTo(src image.Image, mode ResizeMode, fill color.Color, w, h int) (image.Image, error)
func FitToWith(src image.Image, mode ResizeMode, fill color.Color, w, h int, opt FitOptions) (image.Image, error)
type Filter int // FilterDefault, FilterLanczos
func ParseFilter(s string) (Filter, error)
type FitOptions struct {
	Filter           Filter
	Trim             int     // pixels removed from every edge first
	Centre           bool    // use CentreX, CentreY to place ResizeFill's crop
	CentreX, CentreY float64 // 0 to 1
}
func ResizeLanczos(src image.Image, w, h int, box [4]float64) (*image.RGBA, error)
func FitLanczos(src image.Image, w, h int, cx, cy float64) (*image.RGBA, error)
func Trim(src image.Image, n int) (image.Image, error)
func Grey(img image.Image) *image.RGBA

func BlurredRMSE(ref, img image.Image, sigma float64) (float64, error)
func SSIM(ref, img image.Image) (float64, error)
func BlurPillow(plane []uint8, w, h int, radius float64) []uint8
```

`ResizeLanczos` is Pillow's `Image.resize` with `LANCZOS` and a source box,
`FitLanczos` is `ImageOps.fit`, and `Grey` is `convert("L")`; each gives
Pillow's bytes exactly, so `FitToWith` with `ResizeFill` and
`FilterLanczos` is the specification's picture fit. `BlurredRMSE` is the
specification's error measure (sigma 1.5, or 0.75 for the lighter one),
with Pillow's Gaussian blur (`BlurPillow`, one channel); `SSIM` is
scikit-image's `structural_similarity` with its defaults over the three
channels. `DecodeImage` rebuilds a JPEG's colour with libjpeg-turbo's chroma
interpolation.

#### Trying palette designs

```go
type Banks [][]color.RGBA
func PaletteBanks(p Palette, regs Registers) Banks
func BanksCSV(banks Banks) []byte
func ParseBanksCSV(data []byte) (Banks, error)
func (b Banks) Len() int

type BankOptions struct {
	CellW, CellH int     // 0 means 8
	Penalty      float64 // RefPenalty (0.204) is the reference encoder's
}
func EncodeBanks(img image.Image, banks Banks, opt BankOptions) (*image.RGBA, [][]int, error)
func ParseCellSize(s string) (w, h int, err error)

type FitRegistersOptions struct {
	Add        int     // colours to add
	PerPicture int     // the rest default as the specification's tools set them
	Candidates int
	Subsample  int
	Iterations int
	MinMembers int
	Blur       float64
	Seed       uint64
}
type FitStats struct{ MeanDistance, FarShare float64 }
func FitRegisters(pictures []image.Image, base []color.RGBA, opt FitRegistersOptions) ([]color.RGBA, FitStats, FitStats, error)
```

`EncodeBanks` is the encoder of the specification's tools (`zxcommon.render`,
`zxfast.render_fast`) over any colours: each cell takes its two colours
from one bank, cells may be any size dividing the picture (4x4 dithers with
the 4x4 Bayer matrix, other sizes than 8x8 with the 8x8 matrix over the
whole picture), and the span penalty is a parameter. It returns a picture
and the bank chosen per cell, not a screen; over `PaletteBanks` with 8x8
cells and `RefPenalty` it draws what `EncodePicture` encodes, and the tests
check 4x4 cells, 4x8 cells and a penalty of 0.5 against the tools' own
output. `EncodeOptions.Penalty` sets the penalty for an encoding that is
still a screen. `FitRegisters` is the specification's register search
(`tools/z4_palette_search.py`): pixels sampled from the blurred pictures,
colours added greedily from candidates drawn from the sample, then refined
by k-means with the base colours fixed. Its random draws are its own
(SplitMix64, seeded), so a fit repeats exactly but does not reproduce the
Python one's colours.

#### Assets across palettes

```go
type Asset struct {
	// ... bitmap, mask, Attr (one-byte palettes) as before
	Palette   Palette
	Attr2     [][]CellAttr // two-byte palettes
	Registers Registers    // the asset's own register table, or nil
}
func (a *Asset) Matches(p Palette, regs Registers) bool
func (a *Asset) Flip(h, v bool)
func ConvertAsset(a *Asset, to Palette, regs Registers) (*Asset, error)
func (p *Picture) Asset(name string) *Asset
func PictureFromAsset(a *Asset, mode Mode) (*Picture, error)
func CutPicture(p *Picture, r Rect, name string, keepAttrs bool) (*Asset, error)
func PastePicture(p *Picture, a *Asset, x, y int, op PasteOp) (converted bool, err error)
```

An asset records its palette, which fixes the attribute format, and its
register table when it was cut from a picture with loaded registers; a
collection with any register table is written as ZCUT version 3.
`ConvertAsset` rewrites the attributes for another palette and registers,
keeping the bitmap: each cell takes the target colours nearest its ink and
paper (in sRGB), weighted by the pixels each covers, within one bank for a
one-byte palette. `PastePicture` converts on its own when the asset does not
match the picture, and reports it; `Paste` onto a classic `Screen` converts
to ZXPAL0 with the reset registers.

### OCR

Recognises the text on a screen by matching each 8x8 character cell against
the real Spectrum ROM font, returning it as plain lines of text.

```go
func RecognizeScreen(s *Screen) ([]string, error)
func RecognizeText(img image.Image, geom Geometry) ([]string, error)

type Geometry struct {
	OriginX int
	OriginY int
	Scale   int
}

var NativeGeometry = Geometry{OriginX: 0, OriginY: 0, Scale: 1}
```

`RecognizeScreen` takes an already-decoded `Screen` directly; since a
`Screen` is always exactly the native 256x192 pixels, no geometry is
needed. `RecognizeText` is the general form, for any `image.Image` — most
often a captured emulator window screenshot, where the 256x192 display sits
at some offset and scale within a larger image. `NativeGeometry` is the
identity case (origin 0,0, scale 1), for an image that already *is* the raw
256x192 display, pixel for pixel.

Both return 24 lines of text, one per character row, each right-trimmed of
trailing spaces. Recognition matches every cell against the standard
96-glyph ROM set (codes 32-127); a cell containing something outside that
set — a box-drawing border, a UDG, decorative graphics — is matched to its
nearest visual approximation rather than skipped, so a caller reading a
region that mixes text and graphics should expect some garbage characters
there rather than blanks.

### Example

```go
import "github.com/ha1tch/zentools/pkg/scr"

// A .scr file: no geometry needed.
data, _ := os.ReadFile("screen.scr")
screen, _ := scr.Decode(data)
lines, _ := scr.RecognizeScreen(screen)

// A captured emulator window: say where the display sits.
f, _ := os.Open("screenshot.png")
img, _ := png.Decode(f)
geom := scr.Geometry{OriginX: 256, OriginY: 112, Scale: 3}
lines, _ = scr.RecognizeText(img, geom)
```

## pkg/zxn

ZX Spectrum Next graphics: palettes in four layouts, hardware sprite patterns
and attributes, Layer 2 and LoRes bitmaps, Timex screens, tiles, tilemaps and
blocks, and a colour reducer. See [the CLI manual](CLI.md#zx-next) for the file
layouts.

```go
// Colours and palettes
type RoundMode int // RoundNearest, RoundFloor, RoundCeil, RoundDistance
func ParseRoundMode(s string) (RoundMode, error)
type Color333 struct {
	R, G, B  uint8 // 0-7
	Priority bool  // Layer 2 priority bit (bit 7 of an entry's second byte)
}
func ToRGB333(c color.Color, mode RoundMode) Color333
func (c Color333) RGBA() color.RGBA
type Palette struct{ Colors []Color333 }
func (p *Palette) Padded(n int) *Palette
func EncodeNXP(p *Palette) ([]byte, error)
func DecodeNXP(data []byte) (*Palette, error)
type Layout int // LayoutRGB333, LayoutRGB332, LayoutGBR444, LayoutBGR222
func ParseLayout(s string) (Layout, error)
func EncodePalette(p *Palette, l Layout, mode RoundMode) ([]byte, error)
func DecodePalette(data []byte, l Layout) (*Palette, error)         // RGB333 and RGB332
func DecodePaletteRGBA(data []byte, l Layout) ([]color.RGBA, error) // any layout
func StandardPalette() *Palette
func StandardIndex(c color.Color, mode RoundMode) uint8
func StandardTable(pal color.Palette, mode RoundMode) []uint8
func Minimize(p *Palette) (*Palette, int, []uint8)
func RemapImage(img *image.Paletted, table []uint8, pal *Palette) *image.Paletted
func PaletteFromImage(img image.Image, max int, mode RoundMode) (*Palette, error)

// Choosing colours
type ReduceOptions struct {
	Colors      int
	Round       RoundMode
	Dither      Dither   // DitherNone, DitherFloydSteinberg
	Transparent int      // -1 for none; the Next's default is DefaultTransparent (0xE3)
	Palette     *Palette // map to this palette instead of choosing one
}
func Reduce(img image.Image, opt ReduceOptions) (*image.Paletted, *Palette, error)

// Sprites
type Bitmap struct {
	Width, Height int
	Depth         int // 8 or 4 when stored
	Order         PixelOrder // RowMajor, ColumnMajor
	Indices       []uint8    // row by row, one per pixel
	Palette       *Palette
}
type SpriteSheet struct {
	Depth   int
	Sprites []Bitmap
}
func EncodeSPR(s *SpriteSheet) ([]byte, error)
func DecodeSPR(data []byte, depth int) (*SpriteSheet, error)
func SpritesFromImage(img image.Image, pal *Palette, depth int, mode RoundMode) (*SpriteSheet, error)
func SheetImage(s *SpriteSheet, pal *Palette, perRow, transparent int) *image.RGBA
func PaletteImage(p *Palette) *image.RGBA
type SpriteAttr struct { /* X, Y, PaletteOffset, MirrorX, ..., Relative, FourBit, N6, ... */ }
func (a *SpriteAttr) Bytes() ([]byte, error)
func ParseSpriteAttr(data []byte) (SpriteAttr, int, error)
func EncodeAttrs(attrs []SpriteAttr, fixed5 bool) ([]byte, error)
func DecodeAttrs(data []byte, fixed5 bool) ([]SpriteAttr, error)

// Layer 2, LoRes and Timex screens
var Layer2Modes, LoResModes []Layer2Mode // name, size, depth, order
func BitmapFromImage(img image.Image, pal *Palette, depth int, order PixelOrder, mode RoundMode) (*Bitmap, error)
func EncodeNXI(b *Bitmap) ([]byte, error)
func DecodeNXI(data []byte, w, h, depth int, order PixelOrder) (*Bitmap, error)
func BitmapImage(b *Bitmap, pal *Palette, transparent int) *image.RGBA
type HiRes struct { Ink uint8; Pixels [192][512]bool }
type HiColour struct { Pixels [192][256]bool; Attr [192][32]uint8 }
func HiResFromImage(img image.Image, ink uint8) (*HiRes, error)
func HiColourFromImage(img image.Image) (*HiColour, error)
func DecodeHiRes(data []byte, ink uint8) (*HiRes, error)
func DecodeHiColour(data []byte) (*HiColour, error)

// Tiles
type TileOptions struct {
	Width, Height, Depth    int
	Dedup                   Dedup // DedupNone, DedupRepeat, DedupMirror, DedupRotate
	Offset, PaletteOffset   int
	BlockWidth, BlockHeight int
	BlockDedup              bool
}
func CutTiles(img *image.Paletted, opt TileOptions) (*TileSet, *TileMap, *BlockSet, error)
func EncodeNXT(ts *TileSet) ([]byte, error)
func DecodeNXT(data []byte, w, h, depth int) (*TileSet, error)
func EncodeNXM(m *TileMap, sixteen, columns bool) ([]byte, error)
func DecodeNXM(data []byte, w, h int, sixteen, columns bool) (*TileMap, error)
func EncodeNXB(bs *BlockSet, sixteen bool) ([]byte, error)
func TileAt(t []uint8, w, h int, attr uint8, x, y int) uint8
func MapImage(ts *TileSet, m *TileMap, pal *Palette, offset int) *image.RGBA
func TilesImage(ts *TileSet, pal *Palette, perRow int) *image.RGBA
```

A channel becomes 3 bits as c x 7 / 255 rounded to the nearest level, down or
up, or to the level nearest in 8-bit value (`RoundDistance`, gfx2next's
default), and a level v shows as round(v x 255 / 7), as the community tools
compute them. The tests check palettes, sprites, Layer 2 bitmaps, tiles, maps
and blocks byte for byte against gfx2next, nextbmp and nextraw, and document
where those tools disagree. An indexed image keeps its palette and its pixel
indices; `Reduce` chooses colours for any other image (exactly, when it has few
enough RGB333 colours, otherwise by median cut) or maps it onto a given
palette, with optional Floyd-Steinberg dithering. The package began as zenpal,
a separate palette and sprite tool.

## pkg/nex

Reads and writes NEX files, the Next's executable format: a 512-byte header,
an optional loading screen with its palette, optional copper code, 16 KB banks
in the order 5, 2, 0, 1, 3, 4, 6, 7, 8 and on, and any data after them.

```go
type File struct {
	Version                   int // 2 or 3; 0 for the lowest that fits
	PC, SP                    uint16
	EntryBank, Border         uint8
	RAM2MB, Checksum          bool
	CoreVersion               [3]uint8
	Screens, Screen2          uint8            // loading screen flags
	Palette                   []byte           // 512 bytes or nil
	Screen                    map[uint8][]byte // by screen flag
	Copper                    []byte
	Banks                     map[int][]byte   // 16 KB banks by number
	After                     []byte
	StoredChecksum            uint32
	ChecksumOK                bool
	// and the remaining header fields
}
func Encode(f *File) ([]byte, error)
func Decode(data []byte) (*File, error)
func Checksum(file []byte) uint32 // CRC-32C of bytes 512 on, then 0-507
```

The layout follows sjasmplus's `SAVENEX`, and the tests read six files
sjasmplus wrote and write them back byte for byte, except for sjasmplus's V1.3
checksum, which covers a stale copy of the header (`ChecksumOK` reports it;
`Encode` always writes the checksum of the file it writes).

## pkg/zx0

ZX0 compression, a port of Einar Saukas's reference compressor with the same
optimal parse, so the same bytes.

```go
type Options struct {
	Quick     bool // offsets up to MaxOffsetQuick (2176)
	Backwards bool // for in-place decompression from the end
	Classic   bool // the v1 format, as gfx2next writes
}
func Compress(data []byte, opt Options) ([]byte, error)
func Decompress(data []byte, opt Options, limit int) ([]byte, error)
```

Compression takes time in proportion to the data's length times the offsets
searched (up to 32640): about 3 seconds for 13 KB on an ordinary machine, and
45 seconds for an 80 KB Layer 2 screen.

## pkg/snapshot

Reads and writes ZX Spectrum snapshots through a neutral machine-state type,
independent of any emulator. The `.sna` (48K and 128K) and `.z80` (versioned,
compressed) formats are supported.

### The machine state

Every encoder and decoder pivots on one type:

```go
type MachineState struct {
    Model  Model
    CPU    CPU
    Paging Paging
    IO     IO
    Memory Memory
}
```

`Memory` holds the eight 16-kilobyte RAM banks; `CPU` holds the registers,
interrupt mode, and flags; `Paging` holds the 128K paging ports. The model
constants are `Model48K`, `Model128K`, `ModelPlus2`, `ModelPlus2A`, and
`ModelPlus3`; the helper `Is128KFamily` reports whether a model is a 128K
variant.

### Encoding and decoding

```go
func EncodeSNA(s *MachineState) ([]byte, error)
func DecodeSNA(image []byte) (*MachineState, error)
func EncodeSNA128(s *MachineState) ([]byte, error)
func DecodeSNA128(image []byte) (*MachineState, error)
func EncodeZ80(s *MachineState) ([]byte, error)
func EncodeZ80v3(s *MachineState) ([]byte, error)
func DecodeZ80(image []byte) (*MachineState, error)
```

`EncodeSNA` and `EncodeSNA128` write 48K and 128K SNA images respectively;
`DecodeSNA` and `DecodeSNA128` read them. `EncodeZ80` and `EncodeZ80v3` write Z80
images, and `DecodeZ80` reads any supported Z80 version. Decoding is verified
against snapshots produced by z88dk and other tools.

A detail of the 48K SNA format is worth noting: it has no program-counter field
and stores the program counter on the stack. The encoders and decoders handle
this symmetrically, so a round trip preserves both the program counter and the
stack pointer.

### Example

```go
import "github.com/ha1tch/zentools/pkg/snapshot"

state, err := snapshot.DecodeZ80(data)
if err != nil {
    return err
}
sna, err := snapshot.EncodeSNA(state)
```

## pkg/szx

Reads and writes `.szx` (zx-state) snapshots (spec v1.5) via the same
neutral `snapshot.MachineState` the `.sna`/`.z80` codecs use -- a third
codec for one type, not a separate representation.

```go
func Decode(image []byte) (*snapshot.MachineState, error)
func Encode(s *snapshot.MachineState) ([]byte, error)
```

Covers the CPU/paging/memory state every snapshot needs
(`ZXSTZ80REGS`/`ZXSTSPECREGS`/`ZXSTRAMPAGE`); the spec's thirty-odd
further peripheral block types (AY sound, joysticks, disk controllers)
are skipped on read rather than modelled, per the spec's own rule for
unrecognised blocks.

## pkg/rzx

Reads and writes RZX input-recording files (spec v0.13): a frame-by-frame
log of what every CPU `IN` instruction returned during an actual recorded
session, alongside the snapshot it starts from. A `File` is an ordered
list of `Block`s: `CreatorBlock`, `SnapshotBlock` (embedded, via
`pkg/snapshot`/`pkg/szx`, or external -- a filename reference), the
`Frame`-based `RecordingBlock`, and the DSA `SecurityInfoBlock`/
`SecuritySignatureBlock` pair -- decoded and re-encoded verbatim, not
cryptographically verified.

```go
func Decode(image []byte) (*File, error)
func Encode(f *File) ([]byte, error)
```

See the package's own doc comments for each block type's fields, and
[the CLI manual's `zx rzx` section](CLI.md#zx-rzx) for the command-line
surface.

## pkg/build

Turns machine-code bytes into loadable artifacts. It overlays the code onto a
real booted machine state — one embedded boot snapshot per model — and emits
tapes and snapshots that load and run at a given entry point. This is the shared
procedure behind both `zenas build` and `zx snap`.

### The request

```go
type Request struct {
    Name   string // program name (<= 10 characters on tape)
    Code   []byte // assembled machine code
    Origin uint16 // load address of the first byte of Code
    Start  uint16 // entry point for snapshot output
    SP     uint16 // stack pointer for snapshot output
    Model  Model  // target model
}
```

The model is a string type with constants `Model48K` (`"48k"`), `Model128K`
(`"128k"`), `ModelPlus2` (`"plus2"`), `ModelPlus2A` (`"plus2a"`), and
`ModelPlus3` (`"plus3"`).

### Emitters

```go
func EncodeTAP(r Request) []byte
func EncodeTAPWithLoader(r Request) ([]byte, error)
func EncodeTZX(r Request) ([]byte, error)
func EncodeTZXFromTAP(tapImage []byte) ([]byte, error)
func EncodeSNA(r Request) ([]byte, error)
func EncodeZ80(r Request) ([]byte, error)
func EncodeSZX(r Request) ([]byte, error)
func Overlay(r Request) (*snapshot.MachineState, error)
```

`EncodeTAP` produces a CODE tape; `EncodeTAPWithLoader` prepends a BASIC auto-run
loader that jumps to `Start`, giving a tape that runs on its own. `EncodeTZX` and
`EncodeTZXFromTAP` produce TZX output. `EncodeSNA`, `EncodeZ80` and `EncodeSZX`
overlay the code onto the model's boot state and produce a runnable snapshot with
the program counter at `Start` and the stack pointer at `SP`. `Overlay` returns
that machine state itself, for a caller that wants another snapshot format.

`Request` also offers `SPWarning`, which returns a non-empty message when the
stack pointer is positioned where it would collide with the loaded code, and
`CodeEnd`, the address just past the last byte of the code.

### Example

```go
import "github.com/ha1tch/zentools/pkg/build"

req := build.Request{
    Name:   "mygame",
    Code:   code,
    Origin: 0x8000,
    Start:  0x8000,
    SP:     0xFF00,
    Model:  build.Model48K,
}
z80, err := build.EncodeZ80(req)   // a runnable snapshot
tape   := build.EncodeTAP(req)     // a CODE tape
```

## pkg/version

Exposes the library version as a string constant, `version.Version`, kept in
sync with the `VERSION` file at the repository root.

## Composing the packages

The packages are designed to flow into one another. A common pipeline takes
assembled code and produces several distribution formats from one request:

- `build.EncodeTAPWithLoader` for a tape that auto-runs on real hardware;
- `build.EncodeZ80` or `build.EncodeSNA` for a snapshot to test in an emulator;
- `tzx.EncodeFromTAP` over a `build.EncodeTAP` result to add archive metadata.

For inspection and conversion, `tap.Decode`, `tzx.Decode`, and the snapshot
decoders all return structured values you can read, transform, and re-encode.
Because every format the library writes it can also read, conversions compose
cleanly within a kind — tape to tape, snapshot to snapshot — while conversions
across the tape/snapshot divide carry the trade-offs described in the
[CLI tools manual](CLI.md).