-- calendars and interviews: changes the organizer makes in Outlook to an
-- interview invitation HiringPlug sent (people, title, place, notes, time)
-- are taken into HiringPlug.
-- candidate_email pins the invitation's candidate, so people added in Outlook
-- (even as required attendees) are never taken for the candidate. It is
-- deleted with the invitation row, which is deleted with the candidate. An
-- invitation with a pinned candidate may have other required attendees (and
-- up to Outlook's 100 a sync reads); one without keeps the old rule.
-- outlook_body marks an event whose message was rewritten in Outlook: a later
-- HiringPlug change does not send HiringPlug's copy of the message over it.
-- outlook_changed_at is when HiringPlug last took such a change.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE calendar_event ADD COLUMN candidate_email text,
    ADD COLUMN outlook_body boolean NOT NULL DEFAULT false,
    ADD COLUMN outlook_changed_at timestamptz;
UPDATE calendar_event e SET candidate_email = r.email FROM (
    SELECT ce.id, min(a->>'email') AS email
    FROM calendar_event ce, jsonb_array_elements(ce.attendees) a
    WHERE ce.source = 'interview_invitation' AND NOT coalesce((a->>'optional')::boolean, false)
    GROUP BY ce.id HAVING count(*) = 1) r
WHERE e.id = r.id;
ALTER TABLE calendar_event DROP CONSTRAINT calendar_invitation_details,
  ADD CONSTRAINT calendar_invitation_details CHECK (source <> 'interview_invitation' OR
    (source_id IS NOT NULL AND invitation_calendar IS NOT NULL AND invitation_subject IS NOT NULL
     AND jsonb_array_length(attendees) <= 100
     AND (candidate_email IS NOT NULL OR calendar_required_attendees(attendees) = 1)));
ALTER TABLE interview ADD COLUMN outlook_changed_at timestamptz;

-- +goose Down
-- People added in Outlook are dropped (the candidate and the Cc stay); an
-- invitation the old rule cannot hold is deleted.
UPDATE calendar_event SET attendees = coalesce((SELECT jsonb_agg(a) FROM jsonb_array_elements(attendees) a
    WHERE coalesce((a->>'optional')::boolean, false) OR lower(a->>'email') = lower(candidate_email)), '[]'::jsonb)
  WHERE source = 'interview_invitation' AND candidate_email IS NOT NULL;
DELETE FROM calendar_event WHERE source = 'interview_invitation'
  AND (calendar_required_attendees(attendees) <> 1 OR jsonb_array_length(attendees) > 21);
ALTER TABLE calendar_event DROP CONSTRAINT calendar_invitation_details,
  ADD CONSTRAINT calendar_invitation_details CHECK (source <> 'interview_invitation' OR
    (source_id IS NOT NULL AND invitation_calendar IS NOT NULL AND invitation_subject IS NOT NULL
     AND calendar_required_attendees(attendees) = 1 AND jsonb_array_length(attendees) <= 21));
ALTER TABLE interview DROP COLUMN outlook_changed_at;
ALTER TABLE calendar_event DROP COLUMN outlook_changed_at, DROP COLUMN outlook_body, DROP COLUMN candidate_email;
