// file: attrs.go
//
// Hardware sprite attributes: the four or five bytes per sprite sent to port
// 0x57 (or NextRegs 0x35-0x39), for anchor and relative sprites.

package zxn

import (
	"fmt"
)

// SpriteAttr is one sprite's attributes. An anchor sprite stands alone or
// leads a group; the relative sprites that follow it, up to the next anchor,
// belong to its group, composite or unified as the anchor says.
//
// The layout, from the Next's documentation:
//
//	byte 0  X, low 8 bits (relative: X offset from the anchor)
//	byte 1  Y, low 8 bits (relative: Y offset)
//	byte 2  PPPP XM YM R X8 - palette offset, X mirror, Y mirror, rotate,
//	        X bit 8 (relative: 1 adds the anchor's palette offset)
//	byte 3  V E NNNNNN - visible, byte 4 present, pattern 0-63
//	byte 4  anchor:   H N6 T XX YY Y8 - 4-bit, upper half of the pattern
//	                  slot, unified group, X and Y scale (1x, 2x, 4x, 8x),
//	                  Y bit 8
//	        relative: 0 1 N6 XX YY PR - scale (composite groups only),
//	                  1 adds the anchor's pattern number
//
// Without byte 4 (E clear) a sprite is an 8-bit anchor, unscaled, with Y
// below 256.
type SpriteAttr struct {
	X             int   `json:"x"`
	Y             int   `json:"y"`
	PaletteOffset uint8 `json:"palette_offset,omitempty"`
	MirrorX       bool  `json:"mirror_x,omitempty"`
	MirrorY       bool  `json:"mirror_y,omitempty"`
	Rotate        bool  `json:"rotate,omitempty"`
	Visible       bool  `json:"visible"`
	Pattern       uint8 `json:"pattern"`
	// Extended sends byte 4. It is needed for a relative sprite, a 4-bit
	// sprite, scaling or a Y of 256 or more, and may be set without them.
	Extended bool `json:"extended,omitempty"`
	Relative bool `json:"relative,omitempty"`
	// FourBit (anchor) makes the sprite 4-bit; its relatives follow it.
	FourBit bool `json:"four_bit,omitempty"`
	// N6 picks the upper 128 bytes of the 256-byte pattern slot for a
	// 4-bit sprite: the 4-bit pattern number is Pattern x 2 + N6.
	N6 bool `json:"n6,omitempty"`
	// Unified (anchor) makes the group's relatives turn, mirror and scale
	// with the anchor as one sprite.
	Unified bool `json:"unified,omitempty"`
	// ScaleX and ScaleY are 0-3 for 1x, 2x, 4x and 8x.
	ScaleX uint8 `json:"scale_x,omitempty"`
	ScaleY uint8 `json:"scale_y,omitempty"`
	// RelativePalette and RelativePattern (relative) add the anchor's
	// palette offset and pattern number to the sprite's own.
	RelativePalette bool `json:"relative_palette,omitempty"`
	RelativePattern bool `json:"relative_pattern,omitempty"`
}

// needsByte4 reports whether the attributes can only be expressed with
// byte 4.
func (a *SpriteAttr) needsByte4() bool {
	return a.Relative || a.FourBit || a.N6 || a.Unified || a.ScaleX != 0 || a.ScaleY != 0 || (!a.Relative && a.Y > 255)
}

// Bytes returns the sprite's four or five attribute bytes: five when
// Extended is set, four otherwise.
func (a *SpriteAttr) Bytes() ([]byte, error) {
	if a.Relative {
		if a.X < -128 || a.X > 255 || a.Y < -128 || a.Y > 255 {
			return nil, fmt.Errorf("zxn: a relative sprite's offset (%d, %d) is 8 bits", a.X, a.Y)
		}
		if a.FourBit || a.Unified {
			return nil, fmt.Errorf("zxn: 4-bit and unified are set on the anchor, not on a relative sprite")
		}
	} else {
		if a.X < 0 || a.X > 511 || a.Y < 0 || a.Y > 511 {
			return nil, fmt.Errorf("zxn: an anchor's position (%d, %d) is 9 bits", a.X, a.Y)
		}
		if a.RelativePalette || a.RelativePattern {
			return nil, fmt.Errorf("zxn: relative palette and pattern are for relative sprites")
		}
	}
	if a.PaletteOffset > 15 || a.Pattern > 63 || a.ScaleX > 3 || a.ScaleY > 3 {
		return nil, fmt.Errorf("zxn: palette offset %d (0-15), pattern %d (0-63) or scale %d,%d (0-3) out of range", a.PaletteOffset, a.Pattern, a.ScaleX, a.ScaleY)
	}
	if !a.Extended && a.needsByte4() {
		return nil, fmt.Errorf("zxn: these attributes need byte 4; set extended")
	}
	out := []byte{uint8(a.X), uint8(a.Y), a.PaletteOffset << 4, a.Pattern & 0x3F}
	if a.MirrorX {
		out[2] |= 0x08
	}
	if a.MirrorY {
		out[2] |= 0x04
	}
	if a.Rotate {
		out[2] |= 0x02
	}
	if a.Relative && a.RelativePalette || !a.Relative && a.X > 255 {
		out[2] |= 0x01
	}
	if a.Visible {
		out[3] |= 0x80
	}
	if !a.Extended {
		return out, nil
	}
	out[3] |= 0x40
	var b4 uint8
	if a.Relative {
		b4 = 0x40 | a.ScaleX<<3 | a.ScaleY<<1
		if a.N6 {
			b4 |= 0x20
		}
		if a.RelativePattern {
			b4 |= 0x01
		}
	} else {
		b4 = a.ScaleX<<3 | a.ScaleY<<1
		if a.FourBit {
			b4 |= 0x80
		}
		if a.N6 {
			b4 |= 0x40
		}
		if a.Unified {
			b4 |= 0x20
		}
		if a.Y > 255 {
			b4 |= 0x01
		}
	}
	return append(out, b4), nil
}

