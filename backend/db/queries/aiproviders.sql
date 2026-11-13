-- Queries owned by the aiproviders module. Run inside a tenant-scoped transaction.

-- name: ListProviders :many
SELECT provider, model, sealed_key, key_hint, check_status, check_message, checked_at, updated_at
FROM ai_provider ORDER BY provider;

-- name: GetProvider :one
SELECT provider, model, sealed_key, key_hint, check_status, check_message, checked_at, updated_at
FROM ai_provider WHERE provider = @provider;

-- name: SaveProvider :exec
INSERT INTO ai_provider (tenant_id, provider, model, sealed_key, key_hint, check_status, checked_at, updated_by)
VALUES (app_current_tenant(), @provider, @model, @sealed_key, @key_hint, 'queued', now(), app_current_user())
ON CONFLICT (tenant_id, provider) DO UPDATE SET
  model = excluded.model, sealed_key = excluded.sealed_key, key_hint = excluded.key_hint,
  check_status = 'queued', check_message = NULL, checked_at = now(),
  updated_by = excluded.updated_by, updated_at = now();

-- name: MarkChecking :exec
UPDATE ai_provider SET check_status = 'queued', check_message = NULL, checked_at = now()
WHERE provider = @provider;

-- A check's result, kept only if the key checked is still the saved one.
-- name: SaveCheck :execrows
UPDATE ai_provider SET check_status = @check_status, check_message = @check_message, checked_at = now()
WHERE provider = @provider AND sealed_key = @sealed_key;

-- name: DeleteProvider :execrows
DELETE FROM ai_provider WHERE provider = @provider;

-- name: GetChoice :one
SELECT provider FROM ai_provider_choice;

-- name: SetChoice :exec
INSERT INTO ai_provider_choice (tenant_id, provider, updated_by)
VALUES (app_current_tenant(), @provider, app_current_user())
ON CONFLICT (tenant_id) DO UPDATE SET provider = excluded.provider, updated_by = excluded.updated_by, updated_at = now();

-- The AI worker's word on the provider: it answered, or why it did not.
-- failing_since is when the failures began (cleared by an answer).
-- name: ReportProvider :exec
INSERT INTO ai_provider_status (tenant_id, provider, last_ok_at, failed_at, failing_since, error)
VALUES (app_current_tenant(), @provider,
        CASE WHEN @ok::boolean THEN @at::timestamptz END,
        CASE WHEN NOT @ok::boolean THEN @at::timestamptz END,
        CASE WHEN NOT @ok::boolean THEN @at::timestamptz END, sqlc.narg(error))
ON CONFLICT (tenant_id) DO UPDATE SET
  provider = excluded.provider,
  last_ok_at = coalesce(excluded.last_ok_at, ai_provider_status.last_ok_at),
  failed_at = coalesce(excluded.failed_at, ai_provider_status.failed_at),
  failing_since = CASE WHEN @ok::boolean THEN NULL ELSE coalesce(ai_provider_status.failing_since, excluded.failing_since) END,
  error = CASE WHEN @ok::boolean THEN NULL ELSE excluded.error END
WHERE @at::timestamptz >= greatest(coalesce(ai_provider_status.last_ok_at, '-infinity'), coalesce(ai_provider_status.failed_at, '-infinity'));

-- name: ProviderStatus :one
SELECT provider, last_ok_at, failed_at, failing_since, error FROM ai_provider_status;
