package model

import "time"

// StageKind says what being in a stage means for a deal. Nothing identifies won or
// lost by a stage's name.
type StageKind string

const (
	StageOpen StageKind = "open"
	StageWon  StageKind = "won"
	StageLost StageKind = "lost"
)

// Valid reports whether k is one of the known kinds.
func (k StageKind) Valid() bool {
	return k == StageOpen || k == StageWon || k == StageLost
}

// Pipeline is an ordered set of stages a workspace moves deals through. Stages is
// filled when one pipeline is read, and left out of lists.
type Pipeline struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	Name        string          `json:"name"`
	Stages      []PipelineStage `json:"stages,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// PipelineStage is one step of a pipeline. Position is its index among the
// pipeline's stages, starting at 0, with no gaps. Probability is a percentage.
type PipelineStage struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	PipelineID  string    `json:"pipeline_id"`
	Name        string    `json:"name"`
	Position    int       `json:"position"`
	Color       string    `json:"color"`
	Probability int       `json:"probability"`
	Kind        StageKind `json:"kind"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// StageInput holds the fields of a new stage. A nil Position, or one beyond the end,
// means the end.
type StageInput struct {
	Name        string
	Color       string
	Probability int
	Kind        StageKind
	Position    *int
}

// StagePatch holds the fields to change. Nil leaves a field as it is. Position
// changes only through a reorder.
type StagePatch struct {
	Name        *string
	Color       *string
	Probability *int
	Kind        *StageKind
}
