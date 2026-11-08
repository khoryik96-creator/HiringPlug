-- jobs: AI rewrites of an ad's blind version, CV Studio's way. The AI
-- worker paraphrases the ad without the client's identity and runs CV
-- Studio's checks; the result becomes the ad's blind version. Each run is
-- kept (status, warnings, tokens): the tokens count in the company's monthly
-- AI allowance.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE job_ad_blind_rewrite (
  id          text PRIMARY KEY DEFAULT new_id('jbr') CHECK (id ~ '^jbr_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id   text NOT NULL REFERENCES tenant (id),
  ad_id       text NOT NULL REFERENCES job_ad (id) ON DELETE CASCADE,
  status      text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'done', 'failed')),
  tone        text NOT NULL CHECK (tone IN ('professional', 'conversational', 'concise')),
  strength    text NOT NULL CHECK (strength IN ('low', 'medium', 'high')),
  error       text CHECK (length(error) <= 2000),
  warnings    text[] NOT NULL DEFAULT '{}' CHECK (cardinality(warnings) <= 10),
  retried     boolean NOT NULL DEFAULT false,
  model       text CHECK (length(model) <= 200),
  tokens      integer NOT NULL DEFAULT 0 CHECK (tokens >= 0),
  created_by  text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz
);
CREATE INDEX job_ad_blind_rewrite_ad_idx ON job_ad_blind_rewrite (tenant_id, ad_id, created_at DESC);
CREATE INDEX job_ad_blind_rewrite_month_idx ON job_ad_blind_rewrite (tenant_id, created_at);

ALTER TABLE job_ad_blind_rewrite ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON job_ad_blind_rewrite
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS job_ad_blind_rewrite;
