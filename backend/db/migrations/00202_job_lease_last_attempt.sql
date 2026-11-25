-- job queue (1.47.1): a job whose worker dies on its last attempt ends.
-- A claim took back a running job whose lease had run out (its worker
-- died) whatever its attempts, so a job that kills its worker (a CV that
-- crashes the reader, a worker stopped by memory) was claimed, and killed
-- the worker, again and again; what it was for waited forever. Now each
-- claim first ends such jobs whose last attempt it was: dead, saying why,
-- and their dead-job handler runs (an AI-queue job's through the notice of
-- migration 00199; a core job's through the same notice, queued here), so
-- a CV being read shows that its reading failed. Jobs with tries left are
-- taken back as before.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- +goose StatementBegin
-- Ends the running jobs of a queue (and company, unless NULL) whose lease
-- ran out on their last attempt. Returns how many. Rows another claim is
-- ending at the same moment are skipped, never waited for.
CREATE FUNCTION job_end_expired(p_queue text, p_tenant_id text) RETURNS integer
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  r record;
  n integer := 0;
BEGIN
  FOR r IN
    UPDATE job SET status = 'dead', finished_at = now(), locked_by = NULL, locked_until = NULL, updated_at = now(),
                   last_error = left('Its worker stopped during its last attempt (the lease ran out), so it is not tried again.'
                                     || coalesce(' Before: ' || last_error, ''), 4000)
     WHERE id IN (SELECT id FROM job
                   WHERE queue = p_queue AND status = 'running' AND locked_until < now() AND attempts >= max_attempts
                     AND (p_tenant_id IS NULL OR tenant_id = p_tenant_id)
                   LIMIT 100
                     FOR UPDATE SKIP LOCKED)
    RETURNING id, tenant_id, queue, kind, request_id
  LOOP
    n := n + 1;
    -- An AI-queue job's notice is queued by the job_ai_dead_notice trigger.
    IF r.queue = 'core' AND r.kind <> 'background_job.ended' THEN
      PERFORM job_enqueue(r.tenant_id, 'core', 'background_job.ended',
        jsonb_build_object('jobId', r.id, 'kind', r.kind), r.request_id, now(), 5, 5::smallint);
    END IF;
  END LOOP;
  RETURN n;
END
$$;

CREATE OR REPLACE FUNCTION job_claim(p_queue text, p_worker text, p_lease_seconds integer)
RETURNS SETOF job
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT job_end_expired(p_queue, NULL);
  UPDATE job
     SET status = 'running',
         attempts = attempts + 1,
         locked_by = p_worker,
         locked_until = now() + make_interval(secs => p_lease_seconds),
         updated_at = now()
   WHERE id = (
           SELECT id FROM job
            WHERE queue = p_queue
              AND ((status = 'pending' AND run_after <= now())
                OR (status = 'running' AND locked_until < now() AND attempts < max_attempts))  -- a worker died mid-job
            ORDER BY priority, run_after, id
            LIMIT 1
              FOR UPDATE SKIP LOCKED
         )
  RETURNING *
$$;

CREATE OR REPLACE FUNCTION job_claim_for_tenant(p_queue text, p_worker text, p_lease_seconds integer, p_tenant_id text)
RETURNS SETOF job
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT job_end_expired(p_queue, p_tenant_id);
  UPDATE job
     SET status = 'running',
         attempts = attempts + 1,
         locked_by = p_worker,
         locked_until = now() + make_interval(secs => p_lease_seconds),
         updated_at = now()
   WHERE id = (
           SELECT id FROM job
            WHERE queue = p_queue AND tenant_id = p_tenant_id
              AND ((status = 'pending' AND run_after <= now())
                OR (status = 'running' AND locked_until < now() AND attempts < max_attempts))
            ORDER BY priority, run_after, id
            LIMIT 1
              FOR UPDATE SKIP LOCKED
         )
  RETURNING *
$$;

CREATE OR REPLACE FUNCTION job_claim_kinds(p_queue text, p_worker text, p_lease_seconds integer, p_kinds text[])
RETURNS SETOF job
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT job_end_expired(p_queue, NULL);
  UPDATE job
     SET status = 'running',
         attempts = attempts + 1,
         locked_by = p_worker,
         locked_until = now() + make_interval(secs => p_lease_seconds),
         updated_at = now()
   WHERE id = (
           SELECT id FROM job
            WHERE queue = p_queue AND kind = ANY (p_kinds)
              AND ((status = 'pending' AND run_after <= now())
                OR (status = 'running' AND locked_until < now() AND attempts < max_attempts))  -- a worker died mid-job
            ORDER BY priority, run_after, id
            LIMIT 1
              FOR UPDATE SKIP LOCKED
         )
  RETURNING *
$$;

CREATE OR REPLACE FUNCTION job_claim_except_kinds(p_queue text, p_worker text, p_lease_seconds integer, p_kinds text[])
RETURNS SETOF job
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT job_end_expired(p_queue, NULL);
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
                OR (status = 'running' AND locked_until < now() AND attempts < max_attempts))  -- a worker died mid-job
            ORDER BY priority, run_after, id
            LIMIT 1
              FOR UPDATE SKIP LOCKED
         )
  RETURNING *
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION job_end_expired(text, text) FROM PUBLIC;

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION job_claim(p_queue text, p_worker text, p_lease_seconds integer)
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
            WHERE queue = p_queue
              AND ((status = 'pending' AND run_after <= now())
                OR (status = 'running' AND locked_until < now()))  -- a worker died mid-job
            ORDER BY priority, run_after, id
            LIMIT 1
              FOR UPDATE SKIP LOCKED
         )
  RETURNING *
$$;

CREATE OR REPLACE FUNCTION job_claim_for_tenant(p_queue text, p_worker text, p_lease_seconds integer, p_tenant_id text)
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
            WHERE queue = p_queue AND tenant_id = p_tenant_id
              AND ((status = 'pending' AND run_after <= now())
                OR (status = 'running' AND locked_until < now()))
            ORDER BY priority, run_after, id
            LIMIT 1
              FOR UPDATE SKIP LOCKED
         )
  RETURNING *
$$;

CREATE OR REPLACE FUNCTION job_claim_kinds(p_queue text, p_worker text, p_lease_seconds integer, p_kinds text[])
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
            WHERE queue = p_queue AND kind = ANY (p_kinds)
              AND ((status = 'pending' AND run_after <= now())
                OR (status = 'running' AND locked_until < now()))  -- a worker died mid-job
            ORDER BY priority, run_after, id
            LIMIT 1
              FOR UPDATE SKIP LOCKED
         )
  RETURNING *
$$;

CREATE OR REPLACE FUNCTION job_claim_except_kinds(p_queue text, p_worker text, p_lease_seconds integer, p_kinds text[])
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
-- +goose StatementEnd
DROP FUNCTION IF EXISTS job_end_expired(text, text);
