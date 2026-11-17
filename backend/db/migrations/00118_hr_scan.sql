-- contacts module: Find HR, a company's people in HR read from LinkedIn's
-- people search by the recruiter's companion, page by page. One scan per
-- company (a new one replaces it); next_query/next_page are where it goes on,
-- also after a page failed, so it resumes there.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE hr_scan (
  tenant_id             text NOT NULL REFERENCES tenant (id),
  company_id            text NOT NULL,
  linkedin_company_id   text NOT NULL CHECK (linkedin_company_id ~ '^[0-9]{1,24}$'),
  linkedin_company_name text NOT NULL CHECK (length(linkedin_company_name) BETWEEN 1 AND 200),
  country               text CHECK (country ~ '^[A-Z]{2}$'),
  keywords              text[] NOT NULL CHECK (cardinality(keywords) BETWEEN 1 AND 12),
  pages                 integer NOT NULL CHECK (pages BETWEEN 1 AND 10),
  status                text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'stopped', 'done')),
  next_query            integer NOT NULL DEFAULT 0 CHECK (next_query >= 0),
  next_page             integer NOT NULL DEFAULT 1 CHECK (next_page >= 1),
  pages_read            integer NOT NULL DEFAULT 0,
  error                 text CHECK (length(error) <= 300),
  -- [{url, name, title, location, joined, left, current}], at most 400.
  people                jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_array_length(people) <= 400),
  started_by            text NOT NULL,
  started_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, company_id),
  FOREIGN KEY (tenant_id, company_id) REFERENCES company (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE hr_scan ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON hr_scan USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS hr_scan;
