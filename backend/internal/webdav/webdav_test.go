package webdav

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/jobs"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
	"github.com/denuser/fuchsia-photos/backend/internal/testdb"
)

const fixtures = "../media/testdata"

// env is the WebDAV handler on a real database. Jobs are queued but not run.
type env struct {
	t       *testing.T
	server  *httptest.Server
	queries *store.Queries
}

func setup(t *testing.T) *env {
	t.Helper()
	pool := testdb.New(t)
	layout := storage.Layout{Library: t.TempDir(), Cache: t.TempDir()}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	client, err := jobs.NewInsertClient(pool)
	if err != nil {
		t.Fatal(err)
	}
	queries := store.New(pool)
	handler := &Handler{Queries: queries, Ingest: &ingest.Service{Pool: pool, Jobs: client, Layout: layout}, Layout: layout}
	mux := http.NewServeMux()
	mux.Handle("/webdav", handler)
	mux.Handle("/webdav/", handler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return &env{t, server, queries}
}

// client is a WebDAV client logged in as a user with an app password.
type client struct {
	e        *env
	user     store.User
	login    string
	password string
}

func (e *env) addUser(login string) *client {
	e.t.Helper()
	user, err := e.queries.CreateUser(context.Background(), store.CreateUserParams{Login: login, PasswordHash: "x", Role: auth.RoleUser})
	if err != nil {
		e.t.Fatal(err)
	}
	password, hash, err := auth.NewAppPassword()
	if err != nil {
		e.t.Fatal(err)
	}
	_, err = e.queries.CreateAppPassword(context.Background(), store.CreateAppPasswordParams{UserID: user.ID, Name: "phone", TokenHash: hash})
	if err != nil {
		e.t.Fatal(err)
	}
	return &client{e, user, login, password}
}

func (c *client) do(method, path string, body []byte, header map[string]string) *http.Response {
	c.e.t.Helper()
	req, err := http.NewRequest(method, c.e.server.URL+path, bytes.NewReader(body))
	if err != nil {
		c.e.t.Fatal(err)
	}
	if c.login != "" {
		req.SetBasicAuth(c.login, c.password)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	c.e.t.Cleanup(func() { res.Body.Close() })
	return res
}

func (c *client) put(path, fixture string) *http.Response {
	c.e.t.Helper()
	return c.do(http.MethodPut, path, readFixture(c.e.t, fixture), nil)
}

func (c *client) propfind(path, depth string) *http.Response {
	c.e.t.Helper()
	return c.do("PROPFIND", path, nil, map[string]string{"Depth": depth})
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtures, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func body(t *testing.T, res *http.Response) string {
	t.Helper()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func wantStatus(t *testing.T, res *http.Response, status int) {
	t.Helper()
	if res.StatusCode != status {
		t.Fatalf("%s %s: status = %d, want %d: %s", res.Request.Method, res.Request.URL.Path, res.StatusCode, status, body(t, res))
	}
}

func (e *env) photoCount(ownerID int64) int64 {
	e.t.Helper()
	counts, err := e.queries.CountPhotosByStatus(context.Background(), ownerID)
	if err != nil {
		e.t.Fatal(err)
	}
	return counts.Pending + counts.Failed // Jobs do not run, so none is ready.
}

func TestAuthentication(t *testing.T) {
	e := setup(t)
	anna := e.addUser("anna")

	tests := []struct {
		name            string
		login, password string
	}{
		{"no credentials", "", ""},
		{"wrong password", "anna", "wrong"},
		{"another user's login", "boris", anna.password},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &client{e: e, login: tt.login, password: tt.password}
			res := c.propfind("/webdav/", "0")
			wantStatus(t, res, http.StatusUnauthorized)
			if !strings.HasPrefix(res.Header.Get("WWW-Authenticate"), "Basic ") {
				t.Errorf("WWW-Authenticate = %q, want Basic", res.Header.Get("WWW-Authenticate"))
			}
		})
	}

	upper := &client{e: e, login: "ANNA", password: anna.password}
	wantStatus(t, upper.propfind("/webdav/", "0"), http.StatusMultiStatus)
	wantStatus(t, anna.propfind("/webdav", "0"), http.StatusMultiStatus)

	passwords, err := e.queries.ListAppPasswords(context.Background(), anna.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if passwords[0].LastUsedAt == nil {
		t.Error("the last use of the app password was not recorded")
	}

	err = e.queries.SetUserDisabled(context.Background(), store.SetUserDisabledParams{ID: anna.user.ID, Disabled: true})
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, anna.propfind("/webdav/", "0"), http.StatusUnauthorized)
	err = e.queries.SetUserDisabled(context.Background(), store.SetUserDisabledParams{ID: anna.user.ID, Disabled: false})
	if err != nil {
		t.Fatal(err)
	}

	_, err = e.queries.DeleteAppPassword(context.Background(), store.DeleteAppPasswordParams{ID: passwords[0].ID, UserID: anna.user.ID})
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, anna.propfind("/webdav/", "0"), http.StatusUnauthorized)
}

func TestUpload(t *testing.T) {
	e := setup(t)
	anna := e.addUser("anna")

	wantStatus(t, anna.do("MKCOL", "/webdav/DCIM", nil, nil), http.StatusCreated)
	res := anna.do(http.MethodPut, "/webdav/DCIM/IMG_0001.JPG", readFixture(t, "landscape.jpg"), map[string]string{"X-OC-Mtime": "1700000000"})
	wantStatus(t, res, http.StatusCreated)
	if res.Header.Get("ETag") == "" {
		t.Error("PUT returned no ETag")
	}
	if got := e.photoCount(anna.user.ID); got != 1 {
		t.Fatalf("photos = %d, want 1", got)
	}

	res = anna.propfind("/webdav/DCIM/", "1")
	wantStatus(t, res, http.StatusMultiStatus)
	listing := body(t, res)
	size := len(readFixture(t, "landscape.jpg"))
	for _, want := range []string{"/webdav/DCIM/IMG_0001.JPG", "Tue, 14 Nov 2023 22:13:20 GMT", ">" + strconv.Itoa(size) + "<", "image/jpeg"} {
		if !strings.Contains(listing, want) {
			t.Errorf("the listing does not contain %q:\n%s", want, listing)
		}
	}

	// Sending the same file again replaces the entry and adds no photo.
	wantStatus(t, anna.put("/webdav/DCIM/IMG_0001.JPG", "landscape.jpg"), http.StatusNoContent)
	wantStatus(t, anna.put("/webdav/DCIM/copy.jpg", "landscape.jpg"), http.StatusCreated)
	if got := e.photoCount(anna.user.ID); got != 1 {
		t.Errorf("photos = %d, want 1", got)
	}

	res = anna.do(http.MethodGet, "/webdav/DCIM/IMG_0001.JPG", nil, nil)
	wantStatus(t, res, http.StatusOK)
	if got := body(t, res); got != string(readFixture(t, "landscape.jpg")) {
		t.Error("GET did not return the original")
	}

	wantStatus(t, anna.put("/webdav/missing/a.jpg", "landscape.jpg"), http.StatusConflict)
	wantStatus(t, anna.put("/webdav/DCIM", "landscape.jpg"), http.StatusMethodNotAllowed)
	wantStatus(t, anna.do(http.MethodPut, "/webdav/DCIM/notes.txt", []byte("not a photo at all"), nil), http.StatusUnsupportedMediaType)
	wantStatus(t, anna.propfind("/webdav/DCIM/notes.txt", "0"), http.StatusNotFound)
	wantStatus(t, anna.put("/webdav/DCIM/clip.mov", "video.mov"), http.StatusCreated)
	if got := e.photoCount(anna.user.ID); got != 2 {
		t.Errorf("photos = %d, want 2 with the video", got)
	}
	wantStatus(t, anna.do("COPY", "/webdav/DCIM/copy.jpg", nil, map[string]string{"Destination": "/webdav/DCIM/b.jpg"}), http.StatusMethodNotAllowed)
}

// Clearing space on a phone must not delete photos from the library.
func TestDeleteKeepsPhoto(t *testing.T) {
	e := setup(t)
	anna := e.addUser("anna")

	wantStatus(t, anna.do("MKCOL", "/webdav/DCIM", nil, nil), http.StatusCreated)
	wantStatus(t, anna.put("/webdav/DCIM/a.jpg", "landscape.jpg"), http.StatusCreated)
	wantStatus(t, anna.do(http.MethodDelete, "/webdav/DCIM", nil, nil), http.StatusNoContent)
	wantStatus(t, anna.propfind("/webdav/DCIM/a.jpg", "0"), http.StatusNotFound)
	wantStatus(t, anna.propfind("/webdav/DCIM", "0"), http.StatusNotFound)

	if photo := e.photo(anna.user.ID, "landscape.jpg"); photo.DeletedAt != nil {
		t.Error("deleting the file moved its photo to the trash")
	}
}

// A phone that sends its camera roll again must not bring back what the owner
// deleted, as with a folder scan.
func TestTrashedPhotoStaysInTrash(t *testing.T) {
	e := setup(t)
	anna := e.addUser("anna")

	wantStatus(t, anna.put("/webdav/a.jpg", "landscape.jpg"), http.StatusCreated)
	photo := e.photo(anna.user.ID, "landscape.jpg")
	err := e.queries.TrashPhotos(context.Background(), store.TrashPhotosParams{Ids: []int64{photo.ID}, ActorID: anna.user.ID})
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, anna.do(http.MethodDelete, "/webdav/a.jpg", nil, nil), http.StatusNoContent)
	wantStatus(t, anna.put("/webdav/a.jpg", "landscape.jpg"), http.StatusCreated)
	if photo := e.photo(anna.user.ID, "landscape.jpg"); photo.DeletedAt == nil {
		t.Error("sending the photo again took it out of the trash")
	}
}

