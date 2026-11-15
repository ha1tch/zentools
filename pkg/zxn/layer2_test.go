package zxn

import (
	"bytes"
	"image"
	"testing"
)

// TestLayer2AgainstTools checks the three Layer 2 modes against gfx2next and
// nextraw, which agree byte for byte on these files.
func TestLayer2AgainstTools(t *testing.T) {
	for _, c := range []struct {
		png, nxi, nxp string
		mode          string
		embed         bool
	}{
		{"l2_256.png", "l2_256_embed.nxi", "", "256x192", true},
		{"l2_320.png", "l2_320.nxi", "l2_320.nxp", "320x256", false},
		{"l2_640.png", "l2_640.nxi", "l2_640.nxp", "640x256", false},
	} {
		m, err := ParseLayer2Mode(c.mode)
		if err != nil {
			t.Fatal(err)
		}
		img := loadPNG(t, "testdata/"+c.png)
		b, err := BitmapFromImage(img, nil, m.Depth, m.Order, RoundNearest)
		if err != nil {
			t.Fatal(err)
		}
		got, err := EncodeNXI(b)
		if err != nil {
			t.Fatal(err)
		}
		size := 256
		if m.Depth == 4 {
			size = 16
		}
		p, err := PaletteFromImage(img, MaxColors, RoundNearest)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Colors) > size {
			p.Colors = p.Colors[:size]
		}
		pal, _ := EncodeNXP(p.Padded(size))
		if c.embed {
			got = append(pal, got...)
		} else if want := mustRead(t, "testdata/"+c.nxp); !bytes.Equal(pal, want) {
			t.Errorf("%s differs", c.nxp)
		}
		want := mustRead(t, "testdata/"+c.nxi)
		if !bytes.Equal(got, want) {
			t.Errorf("%s: %d bytes differ from the tools' %d", c.nxi, len(got), len(want))
			continue
		}
		// Decoding gives back the image's indices.
		fm, palBytes, ok := Layer2ModeOfSize(len(want))
		if !ok || palBytes != len(want)-m.Bytes() || (m.Name != "640x256" && fm.Name != m.Name) {
			t.Errorf("%s: size %d detected as %v with %d palette bytes", c.nxi, len(want), fm, palBytes)
		}
		back, err := DecodeNXI(want[palBytes:], m.Width, m.Height, m.Depth, m.Order)
		if err != nil {
			t.Fatal(err)
		}
		pi := img.(*image.Paletted)
		for y := 0; y < m.Height; y++ {
			for x := 0; x < m.Width; x++ {
				v := pi.ColorIndexAt(x, y)
				if m.Depth == 4 {
					v &= 15
				}
				if back.At(x, y) != v {
					t.Fatalf("%s: pixel (%d, %d) decodes as %d, want %d", c.nxi, x, y, back.At(x, y), v)
				}
			}
		}
	}
}

func TestLayer2RowMajor4Bit(t *testing.T) {
	b := &Bitmap{Width: 4, Height: 2, Depth: 4, Order: RowMajor, Indices: []uint8{1, 2, 3, 4, 5, 6, 7, 8}}
	data, err := EncodeNXI(b)
	if err != nil || !bytes.Equal(data, []byte{0x12, 0x34, 0x56, 0x78}) {
		t.Fatalf("got % X, %v", data, err)
	}
	b.Order = ColumnMajor
	data, _ = EncodeNXI(b)
	if !bytes.Equal(data, []byte{0x12, 0x56, 0x34, 0x78}) {
		t.Fatalf("column order: got % X", data)
	}
	back, err := DecodeNXI(data, 4, 2, 4, ColumnMajor)
	if err != nil || !bytes.Equal(back.Indices, b.Indices) {
		t.Fatalf("round trip: %v %v", back, err)
	}
	if _, err := EncodeNXI(&Bitmap{Width: 3, Height: 1, Depth: 4, Indices: []uint8{1, 2, 3}}); err == nil {
		t.Error("odd 4-bit width accepted")
	}
	if _, err := EncodeNXI(&Bitmap{Width: 2, Height: 1, Depth: 4, Indices: []uint8{16, 2}}); err == nil {
		t.Error("index 16 accepted at depth 4")
	}
	if _, err := DecodeNXI(make([]byte, 5), 4, 2, 4, RowMajor); err == nil {
		t.Error("short data accepted")
	}
}
