-- Contacts belong to one workspace and may link to one company of that workspace.
-- Deleting one is a soft delete, so crm_app cannot remove rows at all.
--
-- The composite foreign key is what keeps the link inside the workspace: foreign
-- key checks bypass Row-Level Security, so a reference on company_id alone would
-- accept a company from another workspace. It cannot tell a soft-deleted company
-- from a live one; the store does that, and clears links when it deletes a company.
--
-- external_contact_id keeps a later integration with the contacts project possible.
-- Nothing reads it yet. TEXT, because that is how contacts stores its IDs.
CREATE TABLE contacts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspaces (id),
    company_id          UUID,
    external_contact_id TEXT CHECK (length(external_contact_id) <= 255),
    first_name          TEXT NOT NULL CHECK (length(first_name) BETWEEN 1 AND 100),
    last_name           TEXT NOT NULL DEFAULT '' CHECK (length(last_name) <= 100),
    email               TEXT NOT NULL DEFAULT '' CHECK (length(email) <= 254),
    phone               TEXT NOT NULL DEFAULT '' CHECK (length(phone) <= 50),
    title               TEXT NOT NULL DEFAULT '' CHECK (length(title) <= 200),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, company_id) REFERENCES companies (workspace_id, id)
);

REVOKE DELETE ON contacts FROM crm_app;

-- Serves the list order and, by narrowing to one workspace, its search. See
-- 007_companies.sql for why there is no trigram index.
CREATE INDEX contacts_list_idx ON contacts (workspace_id, lower(first_name), lower(last_name), id) WHERE deleted_at IS NULL;

-- Serves unlinking when a company is deleted.
CREATE INDEX contacts_company_idx ON contacts (workspace_id, company_id) WHERE company_id IS NOT NULL;

ALTER TABLE contacts ENABLE ROW LEVEL SECURITY;
ALTER TABLE contacts FORCE ROW LEVEL SECURITY;

CREATE POLICY contacts_tenant ON contacts FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);
