-- Queries owned by the contacts module. Run inside a tenant-scoped
-- transaction: row-level security limits every query to the current tenant.

-- name: InsertContact :one
INSERT INTO contact (
  tenant_id, company_id, first_name, last_name, title, email,
  email_check_status, email_check_reasons, email_checked_at,
  phone, mobile, linkedin_url, status, notes, created_by
) VALUES (
  app_current_tenant(), @company_id, @first_name, @last_name, sqlc.narg(title), sqlc.narg(email),
  sqlc.narg(email_check_status), @email_check_reasons, sqlc.narg(email_checked_at),
  sqlc.narg(phone), sqlc.narg(mobile), sqlc.narg(linkedin_url), @status, sqlc.narg(notes), app_current_user()
)
RETURNING *;

-- name: GetContact :one
SELECT * FROM contact WHERE id = @id;

-- name: ContactExists :one
SELECT EXISTS (SELECT 1 FROM contact WHERE id = @id);

-- name: FindContactIDByEmail :one
SELECT id FROM contact WHERE email = @email;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateContact :one
UPDATE contact SET
  company_id = @company_id,
  first_name = @first_name,
  last_name = @last_name,
  title = sqlc.narg(title),
  email = sqlc.narg(email),
  email_check_status = sqlc.narg(email_check_status),
  email_check_reasons = @email_check_reasons,
  email_checked_at = sqlc.narg(email_checked_at),
  phone = sqlc.narg(phone),
  mobile = sqlc.narg(mobile),
  linkedin_url = sqlc.narg(linkedin_url),
  status = @status,
  notes = sqlc.narg(notes),
  version = version + 1,
  updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- A re-check only replaces the result; it is not an edit (no version change).
-- name: SetEmailCheck :one
UPDATE contact SET
  email_check_status = @email_check_status,
  email_check_reasons = @email_check_reasons,
  email_checked_at = @email_checked_at
WHERE id = @id AND email = @email
RETURNING *;

-- Newest first, keyset pagination on id (IDs sort by creation time).
-- name: ListContacts :many
SELECT id, company_id, first_name, last_name, title, email, email_check_status, phone, mobile, status, updated_at
FROM contact
WHERE (sqlc.narg(cursor)::text IS NULL OR id < sqlc.narg(cursor)::text)
  AND (sqlc.narg(company_id)::text IS NULL OR company_id = sqlc.narg(company_id)::text)
  AND (sqlc.narg(query)::text IS NULL OR search_text LIKE '%' || app_normalize_text(sqlc.narg(query)::text) || '%')
ORDER BY id DESC
LIMIT @row_limit;

-- name: ContactBriefs :many
SELECT id, first_name, last_name, company_id FROM contact WHERE id = ANY(@ids::text[]);
