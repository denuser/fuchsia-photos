// Package metadata reads the metadata of photos and videos with exiftool.
package metadata

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
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
	// MotionTag names the tag that holds the video of a motion photo, empty
	// when the photo has none.
	MotionTag string
	// ContentIdentifier is the tag that the photo and the video of an iPhone
	// Live Photo share, empty for other files.
	ContentIdentifier string
}

// Tags that hold the video embedded in a motion photo: Google's format, which
// other makers use too, Samsung's own, and Google's older MicroVideo, which
// gives only how far from the end of the file the video starts.
var motionTags = []string{"MotionPhotoVideo", "EmbeddedVideoFile", microVideoOffset}

// HuaweiLiveTrailer names Huawei's and Honor's moving pictures, which carry
// no tag: the video follows the JPEG, and the file ends with text fields of
// 20 bytes, the last of them "LIVE_" and the length of what the fields
// follow. Read finds it by the end of the file, so photos processed before
// it was supported get no motion from the backfill.
const HuaweiLiveTrailer = "HuaweiLiveTrailer"

// exiftool does not extract the video of a MicroVideo photo (Pixel 2 and 3
// and other phones up to about 2019), so ExtractMotion cuts it from the end
// of the file.
const microVideoOffset = "MicroVideoOffset"

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

// Read returns the metadata of the photo or the video at path.
func (r *Reader) Read(path string, video bool) (Metadata, error) {
	file := r.tool.ExtractMetadata(path)[0]
	if file.Err != nil {
		return Metadata{}, fmt.Errorf("read metadata of %s: %w", path, file.Err)
	}
	meta, err := fromFields(file.Fields, video)
	if err == nil && !video && meta.MotionTag == "" {
		if _, err := huaweiVideo(path); err == nil {
			meta.MotionTag = HuaweiLiveTrailer
		}
	}
	return meta, err
}

// MotionTag names the tag of stored tags that holds the video of a motion
// photo, as Read does, or is empty.
func MotionTag(tags []byte) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(tags, &fields) != nil {
		return ""
	}
	for _, tag := range motionTags {
		if _, ok := fields[tag]; ok {
			return tag
		}
	}
	return ""
}

func fromFields(fields map[string]any, video bool) (Metadata, error) {
	for _, tag := range fileTags {
		delete(fields, tag)
	}
	meta := Metadata{TakenAt: takenAt(fields, video)}
	if id, ok := fields["ContentIdentifier"].(string); ok {
		meta.ContentIdentifier = strings.TrimSpace(id)
	}
	if !video {
		for _, tag := range motionTags {
			// Without -b, exiftool describes binary data instead of returning it.
			if _, ok := fields[tag]; ok {
				meta.MotionTag = tag
				break
			}
		}
	}
	tags, err := json.Marshal(stripNUL(fields))
	if err != nil {
		return Metadata{}, fmt.Errorf("encode metadata: %w", err)
	}
	meta.Tags = tags
	return meta, nil
}

// ExtractMotion returns the video embedded in a motion photo, from the tag
// Read named. It runs exiftool of its own, as the long-running one returns
// tags as JSON through a buffer too small for a video.
func ExtractMotion(ctx context.Context, path, tag string) ([]byte, error) {
	if tag == HuaweiLiveTrailer {
		data, err := huaweiVideo(path)
		if err != nil {
			return nil, fmt.Errorf("extract motion video of %s: %w", path, err)
		}
		return data, nil
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "exiftool", "-b", "-"+tag, path)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("extract motion video of %s: %w: %s", path, err, strings.TrimSpace(stderr.String()))
	}
	data := stdout.Bytes()
	if tag == microVideoOffset {
		var err error
		if data, err = fileTail(path, strings.TrimSpace(string(data))); err != nil {
			return nil, fmt.Errorf("extract motion video of %s: %w", path, err)
		}
	}
	// Every MP4 starts with an ftyp box.
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		return nil, fmt.Errorf("extract motion video of %s: %s holds no MP4", path, tag)
	}
	return data, nil
}

// fileTail reads the last length bytes of the file at path.
func fileTail(path, length string) ([]byte, error) {
	n, err := strconv.ParseInt(length, 10, 64)
	if err != nil || n <= 0 {
		return nil, fmt.Errorf("%s %q is not a length", microVideoOffset, length)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if n > info.Size() {
		return nil, fmt.Errorf("%s %d is beyond the start of the file", microVideoOffset, n)
	}
	data := make([]byte, n)
	if _, err := f.ReadAt(data, info.Size()-n); err != nil {
		return nil, err
	}
	return data, nil
}

// huaweiVideo returns the video before the trailer of a Huawei or Honor
// moving picture. A tablet from 2018 writes two fields ("540:540" and
// "LIVE_" with the video's length); later writers add a field between the
// video and those two and count it in the length, so the video ends where
// its last MP4 box does.
func huaweiVideo(path string) ([]byte, error) {
	const field, fields = 20, 2
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	trailerAt := info.Size() - field*fields
	if trailerAt <= 0 {
		return nil, errors.New("no moving picture trailer")
	}
	last := make([]byte, field)
	if _, err := f.ReadAt(last, info.Size()-field); err != nil {
		return nil, err
	}
	text := strings.TrimRight(string(last), " \x00")
	length, ok := strings.CutPrefix(text, "LIVE_")
	if !ok {
		return nil, errors.New("no moving picture trailer")
	}
	n, err := strconv.ParseInt(length, 10, 64)
	if err != nil || n <= 0 || n > trailerAt {
		return nil, fmt.Errorf("moving picture length %q is not a length", length)
	}
	data := make([]byte, n)
	if _, err := f.ReadAt(data, trailerAt-n); err != nil {
		return nil, err
	}
	end := mp4Length(data)
	if end == 0 {
		return nil, errors.New("the moving picture holds no MP4")
	}
	return data[:end], nil
}

// mp4Length returns how many bytes at the start of data are whole top-level
// MP4 boxes, the first of them ftyp, or 0.
func mp4Length(data []byte) int {
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		return 0
	}
	pos := 0
	for len(data)-pos >= 8 {
		size := uint64(binary.BigEndian.Uint32(data[pos:]))
		header := uint64(8)
		switch size {
		case 0: // The box runs to the end.
			return len(data)
		case 1: // A 64-bit size follows the type.
			if len(data)-pos < 16 {
				return pos
			}
			size, header = binary.BigEndian.Uint64(data[pos+8:]), 16
		}
		if size < header || size > uint64(len(data)-pos) {
			return pos
		}
		pos += int(size)
	}
	return pos
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

var (
	photoDates = []struct{ date, offset string }{
		{"DateTimeOriginal", "OffsetTimeOriginal"},
		{"CreateDate", "OffsetTimeDigitized"},
	}
	// An iPhone writes CreationDate with the local offset. QuickTime's own
	// CreateDate has none and is in UTC by the standard, which is how
	// parseDate reads a date without an offset, so exiftool's QuickTimeUTC
	// option is not needed.
	videoDates = []struct{ date, offset string }{
		{"DateTimeOriginal", ""},
		{"CreationDate", ""},
		{"CreateDate", ""},
	}
)

func takenAt(fields map[string]any, video bool) *time.Time {
	dates := photoDates
	if video {
		dates = videoDates
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
	Lens                 *string  `json:"lens,omitempty"` // filled from the photos.lens column, not the tags
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
