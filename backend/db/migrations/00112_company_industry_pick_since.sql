-- company: industry_pick_since, set while the AI is picking a new company's
--   industry (asked, or its answer awaited), so the company's page reads the
--   company again once it is picked.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE company ADD COLUMN industry_pick_since timestamptz;

-- +goose Down
ALTER TABLE company DROP COLUMN IF EXISTS industry_pick_since;
