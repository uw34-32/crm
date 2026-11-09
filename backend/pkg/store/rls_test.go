package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"crm/pkg/model"
	"crm/pkg/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pgInsufficientPrivilege = "42501"

func ownerPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	_, ownerURL := testdb.URLs(t)
	pool, err := pgxpool.New(context.Background(), ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// tenants is two workspaces with a shared outsider, for raw SQL against the
// policies. Nothing here goes through a store filter.
type tenants struct {
	pool      *pgxpool.Pool
	a, b      model.Workspace
	ownerA    model.User
	ownerB    model.User
	spare     model.User
	memberOfA model.User
}

func newTenants(t *testing.T) tenants {
	t.Helper()
	pool := testdb.AppPool(t)
	s := New(pool)
	ownerA, ownerB, spare := newUser(t, s), newUser(t, s), newUser(t, s)
	a, b := newWorkspace(t, s, ownerA), newWorkspace(t, s, ownerB)
	return tenants{pool: pool, a: a, b: b, ownerA: ownerA, ownerB: ownerB, spare: spare,
		memberOfA: addMember(t, s, a, ownerA, model.RoleMember)}
}

// rawTx runs fn as crm_app in a transaction that is always rolled back, with
// setting applied first unless it is empty.
func rawTx(t *testing.T, pool *pgxpool.Pool, setting, value string, fn func(ctx context.Context, tx pgx.Tx)) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if setting != "" {
		if _, err := tx.Exec(ctx, `SELECT set_config($1, $2, true)`, setting, value); err != nil {
			t.Fatal(err)
		}
	}
	fn(ctx, tx)
}

func strs(t *testing.T, ctx context.Context, q Querier, sql string, args ...any) []string {
	t.Helper()
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return got
}

func wantDenied(t *testing.T, name string, err error) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgInsufficientPrivilege {
		t.Errorf("%s = %v, want a policy violation (42501)", name, err)
	}
}

func TestRLSReadsAreScopedToTheSetting(t *testing.T) {
	tn := newTenants(t)
	rawTx(t, tn.pool, "app.workspace_id", tn.a.ID, func(ctx context.Context, tx pgx.Tx) {
		// Deliberately no WHERE on the tenant column.
		members := strs(t, ctx, tx, `SELECT DISTINCT workspace_id::text FROM workspace_members`)
		if !slices.Equal(members, []string{tn.a.ID}) {
			t.Errorf("members visible for workspaces %v, want only %s", members, tn.a.ID)
		}
		spaces := strs(t, ctx, tx, `SELECT id::text FROM workspaces`)
		if !slices.Equal(spaces, []string{tn.a.ID}) {
			t.Errorf("workspaces visible %v, want only %s", spaces, tn.a.ID)
		}
		// Asking for B by name finds nothing either.
		if got := strs(t, ctx, tx, `SELECT user_id::text FROM workspace_members WHERE workspace_id = $1`, tn.b.ID); len(got) != 0 {
			t.Errorf("rows of B visible by explicit filter: %v", got)
		}
	})
}

func TestRLSNoSettingReturnsNothing(t *testing.T) {
	tn := newTenants(t)
	for _, tt := range []struct{ name, setting, value string }{
		{"unset", "", ""},
		{"empty workspace", "app.workspace_id", ""},
		{"empty user", "app.user_id", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rawTx(t, tn.pool, tt.setting, tt.value, func(ctx context.Context, tx pgx.Tx) {
				for _, table := range []string{"workspaces", "workspace_members"} {
					var count int
					if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
						t.Fatal(err)
					}
					if count != 0 {
						t.Errorf("%s: %d rows visible", table, count)
					}
				}
			})
		})
	}
	// Outside any transaction too.
	var count int
	if err := tn.pool.QueryRow(context.Background(), `SELECT count(*) FROM workspace_members`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("%d member rows visible without a transaction", count)
	}
}

