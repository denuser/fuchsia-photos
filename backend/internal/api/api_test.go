package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/jobs"
	"github.com/denuser/fuchsia-photos/backend/internal/scan"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
	"github.com/denuser/fuchsia-photos/backend/internal/testdb"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func TestRoutes(t *testing.T) {
	static := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("static"))
	})

	tests := []struct {
		name       string
		db         fakeDB
		path       string
		wantStatus int
		wantBody   string
	}{
		{"healthy", fakeDB{}, "/api/v1/health", http.StatusOK, `"ok"`},
		{"database down", fakeDB{errors.New("down")}, "/api/v1/health", http.StatusServiceUnavailable, `"unavailable"`},
		{"unknown api path is not the frontend", fakeDB{}, "/api/v1/nope", http.StatusNotFound, `"not found"`},
		{"other paths are the frontend", fakeDB{}, "/albums", http.StatusOK, "static"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewHandler(Deps{DB: tt.db, Static: static}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

// env is the API on a real database. Jobs are queued but not run, so tests that
// need previews write them directly.
type env struct {
	t       *testing.T
	server  *httptest.Server
	pool    *pgxpool.Pool
	queries *store.Queries
	layout  storage.Layout
	// importDir is the root of folder scans.
	importDir string
}

func setup(t *testing.T) *env {
	t.Helper()
	return setupWith(t, nil)
}

// setupWith lets a test change the handler's dependencies before it starts.
func setupWith(t *testing.T, change func(*Deps)) *env {
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
	importDir := t.TempDir()
	ingester := &ingest.Service{Pool: pool, Jobs: client, Layout: layout}
	deps := Deps{
		DB:      pool,
		Pool:    pool,
		Queries: queries,
		Ingest:  ingester,
		Scan:    &scan.Service{Ingest: ingester, Dir: importDir},
		Layout:  layout,
		Static:  http.NotFoundHandler(),
	}
	if change != nil {
		change(&deps)
	}
	server := httptest.NewServer(NewHandler(deps))
	t.Cleanup(server.Close)
	return &env{t, server, pool, queries, layout, importDir}
}

func (e *env) addUser(login, role string) store.User {
	e.t.Helper()
	hash, err := auth.HashPassword("password " + login)
	if err != nil {
		e.t.Fatal(err)
	}
	user, err := e.queries.CreateUser(context.Background(), store.CreateUserParams{Login: login, PasswordHash: hash, Role: role})
	if err != nil {
		e.t.Fatal(err)
	}
	return user
}

// browser is an HTTP client with its own cookies.
type browser struct {
	e      *env
	client *http.Client
}

func (e *env) browser() *browser {
	e.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return &browser{e, &http.Client{Jar: jar}}
}

func (e *env) loggedIn(login string) *browser {
	e.t.Helper()
	b := e.browser()
	res := b.json(http.MethodPost, "/api/v1/login", map[string]string{"login": login, "password": "password " + login})
	if res.StatusCode != http.StatusOK {
		e.t.Fatalf("login as %s: status %d", login, res.StatusCode)
	}
	return b
}

func (b *browser) do(req *http.Request) *http.Response {
	b.e.t.Helper()
	res, err := b.client.Do(req)
	if err != nil {
		b.e.t.Fatal(err)
	}
	b.e.t.Cleanup(func() { res.Body.Close() })
	return res
}

func (b *browser) get(path string) *http.Response {
	b.e.t.Helper()
	req, err := http.NewRequest(http.MethodGet, b.e.server.URL+path, nil)
	if err != nil {
		b.e.t.Fatal(err)
	}
	return b.do(req)
}

func (b *browser) json(method, path string, body any) *http.Response {
	b.e.t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		b.e.t.Fatal(err)
	}
	req, err := http.NewRequest(method, b.e.server.URL+path, bytes.NewReader(data))
	if err != nil {
		b.e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	return b.do(req)
}

func (b *browser) upload(fixture string) *http.Response {
	b.e.t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "media", "testdata", fixture))
	if err != nil {
		b.e.t.Fatal(err)
	}
	return b.uploadBytes(fixture, data)
}

func (b *browser) uploadBytes(name string, data []byte) *http.Response {
	b.e.t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", name)
	if err != nil {
		b.e.t.Fatal(err)
	}
	part.Write(data)
	form.Close()

	req, err := http.NewRequest(http.MethodPost, b.e.server.URL+"/api/v1/photos", &body)
	if err != nil {
		b.e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	return b.do(req)
}

func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	var out T
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

type uploadResult struct {
	Photo     photoJSON `json:"photo"`
	Duplicate bool      `json:"duplicate"`
}

func wantStatus(t *testing.T, res *http.Response, status int) {
	t.Helper()
	if res.StatusCode != status {
		body, _ := io.ReadAll(res.Body)
		t.Errorf("%s %s: status = %d, want %d: %s", res.Request.Method, res.Request.URL.Path, res.StatusCode, status, body)
	}
}

func TestLogin(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)

	anonymous := e.browser()
	wantStatus(t, anonymous.get("/api/v1/me"), http.StatusUnauthorized)
	wantStatus(t, anonymous.json(http.MethodPost, "/api/v1/login", map[string]string{"login": "anna", "password": "wrong"}), http.StatusUnauthorized)
	wantStatus(t, anonymous.json(http.MethodPost, "/api/v1/login", map[string]string{"login": "nobody", "password": "wrong"}), http.StatusUnauthorized)

	anna := e.browser()
	res := anna.json(http.MethodPost, "/api/v1/login", map[string]string{"login": "ANNA", "password": "password anna"})
	wantStatus(t, res, http.StatusOK)
	cookie := res.Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie = %+v, want HttpOnly and SameSite=Lax", cookie)
	}

	me := decode[userJSON](t, anna.get("/api/v1/me"))
	if me.Login != "anna" {
		t.Errorf("login = %q", me.Login)
	}

	wantStatus(t, anna.json(http.MethodPost, "/api/v1/logout", nil), http.StatusNoContent)
	wantStatus(t, anna.get("/api/v1/me"), http.StatusUnauthorized)
}

