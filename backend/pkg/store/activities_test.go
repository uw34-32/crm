package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"crm/pkg/model"

	"github.com/jackc/pgx/v5"
)

func newActivity(t *testing.T, s *Store, workspaceID string, in model.ActivityInput) model.Activity {
	t.Helper()
	a, err := s.CreateActivity(context.Background(), workspaceID, in)
	if err != nil {
		t.Fatalf("CreateActivity(%q): %v", in.Subject, err)
	}
	return a
}

func timePtr(t time.Time) *time.Time { return &t }

func activitySubjects(page model.Page[model.Activity]) []string {
	subjects := make([]string, len(page.Items))
	for i, a := range page.Items {
		subjects[i] = a.Subject
	}
	return subjects
}

func TestActivityLifecycle(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	assignee := addMember(t, b.s, b.w, b.owner, model.RoleMember)
	contact := newContact(t, b.s, b.w.ID, "Ada", nil)
	d := b.deal(t, "Engine")
	due := time.Date(2027, 3, 14, 9, 30, 0, 0, time.UTC)

	created := newActivity(t, b.s, b.w.ID, model.ActivityInput{
		Kind: model.ActivityTask, Subject: "Send the proposal", Body: "With the new figures", DueAt: &due,
		AuthorID: b.owner.ID, AssigneeID: &assignee.ID, ContactID: &contact.ID, DealID: &d.ID,
	})
	if created.ID == "" || created.WorkspaceID != b.w.ID || created.Kind != model.ActivityTask || created.Subject != "Send the proposal" ||
		created.Body != "With the new figures" || created.CompletedAt != nil || created.CompanyID != nil {
		t.Errorf("created = %+v", created)
	}
	if created.DueAt == nil || !created.DueAt.Equal(due) {
		t.Errorf("due = %v", created.DueAt)
	}
	if created.AuthorID == nil || *created.AuthorID != b.owner.ID || created.AssigneeID == nil || *created.AssigneeID != assignee.ID {
		t.Errorf("people = %v, %v", created.AuthorID, created.AssigneeID)
	}
	if created.ContactName == nil || *created.ContactName != "Ada" || created.DealTitle == nil || *created.DealTitle != "Engine" {
		t.Errorf("names = %v, %v", created.ContactName, created.DealTitle)
	}

	got, err := b.s.GetActivity(ctx, b.w.ID, created.ID)
	if err != nil || got.ID != created.ID || got.Subject != created.Subject {
		t.Errorf("GetActivity = %+v, %v", got, err)
	}

	updated, err := b.s.UpdateActivity(ctx, b.w.ID, b.owner.ID, created.ID, model.ActivityPatch{
		Subject: strPtr("Send the final proposal"), SetContact: true, SetDue: true,
	})
	if err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}
	if updated.Subject != "Send the final proposal" || updated.Body != created.Body || updated.ContactID != nil || updated.DueAt != nil ||
		updated.DealID == nil || updated.AssigneeID == nil || !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("updated = %+v", updated)
	}

	done, err := b.s.SetActivityCompleted(ctx, b.w.ID, created.ID, true)
	if err != nil || done.CompletedAt == nil {
		t.Fatalf("complete = %+v, %v", done, err)
	}
	again, err := b.s.SetActivityCompleted(ctx, b.w.ID, created.ID, true)
	if err != nil || again.CompletedAt == nil || !again.CompletedAt.Equal(*done.CompletedAt) {
		t.Errorf("completing twice moved the completion time: %v then %v, %v", done.CompletedAt, again.CompletedAt, err)
	}
	reopened, err := b.s.SetActivityCompleted(ctx, b.w.ID, created.ID, false)
	if err != nil || reopened.CompletedAt != nil {
		t.Errorf("reopen = %+v, %v", reopened, err)
	}

	if err := b.s.DeleteActivity(ctx, b.w.ID, b.owner.ID, created.ID); err != nil {
		t.Fatalf("DeleteActivity: %v", err)
	}
	if _, err := b.s.GetActivity(ctx, b.w.ID, created.ID); !errors.Is(err, ErrActivityNotFound) {
		t.Errorf("get after delete = %v", err)
	}
	if _, err := b.s.UpdateActivity(ctx, b.w.ID, b.owner.ID, created.ID, model.ActivityPatch{Subject: strPtr("Zombie")}); !errors.Is(err, ErrActivityNotFound) {
		t.Errorf("update after delete = %v", err)
	}
	if _, err := b.s.SetActivityCompleted(ctx, b.w.ID, created.ID, true); !errors.Is(err, ErrActivityNotFound) {
		t.Errorf("complete after delete = %v", err)
	}
	if err := b.s.DeleteActivity(ctx, b.w.ID, b.owner.ID, created.ID); !errors.Is(err, ErrActivityNotFound) {
		t.Errorf("second delete = %v", err)
	}
	if n := countRows(t, ownerPool(t), `SELECT count(*) FROM activities WHERE id = $1 AND deleted_at IS NOT NULL`, created.ID); n != 1 {
		t.Errorf("soft-deleted rows = %d", n)
	}
}

