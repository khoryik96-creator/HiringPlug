-- Queries owned by the jobs module. Run inside a tenant-scoped transaction:
-- row-level security limits every query to the current tenant, and the
-- salary table also to users with the salary permissions.

-- name: InsertJobOrder :one
INSERT INTO job_order (
  tenant_id, company_id, contact_id, title, status, location, country, work_type, work_arrangement,
  openings, description, intake_date, owner_id, created_by
) VALUES (
  app_current_tenant(), @company_id, sqlc.narg(contact_id), @title, @status, sqlc.narg(location), sqlc.narg(country),
  sqlc.narg(work_type), sqlc.narg(work_arrangement), @openings, sqlc.narg(description), @intake_date, sqlc.narg(owner_id), app_current_user()
)
RETURNING *;

-- name: GetJobOrder :one
SELECT * FROM job_order WHERE id = @id;

-- name: JobOrderExists :one
SELECT EXISTS (SELECT 1 FROM job_order WHERE id = @id);

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateJobOrder :one
UPDATE job_order SET
  company_id = @company_id,
  contact_id = sqlc.narg(contact_id),
  title = @title,
  status = @status,
  location = sqlc.narg(location),
  country = sqlc.narg(country),
  work_type = sqlc.narg(work_type),
  work_arrangement = sqlc.narg(work_arrangement),
  openings = @openings,
  description = sqlc.narg(description),
  intake_date = @intake_date,
  version = version + 1,
  updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- Newest first, keyset pagination on id (IDs sort by creation time).
-- name: ListJobOrders :many
SELECT id, title, company_id, status, location, openings, owner_id, created_at, updated_at
FROM job_order
WHERE (sqlc.narg(cursor)::text IS NULL OR id < sqlc.narg(cursor)::text)
  AND (sqlc.narg(owner_id)::text IS NULL OR owner_id = sqlc.narg(owner_id)::text)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(company_id)::text IS NULL OR company_id = sqlc.narg(company_id)::text)
  AND (sqlc.narg(contact_id)::text IS NULL OR contact_id = sqlc.narg(contact_id)::text)
  AND (sqlc.narg(query)::text IS NULL OR NOT EXISTS (
         -- every word in the title or location, or in the client's name (@company_hits: "n:companyId")
         SELECT 1 FROM app_search_words(sqlc.narg(query)::text) w
         WHERE NOT (strpos(search_text, w.word) > 0 OR (w.n::text || ':' || company_id) = ANY(coalesce(@company_hits::text[], ARRAY[]::text[])))))
ORDER BY id DESC
LIMIT @row_limit;

-- name: JobOrderTitles :many
SELECT id, title, company_id, contact_id, work_type, owner_id, created_at FROM job_order WHERE id = ANY(@ids::text[]);

-- Amounts are read as exact decimal text (numeric -> text), never floats;
-- an empty max_amount means a single figure.
-- name: GetJobSalary :one
SELECT version, min_amount::text AS min_amount, coalesce(max_amount::text, '')::text AS max_amount, currency, period, updated_at
FROM job_order_salary WHERE job_order_id = @job_order_id;

-- name: InsertJobSalary :one
INSERT INTO job_order_salary (job_order_id, tenant_id, min_amount, max_amount, currency, period, updated_by)
VALUES (
  @job_order_id, app_current_tenant(), sqlc.arg(min_amount)::text::numeric, sqlc.narg(max_amount)::text::numeric,
  @currency, @period, app_current_user()
)
ON CONFLICT (job_order_id) DO NOTHING
RETURNING version, min_amount::text AS min_amount, coalesce(max_amount::text, '')::text AS max_amount, currency, period, updated_at;

-- name: UpdateJobSalary :one
UPDATE job_order_salary SET
  min_amount = sqlc.arg(min_amount)::text::numeric,
  max_amount = sqlc.narg(max_amount)::text::numeric,
  currency = @currency,
  period = @period,
  version = version + 1,
  updated_by = app_current_user(),
  updated_at = now()
WHERE job_order_id = @job_order_id AND version = @expected_version
RETURNING version, min_amount::text AS min_amount, coalesce(max_amount::text, '')::text AS max_amount, currency, period, updated_at;

-- Clearing the budget: no row means no budget.
-- name: DeleteJobSalary :execrows
DELETE FROM job_order_salary WHERE job_order_id = @job_order_id AND version = @expected_version;

-- name: OpenJobOrders :many
SELECT id, title, company_id, created_at FROM job_order WHERE status = 'open' ORDER BY id DESC LIMIT @row_limit;

