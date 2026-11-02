-- pipeline: where an application came from (the job board link a candidate used).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE application ADD COLUMN channel text
  CHECK (channel IN ('careers_page', 'jobstreet', 'linkedin', 'indeed', 'facebook', 'referral', 'other'));
COMMENT ON COLUMN application.channel IS 'NULL when a recruiter added the candidate';

-- +goose Down
ALTER TABLE application DROP COLUMN IF EXISTS channel;
