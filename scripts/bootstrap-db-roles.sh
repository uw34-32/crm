#!/bin/bash
# Creates or updates the crm_app database role in the local Compose database.
# Safe to run repeatedly, and works on a volume that already exists, where
# Compose init scripts would not run again. Local development only: production
# roles and passwords belong to whoever operates that database.
set -euo pipefail

cd "$(dirname "$0")/.."

if [ -f .env ]; then
    set -a
    . ./.env
    set +a
fi

# The password travels through the environment rather than the command line, so
# it does not appear in process listings. It must stay URL-safe, because dev.sh
# puts it in DATABASE_URL.
export APP_DB_PASSWORD="${APP_DB_PASSWORD:-crm_app_local_dev}"

docker compose exec -T -e APP_DB_PASSWORD db \
    psql --quiet -v ON_ERROR_STOP=1 -U postgres -d postgres <<'SQL'
\getenv app_password APP_DB_PASSWORD
SELECT 'CREATE ROLE crm_app' WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'crm_app')
\gexec
ALTER ROLE crm_app LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD :'app_password';
SQL
