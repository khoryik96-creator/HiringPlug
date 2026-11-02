-- candidates: work history and education (owned by the candidates module).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE candidate_work (
  tenant_id    text NOT NULL,
  candidate_id text NOT NULL,
  position     integer NOT NULL CHECK (position >= 0), -- 0 = newest
  title        text CHECK (length(title) <= 200),
  company      text CHECK (length(company) <= 200),
  start_date   text CHECK (start_date ~ '^[0-9]{4}(-[0-9]{2})?$'),
  end_date     text CHECK (end_date ~ '^[0-9]{4}(-[0-9]{2})?$'),
  is_current   boolean NOT NULL DEFAULT false,
  description  text CHECK (length(description) <= 4000),
  PRIMARY KEY (tenant_id, candidate_id, position),
  CHECK (title IS NOT NULL OR company IS NOT NULL),
  CHECK (NOT is_current OR end_date IS NULL),
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE candidate_work ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON candidate_work
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

CREATE TABLE candidate_education (
  tenant_id     text NOT NULL,
  candidate_id  text NOT NULL,
  position      integer NOT NULL CHECK (position >= 0),
  qualification text CHECK (length(qualification) <= 200),
  institution   text CHECK (length(institution) <= 200),
  end_year      integer CHECK (end_year BETWEEN 1950 AND 2100),
  PRIMARY KEY (tenant_id, candidate_id, position),
  CHECK (qualification IS NOT NULL OR institution IS NOT NULL),
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE candidate_education ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON candidate_education
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS candidate_education;
DROP TABLE IF EXISTS candidate_work;
