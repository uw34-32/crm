package store

import (
	"context"
	"errors"
	"fmt"

	"crm/pkg/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrActivityNotFound covers a nonexistent activity, a deleted one, one in another
	// workspace, and a malformed ID, so callers cannot tell them apart.
	ErrActivityNotFound = errors.New("activity not found")
	// ErrActivityNeedsLink means the change would leave an activity linked to no
	// contact, company, or deal.
	ErrActivityNeedsLink = errors.New("an activity links to at least one contact, company, or deal")
	// ErrNotATask means a due time, an assignee, or a completed state was aimed at a
	// note, call, or meeting.
	ErrNotATask = errors.New("only a task has a due time, an assignee, or a completed state")
	// ErrDueOutOfRange means a due time outside the years the schema accepts, 1900 to
	// 2200 in UTC. The API refuses one before it gets here.
	ErrDueOutOfRange = errors.New("the due time is out of range")
	// ErrActivityForbidden means the actor may read the activity but not change it:
	// only its author or an owner or admin may.
	ErrActivityForbidden = errors.New("only the author or an owner or admin may change this activity")
)

// The names of the constraints in 011_activities.sql that a write can break.
const (
	activityNeedsLinkCheck  = "activities_needs_a_link"
	activityTaskFieldsCheck = "activities_task_fields"
	activityDueRangeCheck   = "activities_due_at_check"
	pgCheckViolation        = "23514"
)

// activityWriteError turns a broken activity rule into its error. The schema is where
// the rules live, because only there do they hold for the row as it ends up, whatever
// the patch left out.
func activityWriteError(op string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgCheckViolation {
		switch pgErr.ConstraintName {
		case activityNeedsLinkCheck:
			return ErrActivityNeedsLink
		case activityTaskFieldsCheck:
			return ErrNotATask
		case activityDueRangeCheck:
			return ErrDueOutOfRange
		}
	}
	return fmt.Errorf("%s: %w", op, err)
}

// activitySelect reads activities with the current names of what they link to. It
// filters out deleted rows as well, although deleting one also clears the links.
const activitySelect = `
	SELECT a.id::text, a.workspace_id::text, a.kind, a.subject, a.body, a.due_at, a.completed_at,
	       a.author_id::text, a.assignee_id::text,
	       a.contact_id::text, btrim(ct.first_name || ' ' || ct.last_name), a.company_id::text, co.name,
	       a.deal_id::text, d.title, a.created_at, a.updated_at
	  FROM activities a
	  LEFT JOIN contacts ct
	    ON ct.workspace_id = a.workspace_id AND ct.id = a.contact_id AND ct.deleted_at IS NULL
	  LEFT JOIN companies co
	    ON co.workspace_id = a.workspace_id AND co.id = a.company_id AND co.deleted_at IS NULL
	  LEFT JOIN deals d
	    ON d.workspace_id = a.workspace_id AND d.id = a.deal_id AND d.deleted_at IS NULL`

