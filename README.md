# CRM

A team CRM. This repository currently holds the backend: configuration, migrations, database roles, a health route, accounts with token auth, workspaces with members and Row-Level Security, contacts and companies, and pipelines with stages and deals. The plan lives in `odd/tasks/crm-foundation.md`.

## Stack

Go, Chi v5, pgx v5, golang-jwt v5, bcrypt, and PostgreSQL 16 through Docker Compose.

## Layout

| Path | Purpose |
| --- | --- |
| `backend/cmd/server` | Entry point; handles SIGINT and SIGTERM |
| `backend/pkg/config` | Settings from the environment |
| `backend/pkg/migrate` | Embedded SQL migrations and their runner |
| `backend/pkg/auth` | Access tokens and password hashing |
| `backend/pkg/model` | Types shared by the store and the API |
| `backend/pkg/store` | Database access |
| `backend/pkg/app` | Routes, handlers, and middleware |
| `backend/pkg/server` | Wires the above and runs the HTTP server |
| `backend/pkg/testdb` | Connections for database-backed tests |
| `scripts/` | Local development helpers |

## Run

```sh
./dev.sh
curl -si localhost:8085/health
```

`dev.sh` starts the Compose database, waits until it is healthy, creates or updates the app role, then builds and runs the API. The server applies pending migrations on every boot. `GET /health` returns 200 when the database is reachable and 503 when it is not.

## Configuration

Settings come from the environment. `dev.sh` also reads an optional `.env` in the repository root, and Compose reads `POSTGRES_PASSWORD` from the same file. Never commit `.env`.

| Variable | Default in `dev.sh` | Notes |
| --- | --- | --- |
| `DATABASE_URL` | `postgres://crm_app:crm_app_local_dev@localhost:5435/crm?sslmode=disable` | Required by the server; must be the app role |
| `MIGRATION_DATABASE_URL` | `postgres://postgres:postgres@localhost:5435/crm?sslmode=disable` | Required by the server; the role that owns the schema |
| `PORT` | `8085` | |
| `MIGRATION_TIMEOUT` | `5m` | Deadline for the migration lock wait and all pending migrations; the server's built-in default |
| `JWT_SECRET` | local placeholder | Required by the server, which has no default; at least 32 bytes |
| `ACCESS_TOKEN_TTL` | `15m` | The server's built-in default |
| `REFRESH_TOKEN_TTL` | `720h` | The server's built-in default; each rotation issues a token with a fresh lifetime |
| `REFRESH_GRACE` | `10s` | The server's built-in default; `0s` disables retries of a spent token |
| `AUTH_RATE_LIMIT_PER_MINUTE` | `30` | The server's built-in default; per client, on `/auth/register` and `/auth/login`; must be positive |
| `AUTH_SESSION_RATE_LIMIT_PER_MINUTE` | `600` | The server's built-in default; per client, on session and workspace routes; must be positive |
| `CORS_ORIGINS` | SvelteKit, Expo web, and Tauri local origins | Comma-separated exact origins; none by default in the server; wildcards are refused |
| `TRUSTED_PROXIES` | none | Comma-separated addresses or CIDR prefixes; forwarding headers are ignored unless the peer is listed |
| `POSTGRES_PASSWORD` | `postgres` | Local only; applied when the volume is first created |
| `APP_DB_PASSWORD` | `crm_app_local_dev` | Local only; `scripts/bootstrap-db-roles.sh` sets it on every run; keep it URL-safe |
| `TEST_DATABASE_URL` | none | App role on the test database; set together with the next one |
| `TEST_MIGRATION_DATABASE_URL` | none | Owner on the test database; enables database-backed tests |

Compose publishes Postgres on `127.0.0.1:5435` only.

## Database roles

Two roles keep Row-Level Security meaningful:

| Role | Used by | Attributes |
| --- | --- | --- |
| Migration owner (`postgres` in Compose) | Migrations, through `MIGRATION_DATABASE_URL` | Owns the schema and every table |
| `crm_app` | The server's queries, through `DATABASE_URL` | `NOSUPERUSER NOBYPASSRLS`, owns nothing, has data access only |

