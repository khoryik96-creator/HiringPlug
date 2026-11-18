-- documents: why a CV's details could not be read (the parser's error type
--   and where it broke, never CV text), shown under "The details could not be
--   read"; cleared when the details are asked for again.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE document ADD COLUMN parse_error text CHECK (length(parse_error) <= 300);

-- +goose Down
ALTER TABLE document DROP COLUMN IF EXISTS parse_error;