func TestRLSRefusesWritesIntoAnotherWorkspace(t *testing.T) {
	tn := newTenants(t)
	// Each attempt gets its own transaction, because a denial aborts it.
	inA := func(name string, fn func(ctx context.Context, tx pgx.Tx)) {
		t.Run(name, func(t *testing.T) { rawTx(t, tn.pool, "app.workspace_id", tn.a.ID, fn) })
	}
	inA("insert member into B", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, tn.b.ID, tn.spare.ID)
		wantDenied(t, "insert", err)
	})
	inA("insert workspace with another id", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `INSERT INTO workspaces (name) VALUES ('rogue')`)
		wantDenied(t, "insert", err)
	})
	inA("move member from A to B", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `UPDATE workspace_members SET workspace_id = $1 WHERE user_id = $2`, tn.b.ID, tn.memberOfA.ID)
		wantDenied(t, "move", err)
	})
	inA("re-key workspace A as B", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `UPDATE workspaces SET id = gen_random_uuid() WHERE id = $1`, tn.a.ID)
		wantDenied(t, "re-key", err)
	})
	inA("update members of B", func(ctx context.Context, tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `UPDATE workspace_members SET role = 'member' WHERE workspace_id = $1`, tn.b.ID)
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("update = %d rows, %v; want 0 rows", tag.RowsAffected(), err)
		}
	})
	inA("update workspace B", func(ctx context.Context, tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `UPDATE workspaces SET name = 'pwned' WHERE id = $1`, tn.b.ID)
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("update = %d rows, %v; want 0 rows", tag.RowsAffected(), err)
		}
	})
	inA("delete members of B", func(ctx context.Context, tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `DELETE FROM workspace_members WHERE workspace_id = $1`, tn.b.ID)
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("delete = %d rows, %v; want 0 rows", tag.RowsAffected(), err)
		}
	})
	inA("lock workspace B", func(ctx context.Context, tx pgx.Tx) {
		if got := strs(t, ctx, tx, `SELECT id::text FROM workspaces WHERE id = $1 FOR UPDATE`, tn.b.ID); len(got) != 0 {
			t.Errorf("locked %v", got)
		}
	})

	// With no setting at all, writes are refused as well.
	t.Run("insert with no setting", func(t *testing.T) {
		rawTx(t, tn.pool, "", "", func(ctx context.Context, tx pgx.Tx) {
			_, err := tx.Exec(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, tn.a.ID, tn.spare.ID)
			wantDenied(t, "insert", err)
		})
	})

	// B is untouched, seen by the owner, which bypasses the policies.
	var name string
	var owners int
	owner := ownerPool(t)
	ctx := context.Background()
	if err := owner.QueryRow(ctx, `SELECT name FROM workspaces WHERE id = $1`, tn.b.ID).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if err := owner.QueryRow(ctx, `SELECT count(*) FROM workspace_members WHERE workspace_id = $1 AND role = 'owner'`, tn.b.ID).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if name != tn.b.Name || owners != 1 {
		t.Errorf("B changed: name %q, %d owners", name, owners)
	}
}

