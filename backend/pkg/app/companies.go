package app

import (
	"net/http"

	"crm/pkg/model"

	"github.com/go-chi/chi/v5"
)

const (
	companyParam = "companyID"

	maxIndustryRunes = 100
	maxAddressRunes  = 500
)

type companyRequest struct {
	Name     *string `json:"name"`
	Domain   *string `json:"domain"`
	Phone    *string `json:"phone"`
	Industry *string `json:"industry"`
	Address  *string `json:"address"`
}

func (c *companyRequest) empty() bool {
	return c.Name == nil && c.Domain == nil && c.Phone == nil && c.Industry == nil && c.Address == nil
}

// normalize trims and checks the fields that are present, and returns what is
// wrong with them, if anything.
func (c *companyRequest) normalize() string {
	switch {
	case optionalNUL(c.Name, c.Domain, c.Phone, c.Industry, c.Address):
		return msgInvalidBody
	case !requiredFits(c.Name, maxNameRunes):
		return "a name of at most 200 characters is required"
	case !validOptionalDomain(c.Domain):
		return "domain must be a host name such as example.com"
	case !validOptionalPhone(c.Phone):
		return msgInvalidPhone
	case !trimmedFits(c.Industry, maxIndustryRunes):
		return "industry must be at most 100 characters"
	case !trimmedFits(c.Address, maxAddressRunes):
		return "address must be at most 500 characters"
	}
	return ""
}

func decodeCompany(w http.ResponseWriter, r *http.Request) (companyRequest, bool) {
	var req companyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return companyRequest{}, false
	}
	if problem := req.normalize(); problem != "" {
		errResp(w, http.StatusBadRequest, problem)
		return companyRequest{}, false
	}
	return req, true
}

func deref(field *string) string {
	if field == nil {
		return ""
	}
	return *field
}

func (a *App) listCompanies(w http.ResponseWriter, r *http.Request) {
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
	page, err := a.store.ListCompanies(ctx, workspace.ID, params)
	if err != nil {
		crmFailure(w, "list companies", err)
		return
	}
	dataResp(w, http.StatusOK, page)
}

func (a *App) createCompany(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	req, ok := decodeCompany(w, r)
	if !ok {
		return
	}
	if req.Name == nil {
		errResp(w, http.StatusBadRequest, "a name of at most 200 characters is required")
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	company, err := a.store.CreateCompany(ctx, workspace.ID, model.CompanyInput{
		Name: *req.Name, Domain: deref(req.Domain), Phone: deref(req.Phone), Industry: deref(req.Industry), Address: deref(req.Address),
	})
	if err != nil {
		crmFailure(w, "create company", err)
		return
	}
	dataResp(w, http.StatusCreated, company)
}

func (a *App) getCompany(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	company, err := a.store.GetCompany(ctx, workspace.ID, chi.URLParam(r, companyParam))
	if err != nil {
		crmFailure(w, "get company", err)
		return
	}
	dataResp(w, http.StatusOK, company)
}

func (a *App) updateCompany(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	req, ok := decodeCompany(w, r)
	if !ok {
		return
	}
	if req.empty() {
		errResp(w, http.StatusBadRequest, msgNoFields)
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	company, err := a.store.UpdateCompany(ctx, workspace.ID, chi.URLParam(r, companyParam), model.CompanyPatch{
		Name: req.Name, Domain: req.Domain, Phone: req.Phone, Industry: req.Industry, Address: req.Address,
	})
	if err != nil {
		crmFailure(w, "update company", err)
		return
	}
	dataResp(w, http.StatusOK, company)
}

func (a *App) deleteCompany(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	if err := a.store.DeleteCompany(ctx, workspace.ID, chi.URLParam(r, companyParam)); err != nil {
		crmFailure(w, "delete company", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
