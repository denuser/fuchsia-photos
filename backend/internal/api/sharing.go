package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const (
	visibilityPrivate = "private"
	visibilityAll     = "all"
)

type sharingJSON struct {
	Visibility string        `json:"visibility"`
	Users      []userRefJSON `json:"users"`
}

// sharingChange is a change to who can see photos or an album. Fields left out
// change nothing.
type sharingChange struct {
	Visibility *string `json:"visibility"`
	Add        []int64 `json:"add"`
	Remove     []int64 `json:"remove"`
}

func (c sharingChange) valid(w http.ResponseWriter) bool {
	if c.Visibility != nil && *c.Visibility != visibilityPrivate && *c.Visibility != visibilityAll {
		writeError(w, http.StatusBadRequest, "unknown visibility")
		return false
	}
	return true
}

func (s *server) listSharedPhotos(w http.ResponseWriter, r *http.Request) {
	maxRows, ok := pageSize(w, r)
	if !ok {
		return
	}
	afterSortAt, afterID, ok := pageCursor(w, r)
	if !ok {
		return
	}
	from, to, ok := timeRange(w, r)
	if !ok {
		return
	}
	user := currentUser(r)
	photos, err := s.Queries.ListSharedPhotos(r.Context(), store.ListSharedPhotosParams{
		ViewerID:      user.ID,
		IncludeAlbums: user.SharedIncludesAlbums,
		FromTime:      from,
		ToTime:        to,
		AfterSortAt:   afterSortAt,
		AfterID:       afterID,
		MaxRows:       maxRows,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.writePhotoPage(w, r, photos, maxRows)
}

func (s *server) countSharedMonths(w http.ResponseWriter, r *http.Request) {
	tz, ok := timeZoneParam(w, r)
	if !ok {
		return
	}
	user := currentUser(r)
	rows, err := s.Queries.CountSharedMonths(r.Context(), store.CountSharedMonthsParams{
		Tz:            tz,
		ViewerID:      user.ID,
		IncludeAlbums: user.SharedIncludesAlbums,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeMonths(w, rows, func(row store.CountSharedMonthsRow) monthJSON { return monthJSON(row) })
}

func (s *server) listAllPhotos(w http.ResponseWriter, r *http.Request) {
	maxRows, ok := pageSize(w, r)
	if !ok {
		return
	}
	afterSortAt, afterID, ok := pageCursor(w, r)
	if !ok {
		return
	}
	owner, ok := ownerParam(w, r)
	if !ok {
		return
	}
	from, to, ok := timeRange(w, r)
	if !ok {
		return
	}
	photos, err := s.Queries.ListAllPhotos(r.Context(), store.ListAllPhotosParams{
		OwnerID:     owner,
		FromTime:    from,
		ToTime:      to,
		AfterSortAt: afterSortAt,
		AfterID:     afterID,
		MaxRows:     maxRows,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.writePhotoPage(w, r, photos, maxRows)
}

func (s *server) countAllMonths(w http.ResponseWriter, r *http.Request) {
	tz, ok := timeZoneParam(w, r)
	if !ok {
		return
	}
	owner, ok := ownerParam(w, r)
	if !ok {
		return
	}
	rows, err := s.Queries.CountAllMonths(r.Context(), store.CountAllMonthsParams{Tz: tz, OwnerID: owner})
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeMonths(w, rows, func(row store.CountAllMonthsRow) monthJSON { return monthJSON(row) })
}

func ownerParam(w http.ResponseWriter, r *http.Request) (*int64, bool) {
	text := r.URL.Query().Get("owner")
	if text == "" {
		return nil, true
	}
	id, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid owner")
		return nil, false
	}
	return &id, true
}

// timeRange reads the from and to parameters, which months are loaded by.
func timeRange(w http.ResponseWriter, r *http.Request) (from, to *time.Time, ok bool) {
	if from, ok = timeParam(w, r, "from"); !ok {
		return nil, nil, false
	}
	if to, ok = timeParam(w, r, "to"); !ok {
		return nil, nil, false
	}
	return from, to, true
}

func (s *server) listSharedAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := s.Queries.ListSharedAlbums(r.Context(), currentUser(r).ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]albumJSON, len(albums))
	for i, a := range albums {
		out[i] = toAlbumJSON(store.ListAlbumsRow(a))
	}
	writeJSON(w, http.StatusOK, out)
}

// getPhotoSharing is limited to those who manage the photo: a recipient does
// not learn who else can see it.
func (s *server) getPhotoSharing(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	user := currentUser(r)
	visibility, err := s.Queries.GetManagedPhotoVisibility(r.Context(), store.GetManagedPhotoVisibilityParams{
		ID:           id,
		ActorID:      user.ID,
		ActorIsAdmin: isAdmin(user),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	users, err := s.Queries.ListPhotoShareUsers(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	refs := make([]userRefJSON, len(users))
	for i, u := range users {
		refs[i] = userRefJSON{ID: u.ID, Login: u.Login}
	}
	writeJSON(w, http.StatusOK, sharingJSON{Visibility: visibility, Users: refs})
}

func (s *server) changePhotoSharing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
		sharingChange
	}
	if !readJSON(w, r, &req) || !validBulkIDs(w, req.IDs) || !req.valid(w) {
		return
	}
	user := currentUser(r)
	actorIsAdmin := isAdmin(user)
	err := s.inTx(r.Context(), func(q *store.Queries) error {
		if req.Visibility != nil {
			err := q.SetPhotosVisibility(r.Context(), store.SetPhotosVisibilityParams{
				Visibility: *req.Visibility, Ids: req.IDs, ActorID: user.ID, ActorIsAdmin: actorIsAdmin,
			})
			if err != nil {
				return err
			}
		}
		if len(req.Remove) > 0 {
			err := q.UnsharePhotos(r.Context(), store.UnsharePhotosParams{
				Ids: req.IDs, ActorID: user.ID, ActorIsAdmin: actorIsAdmin, UserIds: req.Remove,
			})
			if err != nil {
				return err
			}
		}
		if len(req.Add) > 0 {
			return q.SharePhotos(r.Context(), store.SharePhotosParams{
				Ids: req.IDs, ActorID: user.ID, ActorIsAdmin: actorIsAdmin, UserIds: req.Add,
			})
		}
		return nil
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) getAlbumSharing(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	visibility, err := s.Queries.GetOwnAlbumVisibility(r.Context(), store.GetOwnAlbumVisibilityParams{ID: id, OwnerID: currentUser(r).ID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	users, err := s.Queries.ListAlbumShareUsers(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	refs := make([]userRefJSON, len(users))
	for i, u := range users {
		refs[i] = userRefJSON{ID: u.ID, Login: u.Login}
	}
	writeJSON(w, http.StatusOK, sharingJSON{Visibility: visibility, Users: refs})
}

func (s *server) changeAlbumSharing(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req sharingChange
	if !readJSON(w, r, &req) || !req.valid(w) {
		return
	}
	if _, ok := s.ownAlbum(w, r, id); !ok {
		return
	}
	owner := currentUser(r).ID
	err := s.inTx(r.Context(), func(q *store.Queries) error {
		if req.Visibility != nil {
			err := q.SetAlbumVisibility(r.Context(), store.SetAlbumVisibilityParams{ID: id, OwnerID: owner, Visibility: *req.Visibility})
			if err != nil {
				return err
			}
		}
		if len(req.Remove) > 0 {
			err := q.UnshareAlbum(r.Context(), store.UnshareAlbumParams{AlbumID: id, OwnerID: owner, UserIds: req.Remove})
			if err != nil {
				return err
			}
		}
		if len(req.Add) > 0 {
			return q.ShareAlbum(r.Context(), store.ShareAlbumParams{AlbumID: id, OwnerID: owner, UserIds: req.Add})
		}
		return nil
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// inTx applies all of a sharing change or none of it.
func (s *server) inTx(ctx context.Context, change func(*store.Queries) error) error {
	tx, err := s.Ingest.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := change(store.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
