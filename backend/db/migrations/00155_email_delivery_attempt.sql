-- +goose Up
-- Committed before contacting a provider. If the worker dies after the send
-- or cannot commit its outcome, another worker must not send the same email.
ALTER TABLE email ADD COLUMN delivery_attempted_at timestamptz;

-- +goose Down
ALTER TABLE email DROP COLUMN delivery_attempted_at;
