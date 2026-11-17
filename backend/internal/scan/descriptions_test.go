package scan

import (
	"context"
	"testing"
)

// An import gives a photo the description in its JSON, cleaned, and never
// replaces a description the photo has, so the owner's edits stay.
func TestTakeoutFillsDescriptions(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	files := []archiveFile{
		{yearFolder + "IMG_1.jpg", e.fixture("landscape.jpg")},
		{yearFolder + "IMG_1.jpg.supplemental-metadata.json", []byte(`{"title": "IMG_1.jpg", "photoTakenTime": {"timestamp": "1569055192"}, "description": " Море\r\nи\tгоры \n"}`)},
		{yearFolder + "IMG_2.jpg", e.fixture("rotated.jpg")},
		{yearFolder + "IMG_2.jpg.supplemental-metadata.json", []byte(`{"title": "IMG_2.jpg", "photoTakenTime": {"timestamp": "1569055192"}, "description": ""}`)},
	}
	e.write("takeout-20261115T073434Z-1-001.zip", makeZip(t, files))
	if run := e.finish(e.startKind("", KindTakeout).ID); run.Status != "finished" {
		t.Fatalf("status %s", run.Status)
	}
	described, plain := e.photoID(e.fixture("landscape.jpg")), e.photoID(e.fixture("rotated.jpg"))
	description := func(id int64) string {
		photo, err := e.queries.GetPhoto(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return photo.Description
	}
	if got := description(described); got != "Море\nи горы" {
		t.Errorf("description = %q", got)
	}
	if got := description(plain); got != "" {
		t.Errorf("description of a photo with an empty one = %q", got)
	}

	// The owner's edit stays when the export is imported again.
	if _, err := e.service.Ingest.Pool.Exec(ctx, `UPDATE photos SET description = 'Своё' WHERE id = $1`, described); err != nil {
		t.Fatal(err)
	}
	if run := e.finish(e.startKind("", KindTakeout).ID); run.Status != "finished" {
		t.Fatalf("status %s", run.Status)
	}
	if got := description(described); got != "Своё" {
		t.Errorf("description after importing again = %q", got)
	}
}
