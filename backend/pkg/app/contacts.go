package app

import (
	"net/http"

	"crm/pkg/model"

	"github.com/go-chi/chi/v5"
)

const (
	contactParam = "contactID"

	maxContactNameRunes = 100
	maxTitleRunes       = 200
)

// contactRequest has no field for external_contact_id: nothing reads it yet, so
// clients have no reason to write it.
type contactRequest struct {
	FirstName *string    `json:"first_name"`
	LastName  *string    `json:"last_name"`
	Email     *string    `json:"email"`
	Phone     *string    `json:"phone"`
	Title     *string    `json:"title"`
	CompanyID optionalID `json:"company_id"`
}

func (c *contactRequest) empty() bool {
	return c.FirstName == nil && c.LastName == nil && c.Email == nil && c.Phone == nil && c.Title == nil && !c.CompanyID.Set
}

// normalize trims and checks the fields that are present, and returns what is
// wrong with them, if anything. The company ID is not checked: a malformed one is
// simply a company that cannot be found, with the same answer as any other.
func (c *contactRequest) normalize() string {
	switch {
	case optionalNUL(c.FirstName, c.LastName, c.Email, c.Phone, c.Title):
		return msgInvalidBody
	case !requiredFits(c.FirstName, maxContactNameRunes):
		return "a first name of at most 100 characters is required"
	case !trimmedFits(c.LastName, maxContactNameRunes):
		return "last name must be at most 100 characters"
	case !validOptionalEmail(c.Email):
		return msgInvalidEmail
	case !validOptionalPhone(c.Phone):
		return msgInvalidPhone
	case !trimmedFits(c.Title, maxTitleRunes):
		return "title must be at most 200 characters"
	}
	return ""
}

func decodeContact(w http.ResponseWriter, r *http.Request) (contactRequest, bool) {
	var req contactRequest
	if err := decodeJSON(w, r, &req); err != nil {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return contactRequest{}, false
	}
	if problem := req.normalize(); problem != "" {
		errResp(w, http.StatusBadRequest, problem)
		return contactRequest{}, false
	}
	return req, true
}

func (a *App) listContacts(w http.ResponseWriter, r *http.Request) {
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
	page, err := a.store.ListContacts(ctx, workspace.ID, params)
	if err != nil {
		crmFailure(w, "list contacts", err)
		return
	}
	dataResp(w, http.StatusOK, page)
}

func (a *App) createContact(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	req, ok := decodeContact(w, r)
	if !ok {
		return
	}
	if req.FirstName == nil {
		errResp(w, http.StatusBadRequest, "a first name of at most 100 characters is required")
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	contact, err := a.store.CreateContact(ctx, workspace.ID, model.ContactInput{
		FirstName: *req.FirstName, LastName: deref(req.LastName), Email: deref(req.Email), Phone: deref(req.Phone), Title: deref(req.Title),
		CompanyID: req.CompanyID.Value,
	})
	if err != nil {
		crmFailure(w, "create contact", err)
		return
	}
	dataResp(w, http.StatusCreated, contact)
}

func (a *App) getContact(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	contact, err := a.store.GetContact(ctx, workspace.ID, chi.URLParam(r, contactParam))
	if err != nil {
		crmFailure(w, "get contact", err)
		return
	}
	dataResp(w, http.StatusOK, contact)
}

func (a *App) updateContact(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	req, ok := decodeContact(w, r)
	if !ok {
		return
	}
	if req.empty() {
		errResp(w, http.StatusBadRequest, msgNoFields)
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	contact, err := a.store.UpdateContact(ctx, workspace.ID, chi.URLParam(r, contactParam), model.ContactPatch{
		FirstName: req.FirstName, LastName: req.LastName, Email: req.Email, Phone: req.Phone, Title: req.Title,
		SetCompany: req.CompanyID.Set, CompanyID: req.CompanyID.Value,
	})
	if err != nil {
		crmFailure(w, "update contact", err)
		return
	}
	dataResp(w, http.StatusOK, contact)
}

func (a *App) deleteContact(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	if err := a.store.DeleteContact(ctx, workspace.ID, chi.URLParam(r, contactParam)); err != nil {
		crmFailure(w, "delete contact", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
