package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/magellanica-app/magellanica-photos/backend/internal/ingest"
	"github.com/magellanica-app/magellanica-photos/backend/internal/library"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

func TestLibraryFolders(t *testing.T) {
	folders := t.TempDir()
	for _, dir := range []string{"lightroom/2026", "lightroom/2025", "scans", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(folders, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	e := setupWith(t, func(d *Deps) {
		d.Layout.Folders = folders
		d.Library = &library.Service{Ingest: &ingest.Service{Pool: d.Pool, Jobs: d.Ingest.Jobs, Layout: d.Layout}}
	})
	admin := e.addUser("admin", "admin")
	e.addUser("anna", "user")
	ctx := context.Background()

	// Only an administrator may see or change them.
	anna := e.loggedIn("anna")
	wantStatus(t, anna.get("/api/v1/library-folders"), http.StatusForbidden)
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/library-folders", map[string]any{"path": "scans", "ownerId": admin.ID}), http.StatusForbidden)

	root := e.loggedIn("admin")
	res := root.get("/api/v1/library-folders/available")
	wantStatus(t, res, http.StatusOK)
	if got := decode[[]string](t, res); len(got) != 4 {
		t.Errorf("available = %v, want 4 folders and no hidden one", got)
	}

	bad := []map[string]any{
		{"path": "", "ownerId": admin.ID},
		{"path": "../etc", "ownerId": admin.ID},
		{"path": "missing", "ownerId": admin.ID},
		{"path": "scans", "ownerId": 9999},
		{"path": "scans", "ownerId": admin.ID, "scanInterval": 5},
	}
	for _, body := range bad {
		wantStatus(t, root.json(http.MethodPost, "/api/v1/library-folders", body), http.StatusBadRequest)
	}

	res = root.json(http.MethodPost, "/api/v1/library-folders", map[string]any{"path": "/lightroom/", "ownerId": admin.ID})
	wantStatus(t, res, http.StatusCreated)
	created := decode[libraryFolderJSON](t, res)
	if created.Path != "lightroom" || created.ScanInterval != 600 || created.OwnerID != admin.ID {
		t.Errorf("created = %+v", created)
	}
	var queued int
	e.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind = 'library_scan'`).Scan(&queued)
	if queued != 1 {
		t.Errorf("%d scans queued after creating a folder, want 1", queued)
	}

	// A folder inside, around or equal to a library folder is refused.
	for _, p := range []string{"lightroom", "lightroom/2026"} {
		wantStatus(t, root.json(http.MethodPost, "/api/v1/library-folders", map[string]any{"path": p, "ownerId": admin.ID}), http.StatusConflict)
	}
	res = root.get("/api/v1/library-folders/available")
	if got := decode[[]string](t, res); len(got) != 1 || got[0] != "scans" {
		t.Errorf("available after adding one = %v, want only scans", got)
	}

	path := "/api/v1/library-folders/" + strconv.FormatInt(created.ID, 10)
	wantStatus(t, root.json(http.MethodPatch, path, map[string]any{"scanInterval": 30}), http.StatusBadRequest)
	res = root.json(http.MethodPatch, path, map[string]any{"scanInterval": 3600})
	wantStatus(t, res, http.StatusOK)
	if got := decode[libraryFolderJSON](t, res); got.ScanInterval != 3600 {
		t.Errorf("interval = %d", got.ScanInterval)
	}

	// Scan now does not queue a second scan while one waits.
	wantStatus(t, root.json(http.MethodPost, path+"/scan", nil), http.StatusAccepted)
	e.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind = 'library_scan'`).Scan(&queued)
	if queued != 1 {
		t.Errorf("%d scans queued, want the waiting one only", queued)
	}
	wantStatus(t, root.json(http.MethodPost, "/api/v1/library-folders/9999/scan", nil), http.StatusNotFound)

	// With photos it stays; without, it goes.
	res = root.upload("landscape.jpg")
	wantStatus(t, res, http.StatusCreated)
	photoID := decode[uploadResult](t, res).Photo.ID
	row, err := e.queries.InsertPhotoFile(ctx, store.InsertPhotoFileParams{
		FolderID: created.ID, Path: "a.jpg", PhotoID: &photoID, State: "source", Size: 1, Mtime: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, root.get(path+"/files?kind=bogus"), http.StatusBadRequest)
	wantStatus(t, root.get("/api/v1/library-folders/9999/files?kind=problems"), http.StatusNotFound)
	if _, err := e.queries.InsertPhotoFile(ctx, store.InsertPhotoFileParams{
		FolderID: created.ID, Path: "scan.psd", State: "unsupported", Size: 1, Mtime: time.Now(), SupportVersion: 1,
	}); err != nil {
		t.Fatal(err)
	}
	res = root.get(path + "/files?kind=unsupported")
	wantStatus(t, res, http.StatusOK)
	if got := decode[[]libraryFileJSON](t, res); len(got) != 1 || got[0].Path != "scan.psd" || got[0].State != "unsupported" {
		t.Errorf("unsupported = %+v", got)
	}
	wantStatus(t, root.json(http.MethodDelete, path, nil), http.StatusConflict)
	res = root.get("/api/v1/library-folders")
	wantStatus(t, res, http.StatusOK)
	if got := decode[[]libraryFolderJSON](t, res); len(got) != 1 || got[0].Photos != 1 || got[0].Missing != 0 {
		t.Errorf("list = %+v, want one folder with one photo", got)
	}
	if err := e.queries.DeletePhotoFiles(ctx, []int64{row.ID}); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, root.json(http.MethodDelete, path, nil), http.StatusNoContent)
	wantStatus(t, root.json(http.MethodDelete, path, nil), http.StatusNotFound)
}
