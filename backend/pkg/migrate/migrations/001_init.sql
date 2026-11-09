-- Baseline. Intentionally creates nothing: the runner owns schema_migrations,
-- and business tables arrive with the tasks that use them, starting at 002.
-- go:embed needs at least one file to match, and a statement keeps this a
-- non-empty query.
SELECT 1;
