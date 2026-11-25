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
	"time"

	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
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

// A photo the scan put in the trash for a missing file can be uploaded again: the upload
// becomes its original in the library, and the photo works.
func TestUploadOfAPhotoWhoseLibraryFileIsAway(t *testing.T) {
	folders := t.TempDir()
	e := setupWith(t, func(d *Deps) { d.Layout.Folders = folders })
	e.addUser("anna", "user")
	anna := e.loggedIn("anna")
	ctx := context.Background()

	res := anna.upload("landscape.jpg")
	wantStatus(t, res, http.StatusCreated)
	id := decode[uploadResult](t, res).Photo.ID
	photo, _ := e.queries.GetPhoto(ctx, id)
	if err := os.Remove(e.layout.Original(photo.OwnerID, photo.Sha256, photo.Mime)); err != nil {
		t.Fatal(err)
	}
	// As the scan left it: read from a folder, its file away, in the trash.
	folder, err := e.queries.CreateLibraryFolder(ctx, store.CreateLibraryFolderParams{Path: "lightroom", OwnerID: photo.OwnerID, ScanInterval: 600})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE photos SET source_path = 'lightroom/a.jpg' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.queries.InsertPhotoFile(ctx, store.InsertPhotoFileParams{FolderID: folder.ID, Path: "a.jpg", PhotoID: &id, State: "source", Size: 1, Mtime: time.Now()}); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(id)), http.StatusNoContent)
	wantStatus(t, anna.get(fmt.Sprintf("/api/v1/photos/%d/original", id)), http.StatusNotFound)

	// While no scan has found the file away, as when a share is not mounted for a while, an
	// upload only restores the photo: the file may come back and be edited.
	res = anna.upload("landscape.jpg")
	wantStatus(t, res, http.StatusOK)
	if kept, _ := e.queries.GetPhoto(ctx, id); kept.SourcePath == nil {
		t.Fatal("an upload during an outage took the photo out of its folder")
	}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(id)), http.StatusNoContent)

	// Once two scans did not find it, the upload becomes the original.
	if _, err := e.pool.Exec(ctx, `UPDATE photo_files SET missing_since = now()`); err != nil {
		t.Fatal(err)
	}
	res = anna.upload("landscape.jpg")
	wantStatus(t, res, http.StatusOK)
	if got := decode[uploadResult](t, res); !got.Duplicate || got.Photo.ID != id || !got.Restored {
		t.Fatalf("upload = %+v, want the same photo restored", got)
	}
	wantStatus(t, anna.get(fmt.Sprintf("/api/v1/photos/%d/original", id)), http.StatusOK)
	after, _ := e.queries.GetPhoto(ctx, id)
	if after.SourcePath != nil || after.DeletedAt != nil {
		t.Errorf("source path = %v, deleted = %v; want the photo kept in the library and out of the trash", after.SourcePath, after.DeletedAt)
	}
	if files, _ := e.queries.ListPhotoFiles(ctx, folder.ID); len(files) != 0 {
		t.Errorf("rows of the folder = %+v, want none: the file's row ended with its source", files)
	}
}
