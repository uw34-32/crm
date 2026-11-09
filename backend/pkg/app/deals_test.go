package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type dealBody struct {
	ID                string  `json:"id"`
	WorkspaceID       string  `json:"workspace_id"`
	PipelineID        string  `json:"pipeline_id"`
	StageID           string  `json:"stage_id"`
	Status            string  `json:"status"`
	Title             string  `json:"title"`
	ValueMinor        int64   `json:"value_minor"`
	Currency          string  `json:"currency"`
	ExpectedCloseDate *string `json:"expected_close_date"`
	ClosedAt          *string `json:"closed_at"`
	LostReason        string  `json:"lost_reason"`
	CompanyID         *string `json:"company_id"`
	CompanyName       *string `json:"company_name"`
	ContactID         *string `json:"contact_id"`
	ContactName       *string `json:"contact_name"`
	OwnerID           *string `json:"owner_id"`
	Position          int     `json:"position"`
}

func dealsPath(workspaceID string) string { return "/workspaces/" + workspaceID + "/deals" }

func postDeal(t *testing.T, h http.Handler, token, workspaceID, pipelineID string, fields map[string]any) dealBody {
	t.Helper()
	body := map[string]any{"title": "Deal", "currency": "USD", "pipeline_id": pipelineID}
	for key, value := range fields {
		body[key] = value
	}
	rec := call(t, h, http.MethodPost, dealsPath(workspaceID), token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create deal = %d: %s", rec.Code, rec.Body)
	}
	return data[dealBody](t, rec)
}

func TestDealRoutes(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	p := firstPipeline(t, h, owner.Token, w.ID)
	company := postCompany(t, h, owner.Token, w.ID, map[string]any{"name": "Analytical Engines"})
	contact := postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Ada", "last_name": "Lovelace"})
	base := dealsPath(w.ID)

	rec := call(t, h, http.MethodPost, base, owner.Token, map[string]any{
		"title": "  Big one  ", "value_minor": 1234567, "currency": " eur ", "expected_close_date": "2027-03-14", "pipeline_id": p.ID,
		"company_id": company.ID, "contact_id": contact.ID,
	})
	wantStatus(t, rec, http.StatusCreated, "create")
	created := data[dealBody](t, rec)
	// The owner defaults to the caller, the stage to the first open one.
	if created.Title != "Big one" || created.ValueMinor != 1234567 || created.Currency != "EUR" || created.WorkspaceID != w.ID ||
		created.PipelineID != p.ID || created.StageID != p.Stages[0].ID || created.Status != "open" || created.Position != 0 ||
		created.ClosedAt != nil || created.LostReason != "" {
		t.Errorf("created = %+v", created)
	}
	if created.ExpectedCloseDate == nil || *created.ExpectedCloseDate != "2027-03-14" {
		t.Errorf("date = %v", created.ExpectedCloseDate)
	}
	if created.OwnerID == nil || *created.OwnerID != owner.User.ID {
		t.Errorf("owner = %v", created.OwnerID)
	}
	if created.CompanyName == nil || *created.CompanyName != company.Name || created.ContactName == nil || *created.ContactName != "Ada Lovelace" {
		t.Errorf("names = %v, %v", created.CompanyName, created.ContactName)
	}

	rec = call(t, h, http.MethodGet, base+"/"+created.ID, owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "get")
	if got := data[dealBody](t, rec); got.ID != created.ID || got.Title != created.Title {
		t.Errorf("get = %+v", got)
	}

	rec = call(t, h, http.MethodPatch, base+"/"+created.ID, owner.Token, map[string]any{"title": "Bigger", "value_minor": 5, "currency": "usd"})
	wantStatus(t, rec, http.StatusOK, "update")
	if got := data[dealBody](t, rec); got.Title != "Bigger" || got.ValueMinor != 5 || got.Currency != "USD" || got.CompanyID == nil || got.OwnerID == nil {
		t.Errorf("update = %+v", got)
	}

	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+created.ID, owner.Token, nil), http.StatusNoContent, "delete")
	wantStatus(t, call(t, h, http.MethodGet, base+"/"+created.ID, owner.Token, nil), http.StatusNotFound, "get after delete")
	wantStatus(t, call(t, h, http.MethodPatch, base+"/"+created.ID, owner.Token, map[string]any{"title": "Z"}), http.StatusNotFound, "update after delete")
	wantStatus(t, call(t, h, http.MethodPost, base+"/"+created.ID+"/move", owner.Token, map[string]any{"stage_id": p.Stages[1].ID}), http.StatusNotFound, "move after delete")
	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+created.ID, owner.Token, nil), http.StatusNotFound, "delete after delete")
	rec = call(t, h, http.MethodGet, base, owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "list")
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("an empty list must encode as []: %s", rec.Body)
	}
}

