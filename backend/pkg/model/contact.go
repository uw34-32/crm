package model

import "time"

// Contact is a person a workspace deals with. An empty optional field means the
// value is not known. CompanyID is nil when the contact has no company, and
// CompanyName is the linked company's current name.
type Contact struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	FirstName   string    `json:"first_name"`
	LastName    string    `json:"last_name"`
	Email       string    `json:"email"`
	Phone       string    `json:"phone"`
	Title       string    `json:"title"`
	CompanyID   *string   `json:"company_id"`
	CompanyName *string   `json:"company_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ContactInput holds the fields of a new contact.
type ContactInput struct {
	FirstName string
	LastName  string
	Email     string
	Phone     string
	Title     string
	CompanyID *string
}

// ContactPatch holds the fields to change. A nil field stays as it is. The link
// changes only when SetCompany is true, and a nil CompanyID then removes it.
type ContactPatch struct {
	FirstName  *string
	LastName   *string
	Email      *string
	Phone      *string
	Title      *string
	SetCompany bool
	CompanyID  *string
}
