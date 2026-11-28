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
			json.NewEncoder(w).Encode(ml.Health{Faces: &ml.FaceModel{Model: "sface-test", Dimensions: 128, Scores: true}})
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
	model := ml.FaceModel{Model: "face-test", Dimensions: 128, Scores: true}
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

func TestFaceRetrySettingAndRegroup(t *testing.T) {
	s := setupSharing(t)
	photos := []int64{s.anna.addPhoto("landscape.jpg"), s.anna.addPhoto("landscape.png"), s.anna.addPhoto("landscape.webp")}
	for _, id := range photos {
		markReady(t, s.e, id)
	}
	vector := make([]float32, 128)
	vector[0] = 1
	first, retry := float32(0.95), float32(0.7)
	// The scan goes in photo id order: two faces of the first pass, then one
	// only the contrast retry found.
	answers := []ml.DetectedFace{
		{Box: []float64{0.1, 0.2, 0.2, 0.3}, Embedding: vector, Score: &first},
		{Box: []float64{0.1, 0.2, 0.2, 0.3}, Embedding: vector, Score: &first},
		{Box: []float64{0.1, 0.2, 0.2, 0.3}, Embedding: vector, Score: &retry, Retry: true},
	}
	floor := 0.6
	calls := 0
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(ml.Health{Faces: &ml.FaceModel{Model: "sface-test", Dimensions: 128, RetryFloor: &floor, Scores: true}})
			return
		}
		face := answers[calls]
		calls++
		json.NewEncoder(w).Encode(map[string]any{"model": "sface-test", "faces": []ml.DetectedFace{face}})
	}))
	defer service.Close()
	worker := ml.FaceWorker{Client: ml.NewClient(service.URL, "secret"), Layout: s.e.layout, Pool: s.e.pool}
	if err := worker.Work(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	type face struct {
		person *int64
		manual bool
		retry  bool
		score  *float32
	}
	faceOf := func(photo int64) face {
		t.Helper()
		var f face
		if err := s.e.pool.QueryRow(t.Context(), "SELECT person_id, manual, retry, score FROM photo_faces WHERE photo_id=$1", photo).
			Scan(&f.person, &f.manual, &f.retry, &f.score); err != nil {
			t.Fatal(err)
		}
		return f
	}
	a, b, c := faceOf(photos[0]), faceOf(photos[1]), faceOf(photos[2])
	if a.person == nil || b.person == nil || *a.person != *b.person {
		t.Fatalf("first-pass faces not in one group: %v %v", a.person, b.person)
	}
	if c.person != nil || !c.retry || c.score == nil || *c.score != retry {
		t.Fatalf("retry face under the setting: %+v", c)
	}

	r := s.root.get("/api/v1/settings/faces")
	wantStatus(t, r, 200)
	settings := decode[faceSettingsJSON](t, r)
	if settings.RetryMinScore != ml.DefaultRetryMinScore {
		t.Fatalf("default setting %v", settings.RetryMinScore)
	}
	wantStatus(t, s.anna.json("PUT", "/api/v1/settings/faces", map[string]any{"retryMinScore": 0.65}), 403)
	wantStatus(t, s.root.json("PUT", "/api/v1/settings/faces", map[string]any{"retryMinScore": 1}), 400)
	wantStatus(t, s.root.json("PUT", "/api/v1/settings/faces", map[string]any{"retryMinScore": 0.65}), 200)

	var annaID int64
	if err := s.e.pool.QueryRow(t.Context(), "SELECT id FROM users WHERE login='anna'").Scan(&annaID); err != nil {
		t.Fatal(err)
	}
	regroup := func() {
		t.Helper()
		if err := ml.Regroup(t.Context(), s.e.pool, annaID); err != nil {
			t.Fatal(err)
		}
	}
	regroup()
	c = faceOf(photos[2])
	if c.person == nil || *c.person != *faceOf(photos[0]).person {
		t.Fatalf("retry face did not join after lowering the setting: %v", c.person)
	}

	// An ignored face stays ignored, and a named group keeps its faces, when
	// the setting is raised again and the faces are regrouped.
	group := *faceOf(photos[0]).person
	var faceB int64
	if err := s.e.pool.QueryRow(t.Context(), "SELECT id FROM photo_faces WHERE photo_id=$1", photos[1]).Scan(&faceB); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, s.anna.json("POST", "/api/v1/people", map[string]any{"action": "ignore", "person": group, "face": faceB}), 204)
	wantStatus(t, s.anna.json("POST", "/api/v1/people", map[string]any{"action": "rename", "person": group, "name": "Grandma"}), 204)
	wantStatus(t, s.root.json("PUT", "/api/v1/settings/faces", map[string]any{"retryMinScore": 0.75}), 200)
	regroup()
	if b := faceOf(photos[1]); b.person != nil || !b.manual {
		t.Fatalf("ignored face changed: %+v", b)
	}
	if c := faceOf(photos[2]); c.person == nil || *c.person != group {
		t.Fatalf("face of a named group was regrouped: %v", c.person)
	}
}

