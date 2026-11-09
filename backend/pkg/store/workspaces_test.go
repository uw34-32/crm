package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"crm/pkg/model"
	"crm/pkg/testdb"
)

func newWorkspace(t *testing.T, s *Store, owner model.User) model.Workspace {
	t.Helper()
	w, err := s.CreateWorkspace(context.Background(), owner.ID, "Acme "+owner.ID)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	return w
}

func addMember(t *testing.T, s *Store, w model.Workspace, actor model.User, role model.WorkspaceRole) model.User {
	t.Helper()
	u := newUser(t, s)
	if _, err := s.AddMember(context.Background(), w.ID, actor.ID, u.Email, role); err != nil {
		t.Fatalf("AddMember(%s): %v", role, err)
	}
	return u
}

func TestCreateWorkspaceMakesCreatorOwner(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	if w.Role != model.RoleOwner || w.ID == "" {
		t.Errorf("workspace = %+v", w)
	}
	got, err := s.WorkspaceForMember(ctx, w.ID, owner.ID)
	if err != nil || got.Role != model.RoleOwner {
		t.Fatalf("WorkspaceForMember = %+v, %v", got, err)
	}
	members, err := s.ListMembers(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].UserID != owner.ID || members[0].Role != model.RoleOwner {
		t.Errorf("members = %+v", members)
	}
}

// The workspace and its owner are one transaction, so a failed owner insert
// leaves no workspace that nobody could ever reach.
func TestCreateWorkspaceIsAtomic(t *testing.T) {
	pool := testdb.AppPool(t)
	s := New(pool)
	ctx := context.Background()
	marker := "atomic " + uniqueEmail(t)
	_, err := s.CreateWorkspace(ctx, "00000000-0000-0000-0000-000000000000", marker)
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
	var count int
	if err := ownerPool(t).QueryRow(ctx, `SELECT count(*) FROM workspaces WHERE name = $1`, marker).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("%d workspaces left behind by a failed create", count)
	}
}

func TestListWorkspacesShowsOnlyOwnLiveOnes(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	alice, bob := newUser(t, s), newUser(t, s)
	mine := newWorkspace(t, s, alice)
	gone := newWorkspace(t, s, alice)
	newWorkspace(t, s, bob)
	if err := s.DeleteWorkspace(ctx, gone.ID, alice.ID); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListWorkspaces(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != mine.ID || list[0].Role != model.RoleOwner {
		t.Errorf("list = %+v, want only %s", list, mine.ID)
	}
}

func TestWorkspaceForMemberNotFoundCases(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner, stranger := newUser(t, s), newUser(t, s)
	w := newWorkspace(t, s, owner)
	deleted := newWorkspace(t, s, owner)
	if err := s.DeleteWorkspace(ctx, deleted.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, workspace, user string }{
		{"non-member", w.ID, stranger.ID},
		{"nonexistent", "00000000-0000-0000-0000-000000000000", owner.ID},
		{"deleted", deleted.ID, owner.ID},
		{"malformed workspace", "not-a-uuid", owner.ID},
		{"NUL in workspace", "a\x00b", owner.ID},
		{"malformed user", w.ID, "not-a-uuid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.WorkspaceForMember(ctx, tt.workspace, tt.user); !errors.Is(err, ErrWorkspaceNotFound) {
				t.Errorf("err = %v, want ErrWorkspaceNotFound", err)
			}
		})
	}
}

func TestAuthorizeMemberChange(t *testing.T) {
	o, a, m := model.RoleOwner, model.RoleAdmin, model.RoleMember
	tests := []struct {
		name               string
		actor, target, new model.WorkspaceRole
		allowed            bool
	}{
		{"owner promotes to owner", o, m, o, true},
		{"owner demotes owner", o, o, m, true},
		{"owner adds owner", o, "", o, true},
		{"admin changes member", a, m, a, true},
		{"admin removes member", a, m, "", true},
		{"admin adds admin", a, "", a, true},
		{"admin alters owner", a, o, m, false},
		{"admin removes owner", a, o, "", false},
		{"admin promotes to owner", a, m, o, false},
		{"admin adds owner", a, "", o, false},
		{"member changes member", m, m, a, false},
		{"member removes member", m, m, "", false},
		{"member adds member", m, "", m, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := authorizeMemberChange(tt.actor, tt.target, tt.new)
			if tt.allowed && err != nil {
				t.Errorf("err = %v, want allowed", err)
			}
			if !tt.allowed && !errors.Is(err, ErrWorkspaceForbidden) {
				t.Errorf("err = %v, want ErrWorkspaceForbidden", err)
			}
		})
	}
}

