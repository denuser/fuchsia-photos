package media

/*
#cgo pkg-config: vips
#include <stdlib.h>
#include <vips/vips.h>

typedef struct {
	void *pixels;
	size_t size;
	int width;
	int height;
	int bands;
	VipsBandFormat format;
	VipsInterpretation interpretation;
} RawThumbnail;

static int raw_thumbnail(const char *path, int edge, int *width, int *height, RawThumbnail *result) {
	VipsImage *source = NULL;
	VipsImage *thumbnail = NULL;

	if (vips_magickload(path, &source, NULL))
		goto done;
	// ImageMagick's DNG coder exposes the orientation as metadata.
	const gboolean swap = vips_image_get_orientation_swap(source);
	*width = swap ? vips_image_get_height(source) : vips_image_get_width(source);
	*height = swap ? vips_image_get_width(source) : vips_image_get_height(source);
	if (vips_thumbnail_image(source, &thumbnail, edge, "height", edge,
		"size", VIPS_SIZE_DOWN, NULL))
		goto done;
	result->pixels = vips_image_write_to_memory(thumbnail, &result->size);
	result->width = vips_image_get_width(thumbnail);
	result->height = vips_image_get_height(thumbnail);
	result->bands = vips_image_get_bands(thumbnail);
	result->format = vips_image_get_format(thumbnail);
	result->interpretation = vips_image_get_interpretation(thumbnail);

done:
	if (thumbnail)
		g_object_unref(thumbnail);
	if (source)
		g_object_unref(source);
	return result->pixels ? 0 : -1;
}
*/
import "C"

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/davidbyttow/govips/v2/vips"
)

// renderRAW uses the best available source and reports its upright dimensions.
func renderRAW(ctx context.Context, path string, settings Settings) (Previews, int, int, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return Previews{}, 0, 0, err
	}
	probe, cancel := context.WithTimeout(ctx, 30*time.Second)
	embedded, orientation, edge := rawJPEG(probe, path)
	cancel()

	var embeddedErr error
	if len(embedded) != 0 && edge >= settings.ViewerEdge {
		previews, width, height, err := renderEmbeddedRAW(embedded, orientation, settings)
		if err == nil {
			return previews, width, height, nil
		}
		embeddedErr = err
	}
	if err := ctx.Err(); err != nil {
		return Previews{}, 0, 0, err
	}
	previews, width, height, err := decodeRAW(path, settings)
	if err != nil && embeddedErr == nil && len(embedded) != 0 {
		fallback, w, h, fallbackErr := renderEmbeddedRAW(embedded, orientation, settings)
		if fallbackErr == nil {
			return fallback, w, h, nil
		}
		embeddedErr = fallbackErr
	}
	if err != nil && embeddedErr != nil {
		err = errors.Join(err, embeddedErr)
	}
	return previews, width, height, err
}

func rawJPEG(ctx context.Context, path string) (best []byte, orientation, edge int) {
	output, err := exec.CommandContext(ctx, "exiftool", "-j", "-b", "-n",
		"-Orientation", "-JpgFromRaw", "-PreviewImage", "-OtherImage", path).Output()
	if err != nil {
		return nil, 1, 0
	}
	var records []struct {
		Orientation  int
		JpgFromRaw   string
		PreviewImage string
		OtherImage   string
	}
	if json.Unmarshal(output, &records) != nil || len(records) != 1 {
		return nil, 1, 0
	}
	record := records[0]
	orientation = record.Orientation
	if orientation < 1 || orientation > 8 {
		orientation = 1
	}
	bestArea := 0
	for _, value := range []string{record.JpgFromRaw, record.PreviewImage, record.OtherImage} {
		encoded, ok := strings.CutPrefix(value, "base64:")
		if !ok {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		size, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err == nil && size.Width*size.Height > bestArea {
			best, edge = data, max(size.Width, size.Height)
			bestArea = size.Width * size.Height
		}
	}
	return best, orientation, edge
}

func renderEmbeddedRAW(jpeg []byte, orientation int, settings Settings) (Previews, int, int, error) {
	image, err := vips.LoadImageFromBuffer(jpeg, vips.NewImportParams())
	if err != nil {
		return Previews{}, 0, 0, err
	}
	defer image.Close()
	width, height := image.Width(), image.Height()
	if orientation >= 5 {
		width, height = height, width
	}
	if err := image.SetOrientation(orientation); err != nil {
		return Previews{}, 0, 0, err
	}
	if err := image.ThumbnailWithSize(settings.ViewerEdge, settings.ViewerEdge, vips.InterestingNone, vips.SizeDown); err != nil {
		return Previews{}, 0, 0, err
	}
	previews, err := renderFrom(image, settings)
	return previews, width, height, err
}

func disableVipsCache() {
	C.vips_cache_set_max(0)
}

func decodeRAW(path string, settings Settings) (Previews, int, int, error) {
	filename := C.CString("DNG:" + path)
	defer C.free(unsafe.Pointer(filename))
	var width, height C.int
	var thumbnail C.RawThumbnail
	// Read libvips's error buffer on the thread that ran the operation.
	runtime.LockOSThread()
	result := C.raw_thumbnail(filename, C.int(settings.ViewerEdge), &width, &height, &thumbnail)
	var message string
	if result != 0 {
		message = strings.TrimSpace(C.GoString(C.vips_error_buffer()))
		C.vips_error_clear()
	}
	runtime.UnlockOSThread()
	if result != 0 {
		return Previews{}, 0, 0, fmt.Errorf("decode RAW %s: %s", path, message)
	}
	pixels := unsafe.Slice((*byte)(thumbnail.pixels), int(thumbnail.size))
	image, err := vips.NewImageFromMemory(pixels, int(thumbnail.width), int(thumbnail.height), int(thumbnail.bands),
		vips.BandFormat(thumbnail.format), vips.Interpretation(thumbnail.interpretation))
	C.g_free(C.gpointer(thumbnail.pixels))
	if err != nil {
		return Previews{}, 0, 0, fmt.Errorf("load RAW thumbnail: %w", err)
	}
	defer image.Close()
	previews, err := renderFrom(image, settings)
	return previews, int(width), int(height), err
}
