-- Quick search (like Kano's /find): every word typed must appear (so
-- "edran lenovo" finds Edran at Lenovo), a phone number matches however it
-- was written ("016-766 3880", "+60 16 766 3880"), and names can be looked
-- up loosely for typos ("edran zainl").
-- candidate: also searched by current title and company.
-- company: also searched by phone.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- +goose StatementBegin
-- The query split into words, numbered in order; words with no letter or
-- digit ("-", "&") are left out.
CREATE FUNCTION app_search_words(query text) RETURNS TABLE (n bigint, word text)
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT w.n, w.word
  FROM unnest(string_to_array(app_normalize_text(query), ' ')) WITH ORDINALITY AS w(word, n)
  WHERE w.word ~ '[[:alnum:]]'
$$;

-- The LIKE patterns for every word of the query: search_text LIKE ALL (...).
CREATE FUNCTION app_search_patterns(query text) RETURNS text[]
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT coalesce(array_agg('%' || replace(replace(replace(word, '\', '\\'), '%', '\%'), '_', '\_') || '%'), '{}')
  FROM app_search_words(query)
$$;

-- The digits to look for when the query is a phone number (at least 7
-- digits, nothing but digits, spaces, + - ( ) .): its last 8 digits, so a
-- country code or leading 0 typed or stored either way still matches.
CREATE FUNCTION app_phone_key(query text) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT CASE
    WHEN query ~ '^[+0-9 ().-]+$' AND length(regexp_replace(query, '[^0-9]', '', 'g')) >= 7
    THEN right(regexp_replace(query, '[^0-9]', '', 'g'), 8)
  END
$$;
-- +goose StatementEnd

ALTER TABLE candidate DROP COLUMN search_text;
ALTER TABLE candidate
  ADD COLUMN search_text text GENERATED ALWAYS AS (
    app_normalize_text(first_name || ' ' || last_name || ' ' || coalesce(email, '') || ' '
      || coalesce(current_title, '') || ' ' || coalesce(current_company, ''))
  ) STORED,
  ADD COLUMN name_text text GENERATED ALWAYS AS (app_normalize_text(first_name || ' ' || last_name)) STORED,
  ADD COLUMN phone_digits text GENERATED ALWAYS AS (regexp_replace(coalesce(phone, ''), '[^0-9]', '', 'g')) STORED;
CREATE INDEX candidate_search_trgm_idx ON candidate USING gin (search_text gin_trgm_ops);
CREATE INDEX candidate_name_trgm_idx ON candidate USING gin (name_text gin_trgm_ops);
CREATE INDEX candidate_phone_trgm_idx ON candidate USING gin (phone_digits gin_trgm_ops);

ALTER TABLE contact
  ADD COLUMN name_text text GENERATED ALWAYS AS (app_normalize_text(first_name || ' ' || last_name)) STORED,
  ADD COLUMN phone_digits text GENERATED ALWAYS AS (
    regexp_replace(coalesce(phone, ''), '[^0-9]', '', 'g') || ' ' || regexp_replace(coalesce(mobile, ''), '[^0-9]', '', 'g')
  ) STORED;
CREATE INDEX contact_name_trgm_idx ON contact USING gin (name_text gin_trgm_ops);

ALTER TABLE company
  ADD COLUMN phone_digits text GENERATED ALWAYS AS (regexp_replace(coalesce(phone, ''), '[^0-9]', '', 'g')) STORED;

-- +goose Down
ALTER TABLE company DROP COLUMN IF EXISTS phone_digits;
DROP INDEX IF EXISTS contact_name_trgm_idx;
ALTER TABLE contact DROP COLUMN IF EXISTS phone_digits, DROP COLUMN IF EXISTS name_text;
ALTER TABLE candidate DROP COLUMN IF EXISTS phone_digits, DROP COLUMN IF EXISTS name_text, DROP COLUMN search_text;
ALTER TABLE candidate
  ADD COLUMN search_text text GENERATED ALWAYS AS (
    app_normalize_text(first_name || ' ' || last_name || ' ' || coalesce(email, ''))
  ) STORED;
CREATE INDEX candidate_search_trgm_idx ON candidate USING gin (search_text gin_trgm_ops);
DROP FUNCTION IF EXISTS app_phone_key(text);
DROP FUNCTION IF EXISTS app_search_patterns(text);
DROP FUNCTION IF EXISTS app_search_words(text);
