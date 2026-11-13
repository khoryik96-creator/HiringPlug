-- Queries owned by the candidates module. Run inside a tenant-scoped
-- transaction: row-level security limits every query to the current tenant.

-- name: InsertCandidate :one
INSERT INTO candidate (
  tenant_id, first_name, last_name, email, phone, nationality, residence_country, linkedin_url, availability,
  work_authorization, work_authorization_basis, work_authorization_note,
  notice_period_value, notice_period_unit, industry, specialization,
  current_title, current_company, location, source, tags, skills, owner_id, created_by, availability_since
) VALUES (
  app_current_tenant(), @first_name, @last_name, sqlc.narg(email), sqlc.narg(phone), sqlc.narg(nationality), sqlc.narg(residence_country), sqlc.narg(linkedin_url), sqlc.narg(availability),
  sqlc.narg(work_authorization), sqlc.narg(work_authorization_basis), sqlc.narg(work_authorization_note),
  sqlc.narg(notice_period_value), sqlc.narg(notice_period_unit), sqlc.narg(industry), sqlc.narg(specialization),
  sqlc.narg(current_title), sqlc.narg(current_company), sqlc.narg(location), sqlc.narg(source), @tags, @skills,
  sqlc.narg(owner_id), app_current_user(), CASE WHEN sqlc.narg(availability)::text IS NULL THEN NULL ELSE now() END
)
RETURNING *;

-- name: GetCandidate :one
SELECT * FROM candidate WHERE id = @id;

-- name: CandidateExists :one
SELECT EXISTS (SELECT 1 FROM candidate WHERE id = @id);

-- name: FindCandidateIDByEmail :one
SELECT id FROM candidate WHERE email = @email;

-- Optimistic concurrency: only updates if nobody changed the row since
-- `expected_version` was read. No row returned = stale version (or not found).
-- name: UpdateCandidate :one
UPDATE candidate SET
  first_name = @first_name,
  last_name = @last_name,
  email = sqlc.narg(email),
  phone = sqlc.narg(phone),
  nationality = sqlc.narg(nationality),
  residence_country = sqlc.narg(residence_country),
  linkedin_url = sqlc.narg(linkedin_url),
  availability = sqlc.narg(availability),
  availability_since = CASE WHEN availability IS DISTINCT FROM sqlc.narg(availability) THEN
                         CASE WHEN sqlc.narg(availability)::text IS NULL THEN NULL ELSE now() END
                       ELSE availability_since END,
  work_authorization = sqlc.narg(work_authorization),
  work_authorization_basis = sqlc.narg(work_authorization_basis),
  work_authorization_note = sqlc.narg(work_authorization_note),
  notice_period_value = sqlc.narg(notice_period_value),
  notice_period_unit = sqlc.narg(notice_period_unit),
  industry = sqlc.narg(industry),
  specialization = sqlc.narg(specialization),
  current_title = sqlc.narg(current_title),
  current_company = sqlc.narg(current_company),
  location = sqlc.narg(location),
  source = sqlc.narg(source),
  tags = @tags,
  skills = @skills,
  version = version + 1,
  updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- Newest first, keyset pagination on id (IDs sort by creation time).
-- @phones are the whole numbers a phone number typed as the query can be
-- (findquery.Numbers): never its last digits. With @similar, names like the query (typos) instead, the closest first.
-- name: ListCandidates :many
SELECT c.id, c.first_name, c.last_name, c.email, c.phone, c.current_title, c.current_company,
       c.location, c.work_authorization, c.tags, c.updated_at,
       c.availability, c.availability_since, c.notice_period_value, c.notice_period_unit, c.work_authorization_basis,
       c.nationality, c.residence_country, c.source, c.industry
