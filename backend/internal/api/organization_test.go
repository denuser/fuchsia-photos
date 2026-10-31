package api

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

type page struct {
	Items      []photoJSON `json:"items"`
	NextCursor *string     `json:"nextCursor"`
}

func (b *browser) addPhoto(fixture string) int64 {
	b.e.t.Helper()
	res := b.upload(fixture)
	wantStatus(b.e.t, res, http.StatusCreated)
	return decode[uploadResult](b.e.t, res).Photo.ID
}

func (b *browser) photoIDs(path string) []int64 {
	b.e.t.Helper()
	res := b.get(path)
	wantStatus(b.e.t, res, http.StatusOK)
	items := decode[page](b.e.t, res).Items
	ids := make([]int64, len(items))
	for i, p := range items {
		ids[i] = p.ID
	}
	return ids
}

func wantIDs(t *testing.T, got []int64, want ...int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("ids = %v, want %v", got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("ids = %v, want %v", got, want)
			return
		}
	}
}

func ids(values ...int64) map[string][]int64 {
	return map[string][]int64{"ids": values}
}

func TestTrash(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	e.addUser("root", auth.RoleAdmin)
	anna, boris, root := e.loggedIn("anna"), e.loggedIn("boris"), e.loggedIn("root")

	kept := anna.addPhoto("landscape.jpg")
	trashed := anna.addPhoto("landscape.png")
	markReady(t, e, trashed)

	wantStatus(t, boris.json(http.MethodPost, "/api/v1/photos/trash", ids(trashed)), http.StatusNoContent)
	wantIDs(t, anna.photoIDs("/api/v1/photos"), trashed, kept) // Another user's request changed nothing.

	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(trashed)), http.StatusNoContent)
	wantIDs(t, anna.photoIDs("/api/v1/photos"), kept)
	wantIDs(t, anna.photoIDs("/api/v1/photos/trash"), trashed)
	wantIDs(t, boris.photoIDs("/api/v1/photos/trash"))

	t.Run("only the owner can see a photo in the trash", func(t *testing.T) {
		wantStatus(t, anna.get("/api/v1/photos/"+itoa(trashed)+"/previews/grid"), http.StatusOK)
		wantStatus(t, boris.get("/api/v1/photos/"+itoa(trashed)), http.StatusNotFound)
		wantStatus(t, root.get("/api/v1/photos/"+itoa(trashed)), http.StatusNotFound)
	})

	t.Run("other pages learn about it through events", func(t *testing.T) {
		changed, err := e.queries.ListChangedPhotos(context.Background(), store.ListChangedPhotosParams{
			OwnerID: 1, Since: time.Now().Add(-time.Hour), MaxRows: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, p := range changed {
			found = found || (p.ID == trashed && p.DeletedAt != nil)
		}
		if !found {
			t.Errorf("changed photos %+v do not report the trashed photo", changed)
		}
	})

	t.Run("restore", func(t *testing.T) {
		wantStatus(t, boris.json(http.MethodPost, "/api/v1/photos/restore", ids(trashed)), http.StatusNoContent)
		wantIDs(t, anna.photoIDs("/api/v1/photos/trash"), trashed)

		wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/restore", ids(trashed)), http.StatusNoContent)
		wantIDs(t, anna.photoIDs("/api/v1/photos"), trashed, kept)
		wantIDs(t, anna.photoIDs("/api/v1/photos/trash"))
	})

	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids()), http.StatusBadRequest)
	tooMany := make([]int64, maxBulkPhotos+1)
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(tooMany...)), http.StatusBadRequest)
}

func TestTrashPagination(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	all := []int64{anna.addPhoto("landscape.jpg"), anna.addPhoto("landscape.png"), anna.addPhoto("landscape.webp")}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(all...)), http.StatusNoContent)

	first := decode[page](t, anna.get("/api/v1/photos/trash?limit=2"))
	if len(first.Items) != 2 || first.NextCursor == nil {
		t.Fatalf("first page = %+v", first)
	}
	second := decode[page](t, anna.get("/api/v1/photos/trash?limit=2&cursor="+*first.NextCursor))
	if len(second.Items) != 1 || second.NextCursor != nil {
		t.Errorf("second page = %+v", second)
	}
}

