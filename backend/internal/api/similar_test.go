package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/magellanica-app/magellanica-photos/backend/internal/similar"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

type similarJSON struct {
	Groups []struct {
		Photos []photoJSON `json:"photos"`
	} `json:"groups"`
	Total int `json:"total"`
}

func (b *browser) similarGroups(tier string) similarJSON {
	b.e.t.Helper()
	res := b.get("/api/v1/similar?tier=" + tier)
	wantStatus(b.e.t, res, http.StatusOK)
	return decode[similarJSON](b.e.t, res)
}

// groupIDs lists each group's photo ids, in the order the answer gives.
func groupIDs(s similarJSON) [][]int64 {
	out := [][]int64{}
	for _, g := range s.Groups {
		var ids []int64
		for _, p := range g.Photos {
			ids = append(ids, p.ID)
		}
		out = append(out, ids)
	}
	return out
}

func pair(t *testing.T, e *env, a, b int64, tier string) {
	t.Helper()
	err := e.queries.UpsertSimilarPair(context.Background(), store.UpsertSimilarPairParams{PhotoID: a, OtherID: b, Tier: tier, Distance: 0.01})
	if err != nil {
		t.Fatal(err)
	}
}

func setSize(t *testing.T, e *env, id int64, width, height int, size int64) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), `UPDATE photos SET width = $2, height = $3, size = $4 WHERE id = $1`, id, width, height, size); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, e *env, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSimilarGroups(t *testing.T) {
	s := setupSharing(t)
	a := s.anna.addPhoto("landscape.jpg")
	b := s.anna.addPhoto("landscape.png")
	c := s.anna.addPhoto("landscape.webp")
	d := s.anna.addPhoto("rotated.jpg")
	f := s.anna.addPhoto("large.jpg")
	for _, id := range []int64{a, b, c, d, f} {
		markReady(t, s.e, id)
	}
	// The photo to keep comes first: the largest picture, then the largest file.
	setSize(t, s.e, a, 800, 600, 100)
	setSize(t, s.e, b, 2048, 1536, 50)
	setSize(t, s.e, c, 800, 600, 300)
	setSize(t, s.e, d, 600, 800, 10)
	setSize(t, s.e, f, 600, 800, 10)

	// A near B and B near C put all three in one group; D and F are a pair of their own.
	pair(t, s.e, a, b, similar.TierDuplicate)
	pair(t, s.e, c, b, similar.TierDuplicate)
	pair(t, s.e, d, f, similar.TierSimilar)
	// Someone else's photos make a group that is theirs only.
	v1 := s.vera.addPhoto("huawei.jpg")
	v2 := s.vera.addPhoto("microvideo.jpg")
	markReady(t, s.e, v1)
	markReady(t, s.e, v2)
	pair(t, s.e, v1, v2, similar.TierDuplicate)

	t.Run("a group is the photos joined by pairs, the one to keep first", func(t *testing.T) {
		got := s.anna.similarGroups(similar.TierDuplicate)
		ids := groupIDs(got)
		if got.Total != 1 || len(ids) != 1 || len(ids[0]) != 3 || ids[0][0] != b || ids[0][1] != c || ids[0][2] != a {
			t.Errorf("duplicates %v (total %d), want one group %v", ids, got.Total, []int64{b, c, a})
		}
	})
	t.Run("the tiers are separate", func(t *testing.T) {
		ids := groupIDs(s.anna.similarGroups(similar.TierSimilar))
		if len(ids) != 1 || len(ids[0]) != 2 {
			t.Errorf("similar %v, want the pair of %d and %d", ids, d, f)
		}
	})
	t.Run("a user sees only their own", func(t *testing.T) {
		ids := groupIDs(s.vera.similarGroups(similar.TierDuplicate))
		if len(ids) != 1 || len(ids[0]) != 2 {
			t.Errorf("vera's groups %v, want her one pair", ids)
		}
		for _, id := range ids[0] {
			if id == a || id == b || id == c {
				t.Errorf("vera sees anna's photo %d", id)
			}
		}
		if got := groupIDs(s.boris.similarGroups(similar.TierDuplicate)); len(got) != 0 {
			t.Errorf("boris sees %v, want nothing", got)
		}
	})
	t.Run("a photo in the trash leaves its groups", func(t *testing.T) {
		wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/photos/trash", ids(c)), http.StatusNoContent)
		got := groupIDs(s.anna.similarGroups(similar.TierDuplicate))
		if len(got) != 1 || len(got[0]) != 2 {
			t.Errorf("duplicates %v after trashing %d, want a group of the other two", got, c)
		}
		wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/photos/restore", ids(c)), http.StatusNoContent)
	})
	t.Run("the tier is checked", func(t *testing.T) {
		wantStatus(t, s.anna.get("/api/v1/similar"), http.StatusBadRequest)
		wantStatus(t, s.anna.get("/api/v1/similar?tier=other"), http.StatusBadRequest)
		wantStatus(t, s.e.browser().get("/api/v1/similar?tier=duplicate"), http.StatusUnauthorized)
	})
}

