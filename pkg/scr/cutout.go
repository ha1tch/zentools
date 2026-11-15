// ZCUT asset collections.
//
// A ZCUT file is a container holding any number of independent image assets -
// icons, widgets, sprites, fragments - of differing sizes. Each asset carries
// its own dimensions, a name, and either a colour bitmap with per-cell
// attributes or a plain bitmap (which doubles as a 1-bit transparency mask).
//
// Unlike .scr, a ZCUT stores bitmaps in LINEAR row-major order, not the
// Spectrum display interleave. The interleave is a property of a pixel's
// position on the full screen and is undefined for a free-floating asset, so a
// linear layout is the only well-defined choice. The Spectrum byte conventions
// (1 bit per pixel, one attribute byte per 8x8 cell) are preserved; only the
// ordering is linearised.
//
// Version 2 adds the ZXMODE palettes: each asset records its ZXPAL palette, and
// an asset in a two-byte palette (ZXPAL3, ZXPAL4) stores two attribute planes,
// ink registers then paper registers, as the ZXMODE linear modes lay them out.
// A collection whose assets are all ZXPAL0 is written as version 1, byte for
// byte as before; version 1 files read as ZXPAL0.
//
// Version 3 adds register tables: an asset meant for registers other than its
// palette's reset values (Part 5 makes them loadable) carries its own table of
// register colours, so it renders and converts as it was drawn. A collection
// is written as version 3 only when some asset has a table.
//
// Every attributed asset therefore says how to read its attributes: the
// palette names the attribute format (ZXPAL0 FBPPPIII with flash, ZXPAL1 and
// ZXPAL2 RRPPPIII with a bank, ZXPAL3 and ZXPAL4 one register byte each for
// ink and paper, stored as two planes as ZXMODE lays them out, arrangement A
// of Part 2), and the register table, or its absence, says which colours the
// register numbers stand for. ConvertAsset rewrites an asset for another
// palette or register set.
//
// Layout (all multi-byte fields little-endian):
//
//	file preamble (8 bytes)
//	  +0  "ZCUT"      magic
//	  +4  version u8  = 1; 2 when any asset is not ZXPAL0; 3 when any
//	                  asset carries a register table
//	  +5  flags   u8  reserved (must be 0)
//	  +6  reserved u8 (must be 0)
//	  +7  numAssets u8
//	then numAssets chunks, each:
//	  chunk header (16 bytes)
//	    +0  "IMAG"    tag
//	    +4  payloadLen u32   exact payload size
//	    +8  chunkLen   u32   payload size padded up to a multiple of 8 (stride)
//	    +12 reserved   u32   (must be 0)
//	  payload (chunkLen bytes; payloadLen meaningful, remainder zero pad)
//	    +0  widthPx  u16
//	    +2  heightPx u16
//	    +4  flags    u8   bit0 = has attributes, bit1 = is mask,
//	                      bit2 = two attribute bytes per cell (version 2),
//	                      bit3 = register table follows (version 3)
//	    +5  palette  u8   version 2: ZXPAL palette 0-4; version 1: reserved, 0
//	    +6  reserved u8   (must be 0)
//	    +7  nameLen  u8
//	    +8  name     [roundUp8(nameLen)]  ASCII, zero-padded
//	    ..  bitmap   [ceil(w/8)*h]        linear, row-major
//	    ..  attrs    [cellsW*cellsH]      only if flags bit0; the ink plane
//	                                      when flags bit2
//	    ..  paper    [cellsW*cellsH]      only if flags bit2
//	    ..  registers [3*(last+1)]        only if flags bit3: R, G, B of each
//	                                      of the palette's registers in order
package scr

import (
	"encoding/binary"
	"fmt"
	"image/color"
)

const (
	zcutMagic   = "ZCUT"
	imagTag     = "IMAG"
	zcutVersion = 1
	// zcutVersion2 adds per-asset palettes and two-byte attributes.
	zcutVersion2 = 2
	// zcutVersion3 adds per-asset register tables.
	zcutVersion3 = 3

	assetFlagHasAttrs  = 0x01
	assetFlagIsMask    = 0x02
	assetFlagTwoByte   = 0x04
	assetFlagRegisters = 0x08

	filePreambleLen = 8
	chunkHeaderLen  = 16
	fixedEntryLen   = 8 // w,h,flags,rsv[2],nameLen
)

func roundUp8(n int) int { return (n + 7) &^ 7 }

