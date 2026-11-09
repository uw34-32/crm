package migrate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool returns a pool whose search_path points at a throwaway schema, so
// tests never see each other's tables or leave anything behind in crm_test.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	// Migrations change the schema, so they run as the owner, never the app role.
	url := os.Getenv("TEST_MIGRATION_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_MIGRATION_DATABASE_URL not set; skipping database-backed test")
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

// The app role reaches application tables, present and future, but never the
// runner's bookkeeping or the schema itself.
func TestEmbeddedMigrationsGrantAppRoleLeastPrivilege(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := pool.Exec(ctx, "CREATE TABLE future_table (id int)"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{"new table readable and writable", "SELECT has_table_privilege('crm_app', 'future_table', 'SELECT,INSERT,UPDATE,DELETE')", true},
		{"new table cannot be truncated", "SELECT has_table_privilege('crm_app', 'future_table', 'TRUNCATE')", false},
		{"schema usable", "SELECT has_schema_privilege('crm_app', current_schema(), 'USAGE')", true},
		{"schema closed to DDL", "SELECT has_schema_privilege('crm_app', current_schema(), 'CREATE')", false},
		{"schema_migrations private", "SELECT has_table_privilege('crm_app', 'schema_migrations', 'SELECT,INSERT,UPDATE,DELETE')", false},
		{"role owns no table here", `SELECT EXISTS (SELECT 1 FROM pg_class c
			WHERE c.relnamespace = current_schema()::regnamespace AND c.relowner = 'crm_app'::regrole)`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bool
			if err := pool.QueryRow(ctx, tt.query).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("%s = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// beforePipelines is the embedded migrations that ran before pipelines existed.
func beforePipelines(t *testing.T) fstest.MapFS {
	t.Helper()
	names, err := fs.Glob(embedded, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	subset := fstest.MapFS{}
	for _, name := range names {
		if path.Base(name) >= "009" {
			continue
		}
		content, err := fs.ReadFile(embedded, name)
		if err != nil {
			t.Fatal(err)
		}
		subset[name] = &fstest.MapFile{Data: content}
	}
	if len(subset) != 8 {
		t.Fatalf("%d migrations before pipelines, want 8", len(subset))
	}
	return subset
}

// Workspaces that exist when the migration runs get the same default pipeline a
// new workspace gets, in spite of Row-Level Security being forced on both tables.
func TestMigrationGivesExistingWorkspacesADefaultPipeline(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := apply(ctx, pool, beforePipelines(t)); err != nil {
		t.Fatalf("apply the earlier migrations: %v", err)
	}
	var live, deleted string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (name) VALUES ('Live') RETURNING id::text`).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (name, deleted_at) VALUES ('Deleted', now()) RETURNING id::text`).Scan(&deleted); err != nil {
		t.Fatal(err)
	}

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("apply: %v", err)
	}

	for name, id := range map[string]string{"live": live, "soft-deleted": deleted} {
		rows, err := pool.Query(ctx, `
			SELECT s.position, s.kind, s.pipeline_id::text
			  FROM pipelines p JOIN pipeline_stages s ON s.pipeline_id = p.id AND s.workspace_id = p.workspace_id
			 WHERE p.workspace_id = $1
			 ORDER BY s.position`, id)
		if err != nil {
			t.Fatal(err)
		}
		pipelines := map[string]bool{}
		kinds := map[string]int{}
		position := 0
		for rows.Next() {
			var at int
			var kind, pipeline string
			if err := rows.Scan(&at, &kind, &pipeline); err != nil {
				t.Fatal(err)
			}
			if at != position {
				t.Errorf("%s: position %d, want %d", name, at, position)
			}
			position++
			pipelines[pipeline] = true
			kinds[kind]++
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if len(pipelines) != 1 || kinds["open"] < 1 || kinds["won"] != 1 || kinds["lost"] != 1 {
			t.Errorf("%s: pipelines %v, stage kinds %v", name, pipelines, kinds)
		}
	}
}

// A role bound by the forced policies would see no workspaces and backfill nothing,
// reporting success. The migration has to refuse instead.
func TestMigrationRefusesARoleThatRowLevelSecurityBinds(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := apply(ctx, pool, beforePipelines(t)); err != nil {
		t.Fatalf("apply the earlier migrations: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces (name) VALUES ('Existing')`); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	const role = "crm_migration_probe"
	if _, err := pool.Exec(ctx, `DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '`+role+`') THEN CREATE ROLE `+role+` NOSUPERUSER NOBYPASSRLS; END IF;
	END $$`); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DROP ROLE IF EXISTS `+role); err != nil {
			t.Errorf("drop role: %v", err)
		}
	})

	sql, err := fs.ReadFile(embedded, "migrations/009_pipelines.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `GRANT ALL ON SCHEMA `+schema+` TO `+role+`; GRANT ALL ON ALL TABLES IN SCHEMA `+schema+` TO `+role+`; SET LOCAL ROLE `+role); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, string(sql))
	if err == nil || !strings.Contains(err.Error(), "bypass Row-Level Security") {
		t.Errorf("migration as a bound role = %v, want a refusal", err)
	}
}