-- Job ads.
-- name: InsertJobAd :one
INSERT INTO job_ad (tenant_id, job_order_id, title, location, country, work_type, work_arrangement, salary_text, description, closes_on,
                    client_description, publish_blind, created_by)
VALUES (app_current_tenant(), @job_order_id, @title, sqlc.narg(location), sqlc.narg(country), sqlc.narg(work_type), sqlc.narg(work_arrangement),
        sqlc.narg(salary_text), sqlc.narg(description), sqlc.narg(closes_on), sqlc.narg(client_description), @publish_blind, app_current_user())
RETURNING *;

-- name: GetJobAd :one
SELECT * FROM job_ad WHERE id = @id;

-- name: ListJobAds :many
SELECT * FROM job_ad WHERE job_order_id = @job_order_id ORDER BY id DESC LIMIT 100;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateJobAd :one
UPDATE job_ad SET
  title = @title,
  location = sqlc.narg(location),
  country = sqlc.narg(country),
  work_type = sqlc.narg(work_type),
  work_arrangement = sqlc.narg(work_arrangement),
  salary_text = sqlc.narg(salary_text),
  description = sqlc.narg(description),
  closes_on = sqlc.narg(closes_on),
  status = @status,
  published_at = sqlc.narg(published_at),
  client_description = sqlc.narg(client_description),
  publish_blind = @publish_blind,
  blind_edited = @blind_edited,
  blind_title_edit = sqlc.narg(blind_title_edit),
  blind_description_edit = sqlc.narg(blind_description_edit),
  blind_source_hash = sqlc.narg(blind_source_hash),
  version = version + 1,
  updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- A job that is filled or closed takes its published ads down with it.
-- name: CloseJobAds :execrows
UPDATE job_ad SET status = 'closed', version = version + 1, updated_at = now()
WHERE job_order_id = @job_order_id AND status = 'published';

-- The careers page: published ads still open (no closing date, or not past
-- it on @today, the agency's date: not the database's, which is UTC).
-- name: ListPublishedAds :many
SELECT * FROM job_ad
WHERE status = 'published' AND (closes_on IS NULL OR closes_on >= @today::date)
ORDER BY published_at DESC LIMIT 200;

-- name: GetPublishedAd :one
SELECT * FROM job_ad
WHERE id = @id AND status = 'published' AND (closes_on IS NULL OR closes_on >= @today::date);

-- name: SetJobOwner :execrows
UPDATE job_order SET owner_id = sqlc.narg(owner_id), version = version + 1, updated_at = now() WHERE id = @id;

-- Job descriptions uploaded to fill in an ad.
-- name: InsertJdFile :one
INSERT INTO job_description_file (id, tenant_id, job_id, file_name, content_type, size_bytes, storage_key, created_by, status, text, details)
VALUES (@id, app_current_tenant(), @job_id, @file_name, @content_type, @size_bytes, @storage_key, app_current_user(), @status, sqlc.narg(text), sqlc.narg(details))
RETURNING id, job_id, file_name, content_type, size_bytes, status, error, details, pdf_status, created_at;

-- name: NewJdFileID :one
SELECT new_id('jdf')::text;

-- name: GetJdFile :one
SELECT id, job_id, file_name, content_type, size_bytes, status, error, details, pdf_status, created_at FROM job_description_file WHERE id = @id;

-- name: ListJobJdFiles :many
SELECT id, job_id, file_name, content_type, size_bytes, status, error, details, pdf_status, created_at FROM job_description_file
WHERE job_id = @job_id ORDER BY created_at DESC, id DESC LIMIT 50;

-- name: GetJdFileStorage :one
SELECT storage_key, file_name, content_type, pdf_status, pdf_key FROM job_description_file WHERE id = @id;

-- name: QueueJdPdf :exec
UPDATE job_description_file SET pdf_status = 'queued', pdf_key = NULL, pdf_error = NULL WHERE id = @id;

-- A copy for a file that has none (one uploaded before previews began); no row
-- when it has one, or one is being made (two asking at once queue it once).
-- name: QueueJdPdfIfNone :one
UPDATE job_description_file SET pdf_status = 'queued' WHERE id = @id AND pdf_status IS NULL RETURNING storage_key;

-- A copy arriving for a file deleted (or whose copy was asked again) meanwhile is dropped.
-- name: SaveJdPdf :one
UPDATE job_description_file SET pdf_status = @pdf_status, pdf_key = sqlc.narg(pdf_key), pdf_error = sqlc.narg(pdf_error)
WHERE id = @id AND pdf_status = 'queued'
RETURNING id;

