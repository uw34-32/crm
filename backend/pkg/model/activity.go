package model

import "time"

// ActivityKind says what an activity records. Only a task has a due time, an
// assignee, and a completed state: a note, call, or meeting records something that
// already happened.
type ActivityKind string

const (
	ActivityNote    ActivityKind = "note"
	ActivityCall    ActivityKind = "call"
	ActivityMeeting ActivityKind = "meeting"
	ActivityTask    ActivityKind = "task"
)

// Valid reports whether k is a known kind.
func (k ActivityKind) Valid() bool {
	switch k {
	case ActivityNote, ActivityCall, ActivityMeeting, ActivityTask:
		return true
	}
	return false
}

// Activity is a note, call, meeting, or task about a contact, a company, or a deal,
// linked to at least one of them. The names are the linked rows' current names.
// AuthorID is nil once the author leaves the workspace, and AssigneeID once the
// assignee does.
type Activity struct {
	ID          string       `json:"id"`
	WorkspaceID string       `json:"workspace_id"`
	Kind        ActivityKind `json:"kind"`
	Subject     string       `json:"subject"`
	Body        string       `json:"body"`
	DueAt       *time.Time   `json:"due_at"`
	CompletedAt *time.Time   `json:"completed_at"`
	AuthorID    *string      `json:"author_id"`
	AssigneeID  *string      `json:"assignee_id"`
	ContactID   *string      `json:"contact_id"`
	ContactName *string      `json:"contact_name"`
	CompanyID   *string      `json:"company_id"`
	CompanyName *string      `json:"company_name"`
	DealID      *string      `json:"deal_id"`
	DealTitle   *string      `json:"deal_title"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// ActivityInput holds the fields of a new activity. AuthorID is the member creating it.
type ActivityInput struct {
	Kind       ActivityKind
	Subject    string
	Body       string
	DueAt      *time.Time
	AuthorID   string
	AssigneeID *string
	ContactID  *string
	CompanyID  *string
	DealID     *string
}

// ActivityPatch holds the fields to change. A nil text field stays as it is. The due
// time, the assignee, and each link change only when their Set flag is true, and nil
// then clears them. The kind never changes, and the completed state changes only
// through completing or reopening a task.
type ActivityPatch struct {
	Subject     *string
	Body        *string
	SetDue      bool
	DueAt       *time.Time
	SetAssignee bool
	AssigneeID  *string
	SetContact  bool
	ContactID   *string
	SetCompany  bool
	CompanyID   *string
	SetDeal     bool
	DealID      *string
}

// ActivityState filters tasks by whether they are done.
type ActivityState string

const (
	ActivityOpen      ActivityState = "open"
	ActivityCompleted ActivityState = "completed"
)

// Valid reports whether s is a known state.
func (s ActivityState) Valid() bool {
	return s == ActivityOpen || s == ActivityCompleted
}

// ActivityFilter narrows a list of activities. An empty field does not filter. A
// state or a due bound matches tasks only, since nothing else has them.
type ActivityFilter struct {
	ContactID  string
	CompanyID  string
	DealID     string
	AssigneeID string
	Kind       ActivityKind
	State      ActivityState
	DueBefore  *time.Time
	DueAfter   *time.Time
}
