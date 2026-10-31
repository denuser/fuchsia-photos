package api

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const maxTagName = 100

type tagRefJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type tagJSON struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	PhotoCount int64  `json:"photoCount"`
}

func (s *server) listTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.Queries.ListTags(r.Context(), currentUser(r).ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]tagJSON, len(tags))
	for i, t := range tags {
		out[i] = tagJSON{ID: t.ID, Name: t.Name, PhotoCount: t.PhotoCount}
	}
	writeJSON(w, http.StatusOK, out)
}

// createTag returns the existing tag if the user already has one with this name.
func (s *server) createTag(w http.ResponseWriter, r *http.Request) {
	name, ok := readName(w, r, maxTagName)
	if !ok {
		return
	}
	owner := currentUser(r).ID
	created, err := s.Queries.CreateTag(r.Context(), store.CreateTagParams{OwnerID: owner, Name: name})
	if err == nil {
		writeJSON(w, http.StatusCreated, tagRefJSON{ID: created.ID, Name: created.Name})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		internalError(w, r, err)
		return
	}
	existing, err := s.Queries.GetTagByName(r.Context(), store.GetTagByNameParams{OwnerID: owner, Name: name})
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tagRefJSON{ID: existing.ID, Name: existing.Name})
}

func (s *server) renameTag(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	name, ok := readName(w, r, maxTagName)
	if !ok {
		return
	}
	rows, err := s.Queries.RenameTag(r.Context(), store.RenameTagParams{ID: id, OwnerID: currentUser(r).ID, Name: name})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		writeError(w, http.StatusConflict, "you already have a tag with this name")
		return
	}
	if changed(w, r, rows, err) {
		writeJSON(w, http.StatusOK, tagRefJSON{ID: id, Name: name})
	}
}

func (s *server) deleteTag(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rows, err := s.Queries.DeleteTag(r.Context(), store.DeleteTagParams{ID: id, OwnerID: currentUser(r).ID})
	if changed(w, r, rows, err) {
		w.WriteHeader(http.StatusNoContent)
	}
}

// getPhotoTags lists the current user's own tags on a photo. Tags are personal,
// so other users' tags on the same photo stay hidden.
func (s *server) getPhotoTags(w http.ResponseWriter, r *http.Request) {
	photo, ok := s.visiblePhoto(w, r)
	if !ok {
		return
	}
	tags, err := s.Queries.ListPhotoTags(r.Context(), store.ListPhotoTagsParams{PhotoID: photo.ID, OwnerID: currentUser(r).ID})
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]tagRefJSON, len(tags))
	for i, t := range tags {
		out[i] = tagRefJSON{ID: t.ID, Name: t.Name}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) changePhotoTags(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs    []int64 `json:"ids"`
		Add    []int64 `json:"add"`
		Remove []int64 `json:"remove"`
	}
	if !readJSON(w, r, &req) || !validBulkIDs(w, req.IDs) {
		return
	}
	owner := currentUser(r).ID
	// Both changes apply or neither does.
	tx, err := s.Ingest.Pool.Begin(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	queries := store.New(tx)

	if len(req.Add) > 0 {
		err := queries.AddPhotoTags(r.Context(), store.AddPhotoTagsParams{OwnerID: owner, TagIds: req.Add, PhotoIds: req.IDs})
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	if len(req.Remove) > 0 {
		err := queries.RemovePhotoTags(r.Context(), store.RemovePhotoTagsParams{OwnerID: owner, TagIds: req.Remove, PhotoIds: req.IDs})
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
