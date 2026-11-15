package zxn

import "fmt"

// SpriteSize is the width and height of a Next hardware sprite pattern.
const SpriteSize = 16

// PixelOrder is the order a bitmap's pixels are stored in.
type PixelOrder int

const (
	// RowMajor stores pixels row by row, left to right within a row.
	RowMajor PixelOrder = iota
	// ColumnMajor stores pixels column by column, top to bottom within a
	// column, as the Next's 320x256 and 640x256 Layer 2 modes do.
	ColumnMajor
)

// Bitmap is a grid of palette indices, one per pixel, unpacked whatever the
// stored depth. Indices holds Width x Height entries in row-major order.
type Bitmap struct {
	Width, Height int
	Depth         int // bits per pixel when stored: 8 or 4
	Order         PixelOrder
	Indices       []uint8
	Palette       *Palette // nil when the palette is kept separately
}

// At returns the index at (x, y).
func (b *Bitmap) At(x, y int) uint8 { return b.Indices[y*b.Width+x] }

// SpriteSheet is a sequence of 16x16 sprite patterns, matching the .spr file
// layout: no header and no attributes, the patterns one after another.
type SpriteSheet struct {
	Depth   int // 8 (256 bytes per pattern) or 4 (128 bytes, two pixels per byte)
	Sprites []Bitmap
}

// patternBytes is the size of one stored pattern at a depth.
func patternBytes(depth int) int { return SpriteSize * SpriteSize * depth / 8 }

func checkDepth(depth int) error {
	if depth != 8 && depth != 4 {
		return fmt.Errorf("zxn: sprite depth %d, want 8 or 4", depth)
	}
	return nil
}

// EncodeSPR returns the sheet in the .spr layout. At depth 4 every index must
// be below 16; the left pixel of each pair is the high nibble.
func EncodeSPR(s *SpriteSheet) ([]byte, error) {
	if err := checkDepth(s.Depth); err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(s.Sprites)*patternBytes(s.Depth))
	for i := range s.Sprites {
		sp := &s.Sprites[i]
		if sp.Width != SpriteSize || sp.Height != SpriteSize || len(sp.Indices) != SpriteSize*SpriteSize {
			return nil, fmt.Errorf("zxn: sprite %d is %dx%d, want %dx%d", i, sp.Width, sp.Height, SpriteSize, SpriteSize)
		}
		if s.Depth == 8 {
			out = append(out, sp.Indices...)
			continue
		}
		packed, err := packNibbles(sp.Indices)
		if err != nil {
			return nil, fmt.Errorf("zxn: sprite %d: %w", i, err)
		}
		out = append(out, packed...)
	}
	return out, nil
}

// DecodeSPR parses a .spr file of patterns at the given depth.
func DecodeSPR(data []byte, depth int) (*SpriteSheet, error) {
	if err := checkDepth(depth); err != nil {
		return nil, err
	}
	n := patternBytes(depth)
	if len(data)%n != 0 {
		return nil, fmt.Errorf("zxn: %d bytes is not a whole number of %d-bit patterns (%d bytes each)", len(data), depth, n)
	}
	s := &SpriteSheet{Depth: depth}
	for off := 0; off < len(data); off += n {
		chunk := data[off : off+n]
		sp := Bitmap{Width: SpriteSize, Height: SpriteSize, Depth: depth}
		if depth == 8 {
			sp.Indices = append([]uint8(nil), chunk...)
		} else {
			sp.Indices = unpackNibbles(chunk)
		}
		s.Sprites = append(s.Sprites, sp)
	}
	return s, nil
}

// packNibbles packs indices two per byte, the first in the high nibble.
func packNibbles(idx []uint8) ([]byte, error) {
	out := make([]byte, (len(idx)+1)/2)
	for i, v := range idx {
		if v > 15 {
			return nil, fmt.Errorf("index %d at pixel %d does not fit in 4 bits", v, i)
		}
		if i%2 == 0 {
			out[i/2] = v << 4
		} else {
			out[i/2] |= v
		}
	}
	return out, nil
}

// unpackNibbles is the reverse of packNibbles.
func unpackNibbles(b []byte) []uint8 {
	out := make([]uint8, 0, 2*len(b))
	for _, v := range b {
		out = append(out, v>>4, v&0x0F)
	}
	return out
}