func TestSessionTokenIsNotStored(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	res := e.browser().json(http.MethodPost, "/api/v1/login", map[string]string{"login": "anna", "password": "password anna"})
	token := res.Cookies()[0].Value

	if _, err := e.queries.GetSessionUser(context.Background(), []byte(token)); err == nil {
		t.Error("the raw token works as a stored session key")
	}
}

func TestViewerShowsOriginals(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")

	if decode[userJSON](t, anna.get("/api/v1/me")).ViewerShowsOriginals {
		t.Fatal("the viewer shows originals before the user asks for it")
	}
	res := anna.json(http.MethodPatch, "/api/v1/me", map[string]bool{"viewerShowsOriginals": true})
	wantStatus(t, res, http.StatusOK)
	if !decode[userJSON](t, res).ViewerShowsOriginals {
		t.Error("the response does not show the new setting")
	}
	if !decode[userJSON](t, anna.get("/api/v1/me")).ViewerShowsOriginals {
		t.Error("the setting was not saved")
	}
	if decode[userJSON](t, boris.get("/api/v1/me")).ViewerShowsOriginals {
		t.Error("the setting changed for another user")
	}
}

func TestViewerHiddenFields(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")

	if got := decode[userJSON](t, anna.get("/api/v1/me")).ViewerHiddenFields; got == nil || len(got) != 0 {
		t.Fatalf("hidden fields = %#v, want an empty list", got)
	}

	res := anna.json(http.MethodPatch, "/api/v1/me", map[string][]string{"viewerHiddenFields": {"location", "iso", "location"}})
	wantStatus(t, res, http.StatusOK)
	want := []string{"iso", "location"}
	if got := decode[userJSON](t, res).ViewerHiddenFields; !slices.Equal(got, want) {
		t.Errorf("hidden fields = %v, want %v in the viewer's order without repeats", got, want)
	}

	res = anna.json(http.MethodPatch, "/api/v1/me", map[string][]string{"viewerHiddenFields": {"iso", "password"}})
	wantStatus(t, res, http.StatusBadRequest)
	if got := decode[userJSON](t, anna.get("/api/v1/me")).ViewerHiddenFields; !slices.Equal(got, want) {
		t.Errorf("an invalid request changed the hidden fields to %v", got)
	}

	// Changing another setting leaves the list alone.
	wantStatus(t, anna.json(http.MethodPatch, "/api/v1/me", map[string]bool{"viewerShowsOriginals": true}), http.StatusOK)
	if got := decode[userJSON](t, anna.get("/api/v1/me")).ViewerHiddenFields; !slices.Equal(got, want) {
		t.Errorf("hidden fields = %v after another change, want %v", got, want)
	}
}

