-- contacts module: a LinkedIn check may match a role linked to no LinkedIn
-- company page by the company's exact name; such a result never marks the
-- contact as left.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE contact_linkedin_check ADD COLUMN matched_by_name boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE contact_linkedin_check DROP COLUMN IF EXISTS matched_by_name;
