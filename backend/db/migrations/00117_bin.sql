-- bin module: candidates, contacts and companies an admin deleted wait in the
-- Bin for 30 days (they can be restored), then are deleted for good. A binned
-- row is hidden by the database itself (a restrictive policy), so no list,
-- search or count anywhere shows it; only a transaction that turns on
-- app.show_binned (the bin module's own) sees it. A company's jobs go with
-- it unless moved, and the applications of a binned candidate or job are
-- hidden with them (application.binned, kept by the pipeline module).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE FUNCTION app_show_binned() RETURNS boolean
  LANGUAGE sql STABLE AS $$ SELECT coalesce(current_setting('app.show_binned', true), '') = 'on' $$;

ALTER TABLE candidate ADD COLUMN binned_at timestamptz;
ALTER TABLE contact ADD COLUMN binned_at timestamptz;
ALTER TABLE company ADD COLUMN binned_at timestamptz;
ALTER TABLE job_order ADD COLUMN binned_at timestamptz;
ALTER TABLE application ADD COLUMN binned boolean NOT NULL DEFAULT false;

CREATE POLICY bin_hidden ON candidate AS RESTRICTIVE FOR SELECT USING (binned_at IS NULL OR app_show_binned());
CREATE POLICY bin_hidden ON contact AS RESTRICTIVE FOR SELECT USING (binned_at IS NULL OR app_show_binned());
CREATE POLICY bin_hidden ON company AS RESTRICTIVE FOR SELECT USING (binned_at IS NULL OR app_show_binned());
CREATE POLICY bin_hidden ON job_order AS RESTRICTIVE FOR SELECT USING (binned_at IS NULL OR app_show_binned());
CREATE POLICY bin_hidden ON application AS RESTRICTIVE FOR SELECT USING (NOT binned OR app_show_binned());

-- A record in the Bin no longer holds its email or name: a new one may use
-- it (restoring then says which record has it now).
DROP INDEX candidate_tenant_email_uq;
CREATE UNIQUE INDEX candidate_tenant_email_uq ON candidate (tenant_id, email) WHERE email IS NOT NULL AND binned_at IS NULL;
DROP INDEX contact_tenant_email_uq;
CREATE UNIQUE INDEX contact_tenant_email_uq ON contact (tenant_id, email) WHERE email IS NOT NULL AND binned_at IS NULL;
DROP INDEX company_tenant_name_uq;
CREATE UNIQUE INDEX company_tenant_name_uq ON company (tenant_id, name_key) WHERE binned_at IS NULL;

CREATE TABLE bin_entry (
  id          text PRIMARY KEY DEFAULT new_id('bin') CHECK (id ~ '^bin_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id   text NOT NULL REFERENCES tenant (id),
  kind        text NOT NULL CHECK (kind IN ('candidate', 'contact', 'company')),
  record_id   text NOT NULL CHECK (length(record_id) <= 40),
  label       text NOT NULL CHECK (length(label) <= 300),
  -- What went with it: a company's contacts and jobs ({"contacts": [...], "jobs": [...]}).
  went_with   jsonb NOT NULL DEFAULT '{}'::jsonb,
  binned_by   text NOT NULL,
  binned_name text NOT NULL,
  binned_at   timestamptz NOT NULL DEFAULT now(),
  purge_after timestamptz NOT NULL,
  UNIQUE (tenant_id, id)
);
CREATE INDEX bin_entry_tenant_idx ON bin_entry (tenant_id, binned_at DESC);
CREATE INDEX bin_entry_record_idx ON bin_entry (tenant_id, record_id);
ALTER TABLE bin_entry ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON bin_entry USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS bin_entry;
DROP INDEX IF EXISTS company_tenant_name_uq;
CREATE UNIQUE INDEX company_tenant_name_uq ON company (tenant_id, name_key);
DROP INDEX IF EXISTS contact_tenant_email_uq;
CREATE UNIQUE INDEX contact_tenant_email_uq ON contact (tenant_id, email) WHERE email IS NOT NULL;
DROP INDEX IF EXISTS candidate_tenant_email_uq;
CREATE UNIQUE INDEX candidate_tenant_email_uq ON candidate (tenant_id, email) WHERE email IS NOT NULL;
DROP POLICY IF EXISTS bin_hidden ON application;
DROP POLICY IF EXISTS bin_hidden ON job_order;
DROP POLICY IF EXISTS bin_hidden ON company;
DROP POLICY IF EXISTS bin_hidden ON contact;
DROP POLICY IF EXISTS bin_hidden ON candidate;
ALTER TABLE application DROP COLUMN IF EXISTS binned;
ALTER TABLE job_order DROP COLUMN IF EXISTS binned_at;
ALTER TABLE company DROP COLUMN IF EXISTS binned_at;
ALTER TABLE contact DROP COLUMN IF EXISTS binned_at;
ALTER TABLE candidate DROP COLUMN IF EXISTS binned_at;
DROP FUNCTION IF EXISTS app_show_binned();
