package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"testing"

	"crm/pkg/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAppRoleValidate(t *testing.T) {
	tests := []struct {
		name    string
		role    appRole
		wantErr bool
	}{
		{name: "unprivileged", role: appRole{name: "crm_app"}},
		{name: "superuser", role: appRole{name: "x", super: true}, wantErr: true},
		{name: "bypasses RLS", role: appRole{name: "x", bypassRLS: true}, wantErr: true},
		{name: "owns tables", role: appRole{name: "x", ownedTables: 1}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.role.validate()
			if tt.wantErr && !errors.Is(err, ErrPrivilegedRole) {
				t.Fatalf("validate = %v, want ErrPrivilegedRole", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validate = %v, want nil", err)
			}
		})
	}
}

func TestCheckAppRoleAcceptsAppRole(t *testing.T) {
	pool := testdb.AppPool(t)
	if err := New(pool).CheckAppRole(context.Background()); err != nil {
		t.Fatalf("CheckAppRole = %v, want the test app role accepted", err)
	}
}

func TestCheckAppRoleRefusesSuperuser(t *testing.T) {
	_, ownerURL := testdb.URLs(t)
	pool, err := pgxpool.New(context.Background(), ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := New(pool).CheckAppRole(context.Background()); !errors.Is(err, ErrPrivilegedRole) {
		t.Fatalf("CheckAppRole = %v, want ErrPrivilegedRole", err)
	}
}

// tempRolePool creates a login role with the given attributes and connects as
// it. Creating BYPASSRLS roles needs a superuser owner, as Compose provides.
func tempRolePool(t *testing.T, attributes string) (*pgxpool.Pool, *pgxpool.Pool, string) {
	t.Helper()
	_, ownerURL := testdb.URLs(t)
	ctx := context.Background()
	owner, err := pgxpool.New(ctx, ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Close)

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	role := "crm_test_" + hex.EncodeToString(suffix)
	if _, err := owner.Exec(ctx, "CREATE ROLE "+role+" LOGIN PASSWORD 'test-only' "+attributes); err != nil {
		t.Fatalf("create role: %v", err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(ctx, "DROP OWNED BY "+role+" CASCADE"); err != nil {
			t.Errorf("drop owned: %v", err)
		}
		if _, err := owner.Exec(ctx, "DROP ROLE "+role); err != nil {
			t.Errorf("drop role: %v", err)
		}
	})

	u, err := url.Parse(ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(role, "test-only")
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, owner, role
}

func TestCheckAppRoleRefusesBypassRLS(t *testing.T) {
	pool, _, _ := tempRolePool(t, "NOSUPERUSER BYPASSRLS")
	if err := New(pool).CheckAppRole(context.Background()); !errors.Is(err, ErrPrivilegedRole) {
		t.Fatalf("CheckAppRole = %v, want ErrPrivilegedRole", err)
	}
}

func TestCheckAppRoleRefusesTableOwner(t *testing.T) {
	pool, owner, role := tempRolePool(t, "NOSUPERUSER NOBYPASSRLS")
	ctx := context.Background()
	if _, err := owner.Exec(ctx, "CREATE SCHEMA "+role+" AUTHORIZATION "+role); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, "CREATE TABLE "+role+".owned (id int)"); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, "ALTER TABLE "+role+".owned OWNER TO "+role); err != nil {
		t.Fatal(err)
	}
	if err := New(pool).CheckAppRole(ctx); !errors.Is(err, ErrPrivilegedRole) {
		t.Fatalf("CheckAppRole = %v, want ErrPrivilegedRole", err)
	}
}
