// Package media generates photo previews with libvips.
package media

import (
	"fmt"

	"github.com/davidbyttow/govips/v2/vips"
)

// Limits of the settings. A grid preview is made from the viewer preview and
// never scaled up, so it is never larger than the viewer preview.
const (
	MinQuality    = 1
	MaxQuality    = 100
	MinWebPEffort = 0
	MaxWebPEffort = 6
	MinGridEdge   = 200
	MaxGridEdge   = 800
	MinViewerEdge = 1024
	MaxViewerEdge = 4096
)

// Format is a preview image format.
type Format string

// Supported preview formats.
const (
	WebP Format = "webp"
	JPEG Format = "jpeg"
)

// Settings control how previews are generated. ViewerEdge is the longest side of
// the viewer preview in pixels. GridEdge is the shortest side of the grid
// preview, so that a square cut from it for the grid keeps full resolution.
type Settings struct {
	Format      Format
	WebPQuality int
	WebPEffort  int
	JPEGQuality int
	GridEdge    int
	ViewerEdge  int
}

// DefaultSettings returns the settings chosen from the benchmark. A new database
// starts with the same values.
func DefaultSettings() Settings {
	return Settings{Format: WebP, WebPQuality: 75, WebPEffort: 2, JPEGQuality: 80, GridEdge: 400, ViewerEdge: 2048}
}

// Validate returns an error that describes the first setting out of range.
func (s Settings) Validate() error {
	if s.Format != WebP && s.Format != JPEG {
		return fmt.Errorf("format must be %s or %s", WebP, JPEG)
	}
	for _, field := range []struct {
		name          string
		value, lo, hi int
	}{
		{"WebP quality", s.WebPQuality, MinQuality, MaxQuality},
		{"WebP effort", s.WebPEffort, MinWebPEffort, MaxWebPEffort},
		{"JPEG quality", s.JPEGQuality, MinQuality, MaxQuality},
		{"grid preview size", s.GridEdge, MinGridEdge, MaxGridEdge},
		{"viewer preview size", s.ViewerEdge, MinViewerEdge, MaxViewerEdge},
	} {
		if field.value < field.lo || field.value > field.hi {
			return fmt.Errorf("%s must be from %d to %d", field.name, field.lo, field.hi)
		}
	}
	return nil
}

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
func Render(path string, settings Settings) (Previews, error) {
	viewerImage, err := vips.NewThumbnailWithSizeFromFile(path, settings.ViewerEdge, settings.ViewerEdge, vips.InterestingNone, vips.SizeDown)
	if err != nil {
		return Previews{}, fmt.Errorf("load %s: %w", path, err)
	}
	defer viewerImage.Close()

	viewer, err := encode(viewerImage, settings)
	if err != nil {
		return Previews{}, fmt.Errorf("encode viewer preview of %s: %w", path, err)
	}

	grid, err := RenderGrid(viewer.Data, settings)
	if err != nil {
		return Previews{}, fmt.Errorf("grid preview of %s: %w", path, err)
	}
	return Previews{Viewer: viewer, Grid: grid}, nil
}

// RenderGrid makes the grid preview from the data of a viewer preview. The
// shortest side becomes GridEdge, and a viewer preview smaller than that keeps
// its size, so a panorama is limited by the viewer preview size.
func RenderGrid(viewer []byte, settings Settings) (Preview, error) {
	source, err := vips.NewImageFromBuffer(viewer)
	if err != nil {
		return Preview{}, fmt.Errorf("load viewer preview: %w", err)
	}
	width, height := source.Width(), source.Height()
	source.Close()

	// The thumbnail fits a box, and the side that must become GridEdge sets it.
	if width >= height {
		height = settings.GridEdge
	} else {
		width = settings.GridEdge
	}
	image, err := vips.NewThumbnailWithSizeFromBuffer(viewer, width, height, vips.InterestingNone, vips.SizeDown)
	if err != nil {
		return Preview{}, fmt.Errorf("resize: %w", err)
	}
	defer image.Close()

	grid, err := encode(image, settings)
	if err != nil {
		return Preview{}, fmt.Errorf("encode: %w", err)
	}
	return grid, nil
}

func encode(image *vips.ImageRef, settings Settings) (Preview, error) {
	var (
		data []byte
		err  error
	)
	switch settings.Format {
	case WebP:
		data, _, err = image.ExportWebp(&vips.WebpExportParams{Quality: settings.WebPQuality, ReductionEffort: settings.WebPEffort, StripMetadata: true})
	case JPEG:
		data, _, err = image.ExportJpeg(&vips.JpegExportParams{Quality: settings.JPEGQuality, StripMetadata: true})
	default:
		err = fmt.Errorf("unknown format %q", settings.Format)
	}
	if err != nil {
		return Preview{}, err
	}
	return Preview{Data: data, Width: image.Width(), Height: image.Height()}, nil
}
