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
				settings := DefaultSettings()
				settings.Format = format
				got, err := Render(filepath.Join("testdata", tt.file), settings)
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
	if _, err := Render(path, DefaultSettings()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestRenderUsesSettings(t *testing.T) {
	path := filepath.Join("testdata", "large.jpg")
	for _, format := range []Format{WebP, JPEG} {
		t.Run(string(format), func(t *testing.T) {
			settings := DefaultSettings()
			settings.Format = format
			settings.GridEdge = 200
			settings.ViewerEdge = 1024

			low, high := settings, settings
			low.WebPQuality, low.JPEGQuality = 10, 10
			high.WebPQuality, high.JPEGQuality = 95, 95

			small, err := Render(path, low)
			if err != nil {
				t.Fatal(err)
			}
			check(t, small.Viewer, 1024, 768)
			check(t, small.Grid, 200, 150)

			large, err := Render(path, high)
			if err != nil {
				t.Fatal(err)
			}
			if len(small.Viewer.Data) >= len(large.Viewer.Data) {
				t.Errorf("quality 10 took %d bytes, quality 95 took %d", len(small.Viewer.Data), len(large.Viewer.Data))
			}
		})
	}
}

func TestValidateSettings(t *testing.T) {
	if err := DefaultSettings().Validate(); err != nil {
		t.Errorf("the defaults are invalid: %v", err)
	}
	tests := map[string]func(*Settings){
		"unknown format":    func(s *Settings) { s.Format = "avif" },
		"WebP quality low":  func(s *Settings) { s.WebPQuality = 0 },
		"WebP quality high": func(s *Settings) { s.WebPQuality = 101 },
		"WebP effort low":   func(s *Settings) { s.WebPEffort = -1 },
		"WebP effort high":  func(s *Settings) { s.WebPEffort = 7 },
		"JPEG quality low":  func(s *Settings) { s.JPEGQuality = 0 },
		"JPEG quality high": func(s *Settings) { s.JPEGQuality = 101 },
		"grid size low":     func(s *Settings) { s.GridEdge = 199 },
		"grid size high":    func(s *Settings) { s.GridEdge = 801 },
		"viewer size low":   func(s *Settings) { s.ViewerEdge = 1023 },
		"viewer size high":  func(s *Settings) { s.ViewerEdge = 4097 },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			settings := DefaultSettings()
			change(&settings)
			if err := settings.Validate(); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
