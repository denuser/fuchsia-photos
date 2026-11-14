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

func TestNeedsCopy(t *testing.T) {
	tests := []struct {
		video Video
		want  bool
	}{
		{Video{Codec: "hevc", Width: 3840, Height: 2160}, true},
		{Video{Codec: "vp9", Width: 1920, Height: 1080}, true},
		{Video{Codec: "av1", Width: 1920, Height: 1080}, true},
		{Video{Codec: "h264", Width: 3840, Height: 2160}, false},
		{Video{Codec: "prores", Width: 3840, Height: 2160}, false},
		{Video{Codec: "mpeg4", Width: 640, Height: 480}, false},
		{Video{Codec: "hevc"}, false},
	}
	for _, tt := range tests {
		if got := NeedsCopy(tt.video); got != tt.want {
			t.Errorf("NeedsCopy(%+v) = %v, want %v", tt.video, got, tt.want)
		}
	}
}

// The fixtures are H.264, which needs no copy.
func TestProbeCodec(t *testing.T) {
	video, err := ProbeVideo(context.Background(), filepath.Join("testdata", "video.mov"))
	if err != nil {
		t.Fatal(err)
	}
	if video.Codec != "h264" || video.Width != 320 || video.Height != 240 || NeedsCopy(video) {
		t.Errorf("probe = %+v", video)
	}
}
