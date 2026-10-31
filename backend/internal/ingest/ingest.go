// Package ingest is the single way a photo enters the library, whatever its source.
package ingest

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
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

	queries := store.New(s.Pool)
	existing, err := queries.GetPhotoByHash(ctx, store.GetPhotoByHashParams{OwnerID: ownerID, Sha256: sum})
	if err == nil {
		return Result{Photo: existing, Duplicate: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, fmt.Errorf("look up photo: %w", err)
	}

	path := s.Layout.Original(ownerID, sum, mime)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return Result{}, fmt.Errorf("create photo folder: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return Result{}, fmt.Errorf("move photo into place: %w", err)
	}

	photo, inserted, err := s.record(ctx, store.InsertPhotoParams{
		OwnerID:      ownerID,
		Sha256:       sum,
		Mime:         mime,
		Size:         size,
		OriginalName: cleanName(name),
	}, priority)
	if err != nil {
		return Result{}, err
	}
	return Result{Photo: photo, Duplicate: !inserted}, nil
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

// record creates the photo and its job in one transaction, so neither can exist
// without the other.
func (s *Service) record(ctx context.Context, params store.InsertPhotoParams, priority int) (store.Photo, bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return store.Photo{}, false, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queries := store.New(tx)
	photo, err := queries.InsertPhoto(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		// A concurrent upload of the same photo won the race.
		photo, err = queries.GetPhotoByHash(ctx, store.GetPhotoByHashParams{OwnerID: params.OwnerID, Sha256: params.Sha256})
		if err != nil {
			return store.Photo{}, false, fmt.Errorf("look up photo: %w", err)
		}
		return photo, false, nil
	}
	if err != nil {
		return store.Photo{}, false, fmt.Errorf("insert photo: %w", err)
	}

	_, err = s.Jobs.InsertTx(ctx, tx, jobs.ProcessPhotoArgs{PhotoID: photo.ID}, &river.InsertOpts{Priority: priority})
	if err != nil {
		return store.Photo{}, false, fmt.Errorf("queue photo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return store.Photo{}, false, fmt.Errorf("commit: %w", err)
	}
	return photo, true, nil
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
