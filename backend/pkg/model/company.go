package model

import "time"

// Company is an organization a workspace deals with. An empty optional field
// means the value is not known.
type Company struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Domain      string    `json:"domain"`
	Phone       string    `json:"phone"`
	Industry    string    `json:"industry"`
	Address     string    `json:"address"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CompanyInput holds the fields of a new company.
type CompanyInput struct {
	Name     string
	Domain   string
	Phone    string
	Industry string
	Address  string
}

// CompanyPatch holds the fields to change. Nil leaves a field as it is.
type CompanyPatch struct {
	Name     *string
	Domain   *string
	Phone    *string
	Industry *string
	Address  *string
}
