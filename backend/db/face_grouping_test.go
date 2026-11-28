package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// TestFaceGroupingMigration checks 00048 on faces made before it: ignored
// faces become manual, and only photos with no ignored face and no face in a
// named group are scanned again.
func TestFaceGroupingMigration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool := freshDatabase(t, ctx, url)

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	dir, err := fs.Sub(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 47); err != nil {
		t.Fatal(err)
	}

	var owner int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (login, password_hash, role) VALUES ('anna', 'x', 'user') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	vector := "[1" + strings.Repeat(",0", 127) + "]"
	photo := func(n int) int64 {
		var id int64
		sha := hex.EncodeToString([]byte{byte(n)})
		if err := pool.QueryRow(ctx, `INSERT INTO photos (owner_id, sha256, mime, size, status)
			VALUES ($1, decode(repeat($2, 32), 'hex'), 'image/jpeg', 1, 'ready') RETURNING id`, owner, sha).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO face_scans (photo_id, model, sha256) SELECT id, 'm', sha256 FROM photos WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	group := func(name string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO people (owner_id, name) VALUES ($1, $2) RETURNING id`, owner, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	face := func(photo int64, person *int64) {
		if _, err := pool.Exec(ctx, `INSERT INTO photo_faces (photo_id, person_id, box, embedding) VALUES ($1, $2, '{0,0,0.1,0.1}', $3::halfvec)`, photo, person, vector); err != nil {
			t.Fatal(err)
		}
	}
	plain, named, ignored, empty := photo(1), photo(2), photo(3), photo(4)
	unnamed, grandma := group(""), group("Grandma")
	face(plain, &unnamed)
	face(named, &grandma)
	face(ignored, nil)

	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	scanned := func(photo int64) bool {
		var found bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM face_scans WHERE photo_id=$1)`, photo).Scan(&found); err != nil {
			t.Fatal(err)
		}
		return found
	}
	if scanned(plain) {
		t.Error("a photo with only automatic faces keeps its scan")
	}
	if !scanned(named) || !scanned(ignored) {
		t.Error("a photo with a named group or an ignored face lost its scan")
	}
	if !scanned(empty) {
		t.Error("a photo without faces lost its scan")
	}
	var manual bool
	if err := pool.QueryRow(ctx, `SELECT manual FROM photo_faces WHERE photo_id=$1`, ignored).Scan(&manual); err != nil || !manual {
		t.Errorf("ignored face manual=%v, %v", manual, err)
	}
	var groups []string
	rows, err := pool.Query(ctx, `SELECT name FROM people ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if groups, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil || len(groups) != 1 || groups[0] != "Grandma" {
		t.Errorf("groups after the migration %v, %v", groups, err)
	}
	var score float32
	var pending bool
	if err := pool.QueryRow(ctx, `SELECT retry_min_score, regroup_pending FROM face_settings`).Scan(&score, &pending); err != nil || score != 0.75 || !pending {
		t.Errorf("setting %v, regroup pending %v, %v", score, pending, err)
	}
}

func freshDatabase(t *testing.T, ctx context.Context, url string) *pgxpool.Pool {
	t.Helper()
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := "t_" + hex.EncodeToString(suffix)
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name+" TEMPLATE template0"); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		admin, err := pgx.Connect(ctx, url)
		if err != nil {
			t.Error(err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	return pool
}
