-- Pipelines belong to one workspace, and hold ordered stages. Both soft-delete, so
-- crm_app cannot remove rows at all: a soft-deleted deal keeps pointing at the stage
-- it was in, and a later sync needs to see what went away.
--
-- A stage's kind, never its name, says whether it means won or lost.
--
-- position is a dense index, 0, 1, 2, ... among a pipeline's live stages. The store
-- serializes every change to them on the pipeline's row lock. The unique constraint
-- is the backstop that makes a mistake there fail rather than corrupt the order. It
-- is DEFERRABLE, because only then does PostgreSQL check it at the end of a statement
-- instead of row by row, which is what lets one UPDATE shift or swap positions. A
-- soft-deleted stage has a NULL position, which leaves the ordering, and which the
-- constraint ignores.
--
-- Migrations see the data only if Row-Level Security does not bind them, and a role it
-- does bind would find no workspaces and backfill none, reporting success. FORCE
-- applies to the table owner, so owning the tables is not enough.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = current_user AND (rolsuper OR rolbypassrls)) THEN
        RAISE EXCEPTION 'role % must be a superuser or have BYPASSRLS, because this migration must bypass Row-Level Security to backfill existing workspaces', current_user;
    END IF;
END
$$;

CREATE TABLE pipelines (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces (id),
    name         TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ,
    UNIQUE (workspace_id, id)
);

REVOKE DELETE ON pipelines FROM crm_app;

CREATE INDEX pipelines_list_idx ON pipelines (workspace_id, created_at, id) WHERE deleted_at IS NULL;

-- UNIQUE (workspace_id, pipeline_id, id) is the target of the foreign key that keeps a
-- deal's stage inside the deal's pipeline.
CREATE TABLE pipeline_stages (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    pipeline_id  UUID NOT NULL,
    name         TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    position     INT CHECK (position >= 0),
    color        TEXT NOT NULL CHECK (color ~ '^#[0-9a-f]{6}$'),
    probability  SMALLINT NOT NULL CHECK (probability BETWEEN 0 AND 100),
    kind         TEXT NOT NULL CHECK (kind IN ('open', 'won', 'lost')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ,
    CHECK ((deleted_at IS NULL) = (position IS NOT NULL)),
    UNIQUE (workspace_id, pipeline_id, id),
    UNIQUE (workspace_id, pipeline_id, position) DEFERRABLE INITIALLY IMMEDIATE,
    FOREIGN KEY (workspace_id, pipeline_id) REFERENCES pipelines (workspace_id, id)
);

REVOKE DELETE ON pipeline_stages FROM crm_app;

ALTER TABLE pipelines ENABLE ROW LEVEL SECURITY;
ALTER TABLE pipelines FORCE ROW LEVEL SECURITY;
ALTER TABLE pipeline_stages ENABLE ROW LEVEL SECURITY;
ALTER TABLE pipeline_stages FORCE ROW LEVEL SECURITY;

CREATE POLICY pipelines_tenant ON pipelines FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

CREATE POLICY pipeline_stages_tenant ON pipeline_stages FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

-- The one definition of the default stages, used by this migration and by the store,
-- so existing and new workspaces cannot drift apart. It is not SECURITY DEFINER: it runs
-- with the caller's rights, so for crm_app the policies above still decide.
CREATE FUNCTION create_pipeline_with_default_stages(p_workspace_id UUID, p_name TEXT DEFAULT 'Sales')
RETURNS UUID
LANGUAGE plpgsql
AS $$
DECLARE
    new_id UUID;
BEGIN
    INSERT INTO pipelines (workspace_id, name) VALUES (p_workspace_id, p_name) RETURNING id INTO new_id;
    INSERT INTO pipeline_stages (workspace_id, pipeline_id, position, name, color, probability, kind) VALUES
        (p_workspace_id, new_id, 0, 'Lead',        '#64748b',  10, 'open'),
        (p_workspace_id, new_id, 1, 'Qualified',   '#3b82f6',  25, 'open'),
        (p_workspace_id, new_id, 2, 'Proposal',    '#8b5cf6',  50, 'open'),
        (p_workspace_id, new_id, 3, 'Negotiation', '#f59e0b',  75, 'open'),
        (p_workspace_id, new_id, 4, 'Won',         '#22c55e', 100, 'won'),
        (p_workspace_id, new_id, 5, 'Lost',        '#ef4444',   0, 'lost');
    RETURN new_id;
END
$$;

REVOKE ALL ON FUNCTION create_pipeline_with_default_stages(UUID, TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION create_pipeline_with_default_stages(UUID, TEXT) TO crm_app;

-- Soft-deleted workspaces too, so that every workspace row has one.
SELECT create_pipeline_with_default_stages(id) FROM workspaces;
