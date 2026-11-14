package api

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
	"github.com/denuser/fuchsia-photos/backend/internal/transcode"
	"github.com/denuser/fuchsia-photos/backend/internal/vaapi"
)

// graphics is a transcoder that only reports its graphics.
type graphics struct{ hw vaapi.Hardware }

func (g graphics) Hardware(context.Context) (vaapi.Hardware, error) { return g.hw, nil }

func (g graphics) Transcode(context.Context, string, string, vaapi.Source) error { return nil }

func TestVideoSettings(t *testing.T) {
	e := setup(t)
	e.addUser("root", auth.RoleAdmin)
	e.addUser("anna", auth.RoleUser)
	admin, user := e.loggedIn("root"), e.loggedIn("anna")
	const path = "/api/v1/settings/video"

	wantStatus(t, user.get(path), http.StatusForbidden)
	wantStatus(t, user.json(http.MethodPut, path, map[string]any{"transcode": "off"}), http.StatusForbidden)

	got := decode[videoSettingsJSON](t, admin.get(path))
	if got.Transcode != transcode.ModeView || got.Hardware != nil || got.Unavailable == "" {
		t.Errorf("defaults = %+v, want copies on view, no graphics and the reason", got)
	}
	wantStatus(t, admin.json(http.MethodPut, path, map[string]any{}), http.StatusBadRequest)
	wantStatus(t, admin.json(http.MethodPut, path, map[string]any{"transcode": "always"}), http.StatusBadRequest)
	if got := decode[videoSettingsJSON](t, admin.json(http.MethodPut, path, map[string]any{"transcode": "upload"})); got.Transcode != "upload" {
		t.Errorf("saved = %+v", got)
	}
	if got := decode[videoSettingsJSON](t, admin.get(path)); got.Transcode != "upload" {
		t.Error("the setting was not saved")
	}
}

func TestVideoSettingsShowHardware(t *testing.T) {
	e := setupWith(t, func(d *Deps) {
		d.Transcode = &transcode.Service{
			Queries: d.Queries,
			Layout:  d.Layout,
			Backend: graphics{vaapi.Hardware{Device: "/dev/dri/renderD128", Driver: "Intel iHD driver 25.4.6", LowPower: true, Vendor: vaapi.VendorIntel}},
		}
		d.Transcode.Check(context.Background())
	})
	e.addUser("root", auth.RoleAdmin)
	got := decode[videoSettingsJSON](t, e.loggedIn("root").get("/api/v1/settings/video"))
	if got.Hardware == nil || got.Hardware.Device != "/dev/dri/renderD128" || !got.Hardware.LowPower || got.Unavailable != "" {
		t.Errorf("settings = %+v", got)
	}
}

// The copy is served once it is ready, to whoever sees the item, and a
// request for it queues nothing where the graphics cannot make it.
func TestVideoCopy(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")
	ctx := context.Background()

	video := decode[uploadResult](t, anna.upload("video.mov")).Photo
	path := "/api/v1/photos/" + itoa(video.ID) + "/copy"
	wantStatus(t, anna.get(path), http.StatusNotFound)
	format := "webp"
	if err := e.queries.MarkPhotoReady(ctx, store.MarkPhotoReadyParams{ID: video.ID, PreviewFormat: &format}); err != nil {
		t.Fatal(err)
	}

	if got := decode[map[string]bool](t, anna.json(http.MethodPost, path, nil)); got["queued"] {
		t.Error("a copy was queued without graphics")
	}
	wantStatus(t, boris.json(http.MethodPost, path, nil), http.StatusNotFound)
	wantStatus(t, anna.get(path), http.StatusNotFound)

	if err := storage.WriteFileAtomic(e.layout.Copy(video.ID), []byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	ready := transcode.CopyReady
	if err := e.queries.SetPhotoVideoCopy(ctx, store.SetPhotoVideoCopyParams{ID: video.ID, VideoCopy: &ready}); err != nil {
		t.Fatal(err)
	}
	if got := decode[photoJSON](t, anna.get("/api/v1/photos/"+itoa(video.ID))); got.VideoCopy == nil || *got.VideoCopy != ready {
		t.Errorf("videoCopy = %v", got.VideoCopy)
	}

	req, err := http.NewRequest(http.MethodGet, e.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=2-4")
	res := anna.do(req)
	wantStatus(t, res, http.StatusPartialContent)
	if body, _ := io.ReadAll(res.Body); string(body) != "234" || res.Header.Get("Content-Type") != "video/mp4" {
		t.Errorf("body = %q, headers = %v", body, res.Header)
	}
	wantStatus(t, boris.get(path), http.StatusNotFound)
	wantStatus(t, e.browser().get(path), http.StatusUnauthorized)

	// A still photo plays nothing.
	photo := decode[uploadResult](t, anna.upload("landscape.jpg")).Photo
	markReady(t, e, photo.ID)
	wantStatus(t, anna.get("/api/v1/photos/"+itoa(photo.ID)+"/copy"), http.StatusNotFound)
}
