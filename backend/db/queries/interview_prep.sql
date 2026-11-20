-- Queries owned by the interview_prep module. Run inside a tenant-scoped transaction.

-- name: InsertInterviewQuestion :one
INSERT INTO interview_question (tenant_id, job_id, question, round, kind, asked_on, source, notes, created_by)
VALUES (app_current_tenant(), @job_id, @question, sqlc.narg(round), @kind, sqlc.narg(asked_on), sqlc.narg(source), sqlc.narg(notes), app_current_user())
RETURNING *;

-- name: ListInterviewQuestions :many
SELECT * FROM interview_question
WHERE job_id = @job_id AND (archived_at IS NOT NULL) = @archived::boolean
ORDER BY id DESC LIMIT 500;

-- name: GetInterviewQuestion :one
SELECT * FROM interview_question WHERE id = @id;

-- name: UpdateInterviewQuestion :one
UPDATE interview_question SET
  question = @question, round = sqlc.narg(round), kind = @kind, asked_on = sqlc.narg(asked_on),
  source = sqlc.narg(source), notes = sqlc.narg(notes), archived_at = sqlc.narg(archived_at),
  version = version + 1, updated_at = now()
WHERE id = @id AND version = @version
RETURNING *;

-- name: DeleteInterviewQuestion :execrows
DELETE FROM interview_question WHERE id = @id;

-- The questions the AI prepares from: the job's, not archived, the newest first.
-- name: QuestionsForPrep :many
SELECT * FROM interview_question
WHERE job_id = @job_id AND archived_at IS NULL
ORDER BY id DESC LIMIT 60;

-- name: InsertInterviewPrep :one
INSERT INTO interview_prep (tenant_id, job_id, focus, question_count, created_by)
VALUES (app_current_tenant(), @job_id, sqlc.narg(focus), @question_count, app_current_user())
RETURNING *;

-- name: ListInterviewPreps :many
SELECT * FROM interview_prep
WHERE job_id = @job_id AND (archived_at IS NOT NULL) = @archived::boolean
ORDER BY id DESC LIMIT 50;

-- name: GetInterviewPrep :one
SELECT * FROM interview_prep WHERE id = @id;

-- name: ArchiveInterviewPrep :one
UPDATE interview_prep SET archived_at = sqlc.narg(archived_at) WHERE id = @id RETURNING *;

-- The AI's material (or why there is none), once; a late answer to a run
-- already shown as failed still fills it.
-- name: FinishInterviewPrep :execrows
UPDATE interview_prep SET
  status = @status, content = sqlc.narg(content), error = sqlc.narg(error), model = sqlc.narg(model),
  tokens = @tokens, finished_at = now()
WHERE id = @id AND status = 'queued';

-- name: InterviewPrepTokensSince :one
SELECT coalesce(sum(tokens), 0)::bigint FROM interview_prep WHERE created_at >= @since;
