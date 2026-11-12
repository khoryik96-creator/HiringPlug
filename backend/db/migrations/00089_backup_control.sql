-- Backups from Settings: the nightly backup's switch, a backup asked for with
-- "Back up now", and when the backup service last checked in. One row for
-- the whole server (not company data, like backup_run). The API reads it and
-- changes the switch and the request; the backup service (the admin role)
-- does the rest.
-- app_tenant_count(): how many agencies the server holds, so only the agency
-- that runs a one-agency server sees its backups without further setting.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE backup_control (
  id              boolean PRIMARY KEY DEFAULT true CHECK (id),
  auto            boolean NOT NULL DEFAULT true,
  requested_at    timestamptz,
  service_seen_at timestamptz,
  updated_at      timestamptz NOT NULL DEFAULT now()
);
INSERT INTO backup_control DEFAULT VALUES;
REVOKE INSERT, DELETE ON backup_control FROM hiringplug_app;

-- +goose StatementBegin
CREATE FUNCTION app_tenant_count() RETURNS bigint
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT count(*) FROM tenant
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION app_tenant_count() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_tenant_count() TO hiringplug_app;

-- +goose Down
DROP FUNCTION IF EXISTS app_tenant_count();
DROP TABLE IF EXISTS backup_control;
