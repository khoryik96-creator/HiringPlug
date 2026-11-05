-- candidates: where a candidate lives (an ISO country code; null = not known,
-- shown as NA), read from CVs or entered.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE candidate ADD COLUMN residence_country text CHECK (residence_country ~ '^[A-Z]{2}$');

-- +goose Down
ALTER TABLE candidate DROP COLUMN IF EXISTS residence_country;
