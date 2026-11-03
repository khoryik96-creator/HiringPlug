-- Queries owned by the leads module. Run inside a tenant-scoped transaction.

-- name: ListSources :many
SELECT * FROM lead_source ORDER BY lower(name), id LIMIT 500;

-- name: GetSource :one
SELECT * FROM lead_source WHERE id = @id;

-- name: InsertSource :one
INSERT INTO lead_source (tenant_id, kind, identifier, name, company_id, next_check_at, created_by)
VALUES (app_current_tenant(), @kind, @identifier, @name, sqlc.narg(company_id), @next_check_at, app_current_user())
RETURNING *;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateSource :one
UPDATE lead_source SET name = @name, company_id = sqlc.narg(company_id), active = @active,
  next_check_at = sqlc.narg(next_check_at), version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- name: ScheduleCheck :one
UPDATE lead_source SET next_check_at = @next_check_at WHERE id = @id RETURNING *;

-- name: RecordCheck :exec
UPDATE lead_source SET last_checked_at = now(), last_error = sqlc.narg(last_error), open_leads = @open_leads,
  next_check_at = sqlc.narg(next_check_at), name = @name
WHERE id = @id;

-- name: DeleteSource :execrows
DELETE FROM lead_source WHERE id = @id;

-- name: SourceLeads :many
SELECT * FROM job_lead WHERE source_id = @source_id;

-- name: InsertLead :one
INSERT INTO job_lead (tenant_id, source_id, source_kind, external_id, company_name, company_id, title, location, url, posted_at, notes, created_by)
VALUES (app_current_tenant(), sqlc.narg(source_id), sqlc.narg(source_kind), sqlc.narg(external_id), @company_name, sqlc.narg(company_id),
        @title, sqlc.narg(location), sqlc.narg(url), sqlc.narg(posted_at), sqlc.narg(notes), app_current_user())
RETURNING *;

-- An opening seen again: still listed (reopened if it had closed), with its latest details.
-- name: SeenLead :exec
UPDATE job_lead SET last_seen_at = now(), closed_at = NULL, title = @title, location = sqlc.narg(location), url = sqlc.narg(url),
  company_name = @company_name
WHERE id = @id;

-- name: CloseLead :exec
UPDATE job_lead SET closed_at = now() WHERE id = @id AND closed_at IS NULL;

-- name: GetLead :one
SELECT * FROM job_lead WHERE id = @id;

-- name: ListLeads :many
SELECT * FROM job_lead
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(source_id)::text IS NULL OR source_id = sqlc.narg(source_id))
  AND (sqlc.narg(open)::boolean IS NULL OR (closed_at IS NULL) = sqlc.narg(open))
  AND (sqlc.narg(q)::text IS NULL OR company_name ILIKE '%' || sqlc.narg(q) || '%' OR title ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY id DESC
LIMIT 500;

-- name: UpdateLead :one
UPDATE job_lead SET status = @status, notes = sqlc.narg(notes), company_id = sqlc.narg(company_id), version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;
