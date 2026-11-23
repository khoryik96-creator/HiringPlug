-- Client shortlist links and formatted_cv follow what they point to: a
-- candidate deleted for good (the bin emptied, or erased on request), their
-- application, their CV or the job takes their places on links with them, so
-- a client's page never shows someone who is gone. Rows already pointing at
-- something deleted are removed first (they could only show a dead entry).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
DELETE FROM client_shortlist_candidate s WHERE NOT EXISTS (SELECT 1 FROM candidate c WHERE c.tenant_id = s.tenant_id AND c.id = s.candidate_id)
  OR NOT EXISTS (SELECT 1 FROM application a WHERE a.tenant_id = s.tenant_id AND a.id = s.application_id)
  OR NOT EXISTS (SELECT 1 FROM document d WHERE d.id = s.document_id);
DELETE FROM client_shortlist s WHERE NOT EXISTS (SELECT 1 FROM job_order j WHERE j.tenant_id = s.tenant_id AND j.id = s.job_id);
DELETE FROM formatted_cv f WHERE NOT EXISTS (SELECT 1 FROM document d WHERE d.id = f.document_id)
  OR NOT EXISTS (SELECT 1 FROM candidate c WHERE c.tenant_id = f.tenant_id AND c.id = f.candidate_id);

ALTER TABLE client_shortlist_candidate
  ADD CONSTRAINT client_shortlist_candidate_candidate_fk FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE ON UPDATE CASCADE,
  ADD CONSTRAINT client_shortlist_candidate_application_fk FOREIGN KEY (tenant_id, application_id) REFERENCES application (tenant_id, id) ON DELETE CASCADE,
  ADD CONSTRAINT client_shortlist_candidate_document_fk FOREIGN KEY (document_id) REFERENCES document (id) ON DELETE CASCADE;
ALTER TABLE client_shortlist
  ADD CONSTRAINT client_shortlist_job_fk FOREIGN KEY (tenant_id, job_id) REFERENCES job_order (tenant_id, id) ON DELETE CASCADE;
ALTER TABLE formatted_cv
  ADD CONSTRAINT formatted_cv_document_fk FOREIGN KEY (document_id) REFERENCES document (id) ON DELETE CASCADE,
  ADD CONSTRAINT formatted_cv_candidate_fk FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE formatted_cv DROP CONSTRAINT IF EXISTS formatted_cv_candidate_fk, DROP CONSTRAINT IF EXISTS formatted_cv_document_fk;
ALTER TABLE client_shortlist DROP CONSTRAINT IF EXISTS client_shortlist_job_fk;
ALTER TABLE client_shortlist_candidate DROP CONSTRAINT IF EXISTS client_shortlist_candidate_document_fk,
  DROP CONSTRAINT IF EXISTS client_shortlist_candidate_application_fk, DROP CONSTRAINT IF EXISTS client_shortlist_candidate_candidate_fk;