func TestDismissSimilar(t *testing.T) {
	s := setupSharing(t)
	a := s.anna.addPhoto("landscape.jpg")
	b := s.anna.addPhoto("landscape.png")
	c := s.anna.addPhoto("landscape.webp")
	for _, id := range []int64{a, b, c} {
		markReady(t, s.e, id)
	}
	pair(t, s.e, a, b, similar.TierDuplicate)
	pair(t, s.e, b, c, similar.TierDuplicate)

	// Another user cannot close pairs of photos that are not theirs.
	wantStatus(t, s.vera.json(http.MethodPost, "/api/v1/similar/dismiss", ids(a, b, c)), http.StatusNoContent)
	if got := groupIDs(s.anna.similarGroups(similar.TierDuplicate)); len(got) != 1 || len(got[0]) != 3 {
		t.Fatalf("groups %v after another user's dismissal, want the group intact", got)
	}
	wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/similar/dismiss", ids(a)), http.StatusBadRequest)

	// Closing the pair of A and B leaves B and C together.
	wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/similar/dismiss", ids(a, b)), http.StatusNoContent)
	got := groupIDs(s.anna.similarGroups(similar.TierDuplicate))
	if len(got) != 1 || len(got[0]) != 2 {
		t.Errorf("groups %v, want B and C", got)
	}
	wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/similar/dismiss", ids(a, b, c)), http.StatusNoContent)
	if got := groupIDs(s.anna.similarGroups(similar.TierDuplicate)); len(got) != 0 {
		t.Errorf("groups %v after closing every pair, want none", got)
	}
}

func TestResolveSimilar(t *testing.T) {
	s := setupSharing(t)
	keep := s.anna.addPhoto("landscape.jpg")
	dropA := s.anna.addPhoto("landscape.png")
	dropB := s.anna.addPhoto("landscape.webp")
	foreign := s.vera.addPhoto("rotated.jpg")
	for _, id := range []int64{keep, dropA, dropB, foreign} {
		markReady(t, s.e, id)
	}
	pair(t, s.e, keep, dropA, similar.TierDuplicate)
	pair(t, s.e, keep, dropB, similar.TierDuplicate)

	album := s.anna.newAlbum(t, "Trip", dropA)
	wantStatus(t, s.anna.json(http.MethodPut, "/api/v1/albums/"+itoa(album)+"/cover", map[string]any{"photoId": dropA}), http.StatusOK)
	tag := newTag(t, s.anna, "Sea")
	tagPhotos(t, s.anna, tag, dropB)
	favorite(t, s.anna, true, dropA)
	sharePhoto(t, s.anna, dropB, map[string]any{"add": []int64{s.borisID}})
	sharePhoto(t, s.anna, dropA, map[string]any{"visibility": "all"})
	if _, err := s.e.pool.Exec(context.Background(), `UPDATE photos SET description = 'Sunset' WHERE id = $1`, dropB); err != nil {
		t.Fatal(err)
	}

	resolve := func(b *browser, body map[string]any, want int) {
		t.Helper()
		wantStatus(t, b.json(http.MethodPost, "/api/v1/similar/resolve", body), want)
	}
	// Nothing happens to photos that are not the user's own, or if the same photo is on both sides.
	resolve(s.anna, map[string]any{"keepId": keep, "removeIds": []int64{foreign}}, http.StatusNotFound)
	resolve(s.vera, map[string]any{"keepId": foreign, "removeIds": []int64{keep}}, http.StatusNotFound)
	resolve(s.anna, map[string]any{"keepId": keep, "removeIds": []int64{keep, dropA}}, http.StatusBadRequest)
	resolve(s.anna, map[string]any{"keepId": keep, "removeIds": []int64{}}, http.StatusBadRequest)
	resolve(s.anna, map[string]any{"keepId": keep + 1000, "removeIds": []int64{dropA}}, http.StatusNotFound)
	if n := count(t, s.e, `SELECT count(*) FROM photos WHERE deleted_at IS NOT NULL`); n != 0 {
		t.Fatalf("%d photos in the trash after refused requests, want none", n)
	}

	resolve(s.anna, map[string]any{"keepId": keep, "removeIds": []int64{dropA, dropB}}, http.StatusNoContent)

	if got := s.anna.photoIDs("/api/v1/photos"); len(got) != 1 || got[0] != keep {
		t.Errorf("timeline %v, want only %d", got, keep)
	}
	wantIDs(t, s.anna.photoIDs("/api/v1/photos/trash"), dropB, dropA)
	if got := s.anna.photoIDs("/api/v1/photos?album=" + itoa(album)); len(got) != 1 || got[0] != keep {
		t.Errorf("album %v, want the kept photo", got)
	}
	if n := count(t, s.e, `SELECT count(*) FROM albums WHERE id = $1 AND cover_photo_id = $2`, album, keep); n != 1 {
		t.Error("the cover of the album did not move to the kept photo")
	}
	if n := count(t, s.e, `SELECT count(*) FROM photo_tags WHERE photo_id = $1 AND tag_id = $2`, keep, tag); n != 1 {
		t.Error("the tag did not move to the kept photo")
	}
	if n := count(t, s.e, `SELECT count(*) FROM photo_favorites WHERE photo_id = $1`, keep); n != 1 {
		t.Error("the favorite mark did not move to the kept photo")
	}
	if n := count(t, s.e, `SELECT count(*) FROM photo_shares WHERE photo_id = $1 AND user_id = $2`, keep, s.borisID); n != 1 {
		t.Error("the share did not move to the kept photo")
	}
	got := decode[photoJSON](t, s.anna.get("/api/v1/photos/"+itoa(keep)))
	if got.Description != "Sunset" {
		t.Errorf("description %q, want the one of a removed photo", got.Description)
	}
	if n := count(t, s.e, `SELECT count(*) FROM photos WHERE id = $1 AND visibility = 'all'`, keep); n != 1 {
		t.Error("the choice to show the photo to everyone did not move to the kept photo")
	}
	// The photos that went away can be restored, and are still the user's own.
	wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/photos/restore", ids(dropA)), http.StatusNoContent)
	if got := s.anna.photoIDs("/api/v1/photos"); len(got) != 2 {
		t.Errorf("timeline %v after restoring one photo, want two", got)
	}
}
