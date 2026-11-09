package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
)

type companyBody struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Domain      string `json:"domain"`
	Phone       string `json:"phone"`
	Industry    string `json:"industry"`
	Address     string `json:"address"`
}

type contactBody struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	FirstName   string  `json:"first_name"`
	LastName    string  `json:"last_name"`
	Email       string  `json:"email"`
	Phone       string  `json:"phone"`
	Title       string  `json:"title"`
	CompanyID   *string `json:"company_id"`
	CompanyName *string `json:"company_name"`
}

type pageBody[T any] struct {
	Items   []T  `json:"items"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

const unknownUUID = "00000000-0000-4000-8000-000000000000"

// malformedIDs reach the handler as path segments, already URL-safe.
var malformedIDs = []string{"not-a-uuid", "1", "00000000-0000-4000-8000-00000000000g", "%00", "%27%3B--"}

func postCompany(t *testing.T, h http.Handler, token, workspaceID string, body map[string]any) companyBody {
	t.Helper()
	rec := call(t, h, http.MethodPost, "/workspaces/"+workspaceID+"/companies", token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create company = %d: %s", rec.Code, rec.Body)
	}
	return data[companyBody](t, rec)
}

func postContact(t *testing.T, h http.Handler, token, workspaceID string, body map[string]any) contactBody {
	t.Helper()
	rec := call(t, h, http.MethodPost, "/workspaces/"+workspaceID+"/contacts", token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create contact = %d: %s", rec.Code, rec.Body)
	}
	return data[contactBody](t, rec)
}

func searchPath(workspaceID, resource, q string) string {
	return "/workspaces/" + workspaceID + "/" + resource + "?q=" + url.QueryEscape(q)
}

func TestCompanyRoutes(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	base := "/workspaces/" + w.ID + "/companies"

	rec := call(t, h, http.MethodPost, base, owner.Token, map[string]any{
		"name": "  Acme  ", "domain": " ACME.Example ", "phone": "+1 (555) 010-0000", "industry": "Rockets", "address": "1 Desert Road",
	})
	wantStatus(t, rec, http.StatusCreated, "create")
	created := data[companyBody](t, rec)
	if created.Name != "Acme" || created.Domain != "acme.example" || created.WorkspaceID != w.ID || created.Phone != "+1 (555) 010-0000" {
		t.Errorf("created = %+v", created)
	}

	rec = call(t, h, http.MethodGet, base+"/"+created.ID, owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "get")
	if got := data[companyBody](t, rec); got != created {
		t.Errorf("get = %+v, want %+v", got, created)
	}

	rec = call(t, h, http.MethodPatch, base+"/"+created.ID, owner.Token, map[string]any{"name": "Acme Inc", "phone": ""})
	wantStatus(t, rec, http.StatusOK, "update")
	if got := data[companyBody](t, rec); got.Name != "Acme Inc" || got.Phone != "" || got.Domain != "acme.example" {
		t.Errorf("update = %+v", got)
	}

	rec = call(t, h, http.MethodGet, base, owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "list")
	page := data[pageBody[companyBody]](t, rec)
	if len(page.Items) != 1 || page.Items[0].Name != "Acme Inc" || page.Limit != 50 || page.Offset != 0 || page.HasMore {
		t.Errorf("list = %+v", page)
	}

	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+created.ID, owner.Token, nil), http.StatusNoContent, "delete")
	wantStatus(t, call(t, h, http.MethodGet, base+"/"+created.ID, owner.Token, nil), http.StatusNotFound, "get after delete")
	wantStatus(t, call(t, h, http.MethodPatch, base+"/"+created.ID, owner.Token, map[string]any{"name": "Z"}), http.StatusNotFound, "update after delete")
	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+created.ID, owner.Token, nil), http.StatusNotFound, "delete after delete")
	rec = call(t, h, http.MethodGet, base, owner.Token, nil)
	if page := data[pageBody[companyBody]](t, rec); len(page.Items) != 0 {
		t.Errorf("list after delete = %+v", page)
	}
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("an empty list must encode as []: %s", rec.Body)
	}
}

func TestContactRoutes(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	company := postCompany(t, h, owner.Token, w.ID, map[string]any{"name": "Analytical Engines"})
	base := "/workspaces/" + w.ID + "/contacts"

	rec := call(t, h, http.MethodPost, base, owner.Token, map[string]any{
		"first_name": " Ada ", "last_name": "Lovelace", "email": "ada@example.test", "phone": "+44 20 7946 0000", "title": "Analyst", "company_id": company.ID,
		// A client cannot set this, because nothing reads it yet.
		"external_contact_id": "contacts-42",
	})
	wantStatus(t, rec, http.StatusCreated, "create")
	if strings.Contains(rec.Body.String(), "external_contact_id") || strings.Contains(rec.Body.String(), "contacts-42") {
		t.Errorf("response exposes the external ID: %s", rec.Body)
	}
	created := data[contactBody](t, rec)
	if created.FirstName != "Ada" || created.WorkspaceID != w.ID || created.CompanyID == nil || *created.CompanyID != company.ID ||
		created.CompanyName == nil || *created.CompanyName != "Analytical Engines" {
		t.Errorf("created = %+v", created)
	}

	rec = call(t, h, http.MethodGet, base+"/"+created.ID, owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "get")
	if got := data[contactBody](t, rec); got.Email != "ada@example.test" || got.CompanyID == nil {
		t.Errorf("get = %+v", got)
	}

	// Naming no company leaves the link, and null removes it.
	rec = call(t, h, http.MethodPatch, base+"/"+created.ID, owner.Token, map[string]any{"title": "Countess"})
	wantStatus(t, rec, http.StatusOK, "update")
	if got := data[contactBody](t, rec); got.Title != "Countess" || got.CompanyID == nil {
		t.Errorf("update = %+v", got)
	}
	rec = call(t, h, http.MethodPatch, base+"/"+created.ID, owner.Token, map[string]any{"company_id": nil})
	wantStatus(t, rec, http.StatusOK, "unlink")
	if got := data[contactBody](t, rec); got.CompanyID != nil || got.CompanyName != nil {
		t.Errorf("unlink = %+v", got)
	}
	rec = call(t, h, http.MethodPatch, base+"/"+created.ID, owner.Token, map[string]any{"company_id": company.ID})
	wantStatus(t, rec, http.StatusOK, "relink")
	if got := data[contactBody](t, rec); got.CompanyID == nil || *got.CompanyID != company.ID {
		t.Errorf("relink = %+v", got)
	}

	rec = call(t, h, http.MethodGet, base, owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "list")
	if page := data[pageBody[contactBody]](t, rec); len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Errorf("list = %+v", page)
	}

	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+created.ID, owner.Token, nil), http.StatusNoContent, "delete")
	wantStatus(t, call(t, h, http.MethodGet, base+"/"+created.ID, owner.Token, nil), http.StatusNotFound, "get after delete")
	wantStatus(t, call(t, h, http.MethodPatch, base+"/"+created.ID, owner.Token, map[string]any{"title": "Z"}), http.StatusNotFound, "update after delete")
	wantStatus(t, call(t, h, http.MethodDelete, base+"/"+created.ID, owner.Token, nil), http.StatusNotFound, "delete after delete")
	rec = call(t, h, http.MethodGet, searchPath(w.ID, "contacts", "Ada"), owner.Token, nil)
	if page := data[pageBody[contactBody]](t, rec); len(page.Items) != 0 {
		t.Errorf("search after delete = %+v", page)
	}
}

// Every role reads and writes: nothing about the roles says otherwise, and the
// point of a shared CRM is that the whole team keeps it up to date.
func TestEveryMemberRoleCanWrite(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	for _, role := range []string{"admin", "member"} {
		t.Run(role, func(t *testing.T) {
			m := joined(t, h, owner.Token, w.ID, role)
			company := postCompany(t, h, m.Token, w.ID, map[string]any{"name": "By " + role})
			contact := postContact(t, h, m.Token, w.ID, map[string]any{"first_name": role, "company_id": company.ID})
			wantStatus(t, call(t, h, http.MethodPatch, "/workspaces/"+w.ID+"/companies/"+company.ID, m.Token, map[string]any{"industry": "x"}), http.StatusOK, "update company")
			wantStatus(t, call(t, h, http.MethodPatch, "/workspaces/"+w.ID+"/contacts/"+contact.ID, m.Token, map[string]any{"title": "x"}), http.StatusOK, "update contact")
			wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+w.ID+"/contacts/"+contact.ID, m.Token, nil), http.StatusNoContent, "delete contact")
			wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+w.ID+"/companies/"+company.ID, m.Token, nil), http.StatusNoContent, "delete company")
		})
	}
}

func TestCRMRoutesNeedAuthentication(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	for _, resource := range []string{"companies", "contacts"} {
		wantStatus(t, call(t, h, http.MethodGet, "/workspaces/"+w.ID+"/"+resource, "", nil), http.StatusUnauthorized, resource)
	}
}

func TestCRMSearchTreatsWildcardsAsLiterals(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	acme := postCompany(t, h, owner.Token, w.ID, map[string]any{"name": "Acme 100%", "domain": "acme.example"})
	postCompany(t, h, owner.Token, w.ID, map[string]any{"name": "under_score"})
	postCompany(t, h, owner.Token, w.ID, map[string]any{"name": "underXscore"})
	postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Grace", "email": "grace_h@navy.test", "company_id": acme.ID})
	postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Other", "email": "gracexh@navy.test"})

	companies := func(q string) []string {
		t.Helper()
		rec := call(t, h, http.MethodGet, searchPath(w.ID, "companies", q), owner.Token, nil)
		wantStatus(t, rec, http.StatusOK, "search companies")
		names := []string{}
		for _, c := range data[pageBody[companyBody]](t, rec).Items {
			names = append(names, c.Name)
		}
		return names
	}
	contacts := func(q string) []string {
		t.Helper()
		rec := call(t, h, http.MethodGet, searchPath(w.ID, "contacts", q), owner.Token, nil)
		wantStatus(t, rec, http.StatusOK, "search contacts")
		names := []string{}
		for _, c := range data[pageBody[contactBody]](t, rec).Items {
			names = append(names, c.FirstName)
		}
		return names
	}
	for _, tt := range []struct {
		name, q string
		search  func(string) []string
		want    []string
	}{
		{"company, any case", "ACME", companies, []string{"Acme 100%"}},
		{"company domain", "acme.EXAMPLE", companies, []string{"Acme 100%"}},
		{"company percent", "%", companies, []string{"Acme 100%"}},
		{"company underscore", "_", companies, []string{"under_score"}},
		{"contact, any case", "oTHER", contacts, []string{"Other"}},
		{"contact email", "NAVY.test", contacts, []string{"Grace", "Other"}},
		{"contact underscore", "grace_h", contacts, []string{"Grace"}},
		{"contact by company name", "acme", contacts, []string{"Grace"}},
		{"contact percent through the company", "%", contacts, []string{"Grace"}},
		{"surrounding spaces are ignored", "  acme  ", contacts, []string{"Grace"}},
		{"only spaces means no filter", "   ", contacts, []string{"Grace", "Other"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.search(tt.q); !slices.Equal(got, tt.want) {
				t.Errorf("search %q = %q, want %q", tt.q, got, tt.want)
			}
		})
	}
}

func TestCRMListQueryValidation(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	for _, resource := range []string{"companies", "contacts"} {
		base := "/workspaces/" + w.ID + "/" + resource
		for _, query := range []string{
			"limit=0", "limit=-1", "limit=201", "limit=abc", "limit=", "limit=1.5", "limit=99999999999999999999",
			"offset=-1", "offset=abc", "offset=100001", "offset=", "offset=99999999999999999999",
			"q=%00", "q=a%00b", "q=%FF", "q=" + strings.Repeat("x", 201),
		} {
			t.Run(resource+" "+query[:min(len(query), 30)], func(t *testing.T) {
				rec := call(t, h, http.MethodGet, base+"?"+query, owner.Token, nil)
				wantStatus(t, rec, http.StatusBadRequest, query)
			})
		}
		rec := call(t, h, http.MethodGet, base+"?limit=200&offset=100000&q="+strings.Repeat("x", 200), owner.Token, nil)
		wantStatus(t, rec, http.StatusOK, "the largest allowed values")
	}
}

func TestCRMPaginationOverHTTP(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	for _, name := range []string{"Delta", "alpha", "Charlie", "bravo", "Echo"} {
		postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": name})
		postCompany(t, h, owner.Token, w.ID, map[string]any{"name": name})
	}
	want := []string{"alpha", "bravo", "Charlie", "Delta", "Echo"}

	var contacts, companies []string
	for offset := 0; offset < 5; offset += 2 {
		rec := call(t, h, http.MethodGet, fmt.Sprintf("/workspaces/%s/contacts?limit=2&offset=%d", w.ID, offset), owner.Token, nil)
		wantStatus(t, rec, http.StatusOK, "contacts page")
		page := data[pageBody[contactBody]](t, rec)
		if page.Limit != 2 || page.Offset != offset || page.HasMore != (offset < 4) {
			t.Errorf("contacts page at %d = %+v", offset, page)
		}
		for _, c := range page.Items {
			contacts = append(contacts, c.FirstName)
		}
		rec = call(t, h, http.MethodGet, fmt.Sprintf("/workspaces/%s/companies?limit=2&offset=%d", w.ID, offset), owner.Token, nil)
		wantStatus(t, rec, http.StatusOK, "companies page")
		for _, c := range data[pageBody[companyBody]](t, rec).Items {
			companies = append(companies, c.Name)
		}
	}
	if !slices.Equal(contacts, want) || !slices.Equal(companies, want) {
		t.Errorf("contacts %q, companies %q, want %q", contacts, companies, want)
	}
}

func TestCompanyInputValidation(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	existing := postCompany(t, h, owner.Token, w.ID, map[string]any{"name": "Existing"})
	base := "/workspaces/" + w.ID + "/companies"

	long := func(n int) string { return strings.Repeat("x", n) }
	invalid := []struct {
		name string
		body map[string]any
	}{
		{"blank name", map[string]any{"name": "   "}},
		{"name too long", map[string]any{"name": long(201)}},
		{"NUL in name", map[string]any{"name": "a\x00b"}},
		{"NUL in domain", map[string]any{"name": "a", "domain": "a\x00.test"}},
		{"NUL in phone", map[string]any{"name": "a", "phone": "1\x00"}},
		{"NUL in industry", map[string]any{"name": "a", "industry": "\x00"}},
		{"NUL in address", map[string]any{"name": "a", "address": "\x00"}},
		{"domain with a scheme", map[string]any{"name": "a", "domain": "https://acme.example"}},
		{"domain with no dot", map[string]any{"name": "a", "domain": "localhost"}},
		{"domain with a space", map[string]any{"name": "a", "domain": "ac me.example"}},
		{"domain label starts with a hyphen", map[string]any{"name": "a", "domain": "-acme.example"}},
		{"domain too long", map[string]any{"name": "a", "domain": long(250) + ".test"}},
		{"phone with no digit", map[string]any{"name": "a", "phone": "call me"}},
		{"phone too long", map[string]any{"name": "a", "phone": strings.Repeat("1", 51)}},
		{"industry too long", map[string]any{"name": "a", "industry": long(101)}},
		{"address too long", map[string]any{"name": "a", "address": long(501)}},
		{"wrong type", map[string]any{"name": 12}},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			wantStatus(t, call(t, h, http.MethodPost, base, owner.Token, tt.body), http.StatusBadRequest, "create")
			wantStatus(t, call(t, h, http.MethodPatch, base+"/"+existing.ID, owner.Token, tt.body), http.StatusBadRequest, "update")
		})
	}
	wantStatus(t, call(t, h, http.MethodPost, base, owner.Token, map[string]any{}), http.StatusBadRequest, "create with no name")
	wantStatus(t, call(t, h, http.MethodPatch, base+"/"+existing.ID, owner.Token, map[string]any{}), http.StatusBadRequest, "update with nothing")
	wantStatus(t, call(t, h, http.MethodPost, base, owner.Token, "not an object"), http.StatusBadRequest, "not an object")

	// A failed update changes nothing.
	got := data[companyBody](t, call(t, h, http.MethodGet, base+"/"+existing.ID, owner.Token, nil))
	if got != existing {
		t.Errorf("after failed updates = %+v, want %+v", got, existing)
	}

	rec := call(t, h, http.MethodPost, base, owner.Token, map[string]any{"name": strings.Repeat("é", 200), "domain": "münchen.example", "phone": strings.Repeat("1", 50)})
	wantStatus(t, rec, http.StatusCreated, "values at their limits, counted in characters")
}

func TestContactInputValidation(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	existing := postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Existing"})
	base := "/workspaces/" + w.ID + "/contacts"

	long := func(n int) string { return strings.Repeat("x", n) }
	invalid := []struct {
		name string
		body map[string]any
	}{
		{"blank first name", map[string]any{"first_name": "  "}},
		{"first name too long", map[string]any{"first_name": long(101)}},
		{"last name too long", map[string]any{"first_name": "a", "last_name": long(101)}},
		{"NUL in first name", map[string]any{"first_name": "a\x00"}},
		{"NUL in last name", map[string]any{"first_name": "a", "last_name": "\x00"}},
		{"NUL in email", map[string]any{"first_name": "a", "email": "a\x00@b.test"}},
		{"NUL in phone", map[string]any{"first_name": "a", "phone": "1\x00"}},
		{"NUL in title", map[string]any{"first_name": "a", "title": "\x00"}},
		{"bad email", map[string]any{"first_name": "a", "email": "not-an-email"}},
		{"email with a display name", map[string]any{"first_name": "a", "email": "A <a@b.test>"}},
		{"email too long", map[string]any{"first_name": "a", "email": long(250) + "@b.test"}},
		{"phone with no digit", map[string]any{"first_name": "a", "phone": "none"}},
		{"phone too long", map[string]any{"first_name": "a", "phone": strings.Repeat("1", 51)}},
		{"title too long", map[string]any{"first_name": "a", "title": long(201)}},
		{"wrong type", map[string]any{"first_name": 12}},
		{"company of the wrong type", map[string]any{"first_name": "a", "company_id": 12}},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			wantStatus(t, call(t, h, http.MethodPost, base, owner.Token, tt.body), http.StatusBadRequest, "create")
			wantStatus(t, call(t, h, http.MethodPatch, base+"/"+existing.ID, owner.Token, tt.body), http.StatusBadRequest, "update")
		})
	}
	wantStatus(t, call(t, h, http.MethodPost, base, owner.Token, map[string]any{}), http.StatusBadRequest, "create with no name")
	wantStatus(t, call(t, h, http.MethodPatch, base+"/"+existing.ID, owner.Token, map[string]any{}), http.StatusBadRequest, "update with nothing")

	got := data[contactBody](t, call(t, h, http.MethodGet, base+"/"+existing.ID, owner.Token, nil))
	if got.FirstName != "Existing" || got.Email != "" {
		t.Errorf("after failed updates = %+v", got)
	}
}

// isolation holds two workspaces with a user each, and one company and one linked
// contact in B that A's user tries to reach.
type isolation struct {
	aToken, bToken string
	a, b           string
	companyB       companyBody
	contactB       contactBody
}

func newIsolation(t *testing.T, h http.Handler) isolation {
	t.Helper()
	userA, userB := register(t, h, uniqueEmail(t)), register(t, h, uniqueEmail(t))
	a, b := createWorkspace(t, h, userA.Token), createWorkspace(t, h, userB.Token)
	company := postCompany(t, h, userB.Token, b.ID, map[string]any{"name": "Secret Company", "domain": "secret.example"})
	contact := postContact(t, h, userB.Token, b.ID, map[string]any{"first_name": "Secret", "email": "secret@secret.example", "company_id": company.ID})
	return isolation{aToken: userA.Token, bToken: userB.Token, a: a.ID, b: b.ID, companyB: company, contactB: contact}
}

// wantSame404 checks that every path answers exactly like the first, as a 404.
func wantSame404(t *testing.T, h http.Handler, token, method string, body any, paths []string) {
	t.Helper()
	var first string
	for i, path := range paths {
		rec := call(t, h, method, path, token, body)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404: %s", method, path, rec.Code, rec.Body)
			continue
		}
		if i == 0 {
			first = rec.Body.String()
		} else if got := rec.Body.String(); got != first {
			t.Errorf("%s %s body %q, want the same as %q", method, path, got, first)
		}
	}
}

func TestResourcesInAnotherWorkspaceAnswerLikeAbsentOnes(t *testing.T) {
	h := authHandler(t)
	iso := newIsolation(t, h)

	for _, tt := range []struct {
		resource string
		foreign  string
		deleted  func() string
		patch    map[string]any
	}{
		{"companies", iso.companyB.ID, func() string {
			c := postCompany(t, h, iso.aToken, iso.a, map[string]any{"name": "Deleted"})
			wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+iso.a+"/companies/"+c.ID, iso.aToken, nil), http.StatusNoContent, "delete")
			return c.ID
		}, map[string]any{"name": "pwned"}},
		{"contacts", iso.contactB.ID, func() string {
			c := postContact(t, h, iso.aToken, iso.a, map[string]any{"first_name": "Deleted"})
			wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+iso.a+"/contacts/"+c.ID, iso.aToken, nil), http.StatusNoContent, "delete")
			return c.ID
		}, map[string]any{"first_name": "pwned"}},
	} {
		t.Run(tt.resource, func(t *testing.T) {
			base := "/workspaces/" + iso.a + "/" + tt.resource + "/"
			paths := []string{base + unknownUUID, base + tt.foreign, base + tt.deleted()}
			for _, bad := range malformedIDs {
				paths = append(paths, base+bad)
			}
			wantSame404(t, h, iso.aToken, http.MethodGet, nil, paths)
			wantSame404(t, h, iso.aToken, http.MethodPatch, tt.patch, paths)
			wantSame404(t, h, iso.aToken, http.MethodDelete, nil, paths)
		})
	}

	// The attempts above changed nothing in B.
	company := data[companyBody](t, call(t, h, http.MethodGet, "/workspaces/"+iso.b+"/companies/"+iso.companyB.ID, iso.bToken, nil))
	contact := data[contactBody](t, call(t, h, http.MethodGet, "/workspaces/"+iso.b+"/contacts/"+iso.contactB.ID, iso.bToken, nil))
	if company != iso.companyB {
		t.Errorf("B's company = %+v, want %+v", company, iso.companyB)
	}
	if contact.FirstName != "Secret" || contact.CompanyID == nil {
		t.Errorf("B's contact = %+v", contact)
	}
}

func TestNonMemberGetsTheWorkspace404OnEveryCRMRoute(t *testing.T) {
	h := authHandler(t)
	iso := newIsolation(t, h)
	want := call(t, h, http.MethodGet, "/workspaces/"+unknownUUID, iso.aToken, nil)
	wantStatus(t, want, http.StatusNotFound, "unknown workspace")

	inB := "/workspaces/" + iso.b
	for _, tt := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, inB + "/companies", nil},
		{http.MethodGet, searchPath(iso.b, "companies", "Secret"), nil},
		{http.MethodPost, inB + "/companies", map[string]any{"name": "rogue"}},
		{http.MethodGet, inB + "/companies/" + iso.companyB.ID, nil},
		{http.MethodPatch, inB + "/companies/" + iso.companyB.ID, map[string]any{"name": "pwned"}},
		{http.MethodDelete, inB + "/companies/" + iso.companyB.ID, nil},
		{http.MethodGet, inB + "/contacts", nil},
		{http.MethodGet, searchPath(iso.b, "contacts", "Secret"), nil},
		{http.MethodPost, inB + "/contacts", map[string]any{"first_name": "rogue"}},
		{http.MethodGet, inB + "/contacts/" + iso.contactB.ID, nil},
		{http.MethodPatch, inB + "/contacts/" + iso.contactB.ID, map[string]any{"first_name": "pwned"}},
		{http.MethodDelete, inB + "/contacts/" + iso.contactB.ID, nil},
		// Invalid input must not reveal anything before membership is checked.
		{http.MethodPost, inB + "/contacts", map[string]any{"first_name": ""}},
		{http.MethodGet, inB + "/contacts?limit=abc", nil},
	} {
		rec := call(t, h, tt.method, tt.path, iso.aToken, tt.body)
		wantStatus(t, rec, http.StatusNotFound, tt.method+" "+tt.path)
		if rec.Body.String() != want.Body.String() {
			t.Errorf("%s %s body %q, want %q", tt.method, tt.path, rec.Body, want.Body)
		}
	}
}

func TestListsAndSearchShowNothingFromAnotherWorkspace(t *testing.T) {
	h := authHandler(t)
	iso := newIsolation(t, h)
	for _, path := range []string{
		"/workspaces/" + iso.a + "/companies",
		"/workspaces/" + iso.a + "/contacts",
		searchPath(iso.a, "companies", "Secret"),
		searchPath(iso.a, "contacts", "Secret"),
		searchPath(iso.a, "contacts", "secret.example"),
	} {
		rec := call(t, h, http.MethodGet, path, iso.aToken, nil)
		wantStatus(t, rec, http.StatusOK, path)
		if strings.Contains(rec.Body.String(), "ecret") || !strings.Contains(rec.Body.String(), `"items":[]`) {
			t.Errorf("%s = %s", path, rec.Body)
		}
	}
}

func TestContactCannotLinkAnUnavailableCompanyOverHTTP(t *testing.T) {
	h := authHandler(t)
	iso := newIsolation(t, h)
	base := "/workspaces/" + iso.a + "/contacts"
	mine := postContact(t, h, iso.aToken, iso.a, map[string]any{"first_name": "Mine"})
	deleted := postCompany(t, h, iso.aToken, iso.a, map[string]any{"name": "Deleted"})
	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+iso.a+"/companies/"+deleted.ID, iso.aToken, nil), http.StatusNoContent, "delete company")

	want := call(t, h, http.MethodGet, "/workspaces/"+iso.a+"/companies/"+unknownUUID, iso.aToken, nil)
	wantStatus(t, want, http.StatusNotFound, "unknown company")

	for _, tt := range []struct{ name, companyID string }{
		{"another workspace", iso.companyB.ID},
		{"soft-deleted", deleted.ID},
		{"nonexistent", unknownUUID},
		{"malformed", "not-a-uuid"},
		{"empty", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, attempt := range []struct {
				what, method, path string
				body               map[string]any
			}{
				{"create", http.MethodPost, base, map[string]any{"first_name": "Rogue", "company_id": tt.companyID}},
				{"update", http.MethodPatch, base + "/" + mine.ID, map[string]any{"company_id": tt.companyID}},
			} {
				rec := call(t, h, attempt.method, attempt.path, iso.aToken, attempt.body)
				wantStatus(t, rec, http.StatusNotFound, attempt.what)
				if rec.Body.String() != want.Body.String() {
					t.Errorf("%s body %q, want %q", attempt.what, rec.Body, want.Body)
				}
			}
		})
	}
	rec := call(t, h, http.MethodGet, base, iso.aToken, nil)
	if page := data[pageBody[contactBody]](t, rec); len(page.Items) != 1 || page.Items[0].CompanyID != nil {
		t.Errorf("contacts after the attempts = %+v", page)
	}
}

func TestDeletingACompanyKeepsItsContactsOverHTTP(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	company := postCompany(t, h, owner.Token, w.ID, map[string]any{"name": "Closing Down"})
	contact := postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Survivor", "company_id": company.ID})

	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+w.ID+"/companies/"+company.ID, owner.Token, nil), http.StatusNoContent, "delete company")

	rec := call(t, h, http.MethodGet, "/workspaces/"+w.ID+"/contacts/"+contact.ID, owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "contact after company delete")
	if got := data[contactBody](t, rec); got.CompanyID != nil || got.CompanyName != nil {
		t.Errorf("contact = %+v", got)
	}
	rec = call(t, h, http.MethodGet, searchPath(w.ID, "companies", "Closing"), owner.Token, nil)
	if page := data[pageBody[companyBody]](t, rec); len(page.Items) != 0 {
		t.Errorf("deleted company found by search: %+v", page)
	}
	rec = call(t, h, http.MethodGet, searchPath(w.ID, "contacts", "Closing"), owner.Token, nil)
	if page := data[pageBody[contactBody]](t, rec); len(page.Items) != 0 {
		t.Errorf("contact found through a deleted company: %+v", page)
	}
}

func TestRemovedMemberLosesAccessToContactsAndCompanies(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	member := joined(t, h, owner.Token, w.ID, "member")
	company := postCompany(t, h, member.Token, w.ID, map[string]any{"name": "Made by member"})
	contact := postContact(t, h, member.Token, w.ID, map[string]any{"first_name": "Made by member"})

	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+w.ID+"/members/"+member.User.ID, owner.Token, nil), http.StatusNoContent, "remove member")

	want := call(t, h, http.MethodGet, "/workspaces/"+unknownUUID, member.Token, nil)
	inW := "/workspaces/" + w.ID
	for _, tt := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, inW + "/companies", nil},
		{http.MethodPost, inW + "/companies", map[string]any{"name": "after"}},
		{http.MethodGet, inW + "/companies/" + company.ID, nil},
		{http.MethodPatch, inW + "/companies/" + company.ID, map[string]any{"name": "after"}},
		{http.MethodDelete, inW + "/companies/" + company.ID, nil},
		{http.MethodGet, inW + "/contacts", nil},
		{http.MethodPost, inW + "/contacts", map[string]any{"first_name": "after"}},
		{http.MethodGet, inW + "/contacts/" + contact.ID, nil},
		{http.MethodPatch, inW + "/contacts/" + contact.ID, map[string]any{"first_name": "after"}},
		{http.MethodDelete, inW + "/contacts/" + contact.ID, nil},
	} {
		rec := call(t, h, tt.method, tt.path, member.Token, tt.body)
		wantStatus(t, rec, http.StatusNotFound, tt.method+" "+tt.path)
		if rec.Body.String() != want.Body.String() {
			t.Errorf("%s %s body %q, want %q", tt.method, tt.path, rec.Body, want.Body)
		}
	}
	// The data stays with the workspace.
	wantStatus(t, call(t, h, http.MethodGet, inW+"/companies/"+company.ID, owner.Token, nil), http.StatusOK, "owner reads company")
	wantStatus(t, call(t, h, http.MethodGet, inW+"/contacts/"+contact.ID, owner.Token, nil), http.StatusOK, "owner reads contact")
}

func TestDeletedWorkspaceHidesItsContactsAndCompanies(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	contact := postContact(t, h, owner.Token, w.ID, map[string]any{"first_name": "Orphan"})
	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+w.ID, owner.Token, nil), http.StatusNoContent, "delete workspace")
	wantStatus(t, call(t, h, http.MethodGet, "/workspaces/"+w.ID+"/contacts/"+contact.ID, owner.Token, nil), http.StatusNotFound, "get contact")
	wantStatus(t, call(t, h, http.MethodGet, "/workspaces/"+w.ID+"/companies", owner.Token, nil), http.StatusNotFound, "list companies")
}

func TestCRMResponsesAreJSONWithTheDataEnvelope(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	rec := call(t, h, http.MethodGet, "/workspaces/"+w.ID+"/contacts", owner.Token, nil)
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["data"]; !ok {
		t.Errorf("body = %s", rec.Body)
	}
}
