package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Hardware is a render node whose graphics encode H.264 through VAAPI, as
// DetectHardware found it.
type Hardware struct {
	// Device is the render node, such as /dev/dri/renderD128.
	Device string
	// LowPower tells h264_vaapi to use the low-power encoder, the only one
	// some Intel graphics offer for H.264.
	LowPower bool
	// Driver is the driver's own description, for the administration page.
	Driver string
}

var (
	encSlice   = regexp.MustCompile(`(?m)VAProfileH264High\s*:\s*VAEntrypointEncSlice\s*$`)
	encSliceLP = regexp.MustCompile(`(?m)VAProfileH264High\s*:\s*VAEntrypointEncSliceLP\s*$`)
	vaDriver   = regexp.MustCompile(`(?m)Driver version:\s*(.+)$`)
)

// DetectHardware checks that device exists, that ffmpeg has h264_vaapi and
// that vainfo finds an H.264 encoder on it. The error says what is missing.
func DetectHardware(ctx context.Context, device string) (Hardware, error) {
	if _, err := os.Stat(device); err != nil {
		return Hardware{}, fmt.Errorf("no render node: %w", err)
	}
	encoders, err := run(exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-encoders"))
	if err != nil {
		return Hardware{}, fmt.Errorf("list ffmpeg encoders: %w", err)
	}
	if !strings.Contains(string(encoders), " h264_vaapi ") {
		return Hardware{}, errors.New("this ffmpeg has no h264_vaapi encoder; hardware transcoding needs the image for Intel graphics")
	}
	cmd := exec.CommandContext(ctx, "vainfo", "--display", "drm", "--device", device)
	// vainfo prints the profiles to stdout and its errors to stderr; both are
	// needed to tell what went wrong.
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Hardware{}, fmt.Errorf("vainfo on %s: %w: %s", device, err, lastLine(strings.TrimSpace(string(out))))
	}
	return parseVAInfo(device, string(out))
}

// parseVAInfo reads the output of vainfo. Newer Intel graphics, the N150
// among them, may offer H.264 encoding only in the low-power mode.
func parseVAInfo(device, out string) (Hardware, error) {
	hw := Hardware{Device: device}
	if m := vaDriver.FindStringSubmatch(out); m != nil {
		hw.Driver = strings.TrimSpace(m[1])
	}
	switch {
	case encSlice.MatchString(out):
	case encSliceLP.MatchString(out):
		hw.LowPower = true
	default:
		return Hardware{}, fmt.Errorf("the graphics on %s offer no H.264 encoding", device)
	}
	return hw, nil
}

// NeedsCopy tells whether a video needs an H.264 copy to play in every
// browser, and whether the graphics can decode it to make one. H.264 plays
// everywhere. ProRes, MPEG-4 Part 2 and MJPEG would have to be decoded on the
// CPU, which is too slow on a NAS, so they stay as they are.
func NeedsCopy(video Video) bool {
	switch video.Codec {
	case "hevc", "vp9", "av1":
		return video.Width > 0 && video.Height > 0
	}
	return false
}

// CopySize returns the size of the copy: the shorter side at most 1080, both
// sides even, as the encoder needs.
func CopySize(width, height int) (int, int) {
	short := min(width, height)
	if short > 1080 {
		width, height = width*1080/short, height*1080/short
	}
	return width / 2 * 2, height / 2 * 2
}

// TranscodeArgs returns the ffmpeg arguments that make the H.264 copy of the
// video at in, written to out as MP4. Decoding, scaling and encoding all run
// on the graphics. HDR is tone mapped with OpenCL on the same graphics, the
// only way that takes the HLG of an iPhone; frames are shared with VAAPI, not
// copied. The video is not rotated: its display matrix goes into the copy,
// which plays upright as the original does. The copy carries no metadata
// tags, so that no location goes with it.
func TranscodeArgs(in, out string, video Video, hw Hardware) []string {
	width, height := CopySize(video.Width, video.Height)
	size := "w=" + strconv.Itoa(width) + ":h=" + strconv.Itoa(height)
	args := []string{"-hide_banner", "-nostdin", "-y", "-v", "error", "-noautorotate",
		"-init_hw_device", "vaapi=va:" + hw.Device}
	filter := "scale_vaapi=" + size + ":format=nv12"
	if video.HDR {
		args = append(args, "-init_hw_device", "opencl=ocl@va", "-filter_hw_device", "ocl")
		filter = "scale_vaapi=" + size + ":format=p010," +
			"hwmap=derive_device=opencl:mode=read," +
			"tonemap_opencl=format=nv12:t=bt709:m=bt709:p=bt709:tonemap=bt2390:desat=0," +
			"hwmap=derive_device=vaapi:mode=write:reverse=1,format=vaapi"
	}
	args = append(args,
		"-hwaccel", "vaapi", "-hwaccel_device", "va", "-hwaccel_output_format", "vaapi",
		"-i", in,
		"-map", "0:v:0", "-map", "0:a:0?", "-map_metadata", "-1",
		"-vf", filter,
		"-c:v", "h264_vaapi")
	if hw.LowPower {
		args = append(args, "-low_power", "1")
	}
	return append(args,
		"-profile:v", "high", "-rc_mode", "CQP", "-qp", "24",
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart", "-f", "mp4", out)
}

// Transcode makes the H.264 copy of the video at in and writes it to out.
func Transcode(ctx context.Context, in, out string, video Video, hw Hardware) error {
	if _, err := run(exec.CommandContext(ctx, "ffmpeg", TranscodeArgs(in, out, video, hw)...)); err != nil {
		return fmt.Errorf("transcode %s: %w", in, err)
	}
	return nil
}