func scanActivity(row pgx.Row) (model.Activity, error) {
	var a model.Activity
	err := row.Scan(&a.ID, &a.WorkspaceID, &a.Kind, &a.Subject, &a.Body, &a.DueAt, &a.CompletedAt,
		&a.AuthorID, &a.AssigneeID, &a.ContactID, &a.ContactName, &a.CompanyID, &a.CompanyName,
		&a.DealID, &a.DealTitle, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

func readActivity(ctx context.Context, q Querier, workspaceID, id string) (model.Activity, error) {
	a, err := scanActivity(q.QueryRow(ctx, activitySelect+`
		 WHERE a.workspace_id = $1::text::uuid AND a.id = $2::text::uuid AND a.deleted_at IS NULL`,
		workspaceID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Activity{}, ErrActivityNotFound
	}
	if err != nil {
		return model.Activity{}, fmt.Errorf("read activity: %w", err)
	}
	return a, nil
}

// holdLiveDeal is holdLiveCompany for a deal, for the writes that link an activity.
func holdLiveDeal(ctx context.Context, q Querier, workspaceID, dealID string) error {
	if !validUUID(dealID) {
		return ErrDealNotFound
	}
	var live int
	err := q.QueryRow(ctx, `
		SELECT 1 FROM deals
		 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL
		   FOR SHARE`, workspaceID, dealID).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDealNotFound
	}
	if err != nil {
		return fmt.Errorf("lock deal: %w", err)
	}
	return nil
}

// holdActor confirms the actor is still a member, keeps them one, and returns their
// role as it stands now. Like holdMember it takes a share lock, which a role change or
// a removal has to wait for. A former member is answered like a non-member.
func holdActor(ctx context.Context, q Querier, workspaceID, actorID string) (model.WorkspaceRole, error) {
	if !validUUID(actorID) {
		return "", ErrWorkspaceNotFound
	}
	var role model.WorkspaceRole
	err := q.QueryRow(ctx, `
		SELECT role FROM workspace_members
		 WHERE workspace_id = $1::text::uuid AND user_id = $2::text::uuid
		   FOR SHARE`, workspaceID, actorID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrWorkspaceNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lock actor: %w", err)
	}
	return role, nil
}

// activityLinks names what one write links an activity to. A nil target is left alone.
type activityLinks struct {
	company, contact, assignee, deal *string
}

// hold locks the actor and every target named, always in this order: company, contact,
// members, deal. That is the documented lock order, with a deal last because deal rows
// come after the pipeline. Deleting a company or a member updates deals while it holds
// the company or member row, so an activity write holding a deal while it waited for
// one of those would deadlock with it.
func (l activityLinks) hold(ctx context.Context, q Querier, workspaceID, actorID string) (model.WorkspaceRole, error) {
	if l.company != nil {
		if err := holdLiveCompany(ctx, q, workspaceID, *l.company); err != nil {
			return "", err
		}
	}
	if l.contact != nil {
		if err := holdLiveContact(ctx, q, workspaceID, *l.contact); err != nil {
			return "", err
		}
	}
	role, err := holdActor(ctx, q, workspaceID, actorID)
	if err != nil {
		return "", err
	}
	if l.assignee != nil {
		if err := holdMember(ctx, q, workspaceID, *l.assignee); err != nil {
			return "", err
		}
	}
	if l.deal != nil {
		if err := holdLiveDeal(ctx, q, workspaceID, *l.deal); err != nil {
			return "", err
		}
	}
	return role, nil
}

// ListActivities returns one page of the workspace's live activities as a timeline:
// newest first, with the ID as a tiebreak. Search matches the subject and the body.
func (s *Store) ListActivities(ctx context.Context, workspaceID string, filter model.ActivityFilter, params model.ListParams) (model.Page[model.Activity], error) {
	p := params.Bounded()
	// A malformed filter would fail as an error in SQL. It matches nothing instead.
	for _, id := range []string{filter.ContactID, filter.CompanyID, filter.DealID, filter.AssigneeID} {
		if id != "" && !validUUID(id) {
			return model.Page[model.Activity]{Items: []model.Activity{}, Limit: p.Limit, Offset: p.Offset}, nil
		}
	}
	var page model.Page[model.Activity]
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		rows, err := q.Query(ctx, activitySelect+`
			 WHERE a.workspace_id = $1::text::uuid AND a.deleted_at IS NULL
			   AND ($2::text IS NULL OR a.contact_id = $2::text::uuid)
			   AND ($3::text IS NULL OR a.company_id = $3::text::uuid)
			   AND ($4::text IS NULL OR a.deal_id = $4::text::uuid)
			   AND ($5::text IS NULL OR a.assignee_id = $5::text::uuid)
			   AND ($6::text IS NULL OR a.kind = $6::text)
			   AND ($7::text IS NULL
			        OR ($7::text = 'open' AND a.kind = 'task' AND a.completed_at IS NULL)
			        OR ($7::text = 'completed' AND a.completed_at IS NOT NULL))
			   AND ($8::timestamptz IS NULL OR a.due_at < $8::timestamptz)
			   AND ($9::timestamptz IS NULL OR a.due_at >= $9::timestamptz)
			   AND ($10::text IS NULL OR a.subject ILIKE $10 ESCAPE '\' OR a.body ILIKE $10 ESCAPE '\')
			 ORDER BY a.created_at DESC, a.id DESC
			 LIMIT $11 OFFSET $12`,
			workspaceID, optionalID(filter.ContactID), optionalID(filter.CompanyID), optionalID(filter.DealID),
			optionalID(filter.AssigneeID), optionalID(string(filter.Kind)), optionalID(string(filter.State)),
			filter.DueBefore, filter.DueAfter, containsPattern(p.Search), p.Limit+1, p.Offset)
		if err != nil {
			return fmt.Errorf("list activities: %w", err)
		}
		page, err = collectPage(rows, p, func(row pgx.CollectableRow) (model.Activity, error) { return scanActivity(row) })
		if err != nil {
			return fmt.Errorf("list activities: %w", err)
		}
		return nil
	})
	return page, err
}

