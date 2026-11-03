-- placements module: after a fall-through within the guarantee, the agency
-- owes the client a replacement (or a refund).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE placement ADD COLUMN replacement text NOT NULL DEFAULT 'none'
  CHECK (replacement IN ('none', 'owed', 'provided', 'refunded'));
UPDATE placement SET replacement = 'owed' WHERE status = 'fell_through';
ALTER TABLE placement ADD CONSTRAINT placement_replacement_after_fall_through
  CHECK ((status = 'fell_through') = (replacement <> 'none'));

-- +goose Down
ALTER TABLE placement DROP COLUMN IF EXISTS replacement;
