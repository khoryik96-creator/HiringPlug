-- name: LockApolloAccount :exec
SELECT pg_advisory_xact_lock(hashtextextended('hr-apollo:' || app_current_tenant() || ':' || app_current_user(), 0));

-- name: GetApolloConnection :one
SELECT * FROM hr_apollo_connection WHERE user_id = $1;

-- name: SetApolloFlow :exec
INSERT INTO hr_apollo_connection (flow_id, broker_origin, proof_sealed, authorize_url, expires_at)
VALUES ($1, $2, $3, $4, now() + interval '10 minutes')
ON CONFLICT (tenant_id, user_id) DO UPDATE
SET flow_id = excluded.flow_id, broker_origin = excluded.broker_origin, proof_sealed = excluded.proof_sealed,
  authorize_url = excluded.authorize_url, expires_at = excluded.expires_at, account_name = NULL;

-- name: FinishApolloFlow :one
UPDATE hr_apollo_connection SET account_name = $2, expires_at = now() + interval '30 days', authorize_url = ''
WHERE user_id = app_current_user() AND flow_id = $1 AND account_name IS NULL AND expires_at > now() RETURNING *;

-- name: DeleteApolloConnection :exec
DELETE FROM hr_apollo_connection WHERE user_id = app_current_user() AND flow_id = $1;

-- name: FindApolloLookup :one
SELECT * FROM hr_apollo_lookup
WHERE user_id = app_current_user() AND linkedin_company_id = $1 AND profile_url = $2;

-- name: CreateApolloLookup :one
INSERT INTO hr_apollo_lookup
    (id, flow_id, company_id, linkedin_company_id, profile_url, person_name, linkedin_company_name, linkedin_company_url, state)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'queued') RETURNING *;

-- name: ClaimApolloLookup :one
UPDATE hr_apollo_lookup SET state = 'sending', updated_at = now()
WHERE id = $1 AND state = 'queued' RETURNING *;

-- name: FinishApolloLookup :one
UPDATE hr_apollo_lookup SET state = $2, email_sealed = $3, email_status = $4, reason = $5, updated_at = now()
WHERE id = $1 AND state IN ('sending','pending','unconfirmed')
RETURNING *;

-- name: HRWorkEmail :many
SELECT email FROM contact
WHERE company_id = $1 AND lower(rtrim(linkedin_url, '/')) = lower(sqlc.arg(profile_url)::text)
  AND email IS NOT NULL
ORDER BY id LIMIT 2;

-- name: ExpireApolloFlow :exec
UPDATE hr_apollo_connection SET expires_at = now()
WHERE user_id = app_current_user() AND flow_id = $1 AND account_name IS NULL;

-- name: LockApolloConnectionForLookup :one
SELECT * FROM hr_apollo_connection WHERE user_id = $1 FOR SHARE;

-- name: GetApolloLookup :one
SELECT * FROM hr_apollo_lookup WHERE id = $1;

-- name: ApolloUserActive :one
SELECT EXISTS (SELECT 1 FROM app_user WHERE id = $1 AND active)::boolean;

-- name: ApolloLookupInProgress :one
SELECT EXISTS(
  SELECT 1 FROM hr_apollo_lookup
  WHERE user_id = app_current_user() AND flow_id = $1 AND (
     (state = 'queued' AND updated_at > now() - interval '15 minutes')
     OR (state = 'sending' AND updated_at > now() - interval '1 minute')
  )
)::boolean;

-- name: DeleteCancelledApolloLookup :exec
DELETE FROM hr_apollo_lookup WHERE id = $1 AND state = 'cancelled';
