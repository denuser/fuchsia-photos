package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/ml"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const searchModel = "test-model@1"

type searchResults struct {
	Items     []photoJSON `json:"items"`
	Distances []float64   `json:"distances"`
}

// setupSearch starts the API with a fake ML service whose text vector is
// [1,0,0], so a photo's distance to any query follows from its own vector.
func setupSearch(t *testing.T) (sharingEnv, *ml.Service) {
	t.Helper()
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			json.NewEncoder(w).Encode(ml.Health{Model: searchModel, Dimensions: 3})
		case "/embed/text":
			json.NewEncoder(w).Encode(map[string]any{"model": searchModel, "embedding": []float32{1, 0, 0}, "minSimilarity": 0.5})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fake.Close)
	client := ml.NewClient(fake.URL, "")
	service := &ml.Service{Monitor: ml.NewMonitor(client, searchModel), Client: client, Model: searchModel}
	service.Monitor.Check(context.Background())

	e := setupWith(t, func(d *Deps) { d.ML = service })
	e.addUser("anna", auth.RoleUser)
	borisID := e.addUser("boris", auth.RoleUser).ID
	veraID := e.addUser("vera", auth.RoleUser).ID
	e.addUser("root", auth.RoleAdmin)
	return sharingEnv{
		e: e, anna: e.loggedIn("anna"), boris: e.loggedIn("boris"), vera: e.loggedIn("vera"), root: e.loggedIn("root"),
		borisID: borisID, veraID: veraID,
	}, service
}

// embeddedPhoto uploads a processed photo with the given vector.
func embeddedPhoto(t *testing.T, s sharingEnv, b *browser, fixture string, vector []float32) int64 {
	t.Helper()
	photo := b.addPhoto(fixture)
	markReady(t, s.e, photo)
	err := s.e.queries.SaveEmbedding(context.Background(), store.SaveEmbeddingParams{
		PhotoID: photo, Model: searchModel, Embedding: ml.VectorText(vector),
	})
	if err != nil {
		t.Fatal(err)
	}
	return photo
}

func resultIDs(t *testing.T, b *browser, path string) []int64 {
	t.Helper()
	res := b.get(path)
	wantStatus(t, res, http.StatusOK)
	results := decode[searchResults](t, res)
	if len(results.Distances) != len(results.Items) {
		t.Errorf("%s: %d distances for %d photos", path, len(results.Distances), len(results.Items))
	}
	out := make([]int64, len(results.Items))
	for i, p := range results.Items {
		out[i] = p.ID
	}
	return out
}

func TestSearchFollowsPermissions(t *testing.T) {
	s, _ := setupSearch(t)
	near := embeddedPhoto(t, s, s.anna, "landscape.jpg", []float32{1, 0, 0})
	far := embeddedPhoto(t, s, s.anna, "rotated.jpg", []float32{1, 0.9, 0})
	private := embeddedPhoto(t, s, s.anna, "landscape.png", []float32{1, 0.1, 0})
	trashed := embeddedPhoto(t, s, s.anna, "landscape.webp", []float32{1, 0, 0.1})
	sharePhoto(t, s.anna, near, map[string]any{"add": []int64{s.borisID}})
	shareAlbum(t, s.anna, s.anna.newAlbum(t, "Trip", far), map[string]any{"add": []int64{s.borisID}})
	wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/photos/trash", ids(trashed)), http.StatusNoContent)

	if got, want := resultIDs(t, s.anna, "/api/v1/search?q=sea"), []int64{near, private, far}; !slices.Equal(got, want) {
		t.Errorf("anna finds %v, want %v nearest first, without the trash", got, want)
	}
	if got, want := resultIDs(t, s.boris, "/api/v1/search?q=sea"), []int64{near, far}; !slices.Equal(got, want) {
		t.Errorf("boris finds %v, want only what is shared with him: %v", got, want)
	}
	if got := resultIDs(t, s.vera, "/api/v1/search?q=sea"); len(got) != 0 {
		t.Errorf("vera finds %v, want nothing", got)
	}
	// An administrator searches like any other user.
	if got := resultIDs(t, s.root, "/api/v1/search?q=sea"); len(got) != 0 {
		t.Errorf("the administrator finds %v, want nothing", got)
	}
	if got := resultIDs(t, s.anna, "/api/v1/search?q=sea&limit=1"); !slices.Equal(got, []int64{near}) {
		t.Errorf("with limit 1: %v", got)
	}
	wantStatus(t, s.anna.get("/api/v1/search?q=%20"), http.StatusBadRequest)
	wantStatus(t, s.anna.get("/api/v1/search?q=sea&limit=0"), http.StatusBadRequest)
}

