// file: tiles.go
//
// Tiles (.nxt), tilemaps (.nxm) and blocks of tiles (.nxb), cut from an image
// as gfx2next cuts them, with optional removal of repeated, mirrored and
// rotated tiles.

package zxn

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"strings"
)

// Tilemap attribute bits, in the high byte of a 16-bit map entry: bits 7-4
// the palette offset, then X mirror, Y mirror and rotate; bit 0 is bit 8 of
// a tile index in 512-tile mode.
const (
	AttrMirrorX = 0x08
	AttrMirrorY = 0x04
	AttrRotate  = 0x02
)

// Dedup selects which tiles count as repeats of an earlier tile.
type Dedup int

const (
	// DedupNone keeps every tile, one per map cell.
	DedupNone Dedup = iota
	// DedupRepeat drops tiles identical to an earlier one
	// (gfx2next's -tile-norepeat).
	DedupRepeat
	// DedupMirror also drops tiles that are an earlier one mirrored in X, Y
	// or both, recording the mirroring in the map's attributes
	// (-tile-nomirror).
	DedupMirror
	// DedupRotate also allows a quarter turn (-tile-norotate). Tiles must be
	// square.
	DedupRotate
)

// ParseDedup accepts "none", "repeat", "mirror" and "rotate".
func ParseDedup(s string) (Dedup, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none":
		return DedupNone, nil
	case "repeat":
		return DedupRepeat, nil
	case "mirror":
		return DedupMirror, nil
	case "rotate":
		return DedupRotate, nil
	}
	return DedupNone, fmt.Errorf("unknown tile matching %q (want none, repeat, mirror or rotate)", s)
}

// TileOptions control CutTiles.
type TileOptions struct {
	Width, Height int // tile size in pixels; 0 means 8x8
	Depth         int // 8, 4 or 1 bits per pixel
	Dedup         Dedup
	// Offset is added to every tile index in the map (gfx2next's
	// -tile-offset); it does not apply to blocks.
	Offset int
	// PaletteOffset, 0-15, is ORed into every map entry's palette offset
	// bits (-tile-pal).
	PaletteOffset int
	// BlockWidth and BlockHeight, in tiles, group tiles into blocks; 0 or
	// 1 by 1 means no blocks.
	BlockWidth, BlockHeight int
	// BlockDedup drops blocks identical to an earlier one (-block-norepeat).
	BlockDedup bool
}

// TileSet holds tiles of one size, unpacked to an index per pixel.
type TileSet struct {
	Width, Height int
	Depth         int
	Tiles         [][]uint8 // each Width x Height indices, row by row
}

// TileMap is a grid of map entries: a tile (or block) index in the low bits
// and, for tiles, the attribute byte in bits 15-8.
type TileMap struct {
	Width, Height int      // in cells
	Entries       []uint16 // row by row
}

// BlockSet holds blocks of tiles, each a grid of tile indices.
type BlockSet struct {
	Width, Height int // in tiles
	Blocks        [][]uint16
}

