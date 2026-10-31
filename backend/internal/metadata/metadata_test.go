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
