-- identity module (1.46.0): sign in with Microsoft (Entra ID).
-- tenant: whether the company signs in with Microsoft, its own Microsoft
--   organisation (Entra tenant ID; never personal accounts), and whether it
--   requires Microsoft (password sign-in and resets then only for owners).
--   microsoft_acct_missing: the last Microsoft sign-in through the company's
--   own app came without the optional acct claim (member or guest), so no
--   account was linked by its address (Settings warns: the claim is set on
--   that app).
-- user_microsoft: the Microsoft account (organisation + object ID) linked to
--   a person, and how (linked_by): by their first sign-in (never for owners),
--   their invitation, from Your account, or a reconnect link an owner sent;
--   later sign-ins match it, whatever their email.
-- app_user.microsoft_sessions_after: sessions signed in with Microsoft before
--   it no longer work (the link was removed, or the organisation changed).
-- app_user.microsoft_connect_only: their Microsoft account was unlinked, or
--   they were made an owner, so one is linked again only on purpose (Your account, an invitation or a
--   reconnect link), never by a sign-in's address.
-- user_link kind reconnect: an owner's link for someone to link their
--   Microsoft account again, whatever its address (7 days, once).
-- microsoft_sign_in_flow: a sign-in under way (10 minutes at most). It is
--   started before anyone is signed in, often before the company is known, so
--   the app never reads it directly: only through the app_microsoft_flow_*
--   functions below, by the SHA-256 of the state or of a one-time ticket.
--   A sign-in started from an invitation or reconnect link keeps that link
--   (link_id, link_kind): it finishes only while that very link is unused.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE tenant
  ADD COLUMN microsoft_sign_in boolean NOT NULL DEFAULT false,
  ADD COLUMN microsoft_tenant_id text
    CHECK (microsoft_tenant_id ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
           AND microsoft_tenant_id <> '9188040d-6c67-4c5b-b112-36a304b66dad'),
  ADD COLUMN microsoft_required boolean NOT NULL DEFAULT false,
  ADD COLUMN microsoft_acct_missing boolean NOT NULL DEFAULT false,
  ADD CONSTRAINT tenant_microsoft_sign_in_check
    CHECK ((NOT microsoft_sign_in OR microsoft_tenant_id IS NOT NULL) AND (NOT microsoft_required OR microsoft_sign_in));

ALTER TABLE app_user ADD COLUMN microsoft_sessions_after timestamptz,
  ADD COLUMN microsoft_connect_only boolean NOT NULL DEFAULT false;

ALTER TABLE user_link DROP CONSTRAINT user_link_kind_check,
  ADD CONSTRAINT user_link_kind_check CHECK (kind IN ('invite', 'reset', 'reconnect'));