func TestMove(t *testing.T) {
	e := setup(t)
	anna := e.addUser("anna")

	// Some clients upload under a temporary name and rename the file at the end.
	wantStatus(t, anna.put("/webdav/a.jpg.part", "landscape.jpg"), http.StatusCreated)
	wantStatus(t, anna.do("MOVE", "/webdav/a.jpg.part", nil, map[string]string{"Destination": "/webdav/a.jpg"}), http.StatusCreated)
	wantStatus(t, anna.do(http.MethodGet, "/webdav/a.jpg", nil, nil), http.StatusOK)
	wantStatus(t, anna.propfind("/webdav/a.jpg.part", "0"), http.StatusNotFound)

	// Behind a proxy that rewrites Host, the destination names another host.
	wantStatus(t, anna.do("MOVE", "/webdav/a.jpg", nil, map[string]string{"Destination": "http://nas.example:5173/webdav/c.jpg"}), http.StatusCreated)
	wantStatus(t, anna.do("MOVE", "/webdav/c.jpg", nil, map[string]string{"Destination": "/webdav/a.jpg"}), http.StatusCreated)

	wantStatus(t, anna.do("MKCOL", "/webdav/old", nil, nil), http.StatusCreated)
	wantStatus(t, anna.do("MKCOL", "/webdav/old/inner", nil, nil), http.StatusCreated)
	wantStatus(t, anna.put("/webdav/old/inner/b.png", "landscape.png"), http.StatusCreated)
	wantStatus(t, anna.do("MOVE", "/webdav/old", nil, map[string]string{"Destination": "/webdav/new"}), http.StatusCreated)
	wantStatus(t, anna.do(http.MethodGet, "/webdav/new/inner/b.png", nil, nil), http.StatusOK)
	res := anna.propfind("/webdav/new/inner", "1")
	wantStatus(t, res, http.StatusMultiStatus)
	if listing := body(t, res); !strings.Contains(listing, "/webdav/new/inner/b.png") {
		t.Errorf("the moved folder does not list its file:\n%s", listing)
	}
	wantStatus(t, anna.propfind("/webdav/old", "0"), http.StatusNotFound)

	// x/net/webdav answers 403 to any failed move.
	wantStatus(t, anna.do("MOVE", "/webdav/a.jpg", nil, map[string]string{"Destination": "/webdav/missing/a.jpg"}), http.StatusForbidden)
	wantStatus(t, anna.do("MOVE", "/webdav/new", nil, map[string]string{"Destination": "/webdav/new/inside"}), http.StatusForbidden)
	wantStatus(t, anna.do(http.MethodGet, "/webdav/new/inner/b.png", nil, nil), http.StatusOK)
}

