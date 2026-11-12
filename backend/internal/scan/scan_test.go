package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/jobs"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
	"github.com/denuser/fuchsia-photos/backend/internal/testdb"
)

const fixtures = "../media/testdata"

type env struct {
	t       *testing.T
	service *Service
	queries *store.Queries
	dir     string
	owner   int64
}

// setup queues jobs without running them. Tests drive the scan with step, and
// processing is covered by the jobs package.
func setup(t *testing.T) *env {
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
	user, err := queries.CreateUser(context.Background(), store.CreateUserParams{Login: "anna", PasswordHash: "x", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	service := &Service{Ingest: &ingest.Service{Pool: pool, Jobs: client, Layout: layout}, Dir: dir}
	return &env{t, service, queries, dir, user.ID}
}

func (e *env) copyFixture(fixture, to string) {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtures, fixture))
	if err != nil {
		e.t.Fatal(err)
	}
	e.write(to, data)
}

func (e *env) write(to string, data []byte) {
	e.t.Helper()
	path := filepath.Join(e.dir, to)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o640); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) start(folder string) store.ImportRun {
	e.t.Helper()
	run, err := e.service.Start(context.Background(), e.owner, e.owner, folder)
	if err != nil {
		e.t.Fatal(err)
	}
	return run
}

// finish runs steps until the scan ends. Stable is zero here, so nothing waits.
func (e *env) finish(id int64) store.ImportRun {
	e.t.Helper()
	ctx := context.Background()
	for range 100 {
		_, more, err := e.service.step(ctx, id, 0)
		if err != nil {
			e.t.Fatal(err)
		}
		if !more {
			run, err := e.queries.GetImportRun(ctx, id)
			if err != nil {
				e.t.Fatal(err)
			}
			return run
		}
	}
	e.t.Fatal("the scan did not end")
	return store.ImportRun{}
}

func (e *env) scan(folder string) store.ImportRun {
	e.t.Helper()
	return e.finish(e.start(folder).ID)
}

