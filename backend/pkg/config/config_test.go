package config

import (
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"crm/pkg/auth"
)

// setRequired sets every variable Load insists on.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://example/crm")
	t.Setenv("MIGRATION_DATABASE_URL", "postgres://owner@example/crm")
	t.Setenv("JWT_SECRET", strings.Repeat("s", auth.MinSecretBytes))
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	setRequired(t)
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); !errors.Is(err, ErrMissingDatabaseURL) {
		t.Fatalf("err = %v, want ErrMissingDatabaseURL", err)
	}
}

func TestLoadRequiresMigrationDatabaseURL(t *testing.T) {
	setRequired(t)
	t.Setenv("MIGRATION_DATABASE_URL", "")
	if _, err := Load(); !errors.Is(err, ErrMissingMigrationDatabaseURL) {
		t.Fatalf("err = %v, want ErrMissingMigrationDatabaseURL", err)
	}
}

func TestLoadReadsMigrationDatabaseURL(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MigrationDatabaseURL != "postgres://owner@example/crm" {
		t.Errorf("MigrationDatabaseURL = %q", cfg.MigrationDatabaseURL)
	}
}

func TestLoadDefaultsPort(t *testing.T) {
	setRequired(t)
	t.Setenv("PORT", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "8085" {
		t.Errorf("Port = %q, want 8085", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://example/crm" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
}

func TestLoadReadsPort(t *testing.T) {
	setRequired(t)
	t.Setenv("PORT", "9000")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "9000" {
		t.Errorf("Port = %q, want 9000", cfg.Port)
	}
}

func TestLoadMigrationTimeout(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Duration
		wantErr bool
	}{
		{name: "defaults when unset", value: "", want: 5 * time.Minute},
		{name: "reads a duration", value: "90s", want: 90 * time.Second},
		{name: "rejects garbage", value: "soon", wantErr: true},
		{name: "rejects zero", value: "0s", wantErr: true},
		{name: "rejects negative", value: "-1m", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequired(t)
			t.Setenv("MIGRATION_TIMEOUT", tt.value)
			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.MigrationTimeout != tt.want {
				t.Errorf("MigrationTimeout = %v, want %v", cfg.MigrationTimeout, tt.want)
			}
		})
	}
}

func TestLoadJWTSecret(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr error
	}{
		{"missing", "", ErrMissingJWTSecret},
		{"too short", strings.Repeat("x", auth.MinSecretBytes-1), ErrWeakJWTSecret},
		{"long enough", strings.Repeat("x", auth.MinSecretBytes), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequired(t)
			t.Setenv("JWT_SECRET", tt.secret)
			cfg, err := Load()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && cfg.JWTSecret != tt.secret {
				t.Errorf("JWTSecret not read")
			}
			if tt.wantErr != nil && strings.Contains(err.Error(), "xxxx") {
				t.Errorf("error reveals the secret: %v", err)
			}
		})
	}
}

func TestLoadAuthDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessTokenTTL != 15*time.Minute {
		t.Errorf("AccessTokenTTL = %v", cfg.AccessTokenTTL)
	}
	if cfg.RefreshTokenTTL != 30*24*time.Hour {
		t.Errorf("RefreshTokenTTL = %v", cfg.RefreshTokenTTL)
	}
	if cfg.RefreshGrace != 10*time.Second {
		t.Errorf("RefreshGrace = %v", cfg.RefreshGrace)
	}
	if cfg.AuthRateLimitPerMinute != 30 {
		t.Errorf("AuthRateLimitPerMinute = %d", cfg.AuthRateLimitPerMinute)
	}
	if len(cfg.CORSOrigins) != 0 {
		t.Errorf("CORSOrigins = %v, want none by default", cfg.CORSOrigins)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Errorf("TrustedProxies = %v, want none by default", cfg.TrustedProxies)
	}
}

func TestLoadRefusesBadAuthSettings(t *testing.T) {
	tests := []struct{ name, variable, value string }{
		{"negative access TTL", "ACCESS_TOKEN_TTL", "-1m"},
		{"zero refresh TTL", "REFRESH_TOKEN_TTL", "0s"},
		{"negative grace", "REFRESH_GRACE", "-1s"},
		{"garbage grace", "REFRESH_GRACE", "soon"},
		{"zero rate limit", "AUTH_RATE_LIMIT_PER_MINUTE", "0"},
		{"negative rate limit", "AUTH_RATE_LIMIT_PER_MINUTE", "-5"},
		{"garbage rate limit", "AUTH_RATE_LIMIT_PER_MINUTE", "lots"},
		{"wildcard origin", "CORS_ORIGINS", "*"},
		{"wildcard among origins", "CORS_ORIGINS", "http://localhost:5173,*"},
		{"origin with a path", "CORS_ORIGINS", "http://localhost:5173/app"},
		{"origin with a trailing slash", "CORS_ORIGINS", "http://localhost:5173/"},
		{"origin without a scheme", "CORS_ORIGINS", "localhost:5173"},
		{"null origin", "CORS_ORIGINS", "null"},
		{"garbage proxy", "TRUSTED_PROXIES", "not-an-ip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(tt.variable, tt.value)
			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted %s=%q", tt.variable, tt.value)
			}
		})
	}
}

func TestLoadAllowsZeroGrace(t *testing.T) {
	setRequired(t)
	t.Setenv("REFRESH_GRACE", "0s")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RefreshGrace != 0 {
		t.Errorf("RefreshGrace = %v, want 0", cfg.RefreshGrace)
	}
}

func TestLoadParsesLists(t *testing.T) {
	setRequired(t)
	t.Setenv("CORS_ORIGINS", "http://localhost:5173, tauri://localhost ,http://tauri.localhost")
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8, 192.0.2.1,2001:db8::/32")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	wantOrigins := []string{"http://localhost:5173", "tauri://localhost", "http://tauri.localhost"}
	if !slices.Equal(cfg.CORSOrigins, wantOrigins) {
		t.Errorf("CORSOrigins = %v, want %v", cfg.CORSOrigins, wantOrigins)
	}
	wantProxies := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.0.2.1/32"),
		netip.MustParsePrefix("2001:db8::/32"),
	}
	if !slices.Equal(cfg.TrustedProxies, wantProxies) {
		t.Errorf("TrustedProxies = %v, want %v", cfg.TrustedProxies, wantProxies)
	}
}