func TestLanguage(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")

	if got := decode[userJSON](t, anna.get("/api/v1/me")).Language; got != "auto" {
		t.Fatalf("language = %q, want auto by default", got)
	}
	res := anna.json(http.MethodPatch, "/api/v1/me", map[string]string{"language": "ru"})
	wantStatus(t, res, http.StatusOK)
	if got := decode[userJSON](t, res).Language; got != "ru" {
		t.Errorf("the response shows language %q, want ru", got)
	}
	if got := decode[userJSON](t, boris.get("/api/v1/me")).Language; got != "auto" {
		t.Errorf("the setting changed another user's language to %q", got)
	}

	// An unknown language is refused, and nothing else in the request is applied.
	res = anna.json(http.MethodPatch, "/api/v1/me", map[string]any{"language": "de", "gridSquares": true})
	wantStatus(t, res, http.StatusBadRequest)
	me := decode[userJSON](t, anna.get("/api/v1/me"))
	if me.Language != "ru" || me.GridSquares {
		t.Errorf("an invalid request changed the settings: language %q, grid squares %v", me.Language, me.GridSquares)
	}
}

func TestChangePassword(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	here, other := e.loggedIn("anna"), e.loggedIn("anna")

	wantStatus(t, here.json(http.MethodPost, "/api/v1/me/password",
		map[string]string{"currentPassword": "wrong", "newPassword": "new password"}), http.StatusForbidden)
	wantStatus(t, here.json(http.MethodPost, "/api/v1/me/password",
		map[string]string{"currentPassword": "password anna", "newPassword": "short"}), http.StatusBadRequest)
	wantStatus(t, here.json(http.MethodPost, "/api/v1/me/password",
		map[string]string{"currentPassword": "password anna", "newPassword": "new password"}), http.StatusNoContent)

	wantStatus(t, here.get("/api/v1/me"), http.StatusOK)
	wantStatus(t, other.get("/api/v1/me"), http.StatusUnauthorized)
	wantStatus(t, e.browser().json(http.MethodPost, "/api/v1/login",
		map[string]string{"login": "anna", "password": "new password"}), http.StatusOK)
}

func TestUserManagement(t *testing.T) {
	e := setup(t)
	admin := e.addUser("root", auth.RoleAdmin)
	e.addUser("anna", auth.RoleUser)
	root, anna := e.loggedIn("root"), e.loggedIn("anna")

	t.Run("users cannot manage users", func(t *testing.T) {
		wantStatus(t, anna.get("/api/v1/users"), http.StatusForbidden)
		wantStatus(t, anna.json(http.MethodPost, "/api/v1/users",
			map[string]string{"login": "eve", "password": "long enough", "role": "admin"}), http.StatusForbidden)
		wantStatus(t, anna.json(http.MethodPatch, "/api/v1/users/1", map[string]string{"role": "admin"}), http.StatusForbidden)
	})

	t.Run("create", func(t *testing.T) {
		create := func(login, password, role string) *http.Response {
			return root.json(http.MethodPost, "/api/v1/users", map[string]string{"login": login, "password": password, "role": role})
		}
		wantStatus(t, create("boris", "long enough", "user"), http.StatusCreated)
		wantStatus(t, create("BORIS", "long enough", "user"), http.StatusConflict)
		wantStatus(t, create("no spaces", "long enough", "user"), http.StatusBadRequest)
		wantStatus(t, create("carl", "short", "user"), http.StatusBadRequest)
		wantStatus(t, create("carl", "long enough", "owner"), http.StatusBadRequest)

		users := decode[[]userJSON](t, root.get("/api/v1/users"))
		if len(users) != 3 {
			t.Errorf("got %d users, want 3", len(users))
		}
	})

	t.Run("an admin cannot lock themselves out", func(t *testing.T) {
		path := "/api/v1/users/" + itoa(admin.ID)
		wantStatus(t, root.json(http.MethodPatch, path, map[string]string{"role": "user"}), http.StatusConflict)
		wantStatus(t, root.json(http.MethodPatch, path, map[string]bool{"disabled": true}), http.StatusConflict)
	})

	t.Run("disabling ends sessions and blocks login", func(t *testing.T) {
		user := e.addUser("dora", auth.RoleUser)
		dora := e.loggedIn("dora")
		path := "/api/v1/users/" + itoa(user.ID)
		wantStatus(t, root.json(http.MethodPatch, path, map[string]bool{"disabled": true}), http.StatusOK)
		wantStatus(t, dora.get("/api/v1/me"), http.StatusUnauthorized)
		wantStatus(t, e.browser().json(http.MethodPost, "/api/v1/login",
			map[string]string{"login": "dora", "password": "password dora"}), http.StatusUnauthorized)
	})

	t.Run("resetting a password ends sessions", func(t *testing.T) {
		user := e.addUser("emil", auth.RoleUser)
		emil := e.loggedIn("emil")
		wantStatus(t, root.json(http.MethodPatch, "/api/v1/users/"+itoa(user.ID), map[string]string{"password": "brand new one"}), http.StatusOK)
		wantStatus(t, emil.get("/api/v1/me"), http.StatusUnauthorized)
	})

	wantStatus(t, root.json(http.MethodPatch, "/api/v1/users/999999", map[string]string{"role": "user"}), http.StatusNotFound)
}

