package model

import "time"

// WorkspaceRole is what a member may do in a workspace.
type WorkspaceRole string

const (
	RoleOwner  WorkspaceRole = "owner"
	RoleAdmin  WorkspaceRole = "admin"
	RoleMember WorkspaceRole = "member"
)

// Valid reports whether r is one of the known roles.
func (r WorkspaceRole) Valid() bool {
	return r == RoleOwner || r == RoleAdmin || r == RoleMember
}

// Workspace is the tenant boundary. Role is the caller's role in it.
type Workspace struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Role      WorkspaceRole `json:"role"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// WorkspaceMember is a user's membership, with the identity fellow members see.
// Name is empty on the result of adding a member, which reveals no more of the
// user than the email the caller already typed.
type WorkspaceMember struct {
	UserID    string        `json:"user_id"`
	Email     string        `json:"email"`
	Name      string        `json:"name,omitempty"`
	Role      WorkspaceRole `json:"role"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}
