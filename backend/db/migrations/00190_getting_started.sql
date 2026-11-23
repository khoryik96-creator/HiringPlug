-- identity: whether a person hid the dashboard's getting-started checklist
-- (its steps tick themselves from the company's records; only hiding it is
-- remembered). Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE user_preference ADD COLUMN guide_dismissed boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE user_preference DROP COLUMN IF EXISTS guide_dismissed;
