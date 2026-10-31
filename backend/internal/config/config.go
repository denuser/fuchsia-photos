// Package config reads application settings from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// Config holds the application settings.
type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	LibraryDir    string
	CacheDir      string
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
	return cfg, nil
}

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
