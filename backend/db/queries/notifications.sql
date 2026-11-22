-- Queries owned by the notifications module. Run inside a tenant-scoped transaction.

-- A user's state (no row: never looked, no morning email).
-- name: GetState :one
SELECT * FROM notification_state WHERE user_id = @user_id;

-- Seen up to the time of the list the user was shown (not later: what came
-- after it stays new), never moving back.
-- name: MarkSeen :exec
INSERT INTO notification_state (tenant_id, user_id, seen_at)
VALUES (app_current_tenant(), @user_id, least(now(), @up_to::timestamptz))
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
  seen_at = greatest(coalesce(notification_state.seen_at, '-infinity'), EXCLUDED.seen_at), updated_at = now();

-- Restarting the morning email starts a new chain (so a job still waiting
-- from an earlier one does nothing) due at due_at.
-- name: PutSettings :one
INSERT INTO notification_state (tenant_id, user_id, morning_email, muted_kinds, digest_chain, digest_due_at)
VALUES (app_current_tenant(), @user_id, @morning_email, @muted_kinds::text[],
        CASE WHEN @restart::boolean THEN 1 ELSE 0 END, sqlc.narg(due_at))
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
  morning_email = EXCLUDED.morning_email,
  muted_kinds = EXCLUDED.muted_kinds,
  digest_chain = notification_state.digest_chain + CASE WHEN @restart::boolean THEN 1 ELSE 0 END,
  digest_due_at = CASE WHEN @restart::boolean THEN EXCLUDED.digest_due_at ELSE notification_state.digest_due_at END,
  updated_at = now()
RETURNING *;

-- The row, locked: settings saved at once, or two runs of one day's job (a
-- retry), wait for each other.
-- name: LockState :one
SELECT * FROM notification_state WHERE user_id = @user_id FOR UPDATE;

-- A morning's job ran: the next is due at due_at; emailed is the day emailed (if one was).
-- name: DigestRan :exec
UPDATE notification_state
SET digest_due_at = @due_at, emailed_on = coalesce(sqlc.narg(emailed)::date, emailed_on), updated_at = now()
WHERE user_id = @user_id;
