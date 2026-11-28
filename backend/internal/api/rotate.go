package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/magellanica-app/magellanica-photos/backend/internal/jobs"
	"github.com/magellanica-app/magellanica-photos/backend/internal/media"
	"github.com/magellanica-app/magellanica-photos/backend/internal/storage"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

var (
	errRotateNotFound   = errors.New("photo not found")
	errRotateProcessing = errors.New("the photo is still being processed")
	errRotateVideo      = errors.New("videos cannot be rotated")
)

// rotatePhoto turns a photo of the user clockwise by 90, 180 or 270 degrees,
// with the versions hidden under it. The original is not changed. The
// previews turn at once, from the viewer preview; a render from the original
// follows. The search vector, the hash and the faces are made again.
func (s *server) rotatePhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Degrees int `json:"degrees"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Degrees != 90 && req.Degrees != 180 && req.Degrees != 270 {
		writeError(w, http.StatusBadRequest, "degrees must be 90, 180 or 270")
		return
	}
	ctx := r.Context()
	var chain []int64
	err := pgx.BeginFunc(ctx, s.Ingest.Pool, func(tx pgx.Tx) error {
		var err error
		chain, err = s.rotateChain(ctx, tx, currentUser(r).ID, id, req.Degrees)
		return err
	})
	switch {
	case errors.Is(err, errRotateNotFound):
		writeError(w, http.StatusNotFound, "photo not found")
		return
	case errors.Is(err, errRotateProcessing):
		writeError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, errRotateVideo):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		internalError(w, r, err)
		return
	}
	// A render from the original replaces the turned previews, so turning
	// several times does not lose quality.
	if s.Ingest.Jobs != nil {
		for _, photo := range chain {
			if _, err := s.Ingest.Jobs.Insert(ctx, jobs.RenderPreviewsArgs{PhotoID: photo}, nil); err != nil {
				slog.Warn("queue render after rotation", "photo", photo, "error", err)
			}
		}
	}
	s.getPhoto(w, r)
}

func (s *server) rotateChain(ctx context.Context, tx pgx.Tx, user, id int64, degrees int) ([]int64, error) {
	var owner int64
	var status, mime string
	err := tx.QueryRow(ctx, `SELECT owner_id, status, mime FROM photos p
 WHERE id=$1 AND deleted_at IS NULL AND purged_at IS NULL
 AND NOT EXISTS (SELECT FROM photo_links l WHERE l.hidden_id = p.id)`, id).Scan(&owner, &status, &mime)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && owner != user {
		return nil, errRotateNotFound
	}
	if err != nil {
		return nil, err
	}
	if storage.IsVideo(mime) {
		return nil, errRotateVideo
	}
	if status != "ready" {
		return nil, errRotateProcessing
	}
	rows, err := tx.Query(ctx, `WITH RECURSIVE chain AS (
   SELECT $1::bigint AS id
   UNION
   SELECT l.hidden_id FROM photo_links l JOIN chain c ON l.shown_id = c.id
 )
 SELECT p.id FROM chain JOIN photos p ON p.id = chain.id
 WHERE p.mime NOT LIKE 'video/%' AND p.purged_at IS NULL ORDER BY p.id`, id)
	if err != nil {
		return nil, err
	}
	chain, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, err
	}
	stored, err := s.Queries.WithTx(tx).GetPreviewSettings(ctx)
	if err != nil {
		return nil, err
	}
	for _, photo := range chain {
		if err := s.rotateOne(ctx, tx, photo, degrees, stored); err != nil {
			return nil, fmt.Errorf("rotate photo %d: %w", photo, err)
		}
	}
	// New vectors for search and for similar photos come from the turned preview.
	if _, err := tx.Exec(ctx, "DELETE FROM photo_embeddings WHERE photo_id = ANY($1)", chain); err != nil {
		return nil, err
	}
	// Faces are kept only for the photo that is shown. Their boxes turn now,
	// and a rescan matches them to the faces found on the turned preview.
	if _, err := tx.Exec(ctx, `UPDATE photo_faces SET box = CASE $2::int
     WHEN 90 THEN ARRAY[1-box[2]-box[4], box[1], box[4], box[3]]
     WHEN 180 THEN ARRAY[1-box[1]-box[3], 1-box[2]-box[4], box[3], box[4]]
     ELSE ARRAY[box[2], 1-box[1]-box[3], box[4], box[3]] END
   WHERE photo_id=$1`, id, degrees); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "UPDATE face_scans SET rescan=true WHERE photo_id=$1", id); err != nil {
		return nil, err
	}
	return chain, nil
}

func (s *server) rotateOne(ctx context.Context, tx pgx.Tx, id int64, degrees int, stored store.PreviewSetting) error {
	if err := jobs.LockPhoto(ctx, tx, id); err != nil {
		return err
	}
	var rotation, previewRotation int16
	var format *string
	if err := tx.QueryRow(ctx, "SELECT rotation, preview_rotation, preview_format FROM photos WHERE id=$1", id).
		Scan(&rotation, &previewRotation, &format); err != nil {
		return err
	}
	next := (int(rotation) + degrees) % 360
	var phash *int64
	turned := false
	if delta := (next - int(previewRotation) + 360) % 360; delta != 0 && format != nil {
		viewerPath := s.Layout.Preview(id, storage.PreviewViewer, *format)
		data, err := os.ReadFile(viewerPath)
		switch {
		case errors.Is(err, os.ErrNotExist):
			// No previews yet: the render from the original makes them turned.
		case err != nil:
			return err
		default:
			settings := media.Settings{
				Format:      media.Format(*format),
				WebPQuality: int(stored.WebpQuality),
				WebPEffort:  int(stored.WebpEffort),
				JPEGQuality: int(stored.JpegQuality),
				GridEdge:    int(stored.GridEdge),
				ViewerEdge:  int(stored.ViewerEdge),
			}
			previews, err := media.RotatePreviews(data, delta, settings)
			if err != nil {
				// The render from the original that follows makes it turned.
				slog.Warn("turn preview", "photo", id, "error", err)
				break
			}
			if err := storage.WriteFileAtomic(viewerPath, previews.Viewer.Data); err != nil {
				return err
			}
			if err := storage.WriteFileAtomic(s.Layout.Preview(id, storage.PreviewGrid, *format), previews.Grid.Data); err != nil {
				return err
			}
			if hash, err := media.Hash(previews.Viewer.Data); err == nil {
				v := int64(hash)
				phash = &v
			}
			turned = true
		}
	}
	swap := degrees == 90 || degrees == 270
	_, err := tx.Exec(ctx, `UPDATE photos SET rotation=$2,
   preview_rotation = CASE WHEN $3 THEN $2 ELSE preview_rotation END,
   width = CASE WHEN $4 THEN height ELSE width END,
   height = CASE WHEN $4 THEN width ELSE height END,
   phash = CASE WHEN $3 THEN $5 ELSE phash END,
   preview_version = preview_version + 1, updated_at = now()
 WHERE id=$1`, id, next, turned, swap, phash)
	return err
}
