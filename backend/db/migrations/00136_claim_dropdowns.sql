-- settings: two more dropdowns a company edits, for expense claims:
--   claim_receipt_type (what a receipt is for, e.g. Grab fare, food, drinks;
--   read from receipts) and claim_type (what a claim is, e.g. travel).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE field_option DROP CONSTRAINT field_option_list_check;
ALTER TABLE field_option ADD CONSTRAINT field_option_list_check CHECK (list IN (
  'candidate_source', 'notice_period', 'industry', 'work_type', 'work_arrangement',
  'screening_call', 'reference_check', 'company_benefits', 'pre_offer_process', 'company_category',
  'contact_role', 'specialization', 'it_skill', 'certification', 'claim_receipt_type', 'claim_type'));

-- +goose Down
DELETE FROM field_option WHERE list IN ('claim_receipt_type', 'claim_type');
DELETE FROM field_list_saved WHERE list IN ('claim_receipt_type', 'claim_type');
ALTER TABLE field_option DROP CONSTRAINT field_option_list_check;
ALTER TABLE field_option ADD CONSTRAINT field_option_list_check CHECK (list IN (
  'candidate_source', 'notice_period', 'industry', 'work_type', 'work_arrangement',
  'screening_call', 'reference_check', 'company_benefits', 'pre_offer_process', 'company_category',
  'contact_role', 'specialization', 'it_skill', 'certification'));
