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
  l.checked_at AS linkedin_checked_at, coalesce(l.matched_by_name, false)::boolean AS linkedin_matched_by_name
FROM contact LEFT JOIN contact_mailbox_check m ON m.contact_id = contact.id AND m.email = contact.email
  LEFT JOIN LATERAL (
    SELECT lc.current_job, lc.joined, lc.left_on, lc.checked_at, lc.matched_by_name FROM contact_linkedin_check lc
    WHERE lc.contact_id = contact.id AND lc.status = 'observed' ORDER BY lc.checked_at DESC LIMIT 1
  ) l ON true
WHERE (sqlc.narg(cursor)::text IS NULL OR contact.id < sqlc.narg(cursor)::text)
  AND (sqlc.narg(company_id)::text IS NULL OR contact.company_id = sqlc.narg(company_id)::text)
  AND (sqlc.narg(status)::text IS NULL OR contact.status = sqlc.narg(status)::text)
  AND (sqlc.narg(role)::text IS NULL OR contact.role = sqlc.narg(role)::text)
  AND (NOT sqlc.arg(role_not_set)::boolean OR contact.role IS NULL)
  AND (sqlc.narg(in_ids)::text[] IS NULL OR contact.id = ANY (sqlc.narg(in_ids)::text[])) -- a folder's
  AND (sqlc.narg(query)::text IS NULL
       OR CASE WHEN sqlc.arg(similar_names)::boolean
            THEN app_normalize_text(sqlc.narg(query)::text) <% contact.name_text
            ELSE contact.search_text LIKE ALL (app_search_patterns(sqlc.narg(query)::text))
              OR contact.phone_digits LIKE '%' || app_phone_key(sqlc.narg(query)::text) || '%'
          END)
  -- A boolean search (boolsearch's query for to_tsquery).
  -- The expression of contact_search_doc_idx (00115), so the index answers.
  AND (sqlc.narg(tsquery)::text IS NULL
       OR app_search_doc(contact.first_name || ' ' || contact.last_name, contact.title, contact.email) @@ to_tsquery('english', sqlc.narg(tsquery)::text))
ORDER BY CASE WHEN sqlc.arg(similar_names)::boolean THEN word_similarity(app_normalize_text(coalesce(sqlc.narg(query)::text, '')), contact.name_text) END DESC, contact.id DESC
LIMIT @row_limit;

-- name: ContactBriefs :many
SELECT id, first_name, last_name, company_id, email, phone, mobile FROM contact WHERE id = ANY(@ids::text[]);

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
  linkedin_company_id = sqlc.narg(linkedin_company_id), marked_left = @marked_left, matched_by_name = @matched_by_name,
  moved_to = @moved_to, error = sqlc.narg(error), checked_at = now()
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

-- Contacts at these addresses (any letter case) who have left, or whose
-- mailbox does not exist: an email to them would not reach them.
-- name: UndeliverableContacts :many
SELECT c.id, c.company_id, c.first_name, c.last_name, c.email::text AS email, c.status, m.status AS mailbox_status
FROM contact c LEFT JOIN contact_mailbox_check m ON m.contact_id = c.id AND m.email = c.email
WHERE lower(c.email) = ANY(@emails::text[]) AND (c.status = 'left' OR m.status = 'invalid');

-- Colleagues' addresses, except those found not to work (a check or a real
-- bounce): how the company spells its addresses.
-- name: ColleagueAddresses :many
SELECT c.first_name, c.last_name, c.email::text AS email
FROM contact c LEFT JOIN contact_mailbox_check m ON m.contact_id = c.id AND m.email = c.email
WHERE c.company_id = @company_id AND c.id <> @contact_id AND c.email IS NOT NULL AND m.status IS DISTINCT FROM 'invalid'
ORDER BY c.id LIMIT 500;

-- A real email to the address bounced: its mailbox is gone (stronger than any
-- check, and kept by the monthly re-checks).
-- name: MarkMailboxBounced :many
INSERT INTO contact_mailbox_check (tenant_id, contact_id, email, status, reason, outcome, control_id, control_expires_at, requested_by, checked_at)
SELECT c.tenant_id, c.id, c.email, 'invalid', @reason, 'bounced', @control_id, now(), 'mail-bounce', now()
FROM contact c WHERE lower(c.email) = @email::text
ON CONFLICT (tenant_id, contact_id) DO UPDATE SET
  email = EXCLUDED.email, status = 'invalid', reason = EXCLUDED.reason, outcome = 'bounced',
  rechecks = 0, recheck_at = NULL, checked_at = now()
