-- interview_prep module: a job's interview insights. The questions asked at
-- its interviews (archived once no longer asked), and the prep material the
-- AI writes from the job, the client and those questions.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE interview_question (
  id          text PRIMARY KEY DEFAULT new_id('ivq') CHECK (id ~ '^ivq_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id   text NOT NULL REFERENCES tenant (id),
  job_id      text NOT NULL,
  question    text NOT NULL CHECK (length(btrim(question)) BETWEEN 1 AND 2000),
  round       text CHECK (round IN ('screening', 'hr', 'technical', 'hiring_manager', 'panel', 'case_study', 'final', 'other')),
  kind        text NOT NULL CHECK (kind IN ('technical', 'situational', 'behavioural', 'culture', 'other')),
  asked_on    date,
  source      text CHECK (length(source) <= 200),
  notes       text CHECK (length(notes) <= 2000),
  archived_at timestamptz,
  version     integer NOT NULL DEFAULT 1,
  created_by  text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, job_id) REFERENCES job_order (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX interview_question_job_idx ON interview_question (tenant_id, job_id, id DESC);

CREATE TABLE interview_prep (
  id             text PRIMARY KEY DEFAULT new_id('ivp') CHECK (id ~ '^ivp_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id      text NOT NULL REFERENCES tenant (id),
  job_id         text NOT NULL,
  status         text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'done', 'failed')),
  focus          text CHECK (length(focus) <= 1000),
  content        jsonb,
  error          text,
  model          text,
  tokens         integer NOT NULL DEFAULT 0,
  question_count integer NOT NULL DEFAULT 0,
  archived_at    timestamptz,
  created_by     text NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  finished_at    timestamptz,
  FOREIGN KEY (tenant_id, job_id) REFERENCES job_order (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX interview_prep_job_idx ON interview_prep (tenant_id, job_id, id DESC);
CREATE INDEX interview_prep_month_idx ON interview_prep (tenant_id, created_at);

-- Every table holding tenant data has row-level security (a test checks this).
ALTER TABLE interview_question ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON interview_question
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE interview_prep ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON interview_prep
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS interview_prep;
DROP TABLE IF EXISTS interview_question;
