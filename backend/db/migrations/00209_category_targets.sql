-- reports (1.59.0): activity targets for any stage category (1st, 2nd and
-- Final Interview, or a company's own), by category key: how many candidates
-- the user should move into a stage of that category in the week or month.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE activity_target
  ADD COLUMN categories jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(categories) = 'object');

-- +goose Down
ALTER TABLE activity_target DROP COLUMN IF EXISTS categories;
