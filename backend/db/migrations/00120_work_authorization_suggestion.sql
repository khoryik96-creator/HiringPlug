-- candidates module: a work authorization suggested from the candidate's CV
-- ({status, basis, reason}), shown striped on their card until someone
-- confirms or sets one. Only kept while none is set.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE candidate ADD COLUMN wa_suggestion jsonb CHECK (wa_suggestion IS NULL OR jsonb_typeof(wa_suggestion) = 'object');

-- +goose Down
ALTER TABLE candidate DROP COLUMN IF EXISTS wa_suggestion;
