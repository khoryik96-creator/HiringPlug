-- +goose Up
ALTER TABLE calendar_event
  ADD COLUMN candidate_response text CHECK (candidate_response IN ('not_responded','accepted','tentative','declined')),
  ADD COLUMN candidate_reminder boolean NOT NULL DEFAULT false,
  ADD COLUMN candidate_reminder_version integer NOT NULL DEFAULT 1,
  ADD COLUMN candidate_reminder_queued_for timestamptz,
  ADD COLUMN candidate_reminder_sent_for timestamptz;

-- +goose Down
ALTER TABLE calendar_event
  DROP COLUMN candidate_response,
  DROP COLUMN candidate_reminder,
  DROP COLUMN candidate_reminder_version,
  DROP COLUMN candidate_reminder_queued_for,
  DROP COLUMN candidate_reminder_sent_for;
