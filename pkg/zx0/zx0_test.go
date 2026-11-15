package zx0

import (
	"bytes"
	"math/rand"
	"os"
	"testing"
)

// TestAgainstReference compresses the inputs in testdata as the reference
// zx0 did (v2, -c, -b and -q) and requires the same bytes, then decompresses
// the reference output.
func TestAgainstReference(t *testing.T) {
	for _, name := range []string{"a", "b", "c"} {
		in, err := os.ReadFile("testdata/" + name + ".bin")
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range []struct {
			ext string
			opt Options
		}{
			{"v2", Options{}},
			{"v1", Options{Classic: true}},
			{"back", Options{Backwards: true}},
			{"quick", Options{Quick: true}},
		} {
			want, err := os.ReadFile("testdata/" + name + "." + v.ext)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Compress(in, v.opt)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s.%s: %d bytes differ from the reference's %d", name, v.ext, len(got), len(want))
			}
			back, err := Decompress(want, v.opt, 0)
			if err != nil {
				t.Fatalf("%s.%s: %v", name, v.ext, err)
			}
			if !bytes.Equal(back, in) {
				t.Errorf("%s.%s: decompresses to %d bytes, not the input's %d", name, v.ext, len(back), len(in))
			}
		}
	}
}

func TestRoundTripsAndErrors(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, n := range []int{1, 2, 3, 17, 300, 2000} {
		in := make([]byte, n)
		for i := range in {
			in[i] = byte(r.Intn(4))
		}
		for _, opt := range []Options{{}, {Classic: true}, {Backwards: true}, {Quick: true}} {
			c, err := Compress(in, opt)
			if err != nil {
				t.Fatal(err)
			}
			d, err := Decompress(c, opt, 0)
			if err != nil || !bytes.Equal(d, in) {
				t.Fatalf("%d bytes, %+v: round trip failed: %v", n, opt, err)
			}
		}
	}
	if _, err := Compress(nil, Options{}); err == nil {
		t.Error("empty input accepted")
	}
	c, _ := Compress(bytes.Repeat([]byte{7}, 1000), Options{})
	if _, err := Decompress(c[:len(c)-1], Options{}, 0); err == nil {
		t.Error("truncated data accepted")
	}
	if _, err := Decompress(c, Options{}, 100); err == nil {
		t.Error("output past the limit accepted")
	}
}
