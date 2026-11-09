package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"crm/pkg/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	pgForeignKeyViolation = "23503"

	// maxListedRows caps the unpaginated lists. A workspace or a user's list of
	// workspaces beyond this is outside what this version is built for.
	maxListedRows = 1000
)

var (
	// ErrWorkspaceNotFound covers a nonexistent workspace, a deleted one, and one
	// the user does not belong to, so callers cannot tell them apart.
	ErrWorkspaceNotFound = errors.New("workspace not found")
	// ErrWorkspaceForbidden means the member's role does not allow the action.
	ErrWorkspaceForbidden = errors.New("not allowed in this workspace")
	// ErrMemberNotFound means the target user is not a member of the workspace.
	ErrMemberNotFound = errors.New("member not found")
	// ErrAlreadyMember means the user already belongs to the workspace.
	ErrAlreadyMember = errors.New("user is already a member")
	// ErrLastOwner means the change would leave the workspace with no owner.
	ErrLastOwner = errors.New("a workspace must keep at least one owner")
)

func hasCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

const workspaceColumns = `w.id::text, w.name, w.created_at, w.updated_at`

func scanWorkspace(row pgx.Row, role *model.WorkspaceRole) (model.Workspace, error) {
	var w model.Workspace
	dest := []any{&w.ID, &w.Name, &w.CreatedAt, &w.UpdatedAt}
	if role != nil {
		dest = append(dest, role)
	}
	if err := row.Scan(dest...); err != nil {
		return model.Workspace{}, err
	}
	if role != nil {
		w.Role = *role
	}
	return w, nil
}

