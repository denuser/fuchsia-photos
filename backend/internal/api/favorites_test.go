package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/magellanica-app/magellanica-photos/backend/internal/auth"
)

func favorite(t *testing.T, b *browser, value bool, photos ...int64) {
	t.Helper()
	wantStatus(t, b.json(http.MethodPost, "/api/v1/photos/favorites", map[string]any{"ids": photos, "favorite": value}), http.StatusNoContent)
}

// A favorite is each user's own mark on a photo the user can see. The
// Favorites page lists the user's own and others' favorites; the viewer
// tells whether a photo is one; purging the photo takes the marks.
func TestFavorites(t *testing.T) {
	e := setup(t)
	annaUser := e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, borisBrowser := e.loggedIn("anna"), e.loggedIn("boris")

	own := anna.addPhoto("landscape.jpg")
	other := anna.addPhoto("landscape.png")
	shared := borisBrowser.addPhoto("landscape.heic")
	private := borisBrowser.addPhoto("landscape.webp")
	for _, id := range []int64{own, other, shared, private} {
		markReady(t, e, id)
	}
	sharePhoto(t, borisBrowser, shared, map[string]any{"add": []int64{annaUser.ID}})

	// A photo anna cannot see is not marked.
	favorite(t, anna, true, own, shared, private)
	wantIDs(t, anna.photoIDs("/api/v1/photos?favorites=true&withShared=true"), shared, own)
	wantIDs(t, anna.photoIDs("/api/v1/photos?favorites=true"), own)
	if months := anna.months("/api/v1/photos/months?tz=UTC&favorites=true&withShared=true"); len(months) != 1 || months[0].Count != 2 {
		t.Errorf("months = %v", months)
	}
	wantStatus(t, anna.get("/api/v1/photos?favorites=true&album=1"), http.StatusBadRequest)
	wantStatus(t, anna.get("/api/v1/photos?favorites=maybe"), http.StatusBadRequest)

	// The mark is anna's own.
	isFavorite := func(b *browser, id int64) bool {
		t.Helper()
		got := decode[photoJSON](t, b.get("/api/v1/photos/"+itoa(id)))
		if got.Favorite == nil {
			t.Fatalf("photo %d: no favorite field", id)
		}
		return *got.Favorite
	}
	if !isFavorite(anna, shared) || isFavorite(borisBrowser, shared) {
		t.Error("the mark is not anna's own")
	}
	wantIDs(t, borisBrowser.photoIDs("/api/v1/photos?favorites=true&withShared=true"))
	if items := decode[timeline](t, anna.get("/api/v1/photos")).Items; len(items) == 0 || items[0].Favorite != nil {
		t.Error("list items carry the mark")
	}

	favorite(t, anna, false, shared)
	wantIDs(t, anna.photoIDs("/api/v1/photos?favorites=true&withShared=true"), own)
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/favorites", map[string]any{"ids": []int64{own}}), http.StatusBadRequest)

	// Purging takes the marks of every user.
	favorite(t, borisBrowser, true, private)
	wantStatus(t, borisBrowser.json(http.MethodPost, "/api/v1/photos/trash", ids(private)), http.StatusNoContent)
	wantStatus(t, borisBrowser.json(http.MethodPost, "/api/v1/photos/trash/empty", nil), http.StatusNoContent)
	var marks int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM photo_favorites WHERE photo_id = $1`, private).Scan(&marks); err != nil || marks != 0 {
		t.Errorf("%d marks left on a purged photo, err = %v", marks, err)
	}
}
