-- An imported file whose new candidate was merged into another while it was
-- being read went to that candidate: it is shown as "added" (to a
-- candidate who was already there), not "created".
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE cv_import_item ADD COLUMN merged boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE cv_import_item DROP COLUMN merged;
