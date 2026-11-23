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

-- One run at a time per candidate: the second of two clicks waits here, then sees the first.
-- name: LockStudioCandidate :exec
SELECT pg_advisory_xact_lock(hashtextextended('studio_cv:' || app_current_tenant() || ':' || @candidate_id::text, 0));

-- A run still being made (one queued for over 15 minutes is taken as lost, so it never blocks for good).
-- name: StudioCvRunning :one
SELECT EXISTS (SELECT 1 FROM studio_cv WHERE candidate_id = @candidate_id AND status = 'queued' AND created_at > now() - interval '15 minutes');

-- name: InsertStudioCv :one
INSERT INTO studio_cv (id, tenant_id, candidate_id, source_document_id, blind, neutral_gender, correct_language, alignment, created_by, replace_document_ids)
VALUES (@id, app_current_tenant(), @candidate_id, @source_document_id, @blind, @neutral_gender, @correct_language, @alignment, app_current_user(), @replace_document_ids::text[])
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

-- Merging candidates: the duplicate's formatting runs follow it.
-- name: MoveStudioCvs :exec
UPDATE studio_cv SET candidate_id = @to_id WHERE candidate_id = @from_id;

-- How a formatted CV was made (blind, with salary): only those without salary go to clients.
-- name: RecordFormattedCv :exec
INSERT INTO formatted_cv (tenant_id, document_id, candidate_id, blind, include_salary)
VALUES (app_current_tenant(), @document_id, @candidate_id, @blind, @include_salary)
ON CONFLICT (tenant_id, document_id) DO NOTHING;

-- name: GetFormattedCv :one
SELECT * FROM formatted_cv WHERE document_id = @document_id;

-- name: MoveFormattedCvs :exec
UPDATE formatted_cv SET candidate_id = @to_id WHERE candidate_id = @from_id;
