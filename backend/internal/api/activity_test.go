package api

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/magellanica-app/magellanica-photos/backend/internal/auth"
)

func TestActivityJournal(t *testing.T) {
	e := setupWith(t, func(d *Deps) {
		d.ActivityLog = true
		d.ActivityMaintenanceToken = "maintenance-secret"
	})
	user := e.addUser("demo", auth.RoleUser)
	b := e.browser()
	login := func(body, marker string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, e.server.URL+"/api/v1/login", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 15) Chrome/130.0.0.0")
		if marker != "" {
			req.Header.Set("X-Activity-Maintenance", marker)
		}
		return b.do(req)
	}
	const valid = `{"login":"demo","password":"password demo"}`
	wantStatus(t, login(valid, "maintenance-secret"), http.StatusOK)
	wantStatus(t, login("{", ""), http.StatusBadRequest)
	wantStatus(t, login(`{"login":"someone","password":"wrong"}`, ""), http.StatusUnauthorized)
	wantStatus(t, login(`{"login":"demo","password":"wrong"}`, ""), http.StatusUnauthorized)
	wantStatus(t, login(valid, "invalid-marker"), http.StatusOK)
	wantStatus(t, b.json(http.MethodPost, "/api/v1/photos", nil), http.StatusBadRequest)

	res := b.upload("landscape.jpg")
	wantStatus(t, res, http.StatusCreated)
	photo := decode[uploadResult](t, res).Photo
	wantStatus(t, b.upload("landscape.jpg"), http.StatusOK)
	wantStatus(t, b.json(http.MethodPost, "/api/v1/photos/trash", ids(photo.ID)), http.StatusNoContent)
	wantStatus(t, b.upload("landscape.jpg"), http.StatusOK)

	ctx := context.Background()
	rows, err := e.pool.Query(ctx, `
		SELECT action, outcome, http_status, account_id, session_id, browser, os, media_id, generation
		FROM activity.events ORDER BY id
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var session string
	for _, expected := range []struct {
		action, outcome string
		status          int
		account, media  bool
	}{
		{"login", "invalid_request", 400, false, false},
		{"login", "rejected", 401, false, false},
		{"login", "rejected", 401, true, false},
		{"login", "succeeded", 200, true, false},
		{"upload", "rejected", 400, true, false},
		{"upload", "accepted", 201, true, true},
		{"upload", "duplicate", 200, true, true},
		{"upload", "restored", 200, true, true},
	} {
		if !rows.Next() {
			t.Fatalf("missing %v: %v", expected, rows.Err())
		}
		var action, outcome, browser, os string
		var status int
		var account, media, generation *int64
		var sessionID *string
		if err := rows.Scan(&action, &outcome, &status, &account, &sessionID, &browser, &os, &media, &generation); err != nil {
			t.Fatal(err)
		}
		if action != expected.action || outcome != expected.outcome || status != expected.status {
			t.Errorf("got %s %s %d, want %v", action, outcome, status, expected)
		}
		if (account != nil) != expected.account || account != nil && *account != user.ID {
			t.Errorf("%s: account = %v", outcome, account)
		}
		if (media != nil) != expected.media || media != nil && *media != photo.ID ||
			(generation != nil) != expected.media || generation != nil && *generation != 1 {
			t.Errorf("%s: media = %v, generation = %v", outcome, media, generation)
		}
		if outcome == "succeeded" && sessionID != nil {
			session = *sessionID
		}
		if action == "upload" || outcome == "succeeded" {
			if sessionID == nil || session == "" || *sessionID != session {
				t.Errorf("%s: missing or unmatched session ID", outcome)
			}
		} else if sessionID != nil {
			t.Errorf("%s: rejected login has a session", outcome)
		}
		if action == "login" && (browser != "Chrome" || os != "Android") {
			t.Errorf("client = %s on %s", browser, os)
		}
	}
	if rows.Next() || rows.Err() != nil {
		t.Fatalf("unexpected journal rows or error: %v", rows.Err())
	}

	wantStatus(t, b.upload("video.mp4"), http.StatusCreated)
	var videoMIME string
	if err := e.pool.QueryRow(ctx, "SELECT media_type FROM activity.events ORDER BY id DESC LIMIT 1").Scan(&videoMIME); err != nil {
		t.Fatal(err)
	}
	if videoMIME != "video/mp4" {
		t.Errorf("journal video media type = %q", videoMIME)
	}

	album := decode[albumJSON](t, b.json(http.MethodPost, "/api/v1/albums", map[string]string{"name": "Trip"}))
	if _, err := e.pool.Exec(ctx, `
		CREATE FUNCTION reject_album_insert() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'album unavailable';
		END;
		$$;
		CREATE TRIGGER reject_album_insert BEFORE INSERT ON album_photos
		FOR EACH ROW EXECUTE FUNCTION reject_album_insert();
	`); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../media/testdata/rotated.jpg")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, b.uploadTo("/api/v1/photos?album="+itoa(album.ID), "rotated.jpg", data), http.StatusInternalServerError)
	var outcome, mime string
	var status, size int
	if err := e.pool.QueryRow(ctx, `
		SELECT outcome, http_status, media_type, size_bytes FROM activity.events ORDER BY id DESC LIMIT 1
	`).Scan(&outcome, &status, &mime, &size); err != nil {
		t.Fatal(err)
	}
	if outcome != "accepted" || status != 500 || mime != "image/jpeg" || size != len(data) {
		t.Errorf("album failure journal entry = %s %d %s %d", outcome, status, mime, size)
	}

	if _, err := e.pool.Exec(ctx, "ALTER TABLE activity.events RENAME TO unavailable"); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, login(valid, ""), http.StatusOK)
	wantStatus(t, b.upload("rotated.jpg"), http.StatusOK)
}

func TestActivityJournalDisabled(t *testing.T) {
	e := setup(t)
	e.addUser("demo", auth.RoleUser)
	wantStatus(t, e.loggedIn("demo").upload("landscape.jpg"), http.StatusCreated)
	var count int
	if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM activity.events").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("got %d events with recording disabled", count)
	}
}

func TestActivityPage(t *testing.T) {
	e := setupWith(t, func(d *Deps) { d.ActivityLog = true })
	e.addUser("demo", auth.RoleUser)
	e.addUser("boss", auth.RoleAdmin)
	wantStatus(t, e.loggedIn("demo").get("/api/v1/activity"), http.StatusForbidden)
	wantStatus(t, e.browser().get("/api/v1/activity"), http.StatusUnauthorized)

	// A page of the journal holds 100 events, so the second page needs more.
	if _, err := e.pool.Exec(context.Background(), `
		INSERT INTO activity.events (action, outcome, http_status, browser, os)
		SELECT 'login', 'rejected', 401, 'Firefox', 'Linux' FROM generate_series(1, 120)
	`); err != nil {
		t.Fatal(err)
	}
	admin := e.loggedIn("boss")
	wantStatus(t, admin.get("/api/v1/activity?before=x"), http.StatusBadRequest)
	first := decode[activityJSON](t, admin.get("/api/v1/activity"))
	if !first.Enabled || len(first.Events) != activityPageSize || first.Next == nil || *first.Next != first.Events[99].ID {
		t.Fatalf("first page: enabled %v, %d events, next %v", first.Enabled, len(first.Events), first.Next)
	}
	if first.Events[0].ID < first.Events[1].ID {
		t.Errorf("events are not newest first: %+v", first.Events[:2])
	}
	if len(first.Days) != 1 || first.Days[0].RejectedLogins != 120 || first.Days[0].Logins != 2 {
		t.Errorf("days = %+v", first.Days)
	}
	second := decode[activityJSON](t, admin.get("/api/v1/activity?before="+itoa(*first.Next)))
	// 120 rejected logins and the two successful ones make 122 events.
	if len(second.Events) != 22 || second.Next != nil || second.Events[0].ID >= *first.Next {
		t.Errorf("second page: %d events, next %v", len(second.Events), second.Next)
	}
}
