package scan

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const (
	// The folder every Takeout export starts with, in any language of the account.
	takeoutRoot = "Takeout"
	// The JSON Google Takeout keeps next to each photo. Its name is in English
	// whatever the language of the account.
	takeoutSidecarSuffix = ".supplemental-metadata.json"
	// A JSON file larger than this is not one Google Photos writes.
	maxSidecarBytes = 1 << 20
	// A .tgz part is read in long slices, as finding an entry means unpacking
	// everything before it. A slice must leave room within jobTimeout.
	tarSlice = 20 * time.Minute

	pgDataException = "22"
)

// partNumbers matches the name of a part without its extension, such as
// takeout-20261115T073434Z-1-001, and keeps what all parts of one export share.
// A browser that downloads a part twice adds " (1)", and the copy is the same part.
var partNumbers = regexp.MustCompile(`^(takeout-\d{8}T\d{6}Z)(-\d+)+( \(\d+\))?$`)

var archiveExtensions = []string{".tar.gz", ".tgz", ".zip"}

// takeoutPart recognizes a file a Takeout scan opens as an archive.
func takeoutPart(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range archiveExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func isZip(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".zip")
}

// partExport names the export a part belongs to: its path without the numbers
// of the part, so that all parts of one export share it.
func partExport(p string) string {
	dir, name := path.Split(p)
	lower := strings.ToLower(name)
	for _, ext := range archiveExtensions {
		if strings.HasSuffix(lower, ext) {
			name = name[:len(name)-len(ext)]
			break
		}
	}
	if m := partNumbers.FindStringSubmatch(name); m != nil {
		name = m[1]
	}
	return path.Join(dir, name)
}

// splitUnpacked splits the path of a file in an unpacked export into the export,
// which is the path of its Takeout folder, and the path inside the export, which
// starts with that folder as the paths inside a part do.
func splitUnpacked(p string) (export, inner string, ok bool) {
	parts := strings.Split(p, "/")
	for i := range len(parts) - 1 {
		if parts[i] == takeoutRoot {
			return strings.Join(parts[:i+1], "/"), strings.Join(parts[i:], "/"), true
		}
	}
	return "", "", false
}

func inTakeoutFolder(p string) bool {
	_, _, ok := splitUnpacked(p)
	return ok
}

// partEntry tells whether a scan reads an entry of a part. Only the Takeout
// folder is read, so that an archive of something else adds nothing.
func partEntry(name string) bool {
	return strings.HasPrefix(name, takeoutRoot+"/") && !skipped(path.Base(name))
}

// entry is one file of an export.
type entry struct {
	export, path, source string
}

func (s *Service) processTakeout(ctx context.Context, run store.ImportRun, slice time.Duration) (time.Duration, bool, error) {
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

	// A .tgz part gets a longer slice, counted from the start of the step too,
	// so that one step never runs past jobTimeout.
	deadline := time.Now().Add(slice)
	tarDeadline := deadline
	if slice > 0 {
		tarDeadline = time.Now().Add(tarSlice)
	}
	started := false
	for {
		stopped, err := s.stopped(ctx, run.ID)
		if err != nil {
			return 0, false, err
		}
		if stopped {
			return 0, false, queries.DeletePendingImportFiles(ctx, run.ID)
		}

		files, err := queries.ListPendingImportFiles(ctx, store.ListPendingImportFilesParams{RunID: run.ID, MaxRows: processBatch})
		if err != nil {
			return 0, false, fmt.Errorf("list files: %w", err)
		}
		if len(files) == 0 {
			// Matching needs every part read, and is quick: it runs in the next step.
			return 0, true, queries.StartMatching(ctx, run.ID)
		}
		for _, file := range files {
			// A part read whole may take long: the next one waits for a new step.
			if started && !time.Now().Before(deadline) {
				return 0, true, nil
			}
			started = true
			done, err := s.readTakeoutFile(ctx, root, run, file, deadline, tarDeadline)
			if err != nil || !done {
				return 0, err == nil, err
			}
		}
		if !time.Now().Before(deadline) {
			return 0, true, nil
		}
	}
}

// stopped reads the run again, which notices a request to stop.
func (s *Service) stopped(ctx context.Context, runID int64) (bool, error) {
	current, err := s.queries().GetImportRun(ctx, runID)
	if err != nil {
		return false, fmt.Errorf("load run: %w", err)
	}
	return current.Status != statusImporting, nil
}

