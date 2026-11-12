// Package jobs defines the background jobs of the application.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

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

// ProcessPhotoArgs queues the processing of an ingested photo or video.
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
	pool     *pgxpool.Pool
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
	video := storage.IsVideo(photo.Mime)

	meta, err := w.metadata.Read(path, video)
	if err != nil {
		// The photo is still usable without its tags.
		slog.Warn("read metadata", "photo", photo.ID, "error", err)
		meta = metadata.Metadata{}
	}

	stored, err := w.queries.GetPreviewSettings(ctx)
	if err != nil {
		return fmt.Errorf("load preview settings: %w", err)
	}
	settings := media.Settings{
		Format:      media.Format(stored.Format),
		WebPQuality: int(stored.WebpQuality),
		WebPEffort:  int(stored.WebpEffort),
		JPEGQuality: int(stored.JpegQuality),
		GridEdge:    int(stored.GridEdge),
		ViewerEdge:  int(stored.ViewerEdge),
	}

	var (
		previews      media.Previews
		width, height int
		duration      *float64
		hasMotion     bool
	)
	if video {
		previews, width, height, duration, err = renderVideo(ctx, path, settings)
		if err != nil {
			return err
		}
	} else {
		width, height, err = media.Dimensions(path)
		if err != nil {
			return &unreadableError{err}
		}
		previews, err = media.Render(path, settings)
		if err != nil {
			return &unreadableError{err}
		}
		hasMotion = w.extractMotion(ctx, photo.ID, path, meta.MotionTag)
	}

	format := stored.Format
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
		Duration:      duration,
		HasMotion:     hasMotion,
	})
	if err != nil {
		return fmt.Errorf("update photo: %w", err)
	}

	// The other file of a Live Photo may have come first.
	if meta.ContentIdentifier != "" {
		if _, err := PairLivePhotos(ctx, w.pool, &photo.OwnerID, &meta.ContentIdentifier); err != nil {
			// The pair_live_photos job tries again.
			slog.Warn("pair live photo", "photo", photo.ID, "error", err)
		}
	}
	return nil
}

// PairLivePhotos pairs the photos and videos of Live Photos that have no pair
// yet, those of one owner and identifier when they are given, and returns how
// many pairs it made. Pairing runs under a lock of its own, so files that
// finish processing at the same moment are paired once. It also gives back
// the motion to paired photos whose processing overwrote it while a pairing
// was being committed; processing calls it after every photo with the tag.
func PairLivePhotos(ctx context.Context, pool *pgxpool.Pool, ownerID *int64, contentIdentifier *string) (int, error) {
	paired := 0
	for {
		var rows []store.PairLivePhotosRow
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			queries := store.New(tx)
			if err := queries.LockLivePhotoPairing(ctx); err != nil {
				return err
			}
			var err error
			rows, err = queries.PairLivePhotos(ctx, store.PairLivePhotosParams{
				OwnerID:           ownerID,
				ContentIdentifier: contentIdentifier,
			})
			if err != nil || len(rows) > 0 {
				return err
			}
			return queries.RepairLivePhotoMotion(ctx, store.RepairLivePhotoMotionParams{
				OwnerID:           ownerID,
				ContentIdentifier: contentIdentifier,
			})
		})
		if err != nil {
			return paired, fmt.Errorf("pair live photos: %w", err)
		}
		if len(rows) == 0 {
			return paired, nil
		}
		paired += len(rows)
	}
}

// renderVideo makes the previews of a video from its poster frame. A missing
// ffmpeg or a cancelled job is worth a retry; anything else means the file
// cannot be read.
func renderVideo(ctx context.Context, path string, settings media.Settings) (media.Previews, int, int, *float64, error) {
	unreadable := func(err error) error {
		if media.ToolMissing(err) || ctx.Err() != nil {
			return err
		}
		return &unreadableError{err}
	}
	video, err := media.ProbeVideo(ctx, path)
	if err != nil {
		return media.Previews{}, 0, 0, nil, unreadable(err)
	}
	previews, width, height, err := media.RenderVideo(ctx, path, video, settings)
	if err != nil {
		return media.Previews{}, 0, 0, nil, unreadable(err)
	}
	var duration *float64
	if video.Duration > 0 {
		duration = &video.Duration
	}
	return previews, width, height, duration, nil
}