RETURNING contact_id;

-- name: CountMailboxesChecking :one
SELECT count(*)::int FROM contact_mailbox_check WHERE status = 'checking';

-- The Bin (bin module): binned contacts are hidden by the database; these run with app.show_binned on.
-- name: BinContacts :many
UPDATE contact SET binned_at = now() WHERE id = ANY(@ids::text[]) AND binned_at IS NULL
RETURNING id, btrim(first_name || ' ' || last_name)::text AS label;

-- name: RestoreContacts :execrows
UPDATE contact SET binned_at = NULL WHERE id = ANY(@ids::text[]) AND binned_at IS NOT NULL;

-- name: PurgeContacts :execrows
DELETE FROM contact WHERE id = ANY(@ids::text[]) AND binned_at IS NOT NULL;

-- name: ContactIDsOfCompany :many
SELECT id FROM contact WHERE company_id = @company_id AND binned_at IS NULL ORDER BY id;

-- name: MoveContactsToCompany :execrows
UPDATE contact SET company_id = @to_id, version = version + 1, updated_at = now()
WHERE company_id = @from_id AND binned_at IS NULL;

-- name: ContactLabel :one
SELECT btrim(first_name || ' ' || last_name)::text AS label FROM contact WHERE id = @id;

-- Find HR: one LinkedIn people scan per company.
-- name: GetHrScan :one
SELECT * FROM hr_scan WHERE company_id = @company_id;

-- name: LockHrScan :one
SELECT * FROM hr_scan WHERE company_id = @company_id FOR UPDATE;

-- name: StartHrScan :one
INSERT INTO hr_scan (tenant_id, company_id, linkedin_company_id, linkedin_company_name, linkedin_company_url, country, keywords, pages, started_by)
VALUES (app_current_tenant(), @company_id, @linkedin_company_id, @linkedin_company_name, sqlc.narg(linkedin_company_url), sqlc.narg(country), @keywords, @pages, app_current_user())
ON CONFLICT (tenant_id, company_id) DO UPDATE SET
  linkedin_company_id = EXCLUDED.linkedin_company_id, linkedin_company_name = EXCLUDED.linkedin_company_name,
  linkedin_company_url = EXCLUDED.linkedin_company_url,
  country = EXCLUDED.country, keywords = EXCLUDED.keywords, pages = EXCLUDED.pages, status = 'running',
  next_query = 0, next_page = 1, pages_read = 0, error = NULL, people = '[]'::jsonb,
  started_by = EXCLUDED.started_by, started_at = now(), updated_at = now()
RETURNING *;

-- name: SaveHrScan :one
UPDATE hr_scan SET status = @status, next_query = @next_query, next_page = @next_page, pages_read = @pages_read,
  error = sqlc.narg(error), people = @people, updated_at = now()
WHERE company_id = @company_id
RETURNING *;

-- Contacts who are, or may be, the people a Find HR scan found, anywhere in
-- the agency: the same LinkedIn profile (@urls in lower case, as stored); the
-- same name words in any order (app_person_name_words: titles, bin/binti and
-- the like aside); or, for names of two words or more, one name inside the
-- other ("Aisyah Ahmad", "Nur Aisyah binti Ahmad"). i is the person's place
-- in @names, from 1.
-- name: ContactsLike :many
WITH person AS (
  SELECT u.i::int AS i, (@urls::text[])[u.i] AS url, app_person_name_words(u.name) AS words
  FROM unnest(@names::text[]) WITH ORDINALITY AS u(name, i)
), found AS (
  SELECT p.i, c.id FROM person p JOIN contact c ON lower(rtrim(c.linkedin_url, '/')) = p.url
  UNION
  -- Two words in common, found by the index; which relation they have is checked below.
  SELECT p.i, c.id FROM person p, unnest(p.words) AS a(w), unnest(p.words) AS b(w), contact c
  WHERE a.w < b.w AND c.name_words @> ARRAY[a.w, b.w]
  UNION
  SELECT p.i, c.id FROM person p JOIN contact c ON cardinality(p.words) = 1 AND c.name_words = p.words
)
SELECT f.i, c.id, btrim(c.first_name || ' ' || c.last_name)::text AS name, c.title, c.company_id, c.email, c.email_check_status,
       c.status, c.updated_at,
       (CASE WHEN lower(rtrim(c.linkedin_url, '/')) = p.url THEN 'profile'
             WHEN c.name_words = p.words THEN 'same_name'
             ELSE 'similar_name' END)::text AS kind
