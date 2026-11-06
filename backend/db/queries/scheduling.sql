-- Queries owned by the scheduling module. Run inside a tenant-scoped transaction.

-- name: GetSchedule :one
SELECT * FROM schedule WHERE user_id = @user_id;

-- Locks the host's schedule while a booking is made, so two people cannot take the same time.
-- name: LockSchedule :one
SELECT * FROM schedule WHERE user_id = @user_id FOR UPDATE;

-- name: PutSchedule :one
INSERT INTO schedule (tenant_id, user_id, time_zone, hours, buffer_minutes, min_notice_hours, max_days_ahead)
VALUES (app_current_tenant(), @user_id, @time_zone, @hours, @buffer_minutes, @min_notice_hours, @max_days_ahead)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
  time_zone = excluded.time_zone, hours = excluded.hours, buffer_minutes = excluded.buffer_minutes,
  min_notice_hours = excluded.min_notice_hours, max_days_ahead = excluded.max_days_ahead, updated_at = now()
RETURNING *;

-- name: SetCalendarURL :exec
UPDATE schedule SET calendar_url = sqlc.narg(calendar_url),
  calendar_status = CASE WHEN sqlc.narg(calendar_url)::text IS NULL THEN NULL ELSE 'pending' END,
  calendar_error = NULL, calendar_read_at = NULL, calendar_asked_at = CASE WHEN sqlc.narg(calendar_url)::text IS NULL THEN NULL ELSE now() END
WHERE user_id = @user_id;

-- Marks a new read as queued, unless one was queued in the last 10 minutes.
-- name: AskCalendarRead :execrows
UPDATE schedule SET calendar_asked_at = now()
WHERE user_id = @user_id AND calendar_url IS NOT NULL
  AND (calendar_asked_at IS NULL OR calendar_asked_at < now() - interval '10 minutes');

-- name: SaveCalendarRead :exec
UPDATE schedule SET calendar_status = @calendar_status, calendar_error = sqlc.narg(calendar_error), calendar_read_at = now()
WHERE user_id = @user_id;

-- name: ClearBusy :exec
DELETE FROM calendar_busy WHERE user_id = @user_id;

-- name: InsertBusy :exec
INSERT INTO calendar_busy (tenant_id, user_id, starts_at, ends_at)
SELECT app_current_tenant(), @user_id, (@starts::timestamptz[])[i], (@ends::timestamptz[])[i]
FROM generate_subscripts(@starts::timestamptz[], 1) AS i;

-- name: CalendarBusy :many
SELECT starts_at, ends_at FROM calendar_busy WHERE user_id = @user_id AND ends_at > @from_time::timestamptz AND starts_at < @to_time::timestamptz;

-- name: CountBusy :one
SELECT count(*)::int FROM calendar_busy WHERE user_id = @user_id AND ends_at > now();

-- name: InsertBookingLink :one
INSERT INTO booking_link (tenant_id, token_hash, host_user_id, candidate_id, application_id, title, duration_minutes, location, expires_at, created_by)
VALUES (app_current_tenant(), @token_hash, @host_user_id, @candidate_id, sqlc.narg(application_id), @title, @duration_minutes, sqlc.narg(location), @expires_at, app_current_user())
RETURNING *;

-- name: BookingLinkByToken :one
SELECT * FROM booking_link WHERE token_hash = @token_hash;

-- name: LockBookingLink :one
SELECT * FROM booking_link WHERE token_hash = @token_hash FOR UPDATE;

-- name: GetBookingLink :one
SELECT * FROM booking_link WHERE id = @id FOR UPDATE;

-- name: CandidateBookingLinks :many
SELECT * FROM booking_link WHERE candidate_id = @candidate_id ORDER BY created_at DESC, id DESC LIMIT 50;

-- The host's other bookings: busy too.
-- name: HostBookings :many
SELECT starts_at, duration_minutes FROM booking_link
WHERE host_user_id = @host_user_id AND status = 'booked' AND id <> @except_id
  AND starts_at < @to_time::timestamptz AND starts_at + make_interval(mins => duration_minutes) > @from_time::timestamptz;

-- name: Book :one
UPDATE booking_link SET status = 'booked', starts_at = @starts_at, sequence = sequence + 1, updated_at = now()
WHERE id = @id RETURNING *;

-- name: CancelBookingLink :one
UPDATE booking_link SET status = 'cancelled', sequence = sequence + 1, updated_at = now()
WHERE id = @id RETURNING *;

-- The public page knows only the token: its company, outside any tenant scope.
-- name: BookingTenant :one
SELECT tenant_id::text AS tenant_id, tenant_name::text AS tenant_name FROM app_booking_tenant(@token_hash::text);
