package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"crm/pkg/model"

	"github.com/jackc/pgx/v5"
)

var (
	// ErrPipelineNotFound covers a nonexistent pipeline, a deleted one, one in another
	// workspace, and a malformed ID, so callers cannot tell them apart.
	ErrPipelineNotFound = errors.New("pipeline not found")
	// ErrStageNotFound is the same for a stage, and also covers a stage of another
	// pipeline in the same workspace.
	ErrStageNotFound = errors.New("stage not found")
	// ErrLastOpenStage means the change would leave a pipeline with no open stage,
	// and so with nowhere for a new deal to start.
	ErrLastOpenStage = errors.New("a pipeline must keep at least one open stage")
	// ErrStageOrderMismatch means a reorder did not name every live stage exactly once.
	ErrStageOrderMismatch = errors.New("the order must name every stage of the pipeline exactly once")
)

const (
	pipelineColumns = `p.id::text, p.workspace_id::text, p.name, p.created_at, p.updated_at`
	stageColumns    = `st.id::text, st.workspace_id::text, st.pipeline_id::text, st.name, st.position, st.color, st.probability, st.kind, st.created_at, st.updated_at`
)

func scanPipeline(row pgx.Row) (model.Pipeline, error) {
	var p model.Pipeline
	err := row.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func scanStage(row pgx.Row) (model.PipelineStage, error) {
	var st model.PipelineStage
	err := row.Scan(&st.ID, &st.WorkspaceID, &st.PipelineID, &st.Name, &st.Position, &st.Color, &st.Probability, &st.Kind, &st.CreatedAt, &st.UpdatedAt)
	return st, err
}

// requireManager takes the workspace lock and re-reads the actor's role, and refuses
// anyone but an owner or admin. Pipelines and stages are shared structure, so unlike
// deals they are not open to every member.
func requireManager(ctx context.Context, q Querier, workspaceID, actorID string) error {
	actor, err := lockActor(ctx, q, workspaceID, actorID)
	if err != nil {
		return err
	}
	if !canManageMembers(actor) {
		return ErrWorkspaceForbidden
	}
	return nil
}

// lockPipeline confirms the pipeline is live and in this workspace, and holds its row
// lock until the transaction ends. That lock is the one thing every change to a
// pipeline's stages or to its deals' board positions has in common, so those changes
// run one at a time and each reads what the last one committed.
//
// FOR NO KEY UPDATE, because it conflicts with itself and with the soft delete, but
// not with the foreign key checks of rows inserted under this pipeline.
//
// Lock order, which keeps the writers of a workspace from deadlocking: the workspace
// row, then the rows a deal links to, then the pipeline, then the deals. See
// lockActor, holdLiveCompany, and unlinkDeals.
func lockPipeline(ctx context.Context, q Querier, workspaceID, pipelineID string) error {
	if !validUUID(pipelineID) {
		return ErrPipelineNotFound
	}
	var live int
	err := q.QueryRow(ctx, `
		SELECT 1 FROM pipelines
		 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL
		   FOR NO KEY UPDATE`, workspaceID, pipelineID).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPipelineNotFound
	}
	if err != nil {
		return fmt.Errorf("lock pipeline: %w", err)
	}
	return nil
}

func readStages(ctx context.Context, q Querier, workspaceID, pipelineID string) ([]model.PipelineStage, error) {
	rows, err := q.Query(ctx, `
		SELECT `+stageColumns+`
		  FROM pipeline_stages st
		 WHERE st.workspace_id = $1::text::uuid AND st.pipeline_id = $2::text::uuid AND st.deleted_at IS NULL
		 ORDER BY st.position
		 LIMIT $3`, workspaceID, pipelineID, maxListedRows)
	if err != nil {
		return nil, fmt.Errorf("read stages: %w", err)
	}
	stages, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.PipelineStage, error) { return scanStage(row) })
	if err != nil {
		return nil, fmt.Errorf("read stages: %w", err)
	}
	return stages, nil
}

