package app

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

type stageBody struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	PipelineID  string `json:"pipeline_id"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	Color       string `json:"color"`
	Probability int    `json:"probability"`
	Kind        string `json:"kind"`
}

type pipelineBody struct {
	ID          string      `json:"id"`
	WorkspaceID string      `json:"workspace_id"`
	Name        string      `json:"name"`
	Stages      []stageBody `json:"stages"`
}

func pipelinesPath(workspaceID string) string { return "/workspaces/" + workspaceID + "/pipelines" }

func firstPipeline(t *testing.T, h http.Handler, token, workspaceID string) pipelineBody {
	t.Helper()
	rec := call(t, h, http.MethodGet, pipelinesPath(workspaceID), token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list pipelines = %d: %s", rec.Code, rec.Body)
	}
	page := data[pageBody[pipelineBody]](t, rec)
	if len(page.Items) == 0 {
		t.Fatal("no pipelines")
	}
	rec = call(t, h, http.MethodGet, pipelinesPath(workspaceID)+"/"+page.Items[0].ID, token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get pipeline = %d: %s", rec.Code, rec.Body)
	}
	return data[pipelineBody](t, rec)
}

func postPipeline(t *testing.T, h http.Handler, token, workspaceID, name string) pipelineBody {
	t.Helper()
	rec := call(t, h, http.MethodPost, pipelinesPath(workspaceID), token, map[string]any{"name": name})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create pipeline = %d: %s", rec.Code, rec.Body)
	}
	return data[pipelineBody](t, rec)
}

func stageWithKind(t *testing.T, p pipelineBody, kind string) stageBody {
	t.Helper()
	for _, stage := range p.Stages {
		if stage.Kind == kind {
			return stage
		}
	}
	t.Fatalf("no %s stage in %+v", kind, p.Stages)
	return stageBody{}
}

// callRaw sends bytes as they are, for bodies json.Marshal would not produce.
func callRaw(t *testing.T, h http.Handler, method, path, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestNewWorkspaceHasADefaultPipelineOverHTTP(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)

	p := firstPipeline(t, h, owner.Token, w.ID)
	if p.Name == "" || p.WorkspaceID != w.ID || len(p.Stages) < 3 {
		t.Fatalf("pipeline = %+v", p)
	}
	if stageWithKind(t, p, "open").Position != 0 || stageWithKind(t, p, "won").Probability != 100 || stageWithKind(t, p, "lost").Probability != 0 {
		t.Errorf("stages = %+v", p.Stages)
	}
	// A list does not carry stages.
	rec := call(t, h, http.MethodGet, pipelinesPath(w.ID), owner.Token, nil)
	if strings.Contains(rec.Body.String(), `"stages"`) {
		t.Errorf("list carries stages: %s", rec.Body)
	}
}

func TestPipelineRoutes(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	base := pipelinesPath(w.ID)

	rec := call(t, h, http.MethodPost, base, owner.Token, map[string]any{"name": "  Renewals  "})
	wantStatus(t, rec, http.StatusCreated, "create")
	created := data[pipelineBody](t, rec)
	if created.Name != "Renewals" || len(created.Stages) == 0 {
		t.Errorf("created = %+v", created)
	}

	rec = call(t, h, http.MethodGet, base+"/"+created.ID, owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "get")
	if got := data[pipelineBody](t, rec); got.Name != "Renewals" || len(got.Stages) != len(created.Stages) {
		t.Errorf("get = %+v", got)
	}

	rec = call(t, h, http.MethodPatch, base+"/"+created.ID, owner.Token, map[string]any{"name": "Upsells"})
	wantStatus(t, rec, http.StatusOK, "rename")
	if got := data[pipelineBody](t, rec); got.Name != "Upsells" {
		t.Errorf("rename = %+v", got)
	}

	rec = call(t, h, http.MethodGet, base+"?q=upse", owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "search")
	if page := data[pageBody[pipelineBody]](t, rec); len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Errorf("search = %+v", page)
	}
	rec = call(t, h, http.MethodGet, base+"?limit=1", owner.Token, nil)
	if page := data[pageBody[pipelineBody]](t, rec); len(page.Items) != 1 || !page.HasMore || page.Limit != 1 {
		t.Errorf("page = %+v", page)
	}
	wantStatus(t, call(t, h, http.MethodGet, base+"?limit=201", owner.Token, nil), http.StatusBadRequest, "limit too large")

	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+created.ID, owner.Token, nil), http.StatusNoContent, "delete")
	wantStatus(t, call(t, h, http.MethodGet, base+"/"+created.ID, owner.Token, nil), http.StatusNotFound, "get after delete")
	wantStatus(t, call(t, h, http.MethodPatch, base+"/"+created.ID, owner.Token, map[string]any{"name": "Z"}), http.StatusNotFound, "rename after delete")
	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+created.ID, owner.Token, nil), http.StatusNotFound, "delete after delete")
}

func TestStageRoutes(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	p := postPipeline(t, h, owner.Token, w.ID, "Stages")
	base := pipelinesPath(w.ID) + "/" + p.ID + "/stages"

	// The kind, color, and probability have defaults. Color is stored in lowercase.
	rec := call(t, h, http.MethodPost, base, owner.Token, map[string]any{"name": " Demo "})
	wantStatus(t, rec, http.StatusCreated, "create with defaults")
	demo := data[stageBody](t, rec)
	if demo.Name != "Demo" || demo.Kind != "open" || demo.Probability != 0 || !strings.HasPrefix(demo.Color, "#") || demo.Position != len(p.Stages) {
		t.Errorf("demo = %+v", demo)
	}
	rec = call(t, h, http.MethodPost, base, owner.Token, map[string]any{"name": "Signed", "kind": "won", "color": " #ABCDEF ", "position": 0})
	wantStatus(t, rec, http.StatusCreated, "create in full")
	signed := data[stageBody](t, rec)
	if signed.Kind != "won" || signed.Probability != 100 || signed.Color != "#abcdef" || signed.Position != 0 {
		t.Errorf("signed = %+v", signed)
	}
	rec = call(t, h, http.MethodPost, base, owner.Token, map[string]any{"name": "Beyond", "position": 9999})
	wantStatus(t, rec, http.StatusCreated, "create beyond the end")
	beyond := data[stageBody](t, rec)
	if beyond.Position != len(p.Stages)+2 {
		t.Errorf("beyond = %+v", beyond)
	}

	rec = call(t, h, http.MethodPatch, base+"/"+demo.ID, owner.Token, map[string]any{"name": "Demo call", "probability": 35, "color": "#000000", "kind": "lost"})
	wantStatus(t, rec, http.StatusOK, "update")
	if got := data[stageBody](t, rec); got.Name != "Demo call" || got.Probability != 35 || got.Color != "#000000" || got.Kind != "lost" {
		t.Errorf("update = %+v", got)
	}

	// Reorder takes the whole list.
	current := data[pipelineBody](t, call(t, h, http.MethodGet, pipelinesPath(w.ID)+"/"+p.ID, owner.Token, nil))
	ids := make([]string, len(current.Stages))
	for i, stage := range current.Stages {
		ids[i] = stage.ID
	}
	slices.Reverse(ids)
	rec = call(t, h, http.MethodPut, base+"/order", owner.Token, map[string]any{"stage_ids": ids})
	wantStatus(t, rec, http.StatusOK, "reorder")
	got := data[[]stageBody](t, rec)
	if len(got) != len(ids) || got[0].ID != ids[0] || got[0].Position != 0 {
		t.Errorf("reorder = %+v", got)
	}
	wantStatus(t, call(t, h, http.MethodPut, base+"/order", owner.Token, map[string]any{"stage_ids": ids[1:]}), http.StatusConflict, "reorder with a stage missing")
	wantStatus(t, call(t, h, http.MethodPut, base+"/order", owner.Token, map[string]any{"stage_ids": append(ids[1:], "not-a-uuid")}), http.StatusConflict, "reorder with a malformed ID")
	wantStatus(t, call(t, h, http.MethodPut, base+"/order", owner.Token, map[string]any{}), http.StatusBadRequest, "reorder with no list")
	wantStatus(t, call(t, h, http.MethodPut, base+"/order", owner.Token, map[string]any{"stage_ids": nil}), http.StatusBadRequest, "reorder with null")

	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+beyond.ID, owner.Token, nil), http.StatusNoContent, "delete")
	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+beyond.ID, owner.Token, nil), http.StatusNotFound, "delete after delete")
	wantStatus(t, call(t, h, http.MethodPatch, base+"/"+beyond.ID, owner.Token, map[string]any{"name": "Z"}), http.StatusNotFound, "update after delete")
}

func TestStageRulesAnswerConflict(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	p := postPipeline(t, h, owner.Token, w.ID, "One open left")
	base := pipelinesPath(w.ID) + "/" + p.ID + "/stages"

	var opens []string
	for _, stage := range p.Stages {
		if stage.Kind == "open" {
			opens = append(opens, stage.ID)
		}
	}
	for _, id := range opens[1:] {
		wantStatus(t, call(t, h, http.MethodDelete, base+"/"+id, owner.Token, nil), http.StatusNoContent, "delete a spare")
	}
	last := opens[0]
	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+last, owner.Token, nil), http.StatusConflict, "delete the last open stage")
	wantStatus(t, call(t, h, http.MethodPatch, base+"/"+last, owner.Token, map[string]any{"kind": "won"}), http.StatusConflict, "close the last open stage")

	// A stage or pipeline holding a live deal stays.
	rec := call(t, h, http.MethodPost, "/workspaces/"+w.ID+"/deals", owner.Token, map[string]any{"title": "Holder", "currency": "USD", "pipeline_id": p.ID})
	wantStatus(t, rec, http.StatusCreated, "create deal")
	deal := data[dealBody](t, rec)
	wantStatus(t, call(t, h, http.MethodDelete, pipelinesPath(w.ID)+"/"+p.ID, owner.Token, nil), http.StatusConflict, "delete a pipeline with a deal")
	wantStatus(t, call(t, h, http.MethodPost, base, owner.Token, map[string]any{"name": "New open"}), http.StatusCreated, "add another open stage")
	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+last, owner.Token, nil), http.StatusConflict, "delete a stage with a deal")
	wantStatus(t, call(t, h, http.MethodPatch, base+"/"+last, owner.Token, map[string]any{"kind": "lost"}), http.StatusConflict, "redefine a stage with a deal")
	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+w.ID+"/deals/"+deal.ID, owner.Token, nil), http.StatusNoContent, "delete the deal")
	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+last, owner.Token, nil), http.StatusNoContent, "delete the emptied stage")
	wantStatus(t, call(t, h, http.MethodDelete, pipelinesPath(w.ID)+"/"+p.ID, owner.Token, nil), http.StatusNoContent, "delete the emptied pipeline")

	// The workspace keeps its last pipeline, so a deal always has somewhere to start.
	remaining := firstPipeline(t, h, owner.Token, w.ID)
	rec = call(t, h, http.MethodDelete, pipelinesPath(w.ID)+"/"+remaining.ID, owner.Token, nil)
	wantStatus(t, rec, http.StatusConflict, "delete the last pipeline")
	if !strings.Contains(rec.Body.String(), "at least one pipeline") {
		t.Errorf("last pipeline: body = %s", rec.Body)
	}
}

func TestPipelineRoleLimitsOverHTTP(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	admin := joined(t, h, owner.Token, w.ID, "admin")
	member := joined(t, h, owner.Token, w.ID, "member")
	p := firstPipeline(t, h, owner.Token, w.ID)
	open := stageWithKind(t, p, "open")
	base := pipelinesPath(w.ID)
	item := base + "/" + p.ID
	stages := item + "/stages"

	for _, route := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, base, map[string]any{"name": "x"}},
		{http.MethodPatch, item, map[string]any{"name": "x"}},
		{http.MethodDelete, item, nil},
		{http.MethodPost, stages, map[string]any{"name": "x"}},
		{http.MethodPatch, stages + "/" + open.ID, map[string]any{"name": "x"}},
		{http.MethodDelete, stages + "/" + open.ID, nil},
		{http.MethodPut, stages + "/order", map[string]any{"stage_ids": []string{open.ID}}},
	} {
		wantStatus(t, call(t, h, route.method, route.path, member.Token, route.body), http.StatusForbidden, "member "+route.method+" "+route.path)
	}
	wantStatus(t, call(t, h, http.MethodGet, base, member.Token, nil), http.StatusOK, "member lists")
	wantStatus(t, call(t, h, http.MethodGet, item, member.Token, nil), http.StatusOK, "member reads")

	rec := call(t, h, http.MethodPost, base, admin.Token, map[string]any{"name": "By admin"})
	wantStatus(t, rec, http.StatusCreated, "admin creates")
	byAdmin := data[pipelineBody](t, rec)
	wantStatus(t, call(t, h, http.MethodPatch, base+"/"+byAdmin.ID, admin.Token, map[string]any{"name": "Renamed"}), http.StatusOK, "admin renames")
	wantStatus(t, call(t, h, http.MethodPost, base+"/"+byAdmin.ID+"/stages", admin.Token, map[string]any{"name": "Added"}), http.StatusCreated, "admin adds a stage")
	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+byAdmin.ID, admin.Token, nil), http.StatusNoContent, "admin deletes")
}

func TestPipelineAndStageInputValidation(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	p := firstPipeline(t, h, owner.Token, w.ID)
	open := stageWithKind(t, p, "open")
	item := pipelinesPath(w.ID) + "/" + p.ID
	stages := item + "/stages"
	long := func(n int) string { return strings.Repeat("x", n) }

	for _, tt := range []struct {
		name, method, path string
		body               any
	}{
		{"create pipeline: no name", http.MethodPost, pipelinesPath(w.ID), map[string]any{}},
		{"create pipeline: blank name", http.MethodPost, pipelinesPath(w.ID), map[string]any{"name": "  "}},
		{"create pipeline: long name", http.MethodPost, pipelinesPath(w.ID), map[string]any{"name": long(201)}},
		{"create pipeline: NUL", http.MethodPost, pipelinesPath(w.ID), map[string]any{"name": "a\x00b"}},
		{"create pipeline: null name", http.MethodPost, pipelinesPath(w.ID), map[string]any{"name": nil}},
		{"rename: no fields", http.MethodPatch, item, map[string]any{}},
		{"rename: blank", http.MethodPatch, item, map[string]any{"name": " "}},
		{"rename: null", http.MethodPatch, item, map[string]any{"name": nil}},
		{"create stage: no name", http.MethodPost, stages, map[string]any{}},
		{"create stage: long name", http.MethodPost, stages, map[string]any{"name": long(101)}},
		{"create stage: NUL", http.MethodPost, stages, map[string]any{"name": "a\x00b"}},
		{"create stage: bad color", http.MethodPost, stages, map[string]any{"name": "x", "color": "red"}},
		{"create stage: short color", http.MethodPost, stages, map[string]any{"name": "x", "color": "#fff"}},
		{"create stage: probability above", http.MethodPost, stages, map[string]any{"name": "x", "probability": 101}},
		{"create stage: probability below", http.MethodPost, stages, map[string]any{"name": "x", "probability": -1}},
		{"create stage: fractional probability", http.MethodPost, stages, map[string]any{"name": "x", "probability": 1.5}},
		{"create stage: unknown kind", http.MethodPost, stages, map[string]any{"name": "x", "kind": "closed"}},
		{"create stage: negative position", http.MethodPost, stages, map[string]any{"name": "x", "position": -1}},
		{"update stage: no fields", http.MethodPatch, stages + "/" + open.ID, map[string]any{}},
		{"update stage: null name", http.MethodPatch, stages + "/" + open.ID, map[string]any{"name": nil}},
		{"update stage: null color", http.MethodPatch, stages + "/" + open.ID, map[string]any{"color": nil}},
		{"update stage: null kind", http.MethodPatch, stages + "/" + open.ID, map[string]any{"kind": nil}},
		{"update stage: null probability", http.MethodPatch, stages + "/" + open.ID, map[string]any{"probability": nil}},
		{"update stage: position goes through the order route", http.MethodPatch, stages + "/" + open.ID, map[string]any{"position": 1}},
		{"update stage: bad color", http.MethodPatch, stages + "/" + open.ID, map[string]any{"color": "blue"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantStatus(t, call(t, h, tt.method, tt.path, owner.Token, tt.body), http.StatusBadRequest, tt.name)
		})
	}
	t.Run("invalid UTF-8", func(t *testing.T) {
		body := []byte("{\"name\":\"a\xffb\"}")
		wantStatus(t, callRaw(t, h, http.MethodPost, pipelinesPath(w.ID), owner.Token, body), http.StatusBadRequest, "pipeline")
		wantStatus(t, callRaw(t, h, http.MethodPost, stages, owner.Token, body), http.StatusBadRequest, "stage")
	})
	got := firstPipeline(t, h, owner.Token, w.ID)
	if len(got.Stages) != len(p.Stages) || got.Stages[0].Name != open.Name {
		t.Errorf("a refused request changed the pipeline: %+v", got)
	}
}

// Everything out of reach gets the same bytes back, so existence does not leak.
func TestPipelinesAndStagesOutOfReachAnswerAlike(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	stranger := register(t, h, uniqueEmail(t))
	mine := createWorkspace(t, h, owner.Token)
	theirs := createWorkspace(t, h, stranger.Token)
	ofTheirs := firstPipeline(t, h, stranger.Token, theirs.ID)
	ofMine := firstPipeline(t, h, owner.Token, mine.ID)
	sibling := postPipeline(t, h, owner.Token, mine.ID, "Sibling")
	deleted := postPipeline(t, h, owner.Token, mine.ID, "Deleted")
	wantStatus(t, call(t, h, http.MethodDelete, pipelinesPath(mine.ID)+"/"+deleted.ID, owner.Token, nil), http.StatusNoContent, "delete pipeline")
	deletedStage := data[stageBody](t, call(t, h, http.MethodPost, pipelinesPath(mine.ID)+"/"+ofMine.ID+"/stages", owner.Token, map[string]any{"name": "Gone"}))
	wantStatus(t, call(t, h, http.MethodDelete, pipelinesPath(mine.ID)+"/"+ofMine.ID+"/stages/"+deletedStage.ID, owner.Token, nil), http.StatusNoContent, "delete stage")

	pipelineIDs := map[string]string{"another workspace": ofTheirs.ID, "soft-deleted": deleted.ID, "nonexistent": unknownUUID}
	for _, bad := range malformedIDs {
		pipelineIDs["malformed "+bad] = bad
	}
	want := fmt.Sprintf("{\"error\":%q}\n", "pipeline not found")
	for name, id := range pipelineIDs {
		for _, route := range []struct {
			method, suffix string
			body           any
		}{
			{http.MethodGet, "", nil},
			{http.MethodPatch, "", map[string]any{"name": "x"}},
			{http.MethodDelete, "", nil},
			{http.MethodPost, "/stages", map[string]any{"name": "x"}},
			{http.MethodPut, "/stages/order", map[string]any{"stage_ids": []string{}}},
			{http.MethodPatch, "/stages/" + ofTheirs.Stages[0].ID, map[string]any{"name": "x"}},
			{http.MethodDelete, "/stages/" + ofTheirs.Stages[0].ID, nil},
		} {
			rec := call(t, h, route.method, pipelinesPath(mine.ID)+"/"+id+route.suffix, owner.Token, route.body)
			if rec.Code != http.StatusNotFound || rec.Body.String() != want {
				t.Errorf("%s %s (%s) = %d %q, want 404 %q", route.method, route.suffix, name, rec.Code, rec.Body, want)
			}
		}
	}

	stageIDs := map[string]string{"another workspace": ofTheirs.Stages[0].ID, "another pipeline": sibling.Stages[0].ID, "soft-deleted": deletedStage.ID, "nonexistent": unknownUUID}
	for _, bad := range malformedIDs {
		stageIDs["malformed "+bad] = bad
	}
	want = fmt.Sprintf("{\"error\":%q}\n", "stage not found")
	for name, id := range stageIDs {
		path := pipelinesPath(mine.ID) + "/" + ofMine.ID + "/stages/" + id
		for _, rec := range []*httptest.ResponseRecorder{
			call(t, h, http.MethodPatch, path, owner.Token, map[string]any{"name": "x"}),
			call(t, h, http.MethodDelete, path, owner.Token, nil),
		} {
			if rec.Code != http.StatusNotFound || rec.Body.String() != want {
				t.Errorf("stage %s = %d %q, want 404 %q", name, rec.Code, rec.Body, want)
			}
		}
	}
	// Nothing of theirs changed.
	if got := firstPipeline(t, h, stranger.Token, theirs.ID); got.Name != ofTheirs.Name || got.Stages[0].Name != ofTheirs.Stages[0].Name {
		t.Errorf("their pipeline = %+v", got)
	}
}

func TestNonMemberGetsTheWorkspace404OnEveryPipelineAndDealRoute(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	stranger := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	p := firstPipeline(t, h, owner.Token, w.ID)
	stage := p.Stages[0]
	deal := data[dealBody](t, call(t, h, http.MethodPost, "/workspaces/"+w.ID+"/deals", owner.Token, map[string]any{"title": "x", "currency": "USD", "pipeline_id": p.ID}))
	want := fmt.Sprintf("{\"error\":%q}\n", msgWorkspaceNotFound)
	root := "/workspaces/" + w.ID
	for _, route := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, root + "/pipelines", nil},
		{http.MethodPost, root + "/pipelines", map[string]any{"name": "x"}},
		{http.MethodGet, root + "/pipelines/" + p.ID, nil},
		{http.MethodPatch, root + "/pipelines/" + p.ID, map[string]any{"name": "x"}},
		{http.MethodDelete, root + "/pipelines/" + p.ID, nil},
		{http.MethodPost, root + "/pipelines/" + p.ID + "/stages", map[string]any{"name": "x"}},
		{http.MethodPut, root + "/pipelines/" + p.ID + "/stages/order", map[string]any{"stage_ids": []string{}}},
		{http.MethodPatch, root + "/pipelines/" + p.ID + "/stages/" + stage.ID, map[string]any{"name": "x"}},
		{http.MethodDelete, root + "/pipelines/" + p.ID + "/stages/" + stage.ID, nil},
		{http.MethodGet, root + "/deals", nil},
		{http.MethodPost, root + "/deals", map[string]any{"title": "x", "currency": "USD", "pipeline_id": p.ID}},
		{http.MethodGet, root + "/deals/" + deal.ID, nil},
		{http.MethodPatch, root + "/deals/" + deal.ID, map[string]any{"title": "x"}},
		{http.MethodDelete, root + "/deals/" + deal.ID, nil},
		{http.MethodPost, root + "/deals/" + deal.ID + "/move", map[string]any{"stage_id": stage.ID}},
	} {
		rec := call(t, h, route.method, route.path, stranger.Token, route.body)
		if rec.Code != http.StatusNotFound || rec.Body.String() != want {
			t.Errorf("%s %s = %d %q, want 404 %q", route.method, route.path, rec.Code, rec.Body, want)
		}
		wantStatus(t, call(t, h, route.method, route.path, "", route.body), http.StatusUnauthorized, "no token: "+route.path)
	}
}
