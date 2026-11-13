-- Queries owned by the offers module. Run inside a tenant-scoped transaction.

-- Which of these applications have an offer (merging candidates keeps them).
-- name: ApplicationsWithOffers :many
SELECT DISTINCT application_id FROM offer WHERE application_id = ANY(@ids::text[]);

-- name: InsertOffer :one
INSERT INTO offer (tenant_id, application_id, start_date, expires_on, terms, created_by)
VALUES (app_current_tenant(), sqlc.arg(application_id), sqlc.narg(start_date), sqlc.narg(expires_on), sqlc.narg(offer_terms), app_current_user())
RETURNING *;

-- name: GetOffer :one
SELECT * FROM offer WHERE id = @id;

-- name: ListOffers :many
SELECT * FROM offer
WHERE (sqlc.narg(application_id)::text IS NULL OR application_id = sqlc.narg(application_id))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
ORDER BY id DESC
LIMIT 500;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateOffer :one
UPDATE offer SET status = @status, start_date = sqlc.narg(start_date), expires_on = sqlc.narg(expires_on), terms = sqlc.narg(terms),
  requested_by = sqlc.narg(requested_by), decided_by = sqlc.narg(decided_by), decided_at = sqlc.narg(decided_at),
  decision_note = sqlc.narg(decision_note), sent_at = sqlc.narg(sent_at), answered_at = sqlc.narg(answered_at),
  version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- Visible only with offers.salary:read (row-level security).
-- name: OfferSalaries :many
SELECT * FROM offer_salary WHERE offer_id = ANY(@ids::text[]);

-- Writable only with offers.salary:update (row-level security).
-- name: PutOfferSalary :exec
INSERT INTO offer_salary (offer_id, tenant_id, amount, currency, period, updated_by)
VALUES (@offer_id, app_current_tenant(), sqlc.arg(amount)::text::numeric, @currency, @period, app_current_user())
ON CONFLICT (offer_id) DO UPDATE SET amount = EXCLUDED.amount, currency = EXCLUDED.currency, period = EXCLUDED.period,
  updated_by = EXCLUDED.updated_by, updated_at = now();

-- name: DeleteOfferSalary :exec
DELETE FROM offer_salary WHERE offer_id = @offer_id;
