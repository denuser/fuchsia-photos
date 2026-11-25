package ingest

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

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
