-- Queries owned by the matching module. Run inside a tenant-scoped transaction.
-- Lists never return sent_job / sent_cv: they are only for checking the AI's quotes.

-- name: InsertMatch :one
INSERT INTO job_match (tenant_id, application_id, job_id, sent_job, sent_cv, job_hash, created_by)
VALUES (app_current_tenant(), @application_id, @job_id, @sent_job, @sent_cv, @job_hash, app_current_user())
RETURNING id, application_id, job_id, status, score, summary, reasons, dropped, error, model, created_at, scored_at, job_hash,
  dimensions, strengths, gaps;

-- The latest score of each of a job's applications.
-- name: LatestForJob :many
SELECT DISTINCT ON (application_id)
  id, application_id, job_id, status, score, summary, reasons, dropped, error, model, created_at, scored_at, job_hash,
  dimensions, strengths, gaps
FROM job_match
WHERE job_id = @job_id
ORDER BY application_id, created_at DESC, id DESC
LIMIT 1000;

-- name: LatestForApplication :many
SELECT id, application_id, job_id, status, score, summary, reasons, dropped, error, model, created_at, scored_at, job_hash,
  dimensions, strengths, gaps
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
    dimensions = @dimensions, strengths = @strengths, gaps = @gaps,
    error = sqlc.narg(error), model = sqlc.narg(model), input_tokens = @input_tokens, output_tokens = @output_tokens,
    scored_at = now(), sent_job = NULL, sent_cv = NULL
WHERE id = @id AND status = 'queued';

-- Tokens the company used for matching since the start of the month.
-- name: TokensSince :one
SELECT ((SELECT coalesce(sum(m.input_tokens + m.output_tokens), 0) FROM job_match m WHERE m.created_at >= @since::timestamptz)
      + (SELECT coalesce(sum(t.tokens), 0) FROM talent_search t WHERE t.created_at >= @since::timestamptz))::bigint;

-- AI candidate searches (CV Studio's AI Crawler). A user sees only their own.
-- name: InsertTalentSearch :one
INSERT INTO talent_search (tenant_id, created_by, status, jd, boolean, filters, strict, considered, sent, summary, finished_at)
VALUES (app_current_tenant(), app_current_user(), @status, sqlc.narg(jd), sqlc.narg(boolean), @filters, @strict, @considered, @sent,
        sqlc.narg(summary), CASE WHEN @status::text = 'queued' THEN NULL ELSE now() END)
RETURNING *;

-- name: GetTalentSearch :one
SELECT * FROM talent_search WHERE id = @id AND created_by = app_current_user();

-- name: ListTalentSearches :many
SELECT * FROM talent_search WHERE created_by = app_current_user() ORDER BY created_at DESC, id DESC LIMIT 10;

-- The AI's answer (in the Go worker: no user, so by id).
-- name: TalentSearchForAnswer :one
SELECT * FROM talent_search WHERE id = @id;

-- name: FinishTalentSearch :execrows
UPDATE talent_search
   SET status = @status, results = @results, summary = sqlc.narg(summary), must_haves = @must_haves,
       nice_to_haves = @nice_to_haves, suggested_boolean = sqlc.narg(suggested_boolean), error = sqlc.narg(error),
       model = sqlc.narg(model), tokens = @tokens, finished_at = now()
 WHERE id = @id AND status = 'queued';
