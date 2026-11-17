package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/jobs"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
)

// setTakenAt gives a processed file an EXIF date without an offset.
func setTakenAt(t *testing.T, e *env, id int64, date string) {
	t.Helper()
	exif, err := json.Marshal(map[string]string{"DateTimeOriginal": date})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(context.Background(), `UPDATE photos SET exif = exif || $2 WHERE id = $1`, id, exif); err != nil {
		t.Fatal(err)
	}
}

// recordEdited records an edited copy and its original as an import's
// matching does.
func recordEdited(t *testing.T, e *env, edited, original int64) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(), `
		INSERT INTO imported_metadata (photo_id, original_id, export, export_time, path, sidecar)
		VALUES ($1, $2, 'takeout-20261115T073434Z', '2026-11-15T07:34:34Z', 'Takeout/x.json', '{}')`, edited, original)
	if err != nil {
		t.Fatal(err)
	}
}

func linkVersions(t *testing.T, e *env) int {
	t.Helper()
	linked, err := jobs.LinkVersions(context.Background(), e.pool, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return linked
}

// linkAbove returns the photo a file is hidden under and the kind of link,
// or zero and "" when it is not hidden.
func linkAbove(t *testing.T, e *env, id int64) (int64, string) {
	t.Helper()
	link, err := e.queries.GetLinkAbove(context.Background(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return link.ShownID, link.Kind
}

// An edited copy that an import recorded hides its original once both are
// processed with the same EXIF date. The copy takes over the original's
// albums, cover, shares and tags, and the motion of its Live Photo, whose
// video moves to the original if it was paired with the copy. Only the owner
// sees the original; the trash, restoring and purging move the whole chain.
func TestEditedVersions(t *testing.T) {
	e := setup(t)
	annaUser := e.addUser("anna", auth.RoleUser)
	boris := e.addUser("boris", auth.RoleUser)
	anna, borisBrowser := e.loggedIn("anna"), e.loggedIn("boris")
	ctx := context.Background()

	// The copy comes first, so the video pairs with it before the matching
	// records it as a copy.
	edited := anna.addPhoto("landscape.heic")
	original := anna.addPhoto("landscape.jpg")
	video := anna.addPhoto("video.mov")
	other := anna.addPhoto("landscape.png")
	for _, id := range []int64{edited, original, video} {
		markLive(t, e, id, "LIVE-9")
	}
	markReady(t, e, other)
	setTakenAt(t, e, edited, "2019:09:21 10:00:00")
	setTakenAt(t, e, original, "2019:09:21 10:00:00")
	if paired := pairLivePhotos(t, e); paired != 1 {
		t.Fatalf("paired %d, want 1", paired)
	}
	if shown, _ := linkAbove(t, e, video); shown != edited {
		t.Fatalf("video paired with %d, want the copy %d", shown, edited)
	}

	// What the original had before the link.
	album := anna.newAlbum(t, "Trip", original, other)
	wantStatus(t, anna.json(http.MethodPut, "/api/v1/albums/"+itoa(album)+"/cover", map[string]any{"photoId": original}), http.StatusOK)
	res := anna.json(http.MethodPost, "/api/v1/tags", map[string]string{"name": "sea"})
	wantStatus(t, res, http.StatusCreated)
	tag := decode[tagRefJSON](t, res).ID
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/tags", map[string][]int64{"ids": {original}, "add": {tag}}), http.StatusNoContent)
	sharePhoto(t, anna, original, map[string]any{"add": []int64{boris.ID}})
	favorite(t, anna, true, original)
	favorite(t, borisBrowser, true, original)
	archive(t, borisBrowser, true, original)

	recordEdited(t, e, edited, original)
	if linked := linkVersions(t, e); linked != 1 {
		t.Fatalf("linked %d, want 1", linked)
	}
	if shown, kind := linkAbove(t, e, original); shown != edited || kind != storage.LinkEdited {
		t.Fatalf("original under %d as %q", shown, kind)
	}
	if shown, kind := linkAbove(t, e, video); shown != original || kind != storage.LinkLive {
		t.Fatalf("video under %d as %q, want the original", shown, kind)
	}
	if linked := linkVersions(t, e); linked != 0 {
		t.Errorf("a second run linked %d", linked)
	}

	// Lists show the copy only.
	wantIDs(t, anna.photoIDs("/api/v1/photos"), other, edited)
	wantIDs(t, anna.photoIDs("/api/v1/photos?album="+itoa(album)), other, edited)
	if got := decode[albumJSON](t, anna.get("/api/v1/albums/"+itoa(album))); got.PhotoCount != 2 || got.CoverPhotoID == nil || *got.CoverPhotoID != edited {
		t.Errorf("album: %d photos, cover %v", got.PhotoCount, got.CoverPhotoID)
	}
	tags, err := e.queries.ListTags(ctx, annaUser.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].PhotoCount != 1 {
		t.Errorf("tags = %+v", tags)
	}
	wantIDs(t, anna.photoIDs("/api/v1/photos?tag="+itoa(tag)), edited)
	wantIDs(t, anna.photoIDs("/api/v1/photos?favorites=true"), edited)
	wantIDs(t, borisBrowser.photoIDs("/api/v1/photos?favorites=true&withShared=true"), edited)
	wantIDs(t, borisBrowser.photoIDs("/api/v1/photos?archived=true&withShared=true"), edited)

	// The owner's viewer gets the original as a version; the copy plays the
	// original's video.
	got := decode[photoJSON](t, anna.get("/api/v1/photos/"+itoa(edited)))
	if !got.HasMotion || len(got.Versions) != 1 || got.Versions[0].ID != original || got.Versions[0].Kind != storage.LinkEdited {
		t.Errorf("copy: has motion %v, versions %+v", got.HasMotion, got.Versions)
	}
	if got := decode[photoJSON](t, anna.get("/api/v1/photos/"+itoa(original))); !got.HasMotion || len(got.Versions) != 1 || got.Versions[0].Kind != storage.LinkLive {
		t.Errorf("original: has motion %v, versions %+v", got.HasMotion, got.Versions)
	}
	wantStatus(t, anna.get("/api/v1/photos/"+itoa(original)+"/previews/viewer"), http.StatusOK)
	movie, err := os.ReadFile(filepath.Join("..", "media", "testdata", "video.mov"))
	if err != nil {
		t.Fatal(err)
	}
	res = anna.get("/api/v1/photos/" + itoa(edited) + "/motion")
	wantStatus(t, res, http.StatusOK)
	if body, _ := io.ReadAll(res.Body); !bytes.Equal(body, movie) {
		t.Errorf("motion of the copy: %d bytes", len(body))
	}

	// Others see the copy, which took the original's share, but not the
	// original, and get no versions.
	archive(t, borisBrowser, false, edited)
	wantIDs(t, borisBrowser.photoIDs("/api/v1/photos/shared"), edited)
	if got := decode[photoJSON](t, borisBrowser.get("/api/v1/photos/"+itoa(edited))); len(got.Versions) != 0 {
		t.Errorf("versions shown to another user: %+v", got.Versions)
	}
	wantStatus(t, borisBrowser.get("/api/v1/photos/"+itoa(original)), http.StatusNotFound)
	wantStatus(t, borisBrowser.get("/api/v1/photos/"+itoa(original)+"/original"), http.StatusNotFound)
	wantStatus(t, borisBrowser.get("/api/v1/photos/"+itoa(edited)+"/motion"), http.StatusOK)

	// Uploading the original again answers with the copy.
	res = anna.upload("landscape.jpg")
	wantStatus(t, res, http.StatusOK)
	if result := decode[uploadResult](t, res); !result.Duplicate || result.Photo.ID != edited {
		t.Errorf("upload of the original again = %+v", result)
	}

	// The trash and restoring move the chain; the hidden ones do not move alone.
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(original)), http.StatusNoContent)
	if current, _ := e.queries.GetPhoto(ctx, original); current.DeletedAt != nil {
		t.Error("the original went into the trash on its own")
	}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(edited)), http.StatusNoContent)
	wantIDs(t, anna.photoIDs("/api/v1/photos/trash"), edited)
	for _, id := range []int64{original, video} {
		if current, _ := e.queries.GetPhoto(ctx, id); current.DeletedAt == nil {
			t.Errorf("photo %d stayed out of the trash", id)
		}
	}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/restore", ids(edited)), http.StatusNoContent)
	for _, id := range []int64{original, video} {
		if current, _ := e.queries.GetPhoto(ctx, id); current.DeletedAt != nil {
			t.Errorf("photo %d stayed in the trash", id)
		}
	}

	// Purging the copy takes the original and the video.
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(edited)), http.StatusNoContent)
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash/empty", nil), http.StatusNoContent)
	for _, id := range []int64{edited, original, video} {
		current, err := e.queries.GetPhoto(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if current.PurgedAt == nil {
			t.Errorf("photo %d was not purged", id)
		}
		path := e.layout.Original(current.OwnerID, current.Sha256, current.Mime)
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the original of %d is still there: %v", id, err)
		}
	}
}

// A file whose name only looks like an edited copy's stays a photo of its own:
// without the same EXIF date, or while one of the two is in the trash, nothing
// is linked.
func TestEditedVersionsNeedTheSameDate(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")

	edited := anna.addPhoto("landscape.heic")
	original := anna.addPhoto("landscape.jpg")
	markReady(t, e, edited)
	markReady(t, e, original)
	setTakenAt(t, e, edited, "2019:09:21 10:00:01")
	setTakenAt(t, e, original, "2019:09:21 10:00:00")
	recordEdited(t, e, edited, original)
	if linked := linkVersions(t, e); linked != 0 {
		t.Fatalf("linked %d files with different dates", linked)
	}

	setTakenAt(t, e, edited, "2019:09:21 10:00:00")
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(original)), http.StatusNoContent)
	if linked := linkVersions(t, e); linked != 0 {
		t.Fatalf("linked %d with the original in the trash", linked)
	}
	// Restoring the original links it.
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/restore", ids(original)), http.StatusNoContent)
	if shown, kind := linkAbove(t, e, original); shown != edited || kind != storage.LinkEdited {
		t.Errorf("original under %d as %q after restoring", shown, kind)
	}
}
