-- Queries owned by the placements module. Run inside a tenant-scoped transaction.

-- name: InsertPlacement :one
INSERT INTO placement (tenant_id, application_id, recruiter_id, start_date, guarantee_days, created_by)
VALUES (app_current_tenant(), @application_id, sqlc.narg(recruiter_id), @start_date, @guarantee_days, app_current_user())
RETURNING *;

-- name: GetPlacement :one
SELECT * FROM placement WHERE id = @id;

-- name: ListPlacements :many
SELECT * FROM placement
WHERE (sqlc.narg(application_id)::text IS NULL OR application_id = sqlc.narg(application_id))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(invoice_status)::text IS NULL OR invoice_status = sqlc.narg(invoice_status))
  AND (sqlc.narg(start_from)::date IS NULL OR start_date >= sqlc.narg(start_from))
  AND (sqlc.narg(start_to)::date IS NULL OR start_date <= sqlc.narg(start_to))
ORDER BY start_date DESC, id DESC
LIMIT 500;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdatePlacement :one
UPDATE placement SET recruiter_id = sqlc.narg(recruiter_id), start_date = @start_date, guarantee_days = @guarantee_days,
  status = @status, left_on = sqlc.narg(left_on), status_note = sqlc.narg(status_note),
  invoice_status = @invoice_status, invoice_number = sqlc.narg(invoice_number),
  invoiced_on = sqlc.narg(invoiced_on), paid_on = sqlc.narg(paid_on),
  version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- Visible only with placements.fees:read (row-level security).
-- name: PlacementMoney :many
SELECT * FROM placement_money WHERE placement_id = ANY(@ids::text[]);

-- Writable only with placements.fees:update (row-level security).
-- name: PutPlacementMoney :exec
INSERT INTO placement_money (placement_id, tenant_id, salary_amount, salary_currency, salary_period,
                             fee_percent, fee_amount, fee_currency, updated_by)
VALUES (@placement_id, app_current_tenant(), sqlc.narg(salary_amount)::text::numeric, sqlc.narg(salary_currency), sqlc.narg(salary_period),
        sqlc.narg(fee_percent)::text::numeric, sqlc.narg(fee_amount)::text::numeric, sqlc.narg(fee_currency), app_current_user())
ON CONFLICT (placement_id) DO UPDATE SET
  salary_amount = EXCLUDED.salary_amount, salary_currency = EXCLUDED.salary_currency, salary_period = EXCLUDED.salary_period,
  fee_percent = EXCLUDED.fee_percent, fee_amount = EXCLUDED.fee_amount, fee_currency = EXCLUDED.fee_currency,
  updated_by = EXCLUDED.updated_by, updated_at = now();

-- name: DeletePlacementMoney :exec
DELETE FROM placement_money WHERE placement_id = @placement_id;

-- Placements starting in a period, with their fee when the user may see it
-- (row-level security leaves the money out otherwise) (reports).
-- name: PlacementFacts :many
SELECT p.id, p.recruiter_id, p.status, p.start_date, m.fee_amount, m.fee_currency
FROM placement p LEFT JOIN placement_money m ON m.tenant_id = p.tenant_id AND m.placement_id = p.id
WHERE p.start_date >= @from_day AND p.start_date <= @to_day
ORDER BY p.start_date
LIMIT 20000;
