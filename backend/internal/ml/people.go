package ml

import (
	"context"
	"errors"
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
func PeopleJob() *river.PeriodicJob {
	return river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) { return FaceScanArgs{}, nil }, &river.PeriodicJobOpts{RunOnStart: true})
}

type FaceWorker struct {
	river.WorkerDefaults[FaceScanArgs]
	Client *Client
	Pool   *pgxpool.Pool
	Layout storage.Layout
}

func (w *FaceWorker) Timeout(*river.Job[FaceScanArgs]) time.Duration { return 3 * time.Minute }
func (w *FaceWorker) Work(ctx context.Context, _ *river.Job[FaceScanArgs]) error {
	if w.Client == nil || w.Client.token == "" {
		return nil
	}
	health, err := w.Client.Health(ctx)
	if err != nil || health.Faces == nil {
		return nil
	}
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		var id, owner int64
		var hash []byte
		var format string
		err = w.Pool.QueryRow(ctx, `SELECT p.id,p.owner_id,p.sha256,p.preview_format FROM photos p
   WHERE p.status='ready' AND p.deleted_at IS NULL AND p.purged_at IS NULL AND p.preview_format IS NOT NULL
   AND p.mime NOT LIKE 'video/%' AND NOT EXISTS(SELECT FROM photo_links WHERE hidden_id=p.id)
   AND NOT EXISTS(SELECT FROM face_scans s WHERE s.photo_id=p.id AND s.sha256=p.sha256
   AND (s.retry_at IS NULL OR (s.model=$1 AND s.retry_at>now()))) ORDER BY p.id LIMIT 1`, health.Faces.Model).Scan(&id, &owner, &hash, &format)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		data, readErr := os.ReadFile(w.Layout.Preview(id, storage.PreviewViewer, format))
		var faces []DetectedFace
		if readErr == nil {
			faces, readErr = w.Client.DetectFaces(ctx, data, *health.Faces)
		}
		err = pgx.BeginFunc(ctx, w.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", owner); err != nil {
				return err
			}
			var current int64
			err := tx.QueryRow(ctx, `SELECT id FROM photos WHERE id=$1 AND owner_id=$2 AND sha256=$3 AND status='ready' AND deleted_at IS NULL AND purged_at IS NULL AND NOT EXISTS(SELECT FROM photo_links WHERE hidden_id=$1) FOR UPDATE`, id, owner, hash).Scan(&current)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			var complete bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM face_scans WHERE photo_id=$1 AND sha256=$2 AND retry_at IS NULL)", id, hash).Scan(&complete); err != nil || complete {
				return err
			}
			if readErr != nil {
				_, err = tx.Exec(ctx, `INSERT INTO face_scans(photo_id,model,sha256,retry_at,error) VALUES($1,$2,$3,now()+interval '10 minutes',$4)
      ON CONFLICT(photo_id) DO UPDATE SET retry_at=excluded.retry_at,model=excluded.model,error=excluded.error,scanned_at=now() WHERE face_scans.retry_at IS NOT NULL`, id, health.Faces.Model, hash, readErr.Error())
				return err
			}
			if _, err = tx.Exec(ctx, "DELETE FROM face_scans WHERE photo_id=$1", id); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "INSERT INTO face_scans(photo_id,model,sha256) VALUES($1,$2,$3)", id, health.Faces.Model, hash); err != nil {
				return err
			}
			for _, face := range faces {
				vector := VectorText(face.Embedding)
				var person int64
				// Match the oldest visible face of each group. An uncertain match starts a new group.
				err := tx.QueryRow(ctx, `WITH anchors AS (SELECT DISTINCT ON (person_id) person_id,embedding FROM visible_photo_faces
        WHERE owner_id=$1 AND model=$2 AND person_id IS NOT NULL ORDER BY person_id,id),
        ranked AS (SELECT person_id,embedding <=> $3::halfvec AS distance FROM anchors ORDER BY distance LIMIT 2),
        nearest AS (SELECT *,lead(distance) OVER(ORDER BY distance) AS second FROM ranked ORDER BY distance LIMIT 1)
        SELECT person_id FROM nearest WHERE distance<0.45 AND (second IS NULL OR second-distance>0.06)`, owner, health.Faces.Model, vector).Scan(&person)
				if errors.Is(err, pgx.ErrNoRows) {
					err = tx.QueryRow(ctx, "INSERT INTO people(owner_id) VALUES($1) RETURNING id", owner).Scan(&person)
				}
				if err != nil {
					return err
				}
				if _, err = tx.Exec(ctx, "INSERT INTO photo_faces(photo_id,person_id,box,embedding) VALUES($1,$2,$3,$4::halfvec)", id, person, face.Box, vector); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if readErr != nil {
			return readErr
		}
	}
	return nil
}
