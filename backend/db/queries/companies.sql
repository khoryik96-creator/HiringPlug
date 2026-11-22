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
  AND (sqlc.narg(in_ids)::text[] IS NULL OR id = ANY (sqlc.narg(in_ids)::text[])) -- a folder's
  -- The page's name is letters, digits, _ and - only (the contract), so it is safe in the pattern.
  AND (sqlc.narg(linkedin_company)::text IS NULL
       OR linkedin_url ~* ('^https://www\.linkedin\.com/company/' || sqlc.narg(linkedin_company)::text || '(/|$)'))
  AND (sqlc.narg(query)::text IS NULL
       OR search_text LIKE ALL (app_search_patterns(sqlc.narg(query)::text))
       OR phone_digits LIKE '%' || app_phone_key(sqlc.narg(query)::text) || '%')
  -- A boolean search (boolsearch's query for to_tsquery).
  -- The expression of company_search_doc_idx (00116), so the index answers.
  AND (sqlc.narg(tsquery)::text IS NULL
       OR app_search_doc(name, website, industry, industry_terms, industry_hint, city, about) @@ to_tsquery('english', sqlc.narg(tsquery)::text))
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

-- A company just added: its first reading fills its empty details.
-- name: SetFillEmpty :exec
UPDATE company SET autofill_fill_empty = @fill_empty, autofill_name_guessed = @name_guessed WHERE id = @id;

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

-- name: CompanyLinkedin :one
SELECT coalesce(linkedin_url, '')::text FROM company WHERE id = @id;

-- For each word of a job search (numbered as app_search_words numbers them),
-- the companies whose name or website has it: "n:companyId".
-- name: CompanyWordHits :many
SELECT (w.n::text || ':' || c.id)::text AS hit
FROM app_search_words(@query::text) w
JOIN company c ON strpos(c.search_text, w.word) > 0
LIMIT 5000;

-- A client's benefits and pre-offer process (JSON arrays; see the migration).
-- name: GetCompanyBenefits :one
SELECT benefits, pre_offer, grades, version, updated_at FROM company_benefits WHERE company_id = @company_id;

-- The first save (nothing is saved yet, or someone saved first: no row).
-- name: InsertCompanyBenefits :one
INSERT INTO company_benefits (tenant_id, company_id, benefits, pre_offer, grades, updated_by)
VALUES (app_current_tenant(), @company_id, @benefits, @pre_offer, @grades, app_current_user())
ON CONFLICT (tenant_id, company_id) DO NOTHING
RETURNING version, updated_at;

-- name: UpdateCompanyBenefits :one
UPDATE company_benefits
SET benefits = @benefits, pre_offer = @pre_offer, grades = @grades, version = version + 1, updated_by = app_current_user(), updated_at = now()
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
-- The words of a company's industries and subindustries, for its search.
-- name: SetIndustryTerms :exec
UPDATE company c SET industry_terms = nullif(v.terms, '')
FROM (SELECT unnest(@ids::text[]) AS id, unnest(@terms::text[]) AS terms) v
WHERE c.id = v.id AND c.industry_terms IS DISTINCT FROM nullif(v.terms, '');
-- name: ClearIndustryTermsExcept :exec
UPDATE company SET industry_terms = NULL WHERE industry_terms IS NOT NULL AND id <> ALL(@ids::text[]);
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

-- How many companies are in a category (Settings refuses to remove one in use).
-- name: CountCompaniesInCategory :one
SELECT count(*) FROM company WHERE category = @category;

-- name: SetIndustryHint :exec
UPDATE company SET industry_hint = sqlc.narg(industry_hint) WHERE id = @id;

-- name: InsertIndustryPick :one
INSERT INTO company_industry_pick (tenant_id, company_id)
VALUES (app_current_tenant(), @company_id)
RETURNING id;

-- The AI's answer, once (a repeated delivery changes nothing).
-- name: SaveIndustryPick :one
UPDATE company_industry_pick
SET status = @status, choice_id = sqlc.narg(choice_id), error = sqlc.narg(error), model = sqlc.narg(model),
    input_tokens = @input_tokens, output_tokens = @output_tokens
WHERE id = @id AND status = 'queued'
RETURNING company_id;

-- name: IndustryPickTokensSince :one
SELECT coalesce(sum(input_tokens + output_tokens), 0)::bigint FROM company_industry_pick WHERE created_at >= @since::timestamptz;

-- name: SetIndustryPickSince :exec
UPDATE company SET industry_pick_since = sqlc.narg(since) WHERE id = @id;

-- The Bin (bin module): a binned company is hidden by the database; these run with app.show_binned on.
-- name: BinCompany :one
UPDATE company SET binned_at = now() WHERE id = @id AND binned_at IS NULL RETURNING name;

-- name: RestoreCompany :execrows
UPDATE company SET binned_at = NULL WHERE id = @id AND binned_at IS NOT NULL;

-- name: PurgeCompany :many
WITH docs AS (DELETE FROM company_document WHERE company_id = @id
               AND EXISTS (SELECT 1 FROM company WHERE id = @id AND binned_at IS NOT NULL) RETURNING storage_key),
     gone AS (DELETE FROM company WHERE id = @id AND binned_at IS NOT NULL RETURNING id)
SELECT storage_key FROM docs WHERE EXISTS (SELECT 1 FROM gone);

-- name: CompanyLabel :one
SELECT name FROM company WHERE id = @id;

-- Possible duplicates of a company: the same website, LinkedIn page or name
-- without its legal form, or a name much like it. Newest first.
-- name: SimilarCompanies :many
SELECT c.id, c.name, c.city, c.website, c.industry,
       coalesce(c.site_key = me.site_key, false)::boolean AS same_website,
       coalesce(lower(rtrim(c.linkedin_url, '/')) = lower(rtrim(me.linkedin_url, '/')), false)::boolean AS same_linkedin,
       (me.name_core <> '' AND (c.name_core = me.name_core OR similarity(c.name_core, me.name_core) >= 0.6))::boolean AS similar_name
FROM company c, (SELECT x.site_key, x.linkedin_url, x.name_core FROM company x WHERE x.id = @id) me
WHERE c.id <> @id
  AND (c.site_key = me.site_key
       OR lower(rtrim(c.linkedin_url, '/')) = lower(rtrim(me.linkedin_url, '/'))
       OR (me.name_core <> '' AND (c.name_core = me.name_core OR similarity(c.name_core, me.name_core) >= 0.6)))
ORDER BY c.id DESC
LIMIT 20;

-- Every group of companies sharing a website, a LinkedIn page or a name
-- without its legal form (the Possible duplicates list).
-- name: CompanyDuplicateGroups :many
SELECT kind::text AS kind, ids::text[] AS ids FROM (
  SELECT 'similar_name' AS kind, array_agg(id ORDER BY id) AS ids FROM company WHERE name_core <> '' GROUP BY name_core HAVING count(*) > 1
  UNION ALL
  SELECT 'same_website', array_agg(id ORDER BY id) FROM company WHERE site_key IS NOT NULL GROUP BY site_key HAVING count(*) > 1
  UNION ALL
  SELECT 'same_linkedin', array_agg(id ORDER BY id) FROM company WHERE linkedin_url IS NOT NULL
  GROUP BY lower(rtrim(linkedin_url, '/')) HAVING count(*) > 1
) g
ORDER BY ids[cardinality(ids)] DESC
LIMIT 500;

-- Merging duplicates: the duplicate's terms and benefits only if the company kept has none; its documents always.
-- name: MoveTermsIfNone :exec
UPDATE company_terms t SET company_id = @to_id
WHERE t.company_id = @from_id AND NOT EXISTS (SELECT 1 FROM company_terms k WHERE k.company_id = @to_id);

-- name: MoveBenefitsIfNone :exec
UPDATE company_benefits t SET company_id = @to_id
WHERE t.company_id = @from_id AND NOT EXISTS (SELECT 1 FROM company_benefits k WHERE k.company_id = @to_id);

-- name: MoveCompanyDocuments :exec
UPDATE company_document SET company_id = @to_id WHERE company_id = @from_id;

-- The duplicate, once everything has moved (suggestions waiting for it go with it).
-- name: DeleteMergedCompany :execrows
DELETE FROM company WHERE id = @id;

-- The companies of the Possible duplicates list.
-- name: CompanyLookalikes :many
SELECT id, name, city, website, industry FROM company WHERE id = ANY(@ids::text[]);
