package api

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const (
	maxUploadBytes   = 512 << 20
	defaultPageSize  = 200
	maxPageSize      = 500
	previewCacheRule = "private, max-age=31536000, immutable"
)

// EXIF stays out of this type on purpose: it can hold GPS coordinates, and
// visible photos are not always the viewer's own.
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
}

func toPhotoJSON(p store.Photo) photoJSON {
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
	}
}

func toPhotosJSON(photos []store.Photo) []photoJSON {
	out := make([]photoJSON, len(photos))
	for i, p := range photos {
		out[i] = toPhotoJSON(p)
	}
	return out
}

func (s *server) listPhotos(w http.ResponseWriter, r *http.Request) {
	params := store.ListTimelineParams{OwnerID: currentUser(r).ID, MaxRows: defaultPageSize}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 1 || n > maxPageSize {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		params.MaxRows = int32(n)
	}
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		sortAt, id, err := parseCursor(cursor)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		params.AfterSortAt, params.AfterID = &sortAt, &id
	}

	photos, err := s.Queries.ListTimeline(r.Context(), params)
	if err != nil {
		internalError(w, r, err)
		return
	}
	var next *string
	if len(photos) == int(params.MaxRows) {
		last := photos[len(photos)-1]
		cursor := fmt.Sprintf("%d_%d", last.SortAt.UnixMicro(), last.ID)
		next = &cursor
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": toPhotosJSON(photos), "nextCursor": next})
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
		status := http.StatusCreated
		if res.Duplicate {
			status = http.StatusOK
		}
		writeJSON(w, status, map[string]any{"photo": toPhotoJSON(res.Photo), "duplicate": res.Duplicate})
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
		ViewerIsAdmin: user.Role == auth.RoleAdmin,
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
	if photo, ok := s.visiblePhoto(w, r); ok {
		writeJSON(w, http.StatusOK, toPhotoJSON(photo))
	}
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