FROM found f
JOIN person p ON p.i = f.i
JOIN contact c ON c.id = f.id
WHERE lower(rtrim(c.linkedin_url, '/')) = p.url OR c.name_words = p.words
   OR (cardinality(p.words) >= 2 AND cardinality(c.name_words) >= 2 AND (c.name_words <@ p.words OR c.name_words @> p.words))
ORDER BY f.i, c.id;

-- Keep a billing contact's current company and address stable during handoff to the mail provider.
-- name: LockInvoiceRecipient :one
SELECT id FROM contact WHERE id = @id AND company_id = @company_id AND lower(email) = lower(@address::text)
FOR SHARE;

-- Merging duplicate companies: the duplicate's contacts, LinkedIn checks and HR searches go to the one kept.
-- name: MoveCompanyContacts :execrows
UPDATE contact SET company_id = @to_id WHERE company_id = @from_id;

-- name: MoveCompanyLinkedinChecks :exec
UPDATE contact_linkedin_check SET company_id = @to_id WHERE company_id = @from_id;

-- name: MoveCompanyHRScans :exec
UPDATE hr_scan SET company_id = @to_id WHERE company_id = @from_id;

-- Merging duplicate contacts: the duplicate's checks go to the contact kept.
-- name: MoveEmploymentChecks :exec
UPDATE contact_employment_check SET contact_id = @to_id WHERE contact_id = @from_id;

-- name: MoveLinkedinChecks :exec
UPDATE contact_linkedin_check SET contact_id = @to_id WHERE contact_id = @from_id;

-- name: MoveMailboxChecks :exec
UPDATE contact_mailbox_check SET contact_id = @to_id WHERE contact_id = @from_id;

-- The duplicate, once everything has moved.
-- name: DeleteMergedContact :execrows
DELETE FROM contact WHERE id = @id;

-- Possible duplicates of a contact: the same LinkedIn profile, the same name
-- words (any order, titles aside), the same phone (9 digits or more), or a name much like it at
-- the same company. Newest first.
-- name: SimilarContacts :many
SELECT c.id, c.company_id, c.first_name, c.last_name, c.title, c.email,
       coalesce(lower(rtrim(c.linkedin_url, '/')) = lower(rtrim(me.linkedin_url, '/')), false)::boolean AS same_linkedin,
       (cardinality(me.name_words) >= 2 AND c.name_words = me.name_words)::boolean AS same_name,
       (string_to_array(c.phone_digits, ' ') && me.phones)::boolean AS same_phone,
       (c.company_id = me.company_id AND similarity(c.name_text, me.name_text) >= 0.6)::boolean AS similar_name
FROM contact c, (SELECT x.company_id, x.linkedin_url, x.name_words, x.name_text,
                         ARRAY(SELECT p FROM unnest(string_to_array(x.phone_digits, ' ')) AS p WHERE length(p) >= 9) AS phones
                  FROM contact x WHERE x.id = @id) me
WHERE c.id <> @id
  AND (lower(rtrim(c.linkedin_url, '/')) = lower(rtrim(me.linkedin_url, '/'))
       OR (cardinality(me.name_words) >= 2 AND c.name_words = me.name_words)
       OR string_to_array(c.phone_digits, ' ') && me.phones
       OR (c.company_id = me.company_id AND similarity(c.name_text, me.name_text) >= 0.6))
ORDER BY c.id DESC
LIMIT 20;

-- Every group of contacts sharing a LinkedIn profile or the same name words
-- (two or more) at the same company (the Possible duplicates list).
-- name: ContactDuplicateGroups :many
SELECT kind::text AS kind, ids::text[] AS ids FROM (
  SELECT 'same_name' AS kind, array_agg(id ORDER BY id) AS ids FROM contact
  WHERE cardinality(name_words) >= 2 GROUP BY company_id, name_words HAVING count(*) > 1
  UNION ALL
  SELECT 'same_linkedin', array_agg(id ORDER BY id) FROM contact WHERE linkedin_url IS NOT NULL
  GROUP BY lower(rtrim(linkedin_url, '/')) HAVING count(*) > 1
) g
ORDER BY ids[cardinality(ids)] DESC
LIMIT 500;

-- The contacts of the Possible duplicates list.
-- name: ContactLookalikes :many
SELECT id, company_id, first_name, last_name, title, email FROM contact WHERE id = ANY(@ids::text[]);
