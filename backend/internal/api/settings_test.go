package api

import (
	"net/http"
	"testing"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
)

func TestPreviewSettings(t *testing.T) {
	e := setup(t)
	e.addUser("root", auth.RoleAdmin)
	e.addUser("anna", auth.RoleUser)
	admin, user := e.loggedIn("root"), e.loggedIn("anna")
	const path = "/api/v1/settings/previews"

	t.Run("only an administrator can use it", func(t *testing.T) {
		wantStatus(t, user.get(path), http.StatusForbidden)
		wantStatus(t, user.json(http.MethodPut, path, map[string]any{"format": "jpeg"}), http.StatusForbidden)
	})

	defaults := previewSettingsJSON{Format: "webp", WebPQuality: 75, WebPEffort: 2, JPEGQuality: 80, GridEdge: 400, ViewerEdge: 2048}

	t.Run("a new database starts with the defaults", func(t *testing.T) {
		if got := decode[previewSettingsJSON](t, admin.get(path)); got != defaults {
			t.Errorf("settings = %+v, want %+v", got, defaults)
		}
	})

	t.Run("invalid settings are rejected and change nothing", func(t *testing.T) {
		invalid := defaults
		invalid.GridEdge = 100
		wantStatus(t, admin.json(http.MethodPut, path, invalid), http.StatusBadRequest)
		wantStatus(t, admin.json(http.MethodPut, path, map[string]any{"format": "jpeg"}), http.StatusBadRequest)
		if got := decode[previewSettingsJSON](t, admin.get(path)); got != defaults {
			t.Errorf("settings = %+v, want %+v", got, defaults)
		}
	})

	t.Run("saved settings are returned", func(t *testing.T) {
		want := previewSettingsJSON{Format: "jpeg", WebPQuality: 60, WebPEffort: 4, JPEGQuality: 90, GridEdge: 500, ViewerEdge: 3000}
		if got := decode[previewSettingsJSON](t, admin.json(http.MethodPut, path, want)); got != want {
			t.Errorf("response = %+v, want %+v", got, want)
		}
		if got := decode[previewSettingsJSON](t, admin.get(path)); got != want {
			t.Errorf("settings = %+v, want %+v", got, want)
		}
	})
}

func TestMetadataSettings(t *testing.T) {
	e := setup(t)
	e.addUser("root", auth.RoleAdmin)
	e.addUser("anna", auth.RoleUser)
	admin, user := e.loggedIn("root"), e.loggedIn("anna")
	const path = "/api/v1/settings/metadata"

	wantStatus(t, user.get(path), http.StatusForbidden)
	wantStatus(t, user.json(http.MethodPut, path, map[string]any{"showLocation": true}), http.StatusForbidden)

	if got := decode[metadataSettingsJSON](t, admin.get(path)); got.ShowLocation {
		t.Error("locations are shown by default")
	}
	wantStatus(t, admin.json(http.MethodPut, path, map[string]any{}), http.StatusBadRequest)
	if got := decode[metadataSettingsJSON](t, admin.json(http.MethodPut, path, map[string]any{"showLocation": true})); !got.ShowLocation {
		t.Error("the response does not show the saved setting")
	}
	if got := decode[metadataSettingsJSON](t, admin.get(path)); !got.ShowLocation {
		t.Error("the setting was not saved")
	}
}
