-- auditlog module: who was shown a salary (salary_view). Only this module
-- touches this table. The audit log itself (audit_log) is read through
-- platform/audit.

-- One showing of a salary to the transaction's user (nothing for the
-- system): the day's line is added or counted again.
-- name: RecordSalaryView :exec
SELECT app_record_salary_view(@kind::text, sqlc.narg(candidate_id)::text, @candidate_ids::text[], @day::date, @at::timestamptz);

-- A candidate merge: the duplicate's salary views go to the kept candidate.
-- name: MoveSalaryViews :exec
SELECT app_move_salary_views(@from_id::text, @to_id::text);

-- Salary views, newest day first (and the newest line first within a day),
-- after where the previous page ended (its last line's day and id, which a
-- merge may since have taken away). A list or export line counts, and is found by,
-- only the candidates the reader sees (row-level security hides a line
-- with none of them).
-- name: ListSalaryViews :many
SELECT id, user_id, kind, candidate_id,
       (CASE WHEN candidate_id IS NOT NULL THEN 1
             WHEN app_scope_all() THEN cardinality(candidate_ids)
             ELSE (SELECT count(*) FROM unnest(candidate_ids) AS c (id) WHERE app_sees_candidate(c.id)) END)::integer AS candidates,
       day, count, first_at, last_at
FROM salary_view
WHERE (sqlc.narg(user_id)::text IS NULL OR user_id = sqlc.narg(user_id))
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind))
  AND (sqlc.narg(candidate_id)::text IS NULL OR candidate_id = sqlc.narg(candidate_id)
       OR (sqlc.narg(candidate_id) = ANY (candidate_ids) AND app_sees_candidate(sqlc.narg(candidate_id))))
  AND (sqlc.narg(from_day)::date IS NULL OR day >= sqlc.narg(from_day))
  AND (sqlc.narg(to_day)::date IS NULL OR day <= sqlc.narg(to_day))
  AND (sqlc.narg(after_day)::date IS NULL OR (day, id) < (sqlc.narg(after_day), @after_id::text))
ORDER BY day DESC, id DESC
LIMIT @row_limit;

