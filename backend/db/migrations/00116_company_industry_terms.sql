-- company.industry_terms: the words of every industry and subindustry a
-- company has (current or not), kept by the industries module, so a boolean
-- company search finds "insurance" for a company tagged Financial Services >
-- Insurance, not only its main industry.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE company ADD COLUMN industry_terms text CHECK (length(industry_terms) <= 10000);
UPDATE company c SET industry_terms = t.terms
FROM (
  SELECT a.company_id, string_agg(DISTINCT concat_ws(' ', p.label, i.label), ' ') AS terms
  FROM industry_assignment a
  JOIN industry i ON i.id = a.category_id
  LEFT JOIN industry p ON p.id = i.parent_id
  WHERE a.company_id IS NOT NULL
  GROUP BY a.company_id
) t
WHERE t.company_id = c.id;
DROP INDEX IF EXISTS company_search_doc_idx;
CREATE INDEX company_search_doc_idx ON company
  USING gin (app_search_doc(name, website, industry, industry_terms, industry_hint, city, about));

-- +goose Down
DROP INDEX IF EXISTS company_search_doc_idx;
CREATE INDEX company_search_doc_idx ON company USING gin (app_search_doc(name, website, industry, industry_hint, city, about));
ALTER TABLE company DROP COLUMN IF EXISTS industry_terms;
