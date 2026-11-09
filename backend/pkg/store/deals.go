package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"crm/pkg/model"

	"github.com/jackc/pgx/v5"
)

var (
	// ErrDealNotFound covers a nonexistent deal, a deleted one, one in another
	// workspace, and a malformed ID, so callers cannot tell them apart.
	ErrDealNotFound = errors.New("deal not found")
	// ErrStageNotOpen means a new deal was aimed at a won or lost stage. A deal starts
	// open, and closes through a move, which records when and why.
	ErrStageNotOpen = errors.New("a deal starts in an open stage")
	// ErrLostReasonRequired means a move into a lost stage came without a reason.
	ErrLostReasonRequired = errors.New("a lost reason is required")
	// ErrLostReasonNotAllowed means a move into an open or won stage came with a reason.
	ErrLostReasonNotAllowed = errors.New("only a lost stage takes a lost reason")
	// ErrPipelineHasDeals means a pipeline to delete still holds live deals.
	ErrPipelineHasDeals = errors.New("the pipeline still holds deals")
	// ErrStageHasDeals means a stage to delete or redefine still holds live deals.
	ErrStageHasDeals = errors.New("the stage still holds deals")
)

// dealSelect reads deals with their status and linked names. It filters out a deleted
// company or contact as well, although deleting one also clears its links. The ordering
// column of a soft-deleted deal is NULL, so this is for live deals only.
const dealSelect = `
	SELECT d.id::text, d.workspace_id::text, d.pipeline_id::text, d.stage_id::text, st.kind, d.title, d.value_minor,
	       d.currency, to_char(d.expected_close_date, 'YYYY-MM-DD'), d.closed_at, d.lost_reason,
	       d.company_id::text, co.name, d.contact_id::text, btrim(ct.first_name || ' ' || ct.last_name),
	       d.owner_id::text, d.position, d.created_at, d.updated_at
	  FROM deals d
	  JOIN pipeline_stages st
	    ON st.workspace_id = d.workspace_id AND st.pipeline_id = d.pipeline_id AND st.id = d.stage_id
	  LEFT JOIN companies co
	    ON co.workspace_id = d.workspace_id AND co.id = d.company_id AND co.deleted_at IS NULL
	  LEFT JOIN contacts ct
	    ON ct.workspace_id = d.workspace_id AND ct.id = d.contact_id AND ct.deleted_at IS NULL`

