-- Queries owned by the companies module. Run inside a tenant-scoped
-- transaction: row-level security limits every query to the current tenant.

-- name: InsertCompany :one
INSERT INTO company (
  tenant_id, name, type, category, website, linkedin_url, industry, size, phone, address, city, country,
  registration_number, about, created_by
) VALUES (
  app_current_tenant(), @name, @type, sqlc.narg(category), sqlc.narg(website), sqlc.narg(linkedin_url), sqlc.narg(industry), sqlc.narg(size),
  sqlc.narg(phone), sqlc.narg(address), sqlc.narg(city), sqlc.narg(country),
  sqlc.narg(registration_number), sqlc.narg(about), app_current_user()
)
RETURNING *;

-- name: GetCompany :one
SELECT * FROM company WHERE id = @id;

-- name: CompanyExists :one
SELECT EXISTS (SELECT 1 FROM company WHERE id = @id);

-- name: FindCompanyIDByName :one
SELECT id FROM company WHERE name_key = app_normalize_text(@name::text);

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateCompany :one
UPDATE company SET
  name = @name,
  type = @type,
  category = sqlc.narg(category),
  website = sqlc.narg(website),
  linkedin_url = sqlc.narg(linkedin_url),
  industry = sqlc.narg(industry),
  size = sqlc.narg(size),
  phone = sqlc.narg(phone),
  address = sqlc.narg(address),
  city = sqlc.narg(city),
  country = sqlc.narg(country),
  registration_number = sqlc.narg(registration_number),
  about = sqlc.narg(about),
  version = version + 1,
  updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- Newest first, keyset pagination on id (IDs sort by creation time).
-- name: ListCompanies :many
SELECT id, name, type, category, industry, city, website, linkedin_url, updated_at
FROM company
WHERE (sqlc.narg(cursor)::text IS NULL OR id < sqlc.narg(cursor)::text)
  AND (sqlc.narg(type)::text IS NULL OR type = sqlc.narg(type)::text)
  AND (sqlc.narg(category)::text IS NULL OR category = sqlc.narg(category)::text)
  AND (sqlc.narg(query)::text IS NULL
       OR search_text LIKE ALL (app_search_patterns(sqlc.narg(query)::text))
       OR phone_digits LIKE '%' || app_phone_key(sqlc.narg(query)::text) || '%')
ORDER BY id DESC
LIMIT @row_limit;

-- name: CompanyNames :many
SELECT id, name FROM company WHERE id = ANY(@ids::text[]);

-- name: AutofillState :one
SELECT autofill_status, autofill_error, autofill_checked_at, website FROM company WHERE id = @id;

-- name: SetAutofill :exec
UPDATE company SET autofill_status = @autofill_status, autofill_error = sqlc.narg(autofill_error),
  autofill_checked_at = CASE WHEN @autofill_status IN ('done', 'failed') THEN now() ELSE autofill_checked_at END
WHERE id = @id;

-- A new reading replaces the suggestions still waiting.
-- name: ClearSuggestions :exec
DELETE FROM company_suggestion WHERE company_id = @company_id AND status = 'pending';

-- name: InsertSuggestion :exec
INSERT INTO company_suggestion (tenant_id, company_id, field, value, source)
VALUES (app_current_tenant(), @company_id, @field, @value, @source);

-- name: PendingSuggestions :many
SELECT * FROM company_suggestion WHERE company_id = @company_id AND status = 'pending' ORDER BY id;

-- name: GetSuggestion :one
SELECT * FROM company_suggestion WHERE id = @id AND company_id = @company_id;

-- name: DecideSuggestion :exec
UPDATE company_suggestion SET status = @status WHERE id = @id;

-- name: CompanyWebsite :one
SELECT coalesce(website, '')::text FROM company WHERE id = @id;