At startup the server reads the role behind `DATABASE_URL` and refuses to boot if it is a superuser, has `BYPASSRLS`, or owns a table. The check runs before migrations, so a refused server changes nothing. After migrations it also checks every workspace-scoped table; see [Tenancy](#tenancy).

`crm_app` needs a password, so no migration creates it. `scripts/bootstrap-db-roles.sh` creates or updates it in the Compose database; it is safe to rerun and works on an existing volume. Migrations fail with a clear error if the role is missing. Elsewhere, create the role yourself with the same attributes before the first boot.

`002_app_role_grants.sql` gives `crm_app` `SELECT`, `INSERT`, `UPDATE`, and `DELETE` on every table the owner creates afterwards, through default privileges. Those apply only to objects created by the role that set them, so always migrate as the same owner. A table that needs less must `REVOKE` in its migration.

## API

Successful responses have the shape `{"data": ...}` and failures `{"error": "message"}`.

| Route | Body | Success | Notes |
| --- | --- | --- | --- |
| `GET /health` | | 200 | 503 when the database is unreachable |
| `POST /auth/register` | `email`, `name`, `password` | 201 with `token`, `refresh_token`, `user` | 409 when the email is taken, compared case-insensitively |
| `POST /auth/login` | `email`, `password` | 200 with `token`, `refresh_token`, `user` | 401 with the same response for a wrong password and an unknown email |
| `POST /auth/refresh` | `refresh_token` | 200 with `token`, `refresh_token`, `user` | 401 for any invalid token |
| `POST /auth/logout` | `refresh_token` | 204 | Also 204 for an unknown token |
| `GET /auth/me` | | 200 with the user | Needs `Authorization: Bearer <token>` |
| `POST /workspaces` | `name` | 201 with the workspace | The creator becomes its owner |
| `GET /workspaces` | | 200 with the caller's workspaces and role in each | |
| `GET /workspaces/{id}` | | 200 with the workspace and the caller's role | |
| `PATCH /workspaces/{id}` | `name` | 200 with the workspace | Owner or admin |
| `DELETE /workspaces/{id}` | | 204 | Owner; a soft delete |
| `POST /workspaces/{id}/leave` | | 204 | 409 for the last owner |
| `GET /workspaces/{id}/members` | | 200 with the members | Any member |
| `POST /workspaces/{id}/members` | `email`, `role` | 201 with the member | Owner or admin; 404 for an unknown email, 409 for an existing member |
| `PATCH /workspaces/{id}/members/{userID}` | `role` | 200 with the member | Owner or admin; 409 for the last owner |
| `DELETE /workspaces/{id}/members/{userID}` | | 204 | Owner or admin; 409 for the last owner |
| `GET /workspaces/{id}/companies` | | 200 with a page of companies | See [Contacts and companies](#contacts-and-companies) |
| `POST /workspaces/{id}/companies` | `name`, optional `domain`, `phone`, `industry`, `address` | 201 with the company | |
| `GET /workspaces/{id}/companies/{companyID}` | | 200 with the company | |
| `PATCH /workspaces/{id}/companies/{companyID}` | Any of the create fields | 200 with the company | |
| `DELETE /workspaces/{id}/companies/{companyID}` | | 204 | A soft delete; its contacts and deals stay and lose the link |
| `GET /workspaces/{id}/contacts` | | 200 with a page of contacts | |
| `POST /workspaces/{id}/contacts` | `first_name`, optional `last_name`, `email`, `phone`, `title`, `company_id` | 201 with the contact | |
| `GET /workspaces/{id}/contacts/{contactID}` | | 200 with the contact | |
| `PATCH /workspaces/{id}/contacts/{contactID}` | Any of the create fields | 200 with the contact | `company_id: null` removes the link |
| `DELETE /workspaces/{id}/contacts/{contactID}` | | 204 | A soft delete; its deals stay and lose the link |
| `GET /workspaces/{id}/pipelines` | | 200 with a page of pipelines, without stages | See [Pipelines and deals](#pipelines-and-deals) |
| `POST /workspaces/{id}/pipelines` | `name` | 201 with the pipeline and default stages | Owner or admin |
| `GET /workspaces/{id}/pipelines/{pipelineID}` | | 200 with the pipeline and its stages in order | |
| `PATCH /workspaces/{id}/pipelines/{pipelineID}` | `name` | 200 with the pipeline | Owner or admin |
| `DELETE /workspaces/{id}/pipelines/{pipelineID}` | | 204 | Owner or admin; a soft delete; 409 while it holds live deals, or for the workspace's last pipeline |
| `POST /workspaces/{id}/pipelines/{pipelineID}/stages` | `name`, optional `color`, `probability`, `kind`, `position` | 201 with the stage | Owner or admin |
| `PATCH /workspaces/{id}/pipelines/{pipelineID}/stages/{stageID}` | Any of `name`, `color`, `probability`, `kind` | 200 with the stage | Owner or admin; 409 to change the kind while it holds live deals, or to take away the last open stage |
| `PUT /workspaces/{id}/pipelines/{pipelineID}/stages/order` | `stage_ids` | 200 with the stages | Owner or admin; 409 unless it lists every stage exactly once |
| `DELETE /workspaces/{id}/pipelines/{pipelineID}/stages/{stageID}` | | 204 | Owner or admin; a soft delete; 409 while it holds live deals, or for the last open stage |
| `GET /workspaces/{id}/deals` | | 200 with a page of deals | Filters: `pipeline_id`, `stage_id`, `owner_id`, `status`, `q` |
| `POST /workspaces/{id}/deals` | `title`, `currency`, `pipeline_id`, optional `stage_id`, `value_minor`, `expected_close_date`, `company_id`, `contact_id`, `owner_id` | 201 with the deal | Starts in the first open stage and owned by the caller unless told otherwise |
| `GET /workspaces/{id}/deals/{dealID}` | | 200 with the deal | |
| `PATCH /workspaces/{id}/deals/{dealID}` | Any of `title`, `value_minor`, `currency`, `expected_close_date`, `company_id`, `contact_id`, `owner_id` | 200 with the deal | `null` clears the date or a reference |
| `POST /workspaces/{id}/deals/{dealID}/move` | `stage_id`, optional `position`, `lost_reason` | 200 with the deal | See [Moving a deal](#moving-a-deal) |
| `DELETE /workspaces/{id}/deals/{dealID}` | | 204 | A soft delete; closes the gap on the board |
| `GET /workspaces/{id}/activities` | | 200 with a page of activities | Newest first. Filters: `contact_id`, `company_id`, `deal_id`, `assignee_id`, `kind`, `status`, `due_before`, `due_after` |
| `POST /workspaces/{id}/activities` | `kind`, `subject`, and at least one of `contact_id`, `company_id`, `deal_id`; optional `body`, and for a task `due_at` and `assignee_id` | 201 with the activity | The caller is the author |
| `GET /workspaces/{id}/activities/{activityID}` | | 200 with the activity | |
| `PATCH /workspaces/{id}/activities/{activityID}` | Any of `subject`, `body`, `due_at`, `assignee_id`, `contact_id`, `company_id`, `deal_id` | 200 with the activity | The author, an owner, or an admin; others get a 403 |
| `POST /workspaces/{id}/activities/{activityID}/complete` | | 200 with the task | Any member; completing again keeps the first time. A 400 for anything but a task |
| `POST /workspaces/{id}/activities/{activityID}/reopen` | | 200 with the task | Any member. A 400 for anything but a task |
| `DELETE /workspaces/{id}/activities/{activityID}` | | 204 | A soft delete. The author, an owner, or an admin |

Workspace routes need `Authorization: Bearer <token>`. Roles are `owner`, `admin`, and `member`.

### Contacts and companies

Every member of a workspace, whatever their role, can read and write its contacts and companies.

A contact or company that is in another workspace, soft-deleted, nonexistent, or named by a malformed ID gets the same 404 with an identical body. The same goes for a `company_id` in a request body: a contact can link only a live company in its own workspace. Anything else that is invalid gets a 400: text beyond its limit, a NUL character, invalid UTF-8, a `PATCH` with no fields, or a `PATCH` that sets a text field to `null`. Text is trimmed, a `PATCH` changes only the fields it names, and an empty string clears an optional field.

| Field | Limit |
| --- | --- |
| Company `name` | Required; 200 characters |
| Company `domain` | A host name with no scheme, stored in lowercase |
| Company `industry`, `address` | 100 and 500 characters |
| Contact `first_name` | Required; 100 characters. Someone with one name goes here |
| Contact `last_name`, `title` | 100 and 200 characters |
| Contact `email` | A bare address. It does not have to be unique, because people share addresses such as `info@` |
| `phone` | 50 characters with at least one digit, stored as typed |

Lists take `limit` (1 to 200, default 50), `offset` (0 to 100,000), and `q` (at most 200 characters), and a value out of range gets a 400. A list answers with `items`, `limit`, `offset`, and `has_more`. Companies are ordered by name and contacts by first and last name, ignoring case, with the ID as a tiebreak.

`q` is a case-insensitive substring. `%`, `_`, and `\` in it are literal. Companies match on name and domain. Contacts match on full name, email, and the linked company's name and domain.

Deleting a company keeps its contacts, because they are still people the workspace knows, and clears their link in the same transaction, moving `updated_at`. A write that links a company holds a share lock on it, so a link and a delete cannot race into a contact pointing at a deleted company.

Search scans the live rows of one workspace, in list order, until the page is full. The `(workspace_id, ...)` list indexes make that scan tenant-sized, but no index narrows it further. A trigram index would not help: `ILIKE` is not leakproof, so under Row-Level Security PostgreSQL has to apply it after the policy and cannot use it as an index condition. If a workspace outgrows this, it needs a different search design rather than one more index.

`contacts.external_contact_id` keeps a later integration with the `contacts` project possible. It is nullable, and neither read nor written by the API.

### Pipelines and deals

Every member can read and write deals. Only an owner or admin can change pipelines and stages, and the role is re-read inside the transaction.

A workspace always keeps at least one pipeline, so a deal always has somewhere to start: deleting the last one gets a 409. Pipeline creates and deletes run one at a time on the workspace row lock, so two concurrent deletes cannot remove the last two. A new workspace gets a default pipeline, `Sales`, in the same transaction, with the stages Lead, Qualified, Proposal, Negotiation, Won, and Lost. `009_pipelines.sql` gave every existing workspace the same one, through the one SQL function that defines the default stages. A new pipeline starts with those stages too, so it always has an open stage.

A stage's `kind`, one of `open`, `won`, or `lost`, says what being in it means. Nothing reads a stage's name to decide that. A pipeline keeps at least one open stage. Stages soft-delete, like pipelines and deals, so a soft-deleted deal keeps a valid reference to the stage it was in.

| Field | Limit |
| --- | --- |
| Pipeline `name` | Required; 200 characters |
| Stage `name` | Required; 100 characters |
| Stage `color` | `#rrggbb`, stored in lowercase; `#64748b` by default |
| Stage `probability` | 0 to 100; by default 100 for a won stage and 0 otherwise |
| Stage `kind` | `open` by default |
| Stage or deal `position` | Not negative. Beyond the end means the end, because the board may have shrunk since the client read it |
| Deal `title` | Required; 200 characters |
| Deal `value_minor` | A whole number of minor units, such as cents, from 0 to 999999999999999, a bound well inside what a JSON double holds exactly. A fraction is refused |
| Deal `currency` | Required; three letters, stored in uppercase. The format is checked, not membership of the ISO list |
| Deal `expected_close_date` | `YYYY-MM-DD`, from 1900 to 2200 |
| Deal `lost_reason` | 500 characters |

Lists take `limit`, `offset`, and `q` as described under [Contacts and companies](#contacts-and-companies). Pipelines are ordered oldest first, and `q` matches the name. Deals are in board order: by pipeline, then stage position, then position in the stage. `q` matches the title and the linked company's and contact's names. `pipeline_id`, `stage_id`, `owner_id`, and `status` (`open`, `won`, or `lost`) filter deals, and a malformed filter gets a 400.

A deal's `company_id`, `contact_id`, and `owner_id` must be a live company, a live contact, and a current member of the workspace. Like a pipeline or stage out of reach, anything else gets the same 404 with an identical body, whether it is in another workspace, soft-deleted, nonexistent, or malformed. A write that links one holds a share lock on it, so a link and a delete cannot race into a dangling reference. Invalid UTF-8 and NUL characters get a 400. A `PATCH` that sets a text field or `value_minor` to `null` gets a 400, and `null` on `expected_close_date` or a reference clears it. A `PATCH` cannot set the stage, position, or lost reason, and a deal stays in its pipeline.

Deleting a company or contact keeps its deals and clears their links in the same transaction, moving `updated_at`, soft-deleted deals included. Removing a member, or a member leaving, keeps their deals with `owner_id` set to `null`, for a manager to reassign: picking a new owner is a judgement the removal cannot make, and someone who left has nobody to hand over to. Rejoining does not bring the deals back.

#### Moving a deal

A move puts a deal at a position in a stage of its own pipeline, and every member may do it. A stage of another pipeline gets a 404. A new deal starts in an open stage, and closes only through a move.

| Into | `closed_at` | `lost_reason` |
| --- | --- | --- |
| `open` | Cleared | Cleared; a 400 if one is sent, even blank or `null` |
| `won` | Set to now | Cleared; a 400 if one is sent, even blank or `null` |
| `lost` | Set to now | Required, or a 400 |
| The stage it is already in | Unchanged | Unchanged, unless a lost stage is given a new one |

`position` is a dense index, 0, 1, 2, ... among a stage's live deals, and the same goes for stages in a pipeline. So a client can use it directly as the place on the board, and a move names where it wants the deal, not how to compute a key between two neighbours. The cost is that a move rewrites the neighbours it shifts, which also moves their `updated_at` so that a later sync sees the new order.

Every write to one pipeline's structure and positions first takes that pipeline's row lock, so concurrent moves run one at a time, and each reads the board the last one left. No two deals can share a position, and no move works from a stale board. A deferrable unique constraint backs this up, refusing a duplicate at the end of a statement. An edit needs no lock, because it writes only the fields it names, and it cannot undo a move or be undone by one. Moves of one deal do not detect each other: the last to run wins. One pipeline's moves are serial, which is fine for the size of team this serves, and different pipelines do not wait for each other.

Writers take locks in one order, which keeps them from deadlocking: the workspace row, then the rows a deal or activity links to (company, contact, member), then the pipeline, then deals, then activities, several of them in ID order.

### Activities

An activity is a `note`, `call`, `meeting`, or `task` about a contact, a company, or a deal of the workspace. It links to at most one of each and to at least one, so a deal's, contact's, or company's timeline is a filtered list. Lists are newest first, by `created_at` and then `id`, which keeps paging stable; `q` matches the subject and body.

| Field | Limit |
| --- | --- |
| `kind` | Required on create; it never changes, so a `PATCH` that sends it gets a 400 |
| `subject` | Required; 200 characters |
| `body` | 10000 characters; empty by default |
| `due_at` | A task only. RFC 3339 with a zone, such as `2027-03-14T09:30:00Z`, from 1900 to 2200 in UTC |
| `assignee_id` | A task only. A current member of the workspace |
| `completed_at` | A task only, and only through `complete` and `reopen`; a body that sends it gets a 400 |
| `author_id` | The member who created the activity; a body that sends it gets a 400 |

Only a task has a due time, an assignee, and a completed state: a note, call, or meeting records something that already happened, at `created_at`. The schema holds the same rule. `status=open` lists tasks not yet completed and `status=completed` the completed ones; `due_before` is exclusive and `due_after` inclusive, and both match tasks only. A malformed filter gets a 400.

Every member reads and creates activities and completes or reopens tasks, because a task is shared work. Only the author, an owner, or an admin edits or deletes one, so nobody rewrites a colleague's notes; the role is re-read inside the transaction. Links and the assignee follow the rules for a deal's links: anything out of reach gets the same 404 with an identical body, and a write that links one holds a share lock on it. Invalid UTF-8 and NUL characters get a 400. A `PATCH` that sets `subject` or `body` to `null` gets a 400, `null` on `due_at`, `assignee_id`, or a link clears it, and clearing the last link gets a 400. On a create, `null` counts as left out.

Deleting a contact, company, or deal clears that link on its activities in the same transaction, moving `updated_at`. An activity left with no link at all is soft-deleted in the same statement, since nobody could reach it any more. Removing a member, or a member leaving, sets `author_id` and `assignee_id` to `null` where they were that member; the activities stay. Rejoining does not bring the roles back.

Passwords are 8 to 72 bytes, the limit of bcrypt.

### Refresh tokens

Each login starts a token family. Only SHA-256 hashes are stored. A refresh spends the presented token and returns a successor, so clients must store the new token every time.

Presenting a spent token revokes the whole family, which ends that login on every client holding it, and logs a `security: refresh_token_reuse` line with the family and user IDs. Other logins of the same user are unaffected. Logout revokes one family.

The exception is a retry within `REFRESH_GRACE` of the rotation, for a lost response or parallel refreshes. It receives another successor and spends nothing, so a token already delivered stays valid, and several tokens can be live for a moment. The next rotation retires all of them except its own successor, so a thief replaying a token inside the window cannot keep a second chain: whichever holder rotates second presents a spent token, which past the window revokes the family. A retry whose successor has already been spent gets a 401 and revokes nothing.

### Rate limit and proxies

Budgets are per client and refill continuously, and a spent one answers 429 with `Retry-After`. `/auth/register` and `/auth/login` share `AUTH_RATE_LIMIT_PER_MINUTE`, which protects bcrypt. `/auth/refresh`, `/auth/logout`, and `/auth/me` share the far larger `AUTH_SESSION_RATE_LIMIT_PER_MINUTE`, because a whole team can sit behind one address. `/workspaces` has its own bucket of that same size, so a busy screen cannot starve the refresh that keeps it signed in. Adding a member spends a further bucket the size of `AUTH_RATE_LIMIT_PER_MINUTE`, because its answer reveals whether an email has an account. A client is an IPv4 address or an IPv6 /64. Limits live in the memory of one server process, so several processes would each grant the full budget.

The limiter tracks at most 100,000 clients. Beyond that, a client falls into a fixed table of overflow buckets, chosen by a seeded hash of its IPv4 /24 or IPv6 /32. Filling the map from one block therefore drains only that block's bucket, not everyone's. A client in the attacker's block still shares its budget while the map is full, and an attacker spread over very many blocks can drain most of the table; a limit in front of the server is the answer to that.

### Password work and deadlines

bcrypt runs in at most as many slots as CPU cores, with a minimum of two. A request waits up to 3 seconds for a slot, then gets 503 with `Retry-After`. Unknown emails and wrong passwords wait for the same slot, so saturation does not tell them apart. Each database step of a request is limited to 5 seconds.

A NUL character in any JSON string gets a 400.

The client address is the connection's peer. Behind a reverse proxy, list it in `TRUSTED_PROXIES`, or every client shares the proxy's budget; `X-Forwarded-For` and `X-Forwarded-Proto` are read only from listed peers.

## Tenancy

A workspace is the tenant boundary. A user can belong to many, with one role in each:

| Action | `owner` | `admin` | `member` |
| --- | --- | --- | --- |
| Read the workspace and its members; leave | yes | yes | yes |
| Rename the workspace | yes | yes | no |
| Add, change, or remove `admin` and `member` | yes | yes | no |
| Make, change, or remove an `owner` | yes | no | no |
| Delete the workspace | yes | no | no |
| Change pipelines and stages | yes | yes | no |
| Read pipelines; read and write contacts, companies, and deals | yes | yes | yes |
| Read and create activities; complete and reopen tasks | yes | yes | yes |
| Edit or delete an activity someone else wrote | yes | yes | no |

A workspace always keeps at least one owner. Every change to a workspace or its members first takes that workspace's row lock and re-reads the actor's role, so concurrent changes cannot remove the last owner, and a role that changed after the middleware ran is not trusted.

The membership middleware (`requireWorkspace`) resolves the workspace from the route and places the workspace and role in the request context. Any failure ends the request. A non-member, a nonexistent workspace, a deleted workspace, and a malformed ID all get the same 404 with an identical body.

### Row-Level Security

Isolation has two layers. Store queries filter on `workspace_id` explicitly, and PostgreSQL policies enforce the same boundary if a filter is ever forgotten. The policies read two transaction-local settings. Unset or empty means NULL, which matches nothing, so they fail closed.

| Setting | Grants | Set by | Used for |
| --- | --- | --- | --- |
| `app.workspace_id` | Full access to one workspace | `Store.InWorkspace` | Everything inside a workspace, after the middleware has confirmed membership |
| `app.user_id` | Read-only: the caller's own membership rows, and the workspaces they belong to | The store, internally | The two lookups that happen before a workspace is known: the membership check and listing workspaces |

`app.user_id` solves the chicken-and-egg problem without `SECURITY DEFINER` functions: it cannot reveal fellow members or allow any write. The `workspace_members` policy compares a column and never queries its own table, so it cannot recurse. Creating a workspace generates the ID first and sets it as `app.workspace_id`, so both inserts pass the policies in one transaction. A fresh ID cannot name another tenant's workspace.

The migration owner is a superuser in Compose, and superusers always bypass Row-Level Security, so migrations are unaffected even with `FORCE`. A non-superuser owner would be bound by `FORCE` and need `BYPASSRLS` for data migrations. `009_pipelines.sql` backfills data, so it refuses to run without one or the other, rather than see no workspaces and backfill nothing.

Row-Level Security here defends against store bugs, not against SQL injection: code that can run arbitrary SQL as `crm_app` can also call `set_config`. It also does not enforce the soft delete, which is the store's and the middleware's job.

### Adding a workspace-scoped table

1. Give it `workspace_id UUID NOT NULL` and `updated_at TIMESTAMPTZ NOT NULL`. Use `deleted_at` for business data.
2. Enable and force Row-Level Security, with a `FOR ALL` policy with both `USING` and `WITH CHECK`:

   ```sql
   ALTER TABLE contacts ENABLE ROW LEVEL SECURITY;
   ALTER TABLE contacts FORCE ROW LEVEL SECURITY;
   CREATE POLICY contacts_tenant ON contacts FOR ALL
       USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
       WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);
   ```

3. Reference other tenant tables with a composite foreign key. Foreign key checks bypass Row-Level Security, so a plain `contact_id` would accept a contact from another workspace:

   ```sql
   CREATE TABLE contacts (
       id           UUID NOT NULL DEFAULT gen_random_uuid(),
       workspace_id UUID NOT NULL REFERENCES workspaces (id),
       -- ...
       PRIMARY KEY (id),
       UNIQUE (workspace_id, id)
   );
   CREATE TABLE deals (
       -- ...
       workspace_id UUID NOT NULL,
       contact_id   UUID NOT NULL,
       assigned_to  UUID NOT NULL,
       FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id),
       FOREIGN KEY (workspace_id, assigned_to) REFERENCES workspace_members (workspace_id, user_id)
   );
   ```

4. Run every query through `Store.InWorkspace`, and still filter on `workspace_id`. Take the ID from the workspace in the request context, never from a token claim or a request field:

   ```go
   func (s *Store) ListContacts(ctx context.Context, workspaceID string) ([]model.Contact, error) {
       var contacts []model.Contact
       err := s.InWorkspace(ctx, workspaceID, func(q Querier) error {
           rows, err := q.Query(ctx, `SELECT ... FROM contacts WHERE workspace_id = $1::text::uuid`, workspaceID)
           // ...
       })
       return contacts, err
   }
   ```

Any table with a `workspace_id` column is checked at server startup and by `TestTenantTablesAreProtected`: Row-Level Security enabled and forced, a non-nullable tenant column, `updated_at`, and a `FOR ALL` policy with `USING` and `WITH CHECK`, with no `USING (true)`. A table that fails stops the server. Never set `app.workspace_id` with session scope or outside a transaction: on a pooled connection it would carry over to the next request.

## Test

Database-backed tests connect the way the server does: `TEST_DATABASE_URL` is the app role and `TEST_MIGRATION_DATABASE_URL` the owner. They skip when both are unset and fail when only one is set. To run them, create the test database once and set both variables:

```sh
docker compose up -d --wait db
./scripts/bootstrap-db-roles.sh
docker compose exec db createdb -U postgres crm_test   # first time only
cd backend
TEST_DATABASE_URL='postgres://crm_app:crm_app_local_dev@localhost:5435/crm_test?sslmode=disable' \
TEST_MIGRATION_DATABASE_URL='postgres://postgres:postgres@localhost:5435/crm_test?sslmode=disable' \
go test -race ./...
```

Migration tests, and the store tests that sabotage the schema, run in throwaway schemas and drop them afterwards. Other database-backed tests migrate the `public` schema of `crm_test`. Isolation tests connect as `crm_app` and run raw SQL with no tenant filter, so they prove the policies independently of the store.

## Migrations

Add `backend/pkg/migrate/migrations/NNN_name.sql`. Pending files run in version order inside one transaction, so a failure rolls back the whole run, and statements that refuse to run in a transaction, such as `CREATE INDEX CONCURRENTLY`, are not supported. `001_init.sql` is an empty baseline; the runner itself creates `schema_migrations`.
