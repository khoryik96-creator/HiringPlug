-- careers: each company's public careers page address ("contoh-recruitment").
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE tenant ADD COLUMN careers_slug text UNIQUE
  CHECK (careers_slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(careers_slug) BETWEEN 3 AND 60);

-- A free address made from a name: "Contoh Recruitment Sdn. Bhd." -> "contoh-recruitment-sdn-bhd",
-- then "-2", "-3" ... if taken. SECURITY DEFINER: it must see every company's address.
-- +goose StatementBegin
CREATE FUNCTION app_unique_careers_slug(p_name text) RETURNS text
  LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  base text := left(trim(both '-' from regexp_replace(lower(p_name), '[^a-z0-9]+', '-', 'g')), 50);
  candidate text;
  n integer := 1;
BEGIN
  IF length(base) < 3 THEN
    base := 'company';
  END IF;
  candidate := base;
  WHILE EXISTS (SELECT 1 FROM tenant WHERE careers_slug = candidate) LOOP
    n := n + 1;
    candidate := base || '-' || n;
  END LOOP;
  RETURN candidate;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION app_unique_careers_slug(text) FROM PUBLIC;

-- +goose StatementBegin
CREATE FUNCTION app_default_careers_slug() RETURNS trigger
  LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
  IF NEW.careers_slug IS NULL THEN
    NEW.careers_slug := app_unique_careers_slug(NEW.name);
  END IF;
  RETURN NEW;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER tenant_careers_slug BEFORE INSERT ON tenant FOR EACH ROW EXECUTE FUNCTION app_default_careers_slug();

-- Existing companies, one at a time so each sees the addresses given before it.
-- +goose StatementBegin
DO $$
DECLARE t record;
BEGIN
  FOR t IN SELECT id, name FROM tenant WHERE careers_slug IS NULL ORDER BY created_at LOOP
    UPDATE tenant SET careers_slug = app_unique_careers_slug(t.name) WHERE id = t.id;
  END LOOP;
END
$$;
-- +goose StatementEnd

-- The public careers page knows only the address. This turns it into the
-- company, and returns nothing else; everything after runs in that company's
-- tenant scope, where row-level security applies as usual.
CREATE FUNCTION app_careers_tenant(p_slug text) RETURNS TABLE (tenant_id text, tenant_name text)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
    SELECT id, name FROM tenant WHERE careers_slug = lower(p_slug)
$$;
REVOKE ALL ON FUNCTION app_careers_tenant(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_careers_tenant(text) TO hiringplug_app;

-- +goose Down
DROP FUNCTION IF EXISTS app_careers_tenant(text);
DROP TRIGGER IF EXISTS tenant_careers_slug ON tenant;
DROP FUNCTION IF EXISTS app_default_careers_slug();
DROP FUNCTION IF EXISTS app_unique_careers_slug(text);
ALTER TABLE tenant DROP COLUMN IF EXISTS careers_slug;