func TestPurge(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")

	id := anna.addPhoto("landscape.jpg")
	markReady(t, e, id)
	photo, err := e.queries.GetPhoto(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	original := e.layout.Original(photo.OwnerID, photo.Sha256, photo.Mime)
	preview := e.layout.Preview(id, "grid", "webp")

	exists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}

	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/purge", ids(id)), http.StatusNoContent)
	if !exists(original) {
		t.Fatal("a photo outside the trash was purged")
	}

	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(id)), http.StatusNoContent)
	wantStatus(t, boris.json(http.MethodPost, "/api/v1/photos/purge", ids(id)), http.StatusNoContent)
	if !exists(original) {
		t.Fatal("another user purged the photo")
	}

	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/purge", ids(id)), http.StatusNoContent)
	if exists(original) || exists(preview) {
		t.Error("the files of a purged photo are still there")
	}
	wantStatus(t, anna.get("/api/v1/photos/"+itoa(id)), http.StatusNotFound)

	// The hash is free again, so the same file becomes a new photo.
	again := anna.addPhoto("landscape.jpg")
	if again == id {
		t.Errorf("a purged id came back: %d", again)
	}
	if !exists(original) {
		t.Error("the new upload has no original")
	}
}

func TestEmptyTrash(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")

	kept := anna.addPhoto("landscape.jpg")
	first, second := anna.addPhoto("landscape.png"), anna.addPhoto("landscape.webp")
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(first, second)), http.StatusNoContent)
	other := boris.addPhoto("landscape.png")
	wantStatus(t, boris.json(http.MethodPost, "/api/v1/photos/trash", ids(other)), http.StatusNoContent)

	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash/empty", nil), http.StatusNoContent)

	wantIDs(t, anna.photoIDs("/api/v1/photos/trash"))
	wantIDs(t, anna.photoIDs("/api/v1/photos"), kept)
	wantIDs(t, boris.photoIDs("/api/v1/photos/trash"), other)
}

func TestUploadRestoresFromTrash(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")

	id := anna.addPhoto("landscape.jpg")
	album := decode[albumJSON](t, anna.json(http.MethodPost, "/api/v1/albums", map[string]string{"name": "Trip"}))
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/albums/"+itoa(album.ID)+"/photos", ids(id)), http.StatusNoContent)
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(id)), http.StatusNoContent)

	type result struct {
		Photo     photoJSON `json:"photo"`
		Duplicate bool      `json:"duplicate"`
		Restored  bool      `json:"restored"`
	}
	res := anna.upload("landscape.jpg")
	wantStatus(t, res, http.StatusOK)
	first := decode[result](t, res)
	if first.Photo.ID != id || !first.Duplicate || !first.Restored || first.Photo.DeletedAt != nil {
		t.Errorf("upload = %+v", first)
	}
	wantIDs(t, anna.photoIDs("/api/v1/photos/trash"))
	wantIDs(t, anna.photoIDs("/api/v1/photos?album="+itoa(album.ID)), id)

	if again := decode[result](t, anna.upload("landscape.jpg")); again.Restored {
		t.Errorf("a duplicate outside the trash was reported as restored: %+v", again)
	}
}

