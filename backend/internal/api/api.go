// Package api implements the REST API described in api/openapi.yaml.
package api

import (
	"context"
	"crypto/rand"
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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/magellanica-app/magellanica-photos/backend/internal/auth"
	"github.com/magellanica-app/magellanica-photos/backend/internal/config"
	"github.com/magellanica-app/magellanica-photos/backend/internal/ingest"
	"github.com/magellanica-app/magellanica-photos/backend/internal/jobs"
	"github.com/magellanica-app/magellanica-photos/backend/internal/library"
	"github.com/magellanica-app/magellanica-photos/backend/internal/ml"
	"github.com/magellanica-app/magellanica-photos/backend/internal/scan"
	"github.com/magellanica-app/magellanica-photos/backend/internal/storage"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
	"github.com/magellanica-app/magellanica-photos/backend/internal/transcode"
)

// Pinger reports whether the database is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the handler's dependencies.
type Deps struct {
	ActivityLog              bool
	ActivityMaintenanceToken string
	DB                       Pinger
	// Pool reads the job queue, which the generated queries do not cover.
	Pool    *pgxpool.Pool
	Queries *store.Queries
	Ingest  *ingest.Service
	Scan    *scan.Service
	Library *library.Service
	Layout  storage.Layout
	// ML embeds search texts. Its monitor tells whether the service is up.
	ML *ml.Service
	// Transcode makes H.264 copies of videos, where the graphics allow.
	Transcode    *transcode.Service
	CookieSecure bool
	// PublicURL, SessionSecret and OIDCProviders set up sign-in through providers.
	PublicURL     string
	SessionSecret string
	OIDCProviders []config.OIDCProvider
	// TOTP encrypts the secrets of two-factor sign-in and signs the step between password and code.
	TOTP auth.TOTPKey
	// WebDAV serves the WebDAV tree under /webdav, if set.
	WebDAV http.Handler
	// Static serves everything outside /api/.
	Static http.Handler
}

type server struct {
	Deps
	// logins slows down repeated wrong passwords for one login.
	logins *auth.Limiter
	// codes does the same for wrong two-factor codes, per user id. It is apart from logins, whose
	// keys are whatever a visitor types as a login.
	codes *auth.Limiter
	// linkTries slows down wrong passwords of one public link, and linkKey signs the cookie that
	// says a visitor gave it right.
	linkTries *auth.Limiter
	linkKey   []byte
	oidc      []*oidcProvider
}

