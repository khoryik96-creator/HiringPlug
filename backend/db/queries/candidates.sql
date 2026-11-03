-- Queries owned by the candidates module. Run inside a tenant-scoped
-- transaction: row-level security limits every query to the current tenant.

-- name: InsertCandidate :one
INSERT INTO candidate (
  tenant_id, first_name, last_name, email, phone, nationality,
  work_authorization, work_authorization_basis, work_authorization_note,
  notice_period_value, notice_period_unit, industry, specialization,
  current_title, current_company, location, source, tags, skills, created_by
) VALUES (
  app_current_tenant(), @first_name, @last_name, sqlc.narg(email), sqlc.narg(phone), sqlc.narg(nationality),
  sqlc.narg(work_authorization), sqlc.narg(work_authorization_basis), sqlc.narg(work_authorization_note),
  sqlc.narg(notice_period_value), sqlc.narg(notice_period_unit), sqlc.narg(industry), sqlc.narg(specialization),
  sqlc.narg(current_title), sqlc.narg(current_company), sqlc.narg(location), sqlc.narg(source), @tags, @skills, app_current_user()
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
-- name: ListCandidates :many
SELECT id, first_name, last_name, email, phone, current_title, current_company,
       location, work_authorization, tags, updated_at
FROM candidate
WHERE (sqlc.narg(cursor)::text IS NULL OR id < sqlc.narg(cursor)::text)
  AND (sqlc.narg(query)::text IS NULL OR search_text LIKE '%' || app_normalize_text(sqlc.narg(query)::text) || '%')
  AND (sqlc.narg(tag)::text IS NULL OR app_lower_array(tags) @> ARRAY[lower(sqlc.narg(tag)::text)])
  AND (sqlc.narg(skill)::text IS NULL OR app_lower_array(skills) @> ARRAY[lower(sqlc.narg(skill)::text)])
ORDER BY id DESC
LIMIT @row_limit;

-- Amounts are numeric in the database; the Go code converts them to and
-- from exact decimal strings, so no precision is ever lost.
-- name: GetCompensation :one
SELECT * FROM candidate_compensation WHERE candidate_id = @candidate_id;

-- name: InsertCompensation :one
INSERT INTO candidate_compensation (
  candidate_id, tenant_id, current_amount, current_currency, current_period,
  expected_min, expected_max, expected_currency, expected_period, updated_by
) VALUES (
  @candidate_id, app_current_tenant(),
  sqlc.narg(current_amount)::text::numeric, sqlc.narg(current_currency), sqlc.narg(current_period),
  sqlc.narg(expected_min)::text::numeric, sqlc.narg(expected_max)::text::numeric,
  sqlc.narg(expected_currency), sqlc.narg(expected_period), app_current_user()
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
       (similarity(app_normalize_text(first_name || ' ' || last_name), app_normalize_text(@full_name::text)) >= 0.6)::boolean AS similar_name
FROM candidate
WHERE id <> @id
  AND ((phone IS NOT NULL AND phone = sqlc.narg(phone)::text)
       OR similarity(app_normalize_text(first_name || ' ' || last_name), app_normalize_text(@full_name::text)) >= 0.6)
ORDER BY similarity(app_normalize_text(first_name || ' ' || last_name), app_normalize_text(@full_name::text)) DESC, id DESC
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
