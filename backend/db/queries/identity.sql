-- Queries owned by the identity module. Run inside a tenant-scoped transaction.

-- name: GetUser :one
SELECT id, tenant_id, email, name, role, active FROM app_user WHERE id = @id;

-- name: GetTenant :one
SELECT id, name FROM tenant WHERE id = @id;

-- name: UserNames :many
SELECT id, name FROM app_user WHERE id = ANY(@ids::text[]);
