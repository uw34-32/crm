// Package migrate applies the embedded SQL migrations and records which
// versions have run, so applying them again is a no-op.
package migrate

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var embedded embed.FS

// lockKey serializes concurrent runners. Its value is arbitrary but must stay
// stable across releases.
const lockKey int64 = 7_260_001

type migration struct {
	version string
	sql     string
}

// Apply runs every embedded migration that is not yet recorded in
// schema_migrations.
func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	return apply(ctx, pool, embedded)
}

// apply runs all pending migrations in one transaction: a failure rolls the
// whole run back, and the transaction-scoped lock releases itself on every
// path. Migrations therefore cannot use statements that refuse to run inside a
// transaction, such as CREATE INDEX CONCURRENTLY.
func apply(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	migrations, err := load(fsys)
	if err != nil {
		return err
	}

	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
			return fmt.Errorf("acquire migration lock: %w", err)
		}
		if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
			return fmt.Errorf("create schema_migrations: %w", err)
		}

		rows, err := tx.Query(ctx, "SELECT version FROM schema_migrations")
		if err != nil {
			return fmt.Errorf("read schema_migrations: %w", err)
		}
		applied, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("read schema_migrations: %w", err)
		}

		for _, m := range migrations {
			if slices.Contains(applied, m.version) {
				continue
			}
			if _, err := tx.Exec(ctx, m.sql); err != nil {
				return fmt.Errorf("apply migration %s: %w", m.version, err)
			}
			if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", m.version); err != nil {
				return fmt.Errorf("record migration %s: %w", m.version, err)
			}
		}
		return nil
	})
}

// load reads migrations/*.sql from fsys, ordered by version. The version is
// the file name without its extension.
func load(fsys fs.FS) ([]migration, error) {
	names, err := fs.Glob(fsys, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	slices.Sort(names)

	migrations := make([]migration, 0, len(names))
	for _, name := range names {
		sql, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		migrations = append(migrations, migration{
			version: strings.TrimSuffix(path.Base(name), ".sql"),
			sql:     string(sql),
		})
	}
	return migrations, nil
}
