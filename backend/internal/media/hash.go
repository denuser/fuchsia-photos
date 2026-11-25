package media

import (
	"fmt"

	"github.com/davidbyttow/govips/v2/vips"
)

const (
	hashColumns = 9
	hashRows    = 8
)

// Hash returns the difference hash of an image: it is reduced to nine by eight
// grey pixels, and each of the 64 bits tells whether a pixel is brighter than
// its right neighbour. A copy that was resized or saved again differs from the
// original in a few bits, and a different picture in many. It does not survive
// a crop, which is what the vector of the ML service is for.
func Hash(data []byte) (uint64, error) {
	image, err := vips.NewImageFromBuffer(data)
	if err != nil {
		return 0, fmt.Errorf("load: %w", err)
	}
	defer image.Close()

	if image.HasAlpha() {
		if err := image.Flatten(&vips.Color{R: 255, G: 255, B: 255}); err != nil {
			return 0, fmt.Errorf("flatten: %w", err)
		}
	}
	if err := image.ToColorSpace(vips.InterpretationBW); err != nil {
		return 0, fmt.Errorf("grey: %w", err)
	}
	err = image.ResizeWithVScale(float64(hashColumns)/float64(image.Width()), float64(hashRows)/float64(image.Height()), vips.KernelLinear)
	if err != nil {
		return 0, fmt.Errorf("resize: %w", err)
	}
	if image.Width() != hashColumns || image.Height() != hashRows {
		return 0, fmt.Errorf("resize gave %dx%d", image.Width(), image.Height())
	}
	pixels, err := image.ToBytes()
	if err != nil {
		return 0, fmt.Errorf("read pixels: %w", err)
	}
	bands := image.Bands()
	if len(pixels) < hashColumns*hashRows*bands {
		return 0, fmt.Errorf("read %d bytes of pixels", len(pixels))
	}
	at := func(x, y int) byte { return pixels[(y*hashColumns+x)*bands] }

	var hash uint64
	for y := 0; y < hashRows; y++ {
		for x := 0; x < hashColumns-1; x++ {
			hash <<= 1
			if at(x, y) > at(x+1, y) {
				hash |= 1
			}
		}
	}
	return hash, nil
}
