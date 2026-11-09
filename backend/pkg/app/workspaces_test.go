package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"crm/pkg/store"
	"crm/pkg/testdb"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

func call(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	r := httptest.NewRequest(method, path, reader)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int, what string) {
	t.Helper()
	if rec.Code != want {
		t.Errorf("%s = %d, want %d: %s", what, rec.Code, want, rec.Body)
	}
}

type workspaceBody struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

func data[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var body struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return body.Data
}

func createWorkspace(t *testing.T, h http.Handler, token string) workspaceBody {
	t.Helper()
	rec := call(t, h, http.MethodPost, "/workspaces", token, map[string]string{"name": "Acme"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create workspace = %d: %s", rec.Code, rec.Body)
	}
	return data[workspaceBody](t, rec)
}

func joined(t *testing.T, h http.Handler, ownerToken, workspaceID, role string) session {
	t.Helper()
	u := register(t, h, uniqueEmail(t))
	rec := call(t, h, http.MethodPost, "/workspaces/"+workspaceID+"/members", ownerToken, map[string]string{"email": u.User.Email, "role": role})
	if rec.Code != http.StatusCreated {
		t.Fatalf("add %s = %d: %s", role, rec.Code, rec.Body)
	}
	return u
}

func TestWorkspaceLifecycle(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	if w.Role != "owner" || w.Name != "Acme" {
		t.Errorf("created = %+v", w)
	}

	rec := call(t, h, http.MethodGet, "/workspaces", owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "list")
	if list := data[[]workspaceBody](t, rec); len(list) != 1 || list[0].ID != w.ID {
		t.Errorf("list = %+v", list)
	}

	rec = call(t, h, http.MethodGet, "/workspaces/"+w.ID, owner.Token, nil)
	wantStatus(t, rec, http.StatusOK, "get")
	if got := data[workspaceBody](t, rec); got.Role != "owner" {
		t.Errorf("get = %+v", got)
	}

	rec = call(t, h, http.MethodPatch, "/workspaces/"+w.ID, owner.Token, map[string]string{"name": "  Renamed  "})
	wantStatus(t, rec, http.StatusOK, "rename")
	if got := data[workspaceBody](t, rec); got.Name != "Renamed" {
		t.Errorf("rename = %+v", got)
	}

	member := joined(t, h, owner.Token, w.ID, "member")
	rec = call(t, h, http.MethodGet, "/workspaces/"+w.ID+"/members", member.Token, nil)
	wantStatus(t, rec, http.StatusOK, "list members")
	if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "$2a$") {
		t.Error("member list exposes password material")
	}

	rec = call(t, h, http.MethodPatch, "/workspaces/"+w.ID+"/members/"+member.User.ID, owner.Token, map[string]string{"role": "admin"})
	wantStatus(t, rec, http.StatusOK, "change role")
	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+w.ID+"/members/"+member.User.ID, owner.Token, nil), http.StatusNoContent, "remove")
	wantStatus(t, call(t, h, http.MethodGet, "/workspaces/"+w.ID, member.Token, nil), http.StatusNotFound, "get after removal")

	again := joined(t, h, owner.Token, w.ID, "member")
	wantStatus(t, call(t, h, http.MethodPost, "/workspaces/"+w.ID+"/leave", again.Token, nil), http.StatusNoContent, "leave")
	wantStatus(t, call(t, h, http.MethodGet, "/workspaces/"+w.ID, again.Token, nil), http.StatusNotFound, "get after leaving")

	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+w.ID, owner.Token, nil), http.StatusNoContent, "delete")
	wantStatus(t, call(t, h, http.MethodGet, "/workspaces/"+w.ID, owner.Token, nil), http.StatusNotFound, "get after delete")
}