func TestUsersAreSeparate(t *testing.T) {
	e := setup(t)
	anna := e.addUser("anna")
	boris := e.addUser("boris")

	wantStatus(t, anna.do("MKCOL", "/webdav/DCIM", nil, nil), http.StatusCreated)
	wantStatus(t, anna.put("/webdav/DCIM/a.jpg", "landscape.jpg"), http.StatusCreated)

	res := boris.propfind("/webdav/", "1")
	wantStatus(t, res, http.StatusMultiStatus)
	if listing := body(t, res); strings.Contains(listing, "DCIM") {
		t.Errorf("boris sees anna's folder:\n%s", listing)
	}
	wantStatus(t, boris.do(http.MethodGet, "/webdav/DCIM/a.jpg", nil, nil), http.StatusNotFound)

	// The same file makes a photo of each user's own.
	wantStatus(t, boris.do("MKCOL", "/webdav/DCIM", nil, nil), http.StatusCreated)
	wantStatus(t, boris.put("/webdav/DCIM/a.jpg", "landscape.jpg"), http.StatusCreated)
	if got := e.photoCount(boris.user.ID); got != 1 {
		t.Errorf("boris's photos = %d, want 1", got)
	}
}

func (e *env) photo(ownerID int64, fixture string) store.Photo {
	e.t.Helper()
	sum := sha256.Sum256(readFixture(e.t, fixture))
	photo, err := e.queries.GetPhotoByHash(context.Background(), store.GetPhotoByHashParams{OwnerID: ownerID, Sha256: sum[:]})
	if err != nil {
		e.t.Fatal(err)
	}
	return photo
}

