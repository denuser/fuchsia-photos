package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/davidbyttow/govips/v2/vips"

	"github.com/magellanica-app/magellanica-photos/backend/internal/media"
	"github.com/magellanica-app/magellanica-photos/backend/internal/storage"
)

// realPreviews writes previews made from a fixture in place of the stand-ins
// of markReady, with the photo's size.
func realPreviews(t *testing.T, e *env, id int64, fixture string) {
	t.Helper()
	previews, err := media.Render(filepath.Join("..", "media", "testdata", fixture), media.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteFileAtomic(e.layout.Preview(id, storage.PreviewViewer, "webp"), previews.Viewer.Data); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteFileAtomic(e.layout.Preview(id, storage.PreviewGrid, "webp"), previews.Grid.Data); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(t.Context(), "UPDATE photos SET width=800, height=600 WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
}

func previewSize(t *testing.T, e *env, id int64) (int, int) {
	t.Helper()
	data, err := os.ReadFile(e.layout.Preview(id, storage.PreviewViewer, "webp"))
	if err != nil {
		t.Fatal(err)
	}
	image, err := vips.NewImageFromBuffer(data)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	return image.Width(), image.Height()
}

func TestRotatePhoto(t *testing.T) {
	s := setupSharing(t)
	ctx := t.Context()
	photo := s.anna.addPhoto("landscape.jpg")
	markReady(t, s.e, photo)
	realPreviews(t, s.e, photo, "landscape.jpg")
	// A RAW hidden under the photo turns with it.
	raw := s.anna.addPhoto("rotated.dng")
	markReady(t, s.e, raw)
	if _, err := s.e.pool.Exec(ctx, "INSERT INTO photo_links(hidden_id,shown_id,kind) VALUES($1,$2,'raw')", raw, photo); err != nil {
		t.Fatal(err)
	}
	// A face found on the photo turns with it and waits for a rescan, and the
	// search vector is made again.
	if _, err := s.e.pool.Exec(ctx, `INSERT INTO face_scans(photo_id, model, sha256) SELECT id, 'm', sha256 FROM photos WHERE id=$1`, photo); err != nil {
		t.Fatal(err)
	}
	vector := "[1" + repeatString(",0", 127) + "]"
	if _, err := s.e.pool.Exec(ctx, `INSERT INTO photo_faces(photo_id, box, embedding) VALUES($1, '{0.1,0.2,0.2,0.3}', $2::halfvec)`, photo, vector); err != nil {
		t.Fatal(err)
	}
	if _, err := s.e.pool.Exec(ctx, `INSERT INTO photo_embeddings(photo_id, model, embedding) VALUES($1, 'm', '[1,0,0]'::halfvec)`, photo); err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/photos/" + itoa(photo) + "/rotate"
	wantStatus(t, s.boris.json("POST", path, map[string]any{"degrees": 90}), 404)
	wantStatus(t, s.anna.json("POST", path, map[string]any{"degrees": 45}), 400)
	wantStatus(t, s.anna.json("POST", "/api/v1/photos/"+itoa(raw)+"/rotate", map[string]any{"degrees": 90}), 404)

	r := s.anna.json("POST", path, map[string]any{"degrees": 90})
	wantStatus(t, r, 200)
	turned := decode[photoJSON](t, r)
	if turned.Rotation != 90 || turned.Width == nil || *turned.Width != 600 || *turned.Height != 800 {
		t.Fatalf("turned photo: rotation %d, size %v x %v", turned.Rotation, turned.Width, turned.Height)
	}
	if w, h := previewSize(t, s.e, photo); w >= h {
		t.Errorf("viewer preview %dx%d after a turn of 90", w, h)
	}
	var rawRotation, previewRotation int16
	var box []float32
	var rescan, embedded bool
	if err := s.e.pool.QueryRow(ctx, `SELECT (SELECT rotation FROM photos WHERE id=$2), p.preview_rotation,
   (SELECT box FROM photo_faces WHERE photo_id=$1), (SELECT rescan FROM face_scans WHERE photo_id=$1),
   EXISTS (SELECT FROM photo_embeddings WHERE photo_id=$1)
 FROM photos p WHERE p.id=$1`, photo, raw).Scan(&rawRotation, &previewRotation, &box, &rescan, &embedded); err != nil {
		t.Fatal(err)
	}
	if rawRotation != 90 || previewRotation != 90 {
		t.Errorf("RAW rotation %d, preview rotation %d", rawRotation, previewRotation)
	}
	// A box at 0.1, 0.2 of 0.2 by 0.3 lies, turned clockwise, at 0.5, 0.1 of 0.3 by 0.2.
	want := []float32{0.5, 0.1, 0.3, 0.2}
	for i := range want {
		if d := box[i] - want[i]; d > 1e-5 || d < -1e-5 {
			t.Fatalf("turned box %v, want %v", box, want)
		}
	}
	if !rescan || embedded {
		t.Errorf("rescan %v, search vector kept %v", rescan, embedded)
	}

	// Three more quarter turns bring it back.
	r = s.anna.json("POST", path, map[string]any{"degrees": 270})
	wantStatus(t, r, 200)
	if back := decode[photoJSON](t, r); back.Rotation != 0 || *back.Width != 800 {
		t.Fatalf("turned back: rotation %d, width %d", back.Rotation, *back.Width)
	}
	if w, h := previewSize(t, s.e, photo); w <= h {
		t.Errorf("viewer preview %dx%d after turning back", w, h)
	}

	video := s.anna.addPhoto("video.mov")
	markReady(t, s.e, video)
	wantStatus(t, s.anna.json("POST", "/api/v1/photos/"+itoa(video)+"/rotate", map[string]any{"degrees": 90}), 400)

	pending := s.anna.addPhoto("landscape.png")
	if _, err := s.e.pool.Exec(ctx, "UPDATE photos SET status='pending' WHERE id=$1", pending); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, s.anna.json("POST", "/api/v1/photos/"+itoa(pending)+"/rotate", map[string]any{"degrees": 90}), 409)

	if _, err := s.e.pool.Exec(ctx, "UPDATE photos SET deleted_at=now() WHERE id=$1", photo); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, s.anna.json("POST", path, map[string]any{"degrees": 90}), 404)
}

func TestNewFileResetsTheRotation(t *testing.T) {
	s := setupSharing(t)
	photo := s.anna.addPhoto("landscape.jpg")
	markReady(t, s.e, photo)
	if _, err := s.e.pool.Exec(t.Context(), "UPDATE photos SET rotation=90 WHERE id=$1", photo); err != nil {
		t.Fatal(err)
	}
	if _, err := s.e.pool.Exec(t.Context(), "UPDATE photos SET sha256=decode(repeat('cd', 32), 'hex') WHERE id=$1", photo); err != nil {
		t.Fatal(err)
	}
	var rotation int16
	if err := s.e.pool.QueryRow(t.Context(), "SELECT rotation FROM photos WHERE id=$1", photo).Scan(&rotation); err != nil || rotation != 0 {
		t.Fatalf("rotation after a new file %d, %v", rotation, err)
	}
}

func repeatString(s string, n int) string {
	out := ""
	for range n {
		out += s
	}
	return out
}
