-- candidates module: candidate profiles, and their salary details in a
-- separate table that the database itself only shows to permitted users.

-- +goose Up
CREATE TABLE candidate (
  id                        text PRIMARY KEY DEFAULT new_id('cand') CHECK (id ~ '^cand_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id                 text NOT NULL REFERENCES tenant (id),
  version                   integer NOT NULL DEFAULT 1 CHECK (version >= 1),
  first_name                text NOT NULL CHECK (length(first_name) BETWEEN 1 AND 100),
  last_name                 text NOT NULL DEFAULT '' CHECK (length(last_name) <= 100),
  email                     text CHECK (email = lower(email) AND length(email) <= 254),
  phone                     text CHECK (phone ~ '^\+[1-9][0-9]{6,14}$'),
  nationality               text CHECK (nationality ~ '^[A-Z]{2}$'),
  work_authorization        text CHECK (work_authorization IN ('local', 'expat_eligible', 'requires_visa')),
  work_authorization_basis  text CHECK (work_authorization_basis IN ('rp_t', 'permanent_resident', 'spouse_visa', 'other')),
  work_authorization_note   text CHECK (length(work_authorization_note) <= 500),
  notice_period_value       integer CHECK (notice_period_value BETWEEN 0 AND 365),
  notice_period_unit        text CHECK (notice_period_unit IN ('day', 'week', 'month')),
  industry                  text CHECK (length(industry) <= 100),
  specialization            text CHECK (length(specialization) <= 100),
  current_title             text CHECK (length(current_title) <= 200),
  current_company           text CHECK (length(current_company) <= 200),
  location                  text CHECK (length(location) <= 200),
  source                    text CHECK (source IN ('jobstreet', 'linkedin', 'referral', 'careers_page', 'direct', 'other')),
  -- Name + email, lower-cased and without accents, for search.
  search_text               text GENERATED ALWAYS AS (
                              app_normalize_text(first_name || ' ' || last_name || ' ' || coalesce(email, ''))
                            ) STORED,
  created_by                text NOT NULL,
  created_at                timestamptz NOT NULL DEFAULT now(),
  updated_at                timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  -- The same rules the Go domain code applies, enforced again by the database.
  CONSTRAINT candidate_basis_only_for_expat_eligible CHECK (
    (work_authorization = 'expat_eligible') IS NOT DISTINCT FROM (work_authorization_basis IS NOT NULL)
    OR (work_authorization IS NULL AND work_authorization_basis IS NULL)
  ),
  CONSTRAINT candidate_note_needs_status CHECK (work_authorization_note IS NULL OR work_authorization IS NOT NULL),
  CONSTRAINT candidate_notice_period_complete CHECK ((notice_period_value IS NULL) = (notice_period_unit IS NULL))
);

-- One candidate per email address within a tenant.
CREATE UNIQUE INDEX candidate_tenant_email_uq ON candidate (tenant_id, email) WHERE email IS NOT NULL;
CREATE INDEX candidate_tenant_id_idx ON candidate (tenant_id, id DESC);
CREATE INDEX candidate_search_trgm_idx ON candidate USING gin (search_text gin_trgm_ops);

ALTER TABLE candidate ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON candidate
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- Salary details. Kept out of the candidate table so that the database can
-- refuse to return them: even a buggy query cannot read a row unless the
-- transaction carries candidates.compensation:read.
CREATE TABLE candidate_compensation (
  candidate_id       text PRIMARY KEY,
  tenant_id          text NOT NULL,
  version            integer NOT NULL DEFAULT 1 CHECK (version >= 1),
  current_amount     numeric(14, 2) CHECK (current_amount >= 0),
  current_currency   text CHECK (current_currency ~ '^[A-Z]{3}$'),
  current_period     text CHECK (current_period IN ('hour', 'day', 'month', 'year')),
  expected_min       numeric(14, 2) CHECK (expected_min >= 0),
  expected_max       numeric(14, 2) CHECK (expected_max >= 0),
  expected_currency  text CHECK (expected_currency ~ '^[A-Z]{3}$'),
  expected_period    text CHECK (expected_period IN ('hour', 'day', 'month', 'year')),
  updated_by         text NOT NULL,
  updated_at         timestamptz NOT NULL DEFAULT now(),
  -- Same tenant as the candidate, guaranteed by the database.
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT compensation_current_complete CHECK (
    (current_amount IS NULL) = (current_currency IS NULL) AND (current_amount IS NULL) = (current_period IS NULL)
  ),
  CONSTRAINT compensation_expected_complete CHECK (
    (expected_min IS NULL) = (expected_currency IS NULL) AND (expected_min IS NULL) = (expected_period IS NULL)
    AND (expected_max IS NULL OR expected_min IS NOT NULL)
  ),
  CONSTRAINT compensation_expected_range CHECK (expected_max IS NULL OR expected_max >= expected_min)
);

ALTER TABLE candidate_compensation ENABLE ROW LEVEL SECURITY;
CREATE POLICY compensation_select ON candidate_compensation FOR SELECT
  USING (tenant_id = app_current_tenant() AND app_has_permission('candidates.compensation:read'));
CREATE POLICY compensation_insert ON candidate_compensation FOR INSERT
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('candidates.compensation:update'));
CREATE POLICY compensation_update ON candidate_compensation FOR UPDATE
  USING (tenant_id = app_current_tenant() AND app_has_permission('candidates.compensation:update'))
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('candidates.compensation:update'));
CREATE POLICY compensation_delete ON candidate_compensation FOR DELETE
  USING (tenant_id = app_current_tenant() AND app_has_permission('candidates.compensation:update'));

-- +goose Down
DROP TABLE IF EXISTS candidate_compensation;
DROP TABLE IF EXISTS candidate;