FROM candidate c
WHERE (sqlc.narg(cursor)::text IS NULL OR c.id < sqlc.narg(cursor)::text)
  AND (sqlc.narg(query)::text IS NULL
       OR CASE WHEN sqlc.arg(similar_names)::boolean
            THEN app_normalize_text(sqlc.narg(query)::text) <% c.name_text
            ELSE c.search_text LIKE ALL (app_search_patterns(sqlc.narg(query)::text))
              OR c.phone = ANY (@phones::text[])
          END)
  AND (sqlc.narg(tag)::text IS NULL OR app_lower_array(c.tags) @> ARRAY[lower(sqlc.narg(tag)::text)])
  AND (sqlc.narg(skill)::text IS NULL OR app_lower_array(c.skills) @> ARRAY[lower(sqlc.narg(skill)::text)])
  AND (sqlc.narg(owner)::text IS NULL OR c.owner_id = sqlc.narg(owner)::text)
ORDER BY CASE WHEN sqlc.arg(similar_names)::boolean THEN word_similarity(app_normalize_text(coalesce(sqlc.narg(query)::text, '')), c.name_text) END DESC, c.id DESC
LIMIT @row_limit;

-- Advanced search: every filter given must hold. With @ts (a boolean
-- search as a text search query), a candidate is found when their profile
-- matches or they are one of @cv_ids (whose CVs match, found by the
-- documents module); a profile match does not count for @excluded_ids
-- (whose CVs have a word left out with NOT). Newest first, keyset
-- pagination on id.
-- name: SearchCandidates :many
SELECT c.id, c.first_name, c.last_name, c.email, c.phone, c.current_title, c.current_company,
       c.location, c.work_authorization, c.tags, c.updated_at,
       c.availability, c.availability_since, c.notice_period_value, c.notice_period_unit, c.work_authorization_basis,
       c.nationality, c.residence_country, c.source, c.industry,
       (sqlc.narg(ts)::text IS NOT NULL AND c.profile_terms @@ to_tsquery('english', sqlc.narg(ts)::text))::boolean AS profile_hit
