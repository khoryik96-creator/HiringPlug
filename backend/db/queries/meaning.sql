-- meaning module: what the meaning model read of candidates and searches,
-- and the comparisons. Vectors come in as text ('[0.1,0.2,...]') and are
-- compared here; Go never reads them back.

-- name: GetSweep :one
SELECT tenant_id, chain, cursor_id, passes, due_at, ran_at, model, unavailable_until FROM meaning_sweep;

-- The company's read-through, made if it has none (inserted: it was made now).
-- name: EnsureSweep :one
INSERT INTO meaning_sweep (tenant_id) VALUES (app_current_tenant())
ON CONFLICT (tenant_id) DO UPDATE SET tenant_id = EXCLUDED.tenant_id
RETURNING tenant_id, chain, cursor_id, passes, due_at, ran_at, model, unavailable_until, (xmax = 0)::boolean AS inserted;

-- The read-through stalled (its job died) or must start again: a new chain.
-- name: RestartSweep :one
UPDATE meaning_sweep SET chain = chain + 1, due_at = now() RETURNING chain;

-- name: SweepRan :exec
UPDATE meaning_sweep SET cursor_id = @cursor_id, passes = passes + @finished::int, due_at = @due_at, ran_at = now()
WHERE chain = @chain;

-- name: SetSweepModel :exec
UPDATE meaning_sweep SET model = @model, unavailable_until = NULL WHERE model IS DISTINCT FROM @model OR unavailable_until IS NOT NULL;

-- name: SetUnavailable :exec
UPDATE meaning_sweep SET unavailable_until = @until;

-- What has been read of these candidates, and by which model.
-- name: ReadOf :many
SELECT candidate_id, fingerprint, model FROM meaning_candidate WHERE candidate_id = ANY(@candidate_ids::text[]);

-- name: PendingOf :many
SELECT candidate_id, fingerprint, queued_at FROM meaning_pending WHERE candidate_id = ANY(@candidate_ids::text[]);

-- name: CountPending :one
SELECT count(*) FROM meaning_pending WHERE queued_at > now() - interval '30 minutes';

-- Reads queued long ago and never back (the job died): forgotten, so they are queued again.
-- name: ForgetStalePending :exec
DELETE FROM meaning_pending WHERE queued_at <= now() - interval '30 minutes';

-- name: MarkPending :exec
INSERT INTO meaning_pending (tenant_id, candidate_id, fingerprint)
SELECT app_current_tenant(), unnest(@candidate_ids::text[]), unnest(@fingerprints::text[])
ON CONFLICT (tenant_id, candidate_id) DO UPDATE SET fingerprint = EXCLUDED.fingerprint, queued_at = now();

-- name: ClearPending :exec
DELETE FROM meaning_pending WHERE candidate_id = @candidate_id AND fingerprint = @fingerprint;

-- A candidate read again: what was read before goes.
-- name: DeleteRoles :exec
DELETE FROM meaning_role WHERE candidate_id = @candidate_id;

-- name: DeletePassages :exec
DELETE FROM meaning_passage WHERE candidate_id = @candidate_id;

-- name: SaveRead :exec
INSERT INTO meaning_candidate (tenant_id, candidate_id, fingerprint, model, profile, read_at)
VALUES (app_current_tenant(), @candidate_id, @fingerprint, @model, sqlc.narg(profile)::text::vector, now())
ON CONFLICT (tenant_id, candidate_id) DO UPDATE
SET fingerprint = EXCLUDED.fingerprint, model = EXCLUDED.model, profile = EXCLUDED.profile, read_at = now();

-- Levels and years of -1 are none.
-- name: InsertRoles :exec
INSERT INTO meaning_role (tenant_id, candidate_id, ordinal, title, family, level, end_year, field, work)
SELECT app_current_tenant(), sqlc.arg(candidate_id)::text, r.ordinal, r.title, nullif(r.family, ''), nullif(r.level, -1),
       nullif(r.end_year, -1), r.field::vector, r.work::vector
FROM (SELECT unnest(@ordinals::smallint[]) AS ordinal, unnest(@titles::text[]) AS title, unnest(@families::text[]) AS family,
             unnest(@levels::smallint[]) AS level, unnest(@end_years::smallint[]) AS end_year,
             unnest(@fields::text[]) AS field, unnest(@works::text[]) AS work) r;

-- name: InsertPassages :exec
INSERT INTO meaning_passage (tenant_id, candidate_id, ordinal, document_id, passage, vec)
SELECT app_current_tenant(), sqlc.arg(candidate_id)::text, p.ordinal, p.document_id, p.passage, p.vec::vector
FROM (SELECT unnest(@ordinals::smallint[]) AS ordinal, unnest(@document_ids::text[]) AS document_id,
             unnest(@passages::text[]) AS passage, unnest(@vecs::text[]) AS vec) p;

-- name: CountRead :one
SELECT count(*) FROM meaning_candidate WHERE model = @model;

-- A search or a job, as read.
-- name: QueryByHash :one
SELECT id, status, level, describes, exact, understood, created_at, read_at, (role IS NOT NULL)::boolean AS has_role,
       (context IS NOT NULL)::boolean AS has_context,
       (SELECT count(*) FROM meaning_query_need n WHERE n.query_id = q.id)::int AS needs
