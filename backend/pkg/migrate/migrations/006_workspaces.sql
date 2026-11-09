-- Workspaces are the tenant boundary. Row-Level Security here is the second
-- layer behind the explicit workspace_id filter in every store query.
--
-- Two transaction-local settings drive the policies, and both fail closed: unset
-- or empty means NULL, and comparing with NULL matches nothing.
--   app.workspace_id  full access to one workspace, set only after the membership
--                     middleware has confirmed the caller belongs to it.
--   app.user_id       read-only view of the caller's own memberships, for the two
--                     lookups that happen before a workspace is known.
--
-- FORCE applies these policies to the table owner as well. Migrations still run
-- unaffected because the owner in Compose is a superuser, and superusers always
-- bypass Row-Level Security.
CREATE TABLE workspaces (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

-- Deleting a workspace is a soft delete. Nothing in the app removes the row.
REVOKE DELETE ON workspaces FROM crm_app;

-- The composite primary key doubles as the target of future composite foreign
-- keys such as (workspace_id, assigned_to). RESTRICT on the user: removing an
-- account must first resolve the workspaces it owns.
CREATE TABLE workspace_members (
    workspace_id UUID NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    role         TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);

CREATE INDEX workspace_members_user_id_idx ON workspace_members (user_id);

ALTER TABLE workspaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspaces FORCE ROW LEVEL SECURITY;
ALTER TABLE workspace_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace_members FORCE ROW LEVEL SECURITY;

CREATE POLICY workspaces_tenant ON workspaces FOR ALL
    USING (id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

CREATE POLICY workspace_members_tenant ON workspace_members FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

-- SELECT only. This policy must not query workspace_members, or it would recurse.
CREATE POLICY workspace_members_self_read ON workspace_members FOR SELECT
    USING (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid);

-- SELECT only. Its subquery sees just the caller's rows through the policy above,
-- which does not refer back to workspaces, so there is no cycle.
CREATE POLICY workspaces_member_read ON workspaces FOR SELECT
    USING (id IN (SELECT workspace_id FROM workspace_members
                   WHERE user_id = NULLIF(current_setting('app.user_id', true), '')::uuid));
