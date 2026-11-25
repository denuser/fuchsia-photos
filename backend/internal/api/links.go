package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/magellanica-app/magellanica-photos/backend/internal/auth"
	"github.com/magellanica-app/magellanica-photos/backend/internal/storage"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
	"github.com/magellanica-app/magellanica-photos/backend/internal/transcode"
)

const (
	maxAlbumLinks = 10
	maxLinkDays   = 365
	// unlockLifetime is how long a visitor stays unlocked after giving a link's password.
	unlockLifetime = 12 * time.Hour
	publicBase     = "/api/v1/public/"
	linkCookie     = "magellanica_link"
)

// publicPath is the page a link opens, on which a tunnel or proxy can allow the whole of it:
// the page under /s/, the API under /api/v1/public/ and the static files.
func publicPath(token string) string { return "/s/" + token }

type linkJSON struct {
	ID int64 `json:"id"`
	// Path is the page's address on this server. URL is the full address, built from PUBLIC_URL, and is empty
	// when that is not set, so the interface falls back to the address it is opened at.
	Path        string     `json:"path"`
	URL         string     `json:"url"`
	HasPassword bool       `json:"hasPassword"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

func (s *server) toLinkJSON(l store.AlbumLink) linkJSON {
	out := linkJSON{ID: l.ID, Path: publicPath(l.Token), HasPassword: l.PasswordHash != "", ExpiresAt: l.ExpiresAt, CreatedAt: l.CreatedAt}
	if s.PublicURL != "" {
		out.URL = s.PublicURL + out.Path
	}
	return out
}

func (s *server) listAlbumLinks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	links, err := s.Queries.ListAlbumLinks(r.Context(), store.ListAlbumLinksParams{AlbumID: id, OwnerID: currentUser(r).ID})
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]linkJSON, len(links))
	for i, l := range links {
		out[i] = s.toLinkJSON(l)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) createAlbumLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		ExpiresInDays *int   `json:"expiresInDays"`
		Password      string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	var expires *time.Time
	if req.ExpiresInDays != nil {
		if *req.ExpiresInDays < 1 || *req.ExpiresInDays > maxLinkDays {
			writeError(w, http.StatusBadRequest, "a link lasts 1 to 365 days")
			return
		}
		at := time.Now().Add(time.Duration(*req.ExpiresInDays) * 24 * time.Hour)
		expires = &at
	}
	hash := ""
	if req.Password != "" {
		if err := auth.ValidatePassword(req.Password); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var err error
		if hash, err = auth.HashPassword(req.Password); err != nil {
			internalError(w, r, err)
			return
		}
	}
	count, err := s.Queries.CountAlbumLinks(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if count >= maxAlbumLinks {
		writeError(w, http.StatusConflict, "an album can have at most 10 links")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		internalError(w, r, err)
		return
	}
	link, err := s.Queries.CreateAlbumLink(r.Context(), store.CreateAlbumLinkParams{
		AlbumID: id, OwnerID: currentUser(r).ID, Token: base64.RawURLEncoding.EncodeToString(raw), PasswordHash: hash, ExpiresAt: expires,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "album not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.toLinkJSON(link))
}

func (s *server) deleteAlbumLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	linkID, err := strconv.ParseInt(r.PathValue("linkId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	n, err := s.Queries.DeleteAlbumLink(r.Context(), store.DeleteAlbumLinkParams{ID: linkID, AlbumID: id, OwnerID: currentUser(r).ID})
	if err != nil {
		internalError(w, r, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// publicHeaders keep what visitors see out of search engines and out of the Referer of what they follow.
func publicHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// publicLink finds the link of the request. An unknown, expired or revoked link, and one of a disabled
// user, are all the same 404. A link with a password is open only to a visitor who gave it, unless the
// caller handles the locked case itself.
func (s *server) publicLink(w http.ResponseWriter, r *http.Request, allowLocked bool) (store.GetPublicLinkRow, string, bool, bool) {
	publicHeaders(w)
	token := r.PathValue("token")
	link, err := s.Queries.GetPublicLink(r.Context(), token)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return link, token, false, false
	}
	if err != nil {
		internalError(w, r, err)
		return link, token, false, false
	}
	unlocked := link.PasswordHash == "" || s.linkUnlocked(r, token)
	if !unlocked && !allowLocked {
		writeError(w, http.StatusUnauthorized, "password required")
		return link, token, false, false
	}
	return link, token, unlocked, true
}

func (s *server) linkMAC(token, expires string) string {
	mac := hmac.New(sha256.New, s.linkKey)
	mac.Write([]byte("album-link:" + token + "." + expires))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *server) linkUnlocked(r *http.Request, token string) bool {
	cookie, err := r.Cookie(linkCookie)
	if err != nil {
		return false
	}
	expires, mac, ok := strings.Cut(cookie.Value, ".")
	if !ok || !hmac.Equal([]byte(mac), []byte(s.linkMAC(token, expires))) {
		return false
	}
	at, err := strconv.ParseInt(expires, 10, 64)
	return err == nil && time.Now().Unix() < at
}

type publicAlbumJSON struct {
	PasswordRequired bool `json:"passwordRequired"`
	// Unlocked is false until a link with a password gets it; the album's name stays out of the answer until then.
	Unlocked  bool       `json:"unlocked"`
	Name      string     `json:"name,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

func (s *server) getPublicAlbum(w http.ResponseWriter, r *http.Request) {
	link, _, unlocked, ok := s.publicLink(w, r, true)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	out := publicAlbumJSON{PasswordRequired: link.PasswordHash != "", Unlocked: unlocked}
	if unlocked {
		out.Name, out.ExpiresAt = link.AlbumName, link.ExpiresAt
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) unlockPublicAlbum(w http.ResponseWriter, r *http.Request) {
	link, token, _, ok := s.publicLink(w, r, true)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	// The limit is per link, so guessing one link's password is slowed down however many addresses guess.
	if ok, wait := s.linkTries.Allow(token); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	if link.PasswordHash == "" || !auth.CheckPassword(link.PasswordHash, req.Password) {
		if link.PasswordHash != "" {
			s.linkTries.Fail(token)
		}
		writeError(w, http.StatusUnauthorized, "wrong password")
		return
	}
	s.linkTries.Reset(token)
	expires := time.Now().Add(unlockLifetime)
	text := strconv.FormatInt(expires.Unix(), 10)
	http.SetCookie(w, &http.Cookie{
		Name:     linkCookie,
		Value:    text + "." + s.linkMAC(token, text),
		Path:     publicBase + token,
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

type publicPhotoJSON struct {
	ID     int64  `json:"id"`
	Kind   string `json:"kind"`
	Width  *int32 `json:"width"`
	Height *int32 `json:"height"`
	// TakenAt is the date only: names, cameras and places stay out of what visitors get.
	TakenAt        *time.Time `json:"takenAt"`
	Duration       *float64   `json:"duration"`
	PreviewVersion int32      `json:"previewVersion"`
}

func (s *server) listPublicPhotos(w http.ResponseWriter, r *http.Request) {
	link, _, _, ok := s.publicLink(w, r, false)
	if !ok {
		return
	}
	rows, err := s.Queries.ListPublicPhotos(r.Context(), link.AlbumID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]publicPhotoJSON, len(rows))
	for i, p := range rows {
		kind := "photo"
		if storage.IsVideo(p.Mime) {
			kind = "video"
		}
		out[i] = publicPhotoJSON{ID: p.ID, Kind: kind, Width: p.Width, Height: p.Height, TakenAt: p.TakenAt, Duration: p.Duration, PreviewVersion: p.PreviewVersion}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// publicPhoto finds a photo of the link's album.
func (s *server) publicPhoto(w http.ResponseWriter, r *http.Request) (store.Photo, bool) {
	link, _, _, ok := s.publicLink(w, r, false)
	if !ok {
		return store.Photo{}, false
	}
	id, ok := pathID(w, r)
	if !ok {
		return store.Photo{}, false
	}
	photo, err := s.Queries.GetPublicPhoto(r.Context(), store.GetPublicPhotoParams{AlbumID: link.AlbumID, PhotoID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return photo, false
	}
	if err != nil {
		internalError(w, r, err)
		return photo, false
	}
	return photo, true
}

func (s *server) getPublicPreview(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind != storage.PreviewGrid && kind != storage.PreviewViewer {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	photo, ok := s.publicPhoto(w, r)
	if !ok {
		return
	}
	if photo.PreviewFormat == nil {
		writeError(w, http.StatusNotFound, "the preview is not ready")
		return
	}
	// Private, so a proxy or a CDN in front does not keep what a revoked link showed.
	w.Header().Set("Cache-Control", previewCacheRule)
	w.Header().Set("Content-Type", "image/"+*photo.PreviewFormat)
	s.serveFile(w, r, s.Layout.Preview(photo.ID, kind, *photo.PreviewFormat))
}

// getPublicVideo plays a video of the album: its H.264 copy when there is one, else the file itself.
// Photos have no original here, only previews, which carry no EXIF or GPS.
func (s *server) getPublicVideo(w http.ResponseWriter, r *http.Request) {
	photo, ok := s.publicPhoto(w, r)
	if !ok {
		return
	}
	if !storage.IsVideo(photo.Mime) {
		writeError(w, http.StatusNotFound, "not a video")
		return
	}
	path := s.Layout.OriginalOf(photo.OwnerID, photo.Sha256, photo.Mime, photo.SourcePath)
	if photo.VideoCopy != nil && *photo.VideoCopy == transcode.CopyReady {
		path = s.Layout.Copy(photo.ID)
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "private, no-cache")
	s.serveFile(w, r, path)
}
