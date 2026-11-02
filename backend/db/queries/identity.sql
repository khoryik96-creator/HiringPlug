-- Queries owned by the identity module. Run inside a tenant-scoped transaction.

-- name: GetUser :one
SELECT id, tenant_id, email, name, role, active FROM app_user WHERE id = @id;

-- name: GetTenant :one
SELECT id, name, careers_slug::text AS careers_slug FROM tenant WHERE id = @id;

-- name: UserNames :many
SELECT id, name FROM app_user WHERE id = ANY(@ids::text[]);

-- The company behind a public careers page address (no tenant scope yet:
-- app_careers_tenant returns only the company's id and name).
-- name: CareersTenant :one
SELECT tenant_id::text AS tenant_id, tenant_name::text AS tenant_name FROM app_careers_tenant(@slug::text);

-- name: ListActiveUsers :many
SELECT id, name, role FROM app_user WHERE active ORDER BY name LIMIT 500;

-- Which of these users are active in the tenant (to check interviewers and owners).
-- name: ActiveUserRoles :many
SELECT id, role FROM app_user WHERE active AND id = ANY(@ids::text[]);

-- Active users' names and addresses (to email interviewers).
-- name: ActiveUserEmails :many
SELECT id, name, email FROM app_user WHERE active AND id = ANY(@ids::text[]);
