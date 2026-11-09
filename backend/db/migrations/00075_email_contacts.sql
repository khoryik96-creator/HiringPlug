-- emails: an email may be written to one of a client's contacts (from the
--   contact's page): email.contact_id, to list them on that page.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE email ADD COLUMN contact_id text;
CREATE INDEX email_contact_idx ON email (tenant_id, contact_id, id) WHERE contact_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS email_contact_idx;
ALTER TABLE email DROP COLUMN IF EXISTS contact_id;
