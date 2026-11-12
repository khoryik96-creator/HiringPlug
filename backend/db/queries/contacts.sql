-- Queries owned by the contacts module. Run inside a tenant-scoped
-- transaction: row-level security limits every query to the current tenant.

-- name: InsertContact :one
INSERT INTO contact (
  tenant_id, company_id, first_name, last_name, title, email,
  email_check_status, email_check_reasons, email_checked_at,
  phone, mobile, linkedin_url, status, role, notes, created_by
) VALUES (
  app_current_tenant(), @company_id, @first_name, @last_name, sqlc.narg(title), sqlc.narg(email),
  sqlc.narg(email_check_status), @email_check_reasons, sqlc.narg(email_checked_at),
  sqlc.narg(phone), sqlc.narg(mobile), sqlc.narg(linkedin_url), @status, sqlc.narg(role), sqlc.narg(notes), app_current_user()
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
  role = sqlc.narg(role),
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
-- With @similar, names like the query (typos) instead, the closest first.
-- A role of '' means contacts whose role is not set.
-- name: ListContacts :many
SELECT id, company_id, first_name, last_name, title, email, email_check_status, phone, mobile, status, role, updated_at
FROM contact
WHERE (sqlc.narg(cursor)::text IS NULL OR id < sqlc.narg(cursor)::text)
  AND (sqlc.narg(company_id)::text IS NULL OR company_id = sqlc.narg(company_id)::text)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(role)::text IS NULL OR coalesce(role, '') = sqlc.narg(role)::text)
  AND (sqlc.narg(query)::text IS NULL
       OR CASE WHEN sqlc.arg(similar_names)::boolean
            THEN app_normalize_text(sqlc.narg(query)::text) <% name_text
            ELSE search_text LIKE ALL (app_search_patterns(sqlc.narg(query)::text))
              OR phone_digits LIKE '%' || app_phone_key(sqlc.narg(query)::text) || '%'
          END)
ORDER BY CASE WHEN sqlc.arg(similar_names)::boolean THEN word_similarity(app_normalize_text(coalesce(sqlc.narg(query)::text, '')), name_text) END DESC, id DESC
LIMIT @row_limit;

-- name: ContactBriefs :many
SELECT id, first_name, last_name, company_id, email FROM contact WHERE id = ANY(@ids::text[]);

-- name: NewEmploymentCheckID :one
SELECT new_id('cec')::text;

-- name: InsertEmploymentCheck :one
INSERT INTO contact_employment_check (id, tenant_id, contact_id, file_name, storage_key, created_by)
VALUES (@id, app_current_tenant(), @contact_id, @file_name, @storage_key, app_current_user())
RETURNING *;

-- name: LatestEmploymentCheck :one
SELECT * FROM contact_employment_check WHERE contact_id = @contact_id ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: GetEmploymentCheck :one
SELECT * FROM contact_employment_check WHERE id = @id;

-- The read result; the stored PDF is no longer needed.
-- name: SaveEmploymentCheck :execrows
UPDATE contact_employment_check SET status = @status, error = @error, result = @result, storage_key = NULL
WHERE id = @id AND status = 'reading';

-- name: FillLinkedinURL :execrows
UPDATE contact SET linkedin_url = @linkedin_url, version = version + 1, updated_at = now()
WHERE id = @id AND linkedin_url IS NULL;