// Asset is one image in a collection: its dimensions, name, a linear bitmap,
// and (for colour assets) per-cell attributes. A mask asset is a bitmap-only
// asset flagged for use as a stencil.
type Asset struct {
	Name   string
	Width  int
	Height int
	IsMask bool
	// Ink is row-major [y][x], length Height x Width; true = set bit.
	Ink [][]bool
	// Attr is the per-cell grid [cellY][cellX]; nil if the asset has no
	// attributes (bitmap-only or mask), or if its palette is a two-byte one.
	// In ZXPAL1 and ZXPAL2 the attribute byte is stored unchanged, with bits
	// 7-6 (Flash and Bright) read as the bank.
	Attr [][]Attribute
	// Palette is the ZXPAL palette the attributes are written for (ZXPAL0
	// for every version 1 asset).
	Palette Palette
	// Attr2 is the per-cell grid for a two-byte palette (ZXPAL3, ZXPAL4): A
	// is the ink register, B the paper register. Nil otherwise.
	Attr2 [][]CellAttr
	// Registers are the register colours the attributes are meant for; nil
	// means the palette's reset values (version 3).
	Registers Registers
}

// HasAttrs reports whether the asset carries attributes.
func (a *Asset) HasAttrs() bool { return a.Attr != nil || a.Attr2 != nil }

// Collection is an ordered set of named assets.
type Collection struct {
	Assets []Asset
}

// Find returns the first asset with the given name, or nil.
func (c *Collection) Find(name string) *Asset {
	for i := range c.Assets {
		if c.Assets[i].Name == name {
			return &c.Assets[i]
		}
	}
	return nil
}

// validate checks an asset's internal consistency before encoding.
func (a *Asset) validate() error {
	if a.Width <= 0 || a.Height <= 0 {
		return fmt.Errorf("asset %q: dimensions must be positive (got %dx%d)", a.Name, a.Width, a.Height)
	}
	if len(a.Name) > 255 {
		return fmt.Errorf("asset %q: name exceeds 255 bytes", a.Name)
	}
	for i := 0; i < len(a.Name); i++ {
		if a.Name[i] < 32 || a.Name[i] > 126 {
			return fmt.Errorf("asset %q: name must be printable ASCII", a.Name)
		}
	}
	if len(a.Ink) != a.Height {
		return fmt.Errorf("asset %q: bitmap has %d rows, want %d", a.Name, len(a.Ink), a.Height)
	}
	for y, row := range a.Ink {
		if len(row) != a.Width {
			return fmt.Errorf("asset %q: bitmap row %d has %d px, want %d", a.Name, y, len(row), a.Width)
		}
	}
	if a.IsMask && a.HasAttrs() {
		return fmt.Errorf("asset %q: a mask cannot carry attributes", a.Name)
	}
	if !a.Palette.Valid() {
		return fmt.Errorf("asset %q: unknown palette %d", a.Name, a.Palette)
	}
	if a.Attr != nil && a.Attr2 != nil {
		return fmt.Errorf("asset %q: has both one-byte and two-byte attributes", a.Name)
	}
	if a.Attr != nil && a.Palette.TwoByte() {
		return fmt.Errorf("asset %q: %s needs two-byte attributes", a.Name, a.Palette)
	}
	if a.Attr2 != nil && !a.Palette.TwoByte() {
		return fmt.Errorf("asset %q: %s has one-byte attributes", a.Name, a.Palette)
	}
	if a.Registers != nil && !a.HasAttrs() {
		return fmt.Errorf("asset %q: a register table goes with attributes", a.Name)
	}
	if err := CheckRegisters(a.Palette, a.Registers); err != nil {
		return fmt.Errorf("asset %q: %w", a.Name, err)
	}
	if a.HasAttrs() {
		if a.Width%8 != 0 || a.Height%8 != 0 {
			return fmt.Errorf("asset %q: attributed asset must be cell-aligned (got %dx%d)", a.Name, a.Width, a.Height)
		}
		cw, ch := a.Width/8, a.Height/8
		rows := len(a.Attr)
		if a.Attr2 != nil {
			rows = len(a.Attr2)
		}
		if rows != ch {
			return fmt.Errorf("asset %q: attr has %d rows, want %d", a.Name, rows, ch)
		}
		for cy := 0; cy < ch; cy++ {
			n := 0
			if a.Attr2 != nil {
				n = len(a.Attr2[cy])
			} else {
				n = len(a.Attr[cy])
			}
			if n != cw {
				return fmt.Errorf("asset %q: attr row %d has %d cells, want %d", a.Name, cy, n, cw)
			}
		}
	}
	return nil
}

func (a *Asset) bitmapBytes() int { return ((a.Width + 7) / 8) * a.Height }
func (a *Asset) attrBytes() int {
	if !a.HasAttrs() {
		return 0
	}
	if a.Attr2 != nil {
		return 2 * (a.Width / 8) * (a.Height / 8)
	}
	return (a.Width / 8) * (a.Height / 8)
}