// CutTiles cuts an indexed image into tiles, row of tiles by row of tiles,
// left to right, and returns the tiles, the map and, when blocks were asked
// for, the blocks (with the map then holding block indices). At depth 4 a
// tile stores its pixels' low four bits and its map entry's palette offset
// is the high four bits of its last pixel's index, as gfx2next does, so an
// image drawn with a 256-colour palette in banks of 16 keeps its banks; at
// depth 1 any index but 0 is a set pixel.
func CutTiles(img *image.Paletted, opt TileOptions) (*TileSet, *TileMap, *BlockSet, error) {
	tw, th := opt.Width, opt.Height
	if tw == 0 && th == 0 {
		tw, th = 8, 8
	}
	if tw <= 0 || th <= 0 {
		return nil, nil, nil, fmt.Errorf("zxn: tile size %dx%d", tw, th)
	}
	if opt.Depth != 8 && opt.Depth != 4 && opt.Depth != 1 {
		return nil, nil, nil, fmt.Errorf("zxn: tile depth %d, want 8, 4 or 1", opt.Depth)
	}
	if tw*th*opt.Depth%8 != 0 {
		return nil, nil, nil, fmt.Errorf("zxn: a %dx%d tile at %d bits is not a whole number of bytes", tw, th, opt.Depth)
	}
	if opt.Dedup == DedupRotate && tw != th {
		return nil, nil, nil, fmt.Errorf("zxn: rotated tiles must be square; got %dx%d", tw, th)
	}
	if opt.Dedup >= DedupMirror && opt.Depth == 1 {
		return nil, nil, nil, fmt.Errorf("zxn: mirrored and rotated tile matching is for 8- and 4-bit tiles")
	}
	if opt.PaletteOffset < 0 || opt.PaletteOffset > 15 {
		return nil, nil, nil, fmt.Errorf("zxn: palette offset %d, want 0 to 15", opt.PaletteOffset)
	}
	bw, bh := opt.BlockWidth, opt.BlockHeight
	if bw == 0 && bh == 0 {
		bw, bh = 1, 1
	}
	if bw <= 0 || bh <= 0 {
		return nil, nil, nil, fmt.Errorf("zxn: block size %dx%d", bw, bh)
	}
	blocks := bw > 1 || bh > 1
	if blocks && opt.Dedup >= DedupMirror {
		return nil, nil, nil, fmt.Errorf("zxn: blocks hold plain tile indices, so they cannot use mirrored or rotated tiles; use repeat")
	}
	b := img.Bounds()
	cw, ch := tw*bw, th*bh
	if b.Dx()%cw != 0 || b.Dy()%ch != 0 || b.Dx() == 0 || b.Dy() == 0 {
		return nil, nil, nil, fmt.Errorf("zxn: the image is %dx%d, not a whole number of %dx%d cells", b.Dx(), b.Dy(), cw, ch)
	}

	ts := &TileSet{Width: tw, Height: th, Depth: opt.Depth}
	// tile cuts the tile at (px, py) and returns its index and attributes.
	tile := func(px, py int) (int, uint8) {
		t := make([]uint8, tw*th)
		var attr uint8
		for y := 0; y < th; y++ {
			for x := 0; x < tw; x++ {
				v := img.ColorIndexAt(b.Min.X+px+x, b.Min.Y+py+y)
				switch opt.Depth {
				case 4:
					attr = v & 0xF0
					v &= 0x0F
				case 1:
					if v != 0 {
						v = 1
					}
				}
				t[y*tw+x] = v
			}
		}
		idx := len(ts.Tiles)
		if opt.Dedup != DedupNone {
			for i, old := range ts.Tiles {
				var m int
				if opt.Dedup == DedupRepeat {
					if equalTiles(old, t) {
						m = matchXY
					}
				} else {
					m = matchTransformed(old, t, tw, th, opt.Dedup == DedupRotate)
				}
				if m != 0 {
					idx = i
					attr |= uint8(m & 0x0E)
					break
				}
			}
		}
		if idx == len(ts.Tiles) {
			ts.Tiles = append(ts.Tiles, t)
		}
		return idx, attr | uint8(opt.PaletteOffset<<4)
	}

	tm := &TileMap{Width: b.Dx() / cw, Height: b.Dy() / ch}
	var bs *BlockSet
	if blocks {
		bs = &BlockSet{Width: bw, Height: bh}
	}
	for my := 0; my < tm.Height; my++ {
		for mx := 0; mx < tm.Width; mx++ {
			if !blocks {
				idx, attr := tile(mx*tw, my*th)
				tm.Entries = append(tm.Entries, uint16(idx+opt.Offset)|uint16(attr)<<8)
				continue
			}
			blk := make([]uint16, 0, bw*bh)
			for y := 0; y < bh; y++ {
				for x := 0; x < bw; x++ {
					idx, _ := tile(mx*cw+x*tw, my*ch+y*th)
					blk = append(blk, uint16(idx))
				}
			}
			bi := len(bs.Blocks)
			if opt.BlockDedup {
				for i, old := range bs.Blocks {
					if equalBlocks(old, blk) {
						bi = i
						break
					}
				}
			}
			if bi == len(bs.Blocks) {
				bs.Blocks = append(bs.Blocks, blk)
			}
			tm.Entries = append(tm.Entries, uint16(bi))
		}
	}
	if !blocks && len(ts.Tiles)-1+opt.Offset > 0x1FF {
		return nil, nil, nil, fmt.Errorf("zxn: %d tiles from offset %d pass index 511, the most a map can hold", len(ts.Tiles), opt.Offset)
	}
	return ts, tm, bs, nil
}

const (
	matchXY       = 1 << 0
	matchRotate   = 1 << 1
	matchMirrorY  = 1 << 2
	matchMirrorX  = 1 << 3
	matchMirrorXY = 1 << 4
	matchAny      = matchXY | matchMirrorY | matchMirrorX | matchMirrorXY
)

