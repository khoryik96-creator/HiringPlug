-- Queries owned by the companies module. Run inside a tenant-scoped
-- transaction: row-level security limits every query to the current tenant.

-- name: InsertCompany :one
INSERT INTO company (
  tenant_id, name, type, website, industry, size, phone, address, city, country,
  registration_number, about, created_by
) VALUES (
  app_current_tenant(), @name, @type, sqlc.narg(website), sqlc.narg(industry), sqlc.narg(size),
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
  website = sqlc.narg(website),
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
SELECT id, name, type, industry, city, website, updated_at
FROM company
WHERE (sqlc.narg(cursor)::text IS NULL OR id < sqlc.narg(cursor)::text)
  AND (sqlc.narg(type)::text IS NULL OR type = sqlc.narg(type)::text)
  AND (sqlc.narg(query)::text IS NULL OR search_text LIKE '%' || app_normalize_text(sqlc.narg(query)::text) || '%')
ORDER BY id DESC
LIMIT @row_limit;
