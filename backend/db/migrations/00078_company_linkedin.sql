-- companies: each company's LinkedIn page (linkedin_url).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE company ADD COLUMN linkedin_url text CHECK (length(linkedin_url) <= 300);

-- +goose Down
ALTER TABLE company DROP COLUMN IF EXISTS linkedin_url;
