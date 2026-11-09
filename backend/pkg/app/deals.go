package app

import (
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"crm/pkg/model"
	"crm/pkg/store"

	"github.com/go-chi/chi/v5"
)

const (
	dealParam = "dealID"

	maxDealTitleRunes  = 200
	maxLostReasonRunes = 500
	currencyLetters    = 3
	// Well below 2^53, so a JSON client that reads numbers as doubles cannot lose a
	// cent. The schema has the same bound.
	maxDealValueMinor = 999_999_999_999_999
	minCloseYear      = 1900
	maxCloseYear      = 2200

	msgInvalidTitle    = "a title of at most 200 characters is required"
	msgInvalidCurrency = "currency must be a three-letter code such as USD"
	msgInvalidFilter   = "pipeline_id, stage_id, and owner_id must be IDs, and status open, won, or lost"
	msgInvalidPipeline = "pipeline_id is required"
)

// dealRequest reads a deal's fields. The stage, position, and lost reason are here only
// to refuse them where they cannot be set: they change through a move, which is what
// keeps a deal's closed state in step with its stage.
type dealRequest struct {
	Title             optionalString `json:"title"`
	ValueMinor        optionalInt    `json:"value_minor"`
	Currency          optionalString `json:"currency"`
	ExpectedCloseDate optionalString `json:"expected_close_date"`
	CompanyID         optionalString `json:"company_id"`
	ContactID         optionalString `json:"contact_id"`
	OwnerID           optionalString `json:"owner_id"`
	PipelineID        optionalString `json:"pipeline_id"`
	StageID           optionalString `json:"stage_id"`
	Position          optionalInt    `json:"position"`
	LostReason        optionalString `json:"lost_reason"`
}

func validCurrency(code string) bool {
	if len(code) != currencyLetters {
		return false
	}
	return !strings.ContainsFunc(code, func(c rune) bool { return c < 'A' || c > 'Z' })
}

func validCloseDate(date string) bool {
	parsed, err := time.Parse(time.DateOnly, date)
	return err == nil && parsed.Year() >= minCloseYear && parsed.Year() <= maxCloseYear
}

// normalize trims and checks the fields that are present, and returns what is wrong
// with them, if anything. References are not checked: a malformed one is simply
// something that cannot be found, with the same answer as any other.
func (d *dealRequest) normalize() string {
	if optionalNUL(d.Title.Value, d.Currency.Value, d.ExpectedCloseDate.Value) {
		return msgInvalidBody
	}
	if !requiredFits(d.Title.Value, maxDealTitleRunes) {
		return msgInvalidTitle
	}
	if v := d.ValueMinor.Value; v != nil && (*v < 0 || *v > maxDealValueMinor) {
		return "value_minor must be a whole number from 0 to 999999999999999"
	}
	if d.Currency.Value != nil {
		*d.Currency.Value = strings.ToUpper(strings.TrimSpace(*d.Currency.Value))
		if !validCurrency(*d.Currency.Value) {
			return msgInvalidCurrency
		}
	}
	if d.ExpectedCloseDate.Value != nil && !validCloseDate(*d.ExpectedCloseDate.Value) {
		return "expected_close_date must be a date such as 2027-03-14, from 1900 to 2200"
	}
	return ""
}

func decodeDeal(w http.ResponseWriter, r *http.Request) (dealRequest, bool) {
	var req dealRequest
	if err := decodeText(w, r, &req); err != nil {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return dealRequest{}, false
	}
	if problem := req.normalize(); problem != "" {
		errResp(w, http.StatusBadRequest, problem)
		return dealRequest{}, false
	}
	return req, true
}

// dealFilter reads the filters of a list. Like the list arguments, anything invalid is
// refused rather than ignored, so a client bug shows up.
func dealFilter(r *http.Request) (model.DealFilter, bool) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return model.DealFilter{}, false
	}
	var filter model.DealFilter
	for _, f := range []struct {
		name string
		dest *string
	}{
		{"pipeline_id", &filter.PipelineID},
		{"stage_id", &filter.StageID},
		{"owner_id", &filter.OwnerID},
	} {
		if query.Has(f.name) {
			*f.dest = query.Get(f.name)
			if !store.ValidID(*f.dest) {
				return model.DealFilter{}, false
			}
		}
	}
	if query.Has("status") {
		filter.Status = model.StageKind(query.Get("status"))
		if !filter.Status.Valid() {
			return model.DealFilter{}, false
		}
	}
	return filter, true
}

