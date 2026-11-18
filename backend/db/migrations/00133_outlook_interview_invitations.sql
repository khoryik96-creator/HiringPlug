-- +goose Up
ALTER TABLE calendar_event DROP CONSTRAINT calendar_event_source_check;
ALTER TABLE calendar_event ADD CHECK (source IN ('local','interview','booking','outlook','interview_invitation'));
ALTER TABLE calendar_event
  ADD COLUMN invitation_calendar TEXT,
  ADD COLUMN invitation_subject TEXT,
  ADD COLUMN reminder_minutes integer NOT NULL DEFAULT 0 CHECK (reminder_minutes BETWEEN 0 AND 1440);
ALTER TABLE calendar_event ADD CONSTRAINT calendar_invitation_details CHECK (source <> 'interview_invitation' OR
  (source_id IS NOT NULL AND invitation_calendar IS NOT NULL AND invitation_subject IS NOT NULL AND jsonb_array_length(attendees)=1));
CREATE UNIQUE INDEX calendar_interview_invitation ON calendar_event(tenant_id,source_id) WHERE source='interview_invitation';

-- +goose Down
DROP INDEX calendar_interview_invitation;
DELETE FROM calendar_event WHERE source='interview_invitation';
ALTER TABLE calendar_event DROP CONSTRAINT calendar_invitation_details;
ALTER TABLE calendar_event DROP CONSTRAINT calendar_event_source_check;
ALTER TABLE calendar_event ADD CHECK (source IN ('local','interview','booking','outlook'));
ALTER TABLE calendar_event DROP COLUMN invitation_calendar, DROP COLUMN invitation_subject, DROP COLUMN reminder_minutes;
