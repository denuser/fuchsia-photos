package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// A photo with a source path is read from its library folder and is never
// removed from it, not even by a permanent delete.
func TestPhotoFromLibraryFolder(t *testing.T) {
	folders := t.TempDir()
	e := setupWith(t, func(d *Deps) { d.Layout.Folders = folders })
	e.addUser("anna", "user")
	anna := e.loggedIn("anna")
	ctx := context.Background()

	res := anna.upload("landscape.jpg")
	wantStatus(t, res, http.StatusCreated)
	id := decode[uploadResult](t, res).Photo.ID

	// The folder's file is told apart from the library's copy by its content.
	const marker = "bytes in the library folder"
	rel := "lightroom/2026/IMG_0001.jpg"
	if err := os.MkdirAll(filepath.Join(folders, filepath.Dir(rel)), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folders, rel), []byte(marker), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE photos SET source_path = $2 WHERE id = $1`, id, rel); err != nil {
		t.Fatal(err)
	}

	res = anna.get(fmt.Sprintf("/api/v1/photos/%d/original", id))
	wantStatus(t, res, http.StatusOK)
	if body, _ := io.ReadAll(res.Body); string(body) != marker {
		t.Errorf("original = %q, want the folder's file", body)
	}

	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(id)), http.StatusNoContent)
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash/empty", nil), http.StatusNoContent)
	photo, err := e.queries.GetPhoto(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if photo.PurgedAt == nil {
		t.Error("the photo was not purged")
	}
	if _, err := os.Stat(filepath.Join(folders, rel)); err != nil {
		t.Errorf("a permanent delete touched the folder's file: %v", err)
	}
}
