# CRM

A team CRM. This repository currently holds the backend foundation: configuration, migrations, database roles, a health route, accounts with token auth, and workspaces with members and Row-Level Security. The plan lives in `odd/tasks/crm-foundation.md`.

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

Workspace routes need `Authorization: Bearer <token>`. Roles are `owner`, `admin`, and `member`.

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

A workspace always keeps at least one owner. Every change to a workspace or its members first takes that workspace's row lock and re-reads the actor's role, so concurrent changes cannot remove the last owner, and a role that changed after the middleware ran is not trusted.

The membership middleware (`requireWorkspace`) resolves the workspace from the route and places the workspace and role in the request context. Any failure ends the request. A non-member, a nonexistent workspace, a deleted workspace, and a malformed ID all get the same 404 with an identical body.

### Row-Level Security

Isolation has two layers. Store queries filter on `workspace_id` explicitly, and PostgreSQL policies enforce the same boundary if a filter is ever forgotten. The policies read two transaction-local settings. Unset or empty means NULL, which matches nothing, so they fail closed.

| Setting | Grants | Set by | Used for |
| --- | --- | --- | --- |
| `app.workspace_id` | Full access to one workspace | `Store.InWorkspace` | Everything inside a workspace, after the middleware has confirmed membership |
| `app.user_id` | Read-only: the caller's own membership rows, and the workspaces they belong to | The store, internally | The two lookups that happen before a workspace is known: the membership check and listing workspaces |

`app.user_id` solves the chicken-and-egg problem without `SECURITY DEFINER` functions: it cannot reveal fellow members or allow any write. The `workspace_members` policy compares a column and never queries its own table, so it cannot recurse. Creating a workspace generates the ID first and sets it as `app.workspace_id`, so both inserts pass the policies in one transaction. A fresh ID cannot name another tenant's workspace.

The migration owner is a superuser in Compose, and superusers always bypass Row-Level Security, so migrations are unaffected even with `FORCE`. A non-superuser owner would be bound by `FORCE` and need `BYPASSRLS` for data migrations.

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

Migration tests run in throwaway schemas and drop them afterwards. Other database-backed tests migrate the `public` schema of `crm_test`. Isolation tests connect as `crm_app` and run raw SQL with no tenant filter, so they prove the policies independently of the store.

## Migrations

Add `backend/pkg/migrate/migrations/NNN_name.sql`. Pending files run in version order inside one transaction, so a failure rolls back the whole run, and statements that refuse to run in a transaction, such as `CREATE INDEX CONCURRENTLY`, are not supported. `001_init.sql` is an empty baseline; the runner itself creates `schema_migrations`.