// app.user_id opens only a read of the caller's own rows. It must not reveal
// fellow members, and must not allow writes.
func TestRLSUserSettingIsReadOnlyAndSelfOnly(t *testing.T) {
	tn := newTenants(t)
	asMember := func(name string, fn func(ctx context.Context, tx pgx.Tx)) {
		t.Run(name, func(t *testing.T) { rawTx(t, tn.pool, "app.user_id", tn.memberOfA.ID, fn) })
	}
	asMember("sees only own membership", func(ctx context.Context, tx pgx.Tx) {
		got := strs(t, ctx, tx, `SELECT user_id::text FROM workspace_members`)
		if !slices.Equal(got, []string{tn.memberOfA.ID}) {
			t.Errorf("visible members %v, want only self", got)
		}
	})
	asMember("sees only own workspaces", func(ctx context.Context, tx pgx.Tx) {
		got := strs(t, ctx, tx, `SELECT id::text FROM workspaces`)
		if !slices.Equal(got, []string{tn.a.ID}) {
			t.Errorf("visible workspaces %v, want only A", got)
		}
	})
	asMember("cannot insert a membership", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, tn.b.ID, tn.memberOfA.ID)
		wantDenied(t, "insert", err)
	})
	asMember("cannot raise own role", func(ctx context.Context, tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `UPDATE workspace_members SET role = 'owner' WHERE user_id = $1`, tn.memberOfA.ID)
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("update = %d rows, %v; want 0 rows", tag.RowsAffected(), err)
		}
	})
	asMember("cannot rename own workspace", func(ctx context.Context, tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `UPDATE workspaces SET name = 'x'`)
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("update = %d rows, %v; want 0 rows", tag.RowsAffected(), err)
		}
	})
	asMember("cannot delete own membership", func(ctx context.Context, tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `DELETE FROM workspace_members WHERE user_id = $1`, tn.memberOfA.ID)
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("delete = %d rows, %v; want 0 rows", tag.RowsAffected(), err)
		}
	})
}

