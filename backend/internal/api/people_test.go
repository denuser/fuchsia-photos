package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/magellanica-app/magellanica-photos/backend/internal/ml"
	"github.com/magellanica-app/magellanica-photos/backend/internal/storage"
)

func TestPeopleIndexAndCorrections(t *testing.T) {
	s := setupSharing(t)
	a := s.anna.addPhoto("landscape.jpg")
	b := s.anna.addPhoto("landscape.png")
	for _, id := range []int64{a, b} {
		markReady(t, s.e, id)
	}
	vector := make([]float32, 128)
	vector[0] = 1
	calls := 0
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(ml.Health{Faces: &ml.FaceModel{Model: "sface-test", Dimensions: 128}})
			return
		}
		calls++
		json.NewEncoder(w).Encode(map[string]any{"model": "sface-test", "faces": []ml.DetectedFace{{Box: []float64{0.1, 0.2, 0.2, 0.3}, Embedding: vector}}})
	}))
	defer service.Close()
	worker := ml.FaceWorker{Client: ml.NewClient(service.URL, "secret"), Layout: s.e.layout, Pool: s.e.pool}
	scan := func() {
		t.Helper()
		if err := worker.Work(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
	}
	scan()
	scan()
	if calls != 2 {
		t.Fatalf("face calls %d, want 2", calls)
	}
	list := func(user *browser, path string) []personItem {
		t.Helper()
		r := user.get("/api/v1/people" + path)
		wantStatus(t, r, 200)
		return decode[[]personItem](t, r)
	}
	groups := list(s.anna, "")
	if len(groups) != 1 || len(list(s.vera, "")) != 0 {
		t.Fatalf("unexpected groups %v", groups)
	}
	id := groups[0].ID
	path := fmt.Sprintf("?person=%d", id)
	faces := list(s.anna, path)
	if len(faces) != 2 || len(list(s.vera, path)) != 0 {
		t.Fatal("unexpected members")
	}
	edit := func(user *browser, body map[string]any, status int) {
		t.Helper()
		wantStatus(t, user.json("POST", "/api/v1/people", body), status)
	}
	rename := map[string]any{"action": "rename", "person": id, "name": "Family"}
	edit(s.vera, rename, 404)
	edit(s.anna, rename, 204)
	edit(s.anna, map[string]any{"action": "move", "person": id, "face": faces[0].ID}, 204)
	groups = list(s.anna, "")
	if len(groups) != 2 {
		t.Fatal("move did not split group")
	}
	edit(s.anna, map[string]any{"action": "merge", "person": groups[1].ID, "target": id}, 204)
	edit(s.anna, map[string]any{"action": "ignore", "person": id, "face": faces[0].ID}, 204)
	scan()
	if len(list(s.anna, path)) != 1 || list(s.anna, "")[0].Name != "Family" {
		t.Fatal("manual changes lost")
	}
	if _, err := s.e.pool.Exec(t.Context(), "UPDATE photos SET deleted_at=now() WHERE id=$1", faces[1].Photo); err != nil {
		t.Fatal(err)
	}
	if len(list(s.anna, "")) != 0 {
		t.Fatal("trashed cover visible")
	}
	if _, err := s.e.pool.Exec(t.Context(), "UPDATE photos SET deleted_at=NULL WHERE id=$1", faces[1].Photo); err != nil {
		t.Fatal(err)
	}
	if len(list(s.anna, path)) != 1 {
		t.Fatal("restore lost face")
	}
}

func TestFaceScanStatusAndBackfill(t *testing.T) {
	s := setupSharing(t)
	ctx := t.Context()
	raw := s.anna.addPhoto("rotated.dng")
	rejected := s.anna.addPhoto("landscape.png")
	hidden := s.anna.addPhoto("rotated-preview-only.dng")
	jpeg := s.anna.addPhoto("rotated.jpg")
	for id, preview := range map[int64]string{raw: "standalone raw preview", rejected: "rejected preview", hidden: "hidden raw preview", jpeg: "paired jpeg preview"} {
		markReady(t, s.e, id)
		if err := storage.WriteFileAtomic(s.e.layout.Preview(id, storage.PreviewViewer, "webp"), []byte(preview)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.e.pool.Exec(ctx, "INSERT INTO photo_links(hidden_id,shown_id,kind) VALUES($1,$2,'raw')", hidden, jpeg); err != nil {
		t.Fatal(err)
	}
	lookup := func(id int64, state string) ml.PhotoFaceScan {
		t.Helper()
		r := s.root.get("/api/v1/ml/faces/" + itoa(id))
		wantStatus(t, r, http.StatusOK)
		got := decode[ml.PhotoFaceScan](t, r)
		if got.PhotoID != id || got.State != state {
			t.Fatalf("scan for %d = %+v, want %s", id, got, state)
		}
		return got
	}
	wantStatus(t, s.anna.get("/api/v1/ml/faces/"+itoa(raw)), http.StatusForbidden)
	wantStatus(t, s.root.get("/api/v1/ml/faces/999999"), http.StatusNotFound)
	lookup(raw, "waiting")
	lookup(hidden, "ineligible")

	var mu sync.Mutex
	var sent []string
	serviceFailure := http.StatusServiceUnavailable
	model := ml.FaceModel{Model: "face-test", Dimensions: 128}
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(ml.Health{Faces: &model})
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			http.Error(w, "cannot read preview", http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, string(body))
		if serviceFailure != 0 {
			http.Error(w, "offline", serviceFailure)
			return
		}
		if string(body) == "rejected preview" {
			http.Error(w, "cannot decode preview", http.StatusUnprocessableEntity)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"model": model.Model, "faces": []ml.DetectedFace{}})
	}))
	defer service.Close()
	client := ml.NewClient(service.URL, "secret")
	monitor := ml.NewMonitor(client, "")
	monitor.Check(ctx)
	worker := ml.FaceWorker{Client: client, Monitor: monitor, Pool: s.e.pool, Layout: s.e.layout}
	run := func() {
		t.Helper()
		if err := worker.Work(ctx, nil); err != nil {
			t.Fatal(err)
		}
	}
	run()
	lookup(raw, "waiting")
	if monitor.FacesAvailable() {
		t.Fatal("face scans still report available after the service failed")
	}
	mu.Lock()
	serviceFailure = http.StatusInternalServerError
	mu.Unlock()
	monitor.Check(ctx)
	run()
	lookup(raw, "waiting")
	mu.Lock()
	serviceFailure = 0
	mu.Unlock()
	monitor.Check(ctx)
	run()
	run()
	if scan := lookup(raw, "completed"); scan.Faces != 0 || scan.ScannedAt == nil || scan.Error != "" {
		t.Fatalf("zero-face scan = %+v", scan)
	}
	if scan := lookup(rejected, "error"); scan.Error == "" {
		t.Fatal("the rejected preview has no recorded error")
	}
	lookup(jpeg, "completed")
	lookup(hidden, "ineligible")
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"standalone raw preview", "standalone raw preview", "standalone raw preview", "rejected preview", "paired jpeg preview"}; !slices.Equal(sent, want) {
		t.Errorf("ML received %q, want %q", sent, want)
	}
}