func TestAlbums(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")

	older, newer := anna.addPhoto("landscape.jpg"), anna.addPhoto("landscape.png")
	markReady(t, e, older)
	markReady(t, e, newer)
	outside := anna.addPhoto("landscape.webp")
	markReady(t, e, outside)
	foreign := boris.addPhoto("large.jpg")

	wantStatus(t, anna.json(http.MethodPost, "/api/v1/albums", map[string]string{"name": "  "}), http.StatusBadRequest)
	res := anna.json(http.MethodPost, "/api/v1/albums", map[string]string{"name": " Trip "})
	wantStatus(t, res, http.StatusCreated)
	album := decode[albumJSON](t, res)
	if album.Name != "Trip" || album.PhotoCount != 0 || album.CoverPhotoID != nil {
		t.Errorf("new album = %+v", album)
	}
	path := "/api/v1/albums/" + itoa(album.ID)

	get := func() albumJSON {
		t.Helper()
		return decode[albumJSON](t, anna.get(path))
	}
	wantCover := func(id int64, chosen bool) {
		t.Helper()
		got := get()
		if got.CoverPhotoID == nil || *got.CoverPhotoID != id || got.CoverChosen != chosen {
			t.Errorf("album = %+v, want cover %d, chosen %v", got, id, chosen)
		}
	}
	setCover := func(id any) *http.Response {
		t.Helper()
		return anna.json(http.MethodPut, path+"/cover", map[string]any{"photoId": id})
	}

	t.Run("other users do not see it", func(t *testing.T) {
		wantStatus(t, boris.get(path), http.StatusNotFound)
		wantStatus(t, boris.get("/api/v1/photos?album="+itoa(album.ID)), http.StatusNotFound)
		wantStatus(t, boris.json(http.MethodPatch, path, map[string]string{"name": "Mine"}), http.StatusNotFound)
		wantStatus(t, boris.json(http.MethodDelete, path, nil), http.StatusNotFound)
		wantStatus(t, boris.json(http.MethodPost, path+"/photos", ids(foreign)), http.StatusNotFound)
		wantStatus(t, boris.json(http.MethodPost, path+"/photos/remove", ids(older)), http.StatusNotFound)
		wantStatus(t, boris.json(http.MethodPut, path+"/cover", map[string]any{"photoId": nil}), http.StatusNotFound)
		if albums := decode[[]albumJSON](t, boris.get("/api/v1/albums")); len(albums) != 0 {
			t.Errorf("albums = %+v", albums)
		}
	})

	t.Run("photos", func(t *testing.T) {
		wantStatus(t, anna.json(http.MethodPost, path+"/photos", ids(older, newer, older, foreign)), http.StatusNoContent)
		wantIDs(t, anna.photoIDs("/api/v1/photos?album="+itoa(album.ID)), newer, older) // The foreign photo is ignored.
		if got := get(); got.PhotoCount != 2 {
			t.Errorf("photo count = %d", got.PhotoCount)
		}
	})

	t.Run("cover", func(t *testing.T) {
		wantCover(newer, false)

		wantStatus(t, setCover(outside), http.StatusNotFound)
		wantStatus(t, setCover(foreign), http.StatusNotFound)
		wantStatus(t, setCover(older), http.StatusOK)
		wantCover(older, true)

		// A chosen cover in the trash gives way to the automatic one, and returns with the photo.
		wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(older)), http.StatusNoContent)
		wantCover(newer, false)
		if got := get(); got.PhotoCount != 1 {
			t.Errorf("photo count with a photo in the trash = %d", got.PhotoCount)
		}
		wantStatus(t, setCover(older), http.StatusNotFound)
		wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/restore", ids(older)), http.StatusNoContent)
		wantCover(older, true)

		wantStatus(t, setCover(nil), http.StatusOK)
		wantCover(newer, false)
	})

	t.Run("a purged cover is forgotten", func(t *testing.T) {
		wantStatus(t, setCover(older), http.StatusOK)
		wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(older)), http.StatusNoContent)
		wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/purge", ids(older)), http.StatusNoContent)
		wantCover(newer, false)
		wantIDs(t, anna.photoIDs("/api/v1/photos?album="+itoa(album.ID)), newer)
	})

	t.Run("remove and delete keep the photos", func(t *testing.T) {
		wantStatus(t, anna.json(http.MethodPatch, path, map[string]string{"name": "Summer"}), http.StatusOK)
		if got := get(); got.Name != "Summer" {
			t.Errorf("name = %q", got.Name)
		}

		wantStatus(t, anna.json(http.MethodPost, path+"/photos/remove", ids(newer)), http.StatusNoContent)
		wantIDs(t, anna.photoIDs("/api/v1/photos?album="+itoa(album.ID)))
		if got := get(); got.CoverPhotoID != nil {
			t.Errorf("an empty album has a cover: %+v", got)
		}

		wantStatus(t, anna.json(http.MethodPost, path+"/photos", ids(newer)), http.StatusNoContent)
		wantStatus(t, anna.json(http.MethodDelete, path, nil), http.StatusNoContent)
		wantStatus(t, anna.get(path), http.StatusNotFound)
		wantIDs(t, anna.photoIDs("/api/v1/photos"), outside, newer)
	})
}