func scanDeal(row pgx.Row) (model.Deal, error) {
	var d model.Deal
	err := row.Scan(&d.ID, &d.WorkspaceID, &d.PipelineID, &d.StageID, &d.Status, &d.Title, &d.ValueMinor,
		&d.Currency, &d.ExpectedCloseDate, &d.ClosedAt, &d.LostReason,
		&d.CompanyID, &d.CompanyName, &d.ContactID, &d.ContactName,
		&d.OwnerID, &d.Position, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func readDeal(ctx context.Context, q Querier, workspaceID, id string) (model.Deal, error) {
	d, err := scanDeal(q.QueryRow(ctx, dealSelect+`
		 WHERE d.workspace_id = $1::text::uuid AND d.id = $2::text::uuid AND d.deleted_at IS NULL`,
		workspaceID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Deal{}, ErrDealNotFound
	}
	if err != nil {
		return model.Deal{}, fmt.Errorf("read deal: %w", err)
	}
	return d, nil
}

// holdMember confirms the user is a current member, and keeps them one until the
// transaction ends. Removing a member locks their row first and then clears their
// deals, so a removal either waits for this link to commit and clears it too, or
// commits first and this finds no row. See holdLiveCompany for why FOR SHARE.
func holdMember(ctx context.Context, q Querier, workspaceID, userID string) error {
	if !validUUID(userID) {
		return ErrMemberNotFound
	}
	var member int
	err := q.QueryRow(ctx, `
		SELECT 1 FROM workspace_members
		 WHERE workspace_id = $1::text::uuid AND user_id = $2::text::uuid
		   FOR SHARE`, workspaceID, userID).Scan(&member)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMemberNotFound
	}
	if err != nil {
		return fmt.Errorf("lock member: %w", err)
	}
	return nil
}

// holdDealLinks locks whichever of a deal's link targets are set, always in this order,
// and before the pipeline or the deal. A nil target is left alone.
func holdDealLinks(ctx context.Context, q Querier, workspaceID string, companyID, contactID, ownerID *string) error {
	if companyID != nil {
		if err := holdLiveCompany(ctx, q, workspaceID, *companyID); err != nil {
			return err
		}
	}
	if contactID != nil {
		if err := holdLiveContact(ctx, q, workspaceID, *contactID); err != nil {
			return err
		}
	}
	if ownerID != nil {
		return holdMember(ctx, q, workspaceID, *ownerID)
	}
	return nil
}

// optionalID makes an empty filter value NULL, which the list query reads as no filter.
func optionalID(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

// ListDeals returns one page of the workspace's live deals in board order: by pipeline,
// then stage position, then position in the stage. Search matches the title and the
// linked company's and contact's names.
func (s *Store) ListDeals(ctx context.Context, workspaceID string, filter model.DealFilter, params model.ListParams) (model.Page[model.Deal], error) {
	p := params.Bounded()
	// A malformed filter would fail as an error in SQL. It matches nothing instead.
	for _, id := range []string{filter.PipelineID, filter.StageID, filter.OwnerID} {
		if id != "" && !validUUID(id) {
			return model.Page[model.Deal]{Items: []model.Deal{}, Limit: p.Limit, Offset: p.Offset}, nil
		}
	}
	var page model.Page[model.Deal]
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		rows, err := q.Query(ctx, dealSelect+`
			 WHERE d.workspace_id = $1::text::uuid AND d.deleted_at IS NULL
			   AND ($2::text IS NULL OR d.pipeline_id = $2::text::uuid)
			   AND ($3::text IS NULL OR d.stage_id = $3::text::uuid)
			   AND ($4::text IS NULL OR d.owner_id = $4::text::uuid)
			   AND ($5::text IS NULL OR st.kind = $5::text)
			   AND ($6::text IS NULL
			        OR d.title ILIKE $6 ESCAPE '\'
			        OR co.name ILIKE $6 ESCAPE '\'
			        OR (ct.first_name || ' ' || ct.last_name) ILIKE $6 ESCAPE '\')
			 ORDER BY d.pipeline_id, st.position, d.position, d.id
			 LIMIT $7 OFFSET $8`,
			workspaceID, optionalID(filter.PipelineID), optionalID(filter.StageID), optionalID(filter.OwnerID),
			optionalID(string(filter.Status)), containsPattern(p.Search), p.Limit+1, p.Offset)
		if err != nil {
			return fmt.Errorf("list deals: %w", err)
		}
		page, err = collectPage(rows, p, func(row pgx.CollectableRow) (model.Deal, error) { return scanDeal(row) })
		if err != nil {
			return fmt.Errorf("list deals: %w", err)
		}
		return nil
	})
	return page, err
}

func countStageDeals(ctx context.Context, q Querier, workspaceID, stageID string) (int, error) {
	var count int
	err := q.QueryRow(ctx, `
		SELECT count(*) FROM deals
		 WHERE workspace_id = $1::text::uuid AND stage_id = $2::text::uuid AND deleted_at IS NULL`,
		workspaceID, stageID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count deals: %w", err)
	}
	return count, nil
}

// startingStage resolves where a new deal goes, under the pipeline lock.
func startingStage(ctx context.Context, q Querier, workspaceID, pipelineID string, stageID *string) (string, error) {
	if stageID != nil {
		kind, err := readStageKind(ctx, q, workspaceID, pipelineID, *stageID)
		if err != nil {
			return "", err
		}
		if kind != model.StageOpen {
			return "", ErrStageNotOpen
		}
		return *stageID, nil
	}
	var first string
	err := q.QueryRow(ctx, `
		SELECT id::text FROM pipeline_stages
		 WHERE workspace_id = $1::text::uuid AND pipeline_id = $2::text::uuid AND deleted_at IS NULL AND kind = $3
		 ORDER BY position LIMIT 1`, workspaceID, pipelineID, model.StageOpen).Scan(&first)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrStageNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read first open stage: %w", err)
	}
	return first, nil
}

