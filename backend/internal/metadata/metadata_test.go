package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	tests := []struct {
		value, offset string
		want          string
	}{
		{"2024:06:01 12:30:45", "", "2024-06-01T12:30:45Z"},
		{"2024:06:01 12:30:45", "+03:00", "2024-06-01T09:30:45Z"},
		{"2024:06:01 12:30:45+03:00", "", "2024-06-01T09:30:45Z"},
		{"2024:06:01 12:30:45.250", "", "2024-06-01T12:30:45.25Z"},
		{"0000:00:00 00:00:00", "", ""},
		{"    :  :     :  :  ", "", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		got, ok := parseDate(tt.value, tt.offset)
		if tt.want == "" {
			if ok {
				t.Errorf("parseDate(%q) = %v, want no date", tt.value, got)
			}
			continue
		}
		if !ok || got.UTC().Format(time.RFC3339Nano) != tt.want {
			t.Errorf("parseDate(%q, %q) = %v, want %s", tt.value, tt.offset, got, tt.want)
		}
	}
}

func TestFromFields(t *testing.T) {
	got, err := fromFields(map[string]any{
		"Directory":        "/data/library",
		"Model":            "Camera\x00",
		"DateTimeOriginal": "2024:06:01 12:30:45",
		"List":             []any{"a\x00"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got.Tags), "/data/library") {
		t.Error("file system tags were kept")
	}
	var tags map[string]any
	if err := json.Unmarshal(got.Tags, &tags); err != nil {
		t.Fatal(err)
	}
	if tags["Model"] != "Camera" {
		t.Errorf("Model = %q", tags["Model"])
	}
	if tags["List"].([]any)[0] != "a" {
		t.Errorf("List = %v", tags["List"])
	}
	if got.TakenAt == nil {
		t.Error("TakenAt is not set")
	}
}

func TestVideoDates(t *testing.T) {
	tests := []struct {
		fields map[string]any
		want   string
	}{
		// An iPhone: CreationDate has the offset, CreateDate is UTC.
		{map[string]any{"CreationDate": "2024:05:01 12:00:00+02:00", "CreateDate": "2024:05:01 10:00:00"}, "2024-05-01T10:00:00Z"},
		// Most Android phones write only CreateDate, in UTC.
		{map[string]any{"CreateDate": "2024:06:01 08:00:00"}, "2024-06-01T08:00:00Z"},
		{map[string]any{"CreateDate": "0000:00:00 00:00:00"}, ""},
	}
	for _, tt := range tests {
		got, err := fromFields(tt.fields, true)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case tt.want == "" && got.TakenAt != nil:
			t.Errorf("%v: TakenAt = %v, want none", tt.fields, got.TakenAt)
		case tt.want != "" && (got.TakenAt == nil || got.TakenAt.UTC().Format(time.RFC3339) != tt.want):
			t.Errorf("%v: TakenAt = %v, want %s", tt.fields, got.TakenAt, tt.want)
		}
	}
}

func TestMotionTag(t *testing.T) {
	for _, tag := range motionTags {
		got, err := fromFields(map[string]any{tag: "(Binary data 2860 bytes, use -b option to extract)"}, false)
		if err != nil {
			t.Fatal(err)
		}
		if got.MotionTag != tag {
			t.Errorf("MotionTag = %q, want %q", got.MotionTag, tag)
		}
	}
	if got, _ := fromFields(map[string]any{"Model": "x"}, false); got.MotionTag != "" {
		t.Errorf("a plain photo has motion tag %q", got.MotionTag)
	}
}

// The fixtures follow Google's formats, made by media/testdata/generate.sh.
func TestExtractMotion(t *testing.T) {
	reader, err := NewReader()
	if err != nil {
		t.Skip("exiftool is not installed:", err)
	}
	defer reader.Close()
	// trailer is how many bytes follow the video.
	for _, c := range []struct {
		file, tag string
		trailer   int
	}{
		{"motion.jpg", "MotionPhotoVideo", 0},
		{"microvideo.jpg", "MicroVideoOffset", 0},
		{"huawei.jpg", HuaweiLiveTrailer, 40},
	} {
		path := filepath.Join("..", "media", "testdata", c.file)
		meta, err := reader.Read(path, false)
		if err != nil {
			t.Fatal(err)
		}
		if meta.MotionTag != c.tag {
			t.Fatalf("%s: MotionTag = %q, want %q", c.file, meta.MotionTag, c.tag)
		}
		video, err := ExtractMotion(context.Background(), path, meta.MotionTag)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasSuffix(data[:len(data)-c.trailer], video) || string(video[4:8]) != "ftyp" {
			t.Errorf("%s: the extracted video is not the end of the file", c.file)
		}
		// A Huawei moving picture has no tag to store.
		if want := c.tag; MotionTag(meta.Tags) != want && want != HuaweiLiveTrailer {
			t.Errorf("%s: stored tags name %q", c.file, MotionTag(meta.Tags))
		}

		if _, err := ExtractMotion(context.Background(), filepath.Join("..", "media", "testdata", "landscape.jpg"), meta.MotionTag); err == nil {
			t.Errorf("%s: a photo without motion gave a video", c.tag)
		}
	}
}

