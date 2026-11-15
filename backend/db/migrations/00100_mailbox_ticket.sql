-- Connecting a mailbox finishes in the app, signed in: the provider's
-- redirect only parks the sign-in code (sealed) and hands that browser a
-- one-time ticket; the person who started it finishes with the ticket. So a
-- link passed to someone else cannot connect their mailbox to your account.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE email_mailbox_state ADD COLUMN sealed_code text, ADD COLUMN ticket_hash text;

-- +goose Down
ALTER TABLE email_mailbox_state DROP COLUMN ticket_hash, DROP COLUMN sealed_code;
