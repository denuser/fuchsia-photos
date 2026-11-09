package api

import (
	"net/http"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const (
	maxAppPasswords       = 20
	maxAppPasswordNameLen = 64
)

type appPasswordJSON struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

func toAppPasswordJSON(p store.AppPassword) appPasswordJSON {
	return appPasswordJSON{ID: p.ID, Name: p.Name, CreatedAt: p.CreatedAt, LastUsedAt: p.LastUsedAt}
}

func (s *server) listAppPasswords(w http.ResponseWriter, r *http.Request) {
	passwords, err := s.Queries.ListAppPasswords(r.Context(), currentUser(r).ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]appPasswordJSON, len(passwords))
	for i, p := range passwords {
		out[i] = toAppPasswordJSON(p)
	}
	writeJSON(w, http.StatusOK, out)
}

// createAppPassword returns the password itself only here: the server keeps
// only its hash.
func (s *server) createAppPassword(w http.ResponseWriter, r *http.Request) {
	name, ok := readName(w, r, maxAppPasswordNameLen)
	if !ok {
		return
	}
	user := currentUser(r)
	count, err := s.Queries.CountAppPasswords(r.Context(), user.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if count >= maxAppPasswords {
		writeError(w, http.StatusConflict, "a user can have at most 20 app passwords")
		return
	}
	password, hash, err := auth.NewAppPassword()
	if err != nil {
		internalError(w, r, err)
		return
	}
	created, err := s.Queries.CreateAppPassword(r.Context(), store.CreateAppPasswordParams{UserID: user.ID, Name: name, TokenHash: hash})
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		appPasswordJSON
		Password string `json:"password"`
	}{toAppPasswordJSON(created), password})
}

func (s *server) deleteAppPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	deleted, err := s.Queries.DeleteAppPassword(r.Context(), store.DeleteAppPasswordParams{ID: id, UserID: currentUser(r).ID})
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
