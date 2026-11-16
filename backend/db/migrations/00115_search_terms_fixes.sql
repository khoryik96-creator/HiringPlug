-- Boolean search fixes:
-- * a candidate's certifications count in the profile's words
--   (candidate.profile_terms), as their skills and tags do;
-- * company and contact searches read an index on app_search_doc(...)
--   instead of a stored column (which every read of a row also fetched);
--   a contact's first and last names are one field there, so
--   "Aisha Rahman" finds her, and a company's LinkedIn industry counts too.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- +goose StatementBegin
-- As app_profile_terms (00064), with the certifications.
CREATE FUNCTION app_profile_terms(title text, company text, skills text[], tags text[], industry text,
                                  specialization text, location text, certifications text[]) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT to_tsvector('english', concat_ws(' ', title, company, array_to_string(skills, ' '), array_to_string(tags, ' '),
                                              industry, specialization, location, array_to_string(certifications, ' ')))
$$;
-- +goose StatementEnd
ALTER TABLE candidate DROP COLUMN profile_terms;
ALTER TABLE candidate ADD COLUMN profile_terms tsvector GENERATED ALWAYS AS (
  app_profile_terms(current_title, current_company, skills, tags, industry, specialization, location, certifications)
) STORED;
CREATE INDEX candidate_profile_terms_idx ON candidate USING gin (profile_terms);

ALTER TABLE company DROP COLUMN search_doc;
CREATE INDEX company_search_doc_idx ON company USING gin (app_search_doc(name, website, industry, industry_hint, city, about));
ALTER TABLE contact DROP COLUMN search_doc;
CREATE INDEX contact_search_doc_idx ON contact USING gin (app_search_doc(first_name || ' ' || last_name, title, email));

-- +goose Down
DROP INDEX IF EXISTS contact_search_doc_idx;
ALTER TABLE contact ADD COLUMN search_doc tsvector
  GENERATED ALWAYS AS (app_search_doc(first_name, last_name, title, email)) STORED;
CREATE INDEX contact_search_doc_idx ON contact USING gin (search_doc);
DROP INDEX IF EXISTS company_search_doc_idx;
ALTER TABLE company ADD COLUMN search_doc tsvector
  GENERATED ALWAYS AS (app_search_doc(name, website, industry, city, about)) STORED;
CREATE INDEX company_search_doc_idx ON company USING gin (search_doc);
ALTER TABLE candidate DROP COLUMN profile_terms;
ALTER TABLE candidate ADD COLUMN profile_terms tsvector GENERATED ALWAYS AS (
  app_profile_terms(current_title, current_company, skills, tags, industry, specialization, location)
) STORED;
CREATE INDEX candidate_profile_terms_idx ON candidate USING gin (profile_terms);
DROP FUNCTION IF EXISTS app_profile_terms(text, text, text[], text[], text, text, text, text[]);
