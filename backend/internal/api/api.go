// Package api implements the REST API described in api/openapi.yaml.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/scan"
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
	Scan         *scan.Service
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
	mux.HandleFunc("PATCH /api/v1/me", s.authenticated(s.updateMe))
	mux.HandleFunc("POST /api/v1/me/password", s.authenticated(s.changePassword))

	mux.HandleFunc("GET /api/v1/users", s.admin(s.listUsers))
	mux.HandleFunc("POST /api/v1/users", s.admin(s.createUser))
	mux.HandleFunc("GET /api/v1/users/directory", s.authenticated(s.listUserDirectory))
	mux.HandleFunc("PATCH /api/v1/users/{id}", s.admin(s.updateUser))

	mux.HandleFunc("GET /api/v1/photos", s.authenticated(s.listPhotos))
	mux.HandleFunc("POST /api/v1/photos", s.authenticated(s.uploadPhoto))
	mux.HandleFunc("GET /api/v1/photos/events", s.authenticated(s.photoEvents))
	mux.HandleFunc("GET /api/v1/photos/months", s.authenticated(s.countPhotoMonths))
	mux.HandleFunc("GET /api/v1/photos/cameras", s.authenticated(s.listCameras))
	mux.HandleFunc("GET /api/v1/photos/shared", s.authenticated(s.listSharedPhotos))
	mux.HandleFunc("GET /api/v1/photos/shared/months", s.authenticated(s.countSharedMonths))
	mux.HandleFunc("GET /api/v1/photos/all", s.admin(s.listAllPhotos))
	mux.HandleFunc("GET /api/v1/photos/all/months", s.admin(s.countAllMonths))
	mux.HandleFunc("POST /api/v1/photos/sharing", s.authenticated(s.changePhotoSharing))
	mux.HandleFunc("GET /api/v1/photos/trash", s.authenticated(s.listTrash))
	mux.HandleFunc("POST /api/v1/photos/trash", s.authenticated(s.trashPhotos))
	mux.HandleFunc("POST /api/v1/photos/trash/empty", s.authenticated(s.emptyTrash))
	mux.HandleFunc("POST /api/v1/photos/restore", s.authenticated(s.restorePhotos))
	mux.HandleFunc("POST /api/v1/photos/purge", s.authenticated(s.purgePhotos))
	mux.HandleFunc("POST /api/v1/photos/tags", s.authenticated(s.changePhotoTags))
	mux.HandleFunc("GET /api/v1/photos/{id}", s.authenticated(s.getPhoto))
	mux.HandleFunc("GET /api/v1/photos/{id}/tags", s.authenticated(s.getPhotoTags))
	mux.HandleFunc("GET /api/v1/photos/{id}/sharing", s.authenticated(s.getPhotoSharing))
	mux.HandleFunc("GET /api/v1/photos/{id}/previews/{kind}", s.authenticated(s.getPreview))
	mux.HandleFunc("GET /api/v1/photos/{id}/original", s.authenticated(s.getOriginal))

	mux.HandleFunc("GET /api/v1/albums", s.authenticated(s.listAlbums))
	mux.HandleFunc("POST /api/v1/albums", s.authenticated(s.createAlbum))
	mux.HandleFunc("GET /api/v1/albums/shared", s.authenticated(s.listSharedAlbums))
	mux.HandleFunc("GET /api/v1/albums/{id}", s.authenticated(s.getAlbum))
	mux.HandleFunc("PATCH /api/v1/albums/{id}", s.authenticated(s.renameAlbum))
	mux.HandleFunc("DELETE /api/v1/albums/{id}", s.authenticated(s.deleteAlbum))
	mux.HandleFunc("PUT /api/v1/albums/{id}/cover", s.authenticated(s.setAlbumCover))
	mux.HandleFunc("GET /api/v1/albums/{id}/sharing", s.authenticated(s.getAlbumSharing))
	mux.HandleFunc("POST /api/v1/albums/{id}/sharing", s.authenticated(s.changeAlbumSharing))
	mux.HandleFunc("POST /api/v1/albums/{id}/photos", s.authenticated(s.addAlbumPhotos))
	mux.HandleFunc("POST /api/v1/albums/{id}/photos/remove", s.authenticated(s.removeAlbumPhotos))

	mux.HandleFunc("GET /api/v1/tags", s.authenticated(s.listTags))
	mux.HandleFunc("POST /api/v1/tags", s.authenticated(s.createTag))
	mux.HandleFunc("PATCH /api/v1/tags/{id}", s.authenticated(s.renameTag))
	mux.HandleFunc("DELETE /api/v1/tags/{id}", s.authenticated(s.deleteTag))

	mux.HandleFunc("GET /api/v1/imports", s.admin(s.listImports))
	mux.HandleFunc("POST /api/v1/imports", s.admin(s.startImport))
	mux.HandleFunc("GET /api/v1/imports/folders", s.admin(s.listImportFolders))
	mux.HandleFunc("GET /api/v1/imports/{id}", s.admin(s.getImport))
	mux.HandleFunc("POST /api/v1/imports/{id}/stop", s.admin(s.stopImport))

	mux.HandleFunc("GET /api/v1/settings/grid", s.authenticated(s.getGridSettings))
	mux.HandleFunc("GET /api/v1/settings/previews", s.admin(s.getPreviewSettings))
	mux.HandleFunc("PUT /api/v1/settings/previews", s.admin(s.updatePreviewSettings))
	mux.HandleFunc("GET /api/v1/settings/metadata", s.admin(s.getMetadataSettings))
	mux.HandleFunc("PUT /api/v1/settings/metadata", s.admin(s.updateMetadataSettings))

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

// readName reads a required display name, such as that of an album or a tag.
func readName(w http.ResponseWriter, r *http.Request, maxRunes int) (string, bool) {
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return "", false
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > maxRunes || strings.ContainsFunc(name, unicode.IsControl) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("a name is 1 to %d characters, without control characters", maxRunes))
		return "", false
	}
	return name, true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}
