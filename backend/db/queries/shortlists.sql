-- Queries owned by the shortlists module. Run inside a tenant-scoped transaction.

-- name: InsertShortlist :one
INSERT INTO client_shortlist (tenant_id, token_hash, job_id, message, expires_at, emailed_to, created_by)
VALUES (app_current_tenant(), @token_hash, @job_id, sqlc.narg(message), @expires_at, sqlc.narg(emailed_to), app_current_user())
RETURNING *;

-- name: InsertShortlistCandidate :one
INSERT INTO client_shortlist_candidate (tenant_id, shortlist_id, position, application_id, candidate_id, document_id, blind, label, note)
VALUES (app_current_tenant(), @shortlist_id, @position, @application_id, @candidate_id, @document_id, @blind, @label, sqlc.narg(note))
RETURNING *;

-- name: GetShortlist :one
SELECT * FROM client_shortlist WHERE id = @id;

-- name: LockShortlist :one
SELECT * FROM client_shortlist WHERE id = @id FOR UPDATE;

-- name: ShortlistByToken :one
SELECT * FROM client_shortlist WHERE token_hash = @token_hash;

-- name: LockShortlistByToken :one
SELECT * FROM client_shortlist WHERE token_hash = @token_hash FOR UPDATE;

-- name: JobShortlists :many
SELECT * FROM client_shortlist WHERE job_id = @job_id ORDER BY created_at DESC, id DESC LIMIT 50;

-- name: ShortlistCandidates :many
SELECT * FROM client_shortlist_candidate WHERE shortlist_id = ANY (@shortlist_ids::text[]) ORDER BY shortlist_id, position;

-- name: LockShortlistCandidate :one
SELECT * FROM client_shortlist_candidate WHERE id = @id AND shortlist_id = @shortlist_id FOR UPDATE;

-- name: AnswerShortlistCandidate :one
UPDATE client_shortlist_candidate SET decision = @decision, comment = sqlc.narg(comment), answered_at = now()
WHERE id = @id AND decision IS NULL
RETURNING *;

-- Marks a visit; the first one sets opened_at. Returns whether it was the first.
-- name: MarkShortlistOpened :one
UPDATE client_shortlist SET opened_at = coalesce(opened_at, now()), last_opened_at = now()
WHERE id = @id
RETURNING (opened_at = last_opened_at)::boolean AS first;

-- name: RevokeShortlist :one
UPDATE client_shortlist SET revoked_at = now(), revoked_by = app_current_user()
WHERE id = @id AND revoked_at IS NULL
RETURNING *;

-- The public page knows only the token: its company, outside any tenant scope.
-- name: ShortlistTenant :one
SELECT tenant_id::text AS tenant_id, tenant_name::text AS tenant_name FROM app_client_shortlist_tenant(@token_hash::text);

-- Which of some applications are on a client link (merging candidates keeps them).
-- name: ApplicationsOnLinks :many
SELECT DISTINCT application_id::text FROM client_shortlist_candidate WHERE application_id = ANY(@application_ids::text[]);

-- Merging candidates: the duplicate's places on links follow it.
-- name: MoveShortlistCandidates :exec
UPDATE client_shortlist_candidate SET candidate_id = @to_id WHERE candidate_id = @from_id;
