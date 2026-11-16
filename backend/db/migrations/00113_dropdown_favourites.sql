-- user_preference.favourites: each person's own favourites in the
-- dropdowns (specialization, industries, IT skills, certifications and
-- countries), shown first for them only: {"itSkill": ["Java"], ...}.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE user_preference ADD COLUMN favourites jsonb NOT NULL DEFAULT '{}'
  CHECK (jsonb_typeof(favourites) = 'object' AND octet_length(favourites::text) <= 65536);

-- +goose Down
ALTER TABLE user_preference DROP COLUMN IF EXISTS favourites;