// extractMotion keeps the video of a motion photo next to its previews and
// tells whether there is one. The photo is still usable without it, so a
// failure only leaves the motion out.
func (w *processPhotoWorker) extractMotion(ctx context.Context, photoID int64, path, tag string) bool {
	if tag == "" {
		return false
	}
	data, err := metadata.ExtractMotion(ctx, path, tag)
	if err == nil {
		err = storage.WriteFileAtomic(w.layout.Motion(photoID), data)
	}
	if err != nil {
		slog.Warn("extract motion photo video", "photo", photoID, "error", err)
		return false
	}
	return true
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

// RenderGridArgs queues making the grid preview of a ready photo again, from
// its viewer preview, with the current grid preview size.
type RenderGridArgs struct {
	PhotoID int64 `json:"photo_id"`
}

// Kind implements river.JobArgs.
func (RenderGridArgs) Kind() string { return "render_grid" }

// InsertOpts implements river.JobArgsWithInsertOpts. New photos go first, and a
// photo already waiting is not queued twice.
func (RenderGridArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 3,
		Priority:    4,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
				rivertype.JobStateRunning, rivertype.JobStateScheduled,
			},
		},
	}
}

type renderGridWorker struct {
	river.WorkerDefaults[RenderGridArgs]
	queries *store.Queries
	layout  storage.Layout
}

func (w *renderGridWorker) Work(ctx context.Context, job *river.Job[RenderGridArgs]) error {
	photo, err := w.queries.GetPhoto(ctx, job.Args.PhotoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load photo: %w", err)
	}
	if photo.Status != "ready" || photo.PurgedAt != nil || photo.PreviewFormat == nil {
		return nil // Processing makes its previews, or it has none.
	}
	format := *photo.PreviewFormat

	viewer, err := os.ReadFile(w.layout.Preview(photo.ID, storage.PreviewViewer, format))
	if err != nil {
		return fmt.Errorf("read viewer preview: %w", err)
	}
	settings, err := w.queries.GetPreviewSettings(ctx)
	if err != nil {
		return fmt.Errorf("load preview settings: %w", err)
	}
	// The photo keeps the format of its viewer preview, which the database
	// records for both previews.
	grid, err := media.RenderGrid(viewer, media.Settings{
		Format:      media.Format(format),
		WebPQuality: int(settings.WebpQuality),
		WebPEffort:  int(settings.WebpEffort),
		JPEGQuality: int(settings.JpegQuality),
		GridEdge:    int(settings.GridEdge),
	})
	if err != nil {
		return fmt.Errorf("render grid preview of photo %d: %w", photo.ID, err)
	}
	if err := storage.WriteFileAtomic(w.layout.Preview(photo.ID, storage.PreviewGrid, format), grid.Data); err != nil {
		return fmt.Errorf("write grid preview: %w", err)
	}
	if err := w.queries.BumpPreviewVersion(ctx, photo.ID); err != nil {
		return fmt.Errorf("update photo: %w", err)
	}
	return nil
}

// QueueGridRenders queues a new grid preview for every ready photo and returns
// how many were queued. Running jobs are left to the server.
func QueueGridRenders(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	return queueForReadyPhotos(ctx, pool, func(id int64) river.JobArgs { return RenderGridArgs{PhotoID: id} })
}

// QueuePreviewRenders queues new previews from the original for every ready
// photo and returns how many were queued.
func QueuePreviewRenders(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	return queueForReadyPhotos(ctx, pool, func(id int64) river.JobArgs { return RenderPreviewsArgs{PhotoID: id} })
}

