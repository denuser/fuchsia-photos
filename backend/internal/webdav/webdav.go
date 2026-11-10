// Package webdav lets sync clients on phones upload photos over WebDAV.
//
// Each user sees a folder tree of their own, kept in the database. A file in it
// records an upload and points to the photo it became, so that a sync client
// finds what it has already sent and does not send it again. Uploads go through
// ingest, and deleting a file leaves its photo in the library.
//
// Clients log in with the user's login and an app password, never with the
// account password.
package webdav

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	dav "golang.org/x/net/webdav"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

// Prefix is where the WebDAV tree is served.
const Prefix = "/webdav"

const (
	// maxFileBytes matches the limit of browser uploads.
	maxFileBytes = 512 << 20
	// Limits that keep a path a sensible size for the database.
	maxPathBytes = 1024
	maxNameBytes = 255
	// touchInterval is how often the last use of an app password is recorded,
	// so that a sync of a thousand files does not write it a thousand times.
	touchInterval = time.Hour
)

// Handler serves the WebDAV tree of the user who logs in.
type Handler struct {
	Queries *store.Queries
	Ingest  *ingest.Service
	Layout  storage.Layout

	mu    sync.Mutex
	locks map[int64]dav.LockSystem
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	user, ok := h.authenticate(rec, r)
	if ok {
		h.serve(rec, r, user)
	}
	// While WebDAV is a hypothesis, every request is logged to see how real
	// clients behave.
	slog.Info("webdav", "method", r.Method, "path", r.URL.Path, "status", rec.status,
		"user", user.Login, "agent", r.UserAgent(), "duration", time.Since(start).Round(time.Millisecond))
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	login, password, ok := r.BasicAuth()
	if !ok {
		unauthorized(w)
		return store.User{}, false
	}
	row, err := h.Queries.GetAppPasswordUser(r.Context(), auth.HashAppPassword(password))
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !strings.EqualFold(row.User.Login, login) {
		unauthorized(w)
		return store.User{}, false
	}
	if err != nil {
		internalError(w, r, err)
		return store.User{}, false
	}
	if row.LastUsedAt == nil || time.Since(*row.LastUsedAt) > touchInterval {
		if err := h.Queries.TouchAppPassword(r.Context(), row.AppPasswordID); err != nil {
			slog.Warn("record app password use", "error", err)
		}
	}
	return row.User, true
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Fuchsia Photos", charset="UTF-8"`)
	http.Error(w, "log in with your login and an app password", http.StatusUnauthorized)
}

func internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("webdav request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request, user store.User) {
	fs := &fileSystem{queries: h.Queries, userID: user.ID}
	switch r.Method {
	case http.MethodPut:
		h.put(w, r, fs)
	case http.MethodGet, http.MethodHead:
		h.get(w, r, fs)
	case http.MethodPost, "COPY":
		// A copy would be a second record of a photo the user already has.
		http.Error(w, "not supported", http.StatusMethodNotAllowed)
	default:
		if r.Method == "MOVE" {
			// x/net/webdav refuses a destination on another host, which is what a
			// reverse proxy that rewrites Host makes of it. A move stays inside the
			// user's tree anyway, so only the path matters.
			if u, err := url.Parse(r.Header.Get("Destination")); err == nil && u.Host != "" {
				r.Header.Set("Destination", u.EscapedPath())
			}
		}
		(&dav.Handler{
			Prefix:     Prefix,
			FileSystem: fs,
			LockSystem: h.lockSystem(user.ID),
			Logger: func(r *http.Request, err error) {
				if err != nil && !os.IsNotExist(err) {
					slog.Warn("webdav", "method", r.Method, "path", r.URL.Path, "error", err)
				}
			},
		}).ServeHTTP(w, r)
	}
}

// lockSystem returns the user's locks. Users have their own, as the same path
// in two users' trees names two different files.
func (h *Handler) lockSystem(userID int64) dav.LockSystem {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.locks == nil {
		h.locks = make(map[int64]dav.LockSystem)
	}
	ls, ok := h.locks[userID]
	if !ok {
		ls = dav.NewMemLS()
		h.locks[userID] = ls
	}
	return ls
}

// requestPath returns the path in the user's tree that a request names.
func requestPath(r *http.Request) (string, bool) {
	rest, ok := strings.CutPrefix(r.URL.Path, Prefix)
	if !ok || rest != "" && rest[0] != '/' {
		return "", false
	}
	return cleanPath(rest), true
}

func cleanPath(name string) string {
	return path.Clean("/" + name)
}

// FolderPath returns the path in a user's tree that a folder link names, such
// as "/Camera" for "Camera/". The root "/" links the whole tree.
func FolderPath(name string) (string, bool) {
	p := cleanPath(strings.TrimSpace(name))
	return p, p == "/" || validPath(p)
}

