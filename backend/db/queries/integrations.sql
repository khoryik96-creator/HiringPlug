-- Queries owned by the integrations module. Run inside a tenant-scoped transaction.

-- name: ApiKeyByHash :one
SELECT key_id::text AS key_id, tenant_id::text AS tenant_id, key_name::text AS key_name, key_role::text AS key_role
FROM app_api_key(sqlc.arg(key_hash)::text);

-- name: ListApiKeys :many
SELECT * FROM api_key WHERE revoked_at IS NULL ORDER BY id DESC LIMIT 200;

-- name: InsertApiKey :one
INSERT INTO api_key (tenant_id, name, role, key_hash, prefix, created_by)
VALUES (app_current_tenant(), @name, @role, @key_hash, @prefix, app_current_user())
RETURNING *;

-- name: RevokeApiKey :execrows
UPDATE api_key SET revoked_at = now() WHERE id = @id AND revoked_at IS NULL;

-- name: ListWebhooks :many
SELECT * FROM webhook ORDER BY id DESC LIMIT 100;

-- name: GetWebhook :one
SELECT * FROM webhook WHERE id = @id;

-- name: InsertWebhook :one
INSERT INTO webhook (tenant_id, url, events, secret, description, created_by)
VALUES (app_current_tenant(), @url, @events::text[], @secret, sqlc.narg(description), app_current_user())
RETURNING *;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateWebhook :one
UPDATE webhook SET url = @url, events = @events::text[], active = @active, description = sqlc.narg(description),
  version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- name: DeleteWebhook :execrows
DELETE FROM webhook WHERE id = @id;

-- name: NewDeliveryID :one
SELECT new_id('whd')::text;

-- name: InsertDelivery :one
INSERT INTO webhook_delivery (id, tenant_id, webhook_id, event, payload)
VALUES (@id, app_current_tenant(), @webhook_id, @event, @payload)
RETURNING *;

-- name: ListDeliveries :many
SELECT * FROM webhook_delivery WHERE webhook_id = @webhook_id ORDER BY id DESC LIMIT 50;

-- A delivery with its webhook's address and secret (the worker).
-- name: GetDeliveryToSend :one
SELECT d.id, d.payload, d.status, d.attempts, d.event, w.url, w.secret, w.active
FROM webhook_delivery d JOIN webhook w ON w.tenant_id = d.tenant_id AND w.id = d.webhook_id
WHERE d.id = @id;

-- name: RecordAttempt :exec
UPDATE webhook_delivery SET status = @status, attempts = attempts + 1, response_status = sqlc.narg(response_status),
  last_error = sqlc.narg(last_error), next_attempt_at = sqlc.narg(next_attempt_at),
  delivered_at = CASE WHEN @status = 'delivered' THEN now() END
WHERE id = @id;

-- A secret saved before secrets were sealed is sealed the next time it is used.
-- name: SealWebhookSecret :exec
UPDATE webhook SET secret = @sealed WHERE secret = @plain;
