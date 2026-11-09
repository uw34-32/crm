-- A deal is one sale moving through one pipeline. Deleting one is a soft delete, so
-- crm_app cannot remove rows at all.
--
-- Every reference is a composite foreign key with workspace_id, because foreign key
-- checks bypass Row-Level Security: on the ID alone, a reference would accept a row of
-- another workspace. (workspace_id, pipeline_id, stage_id) additionally makes it
-- impossible for a deal's stage to belong to a different pipeline than the deal.
--
-- The composite keys cannot tell a soft-deleted row from a live one. The store does
-- that, and clears links when it deletes a company or a contact.
--
-- value_minor is an integer in minor units, never a float. Its bound stays well below
-- 2^53, so a JSON client that reads numbers as doubles cannot lose a cent.
--
-- owner_id is nullable because a deal outlives its owner's membership. The store clears
-- it when it removes a member, which moves updated_at. ON DELETE SET NULL (owner_id) is
-- the backstop, so removing a member can never fail on this key; the column list leaves
-- the NOT NULL workspace_id alone.
--
-- position is a dense index, 0, 1, 2, ... among a stage's live deals, for the board.
-- The store serializes every change to one pipeline's positions on the pipeline's row
-- lock, and this constraint is the backstop. See 009_pipelines.sql for why it is
-- DEFERRABLE and why a soft-deleted row has a NULL position.
CREATE TABLE deals (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspaces (id),
    pipeline_id         UUID NOT NULL,
    stage_id            UUID NOT NULL,
    company_id          UUID,
    contact_id          UUID,
    owner_id            UUID,
    title               TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    value_minor         BIGINT NOT NULL DEFAULT 0 CHECK (value_minor BETWEEN 0 AND 999999999999999),
    currency            TEXT NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    expected_close_date DATE CHECK (expected_close_date BETWEEN '1900-01-01' AND '2200-12-31'),
    closed_at           TIMESTAMPTZ,
    lost_reason         TEXT NOT NULL DEFAULT '' CHECK (length(lost_reason) <= 500),
    position            INT CHECK (position >= 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ,
    CHECK ((deleted_at IS NULL) = (position IS NOT NULL)),
    CHECK (lost_reason = '' OR closed_at IS NOT NULL),
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, stage_id, position) DEFERRABLE INITIALLY IMMEDIATE,
    FOREIGN KEY (workspace_id, pipeline_id) REFERENCES pipelines (workspace_id, id),
    FOREIGN KEY (workspace_id, pipeline_id, stage_id) REFERENCES pipeline_stages (workspace_id, pipeline_id, id),
    FOREIGN KEY (workspace_id, company_id) REFERENCES companies (workspace_id, id),
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id),
    FOREIGN KEY (workspace_id, owner_id) REFERENCES workspace_members (workspace_id, user_id) ON DELETE SET NULL (owner_id)
);

REVOKE DELETE ON deals FROM crm_app;

-- Serves the pipeline filter and the check for live deals before a pipeline is deleted.
CREATE INDEX deals_pipeline_idx ON deals (workspace_id, pipeline_id) WHERE deleted_at IS NULL;

-- Serve the unlinking when a company, contact, or member goes away, and the owner filter.
CREATE INDEX deals_company_idx ON deals (workspace_id, company_id) WHERE company_id IS NOT NULL;
CREATE INDEX deals_contact_idx ON deals (workspace_id, contact_id) WHERE contact_id IS NOT NULL;
CREATE INDEX deals_owner_idx ON deals (workspace_id, owner_id) WHERE owner_id IS NOT NULL;

ALTER TABLE deals ENABLE ROW LEVEL SECURITY;
ALTER TABLE deals FORCE ROW LEVEL SECURITY;

CREATE POLICY deals_tenant ON deals FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);
