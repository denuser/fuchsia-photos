package api

import (
	"net/http"
	"testing"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
)

type sharingEnv struct {
	e                 *env
	anna, boris, vera *browser
	root              *browser
	borisID, veraID   int64
}

func setupSharing(t *testing.T) sharingEnv {
	t.Helper()
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	borisID := e.addUser("boris", auth.RoleUser).ID
	veraID := e.addUser("vera", auth.RoleUser).ID
	e.addUser("root", auth.RoleAdmin)
	return sharingEnv{
		e: e, anna: e.loggedIn("anna"), boris: e.loggedIn("boris"), vera: e.loggedIn("vera"), root: e.loggedIn("root"),
		borisID: borisID, veraID: veraID,
	}
}

func (b *browser) shareFiles(t *testing.T, photo int64, want int) {
	t.Helper()
	for _, path := range []string{"", "/previews/grid", "/previews/viewer", "/original"} {
		res := b.get("/api/v1/photos/" + itoa(photo) + path)
		if res.StatusCode != want {
			t.Errorf("GET photo %d%s = %d, want %d", photo, path, res.StatusCode, want)
		}
		res.Body.Close()
	}
}

func (b *browser) newAlbum(t *testing.T, name string, photos ...int64) int64 {
	t.Helper()
	res := b.json(http.MethodPost, "/api/v1/albums", map[string]string{"name": name})
	wantStatus(t, res, http.StatusCreated)
	id := decode[albumJSON](t, res).ID
	if len(photos) > 0 {
		wantStatus(t, b.json(http.MethodPost, "/api/v1/albums/"+itoa(id)+"/photos", ids(photos...)), http.StatusNoContent)
	}
	return id
}

func sharePhoto(t *testing.T, b *browser, photo int64, change map[string]any) {
	t.Helper()
	change["ids"] = []int64{photo}
	wantStatus(t, b.json(http.MethodPost, "/api/v1/photos/sharing", change), http.StatusNoContent)
}

func shareAlbum(t *testing.T, b *browser, album int64, change map[string]any) {
	t.Helper()
	wantStatus(t, b.json(http.MethodPost, "/api/v1/albums/"+itoa(album)+"/sharing", change), http.StatusNoContent)
}

// The permission matrix: each way of granting access, for each kind of viewer.
func TestPermissionMatrix(t *testing.T) {
	cases := []struct {
		name              string
		grant             func(s sharingEnv, photo int64)
		boris, vera, root int // Expected status of the photo and its files.
	}{
		{
			name:  "private",
			grant: func(sharingEnv, int64) {},
			boris: http.StatusNotFound, vera: http.StatusNotFound, root: http.StatusOK,
		},
		{
			name: "photo visible to all",
			grant: func(s sharingEnv, photo int64) {
				sharePhoto(s.e.t, s.anna, photo, map[string]any{"visibility": "all"})
			},
			boris: http.StatusOK, vera: http.StatusOK, root: http.StatusOK,
		},
		{
			name: "photo shared by login",
			grant: func(s sharingEnv, photo int64) {
				sharePhoto(s.e.t, s.anna, photo, map[string]any{"add": []int64{s.borisID}})
			},
			boris: http.StatusOK, vera: http.StatusNotFound, root: http.StatusOK,
		},
		{
			name: "album visible to all",
			grant: func(s sharingEnv, photo int64) {
				shareAlbum(s.e.t, s.anna, s.anna.newAlbum(s.e.t, "Trip", photo), map[string]any{"visibility": "all"})
			},
			boris: http.StatusOK, vera: http.StatusOK, root: http.StatusOK,
		},
		{
			name: "album shared by login",
			grant: func(s sharingEnv, photo int64) {
				shareAlbum(s.e.t, s.anna, s.anna.newAlbum(s.e.t, "Trip", photo), map[string]any{"add": []int64{s.borisID}})
			},
			boris: http.StatusOK, vera: http.StatusNotFound, root: http.StatusOK,
		},
		{
			name: "private album holding the photo",
			grant: func(s sharingEnv, photo int64) {
				s.anna.newAlbum(s.e.t, "Trip", photo)
			},
			boris: http.StatusNotFound, vera: http.StatusNotFound, root: http.StatusOK,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := setupSharing(t)
			photo := s.anna.addPhoto("landscape.jpg")
			markReady(t, s.e, photo)

			c.grant(s, photo)

			s.anna.shareFiles(t, photo, http.StatusOK)
			s.boris.shareFiles(t, photo, c.boris)
			s.vera.shareFiles(t, photo, c.vera)
			s.root.shareFiles(t, photo, c.root)
		})
	}
}

