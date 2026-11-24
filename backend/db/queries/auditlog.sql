-- auditlog module: who was shown a salary (salary_view). Only this module
-- touches this table. The audit log itself (audit_log) is read through
-- platform/audit.

-- One showing of a salary to the transaction's user (nothing for the
-- system): the day's line is added or counted again.
-- name: RecordSalaryView :exec
SELECT app_record_salary_view(@kind::text, sqlc.narg(candidate_id)::text, @candidate_ids::text[], @day::date, @at::timestamptz);

-- Salary views, newest day first (and the newest line first within a day),
-- after the cursor's line.
-- name: ListSalaryViews :many
SELECT id, user_id, kind, candidate_id,
       (CASE WHEN candidate_id IS NULL THEN cardinality(candidate_ids) ELSE 1 END)::integer AS candidates, day, count, first_at, last_at
FROM salary_view
WHERE (sqlc.narg(user_id)::text IS NULL OR user_id = sqlc.narg(user_id))
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind))
  AND (sqlc.narg(candidate_id)::text IS NULL OR candidate_id = sqlc.narg(candidate_id) OR sqlc.narg(candidate_id) = ANY (candidate_ids))
  AND (sqlc.narg(from_day)::date IS NULL OR day >= sqlc.narg(from_day))
  AND (sqlc.narg(to_day)::date IS NULL OR day <= sqlc.narg(to_day))
  AND (sqlc.narg(cursor)::text IS NULL OR (day, id) < (SELECT c.day, c.id FROM salary_view c WHERE c.id = sqlc.narg(cursor)))
ORDER BY day DESC, id DESC
LIMIT @row_limit;

