-- job_description_file: a job description may be read before its job exists
-- (dropped on the Add job form to fill the job in); job_id is then empty.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE job_description_file ALTER COLUMN job_id DROP NOT NULL;

-- +goose Down
DELETE FROM job_description_file WHERE job_id IS NULL;
ALTER TABLE job_description_file ALTER COLUMN job_id SET NOT NULL;