-- name: DeleteJdFile :one
DELETE FROM job_description_file WHERE id = @id RETURNING job_id, file_name, storage_key, pdf_key;

-- A job description read on the Add job form stays with the job made from it.
-- name: LinkJdFileToJob :execrows
UPDATE job_description_file SET job_id = @job_id WHERE id = @id AND job_id IS NULL;

-- name: SaveJdRead :execrows
UPDATE job_description_file
SET status = @status, error = sqlc.narg(error), text = sqlc.narg(text), details = sqlc.narg(details), ai_tokens = @ai_tokens
WHERE id = @id;

-- AI rewrites of an ad's blind version (CV Studio's way).
-- name: InsertBlindRewrite :one
INSERT INTO job_ad_blind_rewrite (tenant_id, ad_id, tone, strength, source_hash, created_by)
VALUES (app_current_tenant(), @ad_id, @tone, @strength, @source_hash, app_current_user())
RETURNING *;

-- name: GetBlindRewrite :one
SELECT * FROM job_ad_blind_rewrite WHERE id = @id;

-- name: ListBlindRewrites :many
SELECT * FROM job_ad_blind_rewrite WHERE ad_id = @ad_id ORDER BY created_at DESC, id DESC LIMIT 5;

-- name: FinishBlindRewrite :one
UPDATE job_ad_blind_rewrite
   SET status = @status, error = sqlc.narg(error), warnings = @warnings, retried = @retried,
       model = sqlc.narg(model), tokens = @tokens, finished_at = now()
 WHERE id = @id AND status = 'queued'
RETURNING *;

-- name: BlindRewriteTokensSince :one
SELECT coalesce(sum(tokens), 0)::bigint FROM job_ad_blind_rewrite WHERE created_at >= @since;

-- name: JdReadTokensSince :one
SELECT coalesce(sum(ai_tokens), 0)::bigint FROM job_description_file WHERE created_at >= @since AND ai_tokens > 0;

-- The AI's rewrite becomes the ad's blind version, as a person's edit would
-- (the ad's version moves on, so an open edit form sees the change).
-- name: SetAdBlindText :one
UPDATE job_ad SET
  blind_edited = true,
  blind_title_edit = @blind_title_edit,
  blind_description_edit = sqlc.narg(blind_description_edit),
  blind_source_hash = @blind_source_hash,
  version = version + 1,
  updated_at = now()
WHERE id = @id
RETURNING *;

-- A company's jobs, open or closed (its billing adds up their placements).
-- name: JobOrderIDsOfCompany :many
SELECT id FROM job_order WHERE company_id = @company_id;

-- The Bin (bin module): a company's jobs go to the Bin with it, or move to another company.
-- name: BinJobs :exec
UPDATE job_order SET binned_at = now() WHERE id = ANY(@ids::text[]) AND binned_at IS NULL;

-- name: RestoreJobs :exec
UPDATE job_order SET binned_at = NULL WHERE id = ANY(@ids::text[]) AND binned_at IS NOT NULL;

-- name: BinnedJobs :many
SELECT id FROM job_order WHERE id = ANY(@ids::text[]) AND binned_at IS NOT NULL;

-- name: MoveJobsToCompany :execrows
UPDATE job_order SET company_id = @to_id, version = version + 1, updated_at = now()
WHERE company_id = @from_id AND binned_at IS NULL;

-- A contact deleted for good leaves the jobs it was the contact of.
-- name: ClearJobContacts :exec
UPDATE job_order SET contact_id = NULL WHERE contact_id = ANY(@contact_ids::text[]);

-- name: PurgeJobs :many
WITH files AS (DELETE FROM job_description_file
               WHERE job_id IN (SELECT id FROM job_order WHERE id = ANY(@ids::text[]) AND binned_at IS NOT NULL) RETURNING storage_key, pdf_key),
     gone AS (DELETE FROM job_order WHERE id = ANY(@ids::text[]) AND binned_at IS NOT NULL RETURNING id)
SELECT storage_key, pdf_key FROM files;

-- Merging duplicates: the duplicate company's or contact's jobs go to the one kept.
-- name: MoveCompanyJobs :execrows
UPDATE job_order SET company_id = @to_id WHERE company_id = @from_id;

-- name: MoveContactJobs :execrows
UPDATE job_order SET contact_id = @to_id WHERE contact_id = @from_id;

-- The jobs a recruiter owns (notifications: new applicants on them).
-- name: JobOrderIDsOwnedBy :many
SELECT id FROM job_order WHERE owner_id = @owner_id ORDER BY id DESC LIMIT 1000;
