package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/denuser/magellanica-photos/backend/internal/auth"
)

func archive(t *testing.T, b *browser, value bool, photos ...int64) {
	t.Helper()
	wantStatus(t, b.json(http.MethodPost, "/api/v1/photos/archive", map[string]any{"ids": photos, "archived": value}), http.StatusNoContent)
}

// The archive is each user's own: an archived photo leaves that user's
// timeline, its months and its facets, also when it is filtered by tag, and
// stays in albums, favorites and the Archive page. Others are not affected.
func TestArchive(t *testing.T) {
	e := setup(t)
	annaUser := e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")

	kept := anna.addPhoto("landscape.jpg")
	archived := anna.addPhoto("landscape.png")
	shared := boris.addPhoto("landscape.heic")
	for _, id := range []int64{kept, archived, shared} {
		markReady(t, e, id)
	}
	sharePhoto(t, boris, shared, map[string]any{"add": []int64{annaUser.ID}})
	album := anna.newAlbum(t, "Trip", kept, archived)
	res := anna.json(http.MethodPost, "/api/v1/tags", map[string]string{"name": "sea"})
	wantStatus(t, res, http.StatusCreated)
	tag := decode[tagRefJSON](t, res).ID
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/tags", map[string][]int64{"ids": {kept, archived}, "add": {tag}}), http.StatusNoContent)
	favorite(t, anna, true, archived)

	archive(t, anna, true, archived, shared)
	wantIDs(t, anna.photoIDs("/api/v1/photos"), kept)
	wantIDs(t, anna.photoIDs("/api/v1/photos?withShared=true"), kept)
	wantIDs(t, anna.photoIDs("/api/v1/photos?tag="+itoa(tag)), kept)
	if months := anna.months("/api/v1/photos/months?tz=UTC&withShared=true"); len(months) != 1 || months[0].Count != 1 {
		t.Errorf("months = %v", months)
	}
	facets := decode[struct {
		Kinds []nameCountJSON `json:"kinds"`
	}](t, anna.get("/api/v1/photos/cameras?withShared=true"))
	total := int64(0)
	for _, k := range facets.Kinds {
		total += k.Count
	}
	if total != 1 {
		t.Errorf("kinds = %+v, want one photo", facets.Kinds)
	}

	// It stays in albums, favorites and the Archive page.
	wantIDs(t, anna.photoIDs("/api/v1/photos?album="+itoa(album)), archived, kept)
	wantIDs(t, anna.photoIDs("/api/v1/photos?favorites=true"), archived)
	wantIDs(t, anna.photoIDs("/api/v1/photos?archived=true&withShared=true"), shared, archived)
	wantStatus(t, anna.get("/api/v1/photos?archived=true&album="+itoa(album)), http.StatusBadRequest)
	wantStatus(t, anna.get("/api/v1/photos?archived=yes"), http.StatusBadRequest)

	// Boris's own timeline keeps the photo anna archived.
	wantIDs(t, boris.photoIDs("/api/v1/photos"), shared)
	isArchived := func(b *browser, id int64) bool {
		t.Helper()
		got := decode[photoJSON](t, b.get("/api/v1/photos/"+itoa(id)))
		if got.Archived == nil {
			t.Fatalf("photo %d: no archived field", id)
		}
		return *got.Archived
	}
	if !isArchived(anna, shared) || isArchived(boris, shared) {
		t.Error("the archive is not anna's own")
	}

	archive(t, anna, false, shared)
	wantIDs(t, anna.photoIDs("/api/v1/photos?withShared=true"), shared, kept)

	// Purging takes the photo out of every archive.
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(archived)), http.StatusNoContent)
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash/empty", nil), http.StatusNoContent)
	var rows int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM photo_archive WHERE photo_id = $1`, archived).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("%d archive rows left on a purged photo, err = %v", rows, err)
	}
}
