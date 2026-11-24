-- meaning module: what the meaning model read of candidates and searches,
-- and the comparisons. Vectors come in as text ('[0.1,0.2,...]') and are
-- compared here; Go never reads them back.

-- The company's read-through (a plain read: searches never wait on it).
-- name: GetSweep :one
SELECT * FROM meaning_sweep;

-- The company's read-through, made now (no row: another request made it meanwhile).
-- name: StartSweep :one
INSERT INTO meaning_sweep (tenant_id) VALUES (app_current_tenant()) ON CONFLICT (tenant_id) DO NOTHING
RETURNING *;

-- The read-through stalled (its job died) or must start again: a new chain,
-- from where it got to or, for a new model, from the beginning.
-- name: RestartSweep :one
UPDATE meaning_sweep SET chain = chain + 1, due_at = now(), cursor_id = CASE WHEN @from_start::boolean THEN '' ELSE cursor_id END
RETURNING chain;

-- How far the read-through has got, counted at the start of each pass (and every few minutes).
-- name: SetSweepCounts :exec
UPDATE meaning_sweep SET read_count = @read_count::int, total_count = @total_count::int, counted_at = now();

-- Candidates read now by the model the read-through uses: that many more read.
-- name: AddRead :exec
UPDATE meaning_sweep SET read_count = coalesce(read_count, 0) + @n::int WHERE model = @model;

-- name: SweepRan :exec
UPDATE meaning_sweep SET cursor_id = @cursor_id, passes = passes + @finished::int, due_at = @due_at, ran_at = now()
WHERE chain = @chain;

-- The model (and reading rules) that read the latest; changed: everyone is read again.
-- name: SetSweepModel :one
WITH before AS (SELECT model FROM meaning_sweep)
UPDATE meaning_sweep SET model = @model, unavailable_until = NULL,
  read_count = CASE WHEN meaning_sweep.model IS DISTINCT FROM @model THEN 0 ELSE read_count END
WHERE meaning_sweep.model IS DISTINCT FROM @model OR unavailable_until IS NOT NULL
RETURNING ((SELECT before.model FROM before) IS DISTINCT FROM @model)::boolean AS changed;

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

-- A CV moved to the Bin: what was read of it goes at once.
-- name: DeleteDocumentPassages :exec
DELETE FROM meaning_passage WHERE document_id = @document_id;

-- newly: not read by this model before (counted as read).
-- name: SaveRead :one
WITH before AS (SELECT model FROM meaning_candidate WHERE candidate_id = @candidate_id)
INSERT INTO meaning_candidate (tenant_id, candidate_id, fingerprint, model, profile, read_at)
VALUES (app_current_tenant(), @candidate_id, @fingerprint, @model, sqlc.narg(profile)::text::vector, now())
ON CONFLICT (tenant_id, candidate_id) DO UPDATE
SET fingerprint = EXCLUDED.fingerprint, model = EXCLUDED.model, profile = EXCLUDED.profile, read_at = now()
RETURNING ((SELECT before.model FROM before) IS DISTINCT FROM @model)::boolean AS newly;

-- Levels and years of -1 are none.
-- name: InsertRoles :exec
INSERT INTO meaning_role (tenant_id, candidate_id, ordinal, title, family, level, end_year, current, field, work)
SELECT app_current_tenant(), sqlc.arg(candidate_id)::text, r.ordinal, r.title, nullif(r.family, ''), nullif(r.level, -1),
       nullif(r.end_year, -1), r.current, r.field::vector, r.work::vector
FROM (SELECT unnest(@ordinals::smallint[]) AS ordinal, unnest(@titles::text[]) AS title, unnest(@families::text[]) AS family,
             unnest(@levels::smallint[]) AS level, unnest(@end_years::smallint[]) AS end_year,
             unnest(@currents::boolean[]) AS current, unnest(@fields::text[]) AS field, unnest(@works::text[]) AS work) r;

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

-- Searches not used for 90 days are forgotten (the read-through, at the end of each pass).
-- name: ForgetOldQueries :exec
DELETE FROM meaning_query WHERE used_at < now() - interval '90 days';

-- Wider searches of the HNSW indexes (how many neighbours each looks at).
-- name: SearchWidely :exec
SELECT set_config('hnsw.ef_search', @ef::text, true);

-- An index search goes on past the rows the user may not see (or that are
-- left out) until it has found enough (pgvector 0.8 and later).
-- name: SearchIteratively :exec
SELECT set_config('hnsw.iterative_scan', 'relaxed_order', true), set_config('hnsw.max_scan_tuples', @max_tuples::text, true);

-- The nearest titles to the role asked for (read as fields, then with the
-- work), the nearest CV passages and profiles to the whole search: who to
-- look at closely; n candidates each (one may have several titles or
-- passages near), leaving out those already on the job.
-- name: NearByField :many
SELECT x.candidate_id::text FROM (
  SELECT candidate_id, field <=> (SELECT q.role FROM meaning_query q WHERE q.id = @query_id) AS d FROM meaning_role
  WHERE NOT (candidate_id = ANY(@exclude::text[]))
  ORDER BY field <=> (SELECT q.role FROM meaning_query q WHERE q.id = @query_id) LIMIT @rows::int
) x GROUP BY x.candidate_id ORDER BY min(x.d) LIMIT @n::int;

-- name: NearByWork :many
SELECT x.candidate_id::text FROM (
  SELECT candidate_id, work <=> (SELECT q.role FROM meaning_query q WHERE q.id = @query_id) AS d FROM meaning_role
  WHERE NOT (candidate_id = ANY(@exclude::text[]))
  ORDER BY work <=> (SELECT q.role FROM meaning_query q WHERE q.id = @query_id) LIMIT @rows::int
) x GROUP BY x.candidate_id ORDER BY min(x.d) LIMIT @n::int;

-- name: NearByPassage :many
SELECT x.candidate_id::text FROM (
  SELECT candidate_id, vec <=> (SELECT coalesce(q.context, q.whole) FROM meaning_query q WHERE q.id = @query_id) AS d FROM meaning_passage
  WHERE NOT (candidate_id = ANY(@exclude::text[]))
  ORDER BY vec <=> (SELECT coalesce(q.context, q.whole) FROM meaning_query q WHERE q.id = @query_id) LIMIT @rows::int
) x GROUP BY x.candidate_id ORDER BY min(x.d) LIMIT @n::int;

-- name: NearByProfile :many
SELECT candidate_id::text FROM meaning_candidate WHERE profile IS NOT NULL AND NOT (candidate_id = ANY(@exclude::text[]))
ORDER BY profile <=> (SELECT whole FROM meaning_query WHERE id = @query_id) LIMIT @n::int;

-- Then exactly, for those: each title, ...
-- name: RoleScores :many
SELECT r.candidate_id::text, r.ordinal, r.title, r.family, r.level, r.end_year, r.current,
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
SELECT m.candidate_id::text, m.model, coalesce(1 - (m.profile <=> q.whole), 0)::float4 AS sim
FROM meaning_candidate m, (SELECT whole FROM meaning_query WHERE id = @query_id) q
WHERE m.candidate_id = ANY(@candidate_ids::text[]);

-- ... and, for a job, how well their CV meets each need it lists.
-- name: NeedScores :many
SELECT p.candidate_id::text, n.ordinal, max(1 - (p.vec <=> n.vec))::float4 AS sim
FROM meaning_passage p JOIN meaning_query_need n ON n.query_id = @query_id
WHERE p.candidate_id = ANY(@candidate_ids::text[])
GROUP BY p.candidate_id, n.ordinal;
