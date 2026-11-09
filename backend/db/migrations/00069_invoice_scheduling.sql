-- placements: when a placement's invoice email is queued for (scheduled_for),
--   so saving it again does not queue it again, and a changed hour moves it.
-- emails: an email that shows a fee (an invoice) is seen only with
--   placements.fees:read, the permission that guards fees.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE placement_invoice ADD COLUMN scheduled_for timestamptz;

ALTER TABLE email ADD COLUMN fees_restricted boolean NOT NULL DEFAULT false;
CREATE POLICY fees_scope ON email AS RESTRICTIVE FOR SELECT
  USING (NOT fees_restricted OR app_has_permission('placements.fees:read'));

-- +goose Down
DROP POLICY IF EXISTS fees_scope ON email;
ALTER TABLE email DROP COLUMN IF EXISTS fees_restricted;
ALTER TABLE placement_invoice DROP COLUMN IF EXISTS scheduled_for;
