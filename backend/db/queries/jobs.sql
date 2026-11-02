-- Queries owned by the jobs module. Run inside a tenant-scoped transaction:
-- row-level security limits every query to the current tenant, and the
-- salary table also to users with the salary permissions.

-- name: InsertJobOrder :one
INSERT INTO job_order (
  tenant_id, company_id, contact_id, title, status, location, work_type, work_arrangement,
  openings, description, created_by
) VALUES (
  app_current_tenant(), @company_id, sqlc.narg(contact_id), @title, @status, sqlc.narg(location),
  sqlc.narg(work_type), sqlc.narg(work_arrangement), @openings, sqlc.narg(description), app_current_user()
)
RETURNING *;

-- name: GetJobOrder :one
SELECT * FROM job_order WHERE id = @id;

-- name: JobOrderExists :one
SELECT EXISTS (SELECT 1 FROM job_order WHERE id = @id);

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateJobOrder :one
UPDATE job_order SET
  company_id = @company_id,
  contact_id = sqlc.narg(contact_id),
  title = @title,
  status = @status,
  location = sqlc.narg(location),
  work_type = sqlc.narg(work_type),
  work_arrangement = sqlc.narg(work_arrangement),
  openings = @openings,
  description = sqlc.narg(description),
  version = version + 1,
  updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- Newest first, keyset pagination on id (IDs sort by creation time).
-- name: ListJobOrders :many
SELECT id, title, company_id, status, location, openings, updated_at
FROM job_order
WHERE (sqlc.narg(cursor)::text IS NULL OR id < sqlc.narg(cursor)::text)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(company_id)::text IS NULL OR company_id = sqlc.narg(company_id)::text)
  AND (sqlc.narg(query)::text IS NULL OR search_text LIKE '%' || app_normalize_text(sqlc.narg(query)::text) || '%')
ORDER BY id DESC
LIMIT @row_limit;

-- name: JobOrderTitles :many
SELECT id, title, company_id FROM job_order WHERE id = ANY(@ids::text[]);

-- Amounts are read as exact decimal text (numeric -> text), never floats;
-- an empty max_amount means a single figure.
-- name: GetJobSalary :one
SELECT version, min_amount::text AS min_amount, coalesce(max_amount::text, '')::text AS max_amount, currency, period, updated_at
FROM job_order_salary WHERE job_order_id = @job_order_id;

-- name: InsertJobSalary :one
INSERT INTO job_order_salary (job_order_id, tenant_id, min_amount, max_amount, currency, period, updated_by)
VALUES (
  @job_order_id, app_current_tenant(), sqlc.arg(min_amount)::text::numeric, sqlc.narg(max_amount)::text::numeric,
  @currency, @period, app_current_user()
)
ON CONFLICT (job_order_id) DO NOTHING
RETURNING version, min_amount::text AS min_amount, coalesce(max_amount::text, '')::text AS max_amount, currency, period, updated_at;

-- name: UpdateJobSalary :one
UPDATE job_order_salary SET
  min_amount = sqlc.arg(min_amount)::text::numeric,
  max_amount = sqlc.narg(max_amount)::text::numeric,
  currency = @currency,
  period = @period,
  version = version + 1,
  updated_by = app_current_user(),
  updated_at = now()
WHERE job_order_id = @job_order_id AND version = @expected_version
RETURNING version, min_amount::text AS min_amount, coalesce(max_amount::text, '')::text AS max_amount, currency, period, updated_at;

-- Clearing the budget: no row means no budget.
-- name: DeleteJobSalary :execrows
DELETE FROM job_order_salary WHERE job_order_id = @job_order_id AND version = @expected_version;
