package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"crm/pkg/model"
	"crm/pkg/store"

	"github.com/go-chi/chi/v5"
)

const (
	workspaceParam = "workspaceID"
	memberParam    = "userID"

	msgWorkspaceNotFound = "workspace not found"
	msgForbidden         = "you do not have permission to do this"
)

type workspaceContextKey struct{}

// workspaceNotFound is the one answer for a workspace the caller cannot reach,
// whatever the reason, so the response does not reveal that it exists.
func workspaceNotFound(w http.ResponseWriter) {
	errResp(w, http.StatusNotFound, msgWorkspaceNotFound)
}

// requireWorkspace resolves the workspace in the route from the caller's checked
// membership, and stores it with the caller's role. Every failure ends the
// request: nobody reaches a handler without a confirmed membership. It must run
// after requireAuth. Later handlers take the workspace ID from this context, never
// from the request.
func (a *App) requireWorkspace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(contextKey{}).(string)
		if !ok {
			unauthorized(w)
			return
		}
		ctx, cancel := a.dbContext(r)
		defer cancel()
		workspace, err := a.store.WorkspaceForMember(ctx, chi.URLParam(r, workspaceParam), userID)
		if errors.Is(err, store.ErrWorkspaceNotFound) {
			workspaceNotFound(w)
			return
		}
		if err != nil {
			log.Printf("workspace membership: %v", err)
			errResp(w, http.StatusInternalServerError, msgInternal)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), workspaceContextKey{}, workspace)))
	})
}

// workspaceActor returns what requireWorkspace established. A handler mounted
// without that middleware finds nothing and answers 404 rather than proceed.
func workspaceActor(w http.ResponseWriter, r *http.Request) (model.Workspace, string, bool) {
	workspace, ok := r.Context().Value(workspaceContextKey{}).(model.Workspace)
	userID, hasUser := r.Context().Value(contextKey{}).(string)
	if !ok || !hasUser {
		workspaceNotFound(w)
		return model.Workspace{}, "", false
	}
	return workspace, userID, true
}

func workspaceFailure(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, store.ErrWorkspaceNotFound):
		workspaceNotFound(w)
	case errors.Is(err, store.ErrWorkspaceForbidden):
		errResp(w, http.StatusForbidden, msgForbidden)
	case errors.Is(err, store.ErrMemberNotFound):
		errResp(w, http.StatusNotFound, "member not found")
	case errors.Is(err, store.ErrUserNotFound):
		errResp(w, http.StatusNotFound, "no registered user has that email")
	case errors.Is(err, store.ErrAlreadyMember):
		errResp(w, http.StatusConflict, "that user is already a member")
	case errors.Is(err, store.ErrLastOwner):
		errResp(w, http.StatusConflict, "a workspace must keep at least one owner")
	default:
		log.Printf("%s: %v", op, err)
		errResp(w, http.StatusInternalServerError, msgInternal)
	}
}

func decodeWorkspaceName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &req); err != nil || hasNUL(req.Name) {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return "", false
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > maxNameRunes {
		errResp(w, http.StatusBadRequest, "a name of at most 200 characters is required")
		return "", false
	}
	return name, true
}

func decodeRole(raw string, w http.ResponseWriter) (model.WorkspaceRole, bool) {
	role := model.WorkspaceRole(raw)
	if !role.Valid() {
		errResp(w, http.StatusBadRequest, "role must be owner, admin, or member")
		return "", false
	}
	return role, true
}

func (a *App) createWorkspace(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKey{}).(string)
	if !ok {
		unauthorized(w)
		return
	}
	name, ok := decodeWorkspaceName(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	workspace, err := a.store.CreateWorkspace(ctx, userID, name)
	if errors.Is(err, store.ErrUserNotFound) {
		// A valid token for an account that no longer exists.
		unauthorized(w)
		return
	}
	if err != nil {
		workspaceFailure(w, "create workspace", err)
		return
	}
	dataResp(w, http.StatusCreated, workspace)
}

func (a *App) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKey{}).(string)
	if !ok {
		unauthorized(w)
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	list, err := a.store.ListWorkspaces(ctx, userID)
	if err != nil {
		workspaceFailure(w, "list workspaces", err)
		return
	}
	dataResp(w, http.StatusOK, list)
}

func (a *App) getWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	dataResp(w, http.StatusOK, workspace)
}

func (a *App) renameWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	name, ok := decodeWorkspaceName(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	renamed, err := a.store.RenameWorkspace(ctx, workspace.ID, userID, name)
	if err != nil {
		workspaceFailure(w, "rename workspace", err)
		return
	}
	dataResp(w, http.StatusOK, renamed)
}

func (a *App) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	if err := a.store.DeleteWorkspace(ctx, workspace.ID, userID); err != nil {
		workspaceFailure(w, "delete workspace", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) leaveWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	if err := a.store.LeaveWorkspace(ctx, workspace.ID, userID); err != nil {
		workspaceFailure(w, "leave workspace", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) listMembers(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	members, err := a.store.ListMembers(ctx, workspace.ID)
	if err != nil {
		workspaceFailure(w, "list members", err)
		return
	}
	dataResp(w, http.StatusOK, members)
}

func (a *App) addMember(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := decodeJSON(w, r, &req); err != nil || hasNUL(req.Email, req.Role) {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	email := strings.TrimSpace(req.Email)
	if !validEmail(email) {
		errResp(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	role, ok := decodeRole(req.Role, w)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	member, err := a.store.AddMember(ctx, workspace.ID, userID, email, role)
	if err != nil {
		workspaceFailure(w, "add member", err)
		return
	}
	dataResp(w, http.StatusCreated, member)
}

func (a *App) changeMemberRole(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	role, ok := decodeRole(req.Role, w)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	member, err := a.store.ChangeMemberRole(ctx, workspace.ID, userID, chi.URLParam(r, memberParam), role)
	if err != nil {
		workspaceFailure(w, "change member role", err)
		return
	}
	dataResp(w, http.StatusOK, member)
}

func (a *App) removeMember(w http.ResponseWriter, r *http.Request) {
	workspace, userID, ok := workspaceActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.dbContext(r)
	defer cancel()
	if err := a.store.RemoveMember(ctx, workspace.ID, userID, chi.URLParam(r, memberParam)); err != nil {
		workspaceFailure(w, "remove member", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
