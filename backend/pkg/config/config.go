// Package config reads runtime settings from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	defaultPort             = "8085"
	defaultMigrationTimeout = 5 * time.Minute
)

type Config struct {
	// DatabaseURL is the app role the server queries as.
	DatabaseURL string
	// MigrationDatabaseURL is the role that owns the schema and runs migrations.
	MigrationDatabaseURL string
	Port                 string
	// MigrationTimeout bounds the advisory lock wait and all pending migrations.
	MigrationTimeout time.Duration
}

var (
	ErrMissingDatabaseURL          = errors.New("DATABASE_URL is required")
	ErrMissingMigrationDatabaseURL = errors.New("MIGRATION_DATABASE_URL is required")
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
	return cfg, nil
}

// positiveDuration reads a duration variable, using fallback when it is unset.
func positiveDuration(name string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %s", name, raw)
	}
	return d, nil
}
