-- user_preference.dashboard_blocks: the order a person put their dashboard's
-- blocks in (dragged); NULL: the default order.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE user_preference ADD COLUMN dashboard_blocks text[] CHECK (cardinality(dashboard_blocks) <= 20);

-- +goose Down
ALTER TABLE user_preference DROP COLUMN IF EXISTS dashboard_blocks;