func TestActivityMalformedAndUnknownIDsAreNotFound(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	for _, id := range []string{unknownID, "not-a-uuid", ""} {
		if _, err := b.s.GetActivity(ctx, b.w.ID, id); !errors.Is(err, ErrActivityNotFound) {
			t.Errorf("GetActivity(%q) = %v", id, err)
		}
		if _, err := b.s.UpdateActivity(ctx, b.w.ID, b.owner.ID, id, model.ActivityPatch{Subject: strPtr("x")}); !errors.Is(err, ErrActivityNotFound) {
			t.Errorf("UpdateActivity(%q) = %v", id, err)
		}
		if err := b.s.DeleteActivity(ctx, b.w.ID, b.owner.ID, id); !errors.Is(err, ErrActivityNotFound) {
			t.Errorf("DeleteActivity(%q) = %v", id, err)
		}
		if _, err := b.s.SetActivityCompleted(ctx, b.w.ID, id, true); !errors.Is(err, ErrActivityNotFound) {
			t.Errorf("SetActivityCompleted(%q) = %v", id, err)
		}
	}
}

// Only a task has a due time, an assignee, and a completed state, and every live
// activity links to something. The schema holds both rules for the row as it ends up.
func TestActivityKindAndLinkRules(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	contact := newContact(t, b.s, b.w.ID, "Ada", nil)
	due := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, kind := range []model.ActivityKind{model.ActivityNote, model.ActivityCall, model.ActivityMeeting} {
		t.Run(string(kind), func(t *testing.T) {
			base := model.ActivityInput{Kind: kind, Subject: "x", AuthorID: b.owner.ID, ContactID: &contact.ID}
			withDue := base
			withDue.DueAt = &due
			if _, err := b.s.CreateActivity(ctx, b.w.ID, withDue); !errors.Is(err, ErrNotATask) {
				t.Errorf("create with a due time = %v", err)
			}
			withAssignee := base
			withAssignee.AssigneeID = &b.owner.ID
			if _, err := b.s.CreateActivity(ctx, b.w.ID, withAssignee); !errors.Is(err, ErrNotATask) {
				t.Errorf("create with an assignee = %v", err)
			}
			a := newActivity(t, b.s, b.w.ID, base)
			if _, err := b.s.UpdateActivity(ctx, b.w.ID, b.owner.ID, a.ID, model.ActivityPatch{SetDue: true, DueAt: &due}); !errors.Is(err, ErrNotATask) {
				t.Errorf("set a due time = %v", err)
			}
			if _, err := b.s.UpdateActivity(ctx, b.w.ID, b.owner.ID, a.ID, model.ActivityPatch{SetAssignee: true, AssigneeID: &b.owner.ID}); !errors.Is(err, ErrNotATask) {
				t.Errorf("set an assignee = %v", err)
			}
			if _, err := b.s.SetActivityCompleted(ctx, b.w.ID, a.ID, true); !errors.Is(err, ErrNotATask) {
				t.Errorf("complete = %v", err)
			}
			if got, err := b.s.GetActivity(ctx, b.w.ID, a.ID); err != nil || got.CompletedAt != nil || !got.UpdatedAt.Equal(a.UpdatedAt) {
				t.Errorf("a refused change changed the activity: %+v, %v", got, err)
			}
		})
	}

	if _, err := b.s.CreateActivity(ctx, b.w.ID, model.ActivityInput{Kind: model.ActivityNote, Subject: "Orphan", AuthorID: b.owner.ID}); !errors.Is(err, ErrActivityNeedsLink) {
		t.Errorf("create with no link = %v", err)
	}
	a := newActivity(t, b.s, b.w.ID, model.ActivityInput{Kind: model.ActivityNote, Subject: "Linked", AuthorID: b.owner.ID, ContactID: &contact.ID})
	if _, err := b.s.UpdateActivity(ctx, b.w.ID, b.owner.ID, a.ID, model.ActivityPatch{SetContact: true}); !errors.Is(err, ErrActivityNeedsLink) {
		t.Errorf("clear the only link = %v", err)
	}
	// Swapping one link for another in the same patch is fine.
	company := newCompany(t, b.s, b.w.ID, "Analytical Engines")
	got, err := b.s.UpdateActivity(ctx, b.w.ID, b.owner.ID, a.ID, model.ActivityPatch{SetContact: true, SetCompany: true, CompanyID: &company.ID})
	if err != nil || got.ContactID != nil || got.CompanyID == nil {
		t.Errorf("swap the link = %+v, %v", got, err)
	}
}

