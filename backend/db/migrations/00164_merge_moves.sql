-- companies, contacts, notes, emails: merging duplicates moves every record,
-- even those the person merging may not see (sensitive notes, emails with
-- salaries or fees, other people's Outlook threads and Apollo lookups).
-- Row-level security would leave those on the deleted duplicate, orphaned
-- or deleted with it; these functions move them inside the agency, for
-- someone allowed to merge, without showing what they say. And a company
-- website on a shared host (facebook.com/acme, linktr.ee/acme) keeps its
-- page in its duplicate key, so unrelated companies are not "the same
-- website".
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION app_move_subject_notes(p_subject_type text, p_from text, p_to text) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $$
BEGIN
  IF p_subject_type NOT IN ('company', 'contact') OR NOT app_has_permission(
       CASE p_subject_type WHEN 'company' THEN 'companies:merge' ELSE 'contacts:merge' END) THEN
    RAISE EXCEPTION 'not allowed to merge %', p_subject_type USING ERRCODE = '42501';
  END IF;
  UPDATE note SET subject_id = p_to
  WHERE tenant_id = app_current_tenant() AND subject_type = p_subject_type AND subject_id = p_from;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION app_move_contact_emails(p_from text, p_to text) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $$
BEGIN
  IF NOT app_has_permission('contacts:merge') THEN
    RAISE EXCEPTION 'not allowed to merge contacts' USING ERRCODE = '42501';
  END IF;
  UPDATE email SET contact_id = p_to WHERE tenant_id = app_current_tenant() AND contact_id = p_from;
  UPDATE email_outlook_link SET contact_id = p_to WHERE tenant_id = app_current_tenant() AND contact_id = p_from;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION app_move_company_apollo_lookups(p_from text, p_to text) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $$
BEGIN
  IF NOT app_has_permission('companies:merge') THEN
    RAISE EXCEPTION 'not allowed to merge companies' USING ERRCODE = '42501';
  END IF;
  UPDATE hr_apollo_lookup SET company_id = p_to WHERE tenant_id = app_current_tenant() AND company_id = p_from;
END $$;
-- +goose StatementEnd

-- A page on a shared host is its own site: facebook.com/acme,
-- linkedin.com/company/acme, linktr.ee/acme.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app_site_key(value text) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT CASE
    WHEN host IN ('facebook.com', 'fb.com', 'm.facebook.com', 'instagram.com', 'linktr.ee', 'tiktok.com', 'twitter.com', 'x.com',
                  'youtube.com', 'sites.google.com', 'google.com', 'business.google.com', 'wa.me', 'medium.com', 'github.com')
      THEN nullif(host || '/' || coalesce(nullif(split_part(path, '/', 1), ''), ''), host || '/')
    WHEN host = 'linkedin.com'
      THEN nullif(host || '/' || split_part(path, '/', 1) || '/' || split_part(path, '/', 2), host || '//')
    ELSE host
  END
  FROM (SELECT nullif(regexp_replace(rest, '[/?#:].*$', ''), '') AS host,
               regexp_replace(regexp_replace(rest, '^[^/?#]*/?', ''), '[?#].*$', '') AS path
        FROM (SELECT regexp_replace(lower(btrim(coalesce(value, ''))), '^[a-z]+://(www\d*\.)?', '') AS rest) r) s
$$;
-- +goose StatementEnd

-- The stored keys are worked out again with the new rule.
DROP INDEX company_site_key_idx;
ALTER TABLE company DROP COLUMN site_key;
ALTER TABLE company ADD COLUMN site_key text GENERATED ALWAYS AS (app_site_key(website)) STORED;
CREATE INDEX company_site_key_idx ON company (tenant_id, site_key) WHERE site_key IS NOT NULL;

-- +goose Down
DROP FUNCTION IF EXISTS app_move_company_apollo_lookups(text, text);
DROP FUNCTION IF EXISTS app_move_contact_emails(text, text);
DROP FUNCTION IF EXISTS app_move_subject_notes(text, text, text);
