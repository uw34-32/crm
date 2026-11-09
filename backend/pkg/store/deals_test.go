package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"crm/pkg/model"
	"crm/pkg/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// board is a pipeline with one workspace around it, for deal tests.
type board struct {
	s     *Store
	owner model.User
	w     model.Workspace
	p     model.Pipeline
	open  model.PipelineStage
	next  model.PipelineStage
	won   model.PipelineStage
	lost  model.PipelineStage
}

func newBoard(t *testing.T) board {
	t.Helper()
	s := New(testdb.AppPool(t))
	return newBoardOn(t, s)
}

func newBoardOn(t *testing.T, s *Store) board {
	t.Helper()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	p := defaultPipeline(t, s, w.ID)
	return board{s: s, owner: owner, w: w, p: p, open: p.Stages[0], next: p.Stages[1],
		won: stageOfKind(t, p, model.StageWon), lost: stageOfKind(t, p, model.StageLost)}
}

func (b board) deal(t *testing.T, title string) model.Deal {
	t.Helper()
	d, err := b.s.CreateDeal(context.Background(), b.w.ID, model.DealInput{
		PipelineID: b.p.ID, Title: title, Currency: "USD", OwnerID: &b.owner.ID,
	})
	if err != nil {
		t.Fatalf("CreateDeal(%q): %v", title, err)
	}
	return d
}

func (b board) move(t *testing.T, dealID, stageID string, position *int, reason string) model.Deal {
	t.Helper()
	d, err := b.s.MoveDeal(context.Background(), b.w.ID, dealID, model.DealMove{StageID: stageID, Position: position, LostReason: reason})
	if err != nil {
		t.Fatalf("MoveDeal: %v", err)
	}
	return d
}

// stageDeals reads a stage's deals in board order and fails unless their positions
// run 0, 1, 2, ... with no gap and no duplicate.
func (b board) stageDeals(t *testing.T, stageID string) []string {
	t.Helper()
	page, err := b.s.ListDeals(context.Background(), b.w.ID, model.DealFilter{StageID: stageID}, model.ListParams{Limit: model.MaxPageSize})
	if err != nil {
		t.Fatalf("ListDeals: %v", err)
	}
	titles := make([]string, len(page.Items))
	for i, d := range page.Items {
		titles[i] = d.Title
		if d.Position != i {
			t.Errorf("deal %q at index %d has position %d", d.Title, i, d.Position)
		}
	}
	return titles
}

func intPtr(i int) *int { return &i }

func TestDealLifecycle(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	company := newCompany(t, b.s, b.w.ID, "Analytical Engines")
	contact, err := b.s.CreateContact(ctx, b.w.ID, model.ContactInput{FirstName: "Ada", LastName: "Lovelace"})
	if err != nil {
		t.Fatal(err)
	}
	date := "2027-03-14"

	created, err := b.s.CreateDeal(ctx, b.w.ID, model.DealInput{
		PipelineID: b.p.ID, Title: "Big one", ValueMinor: 1234567, Currency: "EUR", ExpectedCloseDate: &date,
		CompanyID: &company.ID, ContactID: &contact.ID, OwnerID: &b.owner.ID,
	})
	if err != nil {
		t.Fatalf("CreateDeal: %v", err)
	}
	// With no stage named, a deal starts in the first open stage.
	if created.ID == "" || created.WorkspaceID != b.w.ID || created.PipelineID != b.p.ID || created.StageID != b.open.ID ||
		created.Status != model.StageOpen || created.Title != "Big one" || created.ValueMinor != 1234567 || created.Currency != "EUR" ||
		created.Position != 0 || created.ClosedAt != nil || created.LostReason != "" {
		t.Errorf("created = %+v", created)
	}
	if created.ExpectedCloseDate == nil || *created.ExpectedCloseDate != date {
		t.Errorf("expected close date = %v", created.ExpectedCloseDate)
	}
	if created.CompanyID == nil || *created.CompanyID != company.ID || created.CompanyName == nil || *created.CompanyName != company.Name {
		t.Errorf("company link = %v, %v", created.CompanyID, created.CompanyName)
	}
	if created.ContactID == nil || *created.ContactID != contact.ID || created.ContactName == nil || *created.ContactName != "Ada Lovelace" {
		t.Errorf("contact link = %v, %v", created.ContactID, created.ContactName)
	}
	if created.OwnerID == nil || *created.OwnerID != b.owner.ID {
		t.Errorf("owner = %v", created.OwnerID)
	}

	got, err := b.s.GetDeal(ctx, b.w.ID, created.ID)
	if err != nil || got.ID != created.ID || got.CompanyName == nil {
		t.Errorf("GetDeal = %+v, %v", got, err)
	}

	// A patch changes only the fields it names.
	value := int64(99)
	updated, err := b.s.UpdateDeal(ctx, b.w.ID, created.ID, model.DealPatch{Title: strPtr("Bigger"), ValueMinor: &value})
	if err != nil {
		t.Fatalf("UpdateDeal: %v", err)
	}
	if updated.Title != "Bigger" || updated.ValueMinor != 99 || updated.Currency != "EUR" || updated.CompanyID == nil ||
		updated.ContactID == nil || updated.OwnerID == nil || updated.ExpectedCloseDate == nil {
		t.Errorf("updated = %+v", updated)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("updated_at did not advance: %v then %v", created.UpdatedAt, updated.UpdatedAt)
	}

	// Setting a reference or the date to nil clears it.
	cleared, err := b.s.UpdateDeal(ctx, b.w.ID, created.ID, model.DealPatch{SetCompany: true, SetContact: true, SetOwner: true, SetExpectedClose: true})
	if err != nil || cleared.CompanyID != nil || cleared.CompanyName != nil || cleared.ContactID != nil || cleared.ContactName != nil ||
		cleared.OwnerID != nil || cleared.ExpectedCloseDate != nil {
		t.Errorf("cleared = %+v, %v", cleared, err)
	}
	relinked, err := b.s.UpdateDeal(ctx, b.w.ID, created.ID, model.DealPatch{SetCompany: true, CompanyID: &company.ID, SetOwner: true, OwnerID: &b.owner.ID})
	if err != nil || relinked.CompanyID == nil || relinked.OwnerID == nil {
		t.Errorf("relinked = %+v, %v", relinked, err)
	}

	second := b.deal(t, "Second")
	third := b.deal(t, "Third")
	if second.Position != 1 || third.Position != 2 {
		t.Errorf("positions = %d, %d", second.Position, third.Position)
	}

	// Deleting closes the gap.
	if err := b.s.DeleteDeal(ctx, b.w.ID, created.ID); err != nil {
		t.Fatalf("DeleteDeal: %v", err)
	}
	if got := b.stageDeals(t, b.open.ID); !slices.Equal(got, []string{"Second", "Third"}) {
		t.Errorf("after delete: %q", got)
	}
	if _, err := b.s.GetDeal(ctx, b.w.ID, created.ID); !errors.Is(err, ErrDealNotFound) {
		t.Errorf("get after delete = %v", err)
	}
	if _, err := b.s.UpdateDeal(ctx, b.w.ID, created.ID, model.DealPatch{Title: strPtr("Zombie")}); !errors.Is(err, ErrDealNotFound) {
		t.Errorf("update after delete = %v", err)
	}
	if _, err := b.s.MoveDeal(ctx, b.w.ID, created.ID, model.DealMove{StageID: b.next.ID}); !errors.Is(err, ErrDealNotFound) {
		t.Errorf("move after delete = %v", err)
	}
	if err := b.s.DeleteDeal(ctx, b.w.ID, created.ID); !errors.Is(err, ErrDealNotFound) {
		t.Errorf("second delete = %v", err)
	}
	var kept int
	if err := ownerPool(t).QueryRow(ctx, `SELECT count(*) FROM deals WHERE id = $1 AND deleted_at IS NOT NULL AND position IS NULL`, created.ID).Scan(&kept); err != nil || kept != 1 {
		t.Errorf("soft-deleted rows = %d, %v", kept, err)
	}
}