// The schema's bounds on a due time are an error of their own, not a failed write,
// for any caller that did not check them first.
func TestActivityDueOutOfRange(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	contact := newContact(t, b.s, b.w.ID, "Ada", nil)
	task := newActivity(t, b.s, b.w.ID, model.ActivityInput{Kind: model.ActivityTask, Subject: "x", AuthorID: b.owner.ID, ContactID: &contact.ID})
	for _, due := range []time.Time{
		time.Date(2200, 12, 31, 23, 59, 59, 500_000_000, time.UTC),
		time.Date(1899, 12, 31, 23, 59, 59, 0, time.UTC),
	} {
		t.Run(due.Format(time.RFC3339Nano), func(t *testing.T) {
			in := model.ActivityInput{Kind: model.ActivityTask, Subject: "x", AuthorID: b.owner.ID, ContactID: &contact.ID, DueAt: &due}
			if _, err := b.s.CreateActivity(ctx, b.w.ID, in); !errors.Is(err, ErrDueOutOfRange) {
				t.Errorf("create = %v", err)
			}
			if _, err := b.s.UpdateActivity(ctx, b.w.ID, b.owner.ID, task.ID, model.ActivityPatch{SetDue: true, DueAt: &due}); !errors.Is(err, ErrDueOutOfRange) {
				t.Errorf("update = %v", err)
			}
		})
	}
}

// A link must be live and in the workspace, and an assignee a current member. Anything
// else is the same not-found error, whatever the reason.
func TestActivityRefusesUnavailableLinks(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	other := newBoardOn(t, b.s)
	deletedContact := newContact(t, b.s, b.w.ID, "Gone", nil)
	if err := b.s.DeleteContact(ctx, b.w.ID, deletedContact.ID); err != nil {
		t.Fatal(err)
	}
	deletedCompany := newCompany(t, b.s, b.w.ID, "Gone")
	if err := b.s.DeleteCompany(ctx, b.w.ID, deletedCompany.ID); err != nil {
		t.Fatal(err)
	}
	deletedDeal := b.deal(t, "Gone")
	if err := b.s.DeleteDeal(ctx, b.w.ID, deletedDeal.ID); err != nil {
		t.Fatal(err)
	}
	foreignContact := newContact(t, b.s, other.w.ID, "Theirs", nil)
	foreignCompany := newCompany(t, b.s, other.w.ID, "Theirs")
	foreignDeal := other.deal(t, "Theirs")
	live := newContact(t, b.s, b.w.ID, "Live", nil)

	cases := []struct {
		name string
		in   model.ActivityInput
		want error
	}{
		{"deleted contact", model.ActivityInput{ContactID: &deletedContact.ID}, ErrContactNotFound},
		{"foreign contact", model.ActivityInput{ContactID: &foreignContact.ID}, ErrContactNotFound},
		{"malformed contact", model.ActivityInput{ContactID: strPtr("nope")}, ErrContactNotFound},
		{"unknown contact", model.ActivityInput{ContactID: strPtr(unknownID)}, ErrContactNotFound},
		{"deleted company", model.ActivityInput{CompanyID: &deletedCompany.ID}, ErrCompanyNotFound},
		{"foreign company", model.ActivityInput{CompanyID: &foreignCompany.ID}, ErrCompanyNotFound},
		{"deleted deal", model.ActivityInput{DealID: &deletedDeal.ID}, ErrDealNotFound},
		{"foreign deal", model.ActivityInput{DealID: &foreignDeal.ID}, ErrDealNotFound},
		{"malformed deal", model.ActivityInput{DealID: strPtr("nope")}, ErrDealNotFound},
		{"foreign assignee", model.ActivityInput{ContactID: &live.ID, AssigneeID: &other.owner.ID}, ErrMemberNotFound},
		{"malformed assignee", model.ActivityInput{ContactID: &live.ID, AssigneeID: strPtr("nope")}, ErrMemberNotFound},
	}
	a := newActivity(t, b.s, b.w.ID, model.ActivityInput{Kind: model.ActivityTask, Subject: "Target", AuthorID: b.owner.ID, ContactID: &live.ID})
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			in.Kind, in.Subject, in.AuthorID = model.ActivityTask, "x", b.owner.ID
			if _, err := b.s.CreateActivity(ctx, b.w.ID, in); !errors.Is(err, tt.want) {
				t.Errorf("create = %v, want %v", err, tt.want)
			}
			patch := model.ActivityPatch{
				SetContact: in.ContactID != nil, ContactID: in.ContactID, SetCompany: in.CompanyID != nil, CompanyID: in.CompanyID,
				SetDeal: in.DealID != nil, DealID: in.DealID, SetAssignee: in.AssigneeID != nil, AssigneeID: in.AssigneeID,
			}
			if _, err := b.s.UpdateActivity(ctx, b.w.ID, b.owner.ID, a.ID, patch); !errors.Is(err, tt.want) {
				t.Errorf("update = %v, want %v", err, tt.want)
			}
		})
	}
	if got, err := b.s.GetActivity(ctx, b.w.ID, a.ID); err != nil || got.ContactID == nil || *got.ContactID != live.ID {
		t.Errorf("refused updates changed the activity: %+v, %v", got, err)
	}
}

