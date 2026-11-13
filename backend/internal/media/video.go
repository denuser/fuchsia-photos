package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"

	"github.com/davidbyttow/govips/v2/vips"
)

// Video is what processing needs to know about a video file.
type Video struct {
	// Duration in seconds, or zero when the file does not say.
	Duration float64
	// HDR means the video uses an HDR transfer function, such as the HLG of
	// an iPhone, whose frames look grey unless they are tone mapped.
	HDR bool
	// Codec is ffprobe's name of the video codec, such as h264 or hevc.
	Codec string
	// Width and Height are the coded size, before any rotation.
	Width, Height int
}

// CheckVideoTools reports whether ffmpeg and ffprobe can be found. Without
// them, videos wait in the queue until the job gives up.
func CheckVideoTools() error {
	for _, tool := range []string{"ffprobe", "ffmpeg"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s: %w", tool, err)
		}
	}
	return nil
}

// ToolMissing tells whether err comes from ffmpeg or ffprobe not being
// installed, which a retry after installing them can fix, unlike a broken file.
func ToolMissing(err error) bool {
	return errors.Is(err, exec.ErrNotFound)
}

// ProbeVideo reads the duration, codec, size and colour transfer of the video
// at path.
func ProbeVideo(ctx context.Context, path string) (Video, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name,width,height,color_transfer:format=duration",
		"-of", "json", path)
	out, err := run(cmd)
	if err != nil {
		return Video{}, fmt.Errorf("probe %s: %w", path, err)
	}
	var probe struct {
		Streams []struct {
			CodecName     string `json:"codec_name"`
			Width         int    `json:"width"`
			Height        int    `json:"height"`
			ColorTransfer string `json:"color_transfer"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return Video{}, fmt.Errorf("probe %s: %w", path, err)
	}
	if len(probe.Streams) == 0 {
		return Video{}, fmt.Errorf("probe %s: no video stream", path)
	}
	stream := probe.Streams[0]
	video := Video{Codec: stream.CodecName, Width: stream.Width, Height: stream.Height}
	switch stream.ColorTransfer {
	case "arib-std-b67", "smpte2084":
		video.HDR = true
	}
	if d, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil && d > 0 {
		video.Duration = d
	}
	return video, nil
}

// toneMap turns HDR frames into SDR with zscale, which Debian's ffmpeg has
// and some other builds, such as Homebrew's, do not.
const toneMap = "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709," +
	"tonemap=tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,format=yuv420p"

// Poster returns a PNG of the frame that stands for the video: one second in,
// or the middle of a shorter video, upright as it plays.
func Poster(ctx context.Context, path string, video Video) ([]byte, error) {
	at := min(1, video.Duration/2)
	if video.HDR {
		frame, err := frame(ctx, path, at, toneMap)
		if err == nil || ToolMissing(err) {
			return frame, err
		}
		slog.Warn("tone map video poster, using the frame as it is", "path", path, "error", err)
	}
	return frame(ctx, path, at, "")
}

func frame(ctx context.Context, path string, at float64, filter string) ([]byte, error) {
	args := []string{"-v", "error", "-ss", strconv.FormatFloat(at, 'f', 3, 64), "-i", path, "-frames:v", "1"}
	if filter != "" {
		args = append(args, "-vf", filter)
	}
	args = append(args, "-f", "image2", "-c:v", "png", "pipe:1")
	out, err := run(exec.CommandContext(ctx, "ffmpeg", args...))
	if err != nil {
		return nil, fmt.Errorf("extract a frame of %s: %w", path, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("extract a frame of %s: no frame", path)
	}
	return out, nil
}

func run(cmd *exec.Cmd) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, lastLine(msg))
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

func lastLine(s string) string {
	return s[strings.LastIndexByte(s, '\n')+1:]
}

// RenderVideo makes the previews of a video from its poster frame, and
// returns the size of the video as it plays.
func RenderVideo(ctx context.Context, path string, video Video, settings Settings) (previews Previews, width, height int, err error) {
	poster, err := Poster(ctx, path, video)
	if err != nil {
		return Previews{}, 0, 0, err
	}
	image, err := vips.NewImageFromBuffer(poster)
	if err != nil {
		return Previews{}, 0, 0, fmt.Errorf("load poster of %s: %w", path, err)
	}
	width, height = image.Width(), image.Height()
	image.Close()

	previews, err = render(poster, settings)
	if err != nil {
		return Previews{}, 0, 0, fmt.Errorf("previews of %s: %w", path, err)
	}
	return previews, width, height, nil
}
