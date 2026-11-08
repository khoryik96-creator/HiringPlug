-- notes: a reference check is a kind of note, like a screening call (its
-- questions are a company setting, the reference_check list).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE note DROP CONSTRAINT IF EXISTS note_kind_check;
ALTER TABLE note ADD CONSTRAINT note_kind_check
  CHECK (kind IN ('general', 'screening', 'interview', 'call', 'reference'));

-- +goose Down
UPDATE note SET kind = 'call' WHERE kind = 'reference'; -- kept, as calls
ALTER TABLE note DROP CONSTRAINT IF EXISTS note_kind_check;
ALTER TABLE note ADD CONSTRAINT note_kind_check
  CHECK (kind IN ('general', 'screening', 'interview', 'call'));
