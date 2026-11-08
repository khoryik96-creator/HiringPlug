-- A worker keeps its claim on a long job: before each long step (a CV
-- Studio CV makes two AI calls of several minutes each) it extends its lease,
-- so another worker does not take the job and pay for the same calls again.
-- Returns false when the worker no longer holds the job (its lease ran out
-- and another worker took it): it must then stop.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE FUNCTION job_extend(p_job_id text, p_worker text, p_lease_seconds integer) RETURNS boolean
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  WITH kept AS (
    UPDATE job
       SET locked_until = now() + make_interval(secs => p_lease_seconds),
           updated_at = now()
     WHERE id = p_job_id AND locked_by = p_worker AND status = 'running'
    RETURNING 1
  )
  SELECT EXISTS (SELECT 1 FROM kept)
$$;
REVOKE ALL ON FUNCTION job_extend(text, text, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION job_extend(text, text, integer) TO hiringplug_app, hiringplug_ai;

-- +goose Down
DROP FUNCTION IF EXISTS job_extend(text, text, integer);