func TestDealStartStageIsChecked(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	p := firstPipeline(t, h, owner.Token, w.ID)
	base := dealsPath(w.ID)

	chosen := postDeal(t, h, owner.Token, w.ID, p.ID, map[string]any{"stage_id": p.Stages[1].ID})
	if chosen.StageID != p.Stages[1].ID {
		t.Errorf("chosen = %+v", chosen)
	}
	rec := call(t, h, http.MethodPost, base, owner.Token, map[string]any{"title": "x", "currency": "USD", "pipeline_id": p.ID, "stage_id": stageWithKind(t, p, "won").ID})
	wantStatus(t, rec, http.StatusBadRequest, "start in a won stage")
	rec = call(t, h, http.MethodPost, base, owner.Token, map[string]any{"title": "x", "currency": "USD", "pipeline_id": p.ID, "stage_id": stageWithKind(t, p, "lost").ID})
	wantStatus(t, rec, http.StatusBadRequest, "start in a lost stage")
}

func TestDealFiltersAndPaginationOverHTTP(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	member := joined(t, h, owner.Token, w.ID, "member")
	p := firstPipeline(t, h, owner.Token, w.ID)
	other := postPipeline(t, h, owner.Token, w.ID, "Other")
	acme := postCompany(t, h, owner.Token, w.ID, map[string]any{"name": "Acme 100%"})
	base := dealsPath(w.ID)

	first := postDeal(t, h, owner.Token, w.ID, p.ID, map[string]any{"title": "First"})
	second := postDeal(t, h, member.Token, w.ID, p.ID, map[string]any{"title": "Second", "company_id": acme.ID})
	third := postDeal(t, h, owner.Token, w.ID, p.ID, map[string]any{"title": "Third"})
	fourth := postDeal(t, h, owner.Token, w.ID, other.ID, map[string]any{"title": "Fourth"})
	wantStatus(t, call(t, h, http.MethodPost, base+"/"+second.ID+"/move", owner.Token, map[string]any{"stage_id": stageWithKind(t, p, "won").ID}), http.StatusOK, "win")
	wantStatus(t, call(t, h, http.MethodPost, base+"/"+third.ID+"/move", owner.Token, map[string]any{"stage_id": stageWithKind(t, p, "lost").ID, "lost_reason": "Price"}), http.StatusOK, "lose")

	for _, tt := range []struct {
		name, query string
		want        []string
	}{
		{"pipeline", "pipeline_id=" + other.ID, []string{fourth.ID}},
		{"stage", "stage_id=" + p.Stages[0].ID, []string{first.ID}},
		{"owner", "owner_id=" + member.User.ID, []string{second.ID}},
		{"open", "pipeline_id=" + p.ID + "&status=open", []string{first.ID}},
		{"won", "status=won", []string{second.ID}},
		{"lost", "status=lost", []string{third.ID}},
		{"search title", "q=FIRST", []string{first.ID}},
		{"search company, literal percent", "q=%25", []string{second.ID}},
		{"combined", "pipeline_id=" + p.ID + "&status=won&q=second", []string{second.ID}},
		{"nothing", "pipeline_id=" + other.ID + "&status=won", []string{}},
		{"unknown pipeline", "pipeline_id=" + unknownUUID, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := call(t, h, http.MethodGet, base+"?"+tt.query, owner.Token, nil)
			wantStatus(t, rec, http.StatusOK, tt.query)
			page := data[pageBody[dealBody]](t, rec)
			got := make([]string, len(page.Items))
			for i, d := range page.Items {
				got[i] = d.ID
			}
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("%s = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
	for _, query := range []string{
		"pipeline_id=nope", "stage_id=nope", "owner_id=nope", "pipeline_id=", "status=closed", "status=",
		"limit=0", "limit=201", "offset=-1", "offset=100001", "q=" + strings.Repeat("x", 201), "q=%00", "q=%ff", "stage_id=%00",
	} {
		wantStatus(t, call(t, h, http.MethodGet, base+"?"+query, owner.Token, nil), http.StatusBadRequest, query)
	}

	rec := call(t, h, http.MethodGet, base+"?limit=2", owner.Token, nil)
	page := data[pageBody[dealBody]](t, rec)
	rest := data[pageBody[dealBody]](t, call(t, h, http.MethodGet, base+"?limit=2&offset=2", owner.Token, nil))
	if len(page.Items) != 2 || !page.HasMore || len(rest.Items) != 2 || rest.HasMore || page.Limit != 2 || rest.Offset != 2 {
		t.Errorf("pages = %+v, %+v", page, rest)
	}
}

func TestMoveDealOverHTTP(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	member := joined(t, h, owner.Token, w.ID, "member")
	p := firstPipeline(t, h, owner.Token, w.ID)
	other := postPipeline(t, h, owner.Token, w.ID, "Other")
	won, lost := stageWithKind(t, p, "won"), stageWithKind(t, p, "lost")
	d := postDeal(t, h, owner.Token, w.ID, p.ID, nil)
	neighbour := postDeal(t, h, owner.Token, w.ID, p.ID, map[string]any{"title": "Neighbour"})
	path := dealsPath(w.ID) + "/" + d.ID + "/move"
	move := func(body map[string]any) (int, dealBody) {
		t.Helper()
		rec := call(t, h, http.MethodPost, path, member.Token, body)
		if rec.Code != http.StatusOK {
			return rec.Code, dealBody{}
		}
		return rec.Code, data[dealBody](t, rec)
	}

	// Every member can move a deal, and positions shift along.
	code, got := move(map[string]any{"stage_id": p.Stages[0].ID, "position": 1})
	if code != http.StatusOK || got.Position != 1 {
		t.Errorf("reorder = %d, %+v", code, got)
	}
	if n := data[dealBody](t, call(t, h, http.MethodGet, dealsPath(w.ID)+"/"+neighbour.ID, owner.Token, nil)); n.Position != 0 {
		t.Errorf("neighbour = %+v", n)
	}

	code, got = move(map[string]any{"stage_id": won.ID})
	if code != http.StatusOK || got.Status != "won" || got.ClosedAt == nil || got.LostReason != "" {
		t.Errorf("into won = %d, %+v", code, got)
	}

	for _, tt := range []struct {
		name string
		body map[string]any
	}{
		{"no reason", map[string]any{"stage_id": lost.ID}},
		{"blank reason", map[string]any{"stage_id": lost.ID, "lost_reason": "   "}},
		{"null reason", map[string]any{"stage_id": lost.ID, "lost_reason": nil}},
	} {
		if code, _ := move(tt.body); code != http.StatusBadRequest {
			t.Errorf("into lost, %s = %d, want 400", tt.name, code)
		}
	}
	code, got = move(map[string]any{"stage_id": lost.ID, "lost_reason": " Too expensive "})
	if code != http.StatusOK || got.Status != "lost" || got.ClosedAt == nil || got.LostReason != "Too expensive" {
		t.Errorf("into lost = %d, %+v", code, got)
	}

	code, got = move(map[string]any{"stage_id": p.Stages[1].ID})
	if code != http.StatusOK || got.Status != "open" || got.ClosedAt != nil || got.LostReason != "" {
		t.Errorf("back to open = %d, %+v", code, got)
	}

	for _, tt := range []struct {
		name string
		body map[string]any
		want int
	}{
		{"no stage", map[string]any{}, http.StatusBadRequest},
		{"null stage", map[string]any{"stage_id": nil}, http.StatusBadRequest},
		{"negative position", map[string]any{"stage_id": p.Stages[0].ID, "position": -1}, http.StatusBadRequest},
		{"fractional position", map[string]any{"stage_id": p.Stages[0].ID, "position": 1.5}, http.StatusBadRequest},
		{"reason on an open stage", map[string]any{"stage_id": p.Stages[0].ID, "lost_reason": "x"}, http.StatusBadRequest},
		{"reason on a won stage", map[string]any{"stage_id": won.ID, "lost_reason": "x"}, http.StatusBadRequest},
		{"long reason", map[string]any{"stage_id": lost.ID, "lost_reason": strings.Repeat("x", 501)}, http.StatusBadRequest},
		{"NUL in reason", map[string]any{"stage_id": lost.ID, "lost_reason": "a\x00b"}, http.StatusBadRequest},
		{"stage of another pipeline", map[string]any{"stage_id": other.Stages[0].ID}, http.StatusNotFound},
		{"malformed stage", map[string]any{"stage_id": "nope"}, http.StatusNotFound},
		{"unknown stage", map[string]any{"stage_id": unknownUUID}, http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if code, _ := move(tt.body); code != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, code, tt.want)
			}
		})
	}
	wantStatus(t, callRaw(t, h, http.MethodPost, path, member.Token, []byte("{\"stage_id\":\""+lost.ID+"\",\"lost_reason\":\"a\xffb\"}")), http.StatusBadRequest, "invalid UTF-8 in reason")
	if got := data[dealBody](t, call(t, h, http.MethodGet, dealsPath(w.ID)+"/"+d.ID, owner.Token, nil)); got.StageID != p.Stages[1].ID || got.Status != "open" {
		t.Errorf("refused moves changed the deal: %+v", got)
	}
}

