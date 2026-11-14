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
-- With @role_not_set, only contacts without a role.
-- name: ListContacts :many
SELECT contact.id, contact.company_id, contact.first_name, contact.last_name, contact.title, contact.email,
  contact.email_check_status, contact.phone, contact.mobile, contact.status, contact.role, contact.updated_at,
  m.status AS mailbox_status, m.checked_at AS mailbox_checked_at, l.current_job AS linkedin_current, l.joined AS linkedin_joined, l.left_on AS linkedin_left,
  l.checked_at AS linkedin_checked_at
FROM contact LEFT JOIN contact_mailbox_check m ON m.contact_id = contact.id AND m.email = contact.email
  LEFT JOIN LATERAL (
    SELECT lc.current_job, lc.joined, lc.left_on, lc.checked_at FROM contact_linkedin_check lc
    WHERE lc.contact_id = contact.id AND lc.status = 'observed' ORDER BY lc.checked_at DESC LIMIT 1
  ) l ON true
WHERE (sqlc.narg(cursor)::text IS NULL OR contact.id < sqlc.narg(cursor)::text)
  AND (sqlc.narg(company_id)::text IS NULL OR contact.company_id = sqlc.narg(company_id)::text)
  AND (sqlc.narg(status)::text IS NULL OR contact.status = sqlc.narg(status)::text)
  AND (sqlc.narg(role)::text IS NULL OR contact.role = sqlc.narg(role)::text)
  AND (NOT sqlc.arg(role_not_set)::boolean OR contact.role IS NULL)
  AND (sqlc.narg(query)::text IS NULL
       OR CASE WHEN sqlc.arg(similar_names)::boolean
            THEN app_normalize_text(sqlc.narg(query)::text) <% contact.name_text
            ELSE contact.search_text LIKE ALL (app_search_patterns(sqlc.narg(query)::text))
              OR contact.phone_digits LIKE '%' || app_phone_key(sqlc.narg(query)::text) || '%'
          END)
ORDER BY CASE WHEN sqlc.arg(similar_names)::boolean THEN word_similarity(app_normalize_text(coalesce(sqlc.narg(query)::text, '')), contact.name_text) END DESC, contact.id DESC
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

-- How many contacts have a role (Settings refuses to remove one in use).
-- name: CountContactsWithRole :one
SELECT count(*) FROM contact WHERE role = @role;

-- Mailbox checks: one per contact, for its current email. A new check of
-- the same email within the control's lifetime keeps the made-up mailbox, so
-- every session asks about the same one; the last answer stays until the new one.
-- name: StartMailboxCheck :one
INSERT INTO contact_mailbox_check (tenant_id, contact_id, email, status, control_id, control_expires_at, requested_by)
VALUES (app_current_tenant(), @contact_id, @email, 'checking', @control_id, @control_expires_at, app_current_user())
ON CONFLICT (tenant_id, contact_id) DO UPDATE SET
  status = 'checking',
  control_id = CASE WHEN contact_mailbox_check.email = EXCLUDED.email AND contact_mailbox_check.control_expires_at > now()
    THEN contact_mailbox_check.control_id ELSE EXCLUDED.control_id END,
  control_expires_at = CASE WHEN contact_mailbox_check.email = EXCLUDED.email AND contact_mailbox_check.control_expires_at > now()
    THEN contact_mailbox_check.control_expires_at ELSE EXCLUDED.control_expires_at END,
  reason = CASE WHEN contact_mailbox_check.email = EXCLUDED.email THEN contact_mailbox_check.reason END,
  provider = CASE WHEN contact_mailbox_check.email = EXCLUDED.email THEN contact_mailbox_check.provider END,
  email = EXCLUDED.email,
  rechecks = 0,
  recheck_at = NULL,
  requested_by = EXCLUDED.requested_by,
  requested_at = now()
RETURNING *;

-- name: GetMailboxCheck :one
SELECT * FROM contact_mailbox_check WHERE contact_id = @contact_id;

-- name: SaveMailboxCheck :one
UPDATE contact_mailbox_check SET
  status = @status, reason = @reason, outcome = @outcome, provider = @provider, attempts = @attempts,
  rechecks = @rechecks, recheck_at = sqlc.narg(recheck_at), checked_at = now()
WHERE contact_id = @contact_id AND email = @email
RETURNING *;

-- name: CompanyContactsWithEmail :many
SELECT c.id, c.email::text AS email, m.status AS mailbox_status
FROM contact c LEFT JOIN contact_mailbox_check m ON m.contact_id = c.id AND m.email = c.email
WHERE c.company_id = @company_id AND c.email IS NOT NULL
ORDER BY c.id;

-- LinkedIn checks (by the recruiter's LinkedIn companion): one per contact
-- and request, answered only by whoever asked, within 15 minutes.
-- name: ContactsForLinkedin :many
SELECT c.id, c.company_id, c.first_name, c.last_name, c.linkedin_url, l.checked_at AS last_checked
FROM contact c LEFT JOIN LATERAL (
  SELECT lc.checked_at FROM contact_linkedin_check lc WHERE lc.contact_id = c.id AND lc.status = 'observed' ORDER BY lc.checked_at DESC LIMIT 1
) l ON true
WHERE (sqlc.narg(company_id)::text IS NULL OR c.company_id = sqlc.narg(company_id)::text)
  AND (sqlc.narg(contact_id)::text IS NULL OR c.id = sqlc.narg(contact_id)::text)
ORDER BY c.first_name, c.last_name, c.id;

-- name: InsertLinkedinCheck :one
INSERT INTO contact_linkedin_check (tenant_id, contact_id, company_id, profile_url, company_url, requested_by)
VALUES (app_current_tenant(), @contact_id, @company_id, @profile_url, @company_url, app_current_user())
RETURNING *;

-- name: GetLinkedinCheck :one
SELECT * FROM contact_linkedin_check WHERE id = @id;

-- name: SaveLinkedinCheck :one
UPDATE contact_linkedin_check SET
  status = @status, joined = sqlc.narg(joined), left_on = sqlc.narg(left_on), current_job = sqlc.narg(current_job),
  linkedin_company_id = sqlc.narg(linkedin_company_id), marked_left = @marked_left, moved_to = @moved_to, error = sqlc.narg(error), checked_at = now()
WHERE id = @id AND status = 'pending'
RETURNING *;

-- name: ExpireLinkedinCheck :exec
UPDATE contact_linkedin_check SET status = 'expired' WHERE id = @id AND status = 'pending';

-- name: LatestLinkedinCheck :one
SELECT * FROM contact_linkedin_check WHERE contact_id = @contact_id AND status IN ('observed', 'failed')
ORDER BY checked_at DESC, id DESC LIMIT 1;

-- A contact found to have left (an edit: the version changes).
-- name: MarkContactLeft :one
UPDATE contact SET status = 'left', version = version + 1, updated_at = now()
WHERE id = @id AND status = 'active' AND company_id = @company_id
RETURNING version;

-- A new made-up mailbox for a check after the old one's lifetime (the monthly re-check).
-- name: RenewMailboxControl :exec
UPDATE contact_mailbox_check SET control_id = @control_id, control_expires_at = @control_expires_at
WHERE contact_id = @contact_id AND email = @email AND control_expires_at <= now();
