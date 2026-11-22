-- +goose Up
-- Two-step sign-in: a code from an authenticator app after the password.
-- The app's secret is sealed (AES-GCM, with a key derived from the sign-in
-- secret); recovery codes are kept as hashes only.
CREATE TABLE user_two_step (
  tenant_id       text NOT NULL REFERENCES tenant (id),
  user_id         text NOT NULL,
  -- Set while the person is adding the app; replaced by secret_sealed once a code confirms it.
  pending_sealed  text,
  secret_sealed   text,
  enabled_at      timestamptz,
  -- The last 30-second step a code was accepted for: a code works once.
  last_step       bigint NOT NULL DEFAULT 0,
  recovery_hashes text[] NOT NULL DEFAULT '{}',
  updated_at      timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id),
  FOREIGN KEY (tenant_id, user_id) REFERENCES app_user (tenant_id, id) ON DELETE CASCADE,
  CHECK ((secret_sealed IS NULL) = (enabled_at IS NULL))
);
ALTER TABLE user_two_step ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON user_two_step
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- An owner can require it of everyone at the company.
ALTER TABLE tenant ADD COLUMN two_step_required boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE tenant DROP COLUMN two_step_required;
DROP TABLE user_two_step;
