// Package config reads application settings from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the application settings.
type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	LibraryDir    string
	CacheDir      string
	ImportDir     string
	ImportStable  time.Duration
	Workers       int
	PreviewFormat string
	CookieSecure  bool
}

// Load reads the settings from the environment.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:      getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		LibraryDir:    getenv("LIBRARY_DIR", "/data/library"),
		CacheDir:      getenv("CACHE_DIR", "/data/cache"),
		ImportDir:     getenv("IMPORT_DIR", "/data/import"),
		PreviewFormat: getenv("PREVIEW_FORMAT", "webp"),
		CookieSecure:  os.Getenv("COOKIE_INSECURE") != "true",
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.PreviewFormat != "webp" && cfg.PreviewFormat != "jpeg" {
		return Config{}, fmt.Errorf("PREVIEW_FORMAT must be webp or jpeg, not %q", cfg.PreviewFormat)
	}
	workers, err := strconv.Atoi(getenv("WORKERS", "2"))
	if err != nil || workers < 1 {
		return Config{}, errors.New("WORKERS must be a positive number")
	}
	cfg.Workers = workers
	stable, err := strconv.Atoi(getenv("IMPORT_STABLE_SECONDS", "60"))
	if err != nil || stable < 0 {
		return Config{}, errors.New("IMPORT_STABLE_SECONDS must be zero or a positive number")
	}
	cfg.ImportStable = time.Duration(stable) * time.Second
	return cfg, nil
}

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
