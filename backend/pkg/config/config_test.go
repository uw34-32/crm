package config

import (
	"errors"
	"testing"
	"time"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); !errors.Is(err, ErrMissingDatabaseURL) {
		t.Fatalf("err = %v, want ErrMissingDatabaseURL", err)
	}
}

func TestLoadDefaultsPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example/crm")
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
	t.Setenv("DATABASE_URL", "postgres://example/crm")
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
			t.Setenv("DATABASE_URL", "postgres://example/crm")
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
