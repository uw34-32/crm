package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"crm/pkg/model"
)

// linkTarget is something an activity links to, with the way to delete it.
type linkTarget struct {
	column string
	link   func(in *model.ActivityInput)
	remove func() error
}

// targets makes one fresh contact, company, and deal of the board, each with its delete.
func targets(t *testing.T, b board) map[string]linkTarget {
	t.Helper()
	ctx := context.Background()
	contact := newContact(t, b.s, b.w.ID, "Leaving", nil)
	company := newCompany(t, b.s, b.w.ID, "Closing Down")
	d := b.deal(t, "Dropped")
	return map[string]linkTarget{
		"contact": {"contact_id", func(in *model.ActivityInput) { in.ContactID = &contact.ID },
			func() error { return b.s.DeleteContact(ctx, b.w.ID, contact.ID) }},
		"company": {"company_id", func(in *model.ActivityInput) { in.CompanyID = &company.ID },
			func() error { return b.s.DeleteCompany(ctx, b.w.ID, company.ID) }},
		"deal": {"deal_id", func(in *model.ActivityInput) { in.DealID = &d.ID },
			func() error { return b.s.DeleteDeal(ctx, b.w.ID, d.ID) }},
	}
}

// An activity outlives what it links to and loses the link in the same transaction. If
// that was its last link, it goes too: nothing is left about something nobody can see,
// and a live activity always links to something.
func TestDeletingALinkedRecordClearsActivityLinks(t *testing.T) {
	for _, name := range []string{"contact", "company", "deal"} {
		t.Run(name, func(t *testing.T) {
			b := newBoard(t)
			ctx := context.Background()
			target := targets(t, b)[name]
			// A second link of another kind, which the delete leaves alone.
			keeperCompany := newCompany(t, b.s, b.w.ID, "Keeper")
			keeperContact := newContact(t, b.s, b.w.ID, "Keeper", nil)
			add := func(subject string, twoLinks bool) model.Activity {
				in := model.ActivityInput{Kind: model.ActivityNote, Subject: subject, AuthorID: b.owner.ID}
				if twoLinks && name == "contact" {
					in.CompanyID = &keeperCompany.ID
				} else if twoLinks {
					in.ContactID = &keeperContact.ID
				}
				target.link(&in)
				return newActivity(t, b.s, b.w.ID, in)
			}
			kept := add("Kept", true)
			orphaned := add("Orphaned", false)
			alreadyDeleted := add("Already gone", true)
			if err := b.s.DeleteActivity(ctx, b.w.ID, b.owner.ID, alreadyDeleted.ID); err != nil {
				t.Fatal(err)
			}

			if err := target.remove(); err != nil {
				t.Fatalf("delete the %s: %v", name, err)
			}

			got, err := b.s.GetActivity(ctx, b.w.ID, kept.ID)
			if err != nil {
				t.Fatalf("an activity with another link did not survive: %v", err)
			}
			if !got.UpdatedAt.After(kept.UpdatedAt) {
				t.Error("unlinking did not advance updated_at, so a later sync would miss it")
			}
			if _, err := b.s.GetActivity(ctx, b.w.ID, orphaned.ID); !errors.Is(err, ErrActivityNotFound) {
				t.Errorf("an activity left with no link = %v, want it deleted with its last link", err)
			}
			owner := ownerPool(t)
			if n := countRows(t, owner, `SELECT count(*) FROM activities WHERE workspace_id = $1 AND `+target.column+` IS NOT NULL
				AND id IN ($2, $3, $4)`, b.w.ID, kept.ID, orphaned.ID, alreadyDeleted.ID); n != 0 {
				t.Errorf("%d activities, deleted ones included, still reference the deleted %s", n, name)
			}
			if n := countRows(t, owner, `SELECT count(*) FROM activities WHERE id = $1 AND deleted_at IS NOT NULL`, orphaned.ID); n != 1 {
				t.Errorf("the orphaned activity is not kept as a soft delete: %d rows", n)
			}
		})
	}
}