func (a *App) listDeals(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	params, ok := listRequest(w, r)
	if !ok {
		return
	}
	filter, ok := dealFilter(r)
	if !ok {
		errResp(w, http.StatusBadRequest, msgInvalidFilter)
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	page, err := a.store.ListDeals(ctx, workspace.ID, filter, params)
	if err != nil {
		crmFailure(w, "list deals", err)
		return
	}
	dataResp(w, http.StatusOK, page)
}

func (a *App) createDeal(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	req, ok := decodeDeal(w, r)
	if !ok {
		return
	}
	problem := ""
	switch {
	case req.Title.Value == nil:
		problem = msgInvalidTitle
	case req.Currency.Value == nil:
		problem = msgInvalidCurrency
	case req.PipelineID.Value == nil:
		problem = msgInvalidPipeline
	case req.Position.Set || req.LostReason.Set:
		problem = "a new deal goes at the end of an open stage; position and lost_reason change through a move"
	}
	if problem != "" {
		errResp(w, http.StatusBadRequest, problem)
		return
	}
	in := model.DealInput{
		PipelineID: *req.PipelineID.Value, StageID: req.StageID.Value, Title: *req.Title.Value, Currency: *req.Currency.Value,
		ExpectedCloseDate: req.ExpectedCloseDate.Value, CompanyID: req.CompanyID.Value, ContactID: req.ContactID.Value, OwnerID: req.OwnerID.Value,
	}
	if in.OwnerID == nil {
		in.OwnerID = &userID
	}
	if req.ValueMinor.Value != nil {
		in.ValueMinor = *req.ValueMinor.Value
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	deal, err := a.store.CreateDeal(ctx, workspace.ID, in)
	if err != nil {
		crmFailure(w, "create deal", err)
		return
	}
	dataResp(w, http.StatusCreated, deal)
}

func (a *App) getDeal(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	deal, err := a.store.GetDeal(ctx, workspace.ID, chi.URLParam(r, dealParam))
	if err != nil {
		crmFailure(w, "get deal", err)
		return
	}
	dataResp(w, http.StatusOK, deal)
}

// updateProblem returns what is wrong with a patch as a whole, if anything.
func (d *dealRequest) updateProblem() string {
	if problem := nullProblem([]namedText{{"title", d.Title}, {"currency", d.Currency}}); problem != "" {
		return problem
	}
	if d.ValueMinor.Set && d.ValueMinor.Value == nil {
		return "value_minor must not be null"
	}
	if d.PipelineID.Set || d.StageID.Set || d.Position.Set || d.LostReason.Set {
		return "a deal stays in its pipeline; stage_id, position, and lost_reason change through a move"
	}
	if !d.Title.Set && !d.ValueMinor.Set && !d.Currency.Set && !d.ExpectedCloseDate.Set && !d.CompanyID.Set && !d.ContactID.Set && !d.OwnerID.Set {
		return msgNoFields
	}
	return ""
}

func (a *App) updateDeal(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	req, ok := decodeDeal(w, r)
	if !ok {
		return
	}
	if problem := req.updateProblem(); problem != "" {
		errResp(w, http.StatusBadRequest, problem)
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	deal, err := a.store.UpdateDeal(ctx, workspace.ID, chi.URLParam(r, dealParam), model.DealPatch{
		Title: req.Title.Value, ValueMinor: req.ValueMinor.Value, Currency: req.Currency.Value,
		SetExpectedClose: req.ExpectedCloseDate.Set, ExpectedCloseDate: req.ExpectedCloseDate.Value,
		SetCompany: req.CompanyID.Set, CompanyID: req.CompanyID.Value,
		SetContact: req.ContactID.Set, ContactID: req.ContactID.Value,
		SetOwner: req.OwnerID.Set, OwnerID: req.OwnerID.Value,
	})
	if err != nil {
		crmFailure(w, "update deal", err)
		return
	}
	dataResp(w, http.StatusOK, deal)
}

func (a *App) moveDeal(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	var req struct {
		StageID    optionalString `json:"stage_id"`
		Position   optionalInt    `json:"position"`
		LostReason optionalString `json:"lost_reason"`
	}
	if err := decodeText(w, r, &req); err != nil {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	reason := strings.TrimSpace(deref(req.LostReason.Value))
	problem := ""
	switch {
	case req.StageID.Value == nil:
		problem = "stage_id is required"
	case req.Position.Value != nil && *req.Position.Value < 0:
		problem = "position must not be negative"
	case hasNUL(reason):
		problem = msgInvalidBody
	case utf8.RuneCountInString(reason) > maxLostReasonRunes:
		problem = "lost_reason must be at most 500 characters"
	}
	if problem != "" {
		errResp(w, http.StatusBadRequest, problem)
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	deal, err := a.store.MoveDeal(ctx, workspace.ID, chi.URLParam(r, dealParam), model.DealMove{
		StageID: *req.StageID.Value, Position: req.Position.intValue(), LostReason: reason,
	})
	if err != nil {
		crmFailure(w, "move deal", err)
		return
	}
	dataResp(w, http.StatusOK, deal)
}

func (a *App) deleteDeal(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	if err := a.store.DeleteDeal(ctx, workspace.ID, chi.URLParam(r, dealParam)); err != nil {
		crmFailure(w, "delete deal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
