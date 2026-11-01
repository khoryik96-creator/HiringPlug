-- The job queue shared by the Go worker and the Python AI worker.
--
-- Both languages use it only through the functions below, so the queue rules
-- (claiming, leases, retries, dead jobs) are written once, here.
--   job_enqueue  : add a job (call it inside the same transaction as your data change)
--   job_claim    : take the next ready job from a queue for a while (a lease)
--   job_complete : mark a claimed job done (call it in the same transaction as the job's effects)
--   job_fail     : record a failure; the job is retried later (with growing waits) or, after
--                  max_attempts, marked dead
--
-- Job kinds, their queue and their payload schema are declared in the API
-- contract (x-jobs in contracts/openapi.yaml).

-- +goose Up
CREATE TABLE job (
  id            text PRIMARY KEY DEFAULT new_id('job') CHECK (id ~ '^job_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id     text NOT NULL REFERENCES tenant (id),
  queue         text NOT NULL CHECK (queue IN ('core', 'ai')),
  kind          text NOT NULL CHECK (kind ~ '^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$'),
  payload       jsonb NOT NULL,
  status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'dead')),
  attempts      integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  max_attempts  integer NOT NULL DEFAULT 5 CHECK (max_attempts BETWEEN 1 AND 50),
  run_after     timestamptz NOT NULL DEFAULT now(),
  locked_by     text,
  locked_until  timestamptz,
  last_error    text,
  request_id    text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  finished_at   timestamptz
);

-- Finds ready jobs fast; done/dead jobs drop out of the index.
CREATE INDEX job_ready_idx ON job (queue, run_after) WHERE status IN ('pending', 'running');
CREATE INDEX job_tenant_idx ON job (tenant_id, created_at DESC);

-- No RLS on this table on purpose: workers serve every tenant. Workers handle
-- each job inside a transaction scoped to job.tenant_id, so the job's effects
-- are tenant-isolated as usual. Only the functions below should touch it.

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
  -- Wakes up idle workers as soon as the enqueueing transaction commits.
  PERFORM pg_notify('job_' || p_queue, new_job_id);
  RETURN new_job_id;
END
$$;

CREATE FUNCTION job_claim(p_queue text, p_worker text, p_lease_seconds integer)
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

-- Returns false if this worker no longer holds the job (its lease expired and
-- another worker took it); the caller must then roll back the job's effects.
CREATE FUNCTION job_complete(p_job_id text, p_worker text) RETURNS boolean
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

-- Returns the job's new status: 'pending' (retried later) or 'dead'.
-- The wait before the next attempt doubles each time: 10s, 20s, 40s ... capped
-- at 1 hour. It is computed here so every worker language backs off the same way.
CREATE FUNCTION job_fail(p_job_id text, p_worker text, p_error text)
RETURNS text
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  UPDATE job
     SET status = CASE WHEN attempts >= max_attempts THEN 'dead' ELSE 'pending' END,
         run_after = now() + make_interval(secs => least(10 * power(2, greatest(attempts, 1) - 1), 3600)),
         finished_at = CASE WHEN attempts >= max_attempts THEN now() END,
         last_error = left(p_error, 4000),
         locked_by = NULL, locked_until = NULL, updated_at = now()
   WHERE id = p_job_id AND locked_by = p_worker AND status = 'running'
  RETURNING status
$$;
-- +goose StatementEnd

-- Functions are executable by everyone by default; restrict them.
REVOKE ALL ON FUNCTION job_enqueue(text, text, text, jsonb, text, timestamptz, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION job_claim(text, text, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION job_complete(text, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION job_fail(text, text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION job_enqueue(text, text, text, jsonb, text, timestamptz, integer) TO hiringplug_app, hiringplug_ai;
GRANT EXECUTE ON FUNCTION job_claim(text, text, integer) TO hiringplug_app, hiringplug_ai;
GRANT EXECUTE ON FUNCTION job_complete(text, text) TO hiringplug_app, hiringplug_ai;
GRANT EXECUTE ON FUNCTION job_fail(text, text, text) TO hiringplug_app, hiringplug_ai;

-- +goose Down
DROP FUNCTION IF EXISTS job_fail(text, text, text);
DROP FUNCTION IF EXISTS job_complete(text, text);
DROP FUNCTION IF EXISTS job_claim(text, text, integer);
DROP FUNCTION IF EXISTS job_enqueue(text, text, text, jsonb, text, timestamptz, integer);
DROP TABLE IF EXISTS job;
