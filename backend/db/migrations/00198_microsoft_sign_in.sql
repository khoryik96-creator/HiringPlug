-- identity module (1.46.0): sign in with Microsoft (Entra ID).
-- tenant: whether the company signs in with Microsoft, its own Microsoft
--   organisation (Entra tenant ID; never personal accounts), and whether it
--   requires Microsoft (password sign-in and resets then only for owners).
-- user_microsoft: the Microsoft account (organisation + object ID) a person
--   signed in with first; later sign-ins match it, whatever their email.
-- microsoft_sign_in_flow: a sign-in under way (10 minutes at most). It is
--   started before anyone is signed in, often before the company is known, so
--   the app never reads it directly: only through the app_microsoft_flow_*
--   functions below, by the SHA-256 of the state or of a one-time ticket.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE tenant
  ADD COLUMN microsoft_sign_in boolean NOT NULL DEFAULT false,
  ADD COLUMN microsoft_tenant_id text
    CHECK (microsoft_tenant_id ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
           AND microsoft_tenant_id <> '9188040d-6c67-4c5b-b112-36a304b66dad'),
  ADD COLUMN microsoft_required boolean NOT NULL DEFAULT false,
  ADD CONSTRAINT tenant_microsoft_sign_in_check
    CHECK ((NOT microsoft_sign_in OR microsoft_tenant_id IS NOT NULL) AND (NOT microsoft_required OR microsoft_sign_in));

