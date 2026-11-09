package store

import (
	"context"
	"errors"
	"fmt"

	"crm/pkg/model"

	"github.com/jackc/pgx/v5"
)

// ErrContactNotFound covers a nonexistent contact, a deleted one, one in another
// workspace, and a malformed ID, so callers cannot tell them apart.
var ErrContactNotFound = errors.New("contact not found")

// contactSelect reads contacts with their company. It filters out a deleted
// company as well, although deleting one also clears its links.
const contactSelect = `
	SELECT ct.id::text, ct.workspace_id::text, ct.first_name, ct.last_name, ct.email, ct.phone, ct.title,
	       ct.company_id::text, co.name, ct.created_at, ct.updated_at
	  FROM contacts ct
	  LEFT JOIN companies co
	    ON co.workspace_id = ct.workspace_id AND co.id = ct.company_id AND co.deleted_at IS NULL`

func scanContact(row pgx.Row) (model.Contact, error) {
	var c model.Contact
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.FirstName, &c.LastName, &c.Email, &c.Phone, &c.Title,
		&c.CompanyID, &c.CompanyName, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func readContact(ctx context.Context, q Querier, workspaceID, id string) (model.Contact, error) {
	c, err := scanContact(q.QueryRow(ctx, contactSelect+`
		 WHERE ct.workspace_id = $1::text::uuid AND ct.id = $2::text::uuid AND ct.deleted_at IS NULL`,
		workspaceID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Contact{}, ErrContactNotFound
	}
	if err != nil {
		return model.Contact{}, fmt.Errorf("read contact: %w", err)
	}
	return c, nil
}

// holdLiveCompany confirms the company is live and in this workspace, and keeps it
// that way until the transaction ends. The foreign key alone could not refuse a
// soft-deleted company. FOR SHARE, because FOR KEY SHARE does not conflict with the
// update that soft-deletes a company: a delete either waits for this link to commit
// and then clears it, or commits first and this finds no row.
//
// Every write that links a contact takes this lock before touching the contact, in
// the same order as a delete, so the two cannot deadlock.
func holdLiveCompany(ctx context.Context, q Querier, workspaceID, companyID string) error {
	if !validUUID(companyID) {
		return ErrCompanyNotFound
	}
	var live int
	err := q.QueryRow(ctx, `
		SELECT 1 FROM companies
		 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL
		   FOR SHARE`, workspaceID, companyID).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCompanyNotFound
	}
	if err != nil {
		return fmt.Errorf("lock company: %w", err)
	}
	return nil
}

// holdLiveContact is holdLiveCompany for a contact, for the writes that link a deal.
func holdLiveContact(ctx context.Context, q Querier, workspaceID, contactID string) error {
	if !validUUID(contactID) {
		return ErrContactNotFound
	}
	var live int
	err := q.QueryRow(ctx, `
		SELECT 1 FROM contacts
		 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL
		   FOR SHARE`, workspaceID, contactID).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrContactNotFound
	}
	if err != nil {
		return fmt.Errorf("lock contact: %w", err)
	}
	return nil
}

// ListContacts returns one page of the workspace's live contacts, ordered by name
// without regard to case and then by ID. Search matches the full name, the email,
// and the linked company's name and domain.
func (s *Store) ListContacts(ctx context.Context, workspaceID string, params model.ListParams) (model.Page[model.Contact], error) {
	p := params.Bounded()
	var page model.Page[model.Contact]
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		rows, err := q.Query(ctx, contactSelect+`
			 WHERE ct.workspace_id = $1::text::uuid AND ct.deleted_at IS NULL
			   AND ($2::text IS NULL
			        OR (ct.first_name || ' ' || ct.last_name) ILIKE $2 ESCAPE '\'
			        OR ct.email ILIKE $2 ESCAPE '\'
			        OR co.name ILIKE $2 ESCAPE '\'
			        OR co.domain ILIKE $2 ESCAPE '\')
			 ORDER BY lower(ct.first_name), lower(ct.last_name), ct.id
			 LIMIT $3 OFFSET $4`, workspaceID, containsPattern(p.Search), p.Limit+1, p.Offset)
		if err != nil {
			return fmt.Errorf("list contacts: %w", err)
		}
		page, err = collectPage(rows, p, func(row pgx.CollectableRow) (model.Contact, error) { return scanContact(row) })
		if err != nil {
			return fmt.Errorf("list contacts: %w", err)
		}
		return nil
	})
	return page, err
}

