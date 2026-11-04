-- CV vs job match, continued.
-- matching: each score remembers the job text it was made from (a hash), so
--   a score is shown as outdated once the job's description or ad changes.
-- documents: the words of each candidate's CV (not formatted ones), indexed
--   for finding possible candidates for a job; kept in step with document
--   by a trigger (uploads, re-reads and merges alike).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE job_match ADD COLUMN job_hash text CHECK (length(job_hash) <= 64);

CREATE TABLE cv_search (
  document_id  text PRIMARY KEY REFERENCES document (id) ON DELETE CASCADE,
  tenant_id    text NOT NULL REFERENCES tenant (id),
  candidate_id text NOT NULL,
  terms        tsvector NOT NULL
);
CREATE INDEX cv_search_terms_idx ON cv_search USING gin (terms);
CREATE INDEX cv_search_candidate_idx ON cv_search (tenant_id, candidate_id);

ALTER TABLE cv_search ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cv_search
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY candidate_scope ON cv_search AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));

-- +goose StatementBegin
CREATE FUNCTION cv_search_sync() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM cv_search WHERE document_id = NEW.id;
  IF NEW.source <> 'formatted' AND NEW.status = 'extracted' AND NEW.extracted_text IS NOT NULL THEN
    INSERT INTO cv_search (document_id, tenant_id, candidate_id, terms)
    VALUES (NEW.id, NEW.tenant_id, NEW.candidate_id, to_tsvector('english', NEW.extracted_text));
  END IF;
  RETURN NULL;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER document_cv_search AFTER INSERT OR UPDATE OF extracted_text, candidate_id, source, status ON document
  FOR EACH ROW EXECUTE FUNCTION cv_search_sync();

INSERT INTO cv_search (document_id, tenant_id, candidate_id, terms)
SELECT id, tenant_id, candidate_id, to_tsvector('english', extracted_text) FROM document
WHERE source <> 'formatted' AND status = 'extracted' AND extracted_text IS NOT NULL;

-- +goose Down
DROP TRIGGER IF EXISTS document_cv_search ON document;
DROP FUNCTION IF EXISTS cv_search_sync();
DROP TABLE IF EXISTS cv_search;
ALTER TABLE job_match DROP COLUMN IF EXISTS job_hash;
