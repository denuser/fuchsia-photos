// Package scan imports photos from a folder on the server into a user's library.
// The source folder is only ever read.
package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/riverqueue/river"

	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/jobs"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const (
	listBatch    = 1000
	processBatch = 20
	// How long one attempt of the job works before it reschedules itself.
	workSlice = 5 * time.Minute
	// A scan interrupted by a crash resumes after this long. It must leave room to
	// list a large folder on a slow disk, which happens in one attempt.
	jobTimeout  = 30 * time.Minute
	maxAttempts = 10

	maxFolderDepth = 3
	maxFolders     = 1000

	pgUniqueViolation = "23505"

	statusListing   = "listing"
	statusImporting = "importing"
)

var (
	// ErrRunning means another scan has not finished.
	ErrRunning = errors.New("another scan is running")
	// ErrNoFolder means the folder is not a folder inside the import root.
	ErrNoFolder = errors.New("no such folder in the import folder")
	// ErrNoOwner means the owner does not exist or cannot log in.
	ErrNoOwner = errors.New("no such active user")
)

// Service starts and runs folder scans.
type Service struct {
	Ingest *ingest.Service
	// Dir is the import root. Nothing outside it can be scanned.
	Dir string
	// Stable is how long a file's size and modification time must stay the same
	// before it counts as completely copied.
	Stable time.Duration
}

// Args queues the work of one scan.
type Args struct {
	RunID int64 `json:"run_id"`
}

// Kind implements river.JobArgs.
func (Args) Kind() string { return "folder_scan" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (Args) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.QueueImport, MaxAttempts: maxAttempts}
}

// Register adds the scan worker to a job client.
func (s *Service) Register(workers *river.Workers) {
	river.AddWorker(workers, &worker{service: s})
}

func (s *Service) queries() *store.Queries {
	return store.New(s.Ingest.Pool)
}

// Start creates a run and queues its job in one transaction.
func (s *Service) Start(ctx context.Context, startedBy, ownerID int64, folder string) (store.ImportRun, error) {
	folder = strings.Trim(folder, "/")
	if err := s.checkFolder(folder); err != nil {
		return store.ImportRun{}, err
	}
	owner, err := s.queries().GetUser(ctx, ownerID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner.Disabled) {
		return store.ImportRun{}, ErrNoOwner
	}
	if err != nil {
		return store.ImportRun{}, fmt.Errorf("look up owner: %w", err)
	}

	tx, err := s.Ingest.Pool.Begin(ctx)
	if err != nil {
		return store.ImportRun{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	run, err := store.New(tx).CreateImportRun(ctx, store.CreateImportRunParams{OwnerID: ownerID, StartedBy: startedBy, Folder: folder})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return store.ImportRun{}, ErrRunning
	}
	if err != nil {
		return store.ImportRun{}, fmt.Errorf("create run: %w", err)
	}
	if _, err := s.Ingest.Jobs.InsertTx(ctx, tx, Args{RunID: run.ID}, nil); err != nil {
		return store.ImportRun{}, fmt.Errorf("queue scan: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return store.ImportRun{}, fmt.Errorf("commit: %w", err)
	}
	return run, nil
}

func (s *Service) checkFolder(folder string) error {
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return fmt.Errorf("open import folder: %w", err)
	}
	defer root.Close()

	// A link is not followed, so the check agrees with what a scan would walk.
	info, err := root.Lstat(rootPath(folder))
	if err != nil || !info.IsDir() {
		return ErrNoFolder
	}
	return nil
}

func rootPath(folder string) string {
	if folder == "" {
		return "."
	}
	return folder
}

// Folders lists the folders that can be chosen for a scan, without the root.
func (s *Service) Folders() ([]string, error) {
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return nil, fmt.Errorf("open import folder: %w", err)
	}
	defer root.Close()

	folders := []string{}
	err = fs.WalkDir(root.FS(), ".", func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			if p == "." {
				return err
			}
			return fs.SkipDir // An unreadable folder is reported when it is scanned.
		}
		if p == "." || !entry.IsDir() {
			return nil
		}
		if skipped(entry.Name()) || !utf8.ValidString(p) {
			return fs.SkipDir
		}
		folders = append(folders, p)
		if len(folders) >= maxFolders {
			return fs.SkipAll
		}
		if strings.Count(p, "/")+1 >= maxFolderDepth {
			return fs.SkipDir
		}
		return nil
	})
	return folders, err
}

