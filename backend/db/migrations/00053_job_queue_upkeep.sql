-- Job queue upkeep:
--  * priorities: a job someone waits for on screen (an assistant question, an
--    email) goes before bulk work (PDF copies, webhooks). Declared per job kind
--    in the contract (x-jobs priority); a result job inherits the priority of
--    the job that made it.
--  * a finished job's payload is emptied: payloads hold CV text, details read
--    from CVs and whole PDFs, which must not stay behind in the queue (for
--    data protection requests, and the table's size). Dead jobs keep theirs,
--    so they can be tried again, until they are pruned.
--  * job_prune deletes finished and dead jobs after a while (the Go worker
--    calls it every hour).
--  * job_summary and job_retry: each company's view of its background jobs
--    (Settings > System).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE job ADD COLUMN priority smallint NOT NULL DEFAULT 5 CHECK (priority BETWEEN 1 AND 9);

DROP INDEX job_ready_idx;
CREATE INDEX job_ready_idx ON job (queue, priority, run_after) WHERE status IN ('pending', 'running');
CREATE INDEX job_finished_idx ON job (finished_at) WHERE status IN ('done', 'dead');

-- What finished jobs held until now.
UPDATE job SET payload = '{}'::jsonb WHERE status = 'done' AND payload <> '{}'::jsonb;

DROP FUNCTION job_enqueue(text, text, text, jsonb, text, timestamptz, integer);

-- +goose StatementBegin
CREATE FUNCTION job_enqueue(
  p_tenant_id text, p_queue text, p_kind text, p_payload jsonb,
  p_request_id text DEFAULT NULL, p_run_after timestamptz DEFAULT now(), p_max_attempts integer DEFAULT 5,
  p_priority smallint DEFAULT 5
) RETURNS text
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  new_job_id text;
BEGIN
  INSERT INTO job (tenant_id, queue, kind, payload, request_id, run_after, max_attempts, priority)
  VALUES (p_tenant_id, p_queue, p_kind, p_payload, p_request_id, p_run_after, p_max_attempts, p_priority)
  RETURNING id INTO new_job_id;
  -- Wakes up idle workers as soon as the enqueueing transaction commits.
  PERFORM pg_notify('job_' || p_queue, new_job_id);
  RETURN new_job_id;
END
$$;

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

CREATE OR REPLACE FUNCTION job_complete(p_job_id text, p_worker text) RETURNS boolean
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  WITH done AS (
    UPDATE job
       SET status = 'done', finished_at = now(), updated_at = now(),
           locked_by = NULL, locked_until = NULL, last_error = NULL,
           payload = '{}'::jsonb  -- its work is saved; the personal data it carried goes
     WHERE id = p_job_id AND locked_by = p_worker AND status = 'running'
    RETURNING 1
  )
  SELECT EXISTS (SELECT 1 FROM done)
$$;

-- Deletes jobs finished or dead for longer than p_keep, at most p_limit at a
-- time. Returns how many were deleted.
CREATE FUNCTION job_prune(p_keep interval, p_limit integer DEFAULT 5000) RETURNS integer
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  WITH gone AS (
    DELETE FROM job
     WHERE id IN (SELECT id FROM job
                   WHERE status IN ('done', 'dead') AND finished_at < now() - p_keep
                   LIMIT p_limit)
    RETURNING 1
  )
  SELECT count(*)::integer FROM gone
$$;

-- One company's background jobs by kind: waiting, running, failed and retrying,
-- dead (in the last p_since), and how long the oldest waiting one has waited.
CREATE FUNCTION job_summary(p_tenant_id text, p_since interval)
RETURNS TABLE (kind text, waiting integer, running integer, retrying integer, dead integer, oldest_waiting timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT kind,
         count(*) FILTER (WHERE status = 'pending' AND attempts = 0)::integer,
         count(*) FILTER (WHERE status = 'running')::integer,
         count(*) FILTER (WHERE status = 'pending' AND attempts > 0)::integer,
         count(*) FILTER (WHERE status = 'dead' AND finished_at > now() - p_since)::integer,
         min(created_at) FILTER (WHERE status = 'pending' AND run_after <= now())
    FROM job
   WHERE tenant_id = p_tenant_id
     AND (status IN ('pending', 'running') OR (status = 'dead' AND finished_at > now() - p_since))
   GROUP BY kind
   ORDER BY kind
$$;

-- One company's jobs that failed for good (newest first), without payloads.
CREATE FUNCTION job_dead(p_tenant_id text, p_since interval, p_limit integer)
RETURNS TABLE (id text, kind text, attempts integer, last_error text, finished_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT id, kind, attempts, last_error, finished_at
    FROM job
   WHERE tenant_id = p_tenant_id AND status = 'dead' AND finished_at > now() - p_since
   ORDER BY finished_at DESC, id
   LIMIT p_limit
$$;

-- Tries a dead job again, from its first attempt. Returns false if the
-- company has no such dead job.
CREATE FUNCTION job_retry(p_tenant_id text, p_job_id text) RETURNS boolean
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  q text;
BEGIN
  UPDATE job
     SET status = 'pending', attempts = 0, run_after = now(), finished_at = NULL,
         last_error = NULL, updated_at = now()
   WHERE id = p_job_id AND tenant_id = p_tenant_id AND status = 'dead'
  RETURNING queue INTO q;
  IF q IS NULL THEN
    RETURN false;
  END IF;
  PERFORM pg_notify('job_' || q, p_job_id);
  RETURN true;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION job_enqueue(text, text, text, jsonb, text, timestamptz, integer, smallint) FROM PUBLIC;
REVOKE ALL ON FUNCTION job_prune(interval, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION job_summary(text, interval) FROM PUBLIC;
REVOKE ALL ON FUNCTION job_dead(text, interval, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION job_retry(text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION job_enqueue(text, text, text, jsonb, text, timestamptz, integer, smallint) TO hiringplug_app, hiringplug_ai;
GRANT EXECUTE ON FUNCTION job_prune(interval, integer) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION job_summary(text, interval) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION job_dead(text, interval, integer) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION job_retry(text, text) TO hiringplug_app;

-- +goose Down
DROP FUNCTION IF EXISTS job_retry(text, text);
DROP FUNCTION IF EXISTS job_dead(text, interval, integer);
DROP FUNCTION IF EXISTS job_summary(text, interval);
DROP FUNCTION IF EXISTS job_prune(interval, integer);
DROP FUNCTION IF EXISTS job_enqueue(text, text, text, jsonb, text, timestamptz, integer, smallint);
-- +goose StatementBegin
CREATE FUNCTION job_enqueue(
  p_tenant_id text, p_queue text, p_kind text, p_payload jsonb,
  p_request_id text DEFAULT NULL, p_run_after timestamptz DEFAULT now(), p_max_attempts integer DEFAULT 5
) RETURNS text
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  new_job_id text;
BEGIN
  INSERT INTO job (tenant_id, queue, kind, payload, request_id, run_after, max_attempts)
  VALUES (p_tenant_id, p_queue, p_kind, p_payload, p_request_id, p_run_after, p_max_attempts)
  RETURNING id INTO new_job_id;
  PERFORM pg_notify('job_' || p_queue, new_job_id);
  RETURN new_job_id;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION job_enqueue(text, text, text, jsonb, text, timestamptz, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION job_enqueue(text, text, text, jsonb, text, timestamptz, integer) TO hiringplug_app, hiringplug_ai;
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
            ORDER BY run_after, id
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
            ORDER BY run_after, id
            LIMIT 1
              FOR UPDATE SKIP LOCKED
         )
  RETURNING *
$$;

CREATE OR REPLACE FUNCTION job_complete(p_job_id text, p_worker text) RETURNS boolean
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  WITH done AS (
    UPDATE job
       SET status = 'done', finished_at = now(), updated_at = now(),
           locked_by = NULL, locked_until = NULL, last_error = NULL
     WHERE id = p_job_id AND locked_by = p_worker AND status = 'running'
    RETURNING 1
  )
  SELECT EXISTS (SELECT 1 FROM done)
$$;
-- +goose StatementEnd
DROP INDEX IF EXISTS job_finished_idx;
DROP INDEX IF EXISTS job_ready_idx;
CREATE INDEX job_ready_idx ON job (queue, run_after) WHERE status IN ('pending', 'running');
ALTER TABLE job DROP COLUMN priority;