// A member's activities stay when they go: the author is no longer known, and a task
// they were assigned has no assignee, for a manager to reassign. Rejoining does not
// bring either back, and the same user's activities elsewhere stay.
func TestRemovingAMemberClearsTheirActivityRoles(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	leaver := addMember(t, b.s, b.w, b.owner, model.RoleMember)
	contact := newContact(t, b.s, b.w.ID, "Ada", nil)
	written := newActivity(t, b.s, b.w.ID, model.ActivityInput{Kind: model.ActivityNote, Subject: "Theirs", AuthorID: leaver.ID, ContactID: &contact.ID})
	assigned := newActivity(t, b.s, b.w.ID, model.ActivityInput{Kind: model.ActivityTask, Subject: "For them", AuthorID: b.owner.ID, AssigneeID: &leaver.ID, ContactID: &contact.ID})
	elsewhere := newWorkspace(t, b.s, leaver)
	contactElsewhere := newContact(t, b.s, elsewhere.ID, "Elsewhere", nil)
	theirsElsewhere := newActivity(t, b.s, elsewhere.ID, model.ActivityInput{Kind: model.ActivityTask, Subject: "Elsewhere", AuthorID: leaver.ID, AssigneeID: &leaver.ID, ContactID: &contactElsewhere.ID})

	if err := b.s.RemoveMember(ctx, b.w.ID, b.owner.ID, leaver.ID); err != nil {
		t.Fatal(err)
	}

	got, err := b.s.GetActivity(ctx, b.w.ID, written.ID)
	if err != nil || got.AuthorID != nil || !got.UpdatedAt.After(written.UpdatedAt) {
		t.Errorf("their note after removal = %+v, %v", got, err)
	}
	got, err = b.s.GetActivity(ctx, b.w.ID, assigned.ID)
	if err != nil || got.AssigneeID != nil || got.AuthorID == nil || *got.AuthorID != b.owner.ID || !got.UpdatedAt.After(assigned.UpdatedAt) {
		t.Errorf("their task after removal = %+v, %v", got, err)
	}
	if got, err := b.s.GetActivity(ctx, elsewhere.ID, theirsElsewhere.ID); err != nil || got.AuthorID == nil || got.AssigneeID == nil {
		t.Errorf("their activity in another workspace = %+v, %v", got, err)
	}
	if _, err := b.s.AddMember(ctx, b.w.ID, b.owner.ID, leaver.Email, model.RoleMember); err != nil {
		t.Fatal(err)
	}
	if got, err := b.s.GetActivity(ctx, b.w.ID, written.ID); err != nil || got.AuthorID != nil {
		t.Errorf("after rejoining = %+v, %v", got, err)
	}
	// The removed author is a stranger to the note now: only a manager may edit it.
	if _, err := b.s.UpdateActivity(ctx, b.w.ID, leaver.ID, written.ID, model.ActivityPatch{Subject: strPtr("Mine again")}); !errors.Is(err, ErrActivityForbidden) {
		t.Errorf("the former author's edit = %v", err)
	}
}

