package transcode_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/jobs"
	"github.com/denuser/fuchsia-photos/backend/internal/media"
	"github.com/denuser/fuchsia-photos/backend/internal/metadata"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
	"github.com/denuser/fuchsia-photos/backend/internal/testdb"
	"github.com/denuser/fuchsia-photos/backend/internal/transcode"
)

func TestMain(m *testing.M) {
	if err := media.Startup(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// fake stands for the graphics: it records what it was asked to transcode,
// and the sample videos, which are H.264, probe as HEVC.
type fake struct {
	mu     sync.Mutex
	inputs []string
	fail   bool
}

func (f *fake) transcode(_ context.Context, in, out string, video media.Video, _ media.Hardware) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs = append(f.inputs, in)
	if f.fail {
		return errors.New("graphics on fire")
	}
	return os.WriteFile(out, []byte("copy of "+filepath.Base(in)), 0o600)
}

func (f *fake) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.inputs...)
}

func probeAsHEVC(ctx context.Context, path string) (media.Video, error) {
	video, err := media.ProbeVideo(ctx, path)
	video.Codec = "hevc"
	return video, err
}

type env struct {
	pool    *pgxpool.Pool
	queries *store.Queries
	layout  storage.Layout
	ingest  *ingest.Service
	service *transcode.Service
	fake    *fake
	owner   int64
}

