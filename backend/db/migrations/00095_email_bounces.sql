-- email.bounced_to / bounced_at: recipients whose mail server returned a sent
-- email, found as bounce notices in the sending Microsoft 365 mailbox's inbox
-- (only the addresses and when are kept, never the notice itself).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE email ADD COLUMN bounced_to text[] NOT NULL DEFAULT '{}' CHECK (cardinality(bounced_to) <= 50);
ALTER TABLE email ADD COLUMN bounced_at timestamptz;

-- +goose Down
ALTER TABLE email DROP COLUMN IF EXISTS bounced_at;
ALTER TABLE email DROP COLUMN IF EXISTS bounced_to;
