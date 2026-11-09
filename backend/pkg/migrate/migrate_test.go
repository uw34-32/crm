package migrate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool returns a pool whose search_path points at a throwaway schema, so
// tests never see each other's tables or leave anything behind in crm_test.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database-backed test")
	}
	ctx := context.Background()

	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "test_" + hex.EncodeToString(suffix)

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(admin.Close)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
	})

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func appliedVersions(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return versions
}

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var regclass *string
	if err := pool.QueryRow(context.Background(), "SELECT to_regclass($1)::text", name).Scan(&regclass); err != nil {
		t.Fatal(err)
	}
	return regclass != nil
}

func TestLoadSortsByVersion(t *testing.T) {
	got, err := load(fstest.MapFS{
		"migrations/002_b.sql": {Data: []byte("SELECT 2;")},
		"migrations/001_a.sql": {Data: []byte("SELECT 1;")},
		"migrations/notes.txt": {Data: []byte("ignored")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].version != "001_a" || got[1].version != "002_b" {
		t.Fatalf("got %+v, want 001_a then 002_b", got)
	}
}

func TestEmbeddedMigrationsStartWithInit(t *testing.T) {
	got, err := load(embedded)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].version != "001_init" {
		t.Fatalf("got %+v, want 001_init first", got)
	}
}

func TestApplyEmbeddedTwiceIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	first := appliedVersions(t, pool)
	if len(first) == 0 || first[0] != "001_init" {
		t.Fatalf("after first apply got %v, want 001_init recorded", first)
	}

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if second := appliedVersions(t, pool); len(second) != len(first) {
		t.Fatalf("second apply changed versions: %v -> %v", first, second)
	}
}

// The statement is deliberately not idempotent: running it twice fails, so a
// pass proves the runner skipped it the second time.
func TestApplySkipsRecordedMigrations(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	fsys := fstest.MapFS{"migrations/001_a.sql": {Data: []byte("CREATE TABLE a (id int);")}}

	for i := range 2 {
		if err := apply(ctx, pool, fsys); err != nil {
			t.Fatalf("apply %d: %v", i+1, err)
		}
	}

	fsys["migrations/002_b.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE b (id int); CREATE TABLE c (id int);")}
	if err := apply(ctx, pool, fsys); err != nil {
		t.Fatalf("apply new migration: %v", err)
	}
	for _, table := range []string{"a", "b", "c"} {
		if !tableExists(t, pool, table) {
			t.Errorf("table %s missing", table)
		}
	}
	if got := appliedVersions(t, pool); len(got) != 2 {
		t.Fatalf("got versions %v, want 2", got)
	}
}

func TestApplyFailureRollsBack(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	fsys := fstest.MapFS{
		"migrations/001_ok.sql":  {Data: []byte("CREATE TABLE ok (id int);")},
		"migrations/002_bad.sql": {Data: []byte("CREATE TABLE half (id int); SELECT not_a_column;")},
	}

	if err := apply(ctx, pool, fsys); err == nil {
		t.Fatal("expected an error from the broken migration")
	}
	if tableExists(t, pool, "ok") || tableExists(t, pool, "half") {
		t.Error("failed run left tables behind")
	}
	if tableExists(t, pool, "schema_migrations") {
		t.Errorf("failed run left versions behind: %v", appliedVersions(t, pool))
	}
}

func TestApplyConcurrentRunsApplyOnce(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	fsys := fstest.MapFS{"migrations/001_a.sql": {Data: []byte("CREATE TABLE a (id int);")}}

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Go(func() { errs[i] = apply(ctx, pool, fsys) })
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("run %d: %v", i, err)
		}
	}
	if got := appliedVersions(t, pool); len(got) != 1 {
		t.Fatalf("got versions %v, want 1", got)
	}
}
