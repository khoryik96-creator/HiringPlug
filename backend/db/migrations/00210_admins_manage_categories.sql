-- pipeline (1.59.1): admins add, rename and delete stage categories too, not
-- only owners. Admin roles a company has customised get the permission, as
-- they would by default.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
UPDATE role_setting SET permissions = array_append(permissions, 'pipeline.categories:manage')
WHERE role = 'admin' AND NOT ('pipeline.categories:manage' = ANY (permissions));

-- +goose Down
UPDATE role_setting SET permissions = array_remove(permissions, 'pipeline.categories:manage') WHERE role = 'admin';
