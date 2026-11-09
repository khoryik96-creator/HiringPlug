-- companies: each client's benefits and pre-offer process (company_benefits),
--   filled in from the agency's template.
-- settings: the template is two of the company's lists (company_benefits and
--   pre_offer_process), whose items may carry a hint (what to write). The
--   lists' check now names every list: screening_call and reference_check
--   could not be saved before.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE field_option DROP CONSTRAINT field_option_list_check;
ALTER TABLE field_option ADD CONSTRAINT field_option_list_check CHECK (list IN (
  'candidate_source', 'notice_period', 'industry', 'work_type', 'work_arrangement',
  'screening_call', 'reference_check', 'company_benefits', 'pre_offer_process'));
ALTER TABLE field_option ADD COLUMN hint text CHECK (length(hint) BETWEEN 1 AND 300);

-- benefits: [{key, label, value, remarks}]; pre_offer: [{key, label, applies,
-- remarks}], in the template's order when saved (labels kept, so an item later
-- taken out of the template still reads).
CREATE TABLE company_benefits (
  tenant_id  text NOT NULL,
  company_id text NOT NULL,
  benefits   jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(benefits) = 'array'),
  pre_offer  jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(pre_offer) = 'array'),
  version    integer NOT NULL DEFAULT 1,
  updated_by text NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, company_id),
  FOREIGN KEY (tenant_id, company_id) REFERENCES company (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE company_benefits ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON company_benefits
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS company_benefits;
DELETE FROM field_option WHERE list IN ('screening_call', 'reference_check', 'company_benefits', 'pre_offer_process');
DELETE FROM field_list_saved WHERE list IN ('screening_call', 'reference_check', 'company_benefits', 'pre_offer_process');
ALTER TABLE field_option DROP COLUMN IF EXISTS hint;
ALTER TABLE field_option DROP CONSTRAINT field_option_list_check;
ALTER TABLE field_option ADD CONSTRAINT field_option_list_check CHECK (list IN (
  'candidate_source', 'notice_period', 'industry', 'work_type', 'work_arrangement'));
