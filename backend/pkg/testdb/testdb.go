// Package testdb gives database-backed tests the same two connections the
// server uses: the app role for queries and the migration owner for schema
// changes. It is imported only by tests.
package testdb

import (
	"context"
	"os"
	"testing"

	"crm/pkg/migrate"

	"github.com/jackc/pgx/v5/pgxpool"
)

// URLs returns the app-role and owner connection strings. It skips the test
// when neither is set and fails it when only one is, so a half-configured run
// cannot pass by silently skipping.
func URLs(t *testing.T) (app, owner string) {
	t.Helper()
	app = os.Getenv("TEST_DATABASE_URL")
	owner = os.Getenv("TEST_MIGRATION_DATABASE_URL")
	switch {
	case app == "" && owner == "":
		t.Skip("TEST_DATABASE_URL and TEST_MIGRATION_DATABASE_URL not set; skipping database-backed test")
	case app == "" || owner == "":
		t.Fatal("set both TEST_DATABASE_URL (app role) and TEST_MIGRATION_DATABASE_URL (migration owner)")
	}
	return app, owner
}

// AppPool applies migrations as the owner and returns a pool connected as the
// app role, the way the server runs.
func AppPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	appURL, ownerURL := URLs(t)
	ctx := context.Background()

	owner, err := pgxpool.New(ctx, ownerURL)
	if err != nil {
		t.Fatalf("connect as owner: %v", err)
	}
	defer owner.Close()
	if err := migrate.Apply(ctx, owner); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatalf("connect as app role: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
