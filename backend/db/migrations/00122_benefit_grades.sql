-- companies: a client's benefits may differ by grade. Its grades are kept in
-- order (e.g. ["Executive", "Manager", "Senior Manager"]); a benefit that
-- differs has a value per grade in its byGrade list (inside benefits).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE company_benefits
  ADD COLUMN grades jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(grades) = 'array');

-- +goose Down
ALTER TABLE company_benefits DROP COLUMN IF EXISTS grades;