func readPipeline(ctx context.Context, q Querier, workspaceID, id string) (model.Pipeline, error) {
	p, err := scanPipeline(q.QueryRow(ctx, `
		SELECT `+pipelineColumns+`
		  FROM pipelines p
		 WHERE p.workspace_id = $1::text::uuid AND p.id = $2::text::uuid AND p.deleted_at IS NULL`,
		workspaceID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Pipeline{}, ErrPipelineNotFound
	}
	if err != nil {
		return model.Pipeline{}, fmt.Errorf("read pipeline: %w", err)
	}
	p.Stages, err = readStages(ctx, q, workspaceID, id)
	if err != nil {
		return model.Pipeline{}, err
	}
	return p, nil
}

// ListPipelines returns one page of the workspace's live pipelines without their
// stages, oldest first. Search matches the name.
func (s *Store) ListPipelines(ctx context.Context, workspaceID string, params model.ListParams) (model.Page[model.Pipeline], error) {
	p := params.Bounded()
	var page model.Page[model.Pipeline]
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		rows, err := q.Query(ctx, `
			SELECT `+pipelineColumns+`
			  FROM pipelines p
			 WHERE p.workspace_id = $1::text::uuid AND p.deleted_at IS NULL
			   AND ($2::text IS NULL OR p.name ILIKE $2 ESCAPE '\')
			 ORDER BY p.created_at, p.id
			 LIMIT $3 OFFSET $4`, workspaceID, containsPattern(p.Search), p.Limit+1, p.Offset)
		if err != nil {
			return fmt.Errorf("list pipelines: %w", err)
		}
		page, err = collectPage(rows, p, func(row pgx.CollectableRow) (model.Pipeline, error) { return scanPipeline(row) })
		if err != nil {
			return fmt.Errorf("list pipelines: %w", err)
		}
		return nil
	})
	return page, err
}

// CreatePipeline inserts a pipeline with the default stages, so that it is usable
// right away and has an open stage from the start. Owners and admins only.
func (s *Store) CreatePipeline(ctx context.Context, workspaceID, actorID, name string) (model.Pipeline, error) {
	var p model.Pipeline
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		if err := requireManager(ctx, q, workspaceID, actorID); err != nil {
			return err
		}
		var id string
		if err := q.QueryRow(ctx, `SELECT create_pipeline_with_default_stages($1::text::uuid, $2)::text`, workspaceID, name).Scan(&id); err != nil {
			return fmt.Errorf("create pipeline: %w", err)
		}
		var err error
		p, err = readPipeline(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Pipeline{}, err
	}
	return p, nil
}

// GetPipeline returns a live pipeline with its stages in order, or ErrPipelineNotFound.
func (s *Store) GetPipeline(ctx context.Context, workspaceID, id string) (model.Pipeline, error) {
	if !validUUID(id) {
		return model.Pipeline{}, ErrPipelineNotFound
	}
	var p model.Pipeline
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		var err error
		p, err = readPipeline(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Pipeline{}, err
	}
	return p, nil
}

// RenamePipeline is open to owners and admins.
func (s *Store) RenamePipeline(ctx context.Context, workspaceID, actorID, id, name string) (model.Pipeline, error) {
	var p model.Pipeline
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		if err := requireManager(ctx, q, workspaceID, actorID); err != nil {
			return err
		}
		if err := lockPipeline(ctx, q, workspaceID, id); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `
			UPDATE pipelines SET name = $3, updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid`, workspaceID, id, name); err != nil {
			return fmt.Errorf("rename pipeline: %w", err)
		}
		var err error
		p, err = readPipeline(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Pipeline{}, err
	}
	return p, nil
}

// DeletePipeline soft-deletes a pipeline. Owners and admins only.
func (s *Store) DeletePipeline(ctx context.Context, workspaceID, actorID, id string) error {
	return s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		if err := requireManager(ctx, q, workspaceID, actorID); err != nil {
			return err
		}
		if err := lockPipeline(ctx, q, workspaceID, id); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `
			UPDATE pipelines SET deleted_at = now(), updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid`, workspaceID, id); err != nil {
			return fmt.Errorf("delete pipeline: %w", err)
		}
		return nil
	})
}

