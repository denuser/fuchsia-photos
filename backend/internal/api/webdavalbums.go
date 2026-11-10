package api

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/denuser/fuchsia-photos/backend/internal/store"
	"github.com/denuser/fuchsia-photos/backend/internal/webdav"
)

const maxWebDAVAlbums = 100

type webdavAlbumJSON struct {
	ID        int64  `json:"id"`
	Path      string `json:"path"`
	AlbumID   int64  `json:"albumId"`
	AlbumName string `json:"albumName"`
}

func (s *server) listWebDAVAlbums(w http.ResponseWriter, r *http.Request) {
	links, err := s.Queries.ListWebDAVAlbums(r.Context(), currentUser(r).ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]webdavAlbumJSON, len(links))
	for i, l := range links {
		out[i] = webdavAlbumJSON(l)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) createWebDAVAlbum(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		AlbumID int64  `json:"albumId"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	folder, ok := webdav.FolderPath(req.Path)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid folder")
		return
	}
	user := currentUser(r)
	count, err := s.Queries.CountWebDAVAlbums(r.Context(), user.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if count >= maxWebDAVAlbums {
		writeError(w, http.StatusConflict, "a user can link at most 100 folders")
		return
	}
	link, err := s.Queries.CreateWebDAVAlbum(r.Context(), store.CreateWebDAVAlbumParams{UserID: user.ID, Path: folder, AlbumID: req.AlbumID})
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "album not found")
	case errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation:
		writeError(w, http.StatusConflict, "this folder is already linked to an album")
	case err != nil:
		internalError(w, r, err)
	default:
		writeJSON(w, http.StatusCreated, webdavAlbumJSON(link))
	}
}

func (s *server) deleteWebDAVAlbum(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	deleted, err := s.Queries.DeleteWebDAVAlbum(r.Context(), store.DeleteWebDAVAlbumParams{ID: id, UserID: currentUser(r).ID})
	if err != nil {
		internalError(w, r, err)
		return
	}
	if deleted == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listWebDAVFolders offers the folders sync clients have made, to link one.
func (s *server) listWebDAVFolders(w http.ResponseWriter, r *http.Request) {
	folders, err := s.Queries.ListWebDAVFolders(r.Context(), currentUser(r).ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if folders == nil {
		folders = []string{}
	}
	writeJSON(w, http.StatusOK, folders)
}
