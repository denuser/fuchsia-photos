package transcoder

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/vaapi"
)

var intel = vaapi.Hardware{Device: "/dev/dri/renderD128", Driver: "Intel iHD driver", Vendor: vaapi.VendorIntel}

type env struct {
	server  *Server
	http    *httptest.Server
	client  *Client
	library string
	cache   string
}

func setup(t *testing.T, transcode func(ctx context.Context, in, out string, video vaapi.Source, hw vaapi.Hardware) error) env {
	t.Helper()
	library, cache := t.TempDir(), t.TempDir()
	s := &Server{
		Token:     "secret",
		Library:   library,
		Cache:     cache,
		Device:    "/dev/dri/renderD128",
		Detect:    func(context.Context, string) (vaapi.Hardware, error) { return intel, nil },
		Transcode: transcode,
	}
	h := httptest.NewServer(s.Handler())
	t.Cleanup(h.Close)
	return env{s, h, NewClient(h.URL, "secret"), library, cache}
}

// touch makes a file, as the application has the video and the file for its
// copy before it calls the service.
func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeCopy(_ context.Context, in, out string, _ vaapi.Source, _ vaapi.Hardware) error {
	return os.WriteFile(out, []byte("copy of "+filepath.Base(in)), 0o600)
}

func TestTranscode(t *testing.T) {
	e := setup(t, writeCopy)
	ctx := context.Background()
	hw, err := e.client.Hardware(ctx)
	if err != nil || hw != intel {
		t.Fatalf("hardware = %+v, %v", hw, err)
	}

	video := vaapi.Source{Width: 3840, Height: 2160, HDR: true}
	out := filepath.Join(e.cache, "previews", "01", "1-copy.mp4")
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		t.Fatal(err)
	}
	// From the library, and from the cache, where the motion of a motion
	// photo lives.
	for _, in := range []string{filepath.Join(e.library, "originals", "1", "ab", "ab.mov"), filepath.Join(e.cache, "previews", "01", "1-motion.mp4")} {
		touch(t, in)
		touch(t, out)
		if err := e.client.Transcode(ctx, in, out, video); err != nil {
			t.Fatalf("transcode %s: %v", in, err)
		}
		if data, _ := os.ReadFile(out); string(data) != "copy of "+filepath.Base(in) {
			t.Errorf("copy = %q", data)
		}
	}
}

// Only the library and the cache are read, and only the cache is written.
func TestPaths(t *testing.T) {
	e := setup(t, writeCopy)
	ctx := context.Background()
	video := vaapi.Source{Width: 1920, Height: 1080}
	in := filepath.Join(e.library, "a.mov")
	out := filepath.Join(e.cache, "a.mp4")
	for name, paths := range map[string][2]string{
		"input elsewhere":   {"/etc/passwd", out},
		"input relative":    {"a.mov", out},
		"input climbs out":  {e.library + "/../a.mov", out},
		"input next to it":  {e.cache + "2/a.mov", out},
		"output in library": {in, filepath.Join(e.library, "a.mp4")},
		"output climbs out": {in, e.cache + "/x/../../a.mp4"},
		"output is cache":   {in, e.cache},
	} {
		err := e.client.Transcode(ctx, paths[0], paths[1], video)
		if err == nil || errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "400") {
			t.Errorf("%s: %v, want a bad request", name, err)
		}
	}
	if err := e.client.Transcode(ctx, in, out, vaapi.Source{}); err == nil {
		t.Error("no size: no error")
	}
}

func TestToken(t *testing.T) {
	e := setup(t, writeCopy)
	for _, token := range []string{"", "wrong"} {
		_, err := NewClient(e.http.URL, token).Hardware(context.Background())
		if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "401") {
			t.Errorf("token %q: %v", token, err)
		}
	}
}

