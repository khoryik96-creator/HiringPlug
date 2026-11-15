-- An email's attachments are listed (and sent) in the order they were added.
-- now() is the same for every row of a transaction, so CVs attached to one
-- email had equal times and came back in any order: clock_timestamp() keeps
-- the moment each row is written.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE email_attachment ALTER COLUMN created_at SET DEFAULT clock_timestamp();

-- +goose Down
ALTER TABLE email_attachment ALTER COLUMN created_at SET DEFAULT now();
