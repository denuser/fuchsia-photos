// Command fuchsia-photos-transcoder is the service that makes H.264 copies of
// videos on the graphics for the application. It runs in the image for one
// maker of graphics, next to the application, with the library and the cache
// at the same paths. See "Video copies" in docs/deployment.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/transcoder"
)

func main() {
	if err := run(); err != nil {
		slog.Error("transcoder stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(getenv("LOG_LEVEL", "info"))); err != nil {
		return errors.New("LOG_LEVEL must be debug, info, warn or error")
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler = slog.NewTextHandler(os.Stderr, opts)
	if getenv("LOG_FORMAT", "text") == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))

	token := os.Getenv("TRANSCODER_TOKEN")
	if token == "" {
		return errors.New("TRANSCODER_TOKEN is required: the same value as the application's")
	}
	server := &transcoder.Server{
		Token:   token,
		Library: getenv("LIBRARY_DIR", "/data/library"),
		Cache:   getenv("CACHE_DIR", "/data/cache"),
		Device:  getenv("VAAPI_DEVICE", "/dev/dri/renderD128"),
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Found now, so that the log says at once whether the graphics work; a
	// failure is tried again when the application asks.
	if _, err := server.Hardware(ctx); err != nil {
		slog.Warn("the graphics cannot make H.264 copies", "error", err)
	}

	addr := getenv("TRANSCODER_ADDR", ":8091")
	srv := &http.Server{
		Addr:    addr,
		Handler: server.Handler(),
		// No write timeout: a copy takes as long as it takes, and the
		// application's request ends when its job does.
		ReadHeaderTimeout: 10 * time.Second,
		// Stopping the service kills ffmpeg through the request's context.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", addr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
