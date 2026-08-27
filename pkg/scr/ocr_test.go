package scr

import (
	"image"
	"image/png"
	"os"
	"testing"
)

// TestRecognizeScreenFromSCR decodes a real .scr fixture captured from a
// running CSpect session (the ADD HL,A Z80N ground-truth test from the
// zen80 investigation) and checks the recognised text against the answer
// independently confirmed at the time by direct visual inspection of the
// same screenshot. This is the no-geometry path: a .scr file is always
// exactly the native 256x192 pixels.
func TestRecognizeScreenFromSCR(t *testing.T) {
	raw, err := os.ReadFile("testdata/ocr-basic-output.scr")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	screen, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	lines, err := RecognizeScreen(screen)
	if err != nil {
		t.Fatalf("RecognizeScreen: %v", err)
	}
	if len(lines) != 24 {
		t.Fatalf("got %d lines, want 24", len(lines))
	}
	if lines[0] != "F=129 HL=4101" {
		t.Errorf("line 0 = %q, want %q", lines[0], "F=129 HL=4101")
	}
	if lines[23] != "0 OK, 20:1" {
		t.Errorf("line 23 = %q, want %q", lines[23], "0 OK, 20:1")
	}
	for i := 1; i < 23; i++ {
		if lines[i] != "" {
			t.Errorf("line %d = %q, want empty", i, lines[i])
		}
	}
}

// TestRecognizeTextWithGeometry exercises the geometry path directly
// against a raw CSpect window screenshot (not cropped to native
// resolution first): -w3 renders the full bordered 320x256 display at 3x
// scale within a larger capture, so the 256x192 inner display sits at a
// non-zero, non-trivial origin. This is the same screenshot the .scr
// fixture above was cropped from, so the two tests check the same known
// answer through two independent code paths.
func TestRecognizeTextWithGeometry(t *testing.T) {
	f, err := os.Open("testdata/ocr-cspect-w3-screenshot.png")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	geom := Geometry{OriginX: 256, OriginY: 112, Scale: 3}
	lines, err := RecognizeText(img, geom)
	if err != nil {
		t.Fatalf("RecognizeText: %v", err)
	}
	if lines[0] != "F=129 HL=4101" {
		t.Errorf("line 0 = %q, want %q", lines[0], "F=129 HL=4101")
	}
	if lines[23] != "0 OK, 20:1" {
		t.Errorf("line 23 = %q, want %q", lines[23], "0 OK, 20:1")
	}
}

// TestRecognizeTextColouredInk checks recognition against a real NextZXOS
// menu screenshot, whose shortcut letters are rendered in blue rather than
// plain black -- the specific case that caught a real bug during
// development (an absolute-darkness ink threshold silently dropped every
// coloured letter while black text on the same screen recognised
// correctly). It also mixes a white menu-box background with the grey
// desktop behind it, exercising per-cell (rather than screen-wide) paper
// colour detection.
func TestRecognizeTextColouredInk(t *testing.T) {
	f, err := os.Open("testdata/ocr-cspect-w3-menu.png")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	geom := Geometry{OriginX: 256, OriginY: 112, Scale: 3}
	lines, err := RecognizeText(img, geom)
	if err != nil {
		t.Fatalf("RecognizeText: %v", err)
	}

	want := []string{
		"DiagTest",
		"Browser",
		"Command Line",
		"NextBASIC",
		"Calculator",
		"Guide",
		"More...",
	}
	for _, w := range want {
		found := false
		for _, l := range lines {
			if containsSubstring(l, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected menu item %q not found in recognised text: %q", w, lines)
		}
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestRecognizeTextRejectsNothingBlankIsSpace confirms a genuinely blank
// region reads back as all-space lines, not garbage -- the degenerate case
// bestMatch's empty-cell short-circuit exists for.
func TestRecognizeTextBlankIsSpace(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 256, 192))
	// Leave every pixel at its zero value (transparent/black); isInk's
	// per-cell paper detection will treat that uniform colour as paper,
	// so every cell should recognise as a space.
	lines, err := RecognizeText(img, NativeGeometry)
	if err != nil {
		t.Fatalf("RecognizeText: %v", err)
	}
	for i, l := range lines {
		if l != "" {
			t.Errorf("line %d = %q on a blank image, want empty", i, l)
		}
	}
}