CREATE TABLE user_microsoft (
  tenant_id     text NOT NULL REFERENCES tenant (id),
  user_id       text NOT NULL,
  ms_tenant_id  text NOT NULL CHECK (ms_tenant_id ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'),
  ms_object_id  text NOT NULL CHECK (ms_object_id ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'),
  linked_at     timestamptz NOT NULL DEFAULT now(),
  last_used_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id),
  UNIQUE (tenant_id, ms_tenant_id, ms_object_id),
  FOREIGN KEY (tenant_id, user_id) REFERENCES app_user (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX user_microsoft_account_idx ON user_microsoft (ms_tenant_id, ms_object_id);
ALTER TABLE user_microsoft ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON user_microsoft
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

CREATE TABLE microsoft_sign_in_flow (
  -- SHA-256 of the state sent to Microsoft (or, for a company choice, of a random key).
  key_hash     text PRIMARY KEY CHECK (key_hash ~ '^[0-9a-f]{64}$'),
  -- SHA-256 of the random value the starting browser tab keeps: only it can finish.
  binding_hash text NOT NULL CHECK (binding_hash ~ '^[0-9a-f]{64}$'),
  -- The company it was started for; NULL: any company of the account's organisation (the server's app).
  tenant_id    text REFERENCES tenant (id) ON DELETE CASCADE,
  client_id    text NOT NULL CHECK (length(client_id) BETWEEN 1 AND 200),
  authority    text NOT NULL CHECK (authority = 'organizations' OR authority ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'),
  -- Sealed (AES-GCM, key from HP_AUTH_JWT_SECRET): the PKCE verifier and nonce,
  -- or, once verified, the Microsoft account waiting for a company choice.
  sealed       text NOT NULL,
  verified     boolean NOT NULL DEFAULT false,
  -- Set once, by Microsoft's redirect: the sealed code and the one-time ticket's SHA-256.
  sealed_code  text,
  ticket_hash  text UNIQUE CHECK (ticket_hash ~ '^[0-9a-f]{64}$'),
  expires_at   timestamptz NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  CHECK (expires_at <= created_at + interval '15 minutes')
);
CREATE INDEX microsoft_sign_in_flow_expires_idx ON microsoft_sign_in_flow (expires_at);
-- Never read or written by the app directly (no policy lets it), only through
-- the functions below.
ALTER TABLE microsoft_sign_in_flow ENABLE ROW LEVEL SECURITY;
CREATE POLICY no_direct_access ON microsoft_sign_in_flow USING (false) WITH CHECK (false);
REVOKE ALL ON microsoft_sign_in_flow FROM hiringplug_app;

-- A sign-in started (old ones are cleared on the way).
-- +goose StatementBegin
CREATE FUNCTION app_microsoft_flow_save(p_key_hash text, p_binding_hash text, p_tenant_id text, p_client_id text,
  p_authority text, p_sealed text, p_verified boolean, p_ticket_hash text, p_expires_at timestamptz)
RETURNS void
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
  DELETE FROM microsoft_sign_in_flow WHERE expires_at < now();
  INSERT INTO microsoft_sign_in_flow (key_hash, binding_hash, tenant_id, client_id, authority, sealed, verified, ticket_hash, expires_at)
  VALUES (p_key_hash, p_binding_hash, p_tenant_id, p_client_id, p_authority, p_sealed, p_verified, p_ticket_hash, p_expires_at);
END
$$;
-- +goose StatementEnd

-- Microsoft's redirect parks the sealed code on its sign-in, once, with the
-- ticket's hash; 1 when it did.
-- +goose StatementBegin
CREATE FUNCTION app_microsoft_flow_park(p_key_hash text, p_sealed_code text, p_ticket_hash text)
RETURNS integer
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  WITH parked AS (
    UPDATE microsoft_sign_in_flow SET sealed_code = p_sealed_code, ticket_hash = p_ticket_hash
    WHERE key_hash = p_key_hash AND NOT verified AND ticket_hash IS NULL AND expires_at > now()
    RETURNING 1)
  SELECT count(*)::integer FROM parked
$$;
-- +goose StatementEnd

-- A ticket works once: the sign-in it names is removed as it is read.
-- +goose StatementBegin
CREATE FUNCTION app_microsoft_flow_take(p_ticket_hash text)
RETURNS TABLE (key_hash text, binding_hash text, tenant_id text, client_id text, authority text, sealed text, verified boolean, sealed_code text)
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  DELETE FROM microsoft_sign_in_flow f WHERE f.ticket_hash = p_ticket_hash AND f.expires_at > now()
  RETURNING f.key_hash, f.binding_hash, f.tenant_id, f.client_id, f.authority, f.sealed, f.verified, f.sealed_code
$$;
-- +goose StatementEnd

-- The companies that sign in with Microsoft: the one with a careers page
-- address, or (NULL) up to two of them (to tell whether there is just one).
-- +goose StatementBegin
CREATE FUNCTION app_microsoft_sign_in_tenants(p_slug text)
RETURNS TABLE (tenant_id text, microsoft_tenant_id text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT t.id, t.microsoft_tenant_id FROM tenant t
  WHERE t.microsoft_sign_in AND (p_slug IS NULL OR t.careers_slug = lower(p_slug))
  ORDER BY t.id LIMIT 2
$$;
-- +goose StatementEnd

-- The companies that sign in with Microsoft where someone with this email is
-- on the team (active or invited): where to send them before they sign in.
-- +goose StatementBegin
CREATE FUNCTION app_microsoft_tenants_by_email(p_email text)
RETURNS TABLE (tenant_id text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT DISTINCT u.tenant_id FROM app_user u JOIN tenant t ON t.id = u.tenant_id
  WHERE t.microsoft_sign_in AND u.email = lower(p_email) AND (u.active OR u.invited)
  ORDER BY u.tenant_id LIMIT 20
$$;
-- +goose StatementEnd

-- Whom a Microsoft account that Microsoft has just vouched for may sign in
-- as, at every company of its organisation that signs in with Microsoft: the
-- person it was linked to, or else (not linked yet) the person on the team
-- with one of its addresses who has no Microsoft account linked. Only those
-- active or invited. linked says which.
-- +goose StatementBegin
CREATE FUNCTION app_microsoft_accounts(p_ms_tenant_id text, p_ms_object_id text, p_emails text[])
RETURNS TABLE (tenant_id text, tenant_name text, careers_slug text, user_id text, linked boolean)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  WITH t AS (
    SELECT id, name, careers_slug FROM tenant WHERE microsoft_sign_in AND microsoft_tenant_id = p_ms_tenant_id),
  by_link AS (
    SELECT m.tenant_id, m.user_id, true AS linked FROM user_microsoft m JOIN t ON t.id = m.tenant_id
    WHERE m.ms_tenant_id = p_ms_tenant_id AND m.ms_object_id = p_ms_object_id),
  by_email AS (
    SELECT u.tenant_id, u.id AS user_id, false AS linked FROM app_user u JOIN t ON t.id = u.tenant_id
    WHERE u.email = ANY (p_emails)
      AND NOT EXISTS (SELECT 1 FROM user_microsoft m WHERE m.tenant_id = u.tenant_id AND m.user_id = u.id)
      AND NOT EXISTS (SELECT 1 FROM by_link l WHERE l.tenant_id = u.tenant_id))
  SELECT t.id, t.name, t.careers_slug, u.id, f.linked
  FROM (SELECT * FROM by_link UNION ALL SELECT * FROM by_email) f
  JOIN t ON t.id = f.tenant_id
  JOIN app_user u ON u.tenant_id = f.tenant_id AND u.id = f.user_id
  WHERE u.active OR u.invited
  ORDER BY t.name, t.id, u.id
  LIMIT 50
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION app_microsoft_flow_save(text, text, text, text, text, text, boolean, text, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_microsoft_flow_park(text, text, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_microsoft_flow_take(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_microsoft_sign_in_tenants(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_microsoft_tenants_by_email(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_microsoft_accounts(text, text, text[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_microsoft_flow_save(text, text, text, text, text, text, boolean, text, timestamptz) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION app_microsoft_flow_park(text, text, text) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION app_microsoft_flow_take(text) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION app_microsoft_sign_in_tenants(text) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION app_microsoft_tenants_by_email(text) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION app_microsoft_accounts(text, text, text[]) TO hiringplug_app;

-- +goose Down
DROP FUNCTION IF EXISTS app_microsoft_accounts(text, text, text[]);
DROP FUNCTION IF EXISTS app_microsoft_tenants_by_email(text);
DROP FUNCTION IF EXISTS app_microsoft_sign_in_tenants(text);
DROP FUNCTION IF EXISTS app_microsoft_flow_take(text);
DROP FUNCTION IF EXISTS app_microsoft_flow_park(text, text, text);
DROP FUNCTION IF EXISTS app_microsoft_flow_save(text, text, text, text, text, text, boolean, text, timestamptz);
DROP TABLE IF EXISTS microsoft_sign_in_flow;
DROP TABLE IF EXISTS user_microsoft;
ALTER TABLE tenant DROP CONSTRAINT IF EXISTS tenant_microsoft_sign_in_check,
  DROP COLUMN IF EXISTS microsoft_required, DROP COLUMN IF EXISTS microsoft_tenant_id, DROP COLUMN IF EXISTS microsoft_sign_in;
