-- matching: CV Studio's six ratings (skills, experience, industry, education,
-- leadership, keywords) with a note each, and the AI's strengths and gaps.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE job_match
  ADD COLUMN dimensions jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(dimensions) = 'array'),
  ADD COLUMN strengths text[] NOT NULL DEFAULT '{}' CHECK (cardinality(strengths) <= 5),
  ADD COLUMN gaps text[] NOT NULL DEFAULT '{}' CHECK (cardinality(gaps) <= 5);

-- +goose Down
ALTER TABLE job_match DROP COLUMN IF EXISTS dimensions, DROP COLUMN IF EXISTS strengths, DROP COLUMN IF EXISTS gaps;
