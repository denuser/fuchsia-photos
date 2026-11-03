// Package metadata reads photo metadata with exiftool.
package metadata

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/barasher/go-exiftool"
)

// Tags about the file on disk rather than the photo.
var fileTags = []string{
	"SourceFile", "ExifToolVersion", "Directory", "FileName", "FileModifyDate",
	"FileAccessDate", "FileInodeChangeDate", "FilePermissions",
}

// Metadata is what the application keeps from a photo's tags.
type Metadata struct {
	Tags    []byte // JSON
	TakenAt *time.Time
}

// Reader keeps one exiftool process running, because starting Perl for every
// photo would be slow. It handles one photo at a time.
type Reader struct {
	tool *exiftool.Exiftool
}

// NewReader starts exiftool.
func NewReader() (*Reader, error) {
	const bufferSize = 4 << 20
	tool, err := exiftool.NewExiftool(
		exiftool.NoPrintConversion(),
		exiftool.Buffer(make([]byte, bufferSize), bufferSize),
	)
	if err != nil {
		return nil, fmt.Errorf("start exiftool: %w", err)
	}
	return &Reader{tool: tool}, nil
}

// Close stops exiftool.
func (r *Reader) Close() error {
	return r.tool.Close()
}

// Read returns the metadata of the photo at path.
func (r *Reader) Read(path string) (Metadata, error) {
	file := r.tool.ExtractMetadata(path)[0]
	if file.Err != nil {
		return Metadata{}, fmt.Errorf("read metadata of %s: %w", path, file.Err)
	}
	return fromFields(file.Fields)
}

func fromFields(fields map[string]any) (Metadata, error) {
	for _, tag := range fileTags {
		delete(fields, tag)
	}
	tags, err := json.Marshal(stripNUL(fields))
	if err != nil {
		return Metadata{}, fmt.Errorf("encode metadata: %w", err)
	}
	return Metadata{Tags: tags, TakenAt: takenAt(fields)}, nil
}

// stripNUL removes NUL characters, which some cameras write and jsonb rejects.
func stripNUL(v any) any {
	switch v := v.(type) {
	case string:
		return strings.ReplaceAll(v, "\x00", "")
	case []any:
		for i := range v {
			v[i] = stripNUL(v[i])
		}
	case map[string]any:
		for k := range v {
			v[k] = stripNUL(v[k])
		}
	}
	return v
}

func takenAt(fields map[string]any) *time.Time {
	dates := []struct{ date, offset string }{
		{"DateTimeOriginal", "OffsetTimeOriginal"},
		{"CreateDate", "OffsetTimeDigitized"},
	}
	for _, d := range dates {
		value, _ := fields[d.date].(string)
		offset, _ := fields[d.offset].(string)
		if t, ok := parseDate(value, offset); ok {
			return &t
		}
	}
	return nil
}

// parseDate parses an EXIF date. Dates without an offset are taken as UTC,
// because the camera's time zone is unknown.
func parseDate(value, offset string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse("2006:01:02 15:04:05.999999999Z07:00", value); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006:01:02 15:04:05.999999999Z07:00", value+strings.TrimSpace(offset)); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006:01:02 15:04:05.999999999", value); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// Camera holds the shooting details of a photo. It leaves out the location
// and anything else that could identify a place or a person, so it can go to
// everyone who may see the photo.
type Camera struct {
	Make                 *string  `json:"make,omitempty"`
	Model                *string  `json:"model,omitempty"`
	Lens                 *string  `json:"lens,omitempty"`
	FocalLength          *float64 `json:"focalLength,omitempty"`
	FocalLength35mm      *float64 `json:"focalLength35mm,omitempty"`
	FNumber              *float64 `json:"fNumber,omitempty"`
	ExposureTime         *float64 `json:"exposureTime,omitempty"`
	ISO                  *float64 `json:"iso,omitempty"`
	ExposureCompensation *float64 `json:"exposureCompensation,omitempty"`
	Flash                *bool    `json:"flash,omitempty"`
}

// Location is where a photo was taken, in degrees. South and west are
// negative.
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Details picks the camera details and the location out of tags stored by
// Read. Each is nil when the tags do not hold it.
func Details(tags []byte) (*Camera, *Location) {
	if len(tags) == 0 {
		return nil, nil
	}
	var fields map[string]any
	if err := json.Unmarshal(tags, &fields); err != nil {
		return nil, nil
	}
	return camera(fields), location(fields)
}

func camera(fields map[string]any) *Camera {
	c := Camera{
		Make:                 text(fields["Make"]),
		Model:                text(fields["Model"]),
		Lens:                 text(fields["LensModel"]),
		FocalLength:          positive(fields["FocalLength"]),
		FocalLength35mm:      positive(fields["FocalLengthIn35mmFormat"]),
		FNumber:              positive(fields["FNumber"]),
		ExposureTime:         positive(fields["ExposureTime"]),
		ISO:                  positive(fields["ISO"]),
		ExposureCompensation: number(fields["ExposureCompensation"]),
	}
	// Bit 0 of the EXIF flash value tells whether the flash fired.
	if flash := number(fields["Flash"]); flash != nil {
		fired := int(*flash)&1 == 1
		c.Flash = &fired
	}
	if c == (Camera{}) {
		return nil
	}
	return &c
}

func location(fields map[string]any) *Location {
	lat, lon := number(fields["GPSLatitude"]), number(fields["GPSLongitude"])
	if lat == nil || lon == nil || math.Abs(*lat) > 90 || math.Abs(*lon) > 180 {
		return nil
	}
	l := Location{Latitude: *lat, Longitude: *lon}
	// exiftool signs the coordinates itself, but a reference letter that
	// disagrees with the sign wins, in case only the raw tag was kept.
	if ref, _ := fields["GPSLatitudeRef"].(string); ref == "S" && l.Latitude > 0 {
		l.Latitude = -l.Latitude
	}
	if ref, _ := fields["GPSLongitudeRef"].(string); ref == "W" && l.Longitude > 0 {
		l.Longitude = -l.Longitude
	}
	// Some cameras write zeros when they have no fix.
	if l == (Location{}) {
		return nil
	}
	return &l
}

func text(v any) *string {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// number reads a numeric tag. Some cameras write a list, such as two ISO
// values, which exiftool returns as a string; the first value is used.
func number(v any) *float64 {
	switch v := v.(type) {
	case float64:
		return &v
	case string:
		fields := strings.Fields(v)
		if len(fields) == 0 {
			return nil
		}
		if n, err := strconv.ParseFloat(fields[0], 64); err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
			return &n
		}
	}
	return nil
}

func positive(v any) *float64 {
	if n := number(v); n != nil && *n > 0 {
		return n
	}
	return nil
}
