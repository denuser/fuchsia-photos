package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
)

func TestRAWVersions(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")
	ctx := context.Background()

	// Ingest only needs the file signature. markReady substitutes for the
	// decoder in this test of pairing and the HTTP API.
	rawData := []byte("II*\x00\x08\x00\x00\x00")
	res := anna.uploadBytes("RAW/landscape.NEF", rawData)
	wantStatus(t, res, http.StatusCreated)
	raw := decode[uploadResult](t, res).Photo
	if raw.Mime != storage.RAWNEF {
		t.Fatalf("RAW MIME = %q", raw.Mime)
	}
	jpeg := anna.addPhoto("landscape.jpg")
	foreign := boris.addPhoto("landscape.jpg")
	exif := func(id int64, subsec string) {
		t.Helper()
		markReady(t, e, id)
		_, err := e.pool.Exec(ctx, `UPDATE photos SET exif = exif || jsonb_build_object(
			'DateTimeOriginal', '2019:09:21 10:00:00', 'SubSecTimeOriginal', $2::text, 'Make', 'Nikon', 'Model', 'D5200')
			WHERE id = $1`, id, subsec)
		if err != nil {
			t.Fatal(err)
		}
	}
	exif(raw.ID, "012")
	exif(jpeg, "013")
	exif(foreign, "012")
	if n := linkVersions(t, e); n != 0 {
		t.Fatalf("linked %d photos with different sub-seconds or owners", n)
	}

	exif(jpeg, "0120")
	if n := linkVersions(t, e); n != 1 {
		t.Fatalf("linked %d photos, want one RAW + JPEG pair", n)
	}
	if n := linkVersions(t, e); n != 0 {
		t.Fatalf("linked %d photos when rerun", n)
	}
	wantIDs(t, anna.photoIDs("/api/v1/photos"), jpeg)
	got := decode[photoJSON](t, anna.get("/api/v1/photos/"+itoa(jpeg)))
	if len(got.Versions) != 1 || got.Versions[0].ID != raw.ID || got.Versions[0].Kind != storage.LinkRAW {
		t.Fatalf("JPEG versions = %+v", got.Versions)
	}
	res = anna.get("/api/v1/photos/" + itoa(raw.ID) + "/original")
	wantStatus(t, res, http.StatusOK)
	data, err := io.ReadAll(res.Body)
	if err != nil || !bytes.Equal(data, rawData) {
		t.Fatalf("downloaded RAW = %q, %v", data, err)
	}

	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(jpeg)), http.StatusNoContent)
	wantIDs(t, anna.photoIDs("/api/v1/photos/trash"), jpeg)
	trashed, err := e.queries.GetPhoto(ctx, raw.ID)
	if err != nil || trashed.DeletedAt == nil {
		t.Fatalf("RAW did not follow JPEG into trash: %v", err)
	}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/restore", ids(jpeg)), http.StatusNoContent)
	restored, err := e.queries.GetPhoto(ctx, raw.ID)
	if err != nil || restored.DeletedAt != nil {
		t.Fatalf("RAW was not restored with JPEG: %v", err)
	}
}
