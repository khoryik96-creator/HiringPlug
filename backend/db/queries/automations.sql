-- Queries owned by the automations module. Run inside a tenant-scoped transaction.

-- name: GetSettings :one
SELECT * FROM automation_settings WHERE tenant_id = app_current_tenant();

-- name: EnsureSettings :one
INSERT INTO automation_settings (tenant_id) VALUES (app_current_tenant())
ON CONFLICT (tenant_id) DO UPDATE SET tenant_id = EXCLUDED.tenant_id
RETURNING *;

-- name: PutSettings :one
UPDATE automation_settings SET default_send_mode = @default_send_mode, updated_at = now()
WHERE tenant_id = app_current_tenant() RETURNING *;

-- Only once per company (seeded false -> true).
-- name: MarkSeeded :execrows
UPDATE automation_settings SET seeded = true WHERE tenant_id = app_current_tenant() AND NOT seeded;

-- name: InsertAutomation :one
INSERT INTO automation (tenant_id, name, enabled, trigger, stage, offset_days, company_id, actions, send_mode, preset, created_by)
VALUES (app_current_tenant(), @name, @enabled, @trigger, sqlc.narg(stage), @offset_days, sqlc.narg(company_id), @actions, @send_mode,
        sqlc.narg(preset), coalesce(app_current_user(), 'system'))
RETURNING *;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateAutomation :one
UPDATE automation SET name = @name, enabled = @enabled, trigger = @trigger, stage = sqlc.narg(stage), offset_days = @offset_days,
  company_id = sqlc.narg(company_id), actions = @actions, send_mode = @send_mode, version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- name: GetAutomation :one
SELECT * FROM automation WHERE id = @id;

-- name: DeleteAutomation :execrows
DELETE FROM automation WHERE id = @id;

-- name: ListAutomations :many
SELECT * FROM automation ORDER BY created_at, id LIMIT 200;

-- name: EnabledAutomations :many
SELECT * FROM automation WHERE enabled ORDER BY id;

-- A run per automation and record: a new date moves a run still waiting;
-- one that already ran stays as it is (no row returned).
-- name: ScheduleRun :one
INSERT INTO automation_run (tenant_id, automation_id, subject_type, subject_id, application_id, due_at)
VALUES (app_current_tenant(), @automation_id, @subject_type, @subject_id, @application_id, @due_at)
ON CONFLICT (automation_id, subject_id) DO UPDATE SET due_at = EXCLUDED.due_at
  WHERE automation_run.status = 'scheduled' AND automation_run.due_at <> EXCLUDED.due_at
RETURNING *;

-- name: GetRun :one
SELECT * FROM automation_run WHERE id = @id;

-- name: LockRun :one
SELECT * FROM automation_run WHERE id = @id FOR UPDATE;

-- name: FinishRun :one
UPDATE automation_run SET status = @status, detail = sqlc.narg(detail),
  finished_at = CASE WHEN @status::text = 'to_approve' THEN NULL ELSE now() END,
  finished_by = sqlc.narg(finished_by)
WHERE id = @id
RETURNING *;

-- name: ListRuns :many
SELECT r.*, a.name AS automation_name, a.actions
FROM automation_run r JOIN automation a ON a.tenant_id = r.tenant_id AND a.id = r.automation_id
WHERE (sqlc.narg(status)::text IS NULL OR r.status = sqlc.narg(status)::text)
ORDER BY CASE WHEN r.status = 'scheduled' THEN r.due_at END, r.id DESC
LIMIT 200;

-- name: InsertTask :one
INSERT INTO recruiter_task (tenant_id, user_id, title, due_on, application_id, run_id, automation_name)
VALUES (app_current_tenant(), @user_id, @title, @due_on, sqlc.narg(application_id), sqlc.narg(run_id), sqlc.narg(automation_name))
RETURNING *;

-- A user's tasks: open ones, and those done in the last 7 days.
-- name: ListTasks :many
SELECT * FROM recruiter_task
WHERE user_id = @user_id AND (done_at IS NULL OR done_at > now() - interval '7 days')
ORDER BY done_at IS NOT NULL, due_on, id
LIMIT 200;

-- name: SetTaskDone :one
UPDATE recruiter_task SET done_at = CASE WHEN @done::boolean THEN coalesce(done_at, now()) END
WHERE id = @id AND user_id = @user_id
RETURNING *;