// readTakeoutFile reads a part or a file of an unpacked export. It reports
// whether the file is done; a part may need more slices. It returns an error
// only for a problem with the scan itself.
func (s *Service) readTakeoutFile(ctx context.Context, root *os.Root, run store.ImportRun, listed store.ListPendingImportFilesRow, deadline, tarDeadline time.Time) (bool, error) {
	queries := s.queries()
	failed := func(cause error) (bool, error) {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		slog.Warn("import file", "run", run.ID, "path", listed.Path, "error", cause)
		return true, queries.MarkImportFileFailed(ctx, store.MarkImportFileFailedParams{RunID: run.ID, Path: listed.Path, Error: new(cause.Error())})
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
		return true, queries.MarkImportFileChanging(ctx, store.MarkImportFileChangingParams{RunID: run.ID, Path: listed.Path})
	}

	var done bool
	name := path.Base(listed.Path)
	switch {
	case isZip(name):
		done, err = s.readZip(ctx, run, listed.Path, file, info.Size(), deadline)
	case takeoutPart(name):
		done, err = s.readTar(ctx, run, listed.Path, file, tarDeadline)
	default:
		if export, inner, ok := splitUnpacked(listed.Path); ok {
			err = s.importEntry(ctx, run, entry{export: export, path: inner}, file)
		}
		done = true
	}
	if errors.Is(err, errBadPart) {
		return failed(err)
	}
	if err != nil || !done {
		return false, err
	}
	return true, queries.FinishImportPart(ctx, store.FinishImportPartParams{RunID: run.ID, Path: listed.Path})
}

// errBadPart wraps a problem with a part that stops it being read further, such
// as a damaged archive. The entries read before it stay.
var errBadPart = errors.New("the archive cannot be read")

func badPart(err error) error {
	return fmt.Errorf("%w: %w", errBadPart, err)
}

// donePaths returns the entries of an export recorded in an earlier slice or
// from another part, such as the same part downloaded twice.
func (s *Service) donePaths(ctx context.Context, runID int64, export string) (map[string]bool, error) {
	paths, err := s.queries().ListTakeoutEntryPaths(ctx, store.ListTakeoutEntryPathsParams{RunID: runID, Export: export})
	if err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}
	done := make(map[string]bool, len(paths))
	for _, p := range paths {
		done[p] = true
	}
	return done, nil
}

// pause tells whether a slice should end before the next entry: every batch it
// checks the time and a request to stop.
func (s *Service) pause(ctx context.Context, runID int64, read int, deadline time.Time) (bool, error) {
	if read == 0 || read%processBatch != 0 {
		return false, nil
	}
	if !time.Now().Before(deadline) {
		return true, nil
	}
	return s.stopped(ctx, runID)
}

// readZip reads the entries of a .zip part not recorded yet. The central
// directory lists them, so a later slice goes straight to the next one.
func (s *Service) readZip(ctx context.Context, run store.ImportRun, p string, file io.ReaderAt, size int64, deadline time.Time) (bool, error) {
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return false, badPart(err)
	}
	export := partExport(p)
	done, err := s.donePaths(ctx, run.ID, export)
	if err != nil {
		return false, err
	}
	read := 0
	for _, f := range archive.File {
		if f.Mode().Type() != 0 || !partEntry(f.Name) || done[f.Name] {
			continue
		}
		if stop, err := s.pause(ctx, run.ID, read, deadline); stop || err != nil {
			return false, err
		}
		read++
		r := &reopener{open: f.Open}
		err := s.importEntry(ctx, run, entry{export: export, path: f.Name, source: p}, r)
		r.Close()
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

// readTar reads a .tgz part from its start, skipping the entries recorded in
// an earlier slice, as a gzip stream can only be read in order.
func (s *Service) readTar(ctx context.Context, run store.ImportRun, p string, file io.Reader, deadline time.Time) (bool, error) {
	unzipped, err := gzip.NewReader(bufio.NewReaderSize(file, 1<<20))
	if err != nil {
		return false, badPart(err)
	}
	defer unzipped.Close()
	export := partExport(p)
	done, err := s.donePaths(ctx, run.ID, export)
	if err != nil {
		return false, err
	}
	archive := tar.NewReader(unzipped)
	read := 0
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return true, nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return false, badPart(err)
		}
		name := strings.TrimPrefix(header.Name, "./")
		if header.Typeflag != tar.TypeReg || !partEntry(name) || done[name] {
			continue
		}
		if stop, err := s.pause(ctx, run.ID, read, deadline); stop || err != nil {
			return false, err
		}
		read++
		if err := s.importEntry(ctx, run, entry{export: export, path: name, source: p}, archive); err != nil {
			return false, err
		}
	}
}