func TestSimilarPhotos(t *testing.T) {
	s, _ := setupSearch(t)
	shared := embeddedPhoto(t, s, s.anna, "landscape.jpg", []float32{1, 0, 0})
	private := embeddedPhoto(t, s, s.anna, "landscape.png", []float32{1, 0.1, 0})
	other := embeddedPhoto(t, s, s.anna, "rotated.jpg", []float32{0, 1, 0})
	sharePhoto(t, s.anna, shared, map[string]any{"add": []int64{s.borisID}})
	sharePhoto(t, s.anna, other, map[string]any{"add": []int64{s.borisID}})

	if got, want := resultIDs(t, s.anna, "/api/v1/photos/"+itoa(shared)+"/similar"), []int64{private, other}; !slices.Equal(got, want) {
		t.Errorf("anna: %v, want %v without the photo itself", got, want)
	}
	if got, want := resultIDs(t, s.boris, "/api/v1/photos/"+itoa(shared)+"/similar"), []int64{other}; !slices.Equal(got, want) {
		t.Errorf("boris: %v, want %v", got, want)
	}
	wantStatus(t, s.boris.get("/api/v1/photos/"+itoa(private)+"/similar"), http.StatusNotFound)
}

func TestSearchWithoutService(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("root", auth.RoleAdmin)
	anna := e.loggedIn("anna")

	wantStatus(t, anna.get("/api/v1/search?q=sea"), http.StatusServiceUnavailable)
	status := decode[map[string]any](t, anna.get("/api/v1/search/status"))
	if status["available"] != false || status["state"] != "disabled" {
		t.Errorf("status = %v, want unavailable and disabled", status)
	}
	// Similar photos need only the stored vectors.
	photo := anna.addPhoto("landscape.jpg")
	markReady(t, e, photo)
	wantStatus(t, anna.get("/api/v1/photos/"+itoa(photo)+"/similar"), http.StatusOK)

	wantStatus(t, anna.get("/api/v1/ml"), http.StatusForbidden)
	wantStatus(t, e.loggedIn("root").get("/api/v1/ml"), http.StatusOK)
}

func TestPurgeRemovesEmbeddings(t *testing.T) {
	s, _ := setupSearch(t)
	photo := embeddedPhoto(t, s, s.anna, "landscape.jpg", []float32{1, 0, 0})
	wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/photos/trash", ids(photo)), http.StatusNoContent)
	wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/photos/purge", ids(photo)), http.StatusNoContent)

	has, err := s.e.queries.HasEmbedding(context.Background(), store.HasEmbeddingParams{PhotoID: photo, Model: searchModel})
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("a permanently deleted photo kept its vector")
	}
}

func TestSearchLeavesOutPhotosBelowTheThreshold(t *testing.T) {
	s, _ := setupSearch(t)
	match := embeddedPhoto(t, s, s.anna, "landscape.jpg", []float32{1, 0.5, 0})
	// Cosine similarity 0.45 to the query, below the service's 0.5.
	embeddedPhoto(t, s, s.anna, "rotated.jpg", []float32{0.45, 0.893, 0})

	if got := resultIDs(t, s.anna, "/api/v1/search?q=sea"); !slices.Equal(got, []int64{match}) {
		t.Errorf("found %v, want only %d", got, match)
	}
	// Similar photos have no threshold: they are ordered, not judged.
	if got := resultIDs(t, s.anna, "/api/v1/photos/"+itoa(match)+"/similar"); len(got) != 1 {
		t.Errorf("similar photos: %v, want the other photo", got)
	}
}
