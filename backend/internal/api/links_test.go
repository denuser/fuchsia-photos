package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/magellanica-app/magellanica-photos/backend/internal/auth"
)

type linkResult = linkJSON

func newLink(t *testing.T, b *browser, album int64, body map[string]any) linkResult {
	t.Helper()
	res := b.json(http.MethodPost, "/api/v1/albums/"+itoa(album)+"/links", body)
	wantStatus(t, res, http.StatusCreated)
	return decode[linkResult](t, res)
}

func publicURL(link linkResult, rest string) string {
	return publicBase + strings.TrimPrefix(link.Path, "/s/") + rest
}

func TestPublicLink(t *testing.T) {
	e := setupWith(t, func(d *Deps) { d.PublicURL = "https://photos.example.org" })
	e.addUser("anna", auth.RoleUser)
	e.addUser("boris", auth.RoleUser)
	anna, boris := e.loggedIn("anna"), e.loggedIn("boris")

	inside := decode[uploadResult](t, anna.upload("landscape.jpg")).Photo.ID
	outside := decode[uploadResult](t, anna.upload("rotated.jpg")).Photo.ID
	markReady(t, e, inside)
	markReady(t, e, outside)
	album := anna.newAlbum(t, "Holiday", inside)

	// Only the owner makes and sees links.
	wantStatus(t, boris.json(http.MethodPost, "/api/v1/albums/"+itoa(album)+"/links", map[string]any{}), http.StatusNotFound)
	if links := decode[[]linkJSON](t, boris.get("/api/v1/albums/"+itoa(album)+"/links")); len(links) != 0 {
		t.Errorf("another user sees %d links", len(links))
	}
	wantStatus(t, e.browser().json(http.MethodPost, "/api/v1/albums/"+itoa(album)+"/links", map[string]any{}), http.StatusUnauthorized)

	link := newLink(t, anna, album, map[string]any{})
	if !strings.HasPrefix(link.URL, "https://photos.example.org/s/") || link.URL != "https://photos.example.org"+link.Path || len(link.Path) < 40 {
		t.Errorf("link = %+v, want an address built from PUBLIC_URL and a long token", link)
	}

	// A visitor needs no account and sees the album's name and its photos, and nothing else.
	visitor := e.browser()
	info := decode[publicAlbumJSON](t, visitor.get(publicURL(link, "")))
	if info.Name != "Holiday" || info.PasswordRequired || !info.Unlocked {
		t.Errorf("album = %+v", info)
	}
	res := visitor.get(publicURL(link, "/photos"))
	wantStatus(t, res, http.StatusOK)
	if res.Header.Get("X-Robots-Tag") == "" || res.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("headers = %v", res.Header)
	}
	photos := decode[[]publicPhotoJSON](t, res)
	if len(photos) != 1 || photos[0].ID != inside || photos[0].Kind != "photo" {
		t.Fatalf("photos = %+v", photos)
	}
	for _, kind := range []string{"grid", "viewer"} {
		res := visitor.get(publicURL(link, "/photos/"+itoa(inside)+"/previews/"+kind))
		wantStatus(t, res, http.StatusOK)
		if got := res.Header.Get("Cache-Control"); !strings.HasPrefix(got, "private") {
			t.Errorf("Cache-Control = %q: a proxy must not keep what a revoked link showed", got)
		}
	}
	// The original, the photo's record and a photo outside the album stay closed.
	wantStatus(t, visitor.get(publicURL(link, "/photos/"+itoa(outside)+"/previews/grid")), http.StatusNotFound)
	wantStatus(t, visitor.get(publicURL(link, "/photos/"+itoa(inside)+"/video")), http.StatusNotFound)
	wantStatus(t, visitor.get("/api/v1/photos/"+itoa(inside)+"/original"), http.StatusUnauthorized)
	wantStatus(t, visitor.get("/api/v1/photos/"+itoa(inside)+"/previews/grid"), http.StatusUnauthorized)
	wantStatus(t, visitor.get(publicBase+"no-such-token/photos"), http.StatusNotFound)
	wantStatus(t, visitor.get(publicURL(link, "/photos/"+itoa(inside)+"/previews/original")), http.StatusNotFound)

	// A photo that leaves the album, or goes to the trash, leaves the link.
	anna.json(http.MethodPost, "/api/v1/albums/"+itoa(album)+"/photos", ids(outside))
	if got := decode[[]publicPhotoJSON](t, visitor.get(publicURL(link, "/photos"))); len(got) != 2 {
		t.Fatalf("%d photos after adding one", len(got))
	}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/photos/trash", ids(outside)), http.StatusNoContent)
	if got := decode[[]publicPhotoJSON](t, visitor.get(publicURL(link, "/photos"))); len(got) != 1 {
		t.Errorf("%d photos with one in the trash", len(got))
	}
	wantStatus(t, visitor.get(publicURL(link, "/photos/"+itoa(outside)+"/previews/grid")), http.StatusNotFound)

	// Revoking closes it at once, and only for its owner.
	listed := decode[[]linkJSON](t, anna.get("/api/v1/albums/"+itoa(album)+"/links"))
	if len(listed) != 1 || listed[0].ID != link.ID {
		t.Fatalf("links = %+v", listed)
	}
	wantStatus(t, boris.json(http.MethodDelete, "/api/v1/albums/"+itoa(album)+"/links/"+itoa(link.ID), nil), http.StatusNotFound)
	wantStatus(t, anna.json(http.MethodDelete, "/api/v1/albums/"+itoa(album)+"/links/"+itoa(link.ID), nil), http.StatusNoContent)
	wantStatus(t, visitor.get(publicURL(link, "/photos")), http.StatusNotFound)
	wantStatus(t, visitor.get(publicURL(link, "")), http.StatusNotFound)
}