// A copy that ffmpeg cannot make is the video's failure; a service that is
// gone, or has no working graphics, is not.
func TestErrors(t *testing.T) {
	e := setup(t, func(context.Context, string, string, vaapi.Source, vaapi.Hardware) error {
		return errors.New("exit status 1: Invalid data found when processing input")
	})
	ctx := context.Background()
	in, out := filepath.Join(e.library, "a.mov"), filepath.Join(e.cache, "a.mp4")
	touch(t, in)
	touch(t, out)
	video := vaapi.Source{Width: 1920, Height: 1080}
	err := e.client.Transcode(ctx, in, out, video)
	if err == nil || errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "Invalid data") {
		t.Errorf("ffmpeg failure: %v", err)
	}

	broken := setup(t, writeCopy)
	broken.server.Detect = func(context.Context, string) (vaapi.Hardware, error) {
		return vaapi.Hardware{}, errors.New("vainfo failed")
	}
	if _, err := broken.client.Hardware(ctx); !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "vainfo failed") {
		t.Errorf("no graphics: %v", err)
	}
	if err := broken.client.Transcode(ctx, filepath.Join(broken.library, "a.mov"), filepath.Join(broken.cache, "a.mp4"), video); !errors.Is(err, ErrUnavailable) {
		t.Errorf("no graphics, transcode: %v", err)
	}

	e.http.Close()
	if err := e.client.Transcode(ctx, in, out, video); !errors.Is(err, ErrUnavailable) {
		t.Errorf("service gone: %v", err)
	}
}

// The graphics are found once, and again after a failure.
func TestDetectRetried(t *testing.T) {
	e := setup(t, writeCopy)
	var calls atomic.Int32
	e.server.Detect = func(context.Context, string) (vaapi.Hardware, error) {
		if calls.Add(1) == 1 {
			return vaapi.Hardware{}, errors.New("not yet")
		}
		return intel, nil
	}
	ctx := context.Background()
	if _, err := e.client.Hardware(ctx); err == nil {
		t.Fatal("first detection: no error")
	}
	for range 2 {
		if _, err := e.client.Hardware(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("detected %d times, want 2", n)
	}
}

// When the application gives up, ffmpeg is stopped.
func TestCancel(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	e := setup(t, func(ctx context.Context, _, _ string, _ vaapi.Source, _ vaapi.Hardware) error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	in, out := filepath.Join(e.library, "a.mov"), filepath.Join(e.cache, "a.mp4")
	touch(t, in)
	touch(t, out)
	errc := make(chan error, 1)
	go func() {
		errc <- e.client.Transcode(ctx, in, out, vaapi.Source{Width: 1920, Height: 1080})
	}()
	<-started
	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the copy went on after the request ended")
	}
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v", err)
	}
}

// A service that cannot open the files is set up wrong: the copies wait for
// it to be fixed rather than fail.
func TestFilesNotShared(t *testing.T) {
	e := setup(t, writeCopy)
	ctx := context.Background()
	in, out := filepath.Join(e.library, "a.mov"), filepath.Join(e.cache, "a.mp4")
	video := vaapi.Source{Width: 1920, Height: 1080}
	touch(t, in)
	if err := e.client.Transcode(ctx, in, out, video); !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "same paths") {
		t.Errorf("no file for the copy: %v", err)
	}
	if err := e.client.Transcode(ctx, filepath.Join(e.library, "b.mov"), out, video); !errors.Is(err, ErrUnavailable) {
		t.Errorf("no video: %v", err)
	}
	touch(t, out)
	if err := os.Chmod(out, 0o400); err != nil {
		t.Fatal(err)
	}
	if os.Getuid() != 0 {
		if err := e.client.Transcode(ctx, in, out, video); !errors.Is(err, ErrUnavailable) {
			t.Errorf("read-only copy: %v", err)
		}
	}
}

func TestWithin(t *testing.T) {
	for path, want := range map[string]bool{
		"/data/cache/a.mp4":       true,
		"/data/cache/x/a.mp4":     true,
		"/data/cache":             false,
		"/data/cache/":            false,
		"/data/cache2/a.mp4":      false,
		"/data/cache/../a.mp4":    false,
		"/data/cache/./a.mp4":     false,
		"data/cache/a.mp4":        false,
		"/data/library/a.mp4":     false,
		"/data/cache/x/../a.mp4":  false,
		"/data/cache//a.mp4":      false,
		"/data/cache/a.mp4/../..": false,
	} {
		if got := within(path, "/data/cache/"); got != want {
			t.Errorf("within(%q) = %v, want %v", path, got, want)
		}
	}
	if within("/a", "") {
		t.Error("an empty folder contains /a")
	}
}