func itoa(n int64) string {
	data, _ := json.Marshal(n)
	return string(data)
}

func TestUpload(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")

	wantStatus(t, e.browser().upload("landscape.jpg"), http.StatusUnauthorized)

	res := anna.upload("landscape.jpg")
	wantStatus(t, res, http.StatusCreated)
	first := decode[uploadResult](t, res)
	if first.Photo.Status != "pending" || first.Photo.Name != "landscape.jpg" || first.Photo.Mime != "image/jpeg" {
		t.Errorf("photo = %+v", first.Photo)
	}

	res = anna.upload("landscape.jpg")
	wantStatus(t, res, http.StatusOK)
	if second := decode[uploadResult](t, res); !second.Duplicate || second.Photo.ID != first.Photo.ID {
		t.Errorf("second upload = %+v", second)
	}

	wantStatus(t, anna.uploadBytes("notes.jpg", []byte("not a photo")), http.StatusUnsupportedMediaType)
}

// The access rules for photos, for every way of reading one.
func TestPhotoAccess(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	e.addUser("root", auth.RoleAdmin)
	anna, boris, root := e.loggedIn("anna"), e.loggedIn("boris"), e.loggedIn("root")

	photo := decode[uploadResult](t, anna.upload("landscape.jpg")).Photo
	markReady(t, e, photo.ID)

	base := "/api/v1/photos/" + itoa(photo.ID)
	paths := []string{base, base + "/previews/grid", base + "/previews/viewer", base + "/original"}
	for _, path := range paths {
		wantStatus(t, anna.get(path), http.StatusOK)
		wantStatus(t, root.get(path), http.StatusOK)
		wantStatus(t, boris.get(path), http.StatusNotFound)
		wantStatus(t, e.browser().get(path), http.StatusUnauthorized)
	}

	missing := "/api/v1/photos/999999"
	notFoundBody := func(res *http.Response) string {
		body, _ := io.ReadAll(res.Body)
		return string(body)
	}
	if a, b := notFoundBody(boris.get(base)), notFoundBody(boris.get(missing)); a != b {
		t.Errorf("a hidden photo looks different from a missing one: %q and %q", a, b)
	}

	if items := decode[timeline](t, boris.get("/api/v1/photos")).Items; len(items) != 0 {
		t.Errorf("boris sees %d photos in the timeline", len(items))
	}
}

