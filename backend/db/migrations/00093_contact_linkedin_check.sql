-- contact_linkedin_check: where a contact works, read from their LinkedIn
-- profile by the HiringPlug LinkedIn companion (a browser extension in the
-- recruiter's own browser). The server makes each check, bound to the
-- contact, their profile and the company's LinkedIn page, and takes back only
-- the dates of the contact's time at that exact company: joined, left and
-- whether it is current. A check not answered within 15 minutes expires.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE contact_linkedin_check (
  id                   text PRIMARY KEY DEFAULT new_id('clc') CHECK (id ~ '^clc_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id            text NOT NULL REFERENCES tenant (id),
  contact_id           text NOT NULL,
  company_id           text NOT NULL,
  profile_url          text NOT NULL CHECK (profile_url ~ '^https://www\.linkedin\.com/in/[A-Za-z0-9_%~-]+$'),
  company_url          text NOT NULL CHECK (company_url ~ '^https://www\.linkedin\.com/company/[A-Za-z0-9_-]+/$'),
  linkedin_company_id  text CHECK (linkedin_company_id ~ '^[0-9]{1,24}$'),
  status               text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'observed', 'failed', 'expired')),
  joined               text CHECK (joined ~ '^(19|20)[0-9]{2}(-(0[1-9]|1[0-2]))?$'),
  left_on              text CHECK (left_on ~ '^(19|20)[0-9]{2}(-(0[1-9]|1[0-2]))?$'),
  current_job          boolean,
  marked_left          boolean NOT NULL DEFAULT false,
  error                text CHECK (length(error) <= 300),
  requested_by         text NOT NULL,
  created_at           timestamptz NOT NULL DEFAULT now(),
  checked_at           timestamptz,
  FOREIGN KEY (tenant_id, contact_id) REFERENCES contact (tenant_id, id) ON DELETE CASCADE,
  -- current: joined, no end; former: both; unknown: neither.
  CHECK (status <> 'observed' OR (current_job IS TRUE AND joined IS NOT NULL AND left_on IS NULL)
    OR (current_job IS FALSE AND joined IS NOT NULL AND left_on IS NOT NULL)
    OR (current_job IS NULL AND joined IS NULL AND left_on IS NULL))
);
CREATE INDEX contact_linkedin_check_contact_idx ON contact_linkedin_check (tenant_id, contact_id, created_at DESC);

ALTER TABLE contact_linkedin_check ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON contact_linkedin_check
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS contact_linkedin_check;
