-- documents: details parsed from a CV (suggestions to review), and the parser's status.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE document
  ADD COLUMN parse_status   text CHECK (parse_status IN ('queued', 'parsed', 'failed')),
  -- Only values whose quote was found in extracted_text are stored (checked in Go).
  ADD COLUMN parsed_profile jsonb,
  ADD COLUMN parser         text;

-- +goose Down
ALTER TABLE document DROP COLUMN IF EXISTS parser, DROP COLUMN IF EXISTS parsed_profile, DROP COLUMN IF EXISTS parse_status;
