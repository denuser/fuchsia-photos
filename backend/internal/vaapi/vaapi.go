// Package vaapi makes H.264 copies of videos on graphics that encode H.264
// through VAAPI. It runs in the transcoder, a service of its own whose image
// carries the drivers of one maker of graphics, and uses no cgo, so that the
// transcoder needs no libraries but ffmpeg's.
package vaapi

import (
	"bytes"
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
	Device string `json:"device"`
	// LowPower tells h264_vaapi to use the low-power encoder, the only one
	// some Intel graphics offer for H.264.
	LowPower bool `json:"lowPower"`
	// Driver is the driver's own description, for the administration page.
	Driver string `json:"driver"`
	// Vendor names the maker of the graphics, such as VendorIntel, which
	// decides how HDR is tone mapped.
	Vendor string `json:"vendor"`
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
		return Hardware{}, errors.New("this ffmpeg has no h264_vaapi encoder; the transcoder needs its image for the graphics")
	}
	cmd := exec.CommandContext(ctx, "vainfo", "--display", "drm", "--device", device)
	// vainfo prints the profiles to stdout and its errors to stderr; both are
	// needed to tell what went wrong.
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Hardware{}, fmt.Errorf("vainfo on %s: %w: %s", device, err, lastLines(strings.TrimSpace(string(out)), 1))
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
	v, ok := vendorByDriver(hw.Driver)
	if !ok {
		return Hardware{}, fmt.Errorf("the driver on %s is not supported: %q", device, hw.Driver)
	}
	hw.Vendor = v.name
	switch {
	case encSlice.MatchString(out):
	case encSliceLP.MatchString(out):
		hw.LowPower = true
	default:
		return Hardware{}, fmt.Errorf("the graphics on %s offer no H.264 encoding", device)
	}
	return hw, nil
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

// Source is what the copy of a video depends on, as the application probed it.
type Source struct {
	Width  int  `json:"width"`
	Height int  `json:"height"`
	HDR    bool `json:"hdr"`
}

// TranscodeArgs returns the ffmpeg arguments that make the H.264 copy of the
// video at in, written to out as MP4. Decoding, scaling and encoding all run
// on the graphics, and HDR is tone mapped as the vendor of the graphics does
// it. The video is not rotated: its display matrix goes into the copy,
// which plays upright as the original does. The copy carries no metadata
// tags, so that no location goes with it.
func TranscodeArgs(in, out string, video Source, hw Hardware) ([]string, error) {
	v, ok := vendorByName(hw.Vendor)
	if !ok {
		return nil, fmt.Errorf("unknown graphics vendor %q", hw.Vendor)
	}
	width, height := CopySize(video.Width, video.Height)
	size := "w=" + strconv.Itoa(width) + ":h=" + strconv.Itoa(height)
	args := []string{"-hide_banner", "-nostdin", "-y", "-v", "error", "-noautorotate",
		"-init_hw_device", "vaapi=va:" + hw.Device}
	filter := "scale_vaapi=" + size + ":format=nv12"
	if video.HDR {
		if v.toneMap == nil {
			return nil, fmt.Errorf("copies of HDR videos are not made on %s graphics yet", v.name)
		}
		var options []string
		options, filter = v.toneMap(size)
		args = append(args, options...)
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
		"-movflags", "+faststart", "-f", "mp4", out), nil
}

// Transcode makes the H.264 copy of the video at in and writes it to out.
func Transcode(ctx context.Context, in, out string, video Source, hw Hardware) error {
	args, err := TranscodeArgs(in, out, video, hw)
	if err != nil {
		return err
	}
	if _, err := run(exec.CommandContext(ctx, "ffmpeg", args...)); err != nil {
		return fmt.Errorf("transcode %s: %w", in, err)
	}
	return nil
}

// run runs a command and returns its output, or an error with the last lines
// it wrote to stderr: ffmpeg ends with a general line, such as "Error opening
// output files", and names the cause in the lines before it.
func run(cmd *exec.Cmd) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, lastLines(msg, 5))
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

// lastLines returns the last n lines of s, joined by " | " to stay on one
// line of the log.
func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.Join(lines, " | ")
}
