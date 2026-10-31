package api

import (
	"net/http"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const maxAlbumName = 200

type albumJSON struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	PhotoCount int64  `json:"photoCount"`
	// CoverPhotoID is the photo shown as the cover, chosen or automatic.
	CoverPhotoID *int64    `json:"coverPhotoId"`
	CoverChosen  bool      `json:"coverChosen"`
	CreatedAt    time.Time `json:"createdAt"`
}

func toAlbumJSON(a store.ListAlbumsRow) albumJSON {
	out := albumJSON{
		ID:         a.ID,
		Name:       a.Name,
		PhotoCount: a.PhotoCount,
		// A chosen cover that left the album or went to the trash no longer applies.
		CoverChosen: a.ChosenCoverID != nil && *a.ChosenCoverID == a.CoverID,
		CreatedAt:   a.CreatedAt,
	}
	if a.CoverID != 0 {
		out.CoverPhotoID = &a.CoverID
	}
	return out
}

func (s *server) listAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := s.Queries.ListAlbums(r.Context(), store.ListAlbumsParams{OwnerID: currentUser(r).ID})
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]albumJSON, len(albums))
	for i, a := range albums {
		out[i] = toAlbumJSON(a)
	}
	writeJSON(w, http.StatusOK, out)
}

// ownAlbum answers 404 for albums of other users, so that a response does not
// reveal which albums exist.
func (s *server) ownAlbum(w http.ResponseWriter, r *http.Request, id int64) (store.ListAlbumsRow, bool) {
	albums, err := s.Queries.ListAlbums(r.Context(), store.ListAlbumsParams{OwnerID: currentUser(r).ID, ID: &id})
	if err != nil {
		internalError(w, r, err)
		return store.ListAlbumsRow{}, false
	}
	if len(albums) == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return store.ListAlbumsRow{}, false
	}
	return albums[0], true
}

func (s *server) writeAlbum(w http.ResponseWriter, r *http.Request, id int64, status int) {
	if album, ok := s.ownAlbum(w, r, id); ok {
		writeJSON(w, status, toAlbumJSON(album))
	}
}

func (s *server) getAlbum(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		s.writeAlbum(w, r, id, http.StatusOK)
	}
}

func (s *server) createAlbum(w http.ResponseWriter, r *http.Request) {
	name, ok := readName(w, r, maxAlbumName)
	if !ok {
		return
	}
	id, err := s.Queries.CreateAlbum(r.Context(), store.CreateAlbumParams{OwnerID: currentUser(r).ID, Name: name})
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.writeAlbum(w, r, id, http.StatusCreated)
}

func (s *server) renameAlbum(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	name, ok := readName(w, r, maxAlbumName)
	if !ok {
		return
	}
	rows, err := s.Queries.RenameAlbum(r.Context(), store.RenameAlbumParams{ID: id, OwnerID: currentUser(r).ID, Name: name})
	if !changed(w, r, rows, err) {
		return
	}
	s.writeAlbum(w, r, id, http.StatusOK)
}

// deleteAlbum keeps the album's photos.
func (s *server) deleteAlbum(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rows, err := s.Queries.DeleteAlbum(r.Context(), store.DeleteAlbumParams{ID: id, OwnerID: currentUser(r).ID})
	if changed(w, r, rows, err) {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *server) setAlbumCover(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		PhotoID *int64 `json:"photoId"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	owner := currentUser(r).ID
	var rows int64
	var err error
	if req.PhotoID == nil {
		rows, err = s.Queries.ClearAlbumCover(r.Context(), store.ClearAlbumCoverParams{ID: id, OwnerID: owner})
	} else {
		rows, err = s.Queries.SetAlbumCover(r.Context(), store.SetAlbumCoverParams{ID: id, OwnerID: owner, PhotoID: req.PhotoID})
	}
	if !changed(w, r, rows, err) {
		return
	}
	s.writeAlbum(w, r, id, http.StatusOK)
}

func (s *server) addAlbumPhotos(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ids, ok := readIDs(w, r)
	if !ok {
		return
	}
	if _, ok := s.ownAlbum(w, r, id); !ok {
		return
	}
	err := s.Queries.AddPhotosToAlbum(r.Context(), store.AddPhotosToAlbumParams{AlbumID: id, OwnerID: currentUser(r).ID, Ids: ids})
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) removeAlbumPhotos(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ids, ok := readIDs(w, r)
	if !ok {
		return
	}
	if _, ok := s.ownAlbum(w, r, id); !ok {
		return
	}
	err := s.Queries.RemovePhotosFromAlbum(r.Context(), store.RemovePhotosFromAlbumParams{AlbumID: id, OwnerID: currentUser(r).ID, Ids: ids})
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// changed reports whether an update found its row, and answers the request if not.
func changed(w http.ResponseWriter, r *http.Request, rows int64, err error) bool {
	if err != nil {
		internalError(w, r, err)
		return false
	}
	if rows == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return false
	}
	return true
}
