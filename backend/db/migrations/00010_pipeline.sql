-- pipeline module: a candidate on a job (an application) and its stage.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE application (
  id               text PRIMARY KEY DEFAULT new_id('appl') CHECK (id ~ '^appl_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id        text NOT NULL REFERENCES tenant (id),
  version          integer NOT NULL DEFAULT 1,
  job_order_id     text NOT NULL,
  candidate_id     text NOT NULL,
  stage            text NOT NULL DEFAULT 'applied'
                   CHECK (stage IN ('applied', 'screening', 'submitted', 'interview', 'offer', 'placed', 'rejected', 'withdrawn')),
  reason           text CHECK (reason IN ('not_qualified', 'salary_mismatch', 'client_rejected', 'failed_interview',
                                          'position_filled', 'candidate_declined', 'accepted_other_offer', 'no_response', 'other')),
  reason_note      text CHECK (length(reason_note) <= 1000),
  stage_changed_at timestamptz NOT NULL DEFAULT now(),
  created_by       text NOT NULL,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  -- A candidate is on a job once.
  UNIQUE (tenant_id, job_order_id, candidate_id),
  -- Job and candidate belong to the same tenant (composite keys).
  FOREIGN KEY (tenant_id, job_order_id) REFERENCES job_order (tenant_id, id),
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id),
  -- The same rule the Go domain code applies: a reason exactly when closed.
  CONSTRAINT application_reason_when_closed CHECK (
    (stage IN ('rejected', 'withdrawn')) = (reason IS NOT NULL) AND (reason_note IS NULL OR reason IS NOT NULL)
  )
);
CREATE INDEX application_job_idx ON application (tenant_id, job_order_id, id);
CREATE INDEX application_candidate_idx ON application (tenant_id, candidate_id, id DESC);

-- Every table holding tenant data has row-level security (a test checks this).
ALTER TABLE application ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON application
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS application;
