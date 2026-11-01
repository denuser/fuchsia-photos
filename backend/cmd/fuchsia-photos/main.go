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

	"github.com/denuser/fuchsia-photos/backend/db"
	"github.com/denuser/fuchsia-photos/backend/internal/api"
	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/config"
	"github.com/denuser/fuchsia-photos/backend/internal/ingest"
	"github.com/denuser/fuchsia-photos/backend/internal/jobs"
	"github.com/denuser/fuchsia-photos/backend/internal/media"
	"github.com/denuser/fuchsia-photos/backend/internal/metadata"
	"github.com/denuser/fuchsia-photos/backend/internal/scan"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
	"github.com/denuser/fuchsia-photos/backend/internal/web"
)

const usage = `usage: fuchsia-photos <command>

commands:
  serve                 apply pending migrations and start the server
  migrate               apply pending migrations and exit
  create-admin <login>  create an administrator, reading the password from stdin`

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
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

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

	reader, err := metadata.NewReader()
	if err != nil {
		return err
	}
	defer reader.Close()

	ingester := &ingest.Service{Pool: pool, Layout: layout}
	scanner := &scan.Service{Ingest: ingester, Dir: cfg.ImportDir, Stable: cfg.ImportStable}
	jobClient, err := jobs.NewClient(pool, jobs.Config{
		Layout:   layout,
		Metadata: reader,
		Format:   media.Format(cfg.PreviewFormat),
		Workers:  cfg.Workers,
		Register: scanner.Register,
	})
	if err != nil {
		return fmt.Errorf("create job client: %w", err)
	}
	ingester.Jobs = jobClient
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

	static, err := web.Handler()
	if err != nil {
		return err
	}
	handler := api.NewHandler(api.Deps{
		DB:           pool,
		Queries:      store.New(pool),
		Ingest:       ingester,
		Scan:         scanner,
		Layout:       layout,
		CookieSecure: cfg.CookieSecure,
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