// CreateActivity inserts an activity written by in.AuthorID. It returns the not-found
// error of whichever contact, company, deal, or assignee it cannot link,
// ErrActivityNeedsLink without a link, and ErrNotATask for a task's fields on anything
// else.
func (s *Store) CreateActivity(ctx context.Context, workspaceID string, in model.ActivityInput) (model.Activity, error) {
	var a model.Activity
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		links := activityLinks{company: in.CompanyID, contact: in.ContactID, assignee: in.AssigneeID, deal: in.DealID}
		if _, err := links.hold(ctx, q, workspaceID, in.AuthorID); err != nil {
			return err
		}
		var id string
		err := q.QueryRow(ctx, `
			INSERT INTO activities (workspace_id, kind, subject, body, due_at, author_id, assignee_id, contact_id, company_id, deal_id)
			VALUES ($1::text::uuid, $2, $3, $4, $5, $6::text::uuid, $7::text::uuid, $8::text::uuid, $9::text::uuid, $10::text::uuid)
			RETURNING id::text`,
			workspaceID, in.Kind, in.Subject, in.Body, in.DueAt, in.AuthorID, in.AssigneeID, in.ContactID, in.CompanyID, in.DealID).Scan(&id)
		if err != nil {
			return activityWriteError("insert activity", err)
		}
		a, err = readActivity(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Activity{}, err
	}
	return a, nil
}

// GetActivity returns a live activity of the workspace, or ErrActivityNotFound.
func (s *Store) GetActivity(ctx context.Context, workspaceID, id string) (model.Activity, error) {
	if !validUUID(id) {
		return model.Activity{}, ErrActivityNotFound
	}
	var a model.Activity
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		var err error
		a, err = readActivity(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Activity{}, err
	}
	return a, nil
}

// refusedChange explains why a guarded write to a live activity changed no row: the
// activity is out of reach, or the actor may not change it.
func refusedChange(ctx context.Context, q Querier, workspaceID, id string) error {
	var live int
	err := q.QueryRow(ctx, `
		SELECT 1 FROM activities
		 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL`,
		workspaceID, id).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrActivityNotFound
	}
	if err != nil {
		return fmt.Errorf("read activity: %w", err)
	}
	return ErrActivityForbidden
}

// UpdateActivity changes the fields the patch names. Only the author or an owner or
// admin may, with the role read inside the transaction.
func (s *Store) UpdateActivity(ctx context.Context, workspaceID, actorID, id string, patch model.ActivityPatch) (model.Activity, error) {
	if !validUUID(id) {
		return model.Activity{}, ErrActivityNotFound
	}
	var a model.Activity
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		var links activityLinks
		if patch.SetCompany {
			links.company = patch.CompanyID
		}
		if patch.SetContact {
			links.contact = patch.ContactID
		}
		if patch.SetAssignee {
			links.assignee = patch.AssigneeID
		}
		if patch.SetDeal {
			links.deal = patch.DealID
		}
		role, err := links.hold(ctx, q, workspaceID, actorID)
		if err != nil {
			return err
		}
		tag, err := q.Exec(ctx, `
			UPDATE activities
			   SET subject = COALESCE($3, subject),
			       body = COALESCE($4, body),
			       due_at = CASE WHEN $5::boolean THEN $6::timestamptz ELSE due_at END,
			       assignee_id = CASE WHEN $7::boolean THEN $8::text::uuid ELSE assignee_id END,
			       contact_id = CASE WHEN $9::boolean THEN $10::text::uuid ELSE contact_id END,
			       company_id = CASE WHEN $11::boolean THEN $12::text::uuid ELSE company_id END,
			       deal_id = CASE WHEN $13::boolean THEN $14::text::uuid ELSE deal_id END,
			       updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL
			   AND ($15::boolean OR author_id = $16::text::uuid)`,
			workspaceID, id, patch.Subject, patch.Body, patch.SetDue, patch.DueAt,
			patch.SetAssignee, patch.AssigneeID, patch.SetContact, patch.ContactID,
			patch.SetCompany, patch.CompanyID, patch.SetDeal, patch.DealID,
			canManageMembers(role), actorID)
		if err != nil {
			return activityWriteError("update activity", err)
		}
		if tag.RowsAffected() == 0 {
			return refusedChange(ctx, q, workspaceID, id)
		}
		a, err = readActivity(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Activity{}, err
	}
	return a, nil
}

// DeleteActivity soft-deletes a live activity. Only the author or an owner or admin may.
func (s *Store) DeleteActivity(ctx context.Context, workspaceID, actorID, id string) error {
	if !validUUID(id) {
		return ErrActivityNotFound
	}
	return s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		role, err := holdActor(ctx, q, workspaceID, actorID)
		if err != nil {
			return err
		}
		tag, err := q.Exec(ctx, `
			UPDATE activities SET deleted_at = now(), updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL
			   AND ($3::boolean OR author_id = $4::text::uuid)`,
			workspaceID, id, canManageMembers(role), actorID)
		if err != nil {
			return fmt.Errorf("delete activity: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return refusedChange(ctx, q, workspaceID, id)
		}
		return nil
	})
}

