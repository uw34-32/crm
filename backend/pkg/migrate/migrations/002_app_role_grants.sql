-- The server connects as crm_app, which must stay NOSUPERUSER NOBYPASSRLS and
-- own no tables, so Row-Level Security applies to it. Migrations run as the
-- owner; these default privileges give crm_app data access to every table the
-- owner creates from now on. A table that needs less must REVOKE explicitly.
--
-- ALTER DEFAULT PRIVILEGES applies to objects created by the role running it,
-- so every migration must keep running as the same owner.
--
-- The role itself is created outside migrations because it needs a password:
-- scripts/bootstrap-db-roles.sh does so in local development.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'crm_app') THEN
        RAISE EXCEPTION 'role crm_app does not exist; create it before migrating (local development: scripts/bootstrap-db-roles.sh)';
    END IF;

    EXECUTE format('GRANT USAGE ON SCHEMA %I TO crm_app', current_schema());
    EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO crm_app', current_schema());
    EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT USAGE, SELECT ON SEQUENCES TO crm_app', current_schema());
END
$$;
