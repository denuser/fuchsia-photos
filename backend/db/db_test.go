package db

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestMigrate(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()

	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// A second run must be a no-op, because the server migrates on every start.
	for range 2 {
		if err := Migrate(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}

	for _, table := range []string{"users", "sessions", "photos", "river_job"} {
		var found bool
		err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&found)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Errorf("table %s does not exist", table)
		}
	}

	t.Run("duplicate photo per owner is rejected", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)

		var owner int64
		err = tx.QueryRow(ctx, `INSERT INTO users (login, password_hash, role)
			VALUES ('alice', 'x', 'user') RETURNING id`).Scan(&owner)
		if err != nil {
			t.Fatal(err)
		}

		insert := `INSERT INTO photos (owner_id, sha256, mime, size)
			VALUES ($1, decode(repeat('ab', 32), 'hex'), 'image/jpeg', 1)`
		if _, err := tx.Exec(ctx, insert, owner); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, insert, owner)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Fatalf("got %v, want unique violation", err)
		}
	})
}
