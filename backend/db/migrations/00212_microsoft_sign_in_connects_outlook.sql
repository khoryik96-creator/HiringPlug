-- identity (1.61.0): a company that signs in with Microsoft may also connect
-- each person's Outlook mail and calendar at that sign-in (one Microsoft
-- consent instead of three). Off until an owner turns it on.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE tenant ADD COLUMN microsoft_connects_outlook boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE tenant DROP COLUMN IF EXISTS microsoft_connects_outlook;
