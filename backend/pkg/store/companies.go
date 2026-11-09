package store

import (
	"context"
	"errors"
	"fmt"

	"crm/pkg/model"

	"github.com/jackc/pgx/v5"
)

// ErrCompanyNotFound covers a nonexistent company, a deleted one, one in another
// workspace, and a malformed ID, so callers cannot tell them apart.
var ErrCompanyNotFound = errors.New("company not found")

const companyColumns = `c.id::text, c.workspace_id::text, c.name, c.domain, c.phone, c.industry, c.address, c.created_at, c.updated_at`

func scanCompany(row pgx.Row) (model.Company, error) {
	var c model.Company
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Domain, &c.Phone, &c.Industry, &c.Address, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// ListCompanies returns one page of the workspace's live companies, ordered by
// name without regard to case and then by ID. Search matches the name and domain.
func (s *Store) ListCompanies(ctx context.Context, workspaceID string, params model.ListParams) (model.Page[model.Company], error) {
	p := params.Bounded()
	var page model.Page[model.Company]
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		rows, err := q.Query(ctx, `
			SELECT `+companyColumns+`
			  FROM companies c
			 WHERE c.workspace_id = $1::text::uuid AND c.deleted_at IS NULL
			   AND ($2::text IS NULL OR c.name ILIKE $2 ESCAPE '\' OR c.domain ILIKE $2 ESCAPE '\')
			 ORDER BY lower(c.name), c.id
			 LIMIT $3 OFFSET $4`, workspaceID, containsPattern(p.Search), p.Limit+1, p.Offset)
		if err != nil {
			return fmt.Errorf("list companies: %w", err)
		}
		page, err = collectPage(rows, p, func(row pgx.CollectableRow) (model.Company, error) { return scanCompany(row) })
		if err != nil {
			return fmt.Errorf("list companies: %w", err)
		}
		return nil
	})
	return page, err
}

// CreateCompany inserts a company into the workspace. It returns
// ErrWorkspaceNotFound if the workspace does not exist.
func (s *Store) CreateCompany(ctx context.Context, workspaceID string, in model.CompanyInput) (model.Company, error) {
	var c model.Company
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		var err error
		c, err = scanCompany(q.QueryRow(ctx, `
			INSERT INTO companies AS c (workspace_id, name, domain, phone, industry, address)
			VALUES ($1::text::uuid, $2, $3, $4, $5, $6)
			RETURNING `+companyColumns,
			workspaceID, in.Name, in.Domain, in.Phone, in.Industry, in.Address))
		if hasCode(err, pgForeignKeyViolation) {
			return ErrWorkspaceNotFound
		}
		if err != nil {
			return fmt.Errorf("insert company: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.Company{}, err
	}
	return c, nil
}

// GetCompany returns a live company of the workspace, or ErrCompanyNotFound.
func (s *Store) GetCompany(ctx context.Context, workspaceID, id string) (model.Company, error) {
	if !validUUID(id) {
		return model.Company{}, ErrCompanyNotFound
	}
	var c model.Company
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		var err error
		c, err = scanCompany(q.QueryRow(ctx, `
			SELECT `+companyColumns+`
			  FROM companies c
			 WHERE c.workspace_id = $1::text::uuid AND c.id = $2::text::uuid AND c.deleted_at IS NULL`,
			workspaceID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCompanyNotFound
		}
		if err != nil {
			return fmt.Errorf("read company: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.Company{}, err
	}
	return c, nil
}

// UpdateCompany changes the fields the patch names, or returns ErrCompanyNotFound.
func (s *Store) UpdateCompany(ctx context.Context, workspaceID, id string, patch model.CompanyPatch) (model.Company, error) {
	if !validUUID(id) {
		return model.Company{}, ErrCompanyNotFound
	}
	var c model.Company
	err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		var err error
		c, err = scanCompany(q.QueryRow(ctx, `
			UPDATE companies AS c
			   SET name = COALESCE($3, c.name),
			       domain = COALESCE($4, c.domain),
			       phone = COALESCE($5, c.phone),
			       industry = COALESCE($6, c.industry),
			       address = COALESCE($7, c.address),
			       updated_at = now()
			 WHERE c.workspace_id = $1::text::uuid AND c.id = $2::text::uuid AND c.deleted_at IS NULL
			 RETURNING `+companyColumns,
			workspaceID, id, patch.Name, patch.Domain, patch.Phone, patch.Industry, patch.Address))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCompanyNotFound
		}
		if err != nil {
			return fmt.Errorf("update company: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.Company{}, err
	}
	return c, nil
}

// DeleteCompany soft-deletes a live company, or returns ErrCompanyNotFound. Its
// contacts and deals stay, because they are still people and sales the workspace
// knows, and lose the link in the same transaction. That covers soft-deleted ones
// too, so nothing points at a company nobody can see. updated_at moves, so a later
// sync notices.
//
// The company's row is updated first. That waits for any link still in progress,
// and the unlinks, new statements, then see it. See holdLiveCompany. Contacts come
// before deals, because a deal being linked may hold a share lock on a contact while
// it waits for the pipeline lock, which unlinkDeals takes.
func (s *Store) DeleteCompany(ctx context.Context, workspaceID, id string) error {
	if !validUUID(id) {
		return ErrCompanyNotFound
	}
	return s.InWorkspace(ctx, workspaceID, func(q Querier) error {
		tag, err := q.Exec(ctx, `
			UPDATE companies SET deleted_at = now(), updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND id = $2::text::uuid AND deleted_at IS NULL`,
			workspaceID, id)
		if err != nil {
			return fmt.Errorf("delete company: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrCompanyNotFound
		}
		if _, err := q.Exec(ctx, `
			UPDATE contacts SET company_id = NULL, updated_at = now()
			 WHERE workspace_id = $1::text::uuid AND company_id = $2::text::uuid`,
			workspaceID, id); err != nil {
			return fmt.Errorf("unlink contacts: %w", err)
		}
		return unlinkDeals(ctx, q, workspaceID, dealCompanyLink, id)
	})
}
