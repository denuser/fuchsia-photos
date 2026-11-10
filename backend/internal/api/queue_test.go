package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/jobs"
)

func TestQueue(t *testing.T) {
	e := setup(t)
	e.addUser("root", auth.RoleAdmin)
	e.addUser("anna", auth.RoleUser)
	admin, user := e.loggedIn("root"), e.loggedIn("anna")
	const path = "/api/v1/queue"

	wantStatus(t, user.get(path), http.StatusForbidden)

	// Jobs are not run in these tests, so both uploads wait in the queue.
	user.addPhoto("landscape.jpg")
	broken := user.addPhoto("large.jpg")
	_, err := e.pool.Exec(context.Background(), `
		UPDATE river_job SET state = 'discarded', attempt = 3, finalized_at = now(),
			errors = ARRAY['{"at": "2026-11-10T03:00:00Z", "attempt": 3, "error": "decode failed"}'::jsonb]
		WHERE (args->>'photo_id')::bigint = $1`, broken)
	if err != nil {
		t.Fatal(err)
	}

	stats := decode[jobs.QueueStats](t, admin.get(path))
	var process *jobs.KindStats
	for i := range stats.Kinds {
		if stats.Kinds[i].Kind == "process_photo" {
			process = &stats.Kinds[i]
		}
	}
	if process == nil {
		t.Fatalf("kinds = %+v, want process_photo", stats.Kinds)
	}
	if process.Waiting != 1 || process.FailedDay != 1 || process.Running != 0 {
		t.Errorf("process_photo = %+v, want 1 waiting and 1 failed", *process)
	}
	if len(stats.Errors) != 1 {
		t.Fatalf("errors = %+v, want one", stats.Errors)
	}
	got := stats.Errors[0]
	if got.Error != "decode failed" || got.PhotoID == nil || *got.PhotoID != broken || got.Attempt != 3 || got.State != "discarded" {
		t.Errorf("error = %+v", got)
	}
}

func TestQueueEvents(t *testing.T) {
	e := setup(t)
	e.addUser("root", auth.RoleAdmin)
	e.addUser("anna", auth.RoleUser)
	admin, user := e.loggedIn("root"), e.loggedIn("anna")
	const path = "/api/v1/queue/events"

	wantStatus(t, user.get(path), http.StatusForbidden)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := admin.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if got := res.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q", got)
	}
	events := newEventReader(res.Body)
	next := func() jobs.QueueStats {
		t.Helper()
		data, err := events.next()
		if err != nil {
			t.Fatal(err)
		}
		var stats jobs.QueueStats
		if err := json.Unmarshal(data, &stats); err != nil {
			t.Fatal(err)
		}
		return stats
	}

	// The first event comes at once, and the next one when the queue changes.
	if first := next(); len(first.Kinds) != 0 {
		t.Errorf("first event = %+v, want an empty queue", first)
	}
	user.addPhoto("landscape.jpg")
	second := next()
	if len(second.Kinds) != 1 || second.Kinds[0].Kind != "process_photo" || second.Kinds[0].Waiting != 1 {
		t.Errorf("second event = %+v, want one photo waiting", second)
	}
}