// EncodeCollection serialises a collection to ZCUT bytes.
func EncodeCollection(c *Collection) ([]byte, error) {
	if len(c.Assets) > 255 {
		return nil, fmt.Errorf("collection has %d assets, max 255", len(c.Assets))
	}
	out := make([]byte, filePreambleLen)
	copy(out, zcutMagic)
	out[4] = zcutVersion
	for i := range c.Assets {
		if (c.Assets[i].Palette != ZXPAL0 || c.Assets[i].Attr2 != nil) && out[4] < zcutVersion2 {
			out[4] = zcutVersion2
		}
		if c.Assets[i].Registers != nil {
			out[4] = zcutVersion3
		}
	}
	out[7] = byte(len(c.Assets))

	for i := range c.Assets {
		a := &c.Assets[i]
		if err := a.validate(); err != nil {
			return nil, err
		}
		payload := encodeAssetPayload(a)
		clen := roundUp8(len(payload))

		hdr := make([]byte, chunkHeaderLen)
		copy(hdr, imagTag)
		binary.LittleEndian.PutUint32(hdr[4:], uint32(len(payload)))
		binary.LittleEndian.PutUint32(hdr[8:], uint32(clen))

		out = append(out, hdr...)
		out = append(out, payload...)
		out = append(out, make([]byte, clen-len(payload))...) // chunk pad
	}
	return out, nil
}

func encodeAssetPayload(a *Asset) []byte {
	nameLen := len(a.Name)
	namePad := roundUp8(nameLen)

	var flags byte
	if a.HasAttrs() {
		flags |= assetFlagHasAttrs
	}
	if a.IsMask {
		flags |= assetFlagIsMask
	}
	if a.Attr2 != nil {
		flags |= assetFlagTwoByte
	}
	if a.Registers != nil {
		flags |= assetFlagRegisters
	}

	p := make([]byte, fixedEntryLen+namePad)
	binary.LittleEndian.PutUint16(p[0:], uint16(a.Width))
	binary.LittleEndian.PutUint16(p[2:], uint16(a.Height))
	p[4] = flags
	p[5] = byte(a.Palette)
	p[7] = byte(nameLen)
	copy(p[fixedEntryLen:], a.Name)

	// bitmap, linear row-major
	rowBytes := (a.Width + 7) / 8
	bmp := make([]byte, rowBytes*a.Height)
	for y := 0; y < a.Height; y++ {
		for x := 0; x < a.Width; x++ {
			if a.Ink[y][x] {
				bmp[y*rowBytes+x/8] |= 1 << (7 - uint(x%8))
			}
		}
	}
	p = append(p, bmp...)

	if a.Attr2 != nil {
		cw, ch := a.Width/8, a.Height/8
		ink := make([]byte, cw*ch)
		paper := make([]byte, cw*ch)
		for cy := 0; cy < ch; cy++ {
			for cx := 0; cx < cw; cx++ {
				ink[cy*cw+cx] = a.Attr2[cy][cx].A
				paper[cy*cw+cx] = a.Attr2[cy][cx].B
			}
		}
		p = append(p, ink...)
		p = append(p, paper...)
	} else if a.HasAttrs() {
		cw, ch := a.Width/8, a.Height/8
		at := make([]byte, cw*ch)
		for cy := 0; cy < ch; cy++ {
			for cx := 0; cx < cw; cx++ {
				at[cy*cw+cx] = a.Attr[cy][cx].Byte()
			}
		}
		p = append(p, at...)
	}
	for _, c := range a.Registers {
		p = append(p, c.R, c.G, c.B)
	}
	return p
}

// DecodeCollection parses ZCUT bytes into a collection, validating structure.
func DecodeCollection(data []byte) (*Collection, error) {
	if len(data) < filePreambleLen {
		return nil, fmt.Errorf("zcut: too short for preamble")
	}
	if string(data[0:4]) != zcutMagic {
		return nil, fmt.Errorf("zcut: bad magic")
	}
	version := data[4]
	if version < zcutVersion || version > zcutVersion3 {
		return nil, fmt.Errorf("zcut: unsupported version %d", version)
	}
	n := int(data[7])

	c := &Collection{}
	off := filePreambleLen
	for i := 0; i < n; i++ {
		if off+chunkHeaderLen > len(data) {
			return nil, fmt.Errorf("zcut: asset %d header past EOF", i)
		}
		if string(data[off:off+4]) != imagTag {
			return nil, fmt.Errorf("zcut: asset %d has tag %q, want IMAG", i, string(data[off:off+4]))
		}
		payloadLen := int(binary.LittleEndian.Uint32(data[off+4:]))
		chunkLen := int(binary.LittleEndian.Uint32(data[off+8:]))
		body := off + chunkHeaderLen
		if body+chunkLen > len(data) {
			return nil, fmt.Errorf("zcut: asset %d payload past EOF", i)
		}
		if chunkLen < payloadLen || chunkLen != roundUp8(payloadLen) {
			return nil, fmt.Errorf("zcut: asset %d chunkLen %d inconsistent with payloadLen %d", i, chunkLen, payloadLen)
		}
		a, err := decodeAssetPayload(data[body:body+payloadLen], i, version)
		if err != nil {
			return nil, err
		}
		c.Assets = append(c.Assets, *a)
		off = body + chunkLen
	}
	return c, nil
}

