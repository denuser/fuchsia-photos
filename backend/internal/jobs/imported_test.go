package jobs

import (
	"testing"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

func TestImportedDetails(t *testing.T) {
	imported := time.Date(2017, 7, 4, 10, 58, 10, 0, time.UTC)
	lat, lon := 41.7, 44.8
	local := []byte(`{"DateTimeOriginal": "2017:07:04 13:58:10"}`)
	zoned := []byte(`{"DateTimeOriginal": "2017:07:04 13:58:10", "OffsetTimeOriginal": "+03:00"}`)
	withGPS := []byte(`{"DateTimeOriginal": "2017:07:04 13:58:10", "GPSLatitude": 52.4, "GPSLongitude": 4.8}`)
	other := []byte(`{"DateTimeOriginal": "2017:07:05 09:00:00"}`)
	mime := "image/jpeg"

	tests := []struct {
		name         string
		row          store.ListImportedForApplyRow
		wantTakenAt  time.Time
		wantLocation bool
	}{
		{"a date without an offset gives way", store.ListImportedForApplyRow{Mime: mime, Exif: local, TakenAt: &imported, Latitude: &lat, Longitude: &lon}, imported, true},
		{"a date with an offset stays", store.ListImportedForApplyRow{Mime: mime, Exif: zoned, TakenAt: &imported}, time.Date(2017, 7, 4, 10, 58, 10, 0, time.UTC), false},
		{"no date takes the import's", store.ListImportedForApplyRow{Mime: mime, Exif: []byte(`{}`), TakenAt: &imported}, imported, false},
		{"the file's location stays", store.ListImportedForApplyRow{Mime: mime, Exif: withGPS, TakenAt: &imported, Latitude: &lat, Longitude: &lon}, imported, false},
		{"an edited copy with its original's date", store.ListImportedForApplyRow{Mime: mime, Exif: local, TakenAt: &imported, Latitude: &lat, Longitude: &lon, Edited: true, OriginalMime: &mime, OriginalExif: local}, imported, true},
		{"an edited copy with another date", store.ListImportedForApplyRow{Mime: mime, Exif: local, TakenAt: &imported, Latitude: &lat, Longitude: &lon, Edited: true, OriginalMime: &mime, OriginalExif: other}, time.Date(2017, 7, 4, 13, 58, 10, 0, time.UTC), false},
		{"an edited copy of an original not processed yet", store.ListImportedForApplyRow{Mime: mime, Exif: local, TakenAt: &imported, Edited: true}, time.Date(2017, 7, 4, 13, 58, 10, 0, time.UTC), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			takenAt, location := importedDetails(tt.row)
			if takenAt == nil || !takenAt.Equal(tt.wantTakenAt) {
				t.Errorf("taken at = %v, want %v", takenAt, tt.wantTakenAt)
			}
			if (location != nil) != tt.wantLocation || (location != nil && (location.Latitude != lat || location.Longitude != lon)) {
				t.Errorf("location = %+v, want one: %v", location, tt.wantLocation)
			}
		})
	}
}
