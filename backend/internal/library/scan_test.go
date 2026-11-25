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
	if err != nil || len(files) != 3 {
		t.Fatalf("known files = %d, %v; want 2 photos and the note that is not one", len(files), err)
	}
	for _, f := range files {
		if f.State == "unsupported" {
			continue
		}
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
	// The conflict stays on the page until the file is fixed, and is not read again meanwhile.
	problems, err := e.queries.ListLibraryFiles(context.Background(), store.ListLibraryFilesParams{FolderID: e.folder.ID, Kind: "problems", MaxRows: 10})
	if err != nil || len(problems) != 1 || problems[0].Problem == nil || *problems[0].Problem != "conflict" || problems[0].Path != "a.jpg" {
		t.Fatalf("problems = %+v, %v; want the conflict of a.jpg", problems, err)
	}
	if res := e.scan(); res.Changed != 0 || res.Conflicts != 0 {
		t.Errorf("rescan = %+v, want the conflict left alone", res)
	}
	e.put("a.jpg", "landscape.webp")
	if res := e.scan(); res.Replaced != 1 {
		t.Fatalf("after fixing the file = %+v", res)
	}
	if problems, _ := e.queries.ListLibraryFiles(context.Background(), store.ListLibraryFilesParams{FolderID: e.folder.ID, Kind: "problems", MaxRows: 10}); len(problems) != 0 {
		t.Errorf("problems = %+v, want none once the file holds its own content", problems)
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

// A file that is not a photo is remembered, so that a rescan does not open it again. It is read
// again when it changes or when the application's list of formats grows.
func TestScanRemembersUnsupportedFiles(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	jpeg, err := os.ReadFile(filepath.Join(fixtures, "landscape.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	notes := bytes.Repeat([]byte("x"), len(jpeg))
	e.putBytes("notes.psd", notes)
	if res := e.scan(); res.Unsupported != 1 || res.Added != 0 {
		t.Fatalf("first scan = %+v", res)
	}
	rows, _ := e.queries.ListPhotoFiles(ctx, e.folder.ID)
	if len(rows) != 1 || rows[0].State != "unsupported" || rows[0].SupportVersion != ingest.SupportVersion {
		t.Fatalf("rows = %+v, want one unsupported row at the current version", rows)
	}

	// The same size and time with other content shows whether the file is opened again: it is not.
	p := filepath.Join(e.layout.Folders, "lightroom", "notes.psd")
	if err := os.WriteFile(p, jpeg, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, rows[0].Mtime, rows[0].Mtime); err != nil {
		t.Fatal(err)
	}
	if res := e.scan(); res.Added != 0 || res.Unsupported != 1 {
		t.Fatalf("rescan = %+v, want the file left alone", res)
	}

	// An application that learns a format reads the files again with no migration.
	if _, err := e.service.Ingest.Pool.Exec(ctx, `UPDATE photo_files SET support_version = 0`); err != nil {
		t.Fatal(err)
	}
	if res := e.scan(); res.Added != 1 {
		t.Fatalf("scan after the formats changed = %+v, want the photo added", res)
	}
	if rows, _ := e.queries.ListPhotoFiles(ctx, e.folder.ID); len(rows) != 1 || rows[0].State != "source" {
		t.Errorf("rows = %+v, want the file to be a source now", rows)
	}

	// A file that goes is forgotten too.
	e.putBytes("other.tmp", []byte("text"))
	e.scan()
	if err := os.Remove(filepath.Join(e.layout.Folders, "lightroom", "other.tmp")); err != nil {
		t.Fatal(err)
	}
	e.scan()
	if rows, _ := e.queries.ListPhotoFiles(ctx, e.folder.ID); len(rows) != 1 {
		t.Errorf("rows = %+v, want the vanished file forgotten", rows)
	}
}

// A new file that could not be read is tried again by every scan.
func TestScanTriesFailedFilesAgain(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.put("a.jpg", "landscape.jpg")
	info, err := os.Stat(filepath.Join(e.layout.Folders, "lightroom", "a.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.queries.InsertPhotoFile(ctx, store.InsertPhotoFileParams{
		FolderID: e.folder.ID, Path: "a.jpg", State: "failed", Size: info.Size(), Mtime: info.ModTime(), Detail: new("permission denied"),
	}); err != nil {
		t.Fatal(err)
	}
	problems, _ := e.queries.ListLibraryFiles(ctx, store.ListLibraryFilesParams{FolderID: e.folder.ID, Kind: "problems", MaxRows: 10})
	if len(problems) != 1 || problems[0].State != "failed" || *problems[0].Detail != "permission denied" {
		t.Fatalf("problems = %+v", problems)
	}
	if res := e.scan(); res.Added != 1 {
		t.Fatalf("scan = %+v, want the file read again and added", res)
	}
	if problems, _ := e.queries.ListLibraryFiles(ctx, store.ListLibraryFilesParams{FolderID: e.folder.ID, Kind: "problems", MaxRows: 10}); len(problems) != 0 {
		t.Errorf("problems = %+v, want none once it could be read", problems)
	}
}

// skipAsRoot skips a test that needs a file the process may not read: root reads anything.
func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root, which ignores file permissions")
	}
}

// A subfolder the scan may not read is not an empty one: its files are not listed, and not
// marking them missing is what keeps their photos out of the trash.
func TestScanKeepsPhotosOfAnUnreadableSubfolder(t *testing.T) {
	skipAsRoot(t)
	e := setup(t)
	ctx := context.Background()
	e.put("a/IMG_0001.jpg", "landscape.jpg")
	e.put("IMG_0002.jpg", "rotated.jpg")
	if res := e.scan(); res.Added != 2 {
		t.Fatalf("first scan = %+v, want 2 added", res)
	}

	dir := filepath.Join(e.layout.Folders, "lightroom", "a")
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o750) })
	// More scans than a file needs to go to the trash after it is missing.
	for i := range 4 {
		res, err := e.service.Scan(ctx, e.folder.ID)
		if err != nil {
			t.Fatalf("scan %d: %v, want the unreadable subfolder to leave it going", i+1, err)
		}
		if res.Missing != 0 || res.Trashed != 0 || res.Listed != 1 {
			t.Fatalf("scan %d = %+v, want 1 file listed and nothing missing or trashed", i+1, res)
		}
	}
	files, _ := e.queries.ListPhotoFiles(ctx, e.folder.ID)
	if len(files) != 2 {
		t.Fatalf("known files = %d, want both kept", len(files))
	}
	for _, f := range files {
		if f.MissedScans != 0 {
			t.Errorf("%s: missed scans = %d, want none counted", f.Path, f.MissedScans)
		}
		photo, err := e.queries.GetPhoto(ctx, *f.PhotoID)
		if err != nil {
			t.Fatal(err)
		}
		if photo.DeletedAt != nil {
			t.Errorf("%s: the photo is in the trash", f.Path)
		}
	}

	// Readable again: the scan sees the file and nothing has changed for its photo.
	if err := os.Chmod(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if res := e.scan(); res.Listed != 2 || res.Missing != 0 || res.Trashed != 0 || res.Added != 0 {
		t.Errorf("scan after the folder was readable = %+v", res)
	}
}