func TestDealMalformedAndUnknownIDsAreNotFound(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	for _, id := range []string{unknownID, "not-a-uuid", ""} {
		if _, err := b.s.GetDeal(ctx, b.w.ID, id); !errors.Is(err, ErrDealNotFound) {
			t.Errorf("GetDeal(%q) = %v", id, err)
		}
		if _, err := b.s.UpdateDeal(ctx, b.w.ID, id, model.DealPatch{Title: strPtr("x")}); !errors.Is(err, ErrDealNotFound) {
			t.Errorf("UpdateDeal(%q) = %v", id, err)
		}
		if _, err := b.s.MoveDeal(ctx, b.w.ID, id, model.DealMove{StageID: b.next.ID}); !errors.Is(err, ErrDealNotFound) {
			t.Errorf("MoveDeal(%q) = %v", id, err)
		}
		if err := b.s.DeleteDeal(ctx, b.w.ID, id); !errors.Is(err, ErrDealNotFound) {
			t.Errorf("DeleteDeal(%q) = %v", id, err)
		}
	}
}

func TestCreateDealRefusesUnavailableReferences(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	ctx := context.Background()
	p := defaultPipeline(t, s, tn.a.ID)
	otherPipeline := newPipeline(t, s, tn.a.ID, tn.ownerA.ID, "Other")
	ofB := defaultPipeline(t, s, tn.b.ID)
	companyOfB := newCompany(t, s, tn.b.ID, "Company of B")
	contactOfB := newContact(t, s, tn.b.ID, "Contact of B", nil)
	deletedCompany := newCompany(t, s, tn.a.ID, "Deleted")
	if err := s.DeleteCompany(ctx, tn.a.ID, deletedCompany.ID); err != nil {
		t.Fatal(err)
	}
	deletedContact := newContact(t, s, tn.a.ID, "Deleted", nil)
	if err := s.DeleteContact(ctx, tn.a.ID, deletedContact.ID); err != nil {
		t.Fatal(err)
	}
	gonePipeline := newPipeline(t, s, tn.a.ID, tn.ownerA.ID, "Gone")
	if err := s.DeletePipeline(ctx, tn.a.ID, tn.ownerA.ID, gonePipeline.ID); err != nil {
		t.Fatal(err)
	}
	spareStage := newStage(t, s, tn.a.ID, tn.ownerA.ID, p.ID, "Spare", model.StageOpen)
	if err := s.DeleteStage(ctx, tn.a.ID, tn.ownerA.ID, p.ID, spareStage.ID); err != nil {
		t.Fatal(err)
	}
	won := stageOfKind(t, p, model.StageWon)
	lost := stageOfKind(t, p, model.StageLost)

	for _, tt := range []struct {
		name string
		in   model.DealInput
		want error
	}{
		{"pipeline of another workspace", model.DealInput{PipelineID: ofB.ID}, ErrPipelineNotFound},
		{"soft-deleted pipeline", model.DealInput{PipelineID: gonePipeline.ID}, ErrPipelineNotFound},
		{"unknown pipeline", model.DealInput{PipelineID: unknownID}, ErrPipelineNotFound},
		{"malformed pipeline", model.DealInput{PipelineID: "not-a-uuid"}, ErrPipelineNotFound},
		{"stage of another pipeline", model.DealInput{PipelineID: p.ID, StageID: &otherPipeline.Stages[0].ID}, ErrStageNotFound},
		{"stage of another workspace", model.DealInput{PipelineID: p.ID, StageID: &ofB.Stages[0].ID}, ErrStageNotFound},
		{"soft-deleted stage", model.DealInput{PipelineID: p.ID, StageID: &spareStage.ID}, ErrStageNotFound},
		{"malformed stage", model.DealInput{PipelineID: p.ID, StageID: strPtr("nope")}, ErrStageNotFound},
		{"won stage", model.DealInput{PipelineID: p.ID, StageID: &won.ID}, ErrStageNotOpen},
		{"lost stage", model.DealInput{PipelineID: p.ID, StageID: &lost.ID}, ErrStageNotOpen},
		{"company of another workspace", model.DealInput{PipelineID: p.ID, CompanyID: &companyOfB.ID}, ErrCompanyNotFound},
		{"soft-deleted company", model.DealInput{PipelineID: p.ID, CompanyID: &deletedCompany.ID}, ErrCompanyNotFound},
		{"malformed company", model.DealInput{PipelineID: p.ID, CompanyID: strPtr("nope")}, ErrCompanyNotFound},
		{"contact of another workspace", model.DealInput{PipelineID: p.ID, ContactID: &contactOfB.ID}, ErrContactNotFound},
		{"soft-deleted contact", model.DealInput{PipelineID: p.ID, ContactID: &deletedContact.ID}, ErrContactNotFound},
		{"malformed contact", model.DealInput{PipelineID: p.ID, ContactID: strPtr("nope")}, ErrContactNotFound},
		{"owner of another workspace", model.DealInput{PipelineID: p.ID, OwnerID: &tn.ownerB.ID}, ErrMemberNotFound},
		{"owner with no membership", model.DealInput{PipelineID: p.ID, OwnerID: &tn.spare.ID}, ErrMemberNotFound},
		{"malformed owner", model.DealInput{PipelineID: p.ID, OwnerID: strPtr("nope")}, ErrMemberNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Title, tt.in.Currency = "Refused", "USD"
			if _, err := s.CreateDeal(ctx, tn.a.ID, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("create = %v, want %v", err, tt.want)
			}
		})
	}
	page, err := s.ListDeals(ctx, tn.a.ID, model.DealFilter{}, model.ListParams{Search: "Refused"})
	if err != nil || len(page.Items) != 0 {
		t.Errorf("a refused create left a row: %+v, %v", page, err)
	}

	// An update refuses the same references, and changes nothing.
	d, err := s.CreateDeal(ctx, tn.a.ID, model.DealInput{PipelineID: p.ID, Title: "Patched", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name  string
		patch model.DealPatch
		want  error
	}{
		{"company of another workspace", model.DealPatch{SetCompany: true, CompanyID: &companyOfB.ID}, ErrCompanyNotFound},
		{"soft-deleted company", model.DealPatch{SetCompany: true, CompanyID: &deletedCompany.ID}, ErrCompanyNotFound},
		{"contact of another workspace", model.DealPatch{SetContact: true, ContactID: &contactOfB.ID}, ErrContactNotFound},
		{"soft-deleted contact", model.DealPatch{SetContact: true, ContactID: &deletedContact.ID}, ErrContactNotFound},
		{"owner of another workspace", model.DealPatch{SetOwner: true, OwnerID: &tn.ownerB.ID}, ErrMemberNotFound},
		{"owner with no membership", model.DealPatch{SetOwner: true, OwnerID: &tn.spare.ID}, ErrMemberNotFound},
	} {
		t.Run("update: "+tt.name, func(t *testing.T) {
			tt.patch.Title = strPtr("Changed")
			if _, err := s.UpdateDeal(ctx, tn.a.ID, d.ID, tt.patch); !errors.Is(err, tt.want) {
				t.Errorf("update = %v, want %v", err, tt.want)
			}
		})
	}
	if got, err := s.GetDeal(ctx, tn.a.ID, d.ID); err != nil || got.Title != "Patched" {
		t.Errorf("deal after refused updates = %+v, %v", got, err)
	}
}

