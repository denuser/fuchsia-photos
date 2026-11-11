package jobs_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/jobs"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

func upload(t *testing.T, e env, file string) store.Photo {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "media", "testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.ingest.Ingest(context.Background(), e.owner, bytes.NewReader(data), file, ingest.PriorityInteractive)
	if err != nil {
		t.Fatal(err)
	}
	return res.Photo
}

func TestProcessVideo(t *testing.T) {
	e := setup(t)
	tests := []struct {
		file          string
		mime          string
		width, height int32
		takenAt       string
	}{
		{"video.mp4", "video/mp4", 240, 320, "2024-06-01T08:00:00Z"},
		{"video.mov", "video/quicktime", 320, 240, "2024-05-01T10:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			uploaded := upload(t, e, tt.file)
			if uploaded.Mime != tt.mime {
				t.Errorf("mime = %s, want %s", uploaded.Mime, tt.mime)
			}
			photo := waitForStatus(t, e, uploaded.ID, "ready")
			if *photo.Width != tt.width || *photo.Height != tt.height {
				t.Errorf("size = %dx%d, want %dx%d", *photo.Width, *photo.Height, tt.width, tt.height)
			}
			if photo.Duration == nil || *photo.Duration != 2 {
				t.Errorf("duration = %v, want 2", photo.Duration)
			}
			if photo.TakenAt == nil || photo.TakenAt.UTC().Format(time.RFC3339) != tt.takenAt {
				t.Errorf("taken at %v, want %s", photo.TakenAt, tt.takenAt)
			}
			if photo.HasMotion {
				t.Error("a video has motion")
			}
			for _, kind := range []string{storage.PreviewGrid, storage.PreviewViewer} {
				if _, err := os.Stat(e.layout.Preview(photo.ID, kind, *photo.PreviewFormat)); err != nil {
					t.Error(err)
				}
			}
			if filepath.Ext(e.layout.Original(photo.OwnerID, photo.Sha256, photo.Mime)) == "" {
				t.Error("the original has no extension")
			}
		})
	}
}

func TestBrokenVideoFailsWithoutRetrying(t *testing.T) {
	e := setup(t)
	broken := append([]byte("\x00\x00\x00\x14ftypisom\x00\x00\x00\x00isom"), bytes.Repeat([]byte("garbage"), 100)...)
	res, err := e.ingest.Ingest(context.Background(), e.owner, bytes.NewReader(broken), "broken.mp4", ingest.PriorityInteractive)
	if err != nil {
		t.Fatal(err)
	}
	if photo := waitForStatus(t, e, res.Photo.ID, "failed"); photo.Error == nil {
		t.Error("the failure has no error message")
	}
}

func TestMotionPhoto(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	photo := waitForStatus(t, e, upload(t, e, "motion.jpg").ID, "ready")
	if !photo.HasMotion || photo.Duration != nil {
		t.Fatalf("has motion = %v, duration = %v", photo.HasMotion, photo.Duration)
	}
	video, err := os.ReadFile(e.layout.Motion(photo.ID))
	if err != nil {
		t.Fatal(err)
	}
	if string(video[4:8]) != "ftyp" {
		t.Error("the motion file is not an MP4")
	}

	plain := waitForStatus(t, e, upload(t, e, "landscape.jpg").ID, "ready")
	if plain.HasMotion {
		t.Error("a plain photo has motion")
	}

	// Purging removes the motion with the previews.
	if err := e.queries.TrashPhotos(ctx, store.TrashPhotosParams{Ids: []int64{photo.ID}, ActorID: e.owner}); err != nil {
		t.Fatal(err)
	}
	if err := e.ingest.Purge(ctx, e.owner, photo.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.layout.Motion(photo.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the motion file is still there: %v", err)
	}
	purged, err := e.queries.GetPhoto(ctx, photo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if purged.HasMotion {
		t.Error("a purged photo keeps has_motion")
	}
}

// Each kind of file has its own limit. The limits are small here so that the
// test writes little.
func TestUploadLimits(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	photo, err := os.ReadFile(filepath.Join("..", "media", "testdata", "landscape.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	video, err := os.ReadFile(filepath.Join("..", "media", "testdata", "video.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	service := *e.ingest
	service.PhotoLimit = int64(len(photo)) - 1
	service.VideoLimit = int64(len(video))

	if _, err := service.Ingest(ctx, e.owner, bytes.NewReader(photo), "a.jpg", ingest.PriorityInteractive); err != ingest.ErrTooLarge {
		t.Errorf("photo over its limit: err = %v, want ErrTooLarge", err)
	}
	if _, err := service.Receive(ctx, e.owner, bytes.NewReader(photo), "a.jpg"); err != ingest.ErrTooLarge {
		t.Errorf("photo over its limit over WebDAV: err = %v, want ErrTooLarge", err)
	}
	// A video of exactly its limit fits.
	if _, err := service.Ingest(ctx, e.owner, bytes.NewReader(video), "v.mp4", ingest.PriorityInteractive); err != nil {
		t.Errorf("video at its limit: %v", err)
	}
	// A folder scan has no limit.
	if _, err := service.Import(ctx, e.owner, bytes.NewReader(photo), "a.jpg"); err != nil {
		t.Errorf("folder scan: %v", err)
	}

	temp, err := os.ReadDir(e.layout.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(temp) != 0 {
		t.Errorf("%d temporary files were left behind", len(temp))
	}
}

// Motion photos processed before motion was kept get it from their stored
// tags, without new previews.
func TestBackfillMotion(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	photo := waitForStatus(t, e, upload(t, e, "motion.jpg").ID, "ready")
	plain := waitForStatus(t, e, upload(t, e, "landscape.jpg").ID, "ready")

	// As an earlier version left it.
	if _, err := e.pool.Exec(ctx, `UPDATE photos SET has_motion = false WHERE id = $1`, photo.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(e.layout.Motion(photo.ID)); err != nil {
		t.Fatal(err)
	}

	if _, err := e.ingest.Jobs.Insert(ctx, jobs.BackfillMotionArgs{}, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		current, err := e.queries.GetPhoto(ctx, photo.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.HasMotion {
			if current.PreviewVersion != photo.PreviewVersion {
				t.Error("the backfill made new previews")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the motion was not backfilled")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(e.layout.Motion(photo.ID)); err != nil {
		t.Error(err)
	}
	if current, _ := e.queries.GetPhoto(ctx, plain.ID); current.HasMotion {
		t.Error("a plain photo got motion")
	}
}
