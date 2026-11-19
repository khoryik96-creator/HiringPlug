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
RETURNING storage_key, upload_provider, upload_account, upload_remote_id;

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

-- Claims storage: the company account receipts are uploaded to (owners and
-- admins; the worker as the system).

-- name: GetClaimStorage :one
SELECT * FROM claim_storage;

-- name: LockClaimStorage :one
SELECT * FROM claim_storage FOR UPDATE;

-- name: SaveClaimStorageFolder :one
INSERT INTO claim_storage (tenant_id, root_folder) VALUES (app_current_tenant(), @root_folder)
ON CONFLICT (tenant_id) DO UPDATE SET root_folder = excluded.root_folder, version = claim_storage.version + 1, updated_at = now()
  WHERE claim_storage.version = @version
RETURNING *;

-- name: ConnectClaimStorage :one
INSERT INTO claim_storage (tenant_id, provider, account, sealed_tokens, app_client_id, status, connected_at, connected_by)
VALUES (app_current_tenant(), @provider, @account, @sealed_tokens, @app_client_id, 'ok', now(), app_current_user())
ON CONFLICT (tenant_id) DO UPDATE SET provider = excluded.provider, account = excluded.account,
  sealed_tokens = excluded.sealed_tokens, app_client_id = excluded.app_client_id, status = 'ok', error = NULL,
  connected_at = now(), connected_by = app_current_user(), version = claim_storage.version + 1, updated_at = now()
RETURNING *;

-- name: SaveClaimStorageTokens :execrows
UPDATE claim_storage SET sealed_tokens = @sealed_tokens
WHERE provider = @provider AND account = @account;

-- name: MarkClaimStorage :execrows
UPDATE claim_storage SET status = @status, error = sqlc.narg(error), version = version + 1, updated_at = now()
WHERE provider = @provider AND account = @account AND (status <> @status OR error IS DISTINCT FROM sqlc.narg(error));

-- name: DisconnectClaimStorage :exec
UPDATE claim_storage SET provider = NULL, account = NULL, sealed_tokens = NULL, app_client_id = NULL,
  status = 'disconnected', error = NULL, connected_at = NULL, connected_by = NULL, version = version + 1, updated_at = now();

-- name: ClaimUploadCounts :one
SELECT count(*) FILTER (WHERE upload_status = 'pending')::int AS pending,
       count(*) FILTER (WHERE upload_status = 'failed')::int AS failed,
       count(*) FILTER (WHERE upload_status = 'done')::int AS done
FROM claim;

-- name: SaveClaimStorageState :exec
INSERT INTO claim_storage_state (tenant_id, user_id, provider, state_hash, app_client_id, sealed_verifier, expires_at)
VALUES (app_current_tenant(), app_current_user(), @provider, @state_hash, @app_client_id, @sealed_verifier, @expires_at)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET provider = excluded.provider, state_hash = excluded.state_hash,
  app_client_id = excluded.app_client_id, sealed_verifier = excluded.sealed_verifier, expires_at = excluded.expires_at,
  sealed_code = NULL, ticket_hash = NULL, consumed = false;

-- name: ParkClaimStorageCode :execrows
UPDATE claim_storage_state SET consumed = true, sealed_code = sqlc.narg(sealed_code), ticket_hash = sqlc.narg(ticket_hash)
WHERE state_hash = @state_hash AND NOT consumed AND expires_at > now();

-- name: ParkedClaimStorageCode :one
SELECT * FROM claim_storage_state WHERE ticket_hash = @ticket_hash AND consumed AND expires_at > now();

-- name: TakeClaimStorageCode :execrows
DELETE FROM claim_storage_state WHERE state_hash = @state_hash AND consumed AND expires_at > now();

-- Uploads. They change no claim version: the claimant's edits are not
-- turned stale by an upload finishing.

-- name: QueueClaimUpload :one
UPDATE claim SET upload_status = 'pending', upload_error = NULL, upload_sequence = upload_sequence + 1
WHERE id = @id RETURNING upload_sequence;

-- name: ClaimsToUploadAgain :many
SELECT id FROM claim
WHERE upload_status IN ('pending', 'failed') AND status IN ('submitted', 'approved', 'paid')
ORDER BY claimed_at, id LIMIT 1000;

-- name: SaveClaimUpload :execrows
UPDATE claim SET upload_status = 'done', upload_path = @upload_path, upload_url = sqlc.narg(upload_url), upload_error = NULL,
  uploaded_at = now(), upload_provider = @upload_provider, upload_account = @upload_account, upload_remote_id = @upload_remote_id
WHERE id = @id AND upload_sequence = @upload_sequence;

-- name: FailClaimUpload :execrows
UPDATE claim SET upload_status = 'failed', upload_error = @upload_error
WHERE id = @id AND upload_sequence = @upload_sequence AND upload_status = 'pending';

-- name: ForgetClaimUpload :exec
UPDATE claim SET upload_remote_id = NULL
WHERE id = @id AND upload_remote_id = @upload_remote_id;

-- name: ResetClaimUpload :exec
UPDATE claim SET upload_status = 'none', upload_error = NULL
WHERE id = @id AND upload_sequence = @upload_sequence AND upload_status = 'pending';