func TestDealInputValidation(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	p := firstPipeline(t, h, owner.Token, w.ID)
	d := postDeal(t, h, owner.Token, w.ID, p.ID, map[string]any{"title": "Original", "value_minor": 10})
	base := dealsPath(w.ID)
	item := base + "/" + d.ID

	withCreate := func(fields map[string]any) map[string]any {
		body := map[string]any{"title": "x", "currency": "USD", "pipeline_id": p.ID}
		for key, value := range fields {
			body[key] = value
		}
		return body
	}
	badFields := []struct {
		name   string
		fields map[string]any
	}{
		{"blank title", map[string]any{"title": "  "}},
		{"long title", map[string]any{"title": strings.Repeat("x", 201)}},
		{"NUL in title", map[string]any{"title": "a\x00b"}},
		{"negative value", map[string]any{"value_minor": -1}},
		{"value above the bound", map[string]any{"value_minor": int64(1_000_000_000_000_000)}},
		{"fractional value", map[string]any{"value_minor": 10.5}},
		{"value as a string", map[string]any{"value_minor": "10"}},
		{"short currency", map[string]any{"currency": "US"}},
		{"long currency", map[string]any{"currency": "USDD"}},
		{"currency with digits", map[string]any{"currency": "U5D"}},
		{"currency not ASCII", map[string]any{"currency": "ÜSD"}},
		{"NUL in currency", map[string]any{"currency": "US\x00"}},
		{"impossible date", map[string]any{"expected_close_date": "2027-02-30"}},
		{"date with a time", map[string]any{"expected_close_date": "2027-02-03T00:00:00Z"}},
		{"date of another format", map[string]any{"expected_close_date": "03/02/2027"}},
		{"blank date", map[string]any{"expected_close_date": ""}},
		{"date out of range", map[string]any{"expected_close_date": "0001-01-01"}},
		{"far future date", map[string]any{"expected_close_date": "9999-01-01"}},
	}
	for _, tt := range badFields {
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
		{"no title", map[string]any{"currency": "USD", "pipeline_id": p.ID}},
		{"no currency", map[string]any{"title": "x", "pipeline_id": p.ID}},
		{"no pipeline", map[string]any{"title": "x", "currency": "USD"}},
		{"null title", map[string]any{"title": nil, "currency": "USD", "pipeline_id": p.ID}},
		{"null currency", map[string]any{"title": "x", "currency": nil, "pipeline_id": p.ID}},
		{"null pipeline", map[string]any{"title": "x", "currency": "USD", "pipeline_id": nil}},
	} {
		t.Run("create: "+tt.name, func(t *testing.T) {
			wantStatus(t, call(t, h, http.MethodPost, base, owner.Token, tt.body), http.StatusBadRequest, tt.name)
		})
	}
	for _, tt := range []struct {
		name string
		body map[string]any
	}{
		{"no fields", map[string]any{}},
		// Stage and position change only through a move, and the closed state with them.
		{"stage", map[string]any{"stage_id": p.Stages[1].ID}},
		{"pipeline", map[string]any{"pipeline_id": p.ID}},
		{"position", map[string]any{"position": 1}},
		{"lost reason", map[string]any{"lost_reason": "x"}},
	} {
		t.Run("update: "+tt.name, func(t *testing.T) {
			wantStatus(t, call(t, h, http.MethodPatch, item, owner.Token, tt.body), http.StatusBadRequest, tt.name)
		})
	}
	t.Run("invalid UTF-8", func(t *testing.T) {
		wantStatus(t, callRaw(t, h, http.MethodPost, base, owner.Token, []byte("{\"title\":\"a\xffb\",\"currency\":\"USD\",\"pipeline_id\":\""+p.ID+"\"}")), http.StatusBadRequest, "create")
		wantStatus(t, callRaw(t, h, http.MethodPatch, item, owner.Token, []byte("{\"title\":\"a\xffb\"}")), http.StatusBadRequest, "update")
	})
	t.Run("the bound itself is accepted", func(t *testing.T) {
		got := postDeal(t, h, owner.Token, w.ID, p.ID, map[string]any{"value_minor": int64(999_999_999_999_999)})
		if got.ValueMinor != 999_999_999_999_999 {
			t.Errorf("value = %d", got.ValueMinor)
		}
	})
	if got := data[dealBody](t, call(t, h, http.MethodGet, item, owner.Token, nil)); got.Title != "Original" || got.ValueMinor != 10 {
		t.Errorf("a refused request changed the deal: %+v", got)
	}
}

