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

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/jobs"
)

// markLive marks a file processed with the ContentIdentifier of a Live Photo,
// as processing leaves it.
func markLive(t *testing.T, e *env, id int64, identifier string) {
	t.Helper()
	markReady(t, e, id)
	exif, err := json.Marshal(map[string]string{"ContentIdentifier": identifier})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(context.Background(), `UPDATE photos SET exif = exif || $2 WHERE id = $1`, id, exif); err != nil {
		t.Fatal(err)
	}
}

func pairLivePhotos(t *testing.T, e *env) int {
	t.Helper()
	paired, err := jobs.PairLivePhotos(context.Background(), e.pool, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return paired
}

// Once paired, the video of a Live Photo leaves every list, its albums and
// tags included, and plays as the motion of its photo for whoever sees the
// photo. It goes into the trash, out of it and away for good with the photo.
func TestLivePhoto(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	boris := e.addUser("boris", auth.RoleUser)
	anna, borisBrowser := e.loggedIn("anna"), e.loggedIn("boris")
	ctx := context.Background()

	video := anna.addPhoto("video.mov")
	markLive(t, e, video, "LIVE-1")
	album := anna.newAlbum(t, "Trip", video)
	res := anna.json(http.MethodPost, "/api/v1/tags", map[string]string{"name": "sea"})
	wantStatus(t, res, http.StatusCreated)
	tag := decode[tagRefJSON](t, res).ID
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/tags", map[string][]int64{"ids": {video}, "add": {tag}}), http.StatusNoContent)
	if paired := pairLivePhotos(t, e); paired != 0 {
		t.Fatalf("paired %d without a photo", paired)
	}

	still := anna.addPhoto("landscape.jpg")
	plain := anna.addPhoto("landscape.heic")
	markReady(t, e, plain)
	markLive(t, e, still, "LIVE-1")
	if paired := pairLivePhotos(t, e); paired != 1 {
		t.Fatalf("paired %d, want 1", paired)
	}

	got := decode[photoJSON](t, anna.get("/api/v1/photos/"+itoa(still)))
	if !got.HasMotion || got.LivePhotoID != nil {
		t.Errorf("photo: has motion = %v, live photo = %v", got.HasMotion, got.LivePhotoID)
	}
	hidden, err := e.queries.GetPhoto(ctx, video)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.LivePhotoID == nil || *hidden.LivePhotoID != still {
		t.Fatalf("video: live photo = %v", hidden.LivePhotoID)
	}
	if toPhotoJSON(hidden, false).LivePhotoID == nil {
		t.Error("events would not tell pages to drop the video")
	}

	wantIDs(t, anna.photoIDs("/api/v1/photos"), plain, still)
	if months := anna.months("/api/v1/photos/months?tz=UTC"); len(months) != 1 || months[0].Count != 2 {
		t.Errorf("months = %v", months)
	}
	wantIDs(t, anna.photoIDs("/api/v1/photos?album="+itoa(album)))
	if got := decode[albumJSON](t, anna.get("/api/v1/albums/"+itoa(album))); got.PhotoCount != 0 || got.CoverPhotoID != nil {
		t.Errorf("album: %d photos, cover %v", got.PhotoCount, got.CoverPhotoID)
	}
	tags, err := e.queries.ListTags(ctx, hidden.OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].PhotoCount != 0 {
		t.Errorf("tags = %+v", tags)
	}

	// The motion is the original of the video, for whoever sees the photo.
	original, err := os.ReadFile(filepath.Join("..", "media", "testdata", "video.mov"))
	if err != nil {
		t.Fatal(err)
	}
	motion := "/api/v1/photos/" + itoa(still) + "/motion"
	res = anna.get(motion)
	wantStatus(t, res, http.StatusOK)
	if body, _ := io.ReadAll(res.Body); !bytes.Equal(body, original) || res.Header.Get("Content-Type") != "video/mp4" {
		t.Errorf("motion: %d bytes, Content-Type %s", len(body), res.Header.Get("Content-Type"))
	}
	wantStatus(t, borisBrowser.get(motion), http.StatusNotFound)
	sharePhoto(t, anna, still, map[string]any{"add": []int64{boris.ID}})
	wantStatus(t, borisBrowser.get(motion), http.StatusOK)
	wantIDs(t, borisBrowser.photoIDs("/api/v1/photos/shared"), still)

	// Uploading the video again answers with its photo.
	res = anna.upload("video.mov")
	wantStatus(t, res, http.StatusOK)
	if result := decode[uploadResult](t, res); !result.Duplicate || result.Photo.ID != still {
		t.Errorf("upload of the video again = %+v", result)
	}

	// The video can be neither trashed nor restored on its own.
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(video)), http.StatusNoContent)
	if current, _ := e.queries.GetPhoto(ctx, video); current.DeletedAt != nil {
		t.Error("the video went into the trash without its photo")
	}

	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(still)), http.StatusNoContent)
	wantIDs(t, anna.photoIDs("/api/v1/photos/trash"), still)
	if current, _ := e.queries.GetPhoto(ctx, video); current.DeletedAt == nil {
		t.Error("the video stayed out of the trash")
	}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/restore", ids(video)), http.StatusNoContent)
	if current, _ := e.queries.GetPhoto(ctx, still); current.DeletedAt == nil {
		t.Error("restoring the video brought back its photo")
	}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/restore", ids(still)), http.StatusNoContent)
	if current, _ := e.queries.GetPhoto(ctx, video); current.DeletedAt != nil {
		t.Error("the video stayed in the trash")
	}

	// Purging the video alone does nothing; purging the photo takes both.
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(still)), http.StatusNoContent)
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/purge", ids(video)), http.StatusNoContent)
	if current, _ := e.queries.GetPhoto(ctx, video); current.PurgedAt != nil {
		t.Error("the video was purged without its photo")
	}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash/empty", nil), http.StatusNoContent)
	current, err := e.queries.GetPhoto(ctx, video)
	if err != nil {
		t.Fatal(err)
	}
	if current.PurgedAt == nil || current.ContentIdentifier != nil {
		t.Errorf("video: purged at %v, content identifier %v", current.PurgedAt, current.ContentIdentifier)
	}
	path := e.layout.Original(hidden.OwnerID, hidden.Sha256, hidden.Mime)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the video's original is still there: %v", err)
	}

	// Uploaded again, the video is a video of its own until its photo comes back.
	res = anna.upload("video.mov")
	wantStatus(t, res, http.StatusCreated)
	again := decode[uploadResult](t, res).Photo
	markLive(t, e, again.ID, "LIVE-1")
	if paired := pairLivePhotos(t, e); paired != 0 {
		t.Errorf("paired %d with a purged photo", paired)
	}
	wantIDs(t, anna.photoIDs("/api/v1/photos"), again.ID, plain)
}