// ParseSpriteAttr reads one sprite's attributes from the start of data,
// four bytes or five as byte 3's E bit says, and returns them with the
// number of bytes used. A byte 4 whose top bits are 01 is a relative
// sprite; a relative sprite's X and Y are its offsets, read as signed.
func ParseSpriteAttr(data []byte) (SpriteAttr, int, error) {
	if len(data) < 4 {
		return SpriteAttr{}, 0, fmt.Errorf("zxn: sprite attributes are 4 or 5 bytes; %d left", len(data))
	}
	a := SpriteAttr{
		X: int(data[0]), Y: int(data[1]),
		PaletteOffset: data[2] >> 4,
		MirrorX:       data[2]&0x08 != 0,
		MirrorY:       data[2]&0x04 != 0,
		Rotate:        data[2]&0x02 != 0,
		Visible:       data[3]&0x80 != 0,
		Extended:      data[3]&0x40 != 0,
		Pattern:       data[3] & 0x3F,
	}
	n := 4
	var b4 uint8
	if a.Extended {
		if len(data) < 5 {
			return SpriteAttr{}, 0, fmt.Errorf("zxn: sprite attributes with E set are 5 bytes; %d left", len(data))
		}
		b4 = data[4]
		n = 5
	}
	if b4&0xC0 == 0x40 {
		a.Relative = true
		a.X, a.Y = int(int8(data[0])), int(int8(data[1]))
		a.RelativePalette = data[2]&0x01 != 0
		a.N6 = b4&0x20 != 0
		a.ScaleX, a.ScaleY = b4>>3&3, b4>>1&3
		a.RelativePattern = b4&0x01 != 0
		return a, n, nil
	}
	if data[2]&0x01 != 0 {
		a.X += 256
	}
	a.FourBit = b4&0x80 != 0
	a.N6 = b4&0x40 != 0
	a.Unified = b4&0x20 != 0
	a.ScaleX, a.ScaleY = b4>>3&3, b4>>1&3
	if b4&0x01 != 0 {
		a.Y += 256
	}
	return a, n, nil
}

// EncodeAttrs returns a table of sprite attributes as port 0x57 takes them,
// four or five bytes each by its Extended flag; with fixed5 every sprite is
// five bytes (Extended is then set on all of them, which changes nothing
// for a sprite whose byte 4 is 0), as tables written for NextReg uploads
// usually are.
func EncodeAttrs(attrs []SpriteAttr, fixed5 bool) ([]byte, error) {
	if len(attrs) > 128 {
		return nil, fmt.Errorf("zxn: %d sprites; the Next has 128", len(attrs))
	}
	var out []byte
	for i := range attrs {
		a := attrs[i]
		if fixed5 {
			a.Extended = true
		}
		b, err := a.Bytes()
		if err != nil {
			return nil, fmt.Errorf("sprite %d: %w", i, err)
		}
		out = append(out, b...)
	}
	return out, nil
}

// DecodeAttrs reads a table of attributes, each four or five bytes as its E
// bit says, or always five with fixed5. Uploaded through port 0x57, a
// table of five-byte entries keeps in step only if every E bit is set.
func DecodeAttrs(data []byte, fixed5 bool) ([]SpriteAttr, error) {
	var out []SpriteAttr
	for off := 0; off < len(data); {
		if fixed5 && len(data)-off < 5 {
			return nil, fmt.Errorf("zxn: %d bytes is not a whole number of 5-byte sprites", len(data))
		}
		a, n, err := ParseSpriteAttr(data[off:])
		if err != nil {
			return nil, fmt.Errorf("sprite %d: %w", len(out), err)
		}
		if fixed5 {
			n = 5 // with E clear the hardware ignores byte 4, and so does this
		}
		out = append(out, a)
		off += n
	}
	if len(out) > 128 {
		return nil, fmt.Errorf("zxn: %d sprites; the Next has 128", len(out))
	}
	return out, nil
}
