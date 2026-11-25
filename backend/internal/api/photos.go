package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	// Time zones are checked against the copy built into the binary, as the
	// image has no time zone database.
	_ "time/tzdata"

	"github.com/jackc/pgx/v5"

	"github.com/magellanica-app/magellanica-photos/backend/internal/ingest"
	"github.com/magellanica-app/magellanica-photos/backend/internal/jobs"
	"github.com/magellanica-app/magellanica-photos/backend/internal/metadata"
	"github.com/magellanica-app/magellanica-photos/backend/internal/storage"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
	"github.com/magellanica-app/magellanica-photos/backend/internal/transcode"
)

const (
	// The largest video, with room for the multipart framing around it.
	// Ingest holds each kind of file to its own limit.
	maxUploadBytes   = ingest.MaxVideoBytes + 1<<20
	defaultPageSize  = 200
	maxPageSize      = 500
	maxBulkPhotos    = 500
	previewCacheRule = "private, max-age=31536000, immutable"
)

// Raw EXIF stays out of this type on purpose: visible photos are not always
// the viewer's own, and EXIF can hold serial numbers and names. Only the camera
// details go out, and the location when an administrator allows it.
type photoJSON struct {
	ID        int64      `json:"id"`
	OwnerID   int64      `json:"ownerId"`
	Status    string     `json:"status"`
	Name      string     `json:"name"`
	Mime      string     `json:"mime"`
	Size      int64      `json:"size"`
	Width     *int32     `json:"width"`
	Height    *int32     `json:"height"`
	TakenAt   *time.Time `json:"takenAt"`
	SortAt    time.Time  `json:"sortAt"`
	CreatedAt time.Time  `json:"createdAt"`
	DeletedAt *time.Time `json:"deletedAt"`
	// Previews are cached for a long time, so their address carries it.
	PreviewVersion int32 `json:"previewVersion"`
	// The length of a video in seconds, null for photos.
	Duration *float64 `json:"duration"`
	// Whether a motion photo or a Live Photo has a video to play.
	HasMotion bool `json:"hasMotion"`
	// Whether the photo is a camera RAW or has one hidden under it.
	HasRAW bool `json:"hasRaw"`
	// The kind of link a photo is hidden under another one with: live for the
	// video of a Live Photo, edited for the original of an edited copy, and
	// raw for a camera RAW paired with a JPEG. Only events carry hidden photos.
	HiddenUnder *string `json:"hiddenUnder"`
	// The versions hidden right under the photo, given only to its owner and
	// only for one photo, as the viewer loads it.
	Versions []versionJSON `json:"versions,omitempty"`
	// Whether the photo is the current user's favorite, and in the user's
	// archive, given only for one photo, as the viewer loads it.
	Favorite *bool `json:"favorite,omitempty"`
	Archived *bool `json:"archived,omitempty"`
	// For a photo read from a library folder, given only for one photo: whether
	// its file cannot be read now, and, to its owner, the file's path under the
	// folders mount.
	FileMissing *bool   `json:"fileMissing,omitempty"`
	SourcePath  *string `json:"sourcePath,omitempty"`
	// The H.264 copy of what the item plays: ready, not_needed, failed, or
	// null while no copy has been tried.
	VideoCopy *string `json:"videoCopy"`
	// Whether a video is HDR, which some browsers play grey: the viewer then
	// waits for the copy. Null for photos and for videos not probed yet.
	HDR *bool `json:"hdr"`

	// The description everyone who can see the photo sees; empty for none.
	Description string `json:"description"`

	Camera   *metadata.Camera   `json:"camera"`
	Location *metadata.Location `json:"location"`
}

// maxDescription is the longest description, in characters.
const maxDescription = 2000

