-- Search by meaning, after its review (1.40.3):
-- meaning_role.current: whether a title is their job now. A past job with no
--   end year was taken for their job now; null for titles read before this
--   (then their job now when they have no end year, as before), and every
--   candidate is read again by the new reading rules.
-- meaning_sweep.read_count, total_count, counted_at: how far the company's
--   read-through has got, counted by the read-through itself (at the start of
--   each pass, and every few minutes during one), so a search no longer
--   counts the whole company on every request.
-- What was read of a candidate in the Bin is not used: their meaning rows are
--   visible only while the candidate is (in the user's scope and not in the
--   Bin). A CV moved to the Bin has its passages removed by the meaning module.
-- job_claim_except_kinds: the AI worker's other slots take every job but
--   search by meaning's, which only its meaning slot reads (the background
--   read-through used to take every slot); an index finds a slot's kinds
--   without walking every job queued before them.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE meaning_role ADD COLUMN current boolean;
-- How often a search was queued to be read again (its job died, or the AI
-- worker is down): each time waits twice as long, so they do not pile up.
ALTER TABLE meaning_query ADD COLUMN rereads smallint NOT NULL DEFAULT 0;
ALTER TABLE meaning_sweep ADD COLUMN read_count integer, ADD COLUMN total_count integer, ADD COLUMN counted_at timestamptz;

-- Whether the user may see this candidate now: in their scope and not in the
-- Bin (the candidate's own row-level security decides).
CREATE FUNCTION app_sees_candidate_now(p_id text) RETURNS boolean
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM candidate WHERE id = p_id)
$$;

DROP POLICY candidate_scope ON meaning_candidate;
CREATE POLICY candidate_scope ON meaning_candidate AS RESTRICTIVE FOR SELECT USING (app_sees_candidate_now(candidate_id));
DROP POLICY candidate_scope ON meaning_role;
CREATE POLICY candidate_scope ON meaning_role AS RESTRICTIVE FOR SELECT USING (app_sees_candidate_now(candidate_id));
DROP POLICY candidate_scope ON meaning_passage;
CREATE POLICY candidate_scope ON meaning_passage AS RESTRICTIVE FOR SELECT USING (app_sees_candidate_now(candidate_id));

-- Like job_claim, but never the given kinds (the AI worker's other slots).
CREATE FUNCTION job_claim_except_kinds(p_queue text, p_worker text, p_lease_seconds integer, p_kinds text[])
RETURNS SETOF job
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  UPDATE job
     SET status = 'running',
         attempts = attempts + 1,
         locked_by = p_worker,
         locked_until = now() + make_interval(secs => p_lease_seconds),
         updated_at = now()
   WHERE id = (
           SELECT id FROM job
            WHERE queue = p_queue AND kind <> ALL (p_kinds)
              AND ((status = 'pending' AND run_after <= now())
                OR (status = 'running' AND locked_until < now()))  -- a worker died mid-job
            ORDER BY priority, run_after, id
            LIMIT 1
              FOR UPDATE SKIP LOCKED
         )
  RETURNING *
$$;
REVOKE ALL ON FUNCTION job_claim_except_kinds(text, text, integer, text[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION job_claim_except_kinds(text, text, integer, text[]) TO hiringplug_app, hiringplug_ai;

-- The meaning slot's claim (job_claim_kinds) by kind, not through every job queued before.
CREATE INDEX job_kind_ready_idx ON job (queue, kind, priority, run_after) WHERE status IN ('pending', 'running');

-- +goose Down
DROP INDEX IF EXISTS job_kind_ready_idx;
DROP FUNCTION IF EXISTS job_claim_except_kinds(text, text, integer, text[]);
DROP POLICY candidate_scope ON meaning_passage;
CREATE POLICY candidate_scope ON meaning_passage AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
DROP POLICY candidate_scope ON meaning_role;
CREATE POLICY candidate_scope ON meaning_role AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
DROP POLICY candidate_scope ON meaning_candidate;
CREATE POLICY candidate_scope ON meaning_candidate AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
DROP FUNCTION IF EXISTS app_sees_candidate_now(text);
ALTER TABLE meaning_sweep DROP COLUMN IF EXISTS counted_at, DROP COLUMN IF EXISTS total_count, DROP COLUMN IF EXISTS read_count;
ALTER TABLE meaning_query DROP COLUMN IF EXISTS rereads;
ALTER TABLE meaning_role DROP COLUMN IF EXISTS current;