// The T4 convention: null on a text field is refused, a left-out field stays, and null
// on a nullable reference removes it. A date works like a reference.
func TestDealNullHandling(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	p := firstPipeline(t, h, owner.Token, w.ID)
	company := postCompany(t, h, owner.Token, w.ID, map[string]any{"name": "Co"})
	contact := postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Ada"})
	d := postDeal(t, h, owner.Token, w.ID, p.ID, map[string]any{"company_id": company.ID, "contact_id": contact.ID, "expected_close_date": "2027-01-01"})
	item := dealsPath(w.ID) + "/" + d.ID

	for _, field := range []string{"title", "currency", "value_minor"} {
		rec := call(t, h, http.MethodPatch, item, owner.Token, map[string]any{field: nil})
		wantStatus(t, rec, http.StatusBadRequest, field+" null")
		if !strings.Contains(rec.Body.String(), field+" must not be null") {
			t.Errorf("%s null: body = %s", field, rec.Body)
		}
	}

	// A left-out field stays.
	rec := call(t, h, http.MethodPatch, item, owner.Token, map[string]any{"title": "Kept links"})
	wantStatus(t, rec, http.StatusOK, "title only")
	if got := data[dealBody](t, rec); got.CompanyID == nil || got.ContactID == nil || got.OwnerID == nil || got.ExpectedCloseDate == nil {
		t.Errorf("patch dropped a field: %+v", got)
	}
	for _, field := range []string{"company_id", "contact_id", "owner_id", "expected_close_date"} {
		rec := call(t, h, http.MethodPatch, item, owner.Token, map[string]any{field: nil})
		wantStatus(t, rec, http.StatusOK, field+" null")
		got := data[dealBody](t, rec)
		var cleared *string
		switch field {
		case "company_id":
			cleared = got.CompanyID
		case "contact_id":
			cleared = got.ContactID
		case "owner_id":
			cleared = got.OwnerID
		case "expected_close_date":
			cleared = got.ExpectedCloseDate
		}
		if cleared != nil {
			t.Errorf("%s after null = %v", field, *cleared)
		}
	}
	// On a create, null counts as left out, as for contacts.
	created := postDeal(t, h, owner.Token, w.ID, p.ID, map[string]any{"company_id": nil, "contact_id": nil, "stage_id": nil, "expected_close_date": nil, "value_minor": nil, "owner_id": nil})
	if created.CompanyID != nil || created.OwnerID == nil || *created.OwnerID != owner.User.ID || created.ValueMinor != 0 {
		t.Errorf("create with nulls = %+v", created)
	}
}

