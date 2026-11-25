-- job queue (1.47.0): no external actions after a restore. A backup holds
-- the job table as it was: emails waiting to be sent, calendar updates,
-- webhooks, automations. Put back (on another server, or this one after
-- they had already happened), they would run again. So `hp restore` moves
-- the waiting and running jobs of the kinds that act outside HiringPlug to
-- a new status, held, which no worker claims (the claim functions take
-- pending and running jobs only), until someone releases or cancels them
-- in Settings > System. Interrupted local jobs are made ready again.
-- restore_marker: the last restore (one row for the server, not company
-- data): which backup, from which server to which, when, how many jobs it
-- held, and when the last of them was released or cancelled (the server's
-- alert email waits until then).
-- A held job cancelled is dead, and tells the Go worker as an AI-queue job
-- failing for good does (background_job.ended, migration 00199), so what
-- it was for (an email to send) shows it was not sent.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE job DROP CONSTRAINT job_status_check;
ALTER TABLE job ADD CONSTRAINT job_status_check CHECK (status IN ('pending', 'running', 'done', 'dead', 'held'));
ALTER TABLE job ADD COLUMN held_at timestamptz;
CREATE INDEX job_held_idx ON job (tenant_id, held_at) WHERE status = 'held';

CREATE TABLE restore_marker (
  id                 boolean PRIMARY KEY DEFAULT true CHECK (id),
  backup_name        text NOT NULL,
  source_server      text,          -- the backup's (its manifest); NULL: a backup too old to say
  destination_server text NOT NULL,
  restored_at        timestamptz NOT NULL DEFAULT now(),
  held               integer NOT NULL DEFAULT 0 CHECK (held >= 0),
  resumed            boolean NOT NULL DEFAULT false, -- restored with -resume-outbound: nothing held
  released_at        timestamptz                     -- the last held job released or cancelled
);
REVOKE INSERT, DELETE ON restore_marker FROM hiringplug_app;

-- +goose StatementBegin
-- After a restore (hp restore, as the database owner): holds the waiting and
-- running jobs of the given kinds; returns how many of each.
CREATE FUNCTION job_hold(p_kinds text[]) RETURNS TABLE (kind text, held integer)
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  WITH h AS (
    UPDATE job SET status = 'held', held_at = now(), locked_by = NULL, locked_until = NULL, updated_at = now()
     WHERE status IN ('pending', 'running') AND kind = ANY (p_kinds)
    RETURNING job.kind
  )
  SELECT h.kind, count(*)::integer FROM h GROUP BY h.kind ORDER BY h.kind
$$;

-- After a restore: jobs that were running when the backup was made run
-- again at once (their worker is gone), the interrupted attempt not counted.
CREATE FUNCTION job_reset_leases() RETURNS integer
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  WITH r AS (
    UPDATE job SET status = 'pending', attempts = greatest(attempts - 1, 0), locked_by = NULL, locked_until = NULL,
                   run_after = now(), updated_at = now()
     WHERE status = 'running'
    RETURNING 1
  )
  SELECT count(*)::integer FROM r
$$;

-- The restore is settled once no job is held any more.
CREATE FUNCTION job_restore_settle() RETURNS void
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  UPDATE restore_marker SET released_at = now()
   WHERE released_at IS NULL AND NOT EXISTS (SELECT 1 FROM job WHERE status = 'held')
$$;

-- One company's held jobs, oldest first, without payloads.
CREATE FUNCTION job_held(p_tenant_id text, p_limit integer)
RETURNS TABLE (id text, kind text, attempts integer, created_at timestamptz, held_at timestamptz, run_after timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT id, kind, attempts, created_at, held_at, run_after
    FROM job
   WHERE tenant_id = p_tenant_id AND status = 'held'
   ORDER BY held_at, id
   LIMIT p_limit
$$;

-- Releases one of a company's held jobs (p_job_id), or all of them (NULL):
-- they run as if just queued (one due later still waits for its time).
-- Returns how many.
CREATE FUNCTION job_release(p_tenant_id text, p_job_id text) RETURNS integer
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  n integer;
BEGIN
  UPDATE job SET status = 'pending', held_at = NULL, run_after = greatest(run_after, now()), updated_at = now()
   WHERE tenant_id = p_tenant_id AND status = 'held' AND (p_job_id IS NULL OR id = p_job_id);
  GET DIAGNOSTICS n = ROW_COUNT;
  IF n > 0 THEN -- wakes idle workers once this commits
    PERFORM pg_notify('job_core', coalesce(p_job_id, ''));
    PERFORM pg_notify('job_ai', coalesce(p_job_id, ''));
  END IF;
  PERFORM job_restore_settle();
  RETURN n;
END
$$;

-- Cancels one of a company's held jobs (p_job_id), or all of them (NULL):
-- dead, never run. Returns how many.
CREATE FUNCTION job_cancel_held(p_tenant_id text, p_job_id text) RETURNS integer
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  n integer;
BEGIN
  UPDATE job SET status = 'dead', finished_at = now(), held_at = NULL, updated_at = now(),
                 last_error = 'Cancelled after the server was restored: not run.'
   WHERE tenant_id = p_tenant_id AND status = 'held' AND (p_job_id IS NULL OR id = p_job_id);
  GET DIAGNOSTICS n = ROW_COUNT;
  PERFORM job_restore_settle();
  RETURN n;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION job_hold(text[]) FROM PUBLIC;
REVOKE ALL ON FUNCTION job_reset_leases() FROM PUBLIC;
REVOKE ALL ON FUNCTION job_restore_settle() FROM PUBLIC;
REVOKE ALL ON FUNCTION job_held(text, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION job_release(text, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION job_cancel_held(text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION job_held(text, integer) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION job_release(text, text) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION job_cancel_held(text, text) TO hiringplug_app;

-- A held job cancelled tells the Go worker too (see 00199).
CREATE OR REPLACE TRIGGER job_ai_dead_notice AFTER UPDATE OF status ON job
  FOR EACH ROW WHEN (NEW.status = 'dead' AND OLD.status IS DISTINCT FROM 'dead' AND (NEW.queue = 'ai' OR OLD.status = 'held'))
  EXECUTE FUNCTION job_ai_dead_notice();

-- +goose Down
CREATE OR REPLACE TRIGGER job_ai_dead_notice AFTER UPDATE OF status ON job
  FOR EACH ROW WHEN (NEW.queue = 'ai' AND NEW.status = 'dead' AND OLD.status IS DISTINCT FROM 'dead')
  EXECUTE FUNCTION job_ai_dead_notice();
DROP FUNCTION IF EXISTS job_cancel_held(text, text);
DROP FUNCTION IF EXISTS job_release(text, text);
DROP FUNCTION IF EXISTS job_held(text, integer);
DROP FUNCTION IF EXISTS job_restore_settle();
DROP FUNCTION IF EXISTS job_reset_leases();
DROP FUNCTION IF EXISTS job_hold(text[]);
DROP TABLE IF EXISTS restore_marker;
UPDATE job SET status = 'pending' WHERE status = 'held';
DROP INDEX IF EXISTS job_held_idx;
ALTER TABLE job DROP COLUMN IF EXISTS held_at;
ALTER TABLE job DROP CONSTRAINT job_status_check;
ALTER TABLE job ADD CONSTRAINT job_status_check CHECK (status IN ('pending', 'running', 'done', 'dead'));
