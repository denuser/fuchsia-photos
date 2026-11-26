package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/magellanica-app/magellanica-photos/backend/internal/ml"
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
