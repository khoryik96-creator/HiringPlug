-- cvformats: CV Studio formatting runs. One of the candidate's own CVs is read
-- by the AI and laid out by CV Studio's rules and renderer in the background;
-- the result is saved with their documents. Tokens used are counted in the
-- company's monthly AI allowance (studio_cv_ai_use, like document_ai_use, so
-- the count does not depend on which candidates the person asking may see).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE studio_cv (
  id                 text PRIMARY KEY DEFAULT new_id('scv') CHECK (id ~ '^scv_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id          text NOT NULL REFERENCES tenant (id),
  candidate_id       text NOT NULL,
  source_document_id text NOT NULL,
  status             text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'done', 'failed')),
  blind              boolean NOT NULL,
  neutral_gender     boolean NOT NULL DEFAULT false,
  correct_language   boolean NOT NULL,
  alignment          text NOT NULL CHECK (alignment IN ('left', 'justify')),
  document_id        text,
  file_name          text,
  error              text CHECK (length(error) <= 2000),
  warning            text CHECK (length(warning) <= 4000),
  pay_removed        integer NOT NULL DEFAULT 0 CHECK (pay_removed >= 0),
  created_by         text NOT NULL,
  created_at         timestamptz NOT NULL DEFAULT now(),
  finished_at        timestamptz
);
CREATE INDEX studio_cv_candidate_idx ON studio_cv (tenant_id, candidate_id, created_at DESC);

ALTER TABLE studio_cv ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON studio_cv
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY candidate_scope ON studio_cv AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));

CREATE TABLE studio_cv_ai_use (
  tenant_id     text NOT NULL REFERENCES tenant (id),
  studio_cv_id  text NOT NULL,
  tokens        integer NOT NULL CHECK (tokens >= 0),
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX studio_cv_ai_use_month_idx ON studio_cv_ai_use (tenant_id, created_at);

ALTER TABLE studio_cv_ai_use ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON studio_cv_ai_use
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS studio_cv_ai_use;
DROP TABLE IF EXISTS studio_cv;
