-- calendars: an appointment's or interview invitation's notes, formatted
-- (bold, lists, links...), sent to Outlook as the event's HTML body. notes
-- keeps their plain text (shown in HyPlug, compared with Outlook's).
-- An interview invitation keeps the candidate as its one required attendee
-- and may have up to 20 optional ones (Outlook's Optional: the meeting's Cc).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE calendar_event ADD COLUMN notes_html text CHECK (notes_html IS NULL OR length(notes_html) <= 30000);

-- +goose StatementBegin
CREATE FUNCTION calendar_required_attendees(people jsonb) RETURNS integer
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE AS $$
  SELECT count(*)::integer FROM jsonb_array_elements(people) AS a
  WHERE NOT coalesce((a->>'optional')::boolean, false)
$$;
-- +goose StatementEnd
ALTER TABLE calendar_event DROP CONSTRAINT calendar_invitation_details,
  ADD CONSTRAINT calendar_invitation_details CHECK (source <> 'interview_invitation' OR
    (source_id IS NOT NULL AND invitation_calendar IS NOT NULL AND invitation_subject IS NOT NULL
     AND calendar_required_attendees(attendees) = 1 AND jsonb_array_length(attendees) <= 21));

-- +goose Down
UPDATE calendar_event SET attendees = (SELECT jsonb_agg(a) FROM jsonb_array_elements(attendees) a WHERE NOT coalesce((a->>'optional')::boolean, false))
  WHERE source = 'interview_invitation';
ALTER TABLE calendar_event DROP CONSTRAINT calendar_invitation_details,
  ADD CONSTRAINT calendar_invitation_details CHECK (source <> 'interview_invitation' OR
    (source_id IS NOT NULL AND invitation_calendar IS NOT NULL AND invitation_subject IS NOT NULL AND jsonb_array_length(attendees)=1));
DROP FUNCTION calendar_required_attendees(jsonb);
ALTER TABLE calendar_event DROP COLUMN notes_html;
