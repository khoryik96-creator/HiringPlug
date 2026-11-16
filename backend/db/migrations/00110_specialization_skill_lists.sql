-- settings: three more dropdowns a company edits, specialization, it_skill
--   and certification (the choices for a candidate's specialization, skills
--   and certifications). Candidates store the words, as for skills.
-- candidate: certifications (e.g. PMP, ACCA), like skills.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE field_option DROP CONSTRAINT field_option_list_check;
ALTER TABLE field_option ADD CONSTRAINT field_option_list_check CHECK (list IN (
  'candidate_source', 'notice_period', 'industry', 'work_type', 'work_arrangement',
  'screening_call', 'reference_check', 'company_benefits', 'pre_offer_process', 'company_category',
  'contact_role', 'specialization', 'it_skill', 'certification'));

ALTER TABLE candidate ADD COLUMN certifications text[] NOT NULL DEFAULT '{}' CHECK (cardinality(certifications) <= 50);

-- +goose Down
ALTER TABLE candidate DROP COLUMN IF EXISTS certifications;
DELETE FROM field_option WHERE list IN ('specialization', 'it_skill', 'certification');
DELETE FROM field_list_saved WHERE list IN ('specialization', 'it_skill', 'certification');
ALTER TABLE field_option DROP CONSTRAINT field_option_list_check;
ALTER TABLE field_option ADD CONSTRAINT field_option_list_check CHECK (list IN (
  'candidate_source', 'notice_period', 'industry', 'work_type', 'work_arrangement',
  'screening_call', 'reference_check', 'company_benefits', 'pre_offer_process', 'company_category',
  'contact_role'));
