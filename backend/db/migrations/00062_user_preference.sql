-- identity: each person's own settings, such as which columns their
-- candidate list shows. Only that person reads or changes them.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE user_preference (
  tenant_id         text NOT NULL REFERENCES tenant(id),
  user_id           text NOT NULL REFERENCES app_user(id),
  candidate_columns text[],          -- NULL: the default columns
  updated_at        timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id)
);

ALTER TABLE user_preference ENABLE ROW LEVEL SECURITY;
CREATE POLICY own_preferences ON user_preference
  USING (tenant_id = app_current_tenant() AND user_id = app_current_user())
  WITH CHECK (tenant_id = app_current_tenant() AND user_id = app_current_user());

-- +goose Down
DROP TABLE IF EXISTS user_preference;
