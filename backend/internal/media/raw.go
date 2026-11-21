package media

/*
#cgo pkg-config: vips libraw
#include <libraw/libraw.h>
#include <stdint.h>
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

// Above this sensor size, use LibRaw's reduced Bayer image. Bound the
// unpacked sensor too because half_size only affects postprocessing.
#define RAW_FULL_PIXELS UINT64_C(120000000)
#define RAW_MAX_PIXELS UINT64_C(240000000)
#define RAW_UNPACK_MB 1536

static int raw_thumbnail(const char *path, int edge, int *width, int *height, RawThumbnail *result, char **error) {
	libraw_data_t *raw = libraw_init(0);
	libraw_processed_image_t *bitmap = NULL;
	VipsImage *source = NULL;
	VipsImage *thumbnail = NULL;
	if (!raw) {
		*error = g_strdup("cannot allocate LibRaw decoder");
		return -1;
	}

	raw->params.use_camera_wb = 1;
	raw->params.use_camera_matrix = 1;
	raw->rawparams.max_raw_memory_mb = RAW_UNPACK_MB;
	int status = libraw_open_file(raw, path);
	if (status != LIBRAW_SUCCESS)
		goto done;

	// LibRaw leaves raw_color set when it found no camera colour conversion.
	// A monochrome sensor does not need a colour matrix.
	if (raw->idata.colors > 1 && raw->rawdata.ioparams.raw_color) {
		*error = g_strdup("LibRaw has no colour data for this camera");
		goto done;
	}

	const uint64_t pixels = (uint64_t)raw->sizes.raw_width * raw->sizes.raw_height;
	if (pixels > RAW_MAX_PIXELS || (pixels > RAW_FULL_PIXELS && !raw->idata.filters)) {
		*error = g_strdup("RAW sensor exceeds the safe decode size");
		goto done;
	}
	raw->params.half_size = pixels > RAW_FULL_PIXELS;
	const int swap = raw->sizes.flip & 4;
	*width = swap ? raw->sizes.height : raw->sizes.width;
	*height = swap ? raw->sizes.width : raw->sizes.height;

	status = libraw_unpack(raw);
	if (status != LIBRAW_SUCCESS)
		goto done;
	// LibRaw ignores half_size for unpacked multi-channel images.
	if (raw->params.half_size && (!raw->idata.filters ||
		raw->rawdata.color3_image || raw->rawdata.color4_image ||
		raw->rawdata.float3_image || raw->rawdata.float4_image)) {
		*error = g_strdup("RAW sensor cannot be processed safely at half size");
		goto done;
	}
	status = libraw_dcraw_process(raw);
	if (status != LIBRAW_SUCCESS)
		goto done;
	bitmap = libraw_dcraw_make_mem_image(raw, &status);
	if (!bitmap)
		goto done;
	if (bitmap->type != LIBRAW_IMAGE_BITMAP || bitmap->bits != 8 ||
		(bitmap->colors != 1 && bitmap->colors != 3)) {
		*error = g_strdup("unexpected LibRaw output format");
		goto done;
	}

	if (!raw->params.half_size) {
		*width = bitmap->width;
		*height = bitmap->height;
	}
	const VipsInterpretation interpretation = bitmap->colors == 1 ?
		VIPS_INTERPRETATION_B_W : VIPS_INTERPRETATION_sRGB;
	// Release LibRaw's large working buffers before resizing the bitmap.
	libraw_close(raw);
	raw = NULL;
	VipsImage *view = vips_image_new_from_memory(bitmap->data, bitmap->data_size,
		bitmap->width, bitmap->height, bitmap->colors, VIPS_FORMAT_UCHAR);
	if (!view || vips_copy(view, &source, "interpretation", interpretation, NULL)) {
		*error = vips_error_buffer_copy();
		if (view)
			g_object_unref(view);
		goto done;
	}
	g_object_unref(view);
	if (vips_thumbnail_image(source, &thumbnail, edge, "height", edge,
		"size", VIPS_SIZE_DOWN, "no_rotate", TRUE, NULL)) {
		*error = vips_error_buffer_copy();
		goto done;
	}
	result->pixels = vips_image_write_to_memory(thumbnail, &result->size);
	if (!result->pixels) {
		*error = vips_error_buffer_copy();
		goto done;
	}
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
	if (bitmap)
		libraw_dcraw_clear_mem(bitmap);
	if (!result->pixels && !*error)
		*error = g_strdup(libraw_strerror(status));
	if (raw)
		libraw_close(raw);
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
	"strings"
	"time"
	"unsafe"

	"github.com/davidbyttow/govips/v2/vips"
)

// rawDecodes serializes sensor decodes, which can hold a large working image.
// Embedded JPEG previews do not wait for it.
var rawDecodes = make(chan struct{}, 1)

// renderRAW uses the best available source and reports its upright dimensions.
// Cancelling ctx stops a job waiting for the decode slot. A job inside
// LibRaw returns after the native call finishes.
func renderRAW(ctx context.Context, path string, settings Settings) (Previews, int, int, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return Previews{}, 0, 0, err
	}
	probe, cancel := context.WithTimeout(ctx, 30*time.Second)
	embedded, orientation, edge, camera := rawJPEG(probe, path)
	cancel()

	var embeddedErr error
	if len(embedded) != 0 && edge >= settings.ViewerEdge {
		previews, width, height, err := renderEmbeddedRAW(embedded, orientation, settings)
		if err == nil {
			return previews, width, height, nil
		}
		embeddedErr = err
	}
	previews, width, height, err := decodeRAWInTurn(ctx, path, settings)
	if ctxErr := ctx.Err(); ctxErr != nil && err != nil {
		return Previews{}, 0, 0, ctxErr
	}
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
	if err != nil && camera != "" {
		err = fmt.Errorf("%s: %w", camera, err)
	}
	return previews, width, height, err
}

func rawJPEG(ctx context.Context, path string) (best []byte, orientation, edge int, camera string) {
	output, err := exec.CommandContext(ctx, "exiftool", "-j", "-b", "-n",
		"-Orientation", "-Make", "-Model", "-JpgFromRaw", "-PreviewImage", "-OtherImage", path).Output()
	if err != nil {
		return nil, 1, 0, ""
	}
	var records []struct {
		Orientation  int
		Make         string
		Model        string
		JpgFromRaw   string
		PreviewImage string
		OtherImage   string
	}
	if json.Unmarshal(output, &records) != nil || len(records) != 1 {
		return nil, 1, 0, ""
	}
	record := records[0]
	camera = strings.TrimSpace(record.Model)
	makeName := strings.TrimSpace(record.Make)
	if !strings.HasPrefix(strings.ToLower(camera), strings.ToLower(makeName)) {
		camera = strings.TrimSpace(makeName + " " + camera)
	}
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
	return best, orientation, edge, camera
}

func renderEmbeddedRAW(jpeg []byte, orientation int, settings Settings) (Previews, int, int, error) {
	params := vips.NewImportParams()
	params.FailOnError.Set(false)
	image, err := vips.LoadImageFromBuffer(jpeg, params)
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

// decodeRAWInTurn waits for the decode slot unless ctx ends first.
func decodeRAWInTurn(ctx context.Context, path string, settings Settings) (Previews, int, int, error) {
	select {
	case rawDecodes <- struct{}{}:
	case <-ctx.Done():
		return Previews{}, 0, 0, ctx.Err()
	}
	defer func() { <-rawDecodes }()
	if err := ctx.Err(); err != nil {
		return Previews{}, 0, 0, err
	}
	return decodeRAW(path, settings)
}

func disableVipsCache() {
	C.vips_cache_set_max(0)
}

func decodeRAW(path string, settings Settings) (Previews, int, int, error) {
	filename := C.CString(path)
	defer C.free(unsafe.Pointer(filename))
	var width, height C.int
	var thumbnail C.RawThumbnail
	var cError *C.char
	if C.raw_thumbnail(filename, C.int(settings.ViewerEdge), &width, &height, &thumbnail, &cError) != 0 {
		message := strings.TrimSpace(C.GoString(cError))
		C.g_free(C.gpointer(cError))
		if message == "" {
			message = "unknown error"
		}
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
