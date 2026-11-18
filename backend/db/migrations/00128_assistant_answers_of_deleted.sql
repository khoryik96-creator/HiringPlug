-- assistant: the AI's answers about a candidate, application or job deleted
--   for good are deleted with it (the Bin now does so); those left behind by
--   earlier deletions are deleted here.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
DELETE FROM assistant_answer a
WHERE (a.subject_type = 'candidate' AND NOT EXISTS (SELECT 1 FROM candidate c WHERE c.tenant_id = a.tenant_id AND c.id = a.subject_id))
   OR (a.subject_type = 'application' AND NOT EXISTS (SELECT 1 FROM application p WHERE p.tenant_id = a.tenant_id AND p.id = a.subject_id))
   OR (a.subject_type = 'job' AND NOT EXISTS (SELECT 1 FROM job j WHERE j.tenant_id = a.tenant_id AND j.id = a.subject_id));

-- +goose Down
SELECT 1;
