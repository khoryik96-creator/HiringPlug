-- Queries owned by the matching module. Run inside a tenant-scoped transaction.
-- Lists never return sent_job / sent_cv: they are only for checking the AI's quotes.

-- name: InsertMatch :one
INSERT INTO job_match (tenant_id, application_id, job_id, sent_job, sent_cv, created_by)
VALUES (app_current_tenant(), @application_id, @job_id, @sent_job, @sent_cv, app_current_user())
RETURNING id, application_id, job_id, status, score, summary, reasons, dropped, error, model, created_at, scored_at;

-- The latest score of each of a job's applications.
-- name: LatestForJob :many
SELECT DISTINCT ON (application_id)
  id, application_id, job_id, status, score, summary, reasons, dropped, error, model, created_at, scored_at
FROM job_match
WHERE job_id = @job_id
ORDER BY application_id, created_at DESC, id DESC
LIMIT 1000;

-- name: LatestForApplication :many
SELECT id, application_id, job_id, status, score, summary, reasons, dropped, error, model, created_at, scored_at
FROM job_match
WHERE application_id = @application_id
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- A match waiting for the AI, with what it was sent, locked while its answer is saved.
-- name: GetQueuedForUpdate :one
SELECT id, sent_job, sent_cv FROM job_match
WHERE id = @id AND status = 'queued'
FOR UPDATE;

-- name: SaveScore :execrows
UPDATE job_match
SET status = @status, score = sqlc.narg(score), summary = sqlc.narg(summary), reasons = @reasons, dropped = @dropped,
    error = sqlc.narg(error), model = sqlc.narg(model), input_tokens = @input_tokens, output_tokens = @output_tokens,
    scored_at = now(), sent_job = NULL, sent_cv = NULL
WHERE id = @id AND status = 'queued';

-- Tokens the company used for matching since the start of the month.
-- name: TokensSince :one
SELECT coalesce(sum(input_tokens + output_tokens), 0)::bigint FROM job_match WHERE created_at >= @since;
