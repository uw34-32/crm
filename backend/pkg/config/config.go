// Package config reads runtime settings from the environment.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"crm/pkg/auth"
)

const (
	defaultPort                   = "8085"
	defaultMigrationTimeout       = 5 * time.Minute
	defaultAccessTokenTTL         = 15 * time.Minute
	defaultRefreshTokenTTL        = 30 * 24 * time.Hour
	defaultRefreshGrace           = 10 * time.Second
	defaultAuthRateLimitPerMinute = 30
)

type Config struct {
	// DatabaseURL is the app role the server queries as.
	DatabaseURL string
	// MigrationDatabaseURL is the role that owns the schema and runs migrations.
	MigrationDatabaseURL string
	Port                 string
	// MigrationTimeout bounds the advisory lock wait and all pending migrations.
	MigrationTimeout time.Duration

	// JWTSecret signs access tokens. It has no default.
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	// RefreshGrace is how long after rotation a spent refresh token may be
	// retried. Zero disables retries.
	RefreshGrace time.Duration
	// AuthRateLimitPerMinute is the per-client budget on auth routes.
	AuthRateLimitPerMinute int
	// CORSOrigins are the browser origins allowed to call the API. Wildcards are
	// refused.
	CORSOrigins []string
	// TrustedProxies are the networks whose forwarding headers are believed.
	TrustedProxies []netip.Prefix
}

var (
	ErrMissingDatabaseURL          = errors.New("DATABASE_URL is required")
	ErrMissingMigrationDatabaseURL = errors.New("MIGRATION_DATABASE_URL is required")
	ErrMissingJWTSecret            = errors.New("JWT_SECRET is required")
	ErrWeakJWTSecret               = fmt.Errorf("JWT_SECRET must be at least %d bytes", auth.MinSecretBytes)
)

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		MigrationDatabaseURL: os.Getenv("MIGRATION_DATABASE_URL"),
		Port:                 os.Getenv("PORT"),
	}
	if cfg.DatabaseURL == "" {
		return cfg, ErrMissingDatabaseURL
	}
	if cfg.MigrationDatabaseURL == "" {
		return cfg, ErrMissingMigrationDatabaseURL
	}
	if cfg.Port == "" {
		cfg.Port = defaultPort
	}
	var err error
	if cfg.MigrationTimeout, err = positiveDuration("MIGRATION_TIMEOUT", defaultMigrationTimeout); err != nil {
		return cfg, err
	}

	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	switch {
	case cfg.JWTSecret == "":
		return cfg, ErrMissingJWTSecret
	case len(cfg.JWTSecret) < auth.MinSecretBytes:
		return cfg, ErrWeakJWTSecret
	}
	if cfg.AccessTokenTTL, err = positiveDuration("ACCESS_TOKEN_TTL", defaultAccessTokenTTL); err != nil {
		return cfg, err
	}
	if cfg.RefreshTokenTTL, err = positiveDuration("REFRESH_TOKEN_TTL", defaultRefreshTokenTTL); err != nil {
		return cfg, err
	}
	if cfg.RefreshGrace, err = duration("REFRESH_GRACE", defaultRefreshGrace); err != nil {
		return cfg, err
	}
	if cfg.RefreshGrace < 0 {
		return cfg, errors.New("REFRESH_GRACE must not be negative")
	}
	if cfg.AuthRateLimitPerMinute, err = positiveInt("AUTH_RATE_LIMIT_PER_MINUTE", defaultAuthRateLimitPerMinute); err != nil {
		return cfg, err
	}
	if cfg.CORSOrigins, err = parseOrigins(os.Getenv("CORS_ORIGINS")); err != nil {
		return cfg, err
	}
	if cfg.TrustedProxies, err = parseProxies(os.Getenv("TRUSTED_PROXIES")); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// duration reads a duration variable, using fallback when it is unset.
func duration(name string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return d, nil
}

func positiveDuration(name string, fallback time.Duration) (time.Duration, error) {
	d, err := duration(name, fallback)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %s", name, d)
	}
	return d, nil
}

func positiveInt(name string, fallback int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %d", name, n)
	}
	return n, nil
}

// splitList splits a comma-separated value, dropping blanks.
func splitList(raw string) []string {
	var items []string
	for item := range strings.SplitSeq(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

// parseOrigins accepts only exact origins: scheme://host[:port]. Browsers
// compare origins byte for byte, so anything else could never match, and a
// wildcard would defeat the allowlist.
func parseOrigins(raw string) ([]string, error) {
	origins := splitList(raw)
	for _, origin := range origins {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil ||
			u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("CORS_ORIGINS: %q is not an exact origin such as https://app.example.com", origin)
		}
	}
	return origins, nil
}

// parseProxies accepts CIDR prefixes and single addresses.
func parseProxies(raw string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, item := range splitList(raw) {
		if prefix, err := netip.ParsePrefix(item); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(item)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES: %q is not an address or CIDR prefix", item)
		}
		addr = addr.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return prefixes, nil
}