// NewHandler returns the application handler.
func NewHandler(deps Deps) http.Handler {
	if deps.ML == nil {
		deps.ML = &ml.Service{Monitor: ml.NewMonitor(nil, "")}
	}
	if deps.Transcode == nil {
		deps.Transcode = &transcode.Service{Queries: deps.Queries, Layout: deps.Layout}
	}
	s := &server{Deps: deps, logins: auth.NewLimiter(), codes: auth.NewLimiter(), linkTries: auth.NewLimiter()}
	// Without SESSION_SECRET the key is made at start, so visitors give a link's password again after a restart.
	s.linkKey = []byte(deps.SessionSecret)
	if len(s.linkKey) == 0 {
		s.linkKey = make([]byte, 32)
		if _, err := rand.Read(s.linkKey); err != nil {
			panic(err)
		}
	}
	for _, p := range deps.OIDCProviders {
		s.oidc = append(s.oidc, &oidcProvider{OIDCProvider: p})
	}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("POST /api/v1/login", s.login)
	mux.HandleFunc("POST /api/v1/login/totp", s.loginTOTP)
	mux.HandleFunc("POST /api/v1/logout", s.logout)
	mux.HandleFunc("GET /api/v1/auth/options", s.listSignInOptions)
	mux.HandleFunc("GET /api/v1/auth/oidc/{id}/login", s.oidcLogin)
	mux.HandleFunc("GET /api/v1/auth/oidc/{id}/callback", s.oidcCallback)

	// What visitors of a public link use. A proxy or tunnel can open these paths, /s/ and the static files
	// to the internet and keep the rest of the application home.
	mux.HandleFunc("GET /api/v1/public/{token}", s.getPublicAlbum)
	mux.HandleFunc("POST /api/v1/public/{token}/unlock", s.unlockPublicAlbum)
	mux.HandleFunc("GET /api/v1/public/{token}/photos", s.listPublicPhotos)
	mux.HandleFunc("GET /api/v1/public/{token}/photos/{id}/previews/{kind}", s.getPublicPreview)
	mux.HandleFunc("GET /api/v1/public/{token}/photos/{id}/video", s.getPublicVideo)

	mux.HandleFunc("GET /api/v1/me", s.authenticated(s.getMe))
	mux.HandleFunc("PATCH /api/v1/me", s.authenticated(s.updateMe))
	mux.HandleFunc("POST /api/v1/me/password", s.authenticated(s.changePassword))
	mux.HandleFunc("GET /api/v1/me/2fa", s.authenticated(s.getTwoFactor))
	mux.HandleFunc("POST /api/v1/me/2fa/setup", s.authenticated(s.setupTwoFactor))
	mux.HandleFunc("POST /api/v1/me/2fa/enable", s.authenticated(s.enableTwoFactor))
	mux.HandleFunc("POST /api/v1/me/2fa/disable", s.authenticated(s.disableTwoFactor))
	mux.HandleFunc("GET /api/v1/me/identities", s.authenticated(s.listIdentities))
	mux.HandleFunc("DELETE /api/v1/me/identities/{id}", s.authenticated(s.deleteIdentity))
	mux.HandleFunc("GET /api/v1/me/app-passwords", s.authenticated(s.listAppPasswords))
	mux.HandleFunc("POST /api/v1/me/app-passwords", s.authenticated(s.createAppPassword))
	mux.HandleFunc("DELETE /api/v1/me/app-passwords/{id}", s.authenticated(s.deleteAppPassword))
	mux.HandleFunc("GET /api/v1/me/webdav-folders", s.authenticated(s.listWebDAVFolders))
	mux.HandleFunc("GET /api/v1/me/webdav-albums", s.authenticated(s.listWebDAVAlbums))
	mux.HandleFunc("POST /api/v1/me/webdav-albums", s.authenticated(s.createWebDAVAlbum))
	mux.HandleFunc("DELETE /api/v1/me/webdav-albums/{id}", s.authenticated(s.deleteWebDAVAlbum))

	mux.HandleFunc("GET /api/v1/users", s.admin(s.listUsers))
	mux.HandleFunc("POST /api/v1/users", s.admin(s.createUser))
	mux.HandleFunc("GET /api/v1/users/directory", s.authenticated(s.listUserDirectory))
	mux.HandleFunc("PATCH /api/v1/users/{id}", s.admin(s.updateUser))

	mux.HandleFunc("GET /api/v1/activity", s.admin(s.getActivity))

	mux.HandleFunc("GET /api/v1/photos", s.authenticated(s.listPhotos))
	mux.HandleFunc("POST /api/v1/photos", s.authenticated(s.uploadPhoto))
	mux.HandleFunc("GET /api/v1/photos/events", s.authenticated(s.photoEvents))
	mux.HandleFunc("GET /api/v1/photos/months", s.authenticated(s.countPhotoMonths))
	mux.HandleFunc("GET /api/v1/photos/memories", s.authenticated(s.listMemories))
	mux.HandleFunc("GET /api/v1/photos/map", s.authenticated(s.listMap))
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
	mux.HandleFunc("POST /api/v1/photos/favorites", s.authenticated(s.changeFavorites))
	mux.HandleFunc("POST /api/v1/photos/archive", s.authenticated(s.changeArchive))
	mux.HandleFunc("GET /api/v1/photos/{id}", s.authenticated(s.getPhoto))
	mux.HandleFunc("PATCH /api/v1/photos/{id}", s.authenticated(s.updatePhoto))
	mux.HandleFunc("GET /api/v1/photos/{id}/tags", s.authenticated(s.getPhotoTags))
	mux.HandleFunc("GET /api/v1/photos/{id}/sharing", s.authenticated(s.getPhotoSharing))
	mux.HandleFunc("GET /api/v1/photos/{id}/previews/{kind}", s.authenticated(s.getPreview))
	mux.HandleFunc("GET /api/v1/photos/{id}/original", s.authenticated(s.getOriginal))
	mux.HandleFunc("GET /api/v1/photos/{id}/video", s.authenticated(s.getVideo))
	mux.HandleFunc("GET /api/v1/photos/{id}/motion", s.authenticated(s.getMotion))
	mux.HandleFunc("GET /api/v1/photos/{id}/copy", s.authenticated(s.getVideoCopy))
	mux.HandleFunc("POST /api/v1/photos/{id}/copy", s.authenticated(s.requestVideoCopy))
	mux.HandleFunc("GET /api/v1/photos/{id}/similar", s.authenticated(s.listSimilarPhotos))

	mux.HandleFunc("GET /api/v1/search", s.authenticated(s.searchPhotos))
	mux.HandleFunc("GET /api/v1/search/status", s.authenticated(s.getSearchStatus))
	mux.HandleFunc("GET /api/v1/ml", s.admin(s.getMLStatus))

	mux.HandleFunc("GET /api/v1/albums", s.authenticated(s.listAlbums))
	mux.HandleFunc("POST /api/v1/albums", s.authenticated(s.createAlbum))
	mux.HandleFunc("GET /api/v1/albums/shared", s.authenticated(s.listSharedAlbums))
	mux.HandleFunc("GET /api/v1/albums/{id}", s.authenticated(s.getAlbum))
	mux.HandleFunc("PATCH /api/v1/albums/{id}", s.authenticated(s.renameAlbum))
	mux.HandleFunc("DELETE /api/v1/albums/{id}", s.authenticated(s.deleteAlbum))
	mux.HandleFunc("PUT /api/v1/albums/{id}/cover", s.authenticated(s.setAlbumCover))
	mux.HandleFunc("GET /api/v1/albums/{id}/links", s.authenticated(s.listAlbumLinks))
	mux.HandleFunc("POST /api/v1/albums/{id}/links", s.authenticated(s.createAlbumLink))
	mux.HandleFunc("DELETE /api/v1/albums/{id}/links/{linkId}", s.authenticated(s.deleteAlbumLink))
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
	mux.HandleFunc("GET /api/v1/library-folders", s.admin(s.listLibraryFolders))
	mux.HandleFunc("POST /api/v1/library-folders", s.admin(s.createLibraryFolder))
	mux.HandleFunc("GET /api/v1/library-folders/available", s.admin(s.listAvailableLibraryFolders))
	mux.HandleFunc("PATCH /api/v1/library-folders/{id}", s.admin(s.updateLibraryFolder))
	mux.HandleFunc("DELETE /api/v1/library-folders/{id}", s.admin(s.deleteLibraryFolder))
	mux.HandleFunc("POST /api/v1/library-folders/{id}/scan", s.admin(s.scanLibraryFolder))
	mux.HandleFunc("GET /api/v1/library-folders/{id}/files", s.admin(s.listLibraryFiles))

	mux.HandleFunc("GET /api/v1/settings/previews", s.admin(s.getPreviewSettings))
	mux.HandleFunc("PUT /api/v1/settings/previews", s.admin(s.updatePreviewSettings))
	mux.HandleFunc("GET /api/v1/settings/metadata", s.admin(s.getMetadataSettings))
	mux.HandleFunc("PUT /api/v1/settings/metadata", s.admin(s.updateMetadataSettings))
	mux.HandleFunc("GET /api/v1/settings/sign-in", s.admin(s.getSignInSettings))
	mux.HandleFunc("PUT /api/v1/settings/sign-in", s.admin(s.updateSignInSettings))
	mux.HandleFunc("GET /api/v1/settings/video", s.admin(s.getVideoSettings))
	mux.HandleFunc("PUT /api/v1/settings/video", s.admin(s.updateVideoSettings))

	mux.HandleFunc("GET /api/v1/queue", s.admin(s.getQueue))
	mux.HandleFunc("GET /api/v1/queue/events", s.admin(s.queueEvents))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	if deps.WebDAV != nil {
		// Both, as a client may be set up with either, and a redirect would lose
		// the method of a WebDAV request.
		mux.Handle("/webdav", deps.WebDAV)
		mux.Handle("/webdav/", deps.WebDAV)
	}
	mux.Handle("/", deps.Static)
	return logRequests(mux)
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

func (s *server) getQueue(w http.ResponseWriter, r *http.Request) {
	stats, err := s.readQueue(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *server) readQueue(ctx context.Context) (jobs.QueueStats, error) {
	stats, err := jobs.ReadQueueStats(ctx, s.Pool)
	if stats.Kinds == nil {
		stats.Kinds = []jobs.KindStats{}
	}
	if stats.Errors == nil {
		stats.Errors = []jobs.JobError{}
	}
	return stats, err
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
