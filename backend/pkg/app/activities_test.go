package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

type activityBody struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	Kind        string  `json:"kind"`
	Subject     string  `json:"subject"`
	Body        string  `json:"body"`
	DueAt       *string `json:"due_at"`
	CompletedAt *string `json:"completed_at"`
	AuthorID    *string `json:"author_id"`
	AssigneeID  *string `json:"assignee_id"`
	ContactID   *string `json:"contact_id"`
	ContactName *string `json:"contact_name"`
	CompanyID   *string `json:"company_id"`
	CompanyName *string `json:"company_name"`
	DealID      *string `json:"deal_id"`
	DealTitle   *string `json:"deal_title"`
}

func activitiesPath(workspaceID string) string { return "/workspaces/" + workspaceID + "/activities" }

func postActivity(t *testing.T, h http.Handler, token, workspaceID string, fields map[string]any) activityBody {
	t.Helper()
	body := map[string]any{"kind": "note", "subject": "Activity"}
	for key, value := range fields {
		body[key] = value
	}
	rec := call(t, h, http.MethodPost, activitiesPath(workspaceID), token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create activity = %d: %s", rec.Code, rec.Body)
	}
	return data[activityBody](t, rec)
}

func subjects(page pageBody[activityBody]) []string {
	out := make([]string, len(page.Items))
	for i, a := range page.Items {
		out[i] = a.Subject
	}
	return out
}

func TestActivityRoutes(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	member := joined(t, h, owner.Token, w.ID, "member")
	p := firstPipeline(t, h, owner.Token, w.ID)
	contact := postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Ada", "last_name": "Lovelace"})
	deal := postDeal(t, h, owner.Token, w.ID, p.ID, map[string]any{"title": "Engine"})
	base := activitiesPath(w.ID)

	rec := call(t, h, http.MethodPost, base, member.Token, map[string]any{
		"kind": "task", "subject": "  Send the proposal  ", "body": "With figures", "due_at": "2027-03-14T09:30:00+01:00",
		"assignee_id": owner.User.ID, "contact_id": contact.ID, "deal_id": deal.ID,
	})
	wantStatus(t, rec, http.StatusCreated, "create")
	created := data[activityBody](t, rec)
	if created.Subject != "Send the proposal" || created.Kind != "task" || created.WorkspaceID != w.ID || created.CompletedAt != nil ||
		created.AuthorID == nil || *created.AuthorID != member.User.ID || created.AssigneeID == nil || *created.AssigneeID != owner.User.ID {
		t.Errorf("created = %+v", created)
	}
	if created.DueAt == nil || !strings.HasPrefix(*created.DueAt, "2027-03-14T") {
		t.Errorf("due = %v", created.DueAt)
	}
	if created.ContactName == nil || *created.ContactName != "Ada Lovelace" || created.DealTitle == nil || *created.DealTitle != "Engine" {
		t.Errorf("names = %v, %v", created.ContactName, created.DealTitle)
	}
	item := base + "/" + created.ID

	note := postActivity(t, h, owner.Token, w.ID, map[string]any{"subject": "Met at the fair", "deal_id": deal.ID})
	rec = call(t, h, http.MethodGet, base+"?deal_id="+deal.ID, owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "deal timeline")
	if got := subjects(data[pageBody[activityBody]](t, rec)); !slices.Equal(got, []string{note.Subject, created.Subject}) {
		t.Errorf("deal timeline = %q, want newest first", got)
	}

	rec = call(t, h, http.MethodGet, item, owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "get")
	if got := data[activityBody](t, rec); got.ID != created.ID {
		t.Errorf("get = %+v", got)
	}

	rec = call(t, h, http.MethodPatch, item, member.Token, map[string]any{"subject": "Send the final proposal", "due_at": nil})
	wantStatus(t, rec, http.StatusOK, "update")
	if got := data[activityBody](t, rec); got.Subject != "Send the final proposal" || got.DueAt != nil || got.ContactID == nil || got.Body != "With figures" {
		t.Errorf("update = %+v", got)
	}

	rec = call(t, h, http.MethodPost, item+"/complete", owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "complete")
	if got := data[activityBody](t, rec); got.CompletedAt == nil {
		t.Errorf("complete = %+v", got)
	}
	rec = call(t, h, http.MethodGet, base+"?status=completed", owner.Token, nil)
	if got := subjects(data[pageBody[activityBody]](t, rec)); !slices.Equal(got, []string{"Send the final proposal"}) {
		t.Errorf("completed = %q", got)
	}
	rec = call(t, h, http.MethodPost, item+"/reopen", member.Token, nil)
	wantStatus(t, rec, http.StatusOK, "reopen")
	if got := data[activityBody](t, rec); got.CompletedAt != nil {
		t.Errorf("reopen = %+v", got)
	}
	wantStatus(t, call(t, h, http.MethodPost, base+"/"+note.ID+"/complete", owner.Token, nil), http.StatusBadRequest, "complete a note")

	wantStatus(t, call(t, h, http.MethodDelete, item, member.Token, nil), http.StatusNoContent, "delete")
	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, item}, {http.MethodPatch, item}, {http.MethodDelete, item},
		{http.MethodPost, item + "/complete"}, {http.MethodPost, item + "/reopen"},
	} {
		wantStatus(t, call(t, h, tt.method, tt.path, owner.Token, map[string]any{"subject": "Z"}), http.StatusNotFound, tt.method+" "+tt.path+" after delete")
	}
	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+note.ID, owner.Token, nil), http.StatusNoContent, "delete the note")
	rec = call(t, h, http.MethodGet, base, owner.Token, nil)
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("an empty list must encode as []: %s", rec.Body)
	}
}