// Everything out of reach gets the same bytes back, so existence does not leak.
func TestDealsOutOfReachAnswerAlike(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	stranger := register(t, h, uniqueEmail(t))
	mine := createWorkspace(t, h, owner.Token)
	theirs := createWorkspace(t, h, stranger.Token)
	ofMine, ofTheirs := firstPipeline(t, h, owner.Token, mine.ID), firstPipeline(t, h, stranger.Token, theirs.ID)
	dealOfTheirs := postDeal(t, h, stranger.Token, theirs.ID, ofTheirs.ID, map[string]any{"title": "Secret"})
	deleted := postDeal(t, h, owner.Token, mine.ID, ofMine.ID, nil)
	wantStatus(t, call(t, h, http.MethodDelete, dealsPath(mine.ID)+"/"+deleted.ID, owner.Token, nil), http.StatusNoContent, "delete")

	ids := map[string]string{"another workspace": dealOfTheirs.ID, "soft-deleted": deleted.ID, "nonexistent": unknownUUID}
	for _, bad := range malformedIDs {
		ids["malformed "+bad] = bad
	}
	want := fmt.Sprintf("{\"error\":%q}\n", "deal not found")
	for name, id := range ids {
		path := dealsPath(mine.ID) + "/" + id
		for _, route := range []struct {
			method, suffix string
			body           any
		}{
			{http.MethodGet, "", nil},
			{http.MethodPatch, "", map[string]any{"title": "x"}},
			{http.MethodDelete, "", nil},
			{http.MethodPost, "/move", map[string]any{"stage_id": ofMine.Stages[0].ID}},
		} {
			rec := call(t, h, route.method, path+route.suffix, owner.Token, route.body)
			if rec.Code != http.StatusNotFound || rec.Body.String() != want {
				t.Errorf("%s (%s) = %d %q, want 404 %q", route.method, name, rec.Code, rec.Body, want)
			}
		}
	}
	if got := data[dealBody](t, call(t, h, http.MethodGet, dealsPath(theirs.ID)+"/"+dealOfTheirs.ID, stranger.Token, nil)); got.Title != "Secret" {
		t.Errorf("their deal = %+v", got)
	}
	// Lists and search show nothing of theirs.
	rec := call(t, h, http.MethodGet, dealsPath(mine.ID)+"?q=Secret", owner.Token, nil)
	if page := data[pageBody[dealBody]](t, rec); len(page.Items) != 0 {
		t.Errorf("search = %+v", page)
	}
	rec = call(t, h, http.MethodGet, dealsPath(mine.ID)+"?pipeline_id="+ofTheirs.ID, owner.Token, nil)
	if page := data[pageBody[dealBody]](t, rec); len(page.Items) != 0 {
		t.Errorf("filter by their pipeline = %+v", page)
	}
}

