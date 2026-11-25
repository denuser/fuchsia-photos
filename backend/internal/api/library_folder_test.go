package api

import (
	"bytes"
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

// Whoever can write to the share may replace an indexed file with a link to a file of the
// application or of another user. Nothing may follow it.
func TestLibraryFolderFileSwappedForALink(t *testing.T) {
	folders := t.TempDir()
	e := setupWith(t, func(d *Deps) { d.Layout.Folders = folders })
	e.addUser("anna", "user")
	e.addUser("ben", "user")
	anna, ben := e.loggedIn("anna"), e.loggedIn("ben")
	ctx := context.Background()

	// Ben's private photo, which Anna must never read.
	res := ben.upload("landscape.jpg")
	wantStatus(t, res, http.StatusCreated)
	bens := decode[uploadResult](t, res).Photo
	benPhoto, err := e.queries.GetPhoto(ctx, bens.ID)
	if err != nil {
		t.Fatal(err)
	}
	secret := e.layout.Original(benPhoto.OwnerID, benPhoto.Sha256, benPhoto.Mime)

	res = anna.upload("rotated.jpg")
	wantStatus(t, res, http.StatusCreated)
	id := decode[uploadResult](t, res).Photo.ID
	rel := "lightroom/IMG_0001.jpg"
	if err := os.MkdirAll(filepath.Join(folders, "lightroom"), 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(folders, rel)
	data, _ := os.ReadFile(filepath.Join("..", "media", "testdata", "rotated.jpg"))
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE photos SET source_path = $2 WHERE id = $1`, id, rel); err != nil {
		t.Fatal(err)
	}
	original := fmt.Sprintf("/api/v1/photos/%d/original", id)
	wantStatus(t, anna.get(original), http.StatusOK)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, path); err != nil {
		t.Fatal(err)
	}
	res = anna.get(original)
	wantStatus(t, res, http.StatusNotFound)
	if body, _ := io.ReadAll(res.Body); bytes.Contains(body, data[:16]) || len(body) > 200 {
		t.Errorf("the link was followed: %d bytes", len(body))
	}
	res = anna.get(fmt.Sprintf("/api/v1/photos/%d", id))
	wantStatus(t, res, http.StatusOK)
	if got := decode[photoJSON](t, res); got.FileMissing == nil || !*got.FileMissing {
		t.Errorf("fileMissing = %v, want true for a file swapped for a link", got.FileMissing)
	}
}
