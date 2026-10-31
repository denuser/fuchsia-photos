// Package api implements the REST API described in api/openapi.yaml.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

// Pinger reports whether the database is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the handler's dependencies.
type Deps struct {
	DB           Pinger
	Queries      *store.Queries
	Ingest       *ingest.Service
	Layout       storage.Layout
	CookieSecure bool
	// Static serves everything outside /api/.
	Static http.Handler
}

type server struct {
	Deps
}

// NewHandler returns the application handler.
func NewHandler(deps Deps) http.Handler {
	s := &server{deps}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("POST /api/v1/login", s.login)
	mux.HandleFunc("POST /api/v1/logout", s.logout)

	mux.HandleFunc("GET /api/v1/me", s.authenticated(s.getMe))
	mux.HandleFunc("POST /api/v1/me/password", s.authenticated(s.changePassword))

	mux.HandleFunc("GET /api/v1/users", s.admin(s.listUsers))
	mux.HandleFunc("POST /api/v1/users", s.admin(s.createUser))
	mux.HandleFunc("PATCH /api/v1/users/{id}", s.admin(s.updateUser))

	mux.HandleFunc("GET /api/v1/photos", s.authenticated(s.listPhotos))
	mux.HandleFunc("POST /api/v1/photos", s.authenticated(s.uploadPhoto))
	mux.HandleFunc("GET /api/v1/photos/events", s.authenticated(s.photoEvents))
	mux.HandleFunc("GET /api/v1/photos/{id}", s.authenticated(s.getPhoto))
	mux.HandleFunc("GET /api/v1/photos/{id}/previews/{kind}", s.authenticated(s.getPreview))
	mux.HandleFunc("GET /api/v1/photos/{id}/original", s.authenticated(s.getOriginal))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	mux.Handle("/", deps.Static)
	return mux
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.DB.Ping(ctx); err != nil {
		slog.Error("health check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// internalError logs the cause, which may hold details the client must not see.
func internalError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) {
		return // The client went away.
	}
	slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func readJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}
