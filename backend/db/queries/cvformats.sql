-- Queries owned by the cvformats module. Run inside a tenant-scoped transaction.

-- name: ListTemplates :many
SELECT * FROM cv_template ORDER BY lower(name), id LIMIT 100;

-- name: GetTemplate :one
SELECT * FROM cv_template WHERE id = @id;

-- name: NewTemplateID :one
SELECT new_id('ctpl')::text;

-- name: InsertTemplate :one
INSERT INTO cv_template (id, tenant_id, name, storage_key, uploaded_by)
VALUES (@id, app_current_tenant(), @name, @storage_key, app_current_user())
RETURNING *;

-- name: DeleteTemplate :one
DELETE FROM cv_template WHERE id = @id RETURNING storage_key;

-- name: NewStudioCvID :one
SELECT new_id('scv')::text;

-- name: InsertStudioCv :one
INSERT INTO studio_cv (id, tenant_id, candidate_id, source_document_id, blind, neutral_gender, correct_language, alignment, created_by)
VALUES (@id, app_current_tenant(), @candidate_id, @source_document_id, @blind, @neutral_gender, @correct_language, @alignment, app_current_user())
RETURNING *;

-- name: GetStudioCv :one
SELECT * FROM studio_cv WHERE id = @id;

-- name: ListStudioCvs :many
SELECT * FROM studio_cv WHERE candidate_id = @candidate_id ORDER BY created_at DESC, id DESC LIMIT 10;

-- name: FinishStudioCv :one
UPDATE studio_cv
   SET status = @status, document_id = sqlc.narg(document_id), file_name = sqlc.narg(file_name),
       error = sqlc.narg(error), warning = sqlc.narg(warning), pay_removed = @pay_removed, finished_at = now()
 WHERE id = @id AND status = 'queued'
RETURNING *;

-- name: RecordStudioCvAIUse :exec
INSERT INTO studio_cv_ai_use (tenant_id, studio_cv_id, tokens) VALUES (app_current_tenant(), @studio_cv_id, @tokens);

-- name: StudioCvTokensSince :one
SELECT coalesce(sum(tokens), 0)::bigint FROM studio_cv_ai_use WHERE created_at >= @since;
