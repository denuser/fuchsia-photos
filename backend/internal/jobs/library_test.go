package jobs_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/magellanica-app/magellanica-photos/backend/internal/ingest"
	"github.com/magellanica-app/magellanica-photos/backend/internal/jobs"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

// A photo in a library folder whose file cannot be read is not at fault: its
// processing waits instead of failing, and goes on when the file is back.
func TestProcessPhotoWaitsForAnOriginalInALibraryFolder(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	photo, err := e.queries.InsertPhoto(ctx, store.InsertPhotoParams{
		OwnerID: e.owner, Sha256: bytes.Repeat([]byte{7}, 32), Mime: "image/jpeg", Size: 1, OriginalName: "gone.jpg",
		SourcePath: new("lightroom/gone.jpg"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ingest.Jobs.Insert(ctx, jobs.ProcessPhotoArgs{PhotoID: photo.ID}, nil); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		var state string
		err := e.pool.QueryRow(ctx, `SELECT state FROM river_job WHERE kind = 'process_photo' AND (args ->> 'photo_id')::bigint = $1`, photo.ID).Scan(&state)
		if err == nil && state == "scheduled" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the job is %q, want it to wait", state)
		}
		time.Sleep(50 * time.Millisecond)
	}
	got, _ := e.queries.GetPhoto(ctx, photo.ID)
	if got.Status != "pending" || got.Error != nil {
		t.Errorf("status = %s, error = %v; want pending with no error", got.Status, got.Error)
	}
}

// An edit of a library file in another program changes the photo in place:
// the same id, the new content processed, the old previews replaced.
func TestEditedFileInALibraryFolderIsProcessedAgain(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	path := filepath.Join(e.layout.Folders, "edit.jpg")
	copyFixture := func(name string) {
		data, err := os.ReadFile(filepath.Join("..", "media", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o640); err != nil {
			t.Fatal(err)
		}
	}

	copyFixture("landscape.jpg")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.ingest.ImportInPlace(ctx, e.owner, file, "edit.jpg", "edit.jpg")
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	ready := waitForStatus(t, e, res.Photo.ID, "ready")
	if *ready.Width != 800 || *ready.Height != 600 {
		t.Fatalf("size = %dx%d, want 800x600", *ready.Width, *ready.Height)
	}

	copyFixture("rotated.jpg")
	file, err = os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	outcome, _, err := e.ingest.ReplaceInPlace(ctx, res.Photo.ID, file, "edit.jpg")
	file.Close()
	if err != nil || outcome != ingest.Replaced {
		t.Fatalf("outcome = %v, err = %v; want Replaced", outcome, err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		photo, err := e.queries.GetPhoto(ctx, res.Photo.ID)
		if err != nil {
			t.Fatal(err)
		}
		if photo.Status == "ready" && photo.Width != nil && *photo.Width == 600 {
			if photo.PreviewVersion <= ready.PreviewVersion {
				t.Errorf("preview version = %d, want it above %d", photo.PreviewVersion, ready.PreviewVersion)
			}
			if bytes.Equal(photo.Sha256, ready.Sha256) {
				t.Error("the hash did not change")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the edited photo was not processed again: status %s", photo.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