func markReady(t *testing.T, e *env, id int64) {
	t.Helper()
	format := "webp"
	for _, kind := range []string{storage.PreviewGrid, storage.PreviewViewer} {
		if err := storage.WriteFileAtomic(e.layout.Preview(id, kind, format), []byte("preview")); err != nil {
			t.Fatal(err)
		}
	}
	err := e.queries.MarkPhotoReady(context.Background(), store.MarkPhotoReadyParams{ID: id, PreviewFormat: &format, Exif: []byte(testExif)})
	if err != nil {
		t.Fatal(err)
	}
}

const testExif = `{"Model":"Canon EOS R5","FNumber":2.8,"SerialNumber":"ABC123","GPSLatitude":59.93,"GPSLongitude":30.33}`

func TestPhotoJSONMetadata(t *testing.T) {
	e := setup(t)
	e.addUser("root", auth.RoleAdmin)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	admin, anna, boris := e.loggedIn("root"), e.loggedIn("anna"), e.loggedIn("boris")
	photo := decode[uploadResult](t, anna.upload("rotated.jpg")).Photo
	markReady(t, e, photo.ID)
	sharePhoto(t, anna, photo.ID, map[string]any{"visibility": "all"})

	check := func(c *browser, wantLocation bool) {
		t.Helper()
		body, _ := io.ReadAll(c.get("/api/v1/photos/" + itoa(photo.ID)).Body)
		if !strings.Contains(string(body), `"model":"Canon EOS R5"`) || !strings.Contains(string(body), `"fNumber":2.8`) {
			t.Errorf("the camera details are missing: %s", body)
		}
		if strings.Contains(string(body), "ABC123") || strings.Contains(string(body), "GPS") {
			t.Errorf("the API exposes raw EXIF: %s", body)
		}
		if got := strings.Contains(string(body), `"location":{"latitude":59.93,"longitude":30.33}`); got != wantLocation {
			t.Errorf("location shown = %v, want %v: %s", got, wantLocation, body)
		}
		page, _ := io.ReadAll(c.get("/api/v1/photos/shared").Body)
		if c == boris && strings.Contains(string(page), "59.93") != wantLocation {
			t.Errorf("location shown in a list = %v, want %v: %s", !wantLocation, wantLocation, page)
		}
	}

	check(anna, false)
	check(boris, false)
	wantStatus(t, admin.json(http.MethodPut, "/api/v1/settings/metadata", map[string]any{"showLocation": true}), http.StatusOK)
	check(anna, true)
	check(boris, true)
}

func TestPreviewNotReady(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	photo := decode[uploadResult](t, anna.upload("landscape.jpg")).Photo
	wantStatus(t, anna.get("/api/v1/photos/"+itoa(photo.ID)+"/previews/grid"), http.StatusNotFound)
	wantStatus(t, anna.get("/api/v1/photos/"+itoa(photo.ID)+"/previews/other"), http.StatusNotFound)
}

func TestPreviewHeaders(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	photo := decode[uploadResult](t, anna.upload("landscape.jpg")).Photo
	markReady(t, e, photo.ID)

	res := anna.get("/api/v1/photos/" + itoa(photo.ID) + "/previews/grid")
	if got := res.Header.Get("Content-Type"); got != "image/webp" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := res.Header.Get("Cache-Control"); !strings.HasPrefix(got, "private") {
		t.Errorf("Cache-Control = %q, want it private", got)
	}

	res = anna.get("/api/v1/photos/" + itoa(photo.ID) + "/original")
	if got := res.Header.Get("Content-Disposition"); !strings.Contains(got, "landscape.jpg") {
		t.Errorf("Content-Disposition = %q", got)
	}
}

type timeline struct {
	Items      []photoJSON `json:"items"`
	NextCursor *string     `json:"nextCursor"`
}

func TestTimelinePagination(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")

	fixtures := []string{"landscape.jpg", "landscape.png", "landscape.webp", "landscape.heic", "large.jpg"}
	for _, f := range fixtures {
		wantStatus(t, anna.upload(f), http.StatusCreated)
	}

	var seen []int64
	path := "/api/v1/photos?limit=2"
	for range len(fixtures) + 1 {
		page := decode[timeline](t, anna.get(path))
		for _, p := range page.Items {
			seen = append(seen, p.ID)
		}
		if page.NextCursor == nil {
			break
		}
		path = "/api/v1/photos?limit=2&cursor=" + *page.NextCursor
	}
	if len(seen) != len(fixtures) {
		t.Fatalf("got %d photos, want %d: %v", len(seen), len(fixtures), seen)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] >= seen[i-1] {
			t.Errorf("photos are not newest first: %v", seen)
		}
	}

	wantStatus(t, anna.get("/api/v1/photos?limit=0"), http.StatusBadRequest)
	wantStatus(t, anna.get("/api/v1/photos?cursor=nonsense"), http.StatusBadRequest)
}