func TestRevokingSharing(t *testing.T) {
	s := setupSharing(t)
	photo := s.anna.addPhoto("landscape.jpg")
	markReady(t, s.e, photo)

	sharePhoto(t, s.anna, photo, map[string]any{"add": []int64{s.borisID}})
	s.boris.shareFiles(t, photo, http.StatusOK)
	sharePhoto(t, s.anna, photo, map[string]any{"remove": []int64{s.borisID}})
	s.boris.shareFiles(t, photo, http.StatusNotFound)

	sharePhoto(t, s.anna, photo, map[string]any{"visibility": "all"})
	s.boris.shareFiles(t, photo, http.StatusOK)
	sharePhoto(t, s.anna, photo, map[string]any{"visibility": "private"})
	s.boris.shareFiles(t, photo, http.StatusNotFound)

	album := s.anna.newAlbum(t, "Trip", photo)
	shareAlbum(t, s.anna, album, map[string]any{"add": []int64{s.borisID}})
	s.boris.shareFiles(t, photo, http.StatusOK)

	t.Run("leaving the album ends access through it", func(t *testing.T) {
		wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/albums/"+itoa(album)+"/photos/remove", ids(photo)), http.StatusNoContent)
		s.boris.shareFiles(t, photo, http.StatusNotFound)
		wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/albums/"+itoa(album)+"/photos", ids(photo)), http.StatusNoContent)
		s.boris.shareFiles(t, photo, http.StatusOK)
	})

	t.Run("deleting the album ends access through it", func(t *testing.T) {
		wantStatus(t, s.anna.json(http.MethodDelete, "/api/v1/albums/"+itoa(album), nil), http.StatusNoContent)
		s.boris.shareFiles(t, photo, http.StatusNotFound)
	})
}

func TestSharedPhotoInTrash(t *testing.T) {
	s := setupSharing(t)
	photo := s.anna.addPhoto("landscape.jpg")
	markReady(t, s.e, photo)
	sharePhoto(t, s.anna, photo, map[string]any{"add": []int64{s.borisID}})

	wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/photos/trash", ids(photo)), http.StatusNoContent)
	s.boris.shareFiles(t, photo, http.StatusNotFound)
	wantIDs(t, s.boris.photoIDs("/api/v1/photos/shared"))

	wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/photos/restore", ids(photo)), http.StatusNoContent)
	s.boris.shareFiles(t, photo, http.StatusOK)
	wantIDs(t, s.boris.photoIDs("/api/v1/photos/shared"), photo)
}

func TestOnlyManagersChangeSharing(t *testing.T) {
	s := setupSharing(t)
	photo := s.anna.addPhoto("landscape.jpg")
	markReady(t, s.e, photo)
	album := s.anna.newAlbum(t, "Trip", photo)
	sharePhoto(t, s.anna, photo, map[string]any{"add": []int64{s.borisID}})
	shareAlbum(t, s.anna, album, map[string]any{"add": []int64{s.borisID}})

	t.Run("a recipient cannot pass a photo on", func(t *testing.T) {
		sharePhoto(t, s.boris, photo, map[string]any{"add": []int64{s.veraID}, "visibility": "all"})
		s.vera.shareFiles(t, photo, http.StatusNotFound)
	})

	t.Run("a recipient cannot pass an album on", func(t *testing.T) {
		res := s.boris.json(http.MethodPost, "/api/v1/albums/"+itoa(album)+"/sharing", map[string]any{"add": []int64{s.veraID}})
		wantStatus(t, res, http.StatusNotFound)
		s.vera.shareFiles(t, photo, http.StatusNotFound)
	})

	t.Run("a recipient does not see who else has access", func(t *testing.T) {
		wantStatus(t, s.boris.get("/api/v1/photos/"+itoa(photo)+"/sharing"), http.StatusNotFound)
		wantStatus(t, s.boris.get("/api/v1/albums/"+itoa(album)+"/sharing"), http.StatusNotFound)
	})

	t.Run("a recipient cannot change the photo or the album", func(t *testing.T) {
		wantStatus(t, s.boris.json(http.MethodPost, "/api/v1/photos/trash", ids(photo)), http.StatusNoContent)
		wantIDs(t, s.anna.photoIDs("/api/v1/photos"), photo)
		wantStatus(t, s.boris.json(http.MethodPatch, "/api/v1/albums/"+itoa(album), map[string]string{"name": "Mine"}), http.StatusNotFound)
		wantStatus(t, s.boris.json(http.MethodDelete, "/api/v1/albums/"+itoa(album), nil), http.StatusNotFound)
		wantStatus(t, s.boris.json(http.MethodPost, "/api/v1/albums/"+itoa(album)+"/photos/remove", ids(photo)), http.StatusNotFound)

		mine := s.boris.newAlbum(t, "Mine")
		wantStatus(t, s.boris.json(http.MethodPost, "/api/v1/albums/"+itoa(mine)+"/photos", ids(photo)), http.StatusNoContent)
		wantIDs(t, s.boris.photoIDs("/api/v1/photos?album="+itoa(mine)))
	})

	t.Run("unknown visibility", func(t *testing.T) {
		res := s.anna.json(http.MethodPost, "/api/v1/photos/sharing", map[string]any{"ids": []int64{photo}, "visibility": "friends"})
		wantStatus(t, res, http.StatusBadRequest)
	})

	t.Run("the state as the owner sees it", func(t *testing.T) {
		res := s.anna.get("/api/v1/photos/" + itoa(photo) + "/sharing")
		wantStatus(t, res, http.StatusOK)
		state := decode[sharingJSON](t, res)
		if state.Visibility != "private" || len(state.Users) != 1 || state.Users[0].Login != "boris" {
			t.Errorf("sharing = %+v", state)
		}
	})

	t.Run("sharing with the owner does nothing", func(t *testing.T) {
		sharePhoto(t, s.root, photo, map[string]any{"add": []int64{1}})
		res := s.anna.get("/api/v1/photos/" + itoa(photo) + "/sharing")
		wantStatus(t, res, http.StatusOK)
		if state := decode[sharingJSON](t, res); len(state.Users) != 1 {
			t.Errorf("sharing = %+v", state)
		}
	})
}

