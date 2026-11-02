-- Queries owned by the interviews module. Run inside a tenant-scoped
-- transaction: row-level security limits every query to the current tenant.

-- name: InsertInterview :one
INSERT INTO interview (tenant_id, application_id, title, starts_at, duration_minutes, ends_at, mode, location,
                       instructions, interviewer_ids, created_by)
VALUES (app_current_tenant(), @application_id, @title, @starts_at, @duration_minutes, @ends_at, @mode, sqlc.narg(location),
        sqlc.narg(instructions), @interviewer_ids::text[], app_current_user())
RETURNING *;

-- name: GetInterview :one
SELECT * FROM interview WHERE id = @id;

-- name: ListApplicationInterviews :many
SELECT * FROM interview WHERE application_id = @application_id ORDER BY starts_at LIMIT 100;

-- Interviews in a time range, optionally only one interviewer's.
-- name: ListInterviews :many
SELECT * FROM interview
WHERE (sqlc.narg(from_time)::timestamptz IS NULL OR ends_at >= sqlc.narg(from_time))
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR starts_at < sqlc.narg(to_time))
  AND (sqlc.narg(interviewer)::text IS NULL OR sqlc.narg(interviewer)::text = ANY(interviewer_ids))
ORDER BY starts_at LIMIT 200;

-- Past interviews (not cancelled or missed) a user interviews on and has not given feedback for.
-- name: FeedbackDue :many
SELECT * FROM interview i
WHERE @user_id::text = ANY(i.interviewer_ids) AND i.status IN ('scheduled', 'completed') AND i.ends_at < now()
  AND NOT EXISTS (SELECT 1 FROM interview_feedback f WHERE f.interview_id = i.id AND f.user_id = @user_id::text)
ORDER BY i.starts_at DESC LIMIT 100;

-- Scheduled interviews overlapping a time for any of the given interviewers.
-- name: Clashes :many
SELECT id, title, starts_at, ends_at, interviewer_ids FROM interview
WHERE status = 'scheduled' AND id <> @exclude_id::text AND interviewer_ids && @interviewer_ids::text[]
  AND starts_at < @ends_at::timestamptz AND ends_at > @starts_at::timestamptz
ORDER BY starts_at LIMIT 20;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateInterview :one
UPDATE interview SET
  title = @title, starts_at = @starts_at, duration_minutes = @duration_minutes,
  ends_at = @ends_at, mode = @mode,
  location = sqlc.narg(location), instructions = sqlc.narg(instructions), interviewer_ids = @interviewer_ids::text[],
  status = @status, version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- name: ListFeedback :many
SELECT * FROM interview_feedback WHERE interview_id = ANY(@interview_ids::text[]) ORDER BY submitted_at;

-- name: UpsertFeedback :exec
INSERT INTO interview_feedback (tenant_id, interview_id, user_id, recommendation, comments)
VALUES (app_current_tenant(), @interview_id, @user_id, @recommendation, sqlc.narg(comments))
ON CONFLICT (tenant_id, interview_id, user_id)
DO UPDATE SET recommendation = EXCLUDED.recommendation, comments = EXCLUDED.comments, submitted_at = now();