// An offset that does not lead to an MP4 gives no video.
func TestExtractMicroVideoBadOffset(t *testing.T) {
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool is not installed")
	}
	data, err := os.ReadFile(filepath.Join("..", "media", "testdata", "microvideo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, mangle := range map[string]func([]byte) []byte{
		"short":     func(d []byte) []byte { return append(d, 0, 0, 0, 0) },
		"truncated": func(d []byte) []byte { return d[:len(d)-100] },
	} {
		path := filepath.Join(dir, name+".jpg")
		if err := os.WriteFile(path, mangle(append([]byte(nil), data...)), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ExtractMotion(context.Background(), path, "MicroVideoOffset"); err == nil {
			t.Errorf("%s: a wrong offset gave a video", name)
		}
	}
}

// Later writers put one more field between the video and the two that end
// the file, and count it in the length. Fields that do not hold a length of
// whole MP4 boxes give no video.
func TestHuaweiTrailers(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "media", "testdata", "huawei.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	photo, video := data[:len(data)-40-5400], data[len(data)-40-5400:len(data)-40]
	if string(video[4:8]) != "ftyp" {
		t.Fatal("the fixture's video is not where the trailer says")
	}
	field := func(s string) string { return fmt.Sprintf("%-20s", s) }
	dir := t.TempDir()
	for name, c := range map[string]struct {
		trailer string
		ok      bool
	}{
		"v2":         {field("v2_f00") + field("0:1000") + field(fmt.Sprint("LIVE_", len(video)+20)), true},
		"too long":   {field("540:540") + field(fmt.Sprint("LIVE_", len(data))), false},
		"short":      {field("540:540") + field(fmt.Sprint("LIVE_", len(video)-4)), false},
		"no trailer": {field("540:540") + field("something else"), false},
	} {
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".jpg")
		file := append(append(append([]byte(nil), photo...), video...), c.trailer...)
		if err := os.WriteFile(path, file, 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := ExtractMotion(context.Background(), path, HuaweiLiveTrailer)
		if c.ok && (err != nil || !bytes.Equal(got, video)) {
			t.Errorf("%s: got %d bytes, %v", name, len(got), err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s: gave a video", name)
		}
	}
}

func TestDetails(t *testing.T) {
	tags := []byte(`{
		"Make": "Canon", "Model": " Canon EOS R5 ", "LensModel": "RF24-70mm F2.8 L IS USM",
		"FocalLength": 35, "FocalLengthIn35mmFormat": 35, "FNumber": 2.8,
		"ExposureTime": 0.004, "ISO": "100 0", "ExposureCompensation": -0.7, "Flash": 16,
		"GPSLatitude": 33.5, "GPSLatitudeRef": "S", "GPSLongitude": -70.2, "GPSLongitudeRef": "W"
	}`)
	camera, location := Details(tags)
	if camera == nil {
		t.Fatal("no camera details")
	}
	if *camera.Model != "Canon EOS R5" || camera.Lens != nil {
		t.Errorf("model = %q, lens = %v", *camera.Model, camera.Lens)
	}
	if *camera.FNumber != 2.8 || *camera.ExposureTime != 0.004 || *camera.FocalLength != 35 {
		t.Errorf("exposure = %+v", camera)
	}
	if *camera.ISO != 100 || *camera.ExposureCompensation != -0.7 || *camera.Flash {
		t.Errorf("iso = %v, compensation = %v, flash = %v", *camera.ISO, *camera.ExposureCompensation, *camera.Flash)
	}
	if location == nil || *location != (Location{Latitude: -33.5, Longitude: -70.2}) {
		t.Errorf("location = %+v", location)
	}
}

func TestDetailsMissing(t *testing.T) {
	for _, tags := range []string{``, `{}`, `{"FNumber": 0, "GPSLatitude": 0, "GPSLongitude": 0}`, `{"Make": " ", "GPSLatitude": 59.9}`} {
		if camera, location := Details([]byte(tags)); camera != nil || location != nil {
			t.Errorf("Details(%s) = %+v, %+v", tags, camera, location)
		}
	}
}
