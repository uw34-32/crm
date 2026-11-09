package store

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"crm/pkg/model"
	"crm/pkg/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func newContact(t *testing.T, s *Store, workspaceID, firstName string, companyID *string) model.Contact {
	t.Helper()
	c, err := s.CreateContact(context.Background(), workspaceID, model.ContactInput{FirstName: firstName, CompanyID: companyID})
	if err != nil {
		t.Fatalf("CreateContact(%q): %v", firstName, err)
	}
	return c
}

func contactNames(page model.Page[model.Contact]) []string {
	names := make([]string, len(page.Items))
	for i, c := range page.Items {
		names[i] = c.FirstName
	}
	return names
}

func TestContactLifecycle(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	w := newWorkspace(t, s, newUser(t, s))
	company := newCompany(t, s, w.ID, "Analytical Engines")

	created, err := s.CreateContact(ctx, w.ID, model.ContactInput{
		FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.test", Phone: "+44 20 7946 0000", Title: "Analyst", CompanyID: &company.ID,
	})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	if created.ID == "" || created.WorkspaceID != w.ID || created.LastName != "Lovelace" || created.Email != "ada@example.test" ||
		created.Phone != "+44 20 7946 0000" || created.Title != "Analyst" {
		t.Errorf("created = %+v", created)
	}
	if created.CompanyID == nil || *created.CompanyID != company.ID || created.CompanyName == nil || *created.CompanyName != company.Name {
		t.Errorf("created link = %v, %v", created.CompanyID, created.CompanyName)
	}

	got, err := s.GetContact(ctx, w.ID, created.ID)
	if err != nil || got.ID != created.ID || got.CompanyName == nil {
		t.Errorf("GetContact = %+v, %v", got, err)
	}

	// A patch changes only the fields it names, and leaves the link alone.
	updated, err := s.UpdateContact(ctx, w.ID, created.ID, model.ContactPatch{Title: strPtr("Countess"), Email: strPtr("")})
	if err != nil {
		t.Fatalf("UpdateContact: %v", err)
	}
	if updated.Title != "Countess" || updated.Email != "" || updated.FirstName != "Ada" || updated.CompanyID == nil {
		t.Errorf("updated = %+v", updated)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("updated_at did not advance: %v then %v", created.UpdatedAt, updated.UpdatedAt)
	}

	// Setting the link to nil removes it.
	unlinked, err := s.UpdateContact(ctx, w.ID, created.ID, model.ContactPatch{SetCompany: true})
	if err != nil || unlinked.CompanyID != nil || unlinked.CompanyName != nil {
		t.Errorf("unlink = %+v, %v", unlinked, err)
	}
	relinked, err := s.UpdateContact(ctx, w.ID, created.ID, model.ContactPatch{SetCompany: true, CompanyID: &company.ID})
	if err != nil || relinked.CompanyID == nil || *relinked.CompanyID != company.ID {
		t.Errorf("relink = %+v, %v", relinked, err)
	}

	if err := s.DeleteContact(ctx, w.ID, created.ID); err != nil {
		t.Fatalf("DeleteContact: %v", err)
	}
	if _, err := s.GetContact(ctx, w.ID, created.ID); !errors.Is(err, ErrContactNotFound) {
		t.Errorf("get after delete = %v", err)
	}
	if _, err := s.UpdateContact(ctx, w.ID, created.ID, model.ContactPatch{Title: strPtr("Zombie")}); !errors.Is(err, ErrContactNotFound) {
		t.Errorf("update after delete = %v", err)
	}
	if err := s.DeleteContact(ctx, w.ID, created.ID); !errors.Is(err, ErrContactNotFound) {
		t.Errorf("second delete = %v", err)
	}
	page, err := s.ListContacts(ctx, w.ID, model.ListParams{Search: "Ada"})
	if err != nil || len(page.Items) != 0 {
		t.Errorf("search after delete = %+v, %v", page, err)
	}
}