// A failure while unlinking activities undoes the delete. The trigger stands in for
// any failure after the delete and the deal unlinking have run.
func TestFailedActivityUnlinkingRollsBackTheDelete(t *testing.T) {
	s, owner := scratchStore(t)
	ctx := context.Background()
	b := newBoardOn(t, s)
	second := addMember(t, s, b.w, b.owner, model.RoleOwner)
	company := newCompany(t, s, b.w.ID, "Kept")
	contact := newContact(t, s, b.w.ID, "Kept", nil)
	d := b.deal(t, "Kept")
	a := newActivity(t, s, b.w.ID, model.ActivityInput{
		Kind: model.ActivityTask, Subject: "Kept", AuthorID: second.ID, AssigneeID: &second.ID,
		CompanyID: &company.ID, ContactID: &contact.ID, DealID: &d.ID,
	})
	if _, err := owner.Exec(ctx, `
		CREATE FUNCTION refuse_activity_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'refused'; END $$;
		CREATE TRIGGER refuse_activity_update BEFORE UPDATE ON activities FOR EACH ROW EXECUTE FUNCTION refuse_activity_update();`); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteCompany(ctx, b.w.ID, company.ID); err == nil {
		t.Error("DeleteCompany succeeded")
	}
	if err := s.DeleteContact(ctx, b.w.ID, contact.ID); err == nil {
		t.Error("DeleteContact succeeded")
	}
	if err := s.DeleteDeal(ctx, b.w.ID, d.ID); err == nil {
		t.Error("DeleteDeal succeeded")
	}
	if err := s.RemoveMember(ctx, b.w.ID, b.owner.ID, second.ID); err == nil {
		t.Error("RemoveMember succeeded")
	}

	if _, err := s.GetCompany(ctx, b.w.ID, company.ID); err != nil {
		t.Errorf("the company stayed deleted: %v", err)
	}
	if _, err := s.GetContact(ctx, b.w.ID, contact.ID); err != nil {
		t.Errorf("the contact stayed deleted: %v", err)
	}
	if _, err := s.GetDeal(ctx, b.w.ID, d.ID); err != nil {
		t.Errorf("the deal stayed deleted: %v", err)
	}
	got, err := s.GetActivity(ctx, b.w.ID, a.ID)
	if err != nil || got.CompanyID == nil || got.ContactID == nil || got.DealID == nil || got.AuthorID == nil || got.AssigneeID == nil {
		t.Errorf("the activity after the failed deletes = %+v, %v", got, err)
	}
	if n := countRows(t, owner, `SELECT count(*) FROM workspace_members WHERE user_id = $1`, second.ID); n != 1 {
		t.Errorf("%d memberships left for a removal that failed", n)
	}
}

// A link racing the delete either commits first and gets cleared, or runs second and
// is refused. Neither order may leave a live activity pointing at a deleted record.
func TestLinkingAnActivityRacingADeleteLeavesNoDanglingLink(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	owner := ownerPool(t)
	anchor := newContact(t, b.s, b.w.ID, "Anchor", nil)
	notFound := map[string]error{"contact": ErrContactNotFound, "company": ErrCompanyNotFound, "deal": ErrDealNotFound}
	for _, name := range []string{"contact", "company", "deal"} {
		t.Run(name, func(t *testing.T) {
			for range 15 {
				target := targets(t, b)[name]
				bystander := newActivity(t, b.s, b.w.ID, model.ActivityInput{Kind: model.ActivityNote, Subject: "Bystander", AuthorID: b.owner.ID, CompanyID: strPtr(newCompany(t, b.s, b.w.ID, "Anchor").ID)})
				in := model.ActivityInput{Kind: model.ActivityNote, Subject: "Racer", AuthorID: b.owner.ID}
				target.link(&in)
				if name != "contact" {
					in.ContactID = &anchor.ID
				}
				patch := model.ActivityPatch{SetContact: in.ContactID != nil && name == "contact", ContactID: in.ContactID,
					SetCompany: name == "company", CompanyID: in.CompanyID, SetDeal: name == "deal", DealID: in.DealID}
				var wg sync.WaitGroup
				errs := make([]error, 3)
				wg.Go(func() { errs[0] = target.remove() })
				wg.Go(func() { _, errs[1] = b.s.CreateActivity(ctx, b.w.ID, in) })
				wg.Go(func() { _, errs[2] = b.s.UpdateActivity(ctx, b.w.ID, b.owner.ID, bystander.ID, patch) })
				wg.Wait()
				if errs[0] != nil {
					t.Fatalf("delete: %v", errs[0])
				}
				for _, err := range errs[1:] {
					if err != nil && !errors.Is(err, notFound[name]) {
						t.Errorf("racing call: %v", err)
					}
				}
				var id string
				switch name {
				case "contact":
					id = *in.ContactID
				case "company":
					id = *in.CompanyID
				case "deal":
					id = *in.DealID
				}
				if n := countRows(t, owner, `SELECT count(*) FROM activities WHERE `+target.column+` = $1`, id); n != 0 {
					t.Fatalf("%d activities reference the deleted %s", n, name)
				}
			}
		})
	}
}

