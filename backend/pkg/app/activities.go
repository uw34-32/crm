package app

import (
	"net/http"
	"net/url"
	"time"
	"unicode/utf8"

	"crm/pkg/model"
	"crm/pkg/store"

	"github.com/go-chi/chi/v5"
)

const (
	activityParam = "activityID"

	maxActivitySubjectRunes = 200
	maxActivityBodyRunes    = 10_000
	minDueYear              = 1900
	maxDueYear              = 2200

	msgInvalidSubject        = "a subject of at most 200 characters is required"
	msgInvalidActivityBody   = "body must be at most 10000 characters"
	msgInvalidKind           = "kind must be note, call, meeting, or task"
	msgInvalidDue            = "due_at must be a time with a zone such as 2027-03-14T09:30:00Z, from 1900 to 2200"
	msgInvalidActivityFilter = "contact_id, company_id, deal_id, and assignee_id must be IDs, kind note, call, meeting, or task, " +
		"status open or completed, and due_before and due_after times such as 2027-03-14T09:30:00Z"
	msgTaskOnlyFields = "only a task has due_at and assignee_id"
	msgFixedFields    = "the kind and the author never change, and completed_at changes through complete and reopen"
)

// activityRequest reads an activity's fields. The completed time and the author are
// here only to refuse them: the author is whoever creates the activity, and the
// completed time changes through completing or reopening a task.
type activityRequest struct {
	Kind        optionalString `json:"kind"`
	Subject     optionalString `json:"subject"`
	Body        optionalString `json:"body"`
	DueAt       optionalString `json:"due_at"`
	AssigneeID  optionalString `json:"assignee_id"`
	ContactID   optionalString `json:"contact_id"`
	CompanyID   optionalString `json:"company_id"`
	DealID      optionalString `json:"deal_id"`
	CompletedAt optionalString `json:"completed_at"`
	AuthorID    optionalString `json:"author_id"`

	due *time.Time
}

// parseDue reads a time with a zone, within the years the schema accepts. The years
// are checked in UTC, as the schema checks them.
func parseDue(raw string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	year := parsed.UTC().Year()
	return parsed, year >= minDueYear && year <= maxDueYear
}

// normalize trims and checks the fields that are present, and returns what is wrong
// with them, if anything. As for deals, references are not checked here: a malformed
// one is something that cannot be found, with the same answer as any other.
func (req *activityRequest) normalize() string {
	if optionalNUL(req.Kind.Value, req.Subject.Value, req.Body.Value, req.DueAt.Value) {
		return msgInvalidBody
	}
	if req.CompletedAt.Set || req.AuthorID.Set {
		return msgFixedFields
	}
	if !requiredFits(req.Subject.Value, maxActivitySubjectRunes) {
		return msgInvalidSubject
	}
	if req.Body.Value != nil && utf8.RuneCountInString(*req.Body.Value) > maxActivityBodyRunes {
		return msgInvalidActivityBody
	}
	if req.Kind.Value != nil && !model.ActivityKind(*req.Kind.Value).Valid() {
		return msgInvalidKind
	}
	if req.DueAt.Value != nil {
		due, ok := parseDue(*req.DueAt.Value)
		if !ok {
			return msgInvalidDue
		}
		req.due = &due
	}
	return ""
}

func decodeActivity(w http.ResponseWriter, r *http.Request) (activityRequest, bool) {
	var req activityRequest
	if err := decodeText(w, r, &req); err != nil {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return activityRequest{}, false
	}
	if problem := req.normalize(); problem != "" {
		errResp(w, http.StatusBadRequest, problem)
		return activityRequest{}, false
	}
	return req, true
}

// activityFilter reads the filters of a list, refusing anything invalid like the list
// arguments do.
func activityFilter(r *http.Request) (model.ActivityFilter, bool) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return model.ActivityFilter{}, false
	}
	var filter model.ActivityFilter
	for _, f := range []struct {
		name string
		dest *string
	}{
		{"contact_id", &filter.ContactID},
		{"company_id", &filter.CompanyID},
		{"deal_id", &filter.DealID},
		{"assignee_id", &filter.AssigneeID},
	} {
		if query.Has(f.name) {
			*f.dest = query.Get(f.name)
			if !store.ValidID(*f.dest) {
				return model.ActivityFilter{}, false
			}
		}
	}
	if query.Has("kind") {
		filter.Kind = model.ActivityKind(query.Get("kind"))
		if !filter.Kind.Valid() {
			return model.ActivityFilter{}, false
		}
	}
	if query.Has("status") {
		filter.State = model.ActivityState(query.Get("status"))
		if !filter.State.Valid() {
			return model.ActivityFilter{}, false
		}
	}
	for _, f := range []struct {
		name string
		dest **time.Time
	}{
		{"due_before", &filter.DueBefore},
		{"due_after", &filter.DueAfter},
	} {
		if query.Has(f.name) {
			bound, ok := parseDue(query.Get(f.name))
			if !ok {
				return model.ActivityFilter{}, false
			}
			*f.dest = &bound
		}
	}
	return filter, true
}

