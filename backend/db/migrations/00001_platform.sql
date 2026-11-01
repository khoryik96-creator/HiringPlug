-- Platform foundations used by every module: database roles, ID generation,
-- and the per-transaction settings that row-level security (RLS) reads.
--
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up

-- Group roles. Real login roles are members of these (see `hp dev-setup` for
-- local development; production creates its own logins).
--   hiringplug_app : the Go API and worker. Subject to RLS; never owns tables.
--   hiringplug_ai  : the Python AI worker. May only call the job queue functions.
-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hiringplug_app') THEN
    CREATE ROLE hiringplug_app NOLOGIN NOSUPERUSER NOBYPASSRLS;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hiringplug_ai') THEN
    CREATE ROLE hiringplug_ai NOLOGIN NOSUPERUSER NOBYPASSRLS;
  END IF;
END
$$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA public TO hiringplug_app, hiringplug_ai;

-- Every table created by later migrations is readable and writable by the app
-- role (RLS still decides which rows). The AI role gets nothing by default.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO hiringplug_app;

-- The app reads goose's migration table at startup to refuse running against
-- a database that is behind the code.
-- +goose StatementBegin
DO $$
BEGIN
  IF to_regclass('public.goose_db_version') IS NOT NULL THEN
    GRANT SELECT ON TABLE public.goose_db_version TO hiringplug_app;
  END IF;
END
$$;
-- +goose StatementEnd

CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;

-- new_id('cand') -> 'cand_01jbz6w0m0k8q9x2r3s4t5v6w7'
-- A type prefix plus a ULID: 48-bit millisecond timestamp + 80 random bits in
-- lower-case Crockford base32. IDs sort by creation time, and the prefix tells
-- you what an ID is when you see it in a log line.
-- +goose StatementBegin
CREATE FUNCTION new_id(prefix text) RETURNS text
LANGUAGE plpgsql VOLATILE PARALLEL SAFE AS $$
DECLARE
  alphabet constant text := '0123456789abcdefghjkmnpqrstvwxyz';
  millis bigint := floor(extract(epoch FROM clock_timestamp()) * 1000);
  rnd bytea := decode(replace(gen_random_uuid()::text, '-', ''), 'hex');
  bits bit(130);
  result text := '';
BEGIN
  IF prefix !~ '^[a-z]{2,8}$' THEN
    RAISE EXCEPTION 'new_id: invalid prefix %', prefix;
  END IF;
  -- Use the 10 fully random bytes of the v4 UUID (skip the version/variant bytes 6 and 8).
  rnd := substring(rnd FROM 1 FOR 6) || substring(rnd FROM 10 FOR 4);
  bits := B'00' || substring(millis::bit(64) FROM 17 FOR 48) || ('x' || encode(rnd, 'hex'))::bit(80);
  FOR i IN 0..25 LOOP
    result := result || substr(alphabet, substring(bits FROM i * 5 + 1 FOR 5)::int + 1, 1);
  END LOOP;
  RETURN prefix || '_' || result;
END
$$;
-- +goose StatementEnd

-- The Go app sets these at the start of every transaction (SET LOCAL semantics,
-- so they can never leak to the next request on a pooled connection):
--   app.tenant_id   : the only tenant whose rows are visible
--   app.user_id     : who is acting (for audit columns)
--   app.permissions : comma-separated permissions, for field-level policies
-- When a setting is missing these return NULL / false, so nothing is visible:
-- forgetting to set the tenant fails closed.
-- +goose StatementBegin
CREATE FUNCTION app_current_tenant() RETURNS text
LANGUAGE sql STABLE PARALLEL SAFE AS $$
  SELECT nullif(current_setting('app.tenant_id', true), '')
$$;

CREATE FUNCTION app_current_user() RETURNS text
LANGUAGE sql STABLE PARALLEL SAFE AS $$
  SELECT nullif(current_setting('app.user_id', true), '')
$$;

CREATE FUNCTION app_has_permission(permission text) RETURNS boolean
LANGUAGE sql STABLE PARALLEL SAFE AS $$
  SELECT coalesce(permission = ANY (string_to_array(nullif(current_setting('app.permissions', true), ''), ',')), false)
$$;

-- Lower-cases, strips accents and squeezes spaces, so "José  Tan" matches "jose tan".
CREATE FUNCTION app_normalize_text(value text) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT btrim(regexp_replace(lower(public.unaccent('public.unaccent'::regdictionary, coalesce(value, ''))), '\s+', ' ', 'g'))
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS app_normalize_text(text);
DROP FUNCTION IF EXISTS app_has_permission(text);
DROP FUNCTION IF EXISTS app_current_user();
DROP FUNCTION IF EXISTS app_current_tenant();
DROP FUNCTION IF EXISTS new_id(text);