func queueForReadyPhotos(ctx context.Context, pool *pgxpool.Pool, args func(id int64) river.JobArgs) (int, error) {
	ids, err := store.New(pool).ListReadyPhotoIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list photos: %w", err)
	}
	client, err := NewInsertClient(pool)
	if err != nil {
		return 0, err
	}
	const batch = 1000
	for start := 0; start < len(ids); start += batch {
		params := make([]river.InsertManyParams, 0, batch)
		for _, id := range ids[start:min(start+batch, len(ids))] {
			params = append(params, river.InsertManyParams{Args: args(id)})
		}
		if _, err := client.InsertMany(ctx, params); err != nil {
			return start, fmt.Errorf("queue jobs: %w", err)
		}
	}
	return len(ids), nil
}

// RenderPreviewsArgs queues making both previews of a ready photo again, from
// its original, with the current settings. After a restore, this fills an
// empty preview folder.
type RenderPreviewsArgs struct {
	PhotoID int64 `json:"photo_id"`
}

// Kind implements river.JobArgs.
func (RenderPreviewsArgs) Kind() string { return "render_previews" }

// InsertOpts implements river.JobArgsWithInsertOpts. New photos go first, and a
// photo already waiting is not queued twice.
func (RenderPreviewsArgs) InsertOpts() river.InsertOpts {
	return RenderGridArgs{}.InsertOpts()
}

type renderPreviewsWorker struct {
	river.WorkerDefaults[RenderPreviewsArgs]
	process *processPhotoWorker
}

func (w *renderPreviewsWorker) Timeout(*river.Job[RenderPreviewsArgs]) time.Duration {
	return processTimeout
}

func (w *renderPreviewsWorker) Work(ctx context.Context, job *river.Job[RenderPreviewsArgs]) error {
	photo, err := w.process.queries.GetPhoto(ctx, job.Args.PhotoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load photo: %w", err)
	}
	if photo.Status != "ready" || photo.PurgedAt != nil {
		return nil // Processing makes its previews, or it has none.
	}

	err = w.process.process(ctx, photo)
	var unreadable *unreadableError
	if errors.As(err, &unreadable) || err != nil && job.Attempt >= job.MaxAttempts {
		// Such as an original missing after a restore: the photo shows its error.
		return w.process.fail(ctx, photo.ID, err)
	}
	if err != nil {
		return err
	}

	// A change of format leaves the old files behind.
	if old := photo.PreviewFormat; old != nil {
		if current, err := w.process.queries.GetPhoto(ctx, photo.ID); err == nil &&
			current.PreviewFormat != nil && *current.PreviewFormat != *old {
			for _, kind := range []string{storage.PreviewViewer, storage.PreviewGrid} {
				if err := os.Remove(w.process.layout.Preview(photo.ID, kind, *old)); err != nil && !errors.Is(err, os.ErrNotExist) {
					slog.Warn("remove old preview", "photo", photo.ID, "error", err)
				}
			}
		}
	}
	if err := w.process.queries.BumpPreviewVersion(ctx, photo.ID); err != nil {
		return fmt.Errorf("update photo: %w", err)
	}
	return nil
}

// BackfillMotionArgs queues the extraction of the videos of motion photos
// processed before the application kept them. It runs at every start and
// once a day, and finds only photos still without their motion, so a photo
// whose video cannot be extracted is tried again then.
type BackfillMotionArgs struct{}

// Kind implements river.JobArgs.
func (BackfillMotionArgs) Kind() string { return "backfill_motion" }

// InsertOpts implements river.JobArgsWithInsertOpts. New photos go first.
func (BackfillMotionArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Priority: 4}
}

type backfillMotionWorker struct {
	river.WorkerDefaults[BackfillMotionArgs]
	process *processPhotoWorker
}

