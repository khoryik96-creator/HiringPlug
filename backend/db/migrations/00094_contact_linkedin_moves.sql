-- contact_linkedin_check.moved_to: when a contact's time at the company
-- ended, their current roles elsewhere as the same LinkedIn profile shows
-- them (up to 3: company page, name, title, since), so the recruiter can
-- follow them to their new company.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE contact_linkedin_check ADD COLUMN moved_to jsonb NOT NULL DEFAULT '[]'
  CHECK (jsonb_typeof(moved_to) = 'array' AND jsonb_array_length(moved_to) <= 3);

-- +goose Down
ALTER TABLE contact_linkedin_check DROP COLUMN IF EXISTS moved_to;
