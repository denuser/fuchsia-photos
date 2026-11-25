package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/magellanica-app/magellanica-photos/backend/internal/jobs"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

// ImportInPlace adds a photo from a library folder without copying it: the
// photo's original stays at sourcePath, relative to the folders mount. It
// hashes the file, and a file the owner already has is answered like a
// duplicate of a folder scan and written nowhere, so that the caller can tell a
// moved file from a repeated one by the photo that comes back. Like a folder
// scan, it leaves photos the owner has deleted as they are.
func (s *Service) ImportInPlace(ctx context.Context, ownerID int64, file io.ReadSeeker, name, sourcePath string) (Result, error) {
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return Result{}, fmt.Errorf("read file: %w", err)
	}
	mime, ok := detectNamedMIME(head[:n], name)
	if !ok {
		return Result{}, ErrUnsupported
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return Result{}, fmt.Errorf("read file: %w", err)
	}
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return Result{}, fmt.Errorf("read file: %w", err)
	}
	sum := hash.Sum(nil)

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queries := store.New(tx)
	if err := queries.LockPhotoFile(ctx, store.LockPhotoFileParams{OwnerID: ownerID, Sha256: sum}); err != nil {
		return Result{}, fmt.Errorf("lock photo: %w", err)
	}
	existing, err := queries.GetPhotoByHash(ctx, store.GetPhotoByHashParams{OwnerID: ownerID, Sha256: sum})
	if err == nil {
		res := importedDuplicate(existing)
		res.MIME, res.Size = mime, size
		return res, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, fmt.Errorf("look up photo: %w", err)
	}

	photo, err := queries.InsertPhoto(ctx, store.InsertPhotoParams{
		OwnerID:      ownerID,
		Sha256:       sum,
		Mime:         mime,
		Size:         size,
		OriginalName: cleanName(name),
		SourcePath:   &sourcePath,
	})
	if err != nil {
		return Result{}, fmt.Errorf("insert photo: %w", err)
	}
	if _, err := s.Jobs.InsertTx(ctx, tx, jobs.ProcessPhotoArgs{PhotoID: photo.ID}, &river.InsertOpts{Priority: PriorityBulk}); err != nil {
		return Result{}, fmt.Errorf("queue photo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("commit: %w", err)
	}
	return Result{Photo: photo, MIME: mime, Size: size}, nil
}

// ReplaceOutcome says what ReplaceInPlace did with a changed file.
type ReplaceOutcome int

const (
	// Replaced means the photo took the file's new content and is processed again.
	Replaced ReplaceOutcome = iota
	// Unchanged means the content is the same; only the file's time differs.
	Unchanged
	// Kept means the photo was deleted permanently, and stays so whatever the
	// file holds now.
	Kept
	// Conflict means the new content is another photo of the owner. The two are
	// not merged, and the file is left as it is.
	Conflict
)

// ReplaceInPlace reads a library folder's file that changed at the path of a
// photo. The photo keeps its id, albums, tags, favorites and shares; its hash,
// its previews, motion, copy, EXIF and embedding follow the new content. Both
// hashes are locked, in a fixed order, as a purge or an upload may race the
// change. For Conflict, other is the photo that has the new content.
func (s *Service) ReplaceInPlace(ctx context.Context, photoID int64, file io.ReadSeeker, name string) (outcome ReplaceOutcome, other int64, err error) {
	queries := store.New(s.Pool)
	photo, err := queries.GetPhoto(ctx, photoID)
	if err != nil {
		return 0, 0, fmt.Errorf("look up photo: %w", err)
	}
	if photo.PurgedAt != nil {
		return Kept, 0, nil
	}

	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return 0, 0, fmt.Errorf("read file: %w", err)
	}
	mime, ok := detectNamedMIME(head[:n], name)
	if !ok {
		return 0, 0, ErrUnsupported
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, 0, fmt.Errorf("read file: %w", err)
	}
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return 0, 0, fmt.Errorf("read file: %w", err)
	}
	sum := hash.Sum(nil)
	if bytes.Equal(sum, photo.Sha256) {
		return Unchanged, 0, nil
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)
	locks := [][]byte{photo.Sha256, sum}
	if bytes.Compare(locks[0], locks[1]) > 0 {
		locks[0], locks[1] = locks[1], locks[0]
	}
	for _, l := range locks {
		if err := q.LockPhotoFile(ctx, store.LockPhotoFileParams{OwnerID: photo.OwnerID, Sha256: l}); err != nil {
			return 0, 0, fmt.Errorf("lock photo: %w", err)
		}
	}

	// Purged or changed while waiting for the locks.
	current, err := q.GetPhoto(ctx, photoID)
	if err != nil {
		return 0, 0, fmt.Errorf("look up photo: %w", err)
	}
	if current.PurgedAt != nil {
		return Kept, 0, nil
	}
	if bytes.Equal(sum, current.Sha256) {
		return Unchanged, 0, nil
	}
	existing, err := q.GetPhotoByHash(ctx, store.GetPhotoByHashParams{OwnerID: photo.OwnerID, Sha256: sum})
	if err == nil {
		return Conflict, existing.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, fmt.Errorf("look up photo: %w", err)
	}

	if _, err := q.ReplacePhotoFile(ctx, store.ReplacePhotoFileParams{ID: photoID, Sha256: sum, Mime: mime, Size: size}); err != nil {
		return 0, 0, fmt.Errorf("replace file: %w", err)
	}
	if err := q.ForgetPhotoEmbedding(ctx, photoID); err != nil {
		return 0, 0, fmt.Errorf("forget embedding: %w", err)
	}
	if _, err := s.Jobs.InsertTx(ctx, tx, jobs.ProcessPhotoArgs{PhotoID: photoID}, &river.InsertOpts{Priority: PriorityBulk}); err != nil {
		return 0, 0, fmt.Errorf("queue photo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit: %w", err)
	}

	// Made from the old content; processing writes them again if the new one has them.
	for _, stale := range []string{s.Layout.Motion(photoID), s.Layout.Copy(photoID)} {
		if err := os.Remove(stale); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("remove stale file", "photo", photoID, "error", err)
		}
	}
	return Replaced, 0, nil
}
