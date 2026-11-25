package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/magellanica-app/magellanica-photos/backend/internal/library"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

const (
	defaultLibraryInterval = 600
	maxLibraryInterval     = 7 * 24 * 3600
)

type libraryFolderJSON struct {
	ID           int64           `json:"id"`
	Path         string          `json:"path"`
	OwnerID      int64           `json:"ownerId"`
	ScanInterval int32           `json:"scanInterval"`
	LastScanAt   *time.Time      `json:"lastScanAt"`
	LastError    *string         `json:"lastError"`
	LastResult   json.RawMessage `json:"lastResult"`
	// Photos are read from the folder; Missing are those whose file is away;
	// Duplicates are files skipped because the owner has them already.
	Photos     int64 `json:"photos"`
	Missing    int64 `json:"missing"`
	Duplicates int64 `json:"duplicates"`
}

func toLibraryFolderJSON(f store.LibraryFolder, sum store.SummarizeLibraryFoldersRow) libraryFolderJSON {
	return libraryFolderJSON{
		ID: f.ID, Path: f.Path, OwnerID: f.OwnerID, ScanInterval: f.ScanInterval,
		LastScanAt: f.LastScanAt, LastError: f.LastError, LastResult: f.LastResult,
		Photos: sum.Photos, Missing: sum.Missing, Duplicates: sum.Duplicates,
	}
}

func (s *server) listLibraryFolders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	folders, err := s.Queries.ListLibraryFolders(ctx)
	if err != nil {
		internalError(w, r, err)
		return
	}
	sums, err := s.Queries.SummarizeLibraryFolders(ctx)
	if err != nil {
		internalError(w, r, err)
		return
	}
	byFolder := map[int64]store.SummarizeLibraryFoldersRow{}
	for _, sum := range sums {
		byFolder[sum.FolderID] = sum
	}
	out := make([]libraryFolderJSON, len(folders))
	for i, f := range folders {
		out[i] = toLibraryFolderJSON(f, byFolder[f.ID])
	}
	writeJSON(w, http.StatusOK, out)
}

// listAvailableLibraryFolders lists the folders of the mount that no library folder covers yet.
func (s *server) listAvailableLibraryFolders(w http.ResponseWriter, r *http.Request) {
	all, err := s.Library.Folders()
	if err != nil {
		internalError(w, r, err)
		return
	}
	existing, err := s.Queries.ListLibraryFolders(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := []string{}
	for _, p := range all {
		free := true
		for _, f := range existing {
			if f.Path == p || strings.HasPrefix(f.Path, p+"/") || strings.HasPrefix(p, f.Path+"/") {
				free = false
				break
			}
		}
		if free {
			out = append(out, p)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func validLibraryInterval(seconds int32) bool {
	return seconds >= 60 && seconds <= maxLibraryInterval
}

func (s *server) createLibraryFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path         string `json:"path"`
		OwnerID      int64  `json:"ownerId"`
		ScanInterval int32  `json:"scanInterval"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	folder := path.Clean(strings.Trim(req.Path, "/"))
	if req.Path == "" || folder == "." || folder == ".." || strings.HasPrefix(folder, "../") {
		writeError(w, http.StatusBadRequest, "choose a folder of the library folders mount")
		return
	}
	if req.ScanInterval == 0 {
		req.ScanInterval = defaultLibraryInterval
	}
	if !validLibraryInterval(req.ScanInterval) {
		writeError(w, http.StatusBadRequest, "the scan interval must be between a minute and a week")
		return
	}
	owner, err := s.Queries.GetUser(ctx, req.OwnerID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner.Disabled) {
		writeError(w, http.StatusBadRequest, "no such active user")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	switch err := s.Library.CheckPath(ctx, folder); {
	case errors.Is(err, library.ErrFolderMissing):
		writeError(w, http.StatusBadRequest, "no such folder in the library folders mount")
		return
	case errors.Is(err, library.ErrOverlap):
		writeError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		internalError(w, r, err)
		return
	}

	created, err := s.Queries.CreateLibraryFolder(ctx, store.CreateLibraryFolderParams{Path: folder, OwnerID: req.OwnerID, ScanInterval: req.ScanInterval})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		writeError(w, http.StatusConflict, library.ErrOverlap.Error())
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if err := s.Library.Queue(ctx, created.ID); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toLibraryFolderJSON(created, store.SummarizeLibraryFoldersRow{}))
}

func (s *server) updateLibraryFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		ScanInterval int32 `json:"scanInterval"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !validLibraryInterval(req.ScanInterval) {
		writeError(w, http.StatusBadRequest, "the scan interval must be between a minute and a week")
		return
	}
	updated, err := s.Queries.SetLibraryFolderInterval(r.Context(), store.SetLibraryFolderIntervalParams{ID: id, ScanInterval: req.ScanInterval})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toLibraryFolderJSON(updated, store.SummarizeLibraryFoldersRow{}))
}

func (s *server) scanLibraryFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := s.Queries.GetLibraryFolder(r.Context(), id); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		internalError(w, r, err)
		return
	}
	// A folder that waits for a scan or is being scanned is not queued again.
	if err := s.Library.Queue(r.Context(), id); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// deleteLibraryFolder stops reading a folder. Its photos would keep pointing into a folder no scan
// looks at, so a folder that still has photos is not removed.
func (s *server) deleteLibraryFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if _, err := s.Queries.GetLibraryFolder(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		internalError(w, r, err)
		return
	}
	n, err := s.Queries.CountLibraryFolderPhotos(ctx, id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, "the folder still has photos; delete them first")
		return
	}
	if err := s.Queries.DeleteLibraryFolder(ctx, id); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