func setup(t *testing.T, mode string, hardware bool) env {
	t.Helper()
	ctx := context.Background()
	pool := testdb.New(t)
	queries := store.New(pool)
	if _, err := queries.UpdateVideoSettings(ctx, mode); err != nil {
		t.Fatal(err)
	}

	layout := storage.Layout{Library: t.TempDir(), Cache: t.TempDir()}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	reader, err := metadata.NewReader()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Close() })

	f := &fake{}
	service := &transcode.Service{Queries: queries, Layout: layout, Transcode: f.transcode, Probe: probeAsHEVC}
	if hardware {
		service.Hardware = &media.Hardware{Device: "/dev/dri/renderD128"}
	} else {
		service.Unavailable = "no render node"
	}
	client, err := jobs.NewClient(pool, jobs.Config{
		Layout: layout, Metadata: reader, Workers: 2,
		Register:  service.Register,
		Queues:    map[string]river.QueueConfig{transcode.Queue: {MaxWorkers: 1}},
		Processed: service.Processed,
	})
	if err != nil {
		t.Fatal(err)
	}
	service.Jobs = client
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Stop(ctx); err != nil {
			t.Error(err)
		}
	})

	user, err := queries.CreateUser(ctx, store.CreateUserParams{Login: "anna", PasswordHash: "x", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	return env{pool, queries, layout, &ingest.Service{Pool: pool, Jobs: client, Layout: layout}, service, f, user.ID}
}

func upload(t *testing.T, e env, name string, data []byte) store.Photo {
	t.Helper()
	if data == nil {
		var err error
		if data, err = os.ReadFile(filepath.Join("..", "media", "testdata", name)); err != nil {
			t.Fatal(err)
		}
	}
	res, err := e.ingest.Ingest(context.Background(), e.owner, bytes.NewReader(data), name, ingest.PriorityInteractive)
	if err != nil {
		t.Fatal(err)
	}
	return res.Photo
}

// waitFor polls the photo until check accepts it.
func waitFor(t *testing.T, e env, id int64, what string, check func(store.Photo) bool) store.Photo {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		photo, err := e.queries.GetPhoto(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if check(photo) {
			return photo
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("photo %d: %s did not happen", id, what)
	return store.Photo{}
}

func ready(p store.Photo) bool { return p.Status == "ready" }

func copyIs(state string) func(store.Photo) bool {
	return func(p store.Photo) bool { return p.VideoCopy != nil && *p.VideoCopy == state }
}

func TestCopyOnUpload(t *testing.T) {
	e := setup(t, transcode.ModeUpload, true)
	video := upload(t, e, "video.mov", nil)
	waitFor(t, e, video.ID, "copy", copyIs(transcode.CopyReady))

	data, err := os.ReadFile(e.layout.Copy(video.ID))
	if err != nil {
		t.Fatal(err)
	}
	original := e.layout.Original(video.OwnerID, video.Sha256, video.Mime)
	if string(data) != "copy of "+filepath.Base(original) {
		t.Errorf("copy = %q", data)
	}
	// No temporary file stays next to the copy.
	entries, err := os.ReadDir(filepath.Dir(e.layout.Copy(video.ID)))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".mp4" && entry.Name() != filepath.Base(e.layout.Copy(video.ID)) {
			t.Errorf("stray file %s", entry.Name())
		}
	}

	// A still photo gets nothing.
	still := upload(t, e, "landscape.jpg", nil)
	waitFor(t, e, still.ID, "processing", ready)
	if _, err := e.service.QueueMissing(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if photo, _ := e.queries.GetPhoto(context.Background(), still.ID); photo.VideoCopy != nil {
		t.Errorf("still photo copy = %s", *photo.VideoCopy)
	}
	if calls := e.fake.calls(); len(calls) != 1 {
		t.Errorf("transcoded %v, want the video once", calls)
	}
}

// Copies on first view wait until someone asks; the motion of a motion photo
// is copied from the video extracted next to the previews.
func TestCopyOnView(t *testing.T) {
	e := setup(t, transcode.ModeView, true)
	ctx := context.Background()
	video := waitFor(t, e, upload(t, e, "video.mp4", nil).ID, "processing", ready)
	motion := waitFor(t, e, upload(t, e, "motion.jpg", nil).ID, "processing", ready)
	if !motion.HasMotion {
		t.Fatal("the motion photo has no motion")
	}
	time.Sleep(500 * time.Millisecond)
	if calls := e.fake.calls(); len(calls) != 0 {
		t.Fatalf("transcoded %v before anyone asked", calls)
	}

	for _, id := range []int64{video.ID, motion.ID} {
		queued, err := e.service.Request(ctx, id)
		if err != nil || !queued {
			t.Fatalf("request %d: queued %v, %v", id, queued, err)
		}
		waitFor(t, e, id, "copy", copyIs(transcode.CopyReady))
	}
	calls := e.fake.calls()
	if len(calls) != 2 || calls[1] != e.layout.Motion(motion.ID) {
		t.Errorf("transcoded %v, want the motion of photo %d second", calls, motion.ID)
	}
}

// Without graphics nothing is queued, whatever the setting says.
func TestNoHardware(t *testing.T) {
	e := setup(t, transcode.ModeUpload, false)
	video := waitFor(t, e, upload(t, e, "video.mov", nil).ID, "processing", ready)
	queued, err := e.service.Request(context.Background(), video.ID)
	if err != nil || queued {
		t.Errorf("request: queued %v, %v", queued, err)
	}
	time.Sleep(500 * time.Millisecond)
	if photo, _ := e.queries.GetPhoto(context.Background(), video.ID); photo.VideoCopy != nil || len(e.fake.calls()) != 0 {
		t.Errorf("copy = %v, calls %v", photo.VideoCopy, e.fake.calls())
	}
}

// The real probe finds H.264, which plays everywhere.
func TestH264NeedsNoCopy(t *testing.T) {
	e := setup(t, transcode.ModeUpload, true)
	e.service.Probe = nil
	video := upload(t, e, "video.mov", nil)
	waitFor(t, e, video.ID, "copy", copyIs(transcode.CopyNotNeeded))
	if calls := e.fake.calls(); len(calls) != 0 {
		t.Errorf("transcoded %v", calls)
	}
}

// A copy that fails twice is not tried again, and the item plays as before.
func TestFailedCopy(t *testing.T) {
	e := setup(t, transcode.ModeUpload, true)
	e.fake.fail = true
	video := upload(t, e, "video.mov", nil)
	waitFor(t, e, video.ID, "failure", copyIs(transcode.CopyFailed))
	if calls := e.fake.calls(); len(calls) != 2 {
		t.Errorf("transcoded %d times, want 2", len(calls))
	}
	if _, err := os.Stat(e.layout.Copy(video.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("copy file: %v", err)
	}
}

// Choosing copies on upload covers the items uploaded before.
func TestQueueMissing(t *testing.T) {
	e := setup(t, transcode.ModeView, true)
	ctx := context.Background()
	first := waitFor(t, e, upload(t, e, "video.mov", nil).ID, "processing", ready)
	second := waitFor(t, e, upload(t, e, "video.mp4", nil).ID, "processing", ready)
	if _, err := e.queries.UpdateVideoSettings(ctx, transcode.ModeUpload); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.Jobs.Insert(ctx, transcode.QueueMissingArgs{}, nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, e, first.ID, "copy", copyIs(transcode.CopyReady))
	waitFor(t, e, second.ID, "copy", copyIs(transcode.CopyReady))
}

// The video of a Live Photo plays as the photo's motion, so the photo gets
// the copy, made from the video's original, whichever file asks for it.
func TestLivePhotoCopy(t *testing.T) {
	e := setup(t, transcode.ModeView, true)
	ctx := context.Background()
	still := waitFor(t, e, upload(t, e, "landscape.jpg", nil).ID, "processing", ready)
	if _, err := e.pool.Exec(ctx, `UPDATE photos SET exif = exif || '{"ContentIdentifier": "LIVE-1"}' WHERE id = $1`, still.ID); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "live.mov")
	data, err := os.ReadFile(filepath.Join("..", "media", "testdata", "video.mov"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("exiftool", "-q", "-overwrite_original", "-Keys:ContentIdentifier=LIVE-1", path).CombinedOutput(); err != nil {
		t.Fatalf("exiftool: %v: %s", err, out)
	}
	if data, err = os.ReadFile(path); err != nil {
		t.Fatal(err)
	}
	video := waitFor(t, e, upload(t, e, "live.mov", data).ID, "pairing", func(p store.Photo) bool { return p.LivePhotoID != nil })

	if queued, err := e.service.Request(ctx, video.ID); err != nil || !queued {
		t.Fatalf("request: queued %v, %v", queued, err)
	}
	waitFor(t, e, still.ID, "copy", copyIs(transcode.CopyReady))
	calls := e.fake.calls()
	if len(calls) != 1 || calls[0] != e.layout.Original(video.OwnerID, video.Sha256, video.Mime) {
		t.Errorf("transcoded %v, want the video's original", calls)
	}
	if _, err := os.Stat(e.layout.Copy(still.ID)); err != nil {
		t.Error(err)
	}
	if photo, _ := e.queries.GetPhoto(ctx, video.ID); photo.VideoCopy != nil {
		t.Errorf("the video itself has copy %s", *photo.VideoCopy)
	}
}
