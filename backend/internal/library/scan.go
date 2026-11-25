// Package library scans library folders: folders of the folders mount whose
// photos are read where they are and never copied into the library. A file's
// identity is its path, so that an edit in another program, which changes the
// file and its hash, does not make a new photo.
package library

import (
	"context"
	"encoding/json"
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
	"github.com/riverqueue/river"

	"github.com/magellanica-app/magellanica-photos/backend/internal/ingest"
	"github.com/magellanica-app/magellanica-photos/backend/internal/jobs"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

const (
	stateSource    = "source"
	stateDuplicate = "duplicate"

	// A file is missing after this many scans in a row did not find it, so that
	// a program that deletes a file before it writes it again is not caught
	// in between.
	missedToMissing = 2

	// The lock of a folder's scan, in the advisory locks' own number space.
	lockKey = 0x6c696272 // "libr"
)

var (
	// ErrRunning means another scan of the folder has not finished.
	ErrRunning = errors.New("another scan of the folder is running")
	// ErrFolderMissing means the folder is not a folder inside the folders mount,
	// as when the share is not mounted.
	ErrFolderMissing = errors.New("the library folder is not there")
	// ErrOverlap means the folder is, holds or lies inside another library folder.
	ErrOverlap = errors.New("the folder overlaps another library folder")
	// ErrEmpty means the folder lists no files although it had some, which a
	// share that is not mounted looks like. Nothing is marked missing.
	ErrEmpty = errors.New("the library folder is empty, though it held files")
)

// Service scans library folders.
type Service struct {
	Ingest *ingest.Service
	// Stable is how long a new file's modification time must lie in the past
	// before the file counts as completely copied.
	Stable time.Duration
}

// Result counts what a scan found. Listed is the number of files the scan
// considered. Added are new photos; Moved are files found under a new path;
// Duplicates repeat a photo the owner already has, and InTrash and Purged repeat
// one in the trash or deleted permanently. Unsupported files are not photos or
// videos; Waiting ones are too new to be complete; Failed ones could not be read.
// Changed files are known files whose size or modification time differ: Replaced
// ones held new content and their photo is processed again, Conflicts held the
// content of another photo and were left as they are. Returned files were away and
// are back; Missing ones were not found; Trashed are the photos the scan moved to
// the trash for it.
type Result struct {
	Listed      int `json:"listed"`
	Added       int `json:"added"`
	Moved       int `json:"moved"`
	Duplicates  int `json:"duplicates"`
	InTrash     int `json:"inTrash"`
	Purged      int `json:"purged"`
	Unsupported int `json:"unsupported"`
	Waiting     int `json:"waiting"`
	Failed      int `json:"failed"`
	Changed     int `json:"changed"`
	Replaced    int `json:"replaced"`
	Conflicts   int `json:"conflicts"`
	Returned    int `json:"returned"`
	Missing     int `json:"missing"`
	Trashed     int `json:"trashed"`
}

type listed struct {
	path  string // Relative to the library folder.
	size  int64
	mtime time.Time
}

// Scan reads a folder once: new files are added, known files are compared by
// size and modification time without being read, and files that stay away are
// reported missing.
func (s *Service) Scan(ctx context.Context, folderID int64) (Result, error) {
	conn, err := s.Ingest.Pool.Acquire(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1, $2)`, lockKey, int32(folderID)).Scan(&locked); err != nil {
		return Result{}, fmt.Errorf("lock folder: %w", err)
	}
	if !locked {
		return Result{}, ErrRunning
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1, $2)`, lockKey, int32(folderID))

	queries := store.New(s.Ingest.Pool)
	folder, err := queries.GetLibraryFolder(ctx, folderID)
	if err != nil {
		return Result{}, fmt.Errorf("look up folder: %w", err)
	}
	run := &scanRun{Service: s, queries: queries, folder: folder, seen: map[string]bool{}, moved: map[int64]bool{}}
	err = run.scan(ctx)
	// The page shows how the last scan ended. A scan stopped by a restart is not
	// recorded, so that it is due again at once.
	if ctx.Err() == nil {
		if recordErr := run.recordScan(context.WithoutCancel(ctx), err); recordErr != nil && err == nil {
			err = recordErr
		}
	}
	return run.result, err
}

