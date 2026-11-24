-- Companies named like a lead's company (Find HR for leads): a trigram index
-- on the name without its legal form, so the name-like search (the % operator
-- at a 0.6 threshold) no longer compares the name with every company.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE INDEX company_name_core_trgm_idx ON company USING gin (name_core gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS company_name_core_trgm_idx;
