package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/magellanica-app/magellanica-photos/backend/internal/auth"
)

// The owner sets a photo's description, cleaned as it is stored; everyone
// who can see the photo sees it, and nobody else may change it.
func TestPhotoDescription(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	boris := e.addUser("boris", auth.RoleUser)
	anna, borisBrowser := e.loggedIn("anna"), e.loggedIn("boris")

	photo := anna.addPhoto("landscape.jpg")
	path := "/api/v1/photos/" + itoa(photo)
	if got := decode[photoJSON](t, anna.get(path)); got.Description != "" {
		t.Fatalf("description of a new photo = %q", got.Description)
	}

	res := anna.json(http.MethodPatch, path, map[string]string{"description": "  Море\r\nи\x00горы \n"})
	wantStatus(t, res, http.StatusOK)
	if got := decode[photoJSON](t, res); got.Description != "Море\nи горы" {
		t.Errorf("description = %q", got.Description)
	}
	wantStatus(t, anna.json(http.MethodPatch, path, map[string]string{"description": strings.Repeat("я", 2001)}), http.StatusBadRequest)
	wantStatus(t, anna.json(http.MethodPatch, path, map[string]any{}), http.StatusBadRequest)

	// Shared, the description goes with the photo, but only the owner edits it.
	sharePhoto(t, anna, photo, map[string]any{"add": []int64{boris.ID}})
	if got := decode[photoJSON](t, borisBrowser.get(path)); got.Description != "Море\nи горы" {
		t.Errorf("description seen by another user = %q", got.Description)
	}
	wantStatus(t, borisBrowser.json(http.MethodPatch, path, map[string]string{"description": "Чужое"}), http.StatusNotFound)

	// An empty one clears it.
	res = anna.json(http.MethodPatch, path, map[string]string{"description": " "})
	wantStatus(t, res, http.StatusOK)
	if got := decode[photoJSON](t, res); got.Description != "" {
		t.Errorf("description after clearing = %q", got.Description)
	}
}
