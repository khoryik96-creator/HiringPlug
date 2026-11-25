-- job queue (1.47.0): an AI-queue job that fails for good reaches the Go
-- worker. The Python worker only records the failure (job_fail, job_give_up);
-- what the job was for (a CV being read) waited forever: a document stayed
-- "Reading details..." after its extraction died. Now the database itself,
-- in the transaction that ends the job, queues a core notice (background_job.ended)
-- whatever ended it (job_fail's last attempt, job_give_up, an operator), and
-- the Go worker runs that kind's dead-job handler (jobqueue OnDead).
-- document_reading_stuck lists documents still shown as being read with no
-- job left for them (from before this, a crash, a restore): the worker's
-- upkeep sets them right. The two indexes keep that, and the check for a
-- reading already under way, cheap.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION job_ai_dead_notice() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM job_enqueue(NEW.tenant_id, 'core', 'background_job.ended',
    jsonb_build_object('jobId', NEW.id, 'kind', NEW.kind), NEW.request_id, now(), 5, 5::smallint);
  RETURN NULL;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION job_ai_dead_notice() FROM PUBLIC;

CREATE TRIGGER job_ai_dead_notice AFTER UPDATE OF status ON job
  FOR EACH ROW WHEN (NEW.queue = 'ai' AND NEW.status = 'dead' AND OLD.status IS DISTINCT FROM 'dead')
  EXECUTE FUNCTION job_ai_dead_notice();

CREATE INDEX job_document_active_idx ON job ((payload->>'documentId'))
  WHERE status IN ('pending', 'running') AND kind LIKE 'document.%';
CREATE INDEX document_reading_idx ON document (updated_at)
  WHERE status = 'queued' OR parse_status = 'queued' OR pdf_status = 'queued';

-- +goose StatementBegin
CREATE FUNCTION document_reading_stuck(p_older interval, p_limit integer)
RETURNS TABLE (tenant_id text, document_id text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT d.tenant_id, d.id
    FROM document d
   WHERE (d.status = 'queued' OR d.parse_status = 'queued' OR d.pdf_status = 'queued')
     AND d.updated_at < now() - p_older
     AND NOT EXISTS (SELECT 1 FROM job j
                      WHERE j.status IN ('pending', 'running') AND j.kind LIKE 'document.%'
                        AND j.payload->>'documentId' = d.id AND j.tenant_id = d.tenant_id)
   ORDER BY d.updated_at
   LIMIT p_limit
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION document_reading_stuck(interval, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION document_reading_stuck(interval, integer) TO hiringplug_app;

-- +goose Down
DROP FUNCTION IF EXISTS document_reading_stuck(interval, integer);
DROP INDEX IF EXISTS document_reading_idx;
DROP INDEX IF EXISTS job_document_active_idx;
DROP TRIGGER IF EXISTS job_ai_dead_notice ON job;
DROP FUNCTION IF EXISTS job_ai_dead_notice();