// Every member reads, writes, and completes activities, but only the author or an owner
// or admin edits or deletes one, so nobody rewrites a colleague's notes.
func TestActivityEditRights(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	author := addMember(t, b.s, b.w, b.owner, model.RoleMember)
	colleague := addMember(t, b.s, b.w, b.owner, model.RoleMember)
	admin := addMember(t, b.s, b.w, b.owner, model.RoleAdmin)
	contact := newContact(t, b.s, b.w.ID, "Ada", nil)
	note := newActivity(t, b.s, b.w.ID, model.ActivityInput{Kind: model.ActivityTask, Subject: "Mine", AuthorID: author.ID, ContactID: &contact.ID})

	if _, err := b.s.UpdateActivity(ctx, b.w.ID, colleague.ID, note.ID, model.ActivityPatch{Subject: strPtr("Theirs now")}); !errors.Is(err, ErrActivityForbidden) {
		t.Errorf("a colleague's edit = %v", err)
	}
	if err := b.s.DeleteActivity(ctx, b.w.ID, colleague.ID, note.ID); !errors.Is(err, ErrActivityForbidden) {
		t.Errorf("a colleague's delete = %v", err)
	}
	if _, err := b.s.SetActivityCompleted(ctx, b.w.ID, note.ID, true); err != nil {
		t.Errorf("completing someone else's task: %v", err)
	}
	if _, err := b.s.UpdateActivity(ctx, b.w.ID, author.ID, note.ID, model.ActivityPatch{Subject: strPtr("Still mine")}); err != nil {
		t.Errorf("the author's edit: %v", err)
	}
	for _, manager := range []model.User{admin, b.owner} {
		if _, err := b.s.UpdateActivity(ctx, b.w.ID, manager.ID, note.ID, model.ActivityPatch{Body: strPtr("Reviewed")}); err != nil {
			t.Errorf("a manager's edit: %v", err)
		}
	}
	if err := b.s.DeleteActivity(ctx, b.w.ID, admin.ID, note.ID); err != nil {
		t.Errorf("an admin's delete: %v", err)
	}

	// The role is read inside the transaction: a demoted admin is a member again.
	other := newActivity(t, b.s, b.w.ID, model.ActivityInput{Kind: model.ActivityNote, Subject: "Owner's", AuthorID: b.owner.ID, ContactID: &contact.ID})
	if _, err := b.s.ChangeMemberRole(ctx, b.w.ID, b.owner.ID, admin.ID, model.RoleMember); err != nil {
		t.Fatal(err)
	}
	if err := b.s.DeleteActivity(ctx, b.w.ID, admin.ID, other.ID); !errors.Is(err, ErrActivityForbidden) {
		t.Errorf("a demoted admin's delete = %v", err)
	}
	// Someone who is no longer a member cannot write at all.
	if err := b.s.RemoveMember(ctx, b.w.ID, b.owner.ID, colleague.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.s.CreateActivity(ctx, b.w.ID, model.ActivityInput{Kind: model.ActivityNote, Subject: "x", AuthorID: colleague.ID, ContactID: &contact.ID}); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("a former member's create = %v", err)
	}
}

