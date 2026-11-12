-- name: EnsureCatalogue :exec
INSERT INTO industry_catalogue(tenant_id) VALUES (app_current_tenant()) ON CONFLICT DO NOTHING;
-- name: LockCatalogue :one
SELECT version FROM industry_catalogue WHERE tenant_id = app_current_tenant() FOR UPDATE;
-- Record saves only read the catalogue: they share this lock, so they never
-- wait for each other, only for a catalogue edit (which takes it exclusively).
-- name: ShareCatalogue :one
SELECT version FROM industry_catalogue WHERE tenant_id = app_current_tenant() FOR SHARE;
-- A label an older client sends that the catalogue lacks; two saves may add it at once.
-- name: InsertLegacyIndustry :one
INSERT INTO industry(label, label_key, aliases, retired, source_key)
VALUES (@label, @label_key, '{}', true, @source_key)
ON CONFLICT (tenant_id, label_key) DO UPDATE SET label_key = excluded.label_key RETURNING *;
-- name: CatalogueVersion :one
SELECT version FROM industry_catalogue WHERE tenant_id = app_current_tenant();
-- name: BumpCatalogue :exec
UPDATE industry_catalogue SET version = version + 1 WHERE tenant_id = app_current_tenant();
-- name: ListIndustries :many
WITH usage AS (
 SELECT category_id, candidate_id, company_id FROM industry_assignment
 UNION ALL
 SELECT i.parent_id, a.candidate_id, a.company_id FROM industry_assignment a JOIN industry i ON i.id=a.category_id WHERE i.parent_id IS NOT NULL
), counts AS (
 SELECT category_id,count(DISTINCT candidate_id)::integer AS candidate_count,count(DISTINCT company_id)::integer AS company_count FROM usage GROUP BY category_id
)
SELECT i.*,coalesce(c.candidate_count,0)::integer AS candidate_count,coalesce(c.company_count,0)::integer AS company_count
FROM industry i LEFT JOIN counts c ON c.category_id=i.id ORDER BY i.parent_id NULLS FIRST,i.label,i.id;
-- name: InsertIndustry :one
INSERT INTO industry(parent_id, label, label_key, aliases, retired, source_key)
VALUES (sqlc.narg(parent_id), @label, @label_key, @aliases::text[], @retired, sqlc.narg(source_key)) RETURNING *;
-- name: UpdateIndustry :exec
UPDATE industry SET parent_id = sqlc.narg(parent_id), label = @label, label_key = @label_key, aliases = @aliases::text[], retired = @retired WHERE id = @id;
-- name: RetireIndustry :exec
UPDATE industry SET retired = true WHERE id = @id OR parent_id = @id;
-- name: DeleteIndustry :exec
DELETE FROM industry WHERE id = @id;
-- name: SubjectIndustries :many
SELECT * FROM industry_assignment WHERE candidate_id = sqlc.narg(candidate_id) OR company_id = sqlc.narg(company_id);
-- name: ClearSubjectIndustries :exec
DELETE FROM industry_assignment WHERE candidate_id = sqlc.narg(candidate_id) OR company_id = sqlc.narg(company_id);
-- name: AssignIndustry :exec
INSERT INTO industry_assignment(category_id, candidate_id, company_id, is_primary) VALUES (@category_id, sqlc.narg(candidate_id), sqlc.narg(company_id), @is_primary);
-- name: PrimaryIndustryAssignments :many
SELECT * FROM industry_assignment WHERE is_primary;
-- Replacing categories: who had them (and whether as their current one),
-- then those rows go, then the replacement is added or made current.
-- name: SubjectsInIndustries :many
SELECT candidate_id, company_id, bool_or(is_primary)::boolean AS was_primary FROM industry_assignment
WHERE category_id = ANY(@ids::text[]) GROUP BY candidate_id, company_id;
-- name: DeleteIndustryAssignments :exec
DELETE FROM industry_assignment WHERE category_id = ANY(@ids::text[]);
-- name: AddReplacementIndustry :exec
WITH moved AS (
  SELECT unnest(@candidate_ids::text[]) AS candidate_id, unnest(@company_ids::text[]) AS company_id, unnest(@primaries::boolean[]) AS was_primary
), made_current AS (
  UPDATE industry_assignment a SET is_primary = true FROM moved m
  WHERE a.category_id = @category_id AND m.was_primary
    AND a.candidate_id IS NOT DISTINCT FROM nullif(m.candidate_id, '') AND a.company_id IS NOT DISTINCT FROM nullif(m.company_id, '')
)
INSERT INTO industry_assignment(category_id, candidate_id, company_id, is_primary)
SELECT @category_id, nullif(m.candidate_id, ''), nullif(m.company_id, ''), m.was_primary FROM moved m
WHERE NOT EXISTS (SELECT 1 FROM industry_assignment a WHERE a.category_id = @category_id
  AND a.candidate_id IS NOT DISTINCT FROM nullif(m.candidate_id, '') AND a.company_id IS NOT DISTINCT FROM nullif(m.company_id, ''));
-- name: AllIndustryAssignments :many
SELECT * FROM industry_assignment ORDER BY candidate_id, company_id;
-- name: CandidatesInIndustries :many
SELECT DISTINCT candidate_id::text FROM industry_assignment WHERE candidate_id IS NOT NULL AND category_id = ANY(@ids::text[]);
