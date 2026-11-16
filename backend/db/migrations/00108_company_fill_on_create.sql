-- A company added with its website is read at once, and what the website
-- says fills the details left empty (and replaces a name guessed from its
-- address); later readings only suggest.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE company
  ADD COLUMN autofill_fill_empty boolean NOT NULL DEFAULT false,
  ADD COLUMN autofill_name_guessed boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE company DROP COLUMN IF EXISTS autofill_name_guessed, DROP COLUMN IF EXISTS autofill_fill_empty;
