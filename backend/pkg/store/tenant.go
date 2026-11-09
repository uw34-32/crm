package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// The settings the Row-Level Security policies read. See 006_workspaces.sql.
const (
	settingWorkspace = "app.workspace_id"
	settingUser      = "app.user_id"
)

// Querier is what a method on workspace-scoped data runs its SQL on. A pgx.Tx
// from InWorkspace satisfies it, and it offers no way to end that transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// InWorkspace runs fn in a transaction scoped to one workspace, which commits if
// fn returns nil and rolls back otherwise. Every query on workspace-scoped data
// goes through here, and still filters on workspace_id itself: the policies are
// the second layer, not a replacement.
//
// workspaceID must come from a checked membership, never from a token claim or a
// request field. The setting is transaction-local, so it cannot outlive the
// transaction on a pooled connection. If it cannot be set, fn does not run.
func (s *Store) InWorkspace(ctx context.Context, workspaceID string, fn func(Querier) error) error {
	if !validUUID(workspaceID) {
		return ErrWorkspaceNotFound
	}
	return s.inTenantTx(ctx, settingWorkspace, workspaceID, fn)
}

func (s *Store) inTenantTx(ctx context.Context, setting, value string, fn func(Querier) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// The final true makes the setting local to this transaction.
		if _, err := tx.Exec(ctx, `SELECT set_config($1, $2, true)`, setting, value); err != nil {
			return fmt.Errorf("set %s: %w", setting, err)
		}
		return fn(tx)
	})
}

// validUUID reports whether s has the canonical UUID shape. Checking first means
// a malformed ID from a URL never reaches SQL, where it would fail as an error
// rather than match nothing.
func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := range len(s) {
		c := s[i]
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// ValidID reports whether s has the shape of an ID, for a caller that must refuse a
// malformed one rather than treat it as not found.
func ValidID(s string) bool {
	return validUUID(s)
}

// ErrUnprotectedTenantTable means a workspace-scoped table could be read or
// written across tenants at the database layer.
var ErrUnprotectedTenantTable = errors.New("workspace-scoped table is not protected by Row-Level Security")

// CheckTenantTables verifies every workspace-scoped table in the current schema
// and returns their names, or ErrUnprotectedTenantTable naming what is wrong. A
// table is workspace-scoped if it has a workspace_id column, so a new one is
// checked without anyone registering it.
func (s *Store) CheckTenantTables(ctx context.Context) ([]string, error) {
	tables, violations, err := tenantTableViolations(ctx, s.pool)
	if err != nil {
		return nil, fmt.Errorf("read tenant tables: %w", err)
	}
	if len(violations) > 0 {
		return tables, fmt.Errorf("%w: %s", ErrUnprotectedTenantTable, strings.Join(violations, "; "))
	}
	return tables, nil
}

func tenantTableViolations(ctx context.Context, q Querier) (tables, violations []string, err error) {
	rows, err := q.Query(ctx, `
		SELECT c.relname::text, c.relrowsecurity, c.relforcerowsecurity,
		       COALESCE((SELECT a.attnotnull FROM pg_attribute a
		                  WHERE a.attrelid = c.oid AND NOT a.attisdropped
		                    AND a.attname = CASE WHEN c.relname = 'workspaces' THEN 'id' ELSE 'workspace_id' END), false),
		       COALESCE((SELECT a.attnotnull FROM pg_attribute a
		                  WHERE a.attrelid = c.oid AND NOT a.attisdropped AND a.attname = 'updated_at'), false),
		       EXISTS (SELECT 1 FROM pg_policy p
		                WHERE p.polrelid = c.oid AND p.polcmd = '*' AND p.polpermissive
		                  AND p.polqual IS NOT NULL AND p.polwithcheck IS NOT NULL),
		       EXISTS (SELECT 1 FROM pg_policy p
		                WHERE p.polrelid = c.oid AND p.polcmd = 'w' AND p.polwithcheck IS NULL),
		       EXISTS (SELECT 1 FROM pg_policy p
		                WHERE p.polrelid = c.oid
		                  AND (pg_get_expr(p.polqual, p.polrelid) = 'true'
		                       OR pg_get_expr(p.polwithcheck, p.polrelid) = 'true'))
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = current_schema()
		   AND c.relkind IN ('r', 'p')
		   AND (c.relname = 'workspaces'
		        OR EXISTS (SELECT 1 FROM pg_attribute a
		                    WHERE a.attrelid = c.oid AND a.attname = 'workspace_id' AND NOT a.attisdropped))
		 ORDER BY c.relname`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var table string
		var enabled, forced, tenantNotNull, updatedAtNotNull, hasAllPolicy, updateWithoutCheck, alwaysTrue bool
		if err := rows.Scan(&table, &enabled, &forced, &tenantNotNull, &updatedAtNotNull, &hasAllPolicy, &updateWithoutCheck, &alwaysTrue); err != nil {
			return nil, nil, err
		}
		tables = append(tables, table)
		for _, problem := range []struct {
			found bool
			what  string
		}{
			{!enabled, "Row-Level Security is not enabled"},
			{!forced, "Row-Level Security is not forced"},
			{!tenantNotNull, "the tenant column is nullable"},
			{!updatedAtNotNull, "updated_at is missing or nullable"},
			{!hasAllPolicy, "no FOR ALL policy has both USING and WITH CHECK"},
			{updateWithoutCheck, "an UPDATE policy lacks WITH CHECK"},
			{alwaysTrue, "a policy uses USING (true) or WITH CHECK (true)"},
		} {
			if problem.found {
				violations = append(violations, table+": "+problem.what)
			}
		}
	}
	return tables, violations, rows.Err()
}
