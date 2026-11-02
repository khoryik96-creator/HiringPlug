-- Queries owned by the notes module. Run inside a tenant-scoped transaction:
-- row-level security limits every query to the current tenant, and sensitive
-- notes to users with notes.sensitive:read.

-- name: InsertNote :one
INSERT INTO note (tenant_id, subject_type, subject_id, kind, body, sensitive, author_id, author_name)
VALUES (app_current_tenant(), @subject_type, @subject_id, @kind, @body, @sensitive, app_current_user(), @author_name)
RETURNING *;

-- name: ListNotes :many
SELECT * FROM note WHERE subject_type = @subject_type AND subject_id = @subject_id ORDER BY id DESC LIMIT 500;

-- name: HiddenSensitiveNotes :one
SELECT app_hidden_sensitive_notes(@subject_type::text, @subject_id::text)::integer AS hidden;

-- name: GetNote :one
SELECT * FROM note WHERE id = @id;

-- name: DeleteNote :execrows
DELETE FROM note WHERE id = @id;
