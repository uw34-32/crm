# CRM

A team CRM. This repository currently holds the backend skeleton: configuration, migrations, database roles, and a health route. The plan lives in `odd/tasks/crm-foundation.md`.

## Stack

Go, Chi v5, pgx v5, and PostgreSQL 16 through Docker Compose.

## Layout

| Path | Purpose |
| --- | --- |
| `backend/cmd/server` | Entry point; handles SIGINT and SIGTERM |
| `backend/pkg/config` | Settings from the environment |
| `backend/pkg/migrate` | Embedded SQL migrations and their runner |
| `backend/pkg/store` | Database access |
| `backend/pkg/app` | Routes and handlers |
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
