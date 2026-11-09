package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"crm/pkg/model"

	"github.com/jackc/pgx/v5/pgxpool"
)

func linkedDeal(t *testing.T, b board, title string, companyID, contactID *string) model.Deal {
	t.Helper()
	d, err := b.s.CreateDeal(context.Background(), b.w.ID, model.DealInput{
		PipelineID: b.p.ID, Title: title, Currency: "USD", CompanyID: companyID, ContactID: contactID,
	})
	if err != nil {
		t.Fatalf("CreateDeal(%q): %v", title, err)
	}
	return d
}

func countRows(t *testing.T, owner *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := owner.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// Deals outlive what they link to, and lose the link in the same transaction, so no
// deal is left pointing at something nobody can see. That covers soft-deleted deals.
func TestDeletingACompanyUnlinksItsDeals(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	company := newCompany(t, b.s, b.w.ID, "Closing Down")
	other := newCompany(t, b.s, b.w.ID, "Still Open")
	contact := newContact(t, b.s, b.w.ID, "Employee", &company.ID)
	linked := linkedDeal(t, b, "Linked", &company.ID, &contact.ID)
	alsoDeleted := linkedDeal(t, b, "Already gone", &company.ID, nil)
	unrelated := linkedDeal(t, b, "Unrelated", &other.ID, nil)
	if err := b.s.DeleteDeal(ctx, b.w.ID, alsoDeleted.ID); err != nil {
		t.Fatal(err)
	}

	if err := b.s.DeleteCompany(ctx, b.w.ID, company.ID); err != nil {
		t.Fatalf("DeleteCompany: %v", err)
	}

	got, err := b.s.GetDeal(ctx, b.w.ID, linked.ID)
	if err != nil {
		t.Fatalf("the deal did not survive: %v", err)
	}
	if got.CompanyID != nil || got.CompanyName != nil {
		t.Errorf("link after delete = %v, %v", got.CompanyID, got.CompanyName)
	}
	if got.ContactID == nil {
		t.Error("deleting a company removed a deal's link to a contact")
	}
	if !got.UpdatedAt.After(linked.UpdatedAt) {
		t.Error("unlinking did not advance updated_at, so a later sync would miss it")
	}
	if got, err := b.s.GetDeal(ctx, b.w.ID, unrelated.ID); err != nil || got.CompanyID == nil {
		t.Errorf("unrelated deal = %+v, %v", got, err)
	}
	if n := countRows(t, ownerPool(t), `SELECT count(*) FROM deals WHERE company_id = $1`, company.ID); n != 0 {
		t.Errorf("%d deals, soft-deleted ones included, still reference the deleted company", n)
	}
}

func TestDeletingAContactUnlinksItsDeals(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	company := newCompany(t, b.s, b.w.ID, "Employer")
	contact := newContact(t, b.s, b.w.ID, "Leaving", nil)
	other := newContact(t, b.s, b.w.ID, "Staying", nil)
	linked := linkedDeal(t, b, "Linked", &company.ID, &contact.ID)
	alsoDeleted := linkedDeal(t, b, "Already gone", nil, &contact.ID)
	unrelated := linkedDeal(t, b, "Unrelated", nil, &other.ID)
	if err := b.s.DeleteDeal(ctx, b.w.ID, alsoDeleted.ID); err != nil {
		t.Fatal(err)
	}

	if err := b.s.DeleteContact(ctx, b.w.ID, contact.ID); err != nil {
		t.Fatalf("DeleteContact: %v", err)
	}

	got, err := b.s.GetDeal(ctx, b.w.ID, linked.ID)
	if err != nil {
		t.Fatalf("the deal did not survive: %v", err)
	}
	if got.ContactID != nil || got.ContactName != nil {
		t.Errorf("link after delete = %v, %v", got.ContactID, got.ContactName)
	}
	if got.CompanyID == nil {
		t.Error("deleting a contact removed a deal's link to a company")
	}
	if !got.UpdatedAt.After(linked.UpdatedAt) {
		t.Error("unlinking did not advance updated_at, so a later sync would miss it")
	}
	if got, err := b.s.GetDeal(ctx, b.w.ID, unrelated.ID); err != nil || got.ContactID == nil {
		t.Errorf("unrelated deal = %+v, %v", got, err)
	}
	if n := countRows(t, ownerPool(t), `SELECT count(*) FROM deals WHERE contact_id = $1`, contact.ID); n != 0 {
		t.Errorf("%d deals, soft-deleted ones included, still reference the deleted contact", n)
	}
}

// A failure while unlinking undoes the delete. The trigger stands in for any failure
// that happens after the delete itself has run.
func TestFailedUnlinkingRollsBackTheDelete(t *testing.T) {
	s, owner := scratchStore(t)
	ctx := context.Background()
	b := newBoardOn(t, s)
	company := newCompany(t, s, b.w.ID, "Kept")
	contact := newContact(t, s, b.w.ID, "Kept", &company.ID)
	deal := linkedDeal(t, b, "Linked", &company.ID, &contact.ID)
	dealOwned, err := s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Owned", Currency: "USD", OwnerID: &b.owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	second := addMember(t, s, b.w, b.owner, model.RoleOwner)
	dealOfSecond, err := s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Second's", Currency: "USD", OwnerID: &second.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		CREATE FUNCTION refuse_deal_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'refused'; END $$;
		CREATE TRIGGER refuse_deal_update BEFORE UPDATE ON deals FOR EACH ROW EXECUTE FUNCTION refuse_deal_update();`); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteCompany(ctx, b.w.ID, company.ID); err == nil {
		t.Error("DeleteCompany succeeded")
	}
	if err := s.DeleteContact(ctx, b.w.ID, contact.ID); err == nil {
		t.Error("DeleteContact succeeded")
	}
	if err := s.RemoveMember(ctx, b.w.ID, b.owner.ID, second.ID); err == nil {
		t.Error("RemoveMember succeeded")
	}
	if err := s.LeaveWorkspace(ctx, b.w.ID, second.ID); err == nil {
		t.Error("LeaveWorkspace succeeded")
	}

	if _, err := s.GetCompany(ctx, b.w.ID, company.ID); err != nil {
		t.Errorf("the company stayed deleted: %v", err)
	}
	gotContact, err := s.GetContact(ctx, b.w.ID, contact.ID)
	if err != nil || gotContact.CompanyID == nil {
		t.Errorf("the contact after the failed delete = %+v, %v", gotContact, err)
	}
	if got, err := s.GetDeal(ctx, b.w.ID, deal.ID); err != nil || got.CompanyID == nil || got.ContactID == nil {
		t.Errorf("the deal after the failed deletes = %+v, %v", got, err)
	}
	if got, err := s.GetDeal(ctx, b.w.ID, dealOfSecond.ID); err != nil || got.OwnerID == nil {
		t.Errorf("the second owner's deal = %+v, %v", got, err)
	}
	if got, err := s.GetDeal(ctx, b.w.ID, dealOwned.ID); err != nil || got.OwnerID == nil {
		t.Errorf("the owned deal = %+v, %v", got, err)
	}
	if n := countRows(t, owner, `SELECT count(*) FROM workspace_members WHERE user_id = $1`, second.ID); n != 1 {
		t.Errorf("%d memberships left for a removal that failed", n)
	}
}

// A link racing the delete either finishes first and gets cleared, or runs second and
// is refused. Neither order may leave a dangling link.
func TestLinkingRacingADeleteLeavesNoDanglingDealLink(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	owner := ownerPool(t)
	t.Run("company", func(t *testing.T) {
		for range 20 {
			company := newCompany(t, b.s, b.w.ID, "Contested")
			bystander := b.deal(t, "Bystander")
			var wg sync.WaitGroup
			errs := make([]error, 3)
			wg.Go(func() { errs[0] = b.s.DeleteCompany(ctx, b.w.ID, company.ID) })
			wg.Go(func() {
				_, errs[1] = b.s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Racer", Currency: "USD", CompanyID: &company.ID})
			})
			wg.Go(func() {
				_, errs[2] = b.s.UpdateDeal(ctx, b.w.ID, bystander.ID, model.DealPatch{SetCompany: true, CompanyID: &company.ID})
			})
			wg.Wait()
			for _, err := range errs {
				if err != nil && !errors.Is(err, ErrCompanyNotFound) {
					t.Errorf("racing call: %v", err)
				}
			}
			if n := countRows(t, owner, `SELECT count(*) FROM deals WHERE company_id = $1`, company.ID); n != 0 {
				t.Fatalf("%d deals reference the deleted company", n)
			}
		}
	})
	t.Run("contact", func(t *testing.T) {
		for range 20 {
			contact := newContact(t, b.s, b.w.ID, "Contested", nil)
			bystander := b.deal(t, "Bystander")
			var wg sync.WaitGroup
			errs := make([]error, 3)
			wg.Go(func() { errs[0] = b.s.DeleteContact(ctx, b.w.ID, contact.ID) })
			wg.Go(func() {
				_, errs[1] = b.s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Racer", Currency: "USD", ContactID: &contact.ID})
			})
			wg.Go(func() {
				_, errs[2] = b.s.UpdateDeal(ctx, b.w.ID, bystander.ID, model.DealPatch{SetContact: true, ContactID: &contact.ID})
			})
			wg.Wait()
			for _, err := range errs {
				if err != nil && !errors.Is(err, ErrContactNotFound) {
					t.Errorf("racing call: %v", err)
				}
			}
			if n := countRows(t, owner, `SELECT count(*) FROM deals WHERE contact_id = $1`, contact.ID); n != 0 {
				t.Fatalf("%d deals reference the deleted contact", n)
			}
		}
	})
}

// Both of these update many deal rows at once. Without one lock order, they deadlock,
// which PostgreSQL resolves by failing one of them.
func TestUnlinkingRacingBoardMovesDoesNotDeadlock(t *testing.T) {
	const deals = 6
	s, _, _ := raceStore(t, deals+6)
	b := newBoardOn(t, s)
	ctx := context.Background()
	second := addMember(t, s, b.w, b.owner, model.RoleOwner)
	stages := []string{b.open.ID, b.next.ID}
	for round := range 15 {
		company := newCompany(t, s, b.w.ID, "Going")
		contact := newContact(t, s, b.w.ID, "Going", &company.ID)
		leaver := addMember(t, s, b.w, b.owner, model.RoleMember)
		ids := make([]string, deals)
		for i := range deals {
			d, err := s.CreateDeal(ctx, b.w.ID, model.DealInput{
				PipelineID: b.p.ID, Title: "Linked", Currency: "USD", CompanyID: &company.ID, ContactID: &contact.ID, OwnerID: &leaver.ID,
			})
			if err != nil {
				t.Fatal(err)
			}
			ids[i] = d.ID
		}
		var wg sync.WaitGroup
		errs := make([]error, deals+3)
		for i, id := range ids {
			wg.Go(func() {
				_, errs[i] = s.MoveDeal(ctx, b.w.ID, id, model.DealMove{StageID: stages[(i+round)%2], Position: intPtr(0)})
			})
		}
		wg.Go(func() { errs[deals] = s.DeleteCompany(ctx, b.w.ID, company.ID) })
		wg.Go(func() { errs[deals+1] = s.DeleteContact(ctx, b.w.ID, contact.ID) })
		wg.Go(func() { errs[deals+2] = s.RemoveMember(ctx, b.w.ID, second.ID, leaver.ID) })
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d, call %d: %v", round, i, err)
			}
		}
		b.stageDeals(t, b.open.ID)
		b.stageDeals(t, b.next.ID)
		for _, id := range ids {
			got, err := s.GetDeal(ctx, b.w.ID, id)
			if err != nil || got.CompanyID != nil || got.ContactID != nil || got.OwnerID != nil {
				t.Fatalf("round %d: deal after unlinking = %+v, %v", round, got, err)
			}
		}
	}
}

func TestRemovingAMemberKeepsTheirDealsWithNoOwner(t *testing.T) {
	removals := map[string]func(s *Store, w, actor, target string) error{
		"removed by a manager": func(s *Store, w, actor, target string) error {
			return s.RemoveMember(context.Background(), w, actor, target)
		},
		"leaving": func(s *Store, w, _, target string) error {
			return s.LeaveWorkspace(context.Background(), w, target)
		},
	}
	for name, remove := range removals {
		t.Run(name, func(t *testing.T) {
			b := newBoard(t)
			ctx := context.Background()
			leaver := addMember(t, b.s, b.w, b.owner, model.RoleMember)
			owned, err := b.s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Owned", Currency: "USD", OwnerID: &leaver.ID})
			if err != nil {
				t.Fatal(err)
			}
			deleted, err := b.s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Deleted", Currency: "USD", OwnerID: &leaver.ID})
			if err != nil {
				t.Fatal(err)
			}
			if err := b.s.DeleteDeal(ctx, b.w.ID, deleted.ID); err != nil {
				t.Fatal(err)
			}
			kept := b.deal(t, "Kept")

			if err := remove(b.s, b.w.ID, b.owner.ID, leaver.ID); err != nil {
				t.Fatalf("removal: %v", err)
			}

			got, err := b.s.GetDeal(ctx, b.w.ID, owned.ID)
			if err != nil {
				t.Fatalf("the deal did not survive: %v", err)
			}
			if got.OwnerID != nil {
				t.Errorf("owner after removal = %v", *got.OwnerID)
			}
			if !got.UpdatedAt.After(owned.UpdatedAt) {
				t.Error("clearing the owner did not advance updated_at, so a later sync would miss it")
			}
			if got, err := b.s.GetDeal(ctx, b.w.ID, kept.ID); err != nil || got.OwnerID == nil || *got.OwnerID != b.owner.ID {
				t.Errorf("another member's deal = %+v, %v", got, err)
			}
			if n := countRows(t, ownerPool(t), `SELECT count(*) FROM deals WHERE owner_id = $1 AND workspace_id = $2`, leaver.ID, b.w.ID); n != 0 {
				t.Errorf("%d deals, soft-deleted ones included, still name the former member", n)
			}
			// The former member can no longer be assigned.
			if _, err := b.s.UpdateDeal(ctx, b.w.ID, owned.ID, model.DealPatch{SetOwner: true, OwnerID: &leaver.ID}); !errors.Is(err, ErrMemberNotFound) {
				t.Errorf("assign the former member = %v", err)
			}
			// Coming back does not bring the deals back.
			if _, err := b.s.AddMember(ctx, b.w.ID, b.owner.ID, leaver.Email, model.RoleMember); err != nil {
				t.Fatal(err)
			}
			if got, err := b.s.GetDeal(ctx, b.w.ID, owned.ID); err != nil || got.OwnerID != nil {
				t.Errorf("after rejoining = %+v, %v", got, err)
			}
		})
	}
}

// Only the membership of this workspace goes: the same user's deals elsewhere stay.
func TestRemovingAMemberLeavesTheirDealsInOtherWorkspaces(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	shared := addMember(t, b.s, b.w, b.owner, model.RoleMember)
	elsewhere := newWorkspace(t, b.s, shared)
	p := defaultPipeline(t, b.s, elsewhere.ID)
	d, err := b.s.CreateDeal(ctx, elsewhere.ID, model.DealInput{PipelineID: p.ID, Title: "Elsewhere", Currency: "USD", OwnerID: &shared.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.s.RemoveMember(ctx, b.w.ID, b.owner.ID, shared.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := b.s.GetDeal(ctx, elsewhere.ID, d.ID); err != nil || got.OwnerID == nil {
		t.Errorf("deal elsewhere = %+v, %v", got, err)
	}
}

// An assignment racing the removal either commits first and gets cleared, or runs
// second and is refused. The removal must not fail on the foreign key either way.
func TestAssigningRacingARemovalLeavesNoFormerMemberOwner(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	owner := ownerPool(t)
	for range 25 {
		leaver := addMember(t, b.s, b.w, b.owner, model.RoleMember)
		bystander := b.deal(t, "Bystander")
		// A deal they already own makes the removal need the pipeline lock, which the
		// racers wait for while they hold a share lock on the member.
		if _, err := b.s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Owned", Currency: "USD", OwnerID: &leaver.ID}); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make([]error, 3)
		wg.Go(func() { errs[0] = b.s.RemoveMember(ctx, b.w.ID, b.owner.ID, leaver.ID) })
		wg.Go(func() {
			_, errs[1] = b.s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Racer", Currency: "USD", OwnerID: &leaver.ID})
		})
		wg.Go(func() {
			_, errs[2] = b.s.UpdateDeal(ctx, b.w.ID, bystander.ID, model.DealPatch{SetOwner: true, OwnerID: &leaver.ID})
		})
		wg.Wait()
		if errs[0] != nil {
			t.Fatalf("the removal failed: %v", errs[0])
		}
		for _, err := range errs[1:] {
			if err != nil && !errors.Is(err, ErrMemberNotFound) {
				t.Errorf("racing call: %v", err)
			}
		}
		if n := countRows(t, owner, `SELECT count(*) FROM deals WHERE owner_id = $1`, leaver.ID); n != 0 {
			t.Fatalf("%d deals name the former member", n)
		}
	}
}

// The schema has a backstop of its own: even a membership deleted outside the store
// cannot fail on, or dangle from, the deal's foreign key.
func TestDealOwnerForeignKeyClearsOnlyTheOwner(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	ctx := context.Background()
	p := defaultPipeline(t, s, tn.a.ID)
	d, err := s.CreateDeal(ctx, tn.a.ID, model.DealInput{PipelineID: p.ID, Title: "Owned", Currency: "USD", OwnerID: &tn.memberOfA.ID})
	if err != nil {
		t.Fatal(err)
	}
	owner := ownerPool(t)
	if _, err := owner.Exec(ctx, `DELETE FROM workspace_members WHERE workspace_id = $1 AND user_id = $2`, tn.a.ID, tn.memberOfA.ID); err != nil {
		t.Fatalf("delete membership: %v", err)
	}
	got, err := s.GetDeal(ctx, tn.a.ID, d.ID)
	if err != nil || got.OwnerID != nil || got.WorkspaceID != tn.a.ID {
		t.Errorf("deal = %+v, %v", got, err)
	}
}

// plantRow inserts a row the foreign keys would refuse, standing in for corrupt data,
// which only a superuser can write. It runs in a scratch schema, so nothing else sees it.
func plantRow(t *testing.T, owner *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("plant: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// Deletes in A must not reach B, neither through the delete itself nor through the
// unlinking that follows it. The store runs as a superuser here, so no policy helps:
// only the store's own workspace filters stand between the two. A member of both
// workspaces owns a deal in B, and a corrupt row of B points at A's company and
// contact, so unlinking without the workspace filter would clear B's links too.
func TestUnlinkingStaysInsideTheWorkspace(t *testing.T) {
	rls, owner := scratchStore(t)
	ctx := context.Background()
	s := New(owner) // A superuser: only the store's own filters apply.
	ownerA, ownerB := newUser(t, rls), newUser(t, rls)
	a, b := newWorkspace(t, rls, ownerA), newWorkspace(t, rls, ownerB)
	shared := addMember(t, rls, a, ownerA, model.RoleMember)
	if _, err := rls.AddMember(ctx, b.ID, ownerB.ID, shared.Email, model.RoleMember); err != nil {
		t.Fatal(err)
	}
	ofB := defaultPipeline(t, rls, b.ID)
	companyA, companyB := newCompany(t, rls, a.ID, "Company of A"), newCompany(t, rls, b.ID, "Company of B")
	contactA, contactB := newContact(t, rls, a.ID, "Contact of A", nil), newContact(t, rls, b.ID, "Contact of B", nil)
	d, err := rls.CreateDeal(ctx, b.ID, model.DealInput{
		PipelineID: ofB.ID, Title: "Deal of B", Currency: "USD", CompanyID: &companyB.ID, ContactID: &contactB.ID, OwnerID: &shared.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	plantRow(t, owner, `
		INSERT INTO deals (workspace_id, pipeline_id, stage_id, title, currency, company_id, contact_id, position)
		VALUES ($1, $2, $3, 'Planted in B', 'USD', $4, $5, 99)`, b.ID, ofB.ID, ofB.Stages[0].ID, companyA.ID, contactA.ID)

	// B's rows are out of reach from A.
	if err := s.DeleteCompany(ctx, a.ID, companyB.ID); !errors.Is(err, ErrCompanyNotFound) {
		t.Errorf("delete B's company = %v", err)
	}
	if err := s.DeleteContact(ctx, a.ID, contactB.ID); !errors.Is(err, ErrContactNotFound) {
		t.Errorf("delete B's contact = %v", err)
	}
	if err := s.RemoveMember(ctx, a.ID, ownerA.ID, ownerB.ID); !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("remove B's owner = %v", err)
	}
	// A's own deletes succeed and unlink only A's rows.
	if err := s.DeleteCompany(ctx, a.ID, companyA.ID); err != nil {
		t.Errorf("delete A's company: %v", err)
	}
	if err := s.DeleteContact(ctx, a.ID, contactA.ID); err != nil {
		t.Errorf("delete A's contact: %v", err)
	}
	if err := s.RemoveMember(ctx, a.ID, ownerA.ID, shared.ID); err != nil {
		t.Errorf("remove the shared member from A: %v", err)
	}

	got, err := rls.GetDeal(ctx, b.ID, d.ID)
	if err != nil || got.CompanyID == nil || got.ContactID == nil || got.OwnerID == nil || *got.OwnerID != shared.ID {
		t.Errorf("B's deal after A's deletes = %+v, %v", got, err)
	}
	if n := countRows(t, owner, `SELECT count(*) FROM deals WHERE workspace_id = $1 AND company_id = $2 AND contact_id = $3`,
		b.ID, companyA.ID, contactA.ID); n != 1 {
		t.Errorf("A's deletes unlinked a row of B: %d planted rows left", n)
	}
}

// These two arrange one exact interleaving. The test holds the pipeline lock, queues the
// delete first and the link second, then lets go, so the delete is the first to get the
// pipeline. If the delete has not by then taken the row the link holds a share lock on,
// the two wait for each other.
func TestRemovalAheadOfAnAssignmentDoesNotDeadlock(t *testing.T) {
	s, _, app := raceStore(t, 6)
	b := newBoardOn(t, s)
	ctx := context.Background()
	owner := ownerPool(t)
	leaver := addMember(t, s, b.w, b.owner, model.RoleMember)
	if _, err := s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Owned", Currency: "USD", OwnerID: &leaver.ID}); err != nil {
		t.Fatal(err)
	}

	release := holdPipeline(t, owner, b.p.ID, app, 2)
	var wg sync.WaitGroup
	var removeErr, createErr error
	wg.Go(func() { removeErr = s.RemoveMember(ctx, b.w.ID, b.owner.ID, leaver.ID) })
	waitQueued(t, owner, app, 1)
	wg.Go(func() {
		_, createErr = s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Racer", Currency: "USD", OwnerID: &leaver.ID})
	})
	release()
	wg.Wait()
	if removeErr != nil {
		t.Errorf("RemoveMember: %v", removeErr)
	}
	if !errors.Is(createErr, ErrMemberNotFound) {
		t.Errorf("CreateDeal = %v, want ErrMemberNotFound", createErr)
	}
	if n := countRows(t, owner, `SELECT count(*) FROM deals WHERE owner_id = $1`, leaver.ID); n != 0 {
		t.Errorf("%d deals name the former member", n)
	}
}

func TestCompanyDeleteAheadOfALinkToItsContactDoesNotDeadlock(t *testing.T) {
	s, _, app := raceStore(t, 6)
	b := newBoardOn(t, s)
	ctx := context.Background()
	owner := ownerPool(t)
	company := newCompany(t, s, b.w.ID, "Going")
	contact := newContact(t, s, b.w.ID, "Employee", &company.ID)
	linkedDeal(t, b, "Linked", &company.ID, nil)

	release := holdPipeline(t, owner, b.p.ID, app, 2)
	var wg sync.WaitGroup
	var deleteErr, createErr error
	wg.Go(func() { deleteErr = s.DeleteCompany(ctx, b.w.ID, company.ID) })
	waitQueued(t, owner, app, 1)
	wg.Go(func() {
		_, createErr = s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Racer", Currency: "USD", ContactID: &contact.ID})
	})
	release()
	wg.Wait()
	if deleteErr != nil {
		t.Errorf("DeleteCompany: %v", deleteErr)
	}
	if createErr != nil {
		t.Errorf("CreateDeal: %v", createErr)
	}
	if n := countRows(t, owner, `SELECT count(*) FROM deals WHERE company_id = $1`, company.ID); n != 0 {
		t.Errorf("%d deals reference the deleted company", n)
	}
}
