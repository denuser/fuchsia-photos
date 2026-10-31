// Package ingest is the single way a photo enters the library, whatever its source.
package ingest

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/denuser/fuchsia-photos/backend/internal/jobs"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

// Job priorities. River runs lower numbers first, so a few new photos do not
// wait behind a bulk import.
const (
	PriorityInteractive = 2
	PriorityBulk        = 4
)

// ErrUnsupported means the file is not a supported image format.
var ErrUnsupported = errors.New("unsupported file format")

// Service adds photos to the library.
type Service struct {
	Pool   *pgxpool.Pool
	Jobs   *river.Client[pgx.Tx]
	Layout storage.Layout
}

// Result describes an ingested photo.
type Result struct {
	Photo store.Photo
	// Duplicate means the owner already had this photo and nothing was added.
	Duplicate bool
	// Restored means the duplicate was in the trash and has been taken out of it.
	Restored bool
}

// Ingest stores the photo read from r and queues its processing.
func (s *Service) Ingest(ctx context.Context, ownerID int64, r io.Reader, name string, priority int) (Result, error) {
	buffered := bufio.NewReaderSize(r, 4096)
	head, err := buffered.Peek(512)
	if err != nil && !errors.Is(err, io.EOF) {
		return Result{}, fmt.Errorf("read upload: %w", err)
	}
	mime, ok := detectMIME(head)
	if !ok {
		return Result{}, ErrUnsupported
	}

	tmp, sum, size, err := s.writeTemp(buffered)
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(tmp) // Fails harmlessly once the file has been moved.

	return s.store(ctx, tmp, store.InsertPhotoParams{
		OwnerID:      ownerID,
		Sha256:       sum,
		Mime:         mime,
		Size:         size,
		OriginalName: cleanName(name),
	}, priority)
}

// Purge permanently deletes a photo from the owner's trash, with its files. It
// does nothing if the photo is not in the trash. Once started, it ignores
// cancellation, so that a closed page cannot separate a record from its files.
func (s *Service) Purge(ctx context.Context, ownerID, photoID int64) error {
	ctx = context.WithoutCancel(ctx)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queries := store.New(tx)
	sum, err := queries.GetTrashedPhotoHash(ctx, store.GetTrashedPhotoHashParams{ID: photoID, OwnerID: ownerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("look up photo: %w", err)
	}
	if err := queries.LockPhotoFile(ctx, store.LockPhotoFileParams{OwnerID: ownerID, Sha256: sum}); err != nil {
		return fmt.Errorf("lock photo: %w", err)
	}
	photo, err := queries.PurgePhoto(ctx, store.PurgePhotoParams{ID: photoID, OwnerID: ownerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // Restored or purged while waiting for the lock.
	}
	if err != nil {
		return fmt.Errorf("delete photo: %w", err)
	}

	// The record goes only if the file does. A commit that fails after this leaves
	// the photo in the trash without its original, and purging it again finishes the job.
	err = os.Remove(s.Layout.Original(photo.OwnerID, photo.Sha256, photo.Mime))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove original: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if photo.PreviewFormat != nil {
		for _, kind := range []string{storage.PreviewGrid, storage.PreviewViewer} {
			err := os.Remove(s.Layout.Preview(photo.ID, kind, *photo.PreviewFormat))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				// A stray preview is harmless: photo ids are never reused.
				slog.Warn("remove preview", "photo", photo.ID, "error", err)
			}
		}
	}
	return nil
}

func (s *Service) writeTemp(r io.Reader) (path string, sum []byte, size int64, err error) {
	tmp, err := os.CreateTemp(s.Layout.TempDir(), "upload-*")
	if err != nil {
		return "", nil, 0, fmt.Errorf("create temporary file: %w", err)
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	hash := sha256.New()
	size, err = io.Copy(io.MultiWriter(tmp, hash), r)
	if err != nil {
		return "", nil, 0, fmt.Errorf("write upload: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return "", nil, 0, fmt.Errorf("flush upload: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return "", nil, 0, fmt.Errorf("close upload: %w", err)
	}
	return tmp.Name(), hash.Sum(nil), size, nil
}

// store moves the temporary file into the library and creates the photo and its
// job in one transaction, so neither the record nor the job can exist alone. An
// original left behind by a failure here is harmless, and the next upload reuses it.
func (s *Service) store(ctx context.Context, tmp string, params store.InsertPhotoParams, priority int) (Result, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queries := store.New(tx)
	err = queries.LockPhotoFile(ctx, store.LockPhotoFileParams{OwnerID: params.OwnerID, Sha256: params.Sha256})
	if err != nil {
		return Result{}, fmt.Errorf("lock photo: %w", err)
	}

	existing, err := queries.GetPhotoByHash(ctx, store.GetPhotoByHashParams{OwnerID: params.OwnerID, Sha256: params.Sha256})
	if err == nil {
		return s.duplicate(ctx, tx, queries, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, fmt.Errorf("look up photo: %w", err)
	}

	path := s.Layout.Original(params.OwnerID, params.Sha256, params.Mime)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return Result{}, fmt.Errorf("create photo folder: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return Result{}, fmt.Errorf("move photo into place: %w", err)
	}

	photo, err := queries.InsertPhoto(ctx, params)
	if err != nil {
		return Result{}, fmt.Errorf("insert photo: %w", err)
	}
	_, err = s.Jobs.InsertTx(ctx, tx, jobs.ProcessPhotoArgs{PhotoID: photo.ID}, &river.InsertOpts{Priority: priority})
	if err != nil {
		return Result{}, fmt.Errorf("queue photo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("commit: %w", err)
	}
	return Result{Photo: photo}, nil
}

// duplicate handles a file the owner already has. Uploading a photo from the
// trash again means the owner wants it back.
func (s *Service) duplicate(ctx context.Context, tx pgx.Tx, queries *store.Queries, photo store.Photo) (Result, error) {
	if photo.DeletedAt == nil {
		return Result{Photo: photo, Duplicate: true}, nil
	}
	restored, err := queries.RestorePhoto(ctx, photo.ID)
	if err != nil {
		return Result{}, fmt.Errorf("restore photo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("commit: %w", err)
	}
	return Result{Photo: restored, Duplicate: true, Restored: true}, nil
}

// detectMIME identifies a file by its content, not by its name.
func detectMIME(head []byte) (string, bool) {
	switch mime := http.DetectContentType(head); mime {
	case "image/jpeg", "image/png", "image/webp":
		return mime, true
	}
	if isHEIC(head) {
		return "image/heic", true
	}
	return "", false
}

func isHEIC(head []byte) bool {
	if len(head) < 12 || string(head[4:8]) != "ftyp" {
		return false
	}
	switch string(head[8:12]) {
	case "heic", "heix", "hevc", "hevx", "heim", "heis", "hevm", "hevs":
		return true
	case "mif1", "msf1":
		// A generic brand: the compatible brands say what the file really holds.
		size := int(head[0])<<24 | int(head[1])<<16 | int(head[2])<<8 | int(head[3])
		for i := 16; i+4 <= size && i+4 <= len(head); i += 4 {
			switch string(head[i : i+4]) {
			case "heic", "heix", "hevc", "hevx":
				return true
			}
		}
	}
	return false
}

func cleanName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.ReplaceAll(strings.ToValidUTF8(name, ""), "\x00", "")
	if name == "." || name == "/" {
		return ""
	}
	for utf8.RuneCountInString(name) > 255 {
		name = string([]rune(name)[:255])
	}
	return name
}