func (a *App) listActivities(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	params, ok := listRequest(w, r)
	if !ok {
		return
	}
	filter, ok := activityFilter(r)
	if !ok {
		errResp(w, http.StatusBadRequest, msgInvalidActivityFilter)
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	page, err := a.store.ListActivities(ctx, workspace.ID, filter, params)
	if err != nil {
		crmFailure(w, "list activities", err)
		return
	}
	dataResp(w, http.StatusOK, page)
}

func (a *App) createActivity(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	req, ok := decodeActivity(w, r)
	if !ok {
		return
	}
	problem := ""
	switch {
	case req.Kind.Value == nil:
		problem = msgInvalidKind
	case req.Subject.Value == nil:
		problem = msgInvalidSubject
	case model.ActivityKind(*req.Kind.Value) != model.ActivityTask && (req.due != nil || req.AssigneeID.Value != nil):
		problem = msgTaskOnlyFields
	}
	if problem != "" {
		errResp(w, http.StatusBadRequest, problem)
		return
	}
	// On a create, null counts as left out.
	in := model.ActivityInput{
		Kind: model.ActivityKind(*req.Kind.Value), Subject: *req.Subject.Value, Body: deref(req.Body.Value), DueAt: req.due,
		AuthorID: userID, AssigneeID: req.AssigneeID.Value,
		ContactID: req.ContactID.Value, CompanyID: req.CompanyID.Value, DealID: req.DealID.Value,
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	activity, err := a.store.CreateActivity(ctx, workspace.ID, in)
	if err != nil {
		crmFailure(w, "create activity", err)
		return
	}
	dataResp(w, http.StatusCreated, activity)
}

func (a *App) getActivity(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	activity, err := a.store.GetActivity(ctx, workspace.ID, chi.URLParam(r, activityParam))
	if err != nil {
		crmFailure(w, "get activity", err)
		return
	}
	dataResp(w, http.StatusOK, activity)
}

// updateProblem returns what is wrong with a patch as a whole, if anything.
func (req *activityRequest) updateProblem() string {
	if problem := nullProblem([]namedText{{"subject", req.Subject}, {"body", req.Body}}); problem != "" {
		return problem
	}
	if req.Kind.Set {
		return msgFixedFields
	}
	if !req.Subject.Set && !req.Body.Set && !req.DueAt.Set && !req.AssigneeID.Set &&
		!req.ContactID.Set && !req.CompanyID.Set && !req.DealID.Set {
		return msgNoFields
	}
	return ""
}

func (a *App) updateActivity(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	req, ok := decodeActivity(w, r)
	if !ok {
		return
	}
	if problem := req.updateProblem(); problem != "" {
		errResp(w, http.StatusBadRequest, problem)
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	activity, err := a.store.UpdateActivity(ctx, workspace.ID, userID, chi.URLParam(r, activityParam), model.ActivityPatch{
		Subject: req.Subject.Value, Body: req.Body.Value,
		SetDue: req.DueAt.Set, DueAt: req.due,
		SetAssignee: req.AssigneeID.Set, AssigneeID: req.AssigneeID.Value,
		SetContact: req.ContactID.Set, ContactID: req.ContactID.Value,
		SetCompany: req.CompanyID.Set, CompanyID: req.CompanyID.Value,
		SetDeal: req.DealID.Set, DealID: req.DealID.Value,
	})
	if err != nil {
		crmFailure(w, "update activity", err)
		return
	}
	dataResp(w, http.StatusOK, activity)
}

func (a *App) deleteActivity(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	if err := a.store.DeleteActivity(ctx, workspace.ID, userID, chi.URLParam(r, activityParam)); err != nil {
		crmFailure(w, "delete activity", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setActivityCompleted answers complete and reopen. Any member may do either, as a
// task is shared work.
func (a *App) setActivityCompleted(completed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspace, _, ok := workspaceActor(w, r)
		if !ok {
			return
		}
		ctx, cancel := a.dbContext(r)
		defer cancel()
		activity, err := a.store.SetActivityCompleted(ctx, workspace.ID, chi.URLParam(r, activityParam), completed)
		if err != nil {
			crmFailure(w, "set activity completed", err)
			return
		}
		dataResp(w, http.StatusOK, activity)
	}
}