// validPath reports whether a new file or folder may have this path.
func validPath(p string) bool {
	if p == "/" || len(p) > maxPathBytes || !utf8.ValidString(p) || strings.ContainsFunc(p, unicode.IsControl) {
		return false
	}
	for _, name := range strings.Split(p[1:], "/") {
		if len(name) > maxNameBytes {
			return false
		}
	}
	return true
}

// put sends an uploaded file to ingest and records it in the tree. The file is
// not kept as it is: the library keeps the original, once per owner.
func (h *Handler) put(w http.ResponseWriter, r *http.Request, fs *fileSystem) {
	ctx := r.Context()
	name, ok := requestPath(r)
	if !ok || !validPath(name) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	existing, err := fs.entry(ctx, name)
	switch {
	case err == nil && existing.IsDir:
		http.Error(w, "a folder has this name", http.StatusMethodNotAllowed)
		return
	case err != nil && !os.IsNotExist(err):
		internalError(w, r, err)
		return
	}
	replaced := err == nil
	parent, err := fs.entry(ctx, path.Dir(name))
	if os.IsNotExist(err) || err == nil && !parent.IsDir {
		http.Error(w, "the folder does not exist", http.StatusConflict)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxFileBytes)
	res, err := h.Ingest.Receive(ctx, fs.userID, body, path.Base(name))
	var tooBig *http.MaxBytesError
	switch {
	case errors.Is(err, ingest.ErrUnsupported):
		http.Error(w, "only JPEG, PNG, WebP and HEIC photos are supported", http.StatusUnsupportedMediaType)
		return
	case errors.As(err, &tooBig):
		http.Error(w, "the file is too large", http.StatusRequestEntityTooLarge)
		return
	case err != nil:
		internalError(w, r, err)
		return
	}

	if err := h.addToAlbum(ctx, fs.userID, name, res.Photo); err != nil {
		internalError(w, r, err)
		return
	}

	modified := time.Now()
	// Some clients send the file's own modification time this way, as ownCloud
	// and Nextcloud accept it. Keeping it lets them see the file as unchanged.
	if mtime, err := strconv.ParseInt(r.Header.Get("X-OC-Mtime"), 10, 64); err == nil && mtime > 0 {
		modified = time.Unix(mtime, 0)
		w.Header().Set("X-OC-Mtime", "accepted")
	}
	// The database keeps microseconds, and the ETag must not change on reading.
	modified = modified.Truncate(time.Microsecond)
	err = h.Queries.PutWebDAVFile(ctx, store.PutWebDAVFileParams{
		UserID:     fs.userID,
		Path:       name,
		Parent:     path.Dir(name),
		Size:       res.Photo.Size,
		ModifiedAt: modified,
		PhotoID:    &res.Photo.ID,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.Header().Set("ETag", etag(modified, res.Photo.Size))
	if replaced {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// addToAlbum adds an uploaded photo to the album linked to its folder. This
// includes a photo the library already had, unless it is in the trash. It runs
// before the file is recorded, so that a client sends a file again after a
// failure.
func (h *Handler) addToAlbum(ctx context.Context, userID int64, name string, photo store.Photo) error {
	albumID, err := h.Queries.FindWebDAVAlbum(ctx, store.FindWebDAVAlbumParams{UserID: userID, FilePath: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find the album of %s: %w", name, err)
	}
	err = h.Queries.AddPhotosToAlbum(ctx, store.AddPhotosToAlbumParams{AlbumID: albumID, OwnerID: userID, Ids: []int64{photo.ID}})
	if err != nil {
		return fmt.Errorf("add a photo to album %d: %w", albumID, err)
	}
	return nil
}

// get serves the original of the photo a file became, while it is there.
func (h *Handler) get(w http.ResponseWriter, r *http.Request, fs *fileSystem) {
	ctx := r.Context()
	name, ok := requestPath(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	entry, err := fs.entry(ctx, name)
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if entry.IsDir {
		http.Error(w, "a folder", http.StatusMethodNotAllowed)
		return
	}
	if entry.PhotoID == nil {
		http.NotFound(w, r)
		return
	}
	photo, err := h.Queries.GetWebDAVPhoto(ctx, store.GetWebDAVPhotoParams{ID: *entry.PhotoID, OwnerID: fs.userID})
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	file, err := os.Open(h.Layout.Original(photo.OwnerID, photo.Sha256, photo.Mime))
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", photo.Mime)
	w.Header().Set("ETag", etag(entry.ModifiedAt, entry.Size))
	http.ServeContent(w, r, path.Base(name), entry.ModifiedAt, file)
}

// etag is the one x/net/webdav makes from the modification time and the size,
// so that a PUT and a PROPFIND give the same.
func etag(modified time.Time, size int64) string {
	return fmt.Sprintf(`"%x%x"`, modified.UnixNano(), size)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}