func TestTags(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")

	first, second, third := anna.addPhoto("landscape.jpg"), anna.addPhoto("landscape.png"), anna.addPhoto("landscape.webp")
	foreign := boris.addPhoto("large.jpg")

	create := func(b *browser, name string, status int) int64 {
		t.Helper()
		res := b.json(http.MethodPost, "/api/v1/tags", map[string]string{"name": name})
		wantStatus(t, res, status)
		return decode[tagRefJSON](t, res).ID
	}
	sea := create(anna, "Sea", http.StatusCreated)
	if same := create(anna, "sea", http.StatusOK); same != sea {
		t.Errorf("creating a tag again gave id %d, want %d", same, sea)
	}
	city := create(anna, "City", http.StatusCreated)
	unused := create(anna, "Unused", http.StatusCreated)
	bTag := create(boris, "Sea", http.StatusCreated)
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/tags", map[string]string{"name": ""}), http.StatusBadRequest)

	change := func(b *browser, body map[string][]int64) {
		t.Helper()
		wantStatus(t, b.json(http.MethodPost, "/api/v1/photos/tags", body), http.StatusNoContent)
	}
	change(anna, map[string][]int64{"ids": {first, second, foreign}, "add": {sea, bTag}})
	change(anna, map[string][]int64{"ids": {second, third}, "add": {city}})

	t.Run("another user's photos and tags are out of reach", func(t *testing.T) {
		change(boris, map[string][]int64{"ids": {first}, "add": {bTag}, "remove": {sea}})
		wantIDs(t, boris.photoIDs("/api/v1/photos?tag="+itoa(bTag)))
		wantIDs(t, anna.photoIDs("/api/v1/photos?tag="+itoa(sea)), second, first)
		wantStatus(t, boris.get("/api/v1/photos/"+itoa(first)+"/tags"), http.StatusNotFound)
		wantStatus(t, boris.json(http.MethodPatch, "/api/v1/tags/"+itoa(sea), map[string]string{"name": "Mine"}), http.StatusNotFound)
		wantStatus(t, boris.json(http.MethodDelete, "/api/v1/tags/"+itoa(sea), nil), http.StatusNotFound)
	})

	t.Run("several tags match any of them", func(t *testing.T) {
		wantIDs(t, anna.photoIDs("/api/v1/photos?tag="+itoa(sea)), second, first)
		wantIDs(t, anna.photoIDs("/api/v1/photos?tag="+itoa(sea)+"&tag="+itoa(city)), third, second, first)
		wantIDs(t, anna.photoIDs("/api/v1/photos?tag="+itoa(unused)))
		wantStatus(t, anna.get("/api/v1/photos?tag=nonsense"), http.StatusBadRequest)
	})

	t.Run("listing", func(t *testing.T) {
		tags := decode[[]tagJSON](t, anna.get("/api/v1/tags"))
		want := []tagJSON{{city, "City", 2}, {sea, "Sea", 2}, {unused, "Unused", 0}}
		if len(tags) != len(want) {
			t.Fatalf("tags = %+v", tags)
		}
		for i := range want {
			if tags[i] != want[i] {
				t.Errorf("tags[%d] = %+v, want %+v", i, tags[i], want[i])
			}
		}
		onPhoto := decode[[]tagRefJSON](t, anna.get("/api/v1/photos/"+itoa(second)+"/tags"))
		if len(onPhoto) != 2 || onPhoto[0].ID != city || onPhoto[1].ID != sea {
			t.Errorf("tags of the photo = %+v", onPhoto)
		}
	})

	t.Run("photos in the trash are not counted", func(t *testing.T) {
		wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(third)), http.StatusNoContent)
		wantIDs(t, anna.photoIDs("/api/v1/photos?tag="+itoa(city)), second)
		tags := decode[[]tagJSON](t, anna.get("/api/v1/tags"))
		if tags[0].PhotoCount != 1 {
			t.Errorf("tags = %+v", tags)
		}
		wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/restore", ids(third)), http.StatusNoContent)
	})

	t.Run("remove, rename, delete", func(t *testing.T) {
		change(anna, map[string][]int64{"ids": {first}, "remove": {sea}})
		wantIDs(t, anna.photoIDs("/api/v1/photos?tag="+itoa(sea)), second)

		wantStatus(t, anna.json(http.MethodPatch, "/api/v1/tags/"+itoa(city), map[string]string{"name": "SEA"}), http.StatusConflict)
		wantStatus(t, anna.json(http.MethodPatch, "/api/v1/tags/"+itoa(city), map[string]string{"name": "Town"}), http.StatusOK)
		wantStatus(t, anna.json(http.MethodPatch, "/api/v1/tags/"+itoa(sea), map[string]string{"name": "SEA"}), http.StatusOK)

		wantStatus(t, anna.json(http.MethodDelete, "/api/v1/tags/"+itoa(city), nil), http.StatusNoContent)
		wantIDs(t, anna.photoIDs("/api/v1/photos?tag="+itoa(city)))
		wantIDs(t, anna.photoIDs("/api/v1/photos"), third, second, first)
	})
}

func TestDateFilter(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	id := anna.addPhoto("landscape.jpg")

	stamp := func(d time.Duration) string { return time.Now().UTC().Add(d).Format(time.RFC3339) }
	wantIDs(t, anna.photoIDs("/api/v1/photos?from="+stamp(-time.Hour)+"&to="+stamp(time.Hour)), id)
	wantIDs(t, anna.photoIDs("/api/v1/photos?from="+stamp(time.Hour)))
	wantIDs(t, anna.photoIDs("/api/v1/photos?to="+stamp(-time.Hour)))
	wantStatus(t, anna.get("/api/v1/photos?from=yesterday"), http.StatusBadRequest)
}
