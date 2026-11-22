-- emails: WhatsApp message templates (filled like email templates, opened in
-- WhatsApp with a chat link); notes: a WhatsApp message sent is logged as a
-- note of its own kind.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE whatsapp_template (
  id         text PRIMARY KEY DEFAULT new_id('wtpl') CHECK (id ~ '^wtpl_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id  text NOT NULL REFERENCES tenant (id),
  version    integer NOT NULL DEFAULT 1,
  name       text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
  body       text NOT NULL CHECK (length(body) BETWEEN 1 AND 4000),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX whatsapp_template_name_idx ON whatsapp_template (tenant_id, lower(name));

ALTER TABLE whatsapp_template ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON whatsapp_template
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE note DROP CONSTRAINT IF EXISTS note_kind_check;
ALTER TABLE note ADD CONSTRAINT note_kind_check
  CHECK (kind IN ('general', 'screening', 'interview', 'call', 'reference', 'whatsapp'));

-- +goose Down
UPDATE note SET kind = 'call' WHERE kind = 'whatsapp'; -- kept, as calls
ALTER TABLE note DROP CONSTRAINT IF EXISTS note_kind_check;
ALTER TABLE note ADD CONSTRAINT note_kind_check
  CHECK (kind IN ('general', 'screening', 'interview', 'call', 'reference'));
DROP TABLE IF EXISTS whatsapp_template;
