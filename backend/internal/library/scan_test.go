package library

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/magellanica-app/magellanica-photos/backend/internal/ingest"
	"github.com/magellanica-app/magellanica-photos/backend/internal/jobs"
	"github.com/magellanica-app/magellanica-photos/backend/internal/storage"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
	"github.com/magellanica-app/magellanica-photos/backend/internal/testdb"
)

const fixtures = "../media/testdata"

type env struct {
	t       *testing.T
	service *Service
	queries *store.Queries
	layout  storage.Layout
	folder  store.LibraryFolder
	owner   int64
}

// setup queues jobs without running them: processing is covered by the jobs package.
func setup(t *testing.T) *env {
	t.Helper()
	pool := testdb.New(t)
	layout := storage.Layout{Library: t.TempDir(), Cache: t.TempDir(), Folders: t.TempDir()}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	client, err := jobs.NewInsertClient(pool)
	if err != nil {
		t.Fatal(err)
	}
	queries := store.New(pool)
	ctx := context.Background()
	user, err := queries.CreateUser(ctx, store.CreateUserParams{Login: "anna", PasswordHash: "x", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	folder, err := queries.CreateLibraryFolder(ctx, store.CreateLibraryFolderParams{Path: "lightroom", OwnerID: user.ID, ScanInterval: 600})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(layout.Folders, "lightroom"), 0o750); err != nil {
		t.Fatal(err)
	}
	service := &Service{Ingest: &ingest.Service{Pool: pool, Jobs: client, Layout: layout}}
	return &env{t, service, queries, layout, folder, user.ID}
}

func (e *env) put(rel, fixture string) {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtures, fixture))
	if err != nil {
		e.t.Fatal(err)
	}
	e.putBytes(rel, data)
}