// A reference in a body that cannot be used gets the same 404 whatever the reason.
func TestDealReferencesOutOfReachAnswerAlike(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	stranger := register(t, h, uniqueEmail(t))
	mine := createWorkspace(t, h, owner.Token)
	theirs := createWorkspace(t, h, stranger.Token)
	p := firstPipeline(t, h, owner.Token, mine.ID)
	ofTheirs := firstPipeline(t, h, stranger.Token, theirs.ID)
	companyOfTheirs := postCompany(t, h, stranger.Token, theirs.ID, map[string]any{"name": "Theirs"})
	contactOfTheirs := postContact(t, h, stranger.Token, theirs.ID, map[string]any{"first_name": "Theirs"})
	deletedCompany := postCompany(t, h, owner.Token, mine.ID, map[string]any{"name": "Gone"})
	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+mine.ID+"/companies/"+deletedCompany.ID, owner.Token, nil), http.StatusNoContent, "delete company")
	deletedContact := postContact(t, h, owner.Token, mine.ID, map[string]any{"first_name": "Gone"})
	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+mine.ID+"/contacts/"+deletedContact.ID, owner.Token, nil), http.StatusNoContent, "delete contact")
	d := postDeal(t, h, owner.Token, mine.ID, p.ID, nil)

	for _, tt := range []struct {
		field string
		want  string
		bad   []string
	}{
		{"company_id", "company not found", []string{companyOfTheirs.ID, deletedCompany.ID, unknownUUID, "nope", "a\x00b"}},
		{"contact_id", "contact not found", []string{contactOfTheirs.ID, deletedContact.ID, unknownUUID, "nope", "a\x00b"}},
		{"owner_id", "member not found", []string{stranger.User.ID, unknownUUID, "nope", "a\x00b"}},
	} {
		wantBody := fmt.Sprintf("{\"error\":%q}\n", tt.want)
		for _, id := range tt.bad {
			for name, rec := range map[string]*httptest.ResponseRecorder{
				"create": call(t, h, http.MethodPost, dealsPath(mine.ID), owner.Token, map[string]any{"title": "x", "currency": "USD", "pipeline_id": p.ID, tt.field: id}),
				"update": call(t, h, http.MethodPatch, dealsPath(mine.ID)+"/"+d.ID, owner.Token, map[string]any{tt.field: id}),
			} {
				if rec.Code != http.StatusNotFound || rec.Body.String() != wantBody {
					t.Errorf("%s %s=%q: %d %q, want 404 %q", name, tt.field, id, rec.Code, rec.Body, wantBody)
				}
			}
		}
	}
	for _, id := range []string{ofTheirs.ID, unknownUUID, "nope"} {
		rec := call(t, h, http.MethodPost, dealsPath(mine.ID), owner.Token, map[string]any{"title": "x", "currency": "USD", "pipeline_id": id})
		if rec.Code != http.StatusNotFound || rec.Body.String() != fmt.Sprintf("{\"error\":%q}\n", "pipeline not found") {
			t.Errorf("pipeline %s = %d %q", id, rec.Code, rec.Body)
		}
	}
	for _, id := range []string{ofTheirs.Stages[0].ID, unknownUUID, "nope"} {
		rec := call(t, h, http.MethodPost, dealsPath(mine.ID), owner.Token, map[string]any{"title": "x", "currency": "USD", "pipeline_id": p.ID, "stage_id": id})
		if rec.Code != http.StatusNotFound || rec.Body.String() != fmt.Sprintf("{\"error\":%q}\n", "stage not found") {
			t.Errorf("stage %s = %d %q", id, rec.Code, rec.Body)
		}
	}
}

