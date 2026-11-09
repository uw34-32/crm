-- An activity is a note, call, meeting, or task about a contact, a company, or a deal
-- of one workspace. Deleting one is a soft delete, so crm_app cannot remove rows at all.
--
-- Every reference is a composite foreign key with workspace_id, because foreign key
-- checks bypass Row-Level Security: on the ID alone, a reference would accept a row of
-- another workspace. The keys cannot tell a soft-deleted row from a live one; the store
-- does that, and clears an activity's link when what it links to is deleted.
--
-- A live activity links to at least one contact, company, or deal. When a delete takes
-- away its last link, the store soft-deletes the activity in the same statement, so the
-- rule always holds and nothing is left about something nobody can see.
--
-- Only a task has a due time, an assignee, and a completed state. A note, call, or
-- meeting records something that already happened, at created_at.
--
-- author_id and assignee_id are nullable because an activity outlives its people's
-- membership. The store clears them when it removes a member, which moves updated_at,
-- and ON DELETE SET NULL is the backstop, as for a deal's owner.
CREATE TABLE activities (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces (id),
    kind         TEXT NOT NULL CHECK (kind IN ('note', 'call', 'meeting', 'task')),
    subject      TEXT NOT NULL CHECK (length(subject) BETWEEN 1 AND 200),
    body         TEXT NOT NULL DEFAULT '' CHECK (length(body) <= 10000),
    due_at       TIMESTAMPTZ CHECK (due_at BETWEEN '1900-01-01 00:00:00+00' AND '2200-12-31 23:59:59+00'),
    completed_at TIMESTAMPTZ,
    author_id    UUID,
    assignee_id  UUID,
    contact_id   UUID,
    company_id   UUID,
    deal_id      UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ,
    CONSTRAINT activities_task_fields
        CHECK (kind = 'task' OR (due_at IS NULL AND completed_at IS NULL AND assignee_id IS NULL)),
    CONSTRAINT activities_needs_a_link
        CHECK (deleted_at IS NOT NULL OR num_nonnulls(contact_id, company_id, deal_id) > 0),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id),
    FOREIGN KEY (workspace_id, company_id) REFERENCES companies (workspace_id, id),
    FOREIGN KEY (workspace_id, deal_id) REFERENCES deals (workspace_id, id),
    FOREIGN KEY (workspace_id, author_id) REFERENCES workspace_members (workspace_id, user_id) ON DELETE SET NULL (author_id),
    FOREIGN KEY (workspace_id, assignee_id) REFERENCES workspace_members (workspace_id, user_id) ON DELETE SET NULL (assignee_id)
);

REVOKE DELETE ON activities FROM crm_app;

-- Serves the timeline, newest first.
CREATE INDEX activities_timeline_idx ON activities (workspace_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;

-- Serve the timelines of one record, and the unlinking when it, or a member, goes away.
CREATE INDEX activities_contact_idx ON activities (workspace_id, contact_id) WHERE contact_id IS NOT NULL;
CREATE INDEX activities_company_idx ON activities (workspace_id, company_id) WHERE company_id IS NOT NULL;
CREATE INDEX activities_deal_idx ON activities (workspace_id, deal_id) WHERE deal_id IS NOT NULL;
CREATE INDEX activities_author_idx ON activities (workspace_id, author_id) WHERE author_id IS NOT NULL;
CREATE INDEX activities_assignee_idx ON activities (workspace_id, assignee_id) WHERE assignee_id IS NOT NULL;

ALTER TABLE activities ENABLE ROW LEVEL SECURITY;
ALTER TABLE activities FORCE ROW LEVEL SECURITY;

CREATE POLICY activities_tenant ON activities FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);
