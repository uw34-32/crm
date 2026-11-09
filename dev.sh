#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"

# Optional local overrides.
if [ -f .env ]; then
    set -a
    . ./.env
    set +a
fi

docker compose up -d --wait db
./scripts/bootstrap-db-roles.sh

# The server queries as the unprivileged app role; only migrations use the owner.
export DATABASE_URL="${DATABASE_URL:-postgres://crm_app:${APP_DB_PASSWORD:-crm_app_local_dev}@localhost:5435/crm?sslmode=disable}"
export MIGRATION_DATABASE_URL="${MIGRATION_DATABASE_URL:-postgres://postgres:${POSTGRES_PASSWORD:-postgres}@localhost:5435/crm?sslmode=disable}"
export PORT="${PORT:-8085}"

# Build and exec rather than `go run`, so SIGINT and SIGTERM reach the server
# directly and trigger its graceful shutdown.
cd backend
go build -o bin/server ./cmd/server
exec ./bin/server