func toPhotoJSON(p store.Photo, showLocation bool) photoJSON {
	camera, location := metadata.Details(p.Exif)
	// The lens comes from the column the timeline filters by, so that the
	// viewer shows the same name.
	if p.Lens != nil {
		if camera == nil {
			camera = &metadata.Camera{}
		}
		camera.Lens = p.Lens
	}
	// A location an import knew comes after the file's own.
	if location == nil && p.ImportedLatitude != nil && p.ImportedLongitude != nil {
		location = &metadata.Location{Latitude: *p.ImportedLatitude, Longitude: *p.ImportedLongitude}
	}
	if !showLocation {
		location = nil
	}
	return photoJSON{
		ID:             p.ID,
		OwnerID:        p.OwnerID,
		Status:         p.Status,
		Name:           p.OriginalName,
		Mime:           p.Mime,
		Size:           p.Size,
		Width:          p.Width,
		Height:         p.Height,
		TakenAt:        p.TakenAt,
		SortAt:         p.SortAt,
		CreatedAt:      p.CreatedAt,
		DeletedAt:      p.DeletedAt,
		Camera:         camera,
		PreviewVersion: p.PreviewVersion,
		Location:       location,
		Duration:       p.Duration,
		HasMotion:      p.HasMotion,
		HasRAW:         p.HasRaw || storage.IsRAW(p.Mime),
		VideoCopy:      p.VideoCopy,
		HDR:            p.Hdr,
		Description:    p.Description,
	}
}

type versionJSON struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
}

func toPhotosJSON(photos []store.Photo, showLocation bool) []photoJSON {
	out := make([]photoJSON, len(photos))
	for i, p := range photos {
		out[i] = toPhotoJSON(p, showLocation)
	}
	return out
}

// showLocation tells whether photo responses may include locations.
func (s *server) showLocation(ctx context.Context) (bool, error) {
	settings, err := s.Queries.GetMetadataSettings(ctx)
	return settings.ShowLocation, err
}

func (s *server) listPhotos(w http.ResponseWriter, r *http.Request) {
	params, ok := s.timelineFilter(w, r)
	if !ok {
		return
	}
	if params.MaxRows, ok = pageSize(w, r); !ok {
		return
	}
	if params.AfterSortAt, params.AfterID, ok = pageCursor(w, r); !ok {
		return
	}
	photos, err := s.Queries.ListTimeline(r.Context(), params)
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.writePhotoPage(w, r, photos, params.MaxRows)
}

func (s *server) countPhotoMonths(w http.ResponseWriter, r *http.Request) {
	tz, ok := timeZoneParam(w, r)
	if !ok {
		return
	}
	filter, ok := s.timelineFilter(w, r)
	if !ok {
		return
	}
	rows, err := s.Queries.CountTimelineMonths(r.Context(), store.CountTimelineMonthsParams{
		Tz:         tz,
		OwnerID:    filter.OwnerID,
		WithShared: filter.WithShared,
		Favorites:  filter.Favorites,
		Archived:   filter.Archived,
		FromTime:   filter.FromTime,
		ToTime:     filter.ToTime,
		AlbumID:    filter.AlbumID,
		TagIds:     filter.TagIds,
		Cameras:    filter.Cameras,
		Lenses:     filter.Lenses,
		Kinds:      filter.Kinds,
		South:      filter.South,
		West:       filter.West,
		North:      filter.North,
		East:       filter.East,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeMonths(w, rows, func(row store.CountTimelineMonthsRow) monthJSON { return monthJSON(row) })
}

// timelineFilter reads the filters of the current user's list of photos, which
// listPhotos, countPhotoMonths and listCameras share. It leaves out the page.
func (s *server) timelineFilter(w http.ResponseWriter, r *http.Request) (store.ListTimelineParams, bool) {
	params := store.ListTimelineParams{OwnerID: currentUser(r).ID}
	query := r.URL.Query()
	var ok bool
	if params.FromTime, ok = timeParam(w, r, "from"); !ok {
		return params, false
	}
	if params.ToTime, ok = timeParam(w, r, "to"); !ok {
		return params, false
	}
	for _, text := range query["tag"] {
		id, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid tag")
			return params, false
		}
		params.TagIds = append(params.TagIds, id)
	}
	// Names as listCameras gives them. A name no photo has matches nothing.
	params.Cameras = query["camera"]
	params.Lenses = query["lens"]
	for _, kind := range query["kind"] {
		switch kind {
		case "photo", "motion", "video", "raw":
			params.Kinds = append(params.Kinds, kind)
		default:
			writeError(w, http.StatusBadRequest, "invalid kind")
			return params, false
		}
	}
	if text := query.Get("bbox"); text != "" {
		west, south, east, north, ok := parseBox(text)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid bbox")
			return params, false
		}
		// Narrowing a box would find where photos were taken, so it follows the
		// setting that hides locations.
		if !s.requireLocations(w, r) {
			return params, false
		}
		params.West, params.South, params.East, params.North = &west, &south, &east, &north
	}
	switch query.Get("withShared") {
	case "", "false":
	case "true":
		params.WithShared = true
	default:
		writeError(w, http.StatusBadRequest, "invalid withShared")
		return params, false
	}
	switch query.Get("favorites") {
	case "", "false":
	case "true":
		params.Favorites = true
	default:
		writeError(w, http.StatusBadRequest, "invalid favorites")
		return params, false
	}
	switch query.Get("archived") {
	case "", "false":
	case "true":
		params.Archived = true
	default:
		writeError(w, http.StatusBadRequest, "invalid archived")
		return params, false
	}
	if text := query.Get("album"); text != "" {
		// Favorites and the archive are the current user's, an album is its
		// owner's.
		if params.Favorites || params.Archived {
			writeError(w, http.StatusBadRequest, "an album cannot be listed by favorites or the archive")
			return params, false
		}
		// An album is one owner's, and its own sharing decides who sees it.
		if params.WithShared {
			writeError(w, http.StatusBadRequest, "an album cannot be listed with shared photos")
			return params, false
		}
		id, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid album")
			return params, false
		}
		album, ok := s.visibleAlbum(w, r, id)
		if !ok {
			return params, false
		}
		// Tags are personal: filtering another user's album by tag would reveal
		// how that user grouped its photos.
		if album.OwnerID != params.OwnerID && len(params.TagIds) > 0 {
			writeError(w, http.StatusBadRequest, "another user's album cannot be filtered by tag")
			return params, false
		}
		params.OwnerID = album.OwnerID
		params.AlbumID = &id
	}
	return params, true
}

