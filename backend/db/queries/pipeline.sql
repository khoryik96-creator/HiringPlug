-- Queries owned by the pipeline module. Run inside a tenant-scoped
-- transaction: row-level security limits every query to the current tenant.

-- name: InsertApplication :one
INSERT INTO application (tenant_id, job_order_id, candidate_id, stage, created_by)
VALUES (app_current_tenant(), @job_order_id, @candidate_id, @stage, app_current_user())
ON CONFLICT (tenant_id, job_order_id, candidate_id) DO NOTHING
RETURNING *;

-- name: FindApplication :one
SELECT id FROM application WHERE job_order_id = @job_order_id AND candidate_id = @candidate_id;

-- name: GetApplication :one
SELECT * FROM application WHERE id = @id;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: MoveApplication :one
UPDATE application SET
  stage = @stage,
  reason = sqlc.narg(reason),
  reason_note = sqlc.narg(reason_note),
  stage_changed_at = now(),
  version = version + 1,
  updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- A job's pipeline, oldest first (the order cards were added).
-- name: ListJobApplications :many
SELECT * FROM application WHERE job_order_id = @job_order_id ORDER BY id LIMIT 1000;

-- name: ListCandidateApplications :many
SELECT * FROM application WHERE candidate_id = @candidate_id ORDER BY id DESC LIMIT 200;