FROM meaning_query q WHERE text_hash = @text_hash;

-- name: InsertQuery :one
INSERT INTO meaning_query (tenant_id, text_hash, title, body, status)
VALUES (app_current_tenant(), @text_hash, sqlc.narg(title), @body, 'reading')
ON CONFLICT (tenant_id, text_hash) DO UPDATE SET used_at = now()
RETURNING id, status, created_at;

-- name: RereadQuery :exec
UPDATE meaning_query SET status = 'reading', created_at = now(), read_at = NULL WHERE id = @id;

-- name: TouchQuery :exec
UPDATE meaning_query SET used_at = now() WHERE id = @id AND used_at < now() - interval '1 hour';

-- name: QueryText :one
SELECT title, body FROM meaning_query WHERE id = @id;

-- name: SaveQuery :execrows
UPDATE meaning_query
SET status = 'ready', role = sqlc.narg(role)::text::vector, whole = sqlc.narg(whole)::text::vector,
    context = sqlc.narg(context)::text::vector, level = sqlc.narg(level), describes = @describes,
    exact = @exact, understood = @understood, read_at = now()
WHERE id = @id;

-- name: FailQuery :exec
UPDATE meaning_query SET status = 'failed', read_at = now() WHERE id = @id;

-- name: DeleteNeeds :exec
DELETE FROM meaning_query_need WHERE query_id = @query_id;

-- name: InsertNeeds :exec
INSERT INTO meaning_query_need (tenant_id, query_id, ordinal, need, vec)
SELECT app_current_tenant(), sqlc.arg(query_id)::text, n.ordinal, n.need, n.vec::vector
FROM (SELECT unnest(@ordinals::smallint[]) AS ordinal, unnest(@needs::text[]) AS need, unnest(@vecs::text[]) AS vec) n;

-- Searches not used for 90 days are forgotten.
-- name: ForgetOldQueries :exec
DELETE FROM meaning_query WHERE used_at < now() - interval '90 days';

-- Wider searches of the HNSW indexes (how many neighbours each looks at).
-- name: SearchWidely :exec
SELECT set_config('hnsw.ef_search', @ef::text, true);

-- The nearest titles to the role asked for (read as fields, then with the
-- work), the nearest CV passages and profiles to the whole search: who to
-- look at closely.
-- name: NearByField :many
SELECT candidate_id::text FROM meaning_role
ORDER BY field <=> (SELECT role FROM meaning_query WHERE id = @query_id) LIMIT @n;

-- name: NearByWork :many
SELECT candidate_id::text FROM meaning_role
ORDER BY work <=> (SELECT role FROM meaning_query WHERE id = @query_id) LIMIT @n;

-- name: NearByPassage :many
SELECT candidate_id::text FROM meaning_passage
ORDER BY vec <=> (SELECT coalesce(context, whole) FROM meaning_query WHERE id = @query_id) LIMIT @n;

-- name: NearByProfile :many
SELECT candidate_id::text FROM meaning_candidate WHERE profile IS NOT NULL
ORDER BY profile <=> (SELECT whole FROM meaning_query WHERE id = @query_id) LIMIT @n;

-- Then exactly, for those: each title, ...
-- name: RoleScores :many
SELECT r.candidate_id::text, r.ordinal, r.title, r.family, r.level, r.end_year,
       (1 - (r.field <=> q.role))::float4 AS field_sim, (1 - (r.work <=> q.role))::float4 AS work_sim
FROM meaning_role r, (SELECT role FROM meaning_query WHERE id = @query_id) q
WHERE r.candidate_id = ANY(@candidate_ids::text[]) AND q.role IS NOT NULL;

-- ... their CV's passage nearest the whole search and the one nearest what
-- else it says ...
-- name: PassageScores :many
SELECT x.candidate_id::text,
       max(x.whole_sim)::float4 AS whole_sim, max(x.context_sim)::float4 AS context_sim,
       (array_agg(x.passage ORDER BY x.best DESC))[1]::text AS passage
FROM (
  SELECT p.candidate_id, p.passage,
         1 - (p.vec <=> q.whole) AS whole_sim,
         coalesce(1 - (p.vec <=> q.context), 0) AS context_sim,
         coalesce(1 - (p.vec <=> q.context), 1 - (p.vec <=> q.whole)) AS best
  FROM meaning_passage p, (SELECT whole, context FROM meaning_query WHERE id = @query_id) q
  WHERE p.candidate_id = ANY(@candidate_ids::text[])
) x
GROUP BY x.candidate_id;

-- ... their whole profile ...
-- name: ProfileScores :many
SELECT m.candidate_id::text, coalesce(1 - (m.profile <=> q.whole), 0)::float4 AS sim
FROM meaning_candidate m, (SELECT whole FROM meaning_query WHERE id = @query_id) q
WHERE m.candidate_id = ANY(@candidate_ids::text[]);

-- ... and, for a job, how well their CV meets each need it lists.
-- name: NeedScores :many
SELECT p.candidate_id::text, n.ordinal, max(1 - (p.vec <=> n.vec))::float4 AS sim
FROM meaning_passage p JOIN meaning_query_need n ON n.query_id = @query_id
WHERE p.candidate_id = ANY(@candidate_ids::text[])
GROUP BY p.candidate_id, n.ordinal;
