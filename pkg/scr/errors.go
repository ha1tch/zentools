package scr

import (
	"fmt"
	"image"
)

// SizeError reports that an image was not the size a conversion needs.
type SizeError struct {
	Got image.Point
	// Want is the size needed; zero means the classic 256x192.
	Want image.Point
}

func (e *SizeError) Error() string {
	want := e.Want
	if want == (image.Point{}) {
		want = image.Pt(Width, Height)
	}
	return fmt.Sprintf("scr: image is %dx%d, want %dx%d (resize before converting)",
		e.Got.X, e.Got.Y, want.X, want.Y)
}