func TestContactMalformedAndUnknownIDsAreNotFound(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	w := newWorkspace(t, s, newUser(t, s))
	for _, id := range []string{unknownID, "not-a-uuid", ""} {
		if _, err := s.GetContact(ctx, w.ID, id); !errors.Is(err, ErrContactNotFound) {
			t.Errorf("GetContact(%q) = %v", id, err)
		}
		if _, err := s.UpdateContact(ctx, w.ID, id, model.ContactPatch{Title: strPtr("x")}); !errors.Is(err, ErrContactNotFound) {
			t.Errorf("UpdateContact(%q) = %v", id, err)
		}
		if err := s.DeleteContact(ctx, w.ID, id); !errors.Is(err, ErrContactNotFound) {
			t.Errorf("DeleteContact(%q) = %v", id, err)
		}
	}
	if _, err := s.CreateContact(ctx, unknownID, model.ContactInput{FirstName: "x"}); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("create in unknown workspace = %v", err)
	}
}

func TestContactSearch(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	w := newWorkspace(t, s, newUser(t, s))
	acme, err := s.CreateCompany(ctx, w.ID, model.CompanyInput{Name: "Acme 100%", Domain: "acme.example"})
	if err != nil {
		t.Fatal(err)
	}
	gone := newCompany(t, s, w.ID, "Vanished Ltd")
	for _, in := range []model.ContactInput{
		{FirstName: "Ada", LastName: "Lovelace", Email: "ada@engines.test"},
		{FirstName: "Grace", LastName: "Hopper", Email: "grace_h@navy.test", CompanyID: &acme.ID},
		{FirstName: "Graceless", LastName: "Xh", Email: "gracexh@navy.test"},
		{FirstName: "Pct%", LastName: "Literal"},
		{FirstName: "Former", CompanyID: &gone.ID},
	} {
		if _, err := s.CreateContact(ctx, w.ID, in); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteCompany(ctx, w.ID, gone.ID); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, q string
		want    []string
	}{
		{"first name, any case", "aDA", []string{"Ada"}},
		{"last name", "hopper", []string{"Grace"}},
		{"full name", "ada lovelace", []string{"Ada"}},
		{"email", "ENGINES.test", []string{"Ada"}},
		{"company name", "acme", []string{"Grace"}},
		{"company domain", "ACME.EXAMPLE", []string{"Grace"}},
		{"underscore is literal", "grace_h", []string{"Grace"}},
		{"percent in the contact and the company", "%", []string{"Grace", "Pct%"}},
		{"a deleted company no longer matches", "vanished", []string{}},
		{"a pattern is not a pattern", "A%a", []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			page, err := s.ListContacts(ctx, w.ID, model.ListParams{Search: tt.q})
			if err != nil {
				t.Fatal(err)
			}
			if got := contactNames(page); !slices.Equal(got, tt.want) {
				t.Errorf("search %q = %q, want %q", tt.q, got, tt.want)
			}
		})
	}
}

