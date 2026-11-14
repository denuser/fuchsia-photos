package vaapi

import (
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestFiltersKnownToFFmpeg checks the options of every filter in the copies'
// filter graphs against the ffmpeg the transcoder ships: an option or a named
// value that build does not know fails every copy at start, which no
// argument test catches. The test image carries the Intel build as
// ffmpeg-intel; elsewhere filters the local ffmpeg lacks are skipped.
func TestFiltersKnownToFFmpeg(t *testing.T) {
	bin := os.Getenv("FFMPEG_INTEL")
	if bin == "" {
		bin = "ffmpeg-intel"
	}
	if _, err := exec.LookPath(bin); err != nil {
		bin = "ffmpeg"
		if _, err := exec.LookPath(bin); err != nil {
			t.Skip("no ffmpeg")
		}
	}

	hw := Hardware{Device: "/dev/dri/renderD128", Vendor: VendorIntel}
	for _, hdr := range []bool{false, true} {
		args, err := TranscodeArgs("in.mov", "out.mp4", Source{HDR: hdr, Width: 3840, Height: 2160}, hw)
		if err != nil {
			t.Fatal(err)
		}
		graph := args[slices.Index(args, "-vf")+1]
		for f := range strings.SplitSeq(graph, ",") {
			name, opts, _ := strings.Cut(f, "=")
			known, ok := filterOptions(t, bin, name)
			if !ok {
				if bin == "ffmpeg-intel" {
					t.Errorf("%s has no filter %s", bin, name)
				} else {
					t.Logf("%s has no filter %s, skipped", bin, name)
				}
				continue
			}
			if opts == "" {
				continue
			}
			for opt := range strings.SplitSeq(opts, ":") {
				key, value, named := strings.Cut(opt, "=")
				if !named {
					// A positional value, such as format=vaapi.
					continue
				}
				consts, ok := known[key]
				if !ok {
					t.Errorf("%s: %s has no option %q", bin, name, key)
					continue
				}
				if len(consts) == 0 {
					continue
				}
				for v := range strings.SplitSeq(value, "+") {
					if _, err := strconv.Atoi(v); err != nil && !slices.Contains(consts, v) {
						t.Errorf("%s: %s option %s takes %v, not %q", bin, name, key, consts, v)
					}
				}
			}
		}
	}
}

var (
	optionLine = regexp.MustCompile(`^   (\w+)\s+<\w+>`)
	constLine  = regexp.MustCompile(`^     (\w+)\s`)
)

// filterOptions lists a filter's options with their named values, as
// `ffmpeg -h filter=` prints them, and false if ffmpeg lacks the filter.
func filterOptions(t *testing.T, bin, filter string) (map[string][]string, bool) {
	t.Helper()
	out, err := exec.Command(bin, "-hide_banner", "-h", "filter="+filter).CombinedOutput()
	if err != nil {
		t.Fatalf("%s -h filter=%s: %v: %s", bin, filter, err, out)
	}
	text := string(out)
	if !strings.HasPrefix(text, "Filter "+filter) {
		return nil, false
	}
	opts := map[string][]string{}
	current := ""
	for line := range strings.SplitSeq(text, "\n") {
		if m := optionLine.FindStringSubmatch(line); m != nil {
			current = m[1]
			opts[current] = nil
		} else if m := constLine.FindStringSubmatch(line); m != nil && current != "" {
			opts[current] = append(opts[current], m[1])
		} else if !strings.HasPrefix(line, " ") {
			current = ""
		}
	}
	return opts, true
}
