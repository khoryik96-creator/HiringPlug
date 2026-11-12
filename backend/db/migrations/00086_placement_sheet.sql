-- placements: the placement sheet. Job title, company name and job type are
-- filled in from the job when the placement is recorded and can be changed
-- on the placement; PO number and end date are optional. Existing placements
-- take them from their jobs.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE placement
  ADD COLUMN job_title    text CHECK (length(job_title) BETWEEN 1 AND 300),
  ADD COLUMN company_name text CHECK (length(company_name) BETWEEN 1 AND 300),
  ADD COLUMN job_type     text NOT NULL DEFAULT 'permanent' CHECK (job_type IN ('permanent', 'contract')),
  ADD COLUMN po_number    text CHECK (length(po_number) BETWEEN 1 AND 100),
  ADD COLUMN end_date     date,
  ADD CONSTRAINT placement_end_after_start CHECK (end_date IS NULL OR end_date >= start_date);

UPDATE placement p
   SET job_title = j.title, company_name = c.name,
       job_type = CASE WHEN j.work_type IN ('contract', 'temporary') THEN 'contract' ELSE 'permanent' END
  FROM application a
  JOIN job_order j ON j.tenant_id = a.tenant_id AND j.id = a.job_order_id
  JOIN company c ON c.tenant_id = j.tenant_id AND c.id = j.company_id
 WHERE a.tenant_id = p.tenant_id AND a.id = p.application_id;

-- +goose Down
ALTER TABLE placement DROP CONSTRAINT IF EXISTS placement_end_after_start,
  DROP COLUMN IF EXISTS end_date, DROP COLUMN IF EXISTS po_number, DROP COLUMN IF EXISTS job_type,
  DROP COLUMN IF EXISTS company_name, DROP COLUMN IF EXISTS job_title;