func (e *env) photoCount() int {
	e.t.Helper()
	var n int
	err := e.service.Ingest.Pool.QueryRow(context.Background(), "SELECT count(*) FROM photos WHERE owner_id = $1", e.owner).Scan(&n)
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

type counts struct{ found, added, duplicates, inTrash, purged, unsupported, changing, failed int64 }

func wantCounts(t *testing.T, run store.ImportRun, want counts) {
	t.Helper()
	got := counts{run.Found, run.Added, run.Duplicates, run.InTrash, run.Purged, run.Unsupported, run.Changing, run.Failed}
	if got != want {
		t.Errorf("counts = %+v, want %+v", got, want)
	}
	if run.Status != "finished" {
		t.Errorf("status = %s, error = %v", run.Status, run.Error)
	}
}

// snapshot describes a folder tree: names, sizes, modification times and contents.
func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var out strings.Builder
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		out.WriteString(path + " " + info.Mode().String() + " " + info.ModTime().String() + "\n")
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out.Write(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestScanTwice(t *testing.T) {
	e := setup(t)
	e.copyFixture("landscape.jpg", "2024/a.jpg")
	e.copyFixture("landscape.png", "2024/deep/er/b.png")
	e.copyFixture("landscape.jpg", "copy-of-a.jpg")
	e.write("notes.txt", []byte("not a photo"))
	before := snapshot(t, e.dir)

	wantCounts(t, e.scan(""), counts{found: 4, added: 2, duplicates: 1, unsupported: 1})
	if got := e.photoCount(); got != 2 {
		t.Errorf("photos = %d, want 2", got)
	}

	wantCounts(t, e.scan(""), counts{found: 4, duplicates: 3, unsupported: 1})
	if got := e.photoCount(); got != 2 {
		t.Errorf("photos after the second run = %d, want 2", got)
	}
	if snapshot(t, e.dir) != before {
		t.Error("the source folder changed")
	}

	t.Run("a known file is not written again", func(t *testing.T) {
		entries, err := os.ReadDir(e.service.Ingest.Layout.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("temporary files left: %v", entries)
		}
	})
}

func TestScanSubfolder(t *testing.T) {
	e := setup(t)
	e.copyFixture("landscape.jpg", "anna/a.jpg")
	e.copyFixture("landscape.png", "boris/b.png")

	run := e.scan("anna/")
	if run.Folder != "anna" {
		t.Errorf("folder = %q", run.Folder)
	}
	wantCounts(t, run, counts{found: 1, added: 1})
}

func TestScanLeavesOutWhatIsNotContent(t *testing.T) {
	e := setup(t)
	e.copyFixture("landscape.jpg", "a.jpg")
	e.copyFixture("landscape.png", ".hidden.png")
	e.copyFixture("landscape.png", "._a.jpg")
	e.copyFixture("landscape.png", "@eaDir/a.jpg/thumb.png")
	e.copyFixture("landscape.png", "#recycle/old.png")
	e.copyFixture("landscape.png", ".git/x.png")
	e.copyFixture("landscape.png", "IMG_0001.AAE")
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.png"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(e.dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(e.dir, "a.jpg"), filepath.Join(e.dir, "link.jpg")); err != nil {
		t.Fatal(err)
	}

	wantCounts(t, e.scan(""), counts{found: 1, added: 1})
}

func TestScanKeepsDeletedPhotosDeleted(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.copyFixture("landscape.jpg", "a.jpg")
	e.copyFixture("landscape.png", "b.png")
	e.scan("")

	photos, err := e.queries.ListTimeline(ctx, store.ListTimelineParams{OwnerID: e.owner, MaxRows: 10})
	if err != nil {
		t.Fatal(err)
	}
	trashed, purged := photos[0], photos[1]
	if err := e.queries.TrashPhotos(ctx, store.TrashPhotosParams{Ids: []int64{trashed.ID, purged.ID}, ActorID: e.owner}); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Ingest.Purge(ctx, e.owner, purged.ID); err != nil {
		t.Fatal(err)
	}

	wantCounts(t, e.scan(""), counts{found: 2, inTrash: 1, purged: 1})

	got, err := e.queries.GetPhoto(ctx, trashed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeletedAt == nil {
		t.Error("the scan restored a photo from the trash")
	}
	original := e.service.Ingest.Layout.Original(e.owner, purged.Sha256, purged.Mime)
	if _, err := os.Stat(original); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the scan brought back a purged file: %v", err)
	}

	t.Run("an upload still brings a purged photo back", func(t *testing.T) {
		name := map[string]string{"image/jpeg": "landscape.jpg", "image/png": "landscape.png"}[purged.Mime]
		file, err := os.Open(filepath.Join(fixtures, name))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		res, err := e.service.Ingest.Ingest(ctx, e.owner, file, name, ingest.PriorityInteractive)
		if err != nil {
			t.Fatal(err)
		}
		if res.Duplicate || res.Photo.ID == purged.ID {
			t.Errorf("result = %+v", res)
		}
		if _, err := os.Stat(original); err != nil {
			t.Error(err)
		}
	})
}

func TestScanSkipsFilesStillBeingCopied(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.copyFixture("landscape.jpg", "done.jpg")
	e.copyFixture("landscape.png", "growing.png")

	run := e.start("")
	if _, _, err := e.service.step(ctx, run.ID, 0); err != nil { // Only the listing.
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(e.dir, "growing.png"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("more"); err != nil {
		t.Fatal(err)
	}
	file.Close()

	wantCounts(t, e.finish(run.ID), counts{found: 2, added: 1, changing: 1})
	problems, err := e.queries.ListImportProblems(ctx, store.ListImportProblemsParams{RunID: run.ID, MaxRows: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || problems[0].Path != "growing.png" || problems[0].State != "changing" {
		t.Errorf("problems = %+v", problems)
	}

	t.Run("the next run picks it up", func(t *testing.T) {
		wantCounts(t, e.scan(""), counts{found: 2, added: 1, duplicates: 1})
	})
}

func TestScanWaitsBeforeImporting(t *testing.T) {
	e := setup(t)
	e.service.Stable = time.Hour
	e.copyFixture("landscape.jpg", "a.jpg")
	ctx := context.Background()
	run := e.start("")

	for range 2 { // After the listing, and again if the job runs early.
		wait, more, err := e.service.step(ctx, run.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !more || wait < 59*time.Minute {
			t.Errorf("wait = %v, more = %v", wait, more)
		}
	}
	if got := e.photoCount(); got != 0 {
		t.Errorf("photos = %d before the wait ended", got)
	}
}

func TestScanReportsFileProblems(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.copyFixture("landscape.jpg", "gone.jpg")
	run := e.start("")
	if _, _, err := e.service.step(ctx, run.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(e.dir, "gone.jpg")); err != nil {
		t.Fatal(err)
	}

	wantCounts(t, e.finish(run.ID), counts{found: 1, failed: 1})
	problems, err := e.queries.ListImportProblems(ctx, store.ListImportProblemsParams{RunID: run.ID, MaxRows: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || problems[0].State != "failed" || problems[0].Error == nil {
		t.Errorf("problems = %+v", problems)
	}
}

func TestScanResumesAfterInterruption(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	// More files than one batch, so that a slice of zero stops partway.
	data, err := os.ReadFile(filepath.Join(fixtures, "landscape.png"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range processBatch + 5 {
		e.write(filepath.Join("many", string(rune('a'+i))+".png"), append(data[:len(data):len(data)], byte(i)))
	}

	run := e.start("")
	if _, _, err := e.service.step(ctx, run.ID, 0); err != nil { // The listing.
		t.Fatal(err)
	}
	_, more, err := e.service.step(ctx, run.ID, 0) // One batch.
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("the scan finished in one slice")
	}
	if got := e.photoCount(); got != processBatch {
		t.Errorf("photos after one slice = %d, want %d", got, processBatch)
	}

	wantCounts(t, e.finish(run.ID), counts{found: processBatch + 5, added: processBatch + 5})
}

func TestStop(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.copyFixture("landscape.jpg", "a.jpg")
	run := e.start("")
	if _, _, err := e.service.step(ctx, run.ID, 0); err != nil {
		t.Fatal(err)
	}
	if n, err := e.queries.StopImportRun(ctx, run.ID); err != nil || n != 1 {
		t.Fatalf("stopped = %d, error = %v", n, err)
	}

	got := e.finish(run.ID)
	if got.Status != "stopped" || got.FinishedAt == nil || got.Added != 0 {
		t.Errorf("run = %+v", got)
	}
	if got := e.photoCount(); got != 0 {
		t.Errorf("photos = %d after a stop", got)
	}
	var pending int
	if err := e.service.Ingest.Pool.QueryRow(ctx, "SELECT count(*) FROM import_files").Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Errorf("%d files left in the list", pending)
	}
}

func TestStart(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.copyFixture("landscape.jpg", "folder/a.jpg")
	if err := os.Symlink(t.TempDir(), filepath.Join(e.dir, "link")); err != nil {
		t.Fatal(err)
	}

	for _, folder := range []string{"nope", "folder/a.jpg", "..", "../..", "folder/../..", "link"} {
		if _, err := e.service.Start(ctx, e.owner, e.owner, folder); !errors.Is(err, ErrNoFolder) {
			t.Errorf("folder %q: error = %v", folder, err)
		}
	}
	if _, err := e.service.Start(ctx, e.owner, 9999, ""); !errors.Is(err, ErrNoOwner) {
		t.Errorf("unknown owner: error = %v", err)
	}
	if err := e.queries.SetUserDisabled(ctx, store.SetUserDisabledParams{ID: e.owner, Disabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.Start(ctx, e.owner, e.owner, ""); !errors.Is(err, ErrNoOwner) {
		t.Errorf("disabled owner: error = %v", err)
	}
	if err := e.queries.SetUserDisabled(ctx, store.SetUserDisabledParams{ID: e.owner, Disabled: false}); err != nil {
		t.Fatal(err)
	}

	run := e.start("folder")
	if _, err := e.service.Start(ctx, e.owner, e.owner, ""); !errors.Is(err, ErrRunning) {
		t.Errorf("second scan: error = %v", err)
	}
	e.finish(run.ID)
	e.start("") // Free again.
}

func TestScanOfAMissingFolderFails(t *testing.T) {
	e := setup(t)
	e.copyFixture("landscape.jpg", "folder/a.jpg")
	run := e.start("folder")
	if err := os.RemoveAll(filepath.Join(e.dir, "folder")); err != nil {
		t.Fatal(err)
	}
	got := e.finish(run.ID)
	if got.Status != "failed" || got.Error == nil {
		t.Errorf("run = %+v", got)
	}
}

func TestFolders(t *testing.T) {
	e := setup(t)
	for _, dir := range []string{"a/b/c/d", "@eaDir/x", ".hidden/y", "#recycle"} {
		if err := os.MkdirAll(filepath.Join(e.dir, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	e.copyFixture("landscape.jpg", "a/photo.jpg")
	got, err := e.service.Folders()
	if err != nil {
		t.Fatal(err)
	}
	if want := "a,a/b,a/b/c"; strings.Join(got, ",") != want {
		t.Errorf("folders = %v, want %s", got, want)
	}
}
