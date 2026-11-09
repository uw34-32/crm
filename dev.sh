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

export DATABASE_URL="${DATABASE_URL:-postgres://postgres:${POSTGRES_PASSWORD:-postgres}@localhost:5435/crm?sslmode=disable}"
export PORT="${PORT:-8085}"

# Build and exec rather than `go run`, so SIGINT and SIGTERM reach the server
# directly and trigger its graceful shutdown.
cd backend
go build -o bin/server ./cmd/server
exec ./bin/server
