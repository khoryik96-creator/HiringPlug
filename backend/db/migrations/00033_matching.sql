-- matching module: how well a candidate fits a job (an application), as a
-- score with reasons quoting the job and the CV, read by the AI.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE job_match (
  id             text PRIMARY KEY DEFAULT new_id('mat') CHECK (id ~ '^mat_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id      text NOT NULL REFERENCES tenant (id),
  application_id text NOT NULL CHECK (length(application_id) <= 40),
  job_id         text NOT NULL CHECK (length(job_id) <= 40),
  status         text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'done', 'failed')),
  score          integer CHECK (score BETWEEN 0 AND 100),
  summary        text CHECK (length(summary) <= 1000),
  -- [{requirement, verdict, evidence, note}], checked against the texts sent
  reasons        jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(reasons) = 'array'),
  dropped        integer NOT NULL DEFAULT 0 CHECK (dropped >= 0),
  error          text CHECK (length(error) <= 1000),
  model          text CHECK (length(model) <= 200),
  input_tokens   integer NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
  output_tokens  integer NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
  -- What the AI was sent (already without names, contact details and
  -- salary), kept only until its answer is checked against it.
  sent_job       text,
  sent_cv        text,
  created_by     text NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  scored_at      timestamptz,
  CHECK ((status = 'done') = (score IS NOT NULL)),
  CHECK (status = 'queued' OR (sent_job IS NULL AND sent_cv IS NULL))
);
CREATE INDEX job_match_application_idx ON job_match (tenant_id, application_id, created_at DESC);
CREATE INDEX job_match_job_idx ON job_match (tenant_id, job_id, created_at DESC);
CREATE INDEX job_match_month_idx ON job_match (tenant_id, created_at);

-- Every table holding tenant data has row-level security (a test checks this).
ALTER TABLE job_match ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON job_match
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS job_match;
