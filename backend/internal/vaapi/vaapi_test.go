package vaapi

import (
	"slices"
	"strings"
	"testing"
)

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
	hw := Hardware{Device: "/dev/dri/renderD128", Vendor: VendorIntel}
	sdrArgs, err := TranscodeArgs("in.mov", "out.mp4", Source{Width: 3840, Height: 2160}, hw)
	if err != nil {
		t.Fatal(err)
	}
	sdr := strings.Join(sdrArgs, " ")
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
	hdr, err := TranscodeArgs("in.mov", "out.mp4", Source{HDR: true, Width: 2160, Height: 3840}, hw)
	if err != nil {
		t.Fatal(err)
	}
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

	if _, err := TranscodeArgs("in.mov", "out.mp4", Source{Width: 3840, Height: 2160}, Hardware{Device: "/dev/dri/renderD128"}); err == nil {
		t.Error("no vendor: no error")
	}

	// AMD makes SDR copies the same way, and no HDR ones yet.
	amdHW := Hardware{Device: "/dev/dri/renderD128", Vendor: VendorAMD}
	amdArgs, err := TranscodeArgs("in.mov", "out.mp4", Source{Width: 3840, Height: 2160}, amdHW)
	if err != nil || !strings.Contains(strings.Join(amdArgs, " "), "-vf scale_vaapi=w=1920:h=1080:format=nv12 -c:v h264_vaapi") {
		t.Errorf("AMD SDR: %v, %v", amdArgs, err)
	}
	if _, err := TranscodeArgs("in.mov", "out.mp4", Source{HDR: true, Width: 3840, Height: 2160}, amdHW); err == nil {
		t.Error("AMD HDR: no error")
	}
}

func TestParseVAInfo(t *testing.T) {
	const head = "vainfo: Driver version: Intel iHD driver for Intel(R) Gen Graphics - 25.4.6 ()\n" +
		"vainfo: Supported profile and entrypoints\n" +
		"      VAProfileH264High               :\tVAEntrypointVLD\n"
	full, err := parseVAInfo("/dev/dri/renderD128", head+
		"      VAProfileH264High               :\tVAEntrypointEncSlice\n"+
		"      VAProfileH264High               :\tVAEntrypointEncSliceLP\n")
	if err != nil || full.LowPower || !strings.Contains(full.Driver, "25.4.6") || full.Vendor != VendorIntel {
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
	radeon, err := parseVAInfo("/dev/dri/renderD128",
		"vainfo: Driver version: Mesa Gallium driver 25.0.7 for AMD Radeon Graphics (radeonsi, gfx1152, LLVM 19.1.7, DRM 3.61, 6.12.0)\n"+
			"      VAProfileH264High               :\tVAEntrypointEncSlice\n")
	if err != nil || radeon.Vendor != VendorAMD || radeon.LowPower {
		t.Errorf("AMD driver: %+v, %v", radeon, err)
	}
	if _, err := parseVAInfo("/dev/dri/renderD128",
		"vainfo: Driver version: Splitted-Desktop Systems VDPAU backend for VA-API - 0.7.4\n"+
			"      VAProfileH264High               :\tVAEntrypointEncSlice\n"); err == nil {
		t.Error("unknown driver: no error")
	}
}
