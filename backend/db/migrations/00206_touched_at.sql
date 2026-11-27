-- Stale records (1.54.0): when someone last did something with a contact or a
-- company, so the lists can show those nobody touched for a year. "Touched"
-- is what people do: the record created or edited, a note written on it, an
-- email sent to the contact, a job opened or changed for the company; a
-- contact touched touches their company too. Automatic work (the monthly
-- email checks, syncing) never does, so updated_at cannot serve.
-- Filled here from what is already known; then kept by the contacts and
-- companies modules (Touch), called by notes, emails and jobs.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE contact ADD COLUMN touched_at timestamptz;
ALTER TABLE company ADD COLUMN touched_at timestamptz;

UPDATE contact c SET touched_at = greatest(
  c.created_at, c.updated_at,
  (SELECT max(n.created_at) FROM note n WHERE n.tenant_id = c.tenant_id AND n.subject_type = 'contact' AND n.subject_id = c.id),
  (SELECT max(e.created_at) FROM email e WHERE e.tenant_id = c.tenant_id AND e.contact_id = c.id),
  (SELECT max(j.updated_at) FROM job_order j WHERE j.tenant_id = c.tenant_id AND j.contact_id = c.id));

UPDATE company co SET touched_at = greatest(
  co.created_at, co.updated_at,
  (SELECT max(n.created_at) FROM note n WHERE n.tenant_id = co.tenant_id AND n.subject_type = 'company' AND n.subject_id = co.id),
  (SELECT max(j.updated_at) FROM job_order j WHERE j.tenant_id = co.tenant_id AND j.company_id = co.id),
  (SELECT max(c.touched_at) FROM contact c WHERE c.tenant_id = co.tenant_id AND c.company_id = co.id));

ALTER TABLE contact ALTER COLUMN touched_at SET DEFAULT now(), ALTER COLUMN touched_at SET NOT NULL;
ALTER TABLE company ALTER COLUMN touched_at SET DEFAULT now(), ALTER COLUMN touched_at SET NOT NULL;
CREATE INDEX contact_touched_idx ON contact (tenant_id, touched_at);
CREATE INDEX company_touched_idx ON company (tenant_id, touched_at);

-- +goose Down
DROP INDEX IF EXISTS company_touched_idx;
DROP INDEX IF EXISTS contact_touched_idx;
ALTER TABLE company DROP COLUMN IF EXISTS touched_at;
ALTER TABLE contact DROP COLUMN IF EXISTS touched_at;
