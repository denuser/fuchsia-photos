package metadata

import (
	"encoding/json"
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
	})
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
	if *camera.Model != "Canon EOS R5" || *camera.Lens != "RF24-70mm F2.8 L IS USM" {
		t.Errorf("model = %q, lens = %q", *camera.Model, *camera.Lens)
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
