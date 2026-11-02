-- candidates: tags (your own labels, e.g. "relocate") and skills.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE candidate
  ADD COLUMN tags   text[] NOT NULL DEFAULT '{}' CHECK (cardinality(tags) <= 50),
  ADD COLUMN skills text[] NOT NULL DEFAULT '{}' CHECK (cardinality(skills) <= 100);
-- Lower-cased copies for filtering (tag = "Relocate" finds "relocate").
CREATE FUNCTION app_lower_array(a text[]) RETURNS text[]
  LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT array_agg(lower(x)) FROM unnest(a) AS x $$;
CREATE INDEX candidate_tags_idx ON candidate USING gin (app_lower_array(tags));
CREATE INDEX candidate_skills_idx ON candidate USING gin (app_lower_array(skills));

-- +goose Down
DROP INDEX IF EXISTS candidate_skills_idx;
DROP INDEX IF EXISTS candidate_tags_idx;
DROP FUNCTION IF EXISTS app_lower_array(text[]);
ALTER TABLE candidate DROP COLUMN IF EXISTS skills, DROP COLUMN IF EXISTS tags;
