-- +goose Up
-- A lead's favourites and saved ad text belong to a lead of the same company
-- (tenant): the references include the tenant, so a row can never point at,
-- or take the evidence slot of, another tenant's lead. A favourite also goes
-- with its user.
ALTER TABLE job_lead ADD CONSTRAINT job_lead_tenant_id_id_key UNIQUE (tenant_id, id);

ALTER TABLE job_lead_favorite DROP CONSTRAINT job_lead_favorite_lead_id_fkey;
ALTER TABLE job_lead_favorite ADD CONSTRAINT job_lead_favorite_lead_fkey
  FOREIGN KEY (tenant_id, lead_id) REFERENCES job_lead (tenant_id, id) ON DELETE CASCADE;
ALTER TABLE job_lead_favorite ADD CONSTRAINT job_lead_favorite_user_fkey
  FOREIGN KEY (tenant_id, user_id) REFERENCES app_user (tenant_id, id) ON DELETE CASCADE;

ALTER TABLE job_lead_evidence DROP CONSTRAINT job_lead_evidence_lead_id_fkey;
ALTER TABLE job_lead_evidence ADD CONSTRAINT job_lead_evidence_lead_fkey
  FOREIGN KEY (tenant_id, lead_id) REFERENCES job_lead (tenant_id, id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE job_lead_evidence DROP CONSTRAINT job_lead_evidence_lead_fkey;
ALTER TABLE job_lead_evidence ADD CONSTRAINT job_lead_evidence_lead_id_fkey
  FOREIGN KEY (lead_id) REFERENCES job_lead (id) ON DELETE CASCADE;
ALTER TABLE job_lead_favorite DROP CONSTRAINT job_lead_favorite_user_fkey;
ALTER TABLE job_lead_favorite DROP CONSTRAINT job_lead_favorite_lead_fkey;
ALTER TABLE job_lead_favorite ADD CONSTRAINT job_lead_favorite_lead_id_fkey
  FOREIGN KEY (lead_id) REFERENCES job_lead (id) ON DELETE CASCADE;
ALTER TABLE job_lead DROP CONSTRAINT job_lead_tenant_id_id_key;
