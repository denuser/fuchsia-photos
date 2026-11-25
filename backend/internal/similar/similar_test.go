package similar

import (
	"bytes"
	"context"
	"crypto/sha256"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/magellanica-app/magellanica-photos/backend/internal/media"
	"github.com/magellanica-app/magellanica-photos/backend/internal/storage"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
	"github.com/magellanica-app/magellanica-photos/backend/internal/testdb"
)

const testModel = "test-model@1"

func TestMain(m *testing.M) {
	if err := media.Startup(); err != nil {
		panic(err)
	}
	code := m.Run()
	media.Shutdown()
	os.Exit(code)
}

func TestClassify(t *testing.T) {
	at := time.Date(2026, 11, 9, 8, 21, 41, 0, time.UTC)
	later := func(d time.Duration) *time.Time { v := at.Add(d); return &v }
	hash := func(v int64) *int64 { return &v }

	tests := []struct {
		name             string
		row              store.FindSimilarCandidatesRow
		tier             string
		ok               bool
		bits             int
		hasBits, hasTime bool
	}{
		{"copy by hash", store.FindSimilarCandidatesRow{Distance: 0.005, Phash: hash(0), OtherPhash: hash(0b111), TakenAt: &at, OtherTakenAt: later(time.Hour)},
			TierDuplicate, true, 3, true, true},
		{"frame by time", store.FindSimilarCandidatesRow{Distance: 0.011, Phash: hash(0), OtherPhash: hash(-1), TakenAt: &at, OtherTakenAt: later(2 * time.Second)},
			TierDuplicate, true, 64, true, true},
		{"another shot of a place", store.FindSimilarCandidatesRow{Distance: 0.011, Phash: hash(0), OtherPhash: hash(-1), TakenAt: &at, OtherTakenAt: later(40 * time.Minute)},
			TierSimilar, true, 64, true, true},
		{"close but no second reason", store.FindSimilarCandidatesRow{Distance: 0.011},
			TierSimilar, true, 0, false, false},
		{"too far for a duplicate", store.FindSimilarCandidatesRow{Distance: 0.019, Phash: hash(5), OtherPhash: hash(5), TakenAt: &at, OtherTakenAt: &at},
			TierSimilar, true, 0, true, true},
		{"a hash of six bits is still a copy", store.FindSimilarCandidatesRow{Distance: 0.01, Phash: hash(0), OtherPhash: hash(0b111111)},
			TierDuplicate, true, 6, true, false},
		{"seven bits is not", store.FindSimilarCandidatesRow{Distance: 0.01, Phash: hash(0), OtherPhash: hash(0b1111111)},
			TierSimilar, true, 7, true, false},
		{"three seconds is a frame", store.FindSimilarCandidatesRow{Distance: 0.01, TakenAt: &at, OtherTakenAt: later(-3 * time.Second)},
			TierDuplicate, true, 0, false, true},
		{"too far for anything", store.FindSimilarCandidatesRow{Distance: 0.031, Phash: hash(0), OtherPhash: hash(0), TakenAt: &at, OtherTakenAt: &at},
			"", false, 0, true, true},
		{"one capture time is missing", store.FindSimilarCandidatesRow{Distance: 0.01, TakenAt: &at},
			TierSimilar, true, 0, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, hashBits, seconds, ok := Classify(tt.row)
			if tier != tt.tier || ok != tt.ok {
				t.Fatalf("tier %q, ok %v; want %q, %v", tier, ok, tt.tier, tt.ok)
			}
			if (hashBits != nil) != tt.hasBits || (hashBits != nil && int(*hashBits) != tt.bits) {
				t.Errorf("hash bits %v, want present %v with %d", hashBits, tt.hasBits, tt.bits)
			}
			if (seconds != nil) != tt.hasTime {
				t.Errorf("seconds %v, want present %v", seconds, tt.hasTime)
			}
		})
	}
}

type env struct {
	pool    *pgxpool.Pool
	queries *store.Queries
	service *Service
}

func setup(t *testing.T) env {
	t.Helper()
	pool := testdb.New(t)
	layout := storage.Layout{Library: t.TempDir(), Cache: t.TempDir()}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	return env{pool: pool, queries: store.New(pool), service: &Service{Model: testModel, Layout: layout, Pool: pool}}
}