func TestActivityFiltersOverHTTP(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	member := joined(t, h, owner.Token, w.ID, "member")
	contact := postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Ada"})
	company := postCompany(t, h, owner.Token, w.ID, map[string]any{"name": "Engines"})
	postActivity(t, h, owner.Token, w.ID, map[string]any{"subject": "Call", "kind": "call", "company_id": company.ID})
	postActivity(t, h, owner.Token, w.ID, map[string]any{"subject": "Early", "kind": "task", "contact_id": contact.ID, "due_at": "2027-01-01T00:00:00Z", "assignee_id": member.User.ID})
	postActivity(t, h, owner.Token, w.ID, map[string]any{"subject": "Late", "kind": "task", "company_id": company.ID, "due_at": "2027-06-01T00:00:00Z"})
	postActivity(t, h, owner.Token, w.ID, map[string]any{"subject": "Note about 50% off", "contact_id": contact.ID})
	base := activitiesPath(w.ID)

	for _, tt := range []struct {
		query string
		want  []string
	}{
		{"", []string{"Note about 50% off", "Late", "Early", "Call"}},
		{"contact_id=" + contact.ID, []string{"Note about 50% off", "Early"}},
		{"company_id=" + company.ID, []string{"Late", "Call"}},
		{"assignee_id=" + member.User.ID, []string{"Early"}},
		{"kind=task", []string{"Late", "Early"}},
		{"status=open", []string{"Late", "Early"}},
		{"status=completed", []string{}},
		{"due_before=2027-03-01T00:00:00Z", []string{"Early"}},
		{"due_after=2027-06-01T00:00:00Z", []string{"Late"}},
		{"q=50%25", []string{"Note about 50% off"}},
		{"deal_id=" + unknownUUID, []string{}},
		{"limit=2&offset=2", []string{"Early", "Call"}},
	} {
		t.Run(tt.query, func(t *testing.T) {
			rec := call(t, h, http.MethodGet, base+"?"+tt.query, owner.Token, nil)
			wantStatus(t, rec, http.StatusOK, tt.query)
			if got := subjects(data[pageBody[activityBody]](t, rec)); !slices.Equal(got, tt.want) {
				t.Errorf("%s = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
	for _, query := range []string{
		"contact_id=nope", "company_id=1", "deal_id=", "assignee_id=x", "kind=email", "status=done",
		"due_before=2027-01-01", "due_after=tomorrow", "due_before=0001-01-01T00:00:00Z", "limit=0", "offset=-1", "q=" + strings.Repeat("x", 201),
	} {
		wantStatus(t, call(t, h, http.MethodGet, base+"?"+query, owner.Token, nil), http.StatusBadRequest, query)
	}
}

func TestActivityInputValidation(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	contact := postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Ada"})
	task := postActivity(t, h, owner.Token, w.ID, map[string]any{"kind": "task", "subject": "Original", "contact_id": contact.ID})
	note := postActivity(t, h, owner.Token, w.ID, map[string]any{"subject": "A note", "contact_id": contact.ID})
	base := activitiesPath(w.ID)
	item := base + "/" + task.ID

	withCreate := func(fields map[string]any) map[string]any {
		body := map[string]any{"kind": "task", "subject": "x", "contact_id": contact.ID}
		for key, value := range fields {
			body[key] = value
		}
		return body
	}
	for _, tt := range []struct {
		name   string
		fields map[string]any
	}{
		{"blank subject", map[string]any{"subject": "  "}},
		{"long subject", map[string]any{"subject": strings.Repeat("x", 201)}},
		{"long body", map[string]any{"body": strings.Repeat("x", 10_001)}},
		{"NUL in subject", map[string]any{"subject": "a\x00b"}},
		{"NUL in body", map[string]any{"body": "a\x00b"}},
		{"due without a zone", map[string]any{"due_at": "2027-03-14T09:30:00"}},
		{"due as a date", map[string]any{"due_at": "2027-03-14"}},
		{"impossible due", map[string]any{"due_at": "2027-02-30T00:00:00Z"}},
		{"due out of range", map[string]any{"due_at": "1800-01-01T00:00:00Z"}},
		{"due before 1900 in UTC", map[string]any{"due_at": "1900-01-01T00:30:00+01:00"}},
		{"due after 2200 in UTC", map[string]any{"due_at": "2200-12-31T23:30:00-01:00"}},
		{"due a fraction into the last second past 2200", map[string]any{"due_at": "2200-12-31T23:59:59.5Z"}},
		{"due a microsecond past 2200", map[string]any{"due_at": "2200-12-31T23:59:59.000001Z"}},
		{"due a fraction before 1900", map[string]any{"due_at": "1899-12-31T23:59:59.9999999Z"}},
		{"blank due", map[string]any{"due_at": ""}},
		{"subject of the wrong type", map[string]any{"subject": 12}},
		{"completed_at", map[string]any{"completed_at": "2027-01-01T00:00:00Z"}},
		{"author_id", map[string]any{"author_id": owner.User.ID}},
	} {
		t.Run("create: "+tt.name, func(t *testing.T) {
			wantStatus(t, call(t, h, http.MethodPost, base, owner.Token, withCreate(tt.fields)), http.StatusBadRequest, tt.name)
		})
		t.Run("update: "+tt.name, func(t *testing.T) {
			wantStatus(t, call(t, h, http.MethodPatch, item, owner.Token, tt.fields), http.StatusBadRequest, tt.name)
		})
	}
	for _, tt := range []struct {
		name string
		body map[string]any
	}{
		{"no kind", map[string]any{"subject": "x", "contact_id": contact.ID}},
		{"unknown kind", map[string]any{"kind": "email", "subject": "x", "contact_id": contact.ID}},
		{"null kind", map[string]any{"kind": nil, "subject": "x", "contact_id": contact.ID}},
		{"no subject", map[string]any{"kind": "note", "contact_id": contact.ID}},
		{"no link", map[string]any{"kind": "note", "subject": "x"}},
		{"null links", map[string]any{"kind": "note", "subject": "x", "contact_id": nil, "company_id": nil, "deal_id": nil}},
		{"a due time on a note", map[string]any{"kind": "note", "subject": "x", "contact_id": contact.ID, "due_at": "2027-01-01T00:00:00Z"}},
		{"an assignee on a call", map[string]any{"kind": "call", "subject": "x", "contact_id": contact.ID, "assignee_id": owner.User.ID}},
	} {
		t.Run("create: "+tt.name, func(t *testing.T) {
			wantStatus(t, call(t, h, http.MethodPost, base, owner.Token, tt.body), http.StatusBadRequest, tt.name)
		})
	}
	for _, tt := range []struct {
		name, path string
		body       map[string]any
	}{
		{"no fields", item, map[string]any{}},
		{"kind", item, map[string]any{"kind": "note", "subject": "Changed"}},
		{"null subject", item, map[string]any{"subject": nil}},
		{"null body", item, map[string]any{"body": nil}},
		{"clearing the only link", item, map[string]any{"contact_id": nil}},
		{"a due time on a note", base + "/" + note.ID, map[string]any{"due_at": "2027-01-01T00:00:00Z"}},
		{"an assignee on a note", base + "/" + note.ID, map[string]any{"assignee_id": owner.User.ID}},
	} {
		t.Run("update: "+tt.name, func(t *testing.T) {
			wantStatus(t, call(t, h, http.MethodPatch, tt.path, owner.Token, tt.body), http.StatusBadRequest, tt.name)
		})
	}
	t.Run("invalid UTF-8", func(t *testing.T) {
		wantStatus(t, callRaw(t, h, http.MethodPost, base, owner.Token, []byte("{\"kind\":\"note\",\"subject\":\"a\xffb\",\"contact_id\":\""+contact.ID+"\"}")), http.StatusBadRequest, "create")
		wantStatus(t, callRaw(t, h, http.MethodPatch, item, owner.Token, []byte("{\"body\":\"a\xffb\"}")), http.StatusBadRequest, "update")
	})
	t.Run("the limits themselves are accepted", func(t *testing.T) {
		postActivity(t, h, owner.Token, w.ID, map[string]any{"subject": strings.Repeat("é", 200), "body": strings.Repeat("é", 10_000), "contact_id": contact.ID})
	})
	if got := data[activityBody](t, call(t, h, http.MethodGet, item, owner.Token, nil)); got.Subject != "Original" || got.DueAt != nil || got.ContactID == nil {
		t.Errorf("a refused request changed the activity: %+v", got)
	}
}

// The T4 convention: null on a text field is refused, a left-out field stays, and null
// on a nullable reference or the due time clears it. On a create, null counts as left out.
func TestActivityNullHandling(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	p := firstPipeline(t, h, owner.Token, w.ID)
	contact := postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Ada"})
	company := postCompany(t, h, owner.Token, w.ID, map[string]any{"name": "Engines"})
	deal := postDeal(t, h, owner.Token, w.ID, p.ID, nil)
	a := postActivity(t, h, owner.Token, w.ID, map[string]any{
		"kind": "task", "contact_id": contact.ID, "company_id": company.ID, "deal_id": deal.ID,
		"due_at": "2027-01-01T00:00:00Z", "assignee_id": owner.User.ID, "body": "Body",
	})
	item := activitiesPath(w.ID) + "/" + a.ID

	rec := call(t, h, http.MethodPatch, item, owner.Token, map[string]any{"subject": "Kept everything"})
	wantStatus(t, rec, http.StatusOK, "subject only")
	if got := data[activityBody](t, rec); got.ContactID == nil || got.CompanyID == nil || got.DealID == nil || got.DueAt == nil || got.AssigneeID == nil || got.Body != "Body" {
		t.Errorf("patch dropped a field: %+v", got)
	}
	rec = call(t, h, http.MethodPatch, item, owner.Token, map[string]any{"body": ""})
	wantStatus(t, rec, http.StatusOK, "empty body")
	if got := data[activityBody](t, rec); got.Body != "" {
		t.Errorf("empty body = %q", got.Body)
	}
	rec = call(t, h, http.MethodPatch, item, owner.Token, map[string]any{"due_at": nil, "assignee_id": nil, "contact_id": nil, "company_id": nil})
	wantStatus(t, rec, http.StatusOK, "clear")
	if got := data[activityBody](t, rec); got.DueAt != nil || got.AssigneeID != nil || got.ContactID != nil || got.CompanyID != nil || got.DealID == nil {
		t.Errorf("after clearing = %+v", got)
	}

	created := postActivity(t, h, owner.Token, w.ID, map[string]any{"kind": "task", "contact_id": contact.ID, "company_id": nil, "deal_id": nil, "due_at": nil, "assignee_id": nil, "body": nil})
	if created.CompanyID != nil || created.DueAt != nil || created.AssigneeID != nil || created.Body != "" {
		t.Errorf("create with nulls = %+v", created)
	}
}

// Every member reads, creates, and completes; only the author or an owner or admin
// edits or deletes.
func TestActivityEditRightsOverHTTP(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	author := joined(t, h, owner.Token, w.ID, "member")
	colleague := joined(t, h, owner.Token, w.ID, "member")
	admin := joined(t, h, owner.Token, w.ID, "admin")
	contact := postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Ada"})
	a := postActivity(t, h, author.Token, w.ID, map[string]any{"kind": "task", "contact_id": contact.ID})
	item := activitiesPath(w.ID) + "/" + a.ID

	rec := call(t, h, http.MethodPatch, item, colleague.Token, map[string]any{"subject": "pwned"})
	wantStatus(t, rec, http.StatusForbidden, "a colleague's edit")
	if !strings.Contains(rec.Body.String(), "author") {
		t.Errorf("forbidden body = %s", rec.Body)
	}
	wantStatus(t, call(t, h, http.MethodDelete, item, colleague.Token, nil), http.StatusForbidden, "a colleague's delete")
	wantStatus(t, call(t, h, http.MethodPost, item+"/complete", colleague.Token, nil), http.StatusOK, "a colleague completes")
	wantStatus(t, call(t, h, http.MethodPatch, item, author.Token, map[string]any{"subject": "Mine"}), http.StatusOK, "the author's edit")
	wantStatus(t, call(t, h, http.MethodPatch, item, owner.Token, map[string]any{"body": "Reviewed"}), http.StatusOK, "the owner's edit")
	wantStatus(t, call(t, h, http.MethodDelete, item, admin.Token, nil), http.StatusNoContent, "an admin's delete")
}

// Everything out of reach gets the same bytes back, so existence does not leak.
func TestActivitiesOutOfReachAnswerAlike(t *testing.T) {
	h := authHandler(t)
	iso := newIsolation(t, h)
	theirs := postActivity(t, h, iso.bToken, iso.b, map[string]any{"kind": "task", "subject": "Secret", "contact_id": iso.contactB.ID})
	contactA := postContact(t, h, iso.aToken, iso.a, map[string]any{"first_name": "Mine"})
	deleted := postActivity(t, h, iso.aToken, iso.a, map[string]any{"kind": "task", "contact_id": contactA.ID})
	wantStatus(t, call(t, h, http.MethodDelete, activitiesPath(iso.a)+"/"+deleted.ID, iso.aToken, nil), http.StatusNoContent, "delete")

	base := activitiesPath(iso.a) + "/"
	paths := []string{base + unknownUUID, base + theirs.ID, base + deleted.ID}
	for _, bad := range malformedIDs {
		paths = append(paths, base+bad)
	}
	wantSame404(t, h, iso.aToken, http.MethodGet, nil, paths)
	wantSame404(t, h, iso.aToken, http.MethodPatch, map[string]any{"subject": "pwned"}, paths)
	wantSame404(t, h, iso.aToken, http.MethodDelete, nil, paths)
	for _, action := range []string{"/complete", "/reopen"} {
		withAction := make([]string, len(paths))
		for i, path := range paths {
			withAction[i] = path + action
		}
		wantSame404(t, h, iso.aToken, http.MethodPost, nil, withAction)
	}
	got := data[activityBody](t, call(t, h, http.MethodGet, activitiesPath(iso.b)+"/"+theirs.ID, iso.bToken, nil))
	if got.Subject != "Secret" || got.CompletedAt != nil {
		t.Errorf("B's activity after the attempts = %+v", got)
	}
}

// A link to anything out of reach answers like a link to something that does not exist,
// whether in another workspace, deleted, or malformed.
func TestActivityLinksOutOfReachAnswerAlike(t *testing.T) {
	h := authHandler(t)
	iso := newIsolation(t, h)
	pA, pB := firstPipeline(t, h, iso.aToken, iso.a), firstPipeline(t, h, iso.bToken, iso.b)
	dealB := postDeal(t, h, iso.bToken, iso.b, pB.ID, nil)
	ownerB := data[[]struct {
		UserID string `json:"user_id"`
	}](t, call(t, h, http.MethodGet, "/workspaces/"+iso.b+"/members", iso.bToken, nil))[0].UserID
	contactA := postContact(t, h, iso.aToken, iso.a, map[string]any{"first_name": "Mine"})
	deletedContact := postContact(t, h, iso.aToken, iso.a, map[string]any{"first_name": "Gone"})
	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+iso.a+"/contacts/"+deletedContact.ID, iso.aToken, nil), http.StatusNoContent, "delete contact")
	deletedCompany := postCompany(t, h, iso.aToken, iso.a, map[string]any{"name": "Gone"})
	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+iso.a+"/companies/"+deletedCompany.ID, iso.aToken, nil), http.StatusNoContent, "delete company")
	deletedDeal := postDeal(t, h, iso.aToken, iso.a, pA.ID, nil)
	wantStatus(t, call(t, h, http.MethodDelete, dealsPath(iso.a)+"/"+deletedDeal.ID, iso.aToken, nil), http.StatusNoContent, "delete deal")
	mine := postActivity(t, h, iso.aToken, iso.a, map[string]any{"kind": "task", "contact_id": contactA.ID})

	for _, tt := range []struct {
		field string
		ids   []string
		extra map[string]any
	}{
		{"contact_id", []string{iso.contactB.ID, deletedContact.ID, unknownUUID, "nope"}, nil},
		{"company_id", []string{iso.companyB.ID, deletedCompany.ID, unknownUUID, "nope"}, nil},
		{"deal_id", []string{dealB.ID, deletedDeal.ID, unknownUUID, "nope"}, nil},
		{"assignee_id", []string{ownerB, unknownUUID, "nope"}, map[string]any{"contact_id": contactA.ID}},
	} {
		t.Run(tt.field, func(t *testing.T) {
			var first string
			for _, id := range tt.ids {
				body := map[string]any{"kind": "task", "subject": "rogue", tt.field: id}
				for key, value := range tt.extra {
					body[key] = value
				}
				for _, rec := range []*httptest.ResponseRecorder{
					call(t, h, http.MethodPost, activitiesPath(iso.a), iso.aToken, body),
					call(t, h, http.MethodPatch, activitiesPath(iso.a)+"/"+mine.ID, iso.aToken, map[string]any{tt.field: id}),
				} {
					if rec.Code != http.StatusNotFound {
						t.Errorf("%s %s = %d, want 404: %s", tt.field, id, rec.Code, rec.Body)
						continue
					}
					if first == "" {
						first = rec.Body.String()
					} else if rec.Body.String() != first {
						t.Errorf("%s %s body %q, want %q", tt.field, id, rec.Body, first)
					}
				}
			}
		})
	}
	if got := data[activityBody](t, call(t, h, http.MethodGet, activitiesPath(iso.a)+"/"+mine.ID, iso.aToken, nil)); got.ContactID == nil || got.CompanyID != nil || got.DealID != nil || got.AssigneeID != nil {
		t.Errorf("refused links changed the activity: %+v", got)
	}
}

func TestNonMemberGetsTheWorkspace404OnEveryActivityRoute(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	stranger := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	contact := postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Ada"})
	a := postActivity(t, h, owner.Token, w.ID, map[string]any{"kind": "task", "contact_id": contact.ID})
	want := fmt.Sprintf("{\"error\":%q}\n", msgWorkspaceNotFound)
	base := activitiesPath(w.ID)
	for _, route := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, base, nil},
		{http.MethodGet, base + "?contact_id=" + contact.ID, nil},
		{http.MethodPost, base, map[string]any{"kind": "note", "subject": "x", "contact_id": contact.ID}},
		// Invalid input must not reveal anything before membership is checked.
		{http.MethodPost, base, map[string]any{"kind": "nope"}},
		{http.MethodGet, base + "?kind=nope", nil},
		{http.MethodGet, base + "/" + a.ID, nil},
		{http.MethodPatch, base + "/" + a.ID, map[string]any{"subject": "x"}},
		{http.MethodDelete, base + "/" + a.ID, nil},
		{http.MethodPost, base + "/" + a.ID + "/complete", nil},
		{http.MethodPost, base + "/" + a.ID + "/reopen", nil},
	} {
		rec := call(t, h, route.method, route.path, stranger.Token, route.body)
		if rec.Code != http.StatusNotFound || rec.Body.String() != want {
			t.Errorf("%s %s = %d %q, want 404 %q", route.method, route.path, rec.Code, rec.Body, want)
		}
		wantStatus(t, call(t, h, route.method, route.path, "", route.body), http.StatusUnauthorized, "no token: "+route.path)
	}
	if got := data[activityBody](t, call(t, h, http.MethodGet, base+"/"+a.ID, owner.Token, nil)); got.CompletedAt != nil || got.Subject != "Activity" {
		t.Errorf("the activity after the attempts = %+v", got)
	}
}

// Deleting a contact keeps an activity that links to something else, and takes one
// that linked only to it.
func TestDeletingAContactUpdatesItsActivitiesOverHTTP(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	p := firstPipeline(t, h, owner.Token, w.ID)
	contact := postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Ada"})
	deal := postDeal(t, h, owner.Token, w.ID, p.ID, nil)
	both := postActivity(t, h, owner.Token, w.ID, map[string]any{"contact_id": contact.ID, "deal_id": deal.ID})
	only := postActivity(t, h, owner.Token, w.ID, map[string]any{"contact_id": contact.ID})

	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+w.ID+"/contacts/"+contact.ID, owner.Token, nil), http.StatusNoContent, "delete contact")
	rec := call(t, h, http.MethodGet, activitiesPath(w.ID)+"/"+both.ID, owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "the activity on the deal")
	if got := data[activityBody](t, rec); got.ContactID != nil || got.ContactName != nil || got.DealID == nil {
		t.Errorf("after the contact went = %+v", got)
	}
	wantStatus(t, call(t, h, http.MethodGet, activitiesPath(w.ID)+"/"+only.ID, owner.Token, nil), http.StatusNotFound, "the activity only on the contact")
}
