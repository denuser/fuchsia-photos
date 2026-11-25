package config

import (
	"strings"
	"testing"
)

func setenv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, name := range []string{"PUBLIC_URL", "SESSION_SECRET", "TOTP_KEY"} {
		t.Setenv(name, "")
	}
	for n := 1; n <= maxOIDCProviders; n++ {
		for _, name := range []string{"NAME", "ISSUER", "CLIENT_ID", "CLIENT_SECRET"} {
			t.Setenv("OIDC_"+string(rune('0'+n))+"_"+name, "")
		}
	}
	t.Setenv("DATABASE_URL", "postgres://x")
	for name, value := range env {
		t.Setenv(name, value)
	}
}

func TestLoadOIDC(t *testing.T) {
	secret := strings.Repeat("s", 32)
	setenv(t, map[string]string{
		"PUBLIC_URL": "https://photos.example.org/", "SESSION_SECRET": secret,
		"OIDC_1_ISSUER": "https://accounts.google.com", "OIDC_1_CLIENT_ID": "id1", "OIDC_1_CLIENT_SECRET": "sec1", "OIDC_1_NAME": "Google",
		"OIDC_3_ISSUER": "https://auth.example.org", "OIDC_3_CLIENT_ID": "id3", "OIDC_3_CLIENT_SECRET": "sec3",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicURL != "https://photos.example.org" {
		t.Errorf("PublicURL = %q, want the trailing slash removed", cfg.PublicURL)
	}
	if len(cfg.OIDC) != 2 || cfg.OIDC[0].ID != "1" || cfg.OIDC[0].Name != "Google" || cfg.OIDC[1].ID != "3" || cfg.OIDC[1].Name != "Single sign-on" {
		t.Errorf("providers = %+v", cfg.OIDC)
	}
}

func TestLoadOIDCNeedsItsSettings(t *testing.T) {
	provider := map[string]string{"OIDC_1_ISSUER": "https://auth.example.org", "OIDC_1_CLIENT_ID": "id", "OIDC_1_CLIENT_SECRET": "sec"}
	with := func(extra map[string]string) map[string]string {
		env := map[string]string{}
		for k, v := range provider {
			env[k] = v
		}
		for k, v := range extra {
			env[k] = v
		}
		return env
	}
	secret := strings.Repeat("s", 32)
	for name, tt := range map[string]struct {
		env  map[string]string
		want string
	}{
		"no public address":            {with(map[string]string{"SESSION_SECRET": secret}), "PUBLIC_URL"},
		"a public address with a path": {with(map[string]string{"PUBLIC_URL": "https://x.org/photos", "SESSION_SECRET": secret}), "PUBLIC_URL"},
		"no scheme":                    {with(map[string]string{"PUBLIC_URL": "photos.example.org", "SESSION_SECRET": secret}), "PUBLIC_URL"},
		"a short secret":               {with(map[string]string{"PUBLIC_URL": "https://x.org", "SESSION_SECRET": "short"}), "SESSION_SECRET"},
		"no client secret":             {with(map[string]string{"PUBLIC_URL": "https://x.org", "SESSION_SECRET": secret, "OIDC_1_CLIENT_SECRET": ""}), "OIDC_1_CLIENT_SECRET"},
		"a client with no issuer":      {map[string]string{"OIDC_2_CLIENT_ID": "id", "PUBLIC_URL": "https://x.org", "SESSION_SECRET": secret}, "OIDC_2_ISSUER"},
	} {
		t.Run(name, func(t *testing.T) {
			setenv(t, tt.env)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one that names %s", err, tt.want)
			}
		})
	}

	// Without providers neither is needed.
	setenv(t, nil)
	if _, err := Load(); err != nil {
		t.Errorf("no providers: %v", err)
	}
}

func TestLoadTOTPKey(t *testing.T) {
	setenv(t, map[string]string{"TOTP_KEY": strings.Repeat("k", 32)})
	if cfg, err := Load(); err != nil || cfg.TOTPKey != strings.Repeat("k", 32) {
		t.Errorf("key = %q, error = %v", cfg.TOTPKey, err)
	}
	setenv(t, map[string]string{"TOTP_KEY": "short"})
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TOTP_KEY") {
		t.Errorf("error = %v, want one that names TOTP_KEY", err)
	}
	// Without it two-factor sign-in is off, which is not an error.
	setenv(t, nil)
	if cfg, err := Load(); err != nil || cfg.TOTPKey != "" {
		t.Errorf("no key: %q, %v", cfg.TOTPKey, err)
	}
}
