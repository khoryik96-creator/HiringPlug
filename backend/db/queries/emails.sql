-- Queries owned by the emails module. Run inside a tenant-scoped transaction.

-- name: ListTemplates :many
SELECT * FROM email_template ORDER BY lower(name) LIMIT 200;

-- name: GetTemplate :one
SELECT * FROM email_template WHERE id = @id;

-- name: InsertTemplate :one
INSERT INTO email_template (tenant_id, name, subject, body, created_by)
VALUES (app_current_tenant(), @name, @subject, @body, app_current_user())
RETURNING *;

-- The starter templates, added the first time a company opens its templates
-- (two people opening them at once add them once).
-- name: SeedTemplate :exec
INSERT INTO email_template (tenant_id, name, subject, body, created_by)
VALUES (app_current_tenant(), @name, @subject, @body, app_current_user())
ON CONFLICT DO NOTHING;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateTemplate :one
UPDATE email_template SET name = @name, subject = @subject, body = @body, version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- name: InsertEmail :one
INSERT INTO email (tenant_id, candidate_id, application_id, to_addresses, cc_addresses, subject, body,
                   sent_by, sender_name, reply_to)
VALUES (app_current_tenant(), sqlc.narg(candidate_id), sqlc.narg(application_id), @to_addresses::text[],
        @cc_addresses::text[], @subject, @body, app_current_user(), @sender_name, @reply_to)
RETURNING *;

-- name: GetEmail :one
SELECT * FROM email WHERE id = @id;

-- name: ListCandidateEmails :many
SELECT * FROM email WHERE candidate_id = @candidate_id ORDER BY id DESC LIMIT 200;

-- name: MarkEmail :exec
UPDATE email SET status = @status, error = sqlc.narg(error),
  sent_at = CASE WHEN @status = 'sent' THEN now() ELSE sent_at END
WHERE id = @id;

-- Merging candidates: the duplicate's emails follow them.
-- name: MoveEmailsToCandidate :execrows
UPDATE email SET candidate_id = @to_id WHERE candidate_id = @from_id;
