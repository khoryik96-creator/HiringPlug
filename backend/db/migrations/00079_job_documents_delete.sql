-- identity: admins who chose their own permissions get the new
--   jobs.documents:delete, to delete the job descriptions kept with a job
--   (owners have every permission).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
UPDATE role_setting SET permissions = array_append(permissions, 'jobs.documents:delete')
WHERE role = 'admin' AND NOT ('jobs.documents:delete' = ANY (permissions));

-- +goose Down
UPDATE role_setting SET permissions = array_remove(permissions, 'jobs.documents:delete');