// CreateWorkspace inserts a workspace, its owner, and its default pipeline in one
// transaction. The ID is generated first because the policies check every insert
// against the setting, so the setting has to be known before the row exists. A
// freshly generated ID cannot name anyone else's workspace, so no membership check
// applies here. It returns ErrUserNotFound if the owner does not exist.
func (s *Store) CreateWorkspace(ctx context.Context, ownerID, name string) (model.Workspace, error) {
	if !validUUID(ownerID) {
		return model.Workspace{}, ErrUserNotFound
	}
	var id string
	if err := s.pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		return model.Workspace{}, fmt.Errorf("generate workspace id: %w", err)
	}

	var created model.Workspace
	err := s.InWorkspace(ctx, id, func(q Querier) error {
		var err error
		created, err = scanWorkspace(q.QueryRow(ctx,
			`INSERT INTO workspaces AS w (id, name) VALUES ($1::text::uuid, $2) RETURNING `+workspaceColumns, id, name), nil)
		if err != nil {
			return fmt.Errorf("insert workspace: %w", err)
		}
		_, err = q.Exec(ctx,
			`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1::text::uuid, $2::text::uuid, $3)`,
			id, ownerID, model.RoleOwner)
		if hasCode(err, pgForeignKeyViolation) {
			return ErrUserNotFound
		}
		if err != nil {
			return fmt.Errorf("insert workspace owner: %w", err)
		}
		if _, err := q.Exec(ctx, `SELECT create_pipeline_with_default_stages($1::text::uuid)`, id); err != nil {
			return fmt.Errorf("create default pipeline: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.Workspace{}, err
	}
	created.Role = model.RoleOwner
	return created, nil
}

// ListWorkspaces returns the user's live workspaces with their role in each. It
// runs before any workspace is known, under the read-only app.user_id policies.
func (s *Store) ListWorkspaces(ctx context.Context, userID string) ([]model.Workspace, error) {
	if !validUUID(userID) {
		return []model.Workspace{}, nil
	}
	list := []model.Workspace{}
	err := s.inTenantTx(ctx, settingUser, userID, func(q Querier) error {
		rows, err := q.Query(ctx, `
			SELECT `+workspaceColumns+`, m.role
			  FROM workspace_members m
			  JOIN workspaces w ON w.id = m.workspace_id
			 WHERE m.user_id = $1::text::uuid AND w.deleted_at IS NULL
			 ORDER BY w.created_at, w.id
			 LIMIT $2`, userID, maxListedRows)
		if err != nil {
			return err
		}
		list, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.Workspace, error) {
			var role model.WorkspaceRole
			return scanWorkspace(row, &role)
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	return list, nil
}

// WorkspaceForMember returns a live workspace with the user's role in it, or
// ErrWorkspaceNotFound. This is the membership check: it runs before a workspace
// may be set, under the read-only app.user_id policies, which show the user only
// their own memberships.
func (s *Store) WorkspaceForMember(ctx context.Context, workspaceID, userID string) (model.Workspace, error) {
	if !validUUID(workspaceID) || !validUUID(userID) {
		return model.Workspace{}, ErrWorkspaceNotFound
	}
	var w model.Workspace
	err := s.inTenantTx(ctx, settingUser, userID, func(q Querier) error {
		var role model.WorkspaceRole
		var err error
		w, err = scanWorkspace(q.QueryRow(ctx, `
			SELECT `+workspaceColumns+`, m.role
			  FROM workspace_members m
			  JOIN workspaces w ON w.id = m.workspace_id
			 WHERE m.workspace_id = $1::text::uuid AND m.user_id = $2::text::uuid AND w.deleted_at IS NULL`,
			workspaceID, userID), &role)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Workspace{}, ErrWorkspaceNotFound
	}
	if err != nil {
		return model.Workspace{}, fmt.Errorf("read workspace membership: %w", err)
	}
	return w, nil
}

// lockActor takes the workspace's row lock and returns the actor's current role.
// Every change to a workspace or its members starts here, which gives three
// guarantees: changes to one workspace run one at a time, so the owner count
// cannot be read stale; the role is the one in force now, not the one the
// middleware saw; and a workspace deleted in the meantime is noticed.
//
// FOR NO KEY UPDATE, because it conflicts with itself and with the soft delete,
// but not with the foreign key checks of rows inserted into this workspace.
func lockActor(ctx context.Context, q Querier, workspaceID, actorID string) (model.WorkspaceRole, error) {
	if !validUUID(actorID) {
		return "", ErrWorkspaceNotFound
	}
	var live int
	err := q.QueryRow(ctx,
		`SELECT 1 FROM workspaces WHERE id = $1::text::uuid AND deleted_at IS NULL FOR NO KEY UPDATE`, workspaceID,
	).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrWorkspaceNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lock workspace: %w", err)
	}
	var role model.WorkspaceRole
	err = q.QueryRow(ctx,
		`SELECT role FROM workspace_members WHERE workspace_id = $1::text::uuid AND user_id = $2::text::uuid`,
		workspaceID, actorID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrWorkspaceNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read actor role: %w", err)
	}
	return role, nil
}

func memberRole(ctx context.Context, q Querier, workspaceID, userID string) (model.WorkspaceRole, error) {
	if !validUUID(userID) {
		return "", ErrMemberNotFound
	}
	var role model.WorkspaceRole
	err := q.QueryRow(ctx,
		`SELECT role FROM workspace_members WHERE workspace_id = $1::text::uuid AND user_id = $2::text::uuid`,
		workspaceID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrMemberNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read member role: %w", err)
	}
	return role, nil
}

// protectLastOwner refuses to take away the role of the only owner. The caller
// must hold the lock from lockActor.
func protectLastOwner(ctx context.Context, q Querier, workspaceID string, current model.WorkspaceRole) error {
	if current != model.RoleOwner {
		return nil
	}
	var owners int
	if err := q.QueryRow(ctx,
		`SELECT count(*) FROM workspace_members WHERE workspace_id = $1::text::uuid AND role = $2`,
		workspaceID, model.RoleOwner).Scan(&owners); err != nil {
		return fmt.Errorf("count owners: %w", err)
	}
	if owners <= 1 {
		return ErrLastOwner
	}
	return nil
}

func canManageMembers(role model.WorkspaceRole) bool {
	return role == model.RoleOwner || role == model.RoleAdmin
}

// authorizeMemberChange applies the role rules: owners and admins manage members,
// and only an owner may touch an owner or make one. target is empty when adding a
// member, and next is empty when removing one.
func authorizeMemberChange(actor, target, next model.WorkspaceRole) error {
	if !canManageMembers(actor) {
		return ErrWorkspaceForbidden
	}
	if actor != model.RoleOwner && (target == model.RoleOwner || next == model.RoleOwner) {
		return ErrWorkspaceForbidden
	}
	return nil
}

// RenameWorkspace is open to owners and admins.
func (s *Store) RenameWorkspace(ctx context.Context, workspaceID, actorID, name string) (model.Workspace, error) {
	var w model.Workspace
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		actor, err := lockActor(ctx, q, workspaceID, actorID)
		if err != nil {
			return err
		}
		if !canManageMembers(actor) {
			return ErrWorkspaceForbidden
		}
		w, err = scanWorkspace(q.QueryRow(ctx, `
			UPDATE workspaces AS w SET name = $2, updated_at = now()
			 WHERE w.id = $1::text::uuid AND w.deleted_at IS NULL
			 RETURNING `+workspaceColumns, workspaceID, name), nil)
		if err != nil {
			return fmt.Errorf("rename workspace: %w", err)
		}
		w.Role = actor
		return nil
	})
	if err != nil {
		return model.Workspace{}, err
	}
	return w, nil
}

// DeleteWorkspace soft-deletes a workspace and is open to owners only. Memberships
// stay in place, and every route answers as if the workspace never existed.
func (s *Store) DeleteWorkspace(ctx context.Context, workspaceID, actorID string) error {
	return s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		actor, err := lockActor(ctx, q, workspaceID, actorID)
		if err != nil {
			return err
		}
		if actor != model.RoleOwner {
			return ErrWorkspaceForbidden
		}
		if _, err := q.Exec(ctx,
			`UPDATE workspaces SET deleted_at = now(), updated_at = now() WHERE id = $1::text::uuid AND deleted_at IS NULL`,
			workspaceID); err != nil {
			return fmt.Errorf("delete workspace: %w", err)
		}
		return nil
	})
}