// CreateDeal inserts a deal at the end of an open stage. It returns the not-found error
// of whichever pipeline, stage, company, contact, or owner it cannot use.
func (s *Store) CreateDeal(ctx context.Context, workspaceID string, in model.DealInput) (model.Deal, error) {
	var d model.Deal
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		if err := holdDealLinks(ctx, q, workspaceID, in.CompanyID, in.ContactID, in.OwnerID); err != nil {
			return err
		}
		if err := lockPipeline(ctx, q, workspaceID, in.PipelineID); err != nil {
			return err
		}
		stageID, err := startingStage(ctx, q, workspaceID, in.PipelineID, in.StageID)
		if err != nil {
			return err
		}
		position, err := countStageDeals(ctx, q, workspaceID, stageID)
		if err != nil {
			return err
		}
		var id string
		err = q.QueryRow(ctx, `
			INSERT INTO deals (workspace_id, pipeline_id, stage_id, title, value_minor, currency, expected_close_date,
			                   company_id, contact_id, owner_id, position)
			VALUES ($1::text::uuid, $2::text::uuid, $3::text::uuid, $4, $5, $6, $7::text::date,
			        $8::text::uuid, $9::text::uuid, $10::text::uuid, $11)
			RETURNING id::text`,
			workspaceID, in.PipelineID, stageID, in.Title, in.ValueMinor, in.Currency, in.ExpectedCloseDate,
			in.CompanyID, in.ContactID, in.OwnerID, position).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert deal: %w", err)
		}
		d, err = readDeal(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Deal{}, err
	}
	return d, nil
}

// GetDeal returns a live deal of the workspace, or ErrDealNotFound.
func (s *Store) GetDeal(ctx context.Context, workspaceID, id string) (model.Deal, error) {
	if !validUUID(id) {
		return model.Deal{}, ErrDealNotFound
	}
	var d model.Deal
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		var err error
		d, err = readDeal(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Deal{}, err
	}
	return d, nil
}

