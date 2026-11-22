package media

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denuser/magellanica-photos/backend/internal/storage"
)

func TestEmbeddedRAWPreviewOrientation(t *testing.T) {
	// The JPEG advertises orientation 6, but the enclosing RAW supplies
	// the authoritative orientation. Exercise overriding it both ways.
	data, err := os.ReadFile(filepath.Join("testdata", "rotated.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ orientation, width, height int }{
		{1, 800, 600},
		{6, 600, 800},
	} {
		previews, w, h, err := renderEmbeddedRAW(data, tc.orientation, DefaultSettings())
		if err != nil {
			t.Fatal(err)
		}
		if w != tc.width || h != tc.height {
			t.Errorf("orientation %d: size %dx%d, want %dx%d", tc.orientation, w, h, tc.width, tc.height)
		}
		check(t, previews.Viewer, tc.width, tc.height)
	}
}

func TestEmbeddedRAWPreviewWithJPEGWarning(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "rotated.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	// Keep the signature intact and insert two extraneous bytes before DQT.
	marker := bytes.Index(data, []byte{0xff, 0xdb})
	if marker < 3 {
		t.Fatal("JPEG fixture has no quantization table")
	}
	jpeg := make([]byte, 0, len(data)+2)
	jpeg = append(jpeg, data[:marker]...)
	jpeg = append(jpeg, 0x42, 0x42)
	jpeg = append(jpeg, data[marker:]...)
	previews, _, _, err := renderEmbeddedRAW(jpeg, 6, DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	check(t, previews.Viewer, 600, 800)
}

func TestRenderRAW(t *testing.T) {
	// The valid synthetic DNG contains a 64x48 Bayer image and a 1200x900
	// JPEG, with orientation 6. The second file has a broken sensor image.
	tests := []struct {
		name, file       string
		edge             int
		width, height    int
		viewerW, viewerH int
	}{
		{"prefer embedded", "rotated.dng", 1024, 900, 1200, 768, 1024},
		{"decode sensor", "rotated.dng", 2048, 48, 64, 48, 64},
		{"fallback to embedded", "rotated-preview-only.dng", 2048, 900, 1200, 900, 1200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.ViewerEdge = tt.edge
			path := filepath.Join("testdata", tt.file)
			previews, width, height, err := RenderPhoto(t.Context(), path, storage.RAWDNG, settings)
			if err != nil {
				t.Fatal(err)
			}
			if width != tt.width || height != tt.height {
				t.Errorf("upright dimensions = %dx%d, want %dx%d", width, height, tt.width, tt.height)
			}
			check(t, previews.Viewer, tt.viewerW, tt.viewerH)
		})
	}
}

func TestRAWFallsBackWithoutColourData(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "rotated.dng"))
	if err != nil {
		t.Fatal(err)
	}
	// Replace the ColorMatrix1 tag with a private tag of the same size.
	matrix := []byte{0x21, 0xc6, 10, 0, 9, 0, 0, 0}
	tag := bytes.Index(data, matrix)
	if tag < 0 {
		t.Fatal("DNG fixture has no ColorMatrix1 tag")
	}
	copy(data[tag:], []byte{0xe9, 0xfd})
	path := filepath.Join(t.TempDir(), "no-colour.dng")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	settings := DefaultSettings()
	if _, _, _, err := decodeRAW(path, settings); err == nil || !strings.Contains(err.Error(), "no colour data") {
		t.Fatalf("sensor decode error = %v, want missing colour data", err)
	}
	previews, _, _, err := RenderPhoto(t.Context(), path, storage.RAWDNG, settings)
	if err != nil {
		t.Fatal(err)
	}
	check(t, previews.Viewer, 900, 1200)
}