func (e *env) putBytes(rel string, data []byte) {
	e.t.Helper()
	p := filepath.Join(e.layout.Folders, "lightroom", rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o640); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) scan() Result {
	e.t.Helper()
	res, err := e.service.Scan(context.Background(), e.folder.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return res
}

func TestScan(t *testing.T) {
	e := setup(t)
	e.put("a/IMG_0001.jpg", "landscape.jpg")
	e.put("IMG_0002.jpg", "rotated.jpg")
	e.putBytes(".hidden.jpg", []byte("x"))
	e.putBytes("notes.txt", []byte("not a photo"))
	e.putBytes("IMG_0001.xmp", []byte("<x/>"))

	res := e.scan()
	if res.Added != 2 || res.Unsupported != 1 || res.Listed != 3 {
		t.Fatalf("first scan = %+v, want 2 added, 1 unsupported of 3 listed", res)
	}
	files, err := e.queries.ListPhotoFiles(context.Background(), e.folder.ID)
	if err != nil || len(files) != 2 {
		t.Fatalf("known files = %d, %v; want 2", len(files), err)
	}
	for _, f := range files {
		photo, err := e.queries.GetPhoto(context.Background(), *f.PhotoID)
		if err != nil {
			t.Fatal(err)
		}
		if photo.SourcePath == nil || *photo.SourcePath != "lightroom/"+f.Path {
			t.Errorf("source path = %v, want lightroom/%s", photo.SourcePath, f.Path)
		}
		if photo.Status != "pending" {
			t.Errorf("status = %s, want pending", photo.Status)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(e.layout.Library, "originals")); len(entries) != 0 {
		t.Errorf("the library holds %d owner folders, want none: nothing is copied", len(entries))
	}

	// A second scan reads no file: the known ones are unchanged, the unsupported one is looked at again.
	res = e.scan()
	if res.Added != 0 || res.Changed != 0 || res.Unsupported != 1 {
		t.Errorf("second scan = %+v", res)
	}
}

func TestScanDuplicates(t *testing.T) {
	e := setup(t)
	e.put("one.jpg", "landscape.jpg")
	e.put("copy of one.jpg", "landscape.jpg")
	res := e.scan()
	if res.Added != 1 || res.Duplicates != 1 {
		t.Fatalf("scan = %+v, want 1 added, 1 duplicate", res)
	}
	files, _ := e.queries.ListPhotoFiles(context.Background(), e.folder.ID)
	states := map[string]int{}
	for _, f := range files {
		states[f.State]++
	}
	if states["source"] != 1 || states["duplicate"] != 1 {
		t.Errorf("states = %v, want one source and one duplicate", states)
	}
	if res := e.scan(); res.Added != 0 || res.Duplicates != 0 {
		t.Errorf("rescan = %+v, want nothing new", res)
	}

	// A photo that is in the library already is never re-pointed to the folder.
	data, _ := os.ReadFile(filepath.Join(fixtures, "rotated.jpg"))
	uploaded, err := e.service.Ingest.Ingest(context.Background(), e.owner, bytes.NewReader(data), "rotated.jpg", ingest.PriorityInteractive)
	if err != nil {
		t.Fatal(err)
	}
	e.put("rotated in folder.jpg", "rotated.jpg")
	if res := e.scan(); res.Added != 0 || res.Duplicates != 1 {
		t.Errorf("scan = %+v, want the library photo's twin as a duplicate", res)
	}
	got, _ := e.queries.GetPhoto(context.Background(), uploaded.Photo.ID)
	if got.SourcePath != nil {
		t.Errorf("source path = %v, want none for a photo of the library", *got.SourcePath)
	}
}

func TestScanMove(t *testing.T) {
	e := setup(t)
	e.put("old/IMG_0001.jpg", "landscape.jpg")
	e.scan()
	before, _ := e.queries.ListPhotoFiles(context.Background(), e.folder.ID)

	if err := os.MkdirAll(filepath.Join(e.layout.Folders, "lightroom", "new"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(e.layout.Folders, "lightroom", "old", "IMG_0001.jpg"), filepath.Join(e.layout.Folders, "lightroom", "new", "IMG_0001.jpg")); err != nil {
		t.Fatal(err)
	}
	res := e.scan()
	if res.Moved != 1 || res.Added != 0 || res.Missing != 0 {
		t.Fatalf("scan = %+v, want one move", res)
	}
	after, _ := e.queries.ListPhotoFiles(context.Background(), e.folder.ID)
	if len(after) != 1 || after[0].ID != before[0].ID || after[0].Path != "new/IMG_0001.jpg" {
		t.Errorf("rows = %+v, want the same row at the new path", after)
	}
	photo, _ := e.queries.GetPhoto(context.Background(), *after[0].PhotoID)
	if photo.SourcePath == nil || *photo.SourcePath != "lightroom/new/IMG_0001.jpg" {
		t.Errorf("source path = %v", photo.SourcePath)
	}
}

func TestScanMissing(t *testing.T) {
	e := setup(t)
	e.put("a.jpg", "landscape.jpg")
	e.put("b.jpg", "rotated.jpg")
	e.scan()
	ctx := context.Background()
	photoOf := func(rel string) store.Photo {
		files, _ := e.queries.ListPhotoFiles(ctx, e.folder.ID)
		for _, f := range files {
			if f.Path == rel {
				p, err := e.queries.GetPhoto(ctx, *f.PhotoID)
				if err != nil {
					t.Fatal(err)
				}
				return p
			}
		}
		t.Fatalf("no row for %s", rel)
		return store.Photo{}
	}

	if err := os.Remove(filepath.Join(e.layout.Folders, "lightroom", "a.jpg")); err != nil {
		t.Fatal(err)
	}
	// One scan is not enough: a program may write the file again at once.
	if res := e.scan(); res.Missing != 1 || res.Trashed != 0 {
		t.Fatalf("first scan = %+v", res)
	}
	if photoOf("a.jpg").DeletedAt != nil {
		t.Fatal("the photo went to the trash after one scan")
	}
	if res := e.scan(); res.Trashed != 1 {
		t.Fatalf("second scan = %+v, want the photo trashed", res)
	}
	if photoOf("a.jpg").DeletedAt == nil {
		t.Fatal("the photo is not in the trash")
	}
	if photoOf("b.jpg").DeletedAt != nil {
		t.Fatal("a photo whose file is there went to the trash")
	}

	// Back at its path, with the same bytes: out of the trash again.
	e.put("a.jpg", "landscape.jpg")
	if res := e.scan(); res.Returned != 1 {
		t.Fatalf("scan after the file came back = %+v", res)
	}
	if photoOf("a.jpg").DeletedAt != nil {
		t.Error("the photo stayed in the trash")
	}
}

func TestScanKeepsPhotoTheOwnerTrashed(t *testing.T) {
	e := setup(t)
	e.put("a.jpg", "landscape.jpg")
	e.put("b.jpg", "rotated.jpg")
	e.scan()
	ctx := context.Background()
	files, _ := e.queries.ListPhotoFiles(ctx, e.folder.ID)
	if err := e.queries.TrashPhotos(ctx, store.TrashPhotosParams{Ids: []int64{*files[0].PhotoID}, ActorID: e.owner}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(e.layout.Folders, "lightroom", files[0].Path)); err != nil {
		t.Fatal(err)
	}
	e.scan()
	e.scan()
	e.put(files[0].Path, map[string]string{"a.jpg": "landscape.jpg", "b.jpg": "rotated.jpg"}[files[0].Path])
	e.scan()
	p, _ := e.queries.GetPhoto(ctx, *files[0].PhotoID)
	if p.DeletedAt == nil {
		t.Error("a photo the owner put in the trash came out of it when its file came back")
	}
}

func TestScanStopsOnEmptyFolder(t *testing.T) {
	e := setup(t)
	e.put("a.jpg", "landscape.jpg")
	e.scan()
	if err := os.Remove(filepath.Join(e.layout.Folders, "lightroom", "a.jpg")); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := e.service.Scan(context.Background(), e.folder.ID); !errors.Is(err, ErrEmpty) {
			t.Fatalf("scan of an emptied folder: err = %v, want ErrEmpty", err)
		}
	}
	files, _ := e.queries.ListPhotoFiles(context.Background(), e.folder.ID)
	if files[0].MissedScans != 0 {
		t.Errorf("missed scans = %d, want none counted", files[0].MissedScans)
	}

	// A folder that is not there at all, as an unmounted share is, stops the same way.
	if err := os.RemoveAll(filepath.Join(e.layout.Folders, "lightroom")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.Scan(context.Background(), e.folder.ID); !errors.Is(err, ErrFolderMissing) {
		t.Fatalf("scan of a missing folder: err = %v, want ErrFolderMissing", err)
	}
}

func TestScanWaitsForNewFiles(t *testing.T) {
	e := setup(t)
	e.service.Stable = time.Hour
	e.put("a.jpg", "landscape.jpg")
	if res := e.scan(); res.Waiting != 1 || res.Added != 0 {
		t.Fatalf("scan = %+v, want the new file to wait", res)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(e.layout.Folders, "lightroom", "a.jpg"), old, old); err != nil {
		t.Fatal(err)
	}
	if res := e.scan(); res.Added != 1 {
		t.Fatalf("scan = %+v, want the file added once it is old enough", res)
	}
}

func TestScanRefusesTwoAtOnce(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	conn, err := e.service.Ingest.Pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1, $2)`, lockKey, int32(e.folder.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.Scan(ctx, e.folder.ID); !errors.Is(err, ErrRunning) {
		t.Fatalf("err = %v, want ErrRunning", err)
	}
}

func (e *env) processJobs(photoID int64) int {
	e.t.Helper()
	var n int
	err := e.service.Ingest.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = 'process_photo' AND (args ->> 'photo_id')::bigint = $1`, photoID).Scan(&n)
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) photoAt(rel string) store.Photo {
	e.t.Helper()
	files, _ := e.queries.ListPhotoFiles(context.Background(), e.folder.ID)
	for _, f := range files {
		if f.Path == rel {
			p, err := e.queries.GetPhoto(context.Background(), *f.PhotoID)
			if err != nil {
				e.t.Fatal(err)
			}
			return p
		}
	}
	e.t.Fatalf("no row for %s", rel)
	return store.Photo{}
}

func TestScanFollowsAChangedFile(t *testing.T) {
	e := setup(t)
	e.put("a.jpg", "landscape.jpg")
	e.put("b.jpg", "rotated.jpg")
	e.scan()
	before := e.photoAt("a.jpg")

	// Only the time changed: the photo stays as it is.
	later := time.Now().Add(-time.Minute)
	if err := os.Chtimes(filepath.Join(e.layout.Folders, "lightroom", "a.jpg"), later, later); err != nil {
		t.Fatal(err)
	}
	if res := e.scan(); res.Changed != 1 || res.Replaced != 0 {
		t.Fatalf("after a change of time = %+v", res)
	}
	if e.processJobs(before.ID) != 1 {
		t.Error("a change of time queued processing again")
	}
	if res := e.scan(); res.Changed != 0 {
		t.Errorf("the new time was not recorded: %+v", res)
	}

	// New content at the same path: the same photo, with the new hash and a new job.
	e.put("a.jpg", "landscape.png")
	res := e.scan()
	if res.Replaced != 1 || res.Added != 0 {
		t.Fatalf("after an edit = %+v", res)
	}
	after := e.photoAt("a.jpg")
	if after.ID != before.ID || bytes.Equal(after.Sha256, before.Sha256) || after.Mime != "image/png" {
		t.Errorf("photo = id %d mime %s, want id %d and the new hash and type", after.ID, after.Mime, before.ID)
	}
	if e.processJobs(before.ID) != 2 {
		t.Error("an edit did not queue processing")
	}
	if res := e.scan(); res.Changed != 0 || res.Replaced != 0 {
		t.Errorf("rescan = %+v", res)
	}

	// Content that is another photo of the owner is not merged into it.
	e.put("a.jpg", "rotated.jpg")
	if res := e.scan(); res.Conflicts != 1 || res.Replaced != 0 {
		t.Fatalf("after copying another photo over it = %+v", res)
	}
	if got := e.photoAt("a.jpg"); !bytes.Equal(got.Sha256, after.Sha256) {
		t.Error("the photo took the hash of another photo")
	}
}

func TestScanKeepsAPurgedPhotoPurged(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.put("a.jpg", "landscape.jpg")
	e.scan()
	p := e.photoAt("a.jpg")
	if err := e.queries.TrashPhotos(ctx, store.TrashPhotosParams{Ids: []int64{p.ID}, ActorID: e.owner}); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Ingest.Purge(ctx, e.owner, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.layout.Folders, "lightroom", "a.jpg")); err != nil {
		t.Fatalf("the purge touched the folder's file: %v", err)
	}
	e.put("a.jpg", "rotated.jpg")
	e.scan()
	got, _ := e.queries.GetPhoto(ctx, p.ID)
	if got.PurgedAt == nil || !bytes.Equal(got.Sha256, p.Sha256) {
		t.Errorf("an edit brought back a purged photo: purged %v", got.PurgedAt)
	}
	if res := e.scan(); res.Changed != 0 || res.Added != 0 {
		t.Errorf("rescan = %+v", res)
	}
}

