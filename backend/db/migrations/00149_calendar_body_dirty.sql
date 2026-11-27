-- calendars: body_dirty marks an event whose notes HyPlug changed and has
-- not sent to Outlook yet, so a change of time alone never replaces the
-- event's body in Outlook (Outlook rewrites formatted bodies, so they cannot
-- be compared with what it holds).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE calendar_event ADD COLUMN body_dirty boolean NOT NULL DEFAULT true;
UPDATE calendar_event SET body_dirty = false WHERE remote_id IS NOT NULL AND version = sent_version;

-- +goose Down
ALTER TABLE calendar_event DROP COLUMN body_dirty;