func TestDeletingACompanyOrContactKeepsDealsOverHTTP(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	p := firstPipeline(t, h, owner.Token, w.ID)
	company := postCompany(t, h, owner.Token, w.ID, map[string]any{"name": "Co"})
	contact := postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Ada"})
	d := postDeal(t, h, owner.Token, w.ID, p.ID, map[string]any{"company_id": company.ID, "contact_id": contact.ID})
	item := dealsPath(w.ID) + "/" + d.ID

	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+w.ID+"/companies/"+company.ID, owner.Token, nil), http.StatusNoContent, "delete company")
	got := data[dealBody](t, call(t, h, http.MethodGet, item, owner.Token, nil))
	if got.CompanyID != nil || got.CompanyName != nil || got.ContactID == nil {
		t.Errorf("after deleting the company = %+v", got)
	}
	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+w.ID+"/contacts/"+contact.ID, owner.Token, nil), http.StatusNoContent, "delete contact")
	got = data[dealBody](t, call(t, h, http.MethodGet, item, owner.Token, nil))
	if got.ContactID != nil || got.ContactName != nil {
		t.Errorf("after deleting the contact = %+v", got)
	}
}

func TestRemovingAMemberKeepsTheirDealsOverHTTP(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	p := firstPipeline(t, h, owner.Token, w.ID)
	removed := joined(t, h, owner.Token, w.ID, "member")
	leaver := joined(t, h, owner.Token, w.ID, "member")
	base := dealsPath(w.ID)

	// Every member can write deals, and create one for another member.
	ofRemoved := postDeal(t, h, removed.Token, w.ID, p.ID, nil)
	ofLeaver := postDeal(t, h, removed.Token, w.ID, p.ID, map[string]any{"owner_id": leaver.User.ID})
	if ofRemoved.OwnerID == nil || *ofRemoved.OwnerID != removed.User.ID || ofLeaver.OwnerID == nil || *ofLeaver.OwnerID != leaver.User.ID {
		t.Fatalf("owners = %v, %v", ofRemoved.OwnerID, ofLeaver.OwnerID)
	}

	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+w.ID+"/members/"+removed.User.ID, owner.Token, nil), http.StatusNoContent, "remove")
	wantStatus(t, call(t, h, http.MethodPost, "/workspaces/"+w.ID+"/leave", leaver.Token, nil), http.StatusNoContent, "leave")

	for _, id := range []string{ofRemoved.ID, ofLeaver.ID} {
		rec := call(t, h, http.MethodGet, base+"/"+id, owner.Token, nil)
		wantStatus(t, rec, http.StatusOK, "the deal survives")
		if got := data[dealBody](t, rec); got.OwnerID != nil {
			t.Errorf("owner after removal = %v", *got.OwnerID)
		}
	}
	// The former member has no access, and cannot be assigned.
	wantStatus(t, call(t, h, http.MethodGet, base, removed.Token, nil), http.StatusNotFound, "former member lists")
	wantStatus(t, call(t, h, http.MethodPatch, base+"/"+ofRemoved.ID, owner.Token, map[string]any{"owner_id": removed.User.ID}), http.StatusNotFound, "assign a former member")
	// A manager can reassign it.
	rec := call(t, h, http.MethodPatch, base+"/"+ofRemoved.ID, owner.Token, map[string]any{"owner_id": owner.User.ID})
	wantStatus(t, rec, http.StatusOK, "reassign")
	if got := data[dealBody](t, rec); got.OwnerID == nil || *got.OwnerID != owner.User.ID {
		t.Errorf("reassigned = %v", got.OwnerID)
	}
}

func TestDeletedWorkspaceHidesItsPipelinesAndDeals(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	p := firstPipeline(t, h, owner.Token, w.ID)
	d := postDeal(t, h, owner.Token, w.ID, p.ID, nil)
	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+w.ID, owner.Token, nil), http.StatusNoContent, "delete")
	for _, path := range []string{pipelinesPath(w.ID), pipelinesPath(w.ID) + "/" + p.ID, dealsPath(w.ID), dealsPath(w.ID) + "/" + d.ID} {
		wantStatus(t, call(t, h, http.MethodGet, path, owner.Token, nil), http.StatusNotFound, path)
	}
}
