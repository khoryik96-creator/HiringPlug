-- emails module: Outlook Mail reads like Outlook - the sender's name and
-- the flag of each message (filled by the next sync). Like the rest of the
-- index, no part of a message's body is stored.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE email_outlook_message
  ADD COLUMN sender_name text NOT NULL DEFAULT '' CHECK (length(sender_name) <= 300),
  ADD COLUMN flagged boolean NOT NULL DEFAULT false;
-- Messages synced before this have no name or flag: the next full sync
-- of each folder fills them in.
UPDATE email_outlook_folder SET cursor_url = NULL;

-- +goose Down
ALTER TABLE email_outlook_message DROP COLUMN sender_name, DROP COLUMN flagged;