func TestPhotoEvents(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream := func(b *browser) *http.Response {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.server.URL+"/api/v1/photos/events", nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := b.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}
	annaEvents, borisEvents := stream(anna), stream(boris)
	if got := annaEvents.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q", got)
	}

	photo := decode[uploadResult](t, anna.upload("landscape.jpg")).Photo

	type event struct {
		Pending int64       `json:"pending"`
		Photos  []photoJSON `json:"photos"`
	}
	read := func(res *http.Response, found func(event) bool) bool {
		decoder := newEventReader(res.Body)
		for {
			data, err := decoder.next()
			if err != nil {
				return false
			}
			var ev event
			if err := json.Unmarshal(data, &ev); err != nil {
				t.Error(err)
				return false
			}
			if found(ev) {
				return true
			}
		}
	}
	saw := read(annaEvents, func(ev event) bool {
		for _, p := range ev.Photos {
			if p.ID == photo.ID && ev.Pending == 1 {
				return true
			}
		}
		return false
	})
	if !saw {
		t.Error("anna did not get an event about her photo")
	}

	// Boris gets his own first event and must never see Anna's photo in it.
	data, err := newEventReader(borisEvents.Body).next()
	if err != nil {
		t.Fatal(err)
	}
	var first event
	if err := json.Unmarshal(data, &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Photos) != 0 || first.Pending != 0 {
		t.Errorf("boris got %+v", first)
	}
}

type eventReader struct {
	r   io.Reader
	buf []byte
}

func newEventReader(r io.Reader) *eventReader { return &eventReader{r: r} }

// next returns the data of the next event, skipping comments.
func (e *eventReader) next() ([]byte, error) {
	chunk := make([]byte, 4096)
	for {
		if end := bytes.Index(e.buf, []byte("\n\n")); end >= 0 {
			block := e.buf[:end]
			e.buf = e.buf[end+2:]
			for line := range bytes.SplitSeq(block, []byte("\n")) {
				if data, ok := bytes.CutPrefix(line, []byte("data: ")); ok {
					return data, nil
				}
			}
			continue
		}
		n, err := e.r.Read(chunk)
		if err != nil {
			return nil, err
		}
		e.buf = append(e.buf, chunk[:n]...)
	}
}

func TestAppPasswords(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna := e.loggedIn("anna")
	boris := e.loggedIn("boris")

	wantStatus(t, anna.json(http.MethodPost, "/api/v1/me/app-passwords", map[string]string{"name": " "}), http.StatusBadRequest)

	res := anna.json(http.MethodPost, "/api/v1/me/app-passwords", map[string]string{"name": "iPhone"})
	wantStatus(t, res, http.StatusCreated)
	created := decode[struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}](t, res)
	if created.Name != "iPhone" || created.Password == "" {
		t.Fatalf("created = %+v", created)
	}
	stored, err := e.queries.GetAppPasswordUser(context.Background(), auth.HashAppPassword(created.Password))
	if err != nil || stored.User.Login != "anna" {
		t.Fatalf("the password does not log in as anna: %v", err)
	}

	res = anna.get("/api/v1/me/app-passwords")
	wantStatus(t, res, http.StatusOK)
	listed := decode[[]map[string]any](t, res)
	if len(listed) != 1 || listed[0]["name"] != "iPhone" {
		t.Fatalf("listed = %v", listed)
	}
	if _, ok := listed[0]["password"]; ok {
		t.Error("the list shows the password")
	}

	res = boris.get("/api/v1/me/app-passwords")
	if listed := decode[[]map[string]any](t, res); len(listed) != 0 {
		t.Errorf("boris sees anna's app passwords: %v", listed)
	}
	wantStatus(t, boris.json(http.MethodDelete, "/api/v1/me/app-passwords/"+itoa(created.ID), nil), http.StatusNotFound)
	wantStatus(t, anna.json(http.MethodDelete, "/api/v1/me/app-passwords/"+itoa(created.ID), nil), http.StatusNoContent)
	if _, err := e.queries.GetAppPasswordUser(context.Background(), auth.HashAppPassword(created.Password)); err == nil {
		t.Error("a deleted app password still logs in")
	}
}

