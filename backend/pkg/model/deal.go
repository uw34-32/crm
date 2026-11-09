package model

import "time"

// Deal is one sale moving through a pipeline. ValueMinor is in minor units of
// Currency, such as cents for USD. Status is the kind of the stage it is in.
// ExpectedCloseDate is YYYY-MM-DD. CompanyName and ContactName are the linked rows'
// current names. OwnerID is nil when the deal has no owner, which happens when its
// owner leaves the workspace. Position is its index in the stage on the board,
// starting at 0, with no gaps.
type Deal struct {
	ID                string     `json:"id"`
	WorkspaceID       string     `json:"workspace_id"`
	PipelineID        string     `json:"pipeline_id"`
	StageID           string     `json:"stage_id"`
	Status            StageKind  `json:"status"`
	Title             string     `json:"title"`
	ValueMinor        int64      `json:"value_minor"`
	Currency          string     `json:"currency"`
	ExpectedCloseDate *string    `json:"expected_close_date"`
	ClosedAt          *time.Time `json:"closed_at"`
	LostReason        string     `json:"lost_reason"`
	CompanyID         *string    `json:"company_id"`
	CompanyName       *string    `json:"company_name"`
	ContactID         *string    `json:"contact_id"`
	ContactName       *string    `json:"contact_name"`
	OwnerID           *string    `json:"owner_id"`
	Position          int        `json:"position"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// DealInput holds the fields of a new deal. A nil StageID means the pipeline's first
// open stage.
type DealInput struct {
	PipelineID        string
	StageID           *string
	Title             string
	ValueMinor        int64
	Currency          string
	ExpectedCloseDate *string
	CompanyID         *string
	ContactID         *string
	OwnerID           *string
}

// DealPatch holds the fields to change. A nil field stays as it is. A date or a link
// changes only when its Set flag is true, and nil then clears it. The stage and
// position change only through a move.
type DealPatch struct {
	Title             *string
	ValueMinor        *int64
	Currency          *string
	SetExpectedClose  bool
	ExpectedCloseDate *string
	SetCompany        bool
	CompanyID         *string
	SetContact        bool
	ContactID         *string
	SetOwner          bool
	OwnerID           *string
}

// DealMove says where to put a deal on the board. A nil Position, or one beyond the
// end, means the end of the stage.
type DealMove struct {
	StageID    string
	Position   *int
	LostReason string
}

// DealFilter narrows a list of deals. An empty field does not filter.
type DealFilter struct {
	PipelineID string
	StageID    string
	OwnerID    string
	Status     StageKind
}