func (r *scanRun) recordScan(ctx context.Context, scanErr error) error {
	params := store.FinishLibraryScanParams{ID: r.folder.ID}
	if scanErr != nil {
		params.LastError = new(scanErr.Error())
	}
	result, err := json.Marshal(r.result)
	if err != nil {
		return fmt.Errorf("record scan: %w", err)
	}
	params.LastResult = result
	if err := r.queries.FinishLibraryScan(ctx, params); err != nil {
		return fmt.Errorf("record scan: %w", err)
	}
	return nil
}

type scanRun struct {
	*Service
	queries *store.Queries
	folder  store.LibraryFolder
	root    *os.Root
	result  Result
	rows    map[string]store.PhotoFile
	seen    map[string]bool
	moved   map[int64]bool
}

func (r *scanRun) scan(ctx context.Context) error {
	root, err := os.OpenRoot(r.Ingest.Layout.Folders)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFolderMissing, err)
	}
	defer root.Close()
	r.root = root

	info, err := root.Lstat(r.folder.Path)
	if err != nil || !info.IsDir() {
		return ErrFolderMissing
	}
	files, complete, err := r.list(ctx)
	if err != nil {
		return err
	}

	rows, err := r.queries.ListPhotoFiles(ctx, r.folder.ID)
	if err != nil {
		return fmt.Errorf("list known files: %w", err)
	}
	if len(files) == 0 && len(rows) > 0 {
		return ErrEmpty
	}
	r.rows = make(map[string]store.PhotoFile, len(rows))
	for _, row := range rows {
		r.rows[row.Path] = row
	}
	for _, f := range files {
		r.seen[f.path] = true
	}

	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		r.result.Listed++
		var err error
		if row, known := r.rows[f.path]; known {
			err = r.known(ctx, row, f)
		} else {
			err = r.add(ctx, f)
		}
		if err != nil {
			return err
		}
	}
	if !complete {
		// A part of the folder could not be read, so what is not listed may only be unreadable.
		slog.Warn("library scan: part of the folder was unreadable, so no file is marked missing", "folder", r.folder.Path)
		return nil
	}
	return r.missing(ctx, rows)
}

// list walks the folder. complete is false if a folder inside it could not be read.
func (r *scanRun) list(ctx context.Context) (files []listed, complete bool, err error) {
	complete = true
	err = fs.WalkDir(r.root.FS(), r.folder.Path, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			if p == r.folder.Path {
				return err
			}
			complete = false
			return fs.SkipDir
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if p != r.folder.Path && skipped(entry.Name()) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// Links are left out, so that they cannot bring in a file twice.
		if entry.IsDir() || !entry.Type().IsRegular() || sidecar(entry.Name()) {
			return nil
		}
		rel := strings.TrimPrefix(p, r.folder.Path+"/")
		if !utf8.ValidString(rel) {
			complete = false // Nothing to compare it with later; leave it out and trust nothing missing.
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			complete = false
			return nil
		}
		files = append(files, listed{rel, info.Size(), info.ModTime()})
		return nil
	})
	if err != nil {
		if errors.Is(err, ctx.Err()) && ctx.Err() != nil {
			return nil, false, err
		}
		return nil, false, fmt.Errorf("%w: %v", ErrFolderMissing, err)
	}
	return files, complete, nil
}

// skipped recognizes what is not the user's own content: hidden files, such as
// the partial files of rsync and the metadata of macOS, and the service
// folders NAS systems keep next to files, such as @eaDir and #recycle.
func skipped(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@") || strings.HasPrefix(name, "#")
}

// sidecar recognizes files that describe a photo rather than hold one. The
// XMP sidecars of Lightroom are read by a later improvement.
func sidecar(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	return ext == ".aae" || ext == ".xmp"
}