// SetActivityCompleted completes or reopens a task, and is open to every member: a
// task is shared work. Completing a completed task keeps the time it was first done.
// It returns ErrNotATask for anything but a task.
func (s *Store) SetActivityCompleted(ctx context.Context, workspaceID, id string, completed bool) (model.Activity, error) {
	if !validUUID(id) {
		return model.Activity{}, ErrActivityNotFound
	}
	var a model.Activity
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		var kind model.ActivityKind
		err := q.QueryRow(ctx, `
			UPDATE activities
			   SET completed_at = CASE WHEN kind <> 'task' THEN completed_at
			                           WHEN $3::boolean THEN COALESCE(completed_at, now())
			                           ELSE NULL END,
			       updated_at = CASE WHEN kind = 'task' THEN now() ELSE updated_at END
			 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL
			 RETURNING kind`, workspaceID, id, completed).Scan(&kind)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrActivityNotFound
		}
		if err != nil {
			return fmt.Errorf("complete activity: %w", err)
		}
		if kind != model.ActivityTask {
			return ErrNotATask
		}
		a, err = readActivity(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Activity{}, err
	}
	return a, nil
}

// The link columns unlinkActivities can clear. These are constants, never input.
const (
	activityContactLink = "contact_id"
	activityCompanyLink = "company_id"
	activityDealLink    = "deal_id"
)

// lockActivities takes the row locks of the activities a multi-row update is about to
// change, in ID order. Two such updates can overlap, as when a company and one of its
// contacts are deleted at once; taking their rows in one order keeps them from
// deadlocking. A write to one activity holds that row only once its links are held, and
// takes nothing after it, so it cannot close a cycle either.
func lockActivities(ctx context.Context, q Querier, workspaceID, where, id string) error {
	if _, err := q.Exec(ctx, `
		SELECT 1 FROM activities
		 WHERE workspace_id = $1::text::uuid AND (`+where+`)
		 ORDER BY id
		   FOR NO KEY UPDATE`, workspaceID, id); err != nil {
		return fmt.Errorf("lock activities: %w", err)
	}
	return nil
}

// unlinkActivities clears one link on every activity that has it, soft-deleted ones
// too, so nothing points at what is going away. An activity whose last link this was
// is soft-deleted in the same statement, which keeps the rule that a live activity
// links to something. updated_at moves, so a later sync notices.
//
// Callers run this last, after they have updated deals: a deal write holds the pipeline
// and then deals, and an activity write holds a deal and then its own row, so activity
// rows come after deals in the lock order. By now the caller has updated the row being
// unlinked, which keeps new links from arriving behind its back. See holdLiveCompany.
func unlinkActivities(ctx context.Context, q Querier, workspaceID, column, id string) error {
	if err := lockActivities(ctx, q, workspaceID, column+` = $2::text::uuid`, id); err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `
		UPDATE activities
		   SET `+column+` = NULL,
		       deleted_at = CASE WHEN deleted_at IS NULL AND num_nonnulls(contact_id, company_id, deal_id) = 1
		                         THEN now() ELSE deleted_at END,
		       updated_at = now()
		 WHERE workspace_id = $1::text::uuid AND `+column+` = $2::text::uuid`, workspaceID, id); err != nil {
		return fmt.Errorf("unlink activities: %w", err)
	}
	return nil
}

// clearActivityMember forgets a departing member on every activity they wrote or were
// assigned, which stays: the note is still about its contact, and the task still needs
// doing, by someone a manager picks. As for deals, rejoining does not bring them back.
func clearActivityMember(ctx context.Context, q Querier, workspaceID, userID string) error {
	const named = `author_id = $2::text::uuid OR assignee_id = $2::text::uuid`
	if err := lockActivities(ctx, q, workspaceID, named, userID); err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `
		UPDATE activities
		   SET author_id = CASE WHEN author_id = $2::text::uuid THEN NULL ELSE author_id END,
		       assignee_id = CASE WHEN assignee_id = $2::text::uuid THEN NULL ELSE assignee_id END,
		       updated_at = now()
		 WHERE workspace_id = $1::text::uuid AND (`+named+`)`, workspaceID, userID); err != nil {
		return fmt.Errorf("clear activity member: %w", err)
	}
	return nil
}
