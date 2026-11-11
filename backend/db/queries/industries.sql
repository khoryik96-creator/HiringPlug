-- name: EnsureCatalogue :exec
INSERT INTO industry_catalogue(tenant_id) VALUES (app_current_tenant()) ON CONFLICT DO NOTHING;
-- name: LockCatalogue :one
SELECT version FROM industry_catalogue WHERE tenant_id = app_current_tenant() FOR UPDATE;
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
-- name: AllIndustryAssignments :many
SELECT * FROM industry_assignment ORDER BY candidate_id, company_id;
-- name: CandidatesInIndustries :many
SELECT DISTINCT candidate_id::text FROM industry_assignment WHERE candidate_id IS NOT NULL AND category_id = ANY(@ids::text[]);
