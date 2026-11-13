package media

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

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

func TestCopySize(t *testing.T) {
	tests := []struct{ w, h, wantW, wantH int }{
		{3840, 2160, 1920, 1080},
		{2160, 3840, 1080, 1920},
		{1920, 1080, 1920, 1080},
		{1280, 720, 1280, 720},
		{1441, 1081, 1438, 1080},
		{321, 241, 320, 240},
	}
	for _, tt := range tests {
		if w, h := CopySize(tt.w, tt.h); w != tt.wantW || h != tt.wantH {
			t.Errorf("CopySize(%d, %d) = %dx%d, want %dx%d", tt.w, tt.h, w, h, tt.wantW, tt.wantH)
		}
	}
}

func TestTranscodeArgs(t *testing.T) {
	hw := Hardware{Device: "/dev/dri/renderD128"}
	sdr := strings.Join(TranscodeArgs("in.mov", "out.mp4", Video{Codec: "hevc", Width: 3840, Height: 2160}, hw), " ")
	for _, want := range []string{
		"-noautorotate -init_hw_device vaapi=va:/dev/dri/renderD128 -hwaccel vaapi",
		"-i in.mov",
		"-map 0:v:0 -map 0:a:0? -map_metadata -1",
		"-vf scale_vaapi=w=1920:h=1080:format=nv12 -c:v h264_vaapi -profile:v high",
		"-c:a aac",
		"-f mp4 out.mp4",
	} {
		if !strings.Contains(sdr, want) {
			t.Errorf("SDR args lack %q:\n%s", want, sdr)
		}
	}
	for _, unwanted := range []string{"opencl", "-low_power"} {
		if strings.Contains(sdr, unwanted) {
			t.Errorf("SDR args have %q:\n%s", unwanted, sdr)
		}
	}

	hw.LowPower = true
	hdr := TranscodeArgs("in.mov", "out.mp4", Video{Codec: "hevc", HDR: true, Width: 2160, Height: 3840}, hw)
	joined := strings.Join(hdr, " ")
	for _, want := range []string{
		"-init_hw_device opencl=ocl@va -filter_hw_device ocl",
		"scale_vaapi=w=1080:h=1920:format=p010,hwmap=derive_device=opencl:mode=read,tonemap_opencl=",
		"-c:v h264_vaapi -low_power 1",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("HDR args lack %q:\n%s", want, joined)
		}
	}
	// Input options go before the input.
	if slices.Index(hdr, "-filter_hw_device") > slices.Index(hdr, "-i") {
		t.Errorf("-filter_hw_device after the input:\n%s", joined)
	}
}

func TestParseVAInfo(t *testing.T) {
	const head = "vainfo: Driver version: Intel iHD driver for Intel(R) Gen Graphics - 25.4.6 ()\n" +
		"vainfo: Supported profile and entrypoints\n" +
		"      VAProfileH264High               :\tVAEntrypointVLD\n"
	full, err := parseVAInfo("/dev/dri/renderD128", head+
		"      VAProfileH264High               :\tVAEntrypointEncSlice\n"+
		"      VAProfileH264High               :\tVAEntrypointEncSliceLP\n")
	if err != nil || full.LowPower || !strings.Contains(full.Driver, "25.4.6") {
		t.Errorf("full encoder: %+v, %v", full, err)
	}
	lp, err := parseVAInfo("/dev/dri/renderD128", head+
		"      VAProfileH264High               :\tVAEntrypointEncSliceLP\n")
	if err != nil || !lp.LowPower {
		t.Errorf("low-power encoder: %+v, %v", lp, err)
	}
	if _, err := parseVAInfo("/dev/dri/renderD128", head); err == nil {
		t.Error("decoder only: no error")
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
