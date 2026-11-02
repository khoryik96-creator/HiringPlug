-- interviews module: interviewers are reminded by email the day before.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- The start time the interviewers were last reminded of, so a reminder is
-- sent once per time (a moved interview is reminded again).
ALTER TABLE interview ADD COLUMN reminded_for timestamptz;

-- +goose Down
ALTER TABLE interview DROP COLUMN IF EXISTS reminded_for;
