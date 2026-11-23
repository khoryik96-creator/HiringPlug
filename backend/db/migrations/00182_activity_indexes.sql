-- Indexes for counting each user's activity in a week or month (reports):
-- calls logged, interviews arranged and placements recorded, by when.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE INDEX note_calls_idx ON note (tenant_id, created_at) WHERE kind IN ('screening', 'reference', 'call');
CREATE INDEX screening_call_day_idx ON screening_call (tenant_id, called_on);
CREATE INDEX interview_created_idx ON interview (tenant_id, created_at);
CREATE INDEX placement_created_idx ON placement (tenant_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS placement_created_idx;
DROP INDEX IF EXISTS interview_created_idx;
DROP INDEX IF EXISTS screening_call_day_idx;
DROP INDEX IF EXISTS note_calls_idx;