func TestSharedLists(t *testing.T) {
	s := setupSharing(t)
	direct := s.anna.addPhoto("landscape.jpg")
	inAlbum := s.anna.addPhoto("landscape.png")
	private := s.anna.addPhoto("landscape.webp")
	for _, photo := range []int64{direct, inAlbum, private} {
		markReady(t, s.e, photo)
	}
	album := s.anna.newAlbum(t, "Trip", inAlbum)
	sharePhoto(t, s.anna, direct, map[string]any{"add": []int64{s.borisID}})
	shareAlbum(t, s.anna, album, map[string]any{"add": []int64{s.borisID}})

	t.Run("album photos stay out of the list by default", func(t *testing.T) {
		wantIDs(t, s.boris.photoIDs("/api/v1/photos/shared"), direct)
		wantIDs(t, s.vera.photoIDs("/api/v1/photos/shared"))
	})

	t.Run("the user can add them, and the setting is personal", func(t *testing.T) {
		res := s.boris.json(http.MethodPatch, "/api/v1/me", map[string]bool{"sharedIncludesAlbums": true})
		wantStatus(t, res, http.StatusOK)
		if !decode[userJSON](t, res).SharedIncludesAlbums {
			t.Error("the setting did not change")
		}
		wantIDs(t, s.boris.photoIDs("/api/v1/photos/shared"), inAlbum, direct)
		wantIDs(t, s.vera.photoIDs("/api/v1/photos/shared"))
	})

	t.Run("the owner's own photos are not shared with them", func(t *testing.T) {
		wantIDs(t, s.anna.photoIDs("/api/v1/photos/shared"))
	})

	t.Run("an administrator sees only what was shared with them here", func(t *testing.T) {
		wantIDs(t, s.root.photoIDs("/api/v1/photos/shared"))
	})

	t.Run("a shared photo stays out of the recipient's library", func(t *testing.T) {
		wantIDs(t, s.boris.photoIDs("/api/v1/photos"))
	})

	t.Run("shared albums", func(t *testing.T) {
		res := s.boris.get("/api/v1/albums/shared")
		wantStatus(t, res, http.StatusOK)
		albums := decode[[]albumJSON](t, res)
		if len(albums) != 1 || albums[0].ID != album || !albums[0].Shared || albums[0].PhotoCount != 1 {
			t.Errorf("shared albums = %+v", albums)
		}

		res = s.vera.get("/api/v1/albums/shared")
		wantStatus(t, res, http.StatusOK)
		if albums := decode[[]albumJSON](t, res); len(albums) != 0 {
			t.Errorf("vera's shared albums = %+v", albums)
		}
		res = s.anna.get("/api/v1/albums/shared")
		wantStatus(t, res, http.StatusOK)
		if albums := decode[[]albumJSON](t, res); len(albums) != 0 {
			t.Errorf("the owner's shared albums = %+v", albums)
		}
	})

	t.Run("opening a shared album", func(t *testing.T) {
		wantStatus(t, s.boris.get("/api/v1/albums/"+itoa(album)), http.StatusOK)
		wantIDs(t, s.boris.photoIDs("/api/v1/photos?album="+itoa(album)), inAlbum)
		wantStatus(t, s.vera.get("/api/v1/albums/"+itoa(album)), http.StatusNotFound)
		wantStatus(t, s.vera.get("/api/v1/photos?album="+itoa(album)), http.StatusNotFound)
	})

	t.Run("tags stay personal", func(t *testing.T) {
		res := s.anna.json(http.MethodPost, "/api/v1/tags", map[string]string{"name": "Private"})
		wantStatus(t, res, http.StatusCreated)
		tag := decode[tagRefJSON](t, res).ID
		res = s.anna.json(http.MethodPost, "/api/v1/photos/tags", map[string]any{"ids": []int64{inAlbum}, "add": []int64{tag}})
		wantStatus(t, res, http.StatusNoContent)

		wantStatus(t, s.boris.get("/api/v1/photos?album="+itoa(album)+"&tag="+itoa(tag)), http.StatusBadRequest)
		res = s.boris.get("/api/v1/photos/" + itoa(inAlbum) + "/tags")
		wantStatus(t, res, http.StatusOK)
		if tags := decode[[]tagRefJSON](t, res); len(tags) != 0 {
			t.Errorf("the recipient sees tags %+v", tags)
		}
	})
}

