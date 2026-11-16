-- The AI's pick of a new company's industry from the agency's own list: one
-- row per ask, with the tokens it used (they count towards the company's
-- monthly AI allowance). A company without an industry gets the one picked;
-- one with an industry keeps it.
-- company: industry_hint, an industry named elsewhere (LinkedIn's) when the
--   company was added, kept only for the AI to pick with (never an industry).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE company ADD COLUMN industry_hint text CHECK (length(industry_hint) <= 200);

CREATE TABLE company_industry_pick (
  id            text PRIMARY KEY DEFAULT new_id('cip') CHECK (id ~ '^cip_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id     text NOT NULL REFERENCES tenant (id),
  company_id    text NOT NULL,
  status        text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'done', 'failed')),
  choice_id     text CHECK (length(choice_id) <= 100),
  error         text CHECK (length(error) <= 1000),
  model         text CHECK (length(model) <= 200),
  input_tokens  integer NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
  output_tokens integer NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX company_industry_pick_month_idx ON company_industry_pick (tenant_id, created_at);

ALTER TABLE company_industry_pick ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON company_industry_pick
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS company_industry_pick;
ALTER TABLE company DROP COLUMN IF EXISTS industry_hint;
