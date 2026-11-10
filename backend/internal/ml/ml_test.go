package ml

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
	"github.com/denuser/fuchsia-photos/backend/internal/testdb"
)

const testModel = "test-model@1"

// fakeService stands in for the ML service. An image whose content is "bad"
// fails on its own; the vector of any other image is [1,0,0].
type fakeService struct {
	mu     sync.Mutex
	model  string
	down   bool
	sent   map[string]int // How often each image name was sent.
	server *httptest.Server
}

func newFakeService(t *testing.T) *fakeService {
	f := &fakeService{model: testModel, sent: map[string]int{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeService) set(change func(*fakeService)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	change(f)
}

func (f *fakeService) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		http.Error(w, "down", http.StatusBadGateway)
		return
	}
	if r.Header.Get("Authorization") != "Bearer secret" {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case "/health":
		json.NewEncoder(w).Encode(Health{Model: f.model, Dimensions: 3, Device: "cpu"})
	case "/embed/text":
		json.NewEncoder(w).Encode(map[string]any{"model": f.model, "embedding": []float32{1, 0, 0}})
	case "/embed/image":
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var results []ImageResult
		for _, header := range r.MultipartForm.File["images"] {
			f.sent[header.Filename]++
			file, _ := header.Open()
			var data [16]byte
			n, _ := file.Read(data[:])
			file.Close()
			if string(data[:n]) == "bad" {
				results = append(results, ImageResult{Error: "decode: broken"})
			} else {
				results = append(results, ImageResult{Embedding: []float32{1, 0, 0}})
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"model": f.model, "results": results})
	default:
		http.NotFound(w, r)
	}
}

func TestMonitorStates(t *testing.T) {
	fake := newFakeService(t)
	monitor := NewMonitor(NewClient(fake.server.URL, "secret"), testModel)
	ctx := context.Background()
	availableCalls := 0
	monitor.OnAvailable(func() { availableCalls++ })

	if got := monitor.Status().State; got != StateUnavailable {
		t.Errorf("before the first check: %s, want unavailable", got)
	}
	if got := monitor.Check(ctx).State; got != StateAvailable {
		t.Errorf("service up: %s, want available", got)
	}
	monitor.Check(ctx)
	if availableCalls != 1 {
		t.Errorf("OnAvailable called %d times for one transition, want 1", availableCalls)
	}

	fake.set(func(f *fakeService) { f.model = "other@1" })
	status := monitor.Check(ctx)
	if status.State != StateMismatch || status.ServiceModel != "other@1" {
		t.Errorf("other model: %+v, want mismatch with the service's model", status)
	}

	fake.set(func(f *fakeService) { f.model = testModel; f.down = true })
	if got := monitor.Check(ctx).State; got != StateUnavailable {
		t.Errorf("service down: %s, want unavailable", got)
	}

	fake.set(func(f *fakeService) { f.down = false })
	monitor.Check(ctx)
	if availableCalls != 2 {
		t.Errorf("OnAvailable called %d times after the service came back, want 2", availableCalls)
	}

	if got := NewMonitor(NewClient(fake.server.URL, "wrong"), testModel).Check(ctx).State; got != StateUnavailable {
		t.Errorf("wrong token: %s, want unavailable", got)
	}
	if got := NewMonitor(nil, testModel).Check(ctx).State; got != StateDisabled {
		t.Errorf("no service: %s, want disabled", got)
	}
}

type embedEnv struct {
	service *Service
	queries *store.Queries
	fake    *fakeService
	owner   int64
}

