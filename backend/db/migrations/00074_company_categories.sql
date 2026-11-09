-- settings: a company_category list (Signed client, Prospective client,
--   Unknown, VIP client to start with), whose options carry a colour
--   (field_option.color); the lists' check names it.
-- companies: each company's category (a key of that list; null: not set,
--   shown as Unknown). Prospects start as prospective clients.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE field_option DROP CONSTRAINT field_option_list_check;
ALTER TABLE field_option ADD CONSTRAINT field_option_list_check CHECK (list IN (
  'candidate_source', 'notice_period', 'industry', 'work_type', 'work_arrangement',
  'screening_call', 'reference_check', 'company_benefits', 'pre_offer_process', 'company_category'));
ALTER TABLE field_option ADD COLUMN color text CHECK (color ~ '^#[0-9a-f]{6}$');

ALTER TABLE company ADD COLUMN category text CHECK (category ~ '^[a-z0-9_]{1,40}$');
UPDATE company SET category = 'prospective_client' WHERE type = 'prospect';
CREATE INDEX company_category_idx ON company (tenant_id, category);

-- +goose Down
DROP INDEX IF EXISTS company_category_idx;
ALTER TABLE company DROP COLUMN IF EXISTS category;
DELETE FROM field_option WHERE list = 'company_category';
DELETE FROM field_list_saved WHERE list = 'company_category';
ALTER TABLE field_option DROP COLUMN IF EXISTS color;
ALTER TABLE field_option DROP CONSTRAINT field_option_list_check;
ALTER TABLE field_option ADD CONSTRAINT field_option_list_check CHECK (list IN (
  'candidate_source', 'notice_period', 'industry', 'work_type', 'work_arrangement',
  'screening_call', 'reference_check', 'company_benefits', 'pre_offer_process'));
