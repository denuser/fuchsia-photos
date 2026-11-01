package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/denuser/fuchsia-photos/backend/internal/scan"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const (
	maxImportRuns     = 50
	maxImportProblems = 500
)

type importRunJSON struct {
	ID          int64      `json:"id"`
	OwnerID     int64      `json:"ownerId"`
	Folder      string     `json:"folder"`
	Status      string     `json:"status"`
	Error       *string    `json:"error"`
	Found       int64      `json:"found"`
	Added       int64      `json:"added"`
	Duplicates  int64      `json:"duplicates"`
	InTrash     int64      `json:"inTrash"`
	Purged      int64      `json:"purged"`
	Unsupported int64      `json:"unsupported"`
	Changing    int64      `json:"changing"`
	Failed      int64      `json:"failed"`
	CreatedAt   time.Time  `json:"createdAt"`
	FinishedAt  *time.Time `json:"finishedAt"`
}

type importProblemJSON struct {
	Path  string  `json:"path"`
	State string  `json:"state"`
	Error *string `json:"error"`
}

func toImportRunJSON(r store.ImportRun) importRunJSON {
	return importRunJSON{
		ID:          r.ID,
		OwnerID:     r.OwnerID,
		Folder:      r.Folder,
		Status:      r.Status,
		Error:       r.Error,
		Found:       r.Found,
		Added:       r.Added,
		Duplicates:  r.Duplicates,
		InTrash:     r.InTrash,
		Purged:      r.Purged,
		Unsupported: r.Unsupported,
		Changing:    r.Changing,
		Failed:      r.Failed,
		CreatedAt:   r.CreatedAt,
		FinishedAt:  r.FinishedAt,
	}
}

func (s *server) listImports(w http.ResponseWriter, r *http.Request) {
	runs, err := s.Queries.ListImportRuns(r.Context(), maxImportRuns)
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]importRunJSON, len(runs))
	for i, run := range runs {
		out[i] = toImportRunJSON(run)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) listImportFolders(w http.ResponseWriter, r *http.Request) {
	folders, err := s.Scan.Folders()
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, folders)
}

func (s *server) startImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OwnerID int64  `json:"ownerId"`
		Folder  string `json:"folder"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	run, err := s.Scan.Start(r.Context(), currentUser(r).ID, req.OwnerID, req.Folder)
	switch {
	case errors.Is(err, scan.ErrNoFolder), errors.Is(err, scan.ErrNoOwner):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, scan.ErrRunning):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		internalError(w, r, err)
	default:
		writeJSON(w, http.StatusCreated, toImportRunJSON(run))
	}
}

func (s *server) getImport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	run, err := s.Queries.GetImportRun(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	problems, err := s.Queries.ListImportProblems(ctx, store.ListImportProblemsParams{RunID: id, MaxRows: maxImportProblems})
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]importProblemJSON, len(problems))
	for i, p := range problems {
		out[i] = importProblemJSON{Path: p.Path, State: p.State, Error: p.Error}
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": toImportRunJSON(run), "problems": out})
}

func (s *server) stopImport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	stopped, err := s.Queries.StopImportRun(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if stopped == 0 {
		writeError(w, http.StatusConflict, "this scan is not running")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
