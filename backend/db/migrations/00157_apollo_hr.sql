-- +goose Up
ALTER TABLE hr_scan ADD COLUMN linkedin_company_url text;

CREATE TABLE hr_apollo_connection (
  tenant_id text NOT NULL DEFAULT app_current_tenant(),
  user_id text NOT NULL DEFAULT app_current_user(),
  flow_id text NOT NULL CHECK (flow_id ~ '^[a-f0-9]{64}$'),
  broker_origin text NOT NULL,
  proof_sealed text NOT NULL,
  authorize_url text NOT NULL,
  account_name text,
  expires_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, user_id),
  FOREIGN KEY (tenant_id, user_id) REFERENCES app_user (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE hr_apollo_lookup (
  tenant_id text NOT NULL DEFAULT app_current_tenant(),
  user_id text NOT NULL DEFAULT app_current_user(),
  id text NOT NULL CHECK (id ~ '^[a-f0-9]{64}$'),
  flow_id text NOT NULL,
  company_id text NOT NULL,
  linkedin_company_id text NOT NULL,
  profile_url text NOT NULL,
  person_name text NOT NULL,
  linkedin_company_name text NOT NULL,
  linkedin_company_url text NOT NULL,
  state text NOT NULL CHECK (state IN ('queued','sending','found','not_found','mismatch','pending','unconfirmed','auth_required','limited','cancelled')),
  email_sealed text,
  email_status text,
  reason text,
  updated_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, user_id, linkedin_company_id, profile_url),
  FOREIGN KEY (tenant_id, user_id) REFERENCES app_user (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, company_id) REFERENCES company (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX hr_apollo_lookup_active_idx
  ON hr_apollo_lookup (tenant_id, user_id, flow_id, state, updated_at)
  WHERE state IN ('queued','sending');

ALTER TABLE hr_apollo_connection ENABLE ROW LEVEL SECURITY;
ALTER TABLE hr_apollo_lookup ENABLE ROW LEVEL SECURITY;
CREATE POLICY own_apollo_connection ON hr_apollo_connection USING (
  tenant_id = app_current_tenant() AND (app_current_user() = 'system' OR (user_id = app_current_user() AND app_has_permission('contacts.apollo:use')))
) WITH CHECK (tenant_id = app_current_tenant() AND user_id = app_current_user() AND app_has_permission('contacts.apollo:use'));
CREATE POLICY own_apollo_lookup ON hr_apollo_lookup USING (
  tenant_id = app_current_tenant() AND (app_current_user() = 'system' OR (user_id = app_current_user() AND app_has_permission('contacts.apollo:use')))
) WITH CHECK (tenant_id = app_current_tenant() AND (app_current_user() = 'system' OR (user_id = app_current_user() AND app_has_permission('contacts.apollo:use'))));

-- +goose Down
DROP TABLE hr_apollo_lookup;
DROP TABLE hr_apollo_connection;
ALTER TABLE hr_scan DROP COLUMN linkedin_company_url;
