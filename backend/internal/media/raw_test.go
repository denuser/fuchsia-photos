package media

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/denuser/fuchsia-photos/backend/internal/storage"
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
