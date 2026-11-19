-- Queries owned by the claims module. Run inside a tenant-scoped transaction;
-- row-level security limits them to the person's own claims unless they may
-- approve claims (claims:approve).

-- name: NewClaimID :one
SELECT new_id('clm')::text;

-- name: InsertClaim :one
INSERT INTO claim (id, tenant_id, user_id, file_name, content_type, size_bytes, sha256, storage_key, currency)
VALUES (@id, app_current_tenant(), app_current_user(), @file_name, @content_type, @size_bytes, @sha256, @storage_key, @currency)
RETURNING *;

-- The person's claim with this exact receipt, if any.
-- name: MyClaimWithReceipt :one
SELECT id FROM claim WHERE user_id = app_current_user() AND sha256 = @sha256;

-- name: GetClaim :one
SELECT * FROM claim WHERE id = @id;

-- name: LockClaim :one
SELECT * FROM claim WHERE id = @id FOR UPDATE;

-- name: LockClaims :many
SELECT * FROM claim WHERE id = ANY (@ids::text[]) ORDER BY id FOR UPDATE;

-- name: ListMyClaims :many
SELECT * FROM claim
WHERE user_id = app_current_user()
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(from_date)::date IS NULL OR coalesce(receipt_date, created_at::date) >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR coalesce(receipt_date, created_at::date) <= sqlc.narg(to_date))
ORDER BY created_at DESC, id DESC
LIMIT @lim OFFSET @off;

-- name: MyClaimTotals :many
SELECT currency, count(*)::int AS claims, coalesce(sum(amount), 0)::numeric(14, 2)::text AS amount
FROM claim
WHERE user_id = app_current_user()
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(from_date)::date IS NULL OR coalesce(receipt_date, created_at::date) >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR coalesce(receipt_date, created_at::date) <= sqlc.narg(to_date))
GROUP BY currency
ORDER BY sum(amount) DESC NULLS LAST, currency;

-- Everyone's claims but drafts (an approver's view).
-- name: ListTeamClaims :many
SELECT * FROM claim
WHERE status <> 'draft'
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(user_id)::text IS NULL OR user_id = sqlc.narg(user_id))
  AND (sqlc.narg(from_date)::date IS NULL OR coalesce(receipt_date, created_at::date) >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR coalesce(receipt_date, created_at::date) <= sqlc.narg(to_date))
ORDER BY claimed_at DESC NULLS LAST, id DESC
LIMIT @lim OFFSET @off;

-- name: TeamClaimTotals :many
SELECT currency, count(*)::int AS claims, coalesce(sum(amount), 0)::numeric(14, 2)::text AS amount
FROM claim
WHERE status <> 'draft'
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(user_id)::text IS NULL OR user_id = sqlc.narg(user_id))
  AND (sqlc.narg(from_date)::date IS NULL OR coalesce(receipt_date, created_at::date) >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR coalesce(receipt_date, created_at::date) <= sqlc.narg(to_date))
GROUP BY currency
ORDER BY sum(amount) DESC NULLS LAST, currency;

-- What the claimant changed (only while it is a draft or rejected).
-- name: SaveClaimFields :one
UPDATE claim SET
  receipt_date = @receipt_date, amount = @amount, currency = @currency, merchant = @merchant,
  receipt_type = @receipt_type, claim_type = @claim_type, purpose = @purpose,
  link_kind = @link_kind, link_id = @link_id, from_receipt = @from_receipt::text[],
  version = version + 1, updated_at = now()
WHERE id = @id AND version = @version AND status IN ('draft', 'rejected')
RETURNING *;

-- What was read from the receipt, in the fields still empty. It is not a
-- change by anyone, so the version stays (an edit made meanwhile still saves).
-- name: SaveReceiptRead :one
UPDATE claim SET
  receipt_date = @receipt_date, amount = @amount, currency = @currency, merchant = @merchant,
  receipt_type = @receipt_type, claim_type = @claim_type, from_receipt = @from_receipt::text[],
  ocr_status = @ocr_status, ocr_error = @ocr_error, ocr_extractor = @ocr_extractor, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: SubmitClaim :one
UPDATE claim SET status = 'submitted', claimed_at = now(), reviewed_at = NULL, reviewed_by = NULL,
  version = version + 1, updated_at = now()
WHERE id = @id AND version = @version AND status IN ('draft', 'rejected')
RETURNING *;

-- name: WithdrawClaim :one
UPDATE claim SET status = 'draft', claimed_at = NULL, version = version + 1, updated_at = now()
WHERE id = @id AND version = @version AND status = 'submitted' AND user_id = app_current_user()
RETURNING *;

-- name: DecideClaim :one
UPDATE claim SET status = @status, reviewed_at = now(), reviewed_by = app_current_user(), review_note = sqlc.narg(review_note),
  version = version + 1, updated_at = now()
WHERE id = @id AND version = @version AND status = 'submitted'
RETURNING *;

-- name: PayClaim :one
UPDATE claim SET status = 'paid', paid_at = now(), version = version + 1, updated_at = now()
WHERE id = @id AND version = @version AND status = 'approved'
RETURNING *;

-- name: DeleteClaim :one
DELETE FROM claim
WHERE id = @id AND user_id = app_current_user() AND status IN ('draft', 'rejected')
RETURNING storage_key;

-- Counted and added up for the dashboard: the person's own claims, or
-- (team) everyone's but drafts.
-- name: ClaimFacts :many
SELECT status, receipt_type, claim_type, link_kind, link_id, user_id,
       to_char(coalesce(receipt_date, created_at::date), 'YYYY-MM') AS month, currency,
       count(*)::int AS claims, coalesce(sum(amount), 0)::numeric(14, 2)::text AS amount
FROM claim
WHERE (CASE WHEN @team::boolean THEN status <> 'draft' ELSE user_id = app_current_user() END)
  AND (sqlc.narg(from_date)::date IS NULL OR coalesce(receipt_date, created_at::date) >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR coalesce(receipt_date, created_at::date) <= sqlc.narg(to_date))
GROUP BY status, receipt_type, claim_type, link_kind, link_id, user_id, month, currency
ORDER BY month, status
LIMIT 5000;

-- A merged duplicate candidate's claims follow the candidate kept.
-- name: MoveCandidateClaims :exec
UPDATE claim SET link_id = @to_id WHERE link_kind = 'candidate' AND link_id = @from_id;

-- Claims holding a Settings option (an option in use is hidden, not removed).
-- name: CountClaimsWithReceiptType :one
SELECT count(*)::int FROM claim WHERE receipt_type = @key;

-- name: CountClaimsWithClaimType :one
SELECT count(*)::int FROM claim WHERE claim_type = @key;

-- name: CurrentClaimTenant :one
SELECT app_current_tenant()::text;
