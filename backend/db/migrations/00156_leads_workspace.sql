-- +goose Up
CREATE TABLE job_lead_favorite (
  tenant_id text NOT NULL REFERENCES tenant (id),
  user_id text NOT NULL,
  lead_id text NOT NULL REFERENCES job_lead (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id, lead_id)
);
ALTER TABLE job_lead_favorite ENABLE ROW LEVEL SECURITY;
CREATE POLICY own_favorites ON job_lead_favorite
  USING (tenant_id = app_current_tenant() AND user_id = app_current_user() AND app_has_permission('leads:read'))
  WITH CHECK (tenant_id = app_current_tenant() AND user_id = app_current_user() AND app_has_permission('leads:read'));

-- Keep full ads off the list's hot path.
CREATE TABLE job_lead_evidence (
  tenant_id text NOT NULL REFERENCES tenant (id),
  lead_id text PRIMARY KEY REFERENCES job_lead (id) ON DELETE CASCADE,
  description text NOT NULL CHECK (length(description) BETWEEN 1 AND 16000),
  observed_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE job_lead_evidence ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON job_lead_evidence
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

CREATE INDEX job_lead_company_search_idx ON job_lead USING gin (company_name gin_trgm_ops);
CREATE INDEX job_lead_title_search_idx ON job_lead USING gin (title gin_trgm_ops);
CREATE INDEX job_lead_discovery_title_idx ON job_lead USING gin (to_tsvector('simple', title))
  WHERE closed_at IS NULL AND source_id IS NOT NULL AND url IS NOT NULL;

-- +goose Down
DROP INDEX job_lead_discovery_title_idx;
DROP INDEX job_lead_title_search_idx;
DROP INDEX job_lead_company_search_idx;
DROP TABLE job_lead_evidence;
DROP TABLE job_lead_favorite;