// The store helper plus a query with no tenant filter must still show one
// tenant: this is what protects T4 onward from a forgotten WHERE.
func TestInWorkspaceScopesAQueryWithoutAFilter(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	ctx := context.Background()
	err := s.InWorkspace(ctx, tn.a.ID, func(q Querier) error {
		got := strs(t, ctx, q, `SELECT DISTINCT workspace_id::text FROM workspace_members`)
		if !slices.Equal(got, []string{tn.a.ID}) {
			t.Errorf("visible %v, want only %s", got, tn.a.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInWorkspaceFailsClosed(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	ctx := context.Background()
	mustNotRun := func(Querier) error {
		t.Error("the function ran without a workspace set")
		return nil
	}

	for _, id := range []string{"", "not-a-uuid", "a\x00b"} {
		if err := s.InWorkspace(ctx, id, mustNotRun); !errors.Is(err, ErrWorkspaceNotFound) {
			t.Errorf("InWorkspace(%q) = %v, want ErrWorkspaceNotFound", id, err)
		}
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.InWorkspace(cancelled, tn.a.ID, mustNotRun); err == nil {
		t.Error("InWorkspace with a cancelled context returned nil")
	}

	// A set_config failure is returned, and the function does not run.
	if err := s.inTenantTx(ctx, "not a valid name", tn.a.ID, mustNotRun); err == nil {
		t.Error("inTenantTx with a failing set_config returned nil")
	}

	// The function's error is returned and rolls the transaction back.
	boom := errors.New("boom")
	err := s.InWorkspace(ctx, tn.a.ID, func(q Querier) error {
		if _, err := q.Exec(ctx, `UPDATE workspaces SET name = 'rolled back' WHERE id = $1`, tn.a.ID); err != nil {
			t.Fatal(err)
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the function's error", err)
	}
	var name string
	if err := ownerPool(t).QueryRow(ctx, `SELECT name FROM workspaces WHERE id = $1`, tn.a.ID).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != tn.a.Name {
		t.Errorf("name = %q, want the update rolled back", name)
	}
}

// One connection, so a later query is certain to reuse the connection the
// setting was made on.
func TestSettingsDoNotLeakAcrossThePool(t *testing.T) {
	tn := newTenants(t)
	appURL, _ := testdb.URLs(t)
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(appURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	single, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer single.Close()
	s := New(single)

	assertClean := func(t *testing.T, pid uint32) {
		t.Helper()
		var after uint32
		var workspace, user *string
		var visible int
		err := single.QueryRow(ctx, `SELECT pg_backend_pid(), current_setting('app.workspace_id', true), current_setting('app.user_id', true),
			(SELECT count(*) FROM workspace_members)`).Scan(&after, &workspace, &user, &visible)
		if err != nil {
			t.Fatal(err)
		}
		if after != pid {
			t.Fatalf("connection changed from %d to %d, so this proves nothing", pid, after)
		}
		if workspace != nil && *workspace != "" {
			t.Errorf("app.workspace_id leaked: %q", *workspace)
		}
		if user != nil && *user != "" {
			t.Errorf("app.user_id leaked: %q", *user)
		}
		if visible != 0 {
			t.Errorf("%d member rows visible after the transaction", visible)
		}
	}

	for _, tt := range []struct {
		name string
		err  error
	}{{"after commit", nil}, {"after rollback", errors.New("boom")}} {
		t.Run("workspace "+tt.name, func(t *testing.T) {
			var pid uint32
			err := s.InWorkspace(ctx, tn.a.ID, func(q Querier) error {
				if err := q.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
					t.Fatal(err)
				}
				return tt.err
			})
			if !errors.Is(err, tt.err) {
				t.Fatal(err)
			}
			assertClean(t, pid)
		})
		t.Run("user "+tt.name, func(t *testing.T) {
			var pid uint32
			err := s.inTenantTx(ctx, settingUser, tn.ownerA.ID, func(q Querier) error {
				if err := q.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
					t.Fatal(err)
				}
				return tt.err
			})
			if !errors.Is(err, tt.err) {
				t.Fatal(err)
			}
			assertClean(t, pid)
		})
	}
}

func TestTenantTablesAreProtected(t *testing.T) {
	s := New(testdb.AppPool(t))
	tables, err := s.CheckTenantTables(context.Background())
	if err != nil {
		t.Fatalf("CheckTenantTables: %v", err)
	}
	// The check must not pass because it looked at nothing.
	for _, want := range []string{"workspaces", "workspace_members", "activities"} {
		if !slices.Contains(tables, want) {
			t.Errorf("checked %v, which lacks %s", tables, want)
		}
	}
}

// A future table with a workspace_id must fail until it is protected properly.
func TestTenantTableCheckCatchesBadTables(t *testing.T) {
	const scope = `NULLIF(current_setting('app.workspace_id', true), '')::uuid`
	const cols = `(workspace_id UUID NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT now())`
	const rls = `ALTER TABLE rls_probe ENABLE ROW LEVEL SECURITY; ALTER TABLE rls_probe FORCE ROW LEVEL SECURITY;`
	good := `CREATE POLICY p ON rls_probe FOR ALL USING (workspace_id = ` + scope + `) WITH CHECK (workspace_id = ` + scope + `);`

	tests := []struct {
		name string
		ddl  string
		want string
	}{
		{"properly protected", `CREATE TABLE rls_probe ` + cols + `;` + rls + good, ""},
		{"no RLS", `CREATE TABLE rls_probe ` + cols + `;` + good, "not enabled"},
		{"enabled but not forced", `CREATE TABLE rls_probe ` + cols + `; ALTER TABLE rls_probe ENABLE ROW LEVEL SECURITY;` + good, "not forced"},
		{"no policy", `CREATE TABLE rls_probe ` + cols + `;` + rls, "no FOR ALL policy"},
		{"USING only", `CREATE TABLE rls_probe ` + cols + `;` + rls + `CREATE POLICY p ON rls_probe FOR ALL USING (workspace_id = ` + scope + `);`, "no FOR ALL policy"},
		{"USING only beside a good policy", `CREATE TABLE rls_probe ` + cols + `;` + rls + good + `CREATE POLICY q ON rls_probe FOR UPDATE USING (workspace_id = ` + scope + `);`, "lacks WITH CHECK"},
		{"USING (true)", `CREATE TABLE rls_probe ` + cols + `;` + rls + good + `CREATE POLICY q ON rls_probe FOR SELECT USING (true);`, "USING (true)"},
		{"WITH CHECK (true)", `CREATE TABLE rls_probe ` + cols + `;` + rls + good + `CREATE POLICY q ON rls_probe FOR INSERT WITH CHECK (true);`, "USING (true)"},
		{"nullable tenant column", `CREATE TABLE rls_probe (workspace_id UUID, updated_at TIMESTAMPTZ NOT NULL);` + rls + good, "nullable"},
		{"no updated_at", `CREATE TABLE rls_probe (workspace_id UUID NOT NULL);` + rls + good, "updated_at"},
	}
	testdb.AppPool(t) // Ensures the schema is migrated.
	owner := ownerPool(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tx, err := owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := tx.Exec(ctx, tt.ddl); err != nil {
				t.Fatal(err)
			}
			tables, violations, err := tenantTableViolations(ctx, tx)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(tables, "rls_probe") {
				t.Fatalf("the check did not see the table: %v", tables)
			}
			joined := strings.Join(violations, "; ")
			if tt.want == "" && joined != "" {
				t.Errorf("violations = %q, want none", joined)
			}
			if !strings.Contains(joined, tt.want) {
				t.Errorf("violations = %q, want %q", joined, tt.want)
			}
		})
	}
}

// FK checks bypass RLS, so a plain foreign key would accept a reference into
// another tenant. The composite pattern documented in the README refuses it.
func TestCompositeForeignKeyRefusesCrossTenantReference(t *testing.T) {
	tn := newTenants(t)
	ctx := context.Background()
	tx, err := ownerPool(t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `CREATE TABLE fk_probe (
		workspace_id UUID NOT NULL,
		assigned_to  UUID NOT NULL,
		FOREIGN KEY (workspace_id, assigned_to) REFERENCES workspace_members (workspace_id, user_id)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO fk_probe VALUES ($1, $2)`, tn.a.ID, tn.ownerA.ID); err != nil {
		t.Errorf("reference inside the tenant: %v", err)
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sp.Exec(ctx, `INSERT INTO fk_probe VALUES ($1, $2)`, tn.a.ID, tn.ownerB.ID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Errorf("reference into another tenant = %v, want a foreign key violation", err)
	}
	sp.Rollback(ctx)
}

// Migrations rely on this: FORCE binds the owner unless it is a superuser.
func TestMigrationOwnerBypassesRLS(t *testing.T) {
	tn := newTenants(t)
	ctx := context.Background()
	owner := ownerPool(t)
	var super bool
	if err := owner.QueryRow(ctx, `SELECT rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&super); err != nil {
		t.Fatal(err)
	}
	if !super {
		t.Fatal("the migration owner is not a superuser, so FORCE would bind it; give it BYPASSRLS or review data migrations")
	}
	var visible int
	err := owner.QueryRow(ctx, `SELECT count(*) FROM workspaces WHERE id = ANY($1::uuid[])`, []string{tn.a.ID, tn.b.ID}).Scan(&visible)
	if err != nil {
		t.Fatal(err)
	}
	if visible != 2 {
		t.Errorf("owner sees %d of 2 workspaces with no setting", visible)
	}
}

func TestAppRoleCannotWeakenRLS(t *testing.T) {
	tn := newTenants(t)
	for _, tt := range []struct{ name, sql string }{
		{"disable", `ALTER TABLE workspace_members DISABLE ROW LEVEL SECURITY`},
		{"no force", `ALTER TABLE workspace_members NO FORCE ROW LEVEL SECURITY`},
		{"drop policy", `DROP POLICY workspace_members_tenant ON workspace_members`},
		{"add policy", `CREATE POLICY open ON workspace_members FOR ALL USING (true) WITH CHECK (true)`},
		{"hard delete", `DELETE FROM workspaces`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rawTx(t, tn.pool, "app.workspace_id", tn.a.ID, func(ctx context.Context, tx pgx.Tx) {
				_, err := tx.Exec(ctx, tt.sql)
				wantDenied(t, tt.name, err)
			})
		})
	}
}
