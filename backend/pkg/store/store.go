// Package store holds all database access for the application.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Ping reports whether the database is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// ErrPrivilegedRole means the connected role could bypass Row-Level Security.
var ErrPrivilegedRole = errors.New("database role is too privileged for the server")

type appRole struct {
	name        string
	super       bool
	bypassRLS   bool
	ownedTables int
}

func (r appRole) validate() error {
	if r.super || r.bypassRLS || r.ownedTables > 0 {
		return fmt.Errorf("%w: role %q has rolsuper=%t, rolbypassrls=%t, and owns %d tables; DATABASE_URL must name a role with none of these, and MIGRATION_DATABASE_URL the owner",
			ErrPrivilegedRole, r.name, r.super, r.bypassRLS, r.ownedTables)
	}
	return nil
}

// CheckAppRole refuses a role that Row-Level Security would not apply to: a
// superuser, a BYPASSRLS role, or the owner of a table. It returns
// ErrPrivilegedRole for those, or the query error if it cannot tell.
func (s *Store) CheckAppRole(ctx context.Context) error {
	var r appRole
	err := s.pool.QueryRow(ctx, `
		SELECT r.rolname, r.rolsuper, r.rolbypassrls,
		       (SELECT count(*) FROM pg_class c
		         JOIN pg_namespace n ON n.oid = c.relnamespace
		        WHERE c.relowner = r.oid
		          AND c.relkind IN ('r', 'p')
		          AND n.nspname NOT IN ('pg_catalog', 'information_schema'))
		  FROM pg_roles r
		 WHERE r.rolname = current_user`,
	).Scan(&r.name, &r.super, &r.bypassRLS, &r.ownedTables)
	if err != nil {
		return fmt.Errorf("read database role: %w", err)
	}
	return r.validate()
}
