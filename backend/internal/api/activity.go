package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

const activityWriteTimeout = 250 * time.Millisecond

func (s *server) maintenanceLogin(r *http.Request) bool {
	token := r.Header.Get("X-Activity-Maintenance")
	if token == "" || s.ActivityMaintenanceToken == "" {
		return false
	}
	got := sha256.Sum256([]byte(token))
	want := sha256.Sum256([]byte(s.ActivityMaintenanceToken))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

func (s *server) recordActivity(r *http.Request, response *responseRecorder, event *store.RecordActivityParams) {
	// A link of a provider to an account starts from a session and has no event to record.
	if response.status == 0 || event.Action == "" {
		return
	}
	event.HttpStatus = int32(response.status)
	if event.Outcome == "" {
		switch {
		case response.status >= 500:
			event.Outcome = "error"
		case event.Action == "login" && response.status == http.StatusBadRequest:
			event.Outcome = "invalid_request"
		default:
			event.Outcome = "rejected"
		}
	}
	event.Browser, event.Os = clientFamilies(r.UserAgent())
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), activityWriteTimeout)
	defer cancel()
	if err := s.Queries.RecordActivity(ctx, *event); err != nil {
		slog.WarnContext(ctx, "record activity", "action", event.Action, "outcome", event.Outcome, "error", err)
	}
}

func clientFamilies(agent string) (browser, os string) {
	agent = strings.ToLower(agent)
	switch {
	case strings.Contains(agent, "edg/"), strings.Contains(agent, "edga/"), strings.Contains(agent, "edgios/"):
		browser = "Edge"
	case strings.Contains(agent, "opr/"):
		browser = "Opera"
	case strings.Contains(agent, "samsungbrowser/"):
		browser = "Samsung Internet"
	case strings.Contains(agent, "firefox/"), strings.Contains(agent, "fxios/"):
		browser = "Firefox"
	case strings.Contains(agent, "chrome/"), strings.Contains(agent, "crios/"):
		browser = "Chrome"
	case strings.Contains(agent, "safari/"):
		browser = "Safari"
	default:
		browser = "Unknown"
	}
	switch {
	case strings.Contains(agent, "android"):
		os = "Android"
	case strings.Contains(agent, "iphone"), strings.Contains(agent, "ipad"), strings.Contains(agent, "ipod"):
		os = "iOS"
	case strings.Contains(agent, "windows"):
		os = "Windows"
	case strings.Contains(agent, "macintosh"):
		os = "macOS"
	case strings.Contains(agent, "cros"):
		os = "ChromeOS"
	case strings.Contains(agent, "linux"):
		os = "Linux"
	default:
		os = "Unknown"
	}
	return browser, os
}

const activityPageSize = 100

type activityDayJSON struct {
	Day                 string `json:"day"`
	Logins              int64  `json:"logins"`
	RejectedLogins      int64  `json:"rejectedLogins"`
	InvalidLogins       int64  `json:"invalidLogins"`
	LoginErrors         int64  `json:"loginErrors"`
	NewFiles            int64  `json:"newFiles"`
	Duplicates          int64  `json:"duplicates"`
	Restored            int64  `json:"restored"`
	RejectedUploads     int64  `json:"rejectedUploads"`
	UploadErrors        int64  `json:"uploadErrors"`
	SessionsWithUploads int64  `json:"sessionsWithUploads"`
}

type activityEventJSON struct {
	ID         int64     `json:"id"`
	OccurredAt time.Time `json:"occurredAt"`
	Action     string    `json:"action"`
	Outcome    string    `json:"outcome"`
	HTTPStatus int32     `json:"httpStatus"`
	AccountID  *int64    `json:"accountId"`
	SessionID  *string   `json:"sessionId"`
	Browser    string    `json:"browser"`
	OS         string    `json:"os"`
	MediaType  *string   `json:"mediaType"`
	SizeBytes  *int64    `json:"sizeBytes"`
	MediaID    *int64    `json:"mediaId"`
}

type activityJSON struct {
	Enabled bool                `json:"enabled"`
	Days    []activityDayJSON   `json:"days"`
	Events  []activityEventJSON `json:"events"`
	// Next is the "before" value of the next page, or null after the last one.
	Next *int64 `json:"next"`
}

// getActivity answers the administrator's page of the journal: the daily
// summary of the last 30 days and one page of events, newest first.
func (s *server) getActivity(w http.ResponseWriter, r *http.Request) {
	before := int64(math.MaxInt64)
	if v := r.URL.Query().Get("before"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid before")
			return
		}
		before = id
	}
	days, err := s.Queries.SummarizeActivity(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	rows, err := s.Queries.ListActivityEvents(r.Context(), store.ListActivityEventsParams{Before: before, RowLimit: activityPageSize + 1})
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := activityJSON{Enabled: s.ActivityLog, Days: make([]activityDayJSON, len(days))}
	for i, d := range days {
		out.Days[i] = activityDayJSON(d)
	}
	if len(rows) > activityPageSize {
		rows = rows[:activityPageSize]
		out.Next = &rows[len(rows)-1].ID
	}
	out.Events = make([]activityEventJSON, len(rows))
	for i, e := range rows {
		out.Events[i] = activityEventJSON{
			ID: e.ID, OccurredAt: e.OccurredAt, Action: e.Action, Outcome: e.Outcome, HTTPStatus: e.HttpStatus,
			AccountID: e.AccountID, SessionID: e.SessionID, Browser: e.Browser, OS: e.Os,
			MediaType: e.MediaType, SizeBytes: e.SizeBytes, MediaID: e.MediaID,
		}
	}
	writeJSON(w, http.StatusOK, out)
}