func TestListActivitiesTimelineAndFilters(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	assignee := addMember(t, b.s, b.w, b.owner, model.RoleMember)
	contact := newContact(t, b.s, b.w.ID, "Ada", nil)
	company := newCompany(t, b.s, b.w.ID, "Analytical Engines")
	d := b.deal(t, "Engine")
	early, late := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	add := func(kind model.ActivityKind, subject string, in model.ActivityInput) model.Activity {
		in.Kind, in.Subject, in.AuthorID = kind, subject, b.owner.ID
		return newActivity(t, b.s, b.w.ID, in)
	}
	add(model.ActivityNote, "Note on Ada", model.ActivityInput{ContactID: &contact.ID, Body: "Likes 100% cotton"})
	add(model.ActivityCall, "Call with the company", model.ActivityInput{CompanyID: &company.ID})
	add(model.ActivityMeeting, "Meeting on the deal", model.ActivityInput{DealID: &d.ID, ContactID: &contact.ID})
	add(model.ActivityTask, "Early task", model.ActivityInput{DealID: &d.ID, DueAt: &early, AssigneeID: &assignee.ID})
	doneTask := add(model.ActivityTask, "Done task", model.ActivityInput{CompanyID: &company.ID, DueAt: &late})
	if _, err := b.s.SetActivityCompleted(ctx, b.w.ID, doneTask.ID, true); err != nil {
		t.Fatal(err)
	}
	add(model.ActivityTask, "Undated task", model.ActivityInput{ContactID: &contact.ID})

	list := func(filter model.ActivityFilter, params model.ListParams) []string {
		t.Helper()
		page, err := b.s.ListActivities(ctx, b.w.ID, filter, params)
		if err != nil {
			t.Fatalf("ListActivities(%+v): %v", filter, err)
		}
		return activitySubjects(page)
	}
	all := []string{"Undated task", "Done task", "Early task", "Meeting on the deal", "Call with the company", "Note on Ada"}
	if got := list(model.ActivityFilter{}, model.ListParams{}); !slices.Equal(got, all) {
		t.Errorf("timeline = %q, want newest first %q", got, all)
	}
	for _, tt := range []struct {
		name   string
		filter model.ActivityFilter
		search string
		want   []string
	}{
		{"contact", model.ActivityFilter{ContactID: contact.ID}, "", []string{"Undated task", "Meeting on the deal", "Note on Ada"}},
		{"company", model.ActivityFilter{CompanyID: company.ID}, "", []string{"Done task", "Call with the company"}},
		{"deal", model.ActivityFilter{DealID: d.ID}, "", []string{"Early task", "Meeting on the deal"}},
		{"assignee", model.ActivityFilter{AssigneeID: assignee.ID}, "", []string{"Early task"}},
		{"kind", model.ActivityFilter{Kind: model.ActivityTask}, "", []string{"Undated task", "Done task", "Early task"}},
		{"open tasks", model.ActivityFilter{State: model.ActivityOpen}, "", []string{"Undated task", "Early task"}},
		{"completed", model.ActivityFilter{State: model.ActivityCompleted}, "", []string{"Done task"}},
		{"due before", model.ActivityFilter{DueBefore: timePtr(late)}, "", []string{"Early task"}},
		{"due after, inclusive", model.ActivityFilter{DueAfter: timePtr(late)}, "", []string{"Done task"}},
		{"due window", model.ActivityFilter{DueAfter: timePtr(early), DueBefore: timePtr(late.Add(time.Second))}, "", []string{"Done task", "Early task"}},
		{"combined", model.ActivityFilter{ContactID: contact.ID, Kind: model.ActivityTask}, "", []string{"Undated task"}},
		{"search the body, wildcards literal", model.ActivityFilter{}, "100%", []string{"Note on Ada"}},
		{"malformed filter", model.ActivityFilter{DealID: "nope"}, "", []string{}},
		{"unknown filter", model.ActivityFilter{AssigneeID: unknownID}, "", []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := list(tt.filter, model.ListParams{Search: tt.search}); !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}

	var paged []string
	for offset := 0; ; offset += 4 {
		page, err := b.s.ListActivities(ctx, b.w.ID, model.ActivityFilter{}, model.ListParams{Limit: 4, Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		paged = append(paged, activitySubjects(page)...)
		if !page.HasMore {
			break
		}
	}
	if !slices.Equal(paged, all) {
		t.Errorf("pages = %q, want %q", paged, all)
	}
}

func TestActivityStoreFiltersWithoutRLS(t *testing.T) {
	tn := newTenants(t)
	ctx := context.Background()
	rls := New(tn.pool)
	contactB := newContact(t, rls, tn.b.ID, "Contact of B", nil)
	companyB := newCompany(t, rls, tn.b.ID, "Company of B")
	ofB := defaultPipeline(t, rls, tn.b.ID)
	dealB, err := rls.CreateDeal(ctx, tn.b.ID, model.DealInput{PipelineID: ofB.ID, Title: "Deal of B", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	ofB2 := newActivity(t, rls, tn.b.ID, model.ActivityInput{Kind: model.ActivityTask, Subject: "Secret", AuthorID: tn.ownerB.ID, ContactID: &contactB.ID})
	contactA := newContact(t, rls, tn.a.ID, "Contact of A", nil)
	s := New(ownerPool(t)) // A superuser: only the store's own filters apply.

	if _, err := s.GetActivity(ctx, tn.a.ID, ofB2.ID); !errors.Is(err, ErrActivityNotFound) {
		t.Errorf("get = %v", err)
	}
	if _, err := s.UpdateActivity(ctx, tn.a.ID, tn.ownerA.ID, ofB2.ID, model.ActivityPatch{Subject: strPtr("pwned")}); !errors.Is(err, ErrActivityNotFound) {
		t.Errorf("update = %v", err)
	}
	if err := s.DeleteActivity(ctx, tn.a.ID, tn.ownerA.ID, ofB2.ID); !errors.Is(err, ErrActivityNotFound) {
		t.Errorf("delete = %v", err)
	}
	if _, err := s.SetActivityCompleted(ctx, tn.a.ID, ofB2.ID, true); !errors.Is(err, ErrActivityNotFound) {
		t.Errorf("complete = %v", err)
	}
	for name, in := range map[string]struct {
		in   model.ActivityInput
		want error
	}{
		"B's contact":  {model.ActivityInput{ContactID: &contactB.ID}, ErrContactNotFound},
		"B's company":  {model.ActivityInput{CompanyID: &companyB.ID}, ErrCompanyNotFound},
		"B's deal":     {model.ActivityInput{DealID: &dealB.ID}, ErrDealNotFound},
		"B's owner":    {model.ActivityInput{ContactID: &contactA.ID, AssigneeID: &tn.ownerB.ID}, ErrMemberNotFound},
		"as B's owner": {model.ActivityInput{ContactID: &contactA.ID, AuthorID: tn.ownerB.ID}, ErrWorkspaceNotFound},
	} {
		in.in.Kind, in.in.Subject = model.ActivityTask, "rogue"
		if in.in.AuthorID == "" {
			in.in.AuthorID = tn.ownerA.ID
		}
		if _, err := s.CreateActivity(ctx, tn.a.ID, in.in); !errors.Is(err, in.want) {
			t.Errorf("link %s = %v, want %v", name, err, in.want)
		}
	}
	page, err := s.ListActivities(ctx, tn.a.ID, model.ActivityFilter{}, model.ListParams{Search: "Secret"})
	if err != nil || len(page.Items) != 0 {
		t.Errorf("list = %+v, %v", page, err)
	}
	if got, err := rls.GetActivity(ctx, tn.b.ID, ofB2.ID); err != nil || got.Subject != "Secret" || got.CompletedAt != nil {
		t.Errorf("B's activity after the attempts = %+v, %v", got, err)
	}
}

func TestActivityRLSIsolation(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	contactA := newContact(t, s, tn.a.ID, "Contact of A", nil)
	contactB := newContact(t, s, tn.b.ID, "Contact of B", nil)
	ofA := newActivity(t, s, tn.a.ID, model.ActivityInput{Kind: model.ActivityNote, Subject: "A", AuthorID: tn.ownerA.ID, ContactID: &contactA.ID})
	ofB := newActivity(t, s, tn.b.ID, model.ActivityInput{Kind: model.ActivityNote, Subject: "B", AuthorID: tn.ownerB.ID, ContactID: &contactB.ID})

	rawTx(t, tn.pool, "app.workspace_id", tn.a.ID, func(ctx context.Context, tx pgx.Tx) {
		if got := strs(t, ctx, tx, `SELECT DISTINCT workspace_id::text FROM activities`); !slices.Equal(got, []string{tn.a.ID}) {
			t.Errorf("workspaces visible %v, want only A", got)
		}
	})
	for _, c := range []struct{ name, setting, value string }{
		{"no setting", "", ""},
		{"empty setting", "app.workspace_id", ""},
		{"user setting grants nothing", "app.user_id", tn.ownerA.ID},
	} {
		t.Run(c.name, func(t *testing.T) {
			rawTx(t, tn.pool, c.setting, c.value, func(ctx context.Context, tx pgx.Tx) {
				var count int
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM activities`).Scan(&count); err != nil {
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
		_, err := tx.Exec(ctx, `INSERT INTO activities (workspace_id, kind, subject, contact_id) VALUES ($1, 'note', 'rogue', $2)`, tn.b.ID, contactB.ID)
		wantDenied(t, "insert", err)
	})
	writes("move a row from A to B", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `UPDATE activities SET workspace_id = $1 WHERE id = $2`, tn.b.ID, ofA.ID)
		wantDenied(t, "move", err)
	})
	writes("update a row of B", func(ctx context.Context, tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `UPDATE activities SET subject = 'pwned' WHERE id = $1`, ofB.ID)
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("update = %d rows, %v; want 0 rows", tag.RowsAffected(), err)
		}
	})
	writes("hard delete is not granted", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `DELETE FROM activities WHERE id = $1`, ofA.ID)
		wantDenied(t, "delete", err)
	})
}

// A foreign key check bypasses Row-Level Security, so these run as the superuser, with
// no policy in the way at all: only the composite keys and checks can refuse them.
func TestActivitySchemaRefusesCrossWorkspaceLinks(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	ctx := context.Background()
	contactA := newContact(t, s, tn.a.ID, "Contact of A", nil)
	contactB := newContact(t, s, tn.b.ID, "Contact of B", nil)
	companyB := newCompany(t, s, tn.b.ID, "Company of B")
	dealB, err := s.CreateDeal(ctx, tn.b.ID, model.DealInput{PipelineID: defaultPipeline(t, s, tn.b.ID).ID, Title: "Deal of B", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	owner := ownerPool(t)
	insert := func(name, code, sql string, args ...any) {
		t.Run(name, func(t *testing.T) {
			tx, err := owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			_, err = tx.Exec(ctx, sql, args...)
			wantPgCode(t, name, err, code)
		})
	}
	const into = `INSERT INTO activities (workspace_id, kind, subject, contact_id, company_id, deal_id, author_id, assignee_id) VALUES ($1, $2, 'x', $3, $4, $5, $6, $7)`
	insert("B's contact", pgFKViolation, into, tn.a.ID, "note", contactB.ID, nil, nil, nil, nil)
	insert("B's company", pgFKViolation, into, tn.a.ID, "note", contactA.ID, companyB.ID, nil, nil, nil)
	insert("B's deal", pgFKViolation, into, tn.a.ID, "note", contactA.ID, nil, dealB.ID, nil, nil)
	insert("B's member as author", pgFKViolation, into, tn.a.ID, "note", contactA.ID, nil, nil, tn.ownerB.ID, nil)
	insert("B's member as assignee", pgFKViolation, into, tn.a.ID, "task", contactA.ID, nil, nil, nil, tn.ownerB.ID)
	insert("no link", pgCheckViolation, into, tn.a.ID, "task", nil, nil, nil, nil, nil)
	insert("an assignee on a note", pgCheckViolation, into, tn.a.ID, "note", contactA.ID, nil, nil, nil, tn.ownerA.ID)
	insert("an unknown kind", pgCheckViolation, into, tn.a.ID, "email", contactA.ID, nil, nil, nil, nil)
	insert("a due time on a call", pgCheckViolation,
		`INSERT INTO activities (workspace_id, kind, subject, contact_id, due_at) VALUES ($1, 'call', 'x', $2, now())`, tn.a.ID, contactA.ID)
	insert("a completed meeting", pgCheckViolation,
		`INSERT INTO activities (workspace_id, kind, subject, contact_id, completed_at) VALUES ($1, 'meeting', 'x', $2, now())`, tn.a.ID, contactA.ID)
}
