// Package transcoder is the service that makes H.264 copies of videos on the
// graphics, and the application's client of it. The service runs in an image
// of its own for each maker of graphics, which carries the drivers, so the
// application's image stays the same everywhere. Both see the library and the
// cache at the same paths: the application sends the paths of the video and
// of the copy, and the service writes the copy where it is told. The queue,
// the settings and the database stay in the application. See "Video copies"
// in docs/architecture.md.
package transcoder

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/vaapi"
)

// Request asks for the copy of the video at In, written to Out.
type Request struct {
	In  string `json:"in"`
	Out string `json:"out"`
	vaapi.Source
}

// Server answers the application's requests.
type Server struct {
	// Token is the bearer token the application sends.
	Token string
	// Library and Cache are the folders the service may read videos from;
	// it writes copies into Cache only.
	Library string
	Cache   string
	// Detect finds the graphics; Transcode makes a copy. They default to
	// vaapi.DetectHardware on Device and vaapi.Transcode.
	Device    string
	Detect    func(ctx context.Context, device string) (vaapi.Hardware, error)
	Transcode func(ctx context.Context, in, out string, video vaapi.Source, hw vaapi.Hardware) error

	mu sync.Mutex
	hw *vaapi.Hardware
	// busy makes one copy at a time, as the graphics would only share
	// themselves between copies.
	busy sync.Mutex
}

// Handler returns the routes of the service.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hardware", s.getHardware)
	mux.HandleFunc("POST /transcode", s.transcode)
	return s.authorize(mux)
}

func (s *Server) authorize(next http.Handler) http.Handler {
	want := []byte("Bearer " + s.Token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Hardware returns the graphics, finding them on the first call that
// succeeds, so that a driver fixed after the start is picked up.
func (s *Server) Hardware(ctx context.Context) (vaapi.Hardware, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hw != nil {
		return *s.hw, nil
	}
	detect := s.Detect
	if detect == nil {
		detect = vaapi.DetectHardware
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	hw, err := detect(ctx, s.Device)
	if err != nil {
		return vaapi.Hardware{}, err
	}
	s.hw = &hw
	slog.Info("graphics can make H.264 copies", "device", hw.Device, "vendor", hw.Vendor, "driver", hw.Driver, "low_power", hw.LowPower)
	return hw, nil
}

func (s *Server) getHardware(w http.ResponseWriter, r *http.Request) {
	hw, err := s.Hardware(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, hw)
}

func (s *Server) transcode(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	if !within(req.In, s.Library) && !within(req.In, s.Cache) {
		writeError(w, http.StatusBadRequest, "the video is outside the library and the cache: "+req.In)
		return
	}
	if !within(req.Out, s.Cache) {
		writeError(w, http.StatusBadRequest, "the copy is outside the cache: "+req.Out)
		return
	}
	if req.Width <= 0 || req.Height <= 0 {
		writeError(w, http.StatusBadRequest, "the size of the video is missing")
		return
	}
	hw, err := s.Hardware(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	// The application has just read the video and made the file for the
	// copy, so a service that cannot open them is set up wrong, which is not
	// the video's fault.
	if err := checkFiles(req.In, req.Out); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error()+
			"; the transcoder needs the library and the cache at the same paths as the application, and to run as its user")
		return
	}
	transcode := s.Transcode
	if transcode == nil {
		transcode = vaapi.Transcode
	}
	s.busy.Lock()
	defer s.busy.Unlock()
	// The application gave up, or the service is stopping: the request's
	// context ends and ffmpeg is killed.
	if err := r.Context().Err(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "the request ended: "+err.Error())
		return
	}
	start := time.Now()
	if err := transcode(r.Context(), req.In, req.Out, req.Source, hw); err != nil {
		if ctxErr := r.Context().Err(); ctxErr != nil {
			writeError(w, http.StatusServiceUnavailable, "the request ended: "+ctxErr.Error())
			return
		}
		slog.Warn("make video copy", "in", req.In, "error", err)
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	slog.Info("made video copy", "in", req.In, "seconds", time.Since(start).Seconds())
	writeJSON(w, http.StatusOK, struct{}{})
}

// checkFiles opens the video for reading and the copy's file for writing,
// without changing either.
func checkFiles(in, out string) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	f.Close()
	f, err = os.OpenFile(out, os.O_WRONLY, 0)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// The folder of the copy is missing or not shared with the
			// application.
			return fmt.Errorf("the file for the copy is missing: %w", err)
		}
		return err
	}
	return f.Close()
}

// within tells whether path is an absolute, clean path inside dir.
func within(path, dir string) bool {
	if dir == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	dir = filepath.Clean(dir)
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		slog.Debug("write response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