func (e *env) addAlbum(owner *client, name string) int64 {
	e.t.Helper()
	id, err := e.queries.CreateAlbum(context.Background(), store.CreateAlbumParams{OwnerID: owner.user.ID, Name: name})
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *env) link(owner *client, path string, albumID int64) {
	e.t.Helper()
	_, err := e.queries.CreateWebDAVAlbum(context.Background(), store.CreateWebDAVAlbumParams{UserID: owner.user.ID, Path: path, AlbumID: albumID})
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) albumSize(owner *client, albumID int64) int64 {
	e.t.Helper()
	albums, err := e.queries.ListAlbums(context.Background(), store.ListAlbumsParams{OwnerID: owner.user.ID, ID: &albumID})
	if err != nil || len(albums) != 1 {
		e.t.Fatalf("album %d: %v", albumID, err)
	}
	return albums[0].PhotoCount
}

func TestFolderAlbums(t *testing.T) {
	e := setup(t)
	anna := e.addUser("anna")
	camera := e.addAlbum(anna, "Camera")
	screenshots := e.addAlbum(anna, "Screenshots")
	// A link may come before the folder exists.
	e.link(anna, "/DCIM", camera)
	e.link(anna, "/DCIM/Screenshots", screenshots)

	wantStatus(t, anna.do("MKCOL", "/webdav/DCIM", nil, nil), http.StatusCreated)
	wantStatus(t, anna.do("MKCOL", "/webdav/DCIM/Screenshots", nil, nil), http.StatusCreated)
	wantStatus(t, anna.do("MKCOL", "/webdav/DCIM/2026", nil, nil), http.StatusCreated)
	wantStatus(t, anna.do("MKCOL", "/webdav/DCIMX", nil, nil), http.StatusCreated)
	wantStatus(t, anna.put("/webdav/DCIM/2026/a.jpg", "landscape.jpg"), http.StatusCreated)
	wantStatus(t, anna.put("/webdav/DCIM/Screenshots/b.png", "landscape.png"), http.StatusCreated)
	wantStatus(t, anna.put("/webdav/DCIMX/c.webp", "landscape.webp"), http.StatusCreated)
	wantStatus(t, anna.put("/webdav/d.heic", "landscape.heic"), http.StatusCreated)
	if got := e.albumSize(anna, camera); got != 1 {
		t.Errorf("Camera has %d photos, want 1: the nearest linked folder decides", got)
	}
	if got := e.albumSize(anna, screenshots); got != 1 {
		t.Errorf("Screenshots has %d photos, want 1", got)
	}

	// A photo the library already has goes to the album too.
	wantStatus(t, anna.put("/webdav/DCIM/Screenshots/copy.jpg", "landscape.jpg"), http.StatusCreated)
	if got := e.albumSize(anna, screenshots); got != 2 {
		t.Errorf("Screenshots has %d photos, want 2", got)
	}

	// Deleting or moving files leaves their photos in the album.
	wantStatus(t, anna.do("MOVE", "/webdav/DCIM/Screenshots/b.png", nil, map[string]string{"Destination": "/webdav/b.png"}), http.StatusCreated)
	wantStatus(t, anna.do(http.MethodDelete, "/webdav/DCIM", nil, nil), http.StatusNoContent)
	if got := e.albumSize(anna, screenshots); got != 2 {
		t.Errorf("Screenshots has %d photos after the delete, want 2", got)
	}

	// A photo in the trash is not added.
	trashed := e.photo(anna.user.ID, "landscape.webp")
	err := e.queries.TrashPhotos(context.Background(), store.TrashPhotosParams{Ids: []int64{trashed.ID}, ActorID: anna.user.ID})
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, anna.do("MKCOL", "/webdav/DCIM", nil, nil), http.StatusCreated)
	wantStatus(t, anna.put("/webdav/DCIM/c.webp", "landscape.webp"), http.StatusCreated)
	err = e.queries.RestorePhotos(context.Background(), store.RestorePhotosParams{Ids: []int64{trashed.ID}, OwnerID: anna.user.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.albumSize(anna, camera); got != 1 {
		t.Errorf("Camera has %d photos, want 1: a photo in the trash was added", got)
	}

	// The root links the whole tree.
	boris := e.addUser("boris")
	all := e.addAlbum(boris, "Phone")
	e.link(boris, "/", all)
	wantStatus(t, boris.put("/webdav/a.jpg", "landscape.jpg"), http.StatusCreated)
	if got := e.albumSize(boris, all); got != 1 {
		t.Errorf("Phone has %d photos, want 1", got)
	}
}