func (r *scanRun) sourcePath(rel string) string {
	return r.folder.Path + "/" + rel
}

func sameTime(a, b time.Time) bool {
	return a.Truncate(time.Microsecond).Equal(b.Truncate(time.Microsecond))
}

// known handles a file at a path the folder has seen before. It never reads the file.
func (r *scanRun) known(ctx context.Context, row store.PhotoFile, f listed) error {
	changed := row.Size != f.size || !sameTime(row.Mtime, f.mtime)
	if row.State == stateDuplicate {
		if !changed {
			return nil
		}
		// What a duplicate was may no longer be what is there: look at it as a new file.
		if err := r.queries.DeletePhotoFiles(ctx, []int64{row.ID}); err != nil {
			return fmt.Errorf("forget file: %w", err)
		}
		delete(r.rows, f.path)
		return r.add(ctx, f)
	}
	if row.MissedScans > 0 || row.MissingSince != nil {
		if err := r.queries.SeePhotoFile(ctx, row.ID); err != nil {
			return fmt.Errorf("record file: %w", err)
		}
		r.result.Returned++
		if row.PhotoID != nil {
			if err := r.returned(ctx, row, *row.PhotoID); err != nil {
				return err
			}
		}
	}
	if changed {
		r.result.Changed++
		return r.replace(ctx, row, f)
	}
	return nil
}

// returned brings back a photo whose file was away: out of the trash, if the scan put it
// there, and into processing, if the jobs skipped it while the file was unavailable.
func (r *scanRun) returned(ctx context.Context, row store.PhotoFile, photoID int64) error {
	if row.TrashedByScan {
		if err := r.restore(ctx, photoID); err != nil {
			return err
		}
	}
	photo, err := r.queries.GetPhoto(ctx, photoID)
	if err != nil {
		return fmt.Errorf("look up photo: %w", err)
	}
	if photo.Status != "ready" && photo.PurgedAt == nil {
		if _, err := r.Ingest.Jobs.Insert(ctx, jobs.ProcessPhotoArgs{PhotoID: photoID}, &river.InsertOpts{Priority: ingest.PriorityBulk}); err != nil {
			return fmt.Errorf("queue photo: %w", err)
		}
	}
	return nil
}