func TestContactPaginationIsBoundedAndStable(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	w := newWorkspace(t, s, newUser(t, s))
	var sameIDs []string
	for range 4 {
		sameIDs = append(sameIDs, newContact(t, s, w.ID, "same", nil).ID)
	}
	slices.Sort(sameIDs)
	newContact(t, s, w.ID, "Zed", nil)
	newContact(t, s, w.ID, "abel", nil)

	first, err := s.ListContacts(ctx, w.ID, model.ListParams{Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.ListContacts(ctx, w.ID, model.ListParams{Limit: 4, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || second.HasMore {
		t.Errorf("has_more = %v then %v", first.HasMore, second.HasMore)
	}
	var names, ids []string
	for _, c := range append(first.Items, second.Items...) {
		names = append(names, c.FirstName)
		ids = append(ids, c.ID)
	}
	if want := []string{"abel", "same", "same", "same", "same", "Zed"}; !slices.Equal(names, want) {
		t.Errorf("order = %q, want %q", names, want)
	}
	if !slices.Equal(ids[1:5], sameIDs) {
		t.Errorf("ties = %v, want by ID %v", ids[1:5], sameIDs)
	}
	capped, err := s.ListContacts(ctx, w.ID, model.ListParams{Limit: 1 << 40, Offset: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	if capped.Limit != model.MaxPageSize || capped.Offset != model.MaxPageOffset || capped.Items == nil {
		t.Errorf("capped = %+v", capped)
	}
}

func TestContactCannotLinkAnUnavailableCompany(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	ctx := context.Background()
	inB := newCompany(t, s, tn.b.ID, "Company of B")
	deleted := newCompany(t, s, tn.a.ID, "Deleted of A")
	if err := s.DeleteCompany(ctx, tn.a.ID, deleted.ID); err != nil {
		t.Fatal(err)
	}
	contact := newContact(t, s, tn.a.ID, "Linker", nil)

	for _, tt := range []struct{ name, companyID string }{
		{"another workspace", inB.ID},
		{"soft-deleted", deleted.ID},
		{"nonexistent", unknownID},
		{"malformed", "not-a-uuid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id := tt.companyID
			if _, err := s.CreateContact(ctx, tn.a.ID, model.ContactInput{FirstName: "New", CompanyID: &id}); !errors.Is(err, ErrCompanyNotFound) {
				t.Errorf("create = %v", err)
			}
			if _, err := s.UpdateContact(ctx, tn.a.ID, contact.ID, model.ContactPatch{SetCompany: true, CompanyID: &id}); !errors.Is(err, ErrCompanyNotFound) {
				t.Errorf("update = %v", err)
			}
		})
	}
	got, err := s.GetContact(ctx, tn.a.ID, contact.ID)
	if err != nil || got.CompanyID != nil {
		t.Errorf("contact after the attempts = %+v, %v", got, err)
	}
	page, err := s.ListContacts(ctx, tn.a.ID, model.ListParams{Search: "New"})
	if err != nil || len(page.Items) != 0 {
		t.Errorf("a refused create left a row: %+v, %v", page, err)
	}
}

// Contacts outlive their company, and lose the link in the same transaction, so
// no contact is left pointing at a company nobody can see.
func TestDeletingACompanyUnlinksItsContacts(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	w := newWorkspace(t, s, newUser(t, s))
	company := newCompany(t, s, w.ID, "Closing Down")
	other := newCompany(t, s, w.ID, "Still Open")
	linked := newContact(t, s, w.ID, "Linked", &company.ID)
	alsoDeleted := newContact(t, s, w.ID, "Already Gone", &company.ID)
	unrelated := newContact(t, s, w.ID, "Unrelated", &other.ID)
	if err := s.DeleteContact(ctx, w.ID, alsoDeleted.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteCompany(ctx, w.ID, company.ID); err != nil {
		t.Fatalf("DeleteCompany: %v", err)
	}

	got, err := s.GetContact(ctx, w.ID, linked.ID)
	if err != nil {
		t.Fatalf("the contact did not survive: %v", err)
	}
	if got.CompanyID != nil || got.CompanyName != nil {
		t.Errorf("link after delete = %v, %v", got.CompanyID, got.CompanyName)
	}
	if !got.UpdatedAt.After(linked.UpdatedAt) {
		t.Error("unlinking did not advance updated_at, so a later sync would miss it")
	}
	if got, err := s.GetContact(ctx, w.ID, unrelated.ID); err != nil || got.CompanyID == nil {
		t.Errorf("unrelated contact = %+v, %v", got, err)
	}
	var dangling int
	if err := ownerPool(t).QueryRow(ctx, `SELECT count(*) FROM contacts WHERE company_id = $1`, company.ID).Scan(&dangling); err != nil {
		t.Fatal(err)
	}
	if dangling != 0 {
		t.Errorf("%d contacts, soft-deleted ones included, still reference the deleted company", dangling)
	}
}

// A create racing the delete either finishes first and gets unlinked, or runs
// second and is refused. Neither order may leave a dangling link.
func TestLinkingRacingACompanyDeleteLeavesNoDanglingLink(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	w := newWorkspace(t, s, newUser(t, s))
	owner := ownerPool(t)
	for range 20 {
		company := newCompany(t, s, w.ID, "Contested")
		bystander := newContact(t, s, w.ID, "Bystander", nil)
		var wg sync.WaitGroup
		errs := make(chan error, 3)
		wg.Go(func() {
			errs <- s.DeleteCompany(ctx, w.ID, company.ID)
		})
		wg.Go(func() {
			_, err := s.CreateContact(ctx, w.ID, model.ContactInput{FirstName: "Racer", CompanyID: &company.ID})
			errs <- err
		})
		wg.Go(func() {
			_, err := s.UpdateContact(ctx, w.ID, bystander.ID, model.ContactPatch{SetCompany: true, CompanyID: &company.ID})
			errs <- err
		})
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil && !errors.Is(err, ErrCompanyNotFound) {
				t.Errorf("racing call: %v", err)
			}
		}
		var dangling int
		if err := owner.QueryRow(ctx, `SELECT count(*) FROM contacts WHERE company_id = $1`, company.ID).Scan(&dangling); err != nil {
			t.Fatal(err)
		}
		if dangling != 0 {
			t.Fatalf("%d contacts reference the deleted company", dangling)
		}
	}
}

func TestContactStoreFiltersWithoutRLS(t *testing.T) {
	tn := newTenants(t)
	ctx := context.Background()
	rls := New(tn.pool)
	companyOfB := newCompany(t, rls, tn.b.ID, "Company of B")
	inB := newContact(t, rls, tn.b.ID, "Secret", &companyOfB.ID)
	inA := newContact(t, rls, tn.a.ID, "Mine", nil)
	s := New(ownerPool(t)) // A superuser: only the store's own filters apply.

	if _, err := s.GetContact(ctx, tn.a.ID, inB.ID); !errors.Is(err, ErrContactNotFound) {
		t.Errorf("get = %v", err)
	}
	if _, err := s.UpdateContact(ctx, tn.a.ID, inB.ID, model.ContactPatch{Title: strPtr("pwned")}); !errors.Is(err, ErrContactNotFound) {
		t.Errorf("update = %v", err)
	}
	if err := s.DeleteContact(ctx, tn.a.ID, inB.ID); !errors.Is(err, ErrContactNotFound) {
		t.Errorf("delete = %v", err)
	}
	if _, err := s.UpdateContact(ctx, tn.a.ID, inA.ID, model.ContactPatch{SetCompany: true, CompanyID: &companyOfB.ID}); !errors.Is(err, ErrCompanyNotFound) {
		t.Errorf("link = %v", err)
	}
	// Deleting B's company through A must not reach B's contacts.
	if err := s.DeleteCompany(ctx, tn.a.ID, companyOfB.ID); !errors.Is(err, ErrCompanyNotFound) {
		t.Errorf("delete company = %v", err)
	}
	if page, err := s.ListContacts(ctx, tn.a.ID, model.ListParams{Search: "Secret"}); err != nil || len(page.Items) != 0 {
		t.Errorf("search = %+v, %v", page, err)
	}
	if page, err := s.ListContacts(ctx, tn.a.ID, model.ListParams{}); err != nil || !slices.Equal(contactNames(page), []string{"Mine"}) {
		t.Errorf("list = %+v, %v", page, err)
	}
	survivor, err := rls.GetContact(ctx, tn.b.ID, inB.ID)
	if err != nil || survivor.Title != "" || survivor.CompanyID == nil {
		t.Errorf("B's contact after the attempts = %+v, %v", survivor, err)
	}
}

func TestContactRLSIsolation(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	inA := newContact(t, s, tn.a.ID, "Contact of A", nil)
	inB := newContact(t, s, tn.b.ID, "Contact of B", nil)

	t.Run("reads", func(t *testing.T) {
		rawTx(t, tn.pool, "app.workspace_id", tn.a.ID, func(ctx context.Context, tx pgx.Tx) {
			if got := strs(t, ctx, tx, `SELECT DISTINCT workspace_id::text FROM contacts`); !slices.Equal(got, []string{tn.a.ID}) {
				t.Errorf("workspaces visible %v, want only A", got)
			}
			if got := strs(t, ctx, tx, `SELECT first_name FROM contacts WHERE id = $1`, inB.ID); len(got) != 0 {
				t.Errorf("B's contact visible by ID: %v", got)
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
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM contacts`).Scan(&count); err != nil {
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
		_, err := tx.Exec(ctx, `INSERT INTO contacts (workspace_id, first_name) VALUES ($1, 'rogue')`, tn.b.ID)
		wantDenied(t, "insert", err)
	})
	writes("move from A to B", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `UPDATE contacts SET workspace_id = $1 WHERE id = $2`, tn.b.ID, inA.ID)
		wantDenied(t, "move", err)
	})
	writes("update B", func(ctx context.Context, tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `UPDATE contacts SET first_name = 'pwned' WHERE id = $1`, inB.ID)
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("update = %d rows, %v; want 0 rows", tag.RowsAffected(), err)
		}
	})
	writes("hard delete is not granted", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `DELETE FROM contacts WHERE id = $1`, inA.ID)
		wantDenied(t, "delete", err)
	})
}

// A foreign key check bypasses Row-Level Security, so only the composite key can
// stop these. Each attempt has the tenant settings arranged to give it the best
// chance, and the last two run as the superuser, with no policy in the way at all.
func TestCompositeForeignKeyRefusesCompanyInAnotherWorkspace(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	companyOfA := newCompany(t, s, tn.a.ID, "Company of A")
	companyOfB := newCompany(t, s, tn.b.ID, "Company of B")
	contactOfA := newContact(t, s, tn.a.ID, "Contact of A", &companyOfA.ID)
	const fkViolation = "23503"

	wantFK := func(t *testing.T, err error) {
		t.Helper()
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != fkViolation {
			t.Errorf("got %v, want a foreign key violation", err)
		}
	}
	// Both settings at once, for a user who could plausibly belong to both.
	bothSettings := func(ctx context.Context, tx pgx.Tx, workspaceID string) {
		t.Helper()
		if _, err := tx.Exec(ctx, `SELECT set_config('app.workspace_id', $1, true), set_config('app.user_id', $2, true)`, workspaceID, tn.ownerA.ID); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("link inside the workspace is accepted", func(t *testing.T) {
		rawTx(t, tn.pool, "", "", func(ctx context.Context, tx pgx.Tx) {
			bothSettings(ctx, tx, tn.a.ID)
			if _, err := tx.Exec(ctx, `INSERT INTO contacts (workspace_id, first_name, company_id) VALUES ($1, 'ok', $2)`, tn.a.ID, companyOfA.ID); err != nil {
				t.Errorf("insert: %v", err)
			}
		})
	})
	t.Run("insert as A referencing B", func(t *testing.T) {
		rawTx(t, tn.pool, "", "", func(ctx context.Context, tx pgx.Tx) {
			bothSettings(ctx, tx, tn.a.ID)
			_, err := tx.Exec(ctx, `INSERT INTO contacts (workspace_id, first_name, company_id) VALUES ($1, 'x', $2)`, tn.a.ID, companyOfB.ID)
			wantFK(t, err)
		})
	})
	t.Run("update as A to reference B", func(t *testing.T) {
		rawTx(t, tn.pool, "", "", func(ctx context.Context, tx pgx.Tx) {
			bothSettings(ctx, tx, tn.a.ID)
			_, err := tx.Exec(ctx, `UPDATE contacts SET company_id = $1 WHERE id = $2`, companyOfB.ID, contactOfA.ID)
			wantFK(t, err)
		})
	})
	t.Run("settings for B cannot insert into A", func(t *testing.T) {
		rawTx(t, tn.pool, "", "", func(ctx context.Context, tx pgx.Tx) {
			bothSettings(ctx, tx, tn.b.ID)
			_, err := tx.Exec(ctx, `INSERT INTO contacts (workspace_id, first_name, company_id) VALUES ($1, 'x', $2)`, tn.a.ID, companyOfB.ID)
			wantDenied(t, "insert", err)
		})
	})
	t.Run("superuser inserting A referencing B", func(t *testing.T) {
		ctx := context.Background()
		tx, err := ownerPool(t).Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		_, err = tx.Exec(ctx, `INSERT INTO contacts (workspace_id, first_name, company_id) VALUES ($1, 'x', $2)`, tn.a.ID, companyOfB.ID)
		wantFK(t, err)
	})
	t.Run("superuser moving a linked contact to B", func(t *testing.T) {
		ctx := context.Background()
		tx, err := ownerPool(t).Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		_, err = tx.Exec(ctx, `UPDATE contacts SET workspace_id = $1 WHERE id = $2`, tn.b.ID, contactOfA.ID)
		wantFK(t, err)
	})
}

// Nothing reads external_contact_id yet: a later integration with the contacts
// project writes it. It must be nullable, and must not reach the API.
func TestExternalContactIDIsStoredButNotExposed(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	w := newWorkspace(t, s, newUser(t, s))
	contact := newContact(t, s, w.ID, "Standalone", nil)
	owner := ownerPool(t)

	var isNull bool
	if err := owner.QueryRow(ctx, `SELECT external_contact_id IS NULL FROM contacts WHERE id = $1`, contact.ID).Scan(&isNull); err != nil || !isNull {
		t.Errorf("default external_contact_id: null = %v, %v", isNull, err)
	}
	if _, err := owner.Exec(ctx, `UPDATE contacts SET external_contact_id = 'contacts-42' WHERE id = $1`, contact.ID); err != nil {
		t.Fatalf("set external_contact_id: %v", err)
	}
	if _, err := s.GetContact(ctx, w.ID, contact.ID); err != nil {
		t.Errorf("get with an external ID: %v", err)
	}
}
