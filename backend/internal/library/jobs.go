package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

// Queue runs the scans of library folders. It has one worker, so a first scan of
// a large folder does not take workers from photo processing or from imports.
const Queue = "library"

// scanTimeout ends a scan that runs long, and it is also how long a scan that a crash left
// as running keeps its folder from being queued again, as River counts it as running until its
// timeout. A scan stopped at the limit records nothing and is due again a minute later, and
// continues with what is still unknown, as the files it recorded are not read again, so a first
// scan of a large share takes several runs.
const scanTimeout = 30 * time.Minute

// ScanArgs queues the scan of one library folder.
type ScanArgs struct {
	FolderID int64 `json:"folder_id"`
}

// Kind implements river.JobArgs.
func (ScanArgs) Kind() string { return "library_scan" }

// InsertOpts implements river.JobArgsWithInsertOpts. A folder is not queued
// while a scan of it waits or runs, so a slow scan is never started twice.
func (ScanArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       Queue,
		MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
			rivertype.JobStateAvailable,
			rivertype.JobStatePending,
			rivertype.JobStateRunning,
			rivertype.JobStateScheduled,
		}},
	}
}

// DueArgs queues the scans of the folders whose interval has passed.
type DueArgs struct{}

// Kind implements river.JobArgs.
func (DueArgs) Kind() string { return "library_due" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (DueArgs) InsertOpts() river.InsertOpts {
	// The states are listed because the default includes completed: with no arguments
	// to tell the runs apart, every run after the first would be dropped until River
	// cleans out the finished one, about a day later.
	return river.InsertOpts{MaxAttempts: 1, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable,
		rivertype.JobStatePending,
		rivertype.JobStateRunning,
		rivertype.JobStateScheduled,
	}}}
}

// PeriodicJob looks for folders to scan every minute; the interval of a folder
// is at least a minute.
func PeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(time.Minute),
		func() (river.JobArgs, *river.InsertOpts) { return DueArgs{}, nil },
		&river.PeriodicJobOpts{RunOnStart: true},
	)
}

// Register adds the workers to a job client.
func (s *Service) Register(workers *river.Workers) {
	river.AddWorker(workers, &scanWorker{service: s})
	river.AddWorker(workers, &dueWorker{service: s})
}

// Queue schedules the scan of a folder now.
func (s *Service) Queue(ctx context.Context, folderID int64) error {
	if _, err := s.Ingest.Jobs.Insert(ctx, ScanArgs{FolderID: folderID}, nil); err != nil {
		return fmt.Errorf("queue scan: %w", err)
	}
	return nil
}

type scanWorker struct {
	river.WorkerDefaults[ScanArgs]
	service *Service
}

func (w *scanWorker) Timeout(*river.Job[ScanArgs]) time.Duration { return scanTimeout }

func (w *scanWorker) Work(ctx context.Context, job *river.Job[ScanArgs]) error {
	res, err := w.service.Scan(ctx, job.Args.FolderID)
	switch {
	case errors.Is(err, ErrRunning), errors.Is(err, pgx.ErrNoRows):
		return nil // Another scan has it, or the folder was removed.
	case err != nil:
		// The scan recorded why it stopped; the next interval tries again.
		slog.Warn("library scan", "folder", job.Args.FolderID, "error", err)
		return nil
	}
	slog.Info("library scan", "folder", job.Args.FolderID, "listed", res.Listed, "added", res.Added,
		"moved", res.Moved, "replaced", res.Replaced, "missing", res.Missing)
	return nil
}

type dueWorker struct {
	river.WorkerDefaults[DueArgs]
	service *Service
}

func (w *dueWorker) Work(ctx context.Context, _ *river.Job[DueArgs]) error {
	folders, err := store.New(w.service.Ingest.Pool).ListLibraryFoldersDue(ctx)
	if err != nil {
		return fmt.Errorf("list folders: %w", err)
	}
	for _, folder := range folders {
		if err := w.service.Queue(ctx, folder.ID); err != nil {
			return err
		}
	}
	return nil
}