func TestListDealsFilters(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	s := b.s
	second := addMember(t, s, b.w, b.owner, model.RoleMember)
	other := newPipeline(t, s, b.w.ID, b.owner.ID, "Other")
	acme, err := s.CreateCompany(ctx, b.w.ID, model.CompanyInput{Name: "Acme 100%"})
	if err != nil {
		t.Fatal(err)
	}
	ada, err := s.CreateContact(ctx, b.w.ID, model.ContactInput{FirstName: "Ada", LastName: "Lovelace"})
	if err != nil {
		t.Fatal(err)
	}

	b.deal(t, "Alpha")
	beta, err := s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Beta", Currency: "USD", OwnerID: &second.ID, CompanyID: &acme.ID})
	if err != nil {
		t.Fatal(err)
	}
	gamma, err := s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, Title: "Gamma_x", Currency: "USD", ContactID: &ada.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: other.ID, Title: "Delta", Currency: "USD"}); err != nil {
		t.Fatal(err)
	}
	gone := b.deal(t, "Gone")
	if err := s.DeleteDeal(ctx, b.w.ID, gone.ID); err != nil {
		t.Fatal(err)
	}
	b.move(t, beta.ID, b.won.ID, nil, "")
	b.move(t, gamma.ID, b.lost.ID, nil, "Too expensive")

	for _, tt := range []struct {
		name   string
		filter model.DealFilter
		q      string
		want   []string
	}{
		{"pipeline", model.DealFilter{PipelineID: b.p.ID}, "", []string{"Alpha", "Beta", "Gamma_x"}},
		{"other pipeline", model.DealFilter{PipelineID: other.ID}, "", []string{"Delta"}},
		{"stage", model.DealFilter{StageID: b.won.ID}, "", []string{"Beta"}},
		{"owner", model.DealFilter{OwnerID: second.ID}, "", []string{"Beta"}},
		{"status open", model.DealFilter{PipelineID: b.p.ID, Status: model.StageOpen}, "", []string{"Alpha"}},
		{"status won", model.DealFilter{Status: model.StageWon}, "", []string{"Beta"}},
		{"status lost", model.DealFilter{Status: model.StageLost}, "", []string{"Gamma_x"}},
		{"combined that match", model.DealFilter{PipelineID: b.p.ID, StageID: b.won.ID, OwnerID: second.ID, Status: model.StageWon}, "beta", []string{"Beta"}},
		{"combined that do not", model.DealFilter{PipelineID: other.ID, OwnerID: second.ID}, "", []string{}},
		{"search a title", model.DealFilter{}, "ALPHA", []string{"Alpha"}},
		{"search a company", model.DealFilter{}, "acme", []string{"Beta"}},
		{"search a contact", model.DealFilter{}, "ada love", []string{"Gamma_x"}},
		{"underscore is literal", model.DealFilter{}, "a_x", []string{"Gamma_x"}},
		{"percent is literal", model.DealFilter{}, "%", []string{"Beta"}},
		{"a deleted deal does not show", model.DealFilter{}, "Gone", []string{}},
		{"unknown pipeline", model.DealFilter{PipelineID: unknownID}, "", []string{}},
		{"malformed pipeline", model.DealFilter{PipelineID: "nope"}, "", []string{}},
		{"malformed stage", model.DealFilter{StageID: "nope"}, "", []string{}},
		{"malformed owner", model.DealFilter{OwnerID: "nope"}, "", []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			page, err := s.ListDeals(ctx, b.w.ID, tt.filter, model.ListParams{Search: tt.q})
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(page.Items))
			for i, d := range page.Items {
				got[i] = d.Title
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("list = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListDealsPaginationIsBoundedAndStable(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	var want []string
	for i := range 5 {
		want = append(want, b.deal(t, fmt.Sprintf("Deal %d", i)).ID)
	}
	first, err := b.s.ListDeals(ctx, b.w.ID, model.DealFilter{}, model.ListParams{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.s.ListDeals(ctx, b.w.ID, model.DealFilter{}, model.ListParams{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || second.HasMore {
		t.Errorf("has_more = %v then %v", first.HasMore, second.HasMore)
	}
	var got []string
	for _, d := range append(first.Items, second.Items...) {
		got = append(got, d.ID)
	}
	if !slices.Equal(got, want) {
		t.Errorf("order = %v, want board order %v", got, want)
	}
	capped, err := b.s.ListDeals(ctx, b.w.ID, model.DealFilter{}, model.ListParams{Limit: 1 << 40, Offset: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	if capped.Limit != model.MaxPageSize || capped.Offset != model.MaxPageOffset || capped.Items == nil {
		t.Errorf("capped = %+v", capped)
	}
}

func TestMoveDealClosesAndReopens(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	d := b.deal(t, "Mover")

	moved := b.move(t, d.ID, b.next.ID, nil, "")
	if moved.StageID != b.next.ID || moved.Status != model.StageOpen || moved.ClosedAt != nil || moved.LostReason != "" {
		t.Errorf("open to open = %+v", moved)
	}
	if !moved.UpdatedAt.After(d.UpdatedAt) {
		t.Error("a move did not advance updated_at")
	}

	won := b.move(t, d.ID, b.won.ID, nil, "")
	if won.Status != model.StageWon || won.ClosedAt == nil || won.LostReason != "" {
		t.Errorf("into won = %+v", won)
	}

	// Into lost needs a reason, and a refused move changes nothing.
	if _, err := b.s.MoveDeal(ctx, b.w.ID, d.ID, model.DealMove{StageID: b.lost.ID}); !errors.Is(err, ErrLostReasonRequired) {
		t.Errorf("into lost without a reason = %v", err)
	}
	if got, err := b.s.GetDeal(ctx, b.w.ID, d.ID); err != nil || got.Status != model.StageWon || got.LostReason != "" {
		t.Errorf("after a refused move = %+v, %v", got, err)
	}
	lost := b.move(t, d.ID, b.lost.ID, nil, "Budget cut")
	if lost.Status != model.StageLost || lost.ClosedAt == nil || lost.LostReason != "Budget cut" {
		t.Errorf("into lost = %+v", lost)
	}
	if !lost.ClosedAt.After(*won.ClosedAt) {
		t.Error("changing closed stage did not record a new closing time")
	}

	// A reason makes sense only for a lost stage.
	if _, err := b.s.MoveDeal(ctx, b.w.ID, d.ID, model.DealMove{StageID: b.open.ID, LostReason: "Nonsense"}); !errors.Is(err, ErrLostReasonNotAllowed) {
		t.Errorf("reason on open = %v", err)
	}
	if _, err := b.s.MoveDeal(ctx, b.w.ID, d.ID, model.DealMove{StageID: b.won.ID, LostReason: "Nonsense"}); !errors.Is(err, ErrLostReasonNotAllowed) {
		t.Errorf("reason on won = %v", err)
	}

	// Moving between two lost stages is still moving into a lost stage.
	lost2 := newStage(t, b.s, b.w.ID, b.owner.ID, b.p.ID, "Lost to a rival", model.StageLost)
	if _, err := b.s.MoveDeal(ctx, b.w.ID, d.ID, model.DealMove{StageID: lost2.ID}); !errors.Is(err, ErrLostReasonRequired) {
		t.Errorf("between lost stages without a reason = %v", err)
	}

	// Reordering inside a closed stage is not closing again: it keeps the time, and
	// the reason unless a new one is given.
	if got := b.move(t, d.ID, b.lost.ID, intPtr(0), ""); !got.ClosedAt.Equal(*lost.ClosedAt) || got.LostReason != "Budget cut" {
		t.Errorf("reorder inside lost = %+v", got)
	}
	if got := b.move(t, d.ID, b.lost.ID, intPtr(0), "Rethought"); !got.ClosedAt.Equal(*lost.ClosedAt) || got.LostReason != "Rethought" {
		t.Errorf("new reason inside lost = %+v", got)
	}

	reopened := b.move(t, d.ID, b.open.ID, nil, "")
	if reopened.Status != model.StageOpen || reopened.ClosedAt != nil || reopened.LostReason != "" {
		t.Errorf("back to open = %+v", reopened)
	}

	// Won after lost clears the reason.
	b.move(t, d.ID, b.lost.ID, nil, "Again")
	if got := b.move(t, d.ID, b.won.ID, nil, ""); got.LostReason != "" || got.ClosedAt == nil {
		t.Errorf("lost to won = %+v", got)
	}
	if got, err := b.s.GetDeal(ctx, b.w.ID, d.ID); err != nil || got.Status != model.StageWon {
		t.Errorf("read back = %+v, %v", got, err)
	}
}

func TestMoveDealRefusesAStageOutOfReach(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	ctx := context.Background()
	p := defaultPipeline(t, s, tn.a.ID)
	other := newPipeline(t, s, tn.a.ID, tn.ownerA.ID, "Other")
	ofB := defaultPipeline(t, s, tn.b.ID)
	spare := newStage(t, s, tn.a.ID, tn.ownerA.ID, p.ID, "Spare", model.StageOpen)
	if err := s.DeleteStage(ctx, tn.a.ID, tn.ownerA.ID, p.ID, spare.ID); err != nil {
		t.Fatal(err)
	}
	d, err := s.CreateDeal(ctx, tn.a.ID, model.DealInput{PipelineID: p.ID, Title: "Stays", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	for name, stageID := range map[string]string{
		"another pipeline":  other.Stages[0].ID,
		"another workspace": ofB.Stages[0].ID,
		"soft-deleted":      spare.ID,
		"unknown":           unknownID,
		"malformed":         "not-a-uuid",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := s.MoveDeal(ctx, tn.a.ID, d.ID, model.DealMove{StageID: stageID}); !errors.Is(err, ErrStageNotFound) {
				t.Errorf("move = %v", err)
			}
		})
	}
	if got, err := s.GetDeal(ctx, tn.a.ID, d.ID); err != nil || got.StageID != d.StageID || got.Position != 0 {
		t.Errorf("after the attempts = %+v, %v", got, err)
	}
}

func TestMoveDealPositions(t *testing.T) {
	b := newBoard(t)
	deals := map[string]model.Deal{}
	for _, title := range []string{"A", "B", "C", "D"} {
		deals[title] = b.deal(t, title)
	}
	target, err := b.s.CreateDeal(context.Background(), b.w.ID, model.DealInput{PipelineID: b.p.ID, StageID: &b.next.ID, Title: "X", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	want := func(stageID string, titles ...string) {
		t.Helper()
		if got := b.stageDeals(t, stageID); !slices.Equal(got, titles) {
			t.Errorf("stage order = %q, want %q", got, titles)
		}
	}
	want(b.open.ID, "A", "B", "C", "D")

	// Within a stage, both ways.
	if got := b.move(t, deals["A"].ID, b.open.ID, intPtr(2), ""); got.Position != 2 {
		t.Errorf("position = %d", got.Position)
	}
	want(b.open.ID, "B", "C", "A", "D")
	b.move(t, deals["D"].ID, b.open.ID, intPtr(0), "")
	want(b.open.ID, "D", "B", "C", "A")
	// Staying put is a no-op on the order.
	b.move(t, deals["B"].ID, b.open.ID, intPtr(1), "")
	want(b.open.ID, "D", "B", "C", "A")
	// Beyond the end means the last place, and no position means the end too.
	if got := b.move(t, deals["D"].ID, b.open.ID, intPtr(1000), ""); got.Position != 3 {
		t.Errorf("clamped position = %d", got.Position)
	}
	want(b.open.ID, "B", "C", "A", "D")
	b.move(t, deals["B"].ID, b.open.ID, nil, "")
	want(b.open.ID, "C", "A", "D", "B")

	// Across stages: the source closes its gap, and the target makes room.
	if got := b.move(t, deals["A"].ID, b.next.ID, intPtr(0), ""); got.Position != 0 {
		t.Errorf("position = %d", got.Position)
	}
	want(b.open.ID, "C", "D", "B")
	want(b.next.ID, "A", "X")
	b.move(t, deals["C"].ID, b.next.ID, nil, "")
	want(b.open.ID, "D", "B")
	want(b.next.ID, "A", "X", "C")
	if got := b.move(t, deals["D"].ID, b.next.ID, intPtr(99), ""); got.Position != 3 {
		t.Errorf("clamped position across stages = %d", got.Position)
	}
	want(b.open.ID, "B")
	want(b.next.ID, "A", "X", "C", "D")

	// Moving the last deal out leaves an empty stage, not a broken one.
	b.move(t, deals["B"].ID, b.next.ID, intPtr(2), "")
	want(b.open.ID)
	want(b.next.ID, "A", "X", "B", "C", "D")

	// Shifted neighbours advance updated_at, so a later sync sees the new order.
	before, err := b.s.GetDeal(context.Background(), b.w.ID, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	b.move(t, deals["D"].ID, b.next.ID, intPtr(0), "")
	after, err := b.s.GetDeal(context.Background(), b.w.ID, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Position != before.Position+1 || !after.UpdatedAt.After(before.UpdatedAt) {
		t.Errorf("neighbour went from %+v to %+v", before, after)
	}
}

// raceStore connects as the app role under a unique application name, with enough
// connections for every racer, so the test can count the ones waiting on a lock.
func raceStore(t *testing.T, connections int32) (*Store, *pgxpool.Pool, string) {
	t.Helper()
	testdb.AppPool(t) // Applies migrations.
	appURL, _ := testdb.URLs(t)
	cfg, err := pgxpool.ParseConfig(appURL)
	if err != nil {
		t.Fatal(err)
	}
	name := "race-" + uniqueEmail(t)
	cfg.ConnConfig.RuntimeParams["application_name"] = name
	cfg.MaxConns = connections
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return New(pool), pool, name
}

// holdPipeline takes a pipeline's lock in a transaction of the test's own, and returns
// a function that releases it after the expected number of racers are queued behind
// it. That makes the race deterministic: all of them have read the board before any
// of them can act on it, and they are certain to overlap.
func holdPipeline(t *testing.T, owner *pgxpool.Pool, pipelineID, app string, racers int) (release func()) {
	t.Helper()
	ctx := context.Background()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	if _, err := tx.Exec(ctx, `SELECT 1 FROM pipelines WHERE id = $1 FOR NO KEY UPDATE`, pipelineID); err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		waitQueued(t, owner, app, racers)
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

// Every racer asks for the same position at once. Each must see the board the previous
// one left, so all arrive, none twice, with no gap and no duplicate position.
func TestConcurrentMovesQueueAndKeepPositionsDense(t *testing.T) {
	const racers = 8
	s, _, app := raceStore(t, racers+2)
	b := newBoardOn(t, s)
	ctx := context.Background()
	owner := ownerPool(t)
	titles := make([]string, racers)
	ids := make([]string, racers)
	for i := range racers {
		titles[i] = fmt.Sprintf("Racer %d", i)
		ids[i] = b.deal(t, titles[i]).ID
	}
	settled, err := s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: b.p.ID, StageID: &b.next.ID, Title: "Settled", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}

	release := holdPipeline(t, owner, b.p.ID, app, racers)
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i, id := range ids {
		wg.Go(func() {
			_, errs[i] = s.MoveDeal(ctx, b.w.ID, id, model.DealMove{StageID: b.next.ID, Position: intPtr(0)})
		})
	}
	release()
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("racer %d: %v", i, err)
		}
	}

	if left := b.stageDeals(t, b.open.ID); len(left) != 0 {
		t.Errorf("still in the source stage: %q", left)
	}
	got := b.stageDeals(t, b.next.ID)
	if len(got) != racers+1 || got[racers] != settled.Title {
		t.Errorf("target stage = %q", got)
	}
	slices.Sort(got)
	want := append(slices.Clone(titles), settled.Title)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("target stage holds %q, want %q", got, want)
	}
}

// Moves back and forth in both directions, the pattern that deadlocks or duplicates
// if anything is out of order. No barrier here: the interleaving is left to chance
// over many rounds, to complement the deterministic test above.
func TestConcurrentMovesInBothDirectionsStayConsistent(t *testing.T) {
	const racers = 6
	s, _, _ := raceStore(t, racers+2)
	b := newBoardOn(t, s)
	ctx := context.Background()
	stages := []string{b.open.ID, b.next.ID}
	var ids []string
	for i := range racers {
		ids = append(ids, b.deal(t, fmt.Sprintf("Racer %d", i)).ID)
	}
	for round := range 15 {
		var wg sync.WaitGroup
		errs := make([]error, racers)
		for i, id := range ids {
			wg.Go(func() {
				_, errs[i] = s.MoveDeal(ctx, b.w.ID, id, model.DealMove{StageID: stages[(i+round)%2], Position: intPtr(i % 2)})
			})
		}
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d, racer %d: %v", round, i, err)
			}
		}
		if got := len(b.stageDeals(t, b.open.ID)) + len(b.stageDeals(t, b.next.ID)); got != racers {
			t.Fatalf("round %d: %d deals on the board, want %d", round, got, racers)
		}
	}
}

// Moves of one deal to different stages: the last to run wins, and what it leaves is
// a coherent deal, never a won deal with a lost reason or an open one with a closing
// time.
func TestConcurrentMovesOfOneDealLeaveItCoherent(t *testing.T) {
	const racers = 6
	s, _, app := raceStore(t, racers+2)
	b := newBoardOn(t, s)
	ctx := context.Background()
	owner := ownerPool(t)
	targets := []model.DealMove{
		{StageID: b.won.ID},
		{StageID: b.lost.ID, LostReason: "Reason"},
		{StageID: b.open.ID},
		{StageID: b.next.ID},
		{StageID: b.lost.ID, LostReason: "Other reason"},
		{StageID: b.won.ID},
	}
	for round := range 5 {
		d := b.deal(t, fmt.Sprintf("Contested %d", round))
		release := holdPipeline(t, owner, b.p.ID, app, racers)
		var wg sync.WaitGroup
		errs := make([]error, racers)
		for i, mv := range targets {
			wg.Go(func() { _, errs[i] = s.MoveDeal(ctx, b.w.ID, d.ID, mv) })
		}
		release()
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("racer %d: %v", i, err)
			}
		}
		got, err := s.GetDeal(ctx, b.w.ID, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		switch got.Status {
		case model.StageOpen:
			if got.ClosedAt != nil || got.LostReason != "" {
				t.Errorf("incoherent open deal: %+v", got)
			}
		case model.StageWon:
			if got.ClosedAt == nil || got.LostReason != "" {
				t.Errorf("incoherent won deal: %+v", got)
			}
		case model.StageLost:
			if got.ClosedAt == nil || got.LostReason == "" {
				t.Errorf("incoherent lost deal: %+v", got)
			}
		}
		for _, stage := range []string{b.open.ID, b.next.ID, b.won.ID, b.lost.ID} {
			b.stageDeals(t, stage)
		}
	}
}

// A move waiting for the lock and an edit that needs no lock must both survive: the
// move touches only what a move owns, rather than writing back what it read.
func TestMoveDoesNotLoseAConcurrentEdit(t *testing.T) {
	s, _, app := raceStore(t, 4)
	b := newBoardOn(t, s)
	ctx := context.Background()
	d := b.deal(t, "Before")

	release := holdPipeline(t, ownerPool(t), b.p.ID, app, 1)
	var wg sync.WaitGroup
	var moveErr error
	wg.Go(func() {
		_, moveErr = s.MoveDeal(ctx, b.w.ID, d.ID, model.DealMove{StageID: b.lost.ID, LostReason: "Reason"})
	})
	// Wait until the move is queued, then edit while it waits.
	waitQueued(t, ownerPool(t), app, 1)
	value := int64(500)
	if _, err := s.UpdateDeal(ctx, b.w.ID, d.ID, model.DealPatch{Title: strPtr("After"), ValueMinor: &value}); err != nil {
		t.Fatalf("UpdateDeal: %v", err)
	}
	release()
	wg.Wait()
	if moveErr != nil {
		t.Fatalf("MoveDeal: %v", moveErr)
	}
	got, err := s.GetDeal(ctx, b.w.ID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "After" || got.ValueMinor != 500 || got.Status != model.StageLost || got.LostReason != "Reason" {
		t.Errorf("after both = %+v", got)
	}
}

func waitQueued(t *testing.T, owner *pgxpool.Pool, app string, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := owner.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity WHERE application_name = $1 AND wait_event_type = 'Lock'`, app).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d queued", waiting, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPipelineAndStageDeletionRefuseLiveDeals(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	spare := newPipeline(t, b.s, b.w.ID, b.owner.ID, "Spare")
	d, err := b.s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: spare.ID, Title: "Holder", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	holder := spare.Stages[0]

	if err := b.s.DeletePipeline(ctx, b.w.ID, b.owner.ID, spare.ID); !errors.Is(err, ErrPipelineHasDeals) {
		t.Errorf("delete a pipeline with a live deal = %v", err)
	}
	if err := b.s.DeleteStage(ctx, b.w.ID, b.owner.ID, spare.ID, holder.ID); !errors.Is(err, ErrStageHasDeals) {
		t.Errorf("delete a stage with a live deal = %v", err)
	}
	// Changing a stage's kind would strand its deals in the wrong closed state.
	won := model.StageWon
	if _, err := b.s.UpdateStage(ctx, b.w.ID, b.owner.ID, spare.ID, holder.ID, model.StagePatch{Kind: &won}); !errors.Is(err, ErrStageHasDeals) {
		t.Errorf("change the kind of a stage with a live deal = %v", err)
	}
	// Keeping the kind, or changing anything else, is fine.
	open := model.StageOpen
	if _, err := b.s.UpdateStage(ctx, b.w.ID, b.owner.ID, spare.ID, holder.ID, model.StagePatch{Name: strPtr("Renamed"), Kind: &open}); err != nil {
		t.Errorf("rename a stage with a live deal: %v", err)
	}

	// A soft-deleted deal holds nothing, and keeps a valid reference to what it was in.
	if err := b.s.DeleteDeal(ctx, b.w.ID, d.ID); err != nil {
		t.Fatal(err)
	}
	if err := b.s.DeleteStage(ctx, b.w.ID, b.owner.ID, spare.ID, holder.ID); err != nil {
		t.Errorf("delete the emptied stage: %v", err)
	}
	if err := b.s.DeletePipeline(ctx, b.w.ID, b.owner.ID, spare.ID); err != nil {
		t.Errorf("delete the emptied pipeline: %v", err)
	}
}

// A create or move racing a delete either commits first, and the delete refuses, or
// runs second, and is refused. Neither order may leave a live deal in a deleted place.
func TestDealWritesRacingADeletionLeaveNoStrandedDeal(t *testing.T) {
	b := newBoard(t)
	ctx := context.Background()
	owner := ownerPool(t)
	stranded := func(sql string, args ...any) int {
		t.Helper()
		var n int
		if err := owner.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for range 20 {
		p := newPipeline(t, b.s, b.w.ID, b.owner.ID, "Contested")
		var wg sync.WaitGroup
		var delErr, createErr error
		wg.Go(func() { delErr = b.s.DeletePipeline(ctx, b.w.ID, b.owner.ID, p.ID) })
		wg.Go(func() {
			_, createErr = b.s.CreateDeal(ctx, b.w.ID, model.DealInput{PipelineID: p.ID, Title: "Racer", Currency: "USD"})
		})
		wg.Wait()
		if delErr != nil && !errors.Is(delErr, ErrPipelineHasDeals) {
			t.Errorf("delete: %v", delErr)
		}
		if createErr != nil && !errors.Is(createErr, ErrPipelineNotFound) {
			t.Errorf("create: %v", createErr)
		}
		if n := stranded(`SELECT count(*) FROM deals d JOIN pipelines p ON p.id = d.pipeline_id
			WHERE p.id = $1 AND p.deleted_at IS NOT NULL AND d.deleted_at IS NULL`, p.ID); n != 0 {
			t.Fatalf("%d live deals in a deleted pipeline", n)
		}
	}
	for range 20 {
		stage := newStage(t, b.s, b.w.ID, b.owner.ID, b.p.ID, "Contested", model.StageOpen)
		d := b.deal(t, "Racer")
		var wg sync.WaitGroup
		var delErr, moveErr error
		wg.Go(func() { delErr = b.s.DeleteStage(ctx, b.w.ID, b.owner.ID, b.p.ID, stage.ID) })
		wg.Go(func() { _, moveErr = b.s.MoveDeal(ctx, b.w.ID, d.ID, model.DealMove{StageID: stage.ID}) })
		wg.Wait()
		if delErr != nil && !errors.Is(delErr, ErrStageHasDeals) {
			t.Errorf("delete: %v", delErr)
		}
		if moveErr != nil && !errors.Is(moveErr, ErrStageNotFound) {
			t.Errorf("move: %v", moveErr)
		}
		if n := stranded(`SELECT count(*) FROM deals d JOIN pipeline_stages st ON st.id = d.stage_id
			WHERE st.id = $1 AND st.deleted_at IS NOT NULL AND d.deleted_at IS NULL`, stage.ID); n != 0 {
			t.Fatalf("%d live deals in a deleted stage", n)
		}
		if moveErr == nil {
			if err := b.s.DeleteDeal(ctx, b.w.ID, d.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestDealStoreFiltersWithoutRLS(t *testing.T) {
	tn := newTenants(t)
	ctx := context.Background()
	rls := New(tn.pool)
	ofA, ofB := defaultPipeline(t, rls, tn.a.ID), defaultPipeline(t, rls, tn.b.ID)
	dealOfB, err := rls.CreateDeal(ctx, tn.b.ID, model.DealInput{PipelineID: ofB.ID, Title: "Secret", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rls.CreateDeal(ctx, tn.a.ID, model.DealInput{PipelineID: ofA.ID, Title: "Mine", Currency: "USD"}); err != nil {
		t.Fatal(err)
	}
	s := New(ownerPool(t)) // A superuser: only the store's own filters apply.

	if _, err := s.GetDeal(ctx, tn.a.ID, dealOfB.ID); !errors.Is(err, ErrDealNotFound) {
		t.Errorf("get = %v", err)
	}
	if _, err := s.UpdateDeal(ctx, tn.a.ID, dealOfB.ID, model.DealPatch{Title: strPtr("pwned")}); !errors.Is(err, ErrDealNotFound) {
		t.Errorf("update = %v", err)
	}
	if _, err := s.MoveDeal(ctx, tn.a.ID, dealOfB.ID, model.DealMove{StageID: ofB.Stages[1].ID}); !errors.Is(err, ErrDealNotFound) {
		t.Errorf("move = %v", err)
	}
	if err := s.DeleteDeal(ctx, tn.a.ID, dealOfB.ID); !errors.Is(err, ErrDealNotFound) {
		t.Errorf("delete = %v", err)
	}
	if _, err := s.CreateDeal(ctx, tn.a.ID, model.DealInput{PipelineID: ofB.ID, Title: "x", Currency: "USD"}); !errors.Is(err, ErrPipelineNotFound) {
		t.Errorf("create in B's pipeline = %v", err)
	}
	if page, err := s.ListDeals(ctx, tn.a.ID, model.DealFilter{}, model.ListParams{}); err != nil || len(page.Items) != 1 || page.Items[0].Title != "Mine" {
		t.Errorf("list = %+v, %v", page, err)
	}
	if page, err := s.ListDeals(ctx, tn.a.ID, model.DealFilter{PipelineID: ofB.ID}, model.ListParams{}); err != nil || len(page.Items) != 0 {
		t.Errorf("list by B's pipeline = %+v, %v", page, err)
	}
	survivor, err := rls.GetDeal(ctx, tn.b.ID, dealOfB.ID)
	if err != nil || survivor.Title != "Secret" || survivor.StageID != ofB.Stages[0].ID {
		t.Errorf("B's deal after the attempts = %+v, %v", survivor, err)
	}
}

func TestDealRLSIsolation(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	ctx := context.Background()
	ofA, ofB := defaultPipeline(t, s, tn.a.ID), defaultPipeline(t, s, tn.b.ID)
	inA, err := s.CreateDeal(ctx, tn.a.ID, model.DealInput{PipelineID: ofA.ID, Title: "Deal of A", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	inB, err := s.CreateDeal(ctx, tn.b.ID, model.DealInput{PipelineID: ofB.ID, Title: "Deal of B", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("reads", func(t *testing.T) {
		rawTx(t, tn.pool, "app.workspace_id", tn.a.ID, func(ctx context.Context, tx pgx.Tx) {
			if got := strs(t, ctx, tx, `SELECT DISTINCT workspace_id::text FROM deals`); !slices.Equal(got, []string{tn.a.ID}) {
				t.Errorf("workspaces visible %v, want only A", got)
			}
			if got := strs(t, ctx, tx, `SELECT title FROM deals WHERE id = $1`, inB.ID); len(got) != 0 {
				t.Errorf("B's deal visible by ID: %v", got)
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
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM deals`).Scan(&count); err != nil {
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
		_, err := tx.Exec(ctx, `INSERT INTO deals (workspace_id, pipeline_id, stage_id, title, currency, position) VALUES ($1, $2, $3, 'rogue', 'USD', 99)`,
			tn.b.ID, ofB.ID, ofB.Stages[0].ID)
		wantDenied(t, "insert", err)
	})
	writes("move from A to B", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `UPDATE deals SET workspace_id = $1 WHERE id = $2`, tn.b.ID, inA.ID)
		wantDenied(t, "move", err)
	})
	writes("update B", func(ctx context.Context, tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `UPDATE deals SET title = 'pwned' WHERE id = $1`, inB.ID)
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("update = %d rows, %v; want 0 rows", tag.RowsAffected(), err)
		}
	})
	writes("hard delete is not granted", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `DELETE FROM deals WHERE id = $1`, inA.ID)
		wantDenied(t, "delete", err)
	})
}