func (e env) user(t *testing.T, login string) int64 {
	t.Helper()
	user, err := e.queries.CreateUser(context.Background(), store.CreateUserParams{Login: login, PasswordHash: "x", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	return user.ID
}

type photoSpec struct {
	vector string // such as "[1,0,0]"
	hash   *int64
	taken  *time.Time
}

// photo adds a processed photo with a vector, a hash and a capture time.
func (e env) photo(t *testing.T, owner int64, name string, spec photoSpec) int64 {
	t.Helper()
	ctx := context.Background()
	sum := sha256.Sum256([]byte(name))
	photo, err := e.queries.InsertPhoto(ctx, store.InsertPhotoParams{
		OwnerID: owner, Sha256: sum[:], Mime: "image/jpeg", Size: 1, OriginalName: name,
	})
	if err != nil {
		t.Fatal(err)
	}
	format := "webp"
	if err := e.queries.MarkPhotoReady(ctx, store.MarkPhotoReadyParams{ID: photo.ID, PreviewFormat: &format, Exif: []byte("{}"), TakenAt: spec.taken}); err != nil {
		t.Fatal(err)
	}
	if spec.hash != nil {
		if err := e.queries.SavePerceptualHash(ctx, store.SavePerceptualHashParams{ID: photo.ID, Phash: spec.hash}); err != nil {
			t.Fatal(err)
		}
	}
	e.embed(t, photo.ID, spec.vector)
	return photo.ID
}

func (e env) embed(t *testing.T, photo int64, vector string) {
	t.Helper()
	if err := e.queries.SaveEmbedding(context.Background(), store.SaveEmbeddingParams{PhotoID: photo, Model: testModel, Embedding: vector}); err != nil {
		t.Fatal(err)
	}
}

func (e env) pairs(t *testing.T, owner int64, tier string) [][2]int64 {
	t.Helper()
	rows, err := e.queries.ListSimilarPairs(context.Background(), store.ListSimilarPairsParams{OwnerID: owner, Tier: tier})
	if err != nil {
		t.Fatal(err)
	}
	var out [][2]int64
	for _, r := range rows {
		out = append(out, [2]int64{r.PhotoA, r.PhotoB})
	}
	return out
}

func (e env) run(t *testing.T) {
	t.Helper()
	if err := e.service.Run(context.Background(), time.Minute); err != nil {
		t.Fatal(err)
	}
}

func i64(v int64) *int64 { return &v }

func TestRunFindsPairs(t *testing.T) {
	e := setup(t)
	at := time.Date(2026, 11, 9, 8, 21, 41, 0, time.UTC)
	later := func(d time.Duration) *time.Time { v := at.Add(d); return &v }
	owner := e.user(t, "anna")

	// A vector 0.011 from the first and 0.019 from the second, as measured on real photos.
	near, further, far := "[1,0.15,0]", "[1,0.2,0]", "[0,1,0]"
	original := e.photo(t, owner, "original", photoSpec{vector: "[1,0,0]", hash: i64(0), taken: &at})
	resized := e.photo(t, owner, "resized", photoSpec{vector: near, hash: i64(0b101), taken: later(time.Hour)})
	frame := e.photo(t, owner, "frame", photoSpec{vector: further, hash: i64(-1), taken: later(2 * time.Second)})
	elsewhere := e.photo(t, owner, "elsewhere", photoSpec{vector: far, hash: i64(0), taken: &at})

	// Another owner's photo with the same vector pairs with nothing of anna's.
	bob := e.user(t, "bob")
	e.photo(t, bob, "bob's", photoSpec{vector: "[1,0,0]", hash: i64(0), taken: &at})

	e.run(t)

	duplicates := e.pairs(t, owner, TierDuplicate)
	if len(duplicates) != 1 || duplicates[0] != [2]int64{original, resized} {
		t.Errorf("duplicates %v, want only the resized copy %v", duplicates, [2]int64{original, resized})
	}
	similar := map[[2]int64]bool{}
	for _, p := range e.pairs(t, owner, TierSimilar) {
		similar[p] = true
	}
	// The frame is 0.019 from the original, too far for a duplicate, and 0.001 from the
	// resized copy, an hour away from it with a hash far from it: only similar.
	if !similar[[2]int64{original, frame}] {
		t.Errorf("similar %v, want the original and the frame", similar)
	}
	for p := range similar {
		if p[0] == elsewhere || p[1] == elsewhere {
			t.Errorf("a photo of another scene is in a pair: %v", p)
		}
	}
	if got := e.pairs(t, bob, TierSimilar); len(got) != 0 {
		t.Errorf("bob has pairs %v, want none", got)
	}
	if got := e.pairs(t, bob, TierDuplicate); len(got) != 0 {
		t.Errorf("bob has duplicate pairs %v, want none", got)
	}
}

func TestRunLooksOnce(t *testing.T) {
	e := setup(t)
	owner := e.user(t, "anna")
	a := e.photo(t, owner, "a", photoSpec{vector: "[1,0,0]", hash: i64(0)})
	b := e.photo(t, owner, "b", photoSpec{vector: "[1,0.15,0]", hash: i64(1)})
	e.run(t)

	waiting, err := e.queries.ListPhotosToCheck(context.Background(), store.ListPhotosToCheckParams{Model: testModel, MaxRows: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(waiting) != 0 {
		t.Errorf("%d photos wait after a run, want none", len(waiting))
	}
	if got := e.pairs(t, owner, TierDuplicate); len(got) != 1 || got[0] != [2]int64{a, b} {
		t.Errorf("duplicates %v, want the pair of %d and %d", got, a, b)
	}

	// A new vector for a photo is a reason to look at it again.
	time.Sleep(10 * time.Millisecond)
	e.embed(t, b, "[0,1,0]")
	waiting, err = e.queries.ListPhotosToCheck(context.Background(), store.ListPhotosToCheckParams{Model: testModel, MaxRows: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(waiting) != 1 || waiting[0].PhotoID != b {
		t.Fatalf("waiting after a new vector: %v, want photo %d", waiting, b)
	}
	e.run(t)
	if got := e.pairs(t, owner, TierDuplicate); len(got) != 0 {
		t.Errorf("duplicates %v after the vector moved away, want none", got)
	}
}

func TestDismissedPairStays(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	owner := e.user(t, "anna")
	a := e.photo(t, owner, "a", photoSpec{vector: "[1,0,0]", hash: i64(0)})
	b := e.photo(t, owner, "b", photoSpec{vector: "[1,0.15,0]", hash: i64(1)})
	e.run(t)

	n, err := e.queries.DismissSimilarPairs(ctx, store.DismissSimilarPairsParams{OwnerID: owner, Ids: []int64{a, b}})
	if err != nil || n != 1 {
		t.Fatalf("dismiss: %d pairs, %v; want 1", n, err)
	}
	if got := e.pairs(t, owner, TierDuplicate); len(got) != 0 {
		t.Errorf("duplicates %v after dismissing, want none", got)
	}

	// A new vector, which makes the search look again, does not bring the pair back.
	time.Sleep(10 * time.Millisecond)
	e.embed(t, a, "[1,0.01,0]")
	e.run(t)
	if got := e.pairs(t, owner, TierDuplicate); len(got) != 0 {
		t.Errorf("duplicates %v after another look, want none", got)
	}

	// Another owner cannot dismiss it.
	bob := e.user(t, "bob")
	if n, err := e.queries.DismissSimilarPairs(ctx, store.DismissSimilarPairsParams{OwnerID: bob, Ids: []int64{a, b}}); err != nil || n != 0 {
		t.Errorf("another owner dismissed %d pairs, %v; want 0", n, err)
	}
}

func TestTrashedAndHiddenPhotosAreLeftOut(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	owner := e.user(t, "anna")
	a := e.photo(t, owner, "a", photoSpec{vector: "[1,0,0]", hash: i64(0)})
	trashed := e.photo(t, owner, "trashed", photoSpec{vector: "[1,0.15,0]", hash: i64(0)})
	hidden := e.photo(t, owner, "hidden", photoSpec{vector: "[1,0.15,0]", hash: i64(0)})
	if err := e.queries.TrashPhotos(ctx, store.TrashPhotosParams{Ids: []int64{trashed}, ActorID: owner}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO photo_links (hidden_id, shown_id, kind) VALUES ($1, $2, 'edited')`, hidden, a); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	for _, tier := range []string{TierDuplicate, TierSimilar} {
		if got := e.pairs(t, owner, tier); len(got) != 0 {
			t.Errorf("%s pairs %v, want none while the other photos are in the trash or hidden", tier, got)
		}
	}
}

func TestRunHashesPreviews(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	owner := e.user(t, "anna")
	sum := sha256.Sum256([]byte("hashed"))
	photo, err := e.queries.InsertPhoto(ctx, store.InsertPhotoParams{OwnerID: owner, Sha256: sum[:], Mime: "image/jpeg", Size: 1, OriginalName: "hashed"})
	if err != nil {
		t.Fatal(err)
	}
	format := "webp"
	if err := e.queries.MarkPhotoReady(ctx, store.MarkPhotoReadyParams{ID: photo.ID, PreviewFormat: &format, Exif: []byte("{}")}); err != nil {
		t.Fatal(err)
	}
	// A gradient from dark to bright, left to right, stored under the viewer preview's name.
	var data bytes.Buffer
	img := image.NewGray(image.Rect(0, 0, 90, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 90; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8(x * 255 / 89)})
		}
	}
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteFileAtomic(e.service.Layout.Preview(photo.ID, storage.PreviewViewer, format), data.Bytes()); err != nil {
		t.Fatal(err)
	}
	e.run(t)

	stored, err := e.queries.GetPhoto(ctx, photo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Phash == nil {
		t.Fatal("the preview was not hashed")
	}
	// Every pixel is darker than its right neighbour's: no bit is set.
	if *stored.Phash != 0 {
		t.Errorf("hash %064b, want all zeros for a gradient that gets brighter to the right", uint64(*stored.Phash))
	}
}