func TestPublicLinkWithoutPublicURL(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	link := newLink(t, anna, anna.newAlbum(t, "A"), map[string]any{})
	if link.URL != "" || !strings.HasPrefix(link.Path, "/s/") {
		t.Errorf("link = %+v, want only a path, for the interface to complete", link)
	}
}

func TestPublicLinkExpires(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	album := anna.newAlbum(t, "A")
	for _, days := range []int{0, 366, -1} {
		wantStatus(t, anna.json(http.MethodPost, "/api/v1/albums/"+itoa(album)+"/links", map[string]any{"expiresInDays": days}), http.StatusBadRequest)
	}
	link := newLink(t, anna, album, map[string]any{"expiresInDays": 7})
	if link.ExpiresAt == nil || time.Until(*link.ExpiresAt) < 6*24*time.Hour {
		t.Fatalf("expires at %v", link.ExpiresAt)
	}
	visitor := e.browser()
	wantStatus(t, visitor.get(publicURL(link, "")), http.StatusOK)

	if _, err := e.pool.Exec(context.Background(), "UPDATE album_links SET expires_at = now() - interval '1 second' WHERE id = $1", link.ID); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, visitor.get(publicURL(link, "")), http.StatusNotFound)
	wantStatus(t, visitor.get(publicURL(link, "/photos")), http.StatusNotFound)
}

