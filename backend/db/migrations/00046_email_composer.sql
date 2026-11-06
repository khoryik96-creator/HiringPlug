-- emails: formatted messages with a plain-text version, bcc, attachments
-- (the candidate's CVs or uploaded files) and image banners; an email that
-- shows salary is hidden from roles that may not see salaries (unless the
-- company chose otherwise: settings' tenant_policy).
-- templates: a formatted message (with its signature and banner); body keeps
--   its plain-text version.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE email
  ADD COLUMN body_html text CHECK (length(body_html) <= 200000),
  ADD COLUMN bcc_addresses text[] NOT NULL DEFAULT '{}' CHECK (cardinality(bcc_addresses) <= 10),
  ADD COLUMN salary boolean NOT NULL DEFAULT false,
  ADD COLUMN salary_restricted boolean NOT NULL DEFAULT false;
CREATE POLICY salary_scope ON email AS RESTRICTIVE FOR SELECT
  USING (NOT salary_restricted OR app_has_permission('candidates.compensation:read'));

ALTER TABLE email_template ADD COLUMN body_html text CHECK (length(body_html) <= 100000);

CREATE TABLE email_banner (
  id           text PRIMARY KEY DEFAULT new_id('ebn') CHECK (id ~ '^ebn_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id    text NOT NULL REFERENCES tenant (id),
  name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
  content_type text NOT NULL CHECK (content_type IN ('image/png', 'image/jpeg', 'image/gif')),
  size_bytes   integer NOT NULL CHECK (size_bytes BETWEEN 1 AND 1048576),
  storage_key  text NOT NULL,
  created_by   text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz -- kept for emails already queued with it
);
CREATE INDEX email_banner_tenant_idx ON email_banner (tenant_id, created_at);

CREATE TABLE email_attachment (
  id           text PRIMARY KEY DEFAULT new_id('eat') CHECK (id ~ '^eat_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id    text NOT NULL REFERENCES tenant (id),
  email_id     text REFERENCES email (id) ON DELETE CASCADE, -- null while uploaded and not yet sent
  file_name    text NOT NULL CHECK (length(file_name) BETWEEN 1 AND 255),
  content_type text NOT NULL CHECK (length(content_type) BETWEEN 1 AND 255),
  size_bytes   integer NOT NULL CHECK (size_bytes BETWEEN 1 AND 10485760),
  storage_key  text NOT NULL,
  created_by   text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX email_attachment_email_idx ON email_attachment (tenant_id, email_id);

CREATE TABLE tenant_policy (
  tenant_id text NOT NULL REFERENCES tenant (id),
  key       text NOT NULL CHECK (key IN ('email_salary_visibility')),
  value     text NOT NULL CHECK (length(value) BETWEEN 1 AND 100),
  PRIMARY KEY (tenant_id, key)
);

ALTER TABLE email_banner ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON email_banner
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE email_attachment ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON email_attachment
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
-- An attachment goes with its email: hidden when the email is.
CREATE POLICY email_scope ON email_attachment AS RESTRICTIVE FOR SELECT
  USING (email_id IS NULL OR EXISTS (SELECT 1 FROM email e WHERE e.id = email_id));
ALTER TABLE tenant_policy ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant_policy
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS tenant_policy;
DROP TABLE IF EXISTS email_attachment;
DROP TABLE IF EXISTS email_banner;
ALTER TABLE email_template DROP COLUMN IF EXISTS body_html;
DROP POLICY IF EXISTS salary_scope ON email;
ALTER TABLE email DROP COLUMN IF EXISTS salary_restricted, DROP COLUMN IF EXISTS salary,
  DROP COLUMN IF EXISTS bcc_addresses, DROP COLUMN IF EXISTS body_html;
