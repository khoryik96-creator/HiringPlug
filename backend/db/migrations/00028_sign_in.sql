-- identity module: passwords, invitations and password resets.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE app_user
  ADD COLUMN password_hash text,
  -- Has not chosen a password yet (deactivated until then).
  ADD COLUMN invited boolean NOT NULL DEFAULT false,
  -- Sign-in tokens issued before this (to the second) are refused: set when
  -- the password changes or the user is deactivated.
  ADD COLUMN tokens_valid_after timestamptz,
  ADD COLUMN last_sign_in_at timestamptz;

-- Invitation and password reset links: only their SHA-256 is kept.
CREATE TABLE user_link (
  id         text PRIMARY KEY DEFAULT new_id('ulnk') CHECK (id ~ '^ulnk_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id  text NOT NULL,
  user_id    text NOT NULL,
  kind       text NOT NULL CHECK (kind IN ('invite', 'reset')),
  token_hash text NOT NULL UNIQUE CHECK (token_hash ~ '^[0-9a-f]{64}$'),
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, user_id) REFERENCES app_user (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX user_link_user_idx ON user_link (tenant_id, user_id);

ALTER TABLE user_link ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON user_link
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- Before anyone is signed in, no company is known: these return only what
-- signing in and links need, outside row-level security (like
-- app_careers_tenant).

-- The accounts with an email, at every company (to sign in or send resets).
-- +goose StatementBegin
CREATE FUNCTION app_accounts_by_email(p_email text)
RETURNS TABLE (tenant_id text, tenant_name text, careers_slug text, user_id text, user_name text,
               password_hash text, active boolean, tokens_valid_after timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT u.tenant_id, t.name, t.careers_slug::text, u.id, u.name, u.password_hash, u.active, u.tokens_valid_after
  FROM app_user u JOIN tenant t ON t.id = u.tenant_id
  WHERE u.email = lower(p_email)
  ORDER BY t.name
$$;
-- +goose StatementEnd

-- The company and user behind a link, if it is unused and not expired.
-- +goose StatementBegin
CREATE FUNCTION app_user_link(p_hash text)
RETURNS TABLE (link_id text, tenant_id text, user_id text, kind text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT l.id, l.tenant_id, l.user_id, l.kind FROM user_link l
  WHERE l.token_hash = p_hash AND l.used_at IS NULL AND l.expires_at > now()
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION app_accounts_by_email(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_user_link(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_accounts_by_email(text) TO hiringplug_app;
GRANT EXECUTE ON FUNCTION app_user_link(text) TO hiringplug_app;

-- +goose Down
DROP FUNCTION IF EXISTS app_user_link(text);
DROP FUNCTION IF EXISTS app_accounts_by_email(text);
DROP TABLE IF EXISTS user_link;
ALTER TABLE app_user DROP COLUMN IF EXISTS password_hash, DROP COLUMN IF EXISTS invited,
  DROP COLUMN IF EXISTS tokens_valid_after, DROP COLUMN IF EXISTS last_sign_in_at;