-- For each word of a job search (numbered as app_search_words numbers them),
-- the companies whose name or website has it: "n:companyId".
-- name: CompanyWordHits :many
SELECT (w.n::text || ':' || c.id)::text AS hit
FROM app_search_words(@query::text) w
JOIN company c ON strpos(c.search_text, w.word) > 0
LIMIT 5000;

-- A client's benefits and pre-offer process (JSON arrays; see the migration).
-- name: GetCompanyBenefits :one
SELECT benefits, pre_offer, version, updated_at FROM company_benefits WHERE company_id = @company_id;

-- The first save (nothing is saved yet, or someone saved first: no row).
-- name: InsertCompanyBenefits :one
INSERT INTO company_benefits (tenant_id, company_id, benefits, pre_offer, updated_by)
VALUES (app_current_tenant(), @company_id, @benefits, @pre_offer, app_current_user())
ON CONFLICT (tenant_id, company_id) DO NOTHING
RETURNING version, updated_at;

-- name: UpdateCompanyBenefits :one
UPDATE company_benefits
SET benefits = @benefits, pre_offer = @pre_offer, version = version + 1, updated_by = app_current_user(), updated_at = now()
WHERE company_id = @company_id AND version = @expected_version
RETURNING version, updated_at;

-- A client's terms of business.
-- name: GetCompanyTerms :one
SELECT tiers, salary_currency, signed_on, notes, version, updated_at
FROM company_terms WHERE company_id = @company_id;

-- The first save (no row yet; none returned when someone saved first).
-- name: InsertCompanyTerms :one
INSERT INTO company_terms (tenant_id, company_id, tiers, salary_currency, signed_on, notes, updated_by)
VALUES (app_current_tenant(), @company_id, @tiers, @salary_currency, sqlc.narg(signed_on), sqlc.narg(notes), app_current_user())
ON CONFLICT (tenant_id, company_id) DO NOTHING
RETURNING version, updated_at;

-- name: UpdateCompanyTerms :one
UPDATE company_terms
SET tiers = @tiers, salary_currency = @salary_currency, signed_on = sqlc.narg(signed_on), notes = sqlc.narg(notes),
    version = version + 1, updated_by = app_current_user(), updated_at = now()
WHERE company_id = @company_id AND version = @expected_version
RETURNING version, updated_at;

-- Documents kept with a client's terms or benefits, oldest first.
-- name: ListCompanyDocuments :many
SELECT * FROM company_document WHERE company_id = @company_id ORDER BY created_at, id;

-- name: InsertCompanyDocument :one
INSERT INTO company_document (tenant_id, company_id, section, file_name, content_type, size_bytes, storage_key, created_by)
VALUES (app_current_tenant(), @company_id, @section, @file_name, @content_type, @size_bytes, @storage_key, app_current_user())
RETURNING *;

-- name: GetCompanyDocument :one
SELECT * FROM company_document WHERE id = @id AND company_id = @company_id;

-- name: DeleteCompanyDocument :one
DELETE FROM company_document WHERE id = @id AND company_id = @company_id RETURNING storage_key;

-- name: LegacyIndustries :many
SELECT id, industry::text FROM company WHERE industry IS NOT NULL;
-- name: SetIndustrySummary :exec
UPDATE company SET industry = sqlc.narg(industry), version = version + 1, updated_at = now()
WHERE id = @id AND industry IS DISTINCT FROM sqlc.narg(industry);
-- Many at once, after an industry is renamed, moved or replaced.
-- name: SetIndustrySummaries :exec
UPDATE company r SET industry = v.industry, version = r.version + 1, updated_at = now()
FROM (SELECT unnest(@ids::text[]) AS id, unnest(@industries::text[]) AS industry) v
WHERE r.id = v.id AND r.industry IS DISTINCT FROM v.industry;
-- name: IndustryEmployerID :one
SELECT id FROM company WHERE name_key = app_normalize_text(@name::text);
-- name: TouchCompany :one
UPDATE company SET version = version + 1, updated_at = now() WHERE id = @id AND version = @expected_version RETURNING *;
