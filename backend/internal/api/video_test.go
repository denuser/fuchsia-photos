package api

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

// A video plays from /video, as an MP4 whatever its container, and seeks with
// range requests. Only those who see it may play it.
func TestVideoPlayback(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")

	res := anna.upload("video.mov")
	wantStatus(t, res, http.StatusCreated)
	video := decode[uploadResult](t, res).Photo
	if video.Mime != "video/quicktime" {
		t.Errorf("mime = %s", video.Mime)
	}
	path := "/api/v1/photos/" + itoa(video.ID) + "/video"

	req, err := http.NewRequest(http.MethodGet, e.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-99")
	res = anna.do(req)
	wantStatus(t, res, http.StatusPartialContent)
	if got := res.Header.Get("Content-Type"); got != "video/mp4" {
		t.Errorf("Content-Type = %s, want video/mp4", got)
	}
	if res.Header.Get("Content-Disposition") != "" {
		t.Error("the video is sent as a download")
	}
	if body, _ := io.ReadAll(res.Body); len(body) != 100 {
		t.Errorf("range returned %d bytes", len(body))
	}

	wantStatus(t, boris.get(path), http.StatusNotFound)
	wantStatus(t, e.browser().get(path), http.StatusUnauthorized)

	photo := decode[uploadResult](t, anna.upload("landscape.jpg")).Photo
	wantStatus(t, anna.get("/api/v1/photos/"+itoa(photo.ID)+"/video"), http.StatusNotFound)
}

func TestMotion(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")

	photo := decode[uploadResult](t, anna.upload("motion.jpg")).Photo
	path := "/api/v1/photos/" + itoa(photo.ID) + "/motion"
	wantStatus(t, anna.get(path), http.StatusNotFound) // Not processed yet.

	// Processing is not run here, so the test does its part.
	markReady(t, e, photo.ID)
	if err := storage.WriteFileAtomic(e.layout.Motion(photo.ID), []byte("motion")); err != nil {
		t.Fatal(err)
	}
	format := "webp"
	err := e.queries.MarkPhotoReady(context.Background(), store.MarkPhotoReadyParams{ID: photo.ID, PreviewFormat: &format, HasMotion: true})
	if err != nil {
		t.Fatal(err)
	}

	got := decode[photoJSON](t, anna.get("/api/v1/photos/"+itoa(photo.ID)))
	if !got.HasMotion || got.Duration != nil {
		t.Errorf("has motion = %v, duration = %v", got.HasMotion, got.Duration)
	}
	res := anna.get(path)
	wantStatus(t, res, http.StatusOK)
	if res.Header.Get("Content-Type") != "video/mp4" || res.Header.Get("Cache-Control") != previewCacheRule {
		t.Errorf("headers = %v", res.Header)
	}
	wantStatus(t, boris.get(path), http.StatusNotFound)
}

// Over its limit a file is refused with 413, by the browser upload and over
// WebDAV alike. The limit is small here so that the test writes little.
// The viewer waits for the copy of an HDR video, and starts from the list
// item, so the lists carry hdr as well as the photo itself.
func TestVideoHDR(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")

	video := decode[uploadResult](t, anna.upload("video.mov")).Photo
	photo := decode[uploadResult](t, anna.upload("landscape.jpg")).Photo
	markReady(t, e, video.ID)
	markReady(t, e, photo.ID)
	if got := decode[photoJSON](t, anna.get("/api/v1/photos/"+itoa(video.ID))); got.HDR != nil {
		t.Errorf("hdr of a video not probed = %v, want null", *got.HDR)
	}

	hdr := true
	if err := e.queries.SetVideoHDR(context.Background(), store.SetVideoHDRParams{ID: video.ID, Hdr: &hdr}); err != nil {
		t.Fatal(err)
	}
	if got := decode[photoJSON](t, anna.get("/api/v1/photos/"+itoa(video.ID))); got.HDR == nil || !*got.HDR {
		t.Errorf("hdr = %v, want true", got.HDR)
	}
	list := decode[struct{ Items []photoJSON }](t, anna.get("/api/v1/photos"))
	for _, item := range list.Items {
		switch {
		case item.ID == video.ID && (item.HDR == nil || !*item.HDR):
			t.Errorf("hdr of the video in the list = %v, want true", item.HDR)
		case item.ID == photo.ID && item.HDR != nil:
			t.Errorf("hdr of a photo = %v, want null", *item.HDR)
		}
	}
	if len(list.Items) != 2 {
		t.Errorf("the list has %d items, want 2", len(list.Items))
	}
}

func TestUploadTooLarge(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "media", "testdata", "landscape.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	e := setupWith(t, func(d *Deps) {
		limited := *d.Ingest
		limited.PhotoLimit = int64(len(data)) - 1
		d.Ingest = &limited
	})
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	wantStatus(t, anna.uploadBytes("landscape.jpg", data), http.StatusRequestEntityTooLarge)
	wantStatus(t, anna.upload("video.mp4"), http.StatusCreated)

}
