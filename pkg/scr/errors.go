package scr

import (
	"fmt"
	"image"
)

// SizeError reports that an image passed to FromImage was not 256x192.
type SizeError struct {
	Got image.Point
}

func (e *SizeError) Error() string {
	return fmt.Sprintf("scr: image is %dx%d, want %dx%d (resize before converting)",
		e.Got.X, e.Got.Y, Width, Height)
}
