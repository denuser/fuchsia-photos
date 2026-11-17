package scan

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// A photo Google had in the trash goes to the trash when the import adds it,
// dated when the scan started. A photo the library had before stays where it
// is, and so does one the owner restored, when the export is imported again.
func TestTakeoutTrashesWhatGoogleTrashed(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	had := e.fixture("rotated.jpg")
	if _, err := e.service.Ingest.Ingest(ctx, e.owner, bytes.NewReader(had), "IMG_2.jpg", 0); err != nil {
		t.Fatal(err)
	}
	trash := "Takeout/Google Фото/Корзина/"
	files := []archiveFile{
		{trash + "IMG_1.jpg", e.fixture("landscape.jpg")},
		{trash + "IMG_1.jpg.supplemental-metadata.json", []byte(`{"title": "IMG_1.jpg", "photoTakenTime": {"timestamp": "1569055192"}, "trashed": true}`)},
		{trash + "IMG_2.jpg", had},
		{trash + "IMG_2.jpg.supplemental-metadata.json", []byte(`{"title": "IMG_2.jpg", "photoTakenTime": {"timestamp": "1569055192"}, "trashed": true}`)},
		{yearFolder + "IMG_3.jpg", e.fixture("landscape.png")},
		{yearFolder + "IMG_3.jpg.supplemental-metadata.json", []byte(`{"title": "IMG_3.jpg", "photoTakenTime": {"timestamp": "1569055192"}}`)},
	}
	e.write("takeout-20261115T073434Z-1-001.zip", makeZip(t, files))
	run := e.finish(e.startKind("", KindTakeout).ID)
	if run.Status != "finished" || run.Trashed != 1 {
		t.Fatalf("status %s, trashed %d, want finished and 1", run.Status, run.Trashed)
	}
	deletedAt := func(data []byte) *time.Time {
		t.Helper()
		photo, err := e.queries.GetPhoto(ctx, e.photoID(data))
		if err != nil {
			t.Fatal(err)
		}
		return photo.DeletedAt
	}
	got := deletedAt(e.fixture("landscape.jpg"))
	if got == nil || !got.Equal(run.CreatedAt) {
		t.Errorf("trashed photo: deleted at %v, want %v", got, run.CreatedAt)
	}
	if deletedAt(had) != nil {
		t.Error("a photo the library had went to the trash")
	}
	if deletedAt(e.fixture("landscape.png")) != nil {
		t.Error("a photo Google kept went to the trash")
	}

	// The owner restores it; importing again leaves it restored.
	if _, err := e.service.Ingest.Pool.Exec(ctx, `UPDATE photos SET deleted_at = NULL WHERE id = $1`, e.photoID(e.fixture("landscape.jpg"))); err != nil {
		t.Fatal(err)
	}
	again := e.finish(e.startKind("", KindTakeout).ID)
	if again.Status != "finished" || again.Trashed != 0 || deletedAt(e.fixture("landscape.jpg")) != nil {
		t.Errorf("importing again: status %s, trashed %d", again.Status, again.Trashed)
	}
}
