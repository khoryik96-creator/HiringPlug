-- identity: adding email templates is a permission of its own
--   (emails.templates:create); changing and deleting them stays with
--   emails.templates:manage, now owners' and admins' only by default. Roles a
--   company set itself keep what they could do: those that could manage
--   templates (or are recruiters or sourcers) may add them.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
UPDATE role_setting SET permissions = array_append(permissions, 'emails.templates:create')
WHERE NOT ('emails.templates:create' = ANY (permissions))
  AND ('emails.templates:manage' = ANY (permissions) OR role IN ('admin', 'recruiter', 'sourcer'));

-- +goose Down
UPDATE role_setting SET permissions = array_remove(permissions, 'emails.templates:create');
