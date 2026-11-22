-- notifications: when the morning email's next job is due, so saving the
-- settings can tell a chain that stopped (its job failed for good) and start
-- a new one. Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE notification_state ADD COLUMN digest_due_at timestamptz;

-- +goose Down
ALTER TABLE notification_state DROP COLUMN IF EXISTS digest_due_at;