func equalTiles(a, b []uint8) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalBlocks(a, b []uint16) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// matchTransformed is gfx2next's check_tile_rotate: the ways the new tile n
// is the earlier tile t as stored, mirrored or (with rotate) turned. Every
// way that fits is reported, so a symmetric tile that matches exactly also
// carries the mirror bits it matches under, as gfx2next writes them; a
// rotated match is used only when no unrotated one fits.
func matchTransformed(t, n []uint8, w, h int, rotate bool) int {
	m := matchAny
	r := matchAny | matchRotate
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			xr, yr := w-x-1, h-y-1
			p, pxr, pyr, pxyr := t[y*w+x], t[y*w+xr], t[yr*w+x], t[yr*w+xr]
			o := n[y*w+x]
			or := n[x*h+yr] // read with the tile's width; tiles are square here
			if p != o {
				m &^= matchXY
			}
			if pyr != o {
				m &^= matchMirrorY
			}
			if pxr != o {
				m &^= matchMirrorX
			}
			if pxyr != o {
				m &^= matchMirrorXY
			}
			if rotate {
				if p != or {
					r &^= matchXY
				}
				if pyr != or {
					r &^= matchMirrorX
				}
				if pxr != or {
					r &^= matchMirrorY
				}
				if pxyr != or {
					r &^= matchMirrorXY
				}
			}
		}
	}
	if m&matchAny == 0 && rotate && r&matchAny != 0 {
		m = r
	}
	if m&matchMirrorXY != 0 {
		m = m&^matchMirrorXY | matchMirrorX | matchMirrorY
	}
	if m&matchAny == 0 {
		return 0
	}
	return m
}

// EncodeNXT returns the tiles packed at their depth, one after another: at 4
// bits the left pixel of each pair in the high nibble, at 1 bit the leftmost
// pixel in bit 7.
func EncodeNXT(ts *TileSet) ([]byte, error) {
	var out []byte
	for i, t := range ts.Tiles {
		if len(t) != ts.Width*ts.Height {
			return nil, fmt.Errorf("zxn: tile %d holds %d pixels for %dx%d", i, len(t), ts.Width, ts.Height)
		}
		switch ts.Depth {
		case 8:
			out = append(out, t...)
		case 4:
			p, err := packNibbles(t)
			if err != nil {
				return nil, fmt.Errorf("zxn: tile %d: %w", i, err)
			}
			out = append(out, p...)
		case 1:
			for j := 0; j < len(t); j += 8 {
				var v uint8
				for k := 0; k < 8; k++ {
					v <<= 1
					if t[j+k] != 0 {
						v |= 1
					}
				}
				out = append(out, v)
			}
		default:
			return nil, fmt.Errorf("zxn: tile depth %d, want 8, 4 or 1", ts.Depth)
		}
	}
	return out, nil
}

// DecodeNXT reads tiles of a known size and depth.
func DecodeNXT(data []byte, w, h, depth int) (*TileSet, error) {
	if depth != 8 && depth != 4 && depth != 1 {
		return nil, fmt.Errorf("zxn: tile depth %d, want 8, 4 or 1", depth)
	}
	if w <= 0 || h <= 0 || w*h*depth%8 != 0 {
		return nil, fmt.Errorf("zxn: a %dx%d tile at %d bits is not a whole number of bytes", w, h, depth)
	}
	n := w * h * depth / 8
	if len(data)%n != 0 {
		return nil, fmt.Errorf("zxn: %d bytes is not a whole number of %d-byte tiles", len(data), n)
	}
	ts := &TileSet{Width: w, Height: h, Depth: depth}
	for off := 0; off < len(data); off += n {
		c := data[off : off+n]
		var t []uint8
		switch depth {
		case 8:
			t = append([]uint8(nil), c...)
		case 4:
			t = unpackNibbles(c)
		case 1:
			for _, v := range c {
				for k := 7; k >= 0; k-- {
					t = append(t, v>>uint(k)&1)
				}
			}
		}
		ts.Tiles = append(ts.Tiles, t)
	}
	return ts, nil
}

