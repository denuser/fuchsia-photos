package api

import (
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/magellanica-app/magellanica-photos/backend/internal/similar"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

// maxSimilarGroups is the most groups one answer holds, the newest first.
const maxSimilarGroups = 200

type similarGroupJSON struct {
	// Photos are the photos of the group, the one to keep first: the largest
	// picture, then the largest file.
	Photos []photoJSON `json:"photos"`
}

// listSimilar answers with the groups of one tier among the user's own photos:
// the photos that are linked by an open pair, taken together. A pair that
// the user marked as not the same is not in it.
func (s *server) listSimilar(w http.ResponseWriter, r *http.Request) {
	tier := r.URL.Query().Get("tier")
	if tier != similar.TierDuplicate && tier != similar.TierSimilar {
		writeError(w, http.StatusBadRequest, "tier must be duplicate or similar")
		return
	}
	owner := currentUser(r).ID
	pairs, err := s.Queries.ListSimilarPairs(r.Context(), store.ListSimilarPairsParams{OwnerID: owner, Tier: tier})
	if err != nil {
		internalError(w, r, err)
		return
	}

	// Photos joined by pairs form one group, as A near B and B near C put all three together.
	parent := map[int64]int64{}
	var find func(int64) int64
	find = func(id int64) int64 {
		if parent[id] == id {
			return id
		}
		parent[id] = find(parent[id])
		return parent[id]
	}
	for _, p := range pairs {
		for _, id := range []int64{p.PhotoA, p.PhotoB} {
			if _, ok := parent[id]; !ok {
				parent[id] = id
			}
		}
		parent[find(p.PhotoA)] = find(p.PhotoB)
	}
	members := map[int64][]int64{}
	var ids []int64
	for id := range parent {
		root := find(id)
		members[root] = append(members[root], id)
		ids = append(ids, id)
	}

	out := []similarGroupJSON{}
	total := len(members)
	if len(ids) > 0 {
		photos, err := s.Queries.ListLibraryPhotosByIDs(r.Context(), store.ListLibraryPhotosByIDsParams{Ids: ids, OwnerID: owner})
		if err != nil {
			internalError(w, r, err)
			return
		}
		byID := make(map[int64]store.Photo, len(photos))
		for _, p := range photos {
			byID[p.ID] = p
		}
		var groups [][]store.Photo
		for _, ids := range members {
			var group []store.Photo
			for _, id := range ids {
				if p, ok := byID[id]; ok {
					group = append(group, p)
				}
			}
			if len(group) < 2 {
				continue
			}
			slices.SortFunc(group, func(a, b store.Photo) int {
				if c := pixels(b) - pixels(a); c != 0 {
					return sign(c)
				}
				if b.Size != a.Size {
					return sign(b.Size - a.Size)
				}
				return sign(a.ID - b.ID)
			})
			groups = append(groups, group)
		}
		// The group with the newest photo first.
		newest := func(g []store.Photo) int64 {
			var latest int64
			for _, p := range g {
				latest = max(latest, p.SortAt.UnixNano())
			}
			return latest
		}
		slices.SortFunc(groups, func(a, b []store.Photo) int {
			if c := newest(b) - newest(a); c != 0 {
				return sign(c)
			}
			return sign(a[0].ID - b[0].ID)
		})
		total = len(groups)
		if len(groups) > maxSimilarGroups {
			groups = groups[:maxSimilarGroups]
		}
		showLocation, err := s.showLocation(r.Context())
		if err != nil {
			internalError(w, r, err)
			return
		}
		for _, g := range groups {
			out = append(out, similarGroupJSON{Photos: toPhotosJSON(g, showLocation)})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": out, "total": total})
}

func pixels(p store.Photo) int64 {
	if p.Width == nil || p.Height == nil {
		return 0
	}
	return int64(*p.Width) * int64(*p.Height)
}

func sign(n int64) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

// dismissSimilar marks the photos as not the same: every open pair among them
// is closed and stays closed, whatever the vectors say later.
func (s *server) dismissSimilar(w http.ResponseWriter, r *http.Request) {
	ids, ok := readIDs(w, r)
	if !ok {
		return
	}
	if len(ids) < 2 {
		writeError(w, http.StatusBadRequest, "at least two photos")
		return
	}
	if _, err := s.Queries.DismissSimilarPairs(r.Context(), store.DismissSimilarPairsParams{OwnerID: currentUser(r).ID, Ids: ids}); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolveSimilar keeps one photo of a group and sends the others to the trash,
// after their albums, tags, favorites and shares have moved to the kept one.
func (s *server) resolveSimilar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeepID    int64   `json:"keepId"`
		RemoveIDs []int64 `json:"removeIds"`
	}
	if !readJSON(w, r, &req) || !validBulkIDs(w, req.RemoveIDs) {
		return
	}
	if slices.Contains(req.RemoveIDs, req.KeepID) {
		writeError(w, http.StatusBadRequest, "the photo to keep is also to be removed")
		return
	}
	user := currentUser(r)
	ctx := r.Context()
	all := append([]int64{req.KeepID}, req.RemoveIDs...)
	found, err := s.Queries.ListLibraryPhotosByIDs(ctx, store.ListLibraryPhotosByIDsParams{Ids: all, OwnerID: user.ID})
	if err != nil {
		internalError(w, r, err)
		return
	}
	unique := map[int64]bool{}
	for _, id := range all {
		unique[id] = true
	}
	if len(found) != len(unique) {
		writeError(w, http.StatusNotFound, "photo not found")
		return
	}
	err = pgx.BeginFunc(ctx, s.Ingest.Pool, func(tx pgx.Tx) error {
		queries := store.New(tx)
		if err := queries.LockPhotoLinks(ctx); err != nil {
			return err
		}
		if err := queries.MovePhotoMemberships(ctx, store.MovePhotoMembershipsParams{KeepID: req.KeepID, RemoveIds: req.RemoveIDs}); err != nil {
			return err
		}
		return queries.TrashPhotos(ctx, store.TrashPhotosParams{Ids: req.RemoveIDs, ActorID: user.ID, ActorIsAdmin: false})
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