// UpdateDeal changes the fields the patch names. It needs no pipeline lock, because it
// touches neither the stage nor the position, and it writes only what the patch names,
// so it cannot undo a concurrent move.
func (s *Store) UpdateDeal(ctx context.Context, workspaceID, id string, patch model.DealPatch) (model.Deal, error) {
	if !validUUID(id) {
		return model.Deal{}, ErrDealNotFound
	}
	var d model.Deal
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		var company, contact, owner *string
		if patch.SetCompany {
			company = patch.CompanyID
		}
		if patch.SetContact {
			contact = patch.ContactID
		}
		if patch.SetOwner {
			owner = patch.OwnerID
		}
		if err := holdDealLinks(ctx, q, workspaceID, company, contact, owner); err != nil {
			return err
		}
		tag, err := q.Exec(ctx, `
			UPDATE deals
			   SET title = COALESCE($3, title),
			       value_minor = COALESCE($4, value_minor),
			       currency = COALESCE($5, currency),
			       expected_close_date = CASE WHEN $6::boolean THEN $7::text::date ELSE expected_close_date END,
			       company_id = CASE WHEN $8::boolean THEN $9::text::uuid ELSE company_id END,
			       contact_id = CASE WHEN $10::boolean THEN $11::text::uuid ELSE contact_id END,
			       owner_id = CASE WHEN $12::boolean THEN $13::text::uuid ELSE owner_id END,
			       updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL`,
			workspaceID, id, patch.Title, patch.ValueMinor, patch.Currency,
			patch.SetExpectedClose, patch.ExpectedCloseDate,
			patch.SetCompany, patch.CompanyID, patch.SetContact, patch.ContactID, patch.SetOwner, patch.OwnerID)
		if err != nil {
			return fmt.Errorf("update deal: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrDealNotFound
		}
		d, err = readDeal(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Deal{}, err
	}
	return d, nil
}

// lockDealPipeline finds a live deal's pipeline and takes its lock. A deal's pipeline
// never changes, so reading it before the lock is safe. Everything else about the deal
// must be read after this returns.
func lockDealPipeline(ctx context.Context, q Querier, workspaceID, id string) (string, error) {
	var pipelineID string
	err := q.QueryRow(ctx, `
		SELECT pipeline_id::text FROM deals
		 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL`,
		workspaceID, id).Scan(&pipelineID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrDealNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read deal pipeline: %w", err)
	}
	if err := lockPipeline(ctx, q, workspaceID, pipelineID); err != nil {
		return "", err
	}
	return pipelineID, nil
}

// closingOf works out what a move means for the deal's closed state, or refuses it.
// A move within the stage is not a change of state, so it keeps the closing time, and
// the lost reason unless a new one is given.
func closingOf(target model.StageKind, sameStage bool, currentReason, givenReason string) (stamp bool, reason string, err error) {
	switch target {
	case model.StageLost:
		switch {
		case givenReason != "":
			return !sameStage, givenReason, nil
		case sameStage:
			return false, currentReason, nil
		default:
			return false, "", ErrLostReasonRequired
		}
	case model.StageWon, model.StageOpen:
		if givenReason != "" {
			return false, "", ErrLostReasonNotAllowed
		}
		return target == model.StageWon && !sameStage, "", nil
	default:
		return false, "", fmt.Errorf("unknown stage kind %q", target)
	}
}

// MoveDeal puts a deal at a position in a stage of its own pipeline, and applies what
// that stage's kind means: won or lost sets closed_at, lost requires a reason, and open
// clears both. It runs under the pipeline lock, so concurrent moves on one board happen
// one at a time, each on what the previous one left. Positions stay dense: neighbours
// shift by one, and the constraint on them would refuse anything else.
//
// A stage of another pipeline is ErrStageNotFound.
func (s *Store) MoveDeal(ctx context.Context, workspaceID, id string, move model.DealMove) (model.Deal, error) {
	if !validUUID(id) {
		return model.Deal{}, ErrDealNotFound
	}
	var d model.Deal
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		pipelineID, err := lockDealPipeline(ctx, q, workspaceID, id)
		if err != nil {
			return err
		}
		var currentStage, currentReason string
		var currentPosition int
		err = q.QueryRow(ctx, `
			SELECT stage_id::text, position, lost_reason FROM deals
			 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL`,
			workspaceID, id).Scan(&currentStage, &currentPosition, &currentReason)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDealNotFound
		}
		if err != nil {
			return fmt.Errorf("read deal: %w", err)
		}
		target, err := readStageKind(ctx, q, workspaceID, pipelineID, move.StageID)
		if err != nil {
			return err
		}
		sameStage := strings.EqualFold(move.StageID, currentStage)
		stamp, reason, err := closingOf(target, sameStage, currentReason, move.LostReason)
		if err != nil {
			return err
		}
		count, err := countStageDeals(ctx, q, workspaceID, move.StageID)
		if err != nil {
			return err
		}

		position := insertPosition(move.Position, count)
		if sameStage {
			// Not counting itself, the end of its own stage is one earlier.
			position = min(position, count-1)
			if err := shiftWithinStage(ctx, q, workspaceID, currentStage, id, currentPosition, position); err != nil {
				return err
			}
		} else if err := shiftForArrival(ctx, q, workspaceID, move.StageID, position); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `
			UPDATE deals
			   SET stage_id = $3::text::uuid,
			       position = $4,
			       closed_at = CASE WHEN $5::text = 'open' THEN NULL WHEN $6::boolean THEN now() ELSE closed_at END,
			       lost_reason = $7,
			       updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid`,
			workspaceID, id, move.StageID, position, target, stamp, reason); err != nil {
			return fmt.Errorf("move deal: %w", err)
		}
		if !sameStage {
			if err := closeDealGap(ctx, q, workspaceID, currentStage, currentPosition); err != nil {
				return err
			}
		}
		d, err = readDeal(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Deal{}, err
	}
	return d, nil
}

