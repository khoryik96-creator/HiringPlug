-- companies: a client's terms of business become rates by annual salary
--   (company_terms.tiers), each with its fee, payment terms, guarantee and
--   remarks (another payment method), the salary limits in salary_currency.
--   Terms saved before become one rate for any salary.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE company_terms
  ADD COLUMN tiers jsonb NOT NULL DEFAULT '[]'
    CHECK (jsonb_typeof(tiers) = 'array' AND jsonb_array_length(tiers) <= 20),
  ADD COLUMN salary_currency text NOT NULL DEFAULT 'MYR' CHECK (salary_currency ~ '^[A-Z]{3}$');

UPDATE company_terms SET tiers = jsonb_build_array(jsonb_strip_nulls(jsonb_build_object(
  'feePercent', fee_percent::text, 'paymentTermsDays', payment_terms_days, 'guaranteeDays', guarantee_days)))
WHERE fee_percent IS NOT NULL OR payment_terms_days IS NOT NULL OR guarantee_days IS NOT NULL;

ALTER TABLE company_terms DROP COLUMN fee_percent, DROP COLUMN payment_terms_days, DROP COLUMN guarantee_days;

-- +goose Down
ALTER TABLE company_terms
  ADD COLUMN fee_percent numeric(5, 2) CHECK (fee_percent > 0 AND fee_percent <= 100),
  ADD COLUMN payment_terms_days integer CHECK (payment_terms_days BETWEEN 0 AND 365),
  ADD COLUMN guarantee_days integer CHECK (guarantee_days BETWEEN 0 AND 365);
UPDATE company_terms SET fee_percent = (tiers -> 0 ->> 'feePercent')::numeric,
  payment_terms_days = (tiers -> 0 ->> 'paymentTermsDays')::integer, guarantee_days = (tiers -> 0 ->> 'guaranteeDays')::integer
WHERE jsonb_array_length(tiers) > 0;
ALTER TABLE company_terms DROP COLUMN tiers, DROP COLUMN salary_currency;
