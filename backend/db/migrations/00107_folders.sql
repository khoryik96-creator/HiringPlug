-- folders module: folders of candidates, contacts or companies. Each person
-- makes their own and may share one with others of the same agency, who can
-- then add and take out records; only the owner renames, deletes or shares.
-- folder_item holds record ids only (the records belong to their modules).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE folder (
  id         text PRIMARY KEY DEFAULT new_id('fld') CHECK (id ~ '^fld_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id  text NOT NULL REFERENCES tenant (id),
  kind       text NOT NULL CHECK (kind IN ('candidate', 'contact', 'company')),
  name       text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
  owner_id   text NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX folder_owner_idx ON folder (tenant_id, owner_id, kind);

CREATE TABLE folder_share (
  folder_id  text NOT NULL REFERENCES folder (id) ON DELETE CASCADE,
  tenant_id  text NOT NULL REFERENCES tenant (id),
  user_id    text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (folder_id, user_id)
);
CREATE INDEX folder_share_user_idx ON folder_share (tenant_id, user_id);

CREATE TABLE folder_item (
  folder_id text NOT NULL REFERENCES folder (id) ON DELETE CASCADE,
  tenant_id text NOT NULL REFERENCES tenant (id),
  item_id   text NOT NULL,
  added_by  text NOT NULL,
  added_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (folder_id, item_id)
);
CREATE INDEX folder_item_item_idx ON folder_item (tenant_id, item_id);

-- Every table holding tenant data has row-level security (a test checks this).
ALTER TABLE folder ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON folder
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE folder_share ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON folder_share
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE folder_item ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON folder_item
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS folder_item;
DROP TABLE IF EXISTS folder_share;
DROP TABLE IF EXISTS folder;
