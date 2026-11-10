package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const (
	eventInterval  = 2 * time.Second
	keepAliveEvery = 15 // ticks
	maxEventPhotos = 1000
	// updated_at is set when a transaction starts, so a row can become visible
	// slightly after its timestamp. Clients apply repeated changes harmlessly.
	eventOverlap = 5 * time.Second
)

// photoEvents streams changes to the user's photos and their processing counts.
func (s *server) photoEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	controller := http.NewResponseController(w)
	if err := controller.Flush(); err != nil {
		return
	}

	owner := currentUser(r).ID
	ctx := r.Context()
	since := time.Now().Add(-eventOverlap)
	lastPending, lastFailed := int64(-1), int64(-1)
	idleTicks := 0

	ticker := time.NewTicker(eventInterval)
	defer ticker.Stop()
	for {
		now := time.Now()
		changed, err := s.Queries.ListChangedPhotos(ctx, store.ListChangedPhotosParams{OwnerID: owner, Since: since, MaxRows: maxEventPhotos})
		if err != nil {
			return
		}
		counts, err := s.Queries.CountPhotosByStatus(ctx, owner)
		if err != nil {
			return
		}
		// When the batch is full, continue from its end rather than skip the rest.
		if len(changed) == maxEventPhotos {
			since = changed[len(changed)-1].UpdatedAt
		} else {
			since = now.Add(-eventOverlap)
		}

		if len(changed) > 0 || counts.Pending != lastPending || counts.Failed != lastFailed {
			lastPending, lastFailed = counts.Pending, counts.Failed
			idleTicks = 0
			showLocation, err := s.showLocation(ctx)
			if err != nil {
				return
			}
			payload, err := json.Marshal(map[string]any{
				"pending": counts.Pending,
				"failed":  counts.Failed,
				"photos":  toPhotosJSON(changed, showLocation),
			})
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "event: photos\ndata: %s\n\n", payload); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		} else if idleTicks++; idleTicks >= keepAliveEvery {
			idleTicks = 0
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// queueEvents streams the state of the job queue: once at the start, then
// whenever it changes. River does not report finished jobs, so the stream reads
// the queue at the same interval as photo events.
func (s *server) queueEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	controller := http.NewResponseController(w)
	if err := controller.Flush(); err != nil {
		return
	}

	ctx := r.Context()
	var last []byte
	idleTicks := 0
	ticker := time.NewTicker(eventInterval)
	defer ticker.Stop()
	for {
		stats, err := s.readQueue(ctx)
		if err != nil {
			return
		}
		payload, err := json.Marshal(stats)
		if err != nil {
			return
		}
		if !bytes.Equal(payload, last) {
			last = payload
			idleTicks = 0
			if _, err := fmt.Fprintf(w, "event: queue\ndata: %s\n\n", payload); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		} else if idleTicks++; idleTicks >= keepAliveEvery {
			idleTicks = 0
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
