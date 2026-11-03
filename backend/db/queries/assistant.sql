-- Queries owned by the assistant module. Run inside a tenant-scoped transaction.

-- name: InsertAnswer :one
INSERT INTO assistant_answer (tenant_id, user_id, subject_type, subject_id, kind, question)
VALUES (app_current_tenant(), app_current_user(), @subject_type, @subject_id, @kind, sqlc.narg(question))
RETURNING *;

-- The user's own answers about one subject, newest first.
-- name: ListAnswers :many
SELECT * FROM assistant_answer
WHERE user_id = app_current_user() AND subject_type = @subject_type AND subject_id = @subject_id
ORDER BY created_at DESC, id DESC
LIMIT 20;

-- Tokens the company used since the start of the month.
-- name: TokensSince :one
SELECT coalesce(sum(input_tokens + output_tokens), 0)::bigint FROM assistant_answer WHERE created_at >= @since;

-- name: SaveAnswer :execrows
UPDATE assistant_answer
SET status = @status, answer = sqlc.narg(answer), error = sqlc.narg(error), model = sqlc.narg(model),
    input_tokens = @input_tokens, output_tokens = @output_tokens, answered_at = now()
WHERE id = @id AND status = 'queued';
