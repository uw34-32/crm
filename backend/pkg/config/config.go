// Package config reads runtime settings from the environment.
package config

import (
	"errors"
	"os"
)

const defaultPort = "8085"

type Config struct {
	DatabaseURL string
	Port        string
}

var ErrMissingDatabaseURL = errors.New("DATABASE_URL is required")

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Port:        os.Getenv("PORT"),
	}
	if cfg.DatabaseURL == "" {
		return cfg, ErrMissingDatabaseURL
	}
	if cfg.Port == "" {
		cfg.Port = defaultPort
	}
	return cfg, nil
}
