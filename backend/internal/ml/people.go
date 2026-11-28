package ml

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/magellanica-app/magellanica-photos/backend/internal/storage"
)

type FaceScanArgs struct{}

func (FaceScanArgs) Kind() string { return "scan_people" }
func (FaceScanArgs) InsertOpts() river.InsertOpts {
	opts := (EmbedPhotosArgs{}).InsertOpts()
	opts.Queue = "people"
	return opts
}

// PeopleJob queues a scan every minute while face inference is available.
func PeopleJob(monitor *Monitor) *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(time.Minute),
		func() (river.JobArgs, *river.InsertOpts) {
			if !monitor.FacesAvailable() {
				return nil, nil
			}
			return FaceScanArgs{}, nil
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	)
}

type FaceWorker struct {
	river.WorkerDefaults[FaceScanArgs]
	Client  *Client
	Monitor *Monitor
	Pool    *pgxpool.Pool
	Layout  storage.Layout
}

const faceScanEligible = `p.status='ready' AND p.deleted_at IS NULL AND p.purged_at IS NULL
   AND p.preview_format IS NOT NULL AND p.mime NOT LIKE 'video/%'
   AND NOT EXISTS(SELECT FROM photo_links WHERE hidden_id=p.id)
   AND p.preview_rotation = p.rotation`

type PhotoFaceScan struct {
	PhotoID   int64      `json:"photoId"`
	Name      string     `json:"name"`
	State     string     `json:"state"`
	ScannedAt *time.Time `json:"scannedAt"`
	Faces     int32      `json:"faces"`
	Error     string     `json:"error"`
	// Unmatched counts faces the owner placed or named that the scan after a
	// rotation found no match for; they keep their old vector.
	Unmatched int32 `json:"unmatched"`
}

// LookupPhotoFaceScan reports scan status for the photo's current file.
func LookupPhotoFaceScan(ctx context.Context, pool *pgxpool.Pool, photoID int64) (PhotoFaceScan, error) {
	var scan PhotoFaceScan
	err := pool.QueryRow(ctx, `SELECT p.id,p.original_name,
   CASE WHEN NOT (`+faceScanEligible+`) THEN 'ineligible'
        WHEN s.photo_id IS NULL OR s.rescan THEN 'waiting'
        WHEN s.retry_at IS NOT NULL THEN 'error'
        ELSE 'completed' END,
   s.scanned_at,(SELECT count(*) FROM photo_faces f WHERE f.photo_id=s.photo_id)::int,coalesce(s.error,''),
   (SELECT count(*) FROM photo_faces f WHERE f.photo_id=s.photo_id AND f.unmatched)::int
   FROM photos p LEFT JOIN face_scans s ON s.photo_id=p.id AND s.sha256=p.sha256
   WHERE p.id=$1`, photoID).Scan(&scan.PhotoID, &scan.Name, &scan.State, &scan.ScannedAt, &scan.Faces, &scan.Error, &scan.Unmatched)
	return scan, err
}

func (w *FaceWorker) health(ctx context.Context) (Health, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return w.Client.Health(ctx)
}

func (w *FaceWorker) isPhotoError(err error) bool {
	if errors.Is(err, errFacePreviewSize) {
		return true
	}
	var response *httpError
	if !errors.As(err, &response) {
		return false
	}
	return response.code == http.StatusRequestEntityTooLarge || response.code == http.StatusUnprocessableEntity
}

func (w *FaceWorker) pause(err error) {
	if w.Monitor != nil {
		w.Monitor.faceFailed()
	}
	slog.Warn("pause face scans", "error", err)
}