// importEntry adds a photo or keeps a JSON file of an export and records what
// came of it. A problem with the file goes into the report; an error means a
// problem with the scan itself. A reader that can seek is read twice, so that a
// photo the library has is not written again.
func (s *Service) importEntry(ctx context.Context, run store.ImportRun, e entry, r io.Reader) error {
	params := store.AddTakeoutEntryParams{RunID: run.ID, Export: e.export, Path: e.path, Source: e.source}
	if !utf8.ValidString(e.path) {
		params.Path = strings.ToValidUTF8(e.path, "?")
		params.Outcome, params.Error = "failed", new("the file name is not valid UTF-8")
		return s.addEntry(ctx, params)
	}

	if strings.EqualFold(path.Ext(e.path), ".json") {
		data, err := io.ReadAll(io.LimitReader(r, maxSidecarBytes+1))
		switch {
		case err != nil && ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			params.Outcome, params.Error = "failed", new(err.Error())
		case len(data) > maxSidecarBytes:
			params.Outcome, params.Error = "failed", new("the JSON file is larger than 1 MB")
		case !json.Valid(data):
			params.Outcome, params.Error = "failed", new("the file is not valid JSON")
		default:
			params.Outcome, params.Sidecar = "sidecar", data
		}
		return s.addEntry(ctx, params)
	}

	var res ingest.Result
	var err error
	name := path.Base(e.path)
	if seeker, ok := r.(io.ReadSeeker); ok {
		res, err = s.Ingest.Import(ctx, run.OwnerID, seeker, name)
	} else {
		res, err = s.Ingest.ImportStream(ctx, run.OwnerID, r, name)
	}
	switch {
	case errors.Is(err, ingest.ErrUnsupported):
		params.Outcome = "unsupported"
	case err != nil && ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		slog.Warn("import file", "run", run.ID, "export", e.export, "path", e.path, "error", err)
		params.Outcome, params.Error = "failed", new(err.Error())
	case res.Purged:
		params.Outcome = "purged"
	case res.InTrash:
		params.Outcome = "in_trash"
	case res.Duplicate:
		params.Outcome = "duplicate"
	default:
		params.Outcome = "added"
	}
	if err == nil {
		params.PhotoID = &res.Photo.ID
	}
	return s.addEntry(ctx, params)
}

// addEntry records an entry. JSON that PostgreSQL refuses, such as a string
// with a NUL character, is recorded as a problem with the file.
func (s *Service) addEntry(ctx context.Context, params store.AddTakeoutEntryParams) error {
	err := s.queries().AddTakeoutEntry(ctx, params)
	var pgErr *pgconn.PgError
	if params.Sidecar != nil && errors.As(err, &pgErr) && strings.HasPrefix(pgErr.Code, pgDataException) {
		params.Outcome, params.Sidecar, params.Error = "failed", nil, new(pgErr.Message)
		err = s.queries().AddTakeoutEntry(ctx, params)
	}
	if err != nil {
		return fmt.Errorf("record entry: %w", err)
	}
	return nil
}

// reopener reads an entry of a .zip part and starts it over on a seek to its
// start, which is all ingest.Import needs, by opening the entry again.
type reopener struct {
	open func() (io.ReadCloser, error)
	r    io.ReadCloser
}

func (o *reopener) Read(p []byte) (int, error) {
	if o.r == nil {
		r, err := o.open()
		if err != nil {
			return 0, err
		}
		o.r = r
	}
	return o.r.Read(p)
}

func (o *reopener) Seek(offset int64, whence int) (int64, error) {
	if offset != 0 || whence != io.SeekStart {
		return 0, errors.New("an archive entry can only be read again from its start")
	}
	return 0, o.Close()
}

func (o *reopener) Close() error {
	if o.r == nil {
		return nil
	}
	err := o.r.Close()
	o.r = nil
	return err
}