FROM candidate c
WHERE (sqlc.narg(cursor)::text IS NULL OR c.id < sqlc.narg(cursor)::text)
  AND (sqlc.narg(query)::text IS NULL
       OR c.search_text LIKE ALL (app_search_patterns(sqlc.narg(query)::text))
       OR c.phone = ANY (@phones::text[]))
  AND (sqlc.narg(ts)::text IS NULL
       OR (c.profile_terms @@ to_tsquery('english', sqlc.narg(ts)::text) AND NOT c.id = ANY (@excluded_ids::text[]))
       OR c.id = ANY (@cv_ids::text[]))
  AND (cardinality(@availability::text[]) = 0 OR c.availability = ANY (@availability::text[]))
  AND (cardinality(@wa_kinds::text[]) = 0 OR (CASE
         WHEN c.work_authorization = 'local' THEN 'local'
         WHEN c.work_authorization = 'requires_visa' THEN 'needs_pass'
         WHEN c.work_authorization = 'expat_eligible' AND c.work_authorization_basis = 'permanent_resident' THEN 'pr'
         WHEN c.work_authorization = 'expat_eligible' THEN 'expat'
         ELSE 'unknown' END) = ANY (@wa_kinds::text[]))
  AND (cardinality(@nationality::text[]) = 0 OR c.nationality = ANY (@nationality::text[]))
  AND (cardinality(@residence::text[]) = 0 OR c.residence_country = ANY (@residence::text[]))
  AND (cardinality(@sources::text[]) = 0 OR c.source = ANY (@sources::text[]))
  AND (cardinality(@industries::text[]) = 0 OR lower(c.industry) = ANY (app_lower_array(@industries::text[])) OR c.id = ANY(@industry_candidate_ids::text[]))
  AND (cardinality(@skills::text[]) = 0
       OR (@all_skills::boolean AND app_lower_array(c.skills) @> app_lower_array(@skills::text[]))
       OR (NOT @all_skills::boolean AND app_lower_array(c.skills) && app_lower_array(@skills::text[])))
  AND (cardinality(@tags::text[]) = 0 OR app_lower_array(c.tags) @> app_lower_array(@tags::text[]))
  AND (sqlc.narg(location)::text IS NULL
       OR c.location ILIKE '%' || replace(replace(replace(sqlc.narg(location)::text, '\', '\\'), '%', '\%'), '_', '\_') || '%')
  AND (sqlc.narg(notice_max_days)::int IS NULL OR (c.notice_period_value IS NOT NULL AND
       c.notice_period_value * CASE c.notice_period_unit WHEN 'day' THEN 1 WHEN 'week' THEN 7 ELSE 30 END <= sqlc.narg(notice_max_days)::int))
  -- Row-level security shows salaries only to roles that may see them (the service refuses this filter to others).
  AND (sqlc.narg(salary_max)::text IS NULL OR EXISTS (
       SELECT 1 FROM candidate_compensation cc
       WHERE cc.candidate_id = c.id AND cc.expected_currency = sqlc.narg(salary_currency)::text
         AND cc.expected_min * CASE cc.expected_period WHEN 'year' THEN 1.0 / 12 WHEN 'day' THEN 22 WHEN 'hour' THEN 176 ELSE 1 END
             <= (sqlc.narg(salary_max)::text)::numeric))
  AND (sqlc.narg(owner)::text IS NULL OR c.owner_id = sqlc.narg(owner)::text)
  AND (sqlc.narg(updated_within_days)::int IS NULL OR c.updated_at >= now() - make_interval(days => sqlc.narg(updated_within_days)::int))
ORDER BY c.id DESC
LIMIT @row_limit;

-- Amounts are numeric in the database; the Go code converts them to and
-- from exact decimal strings, so no precision is ever lost.
-- A page of candidates' salaries. Row-level security shows them only to
-- those with candidates.compensation:read: none for others.
-- name: ListCompensations :many
SELECT * FROM candidate_compensation WHERE candidate_id = ANY(@candidate_ids::text[]);

-- name: GetCompensation :one
SELECT * FROM candidate_compensation WHERE candidate_id = @candidate_id;

-- name: InsertCompensation :one
INSERT INTO candidate_compensation (
  candidate_id, tenant_id, current_amount, current_currency, current_period,
  expected_min, expected_max, expected_currency, expected_period, current_description, expected_description, salary_notes, updated_by
) VALUES (
  @candidate_id, app_current_tenant(),
  sqlc.narg(current_amount)::text::numeric, sqlc.narg(current_currency), sqlc.narg(current_period),
  sqlc.narg(expected_min)::text::numeric, sqlc.narg(expected_max)::text::numeric,
  sqlc.narg(expected_currency), sqlc.narg(expected_period), sqlc.narg(current_description), sqlc.narg(expected_description), sqlc.narg(salary_notes), app_current_user()
)
ON CONFLICT (candidate_id) DO NOTHING
RETURNING *;

-- name: UpdateCompensation :one
UPDATE candidate_compensation SET
  current_amount = sqlc.narg(current_amount)::text::numeric,
  current_currency = sqlc.narg(current_currency),
  current_period = sqlc.narg(current_period),
  expected_min = sqlc.narg(expected_min)::text::numeric,
  expected_max = sqlc.narg(expected_max)::text::numeric,
  expected_currency = sqlc.narg(expected_currency),
  expected_period = sqlc.narg(expected_period),
  -- A breakdown explains one figure: keep it only while that figure stays
  -- the same, unless new wording comes with the change.
  current_description = CASE WHEN sqlc.narg(current_amount)::text IS NULL THEN NULL
    WHEN sqlc.narg(current_description)::text IS NOT NULL THEN sqlc.narg(current_description)::text
    WHEN sqlc.narg(current_amount)::text::numeric = current_amount AND sqlc.narg(current_currency)::text = current_currency
      AND sqlc.narg(current_period)::text = current_period THEN current_description END,
  expected_description = CASE WHEN sqlc.narg(expected_min)::text IS NULL THEN NULL
    WHEN sqlc.narg(expected_description)::text IS NOT NULL THEN sqlc.narg(expected_description)::text
    WHEN sqlc.narg(expected_min)::text::numeric = expected_min AND sqlc.narg(expected_max)::text::numeric IS NOT DISTINCT FROM expected_max
      AND sqlc.narg(expected_currency)::text = expected_currency AND sqlc.narg(expected_period)::text = expected_period THEN expected_description END,
  salary_notes = CASE WHEN @salary_notes_set::boolean THEN sqlc.narg(salary_notes)::text ELSE salary_notes END,
  version = version + 1,
  updated_by = app_current_user(),
  updated_at = now()
WHERE candidate_id = @candidate_id AND version = @expected_version
RETURNING *;

-- name: CandidateBriefs :many
SELECT id, first_name, last_name, current_title, email, source FROM candidate WHERE id = ANY(@ids::text[]);

-- Possible duplicates: the same phone, or a name at least 60% alike
-- (trigram similarity, ignoring case and accents). Best matches first.
-- name: SimilarCandidates :many
SELECT id, first_name, last_name, email, phone, current_title, current_company, location, work_authorization, updated_at,
       (phone IS NOT NULL AND phone = sqlc.narg(phone)::text)::boolean AS same_phone,
       (similarity(name_text, app_normalize_text(@full_name::text)) >= 0.6)::boolean AS similar_name
FROM candidate
WHERE id <> @id
  -- name_text % ... is what the name index answers (it finds names at least
  -- 30% alike); the 60% rule is then checked on those few, not on everyone.
  AND ((phone IS NOT NULL AND phone = sqlc.narg(phone)::text)
       OR (name_text % app_normalize_text(@full_name::text)
           AND similarity(name_text, app_normalize_text(@full_name::text)) >= 0.6))
ORDER BY similarity(name_text, app_normalize_text(@full_name::text)) DESC, id DESC
LIMIT 10;

-- Merging: the duplicate's salary moves to the kept candidate (only when it
-- has none), then the duplicate is deleted.
-- name: MoveCompensation :execrows
UPDATE candidate_compensation SET candidate_id = @to_id WHERE candidate_id = @from_id;

-- name: DeleteCandidate :execrows
DELETE FROM candidate WHERE id = @id;

-- Values already used in this company, most used first, for suggestions
-- (keeps spellings consistent: one shared list).
-- name: UsedValues :many
SELECT kind::text AS kind, value::text AS value, n FROM (
  SELECT 'tag' AS kind, t AS value, count(*)::integer AS n FROM candidate, unnest(tags) AS t GROUP BY t
  UNION ALL
  SELECT 'skill', s, count(*)::integer FROM candidate, unnest(skills) AS s GROUP BY s
  UNION ALL
  SELECT 'industry', industry, count(*)::integer FROM candidate WHERE industry IS NOT NULL GROUP BY industry
  UNION ALL
  SELECT 'specialization', specialization, count(*)::integer FROM candidate WHERE specialization IS NOT NULL GROUP BY specialization
) used
ORDER BY kind, n DESC, value
LIMIT 2000;

-- Work history and education, newest first.
-- name: ListWork :many
SELECT * FROM candidate_work WHERE candidate_id = @candidate_id ORDER BY position;

-- name: ListEducation :many
SELECT * FROM candidate_education WHERE candidate_id = @candidate_id ORDER BY position;

-- name: DeleteWork :exec
DELETE FROM candidate_work WHERE candidate_id = @candidate_id;

-- name: DeleteEducation :exec
DELETE FROM candidate_education WHERE candidate_id = @candidate_id;

-- name: InsertWork :exec
INSERT INTO candidate_work (tenant_id, candidate_id, position, title, company, start_date, end_date, is_current, description)
VALUES (app_current_tenant(), @candidate_id, @position, sqlc.narg(title), sqlc.narg(company), sqlc.narg(start_date),
        sqlc.narg(end_date), @is_current, sqlc.narg(description));

-- name: InsertEducation :exec
INSERT INTO candidate_education (tenant_id, candidate_id, position, qualification, institution, end_year)
VALUES (app_current_tenant(), @candidate_id, @position, sqlc.narg(qualification), sqlc.narg(institution), sqlc.narg(end_year));

-- A change to a candidate's own records (history) moves their version on, so
-- forms opened before it are stale. No row = stale version (or not found).
-- name: TouchCandidate :one
UPDATE candidate SET version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- Merging: the duplicate's history moves to the kept candidate only if the
-- kept one has none (histories are not interleaved).
-- name: MoveWorkIfNone :exec
UPDATE candidate_work AS m SET candidate_id = sqlc.arg(to_id)::text
WHERE m.candidate_id = sqlc.arg(from_id)::text
  AND NOT EXISTS (SELECT 1 FROM candidate_work w WHERE w.candidate_id = sqlc.arg(to_id)::text);

-- name: MoveEducationIfNone :exec
UPDATE candidate_education AS m SET candidate_id = sqlc.arg(to_id)::text
WHERE m.candidate_id = sqlc.arg(from_id)::text
  AND NOT EXISTS (SELECT 1 FROM candidate_education e WHERE e.candidate_id = sqlc.arg(to_id)::text);

-- How many candidates the company has (the data protection dashboard).
-- name: CountCandidates :one
SELECT count(*)::integer FROM candidate;

-- Through a database function: whoever hands a candidate over may no longer
-- see them, which a plain UPDATE would be refused for (row-level security).
-- name: SetCandidateOwner :one
SELECT app_set_candidate_owner(@id::text, sqlc.narg(owner_id)::text)::boolean AS changed;

-- A job's market map: candidates whose current title has every word given
-- (no names or contact details: only what the map counts).
-- name: MarketPool :many
SELECT id, current_company, location, residence_country, availability, work_authorization, work_authorization_basis,
       notice_period_value, notice_period_unit
FROM candidate
WHERE current_title IS NOT NULL
  AND app_normalize_text(current_title) LIKE ALL (app_search_patterns(@words::text))
LIMIT 5000;

-- Their pay, for roles that may see salaries (the table's policy enforces it too).
-- name: MarketPay :many
SELECT coalesce(current_amount::text, '')::text AS current_amount, current_currency, current_period,
       coalesce(expected_min::text, '')::text AS expected_min, expected_currency, expected_period
FROM candidate_compensation
WHERE candidate_id = ANY(@ids::text[]);

-- The home page search (Kano's /find), one search: by @phones (the whole
-- numbers a phone number typed can be), by @email exactly, or by @name
-- (every word in the name, email, title or company; with @similar_names,
-- names alike by trigrams or containing one of @probes, the closest
-- first). With @company_words (LIKE patterns), only those whose current or
-- a past company has every word; the service then checks it word for word.
-- name: FindCandidates :many
SELECT c.id, c.first_name, c.last_name, c.email, c.phone, c.current_title, c.current_company
FROM candidate c
WHERE CASE
    WHEN cardinality(@phones::text[]) > 0 THEN c.phone = ANY (@phones::text[])
    WHEN sqlc.narg(email)::text IS NOT NULL THEN c.email = lower(sqlc.narg(email)::text)
    WHEN sqlc.arg(similar_names)::boolean
      THEN app_normalize_text(@name::text) <% c.name_text OR c.name_text LIKE ANY (@probes::text[])
    ELSE c.search_text LIKE ALL (app_search_patterns(@name::text))
  END
  AND (cardinality(@company_words::text[]) = 0
       OR app_normalize_text(coalesce(c.current_company, '')) LIKE ALL (@company_words::text[])
       OR EXISTS (SELECT 1 FROM candidate_work w
                  WHERE w.candidate_id = c.id AND app_normalize_text(coalesce(w.company, '')) LIKE ALL (@company_words::text[])))
ORDER BY CASE WHEN sqlc.arg(similar_names)::boolean THEN word_similarity(app_normalize_text(@name::text), c.name_text) END DESC,
  c.id DESC
LIMIT @row_limit;

-- The companies candidates work or worked at, newest first.
-- name: ListWorkCompanies :many
SELECT candidate_id, company::text AS company, title, is_current
FROM candidate_work
WHERE candidate_id = ANY (@candidate_ids::text[]) AND company IS NOT NULL
ORDER BY candidate_id, position;

-- Screening calls, latest first (by when they were, then when logged).
-- name: ListScreeningCalls :many
SELECT * FROM screening_call WHERE candidate_id = @candidate_id ORDER BY called_on DESC, created_at DESC;

-- The date of the latest call that set the salary (none: no such call).
-- name: LatestSalaryCall :one
SELECT max(called_on)::date AS called_on FROM screening_call WHERE candidate_id = @candidate_id AND salary_recorded;

-- name: InsertScreeningCall :one
INSERT INTO screening_call (tenant_id, candidate_id, called_on, called_by, note_id, salary_recorded, created_by)
VALUES (app_current_tenant(), @candidate_id, @called_on, @called_by, sqlc.narg(note_id), @salary_recorded, app_current_user())
RETURNING *;

-- Merging: the duplicate's screening calls go to the kept candidate.
-- name: MoveScreeningCalls :exec
UPDATE screening_call SET candidate_id = @to_id WHERE candidate_id = @from_id;

-- name: LegacyIndustries :many
SELECT id, industry::text FROM candidate WHERE industry IS NOT NULL;
-- name: SetIndustrySummary :exec
UPDATE candidate SET industry = sqlc.narg(industry), version = version + 1, updated_at = now()
WHERE id = @id AND industry IS DISTINCT FROM sqlc.narg(industry);
-- Many at once, after an industry is renamed, moved or replaced.
-- name: SetIndustrySummaries :exec
UPDATE candidate r SET industry = v.industry, version = r.version + 1, updated_at = now()
FROM (SELECT unnest(@ids::text[]) AS id, unnest(@industries::text[]) AS industry) v
WHERE r.id = v.id AND r.industry IS DISTINCT FROM v.industry;

-- name: InsertScreeningSalarySnapshot :exec
INSERT INTO screening_call_salary(call_id,snapshot) VALUES (@call_id,@snapshot);
-- name: ScreeningSalarySnapshots :many
SELECT * FROM screening_call_salary WHERE call_id=ANY(@call_ids::text[]);

-- ListCandidates in another order (CandidateSort, not newest), paged by
-- offset. A separate query so the newest-first list keeps its index plan.
-- Keep its filters in step with ListCandidates.
-- name: ListCandidatesSorted :many
SELECT c.id, c.first_name, c.last_name, c.email, c.phone, c.current_title, c.current_company,
       c.location, c.work_authorization, c.tags, c.updated_at,
       c.availability, c.availability_since, c.notice_period_value, c.notice_period_unit, c.work_authorization_basis,
       c.nationality, c.residence_country, c.source, c.industry
FROM candidate c
-- Salaries only for roles that may see them (row-level security); for others the sort is newest first.
LEFT JOIN candidate_compensation cc ON cc.candidate_id = c.id
WHERE (sqlc.narg(query)::text IS NULL
       OR CASE WHEN sqlc.arg(similar_names)::boolean
            THEN app_normalize_text(sqlc.narg(query)::text) <% c.name_text
            ELSE c.search_text LIKE ALL (app_search_patterns(sqlc.narg(query)::text))
              OR c.phone = ANY (@phones::text[])
          END)
  AND (sqlc.narg(tag)::text IS NULL OR app_lower_array(c.tags) @> ARRAY[lower(sqlc.narg(tag)::text)])
  AND (sqlc.narg(skill)::text IS NULL OR app_lower_array(c.skills) @> ARRAY[lower(sqlc.narg(skill)::text)])
  AND (sqlc.narg(owner)::text IS NULL OR c.owner_id = sqlc.narg(owner)::text)
ORDER BY
  CASE WHEN @sort::text = 'name' THEN app_normalize_text(c.first_name || ' ' || c.last_name) END ASC,
  CASE WHEN @sort::text = 'name_desc' THEN app_normalize_text(c.first_name || ' ' || c.last_name) END DESC,
  CASE WHEN @sort::text = 'notice' THEN c.notice_period_value * CASE c.notice_period_unit WHEN 'day' THEN 1 WHEN 'week' THEN 7 ELSE 30 END END ASC NULLS LAST,
  CASE WHEN @sort::text = 'notice_desc' THEN c.notice_period_value * CASE c.notice_period_unit WHEN 'day' THEN 1 WHEN 'week' THEN 7 ELSE 30 END END DESC NULLS LAST,
  -- Salaries in different currencies are never compared: ringgit first, then each other currency together.
  CASE WHEN @sort::text LIKE 'current_salary%' THEN CASE cc.current_currency WHEN 'MYR' THEN '' ELSE cc.current_currency END END ASC NULLS LAST,
  CASE WHEN @sort::text LIKE 'expected_salary%' THEN CASE cc.expected_currency WHEN 'MYR' THEN '' ELSE cc.expected_currency END END ASC NULLS LAST,
  CASE WHEN @sort::text = 'current_salary_asc' THEN cc.current_amount * CASE cc.current_period WHEN 'year' THEN 1.0 / 12 WHEN 'day' THEN 22 WHEN 'hour' THEN 176 ELSE 1 END END ASC NULLS LAST,
  CASE WHEN @sort::text = 'current_salary_desc' THEN cc.current_amount * CASE cc.current_period WHEN 'year' THEN 1.0 / 12 WHEN 'day' THEN 22 WHEN 'hour' THEN 176 ELSE 1 END END DESC NULLS LAST,
  CASE WHEN @sort::text = 'expected_salary_asc' THEN cc.expected_min * CASE cc.expected_period WHEN 'year' THEN 1.0 / 12 WHEN 'day' THEN 22 WHEN 'hour' THEN 176 ELSE 1 END END ASC NULLS LAST,
  CASE WHEN @sort::text = 'expected_salary_desc' THEN cc.expected_min * CASE cc.expected_period WHEN 'year' THEN 1.0 / 12 WHEN 'day' THEN 22 WHEN 'hour' THEN 176 ELSE 1 END END DESC NULLS LAST,
  c.id DESC
LIMIT @row_limit OFFSET @row_offset;

-- SearchCandidates in another order (CandidateSort, not newest), paged by
-- offset. A separate query so the newest-first search keeps its index plan.
-- Keep its filters in step with SearchCandidates (a test runs both).
-- name: SearchCandidatesSorted :many
SELECT c.id, c.first_name, c.last_name, c.email, c.phone, c.current_title, c.current_company,
       c.location, c.work_authorization, c.tags, c.updated_at,
       c.availability, c.availability_since, c.notice_period_value, c.notice_period_unit, c.work_authorization_basis,
       c.nationality, c.residence_country, c.source, c.industry,
       (sqlc.narg(ts)::text IS NOT NULL AND c.profile_terms @@ to_tsquery('english', sqlc.narg(ts)::text))::boolean AS profile_hit
FROM candidate c
-- Salaries only for roles that may see them (row-level security); for others the sort is newest first.
LEFT JOIN candidate_compensation cc ON cc.candidate_id = c.id
WHERE (sqlc.narg(query)::text IS NULL
       OR c.search_text LIKE ALL (app_search_patterns(sqlc.narg(query)::text))
       OR c.phone = ANY (@phones::text[]))
  AND (sqlc.narg(ts)::text IS NULL
       OR (c.profile_terms @@ to_tsquery('english', sqlc.narg(ts)::text) AND NOT c.id = ANY (@excluded_ids::text[]))
       OR c.id = ANY (@cv_ids::text[]))
  AND (cardinality(@availability::text[]) = 0 OR c.availability = ANY (@availability::text[]))
  AND (cardinality(@wa_kinds::text[]) = 0 OR (CASE
         WHEN c.work_authorization = 'local' THEN 'local'
         WHEN c.work_authorization = 'requires_visa' THEN 'needs_pass'
         WHEN c.work_authorization = 'expat_eligible' AND c.work_authorization_basis = 'permanent_resident' THEN 'pr'
         WHEN c.work_authorization = 'expat_eligible' THEN 'expat'
         ELSE 'unknown' END) = ANY (@wa_kinds::text[]))
  AND (cardinality(@nationality::text[]) = 0 OR c.nationality = ANY (@nationality::text[]))
  AND (cardinality(@residence::text[]) = 0 OR c.residence_country = ANY (@residence::text[]))
  AND (cardinality(@sources::text[]) = 0 OR c.source = ANY (@sources::text[]))
  AND (cardinality(@industries::text[]) = 0 OR lower(c.industry) = ANY (app_lower_array(@industries::text[])) OR c.id = ANY(@industry_candidate_ids::text[]))
  AND (cardinality(@skills::text[]) = 0
       OR (@all_skills::boolean AND app_lower_array(c.skills) @> app_lower_array(@skills::text[]))
       OR (NOT @all_skills::boolean AND app_lower_array(c.skills) && app_lower_array(@skills::text[])))
  AND (cardinality(@tags::text[]) = 0 OR app_lower_array(c.tags) @> app_lower_array(@tags::text[]))
  AND (sqlc.narg(location)::text IS NULL
       OR c.location ILIKE '%' || replace(replace(replace(sqlc.narg(location)::text, '\', '\\'), '%', '\%'), '_', '\_') || '%')
  AND (sqlc.narg(notice_max_days)::int IS NULL OR (c.notice_period_value IS NOT NULL AND
       c.notice_period_value * CASE c.notice_period_unit WHEN 'day' THEN 1 WHEN 'week' THEN 7 ELSE 30 END <= sqlc.narg(notice_max_days)::int))
  -- Row-level security shows salaries only to roles that may see them (the service refuses this filter to others).
  AND (sqlc.narg(salary_max)::text IS NULL OR EXISTS (
       SELECT 1 FROM candidate_compensation cc
       WHERE cc.candidate_id = c.id AND cc.expected_currency = sqlc.narg(salary_currency)::text
         AND cc.expected_min * CASE cc.expected_period WHEN 'year' THEN 1.0 / 12 WHEN 'day' THEN 22 WHEN 'hour' THEN 176 ELSE 1 END
             <= (sqlc.narg(salary_max)::text)::numeric))
  AND (sqlc.narg(owner)::text IS NULL OR c.owner_id = sqlc.narg(owner)::text)
  AND (sqlc.narg(updated_within_days)::int IS NULL OR c.updated_at >= now() - make_interval(days => sqlc.narg(updated_within_days)::int))
ORDER BY
  CASE WHEN @sort::text = 'name' THEN app_normalize_text(c.first_name || ' ' || c.last_name) END ASC,
  CASE WHEN @sort::text = 'name_desc' THEN app_normalize_text(c.first_name || ' ' || c.last_name) END DESC,
  CASE WHEN @sort::text = 'notice' THEN c.notice_period_value * CASE c.notice_period_unit WHEN 'day' THEN 1 WHEN 'week' THEN 7 ELSE 30 END END ASC NULLS LAST,
  CASE WHEN @sort::text = 'notice_desc' THEN c.notice_period_value * CASE c.notice_period_unit WHEN 'day' THEN 1 WHEN 'week' THEN 7 ELSE 30 END END DESC NULLS LAST,
  -- Salaries in different currencies are never compared: ringgit first, then each other currency together.
  CASE WHEN @sort::text LIKE 'current_salary%' THEN CASE cc.current_currency WHEN 'MYR' THEN '' ELSE cc.current_currency END END ASC NULLS LAST,
  CASE WHEN @sort::text LIKE 'expected_salary%' THEN CASE cc.expected_currency WHEN 'MYR' THEN '' ELSE cc.expected_currency END END ASC NULLS LAST,
  CASE WHEN @sort::text = 'current_salary_asc' THEN cc.current_amount * CASE cc.current_period WHEN 'year' THEN 1.0 / 12 WHEN 'day' THEN 22 WHEN 'hour' THEN 176 ELSE 1 END END ASC NULLS LAST,
  CASE WHEN @sort::text = 'current_salary_desc' THEN cc.current_amount * CASE cc.current_period WHEN 'year' THEN 1.0 / 12 WHEN 'day' THEN 22 WHEN 'hour' THEN 176 ELSE 1 END END DESC NULLS LAST,
  CASE WHEN @sort::text = 'expected_salary_asc' THEN cc.expected_min * CASE cc.expected_period WHEN 'year' THEN 1.0 / 12 WHEN 'day' THEN 22 WHEN 'hour' THEN 176 ELSE 1 END END ASC NULLS LAST,
  CASE WHEN @sort::text = 'expected_salary_desc' THEN cc.expected_min * CASE cc.expected_period WHEN 'year' THEN 1.0 / 12 WHEN 'day' THEN 22 WHEN 'hour' THEN 176 ELSE 1 END END DESC NULLS LAST,
  c.id DESC
LIMIT @row_limit OFFSET @row_offset;