// Timeout leaves room for a library with many motion photos. A job stopped
// by a restart starts over with the photos still left.
func (w *backfillMotionWorker) Timeout(*river.Job[BackfillMotionArgs]) time.Duration {
	return time.Hour
}

func (w *backfillMotionWorker) Work(ctx context.Context, _ *river.Job[BackfillMotionArgs]) error {
	queries, layout := w.process.queries, w.process.layout
	var after int64
	for {
		photos, err := queries.ListPhotosMissingMotion(ctx, store.ListPhotosMissingMotionParams{AfterID: after, MaxRows: 100})
		if err != nil {
			return fmt.Errorf("list photos: %w", err)
		}
		if len(photos) == 0 {
			return nil
		}
		for _, photo := range photos {
			after = photo.ID
			path := layout.Original(photo.OwnerID, photo.Sha256, photo.Mime)
			if !w.process.extractMotion(ctx, photo.ID, path, metadata.MotionTag(photo.Exif)) {
				continue
			}
			if err := queries.SetPhotoMotion(ctx, photo.ID); err != nil {
				return fmt.Errorf("update photo: %w", err)
			}
		}
	}
}

// PairLivePhotosArgs queues the pairing of Live Photos that processing did
// not pair: files processed before the application paired them, and files
// whose pairing failed. It runs at every start and once a day.
type PairLivePhotosArgs struct{}

// Kind implements river.JobArgs.
func (PairLivePhotosArgs) Kind() string { return "pair_live_photos" }

// InsertOpts implements river.JobArgsWithInsertOpts. New photos go first.
func (PairLivePhotosArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Priority: 4}
}

type pairLivePhotosWorker struct {
	river.WorkerDefaults[PairLivePhotosArgs]
	pool *pgxpool.Pool
}

func (w *pairLivePhotosWorker) Work(ctx context.Context, _ *river.Job[PairLivePhotosArgs]) error {
	paired, err := PairLivePhotos(ctx, w.pool, nil, nil)
	if paired > 0 {
		slog.Info("paired live photos", "pairs", paired)
	}
	return err
}

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
	// PeriodicJobs are started besides the cleanup.
	PeriodicJobs []*river.PeriodicJob
	// Queues are added to the default and import queues, such as the ML queue.
	Queues map[string]river.QueueConfig
}

// QueueImport runs folder scans. It is separate so that a scan takes no worker
// from photo processing, and has one worker because only one scan runs at a time.
const QueueImport = "import"

// NewClient returns a client that also runs jobs once started.
func NewClient(pool *pgxpool.Pool, cfg Config) (*river.Client[pgx.Tx], error) {
	queries := store.New(pool)

	workers := river.NewWorkers()
	process := &processPhotoWorker{pool: pool, queries: queries, layout: cfg.Layout, metadata: cfg.Metadata}
	river.AddWorker(workers, process)
	river.AddWorker(workers, &renderPreviewsWorker{process: process})
	river.AddWorker(workers, &renderGridWorker{queries: queries, layout: cfg.Layout})
	river.AddWorker(workers, &cleanupWorker{queries: queries, layout: cfg.Layout})
	river.AddWorker(workers, &backfillMotionWorker{process: process})
	river.AddWorker(workers, &pairLivePhotosWorker{pool: pool})
	if cfg.Register != nil {
		cfg.Register(workers)
	}

	queues := map[string]river.QueueConfig{
		river.QueueDefault: {MaxWorkers: cfg.Workers},
		QueueImport:        {MaxWorkers: 1},
	}
	maps.Copy(queues, cfg.Queues)
	periodic := append([]*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) { return CleanupArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) { return BackfillMotionArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) { return PairLivePhotosArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	}, cfg.PeriodicJobs...)

	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:               queues,
		Workers:              workers,
		RescueStuckJobsAfter: rescueAfter,
		PeriodicJobs:         periodic,
	})
}

// NewInsertClient returns a client that only queues jobs.
func NewInsertClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(pool), &river.Config{})
}
