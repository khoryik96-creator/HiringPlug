-- companies, contacts: keys that spot possible duplicates, so two records of
-- the same company or person can be found and merged. A company's name
-- without its legal form ("Acme Sdn Bhd" and "ACME Berhad" are both "acme")
-- and its website's address ("https://www.acme.com.my/about" is
-- "acme.com.my"), and its LinkedIn page however it is written.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION app_company_name_core(value text) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT coalesce(string_agg(w, ' ' ORDER BY n), '')
  FROM regexp_split_to_table(
         regexp_replace(public.app_normalize_text(regexp_replace(coalesce(value, ''), '\([^)]*\)', ' ', 'g')), '[^[:alnum:]]+', ' ', 'g'),
         ' ') WITH ORDINALITY AS t(w, n)
  WHERE w <> '' AND w <> ALL (ARRAY['sdn', 'bhd', 'berhad', 'sendirian', 'pte', 'ltd', 'limited', 'plc', 'inc', 'incorporated',
    'corp', 'corporation', 'co', 'llc', 'llp', 'gmbh', 'bv', 'sa', 'ag', 'tbk', 'pt', 'the'])
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION app_site_key(value text) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT nullif(regexp_replace(regexp_replace(lower(btrim(coalesce(value, ''))), '^[a-z]+://(www\d*\.)?', ''), '[/?#:].*$', ''), '')
$$;
-- +goose StatementEnd

ALTER TABLE company
  ADD COLUMN name_core text GENERATED ALWAYS AS (app_company_name_core(name)) STORED,
  ADD COLUMN site_key text GENERATED ALWAYS AS (app_site_key(website)) STORED;
CREATE INDEX company_name_core_idx ON company (tenant_id, name_core);
CREATE INDEX company_site_key_idx ON company (tenant_id, site_key) WHERE site_key IS NOT NULL;
-- As contact_linkedin_key_idx (00160) does for contacts.
CREATE INDEX company_linkedin_key_idx ON company (tenant_id, lower(rtrim(linkedin_url, '/'))) WHERE linkedin_url IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS company_linkedin_key_idx;
DROP INDEX IF EXISTS company_site_key_idx;
DROP INDEX IF EXISTS company_name_core_idx;
ALTER TABLE company DROP COLUMN IF EXISTS site_key, DROP COLUMN IF EXISTS name_core;
DROP FUNCTION IF EXISTS app_site_key(text);
DROP FUNCTION IF EXISTS app_company_name_core(text);
