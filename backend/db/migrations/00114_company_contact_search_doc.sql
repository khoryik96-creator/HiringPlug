-- company.search_doc and contact.search_doc: what a boolean search of the
-- company and contact lists reads (boolsearch's to_tsquery), stored and
-- indexed. app_search_doc keeps each field apart (a NEAR or a phrase never
-- runs from one field into the next) and splits emails and web addresses
-- into their words (aisha@majubank.example: aisha, majubank, example), as
-- boolsearch splits what is typed.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION app_search_doc(VARIADIC parts text[]) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  -- 25 stop words between fields take up positions but match nothing:
  -- more than NEAR's widest (20 words).
  SELECT to_tsvector('english'::regconfig, coalesce(string_agg(
    regexp_replace(coalesce(p, ''), '[@./:_+]+', ' ', 'g'),
    ' the the the the the the the the the the the the the the the the the the the the the the the the the '), ''))
  FROM unnest(parts) AS p
$$;
-- +goose StatementEnd

ALTER TABLE company ADD COLUMN search_doc tsvector
  GENERATED ALWAYS AS (app_search_doc(name, website, industry, city, about)) STORED;
CREATE INDEX company_search_doc_idx ON company USING gin (search_doc);

ALTER TABLE contact ADD COLUMN search_doc tsvector
  GENERATED ALWAYS AS (app_search_doc(first_name, last_name, title, email)) STORED;
CREATE INDEX contact_search_doc_idx ON contact USING gin (search_doc);

-- +goose Down
DROP INDEX IF EXISTS contact_search_doc_idx;
ALTER TABLE contact DROP COLUMN IF EXISTS search_doc;
DROP INDEX IF EXISTS company_search_doc_idx;
ALTER TABLE company DROP COLUMN IF EXISTS search_doc;
DROP FUNCTION IF EXISTS app_search_doc(text[]);
