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

-- The dashboard. Counts per job and stage for the given (open) jobs.
-- name: StageCounts :many
SELECT job_order_id, stage, count(*)::integer AS n
FROM application WHERE job_order_id = ANY(@job_ids::text[])
GROUP BY job_order_id, stage;

-- Longest in one open stage (offer and earlier), past the cutoff.
-- name: StuckApplications :many
SELECT * FROM application
WHERE job_order_id = ANY(@job_ids::text[])
  AND stage IN ('applied', 'screening', 'submitted', 'interview', 'offer')
  AND stage_changed_at < now() - make_interval(days => @stuck_days::integer)
ORDER BY stage_changed_at LIMIT 10;

-- name: RecentResults :one
SELECT
  count(*) FILTER (WHERE stage = 'placed' AND stage_changed_at > now() - interval '30 days')::integer AS placed_last_30_days,
  count(*) FILTER (WHERE version > 1 AND stage_changed_at > now() - interval '7 days')::integer AS moved_last_7_days
FROM application;

-- Merging candidates: drop the duplicate's applications to jobs the kept
-- candidate is already on, then move the rest.
-- name: DropOverlappingApplications :execrows
DELETE FROM application a
WHERE a.candidate_id = @from_id
  AND EXISTS (SELECT 1 FROM application k WHERE k.candidate_id = @to_id AND k.job_order_id = a.job_order_id);

-- name: MoveApplicationsToCandidate :execrows
UPDATE application SET candidate_id = @to_id, updated_at = now() WHERE candidate_id = @from_id;