const memberColumns = `m.user_id::text, u.email, u.name, m.role, m.created_at, m.updated_at`

func scanMember(row pgx.Row) (model.WorkspaceMember, error) {
	var m model.WorkspaceMember
	if err := row.Scan(&m.UserID, &m.Email, &m.Name, &m.Role, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return model.WorkspaceMember{}, err
	}
	return m, nil
}

// ListMembers returns the workspace's members, oldest first.
func (s *Store) ListMembers(ctx context.Context, workspaceID string) ([]model.WorkspaceMember, error) {
	members := []model.WorkspaceMember{}
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		rows, err := q.Query(ctx, `
			SELECT `+memberColumns+`
			  FROM workspace_members m
			  JOIN users u ON u.id = m.user_id
			 WHERE m.workspace_id = $1::text::uuid
			 ORDER BY m.created_at, m.user_id
			 LIMIT $2`, workspaceID, maxListedRows)
		if err != nil {
			return fmt.Errorf("list members: %w", err)
		}
		members, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.WorkspaceMember, error) { return scanMember(row) })
		if err != nil {
			return fmt.Errorf("list members: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return members, nil
}

// AddMember adds an existing user by email. Authorization comes first, so only
// someone who may manage members can use this to learn whether an email has an
// account. The lookup reads the ID and nothing else, and the result holds no
// more about the user than the caller supplied. It returns ErrUserNotFound for an
// unknown email and ErrAlreadyMember for an existing member.
func (s *Store) AddMember(ctx context.Context, workspaceID, actorID, email string, role model.WorkspaceRole) (model.WorkspaceMember, error) {
	var m model.WorkspaceMember
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		actor, err := lockActor(ctx, q, workspaceID, actorID)
		if err != nil {
			return err
		}
		if err := authorizeMemberChange(actor, "", role); err != nil {
			return err
		}
		var userID string
		var stored string
		err = q.QueryRow(ctx, `SELECT id::text, email FROM users WHERE lower(email) = lower($1)`, email).Scan(&userID, &stored)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		if err != nil {
			return fmt.Errorf("resolve member email: %w", err)
		}
		var createdAt, updatedAt time.Time
		err = q.QueryRow(ctx, `
			INSERT INTO workspace_members (workspace_id, user_id, role)
			VALUES ($1::text::uuid, $2::text::uuid, $3)
			RETURNING created_at, updated_at`, workspaceID, userID, role).Scan(&createdAt, &updatedAt)
		if hasCode(err, pgUniqueViolation) {
			return ErrAlreadyMember
		}
		if err != nil {
			return fmt.Errorf("insert member: %w", err)
		}
		m = model.WorkspaceMember{UserID: userID, Email: stored, Role: role, CreatedAt: createdAt, UpdatedAt: updatedAt}
		return nil
	})
	if err != nil {
		return model.WorkspaceMember{}, err
	}
	return m, nil
}