// skipped recognizes what is not the user's own content: hidden files, including
// the partial files of rsync and the metadata of macOS, and the service folders
// NAS systems keep next to files, such as thumbnails in @eaDir and #recycle.
func skipped(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@") || strings.HasPrefix(name, "#")
}

// sidecar recognizes files that describe a photo rather than hold one, such as
// the edits Apple Photos exports next to an original in an .aae file, like the
// range a slow-motion video plays slowed. They are left out without a report.
func sidecar(name string) bool {
	return strings.EqualFold(path.Ext(name), ".aae")
}

type worker struct {
	river.WorkerDefaults[Args]
	service *Service
}

func (w *worker) Timeout(*river.Job[Args]) time.Duration { return jobTimeout }

func (w *worker) Work(ctx context.Context, job *river.Job[Args]) error {
	wait, more, err := w.service.step(ctx, job.Args.RunID, workSlice)
	if err != nil {
		if job.Attempt < job.MaxAttempts {
			return err
		}
		// The context may be why the job failed, and the run must still be closed.
		if err := w.service.failRun(context.WithoutCancel(ctx), job.Args.RunID, err); err != nil {
			slog.Error("folder scan", "run", job.Args.RunID, "error", err)
		}
		return river.JobCancel(err)
	}
	if more {
		return river.JobSnooze(max(wait, time.Second))
	}
	return nil
}

// step does one slice of work. It reports how long to wait before the next one and
// whether there is one. All progress is in the database, so a step that was
// interrupted loses nothing.
func (s *Service) step(ctx context.Context, runID int64, slice time.Duration) (wait time.Duration, more bool, err error) {
	queries := s.queries()
	run, err := queries.GetImportRun(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("load run: %w", err)
	}

	switch run.Status {
	case statusListing:
		return s.list(ctx, run)
	case statusImporting:
		return s.process(ctx, run, slice)
	}
	return 0, false, queries.DeletePendingImportFiles(ctx, runID)
}

// list records every file with its size and modification time. Comparing them
// with the file's state after the wait is what shows that it was not being copied.
func (s *Service) list(ctx context.Context, run store.ImportRun) (time.Duration, bool, error) {
	queries := s.queries()
	if err := queries.ResetImportListing(ctx, run.ID); err != nil {
		return 0, false, fmt.Errorf("reset listing: %w", err)
	}

	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return 0, false, s.failRun(ctx, run.ID, fmt.Errorf("open import folder: %w", err))
	}
	defer root.Close()

	batch := store.AddImportFilesParams{RunID: run.ID}
	flush := func() error {
		if len(batch.Paths) == 0 {
			return ctx.Err()
		}
		if err := queries.AddImportFiles(ctx, batch); err != nil {
			return fmt.Errorf("record files: %w", err)
		}
		batch.Paths, batch.Sizes, batch.Mtimes = nil, nil, nil
		return ctx.Err()
	}

	start := rootPath(run.Folder)
	var rootErr error
	err = fs.WalkDir(root.FS(), start, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			if p == start {
				rootErr = err
				return fs.SkipAll
			}
			return s.addProblem(ctx, run.ID, p, err)
		}
		if p != start && skipped(entry.Name()) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.IsDir() && sidecar(entry.Name()) {
			return nil
		}
		// Links are left out, so that they cannot bring in a file twice.
		if !entry.Type().IsRegular() {
			return nil
		}
		if !utf8.ValidString(p) {
			return s.addProblem(ctx, run.ID, p, errors.New("the file name is not valid UTF-8"))
		}
		info, err := entry.Info()
		if err != nil {
			return s.addProblem(ctx, run.ID, p, err)
		}
		batch.Paths = append(batch.Paths, p)
		batch.Sizes = append(batch.Sizes, info.Size())
		batch.Mtimes = append(batch.Mtimes, info.ModTime())
		if len(batch.Paths) >= listBatch {
			return flush()
		}
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	if rootErr != nil {
		return 0, false, s.failRun(ctx, run.ID, fmt.Errorf("read folder: %w", rootErr))
	}
	if err := flush(); err != nil {
		return 0, false, err
	}

	settleAt := time.Now().Add(s.Stable)
	if err := queries.StartImporting(ctx, store.StartImportingParams{ID: run.ID, SettleAt: &settleAt}); err != nil {
		return 0, false, fmt.Errorf("start importing: %w", err)
	}
	return s.Stable, true, nil
}