func TestScanRecordsHowItEnded(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	due := func() int {
		folders, err := e.queries.ListLibraryFoldersDue(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return len(folders)
	}
	if due() != 1 {
		t.Fatal("a folder never scanned is not due")
	}
	e.put("a.jpg", "landscape.jpg")
	e.scan()
	if due() != 0 {
		t.Error("a folder just scanned is due again")
	}
	folder, _ := e.queries.GetLibraryFolder(ctx, e.folder.ID)
	if folder.LastScanAt == nil || folder.LastError != nil || !bytes.Contains(folder.LastResult, []byte(`"added": 1`)) {
		t.Errorf("recorded = %v, %v, %s", folder.LastScanAt, folder.LastError, folder.LastResult)
	}

	if err := os.Remove(filepath.Join(e.layout.Folders, "lightroom", "a.jpg")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.Scan(ctx, e.folder.ID); !errors.Is(err, ErrEmpty) {
		t.Fatalf("err = %v", err)
	}
	folder, _ = e.queries.GetLibraryFolder(ctx, e.folder.ID)
	if folder.LastError == nil || *folder.LastError != ErrEmpty.Error() {
		t.Errorf("last error = %v, want the reason the scan stopped", folder.LastError)
	}
}

// The file of a photo goes, but an identical copy is still there: the copy takes
// its place and the photo is not trashed.
func TestScanPromotesACopyWhenTheSourceGoes(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.put("one.jpg", "landscape.jpg")
	e.put("copy of one.jpg", "landscape.jpg")
	e.scan()
	photo := e.photoAt("one.jpg")

	if err := os.Remove(filepath.Join(e.layout.Folders, "lightroom", "one.jpg")); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		e.scan()
	}
	got, _ := e.queries.GetPhoto(ctx, photo.ID)
	if got.DeletedAt != nil {
		t.Fatal("the photo went to the trash although a copy of its file is still there")
	}
	if got.SourcePath == nil || *got.SourcePath != "lightroom/copy of one.jpg" {
		t.Errorf("source path = %v, want the copy", got.SourcePath)
	}
	files, _ := e.queries.ListPhotoFiles(ctx, e.folder.ID)
	if len(files) != 1 || files[0].State != "source" || files[0].Path != "copy of one.jpg" {
		t.Errorf("rows = %+v, want the copy as the one source", files)
	}
}

// An owner moves files from one library folder to another: the photos stay.
func TestScanFollowsAFileToAnotherFolder(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	other, err := e.queries.CreateLibraryFolder(ctx, store.CreateLibraryFolderParams{Path: "archive", OwnerID: e.owner, ScanInterval: 600})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(e.layout.Folders, "archive"), 0o750); err != nil {
		t.Fatal(err)
	}
	e.put("a.jpg", "landscape.jpg")
	e.put("b.jpg", "rotated.jpg")
	e.scan()
	photo := e.photoAt("a.jpg")

	data, _ := os.ReadFile(filepath.Join(fixtures, "landscape.jpg"))
	if err := os.WriteFile(filepath.Join(e.layout.Folders, "archive", "a.jpg"), data, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(e.layout.Folders, "lightroom", "a.jpg")); err != nil {
		t.Fatal(err)
	}
	// The new place is seen first, as a copy; then the old one is found empty.
	if _, err := e.service.Scan(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		e.scan()
	}
	got, _ := e.queries.GetPhoto(ctx, photo.ID)
	if got.DeletedAt != nil || got.SourcePath == nil || *got.SourcePath != "archive/a.jpg" {
		t.Errorf("photo = trashed %v, source %v; want it kept and read from the archive folder", got.DeletedAt != nil, got.SourcePath)
	}
}
