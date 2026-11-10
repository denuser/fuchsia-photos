package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// slowRequest is how long a request may take before it is logged at info level.
const slowRequest = time.Second

// logRequests logs every request: at debug level when it succeeds quickly, and
// at info level when it fails or is slow. WebDAV logs its own requests.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/webdav" || strings.HasPrefix(r.URL.Path, "/webdav/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		duration := time.Since(start)

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		slog.Log(r.Context(), requestLevel(status, duration, rec.Header().Get("Content-Type")), "request",
			"method", r.Method, "path", r.URL.Path, "status", status, "bytes", rec.bytes,
			"duration", duration.Round(time.Millisecond))
	})
}

func requestLevel(status int, duration time.Duration, contentType string) slog.Level {
	// An event stream stays open for as long as the page does.
	slow := duration >= slowRequest && contentType != "text/event-stream"
	if status >= 400 || slow {
		return slog.LevelInfo
	}
	return slog.LevelDebug
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController flush event streams and serve files.
func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
