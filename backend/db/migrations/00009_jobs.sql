-- jobs module: job orders (agency) or requisitions (in-house), and their
-- salary budget in its own table that only permitted users can see.
-- Committed migrations are never edited (CI enforces this): add a new file.
-- (The table is job_order: "job" is the background job queue.)

-- +goose Up
CREATE TABLE job_order (
  id               text PRIMARY KEY DEFAULT new_id('jo') CHECK (id ~ '^jo_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id        text NOT NULL REFERENCES tenant (id),
  version          integer NOT NULL DEFAULT 1,
  company_id       text NOT NULL,
  contact_id       text,
  title            text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
  status           text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'on_hold', 'filled', 'closed')),
  location         text CHECK (length(location) <= 200),
  work_type        text CHECK (work_type IN ('full_time', 'part_time', 'contract', 'temporary', 'internship')),
  work_arrangement text CHECK (work_arrangement IN ('onsite', 'hybrid', 'remote')),
  openings         integer NOT NULL DEFAULT 1 CHECK (openings BETWEEN 1 AND 999),
  description      text CHECK (length(description) <= 20000),
  search_text      text GENERATED ALWAYS AS (app_normalize_text(title || ' ' || coalesce(location, ''))) STORED,
  created_by       text NOT NULL,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  -- Company and contact belong to the same tenant (composite keys).
  FOREIGN KEY (tenant_id, company_id) REFERENCES company (tenant_id, id),
  FOREIGN KEY (tenant_id, contact_id) REFERENCES contact (tenant_id, id)
);
CREATE INDEX job_order_tenant_id_idx ON job_order (tenant_id, id DESC);
CREATE INDEX job_order_company_idx ON job_order (tenant_id, company_id, id DESC);
CREATE INDEX job_order_search_trgm_idx ON job_order USING gin (search_text gin_trgm_ops);

ALTER TABLE job_order ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON job_order
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- The salary budget: rows are invisible without jobs.salary:read, and only
-- writable with jobs.salary:update, whatever the query.
CREATE TABLE job_order_salary (
  job_order_id text PRIMARY KEY,
  tenant_id    text NOT NULL,
  version      integer NOT NULL DEFAULT 1 CHECK (version >= 1),
  -- A row is a complete budget; clearing the budget deletes the row.
  min_amount   numeric(14, 2) NOT NULL CHECK (min_amount >= 0),
  max_amount   numeric(14, 2) CHECK (max_amount >= 0),
  currency     text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  period       text NOT NULL CHECK (period IN ('hour', 'day', 'month', 'year')),
  updated_by   text NOT NULL,
  updated_at   timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, job_order_id) REFERENCES job_order (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT job_salary_range CHECK (max_amount IS NULL OR max_amount >= min_amount)
);

ALTER TABLE job_order_salary ENABLE ROW LEVEL SECURITY;
CREATE POLICY job_salary_select ON job_order_salary FOR SELECT
  USING (tenant_id = app_current_tenant() AND app_has_permission('jobs.salary:read'));
CREATE POLICY job_salary_insert ON job_order_salary FOR INSERT
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('jobs.salary:update'));
CREATE POLICY job_salary_update ON job_order_salary FOR UPDATE
  USING (tenant_id = app_current_tenant() AND app_has_permission('jobs.salary:update'))
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('jobs.salary:update'));
CREATE POLICY job_salary_delete ON job_order_salary FOR DELETE
  USING (tenant_id = app_current_tenant() AND app_has_permission('jobs.salary:update'));

-- +goose Down
DROP TABLE IF EXISTS job_order_salary;
DROP TABLE IF EXISTS job_order;
