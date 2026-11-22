package jobs

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/magellanica-app/magellanica-photos/backend/internal/metadata"
	"github.com/magellanica-app/magellanica-photos/backend/internal/storage"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

// ApplyImportedMetadata works out the date and the location of processed
// photos that came with metadata from an import, and of their edited copies,
// from their EXIF and that metadata:
//   - an EXIF date with its offset stays; a date without one, which the
//     application reads as UTC, or no date gives way to the time the import
//     knew, which is the real moment;
//   - the file's own location stays; the import's fills a file without one;
//   - an edited copy takes its original's metadata only when both files have
//     the same EXIF date, so that a file whose name only looks like a copy's
//     gets nothing.
//
// Processing calls it after each photo, and an import after matching, so the
// result does not depend on which of them comes first.
func ApplyImportedMetadata(ctx context.Context, queries *store.Queries, ids []int64) error {
	rows, err := queries.ListImportedForApply(ctx, ids)
	if err != nil {
		return fmt.Errorf("list imported metadata: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	params := store.SetImportedDetailsParams{}
	for _, row := range rows {
		takenAt, location := importedDetails(row)
		params.Ids = append(params.Ids, row.PhotoID)
		var at time.Time // The zero time stands for none.
		if takenAt != nil {
			at = *takenAt
		}
		params.TakenAts = append(params.TakenAts, at)
		lat, lon := math.NaN(), math.NaN()
		if location != nil {
			lat, lon = location.Latitude, location.Longitude
		}
		params.Latitudes = append(params.Latitudes, lat)
		params.Longitudes = append(params.Longitudes, lon)
	}
	if err := queries.SetImportedDetails(ctx, params); err != nil {
		return fmt.Errorf("set imported details: %w", err)
	}
	return nil
}

func importedDetails(row store.ListImportedForApplyRow) (*time.Time, *metadata.Location) {
	own := metadata.TakenAt(row.Exif, storage.IsVideo(row.Mime))
	applies := true
	if row.Edited {
		originalMime := ""
		if row.OriginalMime != nil {
			originalMime = *row.OriginalMime
		}
		applies = sameTakenAt(row.Exif, row.Mime, row.OriginalExif, originalMime)
	}
	if !applies {
		return own.At, nil
	}

	takenAt := own.At
	if !own.Zoned && row.TakenAt != nil {
		takenAt = row.TakenAt
	}
	var location *metadata.Location
	if _, fromFile := metadata.Details(row.Exif); fromFile == nil && row.Latitude != nil && row.Longitude != nil {
		location = &metadata.Location{Latitude: *row.Latitude, Longitude: *row.Longitude}
	}
	return takenAt, location
}

// sameTakenAt tells whether an edited copy and its original have the same
// date in their EXIF, as written there. A file without a date matches none.
func sameTakenAt(exif []byte, mime string, originalExif []byte, originalMime string) bool {
	own := metadata.TakenAt(exif, storage.IsVideo(mime))
	original := metadata.TakenAt(originalExif, storage.IsVideo(originalMime))
	return own.Raw != "" && own.Raw == original.Raw
}