// A foreign key check bypasses Row-Level Security, so these run as the superuser, with
// no policy in the way at all, and only the schema can refuse them.
func TestDealSchemaRefusesBadReferences(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	ctx := context.Background()
	ofA, ofB := defaultPipeline(t, s, tn.a.ID), defaultPipeline(t, s, tn.b.ID)
	otherOfA := newPipeline(t, s, tn.a.ID, tn.ownerA.ID, "Other")
	companyOfB := newCompany(t, s, tn.b.ID, "Company of B")
	contactOfB := newContact(t, s, tn.b.ID, "Contact of B", nil)
	d, err := s.CreateDeal(ctx, tn.a.ID, model.DealInput{PipelineID: ofA.ID, Title: "Ok", Currency: "USD", OwnerID: &tn.ownerA.ID})
	if err != nil {
		t.Fatal(err)
	}
	owner := ownerPool(t)
	stage := ofA.Stages[1].ID

	const insert = `INSERT INTO deals (workspace_id, pipeline_id, stage_id, title, currency, position, company_id, contact_id, owner_id)
		VALUES ($1, $2, $3, 'x', 'USD', 99, $4, $5, $6)`
	for _, tt := range []struct {
		name string
		args []any
		code string
	}{
		{"pipeline of another workspace", []any{tn.a.ID, ofB.ID, ofB.Stages[0].ID, nil, nil, nil}, pgFKViolation},
		{"stage of another workspace", []any{tn.a.ID, ofA.ID, ofB.Stages[0].ID, nil, nil, nil}, pgFKViolation},
		{"stage of another pipeline", []any{tn.a.ID, ofA.ID, otherOfA.Stages[0].ID, nil, nil, nil}, pgFKViolation},
		{"company of another workspace", []any{tn.a.ID, ofA.ID, stage, companyOfB.ID, nil, nil}, pgFKViolation},
		{"contact of another workspace", []any{tn.a.ID, ofA.ID, stage, nil, contactOfB.ID, nil}, pgFKViolation},
		{"owner who belongs to another workspace", []any{tn.a.ID, ofA.ID, stage, nil, nil, tn.ownerB.ID}, pgFKViolation},
		{"owner with no membership", []any{tn.a.ID, ofA.ID, stage, nil, nil, tn.spare.ID}, pgFKViolation},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			_, err = tx.Exec(ctx, insert, tt.args...)
			wantPgCode(t, "insert", err, tt.code)
		})
	}
	updates := []struct {
		name string
		sql  string
		args []any
		code string
	}{
		{"to a stage of another pipeline", `UPDATE deals SET stage_id = $1 WHERE id = $2`, []any{otherOfA.Stages[0].ID, d.ID}, pgFKViolation},
		{"to another pipeline, keeping the stage", `UPDATE deals SET pipeline_id = $1 WHERE id = $2`, []any{otherOfA.ID, d.ID}, pgFKViolation},
		{"to another workspace", `UPDATE deals SET workspace_id = $1 WHERE id = $2`, []any{tn.b.ID, d.ID}, pgFKViolation},
		{"a float's worth of value is not representable", `UPDATE deals SET value_minor = 1000000000000000 WHERE id = $1`, []any{d.ID}, "23514"},
		{"a negative value", `UPDATE deals SET value_minor = -1 WHERE id = $1`, []any{d.ID}, "23514"},
		{"a lowercase currency", `UPDATE deals SET currency = 'usd' WHERE id = $1`, []any{d.ID}, "23514"},
		{"a word for a currency", `UPDATE deals SET currency = 'DOLLAR' WHERE id = $1`, []any{d.ID}, "23514"},
		{"a negative position", `UPDATE deals SET position = -1 WHERE id = $1`, []any{d.ID}, "23514"},
		{"a live deal without a position", `UPDATE deals SET position = NULL WHERE id = $1`, []any{d.ID}, "23514"},
		{"a lost reason on a deal that is not closed", `UPDATE deals SET lost_reason = 'x' WHERE id = $1`, []any{d.ID}, "23514"},
		{"a blank title", `UPDATE deals SET title = '' WHERE id = $1`, []any{d.ID}, "23514"},
		{"an absurd expected close date", `UPDATE deals SET expected_close_date = '0001-01-01' WHERE id = $1`, []any{d.ID}, "23514"},
	}
	for _, tt := range updates {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			_, err = tx.Exec(ctx, tt.sql, tt.args...)
			wantPgCode(t, "update", err, tt.code)
		})
	}
	t.Run("two deals at one position", func(t *testing.T) {
		tx, err := owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		_, err = tx.Exec(ctx, `INSERT INTO deals (workspace_id, pipeline_id, stage_id, title, currency, position) VALUES ($1, $2, $3, 'x', 'USD', $4)`,
			tn.a.ID, ofA.ID, d.StageID, d.Position)
		wantPgCode(t, "insert", err, "23505")
	})
}
