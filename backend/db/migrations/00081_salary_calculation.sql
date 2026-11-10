-- +goose Up
ALTER TABLE candidate_compensation
  ADD COLUMN current_description text CHECK (length(current_description) <= 2000),
  ADD COLUMN expected_description text CHECK (length(expected_description) <= 2000);

ALTER TABLE tenant_policy DROP CONSTRAINT tenant_policy_key_check;
ALTER TABLE tenant_policy ADD CONSTRAINT tenant_policy_key_check
  CHECK (key IN ('email_salary_visibility', 'salary_calculation'));
ALTER TABLE tenant_policy DROP CONSTRAINT tenant_policy_value_check;
ALTER TABLE tenant_policy ADD CONSTRAINT tenant_policy_value_check CHECK (length(value) BETWEEN 1 AND 500);

-- +goose Down
DELETE FROM tenant_policy WHERE key = 'salary_calculation';
ALTER TABLE tenant_policy DROP CONSTRAINT tenant_policy_key_check;
ALTER TABLE tenant_policy ADD CONSTRAINT tenant_policy_key_check CHECK (key IN ('email_salary_visibility'));
ALTER TABLE tenant_policy DROP CONSTRAINT tenant_policy_value_check;
ALTER TABLE tenant_policy ADD CONSTRAINT tenant_policy_value_check CHECK (length(value) BETWEEN 1 AND 100);
ALTER TABLE candidate_compensation DROP COLUMN current_description, DROP COLUMN expected_description;
