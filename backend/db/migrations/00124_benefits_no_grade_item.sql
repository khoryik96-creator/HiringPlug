-- settings: "Job grade/level" leaves the benefits template. A client's grades
--   are set at the top of its benefits, each benefit giving a value per grade,
--   so the item said the same thing twice. Agencies that saved their own
--   template keep it as a retired item: hidden, but a company that wrote
--   something in it still shows it.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
UPDATE field_option SET hidden = true WHERE list = 'company_benefits' AND key = 'job_grade';

-- +goose Down
UPDATE field_option SET hidden = false WHERE list = 'company_benefits' AND key = 'job_grade';
