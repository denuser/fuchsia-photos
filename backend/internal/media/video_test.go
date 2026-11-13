package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The fixtures are made by testdata/generate.sh: two seconds of 320x240
// video, which video.mp4 plays as portrait.
func TestRenderVideo(t *testing.T) {
	tests := []struct {
		file          string
		hdr           bool
		width, height int
	}{
		{"video.mp4", false, 240, 320},
		{"video.mov", false, 320, 240},
		{"hdr.mp4", true, 320, 240},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join("testdata", tt.file)
			video, err := ProbeVideo(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			if video.Duration != 2 || video.HDR != tt.hdr {
				t.Errorf("probe = %+v, want 2 s, HDR %v", video, tt.hdr)
			}

			previews, width, height, err := RenderVideo(ctx, path, video, DefaultSettings())
			if err != nil {
				t.Fatal(err)
			}
			if width != tt.width || height != tt.height {
				t.Errorf("size = %dx%d, want %dx%d", width, height, tt.width, tt.height)
			}
			check(t, previews.Viewer, tt.width, tt.height)
			check(t, previews.Grid, tt.width, tt.height)
		})
	}
}

// A frame past the end of a short video would be empty, so the poster comes
// from its middle.
func TestPosterOfShortVideo(t *testing.T) {
	poster, err := Poster(context.Background(), filepath.Join("testdata", "video.mov"), Video{Duration: 0.15})
	if err != nil {
		t.Fatal(err)
	}
	if len(poster) == 0 {
		t.Error("empty poster")
	}
}

func TestProbeBrokenVideo(t *testing.T) {
	broken := filepath.Join(t.TempDir(), "broken.mp4")
	if err := os.WriteFile(broken, append([]byte("\x00\x00\x00\x14ftypisom\x00\x00\x00\x00isom"), make([]byte, 100)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ProbeVideo(context.Background(), broken); err == nil || ToolMissing(err) {
		t.Errorf("broken video: err = %v", err)
	}
	if _, err := ProbeVideo(context.Background(), filepath.Join("testdata", "missing.mp4")); err == nil || ToolMissing(err) {
		t.Errorf("missing file: err = %v", err)
	}
}