// These arrange one exact interleaving, as for deals. The test holds the pipeline lock,
// queues the delete first and the activity write second, then lets go, so the delete is
// the first to reach the deals. Were the write to hold the deal before the company or
// the member, the delete would wait for that deal while the write waited for the row
// the delete had already taken.
func TestCompanyDeleteAheadOfAnActivityLinkDoesNotDeadlock(t *testing.T) {
	s, _, app := raceStore(t, 6)
	b := newBoardOn(t, s)
	ctx := context.Background()
	owner := ownerPool(t)
	company := newCompany(t, s, b.w.ID, "Going")
	d := linkedDeal(t, b, "Linked", &company.ID, nil)

	release := holdPipeline(t, owner, b.p.ID, app, 2)
	var wg sync.WaitGroup
	var deleteErr, createErr error
	wg.Go(func() { deleteErr = s.DeleteCompany(ctx, b.w.ID, company.ID) })
	waitQueued(t, owner, app, 1)
	wg.Go(func() {
		_, createErr = s.CreateActivity(ctx, b.w.ID, model.ActivityInput{
			Kind: model.ActivityNote, Subject: "Racer", AuthorID: b.owner.ID, CompanyID: &company.ID, DealID: &d.ID,
		})
	})
	release()
	wg.Wait()
	if deleteErr != nil {
		t.Errorf("DeleteCompany: %v", deleteErr)
	}
	if !errors.Is(createErr, ErrCompanyNotFound) {
		t.Errorf("CreateActivity = %v, want ErrCompanyNotFound", createErr)
	}
}

func TestRemovalAheadOfAnActivityAssignmentDoesNotDeadlock(t *testing.T) {
	s, _, app := raceStore(t, 6)
	b := newBoardOn(t, s)
	ctx := context.Background()
	owner := ownerPool(t)
	leaver := addMember(t, s, b.w, b.owner, model.RoleMember)
	contact := newContact(t, s, b.w.ID, "Ada", nil)
	d, err := s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Owned", Currency: "USD", OwnerID: &leaver.ID})
	if err != nil {
		t.Fatal(err)
	}

	release := holdPipeline(t, owner, b.p.ID, app, 2)
	var wg sync.WaitGroup
	var removeErr, createErr error
	wg.Go(func() { removeErr = s.RemoveMember(ctx, b.w.ID, b.owner.ID, leaver.ID) })
	waitQueued(t, owner, app, 1)
	wg.Go(func() {
		_, createErr = s.CreateActivity(ctx, b.w.ID, model.ActivityInput{
			Kind: model.ActivityTask, Subject: "Racer", AuthorID: b.owner.ID, AssigneeID: &leaver.ID, ContactID: &contact.ID, DealID: &d.ID,
		})
	})
	release()
	wg.Wait()
	if removeErr != nil {
		t.Errorf("RemoveMember: %v", removeErr)
	}
	if !errors.Is(createErr, ErrMemberNotFound) {
		t.Errorf("CreateActivity = %v, want ErrMemberNotFound", createErr)
	}
	if n := countRows(t, owner, `SELECT count(*) FROM activities WHERE assignee_id = $1`, leaver.ID); n != 0 {
		t.Errorf("%d activities name the former member", n)
	}
}