// Only the files of one owner pair, though another user's share the identifier.
func TestLivePhotoNeedsTheSameOwner(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")

	still := anna.addPhoto("landscape.jpg")
	markLive(t, e, still, "LIVE-1")
	video := boris.addPhoto("video.mov")
	markLive(t, e, video, "LIVE-1")
	if paired := pairLivePhotos(t, e); paired != 0 {
		t.Errorf("paired %d across owners", paired)
	}
	var count int
	err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM photos WHERE live_photo_id IS NOT NULL OR has_motion`).Scan(&count)
	if err != nil || count != 0 {
		t.Errorf("%d photos paired, err = %v", count, err)
	}
}

// A paired photo whose processing wrote its motion flag over the pairing gets
// it back from the next pairing.
func TestLivePhotoMotionRepaired(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	still, video := anna.addPhoto("landscape.jpg"), anna.addPhoto("video.mov")
	markLive(t, e, still, "LIVE-1")
	markLive(t, e, video, "LIVE-1")
	if paired := pairLivePhotos(t, e); paired != 1 {
		t.Fatalf("paired %d, want 1", paired)
	}
	if _, err := e.pool.Exec(context.Background(), `UPDATE photos SET has_motion = false WHERE id = $1`, still); err != nil {
		t.Fatal(err)
	}
	if paired := pairLivePhotos(t, e); paired != 0 {
		t.Errorf("paired %d again", paired)
	}
	if got := decode[photoJSON](t, anna.get("/api/v1/photos/"+itoa(still))); !got.HasMotion {
		t.Error("the photo did not get its motion back")
	}
}

// A video that came in while its photo was in the trash pairs when the photo
// is restored, from the trash or by uploading it again.
func TestLivePhotoPairsOnRestore(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	ctx := context.Background()

	for _, restore := range []string{"from the trash", "by upload"} {
		t.Run(restore, func(t *testing.T) {
			still, video := anna.addPhoto("landscape.jpg"), anna.addPhoto("video.mov")
			markLive(t, e, still, "LIVE-1")
			wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(still)), http.StatusNoContent)
			markLive(t, e, video, "LIVE-1")
			if paired := pairLivePhotos(t, e); paired != 0 {
				t.Fatalf("paired %d with a photo in the trash", paired)
			}

			if restore == "by upload" {
				wantStatus(t, anna.upload("landscape.jpg"), http.StatusOK)
			} else {
				wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/restore", ids(still)), http.StatusNoContent)
			}
			current, err := e.queries.GetPhoto(ctx, video)
			if err != nil {
				t.Fatal(err)
			}
			if current.LivePhotoID == nil || *current.LivePhotoID != still {
				t.Errorf("live photo = %v, want %d", current.LivePhotoID, still)
			}

			// The next run starts from nothing.
			wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(still)), http.StatusNoContent)
			wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash/empty", nil), http.StatusNoContent)
		})
	}
}
