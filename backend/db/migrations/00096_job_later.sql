-- job_later: a running job that cannot start its work yet (a limit is
-- reached, e.g. the email verifier's two checks at a time) goes back to
-- pending, to run at p_run_after, without counting the attempt. The same job
-- waits: no new job is queued for each try.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION job_later(p_job_id text, p_worker text, p_run_after timestamptz) RETURNS boolean
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  WITH later AS (
    UPDATE job
       SET status = 'pending', run_after = p_run_after, attempts = greatest(attempts - 1, 0),
           locked_by = NULL, locked_until = NULL, updated_at = now()
     WHERE id = p_job_id AND locked_by = p_worker AND status = 'running'
    RETURNING 1
  )
  SELECT EXISTS (SELECT 1 FROM later)
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION job_later(text, text, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION job_later(text, text, timestamptz) TO hiringplug_app, hiringplug_ai;

-- +goose Down
DROP FUNCTION IF EXISTS job_later(text, text, timestamptz);
