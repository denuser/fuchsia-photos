// Package config reads application settings from the environment.
package config

import (
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the application settings.
type Config struct {
	ActivityLog              bool
	ActivityMaintenanceToken string
	HTTPAddr                 string
	DatabaseURL              string
	LibraryDir               string
	CacheDir                 string
	ImportDir                string
	ImportStable             time.Duration
	Workers                  int
	CookieSecure             bool
	LogLevel                 slog.Level
	// LogFormat is "text" or "json".
	LogFormat string
	// MLURL is the address of the ML service. Without it ML is off.
	MLURL   string
	MLToken string
	// MLModel is the model whose vectors the application uses, as the service
	// names it, with the version of its preprocessing.
	MLModel string
	// TranscoderURL is the address of the transcoder, which makes H.264
	// copies of videos on the graphics. Without it no copies are made.
	TranscoderURL   string
	TranscoderToken string
	// PublicURL is the address people open the application at, such as https://photos.example.org.
	// Redirect addresses for sign-in through a provider are built from it, never from request headers.
	PublicURL string
	// SessionSecret signs the short-lived cookie that carries a sign-in through a provider.
	SessionSecret string
	// OIDC lists the providers people may sign in with.
	OIDC []OIDCProvider
	// TOTPKey encrypts the secrets of two-factor sign-in. Without it two-factor sign-in cannot be turned on.
	TOTPKey string
}

// OIDCProvider is one OpenID Connect provider, set by OIDC_<n>_* variables with n from 1.
type OIDCProvider struct {
	// ID is n, the provider's number, which appears in the redirect address.
	ID           string
	Name         string
	Issuer       string
	ClientID     string
	ClientSecret string
}

const maxOIDCProviders = 9

// Load reads the settings from the environment.
func Load() (Config, error) {
	cfg := Config{
		ActivityLog:              os.Getenv("ACTIVITY_LOG") == "true",
		ActivityMaintenanceToken: os.Getenv("ACTIVITY_MAINTENANCE_TOKEN"),
		HTTPAddr:                 getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:              os.Getenv("DATABASE_URL"),
		LibraryDir:               getenv("LIBRARY_DIR", "/data/library"),
		CacheDir:                 getenv("CACHE_DIR", "/data/cache"),
		ImportDir:                getenv("IMPORT_DIR", "/data/import"),
		CookieSecure:             os.Getenv("COOKIE_INSECURE") != "true",
		MLURL:                    os.Getenv("ML_URL"),
		MLToken:                  os.Getenv("ML_TOKEN"),
		MLModel:                  getenv("ML_MODEL", "google/siglip2-base-patch16-384@1"),
		TranscoderURL:            os.Getenv("TRANSCODER_URL"),
		TranscoderToken:          os.Getenv("TRANSCODER_TOKEN"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.TranscoderURL != "" && cfg.TranscoderToken == "" {
		return Config{}, errors.New("TRANSCODER_TOKEN is required with TRANSCODER_URL")
	}
	cfg.PublicURL = strings.TrimRight(os.Getenv("PUBLIC_URL"), "/")
	cfg.SessionSecret = os.Getenv("SESSION_SECRET")
	cfg.TOTPKey = os.Getenv("TOTP_KEY")
	if cfg.TOTPKey != "" && len(cfg.TOTPKey) < 32 {
		return Config{}, errors.New("TOTP_KEY must be at least 32 characters, such as the output of: openssl rand -base64 32")
	}
	for n := 1; n <= maxOIDCProviders; n++ {
		prefix := "OIDC_" + strconv.Itoa(n) + "_"
		issuer := os.Getenv(prefix + "ISSUER")
		if issuer == "" {
			if os.Getenv(prefix+"CLIENT_ID") != "" {
				return Config{}, errors.New(prefix + "ISSUER is required with " + prefix + "CLIENT_ID")
			}
			continue
		}
		p := OIDCProvider{
			ID:           strconv.Itoa(n),
			Name:         getenv(prefix+"NAME", "Single sign-on"),
			Issuer:       issuer,
			ClientID:     os.Getenv(prefix + "CLIENT_ID"),
			ClientSecret: os.Getenv(prefix + "CLIENT_SECRET"),
		}
		if p.ClientID == "" || p.ClientSecret == "" {
			return Config{}, errors.New(prefix + "CLIENT_ID and " + prefix + "CLIENT_SECRET are required with " + prefix + "ISSUER")
		}
		cfg.OIDC = append(cfg.OIDC, p)
	}
	if len(cfg.OIDC) > 0 {
		if u, err := url.Parse(cfg.PublicURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Path != "" {
			return Config{}, errors.New("PUBLIC_URL is required with OIDC_1_ISSUER and must look like https://photos.example.org")
		}
		if len(cfg.SessionSecret) < 32 {
			return Config{}, errors.New("SESSION_SECRET of at least 32 characters is required with OIDC_1_ISSUER")
		}
	}
	workers, err := strconv.Atoi(getenv("WORKERS", "3"))
	if err != nil || workers < 1 {
		return Config{}, errors.New("WORKERS must be a positive number")
	}
	cfg.Workers = workers
	stable, err := strconv.Atoi(getenv("IMPORT_STABLE_SECONDS", "60"))
	if err != nil || stable < 0 {
		return Config{}, errors.New("IMPORT_STABLE_SECONDS must be zero or a positive number")
	}
	cfg.ImportStable = time.Duration(stable) * time.Second
	if err := cfg.LogLevel.UnmarshalText([]byte(getenv("LOG_LEVEL", "info"))); err != nil {
		return Config{}, errors.New("LOG_LEVEL must be debug, info, warn or error")
	}
	cfg.LogFormat = getenv("LOG_FORMAT", "text")
	if cfg.LogFormat != "text" && cfg.LogFormat != "json" {
		return Config{}, errors.New("LOG_FORMAT must be text or json")
	}
	return cfg, nil
}

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