// CreateContact inserts a contact into the workspace. It returns ErrCompanyNotFound
// if the company cannot be linked, and ErrWorkspaceNotFound if the workspace does
// not exist.
func (s *Store) CreateContact(ctx context.Context, workspaceID string, in model.ContactInput) (model.Contact, error) {
	var c model.Contact
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		if in.CompanyID != nil {
			if err := holdLiveCompany(ctx, q, workspaceID, *in.CompanyID); err != nil {
				return err
			}
		}
		var id string
		err := q.QueryRow(ctx, `
			INSERT INTO contacts (workspace_id, first_name, last_name, email, phone, title, company_id)
			VALUES ($1::text::uuid, $2, $3, $4, $5, $6, $7::text::uuid)
			RETURNING id::text`,
			workspaceID, in.FirstName, in.LastName, in.Email, in.Phone, in.Title, in.CompanyID).Scan(&id)
		if hasCode(err, pgForeignKeyViolation) {
			return ErrWorkspaceNotFound
		}
		if err != nil {
			return fmt.Errorf("insert contact: %w", err)
		}
		c, err = readContact(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Contact{}, err
	}
	return c, nil
}

// GetContact returns a live contact of the workspace, or ErrContactNotFound.
func (s *Store) GetContact(ctx context.Context, workspaceID, id string) (model.Contact, error) {
	if !validUUID(id) {
		return model.Contact{}, ErrContactNotFound
	}
	var c model.Contact
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		var err error
		c, err = readContact(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Contact{}, err
	}
	return c, nil
}

// UpdateContact changes the fields the patch names. It returns ErrContactNotFound
// for a contact it cannot reach, and ErrCompanyNotFound for a company it cannot link.
func (s *Store) UpdateContact(ctx context.Context, workspaceID, id string, patch model.ContactPatch) (model.Contact, error) {
	if !validUUID(id) {
		return model.Contact{}, ErrContactNotFound
	}
	var c model.Contact
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		if patch.SetCompany && patch.CompanyID != nil {
			if err := holdLiveCompany(ctx, q, workspaceID, *patch.CompanyID); err != nil {
				return err
			}
		}
		tag, err := q.Exec(ctx, `
			UPDATE contacts
			   SET first_name = COALESCE($3, first_name),
			       last_name = COALESCE($4, last_name),
			       email = COALESCE($5, email),
			       phone = COALESCE($6, phone),
			       title = COALESCE($7, title),
			       company_id = CASE WHEN $8::boolean THEN $9::text::uuid ELSE company_id END,
			       updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL`,
			workspaceID, id, patch.FirstName, patch.LastName, patch.Email, patch.Phone, patch.Title, patch.SetCompany, patch.CompanyID)
		if err != nil {
			return fmt.Errorf("update contact: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrContactNotFound
		}
		c, err = readContact(ctx, q, workspaceID, id)
		return err
	})
	if err != nil {
		return model.Contact{}, err
	}
	return c, nil
}

// DeleteContact soft-deletes a live contact, or returns ErrContactNotFound. Its deals
// stay, and lose the link in the same transaction. The contact's row is updated first,
// as in DeleteCompany, so a deal being linked to it either commits first and is seen by
// the unlink, or finds no live contact afterwards.
func (s *Store) DeleteContact(ctx context.Context, workspaceID, id string) error {
	if !validUUID(id) {
		return ErrContactNotFound
	}
	return s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		tag, err := q.Exec(ctx, `
			UPDATE contacts SET deleted_at = now(), updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL`,
			workspaceID, id)
		if err != nil {
			return fmt.Errorf("delete contact: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrContactNotFound
		}
		return unlinkDeals(ctx, q, workspaceID, dealContactLink, id)
	})
}
