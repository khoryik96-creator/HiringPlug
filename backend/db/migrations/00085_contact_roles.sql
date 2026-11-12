-- settings: a contact_role list (Hiring manager, HR to start with), each
--   with a colour, like company categories; the lists' check names it.
-- contacts: each contact's role (a key of that list; null: not set), and an
--   index to filter by it.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE field_option DROP CONSTRAINT field_option_list_check;
ALTER TABLE field_option ADD CONSTRAINT field_option_list_check CHECK (list IN (
  'candidate_source', 'notice_period', 'industry', 'work_type', 'work_arrangement',
  'screening_call', 'reference_check', 'company_benefits', 'pre_offer_process', 'company_category',
  'contact_role'));

ALTER TABLE contact ADD COLUMN role text CHECK (role ~ '^[a-z0-9_]{1,40}$');
CREATE INDEX contact_role_idx ON contact (tenant_id, role);

-- +goose Down
DROP INDEX IF EXISTS contact_role_idx;
ALTER TABLE contact DROP COLUMN IF EXISTS role;
DELETE FROM field_option WHERE list = 'contact_role';
DELETE FROM field_list_saved WHERE list = 'contact_role';
ALTER TABLE field_option DROP CONSTRAINT field_option_list_check;
ALTER TABLE field_option ADD CONSTRAINT field_option_list_check CHECK (list IN (
  'candidate_source', 'notice_period', 'industry', 'work_type', 'work_arrangement',
  'screening_call', 'reference_check', 'company_benefits', 'pre_offer_process', 'company_category'));
