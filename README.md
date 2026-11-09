# CRM

A team CRM. This repository currently holds the backend skeleton: configuration, migrations, and a health route. The plan lives in `odd/tasks/crm-foundation.md`.

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

## Run

```sh
./dev.sh
curl -si localhost:8085/health
```

`dev.sh` starts the Compose database, waits until it is healthy, then builds and runs the API. The server applies pending migrations on every boot. `GET /health` returns 200 when the database is reachable and 503 when it is not.

## Configuration

Settings come from the environment. `dev.sh` also reads an optional `.env` in the repository root, and Compose reads `POSTGRES_PASSWORD` from the same file. Never commit `.env`.

| Variable | Default in `dev.sh` | Notes |
| --- | --- | --- |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5435/crm?sslmode=disable` | Required by the server |
| `PORT` | `8085` | |
| `MIGRATION_TIMEOUT` | `5m` | Deadline for the migration lock wait and all pending migrations; the server's built-in default |
| `POSTGRES_PASSWORD` | `postgres` | Local only; applied when the volume is first created |
| `TEST_DATABASE_URL` | none | Enables database-backed tests |

Compose publishes Postgres on `127.0.0.1:5435` only.

## Test

Database-backed tests skip when `TEST_DATABASE_URL` is unset. To run them, create the test database once and point the variable at it:

```sh
docker compose up -d --wait db
docker compose exec db createdb -U postgres crm_test
cd backend
TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5435/crm_test?sslmode=disable' go test ./...
```

Migration tests run in throwaway schemas and drop them afterwards.

## Migrations

Add `backend/pkg/migrate/migrations/NNN_name.sql`. Pending files run in version order inside one transaction, so a failure rolls back the whole run, and statements that refuse to run in a transaction, such as `CREATE INDEX CONCURRENTLY`, are not supported. `001_init.sql` is an empty baseline; the runner itself creates `schema_migrations`.