// shiftWithinStage moves a deal and the neighbours between its old and new place in one
// statement, because the uniqueness of positions is checked when a statement ends.
func shiftWithinStage(ctx context.Context, q Querier, workspaceID, stageID, id string, from, to int) error {
	if from == to {
		return nil
	}
	if _, err := q.Exec(ctx, `
		UPDATE deals
		   SET position = CASE WHEN id = $3::text::uuid THEN $5::int
		                       WHEN $5::int < $4::int THEN position + 1
		                       ELSE position - 1 END,
		       updated_at = now()
		 WHERE workspace_id = $1::text::uuid AND stage_id = $2::text::uuid
		   AND position BETWEEN LEAST($4::int, $5::int) AND GREATEST($4::int, $5::int)`,
		workspaceID, stageID, id, from, to); err != nil {
		return fmt.Errorf("shift deals within stage: %w", err)
	}
	return nil
}

func shiftForArrival(ctx context.Context, q Querier, workspaceID, stageID string, position int) error {
	if _, err := q.Exec(ctx, `
		UPDATE deals SET position = position + 1, updated_at = now()
		 WHERE workspace_id = $1::text::uuid AND stage_id = $2::text::uuid AND position >= $3`,
		workspaceID, stageID, position); err != nil {
		return fmt.Errorf("make room for deal: %w", err)
	}
	return nil
}

func closeDealGap(ctx context.Context, q Querier, workspaceID, stageID string, position int) error {
	if _, err := q.Exec(ctx, `
		UPDATE deals SET position = position - 1, updated_at = now()
		 WHERE workspace_id = $1::text::uuid AND stage_id = $2::text::uuid AND position > $3`,
		workspaceID, stageID, position); err != nil {
		return fmt.Errorf("close the gap: %w", err)
	}
	return nil
}

// DeleteDeal soft-deletes a live deal and closes the gap it leaves on the board. The
// deal keeps pointing at the stage it was in.
func (s *Store) DeleteDeal(ctx context.Context, workspaceID, id string) error {
	if !validUUID(id) {
		return ErrDealNotFound
	}
	return s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		if _, err := lockDealPipeline(ctx, q, workspaceID, id); err != nil {
			return err
		}
		var stageID string
		var position int
		err := q.QueryRow(ctx, `
			SELECT stage_id::text, position FROM deals
			 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL`,
			workspaceID, id).Scan(&stageID, &position)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDealNotFound
		}
		if err != nil {
			return fmt.Errorf("read deal: %w", err)
		}
		if _, err := q.Exec(ctx, `
			UPDATE deals SET position = NULL, deleted_at = now(), updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid`, workspaceID, id); err != nil {
			return fmt.Errorf("delete deal: %w", err)
		}
		return closeDealGap(ctx, q, workspaceID, stageID, position)
	})
}

// stageHoldsDeals reports whether a stage has live deals. The caller must hold the
// pipeline lock, which keeps a deal from arriving before it acts on the answer.
func stageHoldsDeals(ctx context.Context, q Querier, workspaceID, stageID string) (bool, error) {
	count, err := countStageDeals(ctx, q, workspaceID, stageID)
	return count > 0, err
}

// pipelineHoldsDeals is the same for a pipeline.
func pipelineHoldsDeals(ctx context.Context, q Querier, workspaceID, pipelineID string) (bool, error) {
	var holds bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM deals
		                WHERE workspace_id = $1::text::uuid AND pipeline_id = $2::text::uuid AND deleted_at IS NULL)`,
		workspaceID, pipelineID).Scan(&holds)
	if err != nil {
		return false, fmt.Errorf("check pipeline deals: %w", err)
	}
	return holds, nil
}