func TestAdminView(t *testing.T) {
	s := setupSharing(t)
	annas := s.anna.addPhoto("landscape.jpg")
	markReady(t, s.e, annas)
	borises := s.boris.addPhoto("landscape.png")
	album := s.anna.newAlbum(t, "Trip", annas)

	t.Run("sees everything", func(t *testing.T) {
		wantIDs(t, s.root.photoIDs("/api/v1/photos/all"), borises, annas)
		wantIDs(t, s.root.photoIDs("/api/v1/photos/all?owner=1"), annas)
		wantStatus(t, s.root.get("/api/v1/albums/"+itoa(album)), http.StatusOK)
		wantIDs(t, s.root.photoIDs("/api/v1/photos?album="+itoa(album)), annas)
		wantStatus(t, s.root.get("/api/v1/photos/all?owner=x"), http.StatusBadRequest)
	})

	t.Run("the view is for administrators", func(t *testing.T) {
		wantStatus(t, s.anna.get("/api/v1/photos/all"), http.StatusForbidden)
	})

	t.Run("changes the sharing of another user's photo", func(t *testing.T) {
		sharePhoto(t, s.root, annas, map[string]any{"add": []int64{s.veraID}})
		wantStatus(t, s.root.get("/api/v1/photos/"+itoa(annas)+"/sharing"), http.StatusOK)
		wantIDs(t, s.vera.photoIDs("/api/v1/photos/shared"), annas)
	})

	t.Run("moves it to the trash, and only the owner goes further", func(t *testing.T) {
		wantStatus(t, s.root.json(http.MethodPost, "/api/v1/photos/trash", ids(annas)), http.StatusNoContent)
		wantIDs(t, s.anna.photoIDs("/api/v1/photos/trash"), annas)

		wantStatus(t, s.root.json(http.MethodPost, "/api/v1/photos/purge", ids(annas)), http.StatusNoContent)
		wantStatus(t, s.root.json(http.MethodPost, "/api/v1/photos/restore", ids(annas)), http.StatusNoContent)
		wantIDs(t, s.anna.photoIDs("/api/v1/photos/trash"), annas)

		wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/photos/restore", ids(annas)), http.StatusNoContent)
		wantIDs(t, s.anna.photoIDs("/api/v1/photos"), annas)
	})
}

func TestUserDirectory(t *testing.T) {
	s := setupSharing(t)
	res := s.root.json(http.MethodPatch, "/api/v1/users/"+itoa(s.veraID), map[string]bool{"disabled": true})
	wantStatus(t, res, http.StatusOK)

	res = s.anna.get("/api/v1/users/directory")
	wantStatus(t, res, http.StatusOK)
	users := decode[[]userRefJSON](t, res)
	logins := make([]string, len(users))
	for i, u := range users {
		logins[i] = u.Login
	}
	if len(logins) != 3 || logins[0] != "anna" || logins[1] != "boris" || logins[2] != "root" {
		t.Errorf("directory = %v", logins)
	}

	t.Run("a disabled user cannot receive photos", func(t *testing.T) {
		photo := s.anna.addPhoto("landscape.jpg")
		sharePhoto(t, s.anna, photo, map[string]any{"add": []int64{s.veraID}})
		res := s.anna.get("/api/v1/photos/" + itoa(photo) + "/sharing")
		wantStatus(t, res, http.StatusOK)
		if state := decode[sharingJSON](t, res); len(state.Users) != 0 {
			t.Errorf("sharing = %+v", state)
		}
	})
}
