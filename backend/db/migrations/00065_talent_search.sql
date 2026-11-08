-- matching: AI candidate searches (CV Studio's AI Crawler, on the company's
-- own candidates). Each keeps what was asked (JD, boolean, filters), which
-- candidates were sent to the AI (by reference, never their names), and the
-- AI's ranking and notes. Its tokens count in the monthly AI allowance.
-- Searches are personal: the API shows a user only their own.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE talent_search (
  id           text PRIMARY KEY DEFAULT new_id('tsr') CHECK (id ~ '^tsr_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id    text NOT NULL REFERENCES tenant (id),
  created_by   text NOT NULL,
  status       text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'done', 'failed')),
  jd           text CHECK (length(jd) <= 20000),
  boolean      text CHECK (length(boolean) <= 1000),
  filters      jsonb NOT NULL DEFAULT '{}',
  strict       boolean NOT NULL DEFAULT false,
  considered   integer NOT NULL DEFAULT 0,
  sent         text[] NOT NULL DEFAULT '{}' CHECK (cardinality(sent) <= 20), -- candidate ids, in the order C1, C2...
  results      jsonb NOT NULL DEFAULT '[]',
  summary      text CHECK (length(summary) <= 3000),
  must_haves   text[] NOT NULL DEFAULT '{}',
  nice_to_haves text[] NOT NULL DEFAULT '{}',
  suggested_boolean text CHECK (length(suggested_boolean) <= 1000),
  error        text CHECK (length(error) <= 2000),
  model        text CHECK (length(model) <= 200),
  tokens       integer NOT NULL DEFAULT 0 CHECK (tokens >= 0),
  created_at   timestamptz NOT NULL DEFAULT now(),
  finished_at  timestamptz
);
CREATE INDEX talent_search_user_idx ON talent_search (tenant_id, created_by, created_at DESC);
CREATE INDEX talent_search_month_idx ON talent_search (tenant_id, created_at);

ALTER TABLE talent_search ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON talent_search
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS talent_search;
