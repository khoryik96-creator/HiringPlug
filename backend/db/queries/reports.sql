-- Queries owned by the reports module: activity targets and the forecast's
-- stage weights. Run inside a tenant-scoped transaction.

-- name: ListTargets :many
SELECT * FROM activity_target ORDER BY user_id NULLS FIRST, period;

-- name: DeleteTargets :exec
DELETE FROM activity_target;

-- name: InsertTarget :exec
INSERT INTO activity_target (tenant_id, user_id, period, calls, cvs_sent, interviews, placements, updated_by)
VALUES (app_current_tenant(), sqlc.narg(user_id), @period, sqlc.narg(calls), sqlc.narg(cvs_sent),
        sqlc.narg(interviews), sqlc.narg(placements), app_current_user());

-- name: ListStageWeights :many
SELECT * FROM forecast_stage_weight;

-- name: PutStageWeight :exec
INSERT INTO forecast_stage_weight (tenant_id, stage, percent, months_ahead, updated_by)
VALUES (app_current_tenant(), @stage, @percent, @months_ahead, app_current_user())
ON CONFLICT (tenant_id, stage) DO UPDATE SET
  percent = EXCLUDED.percent, months_ahead = EXCLUDED.months_ahead, updated_by = EXCLUDED.updated_by, updated_at = now();