// replace reads a known file that changed, so that its photo follows the new content.
func (r *scanRun) replace(ctx context.Context, row store.PhotoFile, f listed) error {
	if row.PhotoID == nil {
		return nil
	}
	if time.Since(f.mtime) < r.Stable {
		r.result.Waiting++ // Still being written.
		return nil
	}
	source := r.sourcePath(f.path)
	file, err := r.root.Open(source)
	if err != nil {
		return r.failed(ctx, f, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return r.failed(ctx, f, err)
	}
	if !info.Mode().IsRegular() || info.Size() != f.size || !sameTime(info.ModTime(), f.mtime) {
		r.result.Waiting++
		return nil
	}

	outcome, other, err := r.Ingest.ReplaceInPlace(ctx, *row.PhotoID, file, path.Base(f.path))
	if err != nil {
		// The photo keeps its old content, and the next scan looks at the file again.
		return r.failed(ctx, f, err)
	}
	switch outcome {
	case ingest.Replaced:
		r.result.Replaced++
	case ingest.Conflict:
		r.result.Conflicts++
		slog.Warn("library file holds the content of another photo and is left as it is", "folder", r.folder.Path, "path", f.path, "photo", *row.PhotoID, "other", other)
	}
	if err := r.queries.UpdatePhotoFileStat(ctx, store.UpdatePhotoFileStatParams{ID: row.ID, Size: f.size, Mtime: f.mtime}); err != nil {
		return fmt.Errorf("record file: %w", err)
	}
	return nil
}

// restore takes a photo that a scan moved to the trash back out of it.
func (r *scanRun) restore(ctx context.Context, photoID int64) error {
	photo, err := r.queries.GetPhoto(ctx, photoID)
	if err != nil {
		return fmt.Errorf("look up photo: %w", err)
	}
	if photo.DeletedAt == nil || photo.PurgedAt != nil {
		return nil
	}
	if err := r.queries.RestorePhotos(ctx, store.RestorePhotosParams{Ids: []int64{photoID}, OwnerID: photo.OwnerID}); err != nil {
		return fmt.Errorf("restore photo: %w", err)
	}
	return nil
}

// add handles a file at a path the folder has not seen.
func (r *scanRun) add(ctx context.Context, f listed) error {
	if time.Since(f.mtime) < r.Stable {
		r.result.Waiting++
		return nil
	}
	source := r.sourcePath(f.path)
	file, err := r.root.Open(source)
	if err != nil {
		return r.failed(ctx, f, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return r.failed(ctx, f, err)
	}
	if !info.Mode().IsRegular() || info.Size() != f.size || !sameTime(info.ModTime(), f.mtime) {
		r.result.Waiting++ // It changed since it was listed; the next scan reads it.
		return nil
	}

	res, err := r.Ingest.ImportInPlace(ctx, r.folder.OwnerID, file, path.Base(f.path), source)
	switch {
	case errors.Is(err, ingest.ErrUnsupported):
		r.result.Unsupported++
		return nil
	case err != nil:
		return r.failed(ctx, f, err)
	}

	if !res.Duplicate {
		r.result.Added++
		return r.record(ctx, f, &res.Photo.ID, stateSource)
	}
	existing := res.Photo
	switch {
	case existing.PurgedAt != nil:
		r.result.Purged++
	case existing.SourcePath != nil && *existing.SourcePath == source:
		// The record of the file was lost between the photo and its row.
		r.result.Added++
		return r.record(ctx, f, &existing.ID, stateSource)
	case existing.SourcePath != nil:
		moved, err := r.move(ctx, existing, f)
		if err != nil || moved {
			return err
		}
		r.countDuplicate(existing)
	default:
		r.countDuplicate(existing)
	}
	return r.record(ctx, f, &existing.ID, stateDuplicate)
}

func (r *scanRun) countDuplicate(photo store.Photo) {
	if photo.DeletedAt != nil {
		r.result.InTrash++
	} else {
		r.result.Duplicates++
	}
}

// move takes a photo's row to a new path when the photo's file in this folder is no
// longer where it was.
func (r *scanRun) move(ctx context.Context, photo store.Photo, f listed) (bool, error) {
	old, err := r.queries.GetPhotoFileOfPhoto(ctx, &photo.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look up file: %w", err)
	}
	if old.FolderID != r.folder.ID || r.seen[old.Path] {
		return false, nil
	}

	tx, err := r.Ingest.Pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)
	if err := q.MovePhotoFile(ctx, store.MovePhotoFileParams{ID: old.ID, Path: f.path, Size: f.size, Mtime: f.mtime}); err != nil {
		return false, fmt.Errorf("move file: %w", err)
	}
	source := r.sourcePath(f.path)
	if err := q.SetPhotoSourcePath(ctx, store.SetPhotoSourcePathParams{ID: photo.ID, SourcePath: &source}); err != nil {
		return false, fmt.Errorf("move photo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	r.moved[old.ID] = true
	r.result.Moved++
	if old.TrashedByScan {
		if err := r.restore(ctx, photo.ID); err != nil {
			return true, err
		}
	}
	return true, nil
}

func (r *scanRun) record(ctx context.Context, f listed, photoID *int64, state string) error {
	_, err := r.queries.InsertPhotoFile(ctx, store.InsertPhotoFileParams{
		FolderID: r.folder.ID, Path: f.path, PhotoID: photoID, State: state, Size: f.size, Mtime: f.mtime,
	})
	if err != nil {
		return fmt.Errorf("record file: %w", err)
	}
	return nil
}

// failed reports a problem with one file and goes on; only a stopped scan is an error.
func (r *scanRun) failed(ctx context.Context, f listed, cause error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	slog.Warn("library file", "folder", r.folder.Path, "path", f.path, "error", cause)
	r.result.Failed++
	return nil
}

// missing counts a scan that did not find the files of known rows, and acts on the ones
// that stayed away for missedToMissing scans.
func (r *scanRun) missing(ctx context.Context, rows []store.PhotoFile) error {
	var away, gone []int64
	for _, row := range rows {
		if r.seen[row.Path] || r.moved[row.ID] {
			continue
		}
		if row.State == stateDuplicate {
			gone = append(gone, row.ID)
			continue
		}
		away = append(away, row.ID)
	}
	if len(gone) > 0 {
		if err := r.queries.DeletePhotoFiles(ctx, gone); err != nil {
			return fmt.Errorf("forget files: %w", err)
		}
	}
	if len(away) == 0 {
		return nil
	}
	missed, err := r.queries.MissPhotoFiles(ctx, away)
	if err != nil {
		return fmt.Errorf("count missed scans: %w", err)
	}
	for _, row := range missed {
		r.result.Missing++
		if row.MissedScans < missedToMissing || row.MissingSince != nil {
			continue
		}
		trashed, err := r.trash(ctx, row)
		if err != nil {
			return err
		}
		if err := r.queries.MarkPhotoFilesMissing(ctx, store.MarkPhotoFilesMissingParams{Ids: []int64{row.ID}, TrashedByScan: trashed}); err != nil {
			return fmt.Errorf("mark file missing: %w", err)
		}
		if trashed {
			r.result.Trashed++
		}
	}
	return nil
}

// trash moves the photo of a missing file to the trash and tells whether the scan did it:
// a photo already in the trash, or one hidden under another, stays as it is.
func (r *scanRun) trash(ctx context.Context, row store.PhotoFile) (bool, error) {
	if row.PhotoID == nil {
		return false, nil
	}
	photo, err := r.queries.GetPhoto(ctx, *row.PhotoID)
	if err != nil {
		return false, fmt.Errorf("look up photo: %w", err)
	}
	if photo.DeletedAt != nil || photo.PurgedAt != nil {
		return false, nil
	}
	err = r.queries.TrashPhotos(ctx, store.TrashPhotosParams{Ids: []int64{photo.ID}, ActorID: photo.OwnerID})
	if err != nil {
		return false, fmt.Errorf("trash photo: %w", err)
	}
	after, err := r.queries.GetPhoto(ctx, photo.ID)
	if err != nil {
		return false, fmt.Errorf("look up photo: %w", err)
	}
	return after.DeletedAt != nil, nil
}

const (
	maxFolderDepth = 3
	maxFolders     = 1000
)

// Folders lists the folders of the folders mount that can be chosen as library
// folders, without the mount itself.
func (s *Service) Folders() ([]string, error) {
	root, err := os.OpenRoot(s.Ingest.Layout.Folders)
	if err != nil {
		return nil, fmt.Errorf("open folders mount: %w", err)
	}
	defer root.Close()

	folders := []string{}
	err = fs.WalkDir(root.FS(), ".", func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			if p == "." {
				return err
			}
			return fs.SkipDir
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

// CheckPath tells whether path is a folder inside the folders mount that a library folder may
// have: it exists, is not a link, and holds or is held by no other library folder.
func (s *Service) CheckPath(ctx context.Context, path string) error {
	root, err := os.OpenRoot(s.Ingest.Layout.Folders)
	if err != nil {
		return fmt.Errorf("open folders mount: %w", err)
	}
	defer root.Close()
	info, err := root.Lstat(path)
	if err != nil || !info.IsDir() {
		return ErrFolderMissing
	}
	existing, err := store.New(s.Ingest.Pool).ListLibraryFolders(ctx)
	if err != nil {
		return fmt.Errorf("list folders: %w", err)
	}
	for _, f := range existing {
		if f.Path == path || strings.HasPrefix(f.Path, path+"/") || strings.HasPrefix(path, f.Path+"/") {
			return ErrOverlap
		}
	}
	return nil
}
