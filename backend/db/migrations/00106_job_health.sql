-- For the server's alerts (the worker emails HP_ALERT_EMAIL): background jobs
-- that failed for good lately, and ones waiting far too long (a stopped
-- worker), across every company. Counts only: no company's data.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION job_health(p_since interval, p_waiting interval)
RETURNS TABLE (dead integer, stuck integer)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT count(*) FILTER (WHERE status = 'dead' AND finished_at > now() - p_since)::integer,
         count(*) FILTER (WHERE status = 'pending' AND run_after < now() - p_waiting)::integer
    FROM job
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION job_health(interval, interval) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION job_health(interval, interval) TO hiringplug_app;

-- +goose Down
DROP FUNCTION IF EXISTS job_health(interval, interval);
