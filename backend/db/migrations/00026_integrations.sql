-- integrations module: API keys for other systems, and webhooks that tell
-- them when something happens.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE api_key (
  id           text PRIMARY KEY DEFAULT new_id('akey') CHECK (id ~ '^akey_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id    text NOT NULL REFERENCES tenant (id),
  name         text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
  role         text NOT NULL CHECK (role IN ('admin', 'recruiter', 'sourcer', 'hiring_manager', 'viewer')),
  -- Only the SHA-256 of the key is kept, and its first characters to recognise it.
  key_hash     text NOT NULL UNIQUE CHECK (key_hash ~ '^[0-9a-f]{64}$'),
  prefix       text NOT NULL,
  created_by   text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz,
  revoked_at   timestamptz
);
CREATE INDEX api_key_tenant_idx ON api_key (tenant_id, id DESC);

ALTER TABLE api_key ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON api_key
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- The company and role behind an API key, before any company is known (so
-- outside row-level security, like app_careers_tenant): it returns nothing
-- for unknown or revoked keys, and notes when the key was last used (to the
-- minute).
-- +goose StatementBegin
CREATE FUNCTION app_api_key(p_hash text)
RETURNS TABLE (key_id text, tenant_id text, key_name text, key_role text)
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
  UPDATE api_key k SET last_used_at = now()
  WHERE k.key_hash = p_hash AND k.revoked_at IS NULL
    AND (k.last_used_at IS NULL OR k.last_used_at < now() - interval '1 minute');
  RETURN QUERY SELECT k.id, k.tenant_id, k.name, k.role FROM api_key k WHERE k.key_hash = p_hash AND k.revoked_at IS NULL;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION app_api_key(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_api_key(text) TO hiringplug_app;

CREATE TABLE webhook (
  id          text PRIMARY KEY DEFAULT new_id('whk') CHECK (id ~ '^whk_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id   text NOT NULL REFERENCES tenant (id),
  version     integer NOT NULL DEFAULT 1 CHECK (version >= 1),
  url         text NOT NULL CHECK (length(url) <= 500),
  events      text[] NOT NULL CHECK (cardinality(events) >= 1),
  secret      text NOT NULL,
  active      boolean NOT NULL DEFAULT true,
  description text CHECK (length(description) <= 200),
  created_by  text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);
CREATE INDEX webhook_active_idx ON webhook (tenant_id) WHERE active;

ALTER TABLE webhook ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON webhook
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

CREATE TABLE webhook_delivery (
  id              text PRIMARY KEY DEFAULT new_id('whd') CHECK (id ~ '^whd_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id       text NOT NULL,
  webhook_id      text NOT NULL,
  event           text NOT NULL,
  payload         jsonb NOT NULL,
  status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed')),
  attempts        integer NOT NULL DEFAULT 0,
  response_status integer,
  last_error      text CHECK (length(last_error) <= 1000),
  next_attempt_at timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  delivered_at    timestamptz,
  FOREIGN KEY (tenant_id, webhook_id) REFERENCES webhook (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX webhook_delivery_idx ON webhook_delivery (tenant_id, webhook_id, id DESC);

ALTER TABLE webhook_delivery ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON webhook_delivery
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- Every change is in the audit log; each one a webhook listens for becomes a
-- delivery, queued in the same transaction as the change.
-- +goose StatementBegin
CREATE FUNCTION webhook_fan_out() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  w record;
  delivery text;
BEGIN
  FOR w IN SELECT id FROM webhook WHERE tenant_id = NEW.tenant_id AND active AND NEW.action = ANY (events) LOOP
    delivery := new_id('whd');
    INSERT INTO webhook_delivery (id, tenant_id, webhook_id, event, payload)
    VALUES (delivery, NEW.tenant_id, w.id, NEW.action, jsonb_build_object(
      'id', delivery, 'event', NEW.action, 'occurredAt', NEW.created_at, 'actorId', NEW.actor_id,
      'entity', jsonb_build_object('type', NEW.entity_type, 'id', NEW.entity_id), 'changes', NEW.changes));
    PERFORM job_enqueue(NEW.tenant_id, 'core', 'webhook.deliver', jsonb_build_object('deliveryId', delivery), NEW.request_id);
  END LOOP;
  RETURN NULL;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER audit_log_webhooks AFTER INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION webhook_fan_out();

-- +goose Down
DROP TRIGGER IF EXISTS audit_log_webhooks ON audit_log;
DROP FUNCTION IF EXISTS webhook_fan_out();
DROP TABLE IF EXISTS webhook_delivery;
DROP TABLE IF EXISTS webhook;
DROP FUNCTION IF EXISTS app_api_key(text);
DROP TABLE IF EXISTS api_key;