func TestMemberRoleLimits(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	admin := addMember(t, s, w, owner, model.RoleAdmin)
	member := addMember(t, s, w, owner, model.RoleMember)
	other := addMember(t, s, w, admin, model.RoleMember)
	secondOwner := addMember(t, s, w, owner, model.RoleOwner)

	forbidden := []struct {
		name string
		run  func() error
	}{
		{"member adds", func() error {
			_, err := s.AddMember(ctx, w.ID, member.ID, newUser(t, s).Email, model.RoleMember)
			return err
		}},
		{"member probes an unknown email", func() error {
			_, err := s.AddMember(ctx, w.ID, member.ID, uniqueEmail(t), model.RoleMember)
			return err
		}},
		{"member changes role", func() error {
			_, err := s.ChangeMemberRole(ctx, w.ID, member.ID, other.ID, model.RoleAdmin)
			return err
		}},
		{"member promotes self", func() error {
			_, err := s.ChangeMemberRole(ctx, w.ID, member.ID, member.ID, model.RoleOwner)
			return err
		}},
		{"member removes", func() error { return s.RemoveMember(ctx, w.ID, member.ID, other.ID) }},
		{"member renames", func() error {
			_, err := s.RenameWorkspace(ctx, w.ID, member.ID, "x")
			return err
		}},
		{"member deletes", func() error { return s.DeleteWorkspace(ctx, w.ID, member.ID) }},
		{"admin adds owner", func() error {
			_, err := s.AddMember(ctx, w.ID, admin.ID, newUser(t, s).Email, model.RoleOwner)
			return err
		}},
		{"admin promotes to owner", func() error {
			_, err := s.ChangeMemberRole(ctx, w.ID, admin.ID, other.ID, model.RoleOwner)
			return err
		}},
		{"admin demotes owner", func() error {
			_, err := s.ChangeMemberRole(ctx, w.ID, admin.ID, secondOwner.ID, model.RoleMember)
			return err
		}},
		{"admin removes owner", func() error { return s.RemoveMember(ctx, w.ID, admin.ID, secondOwner.ID) }},
		{"admin deletes", func() error { return s.DeleteWorkspace(ctx, w.ID, admin.ID) }},
	}
	for _, tt := range forbidden {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, ErrWorkspaceForbidden) {
				t.Errorf("err = %v, want ErrWorkspaceForbidden", err)
			}
		})
	}

	// Nothing above changed anyone.
	members, err := s.ListMembers(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 5 {
		t.Errorf("%d members after refused changes, want 5", len(members))
	}

	// And what the roles may do works.
	if _, err := s.ChangeMemberRole(ctx, w.ID, admin.ID, other.ID, model.RoleAdmin); err != nil {
		t.Errorf("admin changes member: %v", err)
	}
	if err := s.RemoveMember(ctx, w.ID, admin.ID, other.ID); err != nil {
		t.Errorf("admin removes admin: %v", err)
	}
	if _, err := s.RenameWorkspace(ctx, w.ID, admin.ID, "Renamed"); err != nil {
		t.Errorf("admin renames: %v", err)
	}
	if err := s.DeleteWorkspace(ctx, w.ID, secondOwner.ID); err != nil {
		t.Errorf("owner deletes: %v", err)
	}
}

func TestRoleIsReadInsideTheTransactionNotTrustedFromTheCaller(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	admin := addMember(t, s, w, owner, model.RoleAdmin)
	member := addMember(t, s, w, owner, model.RoleMember)

	// A caller that passed the middleware as admin, then was demoted.
	if _, err := s.ChangeMemberRole(ctx, w.ID, owner.ID, admin.ID, model.RoleMember); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveMember(ctx, w.ID, admin.ID, member.ID); !errors.Is(err, ErrWorkspaceForbidden) {
		t.Errorf("demoted admin = %v, want ErrWorkspaceForbidden", err)
	}
	// A caller that passed the middleware, then was removed.
	if err := s.RemoveMember(ctx, w.ID, owner.ID, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RenameWorkspace(ctx, w.ID, admin.ID, "x"); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("removed member = %v, want ErrWorkspaceNotFound", err)
	}
}

func TestAddMemberResolvesEmailMinimally(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	u := newUser(t, s)

	got, err := s.AddMember(ctx, w.ID, owner.ID, strings.ToUpper(u.Email), model.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != u.ID || got.Role != model.RoleMember || got.Email != u.Email {
		t.Errorf("member = %+v", got)
	}
	if got.Name != "" {
		t.Errorf("adding a member revealed the name %q", got.Name)
	}
	if _, err := s.AddMember(ctx, w.ID, owner.ID, u.Email, model.RoleMember); !errors.Is(err, ErrAlreadyMember) {
		t.Errorf("second add = %v, want ErrAlreadyMember", err)
	}
	if _, err := s.AddMember(ctx, w.ID, owner.ID, uniqueEmail(t), model.RoleMember); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown email = %v, want ErrUserNotFound", err)
	}
}

func TestChangeAndRemoveUnknownMember(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	outsider := newUser(t, s)
	for _, target := range []string{outsider.ID, "not-a-uuid"} {
		if _, err := s.ChangeMemberRole(ctx, w.ID, owner.ID, target, model.RoleAdmin); !errors.Is(err, ErrMemberNotFound) {
			t.Errorf("change %s = %v, want ErrMemberNotFound", target, err)
		}
		if err := s.RemoveMember(ctx, w.ID, owner.ID, target); !errors.Is(err, ErrMemberNotFound) {
			t.Errorf("remove %s = %v, want ErrMemberNotFound", target, err)
		}
	}
}

