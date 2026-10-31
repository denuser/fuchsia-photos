// Package metadata reads photo metadata with exiftool.
package metadata

import (
	"encoding/json"
	"fmt"
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
