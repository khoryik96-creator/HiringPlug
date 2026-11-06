-- candidates: whether a candidate is open to a move (looking out, passive,
-- not looking, placed); it colours their page. Null = not set.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE candidate ADD COLUMN availability text CHECK (availability IN ('looking', 'passive', 'not_looking', 'placed'));

-- +goose Down
ALTER TABLE candidate DROP COLUMN IF EXISTS availability;