// ChangeMemberRole changes a member's role under the role rules, and refuses to
// leave the workspace without an owner.
func (s *Store) ChangeMemberRole(ctx context.Context, workspaceID, actorID, targetID string, role model.WorkspaceRole) (model.WorkspaceMember, error) {
	var m model.WorkspaceMember
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		actor, err := lockActor(ctx, q, workspaceID, actorID)
		if err != nil {
			return err
		}
		if !canManageMembers(actor) {
			return ErrWorkspaceForbidden
		}
		current, err := memberRole(ctx, q, workspaceID, targetID)
		if err != nil {
			return err
		}
		if err := authorizeMemberChange(actor, current, role); err != nil {
			return err
		}
		if role != model.RoleOwner {
			if err := protectLastOwner(ctx, q, workspaceID, current); err != nil {
				return err
			}
		}
		if _, err := q.Exec(ctx, `
			UPDATE workspace_members SET role = $3, updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND user_id = $2::text::uuid`, workspaceID, targetID, role); err != nil {
			return fmt.Errorf("change member role: %w", err)
		}
		m, err = scanMember(q.QueryRow(ctx, `
			SELECT `+memberColumns+`
			  FROM workspace_members m
			  JOIN users u ON u.id = m.user_id
			 WHERE m.workspace_id = $1::text::uuid AND m.user_id = $2::text::uuid`, workspaceID, targetID))
		if err != nil {
			return fmt.Errorf("read member: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.WorkspaceMember{}, err
	}
	return m, nil
}

func deleteMember(ctx context.Context, q Querier, workspaceID, userID string) error {
	if _, err := q.Exec(ctx,
		`DELETE FROM workspace_members WHERE workspace_id = $1::text::uuid AND user_id = $2::text::uuid`,
		workspaceID, userID); err != nil {
		return fmt.Errorf("delete member: %w", err)
	}
	return nil
}

// RemoveMember removes another member under the role rules, and refuses to leave
// the workspace without an owner.
func (s *Store) RemoveMember(ctx context.Context, workspaceID, actorID, targetID string) error {
	return s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		actor, err := lockActor(ctx, q, workspaceID, actorID)
		if err != nil {
			return err
		}
		if !canManageMembers(actor) {
			return ErrWorkspaceForbidden
		}
		current, err := memberRole(ctx, q, workspaceID, targetID)
		if err != nil {
			return err
		}
		if err := authorizeMemberChange(actor, current, ""); err != nil {
			return err
		}
		if err := protectLastOwner(ctx, q, workspaceID, current); err != nil {
			return err
		}
		return deleteMember(ctx, q, workspaceID, targetID)
	})
}

// LeaveWorkspace removes the caller, whatever their role, unless they are the
// last owner.
func (s *Store) LeaveWorkspace(ctx context.Context, workspaceID, actorID string) error {
	return s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		actor, err := lockActor(ctx, q, workspaceID, actorID)
		if err != nil {
			return err
		}
		if err := protectLastOwner(ctx, q, workspaceID, actor); err != nil {
			return err
		}
		return deleteMember(ctx, q, workspaceID, actorID)
	})
}
