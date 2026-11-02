-- contacts module: people at companies, with the result of their email check.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE contact (
  id                  text PRIMARY KEY DEFAULT new_id('cont') CHECK (id ~ '^cont_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id           text NOT NULL REFERENCES tenant (id),
  version             integer NOT NULL DEFAULT 1,
  -- The company belongs to the same tenant (composite key).
  company_id          text NOT NULL,
  first_name          text NOT NULL CHECK (length(btrim(first_name)) BETWEEN 1 AND 100),
  last_name           text NOT NULL DEFAULT '' CHECK (length(last_name) <= 100),
  title               text CHECK (length(title) <= 200),
  email               text CHECK (email = lower(email) AND length(email) <= 254),
  email_check_status  text CHECK (email_check_status IN ('valid', 'risky', 'invalid', 'unknown')),
  email_check_reasons text[] NOT NULL DEFAULT '{}',
  email_checked_at    timestamptz,
  phone               text CHECK (length(phone) <= 40),
  mobile              text CHECK (length(mobile) <= 40),
  linkedin_url        text CHECK (linkedin_url ~ '^https://' AND length(linkedin_url) <= 300),
  status              text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'left')),
  notes               text CHECK (length(notes) <= 4000),
  search_text         text GENERATED ALWAYS AS (
                        app_normalize_text(first_name || ' ' || last_name || ' ' || coalesce(email, '') || ' ' || coalesce(title, ''))
                      ) STORED,
  created_by          text NOT NULL,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, company_id) REFERENCES company (tenant_id, id),
  -- A check result exists exactly when there is an email.
  CONSTRAINT contact_email_check_complete CHECK (
    (email IS NULL) = (email_check_status IS NULL) AND (email_check_status IS NULL) = (email_checked_at IS NULL)
  )
);
-- One contact per email address within a tenant.
CREATE UNIQUE INDEX contact_tenant_email_uq ON contact (tenant_id, email) WHERE email IS NOT NULL;
CREATE INDEX contact_tenant_id_idx ON contact (tenant_id, id DESC);
CREATE INDEX contact_company_idx ON contact (tenant_id, company_id, id DESC);
CREATE INDEX contact_search_trgm_idx ON contact USING gin (search_text gin_trgm_ops);

-- Every table holding tenant data has row-level security (a test checks this).
ALTER TABLE contact ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON contact
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS contact;
