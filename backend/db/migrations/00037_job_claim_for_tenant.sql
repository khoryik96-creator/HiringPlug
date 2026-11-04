-- Claiming the next job of one company only. Workers serve every company and
-- use job_claim; a worker limited to one company (Worker.Tenant) uses this.
-- Tests do: each test has its own companies and its own settings, and a
-- test's worker must not run another test's jobs.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE FUNCTION job_claim_for_tenant(p_queue text, p_worker text, p_lease_seconds integer, p_tenant_id text)
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
REVOKE ALL ON FUNCTION job_claim_for_tenant(text, text, integer, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION job_claim_for_tenant(text, text, integer, text) TO hiringplug_app;

-- +goose Down
DROP FUNCTION IF EXISTS job_claim_for_tenant(text, text, integer, text);
