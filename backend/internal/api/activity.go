package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net/http"
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
	if response.status == 0 {
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
