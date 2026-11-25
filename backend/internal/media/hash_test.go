package media

import (
	"math/bits"
	"path/filepath"
	"testing"
)

// A copy at another size and quality is a few bits away, another picture many.
func TestHash(t *testing.T) {
	render := func(file string, edge int, format Format, quality int) []byte {
		t.Helper()
		settings := DefaultSettings()
		settings.Format = format
		settings.ViewerEdge = edge
		settings.JPEGQuality = quality
		previews, err := Render(filepath.Join("testdata", file), settings)
		if err != nil {
			t.Fatal(err)
		}
		return previews.Viewer.Data
	}
	hash := func(data []byte) uint64 {
		t.Helper()
		h, err := Hash(data)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}

	original := hash(render("large.jpg", 2048, WebP, 0))
	for name, copyHash := range map[string]uint64{
		"half size":   hash(render("large.jpg", 1024, JPEG, 80)),
		"small":       hash(render("large.jpg", 400, JPEG, 40)),
		"other codec": hash(render("large.jpg", 2048, JPEG, 30)),
	} {
		if distance := bits.OnesCount64(original ^ copyHash); distance > 4 {
			t.Errorf("%s: %d bits away from the original, want at most 4", name, distance)
		}
	}

	other := hash(render("rotated.jpg", 800, JPEG, 80))
	if distance := bits.OnesCount64(original ^ other); distance < 10 {
		t.Errorf("another picture is %d bits away, want at least 10", distance)
	}
}

func TestHashRejectsNoImage(t *testing.T) {
	if _, err := Hash([]byte("not an image")); err == nil {
		t.Error("want an error for data that is no image")
	}
}
