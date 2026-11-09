-- Companies belong to one workspace. Deleting one is a soft delete, so crm_app
-- cannot remove rows at all.
--
-- UNIQUE (workspace_id, id) is the target of composite foreign keys from other
-- tenant tables. Foreign key checks bypass Row-Level Security, so a reference on
-- the ID alone would accept a company from another workspace.
CREATE TABLE companies (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces (id),
    name         TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    domain       TEXT NOT NULL DEFAULT '' CHECK (length(domain) <= 253),
    phone        TEXT NOT NULL DEFAULT '' CHECK (length(phone) <= 50),
    industry     TEXT NOT NULL DEFAULT '' CHECK (length(industry) <= 100),
    address      TEXT NOT NULL DEFAULT '' CHECK (length(address) <= 500),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ,
    UNIQUE (workspace_id, id)
);

REVOKE DELETE ON companies FROM crm_app;

-- Serves the list order and, by narrowing to one workspace, its search. A trigram
-- index would not help: ILIKE is not leakproof, so under Row-Level Security the
-- planner must run it after the policy and cannot use it as an index condition.
CREATE INDEX companies_list_idx ON companies (workspace_id, lower(name), id) WHERE deleted_at IS NULL;

ALTER TABLE companies ENABLE ROW LEVEL SECURITY;
ALTER TABLE companies FORCE ROW LEVEL SECURITY;

CREATE POLICY companies_tenant ON companies FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);
