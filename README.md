# CRM

A team CRM. This repository currently holds the backend foundation: configuration, migrations, database roles, a health route, and accounts with token auth. The plan lives in `odd/tasks/crm-foundation.md`.

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
| `AUTH_SESSION_RATE_LIMIT_PER_MINUTE` | `600` | The server's built-in default; per client, on every other limited route; must be positive |
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

At startup the server reads the role behind `DATABASE_URL` and refuses to boot if it is a superuser, has `BYPASSRLS`, or owns a table. The check runs before migrations, so a refused server changes nothing.

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

Passwords are 8 to 72 bytes, the limit of bcrypt.

### Refresh tokens

Each login starts a token family. Only SHA-256 hashes are stored. A refresh spends the presented token and returns a successor, so clients must store the new token every time.

Presenting a spent token revokes the whole family, which ends that login on every client holding it, and logs a `security: refresh_token_reuse` line with the family and user IDs. Other logins of the same user are unaffected. Logout revokes one family.

The exception is a retry within `REFRESH_GRACE` of the rotation, for a lost response or parallel refreshes. It receives another successor and spends nothing, so a token already delivered stays valid, and several tokens can be live for a moment. The next rotation retires all of them except its own successor, so a thief replaying a token inside the window cannot keep a second chain: whichever holder rotates second presents a spent token, which past the window revokes the family. A retry whose successor has already been spent gets a 401 and revokes nothing.

### Rate limit and proxies

Two budgets per client, refilled continuously, answer 429 with `Retry-After` when spent. `/auth/register` and `/auth/login` share `AUTH_RATE_LIMIT_PER_MINUTE`, which protects bcrypt. `/auth/refresh`, `/auth/logout`, and `/auth/me` share the far larger `AUTH_SESSION_RATE_LIMIT_PER_MINUTE`, because a whole team can sit behind one address. A client is an IPv4 address or an IPv6 /64. Limits live in the memory of one server process, so several processes would each grant the full budget.

The limiter tracks at most 100,000 clients. Beyond that, a client falls into a fixed table of overflow buckets, chosen by a seeded hash of its IPv4 /24 or IPv6 /32. Filling the map from one block therefore drains only that block's bucket, not everyone's. A client in the attacker's block still shares its budget while the map is full, and an attacker spread over very many blocks can drain most of the table; a limit in front of the server is the answer to that.

### Password work and deadlines

bcrypt runs in at most as many slots as CPU cores, with a minimum of two. A request waits up to 3 seconds for a slot, then gets 503 with `Retry-After`. Unknown emails and wrong passwords wait for the same slot, so saturation does not tell them apart. Each database step of a request is limited to 5 seconds.

A NUL character in any JSON string gets a 400.

The client address is the connection's peer. Behind a reverse proxy, list it in `TRUSTED_PROXIES`, or every client shares the proxy's budget; `X-Forwarded-For` and `X-Forwarded-Proto` are read only from listed peers.

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

Migration tests run in throwaway schemas and drop them afterwards. Other database-backed tests migrate the `public` schema of `crm_test`.

## Migrations

Add `backend/pkg/migrate/migrations/NNN_name.sql`. Pending files run in version order inside one transaction, so a failure rolls back the whole run, and statements that refuse to run in a transaction, such as `CREATE INDEX CONCURRENTLY`, are not supported. `001_init.sql` is an empty baseline; the runner itself creates `schema_migrations`.