// A file the scan may not read is a problem of that file only: a new one waits as a problem
// and is added once it can be read, and one already known is left alone, as it is not read
// while its size and time are the same.
func TestScanReportsAnUnreadableFile(t *testing.T) {
	skipAsRoot(t)
	e := setup(t)
	ctx := context.Background()
	e.put("known.jpg", "landscape.jpg")
	if res := e.scan(); res.Added != 1 {
		t.Fatalf("first scan = %+v", res)
	}
	e.put("new.jpg", "rotated.jpg")
	dir := filepath.Join(e.layout.Folders, "lightroom")
	for _, name := range []string{"known.jpg", "new.jpg"} {
		p := filepath.Join(dir, name)
		if err := os.Chmod(p, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(p, 0o640) })
	}

	problems := func() []store.ListLibraryFilesRow {
		rows, err := e.queries.ListLibraryFiles(ctx, store.ListLibraryFilesParams{FolderID: e.folder.ID, Kind: "problems", MaxRows: 10})
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	for i := range 3 {
		res := e.scan()
		if res.Added != 0 || res.Failed != 1 || res.Missing != 0 || res.Trashed != 0 {
			t.Fatalf("scan %d = %+v, want only the new file failed", i+1, res)
		}
	}
	if p := problems(); len(p) != 1 || p[0].Path != "new.jpg" || p[0].State != "failed" || p[0].Detail == nil {
		t.Fatalf("problems = %+v, want new.jpg failed with the reason", p)
	}
	files, _ := e.queries.ListPhotoFiles(ctx, e.folder.ID)
	for _, f := range files {
		if f.Path == "known.jpg" {
			photo, err := e.queries.GetPhoto(ctx, *f.PhotoID)
			if err != nil || photo.DeletedAt != nil || f.MissedScans != 0 {
				t.Errorf("known.jpg: photo %+v, missed %d, err %v; want it left alone", photo.DeletedAt, f.MissedScans, err)
			}
		}
	}

	for _, name := range []string{"known.jpg", "new.jpg"} {
		if err := os.Chmod(filepath.Join(dir, name), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if res := e.scan(); res.Added != 1 || res.Failed != 0 {
		t.Fatalf("scan after the files were readable = %+v, want the new file added", res)
	}
	if p := problems(); len(p) != 0 {
		t.Errorf("problems = %+v, want none once it could be read", p)
	}
}

// A scan cut off in the middle, by a restart or by its time limit, marks nothing missing and
// changes no row: what it had not seen is not away.
func TestInterruptedScanMarksNothing(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.put("a.jpg", "landscape.jpg")
	e.put("b.jpg", "rotated.jpg")
	e.scan()
	if err := os.Remove(filepath.Join(e.layout.Folders, "lightroom", "a.jpg")); err != nil {
		t.Fatal(err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	folder, _ := e.queries.GetLibraryFolder(ctx, e.folder.ID)
	run := &scanRun{Service: e.service, queries: e.queries, folder: folder, seen: map[string]bool{}, moved: map[int64]bool{}}
	if err := run.scan(cancelled); err == nil {
		t.Fatal("a cancelled scan ended without an error")
	}
	files, _ := e.queries.ListPhotoFiles(ctx, e.folder.ID)
	for _, f := range files {
		if f.MissedScans != 0 || f.MissingSince != nil {
			t.Errorf("%s: missed %d times, missing since %v; want nothing marked", f.Path, f.MissedScans, f.MissingSince)
		}
	}
}