func countLiveStages(ctx context.Context, q Querier, workspaceID, pipelineID string) (int, error) {
	var count int
	err := q.QueryRow(ctx, `
		SELECT count(*) FROM pipeline_stages
		 WHERE workspace_id = $1::text::uuid AND pipeline_id = $2::text::uuid AND deleted_at IS NULL`,
		workspaceID, pipelineID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count stages: %w", err)
	}
	return count, nil
}

// insertPosition turns a requested position into a valid one: nil or anything beyond
// the end means the end. A client working from a board that has since shrunk still
// lands where it meant to.
func insertPosition(requested *int, count int) int {
	if requested == nil {
		return count
	}
	return min(max(*requested, 0), count)
}

// CreateStage inserts a stage at a position, moving later stages along. Owners and
// admins only.
func (s *Store) CreateStage(ctx context.Context, workspaceID, actorID, pipelineID string, in model.StageInput) (model.PipelineStage, error) {
	var st model.PipelineStage
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		if err := requireManager(ctx, q, workspaceID, actorID); err != nil {
			return err
		}
		if err := lockPipeline(ctx, q, workspaceID, pipelineID); err != nil {
			return err
		}
		count, err := countLiveStages(ctx, q, workspaceID, pipelineID)
		if err != nil {
			return err
		}
		position := insertPosition(in.Position, count)
		if _, err := q.Exec(ctx, `
			UPDATE pipeline_stages SET position = position + 1, updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND pipeline_id = $2::text::uuid AND position >= $3`,
			workspaceID, pipelineID, position); err != nil {
			return fmt.Errorf("make room for stage: %w", err)
		}
		st, err = scanStage(q.QueryRow(ctx, `
			INSERT INTO pipeline_stages AS st (workspace_id, pipeline_id, name, position, color, probability, kind)
			VALUES ($1::text::uuid, $2::text::uuid, $3, $4, $5, $6, $7)
			RETURNING `+stageColumns,
			workspaceID, pipelineID, in.Name, position, in.Color, in.Probability, in.Kind))
		if err != nil {
			return fmt.Errorf("insert stage: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.PipelineStage{}, err
	}
	return st, nil
}

// readStageKind reads a live stage's kind, under the pipeline lock. It returns
// ErrStageNotFound for a stage out of reach, including one of another pipeline.
func readStageKind(ctx context.Context, q Querier, workspaceID, pipelineID, stageID string) (model.StageKind, error) {
	if !validUUID(stageID) {
		return "", ErrStageNotFound
	}
	var kind model.StageKind
	err := q.QueryRow(ctx, `
		SELECT kind FROM pipeline_stages
		 WHERE workspace_id = $1::text::uuid AND pipeline_id = $2::text::uuid AND id = $3::text::uuid AND deleted_at IS NULL`,
		workspaceID, pipelineID, stageID).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrStageNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read stage: %w", err)
	}
	return kind, nil
}

// protectLastOpenStage refuses a change that takes the given open stage away, if no
// other open stage remains. The caller must hold the pipeline lock.
func protectLastOpenStage(ctx context.Context, q Querier, workspaceID, pipelineID, stageID string) error {
	var others int
	err := q.QueryRow(ctx, `
		SELECT count(*) FROM pipeline_stages
		 WHERE workspace_id = $1::text::uuid AND pipeline_id = $2::text::uuid AND deleted_at IS NULL
		   AND kind = $3 AND id <> $4::text::uuid`,
		workspaceID, pipelineID, model.StageOpen, stageID).Scan(&others)
	if err != nil {
		return fmt.Errorf("count open stages: %w", err)
	}
	if others == 0 {
		return ErrLastOpenStage
	}
	return nil
}

