-- identity (1.57.6): an invitation link keeps the email it was sent in, so
-- the Team page can say whether that email went (sent, or failed and why)
-- instead of "emailed" for one the mail server refused.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE user_link ADD COLUMN email_id text;

-- +goose Down
ALTER TABLE user_link DROP COLUMN IF EXISTS email_id;
