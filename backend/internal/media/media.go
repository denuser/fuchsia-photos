// Package media generates photo previews with libvips.
package media

import (
	"fmt"

	"github.com/davidbyttow/govips/v2/vips"
)

// Preview sizes, as the longest edge in pixels.
const (
	GridEdge   = 400
	ViewerEdge = 2048
)

// Format is a preview image format.
type Format string

// Supported preview formats.
const (
	WebP Format = "webp"
	JPEG Format = "jpeg"
)

// Preview is an encoded preview image.
type Preview struct {
	Data   []byte
	Width  int
	Height int
}

// Previews holds all previews generated for a photo.
type Previews struct {
	Viewer Preview
	Grid   Preview
}

// Startup initializes libvips. Parallelism comes from running several jobs at
// once, so each job gets one thread. The operation cache stays off, because
// every photo is processed only once.
func Startup() error {
	vips.LoggingSettings(nil, vips.LogLevelWarning)
	return vips.Startup(&vips.Config{ConcurrencyLevel: 1})
}

// Shutdown releases libvips.
func Shutdown() {
	vips.Shutdown()
}

// Dimensions returns the size of the photo at path as it is displayed, taking
// the EXIF orientation into account.
func Dimensions(path string) (width, height int, err error) {
	image, err := vips.NewImageFromFile(path)
	if err != nil {
		return 0, 0, fmt.Errorf("load %s: %w", path, err)
	}
	defer image.Close()

	if image.Orientation() >= 5 {
		return image.Height(), image.Width(), nil
	}
	return image.Width(), image.Height(), nil
}

// Render generates the previews of the photo at path. They are upright, in
// sRGB and contain no metadata, so GPS coordinates do not leak through sharing.
//
// The grid preview is made from the viewer preview rather than the original,
// because decoding a HEIC file a second time would be far more expensive.
func Render(path string, format Format) (Previews, error) {
	viewerImage, err := vips.NewThumbnailWithSizeFromFile(path, ViewerEdge, ViewerEdge, vips.InterestingNone, vips.SizeDown)
	if err != nil {
		return Previews{}, fmt.Errorf("load %s: %w", path, err)
	}
	defer viewerImage.Close()

	viewer, err := encode(viewerImage, format)
	if err != nil {
		return Previews{}, fmt.Errorf("encode viewer preview of %s: %w", path, err)
	}

	gridImage, err := vips.NewThumbnailWithSizeFromBuffer(viewer.Data, GridEdge, GridEdge, vips.InterestingNone, vips.SizeDown)
	if err != nil {
		return Previews{}, fmt.Errorf("resize %s: %w", path, err)
	}
	defer gridImage.Close()

	grid, err := encode(gridImage, format)
	if err != nil {
		return Previews{}, fmt.Errorf("encode grid preview of %s: %w", path, err)
	}
	return Previews{Viewer: viewer, Grid: grid}, nil
}

func encode(image *vips.ImageRef, format Format) (Preview, error) {
	var (
		data []byte
		err  error
	)
	switch format {
	case WebP:
		data, _, err = image.ExportWebp(&vips.WebpExportParams{Quality: 75, ReductionEffort: 2, StripMetadata: true})
	case JPEG:
		data, _, err = image.ExportJpeg(&vips.JpegExportParams{Quality: 80, StripMetadata: true})
	default:
		err = fmt.Errorf("unknown format %q", format)
	}
	if err != nil {
		return Preview{}, err
	}
	return Preview{Data: data, Width: image.Width(), Height: image.Height()}, nil
}
