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
	FirstName optionalString `json:"first_name"`
	LastName  optionalString `json:"last_name"`
	Email     optionalString `json:"email"`
	Phone     optionalString `json:"phone"`
	Title     optionalString `json:"title"`
	CompanyID optionalString `json:"company_id"`
}

func (c *contactRequest) empty() bool {
	return !c.FirstName.Set && !c.LastName.Set && !c.Email.Set && !c.Phone.Set && !c.Title.Set && !c.CompanyID.Set
}

func (c *contactRequest) nullProblem() string {
	return nullProblem([]namedText{{"first_name", c.FirstName}, {"last_name", c.LastName}, {"email", c.Email}, {"phone", c.Phone}, {"title", c.Title}})
}

// normalize trims and checks the fields that are present, and returns what is
// wrong with them, if anything. The company ID is not checked: a malformed one is
// simply a company that cannot be found, with the same answer as any other.
func (c *contactRequest) normalize() string {
	switch {
	case optionalNUL(c.FirstName.Value, c.LastName.Value, c.Email.Value, c.Phone.Value, c.Title.Value):
		return msgInvalidBody
	case !requiredFits(c.FirstName.Value, maxContactNameRunes):
		return "a first name of at most 100 characters is required"
	case !trimmedFits(c.LastName.Value, maxContactNameRunes):
		return "last name must be at most 100 characters"
	case !validOptionalEmail(c.Email.Value):
		return msgInvalidEmail
	case !validOptionalPhone(c.Phone.Value):
		return msgInvalidPhone
	case !trimmedFits(c.Title.Value, maxTitleRunes):
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
	if req.FirstName.Value == nil {
		errResp(w, http.StatusBadRequest, "a first name of at most 100 characters is required")
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	contact, err := a.store.CreateContact(ctx, workspace.ID, model.ContactInput{
		FirstName: *req.FirstName.Value, LastName: deref(req.LastName.Value), Email: deref(req.Email.Value), Phone: deref(req.Phone.Value), Title: deref(req.Title.Value),
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
	if problem := req.nullProblem(); problem != "" {
		errResp(w, http.StatusBadRequest, problem)
		return
	}
	if req.empty() {
		errResp(w, http.StatusBadRequest, msgNoFields)
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	contact, err := a.store.UpdateContact(ctx, workspace.ID, chi.URLParam(r, contactParam), model.ContactPatch{
		FirstName: req.FirstName.Value, LastName: req.LastName.Value, Email: req.Email.Value, Phone: req.Phone.Value, Title: req.Title.Value,
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
