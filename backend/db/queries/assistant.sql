-- Queries owned by the assistant module. Run inside a tenant-scoped transaction.

-- name: InsertAnswer :one
INSERT INTO assistant_answer (tenant_id, user_id, subject_type, subject_id, kind, question, used_cv, market_scope)
VALUES (app_current_tenant(), app_current_user(), @subject_type, @subject_id, @kind, sqlc.narg(question), @used_cv,
        CASE WHEN @kind = 'market_map' THEN current_setting('app.scope_users', true) || ':' || current_setting('app.scope_jobs', true) END)
RETURNING *;

-- The user's own answers about one subject, newest first.
-- name: ListAnswers :many
SELECT * FROM assistant_answer
WHERE user_id = app_current_user() AND subject_type = @subject_type AND subject_id = @subject_id
  AND (NOT used_cv OR app_has_permission('documents:read'))
  AND (subject_type NOT IN ('candidate','application') OR app_has_permission('candidates:read'))
  AND (subject_type NOT IN ('job','application') OR app_has_permission('jobs:read'))
  AND (subject_type <> 'application' OR app_has_permission('pipeline:read'))
  AND (kind <> 'market_map' OR (app_has_permission('candidates:read') AND
       (app_scope_all() OR market_scope = current_setting('app.scope_users', true) || ':' || current_setting('app.scope_jobs', true))))
ORDER BY created_at DESC, id DESC
LIMIT 20;

-- Once: no row when it was answered already (a repeated result). Returns who asked.
-- name: SaveAnswer :one
UPDATE assistant_answer
SET status = @status, answer = sqlc.narg(answer), error = sqlc.narg(error), model = sqlc.narg(model),
    input_tokens = @input_tokens, output_tokens = @output_tokens, answered_at = now()
WHERE id = @id AND status = 'queued'
RETURNING user_id;

-- name: InsertEmail :one
INSERT INTO assistant_email (tenant_id, user_id, mode)
VALUES (app_current_tenant(), app_current_user(), @mode)
RETURNING *;

-- Only the person it was written for sees it.
-- name: GetEmail :one
SELECT * FROM assistant_email WHERE id = @id AND user_id = app_current_user();

-- Once: no row when it was saved already (a repeated result). Returns who asked.
-- name: SaveEmail :one
UPDATE assistant_email
SET status = @status, subject = sqlc.narg(subject), body = sqlc.narg(body), error = sqlc.narg(error), model = sqlc.narg(model),
    input_tokens = @input_tokens, output_tokens = @output_tokens, written_at = now()
WHERE id = @id AND status = 'queued'
RETURNING user_id;

-- What the AI said about records deleted for good (the Bin): it describes them.
-- name: ForgetAnswers :exec
DELETE FROM assistant_answer WHERE subject_type = @subject_type AND subject_id = ANY (@ids::text[]);
