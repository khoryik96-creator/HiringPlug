-- job queue: job_give_up ends a job at once (dead, with its reason) when
--   trying again cannot help, e.g. a file larger than the worker may read,
--   instead of retrying it (and downloading it again) until its last attempt.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION job_give_up(p_job_id text, p_worker text, p_error text)
RETURNS text
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  UPDATE job
     SET status = 'dead', finished_at = now(), last_error = left(p_error, 4000),
         locked_by = NULL, locked_until = NULL, updated_at = now()
   WHERE id = p_job_id AND locked_by = p_worker AND status = 'running'
  RETURNING status
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION job_give_up(text, text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION job_give_up(text, text, text) TO hiringplug_app, hiringplug_ai;

-- +goose Down
DROP FUNCTION IF EXISTS job_give_up(text, text, text);