func TestWorkspaceRoutesRequireAuth(t *testing.T) {
	h := authHandler(t)
	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, "/workspaces"},
		{http.MethodPost, "/workspaces"},
		{http.MethodGet, "/workspaces/00000000-0000-0000-0000-000000000000"},
		{http.MethodGet, "/workspaces/00000000-0000-0000-0000-000000000000/members"},
	} {
		wantStatus(t, call(t, h, tt.method, tt.path, "", nil), http.StatusUnauthorized, tt.method+" "+tt.path)
	}
}

// Existence must not leak: a live workspace you are not in, one that does not
// exist, one that was deleted, and a malformed ID all answer byte for byte alike.
func TestNonMemberAndNonexistentGetIdentical404(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	stranger := register(t, h, uniqueEmail(t))
	live := createWorkspace(t, h, owner.Token)
	deleted := createWorkspace(t, h, owner.Token)
	wantStatus(t, call(t, h, http.MethodDelete, "/workspaces/"+deleted.ID, owner.Token, nil), http.StatusNoContent, "delete")

	targets := map[string]struct{ id, token string }{
		"non-member":             {live.ID, stranger.Token},
		"nonexistent":            {"00000000-0000-0000-0000-000000000000", stranger.Token},
		"deleted, as non-member": {deleted.ID, stranger.Token},
		"deleted, as owner":      {deleted.ID, owner.Token},
		"malformed":              {"not-a-uuid", stranger.Token},
		"NUL":                    {"a%00b", stranger.Token},
	}
	routes := []struct {
		method, suffix string
		body           any
	}{
		{http.MethodGet, "", nil},
		{http.MethodPatch, "", map[string]string{"name": "x"}},
		{http.MethodDelete, "", nil},
		{http.MethodPost, "/leave", nil},
		{http.MethodGet, "/members", nil},
		{http.MethodPost, "/members", map[string]string{"email": owner.User.Email, "role": "member"}},
		{http.MethodPatch, "/members/" + owner.User.ID, map[string]string{"role": "member"}},
		{http.MethodDelete, "/members/" + owner.User.ID, nil},
	}
	for _, route := range routes {
		var wantBody, wantHeaders string
		for name, target := range targets {
			rec := call(t, h, route.method, "/workspaces/"+target.id+route.suffix, target.token, route.body)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s (%s) = %d, want 404: %s", route.method, route.suffix, name, rec.Code, rec.Body)
				continue
			}
			if wantBody == "" {
				wantBody, wantHeaders = rec.Body.String(), fmt.Sprint(rec.Header())
				continue
			}
			if rec.Body.String() != wantBody || fmt.Sprint(rec.Header()) != wantHeaders {
				t.Errorf("%s %s (%s) differs: %q %v, want %q %v", route.method, route.suffix, name, rec.Body, rec.Header(), wantBody, wantHeaders)
			}
		}
	}
}

