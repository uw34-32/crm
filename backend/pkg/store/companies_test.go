package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"crm/pkg/model"
	"crm/pkg/testdb"

	"github.com/jackc/pgx/v5"
)

const unknownID = "00000000-0000-4000-8000-000000000000"

func strPtr(s string) *string { return &s }

func newCompany(t *testing.T, s *Store, workspaceID, name string) model.Company {
	t.Helper()
	c, err := s.CreateCompany(context.Background(), workspaceID, model.CompanyInput{Name: name})
	if err != nil {
		t.Fatalf("CreateCompany(%q): %v", name, err)
	}
	return c
}

func companyNames(page model.Page[model.Company]) []string {
	names := make([]string, len(page.Items))
	for i, c := range page.Items {
		names[i] = c.Name
	}
	return names
}

func TestCompanyLifecycle(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)

	created, err := s.CreateCompany(ctx, w.ID, model.CompanyInput{
		Name: "Acme", Domain: "acme.example", Phone: "+1 555 0100", Industry: "Rockets", Address: "1 Desert Road",
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	if created.ID == "" || created.WorkspaceID != w.ID || created.Domain != "acme.example" ||
		created.Phone != "+1 555 0100" || created.Industry != "Rockets" || created.Address != "1 Desert Road" {
		t.Errorf("created = %+v", created)
	}

	got, err := s.GetCompany(ctx, w.ID, created.ID)
	if err != nil || got != created {
		t.Errorf("GetCompany = %+v, %v; want %+v", got, err, created)
	}

	// A patch changes only the fields it names.
	updated, err := s.UpdateCompany(ctx, w.ID, created.ID, model.CompanyPatch{Name: strPtr("Acme Inc"), Phone: strPtr("")})
	if err != nil {
		t.Fatalf("UpdateCompany: %v", err)
	}
	if updated.Name != "Acme Inc" || updated.Phone != "" || updated.Domain != "acme.example" {
		t.Errorf("updated = %+v", updated)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("updated_at did not advance: %v then %v", created.UpdatedAt, updated.UpdatedAt)
	}

	if err := s.DeleteCompany(ctx, w.ID, created.ID); err != nil {
		t.Fatalf("DeleteCompany: %v", err)
	}
	if _, err := s.GetCompany(ctx, w.ID, created.ID); !errors.Is(err, ErrCompanyNotFound) {
		t.Errorf("get after delete = %v", err)
	}
	if _, err := s.UpdateCompany(ctx, w.ID, created.ID, model.CompanyPatch{Name: strPtr("Zombie")}); !errors.Is(err, ErrCompanyNotFound) {
		t.Errorf("update after delete = %v", err)
	}
	if err := s.DeleteCompany(ctx, w.ID, created.ID); !errors.Is(err, ErrCompanyNotFound) {
		t.Errorf("second delete = %v", err)
	}
	page, err := s.ListCompanies(ctx, w.ID, model.ListParams{})
	if err != nil || len(page.Items) != 0 {
		t.Errorf("list after delete = %+v, %v", page, err)
	}

	// The row is kept: this is a soft delete.
	var deleted bool
	if err := ownerPool(t).QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM companies WHERE id = $1`, created.ID).Scan(&deleted); err != nil || !deleted {
		t.Errorf("row after delete: deleted = %v, %v", deleted, err)
	}
}

func TestCompanyMalformedAndUnknownIDsAreNotFound(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	w := newWorkspace(t, s, newUser(t, s))
	for _, id := range []string{unknownID, "not-a-uuid", "", "00000000-0000-4000-8000-00000000000g"} {
		if _, err := s.GetCompany(ctx, w.ID, id); !errors.Is(err, ErrCompanyNotFound) {
			t.Errorf("GetCompany(%q) = %v", id, err)
		}
		if _, err := s.UpdateCompany(ctx, w.ID, id, model.CompanyPatch{Name: strPtr("x")}); !errors.Is(err, ErrCompanyNotFound) {
			t.Errorf("UpdateCompany(%q) = %v", id, err)
		}
		if err := s.DeleteCompany(ctx, w.ID, id); !errors.Is(err, ErrCompanyNotFound) {
			t.Errorf("DeleteCompany(%q) = %v", id, err)
		}
	}
	if _, err := s.CreateCompany(ctx, "not-a-uuid", model.CompanyInput{Name: "x"}); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("create in malformed workspace = %v", err)
	}
	if _, err := s.CreateCompany(ctx, unknownID, model.CompanyInput{Name: "x"}); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("create in unknown workspace = %v", err)
	}
}

func TestCompanySearchTreatsWildcardsAsLiterals(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	w := newWorkspace(t, s, newUser(t, s))
	for _, in := range []model.CompanyInput{
		{Name: "Acme Industries", Domain: "acme.example"},
		{Name: "100% Organic", Domain: "organic.test"},
		{Name: "under_score Ltd", Domain: "underscore.test"},
		{Name: "underXscore Ltd", Domain: "other.test"},
		{Name: `Back\slash Co`, Domain: "slash.test"},
	} {
		if _, err := s.CreateCompany(ctx, w.ID, in); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		name, q string
		want    []string
	}{
		{"name, any case", "aCmE", []string{"Acme Industries"}},
		{"middle of a name", "dustri", []string{"Acme Industries"}},
		{"domain", "ORGANIC.TEST", []string{"100% Organic"}},
		{"percent is literal", "%", []string{"100% Organic"}},
		{"underscore is literal", "under_score", []string{"under_score Ltd"}},
		{"lone underscore is literal", "_", []string{"under_score Ltd"}},
		{"backslash is literal", `\`, []string{`Back\slash Co`}},
		{"no match", "zzz", []string{}},
		{"a pattern is not a pattern", "A%e", []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			page, err := s.ListCompanies(ctx, w.ID, model.ListParams{Search: tt.q})
			if err != nil {
				t.Fatal(err)
			}
			if got := companyNames(page); !slices.Equal(got, tt.want) {
				t.Errorf("search %q = %q, want %q", tt.q, got, tt.want)
			}
		})
	}
}

func TestCompanyPaginationIsBoundedAndStable(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	w := newWorkspace(t, s, newUser(t, s))

	// Order ignores case, and identical names fall back to the ID.
	var sameIDs []string
	for range 5 {
		sameIDs = append(sameIDs, newCompany(t, s, w.ID, "same").ID)
	}
	slices.Sort(sameIDs)
	newCompany(t, s, w.ID, "Banana")
	newCompany(t, s, w.ID, "apple")
	newCompany(t, s, w.ID, "Tail")

	var ids []string
	var names []string
	for offset := 0; ; offset += 3 {
		page, err := s.ListCompanies(ctx, w.ID, model.ListParams{Limit: 3, Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		if page.Limit != 3 || page.Offset != offset {
			t.Errorf("page echoes limit %d, offset %d", page.Limit, page.Offset)
		}
		for _, c := range page.Items {
			ids = append(ids, c.ID)
			names = append(names, c.Name)
		}
		if !page.HasMore {
			if len(page.Items) != 2 {
				t.Errorf("last page has %d items, want 2", len(page.Items))
			}
			break
		}
		if offset > 30 {
			t.Fatal("pagination never ended")
		}
	}
	if want := []string{"apple", "Banana", "same", "same", "same", "same", "same", "Tail"}; !slices.Equal(names, want) {
		t.Errorf("order = %q, want %q", names, want)
	}
	if !slices.Equal(ids[2:7], sameIDs) {
		t.Errorf("ties = %v, want by ID %v", ids[2:7], sameIDs)
	}

	for _, tt := range []struct {
		name       string
		in         model.ListParams
		wantLimit  int
		wantOffset int
	}{
		{"zero limit takes the default", model.ListParams{}, model.DefaultPageSize, 0},
		{"negative limit takes the default", model.ListParams{Limit: -1}, model.DefaultPageSize, 0},
		{"huge limit is capped", model.ListParams{Limit: 1 << 40}, model.MaxPageSize, 0},
		{"negative offset is zero", model.ListParams{Offset: -5}, model.DefaultPageSize, 0},
		{"huge offset is capped", model.ListParams{Offset: 1 << 40}, model.DefaultPageSize, model.MaxPageOffset},
	} {
		t.Run(tt.name, func(t *testing.T) {
			page, err := s.ListCompanies(ctx, w.ID, tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if page.Limit != tt.wantLimit || page.Offset != tt.wantOffset {
				t.Errorf("limit %d, offset %d; want %d, %d", page.Limit, page.Offset, tt.wantLimit, tt.wantOffset)
			}
			if page.Items == nil {
				t.Error("items is nil, which would encode as null")
			}
		})
	}
}

// The migration owner is a superuser and bypasses Row-Level Security, so a store
// on its pool has only its own workspace_id filters to rely on.
func TestCompanyStoreFiltersWithoutRLS(t *testing.T) {
	tn := newTenants(t)
	ctx := context.Background()
	inB := newCompany(t, New(tn.pool), tn.b.ID, "Secret of B")
	s := New(ownerPool(t))

	if _, err := s.GetCompany(ctx, tn.a.ID, inB.ID); !errors.Is(err, ErrCompanyNotFound) {
		t.Errorf("get = %v", err)
	}
	if _, err := s.UpdateCompany(ctx, tn.a.ID, inB.ID, model.CompanyPatch{Name: strPtr("pwned")}); !errors.Is(err, ErrCompanyNotFound) {
		t.Errorf("update = %v", err)
	}
	if err := s.DeleteCompany(ctx, tn.a.ID, inB.ID); !errors.Is(err, ErrCompanyNotFound) {
		t.Errorf("delete = %v", err)
	}
	page, err := s.ListCompanies(ctx, tn.a.ID, model.ListParams{})
	if err != nil || len(page.Items) != 0 {
		t.Errorf("list = %+v, %v", page, err)
	}
	page, err = s.ListCompanies(ctx, tn.a.ID, model.ListParams{Search: "Secret"})
	if err != nil || len(page.Items) != 0 {
		t.Errorf("search = %+v, %v", page, err)
	}
	survivor, err := New(tn.pool).GetCompany(ctx, tn.b.ID, inB.ID)
	if err != nil || survivor.Name != "Secret of B" {
		t.Errorf("B's company after the attempts = %+v, %v", survivor, err)
	}
}

// These run raw SQL as crm_app with no tenant filter, so only the policies stand
// between the two workspaces.
func TestCompanyRLSIsolation(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	inA := newCompany(t, s, tn.a.ID, "Company of A")
	inB := newCompany(t, s, tn.b.ID, "Company of B")

	t.Run("reads", func(t *testing.T) {
		rawTx(t, tn.pool, "app.workspace_id", tn.a.ID, func(ctx context.Context, tx pgx.Tx) {
			if got := strs(t, ctx, tx, `SELECT DISTINCT workspace_id::text FROM companies`); !slices.Equal(got, []string{tn.a.ID}) {
				t.Errorf("workspaces visible %v, want only A", got)
			}
			if got := strs(t, ctx, tx, `SELECT name FROM companies WHERE id = $1`, inB.ID); len(got) != 0 {
				t.Errorf("B's company visible by ID: %v", got)
			}
		})
	})
	for _, tt := range []struct{ name, setting, value string }{
		{"no setting", "", ""},
		{"empty setting", "app.workspace_id", ""},
		{"user setting grants nothing", "app.user_id", tn.ownerA.ID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rawTx(t, tn.pool, tt.setting, tt.value, func(ctx context.Context, tx pgx.Tx) {
				var count int
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM companies`).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Errorf("%d rows visible", count)
				}
			})
		})
	}
	writes := func(name string, fn func(ctx context.Context, tx pgx.Tx)) {
		t.Run(name, func(t *testing.T) { rawTx(t, tn.pool, "app.workspace_id", tn.a.ID, fn) })
	}
	writes("insert into B", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `INSERT INTO companies (workspace_id, name) VALUES ($1, 'rogue')`, tn.b.ID)
		wantDenied(t, "insert", err)
	})
	writes("move from A to B", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `UPDATE companies SET workspace_id = $1 WHERE id = $2`, tn.b.ID, inA.ID)
		wantDenied(t, "move", err)
	})
	writes("update B", func(ctx context.Context, tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `UPDATE companies SET name = 'pwned' WHERE id = $1`, inB.ID)
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("update = %d rows, %v; want 0 rows", tag.RowsAffected(), err)
		}
	})
	writes("soft-delete B", func(ctx context.Context, tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `UPDATE companies SET deleted_at = now() WHERE id = $1`, inB.ID)
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("update = %d rows, %v; want 0 rows", tag.RowsAffected(), err)
		}
	})
	writes("hard delete is not granted", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `DELETE FROM companies WHERE id = $1`, inA.ID)
		wantDenied(t, "delete", err)
	})
}
