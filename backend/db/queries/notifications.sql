-- Queries owned by the notifications module. Run inside a tenant-scoped transaction.

-- A user's state (no row: never looked, no morning email).
-- name: GetState :one
SELECT * FROM notification_state WHERE user_id = @user_id;

-- name: MarkSeen :exec
INSERT INTO notification_state (tenant_id, user_id, seen_at)
VALUES (app_current_tenant(), @user_id, now())
ON CONFLICT (tenant_id, user_id) DO UPDATE SET seen_at = now(), updated_at = now();

-- Turning the morning email on starts a new chain (so a job still waiting
-- from an earlier one does nothing); turning it off keeps the chain number.
-- name: PutSettings :one
INSERT INTO notification_state (tenant_id, user_id, morning_email, muted_kinds, digest_chain)
VALUES (app_current_tenant(), @user_id, @morning_email, @muted_kinds::text[], CASE WHEN @morning_email::boolean THEN 1 ELSE 0 END)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
  morning_email = EXCLUDED.morning_email,
  muted_kinds = EXCLUDED.muted_kinds,
  digest_chain = notification_state.digest_chain
    + CASE WHEN EXCLUDED.morning_email AND NOT notification_state.morning_email THEN 1 ELSE 0 END,
  updated_at = now()
RETURNING *;

-- The morning email's job takes the row first, so two runs of one day's
-- job (a retry) wait for each other.
-- name: LockState :one
SELECT * FROM notification_state WHERE user_id = @user_id FOR UPDATE;

-- name: MarkEmailed :exec
UPDATE notification_state SET emailed_on = @day::date, updated_at = now() WHERE user_id = @user_id;
