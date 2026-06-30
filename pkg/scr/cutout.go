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
// Layout (all multi-byte fields little-endian):
//
//	file preamble (8 bytes)
//	  +0  "ZCUT"      magic
//	  +4  version u8  = 1
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
//	    +4  flags    u8   bit0 = has attributes, bit1 = is mask
//	    +5  reserved [2]  (must be 0)
//	    +7  nameLen  u8
//	    +8  name     [roundUp8(nameLen)]  ASCII, zero-padded
//	    ..  bitmap   [ceil(w/8)*h]        linear, row-major
//	    ..  attrs    [cellsW*cellsH]      only if flags bit0
package scr

import (
	"encoding/binary"
	"fmt"
)

const (
	zcutMagic   = "ZCUT"
	imagTag     = "IMAG"
	zcutVersion = 1

	assetFlagHasAttrs = 0x01
	assetFlagIsMask   = 0x02

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
	// attributes (bitmap-only or mask).
	Attr [][]Attribute
}

// HasAttrs reports whether the asset carries attributes.
func (a *Asset) HasAttrs() bool { return a.Attr != nil }

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
	if a.HasAttrs() {
		if a.Width%8 != 0 || a.Height%8 != 0 {
			return fmt.Errorf("asset %q: attributed asset must be cell-aligned (got %dx%d)", a.Name, a.Width, a.Height)
		}
		cw, ch := a.Width/8, a.Height/8
		if len(a.Attr) != ch {
			return fmt.Errorf("asset %q: attr has %d rows, want %d", a.Name, len(a.Attr), ch)
		}
		for cy, row := range a.Attr {
			if len(row) != cw {
				return fmt.Errorf("asset %q: attr row %d has %d cells, want %d", a.Name, cy, len(row), cw)
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

	p := make([]byte, fixedEntryLen+namePad)
	binary.LittleEndian.PutUint16(p[0:], uint16(a.Width))
	binary.LittleEndian.PutUint16(p[2:], uint16(a.Height))
	p[4] = flags
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

	if a.HasAttrs() {
		cw, ch := a.Width/8, a.Height/8
		at := make([]byte, cw*ch)
		for cy := 0; cy < ch; cy++ {
			for cx := 0; cx < cw; cx++ {
				at[cy*cw+cx] = a.Attr[cy][cx].Byte()
			}
		}
		p = append(p, at...)
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
	if data[4] != zcutVersion {
		return nil, fmt.Errorf("zcut: unsupported version %d", data[4])
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
		a, err := decodeAssetPayload(data[body:body+payloadLen], i)
		if err != nil {
			return nil, err
		}
		c.Assets = append(c.Assets, *a)
		off = body + chunkLen
	}
	return c, nil
}

func decodeAssetPayload(p []byte, idx int) (*Asset, error) {
	if len(p) < fixedEntryLen {
		return nil, fmt.Errorf("zcut: asset %d payload too short", idx)
	}
	w := int(binary.LittleEndian.Uint16(p[0:]))
	h := int(binary.LittleEndian.Uint16(p[2:]))
	flags := p[4]
	nameLen := int(p[7])
	hasAttrs := flags&assetFlagHasAttrs != 0
	isMask := flags&assetFlagIsMask != 0

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
	}
	want := fixedEntryLen + namePad + bmpBytes + attrBytes
	if len(p) != want {
		return nil, fmt.Errorf("zcut: asset %d payloadLen %d, derived %d", idx, len(p), want)
	}

	name := string(p[fixedEntryLen : fixedEntryLen+nameLen])
	for i := 0; i < len(name); i++ {
		if name[i] < 32 || name[i] > 126 {
			return nil, fmt.Errorf("zcut: asset %d name not printable ASCII", idx)
		}
	}

	a := &Asset{Name: name, Width: w, Height: h, IsMask: isMask}

	bmpOff := fixedEntryLen + namePad
	a.Ink = make([][]bool, h)
	for y := 0; y < h; y++ {
		a.Ink[y] = make([]bool, w)
		for x := 0; x < w; x++ {
			b := p[bmpOff+y*rowBytes+x/8]
			a.Ink[y][x] = b&(1<<(7-uint(x%8))) != 0
		}
	}

	if hasAttrs {
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
	return a, nil
}