// EncodeNXM returns the map at one or two bytes per entry (two little-endian
// bytes, attributes in the second), row by row or, with columns, column by
// column (gfx2next's -map-y). An 8-bit map holds indices up to 255 and no
// attributes; a 16-bit map holds indices up to 511.
func EncodeNXM(m *TileMap, sixteen, columns bool) ([]byte, error) {
	if len(m.Entries) != m.Width*m.Height {
		return nil, fmt.Errorf("zxn: map holds %d entries for %dx%d", len(m.Entries), m.Width, m.Height)
	}
	var out []byte
	put := func(e uint16) error {
		if sixteen {
			out = binary.LittleEndian.AppendUint16(out, e)
			return nil
		}
		if e > 0xFF {
			return fmt.Errorf("zxn: map entry %#04x needs a 16-bit map (it has a tile index above 255 or tile attributes)", e)
		}
		out = append(out, uint8(e))
		return nil
	}
	if columns {
		for x := 0; x < m.Width; x++ {
			for y := 0; y < m.Height; y++ {
				if err := put(m.Entries[y*m.Width+x]); err != nil {
					return nil, err
				}
			}
		}
		return out, nil
	}
	for _, e := range m.Entries {
		if err := put(e); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// DecodeNXM reads a map of known size, entry size and order.
func DecodeNXM(data []byte, w, h int, sixteen, columns bool) (*TileMap, error) {
	n := 1
	if sixteen {
		n = 2
	}
	if w <= 0 || h <= 0 || len(data) != w*h*n {
		return nil, fmt.Errorf("zxn: a %dx%d map of %d-byte entries is %d bytes; got %d", w, h, n, w*h*n, len(data))
	}
	m := &TileMap{Width: w, Height: h, Entries: make([]uint16, w*h)}
	for i := 0; i < w*h; i++ {
		var e uint16
		if sixteen {
			e = binary.LittleEndian.Uint16(data[2*i:])
		} else {
			e = uint16(data[i])
		}
		x, y := i%w, i/w
		if columns {
			x, y = i/h, i%h
		}
		m.Entries[y*w+x] = e
	}
	return m, nil
}

// EncodeNXB returns the blocks' tile indices, block by block, row by row
// within a block, at one byte each or two little-endian bytes
// (-block-16bit).
func EncodeNXB(bs *BlockSet, sixteen bool) ([]byte, error) {
	var out []byte
	for i, blk := range bs.Blocks {
		if len(blk) != bs.Width*bs.Height {
			return nil, fmt.Errorf("zxn: block %d holds %d tiles for %dx%d", i, len(blk), bs.Width, bs.Height)
		}
		for _, v := range blk {
			if sixteen {
				out = binary.LittleEndian.AppendUint16(out, v)
			} else if v > 0xFF {
				return nil, fmt.Errorf("zxn: tile index %d in block %d does not fit a byte; use 16-bit blocks", v, i)
			} else {
				out = append(out, uint8(v))
			}
		}
	}
	return out, nil
}

// TileAt returns the tile pixel shown at (x, y) of a map cell whose stored
// tile is t and whose attributes are attr: the tile is turned a quarter
// clockwise if the rotate bit is set, then mirrored in X, then in Y.
func TileAt(t []uint8, w, h int, attr uint8, x, y int) uint8 {
	if attr&AttrMirrorY != 0 {
		y = h - 1 - y
	}
	if attr&AttrMirrorX != 0 {
		x = w - 1 - x
	}
	if attr&AttrRotate != 0 {
		x, y = y, h-1-x
	}
	return t[y*w+x]
}

// MapImage draws a tilemap (not one of blocks) in pal's colours, each cell's
// attributes applied; at depth 4 a cell's palette offset selects its bank of
// 16. Offset is subtracted from each index first (the map's -tile-offset);
// a cell whose tile is missing is drawn black.
func MapImage(ts *TileSet, m *TileMap, pal *Palette, offset int) *image.RGBA {
	w, h := ts.Width, ts.Height
	img := image.NewRGBA(image.Rect(0, 0, m.Width*w, m.Height*h))
	for cy := 0; cy < m.Height; cy++ {
		for cx := 0; cx < m.Width; cx++ {
			e := m.Entries[cy*m.Width+cx]
			attr := uint8(e >> 8)
			idx := int(e&0x1FF) - offset
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					c := color.RGBA{A: 0xFF}
					if idx >= 0 && idx < len(ts.Tiles) {
						v := int(TileAt(ts.Tiles[idx], w, h, attr, x, y))
						if ts.Depth == 4 {
							v |= int(attr & 0xF0)
						}
						if pal != nil && v < len(pal.Colors) {
							c = pal.Colors[v].RGBA()
						}
					}
					img.SetRGBA(cx*w+x, cy*h+y, c)
				}
			}
		}
	}
	return img
}

// TilesImage draws a tile set perRow tiles wide.
func TilesImage(ts *TileSet, pal *Palette, perRow int) *image.RGBA {
	if perRow < 1 {
		perRow = 1
	}
	n := len(ts.Tiles)
	cols := perRow
	if n < cols {
		cols = n
	}
	if cols == 0 {
		cols = 1
	}
	rows := (n + perRow - 1) / perRow
	if rows == 0 {
		rows = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, cols*ts.Width, rows*ts.Height))
	for i, t := range ts.Tiles {
		ox, oy := (i%perRow)*ts.Width, (i/perRow)*ts.Height
		for y := 0; y < ts.Height; y++ {
			for x := 0; x < ts.Width; x++ {
				c := color.RGBA{A: 0xFF}
				if v := int(t[y*ts.Width+x]); pal != nil && v < len(pal.Colors) {
					c = pal.Colors[v].RGBA()
				}
				img.SetRGBA(ox+x, oy+y, c)
			}
		}
	}
	return img
}