func TestWorkspaceRoleLimitsOverHTTP(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	admin := joined(t, h, owner.Token, w.ID, "admin")
	member := joined(t, h, owner.Token, w.ID, "member")
	base := "/workspaces/" + w.ID
	newcomer := register(t, h, uniqueEmail(t))

	tests := []struct {
		name         string
		method, path string
		token        string
		body         any
		want         int
	}{
		{"member reads workspace", http.MethodGet, base, member.Token, nil, http.StatusOK},
		{"member reads members", http.MethodGet, base + "/members", member.Token, nil, http.StatusOK},
		{"member cannot rename", http.MethodPatch, base, member.Token, map[string]string{"name": "x"}, http.StatusForbidden},
		{"member cannot delete", http.MethodDelete, base, member.Token, nil, http.StatusForbidden},
		{"member cannot add", http.MethodPost, base + "/members", member.Token, map[string]string{"email": newcomer.User.Email, "role": "member"}, http.StatusForbidden},
		{"member cannot probe emails", http.MethodPost, base + "/members", member.Token, map[string]string{"email": uniqueEmail(t), "role": "member"}, http.StatusForbidden},
		{"member cannot change role", http.MethodPatch, base + "/members/" + member.User.ID, member.Token, map[string]string{"role": "owner"}, http.StatusForbidden},
		{"member cannot remove", http.MethodDelete, base + "/members/" + admin.User.ID, member.Token, nil, http.StatusForbidden},
		{"admin cannot delete", http.MethodDelete, base, admin.Token, nil, http.StatusForbidden},
		{"admin cannot alter owner", http.MethodPatch, base + "/members/" + owner.User.ID, admin.Token, map[string]string{"role": "member"}, http.StatusForbidden},
		{"admin cannot remove owner", http.MethodDelete, base + "/members/" + owner.User.ID, admin.Token, nil, http.StatusForbidden},
		{"admin cannot create owner", http.MethodPatch, base + "/members/" + member.User.ID, admin.Token, map[string]string{"role": "owner"}, http.StatusForbidden},
		{"admin cannot add owner", http.MethodPost, base + "/members", admin.Token, map[string]string{"email": newcomer.User.Email, "role": "owner"}, http.StatusForbidden},
		{"owner cannot demote last owner", http.MethodPatch, base + "/members/" + owner.User.ID, owner.Token, map[string]string{"role": "member"}, http.StatusConflict},
		{"owner cannot remove last owner", http.MethodDelete, base + "/members/" + owner.User.ID, owner.Token, nil, http.StatusConflict},
		{"last owner cannot leave", http.MethodPost, base + "/leave", owner.Token, nil, http.StatusConflict},
		{"unknown email", http.MethodPost, base + "/members", admin.Token, map[string]string{"email": uniqueEmail(t), "role": "member"}, http.StatusNotFound},
		{"existing member again", http.MethodPost, base + "/members", admin.Token, map[string]string{"email": member.User.Email, "role": "member"}, http.StatusConflict},
		{"unknown target", http.MethodPatch, base + "/members/" + newcomer.User.ID, admin.Token, map[string]string{"role": "member"}, http.StatusNotFound},
		{"malformed target", http.MethodDelete, base + "/members/not-a-uuid", admin.Token, nil, http.StatusNotFound},
		{"bad role", http.MethodPatch, base + "/members/" + member.User.ID, admin.Token, map[string]string{"role": "root"}, http.StatusBadRequest},
		{"admin adds member", http.MethodPost, base + "/members", admin.Token, map[string]string{"email": newcomer.User.Email, "role": "member"}, http.StatusCreated},
		{"admin changes member", http.MethodPatch, base + "/members/" + member.User.ID, admin.Token, map[string]string{"role": "admin"}, http.StatusOK},
		{"admin renames", http.MethodPatch, base, admin.Token, map[string]string{"name": "New"}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantStatus(t, call(t, h, tt.method, tt.path, tt.token, tt.body), tt.want, tt.name)
		})
	}
}

func TestWorkspaceInputValidation(t *testing.T) {
	h := authHandler(t)
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	base := "/workspaces/" + w.ID
	tests := []struct {
		name         string
		method, path string
		body         any
	}{
		{"empty name", http.MethodPost, "/workspaces", map[string]string{"name": "   "}},
		{"long name", http.MethodPost, "/workspaces", map[string]string{"name": strings.Repeat("é", 201)}},
		{"NUL name", http.MethodPost, "/workspaces", map[string]string{"name": "a\x00"}},
		{"not JSON name", http.MethodPost, "/workspaces", 7},
		{"rename empty", http.MethodPatch, base, map[string]string{"name": ""}},
		{"rename NUL", http.MethodPatch, base, map[string]string{"name": "a\x00"}},
		{"bad email", http.MethodPost, base + "/members", map[string]string{"email": "nope", "role": "member"}},
		{"NUL email", http.MethodPost, base + "/members", map[string]string{"email": "a\x00@example.test", "role": "member"}},
		{"bad role on add", http.MethodPost, base + "/members", map[string]string{"email": owner.User.Email, "role": "root"}},
		{"missing role", http.MethodPost, base + "/members", map[string]string{"email": owner.User.Email}},
		{"NUL role", http.MethodPatch, base + "/members/" + owner.User.ID, map[string]string{"role": "owner\x00"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantStatus(t, call(t, h, tt.method, tt.path, owner.Token, tt.body), http.StatusBadRequest, tt.name)
		})
	}
}

