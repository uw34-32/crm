package app

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"crm/pkg/model"

	"github.com/go-chi/chi/v5"
)

const (
	pipelineParam = "pipelineID"
	stageParam    = "stageID"

	maxStageNameRunes = 100
	maxProbability    = 100
	wonProbability    = 100
	defaultStageColor = "#64748b"

	msgInvalidPipelineName = "a name of at most 200 characters is required"
	msgInvalidStageName    = "a name of at most 100 characters is required"
)

func decodePipelineName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeText(w, r, &req); err != nil || hasNUL(req.Name) {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return "", false
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > maxNameRunes {
		errResp(w, http.StatusBadRequest, msgInvalidPipelineName)
		return "", false
	}
	return name, true
}

type stageRequest struct {
	Name        optionalString `json:"name"`
	Color       optionalString `json:"color"`
	Kind        optionalString `json:"kind"`
	Probability optionalInt    `json:"probability"`
	Position    optionalInt    `json:"position"`
}

// validColor reports whether s is #rrggbb in lowercase.
func validColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	return !strings.ContainsFunc(s[1:], func(c rune) bool {
		return (c < '0' || c > '9') && (c < 'a' || c > 'f')
	})
}

// normalize trims and checks the fields that are present, and returns what is wrong
// with them, if anything.
func (s *stageRequest) normalize() string {
	if optionalNUL(s.Name.Value, s.Color.Value, s.Kind.Value) {
		return msgInvalidBody
	}
	if !requiredFits(s.Name.Value, maxStageNameRunes) {
		return msgInvalidStageName
	}
	if s.Color.Value != nil {
		*s.Color.Value = strings.ToLower(strings.TrimSpace(*s.Color.Value))
		if !validColor(*s.Color.Value) {
			return "color must look like #1a2b3c"
		}
	}
	if s.Kind.Value != nil && !model.StageKind(*s.Kind.Value).Valid() {
		return "kind must be open, won, or lost"
	}
	if p := s.Probability.Value; p != nil && (*p < 0 || *p > maxProbability) {
		return "probability must be 0 to 100"
	}
	if p := s.Position.Value; p != nil && *p < 0 {
		return "position must not be negative"
	}
	return ""
}

func decodeStage(w http.ResponseWriter, r *http.Request) (stageRequest, bool) {
	var req stageRequest
	if err := decodeText(w, r, &req); err != nil {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return stageRequest{}, false
	}
	if problem := req.normalize(); problem != "" {
		errResp(w, http.StatusBadRequest, problem)
		return stageRequest{}, false
	}
	return req, true
}

func (a *App) listPipelines(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	params, ok := listRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	page, err := a.store.ListPipelines(ctx, workspace.ID, params)
	if err != nil {
		crmFailure(w, "list pipelines", err)
		return
	}
	dataResp(w, http.StatusOK, page)
}

func (a *App) createPipeline(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	name, ok := decodePipelineName(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	pipeline, err := a.store.CreatePipeline(ctx, workspace.ID, userID, name)
	if err != nil {
		crmFailure(w, "create pipeline", err)
		return
	}
	dataResp(w, http.StatusCreated, pipeline)
}

func (a *App) getPipeline(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	pipeline, err := a.store.GetPipeline(ctx, workspace.ID, chi.URLParam(r, pipelineParam))
	if err != nil {
		crmFailure(w, "get pipeline", err)
		return
	}
	dataResp(w, http.StatusOK, pipeline)
}

func (a *App) renamePipeline(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	name, ok := decodePipelineName(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	pipeline, err := a.store.RenamePipeline(ctx, workspace.ID, userID, chi.URLParam(r, pipelineParam), name)
	if err != nil {
		crmFailure(w, "rename pipeline", err)
		return
	}
	dataResp(w, http.StatusOK, pipeline)
}

func (a *App) deletePipeline(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	if err := a.store.DeletePipeline(ctx, workspace.ID, userID, chi.URLParam(r, pipelineParam)); err != nil {
		crmFailure(w, "delete pipeline", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) createStage(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	req, ok := decodeStage(w, r)
	if !ok {
		return
	}
	if req.Name.Value == nil {
		errResp(w, http.StatusBadRequest, msgInvalidStageName)
		return
	}
	in := model.StageInput{Name: *req.Name.Value, Color: defaultStageColor, Kind: model.StageOpen, Position: req.Position.intValue()}
	if req.Color.Value != nil {
		in.Color = *req.Color.Value
	}
	if req.Kind.Value != nil {
		in.Kind = model.StageKind(*req.Kind.Value)
	}
	if in.Kind == model.StageWon {
		in.Probability = wonProbability
	}
	if p := req.Probability.intValue(); p != nil {
		in.Probability = *p
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	stage, err := a.store.CreateStage(ctx, workspace.ID, userID, chi.URLParam(r, pipelineParam), in)
	if err != nil {
		crmFailure(w, "create stage", err)
		return
	}
	dataResp(w, http.StatusCreated, stage)
}

func (a *App) updateStage(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	req, ok := decodeStage(w, r)
	if !ok {
		return
	}
	problem := nullProblem([]namedText{{"name", req.Name}, {"color", req.Color}, {"kind", req.Kind}})
	if problem == "" && req.Probability.Set && req.Probability.Value == nil {
		problem = "probability must not be null"
	}
	if problem == "" && req.Position.Set {
		problem = "position changes through the stage order route"
	}
	if problem == "" && !req.Name.Set && !req.Color.Set && !req.Kind.Set && !req.Probability.Set {
		problem = msgNoFields
	}
	if problem != "" {
		errResp(w, http.StatusBadRequest, problem)
		return
	}
	patch := model.StagePatch{Name: req.Name.Value, Color: req.Color.Value, Probability: req.Probability.intValue()}
	if req.Kind.Value != nil {
		kind := model.StageKind(*req.Kind.Value)
		patch.Kind = &kind
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	stage, err := a.store.UpdateStage(ctx, workspace.ID, userID, chi.URLParam(r, pipelineParam), chi.URLParam(r, stageParam), patch)
	if err != nil {
		crmFailure(w, "update stage", err)
		return
	}
	dataResp(w, http.StatusOK, stage)
}

func (a *App) reorderStages(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	var req struct {
		StageIDs []string `json:"stage_ids"`
	}
	if err := decodeText(w, r, &req); err != nil {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	if req.StageIDs == nil {
		errResp(w, http.StatusBadRequest, "stage_ids must list every stage of the pipeline")
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	stages, err := a.store.ReorderStages(ctx, workspace.ID, userID, chi.URLParam(r, pipelineParam), req.StageIDs)
	if err != nil {
		crmFailure(w, "reorder stages", err)
		return
	}
	dataResp(w, http.StatusOK, stages)
}

func (a *App) deleteStage(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	if err := a.store.DeleteStage(ctx, workspace.ID, userID, chi.URLParam(r, pipelineParam), chi.URLParam(r, stageParam)); err != nil {
		crmFailure(w, "delete stage", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
