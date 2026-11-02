// Package jobs defines the background jobs of the application.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/denuser/fuchsia-photos/backend/internal/media"
	"github.com/denuser/fuchsia-photos/backend/internal/metadata"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const (
	processTimeout = 2 * time.Minute
	// A job that was running when the NAS went down is picked up after this long.
	// It must be longer than the job timeout.
	rescueAfter = 3 * time.Minute

	tempFileMaxAge = 24 * time.Hour
)

// ProcessPhotoArgs queues the processing of an ingested photo.
type ProcessPhotoArgs struct {
	PhotoID int64 `json:"photo_id"`
}

// Kind implements river.JobArgs.
func (ProcessPhotoArgs) Kind() string { return "process_photo" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (ProcessPhotoArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

type processPhotoWorker struct {
	river.WorkerDefaults[ProcessPhotoArgs]
	queries  *store.Queries
	layout   storage.Layout
	metadata *metadata.Reader
}

func (w *processPhotoWorker) Timeout(*river.Job[ProcessPhotoArgs]) time.Duration {
	return processTimeout
}

func (w *processPhotoWorker) Work(ctx context.Context, job *river.Job[ProcessPhotoArgs]) error {
	photo, err := w.queries.GetPhoto(ctx, job.Args.PhotoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // Deleted while waiting in the queue.
	}
	if err != nil {
		return fmt.Errorf("load photo: %w", err)
	}
	if photo.PurgedAt != nil {
		return nil // Its original is gone.
	}

	err = w.process(ctx, photo)
	var unreadable *unreadableError
	if errors.As(err, &unreadable) {
		// Retrying cannot repair a broken file.
		return w.fail(ctx, photo.ID, err)
	}
	if err != nil && job.Attempt >= job.MaxAttempts {
		return w.fail(ctx, photo.ID, err)
	}
	return err
}

func (w *processPhotoWorker) process(ctx context.Context, photo store.Photo) error {
	path := w.layout.Original(photo.OwnerID, photo.Sha256, photo.Mime)

	meta, err := w.metadata.Read(path)
	if err != nil {
		// The photo is still usable without its tags.
		slog.Warn("read metadata", "photo", photo.ID, "error", err)
		meta = metadata.Metadata{}
	}

	width, height, err := media.Dimensions(path)
	if err != nil {
		return &unreadableError{err}
	}
	settings, err := w.queries.GetPreviewSettings(ctx)
	if err != nil {
		return fmt.Errorf("load preview settings: %w", err)
	}
	previews, err := media.Render(path, media.Settings{
		Format:      media.Format(settings.Format),
		WebPQuality: int(settings.WebpQuality),
		WebPEffort:  int(settings.WebpEffort),
		JPEGQuality: int(settings.JpegQuality),
		GridEdge:    int(settings.GridEdge),
		ViewerEdge:  int(settings.ViewerEdge),
	})
	if err != nil {
		return &unreadableError{err}
	}

	format := settings.Format
	for _, p := range []struct {
		kind    string
		preview media.Preview
	}{
		{storage.PreviewViewer, previews.Viewer},
		{storage.PreviewGrid, previews.Grid},
	} {
		if err := storage.WriteFileAtomic(w.layout.Preview(photo.ID, p.kind, format), p.preview.Data); err != nil {
			return fmt.Errorf("write %s preview: %w", p.kind, err)
		}
	}

	w32, h32 := int32(width), int32(height)
	err = w.queries.MarkPhotoReady(ctx, store.MarkPhotoReadyParams{
		ID:            photo.ID,
		Width:         &w32,
		Height:        &h32,
		TakenAt:       meta.TakenAt,
		Exif:          meta.Tags,
		PreviewFormat: &format,
	})
	if err != nil {
		return fmt.Errorf("update photo: %w", err)
	}
	return nil
}

func (w *processPhotoWorker) fail(ctx context.Context, photoID int64, cause error) error {
	slog.Error("process photo", "photo", photoID, "error", cause)
	message := cause.Error()
	if err := w.queries.MarkPhotoFailed(ctx, store.MarkPhotoFailedParams{ID: photoID, Error: &message}); err != nil {
		return fmt.Errorf("mark photo failed: %w", err)
	}
	return river.JobCancel(cause)
}

type unreadableError struct{ err error }

func (e *unreadableError) Error() string { return e.err.Error() }
func (e *unreadableError) Unwrap() error { return e.err }

// CleanupArgs queues routine housekeeping.
type CleanupArgs struct{}

// Kind implements river.JobArgs.
func (CleanupArgs) Kind() string { return "cleanup" }

type cleanupWorker struct {
	river.WorkerDefaults[CleanupArgs]
	queries *store.Queries
	layout  storage.Layout
}

// Work removes expired sessions and abandoned uploads. It deliberately leaves
// originals without a database record alone: after restoring an older database
// backup, those are the photos uploaded since that backup.
func (w *cleanupWorker) Work(ctx context.Context, _ *river.Job[CleanupArgs]) error {
	if err := w.queries.DeleteExpiredSessions(ctx); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return removeOldFiles(w.layout.TempDir(), time.Now().Add(-tempFileMaxAge))
}

func removeOldFiles(dir string, before time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.ModTime().After(before) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// Config configures the job client.
type Config struct {
	Layout   storage.Layout
	Metadata *metadata.Reader
	Workers  int
	// Register adds workers that live in packages depending on this one.
	Register func(*river.Workers)
}

// QueueImport runs folder scans. It is separate so that a scan takes no worker
// from photo processing, and has one worker because only one scan runs at a time.
const QueueImport = "import"

// NewClient returns a client that also runs jobs once started.
func NewClient(pool *pgxpool.Pool, cfg Config) (*river.Client[pgx.Tx], error) {
	queries := store.New(pool)

	workers := river.NewWorkers()
	river.AddWorker(workers, &processPhotoWorker{queries: queries, layout: cfg.Layout, metadata: cfg.Metadata})
	river.AddWorker(workers, &cleanupWorker{queries: queries, layout: cfg.Layout})
	if cfg.Register != nil {
		cfg.Register(workers)
	}

	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: cfg.Workers},
			QueueImport:        {MaxWorkers: 1},
		},
		Workers:              workers,
		RescueStuckJobsAfter: rescueAfter,
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(24*time.Hour),
				func() (river.JobArgs, *river.InsertOpts) { return CleanupArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true},
			),
		},
	})
}

// NewInsertClient returns a client that only queues jobs.
func NewInsertClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(pool), &river.Config{})
}
