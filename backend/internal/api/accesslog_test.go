package api

import (
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func TestRequestLevel(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		duration    time.Duration
		contentType string
		want        slog.Level
	}{
		{"quick success", http.StatusOK, 10 * time.Millisecond, "application/json", slog.LevelDebug},
		{"not found", http.StatusNotFound, time.Millisecond, "application/json", slog.LevelInfo},
		{"server error", http.StatusInternalServerError, time.Millisecond, "application/json", slog.LevelInfo},
		{"slow", http.StatusOK, 2 * time.Second, "image/webp", slog.LevelInfo},
		{"long event stream", http.StatusOK, time.Hour, "text/event-stream", slog.LevelDebug},
	}
	for _, c := range cases {
		if got := requestLevel(c.status, c.duration, c.contentType); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