// UpdateStage changes the fields the patch names. Owners and admins only.
func (s *Store) UpdateStage(ctx context.Context, workspaceID, actorID, pipelineID, stageID string, patch model.StagePatch) (model.PipelineStage, error) {
	var st model.PipelineStage
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		if err := requireManager(ctx, q, workspaceID, actorID); err != nil {
			return err
		}
		if err := lockPipeline(ctx, q, workspaceID, pipelineID); err != nil {
			return err
		}
		kind, err := readStageKind(ctx, q, workspaceID, pipelineID, stageID)
		if err != nil {
			return err
		}
		if patch.Kind != nil && *patch.Kind != kind && kind == model.StageOpen {
			if err := protectLastOpenStage(ctx, q, workspaceID, pipelineID, stageID); err != nil {
				return err
			}
		}
		st, err = scanStage(q.QueryRow(ctx, `
			UPDATE pipeline_stages AS st
			   SET name = COALESCE($3, st.name),
			       color = COALESCE($4, st.color),
			       probability = COALESCE($5, st.probability),
			       kind = COALESCE($6, st.kind),
			       updated_at = now()
			 WHERE st.workspace_id = $1::text::uuid AND st.id = $2::text::uuid
			 RETURNING `+stageColumns,
			workspaceID, stageID, patch.Name, patch.Color, patch.Probability, patch.Kind))
		if err != nil {
			return fmt.Errorf("update stage: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.PipelineStage{}, err
	}
	return st, nil
}

// ReorderStages gives a pipeline's stages the order listed, which must name every live
// stage exactly once. A partial or stale list is refused rather than merged, so nobody
// scrambles an order they could not see. Owners and admins only.
func (s *Store) ReorderStages(ctx context.Context, workspaceID, actorID, pipelineID string, ids []string) ([]model.PipelineStage, error) {
	var stages []model.PipelineStage
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		if err := requireManager(ctx, q, workspaceID, actorID); err != nil {
			return err
		}
		if err := lockPipeline(ctx, q, workspaceID, pipelineID); err != nil {
			return err
		}
		current, err := readStages(ctx, q, workspaceID, pipelineID)
		if err != nil {
			return err
		}
		wanted := make([]string, len(ids))
		for i, id := range ids {
			wanted[i] = strings.ToLower(id)
		}
		live := make([]string, len(current))
		for i, stage := range current {
			live[i] = stage.ID
		}
		sorted := slices.Sorted(slices.Values(wanted))
		slices.Sort(live)
		if !slices.Equal(sorted, live) {
			return ErrStageOrderMismatch
		}
		// Every ID now comes from the database, so the cast cannot fail.
		if _, err := q.Exec(ctx, `
			UPDATE pipeline_stages AS st SET position = o.ordinality - 1, updated_at = now()
			  FROM unnest($3::text[]) WITH ORDINALITY AS o (id, ordinality)
			 WHERE st.workspace_id = $1::text::uuid AND st.pipeline_id = $2::text::uuid AND st.id = o.id::uuid`,
			workspaceID, pipelineID, wanted); err != nil {
			return fmt.Errorf("reorder stages: %w", err)
		}
		stages, err = readStages(ctx, q, workspaceID, pipelineID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return stages, nil
}

// DeleteStage soft-deletes a stage and closes the gap it leaves. It refuses to remove
// the last open stage. Owners and admins only.
func (s *Store) DeleteStage(ctx context.Context, workspaceID, actorID, pipelineID, stageID string) error {
	return s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		if err := requireManager(ctx, q, workspaceID, actorID); err != nil {
			return err
		}
		if err := lockPipeline(ctx, q, workspaceID, pipelineID); err != nil {
			return err
		}
		kind, err := readStageKind(ctx, q, workspaceID, pipelineID, stageID)
		if err != nil {
			return err
		}
		if kind == model.StageOpen {
			if err := protectLastOpenStage(ctx, q, workspaceID, pipelineID, stageID); err != nil {
				return err
			}
		}
		var position int
		if err := q.QueryRow(ctx, `
			WITH old AS (
				SELECT id, position FROM pipeline_stages
				 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid
			), gone AS (
				UPDATE pipeline_stages AS st SET position = NULL, deleted_at = now(), updated_at = now()
				  FROM old WHERE st.id = old.id
			)
			SELECT position FROM old`, workspaceID, stageID).Scan(&position); err != nil {
			return fmt.Errorf("delete stage: %w", err)
		}
		if _, err := q.Exec(ctx, `
			UPDATE pipeline_stages SET position = position - 1, updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND pipeline_id = $2::text::uuid AND position > $3`,
			workspaceID, pipelineID, position); err != nil {
			return fmt.Errorf("close the gap: %w", err)
		}
		return nil
	})
}