func TestLastOwnerIsProtected(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	if _, err := s.ChangeMemberRole(ctx, w.ID, owner.ID, owner.ID, model.RoleMember); !errors.Is(err, ErrLastOwner) {
		t.Errorf("demote = %v, want ErrLastOwner", err)
	}
	if err := s.RemoveMember(ctx, w.ID, owner.ID, owner.ID); !errors.Is(err, ErrLastOwner) {
		t.Errorf("remove = %v, want ErrLastOwner", err)
	}
	if err := s.LeaveWorkspace(ctx, w.ID, owner.ID); !errors.Is(err, ErrLastOwner) {
		t.Errorf("leave = %v, want ErrLastOwner", err)
	}

	second := addMember(t, s, w, owner, model.RoleOwner)
	if err := s.LeaveWorkspace(ctx, w.ID, owner.ID); err != nil {
		t.Errorf("leave with a second owner: %v", err)
	}
	if err := s.LeaveWorkspace(ctx, w.ID, second.ID); !errors.Is(err, ErrLastOwner) {
		t.Errorf("leave as the remaining owner = %v, want ErrLastOwner", err)
	}
}

// Two owners acting on each other at once is the race that could leave a
// workspace with none. Rounds raise the odds of hitting the window when the lock
// is missing; with it, every round must hold.
func TestLastOwnerHoldsUnderConcurrency(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	demote := func(w model.Workspace, actor, target model.User) func() error {
		return func() error {
			_, err := s.ChangeMemberRole(ctx, w.ID, actor.ID, target.ID, model.RoleMember)
			return err
		}
	}
	remove := func(w model.Workspace, actor, target model.User) func() error {
		return func() error { return s.RemoveMember(ctx, w.ID, actor.ID, target.ID) }
	}
	leave := func(w model.Workspace, actor, _ model.User) func() error {
		return func() error { return s.LeaveWorkspace(ctx, w.ID, actor.ID) }
	}
	type op func(model.Workspace, model.User, model.User) func() error
	tests := []struct {
		name string
		a, b op
	}{
		{"mutual demotion", demote, demote},
		{"mutual removal", remove, remove},
		{"both leave", leave, leave},
		{"one leaves while demoting the other", leave, demote},
		{"one removes while the other demotes", remove, demote},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for round := range 25 {
				first := newUser(t, s)
				w := newWorkspace(t, s, first)
				second := addMember(t, s, w, first, model.RoleOwner)

				var wg sync.WaitGroup
				start := make(chan struct{})
				errs := make([]error, 2)
				for i, run := range []func() error{tt.a(w, first, second), tt.b(w, second, first)} {
					wg.Go(func() {
						<-start
						errs[i] = run()
					})
				}
				close(start)
				wg.Wait()

				for _, err := range errs {
					if err != nil && !errors.Is(err, ErrLastOwner) && !errors.Is(err, ErrWorkspaceForbidden) &&
						!errors.Is(err, ErrWorkspaceNotFound) && !errors.Is(err, ErrMemberNotFound) {
						t.Fatalf("round %d: unexpected error: %v", round, err)
					}
				}
				var owners int
				if err := s.InWorkspace(ctx, w.ID, func(q Querier) error {
					return q.QueryRow(ctx, `SELECT count(*) FROM workspace_members WHERE workspace_id = $1 AND role = 'owner'`, w.ID).Scan(&owners)
				}); err != nil {
					t.Fatal(err)
				}
				if owners < 1 {
					t.Fatalf("round %d: %d owners left: %v", round, owners, errs)
				}
			}
		})
	}
}

// A delete that loses the race to a member change must still leave the change
// refused, because the row lock makes one of them see the other's commit.
func TestChangesAfterDeleteAreRefused(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	member := addMember(t, s, w, owner, model.RoleMember)
	if err := s.DeleteWorkspace(ctx, w.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteWorkspace(ctx, w.ID, owner.ID); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("second delete = %v, want ErrWorkspaceNotFound", err)
	}
	if _, err := s.RenameWorkspace(ctx, w.ID, owner.ID, "x"); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("rename = %v, want ErrWorkspaceNotFound", err)
	}
	if err := s.RemoveMember(ctx, w.ID, owner.ID, member.ID); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("remove = %v, want ErrWorkspaceNotFound", err)
	}
	if err := s.LeaveWorkspace(ctx, w.ID, member.ID); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("leave = %v, want ErrWorkspaceNotFound", err)
	}
}

func TestRenameUpdatesUpdatedAt(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	got, err := s.RenameWorkspace(ctx, w.ID, owner.ID, "New name")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "New name" || !got.UpdatedAt.After(w.UpdatedAt) || got.Role != model.RoleOwner {
		t.Errorf("renamed = %+v, was %+v", got, w)
	}
}
