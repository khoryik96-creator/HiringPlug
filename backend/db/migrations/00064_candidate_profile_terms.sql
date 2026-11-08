-- candidates: the words of a candidate's profile (title, company, skills,
-- tags, industry, specialization, location) as a text search, so an
-- advanced search's boolean (SAP AND (BW OR BPC)) can be looked for in the
-- profile as well as in the CVs.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- +goose StatementBegin
-- array_to_string is only STABLE in general; for text[] it does not depend
-- on settings, so this wrapper may be IMMUTABLE (needed by a generated column).
CREATE FUNCTION app_profile_terms(title text, company text, skills text[], tags text[], industry text,
                                  specialization text, location text) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT to_tsvector('english', concat_ws(' ', title, company, array_to_string(skills, ' '), array_to_string(tags, ' '),
                                              industry, specialization, location))
$$;
-- +goose StatementEnd

ALTER TABLE candidate ADD COLUMN profile_terms tsvector GENERATED ALWAYS AS (
  app_profile_terms(current_title, current_company, skills, tags, industry, specialization, location)
) STORED;
CREATE INDEX candidate_profile_terms_idx ON candidate USING gin (profile_terms);

-- +goose Down
ALTER TABLE candidate DROP COLUMN IF EXISTS profile_terms;
DROP FUNCTION IF EXISTS app_profile_terms(text, text, text[], text[], text, text, text);