func TestWebDAVAlbums(t *testing.T) {
	e := setup(t)
	anna := e.addUser("anna", auth.RoleUser)
	boris := e.addUser("boris", auth.RoleUser)
	annaB := e.loggedIn("anna")
	borisB := e.loggedIn("boris")
	ctx := context.Background()
	album, err := e.queries.CreateAlbum(ctx, store.CreateAlbumParams{OwnerID: anna.ID, Name: "Camera"})
	if err != nil {
		t.Fatal(err)
	}
	borisAlbum, err := e.queries.CreateAlbum(ctx, store.CreateAlbumParams{OwnerID: boris.ID, Name: "Boris"})
	if err != nil {
		t.Fatal(err)
	}
	err = e.queries.CreateWebDAVDir(ctx, store.CreateWebDAVDirParams{UserID: anna.ID, Path: "/DCIM", Parent: "/"})
	if err != nil {
		t.Fatal(err)
	}

	res := annaB.get("/api/v1/me/webdav-folders")
	wantStatus(t, res, http.StatusOK)
	if folders := decode[[]string](t, res); len(folders) != 1 || folders[0] != "/DCIM" {
		t.Errorf("folders = %v", folders)
	}
	if folders := decode[[]string](t, borisB.get("/api/v1/me/webdav-folders")); len(folders) != 0 {
		t.Errorf("boris sees anna's folders: %v", folders)
	}

	link := func(b *browser, path string, albumID int64) *http.Response {
		return b.json(http.MethodPost, "/api/v1/me/webdav-albums", map[string]any{"path": path, "albumId": albumID})
	}
	res = link(annaB, " DCIM/Camera/ ", album)
	wantStatus(t, res, http.StatusCreated)
	created := decode[struct {
		ID        int64  `json:"id"`
		Path      string `json:"path"`
		AlbumID   int64  `json:"albumId"`
		AlbumName string `json:"albumName"`
	}](t, res)
	if created.Path != "/DCIM/Camera" || created.AlbumID != album || created.AlbumName != "Camera" {
		t.Errorf("created = %+v", created)
	}
	wantStatus(t, link(annaB, "/DCIM/Camera", album), http.StatusConflict)
	wantStatus(t, link(annaB, "/", album), http.StatusCreated)
	wantStatus(t, link(annaB, "/Other", borisAlbum), http.StatusNotFound)
	wantStatus(t, link(annaB, "/bad\x01name", album), http.StatusBadRequest)

	res = annaB.get("/api/v1/me/webdav-albums")
	wantStatus(t, res, http.StatusOK)
	if listed := decode[[]map[string]any](t, res); len(listed) != 2 || listed[0]["path"] != "/" || listed[1]["path"] != "/DCIM/Camera" {
		t.Errorf("listed = %v", listed)
	}
	if listed := decode[[]map[string]any](t, borisB.get("/api/v1/me/webdav-albums")); len(listed) != 0 {
		t.Errorf("boris sees anna's links: %v", listed)
	}

	wantStatus(t, borisB.json(http.MethodDelete, "/api/v1/me/webdav-albums/"+itoa(created.ID), nil), http.StatusNotFound)
	wantStatus(t, annaB.json(http.MethodDelete, "/api/v1/me/webdav-albums/"+itoa(created.ID), nil), http.StatusNoContent)

	// Deleting the album removes its links.
	if _, err := e.queries.DeleteAlbum(ctx, store.DeleteAlbumParams{ID: album, OwnerID: anna.ID}); err != nil {
		t.Fatal(err)
	}
	if listed := decode[[]map[string]any](t, annaB.get("/api/v1/me/webdav-albums")); len(listed) != 0 {
		t.Errorf("links of a deleted album: %v", listed)
	}
}
