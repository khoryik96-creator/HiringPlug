-- emails: whose CVs an email carried to someone other than that candidate
-- (a CV sent to a client), for each recruiter's activity in reports. Emails
-- sent before this have none recorded.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE email ADD COLUMN cv_candidate_ids text[] NOT NULL DEFAULT '{}' CHECK (cardinality(cv_candidate_ids) <= 10);
CREATE INDEX email_cvs_sent_idx ON email (tenant_id, created_at) WHERE cardinality(cv_candidate_ids) > 0;

-- +goose Down
DROP INDEX IF EXISTS email_cvs_sent_idx;
ALTER TABLE email DROP COLUMN IF EXISTS cv_candidate_ids;
