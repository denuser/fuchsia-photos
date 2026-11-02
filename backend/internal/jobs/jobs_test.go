package jobs_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/jobs"
	"github.com/denuser/fuchsia-photos/backend/internal/media"
	"github.com/denuser/fuchsia-photos/backend/internal/metadata"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
	"github.com/denuser/fuchsia-photos/backend/internal/testdb"
)

func TestMain(m *testing.M) {
	if err := media.Startup(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

type env struct {
	pool    *pgxpool.Pool
	queries *store.Queries
	layout  storage.Layout
	ingest  *ingest.Service
	owner   int64
}

func setup(t *testing.T) env {
	t.Helper()
	ctx := context.Background()
	pool := testdb.New(t)

	layout := storage.Layout{Library: t.TempDir(), Cache: t.TempDir()}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	reader, err := metadata.NewReader()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Close() })

	client, err := jobs.NewClient(pool, jobs.Config{Layout: layout, Metadata: reader, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Stop(ctx); err != nil {
			t.Error(err)
		}
	})

	queries := store.New(pool)
	user, err := queries.CreateUser(ctx, store.CreateUserParams{Login: "anna", PasswordHash: "x", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	return env{pool, queries, layout, &ingest.Service{Pool: pool, Jobs: client, Layout: layout}, user.ID}
}

func waitForStatus(t *testing.T, e env, id int64, status string) store.Photo {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		photo, err := e.queries.GetPhoto(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if photo.Status == status {
			return photo
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("photo %d did not reach status %s", id, status)
	return store.Photo{}
}

func TestProcessPhoto(t *testing.T) {
	e := setup(t)
	tests := []struct {
		file          string
		mime          string
		width, height int32
	}{
		{"landscape.jpg", "image/jpeg", 800, 600},
		{"landscape.png", "image/png", 800, 600},
		{"landscape.webp", "image/webp", 800, 600},
		{"landscape.heic", "image/heic", 800, 600},
		{"rotated.jpg", "image/jpeg", 600, 800},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "media", "testdata", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			res, err := e.ingest.Ingest(context.Background(), e.owner, bytes.NewReader(data), tt.file, ingest.PriorityInteractive)
			if err != nil {
				t.Fatal(err)
			}
			if res.Photo.Mime != tt.mime {
				t.Errorf("mime = %s, want %s", res.Photo.Mime, tt.mime)
			}

			photo := waitForStatus(t, e, res.Photo.ID, "ready")
			if *photo.Width != tt.width || *photo.Height != tt.height {
				t.Errorf("size = %dx%d, want %dx%d", *photo.Width, *photo.Height, tt.width, tt.height)
			}
			for _, kind := range []string{storage.PreviewGrid, storage.PreviewViewer} {
				if _, err := os.Stat(e.layout.Preview(photo.ID, kind, *photo.PreviewFormat)); err != nil {
					t.Error(err)
				}
			}
			original, err := os.ReadFile(e.layout.Original(photo.OwnerID, photo.Sha256, photo.Mime))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(original, data) {
				t.Error("the stored original differs from the upload")
			}
		})
	}
}

func TestProcessPhotoUsesCurrentSettings(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	ingestFile := func(name string) store.Photo {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "media", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		res, err := e.ingest.Ingest(ctx, e.owner, bytes.NewReader(data), name, ingest.PriorityInteractive)
		if err != nil {
			t.Fatal(err)
		}
		return waitForStatus(t, e, res.Photo.ID, "ready")
	}

	before := ingestFile("landscape.jpg")
	if *before.PreviewFormat != string(media.WebP) {
		t.Errorf("default format = %s, want webp", *before.PreviewFormat)
	}

	_, err := e.queries.UpdatePreviewSettings(ctx, store.UpdatePreviewSettingsParams{
		Format: string(media.JPEG), WebpQuality: 75, WebpEffort: 2, JpegQuality: 60, GridEdge: 200, ViewerEdge: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	after := ingestFile("large.jpg")
	if *after.PreviewFormat != string(media.JPEG) {
		t.Errorf("format = %s, want jpeg", *after.PreviewFormat)
	}
	for kind, width := range map[string]int{storage.PreviewViewer: 1024, storage.PreviewGrid: 200} {
		image, err := vips.NewImageFromFile(e.layout.Preview(after.ID, kind, *after.PreviewFormat))
		if err != nil {
			t.Fatal(err)
		}
		if image.Width() != width {
			t.Errorf("%s width = %d, want %d", kind, image.Width(), width)
		}
		image.Close()
	}

	// The earlier photo keeps its previews, in the format recorded for it.
	if _, err := os.Stat(e.layout.Preview(before.ID, storage.PreviewViewer, *before.PreviewFormat)); err != nil {
		t.Error(err)
	}
}

func TestProcessPhotoKeepsGPSInDatabaseOnly(t *testing.T) {
	e := setup(t)
	data, err := os.ReadFile(filepath.Join("..", "media", "testdata", "rotated.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.ingest.Ingest(context.Background(), e.owner, bytes.NewReader(data), "rotated.jpg", ingest.PriorityInteractive)
	if err != nil {
		t.Fatal(err)
	}
	photo := waitForStatus(t, e, res.Photo.ID, "ready")
	if !bytes.Contains(photo.Exif, []byte("GPSLatitude")) {
		t.Errorf("exif has no GPS: %s", photo.Exif)
	}
	preview, err := os.ReadFile(e.layout.Preview(photo.ID, storage.PreviewViewer, *photo.PreviewFormat))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(preview, []byte("Exif")) {
		t.Error("the preview contains EXIF")
	}
}

func TestBrokenPhotoFailsWithoutRetrying(t *testing.T) {
	e := setup(t)
	broken := append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte("garbage"), 100)...)
	res, err := e.ingest.Ingest(context.Background(), e.owner, bytes.NewReader(broken), "broken.jpg", ingest.PriorityInteractive)
	if err != nil {
		t.Fatal(err)
	}
	photo := waitForStatus(t, e, res.Photo.ID, "failed")
	if photo.Error == nil || *photo.Error == "" {
		t.Error("the failure has no error message")
	}

	var attempt int
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var state string
		err := e.pool.QueryRow(context.Background(),
			`SELECT state, attempt FROM river_job WHERE kind = 'process_photo'`).Scan(&state, &attempt)
		if err != nil {
			t.Fatal(err)
		}
		if state == "cancelled" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if attempt != 1 {
		t.Errorf("attempt = %d, want 1", attempt)
	}
}

func TestIngest(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("..", "media", "testdata", "landscape.jpg"))
	if err != nil {
		t.Fatal(err)
	}

	first, err := e.ingest.Ingest(ctx, e.owner, bytes.NewReader(data), "dir/../a.jpg", ingest.PriorityInteractive)
	if err != nil {
		t.Fatal(err)
	}
	if first.Duplicate {
		t.Error("the first upload is marked as a duplicate")
	}
	if first.Photo.OriginalName != "a.jpg" {
		t.Errorf("name = %q", first.Photo.OriginalName)
	}

	second, err := e.ingest.Ingest(ctx, e.owner, bytes.NewReader(data), "b.jpg", ingest.PriorityInteractive)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate || second.Photo.ID != first.Photo.ID {
		t.Errorf("second upload = %+v, want a duplicate of photo %d", second, first.Photo.ID)
	}

	other, err := e.queries.CreateUser(ctx, store.CreateUserParams{Login: "boris", PasswordHash: "x", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	third, err := e.ingest.Ingest(ctx, other.ID, bytes.NewReader(data), "c.jpg", ingest.PriorityInteractive)
	if err != nil {
		t.Fatal(err)
	}
	if third.Duplicate {
		t.Error("deduplication crossed users")
	}

	_, err = e.ingest.Ingest(ctx, e.owner, bytes.NewReader([]byte("just some text")), "x.jpg", ingest.PriorityInteractive)
	if err != ingest.ErrUnsupported {
		t.Errorf("text file: err = %v, want ErrUnsupported", err)
	}

	temp, err := os.ReadDir(e.layout.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(temp) != 0 {
		t.Errorf("%d temporary files were left behind", len(temp))
	}
}

// A photo moved to the trash before processing still gets its previews, so that
// restoring it does not leave it pending forever.
func TestTrashedPhotoIsStillProcessed(t *testing.T) {
	e := setup(t)
	data, err := os.ReadFile(filepath.Join("..", "media", "testdata", "landscape.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.ingest.Ingest(context.Background(), e.owner, bytes.NewReader(data), "landscape.jpg", ingest.PriorityInteractive)
	if err != nil {
		t.Fatal(err)
	}
	err = e.queries.TrashPhotos(context.Background(), store.TrashPhotosParams{Ids: []int64{res.Photo.ID}, ActorID: e.owner})
	if err != nil {
		t.Fatal(err)
	}
	if photo := waitForStatus(t, e, res.Photo.ID, "ready"); photo.DeletedAt == nil {
		t.Error("processing took the photo out of the trash")
	}
}

// An upload racing with a purge of the same file must not end with a record
// whose original is gone.
func TestPurgeAndUploadOfTheSameFile(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("..", "media", "testdata", "landscape.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	upload := func() (store.Photo, error) {
		res, err := e.ingest.Ingest(ctx, e.owner, bytes.NewReader(data), "landscape.jpg", ingest.PriorityInteractive)
		return res.Photo, err
	}
	for range 20 {
		photo, err := upload()
		if err != nil {
			t.Fatal(err)
		}
		if err := e.queries.TrashPhotos(ctx, store.TrashPhotosParams{Ids: []int64{photo.ID}, ActorID: e.owner}); err != nil {
			t.Fatal(err)
		}

		purged := make(chan error, 1)
		go func() { purged <- e.ingest.Purge(ctx, e.owner, photo.ID) }()
		current, err := upload()
		if err != nil {
			t.Fatal(err)
		}
		if err := <-purged; err != nil {
			t.Fatal(err)
		}

		// If the purge went second, the restored photo was out of its reach.
		if _, err := os.Stat(e.layout.Original(current.OwnerID, current.Sha256, current.Mime)); err != nil {
			t.Fatalf("photo %d has no original: %v", current.ID, err)
		}
		if _, err := e.queries.GetPhoto(ctx, current.ID); err != nil {
			t.Fatalf("photo %d has no record: %v", current.ID, err)
		}
	}
}