func (s *Service) addProblem(ctx context.Context, runID int64, p string, cause error) error {
	err := s.queries().AddImportProblem(ctx, store.AddImportProblemParams{
		RunID: runID,
		Path:  strings.ToValidUTF8(p, "?"),
		Error: new(cause.Error()),
	})
	if err != nil {
		return fmt.Errorf("record problem: %w", err)
	}
	return nil
}

func (s *Service) failRun(ctx context.Context, runID int64, cause error) error {
	slog.Error("folder scan", "run", runID, "error", cause)
	if err := s.queries().FailImportRun(ctx, store.FailImportRunParams{ID: runID, Error: new(cause.Error())}); err != nil {
		return fmt.Errorf("fail run: %w", err)
	}
	return nil
}

func (s *Service) process(ctx context.Context, run store.ImportRun, slice time.Duration) (time.Duration, bool, error) {
	if run.SettleAt != nil {
		if wait := time.Until(*run.SettleAt); wait > 0 {
			return wait, true, nil
		}
	}
	queries := s.queries()
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return 0, false, s.failRun(ctx, run.ID, fmt.Errorf("open import folder: %w", err))
	}
	defer root.Close()

	deadline := time.Now().Add(slice)
	for {
		// Reading the run again notices a request to stop.
		current, err := queries.GetImportRun(ctx, run.ID)
		if err != nil {
			return 0, false, fmt.Errorf("load run: %w", err)
		}
		if current.Status != statusImporting {
			return 0, false, queries.DeletePendingImportFiles(ctx, run.ID)
		}

		files, err := queries.ListPendingImportFiles(ctx, store.ListPendingImportFilesParams{RunID: run.ID, MaxRows: processBatch})
		if err != nil {
			return 0, false, fmt.Errorf("list files: %w", err)
		}
		if len(files) == 0 {
			return 0, false, queries.FinishImportRun(ctx, run.ID)
		}
		for _, file := range files {
			if err := s.importFile(ctx, root, run, file); err != nil {
				return 0, false, err
			}
		}
		if !time.Now().Before(deadline) {
			return 0, true, nil
		}
	}
}

// importFile returns an error only for a problem with the scan itself. A problem
// with the file goes into the report.
func (s *Service) importFile(ctx context.Context, root *os.Root, run store.ImportRun, listed store.ListPendingImportFilesRow) error {
	queries := s.queries()
	failed := func(cause error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		slog.Warn("import file", "run", run.ID, "path", listed.Path, "error", cause)
		return queries.MarkImportFileFailed(ctx, store.MarkImportFileFailedParams{RunID: run.ID, Path: listed.Path, Error: new(cause.Error())})
	}
	finish := func(outcome string) error {
		return queries.FinishImportFile(ctx, store.FinishImportFileParams{RunID: run.ID, Path: listed.Path, Outcome: outcome})
	}

	file, err := root.Open(listed.Path)
	if err != nil {
		return failed(err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return failed(err)
	}
	if !info.Mode().IsRegular() {
		return failed(errors.New("not a regular file"))
	}
	if info.Size() != listed.Size || !sameTime(info.ModTime(), listed.Mtime) {
		return queries.MarkImportFileChanging(ctx, store.MarkImportFileChangingParams{RunID: run.ID, Path: listed.Path})
	}

	res, err := s.Ingest.Import(ctx, run.OwnerID, file, path.Base(listed.Path))
	switch {
	case errors.Is(err, ingest.ErrUnsupported):
		return finish("unsupported")
	case err != nil:
		return failed(err)
	case res.Purged:
		return finish("purged")
	case res.InTrash:
		return finish("in_trash")
	case res.Duplicate:
		return finish("duplicate")
	}
	return finish("added")
}

// sameTime compares at the precision of the database, which keeps microseconds.
func sameTime(a, b time.Time) bool {
	return a.Truncate(time.Microsecond).Equal(b.Truncate(time.Microsecond))
}
