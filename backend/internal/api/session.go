package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const (
	sessionCookie   = "fuchsia_session"
	sessionLifetime = 30 * 24 * time.Hour
)

// Checked for unknown logins, so response times do not reveal which logins exist.
var dummyHash = sync.OnceValue(func() string {
	hash, err := auth.HashPassword("dummy password")
	if err != nil {
		panic(err)
	}
	return hash
})

type userKey struct{}

func currentUser(r *http.Request) store.User {
	return r.Context().Value(userKey{}).(store.User)
}

func (s *server) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		user, err := s.Queries.GetSessionUser(r.Context(), auth.HashSessionToken(cookie.Value))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey{}, user)))
	}
}

func (s *server) admin(next http.HandlerFunc) http.HandlerFunc {
	return s.authenticated(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).Role != auth.RoleAdmin {
			writeError(w, http.StatusForbidden, "administrators only")
			return
		}
		next(w, r)
	})
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	user, err := s.Queries.GetUserByLogin(r.Context(), req.Login)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		internalError(w, r, err)
		return
	}
	hash := user.PasswordHash
	if err != nil {
		hash = dummyHash()
	}
	if !auth.CheckPassword(hash, req.Password) || err != nil || user.Disabled {
		writeError(w, http.StatusUnauthorized, "wrong login or password")
		return
	}

	token, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		internalError(w, r, err)
		return
	}
	expires := time.Now().Add(sessionLifetime)
	err = s.Queries.CreateSession(r.Context(), store.CreateSessionParams{TokenHash: tokenHash, UserID: user.ID, ExpiresAt: expires})
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.setSessionCookie(w, token, expires)
	writeJSON(w, http.StatusOK, toUserJSON(user))
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if err := s.Queries.DeleteSession(r.Context(), auth.HashSessionToken(cookie.Value)); err != nil {
			internalError(w, r, err)
			return
		}
	}
	s.setSessionCookie(w, "", time.Unix(0, 0))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) setSessionCookie(w http.ResponseWriter, value string, expires time.Time) {
	// SameSite=Lax keeps other sites from sending state-changing requests with this cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *server) getMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toUserJSON(currentUser(r)))
}

func (s *server) updateMe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SharedIncludesAlbums *bool     `json:"sharedIncludesAlbums"`
		ViewerShowsOriginals *bool     `json:"viewerShowsOriginals"`
		ViewerHiddenFields   *[]string `json:"viewerHiddenFields"`

		NavigationHiddenOnPhones *bool `json:"navigationHiddenOnPhones"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	var hidden []string
	if req.ViewerHiddenFields != nil {
		var err error
		if hidden, err = normalizeViewerFields(*req.ViewerHiddenFields); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	user := currentUser(r)
	if req.SharedIncludesAlbums != nil {
		var err error
		user, err = s.Queries.SetUserSharedIncludesAlbums(r.Context(), store.SetUserSharedIncludesAlbumsParams{
			ID:                   user.ID,
			SharedIncludesAlbums: *req.SharedIncludesAlbums,
		})
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	if req.ViewerShowsOriginals != nil {
		var err error
		user, err = s.Queries.SetUserViewerShowsOriginals(r.Context(), store.SetUserViewerShowsOriginalsParams{
			ID:                   user.ID,
			ViewerShowsOriginals: *req.ViewerShowsOriginals,
		})
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	if req.ViewerHiddenFields != nil {
		var err error
		user, err = s.Queries.SetUserViewerHiddenFields(r.Context(), store.SetUserViewerHiddenFieldsParams{
			ID:                 user.ID,
			ViewerHiddenFields: hidden,
		})
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	if req.NavigationHiddenOnPhones != nil {
		var err error
		user, err = s.Queries.SetUserNavigationHiddenOnPhones(r.Context(), store.SetUserNavigationHiddenOnPhonesParams{
			ID:                       user.ID,
			NavigationHiddenOnPhones: *req.NavigationHiddenOnPhones,
		})
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, toUserJSON(user))
}

func (s *server) changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	user := currentUser(r)
	if !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		writeError(w, http.StatusForbidden, "the current password is wrong")
		return
	}
	if err := auth.ValidatePassword(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if err := s.Queries.SetUserPassword(r.Context(), store.SetUserPasswordParams{ID: user.ID, PasswordHash: hash}); err != nil {
		internalError(w, r, err)
		return
	}

	// Other devices must log in again with the new password.
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		internalError(w, r, err)
		return
	}
	err = s.Queries.DeleteUserSessionsExcept(r.Context(), store.DeleteUserSessionsExceptParams{
		UserID:    user.ID,
		TokenHash: auth.HashSessionToken(cookie.Value),
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
