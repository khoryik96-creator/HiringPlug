-- Queries owned by the privacy module. Run inside a tenant-scoped transaction.

-- name: GetConsent :one
SELECT * FROM candidate_consent WHERE candidate_id = @candidate_id;

-- name: InsertConsent :one
INSERT INTO candidate_consent (candidate_id, tenant_id, status, method, day, note, recorded_by)
VALUES (@candidate_id, app_current_tenant(), @status, @method, @day, sqlc.narg(note), app_current_user())
ON CONFLICT (candidate_id) DO NOTHING
RETURNING *;

-- Optimistic concurrency: no row returned = stale version.
-- name: UpdateConsent :one
UPDATE candidate_consent
SET status = @status, method = @method, day = @day, note = sqlc.narg(note), recorded_by = app_current_user(),
    version = version + 1, updated_at = now()
WHERE candidate_id = @candidate_id AND version = @version
RETURNING *;

-- Consent given on a form overrides what was recorded: applying is a new consent.
-- A withdrawal stands: the public form cannot prove who filled it in, so it
-- never turns "withdrawn" back into "given" (no row returned then).
-- name: GiveConsent :one
INSERT INTO candidate_consent (candidate_id, tenant_id, status, method, day, recorded_by)
VALUES (@candidate_id, app_current_tenant(), 'given', @method, @day, app_current_user())
ON CONFLICT (candidate_id) DO UPDATE
SET status = 'given', method = EXCLUDED.method, day = EXCLUDED.day, note = NULL,
    recorded_by = EXCLUDED.recorded_by, version = candidate_consent.version + 1, updated_at = now()
WHERE candidate_consent.status <> 'withdrawn'
RETURNING candidate_id;

-- name: ConsentCounts :one
SELECT count(*) FILTER (WHERE status = 'given')::integer AS given,
       count(*) FILTER (WHERE status = 'withdrawn')::integer AS withdrawn
FROM candidate_consent;

-- name: ListWithdrawn :many
SELECT candidate_id, day FROM candidate_consent WHERE status = 'withdrawn' ORDER BY day DESC, candidate_id LIMIT 200;

-- name: InsertRequest :one
INSERT INTO privacy_request (tenant_id, candidate_id, kind, received_on, details, created_by)
VALUES (app_current_tenant(), @candidate_id, @kind, @received_on, sqlc.narg(details), app_current_user())
RETURNING *;

-- name: GetRequest :one
SELECT * FROM privacy_request WHERE id = @id;

-- name: ListCandidateRequests :many
SELECT * FROM privacy_request WHERE candidate_id = @candidate_id ORDER BY created_at DESC, id DESC LIMIT 200;

-- name: ListOpenRequests :many
SELECT * FROM privacy_request WHERE status = 'open' ORDER BY received_on, id LIMIT 500;

-- name: ListClosedRequests :many
SELECT * FROM privacy_request WHERE status <> 'open' ORDER BY closed_at DESC, id DESC LIMIT 20;

-- Optimistic concurrency: no row returned = stale version.
-- name: DecideRequest :one
UPDATE privacy_request
SET status = @status, outcome = sqlc.narg(outcome),
    closed_by = CASE WHEN @status = 'open' THEN NULL ELSE app_current_user() END,
    closed_at = CASE WHEN @status = 'open' THEN NULL ELSE now() END,
    version = version + 1, updated_at = now()
WHERE id = @id AND version = @version
RETURNING *;

-- A merged duplicate's consent and requests follow it (the rule for two
-- consents is in domain.go).
-- name: MoveRequests :exec
UPDATE privacy_request SET candidate_id = @to_id, updated_at = now() WHERE candidate_id = @from_id;

-- name: TakeConsent :one
DELETE FROM candidate_consent WHERE candidate_id = @candidate_id RETURNING *;

-- name: PutConsent :exec
INSERT INTO candidate_consent (candidate_id, tenant_id, status, method, day, note, recorded_by)
VALUES (@candidate_id, app_current_tenant(), @status, @method, @day, sqlc.narg(note), @recorded_by)
ON CONFLICT (candidate_id) DO UPDATE
SET status = EXCLUDED.status, method = EXCLUDED.method, day = EXCLUDED.day, note = EXCLUDED.note,
    recorded_by = EXCLUDED.recorded_by, version = candidate_consent.version + 1, updated_at = now();
