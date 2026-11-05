package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	// Time zones are checked against the copy built into the binary, as the
	// image has no time zone database.
	_ "time/tzdata"

	"github.com/jackc/pgx/v5"

	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/metadata"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const (
	maxUploadBytes   = 512 << 20
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

	Camera   *metadata.Camera   `json:"camera"`
	Location *metadata.Location `json:"location"`
}

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
	if !showLocation {
		location = nil
	}
	return photoJSON{
		ID:        p.ID,
		OwnerID:   p.OwnerID,
		Status:    p.Status,
		Name:      p.OriginalName,
		Mime:      p.Mime,
		Size:      p.Size,
		Width:     p.Width,
		Height:    p.Height,
		TakenAt:   p.TakenAt,
		SortAt:    p.SortAt,
		CreatedAt: p.CreatedAt,
		DeletedAt: p.DeletedAt,
		Camera:    camera,
		Location:  location,
	}
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
		Tz:       tz,
		OwnerID:  filter.OwnerID,
		FromTime: filter.FromTime,
		ToTime:   filter.ToTime,
		AlbumID:  filter.AlbumID,
		TagIds:   filter.TagIds,
		Cameras:  filter.Cameras,
		Lenses:   filter.Lenses,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeMonths(w, rows, func(row store.CountTimelineMonthsRow) monthJSON { return monthJSON(row) })
}

// timelineFilter reads the filters of the current user's list of photos, which
// listPhotos and countPhotoMonths share. It leaves out the page.
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
	if text := query.Get("album"); text != "" {
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

// nameCountJSON is a camera or a lens and the number of photos taken with it.
type nameCountJSON struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// listCameras gives the cameras and lenses of the photos listPhotos gives for
// the same filters. Each list leaves out its own filter, so that the timeline
// can show how many photos another camera or lens would add.
func (s *server) listCameras(w http.ResponseWriter, r *http.Request) {
	filter, ok := s.timelineFilter(w, r)
	if !ok {
		return
	}
	cameras, err := s.Queries.ListCameras(r.Context(), store.ListCamerasParams{
		OwnerID:  filter.OwnerID,
		FromTime: filter.FromTime,
		ToTime:   filter.ToTime,
		AlbumID:  filter.AlbumID,
		TagIds:   filter.TagIds,
		Lenses:   filter.Lenses,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	lenses, err := s.Queries.ListLenses(r.Context(), store.ListLensesParams{
		OwnerID:  filter.OwnerID,
		FromTime: filter.FromTime,
		ToTime:   filter.ToTime,
		AlbumID:  filter.AlbumID,
		TagIds:   filter.TagIds,
		Cameras:  filter.Cameras,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := struct {
		Cameras []nameCountJSON `json:"cameras"`
		Lenses  []nameCountJSON `json:"lenses"`
	}{make([]nameCountJSON, len(cameras)), make([]nameCountJSON, len(lenses))}
	for i, row := range cameras {
		out.Cameras[i] = nameCountJSON(row)
	}
	for i, row := range lenses {
		out.Lenses[i] = nameCountJSON(row)
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
	if err := s.Queries.TrashPhotos(r.Context(), store.TrashPhotosParams{Ids: ids, ActorID: user.ID, ActorIsAdmin: isAdmin(user)}); err != nil {
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
	if err := s.Queries.RestorePhotos(r.Context(), store.RestorePhotosParams{OwnerID: currentUser(r).ID, Ids: ids}); err != nil {
		internalError(w, r, err)
		return
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

		res, err := s.Ingest.Ingest(r.Context(), currentUser(r).ID, part, part.FileName(), ingest.PriorityInteractive)
		if err != nil {
			uploadError(w, r, err)
			return
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
		writeError(w, http.StatusUnsupportedMediaType, "only JPEG, PNG, WebP and HEIC photos are supported")
	case errors.As(err, &tooBig):
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
	writeJSON(w, http.StatusOK, toPhotoJSON(photo, showLocation))
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
	s.serveFile(w, r, s.Layout.Original(photo.OwnerID, photo.Sha256, photo.Mime))
}

func (s *server) serveFile(w http.ResponseWriter, r *http.Request, path string) {
	file, err := os.Open(path)
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
