-- Main settings.
-- settings: each company's dropdown options (field_option; no rows = the
--   built-in options) and which form fields are mandatory (field_rule).
-- candidates: a source is a key from the company's list, no longer one of a
--   fixed six.
-- identity: roles a company has customised get the new settings:read (every
--   form needs it); settings:manage stays with owners and admins by default.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE field_option (
  tenant_id  text NOT NULL REFERENCES tenant (id),
  list       text NOT NULL CHECK (list IN ('candidate_source', 'notice_period', 'industry', 'work_type', 'work_arrangement')),
  key        text NOT NULL CHECK (length(key) BETWEEN 1 AND 100),
  label      text NOT NULL CHECK (length(label) BETWEEN 1 AND 100),
  hidden     boolean NOT NULL DEFAULT false,
  position   integer NOT NULL CHECK (position >= 0),
  PRIMARY KEY (tenant_id, list, key)
);
-- A list a company has saved, even if it hid everything: rows above are the whole list.
CREATE TABLE field_list_saved (
  tenant_id  text NOT NULL REFERENCES tenant (id),
  list       text NOT NULL,
  updated_by text NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, list)
);
CREATE TABLE field_rule (
  tenant_id  text NOT NULL REFERENCES tenant (id),
  form       text NOT NULL CHECK (form IN ('candidate', 'job')),
  field      text NOT NULL CHECK (length(field) BETWEEN 1 AND 50),
  mandatory  boolean NOT NULL,
  PRIMARY KEY (tenant_id, form, field)
);

ALTER TABLE field_option ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON field_option
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE field_list_saved ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON field_list_saved
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE field_rule ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON field_rule
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE candidate DROP CONSTRAINT candidate_source_check;
ALTER TABLE candidate ADD CONSTRAINT candidate_source_check CHECK (source ~ '^[a-z0-9_]{1,40}$');

UPDATE role_setting SET permissions = array_append(permissions, 'settings:read')
WHERE NOT ('settings:read' = ANY (permissions));

-- +goose Down
UPDATE role_setting SET permissions = array_remove(array_remove(permissions, 'settings:read'), 'settings:manage');
UPDATE candidate SET source = 'other' WHERE source NOT IN ('jobstreet', 'linkedin', 'referral', 'careers_page', 'direct', 'other');
ALTER TABLE candidate DROP CONSTRAINT candidate_source_check;
ALTER TABLE candidate ADD CONSTRAINT candidate_source_check CHECK (source IN ('jobstreet', 'linkedin', 'referral', 'careers_page', 'direct', 'other'));
DROP TABLE IF EXISTS field_rule;
DROP TABLE IF EXISTS field_list_saved;
DROP TABLE IF EXISTS field_option;