func TestPublicLinkPassword(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	photo := decode[uploadResult](t, anna.upload("landscape.jpg")).Photo.ID
	markReady(t, e, photo)
	album := anna.newAlbum(t, "Private holiday", photo)
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/albums/"+itoa(album)+"/links", map[string]any{"password": "short"}), http.StatusBadRequest)
	link := newLink(t, anna, album, map[string]any{"password": "a link password"})
	if !link.HasPassword {
		t.Fatal("the link says it has no password")
	}

	visitor := e.browser()
	// Locked, nothing shows, not even the album's name.
	info := decode[publicAlbumJSON](t, visitor.get(publicURL(link, "")))
	if !info.PasswordRequired || info.Unlocked || info.Name != "" {
		t.Errorf("locked album = %+v", info)
	}
	wantStatus(t, visitor.get(publicURL(link, "/photos")), http.StatusUnauthorized)
	wantStatus(t, visitor.get(publicURL(link, "/photos/"+itoa(photo)+"/previews/grid")), http.StatusUnauthorized)

	wantStatus(t, visitor.json(http.MethodPost, publicURL(link, "/unlock"), map[string]string{"password": "wrong"}), http.StatusUnauthorized)
	wantStatus(t, visitor.get(publicURL(link, "/photos")), http.StatusUnauthorized)
	res := visitor.json(http.MethodPost, publicURL(link, "/unlock"), map[string]string{"password": "a link password"})
	wantStatus(t, res, http.StatusNoContent)
	if cookies := res.Cookies(); len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Path != publicURL(link, "") {
		t.Errorf("unlock cookie = %+v, want one HttpOnly cookie for this link's path only", cookies)
	}
	wantStatus(t, visitor.get(publicURL(link, "/photos")), http.StatusOK)
	wantStatus(t, visitor.get(publicURL(link, "/photos/"+itoa(photo)+"/previews/grid")), http.StatusOK)
	if got := decode[publicAlbumJSON](t, visitor.get(publicURL(link, ""))); got.Name != "Private holiday" || !got.Unlocked {
		t.Errorf("unlocked album = %+v", got)
	}

	// The unlock is for this link only: another link of the same album has its own password.
	other := newLink(t, anna, album, map[string]any{"password": "another password"})
	wantStatus(t, visitor.get(publicURL(other, "/photos")), http.StatusUnauthorized)
	// A forged cookie does not unlock.
	forged := e.browser()
	wantStatus(t, forged.json(http.MethodPost, publicURL(other, "/unlock"), map[string]string{"password": "x"}), http.StatusUnauthorized)

	// Wrong passwords slow down per link.
	guesser := e.browser()
	for range 5 {
		wantStatus(t, guesser.json(http.MethodPost, publicURL(link, "/unlock"), map[string]string{"password": "guess"}), http.StatusUnauthorized)
	}
	res = guesser.json(http.MethodPost, publicURL(link, "/unlock"), map[string]string{"password": "a link password"})
	wantStatus(t, res, http.StatusTooManyRequests)
	if res.Header.Get("Retry-After") == "" {
		t.Error("no Retry-After header")
	}
	// A link without a password has nothing to unlock, and the limit of one link leaves another alone.
	wantStatus(t, guesser.json(http.MethodPost, publicURL(other, "/unlock"), map[string]string{"password": "another password"}), http.StatusNoContent)
}

func TestPublicLinkStopsWithItsOwnerOrAlbum(t *testing.T) {
	e := setup(t)
	user := e.addUser("anna", auth.RoleUser)
	e.addUser("root", auth.RoleAdmin)
	anna, root := e.loggedIn("anna"), e.loggedIn("root")
	album := anna.newAlbum(t, "A")
	link := newLink(t, anna, album, map[string]any{})
	visitor := e.browser()
	wantStatus(t, visitor.get(publicURL(link, "")), http.StatusOK)

	wantStatus(t, root.json(http.MethodPatch, "/api/v1/users/"+itoa(user.ID), map[string]bool{"disabled": true}), http.StatusOK)
	wantStatus(t, visitor.get(publicURL(link, "")), http.StatusNotFound)
	wantStatus(t, root.json(http.MethodPatch, "/api/v1/users/"+itoa(user.ID), map[string]bool{"disabled": false}), http.StatusOK)
	wantStatus(t, visitor.get(publicURL(link, "")), http.StatusOK)

	anna = e.loggedIn("anna")
	wantStatus(t, anna.json(http.MethodDelete, "/api/v1/albums/"+itoa(album), nil), http.StatusNoContent)
	wantStatus(t, visitor.get(publicURL(link, "")), http.StatusNotFound)
}

func TestPublicLinkLimit(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	album := anna.newAlbum(t, "A")
	for range maxAlbumLinks {
		newLink(t, anna, album, map[string]any{})
	}
	wantStatus(t, anna.json(http.MethodPost, "/api/v1/albums/"+itoa(album)+"/links", map[string]any{}), http.StatusConflict)
}

func TestPublicLinkVideo(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")
	video := decode[uploadResult](t, anna.upload("video.mp4")).Photo.ID
	markReady(t, e, video)
	link := newLink(t, anna, anna.newAlbum(t, "Clips", video), map[string]any{})
	visitor := e.browser()
	photos := decode[[]publicPhotoJSON](t, visitor.get(publicURL(link, "/photos")))
	if len(photos) != 1 || photos[0].Kind != "video" {
		t.Fatalf("photos = %+v", photos)
	}
	res := visitor.get(publicURL(link, "/photos/"+itoa(video)+"/video"))
	wantStatus(t, res, http.StatusOK)
	if res.Header.Get("Content-Type") != "video/mp4" {
		t.Errorf("Content-Type = %q", res.Header.Get("Content-Type"))
	}
}