// A deal delete writes every deal row it shifts on the board before any activity row.
// The test holds the activity's row, queues the delete of the first deal and then a
// write moving the activity to the second deal, and lets go. Were the delete to reach
// the activities before closing the gap, it would hold the activity while waiting for
// the second deal, which the write holds while waiting for the activity.
func TestDealDeleteAheadOfAnActivityRelinkDoesNotDeadlock(t *testing.T) {
	s, _, app := raceStore(t, 6)
	b := newBoardOn(t, s)
	ctx := context.Background()
	owner := ownerPool(t)
	contact := newContact(t, s, b.w.ID, "Ada", nil)
	going := b.deal(t, "Going")  // Position 0.
	staying := b.deal(t, "Next") // Position 1, shifted when the first goes.
	a := newActivity(t, s, b.w.ID, model.ActivityInput{Kind: model.ActivityNote, Subject: "Moving", AuthorID: b.owner.ID, ContactID: &contact.ID, DealID: &going.ID})

	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	if _, err := tx.Exec(ctx, `SELECT 1 FROM activities WHERE id = $1 FOR NO KEY UPDATE`, a.ID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var deleteErr, relinkErr error
	wg.Go(func() { deleteErr = s.DeleteDeal(ctx, b.w.ID, going.ID) })
	waitQueued(t, owner, app, 1)
	wg.Go(func() {
		_, relinkErr = s.UpdateActivity(ctx, b.w.ID, b.owner.ID, a.ID, model.ActivityPatch{SetDeal: true, DealID: &staying.ID})
	})
	waitQueued(t, owner, app, 2)
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if deleteErr != nil {
		t.Errorf("DeleteDeal: %v", deleteErr)
	}
	if relinkErr != nil {
		t.Errorf("UpdateActivity: %v", relinkErr)
	}
	if got, err := s.GetActivity(ctx, b.w.ID, a.ID); err != nil || got.DealID == nil || *got.DealID != staying.ID {
		t.Errorf("activity = %+v, %v", got, err)
	}
	b.stageDeals(t, b.open.ID)
}

// Every delete here updates many activity rows at once, overlapping each other and the
// board moves on the same deals. Left to chance over many rounds, like the deal test it
// extends: none may deadlock, and every link they clear must be gone.
func TestActivityUnlinkingRacingDeletesAndMovesDoesNotDeadlock(t *testing.T) {
	const activities = 6
	s, _, _ := raceStore(t, 14)
	b := newBoardOn(t, s)
	ctx := context.Background()
	second := addMember(t, s, b.w, b.owner, model.RoleOwner)
	stages := []string{b.open.ID, b.next.ID}
	for round := range 10 {
		company := newCompany(t, s, b.w.ID, "Going")
		contact := newContact(t, s, b.w.ID, "Going", &company.ID)
		leaver := addMember(t, s, b.w, b.owner, model.RoleMember)
		doomed := linkedDeal(t, b, "Doomed", &company.ID, &contact.ID)
		movers := make([]string, 3)
		for i := range movers {
			movers[i] = linkedDeal(t, b, "Mover", &company.ID, &contact.ID).ID
		}
		ids := make([]string, activities)
		for i := range activities {
			deal := doomed.ID
			if i%2 == 1 {
				deal = movers[i%3]
			}
			ids[i] = newActivity(t, s, b.w.ID, model.ActivityInput{
				Kind: model.ActivityTask, Subject: "Linked", AuthorID: leaver.ID, AssigneeID: &leaver.ID,
				CompanyID: &company.ID, ContactID: &contact.ID, DealID: &deal,
			}).ID
		}
		var wg sync.WaitGroup
		errs := make([]error, 4+len(movers))
		wg.Go(func() { errs[0] = s.DeleteCompany(ctx, b.w.ID, company.ID) })
		wg.Go(func() { errs[1] = s.DeleteContact(ctx, b.w.ID, contact.ID) })
		wg.Go(func() { errs[2] = s.RemoveMember(ctx, b.w.ID, second.ID, leaver.ID) })
		wg.Go(func() { errs[3] = s.DeleteDeal(ctx, b.w.ID, doomed.ID) })
		for i, id := range movers {
			wg.Go(func() {
				_, errs[4+i] = s.MoveDeal(ctx, b.w.ID, id, model.DealMove{StageID: stages[(i+round)%2], Position: intPtr(0)})
			})
		}
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d, call %d: %v", round, i, err)
			}
		}
		for i, id := range ids {
			got, err := s.GetActivity(ctx, b.w.ID, id)
			switch {
			case i%2 == 0 && !errors.Is(err, ErrActivityNotFound):
				t.Fatalf("round %d: an activity that lost every link = %+v, %v", round, got, err)
			case i%2 == 1 && (err != nil || got.CompanyID != nil || got.ContactID != nil || got.AuthorID != nil || got.AssigneeID != nil || got.DealID == nil):
				t.Fatalf("round %d: an activity still on a live deal = %+v, %v", round, got, err)
			}
		}
	}
}