// nameCountJSON is a camera, a lens or a media kind and the number of photos
// of it.
type nameCountJSON struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// listCameras gives the cameras, lenses and media kinds of the photos
// listPhotos gives for the same filters. Each list leaves out its own filter,
// so that the timeline can show how many photos another choice would add.
func (s *server) listCameras(w http.ResponseWriter, r *http.Request) {
	filter, ok := s.timelineFilter(w, r)
	if !ok {
		return
	}
	cameras, err := s.Queries.ListCameras(r.Context(), store.ListCamerasParams{
		OwnerID:    filter.OwnerID,
		WithShared: filter.WithShared,
		Favorites:  filter.Favorites,
		Archived:   filter.Archived,
		FromTime:   filter.FromTime,
		ToTime:     filter.ToTime,
		AlbumID:    filter.AlbumID,
		TagIds:     filter.TagIds,
		Lenses:     filter.Lenses,
		Kinds:      filter.Kinds,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	lenses, err := s.Queries.ListLenses(r.Context(), store.ListLensesParams{
		OwnerID:    filter.OwnerID,
		WithShared: filter.WithShared,
		Favorites:  filter.Favorites,
		Archived:   filter.Archived,
		FromTime:   filter.FromTime,
		ToTime:     filter.ToTime,
		AlbumID:    filter.AlbumID,
		TagIds:     filter.TagIds,
		Cameras:    filter.Cameras,
		Kinds:      filter.Kinds,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	kinds, err := s.Queries.ListKinds(r.Context(), store.ListKindsParams{
		OwnerID:    filter.OwnerID,
		WithShared: filter.WithShared,
		Favorites:  filter.Favorites,
		Archived:   filter.Archived,
		FromTime:   filter.FromTime,
		ToTime:     filter.ToTime,
		AlbumID:    filter.AlbumID,
		TagIds:     filter.TagIds,
		Cameras:    filter.Cameras,
		Lenses:     filter.Lenses,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := struct {
		Cameras []nameCountJSON `json:"cameras"`
		Lenses  []nameCountJSON `json:"lenses"`
		Kinds   []nameCountJSON `json:"kinds"`
	}{make([]nameCountJSON, len(cameras)), make([]nameCountJSON, len(lenses)), make([]nameCountJSON, len(kinds))}
	for i, row := range cameras {
		out.Cameras[i] = nameCountJSON(row)
	}
	for i, row := range lenses {
		out.Lenses[i] = nameCountJSON(row)
	}
	for i, row := range kinds {
		out.Kinds[i] = nameCountJSON(row)
	}
	writeJSON(w, http.StatusOK, out)
}

// monthJSON is the number of photos a list has in one month, such as "2024-05".
type monthJSON struct {
	Month string `json:"month"`
	Count int64  `json:"count"`
}

func writeMonths[T any](w http.ResponseWriter, rows []T, convert func(T) monthJSON) {
	out := make([]monthJSON, len(rows))
	for i, row := range rows {
		out[i] = convert(row)
	}
	writeJSON(w, http.StatusOK, out)
}

// timeZoneParam reads the time zone that months are counted in: the browser's,
// so that they match the dates the interface shows.
func timeZoneParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.URL.Query().Get("tz")
	if _, err := time.LoadLocation(name); err != nil || name == "" || name == "Local" {
		writeError(w, http.StatusBadRequest, "invalid tz")
		return "", false
	}
	return name, true
}

// writePhotoPage answers with a page of a list ordered by sort time.
func (s *server) writePhotoPage(w http.ResponseWriter, r *http.Request, photos []store.Photo, maxRows int32) {
	showLocation, err := s.showLocation(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	var next *string
	if len(photos) == int(maxRows) {
		last := photos[len(photos)-1]
		next = cursorFor(last.SortAt, last.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": toPhotosJSON(photos, showLocation), "nextCursor": next})
}

func (s *server) listTrash(w http.ResponseWriter, r *http.Request) {
	maxRows, ok := pageSize(w, r)
	if !ok {
		return
	}
	afterDeletedAt, afterID, ok := pageCursor(w, r)
	if !ok {
		return
	}
	photos, err := s.Queries.ListTrash(r.Context(), store.ListTrashParams{
		OwnerID:        currentUser(r).ID,
		AfterDeletedAt: afterDeletedAt,
		AfterID:        afterID,
		MaxRows:        maxRows,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	showLocation, err := s.showLocation(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	var next *string
	if len(photos) == int(maxRows) {
		last := photos[len(photos)-1]
		next = cursorFor(*last.DeletedAt, last.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": toPhotosJSON(photos, showLocation), "nextCursor": next})
}

func pageSize(w http.ResponseWriter, r *http.Request) (int32, bool) {
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		return defaultPageSize, true
	}
	n, err := strconv.Atoi(limit)
	if err != nil || n < 1 || n > maxPageSize {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return 0, false
	}
	return int32(n), true
}

func pageCursor(w http.ResponseWriter, r *http.Request) (*time.Time, *int64, bool) {
	cursor := r.URL.Query().Get("cursor")
	if cursor == "" {
		return nil, nil, true
	}
	at, id, err := parseCursor(cursor)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return nil, nil, false
	}
	return &at, &id, true
}

func cursorFor(at time.Time, id int64) *string {
	cursor := fmt.Sprintf("%d_%d", at.UnixMicro(), id)
	return &cursor
}

func timeParam(w http.ResponseWriter, r *http.Request, name string) (*time.Time, bool) {
	text := r.URL.Query().Get(name)
	if text == "" {
		return nil, true
	}
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+name)
		return nil, false
	}
	return &at, true
}

// readIDs reads the photos a bulk action applies to. Ids of photos the current
// user may not change are ignored.
func readIDs(w http.ResponseWriter, r *http.Request) ([]int64, bool) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if !readJSON(w, r, &req) {
		return nil, false
	}
	if !validBulkIDs(w, req.IDs) {
		return nil, false
	}
	return req.IDs, true
}

func validBulkIDs(w http.ResponseWriter, ids []int64) bool {
	if len(ids) == 0 || len(ids) > maxBulkPhotos {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("a bulk action takes 1 to %d photos", maxBulkPhotos))
		return false
	}
	return true
}

func (s *server) trashPhotos(w http.ResponseWriter, r *http.Request) {
	ids, ok := readIDs(w, r)
	if !ok {
		return
	}
	user := currentUser(r)
	ctx := r.Context()
	err := pgx.BeginFunc(ctx, s.Ingest.Pool, func(tx pgx.Tx) error {
		queries := store.New(tx)
		if err := queries.LockPhotoLinks(ctx); err != nil {
			return err
		}
		return queries.TrashPhotos(ctx, store.TrashPhotosParams{Ids: ids, ActorID: user.ID, ActorIsAdmin: isAdmin(user)})
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) restorePhotos(w http.ResponseWriter, r *http.Request) {
	ids, ok := readIDs(w, r)
	if !ok {
		return
	}
	owner := currentUser(r).ID
	if err := s.Queries.RestorePhotos(r.Context(), store.RestorePhotosParams{OwnerID: owner, Ids: ids}); err != nil {
		internalError(w, r, err)
		return
	}
	// A video may have come in while its photo was in the trash, and so may
	// the edited copy of an original.
	if _, err := jobs.PairLivePhotos(r.Context(), s.Ingest.Pool, &owner, nil); err != nil {
		slog.Warn("pair live photos", "error", err)
	}
	if _, err := jobs.LinkVersions(r.Context(), s.Ingest.Pool, &owner, ids); err != nil {
		slog.Warn("link versions", "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) purgePhotos(w http.ResponseWriter, r *http.Request) {
	ids, ok := readIDs(w, r)
	if !ok {
		return
	}
	if err := s.purge(r, ids); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) emptyTrash(w http.ResponseWriter, r *http.Request) {
	owner := currentUser(r).ID
	for {
		ids, err := s.Queries.ListTrashedPhotoIDs(r.Context(), store.ListTrashedPhotoIDsParams{OwnerID: owner, MaxRows: maxBulkPhotos})
		if err != nil {
			internalError(w, r, err)
			return
		}
		if len(ids) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err := s.purge(r, ids); err != nil {
			internalError(w, r, err)
			return
		}
	}
}

// purge stops between photos when the client goes away. What it has not reached
// stays in the trash.
func (s *server) purge(r *http.Request, ids []int64) error {
	for _, id := range ids {
		if err := r.Context().Err(); err != nil {
			return err
		}
		if err := s.Ingest.Purge(r.Context(), currentUser(r).ID, id); err != nil {
			return err
		}
	}
	return nil
}

func parseCursor(cursor string) (time.Time, int64, error) {
	micros, idText, ok := strings.Cut(cursor, "_")
	if !ok {
		return time.Time{}, 0, errors.New("malformed cursor")
	}
	sortAt, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return time.Time{}, 0, err
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return time.Time{}, 0, err
	}
	return time.UnixMicro(sortAt), id, nil
}

func (s *server) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var event store.RecordActivityParams
	if s.ActivityLog {
		// One event per request: the browser sends one file in each, so a
		// request with several files keeps the outcome of the last one.
		event.Action = "upload"
		event.AccountID = &user.ID
		event.SessionID, _ = r.Context().Value(activitySessionKey{}).(*string)
		rec := &responseRecorder{ResponseWriter: w}
		w = rec
		defer s.recordActivity(r, rec, &event)
	}
	// The album is a query parameter rather than a form field, as the file
	// streams into ingest and a field after it would come too late.
	var albumID int64
	if value := r.URL.Query().Get("album"); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if _, ok := s.ownAlbum(w, r, id); !ok {
			return
		}
		albumID = id
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	parts, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected a multipart upload")
		return
	}
	for {
		part, err := parts.NextPart()
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "the upload has no file part")
			return
		}
		if err != nil {
			uploadError(w, r, err)
			return
		}
		if part.FormName() != "file" {
			continue
		}

		res, err := s.Ingest.Ingest(r.Context(), user.ID, part, part.FileName(), ingest.PriorityInteractive)
		if res.MIME != "" {
			event.MediaType, event.SizeBytes = &res.MIME, &res.Size
		}
		if err != nil {
			uploadError(w, r, err)
			return
		}
		event.Outcome = "accepted"
		if res.Restored {
			event.Outcome = "restored"
		} else if res.Duplicate {
			event.Outcome = "duplicate"
		}
		event.MediaID = &res.Photo.ID
		// A duplicate is added too, as a WebDAV album does. A duplicate of a
		// hidden file answers with the photo shown at the top of its chain.
		if albumID != 0 {
			err := s.Queries.AddPhotosToAlbum(r.Context(), store.AddPhotosToAlbumParams{AlbumID: albumID, OwnerID: currentUser(r).ID, Ids: []int64{res.Photo.ID}})
			if err != nil {
				internalError(w, r, err)
				return
			}
		}
		showLocation, err := s.showLocation(r.Context())
		if err != nil {
			internalError(w, r, err)
			return
		}
		status := http.StatusCreated
		if res.Duplicate {
			status = http.StatusOK
		}
		writeJSON(w, status, map[string]any{"photo": toPhotoJSON(res.Photo, showLocation), "duplicate": res.Duplicate, "restored": res.Restored})
		return
	}
}

func uploadError(w http.ResponseWriter, r *http.Request, err error) {
	var tooBig *http.MaxBytesError
	switch {
	case errors.Is(err, ingest.ErrUnsupported):
		writeError(w, http.StatusUnsupportedMediaType, ingest.ErrUnsupportedMessage)
	case errors.Is(err, ingest.ErrTooLarge), errors.As(err, &tooBig):
		writeError(w, http.StatusRequestEntityTooLarge, "the file is too large")
	default:
		internalError(w, r, err)
	}
}

// visiblePhoto answers 404 for photos the user may not see, so that a response
// does not reveal which photos exist.
func (s *server) visiblePhoto(w http.ResponseWriter, r *http.Request) (store.Photo, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return store.Photo{}, false
	}
	user := currentUser(r)
	photo, err := s.Queries.GetVisiblePhoto(r.Context(), store.GetVisiblePhotoParams{
		ID:            id,
		ViewerID:      user.ID,
		ViewerIsAdmin: isAdmin(user),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return store.Photo{}, false
	}
	if err != nil {
		internalError(w, r, err)
		return store.Photo{}, false
	}
	return photo, true
}

func (s *server) getPhoto(w http.ResponseWriter, r *http.Request) {
	photo, ok := s.visiblePhoto(w, r)
	if !ok {
		return
	}
	showLocation, err := s.showLocation(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := toPhotoJSON(photo, showLocation)
	favorite, err := s.Queries.IsFavorite(r.Context(), store.IsFavoriteParams{UserID: currentUser(r).ID, PhotoID: photo.ID})
	if err != nil {
		internalError(w, r, err)
		return
	}
	out.Favorite = &favorite
	archived, err := s.Queries.IsArchived(r.Context(), store.IsArchivedParams{UserID: currentUser(r).ID, PhotoID: photo.ID})
	if err != nil {
		internalError(w, r, err)
		return
	}
	out.Archived = &archived
	if photo.SourcePath != nil {
		missing := s.Layout.Unavailable(photo.SourcePath, s.Layout.OriginalOf(photo.OwnerID, photo.Sha256, photo.Mime, photo.SourcePath))
		out.FileMissing = &missing
		if photo.OwnerID == currentUser(r).ID {
			out.SourcePath = photo.SourcePath
		}
	}
	if photo.OwnerID == currentUser(r).ID {
		versions, err := s.Queries.ListPhotoVersions(r.Context(), photo.ID)
		if err != nil {
			internalError(w, r, err)
			return
		}
		for _, v := range versions {
			out.Versions = append(out.Versions, versionJSON{ID: v.ID, Kind: v.Kind})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getPreview(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind != storage.PreviewGrid && kind != storage.PreviewViewer {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	photo, ok := s.visiblePhoto(w, r)
	if !ok {
		return
	}
	if photo.Status != "ready" || photo.PreviewFormat == nil {
		writeError(w, http.StatusNotFound, "the preview is not ready")
		return
	}

	w.Header().Set("Cache-Control", previewCacheRule)
	w.Header().Set("Content-Type", "image/"+*photo.PreviewFormat)
	s.serveFile(w, r, s.Layout.Preview(photo.ID, kind, *photo.PreviewFormat))
}

func (s *server) getOriginal(w http.ResponseWriter, r *http.Request) {
	photo, ok := s.visiblePhoto(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", photo.Mime)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": photo.OriginalName}))
	s.serveFile(w, r, s.Layout.OriginalOf(photo.OwnerID, photo.Sha256, photo.Mime, photo.SourcePath))
}

// getVideo serves the original of a video for playback, as an MP4. QuickTime
// is close enough to MP4 for browsers, and some refuse to play a file sent as
// video/quicktime.
func (s *server) getVideo(w http.ResponseWriter, r *http.Request) {
	photo, ok := s.visiblePhoto(w, r)
	if !ok {
		return
	}
	if !storage.IsVideo(photo.Mime) {
		writeError(w, http.StatusNotFound, "not a video")
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "private, no-cache")
	s.serveFile(w, r, s.Layout.OriginalOf(photo.OwnerID, photo.Sha256, photo.Mime, photo.SourcePath))
}

// getMotion serves the video of a motion photo, or the original of the video
// of a Live Photo. Whoever can see the photo can play it. Like a preview, a
// motion photo's video is made again only with the previews, so its address
// carries their version; the video of a Live Photo never changes.
func (s *server) getMotion(w http.ResponseWriter, r *http.Request) {
	photo, ok := s.visiblePhoto(w, r)
	if !ok {
		return
	}
	if photo.Status != "ready" || !photo.HasMotion {
		writeError(w, http.StatusNotFound, "the photo has no motion")
		return
	}
	path, err := transcode.MotionSource(r.Context(), s.Queries, s.Layout, photo.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", previewCacheRule)
	w.Header().Set("Content-Type", "video/mp4")
	s.serveFile(w, r, path)
}

func (s *server) serveFile(w http.ResponseWriter, r *http.Request, path string) {
	var file *os.File
	var err error
	if s.Layout.IsSource(path) {
		// A file of a library folder may be swapped for a link by whoever can write to the share.
		file, err = s.Layout.OpenSource(path)
		if errors.Is(err, storage.ErrUnsafeSource) {
			err = os.ErrNotExist
		}
	} else {
		file, err = os.Open(path)
	}
	if errors.Is(err, os.ErrNotExist) && (path == "" || s.Layout.IsSource(path)) {
		// An original in a library folder whose file is away or whose share is not mounted, or
		// a motion with no readable source. A file missing from the library or the cache is a
		// fault, and stays an error.
		w.Header().Del("Cache-Control")
		writeError(w, http.StatusNotFound, "the file is not available")
		return
	}
	if err != nil {
		w.Header().Del("Cache-Control")
		internalError(w, r, err)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		w.Header().Del("Cache-Control")
		internalError(w, r, err)
		return
	}
	http.ServeContent(w, r, "", info.ModTime(), file)
}

// updatePhoto changes what the owner may change of a photo: its description.
func (s *server) updatePhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Description *string `json:"description"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Description == nil {
		writeError(w, http.StatusBadRequest, "nothing to change")
		return
	}
	if utf8.RuneCountInString(strings.TrimSpace(*req.Description)) > maxDescription {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("a description is at most %d characters", maxDescription))
		return
	}
	// PostgreSQL text holds no NUL; clean_description makes the other
	// control characters spaces.
	description := strings.ReplaceAll(*req.Description, "\x00", " ")
	photo, err := s.Queries.SetPhotoDescription(r.Context(), store.SetPhotoDescriptionParams{
		ID:          id,
		OwnerID:     currentUser(r).ID,
		Description: description,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	showLocation, err := s.showLocation(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toPhotoJSON(photo, showLocation))
}

// changeFavorites marks photos the current user can see as the user's
// favorites, or takes the marks off. A mark is the user's own, so it moves
// no updated_at and sends no event: the page that changed it updates itself.
func (s *server) changeFavorites(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs      []int64 `json:"ids"`
		Favorite *bool   `json:"favorite"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Favorite == nil {
		writeError(w, http.StatusBadRequest, "favorite is required")
		return
	}
	if !validBulkIDs(w, req.IDs) {
		return
	}
	user := currentUser(r).ID
	var err error
	if *req.Favorite {
		err = s.Queries.AddFavorites(r.Context(), store.AddFavoritesParams{UserID: user, Ids: req.IDs})
	} else {
		err = s.Queries.RemoveFavorites(r.Context(), store.RemoveFavoritesParams{UserID: user, Ids: req.IDs})
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// changeArchive archives photos the current user can see, for the user, or
// takes them out of the archive. Like a favorite mark, it sends no event.
func (s *server) changeArchive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs      []int64 `json:"ids"`
		Archived *bool   `json:"archived"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Archived == nil {
		writeError(w, http.StatusBadRequest, "archived is required")
		return
	}
	if !validBulkIDs(w, req.IDs) {
		return
	}
	user := currentUser(r).ID
	var err error
	if *req.Archived {
		err = s.Queries.AddToArchive(r.Context(), store.AddToArchiveParams{UserID: user, Ids: req.IDs})
	} else {
		err = s.Queries.RemoveFromArchive(r.Context(), store.RemoveFromArchiveParams{UserID: user, Ids: req.IDs})
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
