// Command fuchsia-photos runs the Fuchsia Photos server.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/denuser/fuchsia-photos/backend/db"
	"github.com/denuser/fuchsia-photos/backend/internal/api"
	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/config"
	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/jobs"
	"github.com/denuser/fuchsia-photos/backend/internal/media"
	"github.com/denuser/fuchsia-photos/backend/internal/metadata"
	"github.com/denuser/fuchsia-photos/backend/internal/ml"
	"github.com/denuser/fuchsia-photos/backend/internal/scan"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
	"github.com/denuser/fuchsia-photos/backend/internal/transcode"
	"github.com/denuser/fuchsia-photos/backend/internal/transcoder"
	"github.com/denuser/fuchsia-photos/backend/internal/web"
	"github.com/denuser/fuchsia-photos/backend/internal/webdav"
)

const usage = `usage: fuchsia-photos <command>

commands:
  serve                 apply pending migrations and start the server
  migrate               apply pending migrations and exit
  create-admin <login>  create an administrator, reading the password from stdin
  render-grid           queue new grid previews of all photos with the current
                        settings; a running server makes them
  render-previews       queue new grid and viewer previews of all photos from
                        their originals, as after a restore`

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	switch {
	case command == "serve" && len(args) == 1:
	case command == "migrate" && len(args) == 1:
	case command == "create-admin" && len(args) == 2:
	case command == "render-grid" && len(args) == 1:
	case command == "render-previews" && len(args) == 1:
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	setupLogging(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	switch command {
	case "serve":
		return serve(ctx, cfg, pool)
	case "create-admin":
		return createAdmin(ctx, pool, args[1])
	case "render-grid":
		count, err := jobs.QueueGridRenders(ctx, pool)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Queued new grid previews of %d photos\n", count)
	case "render-previews":
		count, err := jobs.QueuePreviewRenders(ctx, pool)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Queued new previews of %d photos\n", count)
	}
	return nil
}

func createAdmin(ctx context.Context, pool *pgxpool.Pool, login string) error {
	if err := auth.ValidateLogin(login); err != nil {
		return err
	}
	fmt.Fprint(os.Stderr, "Password: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return fmt.Errorf("read password: %w", err)
	}
	password := strings.TrimRight(line, "\r\n")
	if err := auth.ValidatePassword(password); err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = store.New(pool).CreateUser(ctx, store.CreateUserParams{Login: login, PasswordHash: hash, Role: auth.RoleAdmin})
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Created administrator %s\n", login)
	return nil
}

func serve(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) error {
	layout := storage.Layout{Library: cfg.LibraryDir, Cache: cfg.CacheDir}
	if err := layout.Ensure(); err != nil {
		return err
	}

	if err := media.Startup(); err != nil {
		return fmt.Errorf("start libvips: %w", err)
	}
	defer media.Shutdown()
	if err := media.CheckVideoTools(); err != nil {
		slog.Warn("videos cannot be processed until ffmpeg is installed", "error", err)
	}

	reader, err := metadata.NewReader()
	if err != nil {
		return err
	}
	defer reader.Close()

	copies := &transcode.Service{Queries: store.New(pool), Layout: layout}
	if cfg.TranscoderURL != "" {
		copies.Backend = transcoder.NewClient(cfg.TranscoderURL, cfg.TranscoderToken)
	}

	ingester := &ingest.Service{Pool: pool, Layout: layout}
	scanner := &scan.Service{Ingest: ingester, Dir: cfg.ImportDir, Stable: cfg.ImportStable}
	var mlClient *ml.Client
	if cfg.MLURL != "" {
		mlClient = ml.NewClient(cfg.MLURL, cfg.MLToken)
	}
	embedder := &ml.Service{
		Monitor: ml.NewMonitor(mlClient, cfg.MLModel),
		Client:  mlClient,
		Model:   cfg.MLModel,
		Layout:  layout,
		Queries: store.New(pool),
	}
	jobClient, err := jobs.NewClient(pool, jobs.Config{
		Layout:   layout,
		Metadata: reader,
		Workers:  cfg.Workers,
		Register: func(workers *river.Workers) {
			scanner.Register(workers)
			embedder.Register(workers)
			copies.Register(workers)
		},
		Queues: map[string]river.QueueConfig{
			ml.QueueML:      {MaxWorkers: 1},
			transcode.Queue: {MaxWorkers: 1},
		},
		PeriodicJobs: []*river.PeriodicJob{ml.PeriodicJob(), transcode.PeriodicJob()},
		Processed:    copies.Processed,
	})
	if err != nil {
		return fmt.Errorf("create job client: %w", err)
	}
	ingester.Jobs = jobClient
	copies.Jobs = jobClient
	if err := jobClient.Start(ctx); err != nil {
		return fmt.Errorf("start jobs: %w", err)
	}
	defer func() {
		// Jobs stopped here run again on the next start.
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := jobClient.StopAndCancel(stopCtx); err != nil {
			slog.Error("stop jobs", "error", err)
		}
	}()

	// A run starts as soon as the service comes up, rather than within a minute.
	embedder.Monitor.OnAvailable(func() {
		if _, err := jobClient.Insert(ctx, ml.EmbedPhotosArgs{}, nil); err != nil {
			slog.Error("queue embedding", "error", err)
		}
	})
	go embedder.Monitor.Run(ctx)
	// Started once the jobs run, so that a transcoder found at once queues
	// the copies that wait for it.
	go copies.Run(ctx)

	static, err := web.Handler()
	if err != nil {
		return err
	}
	handler := api.NewHandler(api.Deps{
		DB:           pool,
		Pool:         pool,
		Queries:      store.New(pool),
		Ingest:       ingester,
		Scan:         scanner,
		Layout:       layout,
		ML:           embedder,
		Transcode:    copies,
		CookieSecure: cfg.CookieSecure,
		WebDAV:       &webdav.Handler{Queries: store.New(pool), Ingest: ingester, Layout: layout},
		Static:       static,
	})
	return listen(ctx, cfg.HTTPAddr, handler)
}

func listen(ctx context.Context, addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// Ends long-lived event streams on shutdown.
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
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// setupLogging writes logs to stderr, where Docker collects them.
func setupLogging(cfg config.Config) {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	var handler slog.Handler = slog.NewTextHandler(os.Stderr, opts)
	if cfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))
}
