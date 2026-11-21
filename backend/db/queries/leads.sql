-- Queries owned by the leads module. Run inside a tenant-scoped transaction.

-- name: ListSources :many
SELECT * FROM lead_source ORDER BY lower(name), id LIMIT 500;

-- name: GetSource :one
SELECT * FROM lead_source WHERE id = @id;

-- name: InsertSource :one
INSERT INTO lead_source (tenant_id, kind, identifier, name, company_id, next_check_at, created_by)
VALUES (app_current_tenant(), @kind, @identifier, @name, sqlc.narg(company_id), @next_check_at, app_current_user())
RETURNING *;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateSource :one
UPDATE lead_source SET name = @name, company_id = sqlc.narg(company_id), active = @active,
  next_check_at = sqlc.narg(next_check_at), version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- name: ScheduleCheck :one
UPDATE lead_source SET next_check_at = @next_check_at WHERE id = @id RETURNING *;

-- name: RecordCheck :exec
UPDATE lead_source SET last_checked_at = now(), last_error = sqlc.narg(last_error), open_leads = @open_leads,
  next_check_at = sqlc.narg(next_check_at), name = @name
WHERE id = @id;

-- name: DeleteSource :execrows
DELETE FROM lead_source WHERE id = @id;

-- name: SourceLeads :many
SELECT * FROM job_lead WHERE source_id = @source_id;

-- name: InsertLead :one
INSERT INTO job_lead (tenant_id, source_id, source_kind, external_id, company_name, company_id, title, location, url, posted_at, notes, created_by)
VALUES (app_current_tenant(), sqlc.narg(source_id), sqlc.narg(source_kind), sqlc.narg(external_id), @company_name, sqlc.narg(company_id),
        @title, sqlc.narg(location), sqlc.narg(url), sqlc.narg(posted_at), sqlc.narg(notes), app_current_user())
RETURNING *;

-- An opening seen again: still listed (reopened if it had closed), with its latest details.
-- name: SeenLead :exec
UPDATE job_lead SET last_seen_at = now(), closed_at = NULL, title = @title, location = sqlc.narg(location), url = sqlc.narg(url),
  company_name = @company_name
WHERE id = @id;

-- name: CloseLead :exec
UPDATE job_lead SET closed_at = now() WHERE id = @id AND closed_at IS NULL;

-- name: GetLead :one
SELECT * FROM job_lead WHERE id = @id;

-- name: ListLeads :many
SELECT sqlc.embed(l), (f.lead_id IS NOT NULL)::boolean AS favorite
FROM job_lead l
LEFT JOIN job_lead_favorite f ON f.tenant_id = l.tenant_id AND f.lead_id = l.id AND f.user_id = app_current_user()
WHERE (sqlc.narg(status)::text IS NULL OR l.status = sqlc.narg(status))
  AND (sqlc.narg(source_id)::text IS NULL OR l.source_id = sqlc.narg(source_id))
  AND (sqlc.narg(open)::boolean IS NULL OR (l.closed_at IS NULL) = sqlc.narg(open))
  AND (sqlc.narg(q)::text IS NULL OR l.company_name ILIKE '%' || sqlc.narg(q) || '%' OR l.title ILIKE '%' || sqlc.narg(q) || '%')
  AND (NOT @favorites::boolean OR f.lead_id IS NOT NULL)
  AND (sqlc.narg(cursor)::text IS NULL OR l.id < sqlc.narg(cursor))
ORDER BY l.id DESC
LIMIT @row_limit;

-- name: LeadFavorite :one
SELECT EXISTS (SELECT 1 FROM job_lead_favorite WHERE lead_id = @lead_id AND user_id = app_current_user())::boolean AS favorite;

-- name: AddLeadFavorite :execrows
INSERT INTO job_lead_favorite (tenant_id, user_id, lead_id)
SELECT tenant_id, app_current_user(), id FROM job_lead WHERE id = @lead_id
ON CONFLICT DO NOTHING;

-- name: RemoveLeadFavorite :execrows
DELETE FROM job_lead_favorite WHERE lead_id = @lead_id AND user_id = app_current_user();

-- name: UpdateLead :one
UPDATE job_lead SET status = @status, notes = sqlc.narg(notes), company_id = sqlc.narg(company_id), version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- Discovery uses official feed rows and their observed locations, without
-- letting ambiguous remote postings stand in for a country.
-- name: LeadDiscoveryPage :many
SELECT sqlc.embed(l), (f.lead_id IS NOT NULL)::boolean AS favorite
FROM job_lead l
JOIN lead_source s ON s.id = l.source_id AND s.tenant_id = l.tenant_id AND s.active
LEFT JOIN job_lead_favorite f ON f.tenant_id = l.tenant_id AND f.lead_id = l.id AND f.user_id = app_current_user()
WHERE l.closed_at IS NULL AND l.source_id IS NOT NULL AND l.url IS NOT NULL
  AND l.status <> 'ignored'
  AND l.source_kind = ANY(@source_kinds::text[])
  AND l.location ~* @location_re::text
  AND to_tsvector('simple', l.title) @@ to_tsquery('simple', @title_query::text)
  AND (sqlc.narg(cursor)::text IS NULL OR l.id < sqlc.narg(cursor))
ORDER BY l.id DESC
LIMIT @row_limit;

-- name: GetLeadEvidence :one
SELECT * FROM job_lead_evidence WHERE lead_id = @lead_id;

-- name: SaveLeadEvidence :exec
INSERT INTO job_lead_evidence (tenant_id, lead_id, description)
VALUES (app_current_tenant(), @lead_id, @description)
ON CONFLICT (lead_id) DO UPDATE SET description = EXCLUDED.description, observed_at = now();
