-- jobs: job ads, the advertised text of a job (separate from the internal job order).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE job_ad (
  id               text PRIMARY KEY DEFAULT new_id('ad') CHECK (id ~ '^ad_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id        text NOT NULL REFERENCES tenant (id),
  job_order_id     text NOT NULL,
  version          integer NOT NULL DEFAULT 1,
  title            text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
  location         text CHECK (length(location) <= 200),
  work_type        text CHECK (work_type IN ('full_time', 'part_time', 'contract', 'temporary', 'internship')),
  work_arrangement text CHECK (work_arrangement IN ('onsite', 'hybrid', 'remote')),
  -- Published as written ("RM 8,000 - 10,000 a month"); the job's budget stays private.
  salary_text      text CHECK (length(salary_text) <= 100),
  description      text CHECK (length(description) <= 20000),
  closes_on        date,
  status           text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'closed')),
  published_at     timestamptz,
  created_by       text NOT NULL,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  CHECK (status <> 'published' OR (published_at IS NOT NULL AND length(btrim(coalesce(description, ''))) > 0)),
  FOREIGN KEY (tenant_id, job_order_id) REFERENCES job_order (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX job_ad_job_idx ON job_ad (tenant_id, job_order_id, id DESC);

ALTER TABLE job_ad ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON job_ad
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS job_ad;
