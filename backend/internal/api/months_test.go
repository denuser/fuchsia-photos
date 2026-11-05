package api

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

// takenAt makes a photo ready with the given shooting time, which becomes its sort time.
func takenAt(t *testing.T, e *env, id int64, at string) {
	t.Helper()
	when, err := time.Parse(time.RFC3339, at)
	if err != nil {
		t.Fatal(err)
	}
	markReady(t, e, id)
	format := "webp"
	err = e.queries.MarkPhotoReady(context.Background(), store.MarkPhotoReadyParams{ID: id, PreviewFormat: &format, TakenAt: &when})
	if err != nil {
		t.Fatal(err)
	}
}

func (b *browser) months(path string) []monthJSON {
	b.e.t.Helper()
	res := b.get(path)
	wantStatus(b.e.t, res, http.StatusOK)
	return decode[[]monthJSON](b.e.t, res)
}

func wantMonths(t *testing.T, got []monthJSON, want ...monthJSON) {
	t.Helper()
	if want == nil {
		want = []monthJSON{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("months = %v, want %v", got, want)
	}
}

func TestPhotoMonths(t *testing.T) {
	s := setupSharing(t)
	lastOfJanuary := s.anna.addPhoto("landscape.jpg")
	takenAt(t, s.e, lastOfJanuary, "2024-01-31T23:30:00Z")
	february := s.anna.addPhoto("landscape.png")
	takenAt(t, s.e, february, "2024-02-10T12:00:00Z")
	march := s.anna.addPhoto("landscape.webp")
	takenAt(t, s.e, march, "2024-03-05T12:00:00Z")

	t.Run("counts months newest first in the given time zone", func(t *testing.T) {
		wantMonths(t, s.anna.months("/api/v1/photos/months?tz=UTC"),
			monthJSON{"2024-03", 1}, monthJSON{"2024-02", 1}, monthJSON{"2024-01", 1})
		// Half past eleven in the evening in UTC is already February in Moscow.
		wantMonths(t, s.anna.months("/api/v1/photos/months?tz=Europe/Moscow"),
			monthJSON{"2024-03", 1}, monthJSON{"2024-02", 2})
	})

	t.Run("applies the filters of the list", func(t *testing.T) {
		wantMonths(t, s.anna.months("/api/v1/photos/months?tz=UTC&from=2024-02-01T00:00:00Z&to=2024-03-01T00:00:00Z"),
			monthJSON{"2024-02", 1})
		album := s.anna.newAlbum(t, "Trip", march)
		wantMonths(t, s.anna.months("/api/v1/photos/months?tz=UTC&album="+itoa(album)), monthJSON{"2024-03", 1})
		wantMonths(t, s.boris.months("/api/v1/photos/months?tz=UTC"))
	})

	t.Run("needs a known time zone", func(t *testing.T) {
		for _, path := range []string{
			"/api/v1/photos/months",
			"/api/v1/photos/months?tz=Mars/Olympus",
			"/api/v1/photos/months?tz=Local",
			"/api/v1/photos/shared/months?tz=nowhere",
			"/api/v1/photos/all/months",
		} {
			wantStatus(t, s.root.get(path), http.StatusBadRequest)
		}
	})

	t.Run("counts shared photos", func(t *testing.T) {
		sharePhoto(t, s.anna, february, map[string]any{"add": []int64{s.borisID}})
		wantMonths(t, s.boris.months("/api/v1/photos/shared/months?tz=UTC"), monthJSON{"2024-02", 1})
		wantMonths(t, s.vera.months("/api/v1/photos/shared/months?tz=UTC"))
		wantIDs(t, s.boris.photoIDs("/api/v1/photos/shared?from=2024-02-01T00:00:00Z&to=2024-03-01T00:00:00Z"), february)
		wantIDs(t, s.boris.photoIDs("/api/v1/photos/shared?from=2024-03-01T00:00:00Z"))
	})

	t.Run("counts all photos for an administrator", func(t *testing.T) {
		borises := s.boris.addPhoto("landscape.jpg")
		takenAt(t, s.e, borises, "2023-12-24T18:00:00Z")
		wantMonths(t, s.root.months("/api/v1/photos/all/months?tz=UTC"),
			monthJSON{"2024-03", 1}, monthJSON{"2024-02", 1}, monthJSON{"2024-01", 1}, monthJSON{"2023-12", 1})
		wantMonths(t, s.root.months("/api/v1/photos/all/months?tz=UTC&owner="+itoa(s.borisID)), monthJSON{"2023-12", 1})
		wantIDs(t, s.root.photoIDs("/api/v1/photos/all?from=2024-01-01T00:00:00Z&to=2024-03-01T00:00:00Z"), february, lastOfJanuary)
		wantStatus(t, s.anna.get("/api/v1/photos/all/months?tz=UTC"), http.StatusForbidden)
	})
}

func TestNavigationHiddenOnPhones(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")

	if !decode[userJSON](t, anna.get("/api/v1/me")).NavigationHiddenOnPhones {
		t.Fatal("the navigation is shown on phones by default")
	}
	res := anna.json(http.MethodPatch, "/api/v1/me", map[string]bool{"navigationHiddenOnPhones": false})
	wantStatus(t, res, http.StatusOK)
	if decode[userJSON](t, anna.get("/api/v1/me")).NavigationHiddenOnPhones {
		t.Error("the setting was not saved")
	}
}

func TestGridSquares(t *testing.T) {
	e := setup(t)
	e.addUser("root", auth.RoleAdmin)
	e.addUser("anna", auth.RoleUser)
	root, anna := e.loggedIn("root"), e.loggedIn("anna")

	if decode[userJSON](t, anna.get("/api/v1/me")).GridSquares {
		t.Fatal("square tiles are on by default")
	}
	res := anna.json(http.MethodPatch, "/api/v1/me", map[string]bool{"gridSquares": true})
	wantStatus(t, res, http.StatusOK)
	if !decode[userJSON](t, anna.get("/api/v1/me")).GridSquares {
		t.Error("the setting was not saved")
	}
	if decode[userJSON](t, root.get("/api/v1/me")).GridSquares {
		t.Error("the setting changed another user")
	}

	// Any user can read the grid preview size; it follows the preview settings.
	if got := decode[gridSettingsJSON](t, anna.get("/api/v1/settings/grid")).GridEdge; got != 400 {
		t.Errorf("grid edge = %d, want 400", got)
	}
	settings := decode[previewSettingsJSON](t, root.get("/api/v1/settings/previews"))
	settings.GridEdge = 300
	wantStatus(t, root.json(http.MethodPut, "/api/v1/settings/previews", settings), http.StatusOK)
	if got := decode[gridSettingsJSON](t, anna.get("/api/v1/settings/grid")).GridEdge; got != 300 {
		t.Errorf("grid edge = %d, want 300", got)
	}
}