CREATE TABLE user_microsoft (
  tenant_id     text NOT NULL REFERENCES tenant (id),
  user_id       text NOT NULL,
  ms_tenant_id  text NOT NULL CHECK (ms_tenant_id ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'),
  ms_object_id  text NOT NULL CHECK (ms_object_id ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'),
  linked_by     text NOT NULL CHECK (linked_by IN ('sign_in', 'invitation', 'self', 'reconnect_link')),
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
  -- sign_in, or connect: a signed-in person (user_id) connecting their Microsoft account.
  purpose      text NOT NULL DEFAULT 'sign_in' CHECK (purpose IN ('sign_in', 'connect')),
  -- The person it is for: the one connecting, or the one whose invitation it accepts.
  user_id      text,
  -- The invitation or reconnect link it was started from (it is used when
  -- the sign-in finishes, and only if it is still unused, unexpired and theirs).
  link_id      text REFERENCES user_link (id) ON DELETE CASCADE,
  link_kind    text CHECK (link_kind IN ('invite', 'reconnect')),
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
  CHECK (expires_at <= created_at + interval '15 minutes'),
  CHECK (purpose = 'sign_in' OR (tenant_id IS NOT NULL AND user_id IS NOT NULL)),
  CHECK ((link_id IS NULL) = (link_kind IS NULL)),
  CHECK (link_id IS NULL OR (purpose = 'sign_in' AND tenant_id IS NOT NULL AND user_id IS NOT NULL)),
  FOREIGN KEY (tenant_id, user_id) REFERENCES app_user (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX microsoft_sign_in_flow_expires_idx ON microsoft_sign_in_flow (expires_at);
-- Never read or written by the app directly (no policy lets it), only through
-- the functions below.
ALTER TABLE microsoft_sign_in_flow ENABLE ROW LEVEL SECURITY;
CREATE POLICY no_direct_access ON microsoft_sign_in_flow USING (false) WITH CHECK (false);
REVOKE ALL ON microsoft_sign_in_flow FROM hiringplug_app;

-- A sign-in started (old ones are cleared on the way), for p_seconds by the
-- database's clock (the one every check of it uses).
-- +goose StatementBegin
CREATE FUNCTION app_microsoft_flow_save(p_key_hash text, p_binding_hash text, p_tenant_id text, p_purpose text, p_user_id text,
  p_link_id text, p_link_kind text, p_client_id text, p_authority text, p_sealed text, p_verified boolean, p_ticket_hash text, p_seconds integer)
RETURNS void
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
  DELETE FROM microsoft_sign_in_flow WHERE expires_at < now();
  INSERT INTO microsoft_sign_in_flow (key_hash, binding_hash, tenant_id, purpose, user_id, link_id, link_kind, client_id, authority, sealed,
                                      verified, ticket_hash, expires_at)
  VALUES (p_key_hash, p_binding_hash, p_tenant_id, p_purpose, p_user_id, p_link_id, p_link_kind, p_client_id, p_authority, p_sealed, p_verified, p_ticket_hash,
          now() + make_interval(secs => p_seconds));
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
RETURNS TABLE (key_hash text, binding_hash text, tenant_id text, purpose text, user_id text, link_id text, link_kind text,
               client_id text, authority text, sealed text, verified boolean, sealed_code text)
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  DELETE FROM microsoft_sign_in_flow f WHERE f.ticket_hash = p_ticket_hash AND f.expires_at > now()
  RETURNING f.key_hash, f.binding_hash, f.tenant_id, f.purpose, f.user_id, f.link_id, f.link_kind, f.client_id, f.authority, f.sealed,
            f.verified, f.sealed_code
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

-- Whom a Microsoft account that Microsoft has just vouched for may sign in
-- as, at every company of its organisation that signs in with Microsoft: the
-- person it was linked to, or else (not linked yet) the person on the team
-- with one of its addresses (p_addresses: its sign-in name, and its email
-- only when Microsoft says the email's domain is verified; none when
-- Microsoft does not say it is a member) who has no Microsoft account of
-- this organisation linked (a link to another organisation is left over
-- from before the company changed it). Only those active, or invited while
-- their invitation is valid (an invitation link unused and unexpired).
-- linked says which; is_owner and connect_only, because an account is never
-- linked to an owner, or to someone whose account was unlinked, by its
-- address alone (the caller decides).
-- +goose StatementBegin
CREATE FUNCTION app_microsoft_accounts(p_ms_tenant_id text, p_ms_object_id text, p_addresses text[])
RETURNS TABLE (tenant_id text, tenant_name text, careers_slug text, user_id text, linked boolean, is_owner boolean, connect_only boolean)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  WITH t AS (
    SELECT id, name, careers_slug FROM tenant WHERE microsoft_sign_in AND microsoft_tenant_id = p_ms_tenant_id),
  by_link AS (
    SELECT m.tenant_id, m.user_id, true AS linked FROM user_microsoft m JOIN t ON t.id = m.tenant_id
    WHERE m.ms_tenant_id = p_ms_tenant_id AND m.ms_object_id = p_ms_object_id),
  by_address AS (
    SELECT u.tenant_id, u.id AS user_id, false AS linked FROM app_user u JOIN t ON t.id = u.tenant_id
    WHERE u.email = ANY (p_addresses)
      AND NOT EXISTS (SELECT 1 FROM user_microsoft m
                      WHERE m.tenant_id = u.tenant_id AND m.user_id = u.id AND m.ms_tenant_id = p_ms_tenant_id)
      AND NOT EXISTS (SELECT 1 FROM by_link l WHERE l.tenant_id = u.tenant_id))
  SELECT t.id, t.name, t.careers_slug, u.id, f.linked, u.role = 'owner', u.microsoft_connect_only
  FROM (SELECT * FROM by_link UNION ALL SELECT * FROM by_address) f
  JOIN t ON t.id = f.tenant_id
  JOIN app_user u ON u.tenant_id = f.tenant_id AND u.id = f.user_id
  WHERE u.active
     OR (u.invited AND EXISTS (SELECT 1 FROM user_link l
                               WHERE l.tenant_id = u.tenant_id AND l.user_id = u.id AND l.kind = 'invite'
                                 AND l.used_at IS NULL AND l.expires_at > now()))
  ORDER BY t.name, t.id, u.id
  LIMIT 50
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION app_microsoft_flow_save(text, text, text, text, text, text, text, text, text, text, boolean, text, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_microsoft_flow_park(text, text, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_microsoft_flow_take(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_microsoft_sign_in_tenants(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_microsoft_accounts(text, text, text[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_microsoft_flow_save(text, text, text, text, text, text, text, text, text, text, boolean, text, integer) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION app_microsoft_flow_park(text, text, text) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION app_microsoft_flow_take(text) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION app_microsoft_sign_in_tenants(text) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION app_microsoft_accounts(text, text, text[]) TO hiringplug_app;

-- +goose Down
DROP FUNCTION IF EXISTS app_microsoft_accounts(text, text, text[]);
DROP FUNCTION IF EXISTS app_microsoft_sign_in_tenants(text);
DROP FUNCTION IF EXISTS app_microsoft_flow_take(text);
DROP FUNCTION IF EXISTS app_microsoft_flow_park(text, text, text);
DROP FUNCTION IF EXISTS app_microsoft_flow_save(text, text, text, text, text, text, text, text, text, text, boolean, text, integer);
DROP TABLE IF EXISTS microsoft_sign_in_flow;
DROP TABLE IF EXISTS user_microsoft;
DELETE FROM user_link WHERE kind = 'reconnect';
ALTER TABLE user_link DROP CONSTRAINT user_link_kind_check,
  ADD CONSTRAINT user_link_kind_check CHECK (kind IN ('invite', 'reset'));
ALTER TABLE app_user DROP COLUMN IF EXISTS microsoft_connect_only, DROP COLUMN IF EXISTS microsoft_sessions_after;
ALTER TABLE tenant DROP CONSTRAINT IF EXISTS tenant_microsoft_sign_in_check,
  DROP COLUMN IF EXISTS microsoft_acct_missing,
  DROP COLUMN IF EXISTS microsoft_required, DROP COLUMN IF EXISTS microsoft_tenant_id, DROP COLUMN IF EXISTS microsoft_sign_in;
