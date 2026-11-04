package api

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const pgUniqueViolation = "23505"

type userJSON struct {
	ID        int64     `json:"id"`
	Login     string    `json:"login"`
	Role      string    `json:"role"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"createdAt"`
	// SharedIncludesAlbums adds photos of shared albums to the user's list of shared photos.
	SharedIncludesAlbums bool `json:"sharedIncludesAlbums"`
	// ViewerShowsOriginals makes the viewer show original files instead of viewer previews.
	ViewerShowsOriginals bool `json:"viewerShowsOriginals"`
	// ViewerHiddenFields are the photo details the viewer leaves out.
	ViewerHiddenFields []string `json:"viewerHiddenFields"`
	// NavigationHiddenOnPhones hides the navigation drawer on a phone until the menu button opens it.
	NavigationHiddenOnPhones bool `json:"navigationHiddenOnPhones"`
}

type userRefJSON struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

func toUserJSON(u store.User) userJSON {
	hidden := u.ViewerHiddenFields
	if hidden == nil {
		hidden = []string{}
	}
	return userJSON{
		ID:                   u.ID,
		Login:                u.Login,
		Role:                 u.Role,
		Disabled:             u.Disabled,
		CreatedAt:            u.CreatedAt,
		SharedIncludesAlbums: u.SharedIncludesAlbums,
		ViewerShowsOriginals: u.ViewerShowsOriginals,
		ViewerHiddenFields:   hidden,

		NavigationHiddenOnPhones: u.NavigationHiddenOnPhones,
	}
}

// viewerFields are the photo details the viewer can show, in the order it shows them.
var viewerFields = []string{"camera", "lens", "focalLength", "aperture", "exposureTime", "iso", "exposureCompensation", "flash", "location"}

// normalizeViewerFields checks a list of viewer fields and puts it in the
// viewer's order without repeats.
func normalizeViewerFields(fields []string) ([]string, error) {
	chosen := make(map[string]bool, len(fields))
	for _, f := range fields {
		if !slices.Contains(viewerFields, f) {
			return nil, fmt.Errorf("unknown viewer field %q", f)
		}
		chosen[f] = true
	}
	out := []string{}
	for _, f := range viewerFields {
		if chosen[f] {
			out = append(out, f)
		}
	}
	return out, nil
}

func isAdmin(u store.User) bool {
	return u.Role == auth.RoleAdmin
}

func validRole(role string) bool {
	return role == auth.RoleAdmin || role == auth.RoleUser
}

func (s *server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.Queries.ListUsers(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]userJSON, len(users))
	for i, u := range users {
		out[i] = toUserJSON(u)
	}
	writeJSON(w, http.StatusOK, out)
}

// listUserDirectory gives every user the logins to choose from when sharing.
func (s *server) listUserDirectory(w http.ResponseWriter, r *http.Request) {
	users, err := s.Queries.ListActiveUserLogins(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]userRefJSON, len(users))
	for i, u := range users {
		out[i] = userRefJSON{ID: u.ID, Login: u.Login}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := auth.ValidateLogin(req.Login); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validRole(req.Role) {
		writeError(w, http.StatusBadRequest, "unknown role")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		internalError(w, r, err)
		return
	}

	user, err := s.Queries.CreateUser(r.Context(), store.CreateUserParams{Login: req.Login, PasswordHash: hash, Role: req.Role})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		writeError(w, http.StatusConflict, "this login is taken")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toUserJSON(user))
}

func (s *server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
		Password *string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	// An administrator cannot lock themselves out, so there is always at least one.
	if id == currentUser(r).ID && (req.Role != nil || req.Disabled != nil) {
		writeError(w, http.StatusConflict, "you cannot change your own role or disable yourself")
		return
	}
	if req.Role != nil && !validRole(*req.Role) {
		writeError(w, http.StatusBadRequest, "unknown role")
		return
	}
	var hash string
	if req.Password != nil {
		if err := auth.ValidatePassword(*req.Password); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var err error
		if hash, err = auth.HashPassword(*req.Password); err != nil {
			internalError(w, r, err)
			return
		}
	}

	ctx := r.Context()
	if _, err := s.Queries.GetUser(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		internalError(w, r, err)
		return
	}

	if req.Role != nil {
		if err := s.Queries.SetUserRole(ctx, store.SetUserRoleParams{ID: id, Role: *req.Role}); err != nil {
			internalError(w, r, err)
			return
		}
	}
	if req.Disabled != nil {
		if err := s.Queries.SetUserDisabled(ctx, store.SetUserDisabledParams{ID: id, Disabled: *req.Disabled}); err != nil {
			internalError(w, r, err)
			return
		}
	}
	if req.Password != nil {
		if err := s.Queries.SetUserPassword(ctx, store.SetUserPasswordParams{ID: id, PasswordHash: hash}); err != nil {
			internalError(w, r, err)
			return
		}
	}
	if req.Password != nil || (req.Disabled != nil && *req.Disabled) {
		if err := s.Queries.DeleteUserSessions(ctx, id); err != nil {
			internalError(w, r, err)
			return
		}
	}

	user, err := s.Queries.GetUser(ctx, id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserJSON(user))
}
