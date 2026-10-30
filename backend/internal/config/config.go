// Package config reads application settings from the environment.
package config

import (
	"errors"
	"os"
)

// Config holds the application settings.
type Config struct {
	HTTPAddr    string
	DatabaseURL string
}

// Load reads the settings from the environment.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:    os.Getenv("HTTP_ADDR"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	return cfg, nil
}