func (w *FaceWorker) Timeout(*river.Job[FaceScanArgs]) time.Duration { return 3 * time.Minute }
func (w *FaceWorker) Work(ctx context.Context, job *river.Job[FaceScanArgs]) error {
	processed := false
	skip := func() error {
		if job != nil && !processed {
			return river.MetadataSet(ctx, "ml_skipped", true)
		}
		return nil
	}
	if w.Client == nil || w.Client.token == "" {
		return skip()
	}
	health, err := w.health(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || !health.Faces.supported() {
		if err == nil {
			err = errors.New("ML service: face model unavailable")
		}
		w.pause(err)
		return skip()
	}
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		var id, owner int64
		var hash []byte
		var format string
		var version int32
		err = w.Pool.QueryRow(ctx, `SELECT p.id,p.owner_id,p.sha256,p.preview_format,p.preview_version FROM photos p
   WHERE `+faceScanEligible+`
   AND (NOT EXISTS(SELECT FROM face_scans s WHERE s.photo_id=p.id AND s.sha256=p.sha256
   AND (s.retry_at IS NULL OR (s.model=$1 AND s.retry_at>now())))
   OR EXISTS(SELECT FROM face_scans s WHERE s.photo_id=p.id AND s.sha256=p.sha256 AND s.rescan))
   ORDER BY p.id LIMIT 1`, health.Faces.Model).Scan(&id, &owner, &hash, &format, &version)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		data, scanErr := os.ReadFile(w.Layout.Preview(id, storage.PreviewViewer, format))
		var faces []DetectedFace
		if scanErr == nil {
			faces, scanErr = w.Client.DetectFaces(ctx, data, *health.Faces)
			if scanErr != nil {
				photoError := w.isPhotoError(scanErr)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if !photoError {
					w.pause(scanErr)
					return skip()
				}
			}
		}
		err = pgx.BeginFunc(ctx, w.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", owner); err != nil {
				return err
			}
			var current int64
			// A photo turned while its preview was with the service is
			// scanned again from the turned preview.
			err := tx.QueryRow(ctx, `SELECT p.id FROM photos p WHERE p.id=$1 AND p.owner_id=$2 AND p.sha256=$3 AND p.preview_version=$4 AND `+faceScanEligible+` FOR UPDATE`, id, owner, hash, version).Scan(&current)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			var rescan bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM face_scans WHERE photo_id=$1 AND sha256=$2 AND rescan)", id, hash).Scan(&rescan); err != nil {
				return err
			}
			if rescan {
				processed = true
				return rescanFaces(ctx, tx, owner, health.Faces.Model, id, faces, scanErr)
			}
			var complete bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM face_scans WHERE photo_id=$1 AND sha256=$2 AND retry_at IS NULL)", id, hash).Scan(&complete); err != nil || complete {
				return err
			}
			processed = true
			if scanErr != nil {
				_, err = tx.Exec(ctx, `INSERT INTO face_scans(photo_id,model,sha256,retry_at,error) VALUES($1,$2,$3,now()+interval '10 minutes',$4)
      ON CONFLICT(photo_id) DO UPDATE SET retry_at=excluded.retry_at,model=excluded.model,error=excluded.error,scanned_at=now() WHERE face_scans.retry_at IS NOT NULL`, id, health.Faces.Model, hash, scanErr.Error())
				return err
			}
			if _, err = tx.Exec(ctx, "DELETE FROM face_scans WHERE photo_id=$1", id); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "INSERT INTO face_scans(photo_id,model,sha256) VALUES($1,$2,$3)", id, health.Faces.Model, hash); err != nil {
				return err
			}
			minScore, err := RetryMinScore(ctx, tx)
			if err != nil {
				return err
			}
			for _, face := range faces {
				vector := VectorText(face.Embedding)
				person, err := groupFor(ctx, tx, owner, health.Faces.Model, id, vector, face, minScore)
				if err != nil {
					return err
				}
				if _, err = tx.Exec(ctx, "INSERT INTO photo_faces(photo_id,person_id,box,embedding,score,retry) VALUES($1,$2,$3,$4::halfvec,$5,$6)",
					id, person, face.Box, vector, face.Score, face.Retry); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
