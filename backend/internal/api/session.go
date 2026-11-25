package api

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/magellanica-app/magellanica-photos/backend/internal/auth"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

const (
	sessionCookie   = "magellanica_session"
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
type activitySessionKey struct{}

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
		session, err := s.Queries.GetSessionUser(r.Context(), auth.HashSessionToken(cookie.Value))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		ctx := context.WithValue(r.Context(), userKey{}, session.User)
		if session.ActivityID != nil {
			ctx = context.WithValue(ctx, activitySessionKey{}, session.ActivityID)
		}
		next(w, r.WithContext(ctx))
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

// trackLogin starts the activity event of a sign-in, when the journal is on. The caller defers
// the returned function, which records it once the response is written.
func (s *server) trackLogin(w http.ResponseWriter, r *http.Request, track bool) (http.ResponseWriter, *store.RecordActivityParams, func()) {
	event := &store.RecordActivityParams{}
	if !track || !s.ActivityLog {
		return w, event, func() {}
	}
	event.Action = "login"
	rec := &responseRecorder{ResponseWriter: w}
	return rec, event, func() { s.recordActivity(r, rec, event) }
}

// startSession gives the user a session and its cookie, and notes it in the activity event.
// Every way of signing in ends here.
func (s *server) startSession(w http.ResponseWriter, r *http.Request, userID int64, event *store.RecordActivityParams) error {
	token, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(sessionLifetime)
	if event.Action != "" {
		id := rand.Text()
		event.SessionID = &id
	}
	err = s.Queries.CreateSession(r.Context(), store.CreateSessionParams{
		TokenHash: tokenHash, UserID: userID, ExpiresAt: expires, ActivityID: event.SessionID,
	})
	if err != nil {
		event.SessionID = nil
		return err
	}
	event.AccountID = &userID
	event.Outcome = "succeeded"
	s.setSessionCookie(w, token, expires)
	return nil
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	w, event, record := s.trackLogin(w, r, !s.maintenanceLogin(r))
	defer record()
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	// The limit is per login, whether or not the account exists, so it does not tell them apart.
	// It is not per address: behind the proxy the application does not see one.
	if ok, wait := s.logins.Allow(req.Login); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}

	user, err := s.Queries.GetUserByLogin(r.Context(), req.Login)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		internalError(w, r, err)
		return
	}
	if user.ID != 0 {
		event.AccountID = &user.ID
	}
	hash := user.PasswordHash
	if err != nil {
		hash = dummyHash()
	}
	if !auth.CheckPassword(hash, req.Password) || err != nil || user.Disabled {
		s.logins.Fail(req.Login)
		writeError(w, http.StatusUnauthorized, "wrong login or password")
		return
	}
	settings, err := s.Queries.GetSignInSettings(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	if settings.PasswordAdminsOnly && user.Role != auth.RoleAdmin {
		s.logins.Fail(req.Login)
		writeError(w, http.StatusUnauthorized, "wrong login or password")
		return
	}
	s.logins.Reset(req.Login)

	if user.TotpEnabled {
		if !s.TOTP.On() {
			// The check at start turns it off for secrets that cannot be read, so this is a setting
			// that changed while running; refuse rather than sign in without the second step.
			internalError(w, r, errors.New("two-factor sign-in is on for the user, but TOTP_KEY is missing"))
			return
		}
		// The sign-in is recorded by the code step.
		event.Action = ""
		writeJSON(w, http.StatusAccepted, struct {
			TwoFactor bool   `json:"twoFactor"`
			Token     string `json:"token"`
		}{true, s.TOTP.NewPending(user.ID, time.Now())})
		return
	}
	if err := s.startSession(w, r, user.ID, event); err != nil {
		internalError(w, r, err)
		return
	}
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
		GridSquares              *bool `json:"gridSquares"`
		TimelineIncludesShared   *bool `json:"timelineIncludesShared"`

		GridSize *string `json:"gridSize"`
		Language *string `json:"language"`
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
	if req.Language != nil && !slices.Contains(languages, *req.Language) {
		writeError(w, http.StatusBadRequest, "unknown language")
		return
	}
	if req.GridSize != nil && !slices.Contains(gridSizes, *req.GridSize) {
		writeError(w, http.StatusBadRequest, "unknown grid size")
		return
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
	if req.GridSquares != nil {
		var err error
		user, err = s.Queries.SetUserGridSquares(r.Context(), store.SetUserGridSquaresParams{
			ID:          user.ID,
			GridSquares: *req.GridSquares,
		})
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	if req.GridSize != nil {
		var err error
		user, err = s.Queries.SetUserGridSize(r.Context(), store.SetUserGridSizeParams{
			ID:       user.ID,
			GridSize: *req.GridSize,
		})
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	if req.TimelineIncludesShared != nil {
		var err error
		user, err = s.Queries.SetUserTimelineIncludesShared(r.Context(), store.SetUserTimelineIncludesSharedParams{
			ID:                     user.ID,
			TimelineIncludesShared: *req.TimelineIncludesShared,
		})
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	if req.Language != nil {
		var err error
		user, err = s.Queries.SetUserLanguage(r.Context(), store.SetUserLanguageParams{
			ID:       user.ID,
			Language: *req.Language,
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
	if user.PasswordHash == "" {
		// An account made by a provider has no current password. It proves who it is by having just
		// signed in again at the provider, which left the grant cookie.
		if !s.hasSetPasswordGrant(r, user.ID) {
			writeError(w, http.StatusForbidden, "confirm with your provider first")
			return
		}
	} else if !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
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
	// The grant is spent.
	http.SetCookie(w, &http.Cookie{Name: setPasswordCookie, Path: "/api/v1/me/password", MaxAge: -1, HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}
