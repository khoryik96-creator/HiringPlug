-- Queries owned by the emails module. Run inside a tenant-scoped transaction.

-- name: ListTemplates :many
SELECT * FROM email_template ORDER BY lower(name) LIMIT 200;

-- name: GetTemplate :one
SELECT * FROM email_template WHERE id = @id;

-- name: InsertTemplate :one
INSERT INTO email_template (tenant_id, name, subject, body, body_html, created_by)
VALUES (app_current_tenant(), @name, @subject, @body, sqlc.narg(body_html), app_current_user())
RETURNING *;

-- name: DeleteTemplate :execrows
DELETE FROM email_template WHERE id = @id;

-- The starter templates, added the first time a company opens its templates
-- (two people opening them at once add them once).
-- name: SeedTemplate :exec
INSERT INTO email_template (tenant_id, name, subject, body, created_by)
VALUES (app_current_tenant(), @name, @subject, @body, app_current_user())
ON CONFLICT DO NOTHING;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateTemplate :one
UPDATE email_template SET name = @name, subject = @subject, body = @body, body_html = sqlc.narg(body_html),
  version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- name: InsertEmail :one
INSERT INTO email (tenant_id, candidate_id, application_id, to_addresses, cc_addresses, bcc_addresses, subject, body,
                   body_html, salary, salary_restricted, sent_by, sender_name, reply_to)
VALUES (app_current_tenant(), sqlc.narg(candidate_id), sqlc.narg(application_id), @to_addresses::text[],
        @cc_addresses::text[], @bcc_addresses::text[], @subject, @body, sqlc.narg(body_html), @salary, @salary_restricted,
        app_current_user(), @sender_name, @reply_to)
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

-- name: InsertBanner :one
INSERT INTO email_banner (tenant_id, name, content_type, size_bytes, storage_key, created_by)
VALUES (app_current_tenant(), @name, @content_type, @size_bytes, @storage_key, app_current_user())
RETURNING *;

-- name: ListBanners :many
SELECT * FROM email_banner WHERE deleted_at IS NULL ORDER BY created_at, id LIMIT 100;

-- Banners an email shows, deleted ones too (they were in it when it was sent).
-- name: GetBanners :many
SELECT * FROM email_banner WHERE id = ANY(@ids::text[]);

-- name: DeleteBanner :execrows
UPDATE email_banner SET deleted_at = now() WHERE id = @id AND deleted_at IS NULL;

-- name: InsertAttachment :one
INSERT INTO email_attachment (tenant_id, email_id, file_name, content_type, size_bytes, storage_key, created_by)
VALUES (app_current_tenant(), sqlc.narg(email_id), @file_name, @content_type, @size_bytes, @storage_key, app_current_user())
RETURNING *;

-- Files the user uploaded and has not sent yet, among ids.
-- name: StagedAttachments :many
SELECT * FROM email_attachment WHERE id = ANY(@ids::text[]) AND email_id IS NULL AND created_by = app_current_user();

-- name: AttachToEmail :exec
UPDATE email_attachment SET email_id = @email_id WHERE id = ANY(@ids::text[]) AND email_id IS NULL;

-- name: EmailAttachments :many
SELECT * FROM email_attachment WHERE email_id = ANY(@email_ids::text[]) ORDER BY created_at, id;

-- Uploads never sent, left for a day: removed (their files too).
-- name: DeleteStaleAttachments :many
DELETE FROM email_attachment WHERE email_id IS NULL AND created_at < now() - interval '1 day' RETURNING storage_key;