func TestFaceRescanAfterRotation(t *testing.T) {
	s := setupSharing(t)
	ctx := t.Context()
	photo := s.anna.addPhoto("landscape.jpg")
	markReady(t, s.e, photo)
	axis := func(i int) []float32 {
		v := make([]float32, 128)
		v[i] = 1
		return v
	}
	score := float32(0.95)
	face := func(box []float64, v []float32) ml.DetectedFace {
		return ml.DetectedFace{Box: box, Embedding: v, Score: &score}
	}
	var answer []ml.DetectedFace
	status := http.StatusOK
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(ml.Health{Faces: &ml.FaceModel{Model: "sface-test", Dimensions: 128, Scores: true}})
			return
		}
		if status != http.StatusOK {
			http.Error(w, "unsupported preview", status)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "sface-test", "faces": answer})
	}))
	defer service.Close()
	worker := ml.FaceWorker{Client: ml.NewClient(service.URL, "secret"), Layout: s.e.layout, Pool: s.e.pool}
	scan := func() {
		t.Helper()
		if err := worker.Work(ctx, nil); err != nil {
			t.Fatal(err)
		}
	}
	type stored struct {
		id        int64
		person    *int64
		unmatched bool
		box       []float32
	}
	faces := func() []stored {
		t.Helper()
		rows, err := s.e.pool.Query(ctx, "SELECT id, person_id, unmatched, box FROM photo_faces WHERE photo_id=$1 ORDER BY id", photo)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []stored
		for rows.Next() {
			var f stored
			if err := rows.Scan(&f.id, &f.person, &f.unmatched, &f.box); err != nil {
				t.Fatal(err)
			}
			out = append(out, f)
		}
		return out
	}
	rescan := func() {
		t.Helper()
		if _, err := s.e.pool.Exec(ctx, "UPDATE face_scans SET rescan=true WHERE photo_id=$1", photo); err != nil {
			t.Fatal(err)
		}
		scan()
		var pending bool
		if err := s.e.pool.QueryRow(ctx, "SELECT rescan FROM face_scans WHERE photo_id=$1", photo).Scan(&pending); err != nil || pending {
			t.Fatalf("rescan still pending: %v %v", pending, err)
		}
	}

	answer = []ml.DetectedFace{face([]float64{0.1, 0.1, 0.2, 0.2}, axis(0)), face([]float64{0.6, 0.6, 0.2, 0.2}, axis(1))}
	scan()
	before := faces()
	if len(before) != 2 {
		t.Fatalf("first scan stored %d faces", len(before))
	}
	named := before[0]
	if _, err := s.e.pool.Exec(ctx, "UPDATE people SET name='Grandma' WHERE id=$1", *named.person); err != nil {
		t.Fatal(err)
	}

	// The named face is found again a little apart, the other one is not,
	// and a face shows up that the photo on its side hid.
	answer = []ml.DetectedFace{face([]float64{0.12, 0.1, 0.2, 0.2}, axis(0)), face([]float64{0.4, 0.1, 0.1, 0.1}, axis(2))}
	rescan()
	after := faces()
	if len(after) != 2 || after[0].id != named.id || *after[0].person != *named.person || after[0].box[0] < 0.11 {
		t.Fatalf("named face after the rescan: %+v", after)
	}
	if after[1].id == before[1].id || after[1].person == nil {
		t.Fatalf("unmatched automatic face kept or new face ungrouped: %+v", after)
	}

	// A named face the rescan does not find again stays, marked unmatched.
	answer = []ml.DetectedFace{}
	rescan()
	after = faces()
	if len(after) != 1 || after[0].id != named.id || !after[0].unmatched {
		t.Fatalf("after an empty rescan: %+v", after)
	}
	r := s.root.get("/api/v1/ml/faces/" + itoa(photo))
	wantStatus(t, r, 200)
	if lookup := decode[ml.PhotoFaceScan](t, r); lookup.Unmatched != 1 || lookup.State != "completed" {
		t.Fatalf("scan lookup %+v", lookup)
	}

	// A photo error during a rescan keeps the faces.
	status = http.StatusUnprocessableEntity
	rescan()
	if got := faces(); len(got) != 1 || got[0].id != named.id {
		t.Fatalf("faces after a failed rescan: %+v", got)
	}
}

func TestFaceScanOfAPhotoTurnedMeanwhile(t *testing.T) {
	s := setupSharing(t)
	ctx := t.Context()
	photo := s.anna.addPhoto("landscape.jpg")
	markReady(t, s.e, photo)
	v := make([]float32, 128)
	v[0] = 1
	score := float32(0.95)
	calls := 0
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(ml.Health{Faces: &ml.FaceModel{Model: "sface-test", Dimensions: 128, Scores: true}})
			return
		}
		calls++
		box := []float64{0.1, 0.1, 0.2, 0.2}
		if calls == 1 {
			// The owner turns the photo while its old preview is with the service.
			if _, err := s.e.pool.Exec(ctx, "UPDATE photos SET rotation=90, preview_rotation=90, preview_version=preview_version+1 WHERE id=$1", photo); err != nil {
				t.Error(err)
			}
			box = []float64{0.6, 0.6, 0.2, 0.2}
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "sface-test", "faces": []ml.DetectedFace{{Box: box, Embedding: v, Score: &score}}})
	}))
	defer service.Close()
	worker := ml.FaceWorker{Client: ml.NewClient(service.URL, "secret"), Layout: s.e.layout, Pool: s.e.pool}
	if err := worker.Work(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("face calls %d, want 2", calls)
	}
	var boxes [][]float32
	rows, err := s.e.pool.Query(ctx, "SELECT box FROM photo_faces WHERE photo_id=$1", photo)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var b []float32
		if err := rows.Scan(&b); err != nil {
			t.Fatal(err)
		}
		boxes = append(boxes, b)
	}
	if len(boxes) != 1 || boxes[0][0] > 0.2 {
		t.Fatalf("faces from the old preview were kept: %v", boxes)
	}
}