func decodeAssetPayload(p []byte, idx int, version byte) (*Asset, error) {
	if len(p) < fixedEntryLen {
		return nil, fmt.Errorf("zcut: asset %d payload too short", idx)
	}
	w := int(binary.LittleEndian.Uint16(p[0:]))
	h := int(binary.LittleEndian.Uint16(p[2:]))
	flags := p[4]
	nameLen := int(p[7])
	hasAttrs := flags&assetFlagHasAttrs != 0
	isMask := flags&assetFlagIsMask != 0
	twoByte := flags&assetFlagTwoByte != 0
	hasRegs := flags&assetFlagRegisters != 0
	if hasRegs && (version < zcutVersion3 || !hasAttrs) {
		return nil, fmt.Errorf("zcut: asset %d: a register table needs version 3 and attributes", idx)
	}
	pal := ZXPAL0
	if version >= zcutVersion2 {
		pal = Palette(p[5])
		if !pal.Valid() {
			return nil, fmt.Errorf("zcut: asset %d has unknown palette %d", idx, p[5])
		}
		if hasAttrs && twoByte != pal.TwoByte() {
			return nil, fmt.Errorf("zcut: asset %d: two-byte flag does not match %s", idx, pal)
		}
	} else if twoByte {
		return nil, fmt.Errorf("zcut: asset %d: two-byte attributes need version 2", idx)
	}
	if twoByte && !hasAttrs {
		return nil, fmt.Errorf("zcut: asset %d: two-byte flag without attributes", idx)
	}

	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("zcut: asset %d has non-positive dimensions", idx)
	}
	if isMask && hasAttrs {
		return nil, fmt.Errorf("zcut: asset %d is both mask and attributed", idx)
	}
	if hasAttrs && (w%8 != 0 || h%8 != 0) {
		return nil, fmt.Errorf("zcut: asset %d attributed but not cell-aligned", idx)
	}

	namePad := roundUp8(nameLen)
	rowBytes := (w + 7) / 8
	bmpBytes := rowBytes * h
	attrBytes := 0
	if hasAttrs {
		attrBytes = (w / 8) * (h / 8)
		if twoByte {
			attrBytes *= 2
		}
	}
	regBytes := 0
	if hasRegs {
		regBytes = 3 * (pal.Last() + 1)
	}
	want := fixedEntryLen + namePad + bmpBytes + attrBytes + regBytes
	if len(p) != want {
		return nil, fmt.Errorf("zcut: asset %d payloadLen %d, derived %d", idx, len(p), want)
	}

	name := string(p[fixedEntryLen : fixedEntryLen+nameLen])
	for i := 0; i < len(name); i++ {
		if name[i] < 32 || name[i] > 126 {
			return nil, fmt.Errorf("zcut: asset %d name not printable ASCII", idx)
		}
	}

	a := &Asset{Name: name, Width: w, Height: h, IsMask: isMask, Palette: pal}

	bmpOff := fixedEntryLen + namePad
	a.Ink = make([][]bool, h)
	for y := 0; y < h; y++ {
		a.Ink[y] = make([]bool, w)
		for x := 0; x < w; x++ {
			b := p[bmpOff+y*rowBytes+x/8]
			a.Ink[y][x] = b&(1<<(7-uint(x%8))) != 0
		}
	}

	if twoByte {
		cw, ch := w/8, h/8
		atOff := bmpOff + bmpBytes
		a.Attr2 = make([][]CellAttr, ch)
		for cy := 0; cy < ch; cy++ {
			a.Attr2[cy] = make([]CellAttr, cw)
			for cx := 0; cx < cw; cx++ {
				a.Attr2[cy][cx] = CellAttr{A: p[atOff+cy*cw+cx], B: p[atOff+cw*ch+cy*cw+cx]}
			}
		}
	} else if hasAttrs {
		cw, ch := w/8, h/8
		atOff := bmpOff + bmpBytes
		a.Attr = make([][]Attribute, ch)
		for cy := 0; cy < ch; cy++ {
			a.Attr[cy] = make([]Attribute, cw)
			for cx := 0; cx < cw; cx++ {
				a.Attr[cy][cx] = AttributeFromByte(p[atOff+cy*cw+cx])
			}
		}
	}
	if hasRegs {
		off := bmpOff + bmpBytes + attrBytes
		a.Registers = make(Registers, pal.Last()+1)
		for i := range a.Registers {
			a.Registers[i] = color.RGBA{p[off+3*i], p[off+3*i+1], p[off+3*i+2], 0xFF}
		}
	}
	return a, nil
}