func setupEmbed(t *testing.T) embedEnv {
	t.Helper()
	pool := testdb.New(t)
	queries := store.New(pool)
	layout := storage.Layout{Library: t.TempDir(), Cache: t.TempDir()}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	user, err := queries.CreateUser(context.Background(), store.CreateUserParams{Login: "anna", PasswordHash: "x", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeService(t)
	client := NewClient(fake.server.URL, "secret")
	service := &Service{
		Monitor: NewMonitor(client, testModel),
		Client:  client,
		Model:   testModel,
		Layout:  layout,
		Queries: queries,
	}
	service.Monitor.Check(context.Background())
	return embedEnv{service: service, queries: queries, fake: fake, owner: user.ID}
}

// addPhoto adds a photo with the given viewer preview, processed if ready.
func (e embedEnv) addPhoto(t *testing.T, name, preview string, ready bool) int64 {
	t.Helper()
	ctx := context.Background()
	hash := sha256.Sum256([]byte(name))
	photo, err := e.queries.InsertPhoto(ctx, store.InsertPhotoParams{
		OwnerID: e.owner, Sha256: hash[:], Mime: "image/jpeg", Size: 1, OriginalName: name,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		return photo.ID
	}
	format := "webp"
	if err := storage.WriteFileAtomic(e.service.Layout.Preview(photo.ID, storage.PreviewViewer, format), []byte(preview)); err != nil {
		t.Fatal(err)
	}
	if err := e.queries.MarkPhotoReady(ctx, store.MarkPhotoReadyParams{ID: photo.ID, PreviewFormat: &format, Exif: []byte("{}")}); err != nil {
		t.Fatal(err)
	}
	return photo.ID
}

func (e embedEnv) embedded(t *testing.T, photo int64) bool {
	t.Helper()
	has, err := e.queries.HasEmbedding(context.Background(), store.HasEmbeddingParams{PhotoID: photo, Model: testModel})
	if err != nil {
		t.Fatal(err)
	}
	return has
}

func (e embedEnv) timesSent(photo int64) int {
	e.fake.mu.Lock()
	defer e.fake.mu.Unlock()
	return e.fake.sent[fmt.Sprintf("%d.webp", photo)]
}

func TestEmbedPending(t *testing.T) {
	e := setupEmbed(t)
	ctx := context.Background()
	good := e.addPhoto(t, "good", "preview", true)
	bad := e.addPhoto(t, "bad", "bad", true)
	pending := e.addPhoto(t, "pending", "", false)

	done, err := e.service.EmbedPending(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if done != 1 || !e.embedded(t, good) || e.embedded(t, bad) || e.embedded(t, pending) {
		t.Errorf("first run: done %d, embedded good %v, bad %v, pending %v; want only good",
			done, e.embedded(t, good), e.embedded(t, bad), e.embedded(t, pending))
	}
	if e.timesSent(pending) != 0 {
		t.Error("an unprocessed photo was sent")
	}

	// The broken photo is retried until it has failed MaxAttempts times, and
	// the embedded one is not sent again.
	for range MaxAttempts + 2 {
		if _, err := e.service.EmbedPending(ctx, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.timesSent(bad); got != MaxAttempts {
		t.Errorf("broken photo sent %d times, want %d", got, MaxAttempts)
	}
	if got := e.timesSent(good); got != 1 {
		t.Errorf("embedded photo sent %d times, want 1", got)
	}
	counts, err := e.queries.CountEmbeddings(ctx, store.CountEmbeddingsParams{Model: testModel, MaxAttempts: MaxAttempts})
	if err != nil {
		t.Fatal(err)
	}
	if counts != (store.CountEmbeddingsRow{Photos: 2, Embedded: 1, Failed: 1}) {
		t.Errorf("counts = %+v, want 2 photos, 1 embedded, 1 failed", counts)
	}

	// A new model makes every photo work again.
	e.service.Model = "next@1"
	e.service.Monitor = NewMonitor(e.service.Client, "next@1")
	e.fake.set(func(f *fakeService) { f.model = "next@1" })
	e.service.Monitor.Check(ctx)
	if _, err := e.service.EmbedPending(ctx, time.Minute); err != nil {
		t.Fatal(err)
	}
	if got := e.timesSent(good); got != 2 {
		t.Errorf("after a model change the photo was sent %d times in all, want 2", got)
	}
}

func TestEmbedPendingWithoutService(t *testing.T) {
	e := setupEmbed(t)
	ctx := context.Background()
	photo := e.addPhoto(t, "good", "preview", true)

	// The service goes away after the last check: the run stops quietly, marks
	// it unavailable and blames no photo.
	e.fake.set(func(f *fakeService) { f.down = true })
	done, err := e.service.EmbedPending(ctx, time.Minute)
	if err != nil || done != 0 {
		t.Errorf("done %d, err %v; want 0 and no error", done, err)
	}
	if got := e.service.Monitor.Status().State; got != StateUnavailable {
		t.Errorf("state = %s, want unavailable", got)
	}
	failures, err := e.queries.ListEmbeddingFailures(ctx, store.ListEmbeddingFailuresParams{Model: testModel, MaxRows: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 0 {
		t.Errorf("failures = %+v, want none", failures)
	}

	// While unavailable, a run does not call the service at all.
	e.fake.set(func(f *fakeService) { f.down = false })
	if _, err := e.service.EmbedPending(ctx, time.Minute); err != nil {
		t.Fatal(err)
	}
	if e.embedded(t, photo) {
		t.Error("embedded while the monitor reported the service unavailable")
	}

	// The service now runs another model: nothing is stored.
	e.service.Monitor.Check(ctx)
	e.fake.set(func(f *fakeService) { f.model = "other@1" })
	if _, err := e.service.EmbedPending(ctx, time.Minute); err != nil {
		t.Fatal(err)
	}
	if e.embedded(t, photo) {
		t.Error("stored a vector of another model")
	}
	if got := e.service.Monitor.Status().State; got != StateMismatch {
		t.Errorf("state = %s, want mismatch", got)
	}
}

func TestVectorText(t *testing.T) {
	if got := VectorText([]float32{1, -0.5, 0.25}); got != "[1,-0.5,0.25]" {
		t.Errorf("VectorText = %s", got)
	}
	if !strings.HasPrefix(VectorText(nil), "[") {
		t.Error("an empty vector is not bracketed")
	}
}
