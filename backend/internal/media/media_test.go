package media

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/davidbyttow/govips/v2/vips"
)

func TestMain(m *testing.M) {
	if err := Startup(); err != nil {
		panic(err)
	}
	code := m.Run()
	Shutdown()
	os.Exit(code)
}

// The fixtures are made by testdata/generate.sh.
func TestRender(t *testing.T) {
	tests := []struct {
		file                      string
		viewerWidth, viewerHeight int
		gridWidth, gridHeight     int
	}{
		{"landscape.jpg", 800, 600, 400, 300},
		{"landscape.png", 800, 600, 400, 300},
		{"landscape.webp", 800, 600, 400, 300},
		{"landscape.heic", 800, 600, 400, 300},
		{"large.jpg", 2048, 1536, 400, 300},
		{"rotated.jpg", 600, 800, 300, 400},
	}
	for _, tt := range tests {
		for _, format := range []Format{WebP, JPEG} {
			t.Run(tt.file+"/"+string(format), func(t *testing.T) {
				got, err := Render(filepath.Join("testdata", tt.file), format)
				if err != nil {
					t.Fatal(err)
				}
				check(t, got.Viewer, tt.viewerWidth, tt.viewerHeight)
				check(t, got.Grid, tt.gridWidth, tt.gridHeight)
			})
		}
	}
}

func check(t *testing.T, p Preview, width, height int) {
	t.Helper()
	if p.Width != width || p.Height != height {
		t.Errorf("reported size = %dx%d, want %dx%d", p.Width, p.Height, width, height)
	}

	image, err := vips.NewImageFromBuffer(p.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()

	if image.Width() != width || image.Height() != height {
		t.Errorf("encoded size = %dx%d, want %dx%d", image.Width(), image.Height(), width, height)
	}
	if image.HasExif() {
		t.Error("preview contains EXIF")
	}
	if bytes.Contains(p.Data, []byte("Exif")) {
		t.Error("preview data contains an Exif marker")
	}
}

func TestRenderRejectsBrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.jpg")
	if err := os.WriteFile(path, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Render(path, WebP); err == nil {
		t.Fatal("expected an error")
	}
}