// A database failure must never fall through to the handler or read as "found".
func TestWorkspaceMiddlewareFailsClosed(t *testing.T) {
	manager := testManager(t, time.Minute)
	token, err := manager.Generate("00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	a := New(store.New(unreachablePool(t)), manager, WithPasswordCost(bcrypt.MinCost), WithDBTimeout(2*time.Second))
	reached := false
	h := a.requireAuth(a.requireWorkspace(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r = withRouteParam(r, "workspaceID", "00000000-0000-0000-0000-000000000000")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if reached {
		t.Error("the handler ran after the membership check failed")
	}
}

func TestWorkspaceMiddlewareHasDeadline(t *testing.T) {
	manager := testManager(t, time.Minute)
	token, err := manager.Generate("00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	h := New(store.New(silentPool(t)), manager, WithPasswordCost(bcrypt.MinCost), WithDBTimeout(100*time.Millisecond)).Handler()
	r := httptest.NewRequest(http.MethodGet, "/workspaces/00000000-0000-0000-0000-000000000000", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	rec := answersWithin(t, 3*time.Second, "membership check", h, r)
	if rec != nil && rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestHandlersWithoutWorkspaceContextFailClosed(t *testing.T) {
	a := New(store.New(unreachablePool(t)), testManager(t, time.Minute), WithPasswordCost(bcrypt.MinCost))
	for name, handler := range map[string]http.HandlerFunc{
		"get": a.getWorkspace, "rename": a.renameWorkspace, "delete": a.deleteWorkspace, "leave": a.leaveWorkspace,
		"list members": a.listMembers, "add": a.addMember, "change": a.changeMemberRole, "remove": a.removeMember,
	} {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`)))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", name, rec.Code)
		}
	}
}

func TestAddMemberHasItsOwnStrictBudget(t *testing.T) {
	a := New(store.New(testdb.AppPool(t)), testManager(t, time.Minute), WithPasswordCost(bcrypt.MinCost),
		WithAuthRateLimit(1_000_000), WithSessionRateLimit(1_000_000), WithMemberLookupRateLimit(2))
	h := a.Handler()
	owner := register(t, h, uniqueEmail(t))
	w := createWorkspace(t, h, owner.Token)
	add := func() *httptest.ResponseRecorder {
		return call(t, h, http.MethodPost, "/workspaces/"+w.ID+"/members", owner.Token, map[string]string{"email": uniqueEmail(t), "role": "member"})
	}
	wantStatus(t, add(), http.StatusNotFound, "first probe")
	wantStatus(t, add(), http.StatusNotFound, "second probe")
	wantStatus(t, add(), http.StatusTooManyRequests, "third probe")
	wantStatus(t, call(t, h, http.MethodGet, "/workspaces/"+w.ID+"/members", owner.Token, nil), http.StatusOK, "reads stay available")
}

func TestWorkspaceRoutesDoNotSpendTheRefreshBudget(t *testing.T) {
	h := authHandler(t, WithSessionRateLimit(3))
	owner := register(t, h, uniqueEmail(t))
	for range 3 {
		call(t, h, http.MethodGet, "/workspaces", owner.Token, nil)
	}
	wantStatus(t, call(t, h, http.MethodGet, "/workspaces", owner.Token, nil), http.StatusTooManyRequests, "over the workspace budget")
	wantStatus(t, refresh(t, h, owner.RefreshToken), http.StatusOK, "refresh")
}

func withRouteParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}
